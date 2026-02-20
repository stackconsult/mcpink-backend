package statuschecker

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

func httpCheck(url string, timeout time.Duration) error {
	client := &http.Client{Timeout: timeout}
	resp, err := client.Get(url)
	if err != nil {
		return fmt.Errorf("http request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}
	return nil
}

func k8sPodCheck(ctx context.Context, k8s kubernetes.Interface, namespace, labelSelector string) error {
	pods, err := k8s.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{
		LabelSelector: labelSelector,
	})
	if err != nil {
		return fmt.Errorf("failed to list pods: %w", err)
	}

	if len(pods.Items) == 0 {
		return fmt.Errorf("no pods found with selector %q in namespace %q", labelSelector, namespace)
	}

	for _, pod := range pods.Items {
		if pod.Status.Phase != "Running" {
			continue
		}
		for _, cond := range pod.Status.Conditions {
			if cond.Type == "Ready" && cond.Status == "True" {
				return nil
			}
		}
	}
	return fmt.Errorf("no running+ready pods found with selector %q in namespace %q", labelSelector, namespace)
}

func dnsCheck(server, domain string) error {
	resolver := &net.Resolver{
		PreferGo: true,
		Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
			d := net.Dialer{Timeout: 5 * time.Second}
			return d.DialContext(ctx, "udp", server)
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	addrs, err := resolver.LookupHost(ctx, domain)
	if err != nil {
		return fmt.Errorf("dns lookup failed for %q via %s: %w", domain, server, err)
	}
	if len(addrs) == 0 {
		return fmt.Errorf("dns lookup returned no results for %q via %s", domain, server)
	}
	return nil
}

func k8sNodesCheck(ctx context.Context, k8s kubernetes.Interface) error {
	nodes, err := k8s.CoreV1().Nodes().List(ctx, metav1.ListOptions{
		Limit: 1,
	})
	if err != nil {
		return fmt.Errorf("failed to list nodes: %w", err)
	}
	if len(nodes.Items) == 0 {
		return fmt.Errorf("no nodes found in cluster")
	}
	return nil
}
