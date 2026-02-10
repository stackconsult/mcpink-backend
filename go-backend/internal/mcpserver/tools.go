package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/augustdev/autoclip/internal/deployments"
	"github.com/augustdev/autoclip/internal/helpers"
	"github.com/augustdev/autoclip/internal/resources"
	"github.com/augustdev/autoclip/internal/storage/pg/generated/users"
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

func (s *Server) handleCreateService(ctx context.Context, req *mcp.CallToolRequest, input CreateServiceInput) (*mcp.CallToolResult, CreateServiceOutput, error) {
	user := UserFromContext(ctx)
	if user == nil {
		return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: "not authenticated"}}}, CreateServiceOutput{}, nil
	}

	if input.Repo == "" {
		return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: "repo is required"}}}, CreateServiceOutput{}, nil
	}
	if input.Branch == "" {
		input.Branch = "main"
	}
	if input.Name == "" {
		return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: "name is required"}}}, CreateServiceOutput{}, nil
	}

	host, repo, err := s.normalizeServiceRepo(ctx, user, input)
	if err != nil {
		return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: err.Error()}}}, CreateServiceOutput{}, nil
	}
	input.Repo = repo

	buildPack := "nixpacks"
	if input.BuildPack != "" {
		switch input.BuildPack {
		case "nixpacks", "dockerfile", "static", "dockercompose":
			buildPack = input.BuildPack
		default:
			return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("invalid build_pack: %s. Valid options: nixpacks, dockerfile, static, dockercompose", input.BuildPack)}}}, CreateServiceOutput{}, nil
		}
	}

	port := strconv.Itoa(DefaultPort)
	if buildPack == "static" {
		port = "80"
	}
	if input.Port > 0 {
		port = strconv.Itoa(input.Port)
	}

	envVars := make([]deployments.EnvVar, len(input.EnvVars))
	for i, ev := range input.EnvVars {
		envVars[i] = deployments.EnvVar{
			Key:   ev.Key,
			Value: ev.Value,
		}
	}

	isInternalGit := host == "ml.ink"

	s.logger.Info("starting deployment",
		"user_id", user.ID,
		"project", input.Project,
		"repo", input.Repo,
		"branch", input.Branch,
		"build_pack", buildPack,
		"port", port,
		"is_internal_git", isInternalGit,
	)

	var result *deployments.CreateAppResult

	if host == "ml.ink" {
		result, err = s.createServiceFromInternalGit(ctx, user.ID, input, buildPack, port, envVars)
	} else {
		result, err = s.createServiceFromGitHub(ctx, user, input, buildPack, port, envVars)
	}

	if err != nil {
		s.logger.Error("failed to start deployment", "error", err)
		return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("failed to start deployment: %v", err)}}}, CreateServiceOutput{}, nil
	}

	output := CreateServiceOutput{
		ServiceID: result.AppID,
		Name:      result.Name,
		Status:    result.Status,
		Repo:      result.Repo,
		Message:   fmt.Sprintf("Deployment started (workflow_id: %s)", result.WorkflowID),
	}

	return nil, output, nil
}

func (s *Server) normalizeServiceRepo(ctx context.Context, user *users.User, input CreateServiceInput) (host string, normalizedRepo string, err error) {
	repo := strings.TrimSpace(input.Repo)
	if repo == "" {
		return "", "", fmt.Errorf("repo is required")
	}

	host = strings.ToLower(strings.TrimSpace(input.Host))
	if host == "" {
		host = "ml.ink"
	}
	if host != "ml.ink" && host != "github.com" {
		return "", "", fmt.Errorf("invalid host: %s (valid: ml.ink, github.com)", host)
	}

	// Reject URLs, credentials, and paths - only accept simple repo name
	if strings.Contains(repo, "://") || strings.HasPrefix(repo, "git@") {
		return "", "", fmt.Errorf("invalid repo format: pass a repo name (e.g. 'myapp') not a URL")
	}
	if strings.Contains(repo, "@") {
		return "", "", fmt.Errorf("invalid repo format: do not include credentials; pass a repo name (e.g. 'myapp')")
	}
	if strings.Contains(repo, "/") {
		return "", "", fmt.Errorf("invalid repo format: pass repo name only (e.g. 'myapp'), not owner/repo")
	}

	if host == "ml.ink" {
		fullName, resolveErr := s.internalGitSvc.ResolveRepoFullName(ctx, user.ID, repo)
		if resolveErr != nil {
			return "", "", fmt.Errorf("resolve repo name: %w", resolveErr)
		}
		return host, fmt.Sprintf("ml.ink/%s", fullName), nil
	}

	// github.com path — requires GithubUsername
	username := ""
	if user != nil && user.GithubUsername != nil {
		username = strings.TrimSpace(*user.GithubUsername)
	}
	if username == "" {
		return "", "", fmt.Errorf("cannot resolve repo: user has no GitHub username configured")
	}
	return host, fmt.Sprintf("%s/%s", username, repo), nil
}

func (s *Server) createServiceFromGitHub(ctx context.Context, user *users.User, input CreateServiceInput, buildPack, port string, envVars []deployments.EnvVar) (*deployments.CreateAppResult, error) {
	creds, err := s.authService.GetGitHubCredsByUserID(ctx, user.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to get GitHub credentials")
	}

	if creds.GithubAppInstallationID == nil {
		return nil, fmt.Errorf("GitHub App not installed. Please install the GitHub App first")
	}

	repo := strings.TrimPrefix(input.Repo, "github.com/")

	return s.deployService.CreateApp(ctx, deployments.CreateAppInput{
		UserID:         user.ID,
		ProjectRef:     input.Project,
		Repo:           repo,
		Branch:         input.Branch,
		Name:           input.Name,
		BuildPack:      buildPack,
		Port:           port,
		EnvVars:        envVars,
		GitProvider:    "github",
		Memory:         input.Memory,
		CPU:            input.CPU,
		InstallCommand: input.InstallCommand,
		BuildCommand:   input.BuildCommand,
		StartCommand:   input.StartCommand,
		InstallationID: *creds.GithubAppInstallationID,
	})
}

func (s *Server) createServiceFromInternalGit(ctx context.Context, userID string, input CreateServiceInput, buildPack, port string, envVars []deployments.EnvVar) (*deployments.CreateAppResult, error) {
	fullName := strings.TrimPrefix(strings.TrimSpace(input.Repo), "ml.ink/")
	parts := strings.Split(fullName, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return nil, fmt.Errorf("invalid repo format: expected owner/repo, got %s", fullName)
	}

	internalRepo, err := s.internalGitSvc.GetRepoByFullName(ctx, fullName)
	if err != nil {
		return nil, fmt.Errorf("internal repo not found: %s", fullName)
	}

	if internalRepo.UserID != userID {
		return nil, fmt.Errorf("repo belongs to another user")
	}

	return s.deployService.CreateApp(ctx, deployments.CreateAppInput{
		UserID:         userID,
		ProjectRef:     input.Project,
		Repo:           fullName,
		Branch:         input.Branch,
		Name:           input.Name,
		BuildPack:      buildPack,
		Port:           port,
		EnvVars:        envVars,
		GitProvider:    "gitea",
		Memory:         input.Memory,
		CPU:            input.CPU,
		InstallCommand: input.InstallCommand,
		BuildCommand:   input.BuildCommand,
		StartCommand:   input.StartCommand,
	})
}

func (s *Server) handleListServices(ctx context.Context, req *mcp.CallToolRequest, input ListServicesInput) (*mcp.CallToolResult, ListServicesOutput, error) {
	user := UserFromContext(ctx)
	if user == nil {
		return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: "not authenticated"}}}, ListServicesOutput{}, nil
	}

	apps, err := s.deployService.ListApps(ctx, user.ID, 100, 0)
	if err != nil {
		s.logger.Error("failed to list services", "error", err)
		return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("failed to list services: %v", err)}}}, ListServicesOutput{}, nil
	}

	services := make([]ServiceInfo, len(apps))
	for i, app := range apps {
		name := ""
		if app.Name != nil {
			name = *app.Name
		}
		services[i] = ServiceInfo{
			ServiceID:  app.ID,
			Name:       name,
			Status:     app.BuildStatus,
			Repo:       app.Repo,
			URL:        app.Fqdn,
			CommitHash: app.CommitHash,
		}
	}

	return nil, ListServicesOutput{Services: services}, nil
}

func (s *Server) handleRedeployService(ctx context.Context, req *mcp.CallToolRequest, input RedeployServiceInput) (*mcp.CallToolResult, RedeployServiceOutput, error) {
	user := UserFromContext(ctx)
	if user == nil {
		return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: "not authenticated"}}}, RedeployServiceOutput{}, nil
	}

	if input.Name == "" {
		return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: "name is required"}}}, RedeployServiceOutput{}, nil
	}

	projectRef := input.Project
	if projectRef == "" {
		projectRef = "default"
	}

	project, err := s.deployService.GetProjectByRef(ctx, user.ID, projectRef)
	if err != nil {
		return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("project not found: %s", projectRef)}}}, RedeployServiceOutput{}, nil
	}

	app, err := s.deployService.GetAppByNameAndProject(ctx, input.Name, project.ID)
	if err != nil {
		return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("service not found: %s", input.Name)}}}, RedeployServiceOutput{}, nil
	}

	s.logger.Info("starting redeploy",
		"user_id", user.ID,
		"app_id", app.ID,
		"name", input.Name,
	)

	workflowID, err := s.deployService.RedeployApp(ctx, app.ID)
	if err != nil {
		s.logger.Error("failed to start redeploy", "error", err)
		return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("failed to start redeploy: %v", err)}}}, RedeployServiceOutput{}, nil
	}

	name := ""
	if app.Name != nil {
		name = *app.Name
	}

	var commitHash string
	if app.CommitHash != nil {
		commitHash = *app.CommitHash
	}

	output := RedeployServiceOutput{
		ServiceID:  app.ID,
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

	if input.Name == "" {
		return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: "name is required"}}}, CreateResourceOutput{}, nil
	}

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

	result, err := s.resourcesService.ProvisionDatabase(ctx, resources.ProvisionDatabaseInput{
		UserID:    user.ID,
		ProjectID: nil,
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

func (s *Server) handleGetService(ctx context.Context, req *mcp.CallToolRequest, input GetServiceInput) (*mcp.CallToolResult, GetServiceOutput, error) {
	user := UserFromContext(ctx)
	if user == nil {
		return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: "not authenticated"}}}, GetServiceOutput{}, nil
	}

	if input.Name == "" {
		return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: "name is required"}}}, GetServiceOutput{}, nil
	}

	project := "default"
	if input.Project != "" {
		project = input.Project
	}

	app, err := s.deployService.GetAppByName(ctx, deployments.GetAppByNameParams{
		Name:    input.Name,
		Project: project,
		UserID:  user.ID,
	})
	if err != nil {
		return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: err.Error()}}}, GetServiceOutput{}, nil
	}

	runtimeStatus := "pending"
	if app.RuntimeStatus != nil {
		runtimeStatus = *app.RuntimeStatus
	}

	output := GetServiceOutput{
		ServiceID:     app.ID,
		Name:          helpers.Deref(app.Name),
		Project:       project,
		Repo:          app.Repo,
		Branch:        app.Branch,
		CommitHash:    helpers.Deref(app.CommitHash),
		BuildStatus:   app.BuildStatus,
		RuntimeStatus: runtimeStatus,
		URL:           app.Fqdn,
		CreatedAt:     app.CreatedAt.Time.Format(time.RFC3339),
		UpdatedAt:     app.UpdatedAt.Time.Format(time.RFC3339),
		ErrorMessage:  app.ErrorMessage,
	}

	if len(app.BuildProgress) > 0 {
		var progress BuildProgress
		if err := json.Unmarshal(app.BuildProgress, &progress); err == nil {
			output.BuildProgress = &progress
		}
	}

	if input.IncludeEnv {
		var envVars []EnvVar
		if err := json.Unmarshal(app.EnvVars, &envVars); err == nil {
			output.EnvVars = make([]EnvVarInfo, len(envVars))
			for i, ev := range envVars {
				output.EnvVars[i] = EnvVarInfo{Key: ev.Key, Value: ev.Value}
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

func (s *Server) handleDeleteService(ctx context.Context, req *mcp.CallToolRequest, input DeleteServiceInput) (*mcp.CallToolResult, DeleteServiceOutput, error) {
	user := UserFromContext(ctx)
	if user == nil {
		return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: "not authenticated"}}}, DeleteServiceOutput{}, nil
	}

	if input.Name == "" {
		return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: "name is required"}}}, DeleteServiceOutput{}, nil
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
		s.logger.Error("failed to delete service", "error", err)
		return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: err.Error()}}}, DeleteServiceOutput{}, nil
	}

	return nil, DeleteServiceOutput{
		ServiceID: result.AppID,
		Name:      result.Name,
		Message:   "Service deleted successfully",
	}, nil
}
