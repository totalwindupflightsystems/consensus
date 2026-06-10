# OpenCode Lib Index

Plugin internals — implementation modules imported by barrels in `.opencode/plugins/` and tests in `.opencode/tests/`. See `_prompt.md` for the rules.

## Layout

```
.opencode/
├── plugins/           ← barrel re-exports (OpenCode loads these)
├── lib/               ← THIS FOLDER — plugin internals
│   ├── _prompt.md
│   ├── _index.md      ← this file
│   ├── config-utils.ts ← shared logging + config loading
│   └── <plugin>.ts    ← per-plugin implementations
└── tests/             ← test files (import from lib/)
```

---

## Files

### Plugin Implementations (factory + internals)

| File | Plugin Factory | Tools | Notes |
|------|---------------|-------|-------|
| `conductor.ts` | `ConductorPlugin` | 13 | Background subagent orchestration. Throws if SPIRE unavailable AND fallback disabled. Storage: `.conductor/conductor.db` |
| `context-pipeline-hook.ts` | `ContextPipelineHook` | 0 (hooks only) | 7-stage context pipeline before LLM calls. Logging gated behind `AXIOM_CONTEXT_PIPELINE_DEBUG=1` |
| `context-stash.ts` | `ContextStashPlugin` | 29 | Per-session + durable agent context. Backends: local, S3, Postgres. Storage: `.memory-bank/stash/` |
| `feed-ingestion.ts` | `FeedIngestionPlugin` | 8 | RSS/webhook/email/Slack/iCal/API polling with LLM relevance. All tools migrated to Zod schemas (2026-05-18) |
| `graph-harness.ts` | `GraphHarnessPlugin` | 20 | Graph-driven execution. SQLite or Postgres. Storage: `.graph-harness/harness.db` |
| `opencode-session.ts` | `OpenCodeSessionPlugin` | 13 | Session roster across the harness |
| `shellops.ts` | `ShellOpsPlugin` | 26 | ShellOps daemon HTTP client. All 26 tools use Zod args (migrated 2026-05-18 — Bug 9 resolved). Covers exec, classify, terminal, watches, logs, events, nohup, profiles, health, investigate, triage, broadcast. |
| `tree-memory.ts` | `TreeMemoryPlugin` | 12 | DuckDB-native tree memory. Storage: `.tree-memory/` |

### Utility Modules (no plugin factory — used by other plugins)

| File | Exports | Used By |
|------|---------|---------|
| `config-utils.ts` | `loadPluginConfig`, **`pluginWarn`**, **`pluginError`**, **`pluginInfo`**, `deepMerge`, `validatePluginName`, schema helpers | Every plugin (logging + config) |
| `context-pipeline.ts` | `runContextPipeline`, `PIPELINE_STAGES`, `PipelineEvent` types | `context-pipeline-hook.ts` |
| `async-workers.ts` | `runPreCompactCaptureWorker`, `WorkerRecord` types | `compaction.ts` |
| `eval-runner.ts` | `runEval`, eval scenario types | CLI tool (`bun run lib/eval-runner.ts`) |
| `self-improvement.ts` | `validateProposal`, proposal types | `feed-ingestion.ts` (relevance evaluator) |

---

## Key Helpers in `config-utils.ts`

These are used by EVERY plugin. Memorize them.

### `pluginWarn(plugin: string, message: string, extra?: unknown)`
Env-gated structured warning. Writes JSON to stderr ONLY when `AXIOM_PLUGIN_DEBUG=1` or `AXIOM_<PLUGIN>_DEBUG=1` is set. Production: silent.

### `pluginError(plugin: string, message: string, extra?: unknown)`
Same as `pluginWarn` but with `level: "error"`.

### `pluginInfo(plugin: string, message: string, extra?: unknown)`
Same as `pluginWarn` but with `level: "info"`. Use sparingly — most info logs are noise.

### `loadPluginConfig(pluginName, defaults, directory)`
Three-layer config: defaults → `.opencode/config/<plugin>.json` → env vars (`AXIOM_<PLUGIN>__<FIELD>`). Returns merged config.

---

## When Editing Files Here

1. **Edit lib/, NOT plugins/**: The barrel is a 4-line re-export. Real changes go here.
2. **Run tests after**: `cd .opencode && bun test tests/<plugin>.test.ts`
3. **Don't add `console.log`**: Use `pluginInfo` (or omit). The plugin-null-guard SUITE 8 will WARN if you do.
4. **Don't add raw JSON Schema**: Use `tool.schema.*` for tool args.
5. **Update `_index.md`** (this file) if the file's role/exports change.

---

## Stats

- **12 implementation files** in this folder
- **~85 console.* calls migrated** to env-gated helpers (2026-05-18)
- **101 stdio writes remaining** — tracked by plugin-null-guard.test.ts SUITE 8 for incremental cleanup
- All 7 plugin factories return valid `{ tool: object, ...hooks }` — ToolRegistry crash guard

---

## Quick Reference

| Need | Where |
|------|-------|
| Add a new plugin | `_prompt.md` "When Adding a New Plugin" |
| Debug a plugin loader crash | `.memory-bank/best-practices/opencode-plugin-tools-sdk.md` Bug 1-10 |
| Migrate a console.* call | Use `pluginWarn`/`pluginError`/`pluginInfo` from `config-utils.ts` |
| Tool API for an existing plugin | `.opencode/skills/axiom-pattern-generator/REFERENCE.md` |
| Plugin event hooks | https://opencode.ai/docs/plugins/#events |
| Plugin SDK logging | https://opencode.ai/docs/plugins/#logging |

axiom:trace work_item=plugin-loader-bugs-01 spec=specs/70-OpenCode-Plugin.md
