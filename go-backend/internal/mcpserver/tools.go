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

	output := WhoamiOutput{
		UserID:         user.ID,
		GitHubUsername: user.GithubUsername,
		AvatarURL:      user.AvatarUrl,
		HasGitHubApp:   user.GithubAppInstallationID != nil,
	}

	return nil, output, nil
}

func (s *Server) handleDeploy(ctx context.Context, req *mcp.CallToolRequest, input DeployInput) (*mcp.CallToolResult, DeployOutput, error) {
	user := UserFromContext(ctx)
	if user == nil {
		return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: "not authenticated"}}}, DeployOutput{}, nil
	}

	if user.GithubAppInstallationID == nil {
		return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: "GitHub App not installed. Please install the GitHub App first."}}}, DeployOutput{}, nil
	}

	if input.Repo == "" {
		return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: "repo is required"}}}, DeployOutput{}, nil
	}
	if input.Branch == "" {
		return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: "branch is required"}}}, DeployOutput{}, nil
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
		"repo", input.Repo,
		"branch", input.Branch,
		"build_pack", buildPack,
		"port", port,
	)

	result, err := s.deployService.CreateApp(ctx, deployments.CreateAppInput{
		UserID:        user.ID,
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
		Status:  "queued",
		Message: fmt.Sprintf("Deployment started (workflow_id: %s)", result.WorkflowID),
	}

	return nil, output, nil
}
