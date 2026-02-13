package bootstrap

import (
	"fmt"
	"os"
	"strings"

	"github.com/joho/godotenv"
	"github.com/spf13/viper"
)

func LoadConfig[T any]() (T, error) {
	if err := InitConfig(); err != nil {
		var zero T
		return zero, err
	}

	var cfg T
	if err := viper.Unmarshal(&cfg); err != nil {
		var zero T
		return zero, fmt.Errorf("unable to decode config: %w", err)
	}

	return cfg, nil
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

type FirebaseConfig struct {
	ServiceAccountJSON string
}

type NATSConfig struct {
	URL   string
	Token string
}
