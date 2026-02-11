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
	"github.com/augustdev/autoclip/internal/storage/pg/generated/apps"
	"github.com/augustdev/autoclip/internal/storage/pg/generated/dnsrecords"
	"github.com/augustdev/autoclip/internal/storage/pg/generated/githubcreds"
	"github.com/augustdev/autoclip/internal/storage/pg/generated/projects"
	"github.com/augustdev/autoclip/internal/storage/pg/generated/users"
	"github.com/lithammer/shortuuid/v4"
	enumspb "go.temporal.io/api/enums/v1"
	"go.temporal.io/api/serviceerror"
	"go.temporal.io/sdk/client"
)

type Service struct {
	temporalClient   client.Client
	appsQ            apps.Querier
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
	appsQ apps.Querier,
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
		appsQ:            appsQ,
		projectsQ:        projectsQ,
		usersQ:           usersQ,
		ghCredsQ:         ghCredsQ,
		dnsQ:             dnsQ,
		cloudflareClient: cloudflareClient,
		appsDomain:       cfConfig.BaseDomain,
		logger:           logger,
	}
}

type CreateAppInput struct {
	UserID         string
	ProjectRef     string
	GitHubAppUUID  string
	Repo           string
	Branch         string
	Name           string
	BuildPack      string
	Port           string
	EnvVars        []EnvVar
	GitProvider    string // "github" or "gitea"
	PrivateKeyUUID string // for internal git
	SSHCloneURL    string // for internal git
	Memory         string
	CPU            string
	InstallCommand   string
	BuildCommand     string
	StartCommand     string
	InstallationID   int64
	PublishDirectory string
}

type CreateAppResult struct {
	AppID      string
	Name       string
	Status     string
	Repo       string
	WorkflowID string
}

func (s *Service) CreateApp(ctx context.Context, input CreateAppInput) (*CreateAppResult, error) {
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
		_, err := s.appsQ.GetAppByNameAndProject(ctx, apps.GetAppByNameAndProjectParams{
			Name:      &input.Name,
			ProjectID: projectID,
		})
		if err == nil {
			return nil, fmt.Errorf("service %q already exists in this project", input.Name)
		}
	}

	appID := shortuuid.New()
	workflowID := fmt.Sprintf("deploy-%s-%s-%s", input.UserID, input.Repo, input.Branch)

	gitProvider := input.GitProvider
	if gitProvider == "" {
		gitProvider = "github"
	}

	envVarsJSON, _ := json.Marshal(input.EnvVars)

	var publishDir *string
	if input.PublishDirectory != "" {
		publishDir = &input.PublishDirectory
	}

	_, err := s.appsQ.CreateApp(ctx, apps.CreateAppParams{
		ID:               appID,
		UserID:           input.UserID,
		ProjectID:        projectID,
		Repo:             input.Repo,
		Branch:           input.Branch,
		ServerUuid:       "k8s",
		Name:             &input.Name,
		BuildPack:        input.BuildPack,
		Port:             input.Port,
		EnvVars:          envVarsJSON,
		WorkflowID:       workflowID,
		GitProvider:      gitProvider,
		PublishDirectory: publishDir,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create app record: %w", err)
	}

	workflowInput := k8sdeployments.CreateServiceWorkflowInput{
		ServiceID:      appID,
		Repo:           input.Repo,
		Branch:         input.Branch,
		GitProvider:    gitProvider,
		InstallationID: input.InstallationID,
		AppsDomain:     s.appsDomain,
	}

	workflowOptions := client.StartWorkflowOptions{
		ID:        workflowID,
		TaskQueue: k8sdeployments.TaskQueue,
	}

	run, err := s.temporalClient.ExecuteWorkflow(ctx, workflowOptions, k8sdeployments.CreateServiceWorkflow, workflowInput)
	if err != nil {
		s.logger.Error("failed to start deploy workflow",
			"workflowID", workflowID,
			"error", err)
		return nil, fmt.Errorf("failed to start deploy workflow: %w", err)
	}

	s.logger.Info("started deploy workflow",
		"workflowID", workflowID,
		"runID", run.GetRunID())

	return &CreateAppResult{
		AppID:      appID,
		Name:       input.Name,
		Status:     string(BuildStatusQueued),
		Repo:       input.Repo,
		WorkflowID: workflowID,
	}, nil
}

func (s *Service) ListApps(ctx context.Context, userID string, limit, offset int32) ([]apps.App, error) {
	appList, err := s.appsQ.ListAppsByUserID(ctx, apps.ListAppsByUserIDParams{
		UserID: userID,
		Limit:  limit,
		Offset: offset,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list apps: %w", err)
	}
	return appList, nil
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

func (s *Service) GetAppByNameAndProject(ctx context.Context, name, projectID string) (*apps.App, error) {
	app, err := s.appsQ.GetAppByNameAndProject(ctx, apps.GetAppByNameAndProjectParams{
		Name:      &name,
		ProjectID: projectID,
	})
	if err != nil {
		return nil, fmt.Errorf("app not found: %s", name)
	}
	return &app, nil
}

type GetAppByNameParams struct {
	Name    string
	Project string // "default" uses user's default project
	UserID  string
}

func (s *Service) GetAppByName(ctx context.Context, params GetAppByNameParams) (*apps.App, error) {
	project := params.Project
	if project == "" {
		project = "default"
	}

	app, err := s.appsQ.GetAppByNameAndUserProject(ctx, apps.GetAppByNameAndUserProjectParams{
		Name:   &params.Name,
		UserID: params.UserID,
		Ref:    project,
	})
	if err != nil {
		return nil, fmt.Errorf("app not found: %s in project %s", params.Name, project)
	}
	return &app, nil
}

func (s *Service) RedeployApp(ctx context.Context, appID string) (string, error) {
	workflowID := fmt.Sprintf("redeploy-%s-%s", appID, shortuuid.New())
	return s.RedeployAppWithWorkflowID(ctx, appID, workflowID)
}

// RedeployFromGitHubPush starts (or reuses) a redeploy workflow triggered by a GitHub push.
//
// GitHub delivery is at-least-once, so we treat it as potentially duplicated and use a deterministic workflow ID
// derived from the commit SHA (preferred) or delivery ID (fallback).
func (s *Service) RedeployFromGitHubPush(ctx context.Context, appID, afterSHA, deliveryID string) (string, error) {
	key := strings.TrimSpace(afterSHA)
	if key == "" || key == "0000000000000000000000000000000000000000" {
		key = strings.TrimSpace(deliveryID)
	}
	if key == "" {
		key = shortuuid.New()
	}

	workflowID := fmt.Sprintf("redeploy-%s-%s", appID, key)
	return s.RedeployAppWithWorkflowID(ctx, appID, workflowID)
}

// RedeployFromInternalGitPush starts (or reuses) a redeploy workflow triggered by an internal git (Gitea) push.
func (s *Service) RedeployFromInternalGitPush(ctx context.Context, appID, afterSHA string) (string, error) {
	key := strings.TrimSpace(afterSHA)
	if key == "" || key == "0000000000000000000000000000000000000000" {
		key = shortuuid.New()
	}

	workflowID := fmt.Sprintf("redeploy-%s-%s", appID, key)
	return s.RedeployAppWithWorkflowID(ctx, appID, workflowID)
}

func (s *Service) RedeployAppWithWorkflowID(ctx context.Context, appID, workflowID string) (string, error) {
	if workflowID == "" {
		workflowID = fmt.Sprintf("redeploy-%s-%s", appID, shortuuid.New())
	}

	app, err := s.appsQ.GetAppByID(ctx, appID)
	if err != nil {
		return "", fmt.Errorf("app not found: %w", err)
	}

	var installationID int64
	if app.GitProvider == "github" {
		creds, err := s.ghCredsQ.GetGitHubCredsByUserID(ctx, app.UserID)
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
		ServiceID:      appID,
		Repo:           app.Repo,
		Branch:         app.Branch,
		GitProvider:    app.GitProvider,
		InstallationID: installationID,
		AppsDomain:     s.appsDomain,
	})
	if err != nil {
		var alreadyStarted *serviceerror.WorkflowExecutionAlreadyStarted
		if errors.As(err, &alreadyStarted) {
			s.logger.Info("redeploy workflow already started, skipping duplicate",
				"workflowID", workflowID,
				"appID", appID)
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

type DeleteAppParams struct {
	Name    string
	Project string
	UserID  string
}

type DeleteAppResult struct {
	AppID      string
	Name       string
	WorkflowID string
}

func (s *Service) DeleteApp(ctx context.Context, params DeleteAppParams) (*DeleteAppResult, error) {
	project := params.Project
	if project == "" {
		project = "default"
	}

	app, err := s.appsQ.GetAppByNameAndUserProject(ctx, apps.GetAppByNameAndUserProjectParams{
		Name:   &params.Name,
		UserID: params.UserID,
		Ref:    project,
	})
	if err != nil {
		return nil, fmt.Errorf("app not found: %s in project %s", params.Name, project)
	}

	var name string
	if app.Name != nil {
		name = *app.Name
	}

	user, err := s.usersQ.GetUserByID(ctx, app.UserID)
	if err != nil {
		return nil, fmt.Errorf("user not found: %w", err)
	}

	proj, err := s.projectsQ.GetProjectByID(ctx, app.ProjectID)
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

	workflowID := fmt.Sprintf("delete-app-%s", app.ID)

	workflowOptions := client.StartWorkflowOptions{
		ID:        workflowID,
		TaskQueue: k8sdeployments.TaskQueue,
	}

	input := k8sdeployments.DeleteServiceWorkflowInput{
		ServiceID: app.ID,
		Namespace: namespace,
		Name:      serviceName,
	}

	run, err := s.temporalClient.ExecuteWorkflow(ctx, workflowOptions, k8sdeployments.DeleteServiceWorkflow, input)
	if err != nil {
		return nil, fmt.Errorf("failed to start delete workflow: %w", err)
	}

	s.logger.Info("started delete app workflow",
		"app_id", app.ID,
		"name", name,
		"workflow_id", run.GetID())

	return &DeleteAppResult{
		AppID:      app.ID,
		Name:       name,
		WorkflowID: run.GetID(),
	}, nil
}
