package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/augustdev/autoclip/internal/bootstrap"
	"github.com/augustdev/autoclip/internal/deployments"
	"github.com/augustdev/autoclip/internal/storage/pg"
	"go.temporal.io/sdk/worker"
	"go.uber.org/fx"
)

func main() {
	fx.New(
		fx.StopTimeout(1*time.Minute),
		fx.Provide(
			bootstrap.NewLogger,
			bootstrap.NewConfig,
			pg.NewDatabase,
			pg.NewServiceQueries,
			bootstrap.CreateTemporalClient,
			bootstrap.NewTemporalWorker,
			bootstrap.NewNatsClient,
			bootstrap.NewCoolifyClient,
			deployments.NewActivities,
		),
		fx.Invoke(
			deployments.RegisterWorkflowsAndActivities,
			startWorker,
		),
	).Run()
}

func startWorker(lc fx.Lifecycle, worker worker.Worker, logger *slog.Logger) {
	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			logger.Info("Starting temporal workers")
			go func() {
				if err := worker.Run(nil); err != nil {
					logger.Error(fmt.Sprintf("Default worker failed: %v", err))
					os.Exit(1)
				}
			}()
			return nil
		},
		OnStop: func(ctx context.Context) error {
			logger.Info("Stopping temporal workers")
			worker.Stop()
			return nil
		},
	})
}
