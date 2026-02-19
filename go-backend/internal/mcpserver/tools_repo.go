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

	host := input.Host
	if host == "" {
		host = "ml.ink"
	}

	switch host {
	case "ml.ink":
		return s.createPrivateRepo(ctx, user.ID, input)
	case "github.com":
		return s.createGitHubRepo(ctx, user, input)
	default:
		return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: "host must be 'ml.ink' (default) or 'github.com'"}}}, CreateRepoOutput{}, nil
	}
}

func (s *Server) createPrivateRepo(ctx context.Context, userID string, input CreateRepoInput) (*mcp.CallToolResult, CreateRepoOutput, error) {
	projectRef := input.Project
	if projectRef == "" {
		projectRef = "default"
	}

	project, err := s.deployService.GetProjectByRef(ctx, userID, projectRef)
	if err != nil {
		return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("project not found: %s", projectRef)}}}, CreateRepoOutput{}, nil
	}

	result, err := s.internalGitSvc.CreateRepo(ctx, userID, project.ID, input.Name, input.Description, true)
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

	if creds.GithubOauthToken == nil {
		return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: "No GitHub OAuth token found. Please authenticate at https://ml.ink/settings/access"}}}, CreateRepoOutput{}, nil
	}
	oauthToken, err := s.authService.DecryptToken(*creds.GithubOauthToken)
	if err != nil {
		s.logger.Error("failed to decrypt oauth token", "error", err)
		return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: "Failed to decrypt GitHub token. Please re-authenticate"}}}, CreateRepoOutput{}, nil
	}

	repoPayload := map[string]any{
		"name":    input.Name,
		"private": true,
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
		Message:   "Push your code, then call create_service to deploy",
	}, nil
}

func (s *Server) handleGetGitToken(ctx context.Context, req *mcp.CallToolRequest, input GetGitTokenInput) (*mcp.CallToolResult, GetGitTokenOutput, error) {
	user := UserFromContext(ctx)
	if user == nil {
		return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: "not authenticated"}}}, GetGitTokenOutput{}, nil
	}

	if input.Name == "" {
		return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: "name is required"}}}, GetGitTokenOutput{}, nil
	}

	host := input.Host
	if host == "" {
		host = "ml.ink"
	}

	switch host {
	case "ml.ink":
		return s.getPrivateGitToken(ctx, user, input)
	case "github.com":
		return s.getGitHubGitToken(ctx, user, input.Name)
	default:
		return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: "host must be 'ml.ink' (default) or 'github.com'"}}}, GetGitTokenOutput{}, nil
	}
}

func (s *Server) getPrivateGitToken(ctx context.Context, user *users.User, input GetGitTokenInput) (*mcp.CallToolResult, GetGitTokenOutput, error) {
	projectRef := input.Project
	if projectRef == "" {
		projectRef = "default"
	}

	project, err := s.deployService.GetProjectByRef(ctx, user.ID, projectRef)
	if err != nil {
		return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("project not found: %s", projectRef)}}}, GetGitTokenOutput{}, nil
	}

	repo, err := s.internalGitSvc.GetRepoByProjectAndName(ctx, project.ID, input.Name)
	if err != nil {
		return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("repo '%s' not found in project '%s'. Create it first with create_repo", input.Name, projectRef)}}}, GetGitTokenOutput{}, nil
	}

	result, err := s.internalGitSvc.GetPushToken(ctx, user.ID, repo.FullName)
	if err != nil {
		s.logger.Error("failed to get git token", "error", err, "repo", repo.FullName)
		return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("failed to get git token: %v", err)}}}, GetGitTokenOutput{}, nil
	}

	return nil, GetGitTokenOutput{
		GitRemote: result.GitRemote,
		ExpiresAt: result.ExpiresAt,
	}, nil
}

func (s *Server) getGitHubGitToken(ctx context.Context, user *users.User, repoName string) (*mcp.CallToolResult, GetGitTokenOutput, error) {
	creds, err := s.authService.GetGitHubCredsByUserID(ctx, user.ID)
	if err != nil {
		return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: "GitHub not connected"}}}, GetGitTokenOutput{}, nil
	}

	if creds.GithubAppInstallationID == nil {
		return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: "GitHub App not installed"}}}, GetGitTokenOutput{}, nil
	}

	installationToken, err := s.githubAppService.CreateInstallationToken(ctx, *creds.GithubAppInstallationID, []string{repoName})
	if err != nil {
		s.logger.Error("failed to get GitHub git token", "error", err, "repo", repoName)
		return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("failed to get git token: %v", err)}}}, GetGitTokenOutput{}, nil
	}

	ghUser := ""
	if user.GithubUsername != nil {
		ghUser = *user.GithubUsername
	}
	fullRepo := fmt.Sprintf("%s/%s", ghUser, repoName)

	return nil, GetGitTokenOutput{
		GitRemote: fmt.Sprintf("https://x-access-token:%s@github.com/%s.git", installationToken.Token, fullRepo),
		ExpiresAt: installationToken.ExpiresAt.Format("2006-01-02T15:04:05Z"),
	}, nil
}
