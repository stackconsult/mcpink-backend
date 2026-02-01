package coolify

import (
	"context"
	"net/http"
)

type SourcesService struct {
	client *Client
}

type CreateGitHubAppSourceRequest struct {
	Name           string `json:"name"`
	Organization   string `json:"organization,omitempty"`
	APIUrl         string `json:"api_url"`
	HTMLUrl        string `json:"html_url"`
	CustomUser     string `json:"custom_user"`
	CustomPort     int    `json:"custom_port"`
	AppID          int64  `json:"app_id"`
	InstallationID int64  `json:"installation_id"`
	ClientID       string `json:"client_id"`
	ClientSecret   string `json:"client_secret"`
	WebhookSecret  string `json:"webhook_secret"`
	PrivateKeyUUID string `json:"private_key_uuid"`
	IsSystemWide   bool   `json:"is_system_wide"`
}

type GitHubAppSource struct {
	UUID string `json:"uuid"`
	Name string `json:"name"`
}

func (s *SourcesService) CreateGitHubApp(ctx context.Context, req *CreateGitHubAppSourceRequest) (*GitHubAppSource, error) {
	var result GitHubAppSource
	err := s.client.do(ctx, http.MethodPost, "/api/v1/github-apps", nil, req, &result)
	if err != nil {
		return nil, err
	}
	return &result, nil
}
