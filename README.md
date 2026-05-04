# Claude Cache Control Plugin for Bifrost

This Bifrost plugin makes OpenAI-compatible Claude traffic more cache-friendly by adding Claude cache control during Bifrost routing.

[Bifrost](https://github.com/maximhq/bifrost) is a high-performance AI gateway that exposes a single OpenAI-compatible API across providers such as Anthropic, OpenAI, AWS Bedrock, and Google Vertex. This plugin is for Bifrost deployments where coding agents call OpenAI-compatible endpoints like `/v1/chat/completions` or `/v1/responses`, while Bifrost routes those requests to Claude through Anthropic-family providers. That makes Bifrost a useful place to add Claude-specific cache control centrally.

## Problem Statement

Many coding agents and agent runtimes are configured against OpenAI-compatible endpoints so they can swap providers without changing client code. That portability is useful, but it can hide provider-specific features. Claude prompt caching is one of those features: when an agent only knows it is sending OpenAI-style `/v1/chat/completions` or `/v1/responses` requests, it may not add the Anthropic cache control fields Claude needs, even when a large part of the prompt is stable across turns.

The result is unnecessary repeated prompt processing and higher Claude spend. This plugin applies a default cache control policy at the Bifrost provider boundary, after Bifrost has accepted the OpenAI-compatible request and selected a Claude-capable provider. The goal is to make OpenAI-compatible Claude traffic more cost efficient without requiring each coding agent to understand provider-native caching details.

It supports both OpenAI-compatible Bifrost request paths:

- `/v1/chat/completions`: sets Bifrost `ChatParameters.CacheControl` so the routed Claude provider request includes cache control.
- `/v1/responses`: converts Bifrost's normalized Responses request into the provider request body Bifrost will send to Claude, adds cache control, and enables raw body passthrough.

## What It Does

- Runs after Bifrost has normalized and routed the request.
- Acts only on configured providers, defaulting to `anthropic` and `vertex`.
- Acts only on Claude-like models, using `schemas.IsAnthropicModel(...)` plus configured model substring matches.
- Adds provider-native cache control for routed Claude requests that entered Bifrost through OpenAI-compatible endpoints.
- Leaves non-Claude models, unsupported providers, disabled configs, and unsupported request types untouched.
- Skips requests that already include cache control by default.
- Skips requests that already have `RawRequestBody` by default.

## How To Use

Use a plugin release that matches the Bifrost version you are running. Go plugins are loaded into the Bifrost process, so the Bifrost binary and the plugin must be built with compatible Go, OS, libc, architecture, and `github.com/maximhq/bifrost/core` versions.

Custom Bifrost plugins require a dynamically linked Bifrost binary. Bifrost's default static builds are portable, but static Go binaries cannot load `.so` plugins at runtime. See Bifrost's official guide: [Building Dynamically Linked Bifrost Binary](https://docs.getbifrost.ai/plugins/building-dynamic-binary).

The easiest path is:

1. Pick your Bifrost Docker/transport version.
2. Open the [plugin releases](https://github.com/jinnotgin/bifrost-claude-default-caching-plugin/releases).
3. Download the `.so` asset whose release tag and filename match your Bifrost version, Bifrost core version, libc target, and CPU architecture.
4. Mount the `.so` file into the Bifrost container.
5. Add the plugin entry to Bifrost's `config.json`.
6. Start Bifrost and send OpenAI-compatible Claude traffic through Bifrost as usual.

Example release tag:

```text
v0.1.1-bifrost-v1.4.24-core-v1.4.23
```

This means plugin version `0.1.1`, built for Bifrost Docker/transport `v1.4.24`, using Bifrost core module `v1.4.23`. That release provides Alpine/musl assets for `linux/amd64` and `linux/arm64`, and is intended to pair with:

```text
ghcr.io/jinnotgin/bifrost-dynamic-alpine:v1.4.24
```

## Using Bifrost Dynamic Docker

Standard Bifrost images are not always the right runtime for external Go plugins because plugin loading needs a dynamically linked Bifrost binary and is sensitive to the exact build environment. [bifrost-dynamic-docker](https://github.com/jinnotgin/bifrost-dynamic-docker) builds a dynamic/plugin-capable Alpine image for Bifrost, published as:

```text
ghcr.io/jinnotgin/bifrost-dynamic-alpine:<bifrost-version>
```

For Bifrost `v1.4.24`:

```bash
docker pull ghcr.io/jinnotgin/bifrost-dynamic-alpine:v1.4.24
```

Create a local Bifrost data directory:

```bash
mkdir -p ./bifrost-data/plugins
```

Download the matching plugin asset for your container architecture into `./bifrost-data/plugins/`. For example, on Linux ARM64 with Bifrost `v1.4.24`:

```bash
curl -L \
  -o ./bifrost-data/plugins/claude-cache-control-plugin.so \
  https://github.com/jinnotgin/bifrost-claude-default-caching-plugin/releases/download/v0.1.1-bifrost-v1.4.24-core-v1.4.23/claude-default-caching-linux-arm64-musl-bifrost-v1.4.24-core-v1.4.23.so
```

Add the plugin to `./bifrost-data/config.json`:

```json
{
  "plugins": [
    {
      "enabled": true,
      "name": "claude-cache-control-plugin",
      "path": "/app/data/plugins/claude-cache-control-plugin.so",
      "version": 1,
      "config": {
        "enabled": true,
        "providers": ["anthropic", "vertex"],
        "cache_control_type": "ephemeral",
        "ttl": "1h",
        "skip_if_request_has_cache": true,
        "skip_if_raw_request_body": true,
        "strip_scope_on_vertex": true
      }
    }
  ]
}
```

Run Bifrost:

```bash
docker run --rm \
  -p 8080:8080 \
  -v "$PWD/bifrost-data:/app/data" \
  ghcr.io/jinnotgin/bifrost-dynamic-alpine:v1.4.24
```

Open the Bifrost UI at `http://localhost:8080`, configure your Anthropic or Vertex provider, then send OpenAI-compatible Claude requests through Bifrost.

Example chat request:

```bash
curl -X POST http://localhost:8080/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "anthropic/claude-sonnet-4-5",
    "messages": [{"role": "user", "content": "Hello from Bifrost"}]
  }'
```

## Release Matching

Prefer release assets over local builds for Docker use. Each plugin release is labeled with the Bifrost versions it was built against. Match all of these:

| Match | Example |
| --- | --- |
| Bifrost Docker/transport version | `bifrost-v1.4.24` |
| Bifrost core module version | `core-v1.4.23` |
| libc target | `musl` for Alpine images |
| CPU architecture | `linux-amd64` or `linux-arm64` |

If you use `ghcr.io/jinnotgin/bifrost-dynamic-alpine:v1.4.24`, use a plugin release marked for Bifrost `v1.4.24`. If you use a different Bifrost version, choose the corresponding plugin release or build the plugin yourself with the same environment.

## Default Behavior

With no plugin config, the plugin is enabled and injects:

```json
{"cache_control":{"type":"ephemeral"}}
```

Default matching and guard settings:

```json
{
  "enabled": true,
  "providers": ["anthropic", "vertex"],
  "model_substrings": ["claude", "anthropic/claude", "vertex/claude"],
  "cache_control_type": "ephemeral",
  "skip_if_request_has_cache": true,
  "skip_if_raw_request_body": true,
  "strip_scope_on_vertex": true,
  "debug": false
}
```

Optional cache control fields:

- `ttl`: for example `"1h"`
- `scope`: for example `"user"` or `"global"`

`strip_scope_on_vertex` defaults to `true`, so a configured `scope` is stripped when marshaling Vertex Anthropic requests.

## Configuration

See `config.example.json` for a complete Bifrost plugin config example.

Supported plugin config fields:

| Field | Type | Default | Description |
| --- | --- | --- | --- |
| `enabled` | boolean | `true` | Turns the plugin on or off. |
| `providers` | string array | `["anthropic", "vertex"]` | Providers the plugin may modify. |
| `model_substrings` | string array | `["claude", "anthropic/claude", "vertex/claude"]` | Case-insensitive fallback model-name fragments. `schemas.IsAnthropicModel(...)` is also used. |
| `cache_control_type` | string | `"ephemeral"` | Cache control type to inject. Blank values are reset to `"ephemeral"`. |
| `ttl` | string | unset | Optional cache control TTL. |
| `scope` | string | unset | Optional cache control scope. |
| `skip_if_request_has_cache` | boolean | `true` | Avoids overwriting existing cache control. |
| `skip_if_raw_request_body` | boolean | `true` | Avoids replacing a request body already set by another plugin or caller. |
| `strip_scope_on_vertex` | boolean | `true` | Removes cache control `scope` from marshaled Vertex requests. |
| `debug` | boolean | `false` | Prints plugin debug messages to stdout. |

## Build

For local development, build the plugin shared object:

```bash
make build
```

The default output is:

```text
build/claude-cache-control-plugin.so
```

Build this plugin with the exact same Go toolchain and `github.com/maximhq/bifrost/core` version as the Bifrost binary that will load it. This repository currently targets `github.com/maximhq/bifrost/core v1.4.23`.

For a plugin-capable Bifrost runtime, use a dynamically linked Bifrost build. The official Bifrost docs explain why: static builds are the default for portability, but Go's plugin system needs dynamic linking to load `.so` files. They also document the compatibility rule this plugin follows: Bifrost and plugins must use the same OS/libc family, for example Alpine/musl with Alpine/musl or Debian/glibc with Debian/glibc.

## Plugin Name

Bifrost should load this plugin as:

```text
claude-cache-control-plugin
```
