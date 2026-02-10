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

	a.logger.Info("DeleteService completed",
		"serviceID", input.ServiceID,
		"namespace", input.Namespace,
		"name", input.Name)

	return &DeleteServiceResult{Status: StatusDeleted}, nil
}
