package internalgit

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/url"
	"strings"
	"time"

	"code.gitea.io/sdk/gitea"
)

type Client struct {
	config Config
	api    *gitea.Client
}

func NewClient(config Config) (*Client, error) {
	if config.BaseURL == "" {
		return nil, fmt.Errorf("internalgit: GITEA_BASEURL is required")
	}
	if config.AdminToken == "" {
		return nil, fmt.Errorf("internalgit: GITEA_ADMINTOKEN is required")
	}
	if config.WebhookSecret == "" {
		return nil, fmt.Errorf("internalgit: GITEA_WEBHOOKSECRET is required")
	}

	api, err := gitea.NewClient(config.BaseURL, gitea.SetToken(config.AdminToken))
	if err != nil {
		return nil, fmt.Errorf("internalgit: failed to create gitea client: %w", err)
	}

	return &Client{
		config: config,
		api:    api,
	}, nil
}

func (c *Client) Config() Config {
	return c.config
}

func (c *Client) API() *gitea.Client {
	return c.api
}

func (c *Client) GetHTTPSCloneURL(username, repoName, token string) string {
	u, _ := url.Parse(c.config.BaseURL)
	u.User = url.UserPassword(username, token)
	u.Path = fmt.Sprintf("/%s/%s.git", username, repoName)
	return u.String()
}

// userPassword derives a deterministic password from HMAC-SHA256(username, adminToken).
// The "Gp!" prefix satisfies Gitea's password-complexity requirements.
func (c *Client) userPassword(username string) string {
	mac := hmac.New(sha256.New, []byte(c.config.AdminToken))
	mac.Write([]byte(username))
	return "Gp!" + hex.EncodeToString(mac.Sum(nil))[:24]
}

// CreateUserToken creates a temporary Gitea access token via BasicAuth SDK client.
func (c *Client) CreateUserToken(username string) (string, error) {
	userClient, err := gitea.NewClient(c.config.BaseURL, gitea.SetBasicAuth(username, c.userPassword(username)))
	if err != nil {
		return "", fmt.Errorf("create user client: %w", err)
	}

	token, _, err := userClient.CreateAccessToken(gitea.CreateAccessTokenOption{
		Name:   fmt.Sprintf("push-%d", time.Now().UnixMilli()),
		Scopes: []gitea.AccessTokenScope{"write:repository"},
	})
	if err != nil {
		return "", fmt.Errorf("create token: %w", err)
	}
	return token.Token, nil
}

// GetRepoPath returns "ml.ink/{username}/{repo}" (strips "git." prefix and port).
func (c *Client) GetRepoPath(username, repoName string) string {
	u, _ := url.Parse(c.config.BaseURL)
	host := u.Hostname()
	host = strings.TrimPrefix(host, "git.")
	return fmt.Sprintf("%s/%s/%s", host, username, repoName)
}
