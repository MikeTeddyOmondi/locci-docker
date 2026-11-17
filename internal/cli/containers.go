package cli

import (
	"fmt"
	"io"
	"net/url"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/MikeTeddyOmondi/locci-docker/internal/docker"
)

type containerRow struct {
	ID     string   `json:"Id"`
	Names  []string `json:"Names"`
	Image  string   `json:"Image"`
	State  string   `json:"State"`
	Status string   `json:"Status"`
}

func psCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "ps",
		Aliases: []string{"containers"},
		Short:   "List containers",
		RunE: func(cmd *cobra.Command, args []string) error {
			var out struct {
				Containers []containerRow `json:"containers"`
			}
			if err := api().Do("GET", "/api/v1/containers", nil, &out); err != nil {
				return err
			}
			if outputJSON {
				return printJSON(out.Containers)
			}
			rows := [][]string{}
			for _, c := range out.Containers {
				name := ""
				if len(c.Names) > 0 {
					name = strings.TrimPrefix(c.Names[0], "/")
				}
				rows = append(rows, []string{short(c.ID), name, c.Image, c.State, c.Status})
			}
			return table("ID\tNAME\tIMAGE\tSTATE\tSTATUS", rows)
		},
	}
}

func runCmd() *cobra.Command {
	var spec docker.ContainerSpec
	var ports, labels []string
	var noStart bool

	cmd := &cobra.Command{
		Use:     "run [flags] IMAGE [COMMAND...]",
		Short:   "Create and start a container routed at <subdomain>.<domain>",
		Example: `  locci run nginx:alpine --name web --subdomain web -p 80:8080 --network proxy-network`,
		Args:    cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			spec.Image, spec.Command = args[0], args[1:]
			if spec.Name == "" {
				return fmt.Errorf("--name is required")
			}
			if spec.Subdomain == "" {
				spec.Subdomain = spec.Name
			}

			spec.Ports = map[string]string{}
			for _, p := range ports { // CONTAINER:HOST, matching the API's map
				cport, hport, _ := strings.Cut(p, ":")
				spec.Ports[cport] = hport
			}
			spec.Labels = map[string]string{}
			for _, l := range labels {
				k, v, _ := strings.Cut(l, "=")
				spec.Labels[k] = v
			}

			var created docker.Created
			if err := api().Do("POST", "/api/v1/containers", spec, &created); err != nil {
				return err
			}
			if !noStart {
				if err := api().Do("POST", "/api/v1/containers/"+created.ID+"/start", nil, nil); err != nil {
					return err
				}
			}
			if outputJSON {
				return printJSON(created)
			}
			fmt.Printf("%s  %s  https://%s\n", short(created.ID), created.Name, created.Domain)
			return nil
		},
	}
	f := cmd.Flags()
	f.StringVar(&spec.Name, "name", "", "container name (required)")
	f.StringVar(&spec.Subdomain, "subdomain", "", "Traefik subdomain (defaults to --name)")
	f.StringArrayVarP(&ports, "publish", "p", nil, "CONTAINER_PORT:HOST_PORT (repeatable)")
	f.StringArrayVarP(&spec.Environment, "env", "e", nil, "KEY=VALUE (repeatable)")
	f.StringArrayVar(&spec.Networks, "network", nil, "network to join (repeatable)")
	f.StringArrayVarP(&spec.Volumes, "volume", "v", nil, "volume or bind mount, e.g. data:/data (repeatable)")
	f.StringArrayVarP(&labels, "label", "l", nil, "KEY=VALUE label (repeatable)")
	f.BoolVar(&noStart, "no-start", false, "create without starting")
	return cmd
}

// actionCmd builds start/stop/restart/kill/pause/unpause.
func actionCmd(action string) *cobra.Command {
	return &cobra.Command{
		Use:   action + " CONTAINER...",
		Short: strings.ToUpper(action[:1]) + action[1:] + " one or more containers",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			for _, id := range args {
				if err := api().Do("POST", "/api/v1/containers/"+url.PathEscape(id)+"/"+action, nil, nil); err != nil {
					return fmt.Errorf("%s: %w", id, err)
				}
				fmt.Println(id)
			}
			return nil
		},
	}
}

func rmCmd() *cobra.Command {
	var force bool
	cmd := &cobra.Command{
		Use:   "rm CONTAINER...",
		Short: "Remove containers",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			for _, id := range args {
				path := fmt.Sprintf("/api/v1/containers/%s?force=%t", url.PathEscape(id), force)
				if err := api().Do("DELETE", path, nil, nil); err != nil {
					return fmt.Errorf("%s: %w", id, err)
				}
				fmt.Println(id)
			}
			return nil
		},
	}
	cmd.Flags().BoolVarP(&force, "force", "f", false, "stop and remove a running container")
	return cmd
}

func logsCmd() *cobra.Command {
	var follow bool
	var tail string
	cmd := &cobra.Command{
		Use:   "logs CONTAINER",
		Short: "Print container logs",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			path := fmt.Sprintf("/api/v1/containers/%s/logs?follow=%t&tail=%s",
				url.PathEscape(args[0]), follow, url.QueryEscape(tail))
			body, err := api().Stream(path)
			if err != nil {
				return err
			}
			defer body.Close()
			_, err = io.Copy(os.Stdout, body)
			return err
		},
	}
	cmd.Flags().BoolVarP(&follow, "follow", "f", false, "follow log output")
	cmd.Flags().StringVar(&tail, "tail", "200", `number of lines, or "all"`)
	return cmd
}

func inspectCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "inspect CONTAINER",
		Short: "Show container details as JSON",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			raw, err := api().Raw("GET", "/api/v1/containers/"+url.PathEscape(args[0]), nil)
			if err != nil {
				return err
			}
			return printJSON(raw)
		},
	}
}

func statsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "stats CONTAINER",
		Short: "Show a resource usage snapshot",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			var st docker.Stats
			if err := api().Do("GET", "/api/v1/containers/"+url.PathEscape(args[0])+"/stats", nil, &st); err != nil {
				return err
			}
			if outputJSON {
				return printJSON(st)
			}
			return table("CPU %\tMEM USAGE / LIMIT\tMEM %\tNET RX / TX\tPIDS", [][]string{{
				fmt.Sprintf("%.2f%%", st.CPUPercent),
				bytesHuman(st.MemUsage) + " / " + bytesHuman(st.MemLimit),
				fmt.Sprintf("%.2f%%", st.MemPercent),
				bytesHuman(st.NetRx) + " / " + bytesHuman(st.NetTx),
				fmt.Sprint(st.PIDs),
			}})
		},
	}
}

func execCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "exec CONTAINER -- COMMAND [ARGS...]",
		Short: "Run a command in a container and print its output",
		Example: `  locci exec web -- cat /etc/os-release
  (for an interactive shell, use the terminal in the web UI)`,
		Args: cobra.MinimumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			var res docker.ExecResult
			body := map[string][]string{"cmd": args[1:]}
			if err := api().Do("POST", "/api/v1/containers/"+url.PathEscape(args[0])+"/exec", body, &res); err != nil {
				return err
			}
			fmt.Print(res.Output)
			if res.ExitCode != 0 {
				os.Exit(res.ExitCode)
			}
			return nil
		},
	}
}
