package internalgit

import (
	"context"
	"fmt"

	"code.gitea.io/sdk/gitea"
)

// EnsureUser creates a Gitea user with a deterministic password if it doesn't exist.
func (c *Client) EnsureUser(ctx context.Context, username, email string) error {
	_, _, err := c.api.GetUserInfo(username)
	if err == nil {
		return nil
	}

	pass := c.userPassword(username)
	mustChange := false
	visibility := gitea.VisibleTypePrivate
	_, _, err = c.api.AdminCreateUser(gitea.CreateUserOption{
		Username:           username,
		Email:              email,
		Password:           pass,
		MustChangePassword: &mustChange,
		Visibility:         &visibility,
	})
	if err != nil {
		return fmt.Errorf("create gitea user: %w", err)
	}
	return nil
}

// CreateRepoForUser creates a repository under a specific user (requires admin).
func (c *Client) CreateRepoForUser(ctx context.Context, username, repoName, description string, private bool) (*gitea.Repository, error) {
	repo, _, err := c.api.AdminCreateRepo(username, gitea.CreateRepoOption{
		Name:          repoName,
		Description:   description,
		Private:       private,
		AutoInit:      false,
		DefaultBranch: "main",
	})
	if err != nil {
		return nil, err
	}
	return repo, nil
}

func (c *Client) GetRepo(ctx context.Context, owner, repo string) (*gitea.Repository, error) {
	r, _, err := c.api.GetRepo(owner, repo)
	if err != nil {
		return nil, err
	}
	return r, nil
}

func (c *Client) DeleteRepo(ctx context.Context, owner, repo string) error {
	_, err := c.api.DeleteRepo(owner, repo)
	return err
}

func (c *Client) CreateRepoWebhook(ctx context.Context, owner, repo, webhookURL, secret string) (*gitea.Hook, error) {
	hook, _, err := c.api.CreateRepoHook(owner, repo, gitea.CreateHookOption{
		Type: gitea.HookTypeGitea,
		Config: map[string]string{
			"url":          webhookURL,
			"content_type": "json",
			"secret":       secret,
		},
		Events: []string{"push"},
		Active: true,
	})
	if err != nil {
		return nil, err
	}
	return hook, nil
}
