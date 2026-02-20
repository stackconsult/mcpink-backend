package statuschecker

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

const (
	StatusOperational = "OPERATIONAL"
	StatusMajorOutage = "MAJOROUTAGE"

	instatusBaseURL = "https://api.instatus.com/v1"
)

type InstatusClient struct {
	pageID     string
	apiKey     string
	httpClient *http.Client
}

func NewInstatusClient(pageID, apiKey string) *InstatusClient {
	return &InstatusClient{
		pageID: pageID,
		apiKey: apiKey,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

type componentUpdateRequest struct {
	Status string `json:"status"`
}

func (c *InstatusClient) UpdateComponentStatus(ctx context.Context, componentID, status string) error {
	if c.apiKey == "" || c.pageID == "" {
		return nil
	}

	url := fmt.Sprintf("%s/%s/components/%s", instatusBaseURL, c.pageID, componentID)

	body, err := json.Marshal(componentUpdateRequest{Status: status})
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPut, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("instatus api request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("instatus api returned status %d for component %s", resp.StatusCode, componentID)
	}
	return nil
}
