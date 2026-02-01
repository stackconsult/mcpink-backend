package auth

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/lithammer/shortuuid/v4"
	"go.temporal.io/sdk/client"
	"golang.org/x/crypto/bcrypt"

	"github.com/augustdev/autoclip/internal/account"
	"github.com/augustdev/autoclip/internal/coolify"
	"github.com/augustdev/autoclip/internal/github_oauth"
	"github.com/augustdev/autoclip/internal/githubapp"
	"github.com/augustdev/autoclip/internal/helpers"
	"github.com/augustdev/autoclip/internal/storage/pg"
	"github.com/augustdev/autoclip/internal/storage/pg/generated/apikeys"
	"github.com/augustdev/autoclip/internal/storage/pg/generated/githubcreds"
	"github.com/augustdev/autoclip/internal/storage/pg/generated/users"
)

type Service struct {
	config        Config
	db            *pg.DB
	usersQ        users.Querier
	apiKeysQ      apikeys.Querier
	githubCredsQ  githubcreds.Querier
	githubOAuth   *github_oauth.OAuthService
	coolify       *coolify.Client
	githubAppCfg  githubapp.Config
	temporal      client.Client
	logger        *slog.Logger
}

type Session struct {
	Token  string
	UserID string
}

type APIKeyResult struct {
	ID        string
	Name      string
	Prefix    string
	FullKey   string
	CreatedAt time.Time
}

func NewService(
	config Config,
	db *pg.DB,
	usersQ users.Querier,
	apiKeysQ apikeys.Querier,
	githubCredsQ githubcreds.Querier,
	githubOAuth *github_oauth.OAuthService,
	coolify *coolify.Client,
	githubAppCfg githubapp.Config,
	temporal client.Client,
	logger *slog.Logger,
) *Service {
	return &Service{
		config:        config,
		db:            db,
		usersQ:        usersQ,
		apiKeysQ:      apiKeysQ,
		githubCredsQ:  githubCredsQ,
		githubOAuth:   githubOAuth,
		coolify:       coolify,
		githubAppCfg:  githubAppCfg,
		temporal:      temporal,
		logger:        logger,
	}
}

func (s *Service) HandleOAuthCallback(ctx context.Context, code string) (*Session, error) {
	tokenResp, err := s.githubOAuth.ExchangeCode(ctx, code)
	if err != nil {
		return nil, fmt.Errorf("failed to exchange code: %w", err)
	}

	ghUser, err := s.githubOAuth.GetUser(ctx, tokenResp.AccessToken)
	if err != nil {
		return nil, fmt.Errorf("failed to get github user: %w", err)
	}

	encryptedToken, err := s.encryptToken(tokenResp.AccessToken)
	if err != nil {
		return nil, fmt.Errorf("failed to encrypt token: %w", err)
	}

	newScopes := parseScopes(tokenResp.Scope)

	var userID string
	user, err := s.usersQ.GetUserByGitHubID(ctx, ghUser.ID)
	if err != nil {
		// New user - create user and github_creds, trigger async account setup
		userID = shortuuid.New()
		var avatarURL *string
		if ghUser.AvatarURL != "" {
			avatarURL = helpers.Ptr(ghUser.AvatarURL)
		}

		_, err = s.usersQ.CreateUser(ctx, users.CreateUserParams{
			ID:             userID,
			GithubID:       ghUser.ID,
			GithubUsername: ghUser.Login,
			AvatarUrl:      avatarURL,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to create user: %w", err)
		}

		_, err = s.githubCredsQ.CreateGitHubCreds(ctx, githubcreds.CreateGitHubCredsParams{
			UserID:            userID,
			GithubID:          ghUser.ID,
			GithubOauthToken:  encryptedToken,
			GithubOauthScopes: newScopes,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to create github creds: %w", err)
		}

		s.triggerAccountSetup(userID)
	} else {
		userID = user.ID

		// Get existing github creds for downgrade protection
		existingCreds, credsErr := s.githubCredsQ.GetGitHubCredsByUserID(ctx, user.ID)
		if credsErr != nil {
			return nil, fmt.Errorf("failed to get github creds: %w", credsErr)
		}

		// Apply downgrade protection
		finalToken := encryptedToken
		finalScopes := newScopes

		if contains(existingCreds.GithubOauthScopes, "repo") && !contains(newScopes, "repo") {
			oldTokenPlain, decryptErr := s.DecryptToken(existingCreds.GithubOauthToken)
			if decryptErr == nil && s.githubOAuth.IsTokenValid(ctx, oldTokenPlain) {
				finalToken = existingCreds.GithubOauthToken
				finalScopes = existingCreds.GithubOauthScopes
			}
		}

		// Update user profile
		var avatarURLUpdate *string
		if ghUser.AvatarURL != "" {
			avatarURLUpdate = helpers.Ptr(ghUser.AvatarURL)
		}
		_, err = s.usersQ.UpdateUserProfile(ctx, users.UpdateUserProfileParams{
			ID:             user.ID,
			GithubUsername: ghUser.Login,
			AvatarUrl:      avatarURLUpdate,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to update user profile: %w", err)
		}

		// Update github creds
		_, err = s.githubCredsQ.UpdateGitHubOAuthToken(ctx, githubcreds.UpdateGitHubOAuthTokenParams{
			UserID:            user.ID,
			GithubOauthToken:  finalToken,
			GithubOauthScopes: finalScopes,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to update github creds: %w", err)
		}
	}

	token, err := s.generateJWT(userID)
	if err != nil {
		return nil, fmt.Errorf("failed to generate jwt: %w", err)
	}

	return &Session{
		Token:  token,
		UserID: userID,
	}, nil
}

func (s *Service) ValidateJWT(tokenString string) (string, error) {
	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return []byte(s.config.JWTSecret), nil
	})

	if err != nil {
		return "", fmt.Errorf("failed to parse token: %w", err)
	}

	if claims, ok := token.Claims.(jwt.MapClaims); ok && token.Valid {
		if sub, ok := claims["sub"].(string); ok {
			return sub, nil
		}
		return "", fmt.Errorf("invalid subject claim type")
	}

	return "", fmt.Errorf("invalid token")
}

func (s *Service) GenerateAPIKey(ctx context.Context, userID string, name string) (*APIKeyResult, error) {
	keyBytes := make([]byte, 32)
	if _, err := rand.Read(keyBytes); err != nil {
		return nil, fmt.Errorf("failed to generate random key: %w", err)
	}

	fullKey := fmt.Sprintf("dk_live_%x", keyBytes)

	keyHash, err := bcrypt.GenerateFromPassword([]byte(fullKey), bcrypt.DefaultCost)
	if err != nil {
		return nil, fmt.Errorf("failed to hash key: %w", err)
	}

	prefix := fullKey[:16]

	apiKey, err := s.apiKeysQ.CreateAPIKey(ctx, apikeys.CreateAPIKeyParams{
		UserID:    userID,
		Name:      name,
		KeyHash:   string(keyHash),
		KeyPrefix: prefix,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create api key: %w", err)
	}

	return &APIKeyResult{
		ID:        apiKey.ID,
		Name:      apiKey.Name,
		Prefix:    apiKey.KeyPrefix,
		FullKey:   fullKey,
		CreatedAt: apiKey.CreatedAt.Time,
	}, nil
}

func (s *Service) ValidateAPIKey(ctx context.Context, key string) (string, error) {
	if len(key) < 16 {
		return "", fmt.Errorf("invalid api key format")
	}

	prefix := key[:16]

	apiKey, err := s.apiKeysQ.GetAPIKeyByPrefix(ctx, prefix)
	if err != nil {
		return "", fmt.Errorf("api key not found")
	}

	if err := bcrypt.CompareHashAndPassword([]byte(apiKey.KeyHash), []byte(key)); err != nil {
		return "", fmt.Errorf("invalid api key")
	}

	_ = s.apiKeysQ.UpdateAPIKeyLastUsed(ctx, apiKey.ID)

	return apiKey.UserID, nil
}

func (s *Service) RevokeAPIKey(ctx context.Context, userID string, keyID string) error {
	return s.apiKeysQ.RevokeAPIKey(ctx, apikeys.RevokeAPIKeyParams{
		ID:     keyID,
		UserID: userID,
	})
}

func (s *Service) GetUserByID(ctx context.Context, userID string) (*users.User, error) {
	user, err := s.usersQ.GetUserByID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("user not found: %w", err)
	}
	return &user, nil
}

func (s *Service) ListAPIKeys(ctx context.Context, userID string) ([]apikeys.ListAPIKeysByUserIDRow, error) {
	return s.apiKeysQ.ListAPIKeysByUserID(ctx, userID)
}

func (s *Service) SetGitHubAppInstallation(ctx context.Context, userID string, installationID int64) error {
	_, err := s.githubCredsQ.SetGitHubAppInstallation(ctx, githubcreds.SetGitHubAppInstallationParams{
		UserID:                  userID,
		GithubAppInstallationID: helpers.Ptr(installationID),
	})
	return err
}

func (s *Service) ClearGitHubAppInstallation(ctx context.Context, userID string) error {
	_, err := s.githubCredsQ.ClearGitHubAppInstallation(ctx, userID)
	return err
}

func (s *Service) CreateCoolifyGitHubAppSource(ctx context.Context, userID string, installationID int64, githubUsername string) (string, error) {
	sourceName := fmt.Sprintf("gh-%s-%d", githubUsername, installationID)

	req := &coolify.CreateGitHubAppSourceRequest{
		Name:           sourceName,
		APIUrl:         "https://api.github.com",
		HTMLUrl:        "https://github.com",
		CustomUser:     "git",
		CustomPort:     22,
		AppID:          s.githubAppCfg.AppID,
		InstallationID: installationID,
		ClientID:       s.githubAppCfg.ClientID,
		ClientSecret:   s.githubAppCfg.ClientSecret,
		WebhookSecret:  "not-used",
		PrivateKeyUUID: s.githubAppCfg.CoolifyPrivKeyUUID,
		IsSystemWide:   false,
	}

	source, err := s.coolify.Sources.CreateGitHubApp(ctx, req)
	if err != nil {
		return "", fmt.Errorf("failed to create coolify github app source: %w", err)
	}

	_, err = s.usersQ.SetCoolifyGitHubAppUUID(ctx, users.SetCoolifyGitHubAppUUIDParams{
		ID:                   userID,
		CoolifyGithubAppUuid: helpers.Ptr(source.UUID),
	})
	if err != nil {
		return "", fmt.Errorf("failed to save coolify github app uuid: %w", err)
	}

	return source.UUID, nil
}

func (s *Service) GetGitHubCredsByUserID(ctx context.Context, userID string) (*githubcreds.GithubCred, error) {
	creds, err := s.githubCredsQ.GetGitHubCredsByUserID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("github creds not found: %w", err)
	}
	return &creds, nil
}

func (s *Service) EncryptToken(token string) (string, error) {
	return s.encryptToken(token)
}

func (s *Service) generateJWT(userID string) (string, error) {
	now := time.Now()
	claims := jwt.RegisteredClaims{
		Subject:   userID,
		IssuedAt:  jwt.NewNumericDate(now),
		ExpiresAt: jwt.NewNumericDate(now.Add(s.config.JWTExpiry)),
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(s.config.JWTSecret))
}

func (s *Service) encryptToken(token string) (string, error) {
	key := sha256.Sum256([]byte(s.config.APIKeyEncryptionKey))

	block, err := aes.NewCipher(key[:])
	if err != nil {
		return "", err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}

	ciphertext := gcm.Seal(nonce, nonce, []byte(token), nil)
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

func (s *Service) DecryptToken(encrypted string) (string, error) {
	key := sha256.Sum256([]byte(s.config.APIKeyEncryptionKey))

	ciphertext, err := base64.StdEncoding.DecodeString(encrypted)
	if err != nil {
		return "", err
	}

	block, err := aes.NewCipher(key[:])
	if err != nil {
		return "", err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	if len(ciphertext) < gcm.NonceSize() {
		return "", fmt.Errorf("ciphertext too short")
	}

	nonce, ciphertext := ciphertext[:gcm.NonceSize()], ciphertext[gcm.NonceSize():]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", err
	}

	return string(plaintext), nil
}

func parseScopes(scopeStr string) []string {
	if scopeStr == "" {
		return []string{}
	}
	return strings.FieldsFunc(scopeStr, func(r rune) bool {
		return r == ' ' || r == ','
	})
}

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

func (s *Service) triggerAccountSetup(userID string) {
	if s.temporal == nil {
		s.logger.Warn("Temporal client not available, skipping account setup workflow")
		return
	}

	workflowID := fmt.Sprintf("setup-account-%s", userID)
	options := client.StartWorkflowOptions{
		ID:        workflowID,
		TaskQueue: account.TaskQueue,
	}

	input := account.SetupAccountInput{
		UserID: userID,
	}

	_, err := s.temporal.ExecuteWorkflow(context.Background(), options, account.SetupAccountWorkflow, input)
	if err != nil {
		s.logger.Error("Failed to start account setup workflow", "userID", userID, "error", err)
		return
	}

	s.logger.Info("Started account setup workflow", "userID", userID, "workflowID", workflowID)
}
