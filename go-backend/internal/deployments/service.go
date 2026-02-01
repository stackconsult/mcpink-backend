package deployments

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/augustdev/autoclip/internal/coolify"
	"github.com/augustdev/autoclip/internal/storage/pg/generated/services"
	"github.com/google/uuid"
	"go.temporal.io/sdk/client"
)

type Manager struct {
	temporalClient client.Client
	servicesQ      services.Querier
	coolify        *coolify.Client
	logger         *slog.Logger
}

func NewManager(
	temporalClient client.Client,
	servicesQ services.Querier,
	coolify *coolify.Client,
	logger *slog.Logger,
) *Manager {
	return &Manager{
		temporalClient: temporalClient,
		servicesQ:      servicesQ,
		coolify:        coolify,
		logger:         logger,
	}
}

type CreateServiceInput struct {
	UserID    string
	Repo      string
	Branch    string
	Name      string
	BuildPack string
	Port      string
	EnvVars   []EnvVar
}

type CreateServiceResult struct {
	ServiceID  string
	WorkflowID string
}

func (m *Manager) CreateService(ctx context.Context, input CreateServiceInput) (*CreateServiceResult, error) {
	workflowID := fmt.Sprintf("deploy-service-%s", uuid.New().String())

	envVarsJSON, err := json.Marshal(input.EnvVars)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal env vars: %w", err)
	}

	buildPack := input.BuildPack
	if buildPack == "" {
		buildPack = "nixpacks"
	}

	port := input.Port
	if port == "" {
		port = "3000"
	}

	svc, err := m.servicesQ.CreateService(ctx, services.CreateServiceParams{
		UserID:     input.UserID,
		Repo:       input.Repo,
		Branch:     input.Branch,
		ServerUuid: m.coolify.GetMuscleServer(),
		Name:       &input.Name,
		BuildPack:  buildPack,
		Port:       port,
		EnvVars:    envVarsJSON,
		WorkflowID: workflowID,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create service record: %w", err)
	}

	workflowInput := DeployWorkflowInput{
		ServiceID: svc.ID,
		UserID:    input.UserID,
		Repo:      input.Repo,
		Branch:    input.Branch,
		Name:      input.Name,
		BuildPack: buildPack,
		Port:      port,
		EnvVars:   input.EnvVars,
	}

	workflowOptions := client.StartWorkflowOptions{
		ID:        workflowID,
		TaskQueue: "default",
	}

	run, err := m.temporalClient.ExecuteWorkflow(ctx, workflowOptions, DeployToCoolifyWorkflow, workflowInput)
	if err != nil {
		m.logger.Error("Failed to start deploy workflow",
			"serviceID", svc.ID,
			"workflowID", workflowID,
			"error", err)

		errMsg := err.Error()
		_, _ = m.servicesQ.UpdateServiceFailed(ctx, services.UpdateServiceFailedParams{
			ID:           svc.ID,
			ErrorMessage: &errMsg,
		})

		return nil, fmt.Errorf("failed to start deploy workflow: %w", err)
	}

	_ = m.servicesQ.UpdateWorkflowRunID(ctx, services.UpdateWorkflowRunIDParams{
		ID:            svc.ID,
		WorkflowRunID: strPtr(run.GetRunID()),
	})

	m.logger.Info("Started deploy workflow",
		"serviceID", svc.ID,
		"workflowID", workflowID,
		"runID", run.GetRunID())

	return &CreateServiceResult{
		ServiceID:  svc.ID,
		WorkflowID: workflowID,
	}, nil
}

func (m *Manager) GetService(ctx context.Context, id string) (*services.Service, error) {
	svc, err := m.servicesQ.GetServiceByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("failed to get service: %w", err)
	}
	return &svc, nil
}

func (m *Manager) GetServiceByWorkflowID(ctx context.Context, workflowID string) (*services.Service, error) {
	svc, err := m.servicesQ.GetServiceByWorkflowID(ctx, workflowID)
	if err != nil {
		return nil, fmt.Errorf("failed to get service by workflow ID: %w", err)
	}
	return &svc, nil
}

func (m *Manager) ListServices(ctx context.Context, userID string, limit, offset int32) ([]services.Service, error) {
	svcs, err := m.servicesQ.ListServicesByUserID(ctx, services.ListServicesByUserIDParams{
		UserID: userID,
		Limit:  limit,
		Offset: offset,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list services: %w", err)
	}
	return svcs, nil
}

func (m *Manager) CountServices(ctx context.Context, userID string) (int64, error) {
	count, err := m.servicesQ.CountServicesByUserID(ctx, userID)
	if err != nil {
		return 0, fmt.Errorf("failed to count services: %w", err)
	}
	return count, nil
}

func (m *Manager) StopService(ctx context.Context, id string) error {
	svc, err := m.servicesQ.GetServiceByID(ctx, id)
	if err != nil {
		return fmt.Errorf("failed to get service: %w", err)
	}

	if svc.CoolifyAppUuid == nil {
		return fmt.Errorf("service does not have a Coolify app")
	}

	if err := m.coolify.Applications.Stop(ctx, *svc.CoolifyAppUuid); err != nil {
		return fmt.Errorf("failed to stop Coolify application: %w", err)
	}

	stopped := "stopped"
	_, err = m.servicesQ.UpdateRuntimeStatus(ctx, services.UpdateRuntimeStatusParams{
		ID:            id,
		RuntimeStatus: &stopped,
	})
	if err != nil {
		return fmt.Errorf("failed to update runtime status: %w", err)
	}

	return nil
}

func (m *Manager) RestartService(ctx context.Context, id string) error {
	svc, err := m.servicesQ.GetServiceByID(ctx, id)
	if err != nil {
		return fmt.Errorf("failed to get service: %w", err)
	}

	if svc.CoolifyAppUuid == nil {
		return fmt.Errorf("service does not have a Coolify app")
	}

	if _, err := m.coolify.Applications.Restart(ctx, *svc.CoolifyAppUuid); err != nil {
		return fmt.Errorf("failed to restart Coolify application: %w", err)
	}

	running := "running"
	_, err = m.servicesQ.UpdateRuntimeStatus(ctx, services.UpdateRuntimeStatusParams{
		ID:            id,
		RuntimeStatus: &running,
	})
	if err != nil {
		return fmt.Errorf("failed to update runtime status: %w", err)
	}

	return nil
}

func (m *Manager) DeleteService(ctx context.Context, id string) error {
	svc, err := m.servicesQ.GetServiceByID(ctx, id)
	if err != nil {
		return fmt.Errorf("failed to get service: %w", err)
	}

	if svc.CoolifyAppUuid != nil {
		if err := m.coolify.Applications.Delete(ctx, *svc.CoolifyAppUuid); err != nil {
			m.logger.Warn("Failed to delete Coolify application (may not exist)",
				"coolifyUUID", *svc.CoolifyAppUuid,
				"error", err)
		}
	}

	if err := m.servicesQ.DeleteService(ctx, id); err != nil {
		return fmt.Errorf("failed to delete service: %w", err)
	}

	return nil
}

func strPtr(s string) *string {
	return &s
}
