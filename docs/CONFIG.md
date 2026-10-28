# Consensus YAML Configuration Reference

Every setting the `consensus` binary reads from its YAML configuration file, with the
compiled default, the environment variable that overrides it, and what the value actually
controls in the running server.

The single source of truth is the `Config` struct and its `Defaults()` constructor in
[`internal/config/config.go`](../internal/config/config.go). This page documents what that
file declares and what the rest of the tree consumes — nothing is inferred from the sample
`consensus.yaml`. Every key named by CONFIG-INERT-1 now either has a named runtime reader or
is gone from the surface — the keys that were taken off, and why, are listed in
[§6](#6-keys-removed-by-config-inert-1). One known gap remains outside that row's scope and
is stated where it appears: `adapters.opencode.admin_key` (§5.10), which the shim accepts
and does not read.

*Verified against:* `internal/config/config.go`, `cmd/consensus/main.go`,
`internal/cli/serve.go`, `internal/cli/mcp_stdio.go`, `internal/db/db.go`,
`internal/db/sqlite/sqlite.go`, `internal/api/server.go`, `internal/hitl/hitl.go`,
`internal/harness/planning.go`, `internal/harness/executor.go`,
`internal/compression/worker.go`, `internal/crier/client.go`, `consensus.yaml`.

---

## 1. Where the file is read from

`Load()` resolves the first **existing** file in this order (first match wins):

| Order | Path | Notes |
|---|---|---|
| 1 | `--config <path>` | Must exist. A missing explicit path is a startup error, not a silent fall-back to defaults (`LoadWithPath`, `internal/config/config.go:268-270`). |
| 2 | `$CONSENSUS_CONFIG` | Absolute or relative path. |
| 3 | `./consensus.yaml` | The repository ships one; it is also the working-directory default. |
| 4 | `~/.consensus/config.yaml` | Per-user file. |
| 5 | `/etc/consensus/config.yaml` | System-wide file. |

If none exists, the process starts on the compiled defaults and environment overrides alone
(`resolveConfigPath`, `internal/config/config.go:324-345`).

Keys that are absent from the file keep their compiled default. Keys that are present but
**not** in the struct are ignored: the loader uses `yaml.Unmarshal`
(`internal/config/config.go:273`), which does not enable `KnownFields`, so a misspelled key
is dropped silently rather than reported. There is no schema-validation pass over the YAML.

---

## 2. Precedence

The effective value is resolved once, in this order (highest first):

```
CLI flags  >  environment variables  >  YAML file  >  compiled defaults
```

The package doc states this (`internal/config/config.go:3-5`) and `LoadWithPath` implements
it: unmarshal the file over the defaults, then run `applyEnvOverrides`
(`internal/config/config.go:254-289`).

Flags do not read config directly. `serve` and `mcp-stdio` translate the flags they own into
environment variables *before* calling `config.Load()`, which is why a flag always wins
(`internal/cli/serve.go:31-45`, `internal/cli/mcp_stdio.go:44-53`):

| Flag | Becomes | Only when |
|---|---|---|
| `--config <path>` | `config.SetConfigPath(path)` | always when set |
| `--port <n>` | `CONSENSUS_PORT` | flag differs from `8090` |
| `--hostname <h>` | `CONSENSUS_HOSTNAME` | flag differs from `127.0.0.1` |
| `--db-url <url>` | `CONSENSUS_DB_URL` | flag non-empty |
| `--log-level <lvl>` | `CONSENSUS_LOG_LEVEL` | flag differs from `info` |
| `--auto-sync <interval>` | `CONSENSUS_AUTO_SYNC` | flag non-empty (read by the model-registry sync, `cmd/consensus/main.go:346`; not a YAML key) |

`--mcp`, `--adapter`, and `--migrations` are serve-only flags with no env or YAML
counterpart (`internal/cli/serve.go:65-72`). The persistent `--server`, `--api-key`,
`--format`, and `--quiet` flags belong to the CLI client, not the server config
(`internal/cli/root.go:135-144`).

Because the translation is conditional on the flag differing from its own default, passing a
flag *at* its default is not an override: `--port 8090` does not beat `port: 9000` in the
YAML file (`internal/cli/serve.go:31-33`). Use the env var when you need to pin a default
value explicitly.

---

## 3. `${...}` placeholders and how to supply secrets

A YAML value written as `${DEEPSEEK_API_KEY}` is **not** expanded by `yaml.v3`. The string
arrives at `applyEnvOverrides` exactly as written, `${` included, and is therefore *not* a
usable credential.

`applyEnvOverrides` handles this explicitly for `llm.api_key`: it treats a value that is
empty **or** begins with `${` as "no key configured", and substitutes from the environment
(`internal/config/config.go:357-473`). Every other key is used literally — there is no
general-purpose interpolator, so `${...}` in, say, `database.url` is passed through to the
driver unchanged.

Practical consequence: keep `api_key: ${DEEPSEEK_API_KEY}` in the file (or delete the key)
and export the variable. Do not paste a raw key into `consensus.yaml`; the file is meant to
be committed.

### API key resolution ladder (`llm.api_key`)

Evaluated in this order; the last applicable source wins.

| Order | Source | Applies when | Side effect |
|---|---|---|---|
| 1 | `CONSENSUS_API_KEY` | always when set | — |
| 2 | `OPENAI_API_KEY` | `llm.provider: openai` **and** key still empty | — |
| 3 | `ANTHROPIC_API_KEY` | `llm.provider: anthropic` **and** key still empty | — |
| 4 | `DEEPSEEK_API_KEY` | key empty, or key is an unresolved `${...}`, or the file value is concrete but no earlier env source claimed it | If no base URL and no provider override were set, forces `llm.base_url: https://api.deepseek.com/v1`; if that host is DeepSeek and `default_model` is still empty, sets `deepseek-v4-flash` |
| 5 | `OPENROUTER_API_KEY` | always when set | Sets `llm.provider: openrouter` and clears a **file-supplied** `llm.base_url` (an env-supplied one is kept) |

Checked last on purpose: a shell that exports both a leftover DeepSeek key and an OpenRouter
key ends up on OpenRouter (`internal/config/config.go:447-468`).

---

## 4. Startup validation

Before the listener serves traffic, `runServer` performs the checks below
(`cmd/consensus/main.go:107-121` for the pre-flight and validations, `:184` for the key
probe).

### 4.1 Port pre-flight

`cli.ProbePort(server.hostname, server.port)` classifies whatever already occupies the
effective port and exits `1` with a diagnostic naming the occupant class, instead of failing
later with a bare `EADDRINUSE` (`cmd/consensus/main.go:107-110`).

### 4.2 `ApplyStartupValidations` — one fatal, two warnings

| Outcome | Condition | Code |
|---|---|---|
| **Fatal** (exit 1) | `llm.provider` is not `mock` and `llm.api_key` is empty or still starts with `${` | `internal/config/config.go:511-516` |
| **Corrected** + warning | `compression.enabled: true` **and** the resolved `llm.base_url` contains `deepseek` (case-insensitive). Compression is switched off, because that provider exposes no embeddings endpoint and every embedding call would fail | `internal/config/config.go:518-521` |
| **Warning** | `server.hostname` does not provably resolve to loopback. The opencode shim's `/instance/*` surface is intentionally unauthenticated (SPEC-017 §3.10), so a non-loopback bind is an exposure the operator must state | `internal/config/config.go:526-530` |

### 4.3 LLM key probe

After the LLM client is built and before the harness takes traffic, the configured key is
probed once with a 10-second bounded deadline (`cmd/consensus/keyprobe_startup.go`):

| Verdict | Log level | Startup |
|---|---|---|
| key accepted | INFO, key's last 4 chars only | continues |
| 401/403 (invalid or revoked) | ERROR | **continues** — the harness will fail on every call |
| network error | WARN | continues |
| other status (429, 5xx) | WARN | continues |

Skipped entirely for `llm.provider: mock` and under `go test`. No key at all never reaches
the probe — §4.2 already exited the process.

**What is not validated:** a provider/model mismatch is only caught for the two cases above
(a keyless configuration, and compression-on-DeepSeek). An embeddings model pointed at a
provider without an embeddings endpoint is accepted as long as `compression.enabled` is
`false`; an unknown `default_model` string is not checked at startup at all — the first LLM
call reports it. Unknown YAML keys are also ignored (§1).

---

## 5. Section reference

Defaults below are the compiled values from `Defaults()`
(`internal/config/config.go:160-217`) — **not** the values in the shipped `consensus.yaml`,
which pins several keys differently. The comparison column notes the sample-file value where
the two differ.

### 5.1 `server`

| Key | Type | Default (compiled) | Env override | Effect |
|---|---|---|---|---|
| `server.hostname` | string | `127.0.0.1` | `CONSENSUS_HOSTNAME` | Bind address for the API/SSE/shim listener (and for the port pre-flight). A non-loopback value produces the §4.2 exposure warning. |
| `server.port` | int | `8090` | `CONSENSUS_PORT` | Listen port. Parsed with `fmt.Sscanf` whose error is discarded, so a non-numeric value is not rejected (`internal/config/config.go:365-367`). |
| `server.pprof_addr` | string | `127.0.0.1:18090` | none | Loopback-only pprof listener for goroutine/heap/CPU profiles; empty string disables it. Never point it at a public interface — the handlers are unauthenticated (`cmd/consensus/main.go:207-209`). The implemented default is `18090` (`internal/config/config.go:165`, "`server.port`+10000"); the comment in `cmd/consensus/main.go` and the table in [`docs/DEBUGGING.md`](DEBUGGING.md) still say `8095` — the code is authoritative. |

Sample file sets `hostname: 127.0.0.1`, `port: 8090`, and omits `pprof_addr`.

### 5.2 `llm`

| Key | Type | Default (compiled) | Env override | Effect |
|---|---|---|---|---|
| `llm.default_model` | string | `""` (empty) | `CONSENSUS_LLM_MODEL` | Model id sent on every completion. Empty is resolved at startup: `deepseek-v4-flash` on the DeepSeek key/host path (`internal/config/config.go:441-445`), otherwise `gpt-4o` (`cmd/consensus/main.go:164-166`). |
| `llm.provider` | string | `openai` | `CONSENSUS_LLM_PROVIDER`; also forced to `openrouter` by `OPENROUTER_API_KEY` | Selects the client implementation: `openai`, `anthropic`, `openrouter`, or `mock`. `mock` is the credential-free in-process client and is the only provider that skips the §4.2 fatal and the §4.3 probe. |
| `llm.api_key` | string | non-empty placeholder literal — **not** a usable credential (`internal/config/config.go:171`, DF-CONSENSUS-58) | `CONSENSUS_API_KEY`, `OPENAI_API_KEY`, `ANTHROPIC_API_KEY`, `DEEPSEEK_API_KEY`, `OPENROUTER_API_KEY` (see §3) | Bearer credential for completions, embeddings, and the summarizer. |
| `llm.base_url` | string | `""` (provider factory default) | `CONSENSUS_LLM_BASE_URL`, then `OPENROUTER_BASE_URL` | API root, e.g. `https://api.deepseek.com/v1`. Also the host used for embeddings (§5.7). |
| `llm.max_output_tokens` | int | `16384` | none | `llm.Config.MaxTokens` for each completion (`cmd/consensus/main.go:163`); a value `<= 0` falls back to `16384` (`cmd/consensus/main.go:167-169`). |

Sample file pins `default_model: deepseek-v4-flash`, `provider: openai`,
`base_url: https://api.deepseek.com/v1`, `api_key: ${DEEPSEEK_API_KEY}`.

### 5.3 `harness`

| Key | Type | Default (compiled) | Env override | Effect |
|---|---|---|---|---|
| `harness.heartbeat_interval_seconds` | int | `5` | none | Heartbeat poller tick (`h.HeartbeatConfig.Interval`); applied only when `> 0` (`cmd/consensus/main.go:186-188`). This is how often idle sessions are re-dispatched; a session's inbound message also wakes it immediately. |
| `harness.max_consecutive_errors` | int | `3` | none | Circuit-breaker threshold: after this many consecutive failed iterations the session is paused (`internal/harness/circuit.go:124-127`). Applied only when `> 0` (`cmd/consensus/main.go:189-191`). |
| `harness.planning_timeout_seconds` | int | `180` | none | Deadline for one interactive planning session: it becomes `PlanningConfig.Timeout`, applied as the planning context's deadline (`internal/harness/planning.go`, `RunInteractivePlanning`). Wired end to end — `cmd/consensus/main.go` sets `h.PlanningTimeout` and the dispatcher passes `h.planningConfig()` (`internal/harness/executor.go`). Raise it for a slow local model (LM Studio needs 300+); `<= 0` keeps the 180s default. |

Sample file sets `heartbeat_interval_seconds: 5`, `max_consecutive_errors: 3` and
`planning_timeout_seconds: 180`.

**Not in this section, by design:** the per-run turn cap and the spend cap are *session*
state, set when the session is created — `sessions.planning_max_turns`
(default `3`, `internal/harness/context.go:506-532`) and `sessions.budget_limit_cents`
(migration 022, `0` = no limit). A process-wide YAML key for either would be a second
source of truth, so `harness.max_iterations` and `harness.budget_limit_cents` were removed
(§6) rather than wired to a value the harness would have to prefer over the session row.

### 5.4 `database`

Struct is `db.Config` (`internal/db/db.go:70-88`).

| Key | Type | Default (compiled) | Env override | Effect |
|---|---|---|---|---|
| `database.url` | string | `sqlite://$HOME/.consensus/consensus.db` | `CONSENSUS_DB_URL` | Connection string. `sqlite://` URLs get `$HOME`, `${HOME}`, and a leading `~` expanded (`resolveDBURL`, `internal/config/config.go:301-321`); `postgres://` URLs are passed through untouched. Backend is inferred from the scheme. |
| `database.max_open_conns` | int | `8` | none | Pool size for both backends: Postgres `MaxConns` and the SQLite `SetMaxOpenConns` cap. `<= 0` falls back to `4` on SQLite (`internal/db/sqlite/sqlite.go:90-94`). |
| `database.busy_timeout_ms` | int | `0` → effective `5000` | none | SQLite only: lock-retry window before `SQLITE_BUSY`. `<= 0` means 5000 ms (`internal/db/sqlite/sqlite.go:49-53`). Ignored by Postgres. Not in the shipped sample. |

Sample file sets `url: sqlite://dev.db?_journal_mode=WAL` and `max_open_conns: 10` (raised
from 5 after a 16-session load test saturated the old pool; the comment in the file records
the measurement).

### 5.5 `hitl`

| Key | Type | Default (compiled) | Env override | Effect |
|---|---|---|---|---|
| `hitl.auto_pause_on_error_threshold` | int | `3` | none | Consecutive-error threshold persisted into the **global** HITL configuration at startup. Effective for every session without a session-scoped override. `<= 0` keeps the shipped default. |
| `hitl.require_approval_for_destructive` | bool | `true` | none | Global gate: destructive operations park for human approval before they run. |
| `hitl.require_approval_for_schema_changes` | bool | `true` | none | Global gate: schema changes park for human approval. |
| `hitl.approval_timeout_minutes` | int | `60` | none | Lifetime of a pending approval: `expires_at = now + this` (`internal/hitl/hitl.go`, `RequestApproval`). The expiry cron then fails sessions left paused with only expired approvals. `<= 0` keeps the shipped default. |

These four keys are **wired** (CONFIG-INERT-1). `runServer` builds the manager and writes the
global configuration from the resolved config at every start
(`cmd/consensus/main.go` → `hitlConfigurationFromConfig`), so the values in this section are
the ones `GET /api/v1/sessions/{id}/approvals` and the approval gate act on. Two settings
have no YAML key and keep their `internal/hitl/hitl.go` defaults —
`require_approval_for_external_tools: false` and `notify_on_pause: true`. A session-scoped
row written through the API still takes precedence over the global row.

### 5.6 `logging`

| Key | Type | Default (compiled) | Env override | Effect |
|---|---|---|---|---|
| `logging.level` | string | `info` | `CONSENSUS_LOG_LEVEL` | Minimum level of the process logger: `debug`, `info`, `warn` (`warning` accepted) or `error`, case-insensitive. Unknown values fall back to `info` rather than failing startup. Applied through `LoggingConfig.SlogHandler`, which `runServer` and `runMCPStdio` install with `slog.SetDefault` (CONFIG-INERT-1). `serve --log-level` sets the same env var, so the flag works too. |
| `logging.format` | string | `text` | none | `text` emits the human-readable `level=… msg=…` lines; `json` emits one JSON object per line (for log collectors). Anything else is treated as `text`. |

### 5.7 `compression`

Background memory-compression worker (WI-012, SPEC-002 §8). It needs an OpenAI-compatible
`/v1/embeddings` endpoint on the **same host as `llm.base_url`** (`embeddingBaseURL`,
`cmd/consensus/main.go:742-750`), which is why a DeepSeek backend disables it at startup
(§4.2).

| Key | Type | Default (compiled) | Env override | Effect |
|---|---|---|---|---|
| `compression.enabled` | bool | `true` | none | Starts the worker goroutine. Forced to `false` with a warning when the resolved `llm.base_url` contains `deepseek` (`internal/config/config.go:518-521`). The shipped sample sets `false` explicitly. |
| `compression.poll_interval_seconds` | int | `5` | none | Compression-queue poll cadence, in seconds (`cmd/consensus/main.go:231`). |
| `compression.batch_size` | int | `5` | none | Max queued events processed per poll cycle (`cmd/consensus/main.go:232`). |
| `compression.cosine_threshold` | float | `0.85` | none | Minimum cosine similarity for a generated summary to be accepted (`internal/compression/worker.go:480`); the code default constant is also `0.85` (`internal/compression/compression.go:92`). |
| `compression.embedding_model` | string | `text-embedding-3-small` | none | Embedding model id. Must exist on the embeddings host — a DeepSeek host has no embeddings endpoint. Empty keeps the worker default (`cmd/consensus/main.go:234-236`). |

Note: `poll_interval_seconds`, `batch_size`, and `cosine_threshold` are copied into the
worker config without a `> 0` guard (`cmd/consensus/main.go:231-233`), so an explicit `0` in
YAML overrides the worker's own default rather than "unsetting" it. Omit the key to keep the
default.

### 5.8 `api_rate`

Requests-per-minute ceilings, keyed by the **scope stored on the API key**
(`api_keys.scope` is constrained to `admin`, `session`, `readonly`, `webhook` — migration
`002_shim_session_map.sql:41`).

| Key | Type | Default (compiled) | Env override | Effect |
|---|---|---|---|---|
| `api_rate.admin_per_min` | int | `1000` | none | Requests/min allowed for keys with scope `admin`. |
| `api_rate.session_per_min` | int | `100` | none | Requests/min for scope `session`. An unrecognized scope falls back to the built-in `100` constant, **not** to this configured value (`scopeRateLimit`, `internal/api/server.go:486-491`). |
| `api_rate.readonly_per_min` | int | `200` | none | Requests/min for scope `readonly`. |
| `api_rate.webhook_per_min` | int | `500` | none | Requests/min for scope `webhook`. |

Each value is passed to `api.ServerConfig` (`cmd/consensus/main.go:311-314`) and applied by
`resolveRates` only when it is `> 0`; `0` or a negative keeps the built-in table
(`internal/api/server.go:442-482`). Accounting is per key prefix in `api_rate_limits`, and a
request over the ceiling gets `429 RATE_LIMITED`.

### 5.9 `crier`

Inbound crier message-bus settings (CR-IN-001). **The shipped `consensus.yaml` has no
`crier` section.**

The consumer path is `internal/crier` — the client (register / deliver /
retrieve-with-lease / ack) and the intake that appends retrieved messages to the session
ledger — and the section resolves exactly the ladder that client uses. What does *not*
exist is a runner: `consensus serve` starts no intake loop, so these values reach the
package only when an intake caller constructs it with them. This is the **one section this
page still describes as configure-only**, and the server says so out loud: a configured
`crier` section produces a startup warning naming the missing runner
(`ApplyStartupValidations`), so it is never silently accepted.

| Key | Type | Default (compiled) | Env override | Effect |
|---|---|---|---|---|
| `crier.url` | string | `""` — the client ladder ends at `http://localhost:8767` | `CONSENSUS_CRIER_URL` | Crier relay base URL. Resolution ladder: `crier.url`, then the env var, then the package's `DefaultBaseURL` (doc comment on `CrierConfig`). |
| `crier.agent_id` | string | `""` | `CONSENSUS_CRIER_AGENT` | Crier agent identity this instance consumes as (registration id and inbox). Empty leaves identity to the caller per run. |

### 5.10 `adapters`

Also absent from the shipped `consensus.yaml`. Both adapters default to **enabled**.

| Key | Type | Default (compiled) | Env override | Effect |
|---|---|---|---|---|
| `adapters.opencode.enabled` | bool | `true` | none | Mounts the opencode-compatible shim (`cmd/consensus/main.go:444-446`), including the auth-free `/instance/*` surface — see the loopback warning in §4.2. |
| `adapters.opencode.admin_key` | string | `""` | none | **Accepted, not read.** Startup passes it to `opencode.NewServer`, which stores it and no handler reads it — the same known gap recorded in [`runbooks/serve-adapters.md`](runbooks/serve-adapters.md). Leave it empty; the shim's `/instance/*` surface is auth-free by design (§4.2). |
| `adapters.h3.enabled` | bool | `true` | none | Mounts the H3 protocol adapter (`cmd/consensus/main.go:473-480`). |

---

## 6. Keys removed by CONFIG-INERT-1

These keys were declared, defaulted and (in some docs) promised, and no runtime reader
existed for them: setting them changed nothing. They are **gone** from `Config`, from the
shipped `consensus.yaml` and from this page; the value the runtime really uses is named
below. Two consequences worth knowing:

- The loader is a plain `yaml.Unmarshal` with no `KnownFields` (§1), so an older config
  file that still carries one of these keys **keeps loading** — the key is ignored, exactly
  as it was ignored while it existed. There is no strict-decoding mode to turn on.
- The surface is asserted by `TestConfigInertKeysResolved`
  (`internal/config/config_test.go`): a deleted key re-appearing in the struct, or a wired
  key disappearing, fails the test.

| Removed key | Was defaulted to | Where the effective value actually comes from |
|---|---|---|
| `harness.max_iterations` | `100` | `sessions.planning_max_turns`, `COALESCE`d to `3` (`internal/harness/context.go:506-532`) — per-session state set at creation. |
| `harness.budget_limit_cents` | `1000` | `sessions.budget_limit_cents` (migration 022; `0` = no limit), set at session creation and enforced per LLM call in `internal/harness/executor.go`. |
| `harness.transaction_timeout_ms` | `60000` | No second policy: the SQLite lock wait is `database.busy_timeout_ms` → `PRAGMA busy_timeout` (`internal/db/sqlite/sqlite.go:49-77`). |
| `llm.max_context_tokens` | `128000` | Session `context_budget` (per-creation), and `model_registry.max_context` for the model's real window. |
| `database.max_idle_conns` | `0` | Nothing — neither backend had an idle cap to set (the Postgres pool is `pgxpool`, sized by `max_open_conns`). |

Filing a bug against a *current* key is legitimate: every key in §5 now has a named reader.
A `crier` section is the one exception this page still flags (§5.9) — it is reported at
startup instead of being applied, until an intake runner exists.

---

## 7. Worked examples

### Local model on LM Studio (slow, no DeepSeek)

```yaml
llm:
  provider: openai
  base_url: http://127.0.0.1:1234/v1
  api_key: lm-studio        # LM Studio ignores the value but requires one
  default_model: qwen/qwen3.6-35b-a3b
  max_output_tokens: 16384

compression:
  enabled: true             # keep on only if your base_url serves /v1/embeddings
  embedding_model: text-embedding-3-small

harness:
  heartbeat_interval_seconds: 5
  max_consecutive_errors: 5
  planning_timeout_seconds: 600   # slow local planner: whole planning session deadline
```

`harness.planning_timeout_seconds` **is** the slow-model knob now (CONFIG-INERT-1): it
bounds the entire planning session, LLM calls and SQLite writes included, and defaults to
180s. Increase `database.busy_timeout_ms` (SQLite) as well if long planning writes collide
with the heartbeat poller.

### DeepSeek direct (rate-limit tuning)

```yaml
llm:
  base_url: https://api.deepseek.com/v1
  api_key: ${DEEPSEEK_API_KEY}   # export DEEPSEEK_API_KEY instead of pasting a key

compression:
  enabled: false                 # no embeddings endpoint; §4.2 would disable it anyway

api_rate:
  admin_per_min: 1000
  session_per_min: 100
  readonly_per_min: 200
  webhook_per_min: 500
```

### Environment-only deployment (no YAML at all)

```bash
export DEEPSEEK_API_KEY=...      # also picks https://api.deepseek.com/v1
export CONSENSUS_DB_URL="sqlite:///var/lib/consensus/consensus.db"
export CONSENSUS_HOSTNAME=127.0.0.1
export CONSENSUS_PORT=8090
consensus serve
```

Everything not set keeps the compiled default from `Defaults()`
(`internal/config/config.go:160-217`).

---

## 8. Related

- [API reference](API.md) — every `/api/v1/*` endpoint, including the config surface.
- [Integration guide](INTEGRATION.md) — MCP transports and the H3 adapter.
- [Debugging guide](DEBUGGING.md) — the pprof listener configured by `server.pprof_addr`.
- [Runbooks](runbooks/) — deployment, backup/restore, key rotation.
