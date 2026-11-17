// Package config loads settings from the environment (and an optional .env file).
package config

import (
	"os"
	"strconv"

	"github.com/joho/godotenv"
)

type Config struct {
	DockerHost       string
	AuthToken        string
	RabbitMQURL      string
	QueueName        string
	ServerPort       string
	TraefikDomain    string
	DeployJobTimeout int
	GinMode          string
	DatabaseURL      string
	SecretKey        string // LOCCI_SECRET_KEY, base64 32 bytes (internal/secret)
}

// Load reads .env if present, then the process environment.
func Load() Config {
	_ = godotenv.Load() // .env is optional

	return Config{
		DockerHost:       os.Getenv("DOCKER_HOST"),
		AuthToken:        os.Getenv("AUTH_TOKEN"),
		RabbitMQURL:      os.Getenv("RABBITMQ_URL"),
		QueueName:        get("QUEUE_NAME", "deploy.trigger"),
		ServerPort:       get("SERVER_PORT", "8998"),
		TraefikDomain:    get("TRAEFIK_DOMAIN", "locci.cloud"),
		DeployJobTimeout: getInt("DEPLOY_JOB_TIMEOUT", 1800),
		GinMode:          get("GIN_MODE", "release"),
		DatabaseURL:      get("DATABASE_URL", "file:locci.db"),
		SecretKey:        os.Getenv("LOCCI_SECRET_KEY"),
	}
}

func get(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func getInt(key string, def int) int {
	if n, err := strconv.Atoi(os.Getenv(key)); err == nil {
		return n
	}
	return def
}
