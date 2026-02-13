package main

import (
	"time"

	"github.com/augustdev/autoclip/internal/auth"
	"github.com/augustdev/autoclip/internal/authz"
	"github.com/augustdev/autoclip/internal/bootstrap"
	"github.com/augustdev/autoclip/internal/cloudflare"
	"github.com/augustdev/autoclip/internal/deployments"
	"github.com/augustdev/autoclip/internal/github_oauth"
	"github.com/augustdev/autoclip/internal/githubapp"
	"github.com/augustdev/autoclip/internal/internalgit"
	"github.com/augustdev/autoclip/internal/mcp_oauth"
	"github.com/augustdev/autoclip/internal/mcpserver"
	"github.com/augustdev/autoclip/internal/prometheus"
	"github.com/augustdev/autoclip/internal/resources"
	"github.com/augustdev/autoclip/internal/storage/pg"
	"github.com/augustdev/autoclip/internal/turso"
	"github.com/augustdev/autoclip/internal/webhooks"
	"go.uber.org/fx"
)

type config struct {
	fx.Out

	GraphQLAPI bootstrap.GraphQLAPIConfig
	Db         pg.DbConfig
	GitHub     github_oauth.Config
	GitHubApp  githubapp.Config
	Auth       auth.Config
	Temporal   bootstrap.TemporalClientConfig
	Turso      turso.Config
	Gitea      internalgit.Config
	Cloudflare cloudflare.Config
	MCPOAuth   mcp_oauth.Config
	Firebase   bootstrap.FirebaseConfig
	Loki       mcpserver.LokiConfig
	Prometheus prometheus.Config
}

func main() {
	fx.New(
		fx.StopTimeout(15*time.Second),
		fx.Provide(
			bootstrap.NewLogger,
			bootstrap.LoadConfig[config],
			pg.NewDatabase,
			pg.NewUserQueries,
			pg.NewAPIKeyQueries,
			pg.NewServiceQueries,
			pg.NewProjectQueries,
			pg.NewGitHubCredsQueries,
			pg.NewResourceQueries,
			pg.NewInternalReposQueries,
			pg.NewDNSRecordQueries,
			bootstrap.CreateTemporalClient,
			cloudflare.NewClient,
			github_oauth.NewOAuthService,
			githubapp.NewService,
			auth.NewService,
			auth.NewHandlers,
			authz.NewAPIKeyValidator,
			bootstrap.NewTursoClient,
			prometheus.NewClient,
			deployments.NewService,
			resources.NewService,
			bootstrap.NewInternalGitService,
			bootstrap.NewResolver,
			bootstrap.NewTokenValidator,
			mcpserver.NewServer,
			webhooks.NewHandlers,
			mcp_oauth.NewMCPOAuthService,
			mcp_oauth.NewHandlers,
			bootstrap.NewGraphQLRouter,
			bootstrap.NewAuthRouter,
		),
		fx.Invoke(
			bootstrap.StartServer,
		),
	).Run()
}
