package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/augustdev/autoclip/internal/bootstrap"
	"github.com/augustdev/autoclip/internal/dns"
	"github.com/augustdev/autoclip/internal/githubapp"
	"github.com/augustdev/autoclip/internal/k8sdeployments"
	"github.com/augustdev/autoclip/internal/powerdns"
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
	K8sWorker k8sdeployments.Config
	Cluster   bootstrap.ClusterConfig
	PowerDNS  powerdns.Config
}

type Workers struct {
	K8s worker.Worker
	DNS worker.Worker
}

func main() {
	fx.New(
		fx.StopTimeout(1*time.Minute),
		fx.Provide(
			bootstrap.NewLogger,
			bootstrap.LoadConfig[config],
			bootstrap.CreateTemporalClient,
			newWorkers,
			bootstrap.NewK8sClient,
			bootstrap.NewK8sDynamicClient,
			pg.NewDatabase,
			pg.NewServiceQueries,
			pg.NewDeploymentQueries,
			pg.NewProjectQueries,
			pg.NewUserQueries,
			pg.NewDnsQueries,
			powerdns.NewClient,
			pg.NewClusterMap,
			githubapp.NewService,
			k8sdeployments.NewActivities,
			dns.NewActivities,
		),
		fx.Invoke(
			registerAndStart,
		),
	).Run()
}

func newWorkers(c client.Client, clusterMap map[string]clusters.Cluster, cfg bootstrap.ClusterConfig) (*Workers, error) {
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

	slog.Info("Resolved cluster for worker",
		"region", cluster.Region,
		"task_queue", cluster.TaskQueue,
		"has_dns", cluster.HasDns)

	k8sWorker := worker.New(c, cluster.TaskQueue, worker.Options{
		WorkerStopTimeout: 10 * time.Minute,
	})

	w := &Workers{K8s: k8sWorker}

	if cluster.HasDns {
		w.DNS = worker.New(c, dns.TaskQueue, worker.Options{
			WorkerStopTimeout: 10 * time.Minute,
		})
	}

	return w, nil
}

func registerAndStart(
	lc fx.Lifecycle,
	w *Workers,
	activities *k8sdeployments.Activities,
	dnsActivities *dns.Activities,
	logger *slog.Logger,
) {
	k8sdeployments.RegisterWorkflowsAndActivities(w.K8s, activities)
	if w.DNS != nil {
		dns.RegisterWorkflowsAndActivities(w.DNS, dnsActivities)
	}

	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			logger.Info("Starting k8s temporal worker")
			go func() {
				if err := w.K8s.Run(worker.InterruptCh()); err != nil {
					logger.Error(fmt.Sprintf("k8s worker failed: %v", err))
					os.Exit(1)
				}
			}()
			if w.DNS != nil {
				logger.Info("Starting DNS temporal worker")
				go func() {
					if err := w.DNS.Run(worker.InterruptCh()); err != nil {
						logger.Error(fmt.Sprintf("DNS worker failed: %v", err))
						os.Exit(1)
					}
				}()
			}
			return nil
		},
		OnStop: func(ctx context.Context) error {
			logger.Info("Stopping temporal workers")
			w.K8s.Stop()
			if w.DNS != nil {
				w.DNS.Stop()
			}
			return nil
		},
	})
}
