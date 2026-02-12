package k8sdeployments

import (
	"context"
	"fmt"

	"github.com/augustdev/autoclip/internal/storage/pg/generated/services"
)

func (a *Activities) UpdateBuildStatus(ctx context.Context, input UpdateBuildStatusInput) error {
	_, err := a.servicesQ.UpdateBuildStatus(ctx, services.UpdateBuildStatusParams{
		ID:          input.ServiceID,
		BuildStatus: input.BuildStatus,
	})
	if err != nil {
		return fmt.Errorf("update build status to %s: %w", input.BuildStatus, err)
	}
	a.logger.Info("Updated build status", "serviceID", input.ServiceID, "status", input.BuildStatus)
	return nil
}

func (a *Activities) MarkServiceRunning(ctx context.Context, input MarkServiceRunningInput) error {
	_, err := a.servicesQ.UpdateServiceRunning(ctx, services.UpdateServiceRunningParams{
		ID:         input.ServiceID,
		Fqdn:       &input.URL,
		CommitHash: &input.CommitSHA,
	})
	if err != nil {
		return fmt.Errorf("mark service running: %w", err)
	}
	a.logger.Info("Marked service running", "serviceID", input.ServiceID, "url", input.URL)
	return nil
}

func (a *Activities) MarkServiceFailed(ctx context.Context, input MarkServiceFailedInput) error {
	_, err := a.servicesQ.UpdateServiceFailed(ctx, services.UpdateServiceFailedParams{
		ID:           input.ServiceID,
		ErrorMessage: &input.ErrorMessage,
	})
	if err != nil {
		return fmt.Errorf("mark service failed: %w", err)
	}
	a.logger.Info("Marked service failed", "serviceID", input.ServiceID, "error", input.ErrorMessage)
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
