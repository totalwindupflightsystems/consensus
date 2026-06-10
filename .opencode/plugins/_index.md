# OpenCode Plugins Index

OpenCode loads every `.ts`/`.js` file in this directory at startup. Each file is a **barrel** that re-exports a single plugin factory function from `.opencode/lib/`. See `_prompt.md` for the rules and gotchas.

## Quick Map

| File | Plugin | Tools Registered | Hooks | Internals |
|------|--------|-----------------|-------|-----------|
| `agent-depth-guard.ts` | `AgentDepthGuard` | 0 | `tool.execute.before` | inline (small) |
| `axiom.ts` | `CodeOpsPlugin` | 13 | various | `.axiom/plugin/dist/` (compiled) |
| `compaction.ts` | `CodeOpsCompactionPlugin` | 0 | `experimental.session.compacting` | inline (small) |
| `conductor.ts` | `ConductorPlugin` | 13 | session lifecycle | `lib/conductor.ts` |
| `config-tool.ts` | `ConfigToolPlugin` | 1 | none | inline (small) |
| `context-pipeline-hook.ts` | `ContextPipelineHook` | 0 | `experimental.chat.system.transform` | `lib/context-pipeline-hook.ts` + `lib/context-pipeline.ts` |
| `context-stash.ts` | `ContextStashPlugin` | 29 | session lifecycle | `lib/context-stash.ts` |
| `feed-ingestion.ts` | `FeedIngestionPlugin` | 8 | none | `lib/feed-ingestion.ts` |
| `graph-harness.ts` | `GraphHarnessPlugin` | 20 | session events | `lib/graph-harness.ts` |
| `opencode-session.ts` | `OpenCodeSessionPlugin` | 13 | none | `lib/opencode-session.ts` |
| `shellops.ts` | `ShellOpsPlugin` | 26 | `tool.execute.before`, `experimental.chat.system.transform`, `on_session_idle` | `lib/shellops.ts` (Zod migration complete 2026-05-18) |
| `tree-memory.ts` | `TreeMemoryPlugin` | 12 | none | `lib/tree-memory.ts` |

**Total live tool surface: ~165 plugin tools** + OpenCode built-ins + MCP server tools.

---

## Plugin-by-Plugin Notes

### `agent-depth-guard.ts`
Limits subagent recursion depth to prevent fork bombs. Hooks `tool.execute.before` for the `task` tool. Pure function — no DB, no state files. Logs via `client.app.log()` (canonical pattern, see this file as reference for new plugins).

### `axiom.ts`
The Axiom Plugin from `@opencode-ai/plugin` ecosystem. Re-exports compiled `dist/index.js` because OpenCode's loader requires every export to be a function — exporting from `src/` would expose ~66 non-function symbols and crash. Spec: `specs/70-OpenCode-Plugin.md#REQ-PLG-002`.

### `compaction.ts`
Hooks `experimental.session.compacting` to inject custom compaction context from `.opencode/prompts/`. No tools.

### `conductor.ts`
Background subagent orchestration: spawn, status, done, result, cancel, wait, collect, relay, delegate, broadcast, focus, unfocus, pin. **Requires SPIRE OR `allow_spawn_secret_fallback: true`** in `.opencode/config/conductor.local.json` for local dev (see `_prompt.md` Bug 7). Storage: `.conductor/conductor.db` (SQLite).

### `config-tool.ts`
Single tool `codeops_config` — get/set/show/list/describe/schema for plugin configs. Reads/writes `.opencode/config/<plugin>.json`.

### `context-pipeline-hook.ts`
7-stage context pipeline (collection → ranking → packing → injection → execution → compaction → evidence_capture). Fires on `experimental.chat.system.transform` before each LLM call. Token budget: 4000 default. Per-file size cap: 8KB. Logging gated behind `AXIOM_CONTEXT_PIPELINE_DEBUG=1`.

### `context-stash.ts`
Per-session and durable context stash for agent handoff. 29 tools covering full lifecycle: push/pop/peek/list/drop/create/close/enter/exit/headers/ingest/lock/log/migrate/node-events/related/search/summarize/switch/tag/append/apply/archive/cleanup/compact/context/ref/unlock. Storage: `.memory-bank/stash/{suspended,closed,active}/`. Backends: local, S3, Postgres (env-selected via `STASH_BACKEND`). Lifecycle events gated behind `AXIOM_CONTEXT_STASH_DEBUG=1`.

### `feed-ingestion.ts`
RSS/Atom/webhook/email/Slack/iCal/API feed polling with LLM relevance evaluation. 8 tools: list, status, poll, webhook, email, analytics, subscribe, health. **All 8 use Zod schemas** (migrated 2026-05-18 from raw JSON Schema — see Bug 9). Storage: `.axiom/feeds/*.yaml`, `.memory-bank/feed-state/`. Output: `.memory-bank/signals/`. Events gated behind `AXIOM_FEED_INGESTION_DEBUG=1`.

### `graph-harness.ts`
Graph-driven execution engine — model defines a DAG of work, harness drives execution deterministically. 20 tools spanning create/status/inject/modify/split/annotate/abandon/unblock/output/dataflow/message/template-load/template-save/admin/lock/unlock/transfer/session-list/export/import. Storage: `.graph-harness/harness.db` (SQLite, WAL) OR Postgres via `Bun.SQL`. ~150 console.warn calls migrated to `pluginWarn` (debug gated).

### `opencode-session.ts`
Session roster management — spawn/track/list OpenCode sessions across the harness. 13 tools. Schema-aware: degrades gracefully when sessions table is missing graph-harness columns.

### `shellops.ts`
HTTP client for the ShellOps Go daemon (`_tmp/shellops-bin start --port 9876`). **26 tools** exposed via the plugin factory (Zod migration complete 2026-05-18 — Bug 9 resolved). Covers exec, classify, terminal management (create/run/capture/list/destroy), log intelligence (watch_start/query/list/stop, logs_query/similar), sensory events (listen/query/stop), nohup tracking (list/check/output), service profiles (load/query), and high-level ops tools (health, investigate, triage, broadcast). Uses `tool.execute.before` hook for command classification, `experimental.chat.system.transform` for ops briefing injection, and `on_session_idle` for watch-match surfacing.

### `tree-memory.ts`
DuckDB-native tree-structured memory — hierarchical knowledge with path-based queries. 12 tools: init, branch, commit, promote, merge, state, query, peers, log, diff, spawn, status. Storage: `.tree-memory/repo/` (git-backed) + `.tree-memory/tree.duckdb`. Two query paths: direct (path-based) + semantic (vector).

---

## Files NOT in This Folder (and why)

These were moved to `.opencode/lib/` because they're either utility modules (not plugin factories) or plugin internals that crash OpenCode's loader if left here:

- `lib/conductor.ts` — conductor implementation (~30 internal exports)
- `lib/context-stash.ts` — stash implementation
- `lib/graph-harness.ts` — graph engine implementation
- `lib/tree-memory.ts` — tree memory implementation
- `lib/feed-ingestion.ts` — feed implementation
- `lib/opencode-session.ts` — session management
- `lib/context-pipeline-hook.ts` + `lib/context-pipeline.ts` — pipeline implementation
- `lib/config-utils.ts` — `loadPluginConfig` helper + `pluginWarn`/`pluginError`/`pluginInfo` logging helpers (used by every plugin)
- `lib/async-workers.ts` — pre-compact capture workers (utility)
- `lib/eval-runner.ts` — eval runner CLI (utility)
- `lib/self-improvement.ts` — self-improvement proposals (utility)

---

## Quick Reference

| Need | File |
|------|------|
| Add a new plugin | `_prompt.md` "When Adding a New Plugin" |
| Debug a plugin loader crash | `.memory-bank/best-practices/opencode-plugin-tools-sdk.md` Bug 1-10 |
| Tool API reference | `.opencode/skills/axiom-pattern-generator/REFERENCE.md` |
| Plugin events/hooks reference | https://opencode.ai/docs/plugins/#events |
| OpenCode SDK logging API | https://opencode.ai/docs/plugins/#logging |

---

## Stats (as of 2026-05-18)

- **12 plugin barrel files** in this folder (each <10 lines)
- **~85 console.* calls migrated** to env-gated `pluginWarn`/`pluginError`/`pluginInfo` (see `lib/config-utils.ts`)
- **8 OpenCode plugin loader bug categories** documented and tracked by regression tests
- **234 regression tests** across `plugin-null-guard.test.ts` (50 tests) + `plugin-tool-surface.test.ts` (19 tests) + `feed-ingestion.test.ts` (165 tests)

axiom:trace work_item=plugin-loader-bugs-01 spec=specs/70-OpenCode-Plugin.md
