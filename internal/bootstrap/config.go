package bootstrap

import (
	"fmt"
	"os"
	"strings"

	"github.com/augustdev/autoclip/internal/auth"
	"github.com/augustdev/autoclip/internal/github"
	"github.com/augustdev/autoclip/internal/storage/pg"
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
		GitHub     github.Config
		Auth       auth.Config
	}

	if err := viper.Unmarshal(&cfg); err != nil {
		return Config{}, fmt.Errorf("unable to decode config: %w", err)
	}

	return Config{
		GraphQLAPI: cfg.GraphQLAPI,
		Db:         cfg.Db,
		GitHub:     cfg.GitHub,
		Auth:       cfg.Auth,
	}, nil
}

type Config struct {
	fx.Out

	GraphQLAPI GraphQLAPIConfig
	Db         pg.DbConfig
	GitHub     github.Config
	Auth       auth.Config
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
