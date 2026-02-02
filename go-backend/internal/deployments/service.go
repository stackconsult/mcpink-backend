package deployments

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/augustdev/autoclip/internal/coolify"
	"github.com/augustdev/autoclip/internal/storage/pg/generated/apps"
	"github.com/augustdev/autoclip/internal/storage/pg/generated/projects"
	"github.com/lithammer/shortuuid/v4"
	"go.temporal.io/sdk/client"
)

type Service struct {
	temporalClient client.Client
	appsQ          apps.Querier
	projectsQ      projects.Querier
	coolifyClient  *coolify.Client
	logger         *slog.Logger
}

func NewService(
	temporalClient client.Client,
	appsQ apps.Querier,
	projectsQ projects.Querier,
	coolifyClient *coolify.Client,
	logger *slog.Logger,
) *Service {
	return &Service{
		temporalClient: temporalClient,
		appsQ:          appsQ,
		projectsQ:      projectsQ,
		coolifyClient:  coolifyClient,
		logger:         logger,
	}
}

type CreateAppInput struct {
	UserID        string
	ProjectRef    string
	GitHubAppUUID string
	Repo          string
	Branch        string
	Name          string
	BuildPack     string
	Port          string
	EnvVars       []EnvVar
}

type CreateAppResult struct {
	AppID      string
	Name       string
	Status     string
	Repo       string
	WorkflowID string
}

func (s *Service) CreateApp(ctx context.Context, input CreateAppInput) (*CreateAppResult, error) {
	// Look up project by ref or get default
	var projectID string
	if input.ProjectRef != "" {
		project, err := s.projectsQ.GetProjectByRef(ctx, projects.GetProjectByRefParams{
			UserID: input.UserID,
			Ref:    input.ProjectRef,
		})
		if err != nil {
			return nil, fmt.Errorf("project not found: %s", input.ProjectRef)
		}
		projectID = project.ID
	} else {
		project, err := s.projectsQ.GetDefaultProject(ctx, input.UserID)
		if err != nil {
			return nil, fmt.Errorf("default project not found for user")
		}
		projectID = project.ID
	}

	appID := shortuuid.New()
	workflowID := fmt.Sprintf("deploy-%s-%s-%s", input.UserID, input.Repo, input.Branch)

	workflowInput := DeployWorkflowInput{
		AppID:         appID,
		UserID:        input.UserID,
		ProjectID:     projectID,
		GitHubAppUUID: input.GitHubAppUUID,
		Repo:          input.Repo,
		Branch:        input.Branch,
		Name:          input.Name,
		BuildPack:     input.BuildPack,
		Port:          input.Port,
		EnvVars:       input.EnvVars,
	}

	workflowOptions := client.StartWorkflowOptions{
		ID:        workflowID,
		TaskQueue: "default",
	}

	run, err := s.temporalClient.ExecuteWorkflow(ctx, workflowOptions, DeployToCoolifyWorkflow, workflowInput)
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
	project, err := s.projectsQ.GetProjectByRef(ctx, projects.GetProjectByRefParams{
		UserID: userID,
		Ref:    ref,
	})
	if err != nil {
		return nil, fmt.Errorf("project not found: %s", ref)
	}
	return &project, nil
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

func (s *Service) RedeployApp(ctx context.Context, appID, coolifyAppUUID string) (string, error) {
	workflowID := fmt.Sprintf("redeploy-%s-%s", appID, shortuuid.New())

	workflowOptions := client.StartWorkflowOptions{
		ID:        workflowID,
		TaskQueue: "default",
	}

	we, err := s.temporalClient.ExecuteWorkflow(ctx, workflowOptions, RedeployToCoolifyWorkflow, RedeployWorkflowInput{
		AppID:          appID,
		CoolifyAppUUID: coolifyAppUUID,
	})
	if err != nil {
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
	AppID string
	Name  string
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

	// Delete from Coolify if it was deployed
	if app.CoolifyAppUuid != nil && s.coolifyClient != nil {
		if err := s.coolifyClient.Applications.Delete(ctx, *app.CoolifyAppUuid); err != nil {
			s.logger.Warn("failed to delete app from Coolify",
				"app_id", app.ID,
				"coolify_uuid", *app.CoolifyAppUuid,
				"error", err)
		} else {
			s.logger.Info("deleted app from Coolify",
				"app_id", app.ID,
				"coolify_uuid", *app.CoolifyAppUuid)
		}
	}

	// Soft delete in database
	_, err = s.appsQ.SoftDeleteApp(ctx, app.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to delete app: %w", err)
	}

	var name string
	if app.Name != nil {
		name = *app.Name
	}

	s.logger.Info("soft deleted app",
		"app_id", app.ID,
		"name", name)

	return &DeleteAppResult{
		AppID: app.ID,
		Name:  name,
	}, nil
}
