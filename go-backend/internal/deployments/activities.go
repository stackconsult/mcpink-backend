package deployments

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/augustdev/autoclip/internal/coolify"
	"github.com/augustdev/autoclip/internal/storage/pg/generated/services"
	"go.temporal.io/sdk/activity"
)

type Activities struct {
	coolify   *coolify.Client
	servicesQ services.Querier
	logger    *slog.Logger
}

func NewActivities(coolify *coolify.Client, servicesQ services.Querier, logger *slog.Logger) *Activities {
	return &Activities{
		coolify:   coolify,
		servicesQ: servicesQ,
		logger:    logger,
	}
}

type CreateAppInput struct {
	ServiceID     string
	GitHubAppUUID string
	Repo          string
	Branch        string
	Name          string
	BuildPack     string
	Port          string
}

type CreateAppResult struct {
	CoolifyAppUUID string
	ServerID       string
}

func (a *Activities) CreateAppFromPrivateGithub(ctx context.Context, input CreateAppInput) (*CreateAppResult, error) {
	a.logger.Info("Creating app from private GitHub",
		"serviceID", input.ServiceID,
		"repo", input.Repo,
		"branch", input.Branch,
		"gitHubAppUUID", input.GitHubAppUUID)

	cfg := a.coolify.Config()

	gitHubAppUUID := input.GitHubAppUUID
	if gitHubAppUUID == "" {
		gitHubAppUUID = cfg.GitHubAppUUID
	}

	serverID := a.coolify.GetMuscleServer()
	req := &coolify.CreatePrivateGitHubAppRequest{
		ProjectUUID:     cfg.ProjectUUID,
		ServerUUID:      serverID,
		EnvironmentName: cfg.EnvironmentName,
		GitHubAppUUID:   gitHubAppUUID,
		GitRepository:   input.Repo,
		GitBranch:       input.Branch,
		PortsExposes:    input.Port,
		BuildPack:       coolify.BuildPack(input.BuildPack),
		Name:            input.Name,
	}

	resp, err := a.coolify.Applications.CreatePrivateGitHubApp(ctx, req)
	if err != nil {
		a.logger.Error("Failed to create Coolify application",
			"serviceID", input.ServiceID,
			"error", err)
		return nil, fmt.Errorf("failed to create Coolify application: %w", err)
	}

	_, err = a.servicesQ.UpdateServiceCoolifyApp(ctx, services.UpdateServiceCoolifyAppParams{
		ID:             input.ServiceID,
		CoolifyAppUuid: &resp.UUID,
	})
	if err != nil {
		a.logger.Error("Failed to update service with Coolify app UUID",
			"serviceID", input.ServiceID,
			"coolifyUUID", resp.UUID,
			"error", err)
		return nil, fmt.Errorf("failed to update service with Coolify app UUID: %w", err)
	}

	a.logger.Info("Created Coolify application",
		"serviceID", input.ServiceID,
		"coolifyUUID", resp.UUID)

	return &CreateAppResult{CoolifyAppUUID: resp.UUID, ServerID: serverID}, nil
}

type BulkUpdateEnvsInput struct {
	CoolifyAppUUID string
	EnvVars        []EnvVar
}

func (a *Activities) BulkUpdateEnvs(ctx context.Context, input BulkUpdateEnvsInput) error {
	if len(input.EnvVars) == 0 {
		a.logger.Info("No environment variables to update",
			"coolifyUUID", input.CoolifyAppUUID)
		return nil
	}

	a.logger.Info("Updating environment variables",
		"coolifyUUID", input.CoolifyAppUUID,
		"count", len(input.EnvVars))

	envReqs := make([]coolify.CreateEnvRequest, len(input.EnvVars))
	for i, ev := range input.EnvVars {
		envReqs[i] = coolify.CreateEnvRequest{
			Key:   ev.Key,
			Value: ev.Value,
		}
	}

	req := &coolify.BulkUpdateEnvsRequest{Data: envReqs}
	if err := a.coolify.Applications.BulkUpdateEnvs(ctx, input.CoolifyAppUUID, req); err != nil {
		a.logger.Error("Failed to bulk update environment variables",
			"coolifyUUID", input.CoolifyAppUUID,
			"error", err)
		return fmt.Errorf("failed to bulk update environment variables: %w", err)
	}

	a.logger.Info("Successfully updated environment variables",
		"coolifyUUID", input.CoolifyAppUUID,
		"count", len(input.EnvVars))

	return nil
}

type StartAppInput struct {
	CoolifyAppUUID string
}

type StartAppResult struct {
	DeploymentUUID string
}

func (a *Activities) StartApp(ctx context.Context, input StartAppInput) (*StartAppResult, error) {
	a.logger.Info("Starting application", "coolifyUUID", input.CoolifyAppUUID)

	resp, err := a.coolify.Applications.Start(ctx, input.CoolifyAppUUID, nil)
	if err != nil {
		a.logger.Error("Failed to start Coolify application",
			"coolifyUUID", input.CoolifyAppUUID,
			"error", err)
		return nil, fmt.Errorf("failed to start Coolify application: %w", err)
	}

	a.logger.Info("Started Coolify application",
		"coolifyUUID", input.CoolifyAppUUID,
		"deploymentUUID", resp.DeploymentUUID)

	return &StartAppResult{DeploymentUUID: resp.DeploymentUUID}, nil
}

type WaitForRunningInput struct {
	ServiceID      string
	CoolifyAppUUID string
}

type WaitForRunningResult struct {
	FQDN string
}

const (
	pollTimeout  = 3 * time.Minute
	pollInterval = 10 * time.Second
)

func (a *Activities) WaitForRunning(ctx context.Context, input WaitForRunningInput) (*WaitForRunningResult, error) {
	a.logger.Info("Waiting for app to be running",
		"serviceID", input.ServiceID,
		"coolifyUUID", input.CoolifyAppUUID,
		"timeout", pollTimeout)

	deadline := time.Now().Add(pollTimeout)

	for time.Now().Before(deadline) {
		// Check for cancellation
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		app, err := a.coolify.Applications.Get(ctx, input.CoolifyAppUUID)
		if err != nil {
			a.logger.Warn("Failed to get application status, will retry",
				"coolifyUUID", input.CoolifyAppUUID,
				"error", err)
			activity.RecordHeartbeat(ctx, fmt.Sprintf("API error: %v", err))
			time.Sleep(pollInterval)
			continue
		}

		fqdn := ""
		if app.FQDN != nil {
			fqdn = *app.FQDN
		}

		a.logger.Debug("Application status",
			"coolifyUUID", input.CoolifyAppUUID,
			"status", app.Status)

		activity.RecordHeartbeat(ctx, app.Status)

		// Status can be "running", "running:healthy", "running:unknown", etc.
		if strings.HasPrefix(app.Status, "running") {
			_, err = a.servicesQ.UpdateServiceRunning(ctx, services.UpdateServiceRunningParams{
				ID:   input.ServiceID,
				Fqdn: &fqdn,
			})
			if err != nil {
				a.logger.Error("Failed to update service to running",
					"serviceID", input.ServiceID,
					"error", err)
				return nil, fmt.Errorf("failed to update service to running: %w", err)
			}

			a.logger.Info("Application is running",
				"serviceID", input.ServiceID,
				"coolifyUUID", input.CoolifyAppUUID,
				"fqdn", fqdn)

			return &WaitForRunningResult{FQDN: fqdn}, nil
		}

		// Only treat explicit error/failed states as permanent failures
		if strings.HasPrefix(app.Status, "error") || app.Status == "failed" {
			errMsg := fmt.Sprintf("application failed with status: %s", app.Status)
			_, dbErr := a.servicesQ.UpdateServiceFailed(ctx, services.UpdateServiceFailedParams{
				ID:           input.ServiceID,
				ErrorMessage: &errMsg,
			})
			if dbErr != nil {
				a.logger.Error("Failed to update service as failed",
					"serviceID", input.ServiceID,
					"error", dbErr)
			}
			return nil, fmt.Errorf("application failed with status: %s", app.Status)
		}

		time.Sleep(pollInterval)
	}

	// Timeout - return error to trigger retry
	return nil, fmt.Errorf("app not running after %v, will retry", pollTimeout)
}

type UpdateServiceFailedInput struct {
	ServiceID    string
	ErrorMessage string
}

func (a *Activities) UpdateServiceFailed(ctx context.Context, input UpdateServiceFailedInput) error {
	a.logger.Info("Marking service as failed",
		"serviceID", input.ServiceID,
		"error", input.ErrorMessage)

	_, err := a.servicesQ.UpdateServiceFailed(ctx, services.UpdateServiceFailedParams{
		ID:           input.ServiceID,
		ErrorMessage: &input.ErrorMessage,
	})
	if err != nil {
		a.logger.Error("Failed to update service as failed",
			"serviceID", input.ServiceID,
			"error", err)
		return fmt.Errorf("failed to update service as failed: %w", err)
	}

	return nil
}
