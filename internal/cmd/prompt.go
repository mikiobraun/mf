package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/mikiobraun/mf/internal/resolve"
	"github.com/mikiobraun/mf/internal/store"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

// promptSecret reads a secret from the terminal without echoing it. When
// stdin is not a terminal (a pipe, or CI) it reads one line instead, so
// `echo $KEY | mf endpoint add ...` works.
func promptSecret(cmd *cobra.Command, prompt string) (string, error) {
	if term.IsTerminal(int(os.Stdin.Fd())) {
		fmt.Fprint(cmd.ErrOrStderr(), prompt)
		b, err := term.ReadPassword(int(os.Stdin.Fd()))
		fmt.Fprintln(cmd.ErrOrStderr())
		if err != nil {
			return "", fmt.Errorf("reading key: %w", err)
		}
		return strings.TrimSpace(string(b)), nil
	}
	line, err := bufio.NewReader(cmd.InOrStdin()).ReadString('\n')
	if err != nil && line == "" {
		return "", nil // no input piped; treat as "no key"
	}
	return strings.TrimSpace(line), nil
}

// confirm asks a yes/no question. Without a terminal it refuses rather than
// assuming yes, so a piped invocation cannot silently destroy config.
func confirm(cmd *cobra.Command, question string) (bool, error) {
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		return false, fmt.Errorf("%s (not a terminal; pass --force to proceed)", question)
	}
	fmt.Fprintf(cmd.ErrOrStderr(), "%s [y/N] ", question)
	line, err := bufio.NewReader(os.Stdin).ReadString('\n')
	if err != nil {
		return false, nil
	}
	answer := strings.ToLower(strings.TrimSpace(line))
	return answer == "y" || answer == "yes", nil
}

// endpointKeyState reports where an endpoint's key comes from, for display.
func endpointKeyState(s *store.Store, endpoint string) (source string, ok bool) {
	if v := os.Getenv(resolve.KeyEnvVar(endpoint)); v != "" {
		return resolve.KeyEnvVar(endpoint), true
	}
	if _, found := resolve.EndpointKey(s, endpoint); found {
		return "models.secrets", true
	}
	return "", false
}
