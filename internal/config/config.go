package config

import (
	"path/filepath"

	"github.com/joho/godotenv"
	"github.com/kelseyhightower/envconfig"
)

type Config struct {
	AppEnv        string `envconfig:"APP_ENV" default:"dev"`
	HTTPAddr      string `envconfig:"HTTP_ADDR" default:":8080"`
	PostgresDSN   string `envconfig:"POSTGRES_DSN" required:"true"`
	RedisAddr     string `envconfig:"REDIS_ADDR" default:"localhost:6379"`
	RedisPassword string `envconfig:"REDIS_PASSWORD" default:""`
}

func Load() (Config, error) {
	envPaths := []string{".env", filepath.Join(".", ".env")}

	for _, path := range envPaths {
		if err := godotenv.Load(path); err == nil {
			break
		}
	}

	var c Config
	err := envconfig.Process("", &c)
	return c, err
}
