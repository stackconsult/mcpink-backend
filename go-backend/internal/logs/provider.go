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

	// GetBuildLogs fetches deployment/build logs
	GetBuildLogs(ctx context.Context, appIdentifier string, lines int) ([]LogLine, error)
}
