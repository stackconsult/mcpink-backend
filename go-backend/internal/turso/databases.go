package turso

import (
	"context"
	"encoding/json"
	"fmt"
)

type Group struct {
	Name      string   `json:"name"`
	Locations []string `json:"locations"`
	Primary   string   `json:"primary"`
}

type ListGroupsResponse struct {
	Groups []Group `json:"groups"`
}

type CreateGroupRequest struct {
	Name     string `json:"name"`
	Location string `json:"location"`
}

func (c *Client) ListGroups(ctx context.Context) ([]Group, error) {
	path := fmt.Sprintf("/organizations/%s/groups", c.config.OrgSlug)

	respBody, err := c.doRequest(ctx, "GET", path, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to list groups: %w", err)
	}

	var resp ListGroupsResponse
	if err := json.Unmarshal(respBody, &resp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return resp.Groups, nil
}

func (c *Client) CreateGroup(ctx context.Context, req *CreateGroupRequest) (*Group, error) {
	path := fmt.Sprintf("/organizations/%s/groups", c.config.OrgSlug)

	c.logger.Info("creating turso group", "name", req.Name, "location", req.Location)

	respBody, err := c.doRequest(ctx, "POST", path, req)
	if err != nil {
		return nil, fmt.Errorf("failed to create group: %w", err)
	}

	var resp struct {
		Group Group `json:"group"`
	}
	if err := json.Unmarshal(respBody, &resp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	c.logger.Info("turso group created", "name", resp.Group.Name, "primary", resp.Group.Primary)

	return &resp.Group, nil
}

func (c *Client) CreateDatabase(ctx context.Context, req *CreateDatabaseRequest) (*Database, error) {
	path := fmt.Sprintf("/organizations/%s/databases", c.config.OrgSlug)

	c.logger.Info("creating turso database", "name", req.Name, "group", req.Group, "size_limit", req.SizeLimit)

	respBody, err := c.doRequest(ctx, "POST", path, req)
	if err != nil {
		return nil, fmt.Errorf("failed to create database: %w", err)
	}

	var resp CreateDatabaseResponse
	if err := json.Unmarshal(respBody, &resp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	c.logger.Info("turso database created", "db_id", resp.Database.DbID, "hostname", resp.Database.Hostname)

	return &resp.Database, nil
}

func (c *Client) GetDatabase(ctx context.Context, dbName string) (*Database, error) {
	path := fmt.Sprintf("/organizations/%s/databases/%s", c.config.OrgSlug, dbName)

	respBody, err := c.doRequest(ctx, "GET", path, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get database: %w", err)
	}

	var resp struct {
		Database Database `json:"database"`
	}
	if err := json.Unmarshal(respBody, &resp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return &resp.Database, nil
}

func (c *Client) DeleteDatabase(ctx context.Context, dbName string) error {
	path := fmt.Sprintf("/organizations/%s/databases/%s", c.config.OrgSlug, dbName)

	c.logger.Info("deleting turso database", "name", dbName)

	_, err := c.doRequest(ctx, "DELETE", path, nil)
	if err != nil {
		return fmt.Errorf("failed to delete database: %w", err)
	}

	c.logger.Info("turso database deleted", "name", dbName)

	return nil
}
