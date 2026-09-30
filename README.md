# mf — Model File CLI

`mf` manages a local, file-based registry of LLM model configurations, so tools
can read a single source of truth instead of each keeping their own config.

    ~/.config/mf/models.toml      model definitions  (shareable)
    ~/.config/mf/models.secrets   API keys           (mode 0600, never shared)

## Install

    go install github.com/mikiobraun/mf@latest

## Quick start

    mf endpoint add bifrost https://gw.example.com/v1   # prompts for the key
                                                              # note: url includes /v1
    mf models bifrost                                         # what's available
    mf add fast bifrost gemini/gemini-2.5-flash --tags coding,cheap
    mf check
    mf ask fast "say hello"

## The files

Both files are TOML arrays of tables, and both are safe to hand-edit. `mf`
reads them on every command and rewrites them whenever a command changes
something.

### `models.toml`

```toml
[[endpoint]]
name = 'bifrost'                        # referenced by [[model]].endpoint
url  = 'https://gw.example.com/v1'  # API base, including any version prefix

[[model]]
name     = 'fast'                       # the alias you address
endpoint = 'bifrost'                    # must name an [[endpoint]]
model    = 'gemini/gemini-2.5-flash'    # the provider's own model ID
tags     = ['coding', 'cheap']          # omitted entirely when empty
```

| Table | Key | Required | Notes |
|---|---|---|---|
| `[[endpoint]]` | `name` | yes | no slashes; a slash would be ambiguous with `endpoint/model` addressing |
| | `url` | yes | see [What `url` means](#what-url-means) |
| `[[model]]` | `name` | yes | the alias; wins over an `endpoint/model` reading of the same string |
| | `endpoint` | yes | an `[[endpoint]]` `name` |
| | `model` | yes | the provider's model ID, which may itself contain slashes |
| | `tags` | no | addressable as `@tag`, filterable with `--tags` |

An absent optional key means the default, never a broken file. `tags` is
written only when the entry has any.

### `models.secrets`

Same shape, one table, written with mode 0600. The scope of a key is decided by
whether `model` is present:

```toml
[[secret]]
endpoint = 'bifrost'
key      = 'sk-endpoint-wide'   # no `model` key: used by every model on bifrost

[[secret]]
endpoint = 'bifrost'
model    = 'smart'              # with `model`: used by that entry only
key      = 'sk-just-for-smart'
```

`model` names a `[[model]]` `name`, not a provider model ID. Which of the two
scopes applies to a given request is covered under
[Key resolution](#key-resolution).

### Hand-editing

Writes are atomic — `mf` writes a temp file in the same directory and renames it
over the target, so an interrupted command cannot truncate a good config.

Rewrites are **not** lossless. The file is re-marshalled from memory, so
comments, key order, and hand-applied formatting are dropped the next time any
command writes. Keep notes you want to survive outside these files.

## Addressing models

Three forms, tried in this order:

| Form | Example | Notes |
|---|---|---|
| alias | `fast` | matches a `[[model]]` `name` |
| endpoint/model | `bifrost/gemini/gemini-2.5-flash` | splits on the **first** slash, so model IDs may contain slashes |
| tag | `@coding` | resolves to the first match; others are noted on stderr |

## Commands

| Command | Purpose |
|---|---|
| `mf list [--tags a,b] [--json]` | list configured models (`--tags` requires *all* listed tags) |
| `mf endpoint add <name> <url> [--key K]` | add an endpoint; prompts for the key |
| `mf endpoint remove <name> [-f]` | remove an endpoint, its models, and its secrets |
| `mf endpoint list` | list endpoints, model counts, and whether a key is set |
| `mf add <name> <endpoint> [<model>] [--tags a,b] [--key K]` | add a model entry; omit the ID to [pick one](#picking-a-model-id) |
| `mf remove <name>` | remove a model entry, leaving the endpoint |
| `mf get <name> <field>` | print `endpoint`, `url`, `api`, `key`, `model`, or `tags` |
| `mf get <name> --json` | print the whole entry as [JSON](#json-output), without the key |
| `mf set <name> <field> [<value>]` | update `endpoint`, `model`, `tags`, or `key` |
| `mf models <endpoint>` | model IDs the endpoint reports |
| `mf sync <endpoint> [--tags a,b]` | upsert every reported model into `models.toml` |
| `mf check [<name>] [--all]` | endpoint reachable and key accepted; exits non-zero on failure |
| `mf ask <name> <message>` | one-shot message, for smoke-testing |
| `mf exec <name> [--prefix P] -- <cmd>...` | run a command with the model's [settings in its environment](#mf-exec) |

### Picking a model ID

Leave the model ID out and `mf` asks the endpoint what it serves, then opens a
picker. This works on `mf add <name> <endpoint>` and on `mf set <name> model`.

The list filters as you type, matching case-insensitively anywhere in the ID.
`↑` and `↓` move the `>` cursor and scroll the window, enter takes whatever the
cursor is on, and backspace deletes the last character of the filter. The
window holds between five and ten entries, following the terminal height.

```
$ mf add fast bifrost
43 models on bifrost
↑↓ move · enter selects · esc cancels · type to filter

> anthropic/claude-opus-4
  anthropic/claude-sonnet-4
  deepseek/deepseek-v3
  gemini/gemini-2.0-flash
  gemini/gemini-2.1-flash

>
```

After typing `gem` and pressing `↓` once:

```
5 of 43 models on bifrost
↑↓ move · enter selects · esc cancels · type to filter

  gemini/gemini-2.0-flash
> gemini/gemini-2.1-flash
  gemini/gemini-2.2-flash
  gemini/gemini-2.3-flash
  gemini/gemini-2.4-flash

> gem
```

Escape and ctrl-C leave without picking. Both print `mf: no model selected`
and exit non-zero, and nothing is written.

The picker draws on stderr, so a piped stdout stays clean. It needs a terminal
and says so rather than half-drawing into a pipe:

```
$ mf add fast bifrost < /dev/null
mf: picking a model needs a terminal; pass the model ID as an argument instead
```

### What `url` means

An endpoint's `url` is the API base. mf appends only the spec paths `/models`
and `/chat/completions`, and strips a trailing slash. It adds no version
segment, so a `/v1` must be part of the configured URL, and a gateway can
serve the API under `/openai/v1` or `/api`.

```toml
url = "https://gw.example.com/v1"   # correct
url = "https://gw.example.com"      # 404s: no /models at the root
```

If the prefix is missing, requests 404 and mf names the URL it tried.

### Key resolution

Highest precedence first:

1. `MF_KEY_<ENDPOINT>` — endpoint name uppercased, non-alphanumerics become `_`
   (`my-gateway` → `MF_KEY_MY_GATEWAY`)
2. a model-scoped secret in `models.secrets`
3. an endpoint-scoped secret in `models.secrets`

Only entries addressed by alias can pick up a model-scoped secret; an
`endpoint/model` reference falls straight through to the endpoint key.

### `mf sync`

Entries are named after the provider's model ID. Sync is idempotent: it adds
what is missing, merges `--tags` into entries it already owns, and **never
rewrites an entry whose name is an alias for something else** — those are
reported as skipped on stderr. Models that disappear upstream are not deleted.

### `mf exec`

`mf exec fast -- ofc run` replaces mf with `ofc run`, with three variables set:

| Variable | Value |
|---|---|
| `OPENAI_BASE_URL` | the endpoint URL |
| `OPENAI_MODEL` | the model ID |
| `OPENAI_API_KEY` | the key |

Inherited values of these variables are dropped. When no key is configured,
`OPENAI_API_KEY` is removed from the environment, so a key from the shell
cannot reach the wrong endpoint. `--prefix SMOKE_` replaces `OPENAI_`, and
sets `SMOKE_BASE_URL`, `SMOKE_MODEL`, and `SMOKE_API_KEY`. Only endpoints with
the `openai` api are accepted.

### JSON output

`mf list --json` prints an array, and `mf get <name> --json` prints one object:

```json
{
  "name": "fast",
  "endpoint": "bifrost",
  "url": "https://gw.example.com/v1",
  "api": "openai",
  "model": "gemini/gemini-2.5-flash",
  "tags": ["cheap"],
  "has_key": true
}
```

The key is never included. Use `mf get <name> key` or `mf exec` for it.

## Argument conventions

A value that is always required is a positional argument. Flags are reserved
for values you can omit:

```
mf endpoint add <name> <url> [--key K]
mf add <name> <endpoint> [<model>] [--tags a,b] [--key K]
```

`<model>` is bracketed because omitting it is a real alternative rather than a
mistake: `mf` then [asks the endpoint](#picking-a-model-id) and prompts.

Supplying the wrong number of arguments prints what each one is, plus an
example, rather than a count:

```
$ mf ask fast
mf: ask takes 2 arguments, you gave 1 argument

usage:
  mf ask <name> <message>

  <name>     model to address: an alias (fast), an endpoint/model reference
             (bifrost/gemini/gemini-2.5-flash), or a @tag (@coding)
  <message>  the message to send, as a single argument
             (quote it if it contains spaces)

example:
  mf ask fast "say hello"
```

## Environment

| Variable | Effect |
|---|---|
| `MF_CONFIG_DIR` | override the config directory |
| `XDG_CONFIG_HOME` | used as `$XDG_CONFIG_HOME/mf` when `MF_CONFIG_DIR` is unset |
| `MF_KEY_<ENDPOINT>` | override an endpoint's API key |

`--config <dir>` overrides both.

## Reading the registry from Go

The point of a shared registry is that other tools read it. Import
`github.com/mikiobraun/mf/registry` rather than parsing the files yourself, and
you get the addressing forms and the key precedence for free:

```go
import "github.com/mikiobraun/mf/registry"

dir, err := registry.DefaultDir()          // MF_CONFIG_DIR, XDG, ~/.config/mf
s, err := registry.Open(dir)               // missing files yield an empty store
ref, err := registry.Resolve(s, "fast")    // alias, endpoint/model, or @tag
key, ok := ref.Key(s)                      // env override, then model, then endpoint

// ref.Endpoint.URL is the API base, ref.ModelID the provider's model ID.
```

`registry` is the only package outside `internal/`; everything else is mf's own
business. It does no network I/O, so a client that only wants "which URL, which
key, which model ID" pays for a TOML parser and nothing else.

## Status

The network commands (`models`, `sync`, `check`, `ask`) speak to
**OpenAI-compatible** endpoints: `GET {url}/models`,
`POST {url}/chat/completions`, bearer auth.

## Development

    go test ./...
    go build -o mf .

Tests are hermetic: the store and resolver run against temp dirs, and the
provider runs against `httptest` servers. Nothing in the suite needs a network
or a real API key.
