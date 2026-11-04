package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"

	"github.com/mikiobraun/mf/registry"
	"github.com/spf13/cobra"
)

// execve replaces the mf process with the command. It is a package variable
// so tests can capture what would have run instead of being replaced.
var execve = syscall.Exec

func newExecCmd() *cobra.Command {
	var prefix string
	help := argHelp{
		min: 2, max: -1,
		args: [][2]string{
			{"<name>", addrArg},
			{"-- <cmd> [<args>...]", "the command to run, after a literal --"},
		},
		example: "exec fast -- ofc run",
	}
	c := &cobra.Command{
		Use:   "exec <name> -- <cmd> [<args>...]",
		Short: "Run a command with a model's settings in its environment",
		Long: "Run a command with a model's endpoint URL, model ID and key in its\n" +
			"environment, replacing the mf process:\n\n" +
			"  OPENAI_BASE_URL   the endpoint URL\n" +
			"  OPENAI_MODEL      the model ID\n" +
			"  OPENAI_API_KEY    the key; removed from the environment when none is\n" +
			"                    configured, so an inherited key cannot leak\n\n" +
			"--prefix replaces OPENAI_ for tools that read their own names:\n" +
			"--prefix SMOKE_ sets SMOKE_BASE_URL, SMOKE_MODEL and SMOKE_API_KEY.",
		Args: func(c *cobra.Command, args []string) error {
			if err := help.validator()(c, args); err != nil {
				return err
			}
			if c.ArgsLenAtDash() != 1 {
				return fmt.Errorf("exec takes one model name, then -- and the command\n%s", help.details(c))
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			addr, argv := args[0], args[1:]
			s, err := openStore()
			if err != nil {
				return err
			}
			ref, err := registry.Resolve(s, addr)
			if err != nil {
				return err
			}
			warnAmbiguousTag(addr, ref.TagMatches)
			if d := ref.Endpoint.Dialect(); d != registry.APIOpenAI {
				return fmt.Errorf("endpoint %s uses api %q; exec only sets OpenAI-style variables", ref.Endpoint.Name, d)
			}

			path, err := exec.LookPath(argv[0])
			if err != nil {
				return err
			}
			key, hasKey := ref.Key(s)
			env := execEnv(os.Environ(), prefix, ref.Endpoint.URL, ref.ModelID, key, hasKey)
			return execve(path, argv, env)
		},
	}
	c.Flags().StringVar(&prefix, "prefix", "OPENAI_", "variable name prefix, replacing OPENAI_")
	return c
}

// execEnv returns environ with the three model variables set. Any inherited
// value for them is dropped first, and the key variable stays unset when no
// key is configured.
func execEnv(environ []string, prefix, url, model, key string, hasKey bool) []string {
	env := withoutEnv(environ, prefix+"API_KEY")
	env = withEnv(env, prefix+"BASE_URL", url)
	env = withEnv(env, prefix+"MODEL", model)
	if hasKey {
		env = withEnv(env, prefix+"API_KEY", key)
	}
	return env
}

// withEnv returns environ with name set to value, replacing any inherited
// value rather than adding a second entry.
func withEnv(environ []string, name, value string) []string {
	return append(withoutEnv(environ, name), name+"="+value)
}

// withoutEnv returns a copy of environ with every entry for name removed.
func withoutEnv(environ []string, name string) []string {
	out := make([]string, 0, len(environ)+1)
	for _, kv := range environ {
		if n, _, _ := strings.Cut(kv, "="); n != name {
			out = append(out, kv)
		}
	}
	return out
}
