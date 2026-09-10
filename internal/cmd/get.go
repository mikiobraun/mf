package cmd

import (
	"fmt"
	"strings"

	"github.com/mikiobraun/mf/registry"
	"github.com/spf13/cobra"
)

// getFields are the fields `mf get` can print, in help order.
var getFields = []string{"endpoint", "url", "api", "key", "model", "tags"}

func newGetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "get <name> <field>",
		Short: "Print a field of a model, for scripting",
		Long: "Print a field of a model, for scripting.\n\nFields: " + strings.Join(getFields, ", ") +
			"\n\n<name> is an alias, an endpoint/model reference, or an @tag.",
		Args: argHelp{
			min: 2, max: 2,
			args: [][2]string{
				{"<name>", addrArg},
				{"<field>", "field to print: " + strings.Join(getFields, ", ")},
			},
			example: "get fast url",
		}.validator(),
		RunE: func(cmd *cobra.Command, args []string) error {
			addr, field := args[0], args[1]
			s, err := openStore()
			if err != nil {
				return err
			}
			ref, err := registry.Resolve(s, addr)
			if err != nil {
				return err
			}
			warnAmbiguousTag(addr, ref.TagMatches)

			switch field {
			case "endpoint":
				fmt.Fprintln(cmd.OutOrStdout(), ref.Endpoint.Name)
			case "url":
				fmt.Fprintln(cmd.OutOrStdout(), ref.Endpoint.URL)
			case "model":
				fmt.Fprintln(cmd.OutOrStdout(), ref.ModelID)
			case "tags":
				fmt.Fprintln(cmd.OutOrStdout(), strings.Join(ref.Tags, ","))
			case "api":
				fmt.Fprintln(cmd.OutOrStdout(), ref.Endpoint.Dialect())
			case "key":
				key, ok := ref.Key(s)
				if !ok {
					return fmt.Errorf("no key configured for %s (set one with `mf set %s key <value>` or $%s)",
						addr, ref.Name, registry.KeyEnvVar(ref.Endpoint.Name))
				}
				fmt.Fprintln(cmd.OutOrStdout(), key)
			default:
				return fmt.Errorf("unknown field %q (expected one of: %s)", field, strings.Join(getFields, ", "))
			}
			return nil
		},
	}
}
