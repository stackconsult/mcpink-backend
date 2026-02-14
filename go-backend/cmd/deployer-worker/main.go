package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/augustdev/autoclip/internal/bootstrap"
	"github.com/augustdev/autoclip/internal/githubapp"
	"github.com/augustdev/autoclip/internal/internalgit"
	"github.com/augustdev/autoclip/internal/k8sdeployments"
	"github.com/augustdev/autoclip/internal/storage/pg"
	"github.com/augustdev/autoclip/internal/storage/pg/generated/clusters"
	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/worker"
	"go.uber.org/fx"
)

type config struct {
	fx.Out

	Db        pg.DbConfig
	Temporal  bootstrap.TemporalClientConfig
	GitHubApp githubapp.Config
	Gitea     internalgit.Config
	K8sWorker k8sdeployments.Config
	Cluster   bootstrap.ClusterConfig
}

func main() {
	fx.New(
		fx.StopTimeout(1*time.Minute),
		fx.Provide(
			bootstrap.NewLogger,
			bootstrap.LoadConfig[config],
			bootstrap.CreateTemporalClient,
			newTemporalWorker,
			bootstrap.NewK8sClient,
			bootstrap.NewK8sDynamicClient,
			pg.NewDatabase,
			pg.NewServiceQueries,
			pg.NewDeploymentQueries,
			pg.NewProjectQueries,
			pg.NewUserQueries,
			pg.NewCustomDomainQueries,
			pg.NewClusterMap,
			githubapp.NewService,
			internalgit.NewService,
			k8sdeployments.NewActivities,
		),
		fx.Invoke(
			k8sdeployments.RegisterWorkflowsAndActivities,
			startK8sWorker,
		),
	).Run()
}

func newTemporalWorker(c client.Client, clusterMap map[string]clusters.Cluster, cfg bootstrap.ClusterConfig) (worker.Worker, error) {
	region := strings.TrimSpace(cfg.Region)
	if region == "" {
		return nil, fmt.Errorf("cluster.region is required (set CLUSTER_REGION)")
	}
	cluster, ok := clusterMap[region]
	if !ok {
		return nil, fmt.Errorf("unknown region %q", region)
	}
	if cluster.Status != "active" {
		return nil, fmt.Errorf("region %q is not active (status=%q)", region, cluster.Status)
	}
	if cluster.TaskQueue == "" {
		return nil, fmt.Errorf("region %q has empty task_queue", region)
	}
	slog.Info("Resolved cluster for worker", "region", cluster.Region, "task_queue", cluster.TaskQueue)
	return worker.New(c, cluster.TaskQueue, worker.Options{
		WorkerStopTimeout: 10 * time.Minute,
	}), nil
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
