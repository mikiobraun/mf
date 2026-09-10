package cmd

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/mikiobraun/mf/registry"
)

var threeIDs = []string{"anthropic/claude-opus-4", "gemini/gemini-2.5-flash", "openai/gpt-4o"}

// send feeds messages to the model in order and returns the state they left.
func send(p picker, msgs ...tea.Msg) picker {
	for _, msg := range msgs {
		next, _ := p.Update(msg)
		p = next.(picker)
	}
	return p
}

func typed(s string) tea.Msg { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)} }

func pressed(t tea.KeyType) tea.Msg { return tea.KeyMsg{Type: t} }

// manyIDs builds a list long enough to scroll.
func manyIDs(n int) []string {
	var ids []string
	for i := 1; i <= n; i++ {
		ids = append(ids, fmt.Sprintf("model-%02d", i))
	}
	return ids
}

func TestPickerFiltersAsYouType(t *testing.T) {
	p := send(newPicker("gw", threeIDs), typed("g"), typed("e"), typed("m"))
	if p.filter != "gem" {
		t.Errorf("filter = %q, want gem", p.filter)
	}
	if len(p.matches) != 1 || p.matches[0] != "gemini/gemini-2.5-flash" {
		t.Errorf("matches = %v, want the one gemini entry", p.matches)
	}
}

func TestPickerFilterIsCaseInsensitive(t *testing.T) {
	p := send(newPicker("gw", threeIDs), typed("GEM"))
	if len(p.matches) != 1 {
		t.Errorf("matches = %v, want 1", p.matches)
	}
}

func TestPickerBackspaceWidensTheList(t *testing.T) {
	p := send(newPicker("gw", threeIDs), typed("gem"), pressed(tea.KeyBackspace), pressed(tea.KeyBackspace))
	if p.filter != "g" {
		t.Errorf("filter = %q, want g", p.filter)
	}
	// "g" is in gemini and in openai/gpt, but not in anthropic/claude-opus-4.
	if len(p.matches) != 2 {
		t.Errorf("matches = %v, want 2", p.matches)
	}
}

func TestPickerBackspaceOnAnEmptyFilterDoesNothing(t *testing.T) {
	p := send(newPicker("gw", threeIDs), pressed(tea.KeyBackspace))
	if p.filter != "" || len(p.matches) != 3 {
		t.Errorf("filter = %q, matches = %v", p.filter, p.matches)
	}
}

// A new filter invalidates the old cursor position, so it starts from the top.
func TestPickerFilteringResetsTheCursor(t *testing.T) {
	p := send(newPicker("gw", threeIDs), pressed(tea.KeyDown), pressed(tea.KeyDown))
	if p.cursor != 2 {
		t.Fatalf("cursor = %d, want 2", p.cursor)
	}
	p = send(p, typed("a"))
	if p.cursor != 0 {
		t.Errorf("cursor = %d after filtering, want 0", p.cursor)
	}
}

func TestPickerCursorStopsAtBothEnds(t *testing.T) {
	p := send(newPicker("gw", threeIDs), pressed(tea.KeyUp), pressed(tea.KeyUp))
	if p.cursor != 0 {
		t.Errorf("cursor = %d at the top, want 0", p.cursor)
	}
	p = send(p, pressed(tea.KeyDown), pressed(tea.KeyDown), pressed(tea.KeyDown), pressed(tea.KeyDown))
	if p.cursor != 2 {
		t.Errorf("cursor = %d at the bottom, want 2", p.cursor)
	}
}

func TestPickerEnterTakesTheCursorsMatch(t *testing.T) {
	p := send(newPicker("gw", threeIDs), pressed(tea.KeyDown), pressed(tea.KeyEnter))
	if p.chosen != "gemini/gemini-2.5-flash" {
		t.Errorf("chosen = %q, want gemini/gemini-2.5-flash", p.chosen)
	}
}

func TestPickerEnterWithNoMatchesChoosesNothing(t *testing.T) {
	p := send(newPicker("gw", threeIDs), typed("zzz"), pressed(tea.KeyEnter))
	if p.chosen != "" {
		t.Errorf("chosen = %q, want nothing", p.chosen)
	}
	if !p.done {
		t.Error("enter should still end the picker")
	}
}

func TestPickerEscapeAndCtrlCChooseNothing(t *testing.T) {
	for name, key := range map[string]tea.KeyType{"esc": tea.KeyEsc, "ctrl-c": tea.KeyCtrlC} {
		p := send(newPicker("gw", threeIDs), pressed(tea.KeyDown), pressed(key))
		if p.chosen != "" {
			t.Errorf("%s: chosen = %q, want nothing", name, p.chosen)
		}
		if !p.done {
			t.Errorf("%s: should end the picker", name)
		}
	}
}

// The window follows the cursor down and back up, moving no further than it
// has to.
func TestPickerScrollsToFollowTheCursor(t *testing.T) {
	p := send(newPicker("gw", manyIDs(25)), tea.WindowSizeMsg{Width: 80, Height: 11})
	if p.rows != 6 {
		t.Fatalf("rows = %d, want 6", p.rows)
	}
	for i := 0; i < 5; i++ {
		p = send(p, pressed(tea.KeyDown))
	}
	if p.cursor != 5 || p.offset != 0 {
		t.Errorf("cursor = %d, offset = %d; the last visible row should not scroll", p.cursor, p.offset)
	}
	p = send(p, pressed(tea.KeyDown))
	if p.cursor != 6 || p.offset != 1 {
		t.Errorf("cursor = %d, offset = %d; want the window moved by one", p.cursor, p.offset)
	}
	for i := 0; i < 6; i++ {
		p = send(p, pressed(tea.KeyUp))
	}
	if p.cursor != 0 || p.offset != 0 {
		t.Errorf("cursor = %d, offset = %d; want the window back at the top", p.cursor, p.offset)
	}
}

// Filtering down to fewer entries than the window must not leave the window
// scrolled past them.
func TestPickerScrollDoesNotStrandAShortList(t *testing.T) {
	p := send(newPicker("gw", manyIDs(25)), tea.WindowSizeMsg{Width: 80, Height: 11})
	for i := 0; i < 20; i++ {
		p = send(p, pressed(tea.KeyDown))
	}
	if p.offset == 0 {
		t.Fatal("expected the window to have scrolled")
	}
	p = send(p, typed("model-01"))
	if p.offset != 0 || p.cursor != 0 {
		t.Errorf("offset = %d, cursor = %d, want both at 0", p.offset, p.cursor)
	}
}

func TestViewportRowsStaysWithinBounds(t *testing.T) {
	cases := map[int]int{
		4:   pickMinRows,
		9:   pickMinRows,
		12:  7,
		15:  pickMaxRows,
		200: pickMaxRows,
	}
	for height, want := range cases {
		if got := viewportRows(height); got != want {
			t.Errorf("viewportRows(%d) = %d, want %d", height, got, want)
		}
	}
}

func TestPickerViewMarksTheCursorAndShowsTheFilter(t *testing.T) {
	p := send(newPicker("gw", threeIDs), typed("o"), pressed(tea.KeyDown))
	view := p.View()
	if !strings.Contains(view, "> openai/gpt-4o") {
		t.Errorf("the cursor line is not marked:\n%s", view)
	}
	if !strings.Contains(view, "2 of 3 models on gw") {
		t.Errorf("the header does not show the filtered count:\n%s", view)
	}
	if !strings.Contains(view, "esc cancels") {
		t.Errorf("the key hints are missing:\n%s", view)
	}
}

func TestPickerViewSaysWhenNothingMatches(t *testing.T) {
	p := send(newPicker("gw", threeIDs), typed("zzz"))
	if !strings.Contains(p.View(), "nothing matches") {
		t.Errorf("view does not report an empty list:\n%s", p.View())
	}
}

// The view keeps a fixed height so the filter field does not jump about while
// the match count changes under it.
func TestPickerViewHeightIsConstant(t *testing.T) {
	p := send(newPicker("gw", manyIDs(25)), tea.WindowSizeMsg{Width: 80, Height: 24})
	full := strings.Count(p.View(), "\n")
	narrowed := strings.Count(send(p, typed("model-01")).View(), "\n")
	empty := strings.Count(send(p, typed("zzz")).View(), "\n")
	if full != narrowed || full != empty {
		t.Errorf("heights differ: %d full, %d narrowed, %d empty", full, narrowed, empty)
	}
}

func TestPickerViewIsEmptyOnceDone(t *testing.T) {
	p := send(newPicker("gw", threeIDs), pressed(tea.KeyEnter))
	if p.View() != "" {
		t.Errorf("view should be erased on exit, got %q", p.View())
	}
}

func TestFit(t *testing.T) {
	cases := []struct {
		in   string
		w    int
		want string
	}{
		{"short", 10, "short"},
		{"exactly-10", 10, "exactly-10"},
		{"truncate-me", 6, "trunc…"},
		{"anything", 0, "anything"},
		{"anything", -4, "anything"},
		{"x", 1, "x"},
		{"xy", 1, "…"},
	}
	for _, c := range cases {
		if got := fit(c.in, c.w); got != c.want {
			t.Errorf("fit(%q, %d) = %q, want %q", c.in, c.w, got, c.want)
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
// url, and returns the directory.
func storeWithEndpoint(t *testing.T, url string) string {
	t.Helper()
	dir := t.TempDir()
	s, err := registry.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	s.PutEndpoint(registry.Endpoint{Name: "gw", URL: url})
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

// withEntry adds a model entry to the store in dir.
func withEntry(t *testing.T, dir string, m registry.Model) {
	t.Helper()
	s, err := registry.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	s.PutModel(m)
	if err := s.Save(); err != nil {
		t.Fatal(err)
	}
}

// runIn executes the tree against a config directory, returning stdout and the
// error. Tests run without a terminal, which is what the picker refuses on.
func runIn(t *testing.T, args ...string) (string, error) {
	t.Helper()
	root := NewRootCmd()
	root.SetArgs(args)
	out := new(strings.Builder)
	root.SetOut(out)
	root.SetErr(new(strings.Builder))
	return out.String(), root.Execute()
}

// The picker draws; it cannot run down a pipe, so it says so instead of
// hanging or half-drawing.
func TestAddRefusesToPickWithoutATerminal(t *testing.T) {
	dir := storeWithEndpoint(t, modelListServer(t).URL)
	_, err := runIn(t, "--config", dir, "add", "fast", "gw")
	if err == nil || !strings.Contains(err.Error(), "needs a terminal") {
		t.Errorf("err = %v, want a complaint about the terminal", err)
	}
	if !strings.Contains(fmt.Sprint(err), "pass the model ID as an argument") {
		t.Errorf("the error should say what to do instead: %v", err)
	}
}

func TestSetModelRefusesToPickWithoutATerminal(t *testing.T) {
	dir := storeWithEndpoint(t, modelListServer(t).URL)
	withEntry(t, dir, registry.Model{Name: "fast", Endpoint: "gw", Model: "old"})

	_, err := runIn(t, "--config", dir, "set", "fast", "model")
	if err == nil || !strings.Contains(err.Error(), "needs a terminal") {
		t.Errorf("err = %v, want a complaint about the terminal", err)
	}
}

// With a terminal available the endpoint is still consulted, so an empty list
// is reported before anything is drawn.
func TestPickModelRejectsAnEmptyList(t *testing.T) {
	restore := isTerminal
	isTerminal = func() bool { return true }
	t.Cleanup(func() { isTerminal = restore })

	_, err := pickModel(NewRootCmd(), "gw", nil)
	if err == nil || !strings.Contains(err.Error(), "no models") {
		t.Errorf("err = %v, want a complaint about an empty list", err)
	}
}

// A name clash is local, so it must be reported before the endpoint is asked
// for a list that would be thrown away.
func TestAddChecksTheNameBeforePrompting(t *testing.T) {
	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		fmt.Fprint(w, `{"data":[{"id":"m"}]}`)
	}))
	defer srv.Close()

	dir := storeWithEndpoint(t, srv.URL)
	withEntry(t, dir, registry.Model{Name: "fast", Endpoint: "gw", Model: "old"})

	_, err := runIn(t, "--config", dir, "add", "fast", "gw")
	if err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Errorf("err = %v, want an already-exists error", err)
	}
	if hits != 0 {
		t.Errorf("endpoint was queried %d times before the name check", hits)
	}
}

// Only the model field has a list to pick from; the rest still need a value.
func TestSetOtherFieldsStillNeedAValue(t *testing.T) {
	dir := storeWithEndpoint(t, modelListServer(t).URL)
	withEntry(t, dir, registry.Model{Name: "fast", Endpoint: "gw", Model: "old"})

	_, err := runIn(t, "--config", dir, "set", "fast", "tags")
	if err == nil || !strings.Contains(err.Error(), "a value is required") {
		t.Errorf("err = %v, want a missing-value error", err)
	}
}

// A misspelled field is a typo, not a missing argument, so it must say so
// rather than asking for a value it would then reject.
func TestSetReportsAnUnknownFieldBeforeAskingForAValue(t *testing.T) {
	dir := storeWithEndpoint(t, modelListServer(t).URL)
	withEntry(t, dir, registry.Model{Name: "fast", Endpoint: "gw", Model: "old"})

	_, err := runIn(t, "--config", dir, "set", "fast", "modl")
	if err == nil || !strings.Contains(err.Error(), `unknown field "modl"`) {
		t.Errorf("err = %v, want an unknown-field error", err)
	}
}
