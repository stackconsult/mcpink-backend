package k8sdeployments

import (
	"fmt"
	"strconv"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/ptr"
)

func parsePort(port string) int32 {
	p, _ := strconv.ParseInt(port, 10, 32)
	if p == 0 {
		p = 3000
	}
	return int32(p)
}

var allowedMemory = map[string]bool{
	"128Mi": true, "256Mi": true, "512Mi": true,
	"1024Mi": true, "2048Mi": true, "4096Mi": true,
}

var allowedVCPUs = map[string]bool{
	"0.5": true, "1": true, "2": true, "4": true,
}

func validateResourceLimits(memory, vcpus string) error {
	if !allowedMemory[memory] {
		return fmt.Errorf("invalid memory limit %q: must be one of 128Mi, 256Mi, 512Mi, 1024Mi, 2048Mi, 4096Mi", memory)
	}
	if !allowedVCPUs[vcpus] {
		return fmt.Errorf("invalid vcpus limit %q: must be one of 0.5, 1, 2, 4", vcpus)
	}
	return nil
}

func buildNamespace(namespace, tenant, project string) *corev1.Namespace {
	return &corev1.Namespace{
		TypeMeta: metav1.TypeMeta{Kind: "Namespace", APIVersion: "v1"},
		ObjectMeta: metav1.ObjectMeta{
			Name: namespace,
			Labels: map[string]string{
				"dp.ml.ink/tenant":                    tenant,
				"dp.ml.ink/project":                   project,
				"pod-security.kubernetes.io/enforce":   "baseline",
				"pod-security.kubernetes.io/warn":      "baseline",
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

func buildDeployment(namespace, name, imageRef string, port int32, memory, vcpus string) *appsv1.Deployment {
	memLimit := resource.MustParse(memory)
	cpuLimit := resource.MustParse(vcpus)

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
					RuntimeClassName:             ptr.To("gvisor"),
					AutomountServiceAccountToken: ptr.To(false),
					SecurityContext:              &corev1.PodSecurityContext{},
					Containers: []corev1.Container{
						{
							Name:  name,
							Image: imageRef,
							Ports: []corev1.ContainerPort{
								{ContainerPort: port},
							},
							EnvFrom: []corev1.EnvFromSource{
								{SecretRef: &corev1.SecretEnvSource{
									LocalObjectReference: corev1.LocalObjectReference{Name: name + "-env"},
								}},
							},
							Resources: corev1.ResourceRequirements{
								Requests: corev1.ResourceList{
									corev1.ResourceCPU:    cpuLimit.DeepCopy(),
									corev1.ResourceMemory: memLimit.DeepCopy(),
								},
								Limits: corev1.ResourceList{
									corev1.ResourceCPU:    cpuLimit,
									corev1.ResourceMemory: memLimit,
								},
							},
							// gVisor is the security boundary — caps only affect
							// the emulated kernel. allowPrivilegeEscalation=false
							// sets no_new_privs (free, doesn't break root images).
							SecurityContext: &corev1.SecurityContext{
								AllowPrivilegeEscalation: ptr.To(false),
								ReadOnlyRootFilesystem:   ptr.To(false),
							},
							ReadinessProbe: &corev1.Probe{
								ProbeHandler: corev1.ProbeHandler{
									TCPSocket: &corev1.TCPSocketAction{
										Port: intstr.FromInt32(port),
									},
								},
								InitialDelaySeconds: 1,
								PeriodSeconds:       2,
								TimeoutSeconds:      3,
								FailureThreshold:    3,
							},
						},
					},
				},
			},
		},
	}
}

func buildService(namespace, name string, port int32) *corev1.Service {
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
					Port:       port,
					TargetPort: intstr.FromInt32(port),
				},
			},
		},
	}
}

func buildIngress(namespace, name, host string, port int32) *networkingv1.Ingress {
	pathType := networkingv1.PathTypePrefix
	ingressClassName := "traefik"

	return &networkingv1.Ingress{
		TypeMeta: metav1.TypeMeta{Kind: "Ingress", APIVersion: "networking.k8s.io/v1"},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Annotations: map[string]string{
				"traefik.ingress.kubernetes.io/router.middlewares": "dp-system-redirect-to-https@kubernetescrd",
			},
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
												Number: port,
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

func buildCustomDomainIngress(namespace, serviceName, customDomain string, port int32) *networkingv1.Ingress {
	pathType := networkingv1.PathTypePrefix
	ingressClassName := "traefik"
	ingressName := serviceName + "-cd"

	return &networkingv1.Ingress{
		TypeMeta: metav1.TypeMeta{Kind: "Ingress", APIVersion: "networking.k8s.io/v1"},
		ObjectMeta: metav1.ObjectMeta{
			Name:      ingressName,
			Namespace: namespace,
			Annotations: map[string]string{
				"cert-manager.io/cluster-issuer":                  "letsencrypt-prod",
				"traefik.ingress.kubernetes.io/router.middlewares": "dp-system-redirect-to-https@kubernetescrd",
			},
		},
		Spec: networkingv1.IngressSpec{
			IngressClassName: &ingressClassName,
			TLS: []networkingv1.IngressTLS{
				{
					Hosts:      []string{customDomain},
					SecretName: ingressName + "-tls",
				},
			},
			Rules: []networkingv1.IngressRule{
				{
					Host: customDomain,
					IngressRuleValue: networkingv1.IngressRuleValue{
						HTTP: &networkingv1.HTTPIngressRuleValue{
							Paths: []networkingv1.HTTPIngressPath{
								{
									Path:     "/",
									PathType: &pathType,
									Backend: networkingv1.IngressBackend{
										Service: &networkingv1.IngressServiceBackend{
											Name: serviceName,
											Port: networkingv1.ServiceBackendPort{
												Number: port,
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
