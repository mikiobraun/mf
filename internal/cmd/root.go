// Package cmd implements the mf command line.
package cmd

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/mikiobraun/mf/registry"
	"github.com/spf13/cobra"
)

// configDir is set by the --config flag; empty means use the default.
var configDir string

// NewRootCmd builds the mf command tree.
func NewRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "mf",
		Short: "Manage, configure, and test LLM model configurations",
		Long: "mf manages a local, file-based registry of LLM model configurations so\n" +
			"that tools can read a single source of truth.\n\n" +
			"Models are addressed by alias (fast), by endpoint and model ID\n" +
			"(anthropic/claude-opus-4-6), or by tag (@coding).",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.PersistentFlags().StringVar(&configDir, "config", "",
		"config directory (default $MF_CONFIG_DIR, else ~/.config/mf)")

	root.AddCommand(
		newListCmd(),
		newEndpointCmd(),
		newAddCmd(),
		newRemoveCmd(),
		newGetCmd(),
		newSetCmd(),
		newModelsCmd(),
		newSyncCmd(),
		newCheckCmd(),
		newAskCmd(),
		newExecCmd(),
		newRunCmd(),
	)
	return root
}

// openStore loads the registry from the configured directory.
func openStore() (*registry.Store, error) {
	dir := configDir
	if dir == "" {
		var err error
		if dir, err = registry.DefaultDir(); err != nil {
			return nil, err
		}
	}
	return registry.Open(dir)
}

// mustEndpoint looks up an endpoint, with a message listing the known ones.
func mustEndpoint(s *registry.Store, name string) (registry.Endpoint, error) {
	ep, ok := s.Endpoint(name)
	if !ok {
		return registry.Endpoint{}, fmt.Errorf("no endpoint named %q (known endpoints: %s)", name, s.EndpointNames())
	}
	return ep, nil
}

// newTabWriter returns a writer producing the aligned columns used by list
// and check output.
func newTabWriter(w io.Writer) *tabwriter.Writer {
	return tabwriter.NewWriter(w, 0, 0, 4, ' ', 0)
}

// splitTags parses a comma-separated --tags value, dropping empties.
func splitTags(v string) []string {
	if strings.TrimSpace(v) == "" {
		return nil
	}
	parts := strings.Split(v, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if t := strings.TrimSpace(p); t != "" {
			out = append(out, t)
		}
	}
	return out
}

// warnAmbiguousTag notes on stderr when an @tag address matched more than one
// entry, so scripts still get a clean stdout.
func warnAmbiguousTag(addr string, others []string) {
	if len(others) == 0 {
		return
	}
	tag := strings.TrimPrefix(addr, "@")
	fmt.Fprintf(os.Stderr, "note: %s also matches %s; use `mf list --tags %s` to see all\n",
		addr, strings.Join(others, ", "), tag)
}

// errSilent signals "already reported, exit non-zero" — used by check, which
// prints its own per-model failure lines.
var errSilent = errors.New("")
