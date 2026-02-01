package logs

import (
	"context"
	"fmt"

	"github.com/augustdev/autoclip/internal/coolify"
)

type CoolifyProvider struct {
	client *coolify.Client
}

func NewCoolifyProvider(client *coolify.Client) *CoolifyProvider {
	return &CoolifyProvider{client: client}
}

func (p *CoolifyProvider) GetRuntimeLogs(ctx context.Context, coolifyUUID string, lines int) ([]LogLine, error) {
	if coolifyUUID == "" {
		return nil, fmt.Errorf("app not deployed yet")
	}

	if p.client == nil {
		return nil, fmt.Errorf("coolify client not configured")
	}

	coolifyLogs, err := p.client.Applications.GetLogs(ctx, coolifyUUID, &coolify.GetLogsOptions{
		Tail:       lines,
		Timestamps: true,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to fetch runtime logs: %w", err)
	}

	result := make([]LogLine, len(coolifyLogs))
	for i, l := range coolifyLogs {
		result[i] = LogLine{
			Timestamp: l.Timestamp,
			Stream:    l.Stream,
			Message:   l.Message,
		}
	}
	return result, nil
}

func (p *CoolifyProvider) GetBuildLogs(ctx context.Context, coolifyUUID string, lines int) ([]LogLine, error) {
	// TODO: Implement when Coolify deployment logs API is integrated
	// For now, return empty - build logs require deployment UUID tracking
	return []LogLine{}, nil
}
