package deployments

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/augustdev/autoclip/internal/storage/pg/generated/apps"
	"go.temporal.io/sdk/client"
)

type Service struct {
	temporalClient client.Client
	appsQ          apps.Querier
	logger         *slog.Logger
}

func NewService(
	temporalClient client.Client,
	appsQ apps.Querier,
	logger *slog.Logger,
) *Service {
	return &Service{
		temporalClient: temporalClient,
		appsQ:          appsQ,
		logger:         logger,
	}
}

type CreateAppInput struct {
	UserID        string
	GitHubAppUUID string
	Repo          string
	Branch        string
	Name          string
	BuildPack     string
	Port          string
	EnvVars       []EnvVar
}

type CreateAppResult struct {
	WorkflowID string
}

func (s *Service) CreateApp(ctx context.Context, input CreateAppInput) (*CreateAppResult, error) {
	workflowID := fmt.Sprintf("deploy-%s-%s-%s", input.UserID, input.Repo, input.Branch)

	workflowInput := DeployWorkflowInput{
		UserID:        input.UserID,
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
