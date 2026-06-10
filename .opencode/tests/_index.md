# OpenCode Tests Index

Test files for `.opencode/lib/` and `.opencode/plugins/`. See `_prompt.md` for the rules and patterns.

## At a Glance

```bash
cd .opencode && bun test                # full suite
bun test tests/plugin-null-guard.test.ts # plugin loader regression (run before every commit)
```

**Last clean run**: 1571 pass, 27 pre-existing fails (shellops IT-*/SC-*, eval-runner CLI). 0 failures introduced by plugin loader work.

---

## Test Inventory by Category

### Plugin Health (regression — run first)

| File | Suites | Catches |
|------|--------|---------|
| `plugin-null-guard.test.ts` | 7 | Null tools, undefined args, wrong field names (parameters: vs args:), dotted tool names, plugins/ hygiene, missing execute fn, raw JSON Schema args, stdio in plugin code |
| `plugin-tool-surface.test.ts` | 4 | Surface drift — silent renames/removals, unexpected new tools, incomplete metadata, total registry count |
| `shellops-plugin.test.ts` | 4 | Tool surface, daemon HTTP integration (live daemon required), SA-1 routing regression |

### Plugin Functionality

| File | Lib Target |
|------|-----------|
| `conductor.test.ts` | `lib/conductor.ts` |
| `conductor-pcm.test.ts` | `lib/conductor.ts` (config) |
| `context-pipeline.test.ts` | `lib/context-pipeline-hook.ts` + `lib/context-pipeline.ts` |
| `context-stash.test.ts` | `lib/context-stash.ts` |
| `context-stash-integration.test.ts` | `lib/context-stash.ts` — S3+PG integration (requires LocalStack/PG; all skipped by default) |
| `context-stash-pcm.test.ts` | `lib/context-stash.ts` (config) |
| `feed-ingestion.test.ts` | `lib/feed-ingestion.ts` |
| `graph-harness.test.ts` | `lib/graph-harness.ts` |
| `graph-harness-pcm.test.ts` | `lib/graph-harness.ts` (config) |
| `opencode-session.test.ts` | `lib/opencode-session.ts` |
| `opencode-session-pcm.test.ts` | `lib/opencode-session.ts` (config) |
| `tree-memory.test.ts` | `lib/tree-memory.ts` |
| `config-tool.test.ts` | `plugins/config-tool.ts` |
| `config-utils.test.ts` | `lib/config-utils.ts` (helpers + logging) |
| `sqlite-shared.test.ts` | Shared SQLite patterns |

### Eval/Harness/Workers

| File | Targets |
|------|---------|
| `async-workers.test.ts` | `lib/async-workers.ts` (pre-compact capture) |
| `eval-runner.test.ts` | `lib/eval-runner.ts` (CLI runner) |
| `harness-levelup.test.ts` | Harness integration tests |
| `self-improvement.test.ts` | `lib/self-improvement.ts` (proposal validation) |
| `shellops-integration.test.ts` | ShellOps + plugin chain |

---

## When You're Debugging

| Problem | Run |
|---------|-----|
| OpenCode crashes on session prompt | `bun test tests/plugin-null-guard.test.ts` |
| Tool surface looks wrong (missing tools) | `bun test tests/plugin-tool-surface.test.ts` |
| ShellOps daemon endpoints 404 | `bun test tests/shellops-plugin.test.ts` |
| Specific plugin behavior broken | `bun test tests/<plugin>.test.ts` |
| Config loading misbehaving | `bun test tests/config-utils.test.ts tests/<plugin>-pcm.test.ts` |

---

## See Also

- `_prompt.md` — test patterns, stderr capture, debug env vars
- `.opencode/lib/_prompt.md` — what's being tested
- `.memory-bank/best-practices/opencode-plugin-tools-sdk.md` — bug catalog (10 categories)

axiom:trace work_item=plugin-loader-bugs-01 spec=specs/70-OpenCode-Plugin.md
