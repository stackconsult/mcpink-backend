package k8sdeployments

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
)

func (a *Activities) DeleteService(ctx context.Context, input DeleteServiceInput) (*DeleteServiceResult, error) {
	a.logger.Info("DeleteService activity started",
		"serviceID", input.ServiceID,
		"namespace", input.Namespace,
		"name", input.Name)

	// Delete Ingress
	err := a.k8s.NetworkingV1().Ingresses(input.Namespace).Delete(ctx, input.Name, metav1.DeleteOptions{})
	if err != nil && !apierrors.IsNotFound(err) {
		return nil, fmt.Errorf("delete ingress: %w", err)
	}

	// Delete Service
	err = a.k8s.CoreV1().Services(input.Namespace).Delete(ctx, input.Name, metav1.DeleteOptions{})
	if err != nil && !apierrors.IsNotFound(err) {
		return nil, fmt.Errorf("delete service: %w", err)
	}

	// Delete Deployment
	err = a.k8s.AppsV1().Deployments(input.Namespace).Delete(ctx, input.Name, metav1.DeleteOptions{})
	if err != nil && !apierrors.IsNotFound(err) {
		return nil, fmt.Errorf("delete deployment: %w", err)
	}

	// Delete Secret
	err = a.k8s.CoreV1().Secrets(input.Namespace).Delete(ctx, input.Name+"-env", metav1.DeleteOptions{})
	if err != nil && !apierrors.IsNotFound(err) {
		return nil, fmt.Errorf("delete secret: %w", err)
	}

	// Clean up namespace if no deployments remain
	deployments, err := a.k8s.AppsV1().Deployments(input.Namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		a.logger.Warn("Failed to list deployments for namespace cleanup",
			"namespace", input.Namespace, "error", err)
	} else if len(deployments.Items) == 0 {
		if err := a.k8s.CoreV1().Namespaces().Delete(ctx, input.Namespace, metav1.DeleteOptions{}); err != nil && !apierrors.IsNotFound(err) {
			a.logger.Warn("Failed to delete empty namespace",
				"namespace", input.Namespace, "error", err)
		} else {
			a.logger.Info("Deleted empty namespace", "namespace", input.Namespace)
		}
	}

	a.logger.Info("DeleteService completed",
		"serviceID", input.ServiceID,
		"namespace", input.Namespace,
		"name", input.Name)

	return &DeleteServiceResult{Status: StatusDeleted}, nil
}
