# TODO

Where `mf` might go next. These are notes, not designs.

## More `mf run` clients, and `mf configure`

`mf run opencode` exists. Each client is a subcommand with the same shape:
the main model is a positional, further models to offer are more positionals,
and the client's own optional roles are flags (`--small` for opencode; aider
would have `--weak` and `--editor`). Only the named models are configured,
never the whole registry.

Next clients: aider, Claude Code, pi. Each is Go code for now. A template or
data format for client configs should be extracted once a few exist and the
common shape is visible, not designed up front.

`mf configure <client> <models...>` writes the same config to a file instead
of passing it at launch. Open questions:

- keys: a written file cannot rely on mf setting the environment, so either
  the key goes into the file, the user sets `MF_KEY_*` themselves, or it goes
  into the client's own secret store (opencode's `auth.json`)
- which file: merging into the user's global config means JSONC parsing and
  losing their comments; an mf-owned file needs the client to be pointed at it
- re-running must replace what mf wrote before without touching the user's
  own providers

## More endpoint types

Two separate pieces of work.

New wire formats: `anthropic`, Google Cloud, AWS Bedrock.

An endpoint picks its format with the `api` key in `models.toml`, set by
`mf endpoint add --api`. Only `openai` has a client, so the key is undocumented
until a second format works. `anthropic` is the one value that parses today,
and `provider.New` returns a clear error for it.

Preconfigured OpenAI-compatible endpoints, so that `mf endpoint add openrouter`
fills in the URL by itself: DigitalOcean, OpenRouter, and whichever others are
worth shipping a default for.

## `mf import` / `mf export`

What each of these should do is still open.
