package main

import (
	"github.com/augustdev/autoclip/internal/auth"
	"github.com/augustdev/autoclip/internal/authz"
	"github.com/augustdev/autoclip/internal/bootstrap"
	"github.com/augustdev/autoclip/internal/github"
	"github.com/augustdev/autoclip/internal/mcp"
	"github.com/augustdev/autoclip/internal/storage/pg"
	"go.uber.org/fx"
)

func main() {
	fx.New(
		fx.Provide(
			bootstrap.NewLogger,
			bootstrap.NewConfig,
			pg.NewDatabase,
			pg.NewUserQueries,
			pg.NewAPIKeyQueries,
			github.NewOAuthService,
			auth.NewService,
			auth.NewHandlers,
			mcp.NewHandlers,
			authz.NewAPIKeyValidator,
			bootstrap.NewResolver,
			bootstrap.NewTokenValidator,
			bootstrap.NewGraphQLRouter,
			bootstrap.NewAuthRouter,
		),
		fx.Invoke(
			bootstrap.StartServer,
		),
	).Run()
}
