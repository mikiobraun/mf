package cmd

import (
	"fmt"
	"slices"
	"strings"

	"github.com/mikiobraun/mf/internal/store"
	"github.com/spf13/cobra"
)

func newAddCmd() *cobra.Command {
	var tags, key string
	help := argHelp{
		min: 2, max: 3,
		args: [][2]string{
			{"<name>", "alias to create, used to address the model later"},
			{"<endpoint>", "endpoint the model lives on, from `mf endpoint list`"},
			{"[<model>]", "provider model ID; omit it to pick from the endpoint's\nown list"},
		},
		example: "add fast bifrost gemini/gemini-2.5-flash --tags coding,cheap",
	}
	c := &cobra.Command{
		Use:   "add <name> <endpoint> [<model>]",
		Short: "Add a new model entry",
		Args:  help.validator(),
		RunE: func(cmd *cobra.Command, args []string) error {
			name, endpoint := args[0], args[1]
			var modelID string
			if len(args) == 3 {
				modelID = args[2]
			}

			s, err := openStore()
			if err != nil {
				return err
			}
			if _, err := mustEndpoint(s, endpoint); err != nil {
				return err
			}
			if _, exists := s.Model(name); exists {
				return fmt.Errorf("model %q already exists; use `mf set %s ...` to change it", name, name)
			}

			// Settle everything local before asking the endpoint for a list,
			// so a name clash fails before the user picks anything.
			if modelID == "" {
				if modelID, err = pickModelOnEndpoint(cmd, s, endpoint); err != nil {
					return err
				}
			}

			s.PutModel(store.Model{
				Name:     name,
				Endpoint: endpoint,
				Model:    modelID,
				Tags:     splitTags(tags),
			})
			if err := s.Save(); err != nil {
				return err
			}
			if key != "" {
				s.PutSecret(endpoint, name, key)
				if err := s.SaveSecrets(); err != nil {
					return err
				}
			}
			fmt.Fprintf(cmd.OutOrStdout(), "added %s -> %s on %s\n", name, modelID, endpoint)
			return nil
		},
	}
	c.Flags().StringVar(&tags, "tags", "", "tags to attach (comma-separated)")
	c.Flags().StringVar(&key, "key", "", "model-scoped API key, overriding the endpoint key")
	return c
}

func newRemoveCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "remove <name>",
		Short: "Remove a model entry, leaving its endpoint in place",
		Args: argHelp{
			min: 1, max: 1,
			args:    [][2]string{{"<name>", "alias of the model entry to remove"}},
			example: "remove fast",
		}.validator(),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			s, err := openStore()
			if err != nil {
				return err
			}
			if !s.RemoveModel(name) {
				return fmt.Errorf("no model named %q", name)
			}
			if err := s.Save(); err != nil {
				return err
			}
			if err := s.SaveSecrets(); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "removed %s\n", name)
			return nil
		},
	}
}

// setFields are the fields `mf set` accepts, in help order.
var setFields = []string{"endpoint", "model", "tags", "key"}

func newSetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "set <name> <field> [<value>]",
		Short: "Update a field on an existing model entry",
		Long: "Update a field on an existing model entry.\n\nFields: " + strings.Join(setFields, ", ") +
			"\n\nOmitting the value for the model field picks one from the endpoint's\nown list.",
		Args: argHelp{
			min: 2, max: 3,
			args: [][2]string{
				{"<name>", "alias of the model entry to change"},
				{"<field>", "field to set: " + strings.Join(setFields, ", ")},
				{"[<value>]", "the new value; omit it for the model field to pick\nfrom the endpoint's own list"},
			},
			example: "set fast tags coding,cheap",
		}.validator(),
		RunE: func(cmd *cobra.Command, args []string) error {
			name, field := args[0], args[1]

			s, err := openStore()
			if err != nil {
				return err
			}
			m, ok := s.Model(name)
			if !ok {
				return fmt.Errorf("no model named %q", name)
			}
			// Reject an unknown field before asking for a value, so a typo is
			// not reported as a missing argument.
			if field == "url" {
				return fmt.Errorf("url belongs to the endpoint, not the model; edit endpoint %q instead", m.Endpoint)
			}
			if !slices.Contains(setFields, field) {
				return fmt.Errorf("unknown field %q (expected one of: %s)", field, strings.Join(setFields, ", "))
			}

			var value string
			switch {
			case len(args) == 3:
				value = args[2]
			case field == "model":
				if value, err = pickModelOnEndpoint(cmd, s, m.Endpoint); err != nil {
					return err
				}
			default:
				return fmt.Errorf("a value is required: mf set %s %s <value>", name, field)
			}

			switch field {
			case "endpoint":
				if _, err := mustEndpoint(s, value); err != nil {
					return err
				}
				m.Endpoint = value
			case "model":
				m.Model = value
			case "tags":
				m.Tags = splitTags(value)
			case "key":
				s.PutSecret(m.Endpoint, m.Name, value)
				if err := s.SaveSecrets(); err != nil {
					return err
				}
				fmt.Fprintf(cmd.OutOrStdout(), "set key for %s\n", name)
				return nil
			}

			s.PutModel(m)
			if err := s.Save(); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "set %s %s = %s\n", name, field, value)
			return nil
		},
	}
}
