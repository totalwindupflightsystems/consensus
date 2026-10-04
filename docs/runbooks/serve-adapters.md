# Serve flags and protocol adapter configuration

<a id="serve-adapters"></a>

This runbook records the implemented behavior of the `consensus serve` adapter-related flags and YAML keys. The CLI flags `--adapter` and `--mcp` are present in help, but currently do not control the server. Configure the live adapters through the YAML keys below instead.

## `consensus serve` flags

| Flag | Status | Actual behavior | Source evidence |
|---|---|---|---|
| `--adapter` | **Dead: parsed but unread.** | Cobra registers this string flag (default `opencode`), but the `serve` command callback never reads it or passes its value to startup. It cannot enable, disable, or select an adapter. | `internal/cli/serve.go:65-72` declares it; the callback's only flag reads are `port`, `hostname`, `db-url`, `log-level`, and `auto-sync` at `:31-45`. |
| `--mcp` | **Dead: parsed but unread.** | Cobra registers this boolean flag (default `true`), but the callback never reads it. The HTTP MCP handler is mounted unconditionally, regardless of the flag value. | `internal/cli/serve.go:31-45,65-72` (no read of `mcp`); `cmd/consensus/main.go:367-378` constructs and mounts the MCP handler unconditionally. |

Do not rely on either flag to change server behavior. The flags remain accepted for compatibility, but their values have no runtime effect.

## YAML `adapters.*` keys

`internal/config/config.go:23-33` maps the root YAML `adapters` object into `Config.Adapters`; `LoadWithPath` unmarshals the YAML into that struct at `:252-278`. The currently declared adapter keys are:

| YAML key | Status | Actual behavior | Source evidence |
|---|---|---|---|
| `adapters.opencode.enabled` | **Live.** | Enables or disables mounting the OpenCode protocol shim. When true, the server constructs the shim and mounts its route patterns. Default: `true`. | YAML struct fields: `internal/config/config.go:140-150`; default: `:206-213`; runtime check and mount: `cmd/consensus/main.go:395-410`. |
| `adapters.opencode.admin_key` | **Dead: loaded and passed, but unread by the shim.** | YAML decoding fills this field and startup passes it to `opencode.NewServer`, which stores it in a struct field. No OpenCode handler reads that field, so setting the key currently has no runtime effect. Default: empty. | YAML field: `internal/config/config.go:146-150`; pass to constructor: `cmd/consensus/main.go:397-400`; storage only: `internal/shim/opencode/server.go:66,158-168` (no reads of `adminKey` elsewhere in `internal/shim/opencode`). |
| `adapters.h3.enabled` | **Live.** | Enables or disables mounting the H3 protocol shim for H3 clients. When true, the server constructs the H3 shim and mounts its route patterns. Default: `true`. | YAML struct fields and default: `internal/config/config.go:140-155,206-213`; runtime check and mount: `cmd/consensus/main.go:422-432`. |

The YAML decoder loads these declared fields. The two `enabled` values are consumed by server startup; `opencode.admin_key` is parsed and stored on the shim but is not read by its handlers. The server's MCP HTTP endpoint is independent of these adapter switches and is mounted regardless of `--mcp`.

## Verification evidence

The statuses above were determined by tracing the Cobra flag registrations and callback reads, the config struct/YAML unmarshalling path, and the server's adapter/MCP route mounting. This is documentation only; no runtime behavior is changed.
