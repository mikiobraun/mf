package cmd

import (
	"fmt"
	"strings"

	"github.com/mikiobraun/mf/registry"
	"github.com/spf13/cobra"
)

func newEndpointCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "endpoint",
		Short: "Manage endpoint entries",
	}
	c.AddCommand(newEndpointAddCmd(), newEndpointRemoveCmd(), newEndpointListCmd())
	return c
}

func newEndpointAddCmd() *cobra.Command {
	var key, api string
	help := argHelp{
		min: 2, max: 2,
		args: [][2]string{
			{"<name>", "name for the endpoint, used by endpoint/model addressing and `mf add`"},
			{"<url>", "the API base URL, including any version prefix the\nprovider uses (usually .../v1)"},
		},
		example: "endpoint add bifrost https://gw.example.com/v1",
	}
	c := &cobra.Command{
		Use:   "add <name> <url>",
		Short: "Add an endpoint, prompting for its API key",
		Args:  help.validator(),
		RunE: func(cmd *cobra.Command, args []string) error {
			name, url := args[0], args[1]
			if strings.Contains(name, "/") {
				return fmt.Errorf("endpoint name %q must not contain a slash; it would be ambiguous with endpoint/model addressing", name)
			}
			switch api {
			case registry.APIOpenAI, registry.APIAnthropic:
			default:
				return fmt.Errorf("--api must be %q or %q", registry.APIOpenAI, registry.APIAnthropic)
			}

			s, err := openStore()
			if err != nil {
				return err
			}
			if _, exists := s.Endpoint(name); exists {
				return fmt.Errorf("endpoint %q already exists; use `mf endpoint remove %s` first", name, name)
			}

			ep := registry.Endpoint{Name: name, URL: url}
			if api != registry.APIOpenAI {
				ep.API = api
			}
			s.PutEndpoint(ep)
			if err := s.Save(); err != nil {
				return err
			}

			if key == "" {
				key, err = promptSecret(cmd, fmt.Sprintf("API key for %s (leave blank for none): ", name))
				if err != nil {
					return err
				}
			}
			if key != "" {
				s.PutSecret(name, "", key)
				if err := s.SaveSecrets(); err != nil {
					return err
				}
			}
			fmt.Fprintf(cmd.OutOrStdout(), "added endpoint %s (%s)\n", name, url)
			if key == "" {
				fmt.Fprintf(cmd.ErrOrStderr(), "note: no API key stored for %s\n", name)
			}
			return nil
		},
	}
	c.Flags().StringVar(&key, "key", "", "API key (prompts if omitted)")
	c.Flags().StringVar(&api, "api", registry.APIOpenAI, "wire format: openai or anthropic")
	return c
}

func newEndpointRemoveCmd() *cobra.Command {
	var force bool
	c := &cobra.Command{
		Use:   "remove <name>",
		Short: "Remove an endpoint and all its models",
		Args: argHelp{
			min: 1, max: 1,
			args:    [][2]string{{"<name>", "endpoint to remove, along with its models and secrets"}},
			example: "endpoint remove bifrost",
		}.validator(),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			s, err := openStore()
			if err != nil {
				return err
			}
			if _, ok := s.Endpoint(name); !ok {
				return fmt.Errorf("no endpoint named %q (known endpoints: %s)", name, s.EndpointNames())
			}

			var affected []string
			for _, m := range s.Config.Models {
				if m.Endpoint == name {
					affected = append(affected, m.Name)
				}
			}
			if len(affected) > 0 && !force {
				ok, err := confirm(cmd, fmt.Sprintf("remove endpoint %s and its %d model(s) (%s)?",
					name, len(affected), strings.Join(affected, ", ")))
				if err != nil {
					return err
				}
				if !ok {
					fmt.Fprintln(cmd.ErrOrStderr(), "aborted")
					return nil
				}
			}

			models, _ := s.RemoveEndpoint(name)
			if err := s.Save(); err != nil {
				return err
			}
			if err := s.SaveSecrets(); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "removed endpoint %s (%d model(s))\n", name, models)
			return nil
		},
	}
	c.Flags().BoolVarP(&force, "force", "f", false, "do not prompt before removing models")
	return c
}

func newEndpointListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List configured endpoints",
		Args: argHelp{
			min: 0, max: 0,
			example: "endpoint list",
		}.validator(),
		RunE: func(cmd *cobra.Command, _ []string) error {
			s, err := openStore()
			if err != nil {
				return err
			}
			counts := map[string]int{}
			for _, m := range s.Config.Models {
				counts[m.Endpoint]++
			}
			w := newTabWriter(cmd.OutOrStdout())
			fmt.Fprintln(w, "NAME\tURL\tAPI\tMODELS\tKEY")
			for _, e := range s.Config.Endpoints {
				keyState := "-"
				if _, ok := endpointKeyState(s, e.Name); ok {
					keyState = "set"
				}
				fmt.Fprintf(w, "%s\t%s\t%s\t%d\t%s\n", e.Name, e.URL, e.Dialect(), counts[e.Name], keyState)
			}
			if err := w.Flush(); err != nil {
				return err
			}
			if len(s.Config.Endpoints) == 0 {
				fmt.Fprintln(cmd.ErrOrStderr(), "no endpoints configured; add one with `mf endpoint add <name> <url>`")
			}
			return nil
		},
	}
}
