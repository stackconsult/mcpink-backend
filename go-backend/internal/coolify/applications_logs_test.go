package coolify

import (
	"context"
	"os"
	"testing"
)

func TestGetRuntimeLogs_Integration(t *testing.T) {
	baseURL := os.Getenv("COOLIFY_BASE_URL")
	apiKey := os.Getenv("COOLIFY_API_KEY")
	appUUID := os.Getenv("COOLIFY_TEST_APP_UUID")

	if baseURL == "" || apiKey == "" || appUUID == "" {
		t.Skip("COOLIFY_BASE_URL, COOLIFY_API_KEY, and COOLIFY_TEST_APP_UUID must be set")
	}

	client, err := NewClient(Config{
		BaseURL: baseURL,
		Token:   apiKey,
	})
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}

	logs, err := client.Applications.GetRuntimeLogs(context.Background(), appUUID, 10)
	if err != nil {
		t.Fatalf("GetRuntimeLogs failed: %v", err)
	}

	t.Logf("Got %d runtime log entries", len(logs))
	for i, entry := range logs {
		t.Logf("  [%d] %s %s: %s", i, entry.Timestamp, entry.Stream, entry.Message)
	}
}

func TestGetDeploymentLogs_Integration(t *testing.T) {
	baseURL := os.Getenv("COOLIFY_BASE_URL")
	apiKey := os.Getenv("COOLIFY_API_KEY")
	appUUID := os.Getenv("COOLIFY_TEST_APP_UUID")

	if baseURL == "" || apiKey == "" || appUUID == "" {
		t.Skip("COOLIFY_BASE_URL, COOLIFY_API_KEY, and COOLIFY_TEST_APP_UUID must be set")
	}

	client, err := NewClient(Config{
		BaseURL: baseURL,
		Token:   apiKey,
	})
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}

	logs, err := client.Applications.GetDeploymentLogs(context.Background(), appUUID)
	if err != nil {
		t.Fatalf("GetDeploymentLogs failed: %v", err)
	}

	t.Logf("Got %d deployment log entries", len(logs))
	for i, entry := range logs {
		if i < 20 {
			t.Logf("  [%d] %s %s: %.100s", i, entry.Timestamp, entry.Stream, entry.Message)
		}
	}
}
