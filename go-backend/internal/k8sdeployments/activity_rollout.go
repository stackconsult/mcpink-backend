package k8sdeployments

import (
	"context"
	"fmt"
	"strings"
	"time"

	"go.temporal.io/sdk/temporal"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var waitForRolloutPollInterval = 5 * time.Second

func (a *Activities) WaitForRollout(ctx context.Context, input WaitForRolloutInput) (*WaitForRolloutResult, error) {
	for {
		dep, err := a.k8s.AppsV1().Deployments(input.Namespace).Get(ctx, input.DeploymentName, metav1.GetOptions{})
		if err != nil {
			if apierrors.IsNotFound(err) {
				select {
				case <-ctx.Done():
					return nil, fmt.Errorf("wait for rollout timed out waiting for deployment %s/%s to exist: %w", input.Namespace, input.DeploymentName, ctx.Err())
				case <-time.After(waitForRolloutPollInterval):
					continue
				}
			}
			return nil, fmt.Errorf("get deployment: %w", err)
		}

		if err := deploymentRolloutFailure(dep); err != nil {
			return nil, temporal.NewNonRetryableApplicationError(
				fmt.Sprintf("rollout failed for %s/%s: %v", input.Namespace, input.DeploymentName, err),
				"deployment_rollout_failed",
				err,
			)
		}

		desired := int32(1)
		if dep.Spec.Replicas != nil {
			desired = *dep.Spec.Replicas
		}

		if dep.Status.UpdatedReplicas == desired && dep.Status.AvailableReplicas == desired {
			return &WaitForRolloutResult{Status: StatusRunning}, nil
		}

		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("wait for rollout timed out for %s/%s: %s: %w",
				input.Namespace, input.DeploymentName, deploymentRolloutSummary(dep), ctx.Err())
		case <-time.After(waitForRolloutPollInterval):
		}
	}
}

func deploymentRolloutFailure(dep *appsv1.Deployment) error {
	for _, condition := range dep.Status.Conditions {
		switch condition.Type {
		case appsv1.DeploymentReplicaFailure:
			if condition.Status == corev1.ConditionTrue {
				return fmt.Errorf("replica failure (%s): %s", condition.Reason, strings.TrimSpace(condition.Message))
			}
		case appsv1.DeploymentProgressing:
			if condition.Status == corev1.ConditionFalse && condition.Reason == "ProgressDeadlineExceeded" {
				return fmt.Errorf("progress deadline exceeded: %s", strings.TrimSpace(condition.Message))
			}
		}
	}
	return nil
}

func deploymentRolloutSummary(dep *appsv1.Deployment) string {
	desired := int32(1)
	if dep.Spec.Replicas != nil {
		desired = *dep.Spec.Replicas
	}

	summary := fmt.Sprintf("desired=%d updated=%d available=%d unavailable=%d",
		desired, dep.Status.UpdatedReplicas, dep.Status.AvailableReplicas, dep.Status.UnavailableReplicas)

	if len(dep.Status.Conditions) == 0 {
		return summary
	}

	parts := make([]string, 0, len(dep.Status.Conditions))
	for _, condition := range dep.Status.Conditions {
		parts = append(parts,
			fmt.Sprintf("%s=%s(%s)", condition.Type, condition.Status, condition.Reason))
	}
	return summary + ", conditions=[" + strings.Join(parts, ", ") + "]"
}
