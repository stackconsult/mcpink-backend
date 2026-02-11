package k8sdeployments

import (
	"context"
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestWaitForRollout_RetriesUntilDeploymentExists(t *testing.T) {
	prev := waitForRolloutPollInterval
	waitForRolloutPollInterval = 5 * time.Millisecond
	t.Cleanup(func() { waitForRolloutPollInterval = prev })

	client := fake.NewSimpleClientset()
	a := &Activities{k8s: client}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	const namespace = "ns"
	const name = "test-react-vite"
	go func() {
		time.Sleep(20 * time.Millisecond)
		replicas := int32(1)
		_, _ = client.AppsV1().Deployments(namespace).Create(ctx, &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: namespace,
			},
			Spec: appsv1.DeploymentSpec{
				Replicas: &replicas,
			},
			Status: appsv1.DeploymentStatus{
				UpdatedReplicas:   1,
				AvailableReplicas: 1,
			},
		}, metav1.CreateOptions{})
	}()

	res, err := a.WaitForRollout(ctx, WaitForRolloutInput{
		Namespace:      namespace,
		DeploymentName: name,
	})
	if err != nil {
		t.Fatalf("WaitForRollout() error = %v", err)
	}
	if res.Status != StatusRunning {
		t.Fatalf("WaitForRollout() status = %q, want %q", res.Status, StatusRunning)
	}
}
