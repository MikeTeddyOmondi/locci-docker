package cli

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/MikeTeddyOmondi/locci-docker/internal/config"
	"github.com/MikeTeddyOmondi/locci-docker/internal/docker"
	"github.com/MikeTeddyOmondi/locci-docker/internal/jobs"
	"github.com/MikeTeddyOmondi/locci-docker/internal/server"
)

func serveCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "serve",
		Short: "Run the API server (and the RabbitMQ deploy consumer when RABBITMQ_URL is set)",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg := config.Load()
			log := slog.New(slog.NewJSONHandler(os.Stdout, nil))

			if cfg.AuthToken == "" {
				return errors.New("AUTH_TOKEN is required")
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

			srv := &server.Server{Docker: dk, AuthToken: cfg.AuthToken, Log: log}
			log.Info("listening", "port", cfg.ServerPort, "domain", cfg.TraefikDomain)
			return srv.Router(cfg.GinMode).Run(":" + cfg.ServerPort)
		},
	}
}
