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

var middlewareGVR = schema.GroupVersionResource{
	Group:    "traefik.io",
	Version:  "v1alpha1",
	Resource: "middlewares",
}

const certLoaderNamespace = "dp-system"

func certLoaderName(zone string) string {
	return "wc-" + sanitizeDNSLabel(zone) + "-tls-loader"
}

// ApplyCertLoader creates an IngressRoute in dp-system that loads the zone's
// wildcard TLS cert into Traefik's global cert pool. User namespace
// IngressRoutes with tls:{} then get the cert via SNI selection.
func (a *Activities) ApplyCertLoader(ctx context.Context, input ApplyCertLoaderInput) error {
	a.logger.Info("ApplyCertLoader", "zone", input.Zone, "secret", input.SecretName)

	ir := buildCertLoaderIngressRoute(input.Zone, input.SecretName)
	data, err := json.Marshal(ir)
	if err != nil {
		return fmt.Errorf("marshal cert-loader ingressroute: %w", err)
	}

	name := certLoaderName(input.Zone)
	_, err = a.dynClient.Resource(ingressRouteGVR).Namespace(certLoaderNamespace).Patch(
		ctx, name, types.ApplyPatchType, data,
		metav1.PatchOptions{FieldManager: "temporal-worker"},
	)
	if err != nil {
		return fmt.Errorf("apply cert-loader ingressroute: %w", err)
	}
	return nil
}

func (a *Activities) DeleteCertLoader(ctx context.Context, input DeleteCertLoaderInput) error {
	a.logger.Info("DeleteCertLoader", "zone", input.Zone)

	name := certLoaderName(input.Zone)
	err := a.dynClient.Resource(ingressRouteGVR).Namespace(certLoaderNamespace).Delete(ctx, name, metav1.DeleteOptions{})
	if err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("delete cert-loader ingressroute: %w", err)
	}
	return nil
}

func (a *Activities) EnsureRedirectMiddleware(ctx context.Context, input EnsureRedirectMiddlewareInput) error {
	a.logger.Info("EnsureRedirectMiddleware", "namespace", input.Namespace)

	mw := buildRedirectMiddleware(input.Namespace)
	data, err := json.Marshal(mw)
	if err != nil {
		return fmt.Errorf("marshal redirect middleware: %w", err)
	}

	_, err = a.dynClient.Resource(middlewareGVR).Namespace(input.Namespace).Patch(
		ctx, "redirect-https", types.ApplyPatchType, data,
		metav1.PatchOptions{FieldManager: "temporal-worker"},
	)
	if err != nil {
		return fmt.Errorf("apply redirect middleware: %w", err)
	}
	return nil
}

func (a *Activities) ApplySubdomainIngress(ctx context.Context, input ApplySubdomainIngressInput) error {
	a.logger.Info("ApplySubdomainIngress",
		"namespace", input.Namespace,
		"serviceName", input.ServiceName,
		"fqdn", input.FQDN)

	ingressRoute := buildSubdomainIngressRoute(
		input.Namespace,
		input.ServiceName,
		input.FQDN,
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

// buildCertLoaderIngressRoute creates an IngressRoute in dp-system that loads
// the zone wildcard cert into Traefik's global TLS cert pool via its tls.secretName.
// The route uses a dummy host that never matches real traffic.
func buildCertLoaderIngressRoute(zone, secretName string) *unstructured.Unstructured {
	name := certLoaderName(zone)
	return &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "traefik.io/v1alpha1",
			"kind":       "IngressRoute",
			"metadata": map[string]any{
				"name":      name,
				"namespace": certLoaderNamespace,
			},
			"spec": map[string]any{
				"entryPoints": []any{"websecure"},
				"routes": []any{
					map[string]any{
						"match":    fmt.Sprintf("Host(`_certload.%s`)", zone),
						"kind":     "Rule",
						"priority": 1,
						"services": []any{
							map[string]any{
								"name": "traefik",
								"port": 443,
							},
						},
					},
				},
				"tls": map[string]any{
					"secretName": secretName,
				},
			},
		},
	}
}

func buildRedirectMiddleware(namespace string) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "traefik.io/v1alpha1",
			"kind":       "Middleware",
			"metadata": map[string]any{
				"name":      "redirect-https",
				"namespace": namespace,
			},
			"spec": map[string]any{
				"redirectScheme": map[string]any{
					"scheme":    "https",
					"permanent": true,
				},
			},
		},
	}
}

func buildSubdomainIngressRoute(namespace, serviceName, fqdn string, port int32) *unstructured.Unstructured {
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
				"entryPoints": []any{"web", "websecure"},
				"routes": []any{
					map[string]any{
						"match": fmt.Sprintf("Host(`%s`)", fqdn),
						"kind":  "Rule",
						"middlewares": []any{
							map[string]any{
								"name": "redirect-https",
							},
						},
						"services": []any{
							map[string]any{
								"name": serviceName,
								"port": port,
							},
						},
					},
				},
				"tls": map[string]any{},
			},
		},
	}
}
