package k8sdeployments

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/augustdev/autoclip/internal/storage/pg/generated/customdomains"
	"go.temporal.io/sdk/temporal"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
)

var certGVR = schema.GroupVersionResource{
	Group:    "cert-manager.io",
	Version:  "v1",
	Resource: "certificates",
}

func (a *Activities) ApplyCustomDomainCertificate(ctx context.Context, input ApplyCustomDomainCertificateInput) error {
	a.logger.Info("ApplyCustomDomainCertificate",
		"namespace", input.Namespace,
		"serviceName", input.ServiceName,
		"domain", input.Domain)

	cert := buildCustomDomainCertificate(input.Namespace, input.ServiceName, input.Domain)
	data, err := json.Marshal(cert)
	if err != nil {
		return fmt.Errorf("marshal certificate: %w", err)
	}

	_, err = a.dynClient.Resource(certGVR).Namespace(input.Namespace).Patch(
		ctx,
		cert.GetName(),
		types.ApplyPatchType,
		data,
		metav1.PatchOptions{FieldManager: "temporal-worker"},
	)
	if err != nil {
		return fmt.Errorf("apply certificate: %w", err)
	}

	return nil
}

func (a *Activities) WaitForCertificateReady(ctx context.Context, input WaitForCertificateReadyInput) error {
	a.logger.Info("WaitForCertificateReady",
		"namespace", input.Namespace,
		"certificateName", input.CertificateName)

	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			recordHeartbeat(ctx, "polling certificate status")

			cert, err := a.dynClient.Resource(certGVR).Namespace(input.Namespace).Get(ctx, input.CertificateName, metav1.GetOptions{})
			if err != nil {
				return fmt.Errorf("get certificate: %w", err)
			}

			conditions, found, err := unstructuredConditions(cert.Object)
			if err != nil {
				return fmt.Errorf("parse certificate conditions: %w", err)
			}
			if !found {
				continue
			}

			for _, c := range conditions {
				if c.Type == "Ready" && c.Status == "True" {
					a.logger.Info("Certificate is ready", "name", input.CertificateName)
					return nil
				}
				if c.Type == "Ready" && c.Status == "False" {
					if isTerminalCertFailure(c.Reason) {
						return temporal.NewNonRetryableApplicationError(
							fmt.Sprintf("certificate failed permanently: %s — %s", c.Reason, c.Message),
							"cert_terminal_failure",
							nil,
						)
					}
				}
			}
		}
	}
}

func (a *Activities) ApplyCustomDomainIngress(ctx context.Context, input ApplyCustomDomainIngressInput) error {
	a.logger.Info("ApplyCustomDomainIngress",
		"namespace", input.Namespace,
		"serviceName", input.ServiceName,
		"domain", input.Domain)

	svc, err := a.k8s.CoreV1().Services(input.Namespace).Get(ctx, input.ServiceName, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("get service to resolve port: %w", err)
	}
	if len(svc.Spec.Ports) == 0 {
		return fmt.Errorf("service %s has no ports", input.ServiceName)
	}
	port := svc.Spec.Ports[0].Port

	ingress := buildCustomDomainIngress(input.Namespace, input.ServiceName, input.Domain, port)
	data, err := json.Marshal(ingress)
	if err != nil {
		return fmt.Errorf("marshal custom domain ingress: %w", err)
	}

	ingressName := input.ServiceName + "-cd"
	_, err = a.k8s.NetworkingV1().Ingresses(input.Namespace).Patch(ctx, ingressName,
		types.ApplyPatchType, data,
		metav1.PatchOptions{FieldManager: "temporal-worker"})
	if err != nil {
		return fmt.Errorf("apply custom domain ingress: %w", err)
	}

	return nil
}

func (a *Activities) DeleteCustomDomainIngress(ctx context.Context, input DeleteCustomDomainIngressInput) error {
	a.logger.Info("DeleteCustomDomainIngress",
		"namespace", input.Namespace,
		"serviceName", input.ServiceName)

	ingressName := input.ServiceName + "-cd"
	tlsSecretName := ingressName + "-tls"
	certName := ingressName

	err := a.k8s.NetworkingV1().Ingresses(input.Namespace).Delete(ctx, ingressName, metav1.DeleteOptions{})
	if err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("delete custom domain ingress: %w", err)
	}

	err = a.dynClient.Resource(certGVR).Namespace(input.Namespace).Delete(ctx, certName, metav1.DeleteOptions{})
	if err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("delete certificate: %w", err)
	}

	err = a.k8s.CoreV1().Secrets(input.Namespace).Delete(ctx, tlsSecretName, metav1.DeleteOptions{})
	if err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("delete custom domain tls secret: %w", err)
	}

	return nil
}

func (a *Activities) UpdateCustomDomainDBStatus(ctx context.Context, input UpdateCustomDomainStatusInput) error {
	a.logger.Info("UpdateCustomDomainDBStatus",
		"customDomainID", input.CustomDomainID,
		"status", input.Status)

	if input.Status == "active" {
		_, err := a.customDomainsQ.UpdateVerified(ctx, input.CustomDomainID)
		return err
	}

	_, err := a.customDomainsQ.UpdateStatus(ctx, customdomains.UpdateStatusParams{
		ID:     input.CustomDomainID,
		Status: input.Status,
	})
	return err
}

type condition struct {
	Type    string
	Status  string
	Reason  string
	Message string
}

func unstructuredConditions(obj map[string]any) ([]condition, bool, error) {
	status, ok := obj["status"].(map[string]any)
	if !ok {
		return nil, false, nil
	}
	rawConditions, ok := status["conditions"].([]any)
	if !ok {
		return nil, false, nil
	}

	var out []condition
	for _, rc := range rawConditions {
		m, ok := rc.(map[string]any)
		if !ok {
			continue
		}
		out = append(out, condition{
			Type:    strVal(m, "type"),
			Status:  strVal(m, "status"),
			Reason:  strVal(m, "reason"),
			Message: strVal(m, "message"),
		})
	}
	return out, true, nil
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
