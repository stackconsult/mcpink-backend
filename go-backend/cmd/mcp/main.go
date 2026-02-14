package main

import (
	"time"

	"github.com/augustdev/autoclip/internal/auth"
	"github.com/augustdev/autoclip/internal/bootstrap"
	"github.com/augustdev/autoclip/internal/deployments"
	"github.com/augustdev/autoclip/internal/github_oauth"
	"github.com/augustdev/autoclip/internal/githubapp"
	"github.com/augustdev/autoclip/internal/internalgit"
	"github.com/augustdev/autoclip/internal/mcp_oauth"
	"github.com/augustdev/autoclip/internal/mcpserver"
	"github.com/augustdev/autoclip/internal/resources"
	"github.com/augustdev/autoclip/internal/storage/pg"
	"github.com/augustdev/autoclip/internal/turso"
	"go.uber.org/fx"
)

type config struct {
	fx.Out

	MCPAPI         bootstrap.MCPAPIConfig
	TokenValidator bootstrap.TokenValidatorConfig
	Db             pg.DbConfig
	GitHub         github_oauth.Config
	GitHubApp      githubapp.Config
	Auth           auth.Config
	Temporal       bootstrap.TemporalClientConfig
	Turso          turso.Config
	Gitea          internalgit.Config
	MCPOAuth       mcp_oauth.Config
	Firebase       bootstrap.FirebaseConfig
	Loki           mcpserver.LokiConfig
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
			pg.NewDeploymentQueries,
			pg.NewProjectQueries,
			pg.NewGitHubCredsQueries,
			pg.NewResourceQueries,
			pg.NewInternalReposQueries,
			pg.NewCustomDomainQueries,
			pg.NewClusterQueries,
			bootstrap.CreateTemporalClient,
			github_oauth.NewOAuthService,
			githubapp.NewService,
			auth.NewService,
			bootstrap.NewTursoClient,
			deployments.NewClusterResolver,
			deployments.NewService,
			resources.NewService,
			bootstrap.NewInternalGitService,
			bootstrap.NewTokenValidator,
			mcpserver.NewServer,
			mcp_oauth.NewMCPOAuthService,
			mcp_oauth.NewHandlers,
			bootstrap.NewMCPRouter,
		),
		fx.Invoke(
			bootstrap.StartMCPServer,
		),
	).Run()
}
