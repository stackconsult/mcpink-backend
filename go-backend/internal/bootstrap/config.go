package bootstrap

import (
	"fmt"
	"os"
	"strings"

	"github.com/augustdev/autoclip/internal/auth"
	"github.com/augustdev/autoclip/internal/cloudflare"
	"github.com/augustdev/autoclip/internal/github_oauth"
	"github.com/augustdev/autoclip/internal/githubapp"
	"github.com/augustdev/autoclip/internal/internalgit"
	"github.com/augustdev/autoclip/internal/k8sdeployments"
	"github.com/augustdev/autoclip/internal/mcp_oauth"
	"github.com/augustdev/autoclip/internal/mcpserver"
	"github.com/augustdev/autoclip/internal/storage/pg"
	"github.com/augustdev/autoclip/internal/turso"
	"github.com/joho/godotenv"
	"github.com/spf13/viper"
	"go.uber.org/fx"
)

func NewConfig() (Config, error) {
	if err := InitConfig(); err != nil {
		return Config{}, err
	}

	var cfg struct {
		GraphQLAPI GraphQLAPIConfig
		Db         pg.DbConfig
		GitHub     github_oauth.Config
		GitHubApp  githubapp.Config
		Auth       auth.Config
		Temporal   TemporalClientConfig
		NATS       NATSConfig
		Turso      turso.Config
		Gitea      internalgit.Config
		Cloudflare cloudflare.Config
		MCPOAuth   mcp_oauth.Config
		Firebase   FirebaseConfig
		K8sWorker  k8sdeployments.Config
		Loki       mcpserver.LokiConfig
	}

	if err := viper.Unmarshal(&cfg); err != nil {
		return Config{}, fmt.Errorf("unable to decode config: %w", err)
	}

	return Config{
		GraphQLAPI: cfg.GraphQLAPI,
		Db:         cfg.Db,
		GitHub:     cfg.GitHub,
		GitHubApp:  cfg.GitHubApp,
		Auth:       cfg.Auth,
		Temporal:   cfg.Temporal,
		NATS:       cfg.NATS,
		Turso:      cfg.Turso,
		Gitea:      cfg.Gitea,
		Cloudflare: cfg.Cloudflare,
		MCPOAuth:   cfg.MCPOAuth,
		Firebase:   cfg.Firebase,
		K8sWorker:  cfg.K8sWorker,
		Loki:       cfg.Loki,
	}, nil
}

type FirebaseConfig struct {
	ServiceAccountJSON string
}

type Config struct {
	fx.Out

	GraphQLAPI GraphQLAPIConfig
	Db         pg.DbConfig
	GitHub     github_oauth.Config
	GitHubApp  githubapp.Config
	Auth       auth.Config
	Temporal   TemporalClientConfig
	NATS       NATSConfig
	Turso      turso.Config
	Gitea      internalgit.Config
	Cloudflare cloudflare.Config
	MCPOAuth   mcp_oauth.Config
	Firebase   FirebaseConfig
	K8sWorker  k8sdeployments.Config
	Loki       mcpserver.LokiConfig
}

func InitConfig() error {
	_ = godotenv.Load()

	if configFile := os.Getenv("APPLICATION_CONFIG"); configFile != "" {
		viper.SetConfigFile(configFile)
	} else {
		viper.SetConfigName("application")
		viper.AddConfigPath(".")
		viper.SetConfigType("yaml")
	}
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	viper.AutomaticEnv()
	if err := viper.ReadInConfig(); err != nil {
		return fmt.Errorf("error reading config file: %w", err)
	}

	return nil
}

type NATSConfig struct {
	URL   string
	Token string
}
