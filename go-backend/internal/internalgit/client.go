package internalgit

import (
	"fmt"
	"net/url"
	"strings"

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
	if config.SSHURL == "" {
		return nil, fmt.Errorf("internalgit: GITEA_SSHURL is required")
	}
	if strings.HasPrefix(config.SSHURL, "ssh://") {
		return nil, fmt.Errorf("internalgit: GITEA_SSHURL must not start with ssh:// (Coolify rejects ssh:// for git_repository); set GITEA_SSHURL=git@host and GITEA_SSHPORT")
	}
	if config.SSHPort == 0 {
		return nil, fmt.Errorf("internalgit: GITEA_SSHPORT is required")
	}
	if config.SSHPort < 1 || config.SSHPort > 65535 {
		return nil, fmt.Errorf("internalgit: GITEA_SSHPORT must be between 1 and 65535")
	}
	if config.CoolifyPrivateKeyUUID == "" {
		return nil, fmt.Errorf("internalgit: GITEA_COOLIFYPRIVATEKEYUUID is required")
	}
	if config.DeployPublicKey == "" {
		return nil, fmt.Errorf("internalgit: GITEA_DEPLOYPUBLICKEY is required")
	}
	if config.DeployBotUsername == "" {
		return nil, fmt.Errorf("internalgit: GITEA_DEPLOYBOTUSERNAME is required")
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

// GetHTTPSCloneURL returns the HTTPS clone URL with embedded token
func (c *Client) GetHTTPSCloneURL(username, repoName, token string) string {
	u, _ := url.Parse(c.config.BaseURL)
	u.User = url.UserPassword(username, token)
	u.Path = fmt.Sprintf("/%s/%s.git", username, repoName)
	return u.String()
}

// GetSSHCloneURL returns the SSH clone URL
// Format returned is compatible with Coolify "Private Repository (with Deploy Key)":
// - Standard: git@host:user/repo.git (port 22)
// - Non-standard port: git@host:PORT/user/repo.git (Coolify extracts PORT and uses it for SSH)
func (c *Client) GetSSHCloneURL(username, repoName string) string {
	sshBase := strings.TrimSpace(c.config.SSHURL)
	sshPort := c.config.SSHPort

	// Coolify deploy-key clones support a non-standard SSH port only via a special format:
	// `git@host:PORT/owner/repo.git` (Coolify parses `:PORT/` and uses that as the SSH port).
	//
	// We intentionally do NOT return `ssh://git@host:PORT/owner/repo.git` because Coolify's
	// create-app validation rejects `ssh://` for `git_repository`.
	if sshPort != 22 {
		return fmt.Sprintf("%s:%d/%s/%s.git", sshBase, sshPort, username, repoName)
	}

	// Standard scp-style format on port 22.
	return fmt.Sprintf("%s:%s/%s.git", sshBase, username, repoName)
}

// GetRepoPath returns the repo path in format "ml.ink/{username}/{repo}"
func (c *Client) GetRepoPath(username, repoName string) string {
	u, _ := url.Parse(c.config.BaseURL)
	host := u.Host
	// Remove port if present
	for i := len(host) - 1; i >= 0; i-- {
		if host[i] == ':' {
			host = host[:i]
			break
		}
	}
	// Remove "git." prefix if present for cleaner paths
	if len(host) > 4 && host[:4] == "git." {
		host = host[4:]
	}
	return fmt.Sprintf("%s/%s/%s", host, username, repoName)
}

// NewClientWithBasicAuth creates a client authenticated as a specific user
func NewClientWithBasicAuth(baseURL, username, password string) (*gitea.Client, error) {
	return gitea.NewClient(baseURL, gitea.SetBasicAuth(username, password))
}
