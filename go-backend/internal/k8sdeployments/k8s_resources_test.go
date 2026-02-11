package k8sdeployments

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

func TestBuildResourceQuota_Defaults(t *testing.T) {
	quota := buildResourceQuota("ns")

	if quota.Name != "quota" {
		t.Fatalf("name = %q, want %q", quota.Name, "quota")
	}
	if quota.Namespace != "ns" {
		t.Fatalf("namespace = %q, want %q", quota.Namespace, "ns")
	}

	assertResourceQuantity(t, quota.Spec.Hard, corev1.ResourcePods, "40")
	assertResourceQuantity(t, quota.Spec.Hard, corev1.ResourceRequestsCPU, "8")
	assertResourceQuantity(t, quota.Spec.Hard, corev1.ResourceRequestsMemory, "8Gi")
	assertResourceQuantity(t, quota.Spec.Hard, corev1.ResourceLimitsCPU, "16")
	assertResourceQuantity(t, quota.Spec.Hard, corev1.ResourceLimitsMemory, "16Gi")
}

func assertResourceQuantity(t *testing.T, rl corev1.ResourceList, name corev1.ResourceName, expected string) {
	t.Helper()

	got, ok := rl[name]
	if !ok {
		t.Fatalf("missing resource %q", name)
	}

	want := resource.MustParse(expected)
	if got.Cmp(want) != 0 {
		t.Fatalf("resource %q = %s, want %s", name, got.String(), want.String())
	}
}
