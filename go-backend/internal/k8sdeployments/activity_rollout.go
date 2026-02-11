package k8sdeployments

import (
	"context"
	"fmt"
	"time"

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
					return nil, ctx.Err()
				case <-time.After(waitForRolloutPollInterval):
					continue
				}
			}
			return nil, fmt.Errorf("get deployment: %w", err)
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
			return nil, ctx.Err()
		case <-time.After(waitForRolloutPollInterval):
		}
	}
}
