package k8sdeployments

import (
	"context"
	"fmt"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func (a *Activities) WaitForRollout(ctx context.Context, input WaitForRolloutInput) (*WaitForRolloutResult, error) {
	a.logger.Info("WaitForRollout activity started",
		"namespace", input.Namespace,
		"deployment", input.DeploymentName)

	timeout := 120 * time.Second
	interval := 5 * time.Second
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		dep, err := a.k8s.AppsV1().Deployments(input.Namespace).Get(ctx, input.DeploymentName, metav1.GetOptions{})
		if err != nil {
			return nil, fmt.Errorf("get deployment: %w", err)
		}

		desired := int32(1)
		if dep.Spec.Replicas != nil {
			desired = *dep.Spec.Replicas
		}
		updated := dep.Status.UpdatedReplicas
		available := dep.Status.AvailableReplicas

		if updated == desired && available == desired {
			a.logger.Info("WaitForRollout completed",
				"namespace", input.Namespace,
				"deployment", input.DeploymentName)
			return &WaitForRolloutResult{Status: StatusRunning}, nil
		}

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(interval):
		}
	}

	return nil, fmt.Errorf("deployment rollout timed out after %v", timeout)
}
