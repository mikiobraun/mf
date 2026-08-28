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

[[endpoint]]
name = 'anthropic'
url  = 'https://api.anthropic.com'
api  = 'anthropic'                      # omitted entirely when 'openai'

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
| | `api` | no | `openai` (default) or `anthropic` |
| `[[model]]` | `name` | yes | the alias; wins over an `endpoint/model` reading of the same string |
| | `endpoint` | yes | an `[[endpoint]]` `name` |
| | `model` | yes | the provider's model ID, which may itself contain slashes |
| | `tags` | no | addressable as `@tag`, filterable with `--tags` |

An absent optional key means the default, never a broken file. `api` is written
only when it is not `openai`, and `tags` only when the entry has any.

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
| `mf list [--tags a,b]` | list configured models (`--tags` requires *all* listed tags) |
| `mf endpoint add <name> <url> [--key K] [--api openai\|anthropic]` | add an endpoint; prompts for the key |
| `mf endpoint remove <name> [-f]` | remove an endpoint, its models, and its secrets |
| `mf endpoint list` | list endpoints, model counts, and whether a key is set |
| `mf add <name> <endpoint> [<model>] [--tags a,b] [--key K]` | add a model entry; omit the ID to [pick one](#picking-a-model-id) |
| `mf remove <name>` | remove a model entry, leaving the endpoint |
| `mf get <name> <field>` | print `endpoint`, `url`, `key`, `model`, `tags`, or `api` |
| `mf set <name> <field> [<value>]` | update `endpoint`, `model`, `tags`, or `key` |
| `mf models <endpoint>` | model IDs the endpoint reports |
| `mf sync <endpoint> [--tags a,b]` | upsert every reported model into `models.toml` |
| `mf check [<name>] [--all]` | endpoint reachable and key accepted; exits non-zero on failure |
| `mf ask <name> <message>` | one-shot message, for smoke-testing |

### Picking a model ID

Leave the model ID out and `mf` asks the endpoint what it serves, then prompts.
This works on `mf add <name> <endpoint>` and on `mf set <name> model`.

Type a filter to narrow the list, or a number to pick from it. A filter always
applies to the whole list, so typing a second one replaces the first rather
than narrowing further. Long lists show the first 20 entries, and the numbering
covers every match, so a number past the page still works.

```
$ mf add fast bifrost
42 models on bifrost.
Type a filter to narrow, a number to pick, or enter alone to cancel.

   1  anthropic/claude-opus-4
   2  anthropic/claude-sonnet-4
  ...
  20  openai/gpt-4o
  (22 more; type a filter to narrow)

> gemini
  1  gemini/gemini-2.5-flash
  2  gemini/gemini-2.5-pro
  3  gemini/gemini-2.0-flash

> 1
added fast -> gemini/gemini-2.5-flash on bifrost
```

Three things leave without picking: an empty line, ctrl-C, and ctrl-D. All
three print `mf: no model selected` and exit non-zero, and nothing is written.
An empty line is the documented way out because it cannot be confused with a
filter, an empty filter being one that matches everything.

The prompt reads lines rather than keystrokes, so it needs no terminal and a
pipe drives it the same way a person does. Input that runs out before a choice
counts as a cancel, so a scripted `mf add` that forgot its model ID fails
instead of picking something.

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

## Argument conventions

A value that is always required is a positional argument. Flags are reserved
for values you can omit:

```
mf endpoint add <name> <url> [--key K] [--api openai|anthropic]
mf add <name> <endpoint> [<model>] [--tags a,b] [--key K]
```

`<model>` is bracketed because omitting it is a real alternative rather than a
mistake: `mf` then [asks the endpoint](#picking-a-model-id) and prompts.

Supplying the wrong number of arguments prints what each one is, plus an
example, rather than a count:

```
$ mf get fast
mf: get takes 2 arguments, you gave 1 argument

usage:
  mf get <name> <field>

  <name>   model to address: an alias (fast), an endpoint/model reference
           (bifrost/gemini/gemini-2.5-flash), or a @tag (@coding)
  <field>  field to print: endpoint, url, api, key, model, tags

example:
  mf get fast url
```

## Environment

| Variable | Effect |
|---|---|
| `MF_CONFIG_DIR` | override the config directory |
| `XDG_CONFIG_HOME` | used as `$XDG_CONFIG_HOME/mf` when `MF_CONFIG_DIR` is unset |
| `MF_KEY_<ENDPOINT>` | override an endpoint's API key |

`--config <dir>` overrides both.

## Status

Implemented: the full store, all three addressing forms, the model picker, and
the network commands (`models`, `sync`, `check`, `ask`) against
**OpenAI-compatible** endpoints — `GET {url}/models`,
`POST {url}/chat/completions`, bearer auth.

Not yet implemented:

- the `anthropic` wire format — the field parses and round-trips, but building
  a client for it returns a clear error
- `mf import` / `mf export` / `mf configure`

## Development

    go test ./...
    go build -o mf .

Tests are hermetic: the store and resolver run against temp dirs, and the
provider runs against `httptest` servers. Nothing in the suite needs a network
or a real API key.
