package mcp

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/augustdev/autoclip/internal/deployment/flyio"
)

type Server struct {
	flyioClient *flyio.Client
}

type Tool struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	InputSchema map[string]interface{} `json:"inputSchema"`
}

type ToolResult struct {
	Content []ContentBlock `json:"content"`
	IsError bool           `json:"isError,omitempty"`
}

type ContentBlock struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

func NewServer(flyioClient *flyio.Client) *Server {
	return &Server{
		flyioClient: flyioClient,
	}
}

func (s *Server) ListTools() []Tool {
	return []Tool{
		{
			Name:        "flyio_list_apps",
			Description: "List all Fly.io applications in the user's organization",
			InputSchema: map[string]interface{}{
				"type":       "object",
				"properties": map[string]interface{}{},
			},
		},
		{
			Name:        "flyio_app_status",
			Description: "Get the status and details of a specific Fly.io application",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"app_name": map[string]interface{}{
						"type":        "string",
						"description": "Name of the Fly.io application",
					},
				},
				"required": []string{"app_name"},
			},
		},
	}
}

func (s *Server) CallTool(ctx context.Context, name string, arguments map[string]interface{}) (*ToolResult, error) {
	switch name {
	case "flyio_list_apps":
		return s.handleListApps(ctx)
	case "flyio_app_status":
		return s.handleAppStatus(ctx, arguments)
	default:
		return &ToolResult{
			Content: []ContentBlock{{
				Type: "text",
				Text: fmt.Sprintf("Unknown tool: %s", name),
			}},
			IsError: true,
		}, nil
	}
}

func (s *Server) handleListApps(ctx context.Context) (*ToolResult, error) {
	apps, err := s.flyioClient.ListApps(ctx)
	if err != nil {
		return &ToolResult{
			Content: []ContentBlock{{
				Type: "text",
				Text: fmt.Sprintf("Error listing apps: %v", err),
			}},
			IsError: true,
		}, nil
	}

	appsJSON, err := json.MarshalIndent(apps, "", "  ")
	if err != nil {
		return &ToolResult{
			Content: []ContentBlock{{
				Type: "text",
				Text: fmt.Sprintf("Error formatting apps: %v", err),
			}},
			IsError: true,
		}, nil
	}

	return &ToolResult{
		Content: []ContentBlock{{
			Type: "text",
			Text: string(appsJSON),
		}},
	}, nil
}

func (s *Server) handleAppStatus(ctx context.Context, arguments map[string]interface{}) (*ToolResult, error) {
	appName, ok := arguments["app_name"].(string)
	if !ok {
		return &ToolResult{
			Content: []ContentBlock{{
				Type: "text",
				Text: "Missing or invalid app_name parameter",
			}},
			IsError: true,
		}, nil
	}

	status, err := s.flyioClient.GetAppStatus(ctx, appName)
	if err != nil {
		return &ToolResult{
			Content: []ContentBlock{{
				Type: "text",
				Text: fmt.Sprintf("Error getting app status: %v", err),
			}},
			IsError: true,
		}, nil
	}

	statusJSON, err := json.MarshalIndent(status, "", "  ")
	if err != nil {
		return &ToolResult{
			Content: []ContentBlock{{
				Type: "text",
				Text: fmt.Sprintf("Error formatting status: %v", err),
			}},
			IsError: true,
		}, nil
	}

	return &ToolResult{
		Content: []ContentBlock{{
			Type: "text",
			Text: string(statusJSON),
		}},
	}, nil
}
