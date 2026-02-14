package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/augustdev/autoclip/internal/account"
	"github.com/augustdev/autoclip/internal/bootstrap"
	"github.com/augustdev/autoclip/internal/storage/pg"
	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/worker"
	"go.uber.org/fx"
)

type config struct {
	fx.Out

	Db       pg.DbConfig
	Temporal bootstrap.TemporalClientConfig
}

func main() {
	fx.New(
		fx.StopTimeout(1*time.Minute),
		fx.Provide(
			bootstrap.NewLogger,
			bootstrap.LoadConfig[config],
			pg.NewDatabase,
			pg.NewProjectQueries,
			bootstrap.CreateTemporalClient,
			newTemporalWorker,
			account.NewActivities,
		),
		fx.Invoke(
			account.RegisterWorkflowsAndActivities,
			startWorker,
		),
	).Run()
}

func newTemporalWorker(c client.Client) worker.Worker {
	return worker.New(c, "default", worker.Options{
		WorkerStopTimeout: 10 * time.Minute,
	})
}

func startWorker(lc fx.Lifecycle, w worker.Worker, logger *slog.Logger) {
	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			logger.Info("Starting temporal worker")
			go func() {
				if err := w.Run(worker.InterruptCh()); err != nil {
					logger.Error(fmt.Sprintf("Worker failed: %v", err))
					os.Exit(1)
				}
			}()
			return nil
		},
		OnStop: func(ctx context.Context) error {
			logger.Info("Stopping temporal worker")
			w.Stop()
			return nil
		},
	})
}
