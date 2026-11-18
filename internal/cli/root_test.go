package cli

import (
	"testing"

	"github.com/spf13/cobra"
)

// Every command must accept the global -E/--environment flag; a local flag
// with the same name would silently shadow it (as `run --env` once did).
func TestEnvironmentFlagEverywhere(t *testing.T) {
	var walk func(c *cobra.Command)
	walk = func(c *cobra.Command) {
		if c.HasParent() {
			if c.InheritedFlags().Lookup("environment") == nil || c.InheritedFlags().ShorthandLookup("E") == nil {
				t.Errorf("%q does not inherit -E/--environment", c.CommandPath())
			}
		}
		for _, sub := range c.Commands() {
			walk(sub)
		}
	}
	walk(newRoot())
}
