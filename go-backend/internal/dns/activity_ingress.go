package dns

import (
	"context"
	"encoding/json"
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
)

var ingressRouteGVR = schema.GroupVersionResource{
	Group:    "traefik.io",
	Version:  "v1alpha1",
	Resource: "ingressroutes",
}

const certNamespace = "dp-system"

func (a *Activities) ApplySubdomainIngress(ctx context.Context, input ApplySubdomainIngressInput) error {
	a.logger.Info("ApplySubdomainIngress",
		"namespace", input.Namespace,
		"serviceName", input.ServiceName,
		"fqdn", input.FQDN)

	ingressRoute := buildSubdomainIngressRoute(
		input.Namespace,
		input.ServiceName,
		input.FQDN,
		input.CertSecret,
		input.ServicePort,
	)

	data, err := json.Marshal(ingressRoute)
	if err != nil {
		return fmt.Errorf("marshal subdomain ingressroute: %w", err)
	}

	ingressName := input.ServiceName + "-dz"
	_, err = a.dynClient.Resource(ingressRouteGVR).Namespace(input.Namespace).Patch(
		ctx,
		ingressName,
		types.ApplyPatchType,
		data,
		metav1.PatchOptions{FieldManager: "temporal-worker"},
	)
	if err != nil {
		return fmt.Errorf("apply subdomain ingressroute: %w", err)
	}

	return nil
}

func (a *Activities) DeleteIngress(ctx context.Context, input DeleteIngressInput) error {
	a.logger.Info("DeleteIngress", "namespace", input.Namespace, "ingressName", input.IngressName)

	err := a.dynClient.Resource(ingressRouteGVR).Namespace(input.Namespace).Delete(ctx, input.IngressName, metav1.DeleteOptions{})
	if err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("delete ingressroute: %w", err)
	}
	return nil
}

func buildSubdomainIngressRoute(namespace, serviceName, fqdn, certSecretName string, port int32) *unstructured.Unstructured {
	ingressName := serviceName + "-dz"

	return &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "traefik.io/v1alpha1",
			"kind":       "IngressRoute",
			"metadata": map[string]any{
				"name":      ingressName,
				"namespace": namespace,
			},
			"spec": map[string]any{
				"entryPoints": []any{"websecure"},
				"routes": []any{
					map[string]any{
						"match":    fmt.Sprintf("Host(`%s`)", fqdn),
						"kind":     "Rule",
						"services": []any{
							map[string]any{
								"name": serviceName,
								"port": port,
							},
						},
					},
				},
				"tls": map[string]any{
					"secretName": certNamespace + "/" + certSecretName,
				},
			},
		},
	}
}
