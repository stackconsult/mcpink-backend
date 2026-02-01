package mcpserver

import (
	"context"
	"fmt"
	"strconv"

	"github.com/augustdev/autoclip/internal/coolify"
	"github.com/augustdev/autoclip/internal/helpers"
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

	// Get GitHub App UUID from user or config
	githubAppUUID := s.coolifyClient.Config().GitHubAppUUID
	if user.CoolifyGithubAppUuid != nil {
		githubAppUUID = *user.CoolifyGithubAppUuid
	}

	// Default build pack is nixpacks
	buildPack := coolify.BuildPackNixpacks
	if input.BuildPack != "" {
		switch input.BuildPack {
		case "nixpacks":
			buildPack = coolify.BuildPackNixpacks
		case "dockerfile":
			buildPack = coolify.BuildPackDockerfile
		case "static":
			buildPack = coolify.BuildPackStatic
		case "dockercompose", "docker-compose":
			buildPack = coolify.BuildPackDockerCompose
		default:
			return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("invalid build_pack: %s. Valid options: nixpacks, dockerfile, static, dockercompose", input.BuildPack)}}}, DeployOutput{}, nil
		}
	}

	// Default port is 3000
	port := strconv.Itoa(DefaultPort)
	if input.Port > 0 {
		port = strconv.Itoa(input.Port)
	}

	coolifyReq := &coolify.CreatePrivateGitHubAppRequest{
		ProjectUUID:     s.coolifyClient.Config().ProjectUUID,
		ServerUUID:      s.coolifyClient.GetMuscleServer(),
		EnvironmentName: s.coolifyClient.Config().EnvironmentName,
		GitHubAppUUID:   githubAppUUID,
		GitRepository:   input.Repo,
		GitBranch:       input.Branch,
		BuildPack:       buildPack,
		PortsExposes:    port,
	}

	if input.Name != "" {
		coolifyReq.Name = input.Name
	}
	if input.Memory != "" {
		coolifyReq.LimitsMemory = input.Memory
	}
	if input.CPU != "" {
		coolifyReq.LimitsCPUs = input.CPU
	}
	if input.InstallCommand != "" {
		coolifyReq.InstallCommand = input.InstallCommand
	}
	if input.BuildCommand != "" {
		coolifyReq.BuildCommand = input.BuildCommand
	}
	if input.StartCommand != "" {
		coolifyReq.StartCommand = input.StartCommand
	}

	// Default instant_deploy is true
	if input.InstantDeploy != nil {
		coolifyReq.InstantDeploy = input.InstantDeploy
	} else {
		coolifyReq.InstantDeploy = helpers.Ptr(true)
	}

	s.logger.Info("creating application",
		"user_id", user.ID,
		"repo", input.Repo,
		"branch", input.Branch,
		"build_pack", buildPack,
		"port", port,
	)

	createResp, err := s.coolifyClient.Applications.CreatePrivateGitHubApp(ctx, coolifyReq)
	if err != nil {
		s.logger.Error("failed to create application", "error", err)
		return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("failed to create application: %v", err)}}}, DeployOutput{}, nil
	}

	// Handle environment variables
	if len(input.EnvVars) > 0 {
		envReqs := make([]coolify.CreateEnvRequest, len(input.EnvVars))
		for i, ev := range input.EnvVars {
			envReqs[i] = coolify.CreateEnvRequest{
				Key:   ev.Key,
				Value: ev.Value,
			}
		}
		if err := s.coolifyClient.Applications.BulkUpdateEnvs(ctx, createResp.UUID, &coolify.BulkUpdateEnvsRequest{
			Data: envReqs,
		}); err != nil {
			s.logger.Error("failed to set environment variables", "error", err, "uuid", createResp.UUID)
		}
	}

	startResp, err := s.coolifyClient.Applications.Start(ctx, createResp.UUID, nil)
	if err != nil {
		s.logger.Error("failed to start deployment", "error", err)
		return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("failed to start deployment: %v", err)}}}, DeployOutput{}, nil
	}

	app, err := s.coolifyClient.Applications.Get(ctx, createResp.UUID)
	if err != nil {
		s.logger.Error("failed to get application", "error", err)
		return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("failed to get application: %v", err)}}}, DeployOutput{}, nil
	}

	output := DeployOutput{
		DeploymentUUID: startResp.DeploymentUUID,
		UUID:           app.UUID,
		Name:           app.Name,
		Status:         app.Status,
		FQDN:           app.FQDN,
		Message:        "Deployment started",
	}

	return nil, output, nil
}
