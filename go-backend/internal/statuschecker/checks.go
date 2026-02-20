package statuschecker

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/miekg/dns"
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
	m := new(dns.Msg)
	m.SetQuestion(dns.Fqdn(domain), dns.TypeSOA)
	m.RecursionDesired = false

	c := &dns.Client{Timeout: 5 * time.Second}
	_, _, err := c.Exchange(m, server)
	if err != nil {
		return fmt.Errorf("dns server %s unreachable: %w", server, err)
	}
	// Any response (including REFUSED) means the server is alive.
	// Only connection/timeout errors indicate a problem.
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
