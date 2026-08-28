package cmd

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/mikiobraun/mf/internal/store"
	"github.com/spf13/cobra"
)

// pickPageSize caps how many candidates are listed at once. A gateway can
// report several hundred models, and printing all of them scrolls the prompt
// off the screen.
const pickPageSize = 20

// errNoSelection ends a picker that was cancelled or ran out of input.
var errNoSelection = errors.New("no model selected")

// pickModelOnEndpoint asks the endpoint what it serves and prompts for one.
func pickModelOnEndpoint(cmd *cobra.Command, s *store.Store, endpoint string) (string, error) {
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

// pickModel prompts for one of ids and returns it.
//
// The protocol is line-based: a number picks, an empty line gives up, and
// anything else filters. That needs no raw mode and no terminal, so a piped
// stdin drives it the same way a person does and the tests can supply a
// string. The listing goes to stderr, leaving stdout for whatever the command
// itself prints.
func pickModel(cmd *cobra.Command, endpoint string, ids []string) (string, error) {
	if len(ids) == 0 {
		return "", fmt.Errorf("%s reports no models to choose from", endpoint)
	}
	// A command built by hand rather than executed has no context, and a nil
	// one cannot be selected on.
	ctx := cmd.Context()
	if ctx == nil {
		ctx = context.Background()
	}
	out := cmd.ErrOrStderr()
	lines := readLines(cmd.InOrStdin())

	noun := "models"
	if len(ids) == 1 {
		noun = "model"
	}
	fmt.Fprintf(out, "%d %s on %s.\nType a filter to narrow, a number to pick, or enter alone to cancel.\n\n",
		len(ids), noun, endpoint)

	matches := ids
	listMatches(out, matches)
	for {
		fmt.Fprint(out, "\n> ")

		var line string
		select {
		case <-ctx.Done():
			// main turns SIGINT into a cancelled context, which reaches the
			// picker here rather than through the blocked read.
			fmt.Fprintln(out)
			return "", errNoSelection
		case l, more := <-lines:
			if !more {
				// End of input, from ctrl-D or an exhausted pipe. The cursor
				// is still on the prompt line.
				fmt.Fprintln(out)
				return "", errNoSelection
			}
			line = l
		}

		text := strings.TrimSpace(line)
		if text == "" {
			return "", errNoSelection
		}
		switch n, numeric := pickNumber(text); {
		case numeric && (n < 1 || n > len(matches)):
			fmt.Fprintf(out, "pick a number between 1 and %d, or type a filter\n", len(matches))
		case numeric:
			return matches[n-1], nil
		default:
			// Filters apply to the full list, so typing a new one replaces
			// the old rather than narrowing what is already narrowed.
			found := filterIDs(ids, text)
			if len(found) == 0 {
				fmt.Fprintf(out, "nothing matches %q\n", text)
				continue
			}
			matches = found
			listMatches(out, matches)
		}
	}
}

// readLines pumps input into a channel, so that the picker can wait on the
// context as well as on the reader. A read blocks until the user types, and a
// select cannot watch a blocking read directly.
//
// The goroutine outlives a picker that returned early, parked on a read that
// will never complete. It holds nothing but stdin and the process exits moments
// later, so there is nothing to unwind.
func readLines(r io.Reader) <-chan string {
	ch := make(chan string)
	go func() {
		defer close(ch)
		br := bufio.NewReader(r)
		for {
			line, err := br.ReadString('\n')
			if line != "" {
				ch <- line
			}
			if err != nil {
				return
			}
		}
	}()
	return ch
}

// pickNumber reports whether text is a plain positive number, so that a filter
// like "4o" is not mistaken for a selection.
func pickNumber(text string) (int, bool) {
	for _, r := range text {
		if r < '0' || r > '9' {
			return 0, false
		}
	}
	n, err := strconv.Atoi(text)
	return n, err == nil
}

// listMatches prints one page of candidates, numbered from 1. Numbering runs
// over every match, so a number past the page is still selectable.
func listMatches(w io.Writer, matches []string) {
	shown := matches
	if len(shown) > pickPageSize {
		shown = shown[:pickPageSize]
	}
	width := len(strconv.Itoa(len(matches)))
	for i, id := range shown {
		fmt.Fprintf(w, "  %*d  %s\n", width, i+1, id)
	}
	if rest := len(matches) - len(shown); rest > 0 {
		fmt.Fprintf(w, "  (%d more; type a filter to narrow)\n", rest)
	}
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
