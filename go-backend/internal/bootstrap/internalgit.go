package bootstrap

import (
	"fmt"
	"log/slog"

	"github.com/augustdev/autoclip/internal/internalgit"
	"github.com/augustdev/autoclip/internal/storage/pg"
)

func NewInternalGitService(config internalgit.Config, db *pg.DB, logger *slog.Logger) (*internalgit.Service, error) {
	client, err := internalgit.NewClient(config)
	if err != nil {
		return nil, fmt.Errorf("internal git client: %w", err)
	}

	webhookURL := "https://api.ml.ink/webhooks/internal-git"

	svc := internalgit.NewService(internalgit.ServiceConfig{
		Client:     client,
		DB:         db.Pool,
		WebhookURL: webhookURL,
	})

	logger.Info("Internal git service initialized", "baseURL", config.BaseURL)
	return svc, nil
}
