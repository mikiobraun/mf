package cmd

import (
	"fmt"
	"slices"
	"strings"

	"github.com/spf13/cobra"
)

func newTagCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "tag",
		Short: "Add or remove tags on a model entry",
	}
	c.AddCommand(newTagChangeCmd("add"), newTagChangeCmd("remove"))
	return c
}

// newTagChangeCmd builds `tag add` or `tag remove`. Both leave the entry
// with or without the named tags: a tag already there (add) or already
// absent (remove) is noted on stderr and is not an error. The entry's
// resulting tags go to stdout.
func newTagChangeCmd(op string) *cobra.Command {
	short := map[string]string{
		"add":    "Add tags to a model entry",
		"remove": "Remove tags from a model entry",
	}[op]
	return &cobra.Command{
		Use:   op + " <name> <tag>...",
		Short: short,
		Args: argHelp{
			min: 2, max: -1,
			args: [][2]string{
				{"<name>", "alias of the model entry to change"},
				{"<tag>...", "one or more tags, without the @ used to address them"},
			},
			example: "tag " + op + " fast coding cheap",
		}.validator(),
		RunE: func(cmd *cobra.Command, args []string) error {
			name, tags := args[0], args[1:]
			for _, t := range tags {
				if err := checkTag(t); err != nil {
					return err
				}
			}

			s, err := openStore()
			if err != nil {
				return err
			}
			m, ok := s.Model(name)
			if !ok {
				return fmt.Errorf("no model named %q", name)
			}

			for _, t := range tags {
				switch has := m.HasTag(t); {
				case op == "add" && has:
					fmt.Fprintf(cmd.ErrOrStderr(), "note: %s already has tag %s\n", name, t)
				case op == "remove" && !has:
					fmt.Fprintf(cmd.ErrOrStderr(), "note: %s has no tag %s\n", name, t)
				}
			}
			if op == "add" {
				m.Tags, _ = mergeTags(m.Tags, tags)
			} else {
				m.Tags = slices.DeleteFunc(m.Tags, func(t string) bool { return slices.Contains(tags, t) })
				if len(m.Tags) == 0 {
					m.Tags = nil
				}
			}

			s.PutModel(m)
			if err := s.Save(); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%s: %s\n", name, strings.Join(m.Tags, ", "))
			return nil
		},
	}
}

// checkTag rejects what cannot be a tag: an empty string, a comma or
// whitespace, which --tags and `set tags` split on, and a leading @, which
// is the addressing syntax rather than part of the tag.
func checkTag(t string) error {
	switch {
	case t == "":
		return fmt.Errorf("a tag cannot be empty")
	case strings.HasPrefix(t, "@"):
		return fmt.Errorf("tag %q: write tags without the @ (%s); @ is how a tag is addressed", t, strings.TrimPrefix(t, "@"))
	case strings.ContainsAny(t, ", \t\n"):
		return fmt.Errorf("tag %q: a tag cannot contain commas or whitespace; give several tags as separate arguments", t)
	}
	return nil
}
