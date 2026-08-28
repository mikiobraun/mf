package cmd

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// run executes the command tree with args and returns the error, so tests
// assert on what a user actually sees.
func run(t *testing.T, args ...string) error {
	t.Helper()
	root := NewRootCmd()
	root.SetArgs(args)
	root.SetOut(new(strings.Builder))
	root.SetErr(new(strings.Builder))
	return root.Execute()
}

func TestArgErrorNamesEachArgument(t *testing.T) {
	err := run(t, "get", "fast")
	if err == nil {
		t.Fatal("expected an error")
	}
	msg := err.Error()
	for _, want := range []string{
		"get takes 2 arguments, you gave 1 argument",
		"usage:",
		"mf get <name> <field>",
		"<name>",
		"<field>",
		"field to print: endpoint, url, api, key, model, tags",
		"example:",
		"mf get fast url",
	} {
		if !strings.Contains(msg, want) {
			t.Errorf("message missing %q:\n%s", want, msg)
		}
	}
	// The cobra default must be gone.
	if strings.Contains(msg, "accepts") && strings.Contains(msg, "arg(s)") {
		t.Errorf("still showing the cobra default:\n%s", msg)
	}
}

func TestListRejectsPositionalWithTagHint(t *testing.T) {
	err := run(t, "list", "coding")
	if err == nil {
		t.Fatal("expected an error")
	}
	msg := err.Error()
	if !strings.Contains(msg, "list takes no arguments") {
		t.Errorf("got %q", msg)
	}
	if !strings.Contains(msg, "--tags") {
		t.Errorf("should point at --tags:\n%s", msg)
	}
	// cobra's default for NoArgs is a misleading "unknown command".
	if strings.Contains(msg, "unknown command") {
		t.Errorf("still reporting an unknown command:\n%s", msg)
	}
}

func TestRangeAndMaximumPhrasing(t *testing.T) {
	if err := run(t, "set", "fast"); err == nil ||
		!strings.Contains(err.Error(), "set takes 2 or 3 arguments") {
		t.Errorf("set: got %v", err)
	}
	if err := run(t, "check", "a", "b"); err == nil ||
		!strings.Contains(err.Error(), "check takes at most 1 argument") {
		t.Errorf("check: got %v", err)
	}
}

// Arguments that are always required are positional, never flags. A flag that
// can never be omitted is a required argument wearing the wrong syntax.
func TestAlwaysRequiredValuesArePositional(t *testing.T) {
	err := run(t, "add", "foo")
	if err == nil {
		t.Fatal("expected an error")
	}
	msg := err.Error()
	if !strings.Contains(msg, "add takes 2 or 3 arguments, you gave 1 argument") {
		t.Errorf("got %q", msg)
	}
	for _, want := range []string{"<endpoint>", "<model>", "mf add <name> <endpoint> [<model>]"} {
		if !strings.Contains(msg, want) {
			t.Errorf("message missing %q:\n%s", want, msg)
		}
	}

	err = run(t, "endpoint", "add", "foo")
	if err == nil || !strings.Contains(err.Error(), "endpoint add takes 2 arguments") {
		t.Errorf("endpoint add: got %v", err)
	}
}

// The flags these replaced must be gone, not merely unused, so a stale
// invocation fails loudly instead of silently ignoring the value.
func TestRetiredRequiredFlagsAreRejected(t *testing.T) {
	cases := [][]string{
		{"add", "foo", "--endpoint", "bifrost", "--model", "m"},
		{"endpoint", "add", "foo", "--url", "https://x/v1"},
	}
	for _, args := range cases {
		err := run(t, args...)
		if err == nil || !strings.Contains(err.Error(), "unknown flag") {
			t.Errorf("%v: got %v, want an unknown-flag error", args, err)
		}
	}
}

// Only genuinely optional values may be flags.
func TestNoCommandHasARequiredFlag(t *testing.T) {
	var walk func(c *cobra.Command)
	walk = func(c *cobra.Command) {
		for _, sub := range c.Commands() {
			walk(sub)
		}
		c.Flags().VisitAll(func(f *pflag.Flag) {
			if strings.Contains(strings.ToLower(f.Usage), "required") {
				t.Errorf("%s --%s is documented as required; make it a positional argument",
					c.CommandPath(), f.Name)
			}
		})
	}
	walk(NewRootCmd())
}

func TestExpectedPhrasing(t *testing.T) {
	cases := []struct {
		h    argHelp
		want string
	}{
		{argHelp{min: 0, max: 0}, "no arguments"},
		{argHelp{min: 1, max: 1}, "1 argument"},
		{argHelp{min: 2, max: 2}, "2 arguments"},
		{argHelp{min: 0, max: 1}, "at most 1 argument"},
		{argHelp{min: 2, max: 3}, "2 or 3 arguments"},
		{argHelp{min: 1, max: -1}, "at least 1 argument"},
	}
	for _, c := range cases {
		if got := c.h.expected(); got != c.want {
			t.Errorf("expected() = %q, want %q", got, c.want)
		}
	}
}

func TestJoinAnd(t *testing.T) {
	cases := map[string][]string{
		"":                 {},
		"--a":              {"--a"},
		"--a and --b":      {"--a", "--b"},
		"--a, --b and --c": {"--a", "--b", "--c"},
	}
	for want, in := range cases {
		if got := joinAnd(in); got != want {
			t.Errorf("joinAnd(%v) = %q, want %q", in, got, want)
		}
	}
}

// Every command that takes positionals must document them, so no command can
// fall back to the cobra default by omission.
func TestNoCommandUsesTheCobraDefaultArgs(t *testing.T) {
	var walk func(c *cobra.Command)
	walk = func(c *cobra.Command) {
		for _, sub := range c.Commands() {
			walk(sub)
		}
		if !c.Runnable() || c.Name() == "help" || c.Name() == "completion" {
			return
		}
		if c.Args == nil {
			t.Errorf("%s has no Args validator, so cobra's default would apply", c.CommandPath())
		}
	}
	walk(NewRootCmd())
}

// Flag names appear in strings the commands print — argHelp examples and
// hints, and the guidance messages RunE writes when something is missing.
// Those strings are invisible to the compiler, so a flag that is renamed or
// retired leaves working prose behind that recommends a command which now
// fails with "unknown flag". TestRetiredRequiredFlagsAreRejected proves the
// flag is gone; this proves nothing still tells the user to pass it.
//
// The check reads the package's own source and pulls flag-like tokens out of
// every string literal, since the offending text is as likely to sit in a
// Fprintln inside RunE as in cobra metadata. Comments are not scanned: they
// are not user-facing. Shorthands (-f) are not scanned either, being too
// short to distinguish from ordinary prose.
func TestNoStringMentionsAFlagThatDoesNotExist(t *testing.T) {
	known := map[string]bool{"help": true} // cobra adds --help itself
	var collect func(c *cobra.Command)
	collect = func(c *cobra.Command) {
		for _, sub := range c.Commands() {
			collect(sub)
		}
		add := func(f *pflag.Flag) { known[f.Name] = true }
		c.Flags().VisitAll(add)
		c.PersistentFlags().VisitAll(add)
	}
	collect(NewRootCmd())

	// A flag defined on one command may legitimately be named by another —
	// `mf list --tags` is suggested from warnAmbiguousTag — so membership is
	// checked against the whole tree rather than per command.
	flagRef := regexp.MustCompile(`--[a-zA-Z][a-zA-Z0-9-]*`)
	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, ".", func(fi os.FileInfo) bool {
		return !strings.HasSuffix(fi.Name(), "_test.go")
	}, 0)
	if err != nil {
		t.Fatalf("parsing package source: %v", err)
	}
	if len(pkgs) == 0 {
		t.Fatal("no non-test source found; the scan would pass vacuously")
	}

	var scanned int
	for _, pkg := range pkgs {
		ast.Inspect(pkg, func(n ast.Node) bool {
			lit, ok := n.(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				return true
			}
			s, err := strconv.Unquote(lit.Value)
			if err != nil {
				return true
			}
			scanned++
			for _, ref := range flagRef.FindAllString(s, -1) {
				name := strings.TrimPrefix(ref, "--")
				if !known[name] {
					t.Errorf("%s: string mentions --%s, which no command defines:\n\t%q",
						fset.Position(lit.Pos()), name, s)
				}
			}
			return true
		})
	}
	if scanned == 0 {
		t.Fatal("no string literals scanned; the check is not doing anything")
	}
}
