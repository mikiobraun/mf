package cmd

import (
	"fmt"
	"strings"

	"github.com/mikiobraun/mf/registry"
	"github.com/spf13/cobra"
)

func newListCmd() *cobra.Command {
	var tags string
	var asJSON bool
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
			if asJSON {
				return listJSON(cmd, s, want)
			}
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
	c.Flags().BoolVar(&asJSON, "json", false, "print a JSON array of entries, without keys")
	return c
}

// listJSON prints the matching entries as a JSON array, always an array even
// when empty. An entry whose endpoint is missing is still listed, with an
// empty url and api, and noted on stderr.
func listJSON(cmd *cobra.Command, s *registry.Store, want []string) error {
	out := []entryJSON{}
	for _, m := range s.Config.Models {
		if !matchesAll(m, want) {
			continue
		}
		ref, err := registry.Resolve(s, m.Name)
		if err != nil {
			fmt.Fprintf(cmd.ErrOrStderr(), "note: %s\n", err)
			ref = &registry.Ref{Name: m.Name, ModelID: m.Model, Endpoint: registry.Endpoint{Name: m.Endpoint}, Tags: m.Tags, Entry: true}
			e := newEntryJSON(s, ref)
			e.API = ""
			out = append(out, e)
			continue
		}
		out = append(out, newEntryJSON(s, ref))
	}
	return writeJSON(cmd.OutOrStdout(), out)
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
