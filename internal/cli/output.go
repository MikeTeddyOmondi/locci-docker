package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/MikeTeddyOmondi/locci-docker/internal/client"
)

// outputJSON is set by the global -o/--output flag.
var outputJSON bool

// envFlag is the global -E/--env flag; it beats LOCCI_ENV and the saved choice.
var envFlag string

func api() *client.Client {
	cfg := client.LoadConfig()
	if envFlag != "" {
		cfg.Env = envFlag
	}
	return client.New(cfg)
}

// printJSON pretty-prints any JSON value.
func printJSON(v any) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

// table prints rows with aligned columns.
func table(header string, rows [][]string) error {
	w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintln(w, header)
	for _, r := range rows {
		fmt.Fprintln(w, strings.Join(r, "\t"))
	}
	return w.Flush()
}

func short(id string) string {
	id = strings.TrimPrefix(id, "sha256:")
	if len(id) > 12 {
		return id[:12]
	}
	return id
}

func bytesHuman(n uint64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%dB", n)
	}
	div, exp := uint64(unit), 0
	for m := n / unit; m >= unit; m /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f%ciB", float64(n)/float64(div), "KMGTPE"[exp])
}
