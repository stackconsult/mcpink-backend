package deployments

import (
	"context"
	"fmt"
	"log/slog"
	"math/rand"
	"strings"
	"time"

	"github.com/augustdev/autoclip/internal/cloudflare"
	"github.com/augustdev/autoclip/internal/coolify"
	"github.com/augustdev/autoclip/internal/helpers"
	"github.com/augustdev/autoclip/internal/storage/pg/generated/apps"
	"github.com/augustdev/autoclip/internal/storage/pg/generated/dnsrecords"
	"go.temporal.io/sdk/activity"
)

type Activities struct {
	coolify    *coolify.Client
	cloudflare *cloudflare.Client
	appsQ      apps.Querier
	dnsQ       dnsrecords.Querier
	logger     *slog.Logger
}

func NewActivities(
	coolify *coolify.Client,
	cloudflare *cloudflare.Client,
	appsQ apps.Querier,
	dnsQ dnsrecords.Querier,
	logger *slog.Logger,
) *Activities {
	return &Activities{
		coolify:    coolify,
		cloudflare: cloudflare,
		appsQ:      appsQ,
		dnsQ:       dnsQ,
		logger:     logger,
	}
}

func (a *Activities) CreateAppRecord(ctx context.Context, input CreateAppRecordInput) error {
	a.logger.Info("Creating app record",
		"appID", input.AppID,
		"userID", input.UserID,
		"repo", input.Repo,
		"workflowID", input.WorkflowID)

	serverID := a.coolify.GetMuscleServer()

	gitProvider := input.GitProvider
	if gitProvider == "" {
		gitProvider = "github"
	}

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
		GitProvider:   gitProvider,
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
	GitProvider   string
}

type CoolifyAppInput struct {
	AppID          string
	GitHubAppUUID  string
	Repo           string
	Branch         string
	Name           string
	BuildPack      string
	Port           string
	Memory         string
	CPU            string
	InstallCommand string
	BuildCommand   string
	StartCommand   string
}

type InternalGitAppInput struct {
	AppID          string
	PrivateKeyUUID string
	SSHCloneURL    string
	Branch         string
	Name           string
	BuildPack      string
	Port           string
	Memory         string
	CPU            string
	InstallCommand string
	BuildCommand   string
	StartCommand   string
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

	// Static build pack uses nginx:alpine which serves on port 80
	port := input.Port
	if input.BuildPack == "static" {
		port = "80"
	}

	// Apply defaults for resource limits
	memory := input.Memory
	if memory == "" {
		memory = "256m"
	}
	cpu := input.CPU
	if cpu == "" {
		cpu = "0.5"
	}

	req := &coolify.CreatePrivateGitHubAppRequest{
		ProjectUUID:            cfg.ProjectUUID,
		ServerUUID:             serverID,
		EnvironmentName:        cfg.EnvironmentName,
		GitHubAppUUID:          gitHubAppUUID,
		GitRepository:          input.Repo,
		GitBranch:              input.Branch,
		PortsExposes:           port,
		BuildPack:              coolify.BuildPack(input.BuildPack),
		Name:                   input.Name,
		CustomDockerRunOptions: "--runtime=runsc --cap-drop=ALL",
		LimitsMemory:           memory,
		LimitsMemorySwap:       memory, // Same as memory = no swap
		LimitsCPUs:             cpu,
		InstallCommand:         input.InstallCommand,
		BuildCommand:           input.BuildCommand,
		StartCommand:           input.StartCommand,
	}

	// Configure build server if enabled
	if a.coolify.UseBuildServer() {
		useBuildServer := true
		req.UseBuildServer = &useBuildServer
		req.DockerRegistryImageName = fmt.Sprintf("%s/%s", a.coolify.GetRegistryHost(), input.Name)
		a.logger.Info("Using build server with registry",
			"registryImage", req.DockerRegistryImageName)
	}

	a.logger.Debug("Coolify create app request",
		"projectUUID", cfg.ProjectUUID,
		"serverUUID", serverID,
		"environmentName", cfg.EnvironmentName,
		"gitHubAppUUID", gitHubAppUUID,
		"gitRepository", input.Repo,
		"gitBranch", input.Branch,
		"portsExposes", input.Port,
		"buildPack", input.BuildPack,
		"name", input.Name)

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

func (a *Activities) CreateAppFromInternalGit(ctx context.Context, input InternalGitAppInput) (*CoolifyAppResult, error) {
	a.logger.Info("Creating app from internal git",
		"appID", input.AppID,
		"sshCloneURL", input.SSHCloneURL,
		"branch", input.Branch,
		"privateKeyUUID", input.PrivateKeyUUID)

	cfg := a.coolify.Config()
	serverID := a.coolify.GetMuscleServer()

	// Static build pack uses nginx:alpine which serves on port 80
	port := input.Port
	if input.BuildPack == "static" {
		port = "80"
	}

	// Apply defaults for resource limits
	memory := input.Memory
	if memory == "" {
		memory = "256m"
	}
	cpu := input.CPU
	if cpu == "" {
		cpu = "0.5"
	}

	req := &coolify.CreatePrivateDeployKeyRequest{
		ProjectUUID:            cfg.ProjectUUID,
		ServerUUID:             serverID,
		EnvironmentName:        cfg.EnvironmentName,
		PrivateKeyUUID:         input.PrivateKeyUUID,
		GitRepository:          input.SSHCloneURL,
		GitBranch:              input.Branch,
		PortsExposes:           port,
		BuildPack:              coolify.BuildPack(input.BuildPack),
		Name:                   input.Name,
		CustomDockerRunOptions: "--runtime=runsc --cap-drop=ALL",
		LimitsMemory:           memory,
		LimitsMemorySwap:       memory, // Same as memory = no swap
		LimitsCPUs:             cpu,
		InstallCommand:         input.InstallCommand,
		BuildCommand:           input.BuildCommand,
		StartCommand:           input.StartCommand,
	}

	// Configure build server if enabled
	if a.coolify.UseBuildServer() {
		useBuildServer := true
		req.UseBuildServer = &useBuildServer
		req.DockerRegistryImageName = fmt.Sprintf("%s/%s", a.coolify.GetRegistryHost(), input.Name)
		a.logger.Info("Using build server with registry",
			"registryImage", req.DockerRegistryImageName)
	}

	resp, err := a.coolify.Applications.CreatePrivateDeployKey(ctx, req)
	if err != nil {
		a.logger.Error("Failed to create Coolify application from internal git",
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

	a.logger.Info("Created Coolify application from internal git",
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
			Key:         ev.Key,
			Value:       ev.Value,
			IsBuildTime: helpers.Ptr(ev.IsBuildTime),
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
	DeploymentUUID string
}

type WaitForRunningResult struct {
	FQDN string
}

const (
	pollTimeout  = 3 * time.Minute
	pollInterval = 10 * time.Second
)

func (a *Activities) WaitForRunning(ctx context.Context, input WaitForRunningInput) (*WaitForRunningResult, error) {
	a.logger.Info("Waiting for deployment to complete",
		"appID", input.AppID,
		"coolifyUUID", input.CoolifyAppUUID,
		"deploymentUUID", input.DeploymentUUID,
		"timeout", pollTimeout)

	deadline := time.Now().Add(pollTimeout)

	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		deployment, err := a.coolify.Applications.GetDeployment(ctx, input.CoolifyAppUUID, input.DeploymentUUID)
		if err != nil {
			a.logger.Warn("Failed to get deployment status, will retry",
				"coolifyUUID", input.CoolifyAppUUID,
				"deploymentUUID", input.DeploymentUUID,
				"error", err)
			activity.RecordHeartbeat(ctx, fmt.Sprintf("API error: %v", err))
			time.Sleep(pollInterval)
			continue
		}

		a.logger.Debug("Deployment status",
			"coolifyUUID", input.CoolifyAppUUID,
			"deploymentUUID", input.DeploymentUUID,
			"status", deployment.Status)

		activity.RecordHeartbeat(ctx, deployment.Status)

		switch deployment.Status {
		case "finished":
			// Deployment complete - get app info for FQDN and commit hash
			app, err := a.coolify.Applications.Get(ctx, input.CoolifyAppUUID)
			if err != nil {
				a.logger.Error("Failed to get application after deployment",
					"coolifyUUID", input.CoolifyAppUUID,
					"error", err)
				return nil, fmt.Errorf("failed to get application after deployment: %w", err)
			}

			fqdn := ""
			if app.FQDN != nil {
				fqdn = *app.FQDN
			}

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

			a.logger.Info("Deployment completed successfully",
				"appID", input.AppID,
				"coolifyUUID", input.CoolifyAppUUID,
				"deploymentUUID", input.DeploymentUUID,
				"fqdn", fqdn,
				"commitHash", commitHash)

			return &WaitForRunningResult{FQDN: fqdn}, nil

		case "failed", "cancelled_by_user":
			errMsg := fmt.Sprintf("deployment %s: %s", deployment.Status, input.DeploymentUUID)

			// Attach a small, readable log snippet to help debug (frequently contains git/auth failures).
			if logs, logErr := a.coolify.Applications.GetDeploymentLogsForUUID(ctx, input.CoolifyAppUUID, input.DeploymentUUID); logErr == nil && len(logs) > 0 {
				const maxLines = 25
				start := 0
				if len(logs) > maxLines {
					start = len(logs) - maxLines
				}

				var b strings.Builder
				for _, entry := range logs[start:] {
					line := strings.TrimSpace(entry.Message)
					if line == "" {
						continue
					}
					// Keep each line compact (avoid flooding DB/errors).
					if len(line) > 300 {
						line = line[:300] + "…"
					}
					b.WriteString(line)
					b.WriteString("\n")
				}

				snippet := strings.TrimSpace(b.String())
				if snippet != "" {
					errMsg = errMsg + "\n" + snippet
				}
			}

			_, dbErr := a.appsQ.UpdateAppFailed(ctx, apps.UpdateAppFailedParams{
				ID:           input.AppID,
				ErrorMessage: &errMsg,
			})
			if dbErr != nil {
				a.logger.Error("Failed to update app as failed",
					"appID", input.AppID,
					"error", dbErr)
			}
			return nil, fmt.Errorf("deployment %s: %s", deployment.Status, input.DeploymentUUID)

		case "queued", "in_progress":
			// Still in progress, keep polling
		}

		time.Sleep(pollInterval)
	}

	return nil, fmt.Errorf("deployment not finished after %v, will retry", pollTimeout)
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
		// Prefer a matching resource UUID (Coolify returns multiple deployments in some cases).
		for i := range resp.Deployments {
			if resp.Deployments[i].ResourceUUID == input.CoolifyAppUUID && resp.Deployments[i].DeploymentUUID != "" {
				deploymentUUID = resp.Deployments[i].DeploymentUUID
			}
		}
		if deploymentUUID == "" {
			deploymentUUID = resp.Deployments[len(resp.Deployments)-1].DeploymentUUID
		}
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

type CreateDNSRecordInput struct {
	AppID      string
	AppName    string
	ServerUUID string
}

type CreateDNSRecordResult struct {
	FQDN         string
	DNSRecordID  string
	CloudflareID string
}

func (a *Activities) CreateDNSRecord(ctx context.Context, input CreateDNSRecordInput) (*CreateDNSRecordResult, error) {
	serverIP := a.coolify.GetServerIP(input.ServerUUID)
	if serverIP == "" {
		a.logger.Warn("No IP mapping for server, skipping DNS record creation",
			"appID", input.AppID,
			"serverUUID", input.ServerUUID)
		return &CreateDNSRecordResult{}, nil
	}

	baseName := cloudflare.SanitizeSubdomain(input.AppName)

	// Retry loop for atomic subdomain generation with collision handling
	for attempt := 0; attempt < 10; attempt++ {
		suffix := generateRandomSuffix(4)
		subdomain := fmt.Sprintf("%s-%s", baseName, suffix)

		a.logger.Info("Creating DNS record",
			"appID", input.AppID,
			"subdomain", subdomain,
			"serverIP", serverIP,
			"attempt", attempt+1)

		// 1. Create in Cloudflare first
		record, err := a.cloudflare.CreateARecord(ctx, subdomain, serverIP)
		if err != nil {
			a.logger.Warn("Cloudflare create failed, retrying",
				"appID", input.AppID,
				"subdomain", subdomain,
				"attempt", attempt+1,
				"error", err)
			continue
		}

		// 2. Store in DB (unique constraint on subdomain catches race conditions)
		dbRecord, err := a.dnsQ.CreateDNSRecord(ctx, dnsrecords.CreateDNSRecordParams{
			AppID:              &input.AppID,
			CloudflareRecordID: record.ID,
			Subdomain:          subdomain,
			FullDomain:         record.FullDomain,
			TargetIp:           serverIP,
		})
		if err != nil {
			// Unique constraint violation = race condition, cleanup and retry
			_ = a.cloudflare.DeleteDNSRecord(ctx, record.ID)
			a.logger.Warn("DB insert failed (likely collision), retrying",
				"appID", input.AppID,
				"subdomain", subdomain,
				"attempt", attempt+1,
				"error", err)
			continue
		}

		fqdn := fmt.Sprintf("https://%s", record.FullDomain)

		a.logger.Info("Created DNS record",
			"appID", input.AppID,
			"fqdn", fqdn,
			"cloudflareID", record.ID,
			"dnsRecordID", dbRecord.ID)

		return &CreateDNSRecordResult{
			FQDN:         fqdn,
			DNSRecordID:  dbRecord.ID,
			CloudflareID: record.ID,
		}, nil
	}

	return nil, fmt.Errorf("failed to create DNS record after 10 attempts for app %s", input.AppID)
}

func generateRandomSuffix(length int) string {
	const charset = "abcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, length)
	for i := range b {
		b[i] = charset[rand.Intn(len(charset))]
	}
	return string(b)
}

type UpdateCoolifyDomainInput struct {
	CoolifyAppUUID string
	Domain         string
}

func (a *Activities) UpdateCoolifyDomain(ctx context.Context, input UpdateCoolifyDomainInput) error {
	if input.Domain == "" {
		return nil
	}

	a.logger.Info("Updating Coolify app domain",
		"coolifyUUID", input.CoolifyAppUUID,
		"domain", input.Domain)

	_, err := a.coolify.Applications.Update(ctx, input.CoolifyAppUUID, &coolify.UpdateApplicationRequest{
		Domains: &input.Domain,
	})
	if err != nil {
		a.logger.Error("Failed to update Coolify app domain",
			"coolifyUUID", input.CoolifyAppUUID,
			"domain", input.Domain,
			"error", err)
		return fmt.Errorf("failed to update Coolify domain: %w", err)
	}

	a.logger.Info("Updated Coolify app domain",
		"coolifyUUID", input.CoolifyAppUUID,
		"domain", input.Domain)

	return nil
}

func (a *Activities) DeleteDNSRecord(ctx context.Context, appID string) error {
	record, err := a.dnsQ.GetDNSRecordByAppID(ctx, &appID)
	if err != nil {
		// No DNS record for this app - not an error
		a.logger.Debug("No DNS record found for app", "appID", appID)
		return nil
	}

	a.logger.Info("Deleting DNS record",
		"appID", appID,
		"cloudflareID", record.CloudflareRecordID)

	if err := a.cloudflare.DeleteDNSRecord(ctx, record.CloudflareRecordID); err != nil {
		return fmt.Errorf("failed to delete DNS record from Cloudflare: %w", err)
	}

	if err := a.dnsQ.DeleteDNSRecordByAppID(ctx, &appID); err != nil {
		return fmt.Errorf("failed to delete DNS record from database: %w", err)
	}

	a.logger.Info("Deleted DNS record", "appID", appID)
	return nil
}

type DeleteAppFromCoolifyInput struct {
	AppID          string
	CoolifyAppUUID string
}

func (a *Activities) DeleteAppFromCoolify(ctx context.Context, input DeleteAppFromCoolifyInput) error {
	a.logger.Info("Deleting app from Coolify",
		"appID", input.AppID,
		"coolifyUUID", input.CoolifyAppUUID)

	if err := a.coolify.Applications.Delete(ctx, input.CoolifyAppUUID); err != nil {
		a.logger.Error("Failed to delete app from Coolify",
			"appID", input.AppID,
			"coolifyUUID", input.CoolifyAppUUID,
			"error", err)
		return fmt.Errorf("failed to delete app from Coolify: %w", err)
	}

	a.logger.Info("Deleted app from Coolify",
		"appID", input.AppID,
		"coolifyUUID", input.CoolifyAppUUID)

	return nil
}

func (a *Activities) SoftDeleteApp(ctx context.Context, appID string) error {
	a.logger.Info("Soft deleting app", "appID", appID)

	_, err := a.appsQ.SoftDeleteApp(ctx, appID)
	if err != nil {
		a.logger.Error("Failed to soft delete app",
			"appID", appID,
			"error", err)
		return fmt.Errorf("failed to soft delete app: %w", err)
	}

	a.logger.Info("Soft deleted app", "appID", appID)

	return nil
}
