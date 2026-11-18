package cli

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/MikeTeddyOmondi/locci-docker/internal/client"
)

func loginCmd() *cobra.Command {
	var url, username, token, ssoProvider string
	var device bool
	cmd := &cobra.Command{
		Use:   "login",
		Short: "Log in to a locci server and save the token",
		Example: `  locci login --url https://locci.example.com -u mike
  locci login --url https://locci.example.com --sso keycloak
  locci login --url https://locci.example.com --sso keycloak --device   # no browser here
  locci login --url http://localhost:8998 --token $AUTH_TOKEN`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg := client.LoadConfig()
			if url != "" {
				cfg.URL = url
			}

			if device && ssoProvider == "" {
				return errors.New("--device needs --sso <provider>")
			}
			if token == "" && ssoProvider != "" {
				login := ssoLogin
				if device {
					login = ssoDeviceLogin
				}
				t, err := login(cfg, ssoProvider)
				if err != nil {
					return err
				}
				token = t
			}

			if token == "" {
				if username == "" {
					fmt.Fprint(os.Stderr, "username: ")
					line, _ := bufio.NewReader(os.Stdin).ReadString('\n')
					username = strings.TrimSpace(line)
				}
				var out struct {
					Token string `json:"token"`
				}
				body := map[string]string{"username": username, "password": readPassword()}
				if err := client.New(cfg).Do("POST", "/api/v1/auth/login", body, &out); err != nil {
					return err
				}
				token = out.Token
			}
			cfg.Token = token

			var me struct {
				Username string `json:"username"`
				Role     string `json:"role"`
			}
			if err := client.New(cfg).Do("GET", "/api/v1/auth/me", nil, &me); err != nil {
				return err
			}
			if err := client.SaveConfig(cfg); err != nil {
				return err
			}
			fmt.Printf("logged in to %s as %s (%s)\n", cfg.URL, me.Username, me.Role)
			return nil
		},
	}
	cmd.Flags().StringVar(&url, "url", "", "server URL (default http://localhost:8998)")
	cmd.Flags().StringVarP(&username, "username", "u", "", "username")
	cmd.Flags().StringVar(&token, "token", "", "use an existing API token or AUTH_TOKEN instead of a password")
	cmd.Flags().StringVar(&ssoProvider, "sso", "", "sign in through this identity provider in your browser (see the login page for names)")
	cmd.Flags().BoolVar(&device, "device", false, "with --sso: show a code to approve on another device instead of opening a browser here")
	return cmd
}

func logoutCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "logout",
		Short: "Revoke the saved token and forget it",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg := client.LoadConfig()
			_ = client.New(cfg).Do("POST", "/api/v1/auth/logout", nil, nil)
			cfg.Token = ""
			return client.SaveConfig(cfg)
		},
	}
}

func whoamiCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "whoami",
		Short: "Show the logged-in user",
		RunE: func(cmd *cobra.Command, args []string) error {
			raw, err := api().Raw("GET", "/api/v1/auth/me", nil)
			if err != nil {
				return err
			}
			return printJSON(raw)
		},
	}
}

func sessionsCmd() *cobra.Command {
	list := &cobra.Command{
		Use:   "sessions",
		Short: "List your sign-ins (API tokens) on the server",
		Long: `List your sign-ins: one per browser, CLI login or SSO sign-in. A token
expires after LOCCI_TOKEN_TTL_DAYS (default 30) without use.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			var out struct {
				Tokens []struct {
					ID       string `json:"id"`
					Name     string `json:"name"`
					Created  string `json:"created_at"`
					LastUsed string `json:"last_used_at"`
					Expires  string `json:"expires_at"`
					Current  bool   `json:"current"`
				} `json:"tokens"`
			}
			if err := api().Do("GET", "/api/v1/auth/tokens", nil, &out); err != nil {
				return err
			}
			if outputJSON {
				return printJSON(out.Tokens)
			}
			rows := make([][]string, len(out.Tokens))
			for i, t := range out.Tokens {
				id := t.ID
				if t.Current {
					id += " *"
				}
				expires := t.Expires
				if expires == "" {
					expires = "never"
				}
				name := t.Name
				if len(name) > 40 { // browsers name themselves with the whole User-Agent
					name = name[:37] + "..."
				}
				rows[i] = []string{id, name, t.Created, t.LastUsed, expires}
			}
			return table("ID\tNAME\tCREATED\tLAST USED\tEXPIRES", rows)
		},
	}
	list.AddCommand(&cobra.Command{
		Use:   "revoke ID...",
		Short: "Sign out sessions by ID (from locci sessions)",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			for _, id := range args {
				if err := api().Do("DELETE", "/api/v1/auth/tokens/"+id, nil, nil); err != nil {
					return fmt.Errorf("%s: %w", id, err)
				}
				fmt.Println("revoked", id)
			}
			return nil
		},
	})
	return list
}
