# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Commands

    go build -o mf .                                  # build the CLI
    go test ./...                                     # full suite
    go test ./internal/store -run TestRoundTrip -v    # one test
    go vet ./...

The suite is hermetic. The store and resolver run against `t.TempDir()`, the
provider runs against `httptest` servers, and the picker is driven as a
bubbletea model rather than as a running program, so no test needs a network, a
key, or a terminal. Keep it that way: a test that reaches a real endpoint
cannot run in CI, and one that wants a pty cannot either.

To exercise the binary by hand without touching your real registry, point
`MF_CONFIG_DIR` at a scratch directory:

    MF_CONFIG_DIR=/tmp/mf-scratch ./mf endpoint add local http://localhost:11434/v1

## Architecture

Four packages, layered, with dependencies pointing one way only:

    cmd  ──▶  resolve  ──▶  store      (registry lookups; no network)
     └───▶  provider  ──▶  store      (HTTP; no registry mutation)

- **`store`** owns the two files in the config dir: `models.toml` (endpoints and
  model entries, mode 0644) and `models.secrets` (API keys, mode 0600). Opening
  a missing dir or missing file yields an empty store rather than an error, so
  first-run commands work. Writes go to a temp file in the same directory and
  are renamed into place, so an interrupted write cannot truncate a good config.
  `Save()` and `SaveSecrets()` are separate — a command that changes both must
  call both, and `Save()` re-marshals from memory, discarding hand-written
  comments and formatting.
- **`resolve`** turns a user-supplied address into a `Ref` (endpoint, model ID,
  key). It is pure lookup over a loaded store.
- **`provider`** is the HTTP layer behind the `Client` interface
  (`ListModels`, `Chat`). `New` switches on the endpoint's dialect. Only the
  OpenAI wire format is implemented; `anthropic` parses and round-trips but
  returns a clear error when a client is built. Adding a dialect means
  implementing `Client` and adding a case to that switch.
- **`cmd`** is the cobra tree. Each command is a `newXCmd()` constructor
  registered in `NewRootCmd`. `pick.go` holds the model picker, a bubbletea
  model that draws on stderr. It needs a terminal and returns an error without
  one, so the tests drive the model directly with `tea.KeyMsg` values instead
  of running a program; `isTerminal` is a package variable so either branch can
  be taken under test.

Importing bubbletea has a cost that reaches every command, not only the
picker. Its package `init()` calls `lipgloss.HasDarkBackground()`, which sends
`OSC 11` and then `CSI 6n` to the terminal and waits. A terminal that answers
either one costs nothing measurable, and `TERM` starting with `screen`, `tmux`
or `dumb` skips the query outright, but a terminal answering neither costs
`termenv.OSCTimeout`, five seconds, on `mf list` as much as on `mf add`.
Measured at 0.00s against an answering pty and 5.01s against a silent one.

## Invariants

These are load-bearing, and tests fail if they are broken.

**The endpoint URL is used verbatim.** An endpoint's `url` is the API base. mf
appends only spec paths (`/models`, `/chat/completions`) and strips a trailing
slash. It adds no version segment, so a `/v1` must be part of the configured
URL, and a gateway can serve the API under `/openai/v1` or `/api`. A 404 or a
non-JSON 200 is reported as a probable base URL problem, naming the URL that
was actually requested. Guarded by `TestBaseURLIsVerbatim` and
`TestNonV1BasePathIsHonoured`.

**Addressing precedence** in `resolve.Resolve`: an alias matching a `[[model]]`
name wins first, then `endpoint/model` split on the **first** slash (so model
IDs may contain slashes), then `@tag`, which takes the first match and reports
the rest in `Ref.TagMatches` for the caller to note on stderr.

**Key precedence**: `MF_KEY_<ENDPOINT>` (name uppercased, non-alphanumerics to
`_`), then a model-scoped secret, then an endpoint-scoped secret. Only entries
addressed by alias can pick up a model-scoped secret — an `endpoint/model`
reference falls through to the endpoint key. A missing key is not an error at
resolve time, since a local ollama needs none; callers decide.

**Argument conventions**, enforced by meta-tests in `internal/cmd/args_test.go`
that walk the whole command tree:

- Every runnable command sets `Args`, built from an `argHelp{}` literal.
  `TestNoCommandUsesTheCobraDefaultArgs` fails on a command that omits it, so
  do not reach for `cobra.ExactArgs`.
- A value that is always required is a positional, never a flag. Flags are for
  genuinely optional values, and `TestNoCommandHasARequiredFlag` fails on any
  flag whose usage string contains "required".
- `argHelp` produces the count-mismatch error, which names each argument and
  shows a worked command line rather than printing counts.
- An omitted optional positional is the trigger for interactive input, not a
  flag. `mf add <name> <endpoint>` and `mf set <name> model` open the picker
  rather than taking an `--interactive`.

## Output conventions

Machine-readable output goes to `cmd.OutOrStdout()`; notes, warnings, and
"nothing configured" hints go to `cmd.ErrOrStderr()`, so piping stdout stays
clean. Errors returned from `RunE` are printed once by `main` with an `mf: `
prefix; return `errSilent` when a command has already printed its own failure
lines and only needs the non-zero exit (`check` does this).

## Not yet implemented

The `anthropic` wire format, and `mf import` / `mf export` / `mf configure`.
