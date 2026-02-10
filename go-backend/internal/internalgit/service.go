package internalgit

import (
	"context"
	"fmt"
	"time"

	"github.com/augustdev/autoclip/internal/storage/pg/generated/internalrepos"
	"github.com/augustdev/autoclip/internal/storage/pg/generated/users"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Service struct {
	client      *Client
	db          *pgxpool.Pool
	webhookURL  string
	repoQueries internalrepos.Querier
	userQueries users.Querier
}

type ServiceConfig struct {
	Client     *Client
	DB         *pgxpool.Pool
	WebhookURL string
}

func NewService(cfg ServiceConfig) *Service {
	return &Service{
		client:      cfg.Client,
		db:          cfg.DB,
		webhookURL:  cfg.WebhookURL,
		repoQueries: internalrepos.New(cfg.DB),
		userQueries: users.New(cfg.DB),
	}
}

func (s *Service) Client() *Client {
	return s.client
}

// CreateRepo creates a new internal git repository under user's GitHub username.
// Idempotent: if repo already exists for this user, returns credentials.
func (s *Service) CreateRepo(ctx context.Context, userID, repoName, description string, private bool) (*CreateRepoResult, error) {
	user, err := s.userQueries.GetUserByID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get user: %w", err)
	}

	giteaUsername, err := s.ResolveGiteaUsername(ctx, user)
	if err != nil {
		return nil, fmt.Errorf("resolve gitea username: %w", err)
	}

	fullName := fmt.Sprintf("%s/%s", giteaUsername, repoName)

	// Check if repo already exists in our database
	existingRepo, err := s.repoQueries.GetInternalRepoByFullName(ctx, fullName)
	if err == nil {
		if existingRepo.UserID != userID {
			return nil, fmt.Errorf("repo belongs to another user")
		}
		userToken, tokenErr := s.client.CreateUserToken(giteaUsername)
		if tokenErr != nil {
			return nil, fmt.Errorf("create user token: %w", tokenErr)
		}
		return &CreateRepoResult{
			Repo:      s.client.GetRepoPath(giteaUsername, repoName),
			GitRemote: s.client.GetHTTPSCloneURL(giteaUsername, repoName, userToken),
			Message:   "Repo already exists. Push your code, then call create_app to deploy",
		}, nil
	}

	// Create the repo under user's account (or get existing)
	repo, err := s.client.CreateRepoForUser(ctx, giteaUsername, repoName, description, private)
	if err != nil {
		repo, err = s.client.GetRepo(ctx, giteaUsername, repoName)
		if err != nil {
			return nil, fmt.Errorf("failed to create repo: %w", err)
		}
	}

	// Create webhook for the repo
	_, err = s.client.CreateRepoWebhook(ctx, giteaUsername, repoName, s.webhookURL, s.client.config.WebhookSecret)
	if err != nil && !isAlreadyExistsError(err) {
		fmt.Printf("warning: failed to create webhook for %s/%s: %v\n", giteaUsername, repoName, err)
	}

	// Store repo in database (upsert)
	_, err = s.repoQueries.CreateInternalRepo(ctx, internalrepos.CreateInternalRepoParams{
		UserID:   userID,
		Provider: "gitea",
		RepoID:   repo.ID,
		FullName: repo.FullName,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to store repo in database: %w", err)
	}

	// Create a scoped user token for the clone URL
	userToken, err := s.client.CreateUserToken(giteaUsername)
	if err != nil {
		return nil, fmt.Errorf("failed to create user token: %w", err)
	}

	repoPath := s.client.GetRepoPath(giteaUsername, repoName)
	gitRemote := s.client.GetHTTPSCloneURL(giteaUsername, repoName, userToken)

	return &CreateRepoResult{
		Repo:      repoPath,
		GitRemote: gitRemote,
		ExpiresAt: time.Now().Add(365 * 24 * time.Hour).Format(time.RFC3339),
		Message:   "Push your code, then call create_app to deploy",
	}, nil
}

func isAlreadyExistsError(err error) bool {
	if err == nil {
		return false
	}
	s := err.Error()
	return contains(s, "already exist") ||
		contains(s, "already exists") ||
		contains(s, "already been added") ||
		contains(s, "has already been added") ||
		contains(s, "has already been taken")
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsAt(s, substr))
}

func containsAt(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// GetPushToken returns git remote with a per-user scoped token
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

	userToken, err := s.client.CreateUserToken(owner)
	if err != nil {
		return nil, fmt.Errorf("failed to create user token: %w", err)
	}

	gitRemote := s.client.GetHTTPSCloneURL(owner, repoName, userToken)

	return &GetPushTokenResult{
		GitRemote: gitRemote,
		ExpiresAt: time.Now().Add(365 * 24 * time.Hour).Format(time.RFC3339),
	}, nil
}

// DeleteRepo deletes an internal git repository
func (s *Service) DeleteRepo(ctx context.Context, userID, repoFullName string) error {
	repo, err := s.repoQueries.GetInternalRepoByFullName(ctx, repoFullName)
	if err != nil {
		return fmt.Errorf("repo not found: %w", err)
	}
	if repo.UserID != userID {
		return fmt.Errorf("unauthorized: repo belongs to another user")
	}

	owner, repoName := splitFullName(repoFullName)

	if err := s.client.DeleteRepo(ctx, owner, repoName); err != nil {
		return fmt.Errorf("failed to delete repo from gitea: %w", err)
	}

	if err := s.repoQueries.DeleteInternalRepoByFullName(ctx, repoFullName); err != nil {
		return fmt.Errorf("failed to delete repo from database: %w", err)
	}

	return nil
}

// GetRepoByFullName retrieves an internal repo by its full name
func (s *Service) GetRepoByFullName(ctx context.Context, fullName string) (internalrepos.InternalRepo, error) {
	return s.repoQueries.GetInternalRepoByFullName(ctx, fullName)
}

func splitFullName(fullName string) (owner, repo string) {
	for i := 0; i < len(fullName); i++ {
		if fullName[i] == '/' {
			return fullName[:i], fullName[i+1:]
		}
	}
	return "", ""
}
