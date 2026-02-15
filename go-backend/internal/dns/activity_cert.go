package dns

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/augustdev/autoclip/internal/storage/pg/generated/delegatedzones"
	"go.temporal.io/sdk/temporal"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
)

var certGVR = schema.GroupVersionResource{
	Group:    "cert-manager.io",
	Version:  "v1",
	Resource: "certificates",
}

func wildcardCertName(zone string) string {
	return "wc-" + sanitizeDNSLabel(zone)
}

func wildcardSecretName(zone string) string {
	return wildcardCertName(zone) + "-tls"
}

func (a *Activities) ApplyWildcardCert(ctx context.Context, input ApplyWildcardCertInput) error {
	a.logger.Info("ApplyWildcardCert", "zone", input.Zone, "namespace", input.Namespace)

	certName := wildcardCertName(input.Zone)
	secretName := wildcardSecretName(input.Zone)

	cert := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "cert-manager.io/v1",
			"kind":       "Certificate",
			"metadata": map[string]any{
				"name":      certName,
				"namespace": input.Namespace,
			},
			"spec": map[string]any{
				"secretName": secretName,
				"issuerRef": map[string]any{
					"name": "letsencrypt-prod",
					"kind": "ClusterIssuer",
				},
				"dnsNames": []any{"*." + input.Zone, input.Zone},
			},
		},
	}

	data, err := json.Marshal(cert)
	if err != nil {
		return fmt.Errorf("marshal certificate: %w", err)
	}

	_, err = a.dynClient.Resource(certGVR).Namespace(input.Namespace).Patch(
		ctx,
		certName,
		types.ApplyPatchType,
		data,
		metav1.PatchOptions{FieldManager: "temporal-worker"},
	)
	if err != nil {
		return fmt.Errorf("apply wildcard certificate: %w", err)
	}

	return nil
}

func (a *Activities) WaitForCertReady(ctx context.Context, input WaitForCertReadyInput) error {
	a.logger.Info("WaitForCertReady", "zone", input.Zone, "namespace", input.Namespace)

	certName := wildcardCertName(input.Zone)

	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			recordHeartbeat(ctx, "polling wildcard cert status")

			cert, err := a.dynClient.Resource(certGVR).Namespace(input.Namespace).Get(ctx, certName, metav1.GetOptions{})
			if err != nil {
				return fmt.Errorf("get certificate: %w", err)
			}

			conditions, found := parseConditions(cert.Object)
			if !found {
				continue
			}

			for _, c := range conditions {
				if c.Type == "Ready" && c.Status == "True" {
					a.logger.Info("Wildcard certificate is ready", "name", certName)
					return nil
				}
				if c.Type == "Ready" && c.Status == "False" && isTerminalCertFailure(c.Reason) {
					return temporal.NewNonRetryableApplicationError(
						fmt.Sprintf("wildcard cert failed permanently: %s — %s", c.Reason, c.Message),
						"cert_terminal_failure",
						nil,
					)
				}
			}
		}
	}
}

func (a *Activities) UpdateZoneStatus(ctx context.Context, input UpdateZoneStatusInput) error {
	a.logger.Info("UpdateZoneStatus", "zoneID", input.ZoneID, "status", input.Status)

	switch input.Status {
	case "active":
		_, err := a.delegatedZonesQ.UpdateActivated(ctx, delegatedzones.UpdateActivatedParams{
			ID:                 input.ZoneID,
			WildcardCertSecret: &input.WildcardCertSecret,
		})
		return err
	default:
		_, err := a.delegatedZonesQ.UpdateStatus(ctx, delegatedzones.UpdateStatusParams{
			ID:     input.ZoneID,
			Status: input.Status,
		})
		return err
	}
}

func (a *Activities) DeleteCertificate(ctx context.Context, input DeleteCertificateInput) error {
	a.logger.Info("DeleteCertificate", "namespace", input.Namespace, "name", input.CertificateName)

	err := a.dynClient.Resource(certGVR).Namespace(input.Namespace).Delete(ctx, input.CertificateName, metav1.DeleteOptions{})
	if err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("delete certificate: %w", err)
	}

	secretName := input.CertificateName + "-tls"
	err = a.k8s.CoreV1().Secrets(input.Namespace).Delete(ctx, secretName, metav1.DeleteOptions{})
	if err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("delete certificate tls secret: %w", err)
	}

	return nil
}

// sanitizeDNSLabel converts a zone name to a valid K8s resource name fragment.
func sanitizeDNSLabel(zone string) string {
	result := make([]byte, 0, len(zone))
	for i := range zone {
		if zone[i] == '.' {
			result = append(result, '-')
		} else {
			result = append(result, zone[i])
		}
	}
	if len(result) > 63 {
		result = result[:63]
	}
	return string(result)
}

type certCondition struct {
	Type    string
	Status  string
	Reason  string
	Message string
}

func parseConditions(obj map[string]any) ([]certCondition, bool) {
	status, ok := obj["status"].(map[string]any)
	if !ok {
		return nil, false
	}
	rawConditions, ok := status["conditions"].([]any)
	if !ok {
		return nil, false
	}

	var out []certCondition
	for _, rc := range rawConditions {
		m, ok := rc.(map[string]any)
		if !ok {
			continue
		}
		out = append(out, certCondition{
			Type:    strVal(m, "type"),
			Status:  strVal(m, "status"),
			Reason:  strVal(m, "reason"),
			Message: strVal(m, "message"),
		})
	}
	return out, true
}

func strVal(m map[string]any, key string) string {
	v, _ := m[key].(string)
	return v
}

func isTerminalCertFailure(reason string) bool {
	switch reason {
	case "InvalidDomain", "CAA", "RateLimited", "Denied":
		return true
	}
	return false
}

