package githubapp

import (
	"context"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

type Service struct {
	config     Config
	privateKey *rsa.PrivateKey
	client     *http.Client
}

type Installation struct {
	ID      int64   `json:"id"`
	Account Account `json:"account"`
}

type Account struct {
	Login string `json:"login"`
	ID    int64  `json:"id"`
}

func NewService(config Config) (*Service, error) {
	// Handle escaped newlines from env var
	keyData := strings.ReplaceAll(config.PrivateKey, "\\n", "\n")

	block, _ := pem.Decode([]byte(keyData))
	if block == nil {
		return nil, fmt.Errorf("failed to decode PEM block")
	}

	privateKey, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse private key: %w", err)
	}

	return &Service{
		config:     config,
		privateKey: privateKey,
		client:     &http.Client{Timeout: 10 * time.Second},
	}, nil
}

func (s *Service) generateJWT() (string, error) {
	now := time.Now()
	claims := jwt.MapClaims{
		"iat": now.Unix(),
		"exp": now.Add(10 * time.Minute).Unix(),
		"iss": s.config.AppID,
	}

	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	return token.SignedString(s.privateKey)
}

// GetUserInstallation checks if a GitHub user has installed the app
// Returns the installation ID if installed, 0 if not installed
func (s *Service) GetUserInstallation(ctx context.Context, username string) (int64, error) {
	jwtToken, err := s.generateJWT()
	if err != nil {
		return 0, fmt.Errorf("failed to generate JWT: %w", err)
	}

	url := fmt.Sprintf("https://api.github.com/users/%s/installation", username)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return 0, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", jwtToken))
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")

	resp, err := s.client.Do(req)
	if err != nil {
		return 0, fmt.Errorf("failed to get installation: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		// User hasn't installed the app
		return 0, nil
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return 0, fmt.Errorf("github returned status %d: %s", resp.StatusCode, string(body))
	}

	var installation Installation
	if err := json.NewDecoder(resp.Body).Decode(&installation); err != nil {
		return 0, fmt.Errorf("failed to decode installation: %w", err)
	}

	return installation.ID, nil
}

type InstallationToken struct {
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expires_at"`
}

func (s *Service) CreateInstallationToken(ctx context.Context, installationID int64, repositories []string) (*InstallationToken, error) {
	jwtToken, err := s.generateJWT()
	if err != nil {
		return nil, fmt.Errorf("failed to generate JWT: %w", err)
	}

	url := fmt.Sprintf("https://api.github.com/app/installations/%d/access_tokens", installationID)

	// Don't scope to specific repositories - let the token have access to all repos
	// the installation has access to. This is needed because:
	// 1. Newly created repos may not be immediately available for scoping
	// 2. "Selected repositories" installations may not include the target repo
	req, err := http.NewRequestWithContext(ctx, "POST", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", jwtToken))
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to create installation token: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("github returned status %d: %s", resp.StatusCode, string(respBody))
	}

	var tokenResp struct {
		Token     string    `json:"token"`
		ExpiresAt time.Time `json:"expires_at"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return nil, fmt.Errorf("failed to decode token response: %w", err)
	}

	return &InstallationToken{
		Token:     tokenResp.Token,
		ExpiresAt: tokenResp.ExpiresAt,
	}, nil
}

type InstallationInfo struct {
	ID                  int64             `json:"id"`
	RepositorySelection string            `json:"repository_selection"` // "all" or "selected"
	Permissions         map[string]string `json:"permissions"`
}

func (s *Service) GetInstallation(ctx context.Context, installationID int64) (*InstallationInfo, error) {
	jwtToken, err := s.generateJWT()
	if err != nil {
		return nil, fmt.Errorf("failed to generate JWT: %w", err)
	}

	url := fmt.Sprintf("https://api.github.com/app/installations/%d", installationID)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", jwtToken))
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to get installation: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("github returned status %d: %s", resp.StatusCode, string(body))
	}

	var info InstallationInfo
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return nil, fmt.Errorf("failed to decode installation: %w", err)
	}

	return &info, nil
}
