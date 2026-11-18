package cli

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/gorilla/websocket"
	"github.com/spf13/cobra"

	"github.com/MikeTeddyOmondi/locci-docker/internal/tunnel"
)

// errBadToken stops the agent instead of retrying: the token was rotated or
// the environment removed, and retrying won't fix that.
var errBadToken = errors.New("the server refused the agent token (rotated, or the environment was removed)")

func agentCmd() *cobra.Command {
	var server, token, dockerHost string
	cmd := &cobra.Command{
		Use:   "agent",
		Short: "Connect this host's Docker to a locci server (outbound, works behind NAT)",
		Long: `Runs on a Docker host and dials out to a locci server over a WebSocket, so
the server can manage this host without any open port. Create an "agent"
environment on the server first; it gives you the token.`,
		Example: `  LOCCI_AGENT_TOKEN=la_... locci agent --server https://locci.example.com
  docker run -d --restart unless-stopped -v /var/run/docker.sock:/var/run/docker.sock \
    -e LOCCI_AGENT_TOKEN=la_... locci/docker agent --server https://locci.example.com`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if token == "" {
				token = os.Getenv("LOCCI_AGENT_TOKEN")
			}
			if server == "" {
				server = os.Getenv("LOCCI_SERVER")
			}
			if server == "" || token == "" {
				return errors.New("--server (or LOCCI_SERVER) and LOCCI_AGENT_TOKEN (or --token) are required")
			}
			wsURL, err := agentURL(server)
			if err != nil {
				return err
			}
			dial, err := dockerDialer(dockerHost)
			if err != nil {
				return err
			}
			log := slog.New(slog.NewTextHandler(os.Stderr, nil))
			ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
			defer stop()
			return runAgent(ctx, wsURL, token, dial, log)
		},
	}
	cmd.Flags().StringVar(&server, "server", "", "locci server URL, e.g. https://locci.example.com (or LOCCI_SERVER)")
	cmd.Flags().StringVar(&token, "token", "", "agent token; prefer LOCCI_AGENT_TOKEN so it doesn't show in process lists")
	cmd.Flags().StringVar(&dockerHost, "docker-host", "", "Docker to expose (default: DOCKER_HOST, then unix:///var/run/docker.sock)")
	return cmd
}

// agentURL turns https://host into wss://host/api/v1/agent/connect.
func agentURL(server string) (string, error) {
	u, err := url.Parse(strings.TrimRight(server, "/"))
	if err != nil || u.Host == "" {
		return "", fmt.Errorf("invalid server URL %q", server)
	}
	switch u.Scheme {
	case "https":
		u.Scheme = "wss"
	case "http":
		u.Scheme = "ws"
	default:
		return "", fmt.Errorf("server URL must start with https:// (or http:// for testing)")
	}
	u.Path += "/api/v1/agent/connect"
	return u.String(), nil
}

// dockerDialer connects to the local Docker the agent exposes.
func dockerDialer(host string) (func(context.Context) (net.Conn, error), error) {
	if host == "" {
		host = os.Getenv("DOCKER_HOST")
	}
	if host == "" {
		host = "unix:///var/run/docker.sock"
	}
	u, err := url.Parse(host)
	if err != nil {
		return nil, err
	}
	network, addr := "", ""
	switch u.Scheme {
	case "unix":
		network, addr = "unix", u.Path
	case "tcp":
		network, addr = "tcp", u.Host
	default:
		return nil, fmt.Errorf("unsupported docker host %q (use unix:// or tcp://)", host)
	}
	return func(ctx context.Context) (net.Conn, error) {
		return (&net.Dialer{Timeout: 10 * time.Second}).DialContext(ctx, network, addr)
	}, nil
}

// runAgent keeps one tunnel up, reconnecting with backoff until ctx ends.
func runAgent(ctx context.Context, wsURL, token string, dial func(context.Context) (net.Conn, error), log *slog.Logger) error {
	backoff := time.Second
	for {
		started := time.Now()
		err := agentOnce(ctx, wsURL, token, dial, log)
		if ctx.Err() != nil {
			return nil
		}
		if errors.Is(err, errBadToken) {
			return err
		}
		if time.Since(started) > time.Minute {
			backoff = time.Second // it had been up for a while: retry quickly
		}
		log.Warn("tunnel down, reconnecting", "err", err, "in", backoff)
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(backoff):
		}
		if backoff < 30*time.Second {
			backoff *= 2
		}
	}
}

func agentOnce(ctx context.Context, wsURL, token string, dial func(context.Context) (net.Conn, error), log *slog.Logger) error {
	d := websocket.Dialer{HandshakeTimeout: 15 * time.Second, Proxy: http.ProxyFromEnvironment}
	ws, resp, err := d.DialContext(ctx, wsURL, http.Header{"Authorization": {"Bearer " + token}})
	if err != nil {
		if resp != nil && resp.StatusCode == http.StatusUnauthorized {
			return errBadToken
		}
		return err
	}
	log.Info("connected", "server", wsURL, "environment", resp.Header.Get("X-Locci-Environment"))
	return tunnel.Serve(ctx, tunnel.NewConn(ws), dial, log)
}
