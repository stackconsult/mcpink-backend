package logs

import "context"

type LogLine struct {
	Timestamp string `json:"timestamp,omitempty"`
	Stream    string `json:"stream,omitempty"`
	Message   string `json:"message"`
}

type Provider interface {
	// GetRuntimeLogs fetches container stdout/stderr logs
	GetRuntimeLogs(ctx context.Context, appIdentifier string, lines int) ([]LogLine, error)

	// GetDeploymentLogs fetches the latest deployment/build logs
	GetDeploymentLogs(ctx context.Context, appIdentifier string) ([]LogLine, error)
}
