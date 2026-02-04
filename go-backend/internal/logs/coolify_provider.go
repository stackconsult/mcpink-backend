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

	coolifyLogs, err := p.client.Applications.GetRuntimeLogs(ctx, coolifyUUID, lines)
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

func (p *CoolifyProvider) GetDeploymentLogs(ctx context.Context, coolifyUUID string) ([]LogLine, error) {
	if coolifyUUID == "" {
		return nil, fmt.Errorf("app not deployed yet")
	}

	coolifyLogs, err := p.client.Applications.GetDeploymentLogs(ctx, coolifyUUID)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch deployment logs: %w", err)
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
