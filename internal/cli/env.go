package cli

import (
	"errors"
	"fmt"
	"net/url"
	"os"

	"github.com/spf13/cobra"

	"github.com/MikeTeddyOmondi/locci-docker/internal/client"
)

// printAgentSetup shows a new agent token (once) and how to start the agent.
func printAgentSetup(server, token string) {
	fmt.Printf("\nagent token (shown once): %s\n\non the Docker host, run:\n", token)
	fmt.Printf("  LOCCI_AGENT_TOKEN=%s locci agent --server %s\n", token, server)
	fmt.Printf("or in Docker:\n  docker run -d --name locci-agent --restart unless-stopped \\\n")
	fmt.Printf("    -v /var/run/docker.sock:/var/run/docker.sock -e LOCCI_AGENT_TOKEN=%s \\\n", token)
	fmt.Printf("    <locci image> agent --server %s\n", server)
}

type envInfo struct {
	Name           string `json:"name"`
	Label          string `json:"label"`
	Kind           string `json:"kind"`
	URL            string `json:"url"`
	Status         string `json:"status"`
	IsDefault      bool   `json:"is_default"`
	LastSeenAt     string `json:"last_seen_at"`
	HasSecret      bool   `json:"has_secret"`
	Fingerprint    string `json:"ssh_fingerprint"`
	AgentConnected *bool  `json:"agent_connected"`
	AgentToken     string `json:"agent_token"`
}

type hostInfo struct {
	Name          string `json:"name"`
	ServerVersion string `json:"server_version"`
	OS            string `json:"os"`
	Arch          string `json:"arch"`
	CPUs          int    `json:"cpus"`
	Memory        uint64 `json:"memory"`
	Containers    int    `json:"containers"`
	SSHHostKey    string `json:"ssh_host_key"`
	Fingerprint   string `json:"ssh_fingerprint"`
}

func envCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "env",
		Aliases: []string{"environment", "environments"},
		Short:   "Manage environments (Docker hosts) and choose which one commands use",
		Example: `  locci env ls
  locci env add edge --kind ssh --url ssh://deploy@10.0.0.5 --ssh-key ~/.ssh/locci_deploy
  locci env add lab --kind tcp+tls --url tcp://10.0.0.6:2376 --tls-ca ca.pem --tls-cert cert.pem --tls-key key.pem
  locci env use edge          # later commands run against edge
  locci ps -E local           # one command against another environment`,
	}
	cmd.AddCommand(envLsCmd(), envAddCmd(), envEditCmd(), envRmCmd(), envTestCmd(), envUseCmd(), envDefaultCmd())
	return cmd
}

func envLsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "ls",
		Short: "List environments (* marks the one commands use)",
		RunE: func(cmd *cobra.Command, args []string) error {
			c := api()
			var out struct {
				Environments []envInfo `json:"environments"`
			}
			if err := c.Do("GET", "/api/v1/environments", nil, &out); err != nil {
				return err
			}
			if outputJSON {
				return printJSON(out.Environments)
			}
			rows := [][]string{}
			for _, e := range out.Environments {
				mark := " "
				if (c.Env == "" && e.IsDefault) || c.Env == e.Name {
					mark = "*"
				}
				def := ""
				if e.IsDefault {
					def = "yes"
				}
				u := e.URL
				if u == "" {
					u = "(local socket)"
				}
				rows = append(rows, []string{mark, e.Name, e.Kind, u, e.Status, def, e.LastSeenAt})
			}
			return table(" \tNAME\tKIND\tURL\tSTATUS\tDEFAULT\tLAST SEEN", rows)
		},
	}
}

func readFileFlag(path string) (string, error) {
	if path == "" {
		return "", nil
	}
	b, err := os.ReadFile(path)
	return string(b), err
}

func envAddCmd() *cobra.Command {
	var kind, rawURL, label, tlsCA, tlsCert, tlsKey, sshKey string
	var noTest bool
	cmd := &cobra.Command{
		Use:   "add NAME",
		Short: "Add an environment (admin); tests the connection first",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			body := map[string]any{"name": args[0], "kind": kind, "url": rawURL, "label": label}
			for field, path := range map[string]string{"tls_ca": tlsCA, "tls_cert": tlsCert, "tls_key": tlsKey, "ssh_key": sshKey} {
				v, err := readFileFlag(path)
				if err != nil {
					return err
				}
				if v != "" {
					body[field] = v
				}
			}

			c := api()
			if !noTest && kind != "agent" { // an agent can only be seen once it connects
				var res struct {
					OK    bool     `json:"ok"`
					Error string   `json:"error"`
					Host  hostInfo `json:"host"`
				}
				if err := c.Do("POST", "/api/v1/environments/test", body, &res); err != nil {
					return err
				}
				if !res.OK {
					return fmt.Errorf("connection test failed: %s (use --no-test to save anyway)", res.Error)
				}
				printHost(res.Host)
				// Pin exactly the key that was just shown.
				if res.Host.SSHHostKey != "" {
					body["ssh_host_key"] = res.Host.SSHHostKey
				}
			}

			var saved envInfo
			if err := c.Do("POST", "/api/v1/environments", body, &saved); err != nil {
				return err
			}
			if outputJSON {
				return printJSON(saved)
			}
			fmt.Printf("added %s (%s). Use it with `locci env use %s` or `-E %s`.\n", saved.Name, saved.Kind, saved.Name, saved.Name)
			if saved.AgentToken != "" {
				printAgentSetup(c.URL, saved.AgentToken)
			}
			return nil
		},
	}
	f := cmd.Flags()
	f.StringVar(&kind, "kind", "ssh", "connection kind: local, tcp+tls, ssh or agent (the host dials out with locci agent)")
	f.StringVar(&rawURL, "url", "", "tcp://host:2376 (tcp+tls), ssh://user@host[:port][/socket] (ssh); empty for local means DOCKER_HOST or the local socket")
	f.StringVar(&label, "label", "", "display name (default: NAME)")
	f.StringVar(&tlsCA, "tls-ca", "", "CA certificate file (tcp+tls)")
	f.StringVar(&tlsCert, "tls-cert", "", "client certificate file (tcp+tls)")
	f.StringVar(&tlsKey, "tls-key", "", "client key file (tcp+tls), stored encrypted")
	f.StringVar(&sshKey, "ssh-key", "", "unencrypted private key file (ssh), stored encrypted")
	f.BoolVar(&noTest, "no-test", false, "save without testing the connection first")
	return cmd
}

func envEditCmd() *cobra.Command {
	var label, rawURL, tlsCA, tlsCert, tlsKey, sshKey string
	var resetHostKey, noTest, rotate bool
	cmd := &cobra.Command{
		Use:   "edit NAME",
		Short: "Change an environment (admin); only the flags given change, keys stay unless replaced",
		Example: `  locci env edit edge --label "Edge box"
  locci env edit edge --url ssh://deploy@10.0.0.9     # new host: its key is pinned again
  locci env edit edge --reset-host-key                # the host was rebuilt on purpose`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c := api()
			path := "/api/v1/environments/" + url.PathEscape(args[0])
			var cur envInfo
			if err := c.Do("GET", path, nil, &cur); err != nil {
				return err
			}

			body := map[string]any{"name": cur.Name, "kind": cur.Kind}
			if label != "" {
				body["label"] = label
			}
			body["url"] = cur.URL
			if rawURL != "" {
				body["url"] = rawURL
			}
			for field, p := range map[string]string{"tls_ca": tlsCA, "tls_cert": tlsCert, "tls_key": tlsKey, "ssh_key": sshKey} {
				v, err := readFileFlag(p)
				if err != nil {
					return err
				}
				if v != "" {
					body[field] = v
				}
			}
			if resetHostKey {
				body["reset_host_key"] = true
			}
			if rotate {
				body["rotate_agent_token"] = true
			}

			// Test the merged settings; blank keys are taken from the stored environment.
			if !noTest && cur.Kind != "agent" {
				var res struct {
					OK    bool     `json:"ok"`
					Error string   `json:"error"`
					Host  hostInfo `json:"host"`
				}
				if err := c.Do("POST", "/api/v1/environments/test", body, &res); err != nil {
					return err
				}
				if !res.OK {
					return fmt.Errorf("connection test failed: %s (use --no-test to save anyway)", res.Error)
				}
				printHost(res.Host)
				if res.Host.SSHHostKey != "" {
					body["ssh_host_key"] = res.Host.SSHHostKey
				}
			}

			var saved envInfo
			if err := c.Do("PATCH", path, body, &saved); err != nil {
				return err
			}
			if outputJSON {
				return printJSON(saved)
			}
			fmt.Printf("saved %s (%s)\n", saved.Name, saved.Label)
			if saved.AgentToken != "" {
				fmt.Println("the agent using the old token was disconnected")
				printAgentSetup(c.URL, saved.AgentToken)
			}
			return nil
		},
	}
	f := cmd.Flags()
	f.StringVar(&label, "label", "", "new display name")
	f.StringVar(&rawURL, "url", "", "new URL (for ssh this also re-pins the host key)")
	f.StringVar(&tlsCA, "tls-ca", "", "replace the CA certificate (file)")
	f.StringVar(&tlsCert, "tls-cert", "", "replace the client certificate (file)")
	f.StringVar(&tlsKey, "tls-key", "", "replace the client key (file)")
	f.StringVar(&sshKey, "ssh-key", "", "replace the SSH private key (file)")
	f.BoolVar(&resetHostKey, "reset-host-key", false, "forget the pinned SSH host key and pin the one seen now")
	f.BoolVar(&rotate, "rotate-agent-token", false, "agent environments: issue a new token (disconnects the running agent)")
	f.BoolVar(&noTest, "no-test", false, "save without testing the connection first")
	return cmd
}

func printHost(h hostInfo) {
	if outputJSON {
		return
	}
	fmt.Printf("connected: %s, Docker %s, %s %s, %d CPUs, %s memory, %d containers\n",
		h.Name, h.ServerVersion, h.OS, h.Arch, h.CPUs, bytesHuman(h.Memory), h.Containers)
	if h.Fingerprint != "" {
		fmt.Printf("ssh host key: %s (pinned when saved)\n", h.Fingerprint)
	}
}

func envRmCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "rm NAME",
		Short: "Remove an environment (admin); its containers are not touched",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := api().Do("DELETE", "/api/v1/environments/"+url.PathEscape(args[0]), nil, nil); err != nil {
				return err
			}
			fmt.Println("removed", args[0])
			return nil
		},
	}
}

func envTestCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "test [NAME]",
		Short: "Check that a saved environment is reachable (default: the current one)",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c := api()
			name := c.Env
			if len(args) == 1 {
				name = args[0]
			}
			path := "/api/v1/system/info" // current or default environment
			if name != "" {
				path = "/api/v1/environments/" + url.PathEscape(name) + "/info"
				c.Env = "" // already scoped
			}
			var info struct {
				Name            string `json:"Name"`
				ServerVersion   string `json:"ServerVersion"`
				OperatingSystem string `json:"OperatingSystem"`
				Architecture    string `json:"Architecture"`
				NCPU            int    `json:"NCPU"`
				MemTotal        uint64 `json:"MemTotal"`
				Containers      int    `json:"Containers"`
			}
			if err := c.Do("GET", path, nil, &info); err != nil {
				return err
			}
			if outputJSON {
				return printJSON(info)
			}
			printHost(hostInfo{Name: info.Name, ServerVersion: info.ServerVersion, OS: info.OperatingSystem,
				Arch: info.Architecture, CPUs: info.NCPU, Memory: info.MemTotal, Containers: info.Containers})
			return nil
		},
	}
}

func envUseCmd() *cobra.Command {
	var clear bool
	cmd := &cobra.Command{
		Use:   "use NAME",
		Short: "Make later commands use this environment (saved in the CLI config)",
		Args: func(cmd *cobra.Command, args []string) error {
			if clear != (len(args) == 0) {
				return errors.New("give an environment NAME, or --clear to follow the server default")
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg := client.LoadSavedConfig()
			if clear {
				cfg.Env = ""
				if err := client.SaveConfig(cfg); err != nil {
					return err
				}
				fmt.Println("commands now use the server's default environment")
				return nil
			}
			var e envInfo
			if err := api().Do("GET", "/api/v1/environments/"+url.PathEscape(args[0]), nil, &e); err != nil {
				return err
			}
			cfg.Env = e.Name
			if err := client.SaveConfig(cfg); err != nil {
				return err
			}
			fmt.Printf("commands now use %s (%s)\n", e.Name, e.Kind)
			if os.Getenv("LOCCI_ENV") != "" {
				fmt.Fprintln(os.Stderr, "note: LOCCI_ENV is set and overrides this choice")
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&clear, "clear", false, "forget the saved choice and follow the server default")
	return cmd
}

func envDefaultCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "default NAME",
		Short: "Make NAME the server's default environment (admin); the original API routes use it",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := api().Do("POST", "/api/v1/environments/"+url.PathEscape(args[0])+"/default", nil, nil); err != nil {
				return err
			}
			fmt.Println(args[0], "is now the default environment")
			return nil
		},
	}
}
