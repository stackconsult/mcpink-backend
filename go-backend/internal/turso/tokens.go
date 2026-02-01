package turso

import (
	"context"
	"encoding/json"
	"fmt"
)

func (c *Client) CreateAuthToken(ctx context.Context, dbName string, req *CreateTokenRequest) (string, error) {
	path := fmt.Sprintf("/organizations/%s/databases/%s/auth/tokens", c.config.OrgSlug, dbName)

	if req == nil {
		req = &CreateTokenRequest{Authorization: "full-access"}
	}
	if req.Authorization == "" {
		req.Authorization = "full-access"
	}

	c.logger.Info("creating turso auth token", "database", dbName, "authorization", req.Authorization)

	respBody, err := c.doRequest(ctx, "POST", path, req)
	if err != nil {
		return "", fmt.Errorf("failed to create auth token: %w", err)
	}

	var resp CreateTokenResponse
	if err := json.Unmarshal(respBody, &resp); err != nil {
		return "", fmt.Errorf("failed to unmarshal response: %w", err)
	}

	c.logger.Info("turso auth token created", "database", dbName)

	return resp.JWT, nil
}

func (c *Client) CreateReadOnlyToken(ctx context.Context, dbName string, expiration string) (string, error) {
	return c.CreateAuthToken(ctx, dbName, &CreateTokenRequest{
		Authorization: "read-only",
		Expiration:    expiration,
	})
}
