package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/augustdev/autoclip/internal/deployments"
	"github.com/augustdev/autoclip/internal/dns"
	"github.com/augustdev/autoclip/internal/helpers"
	"github.com/augustdev/autoclip/internal/k8sdeployments"
	"github.com/augustdev/autoclip/internal/resources"
	"github.com/augustdev/autoclip/internal/storage/pg/generated/users"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func resolveServicePort(buildPack, publishDir string, requestedPort *int) string {
	// Railpack static serving always binds nginx on 8080.
	if buildPack == "railpack" && publishDir != "" {
		return "8080"
	}
	if requestedPort != nil && *requestedPort > 0 {
		return strconv.Itoa(*requestedPort)
	}
	switch buildPack {
	case "static":
		return "80"
	case "dockerfile":
		return "" // defer to EXPOSE detection in ResolveBuildContext
	default:
		return strconv.Itoa(DefaultPort)
	}
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

func (s *Server) handleListProjects(ctx context.Context, req *mcp.CallToolRequest, input ListProjectsInput) (*mcp.CallToolResult, ListProjectsOutput, error) {
	user := UserFromContext(ctx)
	if user == nil {
		return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: "not authenticated"}}}, ListProjectsOutput{}, nil
	}

	projs, err := s.deployService.ListProjects(ctx, user.ID, 100, 0)
	if err != nil {
		s.logger.Error("failed to list projects", "error", err)
		return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("failed to list projects: %v", err)}}}, ListProjectsOutput{}, nil
	}

	projects := make([]ProjectInfo, len(projs))
	for i, p := range projs {
		projects[i] = ProjectInfo{
			ProjectID: p.ID,
			Name:      p.Name,
			Ref:       p.Ref,
			IsDefault: p.IsDefault,
			CreatedAt: p.CreatedAt.Time.Format(time.RFC3339),
		}
	}

	return nil, ListProjectsOutput{Projects: projects}, nil
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

	buildPack := "railpack"
	if input.BuildPack != "" {
		switch input.BuildPack {
		case "railpack", "dockerfile", "static", "dockercompose":
			buildPack = input.BuildPack
		default:
			return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("invalid build_pack: %s. Valid options: railpack, dockerfile, static, dockercompose", input.BuildPack)}}}, CreateServiceOutput{}, nil
		}
	}

	// Validate and sanitize publish_directory
	publishDir := strings.TrimSpace(input.PublishDirectory)
	if publishDir != "" {
		if buildPack != "railpack" {
			return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: "publish_directory is only supported with build_pack=railpack"}}}, CreateServiceOutput{}, nil
		}
		publishDir = strings.Trim(publishDir, "/")
		if publishDir == "" || strings.Contains(publishDir, "..") || filepath.IsAbs(input.PublishDirectory) {
			return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: "invalid publish_directory: must be a relative path without '..'"}}}, CreateServiceOutput{}, nil
		}
		input.PublishDirectory = publishDir
	}

	// Validate and sanitize root_directory
	rootDir := strings.TrimSpace(input.RootDirectory)
	if rootDir != "" {
		rootDir = strings.Trim(rootDir, "/")
		if rootDir == "" || strings.Contains(rootDir, "..") || filepath.IsAbs(input.RootDirectory) {
			return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: "invalid root_directory: must be a relative path without '..'"}}}, CreateServiceOutput{}, nil
		}
		input.RootDirectory = rootDir
	}

	// Validate and sanitize dockerfile_path
	dockerfilePath := strings.TrimSpace(input.DockerfilePath)
	if dockerfilePath != "" {
		if buildPack != "dockerfile" {
			return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: "dockerfile_path is only supported with build_pack=dockerfile"}}}, CreateServiceOutput{}, nil
		}
		dockerfilePath = strings.Trim(dockerfilePath, "/")
		if dockerfilePath == "" || strings.Contains(dockerfilePath, "..") || filepath.IsAbs(input.DockerfilePath) {
			return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: "invalid dockerfile_path: must be a relative path without '..'"}}}, CreateServiceOutput{}, nil
		}
		input.DockerfilePath = dockerfilePath
	}

	port := resolveServicePort(buildPack, publishDir, input.Port)

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

	var result *deployments.CreateServiceResult

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
		ServiceID: result.ServiceID,
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
		projectRef := input.Project
		if projectRef == "" {
			projectRef = "default"
		}
		project, projErr := s.deployService.GetProjectByRef(ctx, user.ID, projectRef)
		if projErr != nil {
			return "", "", fmt.Errorf("project not found: %s", projectRef)
		}
		internalRepo, repoErr := s.internalGitSvc.GetRepoByProjectAndName(ctx, project.ID, repo)
		if repoErr != nil {
			return "", "", fmt.Errorf("repo '%s' not found in project '%s'. Create it first with create_repo", repo, projectRef)
		}
		if internalRepo.UserID != user.ID {
			return "", "", fmt.Errorf("repo belongs to another user")
		}
		return host, fmt.Sprintf("ml.ink/%s", internalRepo.FullName), nil
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

func (s *Server) createServiceFromGitHub(ctx context.Context, user *users.User, input CreateServiceInput, buildPack, port string, envVars []deployments.EnvVar) (*deployments.CreateServiceResult, error) {
	creds, err := s.authService.GetGitHubCredsByUserID(ctx, user.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to get GitHub credentials")
	}

	if creds.GithubAppInstallationID == nil {
		return nil, fmt.Errorf("GitHub App not installed. Please install the GitHub App first")
	}

	repo := strings.TrimPrefix(input.Repo, "github.com/")

	return s.deployService.CreateService(ctx, deployments.CreateServiceInput{
		UserID:           user.ID,
		ProjectRef:       input.Project,
		Repo:             repo,
		Branch:           input.Branch,
		Name:             input.Name,
		BuildPack:        buildPack,
		Port:             port,
		EnvVars:          envVars,
		GitProvider:      "github",
		Memory:           input.Memory,
		VCPUs:            input.VCPUs,
		BuildCommand:     input.BuildCommand,
		StartCommand:     input.StartCommand,
		InstallationID:   *creds.GithubAppInstallationID,
		PublishDirectory: input.PublishDirectory,
		RootDirectory:    input.RootDirectory,
		DockerfilePath:   input.DockerfilePath,
		Region:           input.Region,
	})
}

func (s *Server) createServiceFromInternalGit(ctx context.Context, userID string, input CreateServiceInput, buildPack, port string, envVars []deployments.EnvVar) (*deployments.CreateServiceResult, error) {
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

	return s.deployService.CreateService(ctx, deployments.CreateServiceInput{
		UserID:           userID,
		ProjectRef:       input.Project,
		Repo:             fullName,
		Branch:           input.Branch,
		Name:             input.Name,
		BuildPack:        buildPack,
		Port:             port,
		EnvVars:          envVars,
		GitProvider:      "internal",
		Memory:           input.Memory,
		VCPUs:            input.VCPUs,
		BuildCommand:     input.BuildCommand,
		StartCommand:     input.StartCommand,
		PublishDirectory: input.PublishDirectory,
		RootDirectory:    input.RootDirectory,
		DockerfilePath:   input.DockerfilePath,
		Region:           input.Region,
	})
}

func (s *Server) handleListServices(ctx context.Context, req *mcp.CallToolRequest, input ListServicesInput) (*mcp.CallToolResult, ListServicesOutput, error) {
	user := UserFromContext(ctx)
	if user == nil {
		return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: "not authenticated"}}}, ListServicesOutput{}, nil
	}

	svcs, err := s.deployService.ListServices(ctx, user.ID, 100, 0)
	if err != nil {
		s.logger.Error("failed to list services", "error", err)
		return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("failed to list services: %v", err)}}}, ListServicesOutput{}, nil
	}

	services := make([]ServiceInfo, len(svcs))
	for i, svc := range svcs {
		name := ""
		if svc.Name != nil {
			name = *svc.Name
		}

		var dep *DeploymentDetails
		if d, err := s.deployService.GetLatestDeployment(ctx, svc.ID); err == nil && d != nil {
			dep = &DeploymentDetails{Status: d.Status}
		}

		services[i] = ServiceInfo{
			ServiceID:  svc.ID,
			Name:       name,
			Repo:       svc.Repo,
			URL:        svc.Fqdn,
			Deployment: dep,
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

	svc, err := s.deployService.GetServiceByNameAndProject(ctx, input.Name, project.ID)
	if err != nil {
		return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("service not found: %s", input.Name)}}}, RedeployServiceOutput{}, nil
	}

	s.logger.Info("starting redeploy",
		"user_id", user.ID,
		"service_id", svc.ID,
		"name", input.Name,
	)

	workflowID, err := s.deployService.RedeployService(ctx, svc.ID)
	if err != nil {
		s.logger.Error("failed to start redeploy", "error", err)
		return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("failed to start redeploy: %v", err)}}}, RedeployServiceOutput{}, nil
	}

	name := ""
	if svc.Name != nil {
		name = *svc.Name
	}

	output := RedeployServiceOutput{
		ServiceID: svc.ID,
		Name:      name,
		Status:    "queued",
		Message:   fmt.Sprintf("Redeploy started (workflow_id: %s)", workflowID),
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

	projectRef := "default"
	project, err := s.deployService.GetProjectByRef(ctx, user.ID, projectRef)
	if err != nil {
		return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("project not found: %s", projectRef)}}}, CreateResourceOutput{}, nil
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
		ProjectID: &project.ID,
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

	svc, err := s.deployService.GetServiceByName(ctx, deployments.GetServiceByNameParams{
		Name:    input.Name,
		Project: project,
		UserID:  user.ID,
	})
	if err != nil {
		return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: err.Error()}}}, GetServiceOutput{}, nil
	}

	depStatus := ""
	var errorMessage *string
	if dep, err := s.deployService.GetLatestDeployment(ctx, svc.ID); err == nil && dep != nil {
		depStatus = dep.Status
		errorMessage = dep.ErrorMessage
	}

	var deployment *DeploymentDetails
	if depStatus != "" {
		deployment = &DeploymentDetails{
			Status:       depStatus,
			ErrorMessage: errorMessage,
		}
	}

	runtime := &RuntimeDetails{
		Status: depStatus,
	}

	output := GetServiceOutput{
		ServiceID:  svc.ID,
		Name:       helpers.Deref(svc.Name),
		Project:    project,
		Repo:       svc.Repo,
		Branch:     svc.Branch,
		URL:        svc.Fqdn,
		CreatedAt:  svc.CreatedAt.Time.Format(time.RFC3339),
		UpdatedAt:  svc.UpdatedAt.Time.Format(time.RFC3339),
		Deployment: deployment,
		Runtime:    runtime,
	}

	if zr, dz, err := s.dnsService.GetCustomDomainForService(ctx, svc.ID); err == nil {
		domain := zr.Name + "." + dz.Zone
		output.CustomDomain = &CustomDomainDetails{
			Domain: domain,
			Status: dz.Status,
			Error:  dz.LastError,
		}
	}

	if input.IncludeEnv {
		var envVars []EnvVar
		if err := json.Unmarshal(svc.EnvVars, &envVars); err == nil {
			output.EnvVars = make([]EnvVarInfo, len(envVars))
			for i, ev := range envVars {
				output.EnvVars[i] = EnvVarInfo(ev)
			}
		}
	}

	if input.DeployLogLines > 0 && deployment != nil {
		limit := min(input.DeployLogLines, MaxLogLines)
		ns := k8sdeployments.NamespaceName(user.ID, project)
		svcName := k8sdeployments.ServiceName(helpers.Deref(svc.Name))
		lines, err := k8sdeployments.QueryBuildLogs(ctx, s.lokiQueryURL, s.lokiUsername, s.lokiPassword, ns, svcName, 24*time.Hour, limit)
		if err == nil && len(lines) > 0 {
			deployment.Logs = strings.Join(lines, "\n")
		}
	}

	if input.RuntimeLogLines > 0 {
		limit := min(input.RuntimeLogLines, MaxLogLines)
		ns := k8sdeployments.NamespaceName(user.ID, project)
		svcName := k8sdeployments.ServiceName(helpers.Deref(svc.Name))
		lines, err := k8sdeployments.QueryRunLogs(ctx, s.lokiQueryURL, s.lokiUsername, s.lokiPassword, ns, svcName, 24*time.Hour, limit)
		if err == nil && len(lines) > 0 {
			runtime.Logs = strings.Join(lines, "\n")
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

	result, err := s.deployService.DeleteService(ctx, deployments.DeleteServiceParams{
		Name:    input.Name,
		Project: project,
		UserID:  user.ID,
	})
	if err != nil {
		s.logger.Error("failed to delete service", "error", err)
		return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: err.Error()}}}, DeleteServiceOutput{}, nil
	}

	return nil, DeleteServiceOutput{
		ServiceID: result.ServiceID,
		Name:      result.Name,
		Message:   "Service deleted successfully",
	}, nil
}

func (s *Server) handleAddCustomDomain(ctx context.Context, req *mcp.CallToolRequest, input AddCustomDomainInput) (*mcp.CallToolResult, AddCustomDomainOutput, error) {
	user := UserFromContext(ctx)
	if user == nil {
		return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: "not authenticated"}}}, AddCustomDomainOutput{}, nil
	}

	if input.Name == "" {
		return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: "name is required"}}}, AddCustomDomainOutput{}, nil
	}
	if input.Domain == "" {
		return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: "domain is required"}}}, AddCustomDomainOutput{}, nil
	}

	project := "default"
	if input.Project != "" {
		project = input.Project
	}

	result, err := s.dnsService.AddCustomDomain(ctx, dns.AddCustomDomainParams{
		Name:    input.Name,
		Project: project,
		UserID:  user.ID,
		Domain:  input.Domain,
	})
	if err != nil {
		return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: err.Error()}}}, AddCustomDomainOutput{}, nil
	}

	return nil, AddCustomDomainOutput{
		ServiceID: result.ServiceID,
		Domain:    result.Domain,
		Status:    result.Status,
		Message:   result.Message,
	}, nil
}

func (s *Server) handleRemoveCustomDomain(ctx context.Context, req *mcp.CallToolRequest, input RemoveCustomDomainInput) (*mcp.CallToolResult, RemoveCustomDomainOutput, error) {
	user := UserFromContext(ctx)
	if user == nil {
		return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: "not authenticated"}}}, RemoveCustomDomainOutput{}, nil
	}

	if input.Name == "" {
		return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: "name is required"}}}, RemoveCustomDomainOutput{}, nil
	}

	project := "default"
	if input.Project != "" {
		project = input.Project
	}

	result, err := s.dnsService.RemoveCustomDomain(ctx, dns.RemoveCustomDomainParams{
		Name:    input.Name,
		Project: project,
		UserID:  user.ID,
	})
	if err != nil {
		return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: err.Error()}}}, RemoveCustomDomainOutput{}, nil
	}

	return nil, RemoveCustomDomainOutput{
		ServiceID: result.ServiceID,
		Message:   result.Message,
	}, nil
}

func (s *Server) handleListDelegations(ctx context.Context, req *mcp.CallToolRequest, input ListDelegationsInput) (*mcp.CallToolResult, ListDelegationsOutput, error) {
	user := UserFromContext(ctx)
	if user == nil {
		return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: "not authenticated"}}}, ListDelegationsOutput{}, nil
	}

	zones, err := s.dnsService.ListDelegations(ctx, user.ID)
	if err != nil {
		return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("failed to list delegations: %v", err)}}}, ListDelegationsOutput{}, nil
	}

	delegations := make([]DelegationInfo, len(zones))
	for i, z := range zones {
		delegations[i] = DelegationInfo{
			ZoneID:    z.ID,
			Zone:      z.Zone,
			Status:    z.Status,
			Error:     z.LastError,
			CreatedAt: z.CreatedAt.Time.Format(time.RFC3339),
		}
	}

	return nil, ListDelegationsOutput{Delegations: delegations}, nil
}
