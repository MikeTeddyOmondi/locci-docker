package cli

import (
	"context"
	"log/slog"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/MikeTeddyOmondi/locci-docker/internal/config"
	"github.com/MikeTeddyOmondi/locci-docker/internal/docker"
	"github.com/MikeTeddyOmondi/locci-docker/internal/jobs"
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

			dk, err := docker.New(cfg.DockerHost, cfg.TraefikDomain, log)
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

			srv := &server.Server{Docker: dk, Store: db, AuthToken: cfg.AuthToken, Log: log}
			log.Info("listening", "port", cfg.ServerPort, "domain", cfg.TraefikDomain)
			return srv.Router(cfg.GinMode).Run(":" + cfg.ServerPort)
		},
	}
}
