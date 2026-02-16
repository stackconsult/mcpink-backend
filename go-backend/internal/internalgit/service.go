package internalgit

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"math/big"
	"net/url"
	"time"

	"github.com/augustdev/autoclip/internal/storage/pg"
	"github.com/augustdev/autoclip/internal/storage/pg/generated/gittokens"
	"github.com/augustdev/autoclip/internal/storage/pg/generated/internalrepos"
	"github.com/augustdev/autoclip/internal/storage/pg/generated/users"
	"github.com/jackc/pgx/v5/pgtype"
)

const (
	tokenPrefix    = "mlg_"
	slugCharset    = "abcdefghijklmnopqrstuvwxyz0123456789"
	slugLen        = 4
	maxSlugRetries = 5
)

type Service struct {
	config      Config
	repoQueries internalrepos.Querier
	tokenQ      gittokens.Querier
	userQueries users.Querier
}

func NewService(config Config, db *pg.DB) (*Service, error) {
	if config.PublicGitURL == "" {
		return nil, fmt.Errorf("internalgit: PublicGitURL is required")
	}

	return &Service{
		config:      config,
		repoQueries: internalrepos.New(db.Pool),
		tokenQ:      gittokens.New(db.Pool),
		userQueries: users.New(db.Pool),
	}, nil
}

// CreateRepo creates a new internal git repository record and returns a push token.
// If repo with same name already exists in the same project, returns a new token for it.
func (s *Service) CreateRepo(ctx context.Context, userID, projectID, repoName, description string, private bool) (*CreateRepoResult, error) {
	user, err := s.userQueries.GetUserByID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get user: %w", err)
	}

	gitUsername, err := s.ResolveUsername(ctx, user)
	if err != nil {
		return nil, fmt.Errorf("resolve username: %w", err)
	}

	// Check if repo already exists in this project
	existingRepo, err := s.repoQueries.GetInternalRepoByProjectAndName(ctx, internalrepos.GetInternalRepoByProjectAndNameParams{
		ProjectID: projectID,
		Name:      repoName,
	})
	if err == nil {
		if existingRepo.UserID != userID {
			return nil, fmt.Errorf("repo belongs to another user")
		}
		owner, gitName := splitFullName(existingRepo.FullName)
		rawToken, err := s.createToken(ctx, userID, &existingRepo.ID, nil)
		if err != nil {
			return nil, fmt.Errorf("create token: %w", err)
		}
		return &CreateRepoResult{
			Repo:      s.repoPath(owner, gitName),
			GitRemote: s.cloneURL(owner, gitName, rawToken),
			ExpiresAt: time.Now().Add(365 * 24 * time.Hour).Format(time.RFC3339),
			Message:   "Repo already exists. Push your code, then call create_app to deploy",
		}, nil
	}

	// Generate full_name with random suffix for global uniqueness: {username}/{name}-{slug}
	var fullName, gitName string
	for attempt := 0; attempt < maxSlugRetries; attempt++ {
		slug, err := randomSlug()
		if err != nil {
			return nil, fmt.Errorf("generate slug: %w", err)
		}
		gitName = fmt.Sprintf("%s-%s", repoName, slug)
		fullName = fmt.Sprintf("%s/%s", gitUsername, gitName)
		// Check if full_name is already taken (extremely unlikely)
		_, err = s.repoQueries.GetInternalRepoByFullName(ctx, fullName)
		if err != nil {
			break // not found = available
		}
		if attempt == maxSlugRetries-1 {
			return nil, fmt.Errorf("failed to generate unique repo path after %d attempts", maxSlugRetries)
		}
	}

	barePath := fmt.Sprintf("%s/%s.git", gitUsername, gitName)
	repo, err := s.repoQueries.CreateInternalRepo(ctx, internalrepos.CreateInternalRepoParams{
		UserID:    userID,
		ProjectID: projectID,
		Name:      repoName,
		CloneUrl:  s.cloneURLWithoutAuth(gitUsername, gitName),
		Provider:  "internal",
		FullName:  fullName,
		BarePath:  &barePath,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to store repo in database: %w", err)
	}

	rawToken, err := s.createToken(ctx, userID, &repo.ID, nil)
	if err != nil {
		return nil, fmt.Errorf("create token: %w", err)
	}

	return &CreateRepoResult{
		Repo:      s.repoPath(gitUsername, gitName),
		GitRemote: s.cloneURL(gitUsername, gitName, rawToken),
		ExpiresAt: time.Now().Add(365 * 24 * time.Hour).Format(time.RFC3339),
		Message:   "Push your code, then call create_app to deploy",
	}, nil
}

// GetPushToken returns git remote with a per-repo scoped token.
func (s *Service) GetPushToken(ctx context.Context, userID, repoFullName string) (*GetPushTokenResult, error) {
	repo, err := s.repoQueries.GetInternalRepoByFullName(ctx, repoFullName)
	if err != nil {
		return nil, fmt.Errorf("repo not found: %w", err)
	}
	if repo.UserID != userID {
		return nil, fmt.Errorf("unauthorized: repo belongs to another user")
	}

	owner, repoName := splitFullName(repoFullName)
	if owner == "" || repoName == "" {
		return nil, fmt.Errorf("invalid repo full name: %s", repoFullName)
	}

	rawToken, err := s.createToken(ctx, userID, &repo.ID, nil)
	if err != nil {
		return nil, fmt.Errorf("create token: %w", err)
	}

	return &GetPushTokenResult{
		GitRemote: s.cloneURL(owner, repoName, rawToken),
		ExpiresAt: time.Now().Add(365 * 24 * time.Hour).Format(time.RFC3339),
	}, nil
}

// DeleteRepo deletes an internal git repository from the database.
func (s *Service) DeleteRepo(ctx context.Context, userID, repoFullName string) error {
	repo, err := s.repoQueries.GetInternalRepoByFullName(ctx, repoFullName)
	if err != nil {
		return fmt.Errorf("repo not found: %w", err)
	}
	if repo.UserID != userID {
		return fmt.Errorf("unauthorized: repo belongs to another user")
	}

	repoID := repo.ID
	if err := s.tokenQ.RevokeTokensByRepoID(ctx, &repoID); err != nil {
		return fmt.Errorf("failed to revoke tokens: %w", err)
	}

	if err := s.repoQueries.DeleteInternalRepoByFullName(ctx, repoFullName); err != nil {
		return fmt.Errorf("failed to delete repo from database: %w", err)
	}

	return nil
}

func (s *Service) GetRepoByFullName(ctx context.Context, fullName string) (internalrepos.InternalRepo, error) {
	return s.repoQueries.GetInternalRepoByFullName(ctx, fullName)
}

func (s *Service) GetRepoByProjectAndName(ctx context.Context, projectID, name string) (internalrepos.InternalRepo, error) {
	return s.repoQueries.GetInternalRepoByProjectAndName(ctx, internalrepos.GetInternalRepoByProjectAndNameParams{
		ProjectID: projectID,
		Name:      name,
	})
}

func (s *Service) createToken(ctx context.Context, userID string, repoID *string, expiresAt *time.Time) (string, error) {
	rawBytes := make([]byte, 32)
	if _, err := rand.Read(rawBytes); err != nil {
		return "", fmt.Errorf("generate random token: %w", err)
	}
	rawToken := tokenPrefix + base64.RawURLEncoding.EncodeToString(rawBytes)

	hash := sha256.Sum256([]byte(rawToken))
	hashStr := hex.EncodeToString(hash[:])

	prefix := rawToken[:8]

	var expiresAtPg pgtype.Timestamptz
	if expiresAt != nil {
		expiresAtPg = pgtype.Timestamptz{Time: *expiresAt, Valid: true}
	}

	_, err := s.tokenQ.CreateToken(ctx, gittokens.CreateTokenParams{
		TokenHash:   hashStr,
		TokenPrefix: prefix,
		UserID:      userID,
		RepoID:      repoID,
		Scopes:      []string{"push", "pull"},
		ExpiresAt:   expiresAtPg,
	})
	if err != nil {
		return "", fmt.Errorf("store token: %w", err)
	}

	return rawToken, nil
}

func (s *Service) cloneURL(owner, repoName, token string) string {
	u, _ := url.Parse(s.config.PublicGitURL)
	u.User = url.UserPassword("x-git-token", token)
	u.Path = fmt.Sprintf("/%s/%s.git", owner, repoName)
	return u.String()
}

func (s *Service) cloneURLWithoutAuth(owner, repoName string) string {
	u, _ := url.Parse(s.config.PublicGitURL)
	u.Path = fmt.Sprintf("/%s/%s.git", owner, repoName)
	return u.String()
}

func (s *Service) repoPath(owner, repoName string) string {
	u, _ := url.Parse(s.config.PublicGitURL)
	host := u.Hostname()
	if len(host) > 4 && host[:4] == "git." {
		host = host[4:]
	}
	return fmt.Sprintf("%s/%s/%s", host, owner, repoName)
}

func splitFullName(fullName string) (owner, repo string) {
	for i := 0; i < len(fullName); i++ {
		if fullName[i] == '/' {
			return fullName[:i], fullName[i+1:]
		}
	}
	return "", ""
}

func randomSlug() (string, error) {
	b := make([]byte, slugLen)
	for i := range b {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(slugCharset))))
		if err != nil {
			return "", err
		}
		b[i] = slugCharset[n.Int64()]
	}
	return string(b), nil
}
