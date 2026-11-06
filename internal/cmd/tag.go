package cmd

import (
	"fmt"
	"slices"
	"strings"

	"github.com/mikiobraun/mf/registry"
	"github.com/spf13/cobra"
)

func newTagCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "tag",
		Short: "Add or remove tags on a model entry",
	}
	c.AddCommand(newTagChangeCmd("add"), newTagChangeCmd("remove"), newTagSwapCmd())
	return c
}

// newTagSwapCmd builds `tag swap`, which moves a tag that names one model
// to another: the entry that has the tag loses it, <name> gets it. A tag
// no entry has yet is given to <name> with a warning; a tag several
// entries have is refused, since there is no single one to take it from.
func newTagSwapCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "swap <name> <tag>",
		Short: "Move a tag from the entry that has it to another",
		Args: argHelp{
			min: 2, max: 2,
			args: [][2]string{
				{"<name>", "alias of the model entry that gets the tag"},
				{"<tag>", "the tag, without the @ used to address it"},
			},
			example: "tag swap fast coding",
		}.validator(),
		RunE: func(cmd *cobra.Command, args []string) error {
			name, tag := args[0], args[1]
			if err := checkTag(tag); err != nil {
				return err
			}

			s, err := openStore()
			if err != nil {
				return err
			}
			m, ok := s.Model(name)
			if !ok {
				return fmt.Errorf("no model named %q", name)
			}

			holders := s.ModelsWithTag(tag)
			switch {
			case len(holders) > 1:
				var names []string
				for _, h := range holders {
					names = append(names, h.Name)
				}
				return fmt.Errorf("tag %s is on %s; swap moves a tag from one entry, so remove it from all but one first (mf tag remove)",
					tag, strings.Join(names, ", "))
			case len(holders) == 0:
				fmt.Fprintf(cmd.ErrOrStderr(), "warning: no entry has tag %s; adding it to %s\n", tag, name)
			case holders[0].Name == name:
				fmt.Fprintf(cmd.ErrOrStderr(), "note: %s already has tag %s\n", name, tag)
				fmt.Fprintf(cmd.OutOrStdout(), "%s: %s\n", name, strings.Join(m.Tags, ", "))
				return nil
			}

			var changed []registry.Model
			if len(holders) == 1 {
				old := holders[0]
				old.Tags = slices.DeleteFunc(old.Tags, func(t string) bool { return t == tag })
				if len(old.Tags) == 0 {
					old.Tags = nil
				}
				s.PutModel(old)
				changed = append(changed, old)
			}
			m.Tags, _ = mergeTags(m.Tags, []string{tag})
			s.PutModel(m)
			changed = append(changed, m)

			if err := s.Save(); err != nil {
				return err
			}
			for _, c := range changed {
				fmt.Fprintf(cmd.OutOrStdout(), "%s: %s\n", c.Name, strings.Join(c.Tags, ", "))
			}
			return nil
		},
	}
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
