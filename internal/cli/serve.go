package cli

import (
	"context"
	"log/slog"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/MikeTeddyOmondi/locci-docker/internal/config"
	"github.com/MikeTeddyOmondi/locci-docker/internal/env"
	"github.com/MikeTeddyOmondi/locci-docker/internal/jobs"
	"github.com/MikeTeddyOmondi/locci-docker/internal/secret"
	"github.com/MikeTeddyOmondi/locci-docker/internal/server"
	"github.com/MikeTeddyOmondi/locci-docker/internal/store"
)

func serveCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "serve",
		Short: "Run the API server (and the RabbitMQ deploy consumer when RABBITMQ_URL is set)",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg := config.Load()
			log := slog.New(slog.NewJSONHandler(os.Stdout, nil))

			db, err := store.Open(cfg.DatabaseURL)
			if err != nil {
				return err
			}
			defer db.Close()

			// First start: register this host as the "local" default environment.
			if _, err := db.EnsureDefaultEnvironment(); err != nil {
				return err
			}

			if users, _ := db.ListUsers(); len(users) == 0 && cfg.AuthToken == "" {
				log.Warn("no users yet and no AUTH_TOKEN: create one with `locci user add <name> --admin`")
			}

			box, err := secret.New(cfg.SecretKey)
			if err != nil {
				return err
			}
			if box == nil {
				log.Warn("LOCCI_SECRET_KEY is not set: remote (tcp+tls, ssh) environments can't be added or used")
			}

			// One Docker connection per environment, checked every 30s for the UI.
			envs := &env.Registry{Store: db, Box: box, DockerHost: cfg.DockerHost, Domain: cfg.TraefikDomain, Log: log}
			defer envs.Close()
			go envs.Run(context.Background(), 30*time.Second)

			// The deploy consumer uses the default environment.
			dk, _, err := envs.Default()
			if err != nil {
				return err
			}

			if cfg.RabbitMQURL != "" {
				consumer := &jobs.Consumer{
					URL:     cfg.RabbitMQURL,
					Queue:   cfg.QueueName,
					Timeout: time.Duration(cfg.DeployJobTimeout) * time.Second,
					Docker:  dk,
					Log:     log,
				}
				go func() {
					// Exit so the supervisor restarts us and the consumer reconnects.
					if err := consumer.Run(context.Background()); err != nil {
						log.Error("deploy consumer stopped", "err", err)
						os.Exit(1)
					}
				}()
			}

			srv := &server.Server{Envs: envs, Store: db, AuthToken: cfg.AuthToken, Log: log}
			log.Info("listening", "port", cfg.ServerPort, "domain", cfg.TraefikDomain)
			return srv.Router(cfg.GinMode).Run(":" + cfg.ServerPort)
		},
	}
}
