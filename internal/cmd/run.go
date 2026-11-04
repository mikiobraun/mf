package cmd

import (
	"github.com/spf13/cobra"
)

// newRunCmd groups one subcommand per client. Each client takes its main
// model as a positional, further models to offer as more positionals, and its
// own optional roles as flags.
func newRunCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "run",
		Short: "Launch a client configured with models from the registry",
	}
	c.AddCommand(newRunOpencodeCmd())
	return c
}

// splitAtDash separates the model addresses from the arguments passed through
// to the client after a literal --.
func splitAtDash(c *cobra.Command, args []string) (models, rest []string) {
	if dash := c.ArgsLenAtDash(); dash >= 0 {
		return args[:dash], args[dash:]
	}
	return args, nil
}
