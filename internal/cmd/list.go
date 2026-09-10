package cmd

import (
	"fmt"
	"strings"

	"github.com/mikiobraun/mf/registry"
	"github.com/spf13/cobra"
)

func newListCmd() *cobra.Command {
	var tags string
	c := &cobra.Command{
		Use:   "list",
		Short: "List all configured models",
		Args: argHelp{
			min: 0, max: 0,
			example: "list --tags coding",
			hint:    "to filter by tag, use the --tags flag rather than a positional argument",
		}.validator(),
		RunE: func(cmd *cobra.Command, _ []string) error {
			s, err := openStore()
			if err != nil {
				return err
			}
			want := splitTags(tags)
			urls := map[string]string{}
			for _, e := range s.Config.Endpoints {
				urls[e.Name] = e.URL
			}

			w := newTabWriter(cmd.OutOrStdout())
			fmt.Fprintln(w, "NAME\tMODEL\tENDPOINT\tTAGS")
			var shown int
			for _, m := range s.Config.Models {
				if !matchesAll(m, want) {
					continue
				}
				shown++
				url := urls[m.Endpoint]
				if url == "" {
					url = "(unknown endpoint " + m.Endpoint + ")"
				}
				fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", m.Name, m.Model, url, strings.Join(m.Tags, ", "))
			}
			if err := w.Flush(); err != nil {
				return err
			}
			if shown == 0 {
				if len(want) > 0 {
					fmt.Fprintf(cmd.ErrOrStderr(), "no models tagged %s\n", strings.Join(want, ", "))
				} else {
					fmt.Fprintln(cmd.ErrOrStderr(), "no models configured; add one with `mf endpoint add` then `mf add`")
				}
			}
			return nil
		},
	}
	c.Flags().StringVar(&tags, "tags", "", "filter by one or more tags (comma-separated)")
	return c
}

// matchesAll reports whether m carries every requested tag.
func matchesAll(m registry.Model, want []string) bool {
	for _, t := range want {
		if !m.HasTag(t) {
			return false
		}
	}
	return true
}
