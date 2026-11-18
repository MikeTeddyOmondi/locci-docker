package cli

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/MikeTeddyOmondi/locci-docker/internal/config"
	"github.com/MikeTeddyOmondi/locci-docker/internal/docker"
	"github.com/MikeTeddyOmondi/locci-docker/internal/env"
	"github.com/MikeTeddyOmondi/locci-docker/internal/jobs"
	"github.com/MikeTeddyOmondi/locci-docker/internal/secret"
	"github.com/MikeTeddyOmondi/locci-docker/internal/server"
	"github.com/MikeTeddyOmondi/locci-docker/internal/sso"
	"github.com/MikeTeddyOmondi/locci-docker/internal/store"
	"github.com/MikeTeddyOmondi/locci-docker/internal/tunnel"
)

// ShutdownGrace is how long `serve` waits for running requests and a running
// deploy job after SIGINT or SIGTERM. Give the container at least this long to
// stop (compose.yaml sets stop_grace_period).
const ShutdownGrace = 25 * time.Second

func serveCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "serve",
		Short: "Run the API server (and the RabbitMQ deploy consumer when RABBITMQ_URL is set)",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg := config.Load()
			log := slog.New(slog.NewJSONHandler(os.Stdout, nil))

			// SIGINT/SIGTERM (Ctrl+C, docker stop) start a graceful shutdown.
			ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
			defer stop()

			db, err := store.Open(cfg.DatabaseURL)
			if err != nil {
				return err
			}
			defer db.Close()
			db.TokenTTL = time.Duration(cfg.TokenTTLDays) * 24 * time.Hour

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
			envs := &env.Registry{Store: db, Box: box, DockerHost: cfg.DockerHost, Domain: cfg.TraefikDomain, Hub: &tunnel.Hub{}, Log: log}
			defer envs.Close()
			go envs.Run(ctx, 30*time.Second)

			jobsDone := make(chan struct{})
			if cfg.RabbitMQURL == "" {
				close(jobsDone)
			} else {
				consumer := &jobs.Consumer{
					URL:     cfg.RabbitMQURL,
					Queue:   cfg.QueueName,
					Timeout: time.Duration(cfg.DeployJobTimeout) * time.Second,
					// Jobs name an environment in "host"; empty means the default.
					Resolve: func(host string) (*docker.Service, error) {
						if host == "" {
							svc, _, err := envs.Default()
							return svc, err
						}
						svc, _, err := envs.Get(host)
						return svc, err
					},
					Log: log,
				}
				go func() {
					defer close(jobsDone)
					// Exit so the supervisor restarts us and the consumer reconnects.
					if err := consumer.Run(ctx); err != nil && ctx.Err() == nil {
						log.Error("deploy consumer stopped", "err", err)
						os.Exit(1)
					}
				}()
			}

			// OIDC sign-in uses the same key for client secrets and its flow cookie.
			sign := &sso.Service{Store: db, Box: box, Log: log}

			proxies, err := server.ParseProxies(cfg.TrustedProxies)
			if err != nil {
				return err
			}
			srv := &server.Server{Envs: envs, SSO: sign, PublicURL: cfg.PublicURL, Store: db, AuthToken: cfg.AuthToken, Log: log, TrustedProxies: proxies}
			httpSrv := srv.HTTPServer(":"+cfg.ServerPort, cfg.GinMode)
			served := make(chan error, 1)
			go func() { served <- httpSrv.ListenAndServe() }()
			log.Info("listening", "port", cfg.ServerPort, "domain", cfg.TraefikDomain)

			select {
			case err := <-served:
				return err // could not listen
			case <-ctx.Done():
			}

			// Stop taking requests, let running ones and a running deploy job
			// finish, but don't wait forever.
			log.Info("shutting down")
			grace, cancel := context.WithTimeout(context.Background(), ShutdownGrace)
			defer cancel()
			if err := httpSrv.Shutdown(grace); err != nil {
				log.Warn("requests still running at shutdown", "err", err)
			}
			select {
			case <-jobsDone:
			case <-grace.Done():
				log.Warn("deploy job still running at shutdown; RabbitMQ will redeliver it")
			}
			return nil
		},
	}
}
