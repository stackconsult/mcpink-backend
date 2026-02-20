package k8sdeployments

import (
	"context"
	"fmt"

	deploymentsdb "github.com/augustdev/autoclip/internal/storage/pg/generated/deployments"
	"github.com/augustdev/autoclip/internal/storage/pg/generated/services"
)

func (a *Activities) UpdateDeploymentBuilding(ctx context.Context, input UpdateDeploymentBuildingInput) error {
	if err := a.deploymentsQ.UpdateDeploymentBuilding(ctx, input.DeploymentID); err != nil {
		return fmt.Errorf("update deployment building: %w", err)
	}
	a.logger.Info("Deployment status → building", "deploymentID", input.DeploymentID)
	return nil
}

func (a *Activities) UpdateDeploymentDeploying(ctx context.Context, input UpdateDeploymentDeployingInput) error {
	if err := a.deploymentsQ.UpdateDeploymentDeploying(ctx, input.DeploymentID); err != nil {
		return fmt.Errorf("update deployment deploying: %w", err)
	}
	a.logger.Info("Deployment status → deploying", "deploymentID", input.DeploymentID)
	return nil
}

func (a *Activities) MarkDeploymentActive(ctx context.Context, input MarkDeploymentActiveInput) error {
	// Supersede the currently-active deployment (if any)
	if err := a.deploymentsQ.SupersedeActiveDeployment(ctx, input.ServiceID); err != nil {
		return fmt.Errorf("supersede active deployment: %w", err)
	}

	// Mark this deployment as active
	if err := a.deploymentsQ.MarkDeploymentActive(ctx, deploymentsdb.MarkDeploymentActiveParams{
		ID:         input.DeploymentID,
		CommitHash: &input.CommitSHA,
		ImageRef:   &input.ImageRef,
	}); err != nil {
		return fmt.Errorf("mark deployment active: %w", err)
	}

	// Set the pointer on the service
	if err := a.servicesQ.SetCurrentDeploymentID(ctx, services.SetCurrentDeploymentIDParams{
		ID:                  input.ServiceID,
		CurrentDeploymentID: &input.DeploymentID,
	}); err != nil {
		return fmt.Errorf("set current deployment id: %w", err)
	}

	// Update service FQDN
	if input.URL != "" {
		if err := a.servicesQ.SetServiceFQDN(ctx, services.SetServiceFQDNParams{
			ID:   input.ServiceID,
			Fqdn: &input.URL,
		}); err != nil {
			a.logger.Warn("Failed to set service FQDN", "serviceID", input.ServiceID, "error", err)
		}
	}

	a.logger.Info("Deployment status → active",
		"deploymentID", input.DeploymentID,
		"serviceID", input.ServiceID)
	return nil
}

func (a *Activities) MarkDeploymentFailed(ctx context.Context, input MarkDeploymentFailedInput) error {
	if err := a.deploymentsQ.MarkDeploymentFailed(ctx, deploymentsdb.MarkDeploymentFailedParams{
		ID:           input.DeploymentID,
		ErrorMessage: &input.ErrorMessage,
	}); err != nil {
		return fmt.Errorf("mark deployment failed: %w", err)
	}
	a.logger.Info("Deployment status → failed",
		"deploymentID", input.DeploymentID,
		"error", input.ErrorMessage)
	return nil
}

func (a *Activities) UpdateDeploymentBuildProgress(ctx context.Context, input UpdateDeploymentBuildProgressInput) error {
	if err := a.deploymentsQ.UpdateDeploymentBuildProgress(ctx, deploymentsdb.UpdateDeploymentBuildProgressParams{
		ID:            input.DeploymentID,
		BuildProgress: input.BuildProgress,
	}); err != nil {
		return fmt.Errorf("update deployment build progress: %w", err)
	}
	return nil
}

func (a *Activities) MarkDeploymentCrashed(ctx context.Context, input MarkDeploymentCrashedInput) error {
	if err := a.deploymentsQ.MarkDeploymentCrashed(ctx, deploymentsdb.MarkDeploymentCrashedParams{
		ID:           input.DeploymentID,
		ErrorMessage: &input.ErrorMessage,
	}); err != nil {
		return fmt.Errorf("mark deployment crashed: %w", err)
	}
	a.logger.Info("Deployment status → crashed",
		"deploymentID", input.DeploymentID,
		"error", input.ErrorMessage)
	return nil
}

func (a *Activities) MarkDeploymentCompleted(ctx context.Context, input MarkDeploymentCompletedInput) error {
	if err := a.deploymentsQ.MarkDeploymentCompleted(ctx, input.DeploymentID); err != nil {
		return fmt.Errorf("mark deployment completed: %w", err)
	}
	a.logger.Info("Deployment status → completed",
		"deploymentID", input.DeploymentID)
	return nil
}

func (a *Activities) MarkDeploymentRemoved(ctx context.Context, input MarkDeploymentRemovedInput) error {
	if err := a.deploymentsQ.MarkDeploymentRemoved(ctx, input.DeploymentID); err != nil {
		return fmt.Errorf("mark deployment removed: %w", err)
	}
	a.logger.Info("Deployment status → removed",
		"deploymentID", input.DeploymentID)
	return nil
}

func (a *Activities) SoftDeleteService(ctx context.Context, serviceID string) error {
	_, err := a.servicesQ.SoftDeleteService(ctx, serviceID)
	if err != nil {
		return fmt.Errorf("soft delete service: %w", err)
	}
	a.logger.Info("Soft-deleted service", "serviceID", serviceID)
	return nil
}
