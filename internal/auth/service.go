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
	"strconv"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"golang.org/x/crypto/bcrypt"

	"github.com/augustdev/autoclip/internal/github"
	"github.com/augustdev/autoclip/internal/storage/pg"
	"github.com/augustdev/autoclip/internal/storage/pg/generated/apikeys"
	"github.com/augustdev/autoclip/internal/storage/pg/generated/users"
)

type Service struct {
	config      Config
	db          *pg.DB
	usersQ      users.Querier
	apiKeysQ    apikeys.Querier
	githubOAuth *github.OAuthService
}

type Session struct {
	Token  string
	UserID int64
	User   *users.User
}

type APIKeyResult struct {
	ID         int64
	Name       string
	Prefix     string
	FullKey    string
	CreatedAt  time.Time
}

func NewService(
	config Config,
	db *pg.DB,
	usersQ users.Querier,
	apiKeysQ apikeys.Querier,
	githubOAuth *github.OAuthService,
) *Service {
	return &Service{
		config:      config,
		db:          db,
		usersQ:      usersQ,
		apiKeysQ:    apiKeysQ,
		githubOAuth: githubOAuth,
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

	user, err := s.usersQ.GetUserByGitHubID(ctx, ghUser.ID)
	if err != nil {
		user, err = s.usersQ.CreateUser(ctx, users.CreateUserParams{
			GithubID:       ghUser.ID,
			GithubUsername: ghUser.Login,
			GithubToken:    encryptedToken,
			AvatarUrl:      pgtype.Text{String: ghUser.AvatarURL, Valid: ghUser.AvatarURL != ""},
		})
		if err != nil {
			return nil, fmt.Errorf("failed to create user: %w", err)
		}
	} else {
		user, err = s.usersQ.UpdateGitHubToken(ctx, users.UpdateGitHubTokenParams{
			ID:             user.ID,
			GithubToken:    encryptedToken,
			GithubUsername: ghUser.Login,
			AvatarUrl:      pgtype.Text{String: ghUser.AvatarURL, Valid: ghUser.AvatarURL != ""},
		})
		if err != nil {
			return nil, fmt.Errorf("failed to update user: %w", err)
		}
	}

	token, err := s.generateJWT(user.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to generate jwt: %w", err)
	}

	return &Session{
		Token:  token,
		UserID: user.ID,
		User:   &user,
	}, nil
}

func (s *Service) ValidateJWT(tokenString string) (int64, error) {
	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return []byte(s.config.JWTSecret), nil
	})

	if err != nil {
		return 0, fmt.Errorf("failed to parse token: %w", err)
	}

	if claims, ok := token.Claims.(jwt.MapClaims); ok && token.Valid {
		switch sub := claims["sub"].(type) {
		case string:
			userID, err := strconv.ParseInt(sub, 10, 64)
			if err != nil {
				return 0, fmt.Errorf("invalid subject claim: %w", err)
			}
			return userID, nil
		case float64:
			return int64(sub), nil
		default:
			return 0, fmt.Errorf("invalid subject claim type")
		}
	}

	return 0, fmt.Errorf("invalid token")
}

func (s *Service) GenerateAPIKey(ctx context.Context, userID int64, name string) (*APIKeyResult, error) {
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

func (s *Service) ValidateAPIKey(ctx context.Context, key string) (int64, error) {
	if len(key) < 16 {
		return 0, fmt.Errorf("invalid api key format")
	}

	prefix := key[:16]

	apiKey, err := s.apiKeysQ.GetAPIKeyByPrefix(ctx, prefix)
	if err != nil {
		return 0, fmt.Errorf("api key not found")
	}

	if err := bcrypt.CompareHashAndPassword([]byte(apiKey.KeyHash), []byte(key)); err != nil {
		return 0, fmt.Errorf("invalid api key")
	}

	_ = s.apiKeysQ.UpdateAPIKeyLastUsed(ctx, apiKey.ID)

	return apiKey.UserID, nil
}

func (s *Service) RevokeAPIKey(ctx context.Context, userID, keyID int64) error {
	return s.apiKeysQ.RevokeAPIKey(ctx, apikeys.RevokeAPIKeyParams{
		ID:     keyID,
		UserID: userID,
	})
}

func (s *Service) GetUserByID(ctx context.Context, userID int64) (*users.User, error) {
	user, err := s.usersQ.GetUserByID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("user not found: %w", err)
	}
	return &user, nil
}

func (s *Service) ListAPIKeys(ctx context.Context, userID int64) ([]apikeys.ListAPIKeysByUserIDRow, error) {
	return s.apiKeysQ.ListAPIKeysByUserID(ctx, userID)
}

func (s *Service) UpdateFlyioCredentials(ctx context.Context, userID int64, encryptedToken, org string) (*users.User, error) {
	user, err := s.usersQ.UpdateFlyioCredentials(ctx, users.UpdateFlyioCredentialsParams{
		ID:         userID,
		FlyioToken: pgtype.Text{String: encryptedToken, Valid: encryptedToken != ""},
		FlyioOrg:   pgtype.Text{String: org, Valid: org != ""},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to update flyio credentials: %w", err)
	}
	return &user, nil
}

func (s *Service) EncryptToken(token string) (string, error) {
	return s.encryptToken(token)
}

func (s *Service) generateJWT(userID int64) (string, error) {
	now := time.Now()
	claims := jwt.RegisteredClaims{
		Subject:   fmt.Sprintf("%d", userID),
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
