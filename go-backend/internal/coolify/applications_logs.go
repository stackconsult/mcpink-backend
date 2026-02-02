package coolify

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"
)

type LogEntry struct {
	Timestamp string `json:"timestamp,omitempty"`
	Message   string `json:"message,omitempty"`
	Stream    string `json:"stream,omitempty"`
}

type runtimeLogsResponse struct {
	Logs string `json:"logs"`
}

type deploymentsResponse struct {
	Count       int          `json:"count"`
	Deployments []Deployment `json:"deployments"`
}

type Deployment struct {
	ID             int    `json:"id"`
	DeploymentUUID string `json:"deployment_uuid"`
	Status         string `json:"status"`
	Commit         string `json:"commit"`
	Logs           string `json:"logs"`
	CreatedAt      string `json:"created_at"`
	UpdatedAt      string `json:"updated_at"`
}

type deploymentLogEntry struct {
	Command   *string `json:"command"`
	Output    string  `json:"output"`
	Type      string  `json:"type"`
	Timestamp string  `json:"timestamp"`
	Hidden    bool    `json:"hidden"`
	Order     int     `json:"order"`
}

type GetLogsOptions struct {
	Lines int
}

func (s *ApplicationsService) GetRuntimeLogs(ctx context.Context, uuid string, lines int) ([]LogEntry, error) {
	if uuid == "" {
		return nil, fmt.Errorf("coolify: uuid is required")
	}

	query := url.Values{}
	if lines > 0 {
		query.Set("lines", strconv.Itoa(lines))
	}

	var resp runtimeLogsResponse
	if err := s.client.do(ctx, "GET", "/api/v1/applications/"+uuid+"/logs", query, nil, &resp); err != nil {
		return nil, fmt.Errorf("failed to get runtime logs: %w", err)
	}

	logLines := strings.Split(resp.Logs, "\n")
	entries := make([]LogEntry, 0, len(logLines))
	for _, line := range logLines {
		if line != "" {
			entries = append(entries, LogEntry{Message: line})
		}
	}
	return entries, nil
}

func (s *ApplicationsService) GetDeploymentLogs(ctx context.Context, uuid string) ([]LogEntry, error) {
	if uuid == "" {
		return nil, fmt.Errorf("coolify: uuid is required")
	}

	query := url.Values{}
	query.Set("skip", "0")
	query.Set("take", "1")

	var resp deploymentsResponse
	if err := s.client.do(ctx, "GET", "/api/v1/deployments/applications/"+uuid, query, nil, &resp); err != nil {
		return nil, fmt.Errorf("failed to get deployments: %w", err)
	}

	if len(resp.Deployments) == 0 {
		return []LogEntry{}, nil
	}

	var rawLogs []deploymentLogEntry
	if err := json.Unmarshal([]byte(resp.Deployments[0].Logs), &rawLogs); err != nil {
		return nil, fmt.Errorf("failed to parse deployment logs: %w", err)
	}

	// Filter out hidden entries and sort by order
	visible := make([]deploymentLogEntry, 0, len(rawLogs))
	for _, log := range rawLogs {
		if !log.Hidden && log.Output != "" {
			visible = append(visible, log)
		}
	}
	sort.Slice(visible, func(i, j int) bool {
		return visible[i].Order < visible[j].Order
	})

	entries := make([]LogEntry, 0, len(visible))
	for _, log := range visible {
		entries = append(entries, LogEntry{
			Timestamp: log.Timestamp,
			Message:   log.Output,
			Stream:    log.Type,
		})
	}
	return entries, nil
}
