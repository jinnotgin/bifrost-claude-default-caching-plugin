# Claude Cache Control Plugin for Bifrost

This plugin intercepts normalized `BifrostResponsesRequest` traffic in `PreLLMHook`, detects Anthropic-family Claude targets, converts the normalized request into an Anthropic Messages request with `anthropic.ToAnthropicResponsesRequest(...)`, injects top-level `cache_control`, then enables raw request body passthrough with `schemas.BifrostContextKeyUseRawRequestBody`.

## What it does

- Accepts standard `/v1/responses` traffic on the Bifrost side.
- Runs **after** Bifrost has normalized and routed the request.
- Only acts on configured providers, defaulting to `anthropic` and `vertex`.
- Only acts on Claude-like models.
- Adds top-level `cache_control` for Anthropic Messages payloads.
- Leaves non-Claude and non-Anthropic-family requests untouched.

## Default behavior

The default config injects:

```json
{"cache_control":{"type":"ephemeral"}}
```

You can optionally add:

- `ttl`: for example `"1h"`
- `scope`: for example `"user"` or `"global"`

## Why this design

This plugin avoids mutating the inbound OpenAI-style `/v1/responses` payload. Instead it waits until Bifrost has selected a provider and then synthesizes the final Anthropic request body. That keeps Anthropic-native fields at the provider boundary, where they belong.

## Notes

- Build this plugin with the **exact same Go toolchain and `github.com/maximhq/bifrost/core` version** as the Bifrost binary that will load it.
- `strip_scope_on_vertex` is enabled by default so that if a scope is configured later, the marshal path can strip it for Vertex.
- The plugin intentionally skips requests that already have `RawRequestBody` set, unless you disable that guard.

## Build

```bash
make build
```

## Example config

See `config.example.json`.
