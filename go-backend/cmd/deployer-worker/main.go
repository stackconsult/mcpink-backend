package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/augustdev/autoclip/internal/bootstrap"
	"github.com/augustdev/autoclip/internal/githubapp"
	"github.com/augustdev/autoclip/internal/k8sdeployments"
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
			bootstrap.CreateTemporalClient,
			bootstrap.NewK8sTemporalWorker,
			bootstrap.NewK8sClient,
			pg.NewDatabase,
			pg.NewServiceQueries,
			pg.NewProjectQueries,
			pg.NewUserQueries,
			githubapp.NewService,
			bootstrap.NewInternalGitService,
			k8sdeployments.NewActivities,
		),
		fx.Invoke(
			k8sdeployments.RegisterWorkflowsAndActivities,
			startK8sWorker,
		),
	).Run()
}

func startK8sWorker(lc fx.Lifecycle, w worker.Worker, logger *slog.Logger) {
	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			logger.Info("Starting k8s temporal worker")
			go func() {
				if err := w.Run(worker.InterruptCh()); err != nil {
					logger.Error(fmt.Sprintf("k8s worker failed: %v", err))
					os.Exit(1)
				}
			}()
			return nil
		},
		OnStop: func(ctx context.Context) error {
			logger.Info("Stopping k8s temporal worker")
			w.Stop()
			return nil
		},
	})
}
