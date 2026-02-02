package mcpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/augustdev/autoclip/internal/deployments"
	"github.com/augustdev/autoclip/internal/resources"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func deref[T any](ptr *T) T {
	if ptr == nil {
		var zero T
		return zero
	}
	return *ptr
}

func (s *Server) handleWhoami(ctx context.Context, req *mcp.CallToolRequest, input WhoamiInput) (*mcp.CallToolResult, WhoamiOutput, error) {
	user := UserFromContext(ctx)
	if user == nil {
		return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: "not authenticated"}}}, WhoamiOutput{}, nil
	}

	hasGitHubApp := false
	creds, err := s.authService.GetGitHubCredsByUserID(ctx, user.ID)
	if err == nil && creds.GithubAppInstallationID != nil {
		hasGitHubApp = true
	}

	output := WhoamiOutput{
		UserID:         user.ID,
		GitHubUsername: user.GithubUsername,
		AvatarURL:      user.AvatarUrl,
		HasGitHubApp:   hasGitHubApp,
	}

	return nil, output, nil
}

func (s *Server) handleCreateApp(ctx context.Context, req *mcp.CallToolRequest, input CreateAppInput) (*mcp.CallToolResult, CreateAppOutput, error) {
	user := UserFromContext(ctx)
	if user == nil {
		return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: "not authenticated"}}}, CreateAppOutput{}, nil
	}

	creds, err := s.authService.GetGitHubCredsByUserID(ctx, user.ID)
	if err != nil {
		return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: "Failed to get GitHub credentials."}}}, CreateAppOutput{}, nil
	}

	if creds.GithubAppInstallationID == nil {
		return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: "GitHub App not installed. Please install the GitHub App first."}}}, CreateAppOutput{}, nil
	}

	if input.Repo == "" {
		return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: "repo is required"}}}, CreateAppOutput{}, nil
	}
	if input.Branch == "" {
		return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: "branch is required"}}}, CreateAppOutput{}, nil
	}
	if input.Name == "" {
		return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: "name is required"}}}, CreateAppOutput{}, nil
	}

	if user.CoolifyGithubAppUuid == nil {
		return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: "GitHub App not installed. Please install the GitHub App first."}}}, CreateAppOutput{}, nil
	}

	githubAppUUID := *user.CoolifyGithubAppUuid

	// Default build pack is nixpacks
	buildPack := "nixpacks"
	if input.BuildPack != "" {
		switch input.BuildPack {
		case "nixpacks", "dockerfile", "static", "dockercompose", "docker-compose":
			buildPack = input.BuildPack
		default:
			return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("invalid build_pack: %s. Valid options: nixpacks, dockerfile, static, dockercompose", input.BuildPack)}}}, CreateAppOutput{}, nil
		}
	}

	// Default port is 3000
	port := strconv.Itoa(DefaultPort)
	if input.Port > 0 {
		port = strconv.Itoa(input.Port)
	}

	// Convert env vars
	envVars := make([]deployments.EnvVar, len(input.EnvVars))
	for i, ev := range input.EnvVars {
		envVars[i] = deployments.EnvVar{
			Key:   ev.Key,
			Value: ev.Value,
		}
	}

	s.logger.Info("starting deployment",
		"user_id", user.ID,
		"project", input.Project,
		"repo", input.Repo,
		"branch", input.Branch,
		"build_pack", buildPack,
		"port", port,
	)

	result, err := s.deployService.CreateApp(ctx, deployments.CreateAppInput{
		UserID:        user.ID,
		ProjectRef:    input.Project,
		GitHubAppUUID: githubAppUUID,
		Repo:          input.Repo,
		Branch:        input.Branch,
		Name:          input.Name,
		BuildPack:     buildPack,
		Port:          port,
		EnvVars:       envVars,
	})
	if err != nil {
		s.logger.Error("failed to start deployment", "error", err)
		return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("failed to start deployment: %v", err)}}}, CreateAppOutput{}, nil
	}

	output := CreateAppOutput{
		AppID:   result.AppID,
		Name:    result.Name,
		Status:  result.Status,
		Repo:    result.Repo,
		Message: fmt.Sprintf("Deployment started (workflow_id: %s)", result.WorkflowID),
	}

	return nil, output, nil
}

func (s *Server) handleListApps(ctx context.Context, req *mcp.CallToolRequest, input ListAppsInput) (*mcp.CallToolResult, ListAppsOutput, error) {
	user := UserFromContext(ctx)
	if user == nil {
		return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: "not authenticated"}}}, ListAppsOutput{}, nil
	}

	apps, err := s.deployService.ListApps(ctx, user.ID, 100, 0)
	if err != nil {
		s.logger.Error("failed to list apps", "error", err)
		return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("failed to list apps: %v", err)}}}, ListAppsOutput{}, nil
	}

	appInfos := make([]AppInfo, len(apps))
	for i, app := range apps {
		name := ""
		if app.Name != nil {
			name = *app.Name
		}
		appInfos[i] = AppInfo{
			AppID:      app.ID,
			Name:       name,
			Status:     app.BuildStatus,
			Repo:       app.Repo,
			URL:        app.Fqdn,
			CommitHash: app.CommitHash,
		}
	}

	return nil, ListAppsOutput{Apps: appInfos}, nil
}

func (s *Server) handleRedeploy(ctx context.Context, req *mcp.CallToolRequest, input RedeployInput) (*mcp.CallToolResult, RedeployOutput, error) {
	user := UserFromContext(ctx)
	if user == nil {
		return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: "not authenticated"}}}, RedeployOutput{}, nil
	}

	if input.Name == "" {
		return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: "name is required"}}}, RedeployOutput{}, nil
	}

	// Get project (default to "default")
	projectRef := input.Project
	if projectRef == "" {
		projectRef = "default"
	}

	// Look up project by ref
	project, err := s.deployService.GetProjectByRef(ctx, user.ID, projectRef)
	if err != nil {
		return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("project not found: %s", projectRef)}}}, RedeployOutput{}, nil
	}

	// Look up app by name and project
	app, err := s.deployService.GetAppByNameAndProject(ctx, input.Name, project.ID)
	if err != nil {
		return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("app not found: %s", input.Name)}}}, RedeployOutput{}, nil
	}

	if app.CoolifyAppUuid == nil {
		return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: "app has not been deployed yet"}}}, RedeployOutput{}, nil
	}

	s.logger.Info("starting redeploy",
		"user_id", user.ID,
		"app_id", app.ID,
		"name", input.Name,
	)

	workflowID, err := s.deployService.RedeployApp(ctx, app.ID, *app.CoolifyAppUuid)
	if err != nil {
		s.logger.Error("failed to start redeploy", "error", err)
		return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("failed to start redeploy: %v", err)}}}, RedeployOutput{}, nil
	}

	name := ""
	if app.Name != nil {
		name = *app.Name
	}

	var commitHash string
	if app.CommitHash != nil {
		commitHash = *app.CommitHash
	}

	output := RedeployOutput{
		AppID:      app.ID,
		Name:       name,
		Status:     "building",
		CommitHash: commitHash,
		Message:    fmt.Sprintf("Redeploy started (workflow_id: %s)", workflowID),
	}

	return nil, output, nil
}

func (s *Server) handleCreateResource(ctx context.Context, req *mcp.CallToolRequest, input CreateResourceInput) (*mcp.CallToolResult, CreateResourceOutput, error) {
	user := UserFromContext(ctx)
	if user == nil {
		return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: "not authenticated"}}}, CreateResourceOutput{}, nil
	}

	// Validate required field: name
	if input.Name == "" {
		return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: "name is required"}}}, CreateResourceOutput{}, nil
	}

	// Check if resources service is available
	if s.resourcesService == nil {
		return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: "resource provisioning is not configured"}}}, CreateResourceOutput{}, nil
	}

	// Set defaults for optional fields
	dbType := DefaultDBType
	if input.Type != "" {
		if input.Type != "sqlite" {
			return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: "invalid type: only 'sqlite' is supported"}}}, CreateResourceOutput{}, nil
		}
		dbType = input.Type
	}

	size := DefaultDBSize
	if input.Size != "" {
		size = input.Size
	}

	region := DefaultRegion
	if input.Region != "" {
		if input.Region != "eu-west" {
			return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: "invalid region: only 'eu-west' is supported"}}}, CreateResourceOutput{}, nil
		}
		region = input.Region
	}

	s.logger.Info("creating resource",
		"user_id", user.ID,
		"name", input.Name,
		"type", dbType,
		"size", size,
		"region", region,
	)

	// Call the resources service to provision the database
	result, err := s.resourcesService.ProvisionDatabase(ctx, resources.ProvisionDatabaseInput{
		UserID:    user.ID,
		ProjectID: nil, // TODO: resolve project from projectRef if provided
		Name:      input.Name,
		Type:      dbType,
		Size:      size,
		Region:    region,
	})
	if err != nil {
		s.logger.Error("failed to create resource", "error", err)
		return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("failed to create resource: %v", err)}}}, CreateResourceOutput{}, nil
	}

	output := CreateResourceOutput{
		ResourceID: result.ResourceID,
		Name:       result.Name,
		Type:       result.Type,
		Region:     result.Region,
		URL:        result.URL,
		AuthToken:  result.AuthToken,
		Status:     result.Status,
	}

	return nil, output, nil
}

func (s *Server) handleListResources(ctx context.Context, req *mcp.CallToolRequest, input ListResourcesInput) (*mcp.CallToolResult, ListResourcesOutput, error) {
	user := UserFromContext(ctx)
	if user == nil {
		return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: "not authenticated"}}}, ListResourcesOutput{}, nil
	}

	if s.resourcesService == nil {
		return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: "resources service is not configured"}}}, ListResourcesOutput{}, nil
	}

	resources, err := s.resourcesService.ListResources(ctx, user.ID, 100, 0)
	if err != nil {
		s.logger.Error("failed to list resources", "error", err)
		return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("failed to list resources: %v", err)}}}, ListResourcesOutput{}, nil
	}

	resourceInfos := make([]ResourceInfo, len(resources))
	for i, r := range resources {
		resourceInfos[i] = ResourceInfo{
			ResourceID: r.ID,
			Name:       r.Name,
			Type:       r.Type,
			Region:     r.Region,
			Status:     r.Status,
			CreatedAt:  r.CreatedAt.Format(time.RFC3339),
		}
	}

	return nil, ListResourcesOutput{Resources: resourceInfos}, nil
}

func (s *Server) handleGetResourceDetails(ctx context.Context, req *mcp.CallToolRequest, input GetResourceDetailsInput) (*mcp.CallToolResult, GetResourceDetailsOutput, error) {
	user := UserFromContext(ctx)
	if user == nil {
		return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: "not authenticated"}}}, GetResourceDetailsOutput{}, nil
	}

	if input.Name == "" {
		return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: "name is required"}}}, GetResourceDetailsOutput{}, nil
	}

	if s.resourcesService == nil {
		return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: "resources service is not configured"}}}, GetResourceDetailsOutput{}, nil
	}

	resource, err := s.resourcesService.GetResourceByName(ctx, user.ID, input.Name)
	if err != nil {
		return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("resource not found: %s", input.Name)}}}, GetResourceDetailsOutput{}, nil
	}

	output := GetResourceDetailsOutput{
		ResourceID: resource.ID,
		Name:       resource.Name,
		Type:       resource.Type,
		Region:     resource.Region,
		Status:     resource.Status,
		CreatedAt:  resource.CreatedAt.Format(time.RFC3339),
		UpdatedAt:  resource.UpdatedAt.Format(time.RFC3339),
	}

	if resource.Credentials != nil {
		output.DatabaseURL = resource.Credentials.URL
		output.AuthToken = resource.Credentials.AuthToken
	}

	return nil, output, nil
}

func (s *Server) handleCreateGitHubRepo(ctx context.Context, req *mcp.CallToolRequest, input CreateGitHubRepoInput) (*mcp.CallToolResult, CreateGitHubRepoOutput, error) {
	user := UserFromContext(ctx)
	if user == nil {
		return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: "not authenticated"}}}, CreateGitHubRepoOutput{}, nil
	}

	if input.Name == "" {
		return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: "name is required"}}}, CreateGitHubRepoOutput{}, nil
	}

	creds, err := s.authService.GetGitHubCredsByUserID(ctx, user.ID)
	if err != nil {
		return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: "GitHub not connected. Please go to https://ml.ink/settings/github?q=repo"}}}, CreateGitHubRepoOutput{}, nil
	}

	if creds.GithubAppInstallationID == nil {
		return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: "GitHub App not installed. Please install at https://ml.ink/settings/github"}}}, CreateGitHubRepoOutput{}, nil
	}

	hasRepoScope := false
	for _, scope := range creds.GithubOauthScopes {
		if scope == "repo" {
			hasRepoScope = true
			break
		}
	}
	if !hasRepoScope {
		return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: "GitHub token missing 'repo' scope. Please re-authenticate at https://ml.ink/settings/github?q=repo"}}}, CreateGitHubRepoOutput{}, nil
	}

	oauthToken, err := s.authService.DecryptToken(creds.GithubOauthToken)
	if err != nil {
		s.logger.Error("failed to decrypt oauth token", "error", err)
		return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: "Failed to decrypt GitHub token. Please re-authenticate at https://ml.ink/settings/github?q=repo"}}}, CreateGitHubRepoOutput{}, nil
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
		return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: "Failed to prepare request"}}}, CreateGitHubRepoOutput{}, nil
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", "https://api.github.com/user/repos", bytes.NewReader(payloadBytes))
	if err != nil {
		return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: "Failed to create request"}}}, CreateGitHubRepoOutput{}, nil
	}

	httpReq.Header.Set("Authorization", fmt.Sprintf("Bearer %s", oauthToken))
	httpReq.Header.Set("Accept", "application/vnd.github+json")
	httpReq.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	httpReq.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(httpReq)
	if err != nil {
		s.logger.Error("failed to create github repo", "error", err)
		return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: "Failed to create repository"}}}, CreateGitHubRepoOutput{}, nil
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)

	if resp.StatusCode == http.StatusUnprocessableEntity {
		if strings.Contains(string(respBody), "name already exists") {
			return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("Repository '%s' already exists", input.Name)}}}, CreateGitHubRepoOutput{}, nil
		}
	}

	if resp.StatusCode != http.StatusCreated {
		s.logger.Error("github api error", "status", resp.StatusCode, "body", string(respBody))
		return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("GitHub API error: %s", string(respBody))}}}, CreateGitHubRepoOutput{}, nil
	}

	var repoResp struct {
		FullName string `json:"full_name"`
		Name     string `json:"name"`
	}
	if err := json.Unmarshal(respBody, &repoResp); err != nil {
		s.logger.Error("failed to parse github response", "error", err)
		return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: "Failed to parse GitHub response"}}}, CreateGitHubRepoOutput{}, nil
	}

	if s.githubAppService == nil {
		return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: "GitHub App service not configured"}}}, CreateGitHubRepoOutput{}, nil
	}

	installationToken, err := s.githubAppService.CreateInstallationToken(ctx, *creds.GithubAppInstallationID, []string{repoResp.Name})
	if err != nil {
		s.logger.Error("failed to create installation token", "error", err)
		return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: "Failed to create access token. The GitHub App may not have access to new repositories. Please check your installation settings."}}}, CreateGitHubRepoOutput{}, nil
	}

	output := CreateGitHubRepoOutput{
		RepoFullName: repoResp.FullName,
		AccessToken:  installationToken.Token,
	}

	return nil, output, nil
}

func (s *Server) handleGetGitHubPushToken(ctx context.Context, req *mcp.CallToolRequest, input GetGitHubPushTokenInput) (*mcp.CallToolResult, GetGitHubPushTokenOutput, error) {
	user := UserFromContext(ctx)
	if user == nil {
		return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: "not authenticated"}}}, GetGitHubPushTokenOutput{}, nil
	}

	if input.Repo == "" {
		return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: "repo is required"}}}, GetGitHubPushTokenOutput{}, nil
	}

	creds, err := s.authService.GetGitHubCredsByUserID(ctx, user.ID)
	if err != nil {
		return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: "GitHub not connected. Please go to https://ml.ink/settings/github"}}}, GetGitHubPushTokenOutput{}, nil
	}

	if creds.GithubAppInstallationID == nil {
		return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: "GitHub App not installed. Please install at https://ml.ink/settings/github"}}}, GetGitHubPushTokenOutput{}, nil
	}

	if s.githubAppService == nil {
		return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: "GitHub App service not configured"}}}, GetGitHubPushTokenOutput{}, nil
	}

	// Extract repo name from owner/repo format
	parts := strings.Split(input.Repo, "/")
	repoName := input.Repo
	if len(parts) == 2 {
		repoName = parts[1]
	}

	installationToken, err := s.githubAppService.CreateInstallationToken(ctx, *creds.GithubAppInstallationID, []string{repoName})
	if err != nil {
		s.logger.Error("failed to create installation token", "error", err)
		return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: "Failed to create access token. The GitHub App may not have access to this repository."}}}, GetGitHubPushTokenOutput{}, nil
	}

	expiresInMinutes := int(time.Until(installationToken.ExpiresAt).Minutes())

	return nil, GetGitHubPushTokenOutput{
		AccessToken:      installationToken.Token,
		ExpiresAt:        installationToken.ExpiresAt.UTC().Format(time.RFC3339),
		ExpiresInMinutes: expiresInMinutes,
	}, nil
}

func (s *Server) handleDebugGitHubApp(ctx context.Context, req *mcp.CallToolRequest, input DebugGitHubAppInput) (*mcp.CallToolResult, DebugGitHubAppOutput, error) {
	user := UserFromContext(ctx)
	if user == nil {
		return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: "not authenticated"}}}, DebugGitHubAppOutput{}, nil
	}

	creds, err := s.authService.GetGitHubCredsByUserID(ctx, user.ID)
	if err != nil {
		return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: "GitHub not connected"}}}, DebugGitHubAppOutput{}, nil
	}

	if creds.GithubAppInstallationID == nil {
		return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: "GitHub App not installed"}}}, DebugGitHubAppOutput{}, nil
	}

	if s.githubAppService == nil {
		return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: "GitHub App service not configured"}}}, DebugGitHubAppOutput{}, nil
	}

	info, err := s.githubAppService.GetInstallation(ctx, *creds.GithubAppInstallationID)
	if err != nil {
		s.logger.Error("failed to get installation info", "error", err)
		return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("Failed to get installation info: %v", err)}}}, DebugGitHubAppOutput{}, nil
	}

	return nil, DebugGitHubAppOutput{
		InstallationID:      info.ID,
		RepositorySelection: info.RepositorySelection,
		Permissions:         info.Permissions,
	}, nil
}

func (s *Server) handleGetAppDetails(ctx context.Context, req *mcp.CallToolRequest, input GetAppDetailsInput) (*mcp.CallToolResult, GetAppDetailsOutput, error) {
	user := UserFromContext(ctx)
	if user == nil {
		return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: "not authenticated"}}}, GetAppDetailsOutput{}, nil
	}

	if input.Name == "" {
		return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: "name is required"}}}, GetAppDetailsOutput{}, nil
	}

	project := "default"
	if input.Project != "" {
		project = input.Project
	}

	// Cap log lines at max
	runtimeLogLines := min(input.RuntimeLogLines, MaxLogLines)
	deployLogLines := min(input.DeployLogLines, MaxLogLines)

	// Look up app by name and project
	app, err := s.deployService.GetAppByName(ctx, deployments.GetAppByNameParams{
		Name:    input.Name,
		Project: project,
		UserID:  user.ID,
	})
	if err != nil {
		return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: err.Error()}}}, GetAppDetailsOutput{}, nil
	}

	runtimeStatus := "pending"
	if app.RuntimeStatus != nil {
		runtimeStatus = *app.RuntimeStatus
	}

	output := GetAppDetailsOutput{
		AppID:         app.ID,
		Name:          deref(app.Name),
		Project:       project,
		Repo:          app.Repo,
		Branch:        app.Branch,
		CommitHash:    deref(app.CommitHash),
		BuildStatus:   app.BuildStatus,
		RuntimeStatus: runtimeStatus,
		URL:           app.Fqdn,
		CreatedAt:     app.CreatedAt.Time.Format(time.RFC3339),
		UpdatedAt:     app.UpdatedAt.Time.Format(time.RFC3339),
		ErrorMessage:  app.ErrorMessage,
	}

	// Include env vars if requested
	if input.IncludeEnv {
		var envVars []EnvVar
		if err := json.Unmarshal(app.EnvVars, &envVars); err == nil {
			output.EnvVars = make([]EnvVarInfo, len(envVars))
			for i, ev := range envVars {
				output.EnvVars[i] = EnvVarInfo{Key: ev.Key, Value: ev.Value}
			}
		}
	}

	// Fetch logs via provider
	if s.logProvider != nil && app.CoolifyAppUuid != nil {
		if runtimeLogLines > 0 {
			logs, err := s.logProvider.GetRuntimeLogs(ctx, *app.CoolifyAppUuid, runtimeLogLines)
			if err != nil {
				s.logger.Warn("failed to fetch runtime logs", "error", err)
			} else {
				lines := make([]string, len(logs))
				for i, l := range logs {
					lines[i] = l.Message
				}
				output.RuntimeLogs = strings.Join(lines, "\n")
			}
		}

		if deployLogLines > 0 {
			logs, err := s.logProvider.GetDeploymentLogs(ctx, *app.CoolifyAppUuid)
			if err != nil {
				s.logger.Warn("failed to fetch deployment logs", "error", err)
			} else {
				// Limit to requested number of lines (from the end)
				if len(logs) > deployLogLines {
					logs = logs[len(logs)-deployLogLines:]
				}
				lines := make([]string, len(logs))
				for i, l := range logs {
					lines[i] = l.Message
				}
				output.DeploymentLogs = strings.Join(lines, "\n")
			}
		}
	}

	return nil, output, nil
}

func (s *Server) handleDeleteResource(ctx context.Context, req *mcp.CallToolRequest, input DeleteResourceInput) (*mcp.CallToolResult, DeleteResourceOutput, error) {
	user := UserFromContext(ctx)
	if user == nil {
		return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: "not authenticated"}}}, DeleteResourceOutput{}, nil
	}

	if input.Name == "" {
		return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: "name is required"}}}, DeleteResourceOutput{}, nil
	}

	if s.resourcesService == nil {
		return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: "resources service is not configured"}}}, DeleteResourceOutput{}, nil
	}

	resource, err := s.resourcesService.GetResourceByName(ctx, user.ID, input.Name)
	if err != nil {
		return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("resource not found: %s", input.Name)}}}, DeleteResourceOutput{}, nil
	}

	if err := s.resourcesService.DeleteResource(ctx, user.ID, resource.ID); err != nil {
		s.logger.Error("failed to delete resource", "error", err)
		return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("failed to delete resource: %v", err)}}}, DeleteResourceOutput{}, nil
	}

	return nil, DeleteResourceOutput{
		ResourceID: resource.ID,
		Name:       resource.Name,
		Message:    "Resource deleted successfully",
	}, nil
}

func (s *Server) handleDeleteApp(ctx context.Context, req *mcp.CallToolRequest, input DeleteAppInput) (*mcp.CallToolResult, DeleteAppOutput, error) {
	user := UserFromContext(ctx)
	if user == nil {
		return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: "not authenticated"}}}, DeleteAppOutput{}, nil
	}

	if input.Name == "" {
		return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: "name is required"}}}, DeleteAppOutput{}, nil
	}

	project := "default"
	if input.Project != "" {
		project = input.Project
	}

	result, err := s.deployService.DeleteApp(ctx, deployments.DeleteAppParams{
		Name:    input.Name,
		Project: project,
		UserID:  user.ID,
	})
	if err != nil {
		s.logger.Error("failed to delete app", "error", err)
		return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: err.Error()}}}, DeleteAppOutput{}, nil
	}

	return nil, DeleteAppOutput{
		AppID:   result.AppID,
		Name:    result.Name,
		Message: "App deleted successfully",
	}, nil
}
