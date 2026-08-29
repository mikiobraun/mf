# TODO

Where `mf` might go next. These are notes, not designs.

## `mf run <tool> <models...>`

Launch a client — Claude Code, pi, Cursor — against specific entries from the
registry, either by setting the environment variables it reads or by writing
the config file it expects.

`OPENAI_BASE_URL` and its neighbours are the lever most tools already
understand, and setting them by hand is what a bifrost, litellm, or OpenRouter
user does today. `mf run` would set them from a registry entry instead.

The per-tool part should be data rather than code: a table describing what each
client wants, so that supporting a new one is an entry rather than a commit.

Open: what those configs actually look like. This needs a survey of the clients
worth supporting before the shape of the data can be decided.

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

## A client library for `models.toml`

A package other tools import to read the registry, instead of each one parsing
the file itself.

Open: whether this earns its keep. `models.toml` is a small TOML document with
a documented shape, and a client that already has a TOML parser may be better
served reading it directly than taking on a dependency. Worth deciding before
building.

## `mf import` / `mf export` / `mf configure`

What each of these should do is still open.
