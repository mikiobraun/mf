package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

// addrArg is the description of a model address, used by every command that
// takes one.
const addrArg = "model to address: an alias (fast), an endpoint/model reference\n" +
	"(bifrost/gemini/gemini-2.5-flash), or a @tag (@coding)"

// argHelp documents a command's positional arguments. A count mismatch is
// reported by naming each argument and showing a worked example, rather than
// by printing the counts and leaving the caller to consult --help.
type argHelp struct {
	// min and max bound the argument count. A max of -1 means unbounded.
	min, max int
	// args lists each positional in order as {placeholder, description}.
	// Optional placeholders are written with brackets, e.g. "[<name>]".
	args [][2]string
	// example is a complete command line, without the leading binary name.
	example string
	// hint is an optional extra line for a likely mistake.
	hint string
}

// validator returns the cobra hook enforcing the spec.
func (h argHelp) validator() cobra.PositionalArgs {
	return func(c *cobra.Command, got []string) error {
		if len(got) >= h.min && (h.max < 0 || len(got) <= h.max) {
			return nil
		}
		return h.err(c, got)
	}
}

func rootName(c *cobra.Command) string {
	if r := c.Root(); r != nil {
		return r.Name()
	}
	return "mf"
}

// path is the command's invocation without the binary name, so the message
// reads "get takes ..." rather than repeating the "mf:" prefix main adds.
func path(c *cobra.Command) string {
	return strings.TrimPrefix(c.CommandPath(), rootName(c)+" ")
}

func (h argHelp) err(c *cobra.Command, got []string) error {
	return fmt.Errorf("%s takes %s, you gave %s\n%s",
		path(c), h.expected(), count(len(got)), h.details(c))
}

// missing reports required flags that were not supplied, in the same shape as
// an argument-count error.
func (h argHelp) missing(c *cobra.Command, flags ...string) error {
	return fmt.Errorf("%s needs %s\n%s",
		path(c), joinAnd(flags), h.details(c))
}

func joinAnd(items []string) string {
	switch len(items) {
	case 0:
		return ""
	case 1:
		return items[0]
	default:
		return strings.Join(items[:len(items)-1], ", ") + " and " + items[len(items)-1]
	}
}

// details renders the usage line, the per-argument descriptions, the example
// and the hint, shared by every argHelp error.
func (h argHelp) details(c *cobra.Command) string {
	var b strings.Builder
	fmt.Fprintf(&b, "\nusage:\n  %s %s", rootName(c), path(c))
	for _, a := range h.args {
		fmt.Fprintf(&b, " %s", a[0])
	}
	b.WriteString("\n")

	if len(h.args) > 0 {
		b.WriteString("\n")
		width := 0
		for _, a := range h.args {
			if len(a[0]) > width {
				width = len(a[0])
			}
		}
		for _, a := range h.args {
			// Indent continuation lines under the description column.
			desc := strings.ReplaceAll(a[1], "\n", "\n  "+strings.Repeat(" ", width+2))
			fmt.Fprintf(&b, "  %-*s  %s\n", width, a[0], desc)
		}
	}
	if h.example != "" {
		fmt.Fprintf(&b, "\nexample:\n  %s %s\n", rootName(c), h.example)
	}
	if h.hint != "" {
		fmt.Fprintf(&b, "\n%s\n", h.hint)
	}
	return strings.TrimRight(b.String(), "\n")
}

// expected renders the accepted argument count as a phrase.
func (h argHelp) expected() string {
	switch {
	case h.min == 0 && h.max == 0:
		return "no arguments"
	case h.max < 0 && h.min > 0:
		return fmt.Sprintf("at least %s", count(h.min))
	case h.min == h.max:
		return count(h.min)
	case h.min == 0:
		return fmt.Sprintf("at most %s", count(h.max))
	default:
		return fmt.Sprintf("%d or %d arguments", h.min, h.max)
	}
}

func count(n int) string {
	if n == 1 {
		return "1 argument"
	}
	return fmt.Sprintf("%d arguments", n)
}
