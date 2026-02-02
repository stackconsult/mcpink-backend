package main

import (
	"time"

	"github.com/augustdev/autoclip/internal/auth"
	"github.com/augustdev/autoclip/internal/authz"
	"github.com/augustdev/autoclip/internal/bootstrap"
	"github.com/augustdev/autoclip/internal/deployments"
	"github.com/augustdev/autoclip/internal/github_oauth"
	"github.com/augustdev/autoclip/internal/githubapp"
	"github.com/augustdev/autoclip/internal/mcpserver"
	"github.com/augustdev/autoclip/internal/resources"
	"github.com/augustdev/autoclip/internal/storage/pg"
	"github.com/augustdev/autoclip/internal/webhooks"
	"go.uber.org/fx"
)

func main() {
	fx.New(
		fx.StopTimeout(15*time.Second),
		fx.Provide(
			bootstrap.NewLogger,
			bootstrap.NewConfig,
			pg.NewDatabase,
			pg.NewUserQueries,
			pg.NewAPIKeyQueries,
			pg.NewAppQueries,
			pg.NewProjectQueries,
			pg.NewGitHubCredsQueries,
			pg.NewResourceQueries,
			bootstrap.CreateTemporalClient,
			github_oauth.NewOAuthService,
			githubapp.NewService,
			auth.NewService,
			auth.NewHandlers,
			authz.NewAPIKeyValidator,
			bootstrap.NewCoolifyClient,
			bootstrap.NewTursoClient,
			bootstrap.NewLogProvider,
			deployments.NewService,
			resources.NewService,
			bootstrap.NewResolver,
			bootstrap.NewTokenValidator,
			mcpserver.NewServer,
			webhooks.NewHandlers,
			bootstrap.NewGraphQLRouter,
			bootstrap.NewAuthRouter,
		),
		fx.Invoke(
			bootstrap.StartServer,
		),
	).Run()
}
