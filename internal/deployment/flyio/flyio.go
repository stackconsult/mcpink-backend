package flyio

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/augustdev/autoclip/internal/deployment"
)

const apiURL = "https://api.fly.io/graphql"

type Client struct {
	token  string
	org    string
	client *http.Client
}

func NewClient(token, org string) *Client {
	return &Client{
		token:  token,
		org:    org,
		client: &http.Client{},
	}
}

func (c *Client) ListApps(ctx context.Context) ([]deployment.App, error) {
	query := `
		query($org: String!) {
			organization(slug: $org) {
				apps {
					nodes {
						id
						name
						status
						organization { slug }
						hostname
					}
				}
			}
		}
	`

	result, err := c.query(ctx, query, map[string]any{"org": c.org})
	if err != nil {
		return nil, err
	}

	data, ok := result["data"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("invalid response structure")
	}

	org, ok := data["organization"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("organization not found")
	}

	appsData, ok := org["apps"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("apps data not found")
	}

	nodes, ok := appsData["nodes"].([]any)
	if !ok {
		return nil, fmt.Errorf("apps nodes not found")
	}

	apps := make([]deployment.App, 0, len(nodes))
	for _, node := range nodes {
		nodeMap, ok := node.(map[string]any)
		if !ok {
			continue
		}

		app := deployment.App{
			ID:       getString(nodeMap, "id"),
			Name:     getString(nodeMap, "name"),
			Status:   getString(nodeMap, "status"),
			Hostname: getString(nodeMap, "hostname"),
		}

		if orgData, ok := nodeMap["organization"].(map[string]any); ok {
			app.Organization = getString(orgData, "slug")
		}

		apps = append(apps, app)
	}

	return apps, nil
}

func (c *Client) GetAppStatus(ctx context.Context, appName string) (*deployment.AppStatus, error) {
	query := `
		query($appName: String!) {
			app(name: $appName) {
				id
				name
				status
				deployed
				hostname
			}
		}
	`

	result, err := c.query(ctx, query, map[string]any{"appName": appName})
	if err != nil {
		return nil, err
	}

	data, ok := result["data"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("invalid response structure")
	}

	appData, ok := data["app"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("app not found")
	}

	return &deployment.AppStatus{
		ID:       getString(appData, "id"),
		Name:     getString(appData, "name"),
		Status:   getString(appData, "status"),
		Deployed: getBool(appData, "deployed"),
		Hostname: getString(appData, "hostname"),
	}, nil
}

func (c *Client) query(ctx context.Context, query string, variables map[string]any) (map[string]any, error) {
	reqBody := map[string]any{
		"query":     query,
		"variables": variables,
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", apiURL, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(body))
	}

	var result map[string]any
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	if errors, ok := result["errors"].([]any); ok && len(errors) > 0 {
		return nil, fmt.Errorf("GraphQL errors: %v", errors)
	}

	return result, nil
}

func getString(m map[string]any, key string) string {
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}

func getBool(m map[string]any, key string) bool {
	if v, ok := m[key].(bool); ok {
		return v
	}
	return false
}
