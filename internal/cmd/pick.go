package cmd

import (
	"errors"
	"fmt"
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/mikiobraun/mf/registry"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

// The list grows with the terminal between these bounds. Fewer than five lines
// is too little to explore with, and more than ten pushes the filter field
// away from where the eye already is.
const (
	pickMinRows = 5
	pickMaxRows = 10
	// pickChrome counts the lines the picker draws besides the list: the
	// header, the key hints, the blank line under them, the blank line above
	// the filter field, and the field itself.
	pickChrome = 5
)

var (
	headStyle   = lipgloss.NewStyle().Bold(true)
	hintStyle   = lipgloss.NewStyle().Faint(true)
	cursorStyle = lipgloss.NewStyle().Bold(true)
	caretStyle  = lipgloss.NewStyle().Reverse(true)
)

// errNoSelection ends a picker that was cancelled.
var errNoSelection = errors.New("no model selected")

// isTerminal reports whether there is a terminal to draw the picker on. It is
// a variable so that a test can take either branch without one.
var isTerminal = func() bool { return term.IsTerminal(int(os.Stdin.Fd())) }

// pickModelOnEndpoint asks the endpoint what it serves and prompts for one.
func pickModelOnEndpoint(cmd *cobra.Command, s *registry.Store, endpoint string) (string, error) {
	client, err := clientForEndpoint(s, endpoint)
	if err != nil {
		return "", err
	}
	ids, err := client.ListModels(cmd.Context())
	if err != nil {
		return "", fmt.Errorf("listing models on %s: %w", endpoint, err)
	}
	return pickModel(cmd, endpoint, ids)
}

// pickModel runs the interactive list and returns the chosen model ID.
//
// It draws on stderr, leaving stdout for whatever the command itself prints.
func pickModel(cmd *cobra.Command, endpoint string, ids []string) (string, error) {
	if len(ids) == 0 {
		return "", fmt.Errorf("%s reports no models to choose from", endpoint)
	}
	if !isTerminal() {
		return "", fmt.Errorf("picking a model needs a terminal; pass the model ID as an argument instead")
	}

	opts := []tea.ProgramOption{tea.WithInput(os.Stdin), tea.WithOutput(cmd.ErrOrStderr())}
	if ctx := cmd.Context(); ctx != nil {
		// Raw mode delivers ctrl-C as a key, but a SIGINT sent from elsewhere
		// still arrives as a cancelled context.
		opts = append(opts, tea.WithContext(ctx))
	}
	final, err := tea.NewProgram(newPicker(endpoint, ids), opts...).Run()
	if err != nil {
		if errors.Is(err, tea.ErrProgramKilled) || errors.Is(err, tea.ErrInterrupted) {
			return "", errNoSelection
		}
		return "", fmt.Errorf("running the picker: %w", err)
	}
	p, ok := final.(picker)
	if !ok || p.chosen == "" {
		return "", errNoSelection
	}
	return p.chosen, nil
}

// picker is the model behind the interactive list. Typing filters the list as
// each key lands, the arrow keys move a cursor through what survives, and
// enter takes what the cursor is on.
type picker struct {
	endpoint string
	ids      []string // everything the endpoint reported, never filtered
	matches  []string // the ids containing filter
	filter   string
	cursor   int // index into matches
	offset   int // index of the first match on screen
	rows     int // how many matches fit
	width    int // terminal columns, 0 until the first size message
	chosen   string
	done     bool
}

func newPicker(endpoint string, ids []string) picker {
	return picker{endpoint: endpoint, ids: ids, matches: ids, rows: pickMinRows}
}

func (p picker) Init() tea.Cmd { return nil }

func (p picker) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		p.width = msg.Width
		p.rows = viewportRows(msg.Height)
		p.scroll()
	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyCtrlC, tea.KeyEsc:
			p.done = true
			return p, tea.Quit
		case tea.KeyEnter:
			if len(p.matches) > 0 {
				p.chosen = p.matches[p.cursor]
			}
			p.done = true
			return p, tea.Quit
		case tea.KeyUp, tea.KeyCtrlP:
			p.move(-1)
		case tea.KeyDown, tea.KeyCtrlN:
			p.move(1)
		case tea.KeyBackspace:
			if r := []rune(p.filter); len(r) > 0 {
				p.setFilter(string(r[:len(r)-1]))
			}
		case tea.KeySpace:
			p.setFilter(p.filter + " ")
		case tea.KeyRunes:
			p.setFilter(p.filter + string(msg.Runes))
		}
	}
	return p, nil
}

func (p picker) View() string {
	if p.done {
		// Drawing nothing makes bubbletea erase the picker, so only the
		// command's own output is left behind.
		return ""
	}

	var b strings.Builder
	head := fmt.Sprintf("%d models on %s", len(p.ids), p.endpoint)
	if p.filter != "" {
		head = fmt.Sprintf("%d of %d models on %s", len(p.matches), len(p.ids), p.endpoint)
	}
	b.WriteString(headStyle.Render(head))
	b.WriteByte('\n')
	b.WriteString(hintStyle.Render("↑↓ move · enter selects · esc cancels · type to filter"))
	b.WriteString("\n\n")

	// Every row is written, empty ones included, so the filter field holds
	// still as the match count changes under it.
	for i := 0; i < p.rows; i++ {
		n := p.offset + i
		switch {
		case len(p.matches) == 0:
			if i == 0 {
				b.WriteString(hintStyle.Render("  nothing matches"))
			}
		case n >= len(p.matches):
		case n == p.cursor:
			b.WriteString(cursorStyle.Render("> " + fit(p.matches[n], p.width-2)))
		default:
			b.WriteString("  " + fit(p.matches[n], p.width-2))
		}
		b.WriteByte('\n')
	}

	b.WriteByte('\n')
	b.WriteString(hintStyle.Render(">") + " " + p.filter + caretStyle.Render(" "))
	return b.String()
}

// setFilter re-runs the filter over the whole list and starts from the top,
// since a cursor position means nothing against a new set of matches.
func (p *picker) setFilter(f string) {
	p.filter = f
	p.matches = filterIDs(p.ids, f)
	p.cursor = 0
	p.offset = 0
}

// move walks the cursor, stopping at either end rather than wrapping.
func (p *picker) move(d int) {
	if len(p.matches) == 0 {
		return
	}
	p.cursor = clamp(p.cursor+d, 0, len(p.matches)-1)
	p.scroll()
}

// scroll shifts the window the least distance that brings the cursor back into
// view, and keeps it from running past the end of a short list.
func (p *picker) scroll() {
	if p.cursor < p.offset {
		p.offset = p.cursor
	}
	if p.cursor >= p.offset+p.rows {
		p.offset = p.cursor - p.rows + 1
	}
	p.offset = clamp(p.offset, 0, max(0, len(p.matches)-p.rows))
}

// viewportRows fits the list to the terminal, within the bounds.
func viewportRows(termHeight int) int {
	return clamp(termHeight-pickChrome, pickMinRows, pickMaxRows)
}

func clamp(v, lo, hi int) int {
	return min(max(v, lo), hi)
}

// fit shortens s to w columns, marking the cut. A width of zero means the
// terminal size is not known yet, so nothing is cut.
func fit(s string, w int) string {
	if w <= 0 {
		return s
	}
	r := []rune(s)
	if len(r) <= w {
		return s
	}
	if w == 1 {
		return "…"
	}
	return string(r[:w-1]) + "…"
}

// filterIDs keeps the ids containing q, case-insensitively.
func filterIDs(ids []string, q string) []string {
	q = strings.ToLower(q)
	var out []string
	for _, id := range ids {
		if strings.Contains(strings.ToLower(id), q) {
			out = append(out, id)
		}
	}
	return out
}
