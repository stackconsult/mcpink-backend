package mcpserver

import (
	"context"
	"fmt"
	"strconv"

	"github.com/augustdev/autoclip/internal/deployments"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

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

func (s *Server) handleDeploy(ctx context.Context, req *mcp.CallToolRequest, input DeployInput) (*mcp.CallToolResult, DeployOutput, error) {
	user := UserFromContext(ctx)
	if user == nil {
		return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: "not authenticated"}}}, DeployOutput{}, nil
	}

	creds, err := s.authService.GetGitHubCredsByUserID(ctx, user.ID)
	if err != nil {
		return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: "Failed to get GitHub credentials."}}}, DeployOutput{}, nil
	}

	if creds.GithubAppInstallationID == nil {
		return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: "GitHub App not installed. Please install the GitHub App first."}}}, DeployOutput{}, nil
	}

	if input.Repo == "" {
		return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: "repo is required"}}}, DeployOutput{}, nil
	}
	if input.Branch == "" {
		return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: "branch is required"}}}, DeployOutput{}, nil
	}
	if input.Name == "" {
		return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: "name is required"}}}, DeployOutput{}, nil
	}

	if user.CoolifyGithubAppUuid == nil {
		return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: "GitHub App not installed. Please install the GitHub App first."}}}, DeployOutput{}, nil
	}

	githubAppUUID := *user.CoolifyGithubAppUuid

	// Default build pack is nixpacks
	buildPack := "nixpacks"
	if input.BuildPack != "" {
		switch input.BuildPack {
		case "nixpacks", "dockerfile", "static", "dockercompose", "docker-compose":
			buildPack = input.BuildPack
		default:
			return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("invalid build_pack: %s. Valid options: nixpacks, dockerfile, static, dockercompose", input.BuildPack)}}}, DeployOutput{}, nil
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
		ProjectName:   input.Project,
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
		return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("failed to start deployment: %v", err)}}}, DeployOutput{}, nil
	}

	output := DeployOutput{
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
			AppID:  app.ID,
			Name:   name,
			Status: app.BuildStatus,
			Repo:   app.Repo,
			URL:    app.Fqdn,
		}
	}

	return nil, ListAppsOutput{Apps: appInfos}, nil
}
