package cmd

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/mikiobraun/mf/internal/provider"
	"github.com/mikiobraun/mf/internal/resolve"
	"github.com/mikiobraun/mf/internal/store"
	"github.com/spf13/cobra"
)

// clientForEndpoint builds a client for a named endpoint, resolving its key.
func clientForEndpoint(s *store.Store, name string) (provider.Client, error) {
	ep, err := mustEndpoint(s, name)
	if err != nil {
		return nil, err
	}
	key, _ := resolve.EndpointKey(s, name)
	return provider.New(ep, key)
}

func newModelsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "models <endpoint-name>",
		Short: "List model IDs an endpoint reports",
		Args: argHelp{
			min: 1, max: 1,
			args:    [][2]string{{"<endpoint-name>", "an endpoint from `mf endpoint list`"}},
			example: "models bifrost",
		}.validator(),
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := openStore()
			if err != nil {
				return err
			}
			client, err := clientForEndpoint(s, args[0])
			if err != nil {
				return err
			}
			ids, err := client.ListModels(cmd.Context())
			if err != nil {
				return fmt.Errorf("listing models on %s: %w", args[0], err)
			}
			for _, id := range ids {
				fmt.Fprintln(cmd.OutOrStdout(), id)
			}
			return nil
		},
	}
}

func newSyncCmd() *cobra.Command {
	var tags string
	c := &cobra.Command{
		Use:   "sync <endpoint-name>",
		Short: "Upsert every model an endpoint reports into models.toml",
		Long: "Fetch all available models from an endpoint and upsert them into\n" +
			"models.toml. Entries are named after the provider's model ID.\n\n" +
			"Entries that already exist keep their model ID: sync never overwrites a\n" +
			"hand-written alias. Models that have disappeared are not deleted.",
		Args: argHelp{
			min: 1, max: 1,
			args:    [][2]string{{"<endpoint-name>", "an endpoint from `mf endpoint list`"}},
			example: "sync bifrost --tags hosted",
		}.validator(),
		RunE: func(cmd *cobra.Command, args []string) error {
			epName := args[0]
			s, err := openStore()
			if err != nil {
				return err
			}
			client, err := clientForEndpoint(s, epName)
			if err != nil {
				return err
			}
			ids, err := client.ListModels(cmd.Context())
			if err != nil {
				return fmt.Errorf("listing models on %s: %w", epName, err)
			}

			newTags := splitTags(tags)
			var added, updated, skipped int
			for _, id := range ids {
				existing, exists := s.Model(id)
				if !exists {
					s.PutModel(store.Model{
						Name: id, Endpoint: epName, Model: id, Tags: newTags,
					})
					added++
					continue
				}
				// The name is taken. If it is an alias pointing somewhere
				// else, leave it entirely alone.
				if existing.Endpoint != epName || existing.Model != id {
					skipped++
					continue
				}
				if merged, changed := mergeTags(existing.Tags, newTags); changed {
					existing.Tags = merged
					s.PutModel(existing)
					updated++
				}
			}
			if err := s.Save(); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "synced %d models from %s (%d new, %d updated)\n",
				len(ids), epName, added, updated)
			if skipped > 0 {
				fmt.Fprintf(cmd.ErrOrStderr(), "note: left %d existing entr(ies) untouched because the name is already an alias for something else\n", skipped)
			}
			return nil
		},
	}
	c.Flags().StringVar(&tags, "tags", "", "tags to attach to newly created entries (comma-separated)")
	return c
}

// mergeTags adds any missing tags, preserving existing order.
func mergeTags(existing, add []string) ([]string, bool) {
	have := make(map[string]bool, len(existing))
	for _, t := range existing {
		have[t] = true
	}
	var changed bool
	for _, t := range add {
		if !have[t] {
			existing = append(existing, t)
			have[t] = true
			changed = true
		}
	}
	return existing, changed
}

func newCheckCmd() *cobra.Command {
	var all bool
	c := &cobra.Command{
		Use:   "check [<name>]",
		Short: "Check that endpoints are reachable and keys are valid",
		Long: "Validate that a model's endpoint is reachable and its API key is\n" +
			"accepted. With no arguments, every configured model is checked.\n\n" +
			"The check lists models on the endpoint; it does not generate tokens.",
		Args: argHelp{
			min: 0, max: 1,
			args:    [][2]string{{"[<name>]", addrArg + "\nomit it to check every configured model"}},
			example: "check fast",
		}.validator(),
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := openStore()
			if err != nil {
				return err
			}
			var targets []string
			if len(args) == 1 && !all {
				targets = []string{args[0]}
			} else {
				for _, m := range s.Config.Models {
					targets = append(targets, m.Name)
				}
				if len(targets) == 0 {
					fmt.Fprintln(cmd.ErrOrStderr(), "no models configured")
					return nil
				}
			}

			// One endpoint may back many models; check each endpoint once.
			type result struct {
				ok  bool
				msg string
			}
			seen := map[string]result{}
			w := newTabWriter(cmd.OutOrStdout())
			var failed bool
			for _, name := range targets {
				ref, err := resolve.Resolve(s, name)
				if err != nil {
					fmt.Fprintf(w, "%s\t✗\t%s\n", name, err)
					failed = true
					continue
				}
				r, done := seen[ref.Endpoint.Name]
				if !done {
					r = checkEndpoint(cmd.Context(), s, ref)
					seen[ref.Endpoint.Name] = r
				}
				mark := "✓"
				if !r.ok {
					mark = "✗"
					failed = true
				}
				fmt.Fprintf(w, "%s\t%s\t%s\n", name, mark, r.msg)
			}
			if err := w.Flush(); err != nil {
				return err
			}
			if failed {
				return errSilent
			}
			return nil
		},
	}
	c.Flags().BoolVar(&all, "all", false, "check every configured model")
	return c
}

func checkEndpoint(ctx context.Context, s *store.Store, ref *resolve.Ref) (r struct {
	ok  bool
	msg string
}) {
	key, _ := ref.Key(s)
	client, err := provider.New(ref.Endpoint, key)
	if err != nil {
		r.msg = err.Error()
		return r
	}
	models, err := client.ListModels(ctx)
	if err != nil {
		r.msg = describeCheckErr(err)
		return r
	}
	r.ok = true
	r.msg = fmt.Sprintf("reachable, key valid (%d models)", len(models))
	return r
}

// describeCheckErr turns a transport or status error into the short phrase
// check prints, with a hint where one is obviously useful.
func describeCheckErr(err error) string {
	var authErr *provider.AuthError
	if errors.As(err, &authErr) {
		return "key rejected: " + authErr.Error()
	}
	msg := err.Error()
	if strings.Contains(msg, "connection refused") {
		return "connection refused (is the endpoint running?)"
	}
	if strings.Contains(msg, "no such host") {
		return "host not found"
	}
	if strings.Contains(msg, "context deadline exceeded") || strings.Contains(msg, "Timeout") {
		return "timed out"
	}
	return msg
}

func newAskCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "ask <name> <message>",
		Short: "Send a single message to a model and print the reply",
		Long: "Send a single message to a model and print the reply. Useful for\n" +
			"smoke-testing a model, including local ones where the first request\n" +
			"triggers a model load.",
		Args: argHelp{
			min: 2, max: 2,
			args: [][2]string{
				{"<name>", addrArg},
				{"<message>", "the message to send, as a single argument\n(quote it if it contains spaces)"},
			},
			example: `ask fast "say hello"`,
		}.validator(),
		RunE: func(cmd *cobra.Command, args []string) error {
			addr, message := args[0], args[1]
			s, err := openStore()
			if err != nil {
				return err
			}
			ref, err := resolve.Resolve(s, addr)
			if err != nil {
				return err
			}
			warnAmbiguousTag(addr, ref.TagMatches)

			key, _ := ref.Key(s)
			client, err := provider.New(ref.Endpoint, key)
			if err != nil {
				return err
			}
			reply, err := client.Chat(cmd.Context(), ref.ModelID, message)
			if err != nil {
				return fmt.Errorf("asking %s: %w", ref.Name, err)
			}
			fmt.Fprintln(cmd.OutOrStdout(), reply)
			return nil
		},
	}
}
