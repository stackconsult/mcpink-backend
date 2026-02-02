package deployments

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/augustdev/autoclip/internal/coolify"
	"github.com/augustdev/autoclip/internal/storage/pg/generated/apps"
	"go.temporal.io/sdk/activity"
)

type Activities struct {
	coolify *coolify.Client
	appsQ   apps.Querier
	logger  *slog.Logger
}

func NewActivities(coolify *coolify.Client, appsQ apps.Querier, logger *slog.Logger) *Activities {
	return &Activities{
		coolify: coolify,
		appsQ:   appsQ,
		logger:  logger,
	}
}

func (a *Activities) CreateAppRecord(ctx context.Context, input CreateAppRecordInput) error {
	a.logger.Info("Creating app record",
		"appID", input.AppID,
		"userID", input.UserID,
		"repo", input.Repo,
		"workflowID", input.WorkflowID)

	serverID := a.coolify.GetMuscleServer()

	_, err := a.appsQ.CreateApp(ctx, apps.CreateAppParams{
		ID:            input.AppID,
		UserID:        input.UserID,
		ProjectID:     input.ProjectID,
		Repo:          input.Repo,
		Branch:        input.Branch,
		ServerUuid:    serverID,
		Name:          &input.Name,
		BuildPack:     input.BuildPack,
		Port:          input.Port,
		EnvVars:       input.EnvVars,
		WorkflowID:    input.WorkflowID,
		WorkflowRunID: &input.WorkflowRunID,
	})
	if err != nil {
		a.logger.Error("Failed to create app record",
			"appID", input.AppID,
			"userID", input.UserID,
			"repo", input.Repo,
			"error", err)
		return fmt.Errorf("failed to create app record: %w", err)
	}

	a.logger.Info("Created app record",
		"appID", input.AppID,
		"workflowID", input.WorkflowID)

	return nil
}

type CreateAppRecordInput struct {
	AppID         string
	UserID        string
	ProjectID     string
	WorkflowID    string
	WorkflowRunID string
	Repo          string
	Branch        string
	Name          string
	BuildPack     string
	Port          string
	EnvVars       []byte
}

type CoolifyAppInput struct {
	AppID         string
	GitHubAppUUID string
	Repo          string
	Branch        string
	Name          string
	BuildPack     string
	Port          string
}

type CoolifyAppResult struct {
	CoolifyAppUUID string
	ServerID       string
}

func (a *Activities) CreateAppFromPrivateGithub(ctx context.Context, input CoolifyAppInput) (*CoolifyAppResult, error) {
	a.logger.Info("Creating app from private GitHub",
		"appID", input.AppID,
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
		ProjectUUID:            cfg.ProjectUUID,
		ServerUUID:             serverID,
		EnvironmentName:        cfg.EnvironmentName,
		GitHubAppUUID:          gitHubAppUUID,
		GitRepository:          input.Repo,
		GitBranch:              input.Branch,
		PortsExposes:           input.Port,
		BuildPack:              coolify.BuildPack(input.BuildPack),
		Name:                   input.Name,
		CustomDockerRunOptions: "--runtime=runsc",
	}

	resp, err := a.coolify.Applications.CreatePrivateGitHubApp(ctx, req)
	if err != nil {
		a.logger.Error("Failed to create Coolify application",
			"appID", input.AppID,
			"error", err)
		return nil, fmt.Errorf("failed to create Coolify application: %w", err)
	}

	_, err = a.appsQ.UpdateAppCoolifyUUID(ctx, apps.UpdateAppCoolifyUUIDParams{
		ID:             input.AppID,
		CoolifyAppUuid: &resp.UUID,
	})
	if err != nil {
		a.logger.Error("Failed to update app with Coolify UUID",
			"appID", input.AppID,
			"coolifyUUID", resp.UUID,
			"error", err)
		return nil, fmt.Errorf("failed to update app with Coolify UUID: %w", err)
	}

	a.logger.Info("Created Coolify application",
		"appID", input.AppID,
		"coolifyUUID", resp.UUID)

	return &CoolifyAppResult{CoolifyAppUUID: resp.UUID, ServerID: serverID}, nil
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
	AppID          string
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
		"appID", input.AppID,
		"coolifyUUID", input.CoolifyAppUUID,
		"timeout", pollTimeout)

	deadline := time.Now().Add(pollTimeout)

	for time.Now().Before(deadline) {
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

		if strings.HasPrefix(app.Status, "running") {
			// Extract short commit hash (first 7 chars) from Coolify response
			var commitHash *string
			if app.GitCommitSHA != "" && len(app.GitCommitSHA) >= 7 {
				short := app.GitCommitSHA[:7]
				commitHash = &short
			}

			_, err = a.appsQ.UpdateAppRunning(ctx, apps.UpdateAppRunningParams{
				ID:         input.AppID,
				Fqdn:       &fqdn,
				CommitHash: commitHash,
			})
			if err != nil {
				a.logger.Error("Failed to update app to running",
					"appID", input.AppID,
					"error", err)
				return nil, fmt.Errorf("failed to update app to running: %w", err)
			}

			a.logger.Info("Application is running",
				"appID", input.AppID,
				"coolifyUUID", input.CoolifyAppUUID,
				"fqdn", fqdn,
				"commitHash", commitHash)

			return &WaitForRunningResult{FQDN: fqdn}, nil
		}

		if strings.HasPrefix(app.Status, "error") || app.Status == "failed" {
			errMsg := fmt.Sprintf("application failed with status: %s", app.Status)
			_, dbErr := a.appsQ.UpdateAppFailed(ctx, apps.UpdateAppFailedParams{
				ID:           input.AppID,
				ErrorMessage: &errMsg,
			})
			if dbErr != nil {
				a.logger.Error("Failed to update app as failed",
					"appID", input.AppID,
					"error", dbErr)
			}
			return nil, fmt.Errorf("application failed with status: %s", app.Status)
		}

		time.Sleep(pollInterval)
	}

	return nil, fmt.Errorf("app not running after %v, will retry", pollTimeout)
}

type UpdateAppFailedInput struct {
	AppID        string
	ErrorMessage string
}

func (a *Activities) UpdateAppFailed(ctx context.Context, input UpdateAppFailedInput) error {
	a.logger.Info("Marking app as failed",
		"appID", input.AppID,
		"error", input.ErrorMessage)

	_, err := a.appsQ.UpdateAppFailed(ctx, apps.UpdateAppFailedParams{
		ID:           input.AppID,
		ErrorMessage: &input.ErrorMessage,
	})
	if err != nil {
		a.logger.Error("Failed to update app as failed",
			"appID", input.AppID,
			"error", err)
		return fmt.Errorf("failed to update app as failed: %w", err)
	}

	return nil
}

type DeployAppInput struct {
	AppID          string
	CoolifyAppUUID string
}

type DeployAppResult struct {
	DeploymentUUID string
}

func (a *Activities) DeployApp(ctx context.Context, input DeployAppInput) (*DeployAppResult, error) {
	a.logger.Info("Deploying application",
		"appID", input.AppID,
		"coolifyUUID", input.CoolifyAppUUID)

	resp, err := a.coolify.Applications.Deploy(ctx, input.CoolifyAppUUID, nil)
	if err != nil {
		a.logger.Error("Failed to deploy Coolify application",
			"coolifyUUID", input.CoolifyAppUUID,
			"error", err)
		return nil, fmt.Errorf("failed to deploy Coolify application: %w", err)
	}

	deploymentUUID := ""
	if len(resp.Deployments) > 0 {
		deploymentUUID = resp.Deployments[0].DeploymentUUID
	}

	a.logger.Info("Deployed Coolify application",
		"appID", input.AppID,
		"coolifyUUID", input.CoolifyAppUUID,
		"deploymentUUID", deploymentUUID)

	return &DeployAppResult{DeploymentUUID: deploymentUUID}, nil
}

func (a *Activities) MarkAppBuilding(ctx context.Context, appID string) error {
	a.logger.Info("Marking app as building", "appID", appID)

	_, err := a.appsQ.UpdateAppRedeploying(ctx, appID)
	if err != nil {
		a.logger.Error("Failed to mark app as building",
			"appID", appID,
			"error", err)
		return fmt.Errorf("failed to mark app as building: %w", err)
	}

	return nil
}
