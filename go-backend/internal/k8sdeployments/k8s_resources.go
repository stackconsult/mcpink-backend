package k8sdeployments

import (
	"strconv"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/ptr"
)

func buildNamespace(namespace, tenant, project string) *corev1.Namespace {
	return &corev1.Namespace{
		TypeMeta: metav1.TypeMeta{Kind: "Namespace", APIVersion: "v1"},
		ObjectMeta: metav1.ObjectMeta{
			Name: namespace,
			Labels: map[string]string{
				"dp.ml.ink/tenant":  tenant,
				"dp.ml.ink/project": project,
			},
		},
	}
}

func buildIngressNetworkPolicy(namespace string) *networkingv1.NetworkPolicy {
	return &networkingv1.NetworkPolicy{
		TypeMeta: metav1.TypeMeta{Kind: "NetworkPolicy", APIVersion: "networking.k8s.io/v1"},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "ingress-isolation",
			Namespace: namespace,
		},
		Spec: networkingv1.NetworkPolicySpec{
			PodSelector: metav1.LabelSelector{},
			PolicyTypes: []networkingv1.PolicyType{networkingv1.PolicyTypeIngress},
			Ingress: []networkingv1.NetworkPolicyIngressRule{
				{From: []networkingv1.NetworkPolicyPeer{{PodSelector: &metav1.LabelSelector{}}}},
				{From: []networkingv1.NetworkPolicyPeer{
					{
						NamespaceSelector: &metav1.LabelSelector{
							MatchLabels: map[string]string{"kubernetes.io/metadata.name": "dp-system"},
						},
						PodSelector: &metav1.LabelSelector{
							MatchLabels: map[string]string{
								"app.kubernetes.io/name":     "traefik",
								"app.kubernetes.io/instance": "traefik",
							},
						},
					},
				}},
				{From: []networkingv1.NetworkPolicyPeer{
					{
						NamespaceSelector: &metav1.LabelSelector{
							MatchLabels: map[string]string{"kubernetes.io/metadata.name": "dp-system"},
						},
						PodSelector: &metav1.LabelSelector{
							MatchLabels: map[string]string{"app": "traefik"},
						},
					},
				}},
			},
		},
	}
}

func buildEgressNetworkPolicy(namespace string) *networkingv1.NetworkPolicy {
	proto := corev1.ProtocolUDP
	protoTCP := corev1.ProtocolTCP
	port53 := intstr.FromInt32(53)

	return &networkingv1.NetworkPolicy{
		TypeMeta: metav1.TypeMeta{Kind: "NetworkPolicy", APIVersion: "networking.k8s.io/v1"},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "egress-isolation",
			Namespace: namespace,
		},
		Spec: networkingv1.NetworkPolicySpec{
			PodSelector: metav1.LabelSelector{},
			PolicyTypes: []networkingv1.PolicyType{networkingv1.PolicyTypeEgress},
			Egress: []networkingv1.NetworkPolicyEgressRule{
				{
					Ports: []networkingv1.NetworkPolicyPort{
						{Protocol: &proto, Port: &port53},
						{Protocol: &protoTCP, Port: &port53},
					},
				},
				{
					To: []networkingv1.NetworkPolicyPeer{
						{
							IPBlock: &networkingv1.IPBlock{
								CIDR: "0.0.0.0/0",
								Except: []string{
									"10.0.0.0/8",
									"172.16.0.0/12",
									"192.168.0.0/16",
									"169.254.169.254/32",
								},
							},
						},
					},
				},
			},
		},
	}
}

func buildResourceQuota(namespace string) *corev1.ResourceQuota {
	return &corev1.ResourceQuota{
		TypeMeta: metav1.TypeMeta{Kind: "ResourceQuota", APIVersion: "v1"},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "quota",
			Namespace: namespace,
		},
		Spec: corev1.ResourceQuotaSpec{
			Hard: corev1.ResourceList{
				corev1.ResourcePods:           resource.MustParse("20"),
				corev1.ResourceRequestsCPU:    resource.MustParse("4"),
				corev1.ResourceRequestsMemory: resource.MustParse("4Gi"),
				corev1.ResourceLimitsCPU:      resource.MustParse("8"),
				corev1.ResourceLimitsMemory:   resource.MustParse("8Gi"),
			},
		},
	}
}

func buildSecret(namespace, name string, envVars map[string]string) *corev1.Secret {
	return &corev1.Secret{
		TypeMeta: metav1.TypeMeta{Kind: "Secret", APIVersion: "v1"},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name + "-env",
			Namespace: namespace,
		},
		Type:       corev1.SecretTypeOpaque,
		StringData: envVars,
	}
}

func buildDeployment(namespace, name, imageRef, port string) *appsv1.Deployment {
	portInt, _ := strconv.ParseInt(port, 10, 32)
	if portInt == 0 {
		portInt = 3000
	}

	return &appsv1.Deployment{
		TypeMeta: metav1.TypeMeta{Kind: "Deployment", APIVersion: "apps/v1"},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels:    map[string]string{"app": name},
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: ptr.To(int32(1)),
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"app": name},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{"app": name},
				},
				Spec: corev1.PodSpec{
					AutomountServiceAccountToken: ptr.To(false),
					SecurityContext: &corev1.PodSecurityContext{},
					Containers: []corev1.Container{
						{
							Name:  "app",
							Image: imageRef,
							Ports: []corev1.ContainerPort{
								{ContainerPort: int32(portInt)},
							},
							EnvFrom: []corev1.EnvFromSource{
								{SecretRef: &corev1.SecretEnvSource{
									LocalObjectReference: corev1.LocalObjectReference{Name: name + "-env"},
								}},
							},
							Resources: corev1.ResourceRequirements{
								Requests: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("100m"),
									corev1.ResourceMemory: resource.MustParse("128Mi"),
								},
								Limits: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("500m"),
									corev1.ResourceMemory: resource.MustParse("512Mi"),
								},
							},
							SecurityContext: &corev1.SecurityContext{
								AllowPrivilegeEscalation: ptr.To(false),
								ReadOnlyRootFilesystem:   ptr.To(false),
								Capabilities: &corev1.Capabilities{
									Drop: []corev1.Capability{"ALL"},
								},
							},
						},
					},
				},
			},
		},
	}
}

func buildService(namespace, name, port string) *corev1.Service {
	portInt, _ := strconv.ParseInt(port, 10, 32)
	if portInt == 0 {
		portInt = 3000
	}

	return &corev1.Service{
		TypeMeta: metav1.TypeMeta{Kind: "Service", APIVersion: "v1"},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: corev1.ServiceSpec{
			Selector: map[string]string{"app": name},
			Ports: []corev1.ServicePort{
				{
					Port:       int32(portInt),
					TargetPort: intstr.FromInt32(int32(portInt)),
				},
			},
		},
	}
}

func buildIngress(namespace, name, host, port string) *networkingv1.Ingress {
	portInt, _ := strconv.ParseInt(port, 10, 32)
	if portInt == 0 {
		portInt = 3000
	}
	pathType := networkingv1.PathTypePrefix
	ingressClassName := "traefik"

	return &networkingv1.Ingress{
		TypeMeta: metav1.TypeMeta{Kind: "Ingress", APIVersion: "networking.k8s.io/v1"},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: networkingv1.IngressSpec{
			IngressClassName: &ingressClassName,
			Rules: []networkingv1.IngressRule{
				{
					Host: host,
					IngressRuleValue: networkingv1.IngressRuleValue{
						HTTP: &networkingv1.HTTPIngressRuleValue{
							Paths: []networkingv1.HTTPIngressPath{
								{
									Path:     "/",
									PathType: &pathType,
									Backend: networkingv1.IngressBackend{
										Service: &networkingv1.IngressServiceBackend{
											Name: name,
											Port: networkingv1.ServiceBackendPort{
												Number: int32(portInt),
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}
}
