# Feedback

## 2026-09-30 — using mf to run an ofc smoke test (Claude Code session)

**Use case:** an agent needed a working endpoint, model and key to run
`ofc run` against a live model. It found `groq/qwen38` with `mf list | grep`,
then passed the settings in as env vars via three calls:
`$(mf get groq/qwen38 url)`, `… model`, `… key`. That worked, and the key
never landed in a file or in output.

### 1. `mf exec <name> -- <cmd>`

Run a command with the model's settings in its environment, e.g.
`OPENAI_BASE_URL`, `OPENAI_API_KEY`, `OPENAI_MODEL`, with an optional
`--prefix SMOKE_` for tools that read their own variable names.

- One call instead of three `$(mf get …)` substitutions.
- The key never appears on a command line or in shell history — which
  matters for agents, whose commands end up in transcripts.

**Implemented** as `mf exec <name> [--prefix P] -- <cmd>...`. mf replaces
itself with the command (`syscall.Exec`) and sets `OPENAI_BASE_URL`,
`OPENAI_MODEL` and `OPENAI_API_KEY`, dropping any inherited values. When no
key is configured, `OPENAI_API_KEY` is removed, so a key from the shell cannot
reach the wrong endpoint. `--prefix SMOKE_` replaces `OPENAI_`
(`SMOKE_BASE_URL`, …). The `--` is required. Endpoints with a non-`openai` api
are rejected. Per-tool variable mappings are left for `mf run` in TODO.md.

### 2. `mf check --generate`

`mf check` lists the endpoint's models but does not generate tokens, so it
reports models that are listed but unusable as fine. Before switching to mf,
four models on a local Ollama endpoint all appeared in `/models` and all
failed on a real request: two with `410 Gone` (retired) and two with
`402 Payment Required`.

An opt-in flag that sends a one-token completion per model and reports
`OK` / `retired (410)` / `needs credits (402)` / other errors would have
found a working model immediately. Opt-in because it costs a request per
model.

**Not implemented yet.** Two notes from reviewing it: `check` currently checks
each endpoint once and does not even confirm the model ID is in `/models`,
which would be a cheap default improvement. Mapping 410/402 to phrases needs
provider errors to carry the HTTP status; today only `AuthError` is typed.

### 3. `mf list --json`

Machine-readable output so scripts and agents don't have to grep the
table.

**Implemented** as `mf list --json` (an array) and `mf get <name> --json`
(one object), both with `{name, endpoint, url, api, model, tags, has_key}`.
The key is never included; `has_key` says whether one is configured. `tags`
is always an array. `mf get` takes either a field or `--json`, not both.
