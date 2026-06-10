# OpenCode Tests Folder Prompt

This folder contains all test files for `.opencode/lib/` (plugin internals) and `.opencode/plugins/` (plugin barrels). Run with `bun test`.

**Parent**: `.memory-bank/_prompt.md`
**Sibling**: `.opencode/lib/_prompt.md`, `.opencode/plugins/_prompt.md`

---

## Critical Rules

### MUST — import from `lib/`, not `plugins/`

Test files import internals (helpers, constants, types) from `.opencode/lib/<plugin>.ts`. The barrel files in `plugins/` only re-export the plugin factory — internals aren't accessible from there.

```ts
// ✅ CORRECT
import { ConductorPlugin, DEFAULT_CONFIG, initConductorDB } from "../lib/conductor.ts";

// ❌ WRONG — DEFAULT_CONFIG and initConductorDB aren't exported by the barrel
import { ConductorPlugin, DEFAULT_CONFIG, initConductorDB } from "../plugins/conductor.ts";
```

### MUST — capture stderr (with debug env var) for log assertion tests

Tests that previously mocked `console.warn`/`console.error` need updating after the logging migration. The lib/ code now uses `pluginWarn`/`pluginError` which write to stderr only when `AXIOM_<PLUGIN>_DEBUG=1` is set.

```ts
// ✅ CORRECT — capture pluginWarn output
const lines: string[] = [];
const origWrite = process.stderr.write.bind(process.stderr);
const prevEnv = process.env.AXIOM_CONDUCTOR_DEBUG;
process.env.AXIOM_CONDUCTOR_DEBUG = "1";
process.stderr.write = ((chunk: string | Uint8Array): boolean => {
  lines.push(typeof chunk === "string" ? chunk : new TextDecoder().decode(chunk));
  return true;
}) as typeof process.stderr.write;
try {
  // ... code under test
} finally {
  process.stderr.write = origWrite;
  if (prevEnv === undefined) delete process.env.AXIOM_CONDUCTOR_DEBUG;
  else process.env.AXIOM_CONDUCTOR_DEBUG = prevEnv;
}
// Assert against `lines`
```

### MUST NOT — `*.test.ts` files in `.opencode/plugins/`

OpenCode loads them as plugins → `beforeAll() outside test runner` crash. Always put tests here.

---

## Test Files

### Regression Suites (run these BEFORE every commit touching plugins)

| File | What It Catches |
|------|-----------------|
| `plugin-null-guard.test.ts` | Plugin loader crashes (null tools, missing args, wrong field names, dotted names, raw JSON Schema, stdio in plugin code) — 7 suites |
| `plugin-tool-surface.test.ts` | Tool surface drift (tools renamed/removed silently, unexpected new tools, incomplete metadata) — 4 suites |
| `shellops-plugin.test.ts` | ShellOps plugin tool surface + daemon HTTP integration — runs live daemon |

### Per-Plugin Tests

| File | Tests | Targets |
|------|-------|---------|
| `conductor.test.ts` | ~107 | `lib/conductor.ts` — full conductor lifecycle |
| `conductor-pcm.test.ts` | ~10 | Plugin Config Management for conductor |
| `context-pipeline.test.ts` | ~39 | `lib/context-pipeline.ts` + `lib/context-pipeline-hook.ts` |
| `context-stash.test.ts` | ~223 | `lib/context-stash.ts` — full stash lifecycle, all backends |
| `context-stash-pcm.test.ts` | ~17 | PCM for context-stash |
| `feed-ingestion.test.ts` | ~165 | `lib/feed-ingestion.ts` — RSS/Atom/webhook/email/Slack/iCal/API |
| `graph-harness.test.ts` | ~430 | `lib/graph-harness.ts` — full graph engine |
| `graph-harness-pcm.test.ts` | ~5 | PCM for graph-harness |
| `opencode-session.test.ts` | ~30 | `lib/opencode-session.ts` |
| `opencode-session-pcm.test.ts` | ~13 | PCM for opencode-session |
| `tree-memory.test.ts` | ~76 | `lib/tree-memory.ts` |
| `config-tool.test.ts` | ~20 | `plugins/config-tool.ts` |
| `config-utils.test.ts` | ~68 | `lib/config-utils.ts` (loadPluginConfig + helpers) |
| `sqlite-shared.test.ts` | ~20 | Shared SQLite patterns (PRAGMA, busy_timeout, init) |

### Eval/Harness Tests

| File | Purpose |
|------|---------|
| `async-workers.test.ts` | Pre-compact capture workers |
| `eval-runner.test.ts` | Eval scenario CLI runner |
| `harness-levelup.test.ts` | Harness level-up integration |
| `self-improvement.test.ts` | Self-improvement proposal validation |
| `shellops-integration.test.ts` | ShellOps + plugin integration tests |

---

## Running Tests

```bash
# Run everything
cd .opencode && bun test

# Run regression suites only (fast, ~1s)
bun test tests/plugin-null-guard.test.ts tests/plugin-tool-surface.test.ts

# Run a specific plugin's tests
bun test tests/conductor.test.ts

# Run with debug logging visible
AXIOM_PLUGIN_DEBUG=1 bun test tests/conductor.test.ts
```

## Pre-Existing Failures (NOT from current work)

When running `bun test`, expect ~27 pre-existing failures that are NOT bugs in current code:

- **shellops `IT-*` and `SC-*` tests** (~26): The shellops plugin uses factory architecture but these tests expect flat top-level exports. Architectural mismatch from earlier refactors.
- **`Real command integration > CLI entry`** (1): eval-runner CLI test that needs specific environment setup.

These are tracked but NOT blockers for plugin work. The plugin-null-guard and plugin-tool-surface suites should be green.

---

## Adding a New Plugin Test

1. Create `tests/<plugin>.test.ts`
2. Import internals from `../lib/<plugin>.ts`
3. Add the plugin to:
   - `plugin-null-guard.test.ts` PLUGINS list (regression coverage)
   - `plugin-tool-surface.test.ts` PLUGIN_SURFACES (expected tool inventory)

---

## Related References

- `.opencode/plugins/_prompt.md` — barrel pattern rules
- `.opencode/lib/_prompt.md` — plugin internals rules
- `.memory-bank/best-practices/opencode-plugin-tools-sdk.md` — full bug catalog
- `specs/70-OpenCode-Plugin.md` — plugin spec
- `specs/121-Pattern-Generator.md` — pattern generator spec

axiom:trace work_item=plugin-loader-bugs-01 spec=specs/70-OpenCode-Plugin.md
