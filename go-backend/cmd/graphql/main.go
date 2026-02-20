package main

import (
	"time"

	"github.com/augustdev/autoclip/internal/auth"
	"github.com/augustdev/autoclip/internal/bootstrap"
	"github.com/augustdev/autoclip/internal/deployments"
	"github.com/augustdev/autoclip/internal/dns"
	"github.com/augustdev/autoclip/internal/github_oauth"
	"github.com/augustdev/autoclip/internal/githubapp"
	"github.com/augustdev/autoclip/internal/internalgit"
	"github.com/augustdev/autoclip/internal/powerdns"
	"github.com/augustdev/autoclip/internal/prometheus"
	"github.com/augustdev/autoclip/internal/resources"
	"github.com/augustdev/autoclip/internal/storage/pg"
	"github.com/augustdev/autoclip/internal/turso"
	"github.com/augustdev/autoclip/internal/webhooks"
	"go.uber.org/fx"
)

type config struct {
	fx.Out

	GraphQLAPI     bootstrap.GraphQLAPIConfig
	TokenValidator bootstrap.TokenValidatorConfig
	Db             pg.DbConfig
	GitHub         github_oauth.Config
	GitHubApp      githubapp.Config
	Auth           auth.Config
	Temporal       bootstrap.TemporalClientConfig
	Turso          turso.Config
	InternalGit    internalgit.Config
	Firebase       bootstrap.FirebaseConfig
	Prometheus     prometheus.Config
	DNS            dns.Config
	PowerDNS       powerdns.Config
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
			pg.NewDnsQueries,
			pg.NewClusterMap,
			bootstrap.CreateTemporalClient,
			github_oauth.NewOAuthService,
			githubapp.NewService,
			auth.NewService,
			auth.NewHandlers,
			bootstrap.NewTursoClient,
			prometheus.NewClient,
			deployments.NewService,
			powerdns.NewClient,
			dns.NewService,
			resources.NewService,
			internalgit.NewService,
			bootstrap.NewLoaderDeps,
			bootstrap.NewResolver,
			bootstrap.NewTokenValidator,
			webhooks.NewHandlers,
			bootstrap.NewGraphQLRouter,
			bootstrap.NewAuthRouter,
		),
		fx.Invoke(
			bootstrap.StartServer,
		),
	).Run()
}
