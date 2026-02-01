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
	"github.com/augustdev/autoclip/internal/storage/pg"
	"go.uber.org/fx"
)

func main() {
	fx.New(
		fx.StopTimeout(1*time.Minute),
		fx.Provide(
			bootstrap.NewLogger,
			bootstrap.NewConfig,
			pg.NewDatabase,
			pg.NewUserQueries,
			pg.NewAPIKeyQueries,
			pg.NewAppQueries,
			pg.NewProjectQueries,
			pg.NewGitHubCredsQueries,
			bootstrap.CreateTemporalClient,
			github_oauth.NewOAuthService,
			githubapp.NewService,
			auth.NewService,
			auth.NewHandlers,
			authz.NewAPIKeyValidator,
			bootstrap.NewCoolifyClient,
			deployments.NewService,
			bootstrap.NewResolver,
			bootstrap.NewTokenValidator,
			mcpserver.NewServer,
			bootstrap.NewGraphQLRouter,
			bootstrap.NewAuthRouter,
		),
		fx.Invoke(
			bootstrap.StartServer,
		),
	).Run()
}
