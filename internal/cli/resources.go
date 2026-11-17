package cli

import (
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

// ---- images ----

func imageCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "image", Aliases: []string{"images"}, Short: "Manage images"}

	ls := &cobra.Command{
		Use:   "ls",
		Short: "List images",
		RunE: func(cmd *cobra.Command, args []string) error {
			var out struct {
				Images []struct {
					ID       string   `json:"Id"`
					RepoTags []string `json:"RepoTags"`
					Size     int64    `json:"Size"`
					Created  int64    `json:"Created"`
				} `json:"images"`
			}
			if err := api().Do("GET", "/api/v1/images", nil, &out); err != nil {
				return err
			}
			if outputJSON {
				return printJSON(out.Images)
			}
			rows := [][]string{}
			for _, im := range out.Images {
				tags := strings.Join(im.RepoTags, ", ")
				if tags == "" {
					tags = "<none>"
				}
				rows = append(rows, []string{short(im.ID), tags, bytesHuman(uint64(im.Size)),
					time.Unix(im.Created, 0).Format("2006-01-02")})
			}
			return table("ID\tTAGS\tSIZE\tCREATED", rows)
		},
	}

	pull := &cobra.Command{
		Use:   "pull IMAGE",
		Short: "Pull an image",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := api().Do("POST", "/api/v1/images/pull", map[string]string{"image": args[0]}, nil); err != nil {
				return err
			}
			fmt.Println("pulled", args[0])
			return nil
		},
	}

	var force bool
	rm := &cobra.Command{
		Use:   "rm IMAGE...",
		Short: "Remove images",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			for _, id := range args {
				path := fmt.Sprintf("/api/v1/images/%s?force=%t", url.PathEscape(id), force)
				if err := api().Do("DELETE", path, nil, nil); err != nil {
					return fmt.Errorf("%s: %w", id, err)
				}
				fmt.Println(id)
			}
			return nil
		},
	}
	rm.Flags().BoolVarP(&force, "force", "f", false, "force removal")

	prune := &cobra.Command{
		Use:   "prune",
		Short: "Remove dangling images",
		RunE: func(cmd *cobra.Command, args []string) error {
			raw, err := api().Raw("POST", "/api/v1/images/prune", nil)
			if err != nil {
				return err
			}
			return printJSON(raw)
		},
	}

	cmd.AddCommand(ls, pull, rm, prune)
	return cmd
}

// ---- networks ----

func networkCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "network", Aliases: []string{"networks"}, Short: "Manage networks"}

	ls := &cobra.Command{
		Use:   "ls",
		Short: "List networks",
		RunE: func(cmd *cobra.Command, args []string) error {
			var out struct {
				Networks []struct {
					ID     string `json:"Id"`
					Name   string `json:"Name"`
					Driver string `json:"Driver"`
					Scope  string `json:"Scope"`
				} `json:"networks"`
			}
			if err := api().Do("GET", "/api/v1/networks", nil, &out); err != nil {
				return err
			}
			if outputJSON {
				return printJSON(out.Networks)
			}
			rows := [][]string{}
			for _, n := range out.Networks {
				rows = append(rows, []string{short(n.ID), n.Name, n.Driver, n.Scope})
			}
			return table("ID\tNAME\tDRIVER\tSCOPE", rows)
		},
	}

	var driver string
	create := &cobra.Command{
		Use:   "create NAME",
		Short: "Create a network",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			var out struct {
				ID string `json:"network_id"`
			}
			body := map[string]string{"name": args[0], "driver": driver}
			if err := api().Do("POST", "/api/v1/networks", body, &out); err != nil {
				return err
			}
			fmt.Println(short(out.ID))
			return nil
		},
	}
	create.Flags().StringVarP(&driver, "driver", "d", "bridge", "network driver")

	rm := &cobra.Command{
		Use:   "rm NETWORK...",
		Short: "Remove networks",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			for _, id := range args {
				if err := api().Do("DELETE", "/api/v1/networks/"+url.PathEscape(id), nil, nil); err != nil {
					return fmt.Errorf("%s: %w", id, err)
				}
				fmt.Println(id)
			}
			return nil
		},
	}

	cmd.AddCommand(ls, create, rm)
	return cmd
}

// ---- volumes ----

func volumeCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "volume", Aliases: []string{"volumes"}, Short: "Manage volumes"}

	ls := &cobra.Command{
		Use:   "ls",
		Short: "List volumes",
		RunE: func(cmd *cobra.Command, args []string) error {
			var out struct {
				Volumes []struct {
					Name       string `json:"Name"`
					Driver     string `json:"Driver"`
					Mountpoint string `json:"Mountpoint"`
				} `json:"volumes"`
			}
			if err := api().Do("GET", "/api/v1/volumes", nil, &out); err != nil {
				return err
			}
			if outputJSON {
				return printJSON(out.Volumes)
			}
			rows := [][]string{}
			for _, v := range out.Volumes {
				rows = append(rows, []string{v.Name, v.Driver, v.Mountpoint})
			}
			return table("NAME\tDRIVER\tMOUNTPOINT", rows)
		},
	}

	create := &cobra.Command{
		Use:   "create NAME",
		Short: "Create a volume",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := api().Do("POST", "/api/v1/volumes", map[string]string{"name": args[0]}, nil); err != nil {
				return err
			}
			fmt.Println(args[0])
			return nil
		},
	}

	var force bool
	rm := &cobra.Command{
		Use:   "rm VOLUME...",
		Short: "Remove volumes",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			for _, name := range args {
				path := fmt.Sprintf("/api/v1/volumes/%s?force=%t", url.PathEscape(name), force)
				if err := api().Do("DELETE", path, nil, nil); err != nil {
					return fmt.Errorf("%s: %w", name, err)
				}
				fmt.Println(name)
			}
			return nil
		},
	}
	rm.Flags().BoolVarP(&force, "force", "f", false, "force removal")

	cmd.AddCommand(ls, create, rm)
	return cmd
}

// ---- system / admin ----

func infoCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "info",
		Short: "Show Docker host information",
		RunE: func(cmd *cobra.Command, args []string) error {
			var info struct {
				Name              string `json:"Name"`
				ServerVersion     string `json:"ServerVersion"`
				OperatingSystem   string `json:"OperatingSystem"`
				Architecture      string `json:"Architecture"`
				NCPU              int    `json:"NCPU"`
				MemTotal          uint64 `json:"MemTotal"`
				Containers        int    `json:"Containers"`
				ContainersRunning int    `json:"ContainersRunning"`
				Images            int    `json:"Images"`
			}
			if outputJSON {
				raw, err := api().Raw("GET", "/api/v1/system/info", nil)
				if err != nil {
					return err
				}
				return printJSON(raw)
			}
			if err := api().Do("GET", "/api/v1/system/info", nil, &info); err != nil {
				return err
			}
			return table("HOST\tDOCKER\tOS\tARCH\tCPUS\tMEMORY\tCONTAINERS\tIMAGES", [][]string{{
				info.Name, info.ServerVersion, info.OperatingSystem, info.Architecture,
				fmt.Sprint(info.NCPU), bytesHuman(info.MemTotal),
				fmt.Sprintf("%d (%d running)", info.Containers, info.ContainersRunning),
				fmt.Sprint(info.Images),
			}})
		},
	}
}

func auditCmd() *cobra.Command {
	var limit int
	cmd := &cobra.Command{
		Use:   "audit",
		Short: "Show recent changes made through the API (admin)",
		RunE: func(cmd *cobra.Command, args []string) error {
			var out struct {
				Audit []struct {
					At       string `json:"at"`
					Username string `json:"username"`
					Method   string `json:"method"`
					Path     string `json:"path"`
					Status   int    `json:"status"`
				} `json:"audit"`
			}
			if err := api().Do("GET", fmt.Sprintf("/api/v1/audit?limit=%d", limit), nil, &out); err != nil {
				return err
			}
			if outputJSON {
				return printJSON(out.Audit)
			}
			rows := [][]string{}
			for _, a := range out.Audit {
				rows = append(rows, []string{a.At, a.Username, a.Method, a.Path, fmt.Sprint(a.Status)})
			}
			return table("TIME\tUSER\tMETHOD\tPATH\tSTATUS", rows)
		},
	}
	cmd.Flags().IntVarP(&limit, "limit", "n", 50, "number of entries")
	return cmd
}
