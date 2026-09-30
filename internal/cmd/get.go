package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/mikiobraun/mf/registry"
	"github.com/spf13/cobra"
)

// getFields are the fields `mf get` can print, in help order.
var getFields = []string{"endpoint", "url", "api", "key", "model", "tags"}

func newGetCmd() *cobra.Command {
	var asJSON bool
	c := &cobra.Command{
		Use:   "get <name> [<field>]",
		Short: "Print a field of a model, for scripting",
		Long: "Print a field of a model, for scripting.\n\nFields: " + strings.Join(getFields, ", ") +
			"\n\n<name> is an alias, an endpoint/model reference, or an @tag.\n\n" +
			"With --json and no field, print the whole entry as a JSON object. The\n" +
			"key is left out; has_key reports whether one is configured.",
		Args: argHelp{
			min: 1, max: 2,
			args: [][2]string{
				{"<name>", addrArg},
				{"[<field>]", "field to print: " + strings.Join(getFields, ", ") +
					"\nomit it with --json to print the whole entry"},
			},
			example: "get fast url",
		}.validator(),
		RunE: func(cmd *cobra.Command, args []string) error {
			addr := args[0]
			switch {
			case asJSON && len(args) == 2:
				return fmt.Errorf("--json prints the whole entry; drop the field %q or drop --json", args[1])
			case !asJSON && len(args) == 1:
				return fmt.Errorf("get needs a field (%s), or --json for the whole entry", strings.Join(getFields, ", "))
			}
			s, err := openStore()
			if err != nil {
				return err
			}
			ref, err := registry.Resolve(s, addr)
			if err != nil {
				return err
			}
			warnAmbiguousTag(addr, ref.TagMatches)

			if asJSON {
				return writeJSON(cmd.OutOrStdout(), newEntryJSON(s, ref))
			}

			switch field := args[1]; field {
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
	c.Flags().BoolVar(&asJSON, "json", false, "print the whole entry as JSON, without the key")
	return c
}

// entryJSON is the shape `mf get --json` and `mf list --json` print. Scripts
// depend on it, so fields are added rather than renamed or removed. The key
// itself is never included.
type entryJSON struct {
	Name     string   `json:"name"`
	Endpoint string   `json:"endpoint"`
	URL      string   `json:"url"`
	API      string   `json:"api"`
	Model    string   `json:"model"`
	Tags     []string `json:"tags"`
	HasKey   bool     `json:"has_key"`
}

func newEntryJSON(s *registry.Store, ref *registry.Ref) entryJSON {
	_, hasKey := ref.Key(s)
	tags := ref.Tags
	if tags == nil {
		tags = []string{}
	}
	return entryJSON{
		Name:     ref.Name,
		Endpoint: ref.Endpoint.Name,
		URL:      ref.Endpoint.URL,
		API:      ref.Endpoint.Dialect(),
		Model:    ref.ModelID,
		Tags:     tags,
		HasKey:   hasKey,
	}
}

func writeJSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}
