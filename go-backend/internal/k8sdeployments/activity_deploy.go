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

	app, err := a.appsQ.GetAppByID(ctx, input.ServiceID)
	if err != nil {
		return nil, fmt.Errorf("get app: %w", err)
	}

	project, err := a.projectsQ.GetProjectByID(ctx, app.ProjectID)
	if err != nil {
		return nil, fmt.Errorf("get project: %w", err)
	}

	user, err := a.usersQ.GetUserByID(ctx, app.UserID)
	if err != nil {
		return nil, fmt.Errorf("get user: %w", err)
	}

	username := resolveUsername(user)
	if username == "" {
		return nil, fmt.Errorf("user %s has no gitea or github username", app.UserID)
	}

	namespace := NamespaceName(username, project.Ref)
	name := ServiceName(*app.Name)
	port := app.Port
	if port == "" {
		port = "3000"
	}

	// Parse env vars
	envVars := make(map[string]string)
	if len(app.EnvVars) > 0 {
		if err := json.Unmarshal(app.EnvVars, &envVars); err != nil {
			var envArr []struct {
				Key   string `json:"key"`
				Value string `json:"value"`
			}
			if err := json.Unmarshal(app.EnvVars, &envArr); err == nil {
				for _, e := range envArr {
					envVars[e.Key] = e.Value
				}
			}
		}
	}
	envVars["PORT"] = port

	// Ensure namespace
	if err := a.ensureNamespace(ctx, namespace, username, project.Ref); err != nil {
		return nil, fmt.Errorf("ensure namespace: %w", err)
	}

	// Apply Secret
	if err := a.applySecret(ctx, namespace, name, envVars); err != nil {
		return nil, fmt.Errorf("apply secret: %w", err)
	}

	// Apply Deployment
	if err := a.applyDeployment(ctx, namespace, name, input.ImageRef, port); err != nil {
		return nil, fmt.Errorf("apply deployment: %w", err)
	}

	// Apply Service
	if err := a.applyService(ctx, namespace, name, port); err != nil {
		return nil, fmt.Errorf("apply service: %w", err)
	}

	// Apply Ingress
	host := fmt.Sprintf("%s.%s", name, input.AppsDomain)
	if err := a.applyIngress(ctx, namespace, name, host, port); err != nil {
		return nil, fmt.Errorf("apply ingress: %w", err)
	}

	url := fmt.Sprintf("https://%s", host)
	a.logger.Info("Deploy completed",
		"serviceID", input.ServiceID,
		"namespace", namespace,
		"name", name,
		"url", url)

	return &DeployResult{
		Namespace:      namespace,
		DeploymentName: name,
		URL:            url,
	}, nil
}

func (a *Activities) ensureNamespace(ctx context.Context, namespace, tenant, project string) error {
	ns := buildNamespace(namespace, tenant, project)
	nsData, _ := json.Marshal(ns)
	_, err := a.k8s.CoreV1().Namespaces().Patch(ctx, namespace,
		types.ApplyPatchType, nsData,
		metav1.PatchOptions{FieldManager: "temporal-worker"})
	if err != nil {
		return fmt.Errorf("apply namespace: %w", err)
	}

	// Ingress network policy
	ingressNP := buildIngressNetworkPolicy(namespace)
	ingressData, _ := json.Marshal(ingressNP)
	_, err = a.k8s.NetworkingV1().NetworkPolicies(namespace).Patch(ctx, "ingress-isolation",
		types.ApplyPatchType, ingressData,
		metav1.PatchOptions{FieldManager: "temporal-worker"})
	if err != nil {
		return fmt.Errorf("apply ingress network policy: %w", err)
	}

	// Egress network policy
	egressNP := buildEgressNetworkPolicy(namespace)
	egressData, _ := json.Marshal(egressNP)
	_, err = a.k8s.NetworkingV1().NetworkPolicies(namespace).Patch(ctx, "egress-isolation",
		types.ApplyPatchType, egressData,
		metav1.PatchOptions{FieldManager: "temporal-worker"})
	if err != nil {
		return fmt.Errorf("apply egress network policy: %w", err)
	}

	// Resource quota
	quota := buildResourceQuota(namespace)
	quotaData, _ := json.Marshal(quota)
	_, err = a.k8s.CoreV1().ResourceQuotas(namespace).Patch(ctx, "quota",
		types.ApplyPatchType, quotaData,
		metav1.PatchOptions{FieldManager: "temporal-worker"})
	if err != nil {
		return fmt.Errorf("apply resource quota: %w", err)
	}

	return nil
}

func (a *Activities) applySecret(ctx context.Context, namespace, name string, envVars map[string]string) error {
	secret := buildSecret(namespace, name, envVars)
	data, _ := json.Marshal(secret)
	_, err := a.k8s.CoreV1().Secrets(namespace).Patch(ctx, name+"-env",
		types.ApplyPatchType, data,
		metav1.PatchOptions{FieldManager: "temporal-worker"})
	return err
}

func (a *Activities) applyDeployment(ctx context.Context, namespace, name, imageRef, port string) error {
	deployment := buildDeployment(namespace, name, imageRef, port)
	data, _ := json.Marshal(deployment)
	_, err := a.k8s.AppsV1().Deployments(namespace).Patch(ctx, name,
		types.ApplyPatchType, data,
		metav1.PatchOptions{FieldManager: "temporal-worker"})
	return err
}

func (a *Activities) applyService(ctx context.Context, namespace, name, port string) error {
	svc := buildService(namespace, name, port)
	data, _ := json.Marshal(svc)
	_, err := a.k8s.CoreV1().Services(namespace).Patch(ctx, name,
		types.ApplyPatchType, data,
		metav1.PatchOptions{FieldManager: "temporal-worker"})
	return err
}

func (a *Activities) applyIngress(ctx context.Context, namespace, name, host, port string) error {
	ingress := buildIngress(namespace, name, host, port)
	data, _ := json.Marshal(ingress)
	_, err := a.k8s.NetworkingV1().Ingresses(namespace).Patch(ctx, name,
		types.ApplyPatchType, data,
		metav1.PatchOptions{FieldManager: "temporal-worker"})
	return err
}
