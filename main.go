// Command mf manages a local registry of LLM model configurations.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"

	"github.com/mikiobraun/mf/internal/cmd"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	if err := cmd.NewRootCmd().ExecuteContext(ctx); err != nil {
		if msg := err.Error(); msg != "" {
			fmt.Fprintln(os.Stderr, "mf: "+msg)
		}
		os.Exit(1)
	}
}
