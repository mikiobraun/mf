package cmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mikiobraun/mf/internal/store"
	"github.com/spf13/cobra"
)

// picker builds a bare command wired to a canned stdin, returning the writer
// the prompt draws on so a test can assert what the user saw.
func picker(input string) (*cobra.Command, *strings.Builder) {
	c := &cobra.Command{}
	shown := new(strings.Builder)
	c.SetIn(strings.NewReader(input))
	c.SetOut(new(strings.Builder))
	c.SetErr(shown)
	return c, shown
}

var threeIDs = []string{"anthropic/claude-opus-4", "gemini/gemini-2.5-flash", "openai/gpt-4o"}

func TestPickModelByNumber(t *testing.T) {
	c, _ := picker("2\n")
	got, err := pickModel(c, "gw", threeIDs)
	if err != nil {
		t.Fatal(err)
	}
	if got != "gemini/gemini-2.5-flash" {
		t.Errorf("picked %q, want gemini/gemini-2.5-flash", got)
	}
}

// A filter renumbers the list, so the number that follows refers to what is on
// screen rather than to the original positions.
func TestPickModelFilterThenNumber(t *testing.T) {
	c, shown := picker("openai\n1\n")
	got, err := pickModel(c, "gw", threeIDs)
	if err != nil {
		t.Fatal(err)
	}
	if got != "openai/gpt-4o" {
		t.Errorf("picked %q, want openai/gpt-4o", got)
	}
	if !strings.Contains(shown.String(), "3 models on gw") {
		t.Errorf("header missing the count and endpoint:\n%s", shown)
	}
}

func TestPickModelFilterIsCaseInsensitive(t *testing.T) {
	c, _ := picker("GEMINI\n1\n")
	got, err := pickModel(c, "gw", threeIDs)
	if err != nil {
		t.Fatal(err)
	}
	if got != "gemini/gemini-2.5-flash" {
		t.Errorf("picked %q, want gemini/gemini-2.5-flash", got)
	}
}

// A filter is replaced, not stacked, so a second one cannot strand the user
// with an empty list they have to back out of.
func TestASecondFilterReplacesTheFirst(t *testing.T) {
	c, _ := picker("gemini\nopenai\n1\n")
	got, err := pickModel(c, "gw", threeIDs)
	if err != nil {
		t.Fatal(err)
	}
	if got != "openai/gpt-4o" {
		t.Errorf("picked %q, want openai/gpt-4o", got)
	}
}

func TestPickModelRejectsOutOfRange(t *testing.T) {
	c, shown := picker("99\n1\n")
	got, err := pickModel(c, "gw", threeIDs)
	if err != nil {
		t.Fatal(err)
	}
	if got != threeIDs[0] {
		t.Errorf("picked %q, want %q", got, threeIDs[0])
	}
	if !strings.Contains(shown.String(), "between 1 and 3") {
		t.Errorf("out-of-range message missing:\n%s", shown)
	}
}

func TestPickModelFilterMissKeepsTheList(t *testing.T) {
	c, shown := picker("zzz\n2\n")
	got, err := pickModel(c, "gw", threeIDs)
	if err != nil {
		t.Fatal(err)
	}
	if got != "gemini/gemini-2.5-flash" {
		t.Errorf("picked %q, want gemini/gemini-2.5-flash", got)
	}
	if !strings.Contains(shown.String(), `nothing matches "zzz"`) {
		t.Errorf("miss not reported:\n%s", shown)
	}
}

// Running out of input is an error rather than a default pick, so a scripted
// `mf add` with no model ID cannot silently choose one. This is what ctrl-D
// does at a terminal.
func TestPickModelWithNoInputSelectsNothing(t *testing.T) {
	c, _ := picker("")
	if _, err := pickModel(c, "gw", threeIDs); !errors.Is(err, errNoSelection) {
		t.Errorf("err = %v, want errNoSelection", err)
	}
}

// An empty line is the way out that needs no control key, and it cannot
// collide with a filter, since an empty filter would match everything.
func TestPickModelEmptyLineCancels(t *testing.T) {
	c, shown := picker("\n1\n")
	if _, err := pickModel(c, "gw", threeIDs); !errors.Is(err, errNoSelection) {
		t.Errorf("err = %v, want errNoSelection", err)
	}
	if !strings.Contains(shown.String(), "enter alone to cancel") {
		t.Errorf("the prompt does not say how to cancel:\n%s", shown)
	}
}

// main turns SIGINT into a cancelled context, so ctrl-C has to leave the
// picker through the context rather than through the blocked read.
func TestPickModelStopsOnACancelledContext(t *testing.T) {
	// A pipe nobody writes to stands in for a user who types nothing, so the
	// context is the only branch that can fire.
	pr, pw := io.Pipe()
	t.Cleanup(func() { pw.Close() })

	c := &cobra.Command{}
	c.SetIn(pr)
	c.SetOut(new(strings.Builder))
	c.SetErr(new(strings.Builder))
	ctx, cancel := context.WithCancel(context.Background())
	c.SetContext(ctx)
	cancel()

	if _, err := pickModel(c, "gw", threeIDs); !errors.Is(err, errNoSelection) {
		t.Errorf("err = %v, want errNoSelection", err)
	}
}

func TestPickModelNeedsSomethingToPickFrom(t *testing.T) {
	c, _ := picker("1\n")
	_, err := pickModel(c, "gw", nil)
	if err == nil || !strings.Contains(err.Error(), "no models") {
		t.Errorf("err = %v, want a complaint about an empty list", err)
	}
}

// Long lists are paged, and a number past the page is still valid because the
// numbering runs over every match.
func TestPickModelPagesLongLists(t *testing.T) {
	var ids []string
	for i := 1; i <= 25; i++ {
		ids = append(ids, fmt.Sprintf("model-%02d", i))
	}
	c, shown := picker("25\n")
	got, err := pickModel(c, "gw", ids)
	if err != nil {
		t.Fatal(err)
	}
	if got != "model-25" {
		t.Errorf("picked %q, want model-25", got)
	}
	out := shown.String()
	if !strings.Contains(out, "(5 more; type a filter to narrow)") {
		t.Errorf("page footer missing:\n%s", out)
	}
	if strings.Contains(out, "model-21") {
		t.Errorf("listed past the page size:\n%s", out)
	}
}

func TestPickNumber(t *testing.T) {
	cases := map[string]struct {
		n  int
		ok bool
	}{
		"3":    {3, true},
		"12":   {12, true},
		"4o":   {0, false},
		"":     {0, false},
		"-1":   {0, false},
		"3.5":  {0, false},
		"v1":   {0, false},
		" 3":   {0, false}, // callers trim, so a space means a filter
		"gpt4": {0, false},
	}
	for in, want := range cases {
		n, ok := pickNumber(in)
		if n != want.n || ok != want.ok {
			t.Errorf("pickNumber(%q) = (%d, %v), want (%d, %v)", in, n, ok, want.n, want.ok)
		}
	}
}

func TestFilterIDs(t *testing.T) {
	if got := filterIDs(threeIDs, "gemini"); len(got) != 1 || got[0] != "gemini/gemini-2.5-flash" {
		t.Errorf("got %v", got)
	}
	if got := filterIDs(threeIDs, "/"); len(got) != 3 {
		t.Errorf("a substring in every id should keep them all, got %v", got)
	}
	if got := filterIDs(threeIDs, "nope"); got != nil {
		t.Errorf("got %v, want nil", got)
	}
}

// storeWithEndpoint writes a config directory holding one endpoint pointed at
// srv, and returns the directory.
func storeWithEndpoint(t *testing.T, url string) string {
	t.Helper()
	dir := t.TempDir()
	s, err := store.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	s.PutEndpoint(store.Endpoint{Name: "gw", URL: url})
	if err := s.Save(); err != nil {
		t.Fatal(err)
	}
	return dir
}

// modelListServer serves two model IDs on /models.
func modelListServer(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"data":[{"id":"gemini/gemini-2.5-flash"},{"id":"openai/gpt-4o"}]}`)
	}))
	t.Cleanup(srv.Close)
	return srv
}

// runWith executes the tree with a canned stdin, returning stdout and the error.
func runWith(t *testing.T, stdin string, args ...string) (string, error) {
	t.Helper()
	root := NewRootCmd()
	root.SetArgs(args)
	root.SetIn(strings.NewReader(stdin))
	out := new(strings.Builder)
	root.SetOut(out)
	root.SetErr(new(strings.Builder))
	return out.String(), root.Execute()
}

func TestAddPicksTheModelWhenTheIDIsOmitted(t *testing.T) {
	dir := storeWithEndpoint(t, modelListServer(t).URL)

	if _, err := runWith(t, "gemini\n1\n", "--config", dir, "add", "fast", "gw"); err != nil {
		t.Fatal(err)
	}

	s, err := store.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	m, ok := s.Model("fast")
	if !ok {
		t.Fatal("fast was not written")
	}
	if m.Model != "gemini/gemini-2.5-flash" {
		t.Errorf("model = %q, want gemini/gemini-2.5-flash", m.Model)
	}
}

// A name clash is local, so it must be reported before the user is asked to
// choose from a list that would be thrown away.
func TestAddChecksTheNameBeforePrompting(t *testing.T) {
	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		fmt.Fprint(w, `{"data":[{"id":"m"}]}`)
	}))
	defer srv.Close()

	dir := storeWithEndpoint(t, srv.URL)
	s, _ := store.Open(dir)
	s.PutModel(store.Model{Name: "fast", Endpoint: "gw", Model: "old"})
	if err := s.Save(); err != nil {
		t.Fatal(err)
	}

	_, err := runWith(t, "1\n", "--config", dir, "add", "fast", "gw")
	if err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Errorf("err = %v, want an already-exists error", err)
	}
	if hits != 0 {
		t.Errorf("endpoint was queried %d times before the name check", hits)
	}
}

func TestSetModelPicksWhenTheValueIsOmitted(t *testing.T) {
	dir := storeWithEndpoint(t, modelListServer(t).URL)
	s, _ := store.Open(dir)
	s.PutModel(store.Model{Name: "fast", Endpoint: "gw", Model: "old"})
	if err := s.Save(); err != nil {
		t.Fatal(err)
	}

	if _, err := runWith(t, "gpt\n1\n", "--config", dir, "set", "fast", "model"); err != nil {
		t.Fatal(err)
	}

	s2, err := store.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	m, _ := s2.Model("fast")
	if m.Model != "openai/gpt-4o" {
		t.Errorf("model = %q, want openai/gpt-4o", m.Model)
	}
}

// Only the model field has a list to pick from; the rest still need a value.
func TestSetOtherFieldsStillNeedAValue(t *testing.T) {
	dir := storeWithEndpoint(t, modelListServer(t).URL)
	s, _ := store.Open(dir)
	s.PutModel(store.Model{Name: "fast", Endpoint: "gw", Model: "old"})
	if err := s.Save(); err != nil {
		t.Fatal(err)
	}

	_, err := runWith(t, "", "--config", dir, "set", "fast", "tags")
	if err == nil || !strings.Contains(err.Error(), "a value is required") {
		t.Errorf("err = %v, want a missing-value error", err)
	}
}

// A misspelled field is a typo, not a missing argument, so it must say so
// rather than asking for a value it would then reject.
func TestSetReportsAnUnknownFieldBeforeAskingForAValue(t *testing.T) {
	dir := storeWithEndpoint(t, modelListServer(t).URL)
	s, _ := store.Open(dir)
	s.PutModel(store.Model{Name: "fast", Endpoint: "gw", Model: "old"})
	if err := s.Save(); err != nil {
		t.Fatal(err)
	}

	_, err := runWith(t, "", "--config", dir, "set", "fast", "modl")
	if err == nil || !strings.Contains(err.Error(), `unknown field "modl"`) {
		t.Errorf("err = %v, want an unknown-field error", err)
	}
}
