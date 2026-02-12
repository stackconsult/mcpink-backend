package deployments

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/augustdev/autoclip/internal/cloudflare"
	"github.com/augustdev/autoclip/internal/k8sdeployments"
	"github.com/augustdev/autoclip/internal/storage/pg/generated/dnsrecords"
	"github.com/augustdev/autoclip/internal/storage/pg/generated/githubcreds"
	"github.com/augustdev/autoclip/internal/storage/pg/generated/projects"
	"github.com/augustdev/autoclip/internal/storage/pg/generated/services"
	"github.com/augustdev/autoclip/internal/storage/pg/generated/users"
	"github.com/lithammer/shortuuid/v4"
	enumspb "go.temporal.io/api/enums/v1"
	"go.temporal.io/api/serviceerror"
	"go.temporal.io/sdk/client"
)

type Service struct {
	temporalClient   client.Client
	servicesQ        services.Querier
	projectsQ        projects.Querier
	usersQ           users.Querier
	ghCredsQ         githubcreds.Querier
	dnsQ             dnsrecords.Querier
	cloudflareClient *cloudflare.Client
	appsDomain       string
	logger           *slog.Logger
}

func NewService(
	temporalClient client.Client,
	servicesQ services.Querier,
	projectsQ projects.Querier,
	usersQ users.Querier,
	ghCredsQ githubcreds.Querier,
	dnsQ dnsrecords.Querier,
	cloudflareClient *cloudflare.Client,
	cfConfig cloudflare.Config,
	logger *slog.Logger,
) *Service {
	return &Service{
		temporalClient:   temporalClient,
		servicesQ:        servicesQ,
		projectsQ:        projectsQ,
		usersQ:           usersQ,
		ghCredsQ:         ghCredsQ,
		dnsQ:             dnsQ,
		cloudflareClient: cloudflareClient,
		appsDomain:       cfConfig.BaseDomain,
		logger:           logger,
	}
}

type CreateServiceInput struct {
	UserID           string
	ProjectRef       string
	GitHubAppUUID    string
	Repo             string
	Branch           string
	Name             string
	BuildPack        string
	Port             string
	EnvVars          []EnvVar
	GitProvider      string // "github" or "gitea"
	Memory           string
	CPU              string
	BuildCommand     string
	StartCommand     string
	InstallationID   int64
	PublishDirectory string
	RootDirectory    string
	DockerfilePath   string
}

type CreateServiceResult struct {
	ServiceID  string
	Name       string
	Status     string
	Repo       string
	WorkflowID string
}

func (s *Service) CreateService(ctx context.Context, input CreateServiceInput) (*CreateServiceResult, error) {
	var projectID string
	if input.ProjectRef != "" {
		project, err := s.getOrCreateProject(ctx, input.UserID, input.ProjectRef)
		if err != nil {
			return nil, err
		}
		projectID = project.ID
	} else {
		project, err := s.projectsQ.GetDefaultProject(ctx, input.UserID)
		if err != nil {
			return nil, fmt.Errorf("default project not found for user")
		}
		projectID = project.ID
	}

	// Check for duplicate name in the same project
	if input.Name != "" {
		_, err := s.servicesQ.GetServiceByNameAndProject(ctx, services.GetServiceByNameAndProjectParams{
			Name:      &input.Name,
			ProjectID: projectID,
		})
		if err == nil {
			return nil, fmt.Errorf("service %q already exists in this project", input.Name)
		}
	}

	svcID := shortuuid.New()
	workflowID := fmt.Sprintf("deploy-%s", svcID)

	gitProvider := input.GitProvider
	if gitProvider == "" {
		gitProvider = "github"
	}

	envVarsJSON, _ := json.Marshal(input.EnvVars)

	buildConfigJSON, _ := json.Marshal(k8sdeployments.BuildConfig{
		RootDirectory:    input.RootDirectory,
		DockerfilePath:   input.DockerfilePath,
		PublishDirectory: input.PublishDirectory,
		BuildCommand:     input.BuildCommand,
		StartCommand:     input.StartCommand,
	})

	memory := input.Memory
	if memory == "" {
		memory = "256Mi"
	}
	cpu := input.CPU
	if cpu == "" {
		cpu = "0.5"
	}

	_, err := s.servicesQ.CreateService(ctx, services.CreateServiceParams{
		ID:          svcID,
		UserID:      input.UserID,
		ProjectID:   projectID,
		Repo:        input.Repo,
		Branch:      input.Branch,
		ServerUuid:  "k8s",
		Name:        &input.Name,
		BuildPack:   input.BuildPack,
		Port:        input.Port,
		EnvVars:     envVarsJSON,
		WorkflowID:  workflowID,
		GitProvider: gitProvider,
		BuildConfig: buildConfigJSON,
		Memory:      memory,
		Cpu:         cpu,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create service record: %w", err)
	}

	workflowInput := k8sdeployments.CreateServiceWorkflowInput{
		ServiceID:      svcID,
		Repo:           input.Repo,
		Branch:         input.Branch,
		GitProvider:    gitProvider,
		InstallationID: input.InstallationID,
		AppsDomain:     s.appsDomain,
	}

	workflowOptions := client.StartWorkflowOptions{
		ID:                                       workflowID,
		TaskQueue:                                k8sdeployments.TaskQueue,
		WorkflowIDConflictPolicy:                 enumspb.WORKFLOW_ID_CONFLICT_POLICY_FAIL,
		WorkflowIDReusePolicy:                    enumspb.WORKFLOW_ID_REUSE_POLICY_REJECT_DUPLICATE,
		WorkflowExecutionErrorWhenAlreadyStarted: true,
	}

	run, err := s.temporalClient.ExecuteWorkflow(ctx, workflowOptions, k8sdeployments.CreateServiceWorkflow, workflowInput)
	if err != nil {
		s.logger.Error("failed to start deploy workflow",
			"workflowID", workflowID,
			"error", err)
		return nil, fmt.Errorf("failed to start deploy workflow: %w", err)
	}

	runID := run.GetRunID()
	if err := s.servicesQ.UpdateWorkflowRunID(ctx, services.UpdateWorkflowRunIDParams{
		ID:            svcID,
		WorkflowRunID: &runID,
	}); err != nil {
		s.logger.Warn("failed to persist workflow run id",
			"serviceID", svcID,
			"workflowID", workflowID,
			"runID", runID,
			"error", err)
	}

	s.logger.Info("started deploy workflow",
		"workflowID", workflowID,
		"runID", run.GetRunID())

	return &CreateServiceResult{
		ServiceID:  svcID,
		Name:       input.Name,
		Status:     string(BuildStatusQueued),
		Repo:       input.Repo,
		WorkflowID: workflowID,
	}, nil
}

func (s *Service) ListServices(ctx context.Context, userID string, limit, offset int32) ([]services.Service, error) {
	svcList, err := s.servicesQ.ListServicesByUserID(ctx, services.ListServicesByUserIDParams{
		UserID: userID,
		Limit:  limit,
		Offset: offset,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list services: %w", err)
	}
	return svcList, nil
}

func (s *Service) GetProjectByRef(ctx context.Context, userID, ref string) (*projects.Project, error) {
	return s.getOrCreateProject(ctx, userID, ref)
}

func (s *Service) getOrCreateProject(ctx context.Context, userID, ref string) (*projects.Project, error) {
	project, err := s.projectsQ.GetProjectByRef(ctx, projects.GetProjectByRefParams{
		UserID: userID,
		Ref:    ref,
	})
	if err == nil {
		return &project, nil
	}

	s.logger.Info("auto-creating project", "user_id", userID, "ref", ref)
	newProject, err := s.projectsQ.CreateProject(ctx, projects.CreateProjectParams{
		UserID: userID,
		Name:   ref,
		Ref:    ref,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create project: %w", err)
	}
	return &newProject, nil
}

func (s *Service) GetServiceByNameAndProject(ctx context.Context, name, projectID string) (*services.Service, error) {
	svc, err := s.servicesQ.GetServiceByNameAndProject(ctx, services.GetServiceByNameAndProjectParams{
		Name:      &name,
		ProjectID: projectID,
	})
	if err != nil {
		return nil, fmt.Errorf("service not found: %s", name)
	}
	return &svc, nil
}

type GetServiceByNameParams struct {
	Name    string
	Project string // "default" uses user's default project
	UserID  string
}

func (s *Service) GetServiceByName(ctx context.Context, params GetServiceByNameParams) (*services.Service, error) {
	project := params.Project
	if project == "" {
		project = "default"
	}

	svc, err := s.servicesQ.GetServiceByNameAndUserProject(ctx, services.GetServiceByNameAndUserProjectParams{
		Name:   &params.Name,
		UserID: params.UserID,
		Ref:    project,
	})
	if err != nil {
		return nil, fmt.Errorf("service not found: %s in project %s", params.Name, project)
	}
	return &svc, nil
}

func (s *Service) RedeployService(ctx context.Context, svcID string) (string, error) {
	workflowID := fmt.Sprintf("redeploy-%s-%s", svcID, shortuuid.New())
	return s.RedeployServiceWithWorkflowID(ctx, svcID, workflowID)
}

// RedeployFromGitHubPush starts (or reuses) a redeploy workflow triggered by a GitHub push.
//
// GitHub delivery is at-least-once, so we treat it as potentially duplicated and use a deterministic workflow ID
// derived from the commit SHA (preferred) or delivery ID (fallback).
func (s *Service) RedeployFromGitHubPush(ctx context.Context, svcID, afterSHA, deliveryID string) (string, error) {
	key := strings.TrimSpace(afterSHA)
	if key == "" || key == "0000000000000000000000000000000000000000" {
		key = strings.TrimSpace(deliveryID)
	}
	if key == "" {
		key = shortuuid.New()
	}

	workflowID := fmt.Sprintf("redeploy-%s-%s", svcID, key)
	return s.RedeployServiceWithWorkflowID(ctx, svcID, workflowID)
}

// RedeployFromInternalGitPush starts (or reuses) a redeploy workflow triggered by an internal git (Gitea) push.
func (s *Service) RedeployFromInternalGitPush(ctx context.Context, svcID, afterSHA string) (string, error) {
	key := strings.TrimSpace(afterSHA)
	if key == "" || key == "0000000000000000000000000000000000000000" {
		key = shortuuid.New()
	}

	workflowID := fmt.Sprintf("redeploy-%s-%s", svcID, key)
	return s.RedeployServiceWithWorkflowID(ctx, svcID, workflowID)
}

func (s *Service) RedeployServiceWithWorkflowID(ctx context.Context, svcID, workflowID string) (string, error) {
	if workflowID == "" {
		workflowID = fmt.Sprintf("redeploy-%s-%s", svcID, shortuuid.New())
	}

	svc, err := s.servicesQ.GetServiceByID(ctx, svcID)
	if err != nil {
		return "", fmt.Errorf("service not found: %w", err)
	}

	var installationID int64
	if svc.GitProvider == "github" {
		creds, err := s.ghCredsQ.GetGitHubCredsByUserID(ctx, svc.UserID)
		if err == nil && creds.GithubAppInstallationID != nil {
			installationID = *creds.GithubAppInstallationID
		}
	}

	workflowOptions := client.StartWorkflowOptions{
		ID:                    workflowID,
		TaskQueue:             k8sdeployments.TaskQueue,
		WorkflowIDReusePolicy: enumspb.WORKFLOW_ID_REUSE_POLICY_REJECT_DUPLICATE,
	}

	we, err := s.temporalClient.ExecuteWorkflow(ctx, workflowOptions, k8sdeployments.RedeployServiceWorkflow, k8sdeployments.RedeployServiceWorkflowInput{
		ServiceID:      svcID,
		Repo:           svc.Repo,
		Branch:         svc.Branch,
		GitProvider:    svc.GitProvider,
		InstallationID: installationID,
		AppsDomain:     s.appsDomain,
	})
	if err != nil {
		var alreadyStarted *serviceerror.WorkflowExecutionAlreadyStarted
		if errors.As(err, &alreadyStarted) {
			s.logger.Info("redeploy workflow already started, skipping duplicate",
				"workflowID", workflowID,
				"serviceID", svcID)
			return workflowID, nil
		}

		s.logger.Error("failed to start redeploy workflow",
			"workflowID", workflowID,
			"error", err)
		return "", fmt.Errorf("failed to start redeploy workflow: %w", err)
	}

	s.logger.Info("started redeploy workflow",
		"workflowID", workflowID,
		"runID", we.GetRunID())

	return we.GetID(), nil
}

type DeleteServiceParams struct {
	Name    string
	Project string
	UserID  string
}

type DeleteServiceResult struct {
	ServiceID  string
	Name       string
	WorkflowID string
}

func (s *Service) DeleteService(ctx context.Context, params DeleteServiceParams) (*DeleteServiceResult, error) {
	project := params.Project
	if project == "" {
		project = "default"
	}

	svc, err := s.servicesQ.GetServiceByNameAndUserProject(ctx, services.GetServiceByNameAndUserProjectParams{
		Name:   &params.Name,
		UserID: params.UserID,
		Ref:    project,
	})
	if err != nil {
		return nil, fmt.Errorf("service not found: %s in project %s", params.Name, project)
	}

	var name string
	if svc.Name != nil {
		name = *svc.Name
	}

	user, err := s.usersQ.GetUserByID(ctx, svc.UserID)
	if err != nil {
		return nil, fmt.Errorf("user not found: %w", err)
	}

	proj, err := s.projectsQ.GetProjectByID(ctx, svc.ProjectID)
	if err != nil {
		return nil, fmt.Errorf("project not found: %w", err)
	}

	username := ""
	if user.GiteaUsername != nil && *user.GiteaUsername != "" {
		username = *user.GiteaUsername
	} else if user.GithubUsername != nil && *user.GithubUsername != "" {
		username = *user.GithubUsername
	}

	namespace := k8sdeployments.NamespaceName(username, proj.Ref)
	serviceName := k8sdeployments.ServiceName(name)

	workflowID := fmt.Sprintf("delete-svc-%s", svc.ID)

	workflowOptions := client.StartWorkflowOptions{
		ID:        workflowID,
		TaskQueue: k8sdeployments.TaskQueue,
	}

	input := k8sdeployments.DeleteServiceWorkflowInput{
		ServiceID: svc.ID,
		Namespace: namespace,
		Name:      serviceName,
	}

	run, err := s.temporalClient.ExecuteWorkflow(ctx, workflowOptions, k8sdeployments.DeleteServiceWorkflow, input)
	if err != nil {
		return nil, fmt.Errorf("failed to start delete workflow: %w", err)
	}

	s.logger.Info("started delete service workflow",
		"service_id", svc.ID,
		"name", name,
		"workflow_id", run.GetID())

	return &DeleteServiceResult{
		ServiceID:  svc.ID,
		Name:       name,
		WorkflowID: run.GetID(),
	}, nil
}
