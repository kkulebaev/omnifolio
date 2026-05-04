package config

import (
	"fmt"

	"github.com/caarlos0/env/v11"
)

type Config struct {
	Port        int    `env:"PORT" envDefault:"8080"`
	DatabaseURL string `env:"DATABASE_URL,required"`
	MasterKey   string `env:"MASTER_KEY,required"`
	LogLevel    string `env:"LOG_LEVEL" envDefault:"info"`
	Env         string `env:"ENV" envDefault:"dev"`

	BootstrapUserEmail    string `env:"BOOTSTRAP_USER_EMAIL"`
	BootstrapUserPassword string `env:"BOOTSTRAP_USER_PASSWORD"`

	AdminAPIKey string `env:"ADMIN_API_KEY,required"`

	SessionIdleTimeout     string `env:"SESSION_IDLE_TIMEOUT" envDefault:"30m"`
	SessionAbsoluteTimeout string `env:"SESSION_ABSOLUTE_TIMEOUT" envDefault:"720h"`
}

func Load() (Config, error) {
	var cfg Config
	if err := env.Parse(&cfg); err != nil {
		return Config{}, fmt.Errorf("parse env: %w", err)
	}
	return cfg, nil
}

func (c Config) IsProduction() bool {
	return c.Env == "prod" || c.Env == "production"
}
