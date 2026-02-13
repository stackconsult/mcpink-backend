package main

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/augustdev/autoclip/internal/bootstrap"
	"github.com/augustdev/autoclip/internal/cloudflare"
	"github.com/augustdev/autoclip/internal/deployments"
	"github.com/augustdev/autoclip/internal/githubapp"
	"github.com/augustdev/autoclip/internal/storage/pg"
	"github.com/augustdev/autoclip/internal/webhooks"
	"github.com/go-chi/chi/v5"
	"go.uber.org/fx"
)

type config struct {
	fx.Out

	GraphQLAPI bootstrap.GraphQLAPIConfig
	Db         pg.DbConfig
	GitHubApp  githubapp.Config
	Temporal   bootstrap.TemporalClientConfig
	Cloudflare cloudflare.Config
}

func main() {
	fx.New(
		fx.StopTimeout(15*time.Second),
		fx.Provide(
			bootstrap.NewLogger,
			bootstrap.LoadConfig[config],
			pg.NewDatabase,
			pg.NewServiceQueries,
			pg.NewGitHubCredsQueries,
			pg.NewInternalReposQueries,
			bootstrap.CreateTemporalClient,
			githubapp.NewService,
			deployments.NewService,
			webhooks.NewHandlers,
			// Transitive deps for deployments.NewService
			pg.NewUserQueries,
			pg.NewProjectQueries,
		),
		fx.Invoke(
			startDeployerServer,
		),
	).Run()
}

func startDeployerServer(lc fx.Lifecycle, handlers *webhooks.Handlers, config bootstrap.GraphQLAPIConfig, logger *slog.Logger) {
	router := chi.NewRouter()
	handlers.RegisterRoutes(router)

	server := &http.Server{
		Addr:    ":" + config.Port,
		Handler: router,
	}

	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			logger.Info("Starting deployer server", "port", config.Port)
			go func() {
				if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
					logger.Error("Deployer server failed", "error", err)
				}
			}()
			return nil
		},
		OnStop: func(ctx context.Context) error {
			logger.Info("Shutting down deployer server...")
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			return server.Shutdown(shutdownCtx)
		},
	})
}
