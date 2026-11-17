package cli

import (
	"context"
	"errors"
	"fmt"
	"html"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"runtime"
	"time"

	"github.com/spf13/cobra"

	"github.com/MikeTeddyOmondi/locci-docker/internal/client"
	"github.com/MikeTeddyOmondi/locci-docker/internal/store"
)

// ssoLogin signs in through the server's identity provider: it listens on
// 127.0.0.1, opens the browser at the server's start URL with that port, and
// waits for the server to redirect back with a token.
func ssoLogin(cfg client.Config, provider string) (string, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", err
	}
	defer ln.Close()
	port := ln.Addr().(*net.TCPAddr).Port

	type result struct {
		token string
		err   error
	}
	done := make(chan result, 1)
	srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/callback" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if e := r.URL.Query().Get("error"); e != "" {
			fmt.Fprintf(w, "<p>Sign-in failed: %s</p><p>You can close this tab.</p>", html.EscapeString(e))
			done <- result{err: errors.New(e)}
			return
		}
		fmt.Fprint(w, "<p>Signed in to locci. You can close this tab and return to the terminal.</p>")
		done <- result{token: r.URL.Query().Get("token")}
	})}
	go srv.Serve(ln)
	defer srv.Shutdown(context.Background())

	start := fmt.Sprintf("%s/api/v1/auth/oidc/%s/start?cli_port=%d", cfg.URL, url.PathEscape(provider), port)
	fmt.Fprintf(os.Stderr, "Opening your browser to sign in with %s.\nIf it doesn't open, visit:\n  %s\n", provider, start)
	_ = openURL(start)

	select {
	case r := <-done:
		return r.token, r.err
	case <-time.After(5 * time.Minute):
		return "", errors.New("timed out waiting for the browser sign-in")
	}
}

// openURL opens the user's browser; tests replace it.
var openURL = openBrowser

func openBrowser(u string) error {
	switch runtime.GOOS {
	case "darwin":
		return exec.Command("open", u).Start()
	case "windows":
		return exec.Command("rundll32", "url.dll,FileProtocolHandler", u).Start()
	default:
		return exec.Command("xdg-open", u).Start()
	}
}

// authCmd holds server-host switches that must work even when nobody can sign
// in (break-glass): it writes the database directly, like `locci user`.
func authCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "auth",
		Short: "Sign-in settings in the local database (run on the server host)",
	}
	cmd.AddCommand(&cobra.Command{
		Use:       "password-login on|off",
		Short:     "Allow or refuse username/password login",
		Example:   "  locci auth password-login on    # break-glass if SSO is broken",
		Args:      cobra.ExactArgs(1),
		ValidArgs: []string{"on", "off"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if args[0] != "on" && args[0] != "off" {
				return errors.New("use on or off")
			}
			return withStore(func(db *store.Store) error {
				if err := db.SetPasswordLogin(args[0] == "on"); err != nil {
					return err
				}
				fmt.Println("password login is", args[0])
				return nil
			})
		},
	})
	return cmd
}
