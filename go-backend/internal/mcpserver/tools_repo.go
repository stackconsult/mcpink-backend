package mcpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strings"

	"github.com/augustdev/autoclip/internal/storage/pg/generated/users"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func (s *Server) handleCreateRepo(ctx context.Context, req *mcp.CallToolRequest, input CreateRepoInput) (*mcp.CallToolResult, CreateRepoOutput, error) {
	user := UserFromContext(ctx)
	if user == nil {
		return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: "not authenticated"}}}, CreateRepoOutput{}, nil
	}

	if input.Name == "" {
		return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: "name is required"}}}, CreateRepoOutput{}, nil
	}

	// Determine target from input (default: ml.ink)
	target := input.Target
	if target == "" {
		target = "ml.ink"
	}

	switch target {
	case "ml.ink":
		return s.createPrivateRepo(ctx, user.ID, input)
	case "github.com":
		return s.createGitHubRepo(ctx, user, input)
	default:
		return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: "target must be 'ml.ink' (default) or 'github.com'"}}}, CreateRepoOutput{}, nil
	}
}

func (s *Server) createPrivateRepo(ctx context.Context, userID string, input CreateRepoInput) (*mcp.CallToolResult, CreateRepoOutput, error) {
	if s.internalGitSvc == nil {
		return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: "internal git not configured"}}}, CreateRepoOutput{}, nil
	}

	private := true
	if input.Private != nil {
		private = *input.Private
	}

	result, err := s.internalGitSvc.CreateRepo(ctx, userID, input.Name, input.Description, private)
	if err != nil {
		s.logger.Error("failed to create internal repo", "error", err, "name", input.Name)
		return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("failed to create repository: %v", err)}}}, CreateRepoOutput{}, nil
	}

	return nil, CreateRepoOutput{
		Repo:      result.Repo,
		GitRemote: result.GitRemote,
		ExpiresAt: result.ExpiresAt,
		Message:   result.Message,
	}, nil
}

func (s *Server) createGitHubRepo(ctx context.Context, user *users.User, input CreateRepoInput) (*mcp.CallToolResult, CreateRepoOutput, error) {
	creds, err := s.authService.GetGitHubCredsByUserID(ctx, user.ID)
	if err != nil {
		return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: "GitHub not connected. Please go to https://ml.ink/settings/access"}}}, CreateRepoOutput{}, nil
	}

	if creds.GithubAppInstallationID == nil {
		return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: "GitHub App not installed. Please install at https://ml.ink/settings/github"}}}, CreateRepoOutput{}, nil
	}

	hasRepoScope := slices.Contains(creds.GithubOauthScopes, "repo")
	if !hasRepoScope {
		return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: "GitHub OAuth `repo` scope is missing. Please re-authenticate at https://ml.ink/settings/access"}}}, CreateRepoOutput{}, nil
	}

	oauthToken, err := s.authService.DecryptToken(creds.GithubOauthToken)
	if err != nil {
		s.logger.Error("failed to decrypt oauth token", "error", err)
		return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: "Failed to decrypt GitHub token. Please re-authenticate"}}}, CreateRepoOutput{}, nil
	}

	isPrivate := true
	if input.Private != nil {
		isPrivate = *input.Private
	}

	repoPayload := map[string]interface{}{
		"name":    input.Name,
		"private": isPrivate,
	}
	if input.Description != "" {
		repoPayload["description"] = input.Description
	}

	payloadBytes, err := json.Marshal(repoPayload)
	if err != nil {
		return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: "Failed to prepare request"}}}, CreateRepoOutput{}, nil
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", "https://api.github.com/user/repos", bytes.NewReader(payloadBytes))
	if err != nil {
		return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: "Failed to create request"}}}, CreateRepoOutput{}, nil
	}

	httpReq.Header.Set("Authorization", fmt.Sprintf("Bearer %s", oauthToken))
	httpReq.Header.Set("Accept", "application/vnd.github+json")
	httpReq.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	httpReq.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(httpReq)
	if err != nil {
		s.logger.Error("failed to create github repo", "error", err)
		return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: "Failed to create repository"}}}, CreateRepoOutput{}, nil
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)

	if resp.StatusCode == http.StatusUnprocessableEntity {
		if strings.Contains(string(respBody), "name already exists") {
			return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("Repository '%s' already exists", input.Name)}}}, CreateRepoOutput{}, nil
		}
	}

	if resp.StatusCode != http.StatusCreated {
		s.logger.Error("github api error", "status", resp.StatusCode, "body", string(respBody))
		return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("GitHub API error: %s", string(respBody))}}}, CreateRepoOutput{}, nil
	}

	var repoResp struct {
		FullName string `json:"full_name"`
		Name     string `json:"name"`
	}
	if err := json.Unmarshal(respBody, &repoResp); err != nil {
		s.logger.Error("failed to parse github response", "error", err)
		return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: "Failed to parse GitHub response"}}}, CreateRepoOutput{}, nil
	}

	installationToken, err := s.githubAppService.CreateInstallationToken(ctx, *creds.GithubAppInstallationID, []string{repoResp.Name})
	if err != nil {
		s.logger.Error("failed to create installation token", "error", err)
		return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: "Failed to create access token. The GitHub App may not have access to new repositories."}}}, CreateRepoOutput{}, nil
	}

	return nil, CreateRepoOutput{
		Repo:      fmt.Sprintf("github.com/%s", repoResp.FullName),
		GitRemote: fmt.Sprintf("https://x-access-token:%s@github.com/%s.git", installationToken.Token, repoResp.FullName),
		ExpiresAt: installationToken.ExpiresAt.Format("2006-01-02T15:04:05Z"),
		Message:   "Push your code, then call create_app to deploy",
	}, nil
}

func (s *Server) handleGetPushToken(ctx context.Context, req *mcp.CallToolRequest, input GetPushTokenInput) (*mcp.CallToolResult, GetPushTokenOutput, error) {
	user := UserFromContext(ctx)
	if user == nil {
		return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: "not authenticated"}}}, GetPushTokenOutput{}, nil
	}

	if input.Repo == "" {
		return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: "repo is required"}}}, GetPushTokenOutput{}, nil
	}

	// Determine source from repo path
	if strings.HasPrefix(input.Repo, "github.com/") {
		return s.getGitHubPushToken(ctx, user.ID, input.Repo)
	}

	if strings.HasPrefix(input.Repo, "ml.ink/") {
		return s.getPrivatePushToken(ctx, user.ID, input.Repo)
	}

	return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: "repo must start with 'github.com/' or 'ml.ink/'"}}}, GetPushTokenOutput{}, nil
}

func (s *Server) getPrivatePushToken(ctx context.Context, userID, repoPath string) (*mcp.CallToolResult, GetPushTokenOutput, error) {
	if s.internalGitSvc == nil {
		return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: "internal git not configured"}}}, GetPushTokenOutput{}, nil
	}

	// Extract full_name from path (ml.ink/username/repo -> username/repo)
	fullName := strings.TrimPrefix(repoPath, "ml.ink/")

	result, err := s.internalGitSvc.GetPushToken(ctx, userID, fullName)
	if err != nil {
		s.logger.Error("failed to get push token", "error", err, "repo", fullName)
		return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("failed to get push token: %v", err)}}}, GetPushTokenOutput{}, nil
	}

	return nil, GetPushTokenOutput{
		GitRemote: result.GitRemote,
		ExpiresAt: result.ExpiresAt,
	}, nil
}

func (s *Server) getGitHubPushToken(ctx context.Context, userID, repoPath string) (*mcp.CallToolResult, GetPushTokenOutput, error) {
	creds, err := s.authService.GetGitHubCredsByUserID(ctx, userID)
	if err != nil {
		return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: "GitHub not connected"}}}, GetPushTokenOutput{}, nil
	}

	if creds.GithubAppInstallationID == nil {
		return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: "GitHub App not installed"}}}, GetPushTokenOutput{}, nil
	}

	// Extract repo from github.com/owner/repo format
	repo := strings.TrimPrefix(repoPath, "github.com/")
	parts := strings.Split(repo, "/")
	repoName := repo
	if len(parts) == 2 {
		repoName = parts[1]
	}

	installationToken, err := s.githubAppService.CreateInstallationToken(ctx, *creds.GithubAppInstallationID, []string{repoName})
	if err != nil {
		s.logger.Error("failed to get GitHub push token", "error", err, "repo", repo)
		return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("failed to get push token: %v", err)}}}, GetPushTokenOutput{}, nil
	}

	return nil, GetPushTokenOutput{
		GitRemote: fmt.Sprintf("https://x-access-token:%s@github.com/%s.git", installationToken.Token, repo),
		ExpiresAt: installationToken.ExpiresAt.Format("2006-01-02T15:04:05Z"),
	}, nil
}
