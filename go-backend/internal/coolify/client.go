package coolify

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

type Config struct {
	BaseURL         string
	Token           string
	Timeout         time.Duration
	ProjectUUID     string
	ServerUUIDs     []string
	EnvironmentName string
	GitHubAppUUID   string
}

type Client struct {
	config     Config
	httpClient *http.Client
	baseURL    *url.URL

	Applications *ApplicationsService
	Sources      *SourcesService
}

func NewClient(config Config) (*Client, error) {
	if config.BaseURL == "" {
		return nil, fmt.Errorf("coolify: BaseURL is required")
	}
	if config.Token == "" {
		return nil, fmt.Errorf("coolify: Token is required")
	}

	baseURL, err := url.Parse(config.BaseURL)
	if err != nil {
		return nil, fmt.Errorf("coolify: invalid BaseURL: %w", err)
	}

	timeout := config.Timeout
	if timeout == 0 {
		timeout = 30 * time.Second
	}

	c := &Client{
		config: config,
		httpClient: &http.Client{
			Timeout: timeout,
		},
		baseURL: baseURL,
	}

	c.Applications = &ApplicationsService{client: c}
	c.Sources = &SourcesService{client: c}

	return c, nil
}

type Error struct {
	StatusCode int
	Message    string
	Body       string
}

func (c *Client) Config() Config {
	return c.config
}

func (c *Client) GetMuscleServer() string {
	// TODO: implement server selection logic
	return c.config.ServerUUIDs[0]
}

func (e *Error) Error() string {
	if e.Message != "" {
		return fmt.Sprintf("coolify: %s (status %d)", e.Message, e.StatusCode)
	}
	return fmt.Sprintf("coolify: request failed with status %d: %s", e.StatusCode, e.Body)
}

func (c *Client) request(ctx context.Context, method, path string, query url.Values, body any) (*http.Response, error) {
	u := *c.baseURL
	u.Path = path
	if query != nil {
		u.RawQuery = query.Encode()
	}

	var bodyReader io.Reader
	if body != nil {
		jsonBody, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("coolify: failed to marshal request body: %w", err)
		}
		bodyReader = bytes.NewReader(jsonBody)
	}

	req, err := http.NewRequestWithContext(ctx, method, u.String(), bodyReader)
	if err != nil {
		return nil, fmt.Errorf("coolify: failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.config.Token)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("coolify: request failed: %w", err)
	}

	return resp, nil
}

func (c *Client) do(ctx context.Context, method, path string, query url.Values, body, result any) error {
	resp, err := c.request(ctx, method, path, query, body)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("coolify: failed to read response body: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		apiErr := &Error{
			StatusCode: resp.StatusCode,
			Body:       string(respBody),
		}

		var errResp struct {
			Message string `json:"message"`
			Error   string `json:"error"`
		}
		if json.Unmarshal(respBody, &errResp) == nil {
			if errResp.Message != "" {
				apiErr.Message = errResp.Message
			} else if errResp.Error != "" {
				apiErr.Message = errResp.Error
			}
		}

		return apiErr
	}

	if result != nil && len(respBody) > 0 {
		if err := json.Unmarshal(respBody, result); err != nil {
			return fmt.Errorf("coolify: failed to decode response: %w", err)
		}
	}

	return nil
}
