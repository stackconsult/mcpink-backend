package k8sdeployments

import (
	"context"
	"encoding/json"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

func (a *Activities) Deploy(ctx context.Context, input DeployInput) (*DeployResult, error) {
	a.logger.Info("Deploy activity started",
		"serviceID", input.ServiceID,
		"imageRef", input.ImageRef,
		"commitSHA", input.CommitSHA)

	id, err := a.resolveServiceIdentity(ctx, input.ServiceID)
	if err != nil {
		return nil, err
	}

	bc := parseBuildConfig(id.App.BuildConfig)
	// Prefer port resolved during build phase (carries EXPOSE detection).
	// Fall back to DB value for in-flight workflows that predate the Port field.
	appPort := input.Port
	if appPort == "" {
		appPort = id.App.Port
	}
	port := effectiveAppPort(id.App.BuildPack, appPort, bc.PublishDirectory)
	portInt := parsePort(port)

	envVars := parseEnvVars(id.App.EnvVars)
	envVars["PORT"] = port

	// Ensure namespace
	if err := a.ensureNamespace(ctx, id.Namespace, id.Tenant, id.ProjectRef); err != nil {
		return nil, fmt.Errorf("ensure namespace: %w", err)
	}

	// Apply Secret
	if err := a.applySecret(ctx, id.Namespace, id.Name, envVars); err != nil {
		return nil, fmt.Errorf("apply secret: %w", err)
	}

	// Apply Deployment
	if err := a.applyDeployment(ctx, id.Namespace, id.Name, input.ImageRef, portInt, id.App.Memory, id.App.Cpu); err != nil {
		return nil, fmt.Errorf("apply deployment: %w", err)
	}

	// Apply Service
	if err := a.applyService(ctx, id.Namespace, id.Name, portInt); err != nil {
		return nil, fmt.Errorf("apply service: %w", err)
	}

	// Apply Ingress
	host := fmt.Sprintf("%s.%s", id.Name, input.AppsDomain)
	if err := a.applyIngress(ctx, id.Namespace, id.Name, host, portInt); err != nil {
		return nil, fmt.Errorf("apply ingress: %w", err)
	}

	url := fmt.Sprintf("https://%s", host)
	a.logger.Info("Deploy completed",
		"serviceID", input.ServiceID,
		"namespace", id.Namespace,
		"name", id.Name,
		"url", url)

	return &DeployResult{
		Namespace:      id.Namespace,
		DeploymentName: id.Name,
		URL:            url,
	}, nil
}

func (a *Activities) ensureNamespace(ctx context.Context, namespace, tenant, project string) error {
	ns := buildNamespace(namespace, tenant, project)
	nsData, err := json.Marshal(ns)
	if err != nil {
		return fmt.Errorf("marshal namespace: %w", err)
	}
	_, err = a.k8s.CoreV1().Namespaces().Patch(ctx, namespace,
		types.ApplyPatchType, nsData,
		metav1.PatchOptions{FieldManager: "temporal-worker"})
	if err != nil {
		return fmt.Errorf("apply namespace: %w", err)
	}

	// Ingress network policy
	ingressNP := buildIngressNetworkPolicy(namespace)
	ingressData, err := json.Marshal(ingressNP)
	if err != nil {
		return fmt.Errorf("marshal ingress network policy: %w", err)
	}
	_, err = a.k8s.NetworkingV1().NetworkPolicies(namespace).Patch(ctx, "ingress-isolation",
		types.ApplyPatchType, ingressData,
		metav1.PatchOptions{FieldManager: "temporal-worker"})
	if err != nil {
		return fmt.Errorf("apply ingress network policy: %w", err)
	}

	// Egress network policy
	egressNP := buildEgressNetworkPolicy(namespace)
	egressData, err := json.Marshal(egressNP)
	if err != nil {
		return fmt.Errorf("marshal egress network policy: %w", err)
	}
	_, err = a.k8s.NetworkingV1().NetworkPolicies(namespace).Patch(ctx, "egress-isolation",
		types.ApplyPatchType, egressData,
		metav1.PatchOptions{FieldManager: "temporal-worker"})
	if err != nil {
		return fmt.Errorf("apply egress network policy: %w", err)
	}

	return nil
}

func (a *Activities) applySecret(ctx context.Context, namespace, name string, envVars map[string]string) error {
	secret := buildSecret(namespace, name, envVars)
	data, err := json.Marshal(secret)
	if err != nil {
		return fmt.Errorf("marshal secret: %w", err)
	}
	_, err = a.k8s.CoreV1().Secrets(namespace).Patch(ctx, name+"-env",
		types.ApplyPatchType, data,
		metav1.PatchOptions{FieldManager: "temporal-worker"})
	return err
}

func (a *Activities) applyDeployment(ctx context.Context, namespace, name, imageRef string, port int32, memory, cpu string) error {
	if err := validateResourceLimits(memory, cpu); err != nil {
		return err
	}
	deployment := buildDeployment(namespace, name, imageRef, port, memory, cpu)
	data, err := json.Marshal(deployment)
	if err != nil {
		return fmt.Errorf("marshal deployment: %w", err)
	}
	_, err = a.k8s.AppsV1().Deployments(namespace).Patch(ctx, name,
		types.ApplyPatchType, data,
		metav1.PatchOptions{FieldManager: "temporal-worker"})
	return err
}

func (a *Activities) applyService(ctx context.Context, namespace, name string, port int32) error {
	svc := buildService(namespace, name, port)
	data, err := json.Marshal(svc)
	if err != nil {
		return fmt.Errorf("marshal service: %w", err)
	}
	_, err = a.k8s.CoreV1().Services(namespace).Patch(ctx, name,
		types.ApplyPatchType, data,
		metav1.PatchOptions{FieldManager: "temporal-worker"})
	return err
}

func (a *Activities) applyIngress(ctx context.Context, namespace, name, host string, port int32) error {
	ingress := buildIngress(namespace, name, host, port)
	data, err := json.Marshal(ingress)
	if err != nil {
		return fmt.Errorf("marshal ingress: %w", err)
	}
	_, err = a.k8s.NetworkingV1().Ingresses(namespace).Patch(ctx, name,
		types.ApplyPatchType, data,
		metav1.PatchOptions{FieldManager: "temporal-worker"})
	return err
}
