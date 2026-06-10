# .opencode

OpenCode project-local configuration.

Layout (v1):
- `.opencode/agents/` - agent definitions (markdown)
- `.opencode/commands/` - slash commands used by Axiom (markdown)
- `.opencode/tools/` - optional custom tools (JS/TS)

Agents (v1):
- `tower-axiom` - primary orchestrator
- `pm-axiom` - planning PM (TODO + implementation plans)
- `specwriter-axiom` - spec writer/librarian
- `best-practices-axiom` - reusable playbooks generator
- `dev-axiom` - implementation agent
- `qa-axiom` - QA verifier
- `spec-verifier-axiom` - spec verifier
- `trace-auditor-axiom` - trace completeness auditor
- `security-review-axiom` - security verifier
- `db-architect-axiom` - DB architect/verifier
- `ci-cd-axiom` - CI/CD verifier
- `sre-ops-axiom` - ops/observability/runbooks linkage
- `docs-runbooks-axiom` - docs and runbooks
- `prompt-mirror-axiom` - prompt mirror maintainer
- `dependency-bot-axiom` - dependency upgrades/CVEs
- `repo-researcher-axiom` - upstream/repo research
- `release-manager-axiom` - release notes/versioning
- `memory-bank-axiom` - Memory Bank maintainer
- `ux-writer-axiom` - user-facing copy
- `devguide-axiom` - legacy dev guide agent (kept for backward compatibility)
- `ralph-wiggum-verify` - verifier captain for Ralph builder-loop steering

Commands (v1):
- `/axiom-bootstrap` - one-command bootstrap (TODO + implementation plans + Ralph loop)
- `/axiom-init` - initialize Axiom structure in a blank repo
- `/axiom-sync-indexes` - sync `_index.md` inventories
- `/axiom-sync-trace` - sync traceability markers and links
- `/axiom-sync-distribution` - sync installer/version/template artifacts
- `/axiom-sync-command-registry` - sync `.axiom/command-registry.yaml` to installed commands
- `/axiom-sync-specs-inventory` - sync `specs/README.md` and `specs/_index.md`
- `/axiom-sync-memory-bank-core` - sync Memory Bank core structure
- `/axiom-sync-work-items` - sync work item hygiene
- `/axiom-sync-version-manifest` - sync `.axiom/.version.md`
- `/axiom-sync-template` - sync `axiom-template/`
- `/axiom-sync-all` - run all sync commands
- `/axiom-loop` - build/refresh a Ralph loop scaffold
- `/axiom-prompt-update` - refresh PROMPT.md / PROMPT-VERIFY.md (prompt bundle)
- `/axiom-spec-request` - feature request -> spec update -> spec verification
- `/axiom-sitrep` - sitrep report
- `/axiom-roadmap-refresh` - refresh TODO + implementation plans
- `/axiom-work-item` - create/refresh a work item (meta-plan + plan artifacts)
- `/axiom-best-practices` - generate/store best practices guidance
- `/axiom-kickoff` - combined kickoff (specs + work item + roadmap + loop)
- `/axiom-plan` - plan a work item
- `/axiom-step` - execute one step
- `/axiom-verify` - verify alignment
- `/memory-bank-update` - update/audit/bootstrap Memory Bank
- `/axiom-todo` - create/update `.memory-bank/TODO.md`
- `/axiom-implementation-plans` - create/update `.memory-bank/implementation-plans/`
- `/axiom-meta-plan` - produce meta-planning + plan artifacts for a work item

Ralph loop scaffolding:
- `/ralph-wiggum-loop` - generate `PROMPT.md` + runner scripts (see `.opencode/skills/ralph-wiggum-loop/SKILL.md`)

Skills (optional):
- `axiom-todo` - on-demand guidance for TODO maintenance
- `axiom-implementation-plans` - on-demand guidance for implementation plan maintenance
- `meta-plan-axiom` - on-demand guidance for meta-planning and plan artifacts

Docs:
- Config: https://opencode.ai/docs/config/
- Agents: https://opencode.ai/docs/agents/
- Commands: https://opencode.ai/docs/commands/
- Custom tools: https://opencode.ai/docs/custom-tools/

---

## Plugin Configuration System (v1)

Every Axiom TypeScript plugin supports a **three-layer configuration system** — no source code changes needed to tune behaviour.

### How it works

Config is loaded in this priority order (highest wins):

1. **Defaults** — hardcoded `DEFAULT_*_CONFIG` constant in the plugin
2. **Config file** — `.opencode/config/<plugin>.json` (commit to share with the team)
3. **`.local.json` override** — `.opencode/config/<plugin>.local.json` (gitignored, personal)
4. **Env vars** — `AXIOM_<PLUGIN_UPPER>_<KEY>` (highest priority; great for CI)

### Supported plugins

| Plugin | Config file | Env prefix |
|---|---|---|
| `graph-harness` | `.opencode/config/graph-harness.json` | `AXIOM_GRAPH_HARNESS_` |
| `opencode-session` | `.opencode/config/opencode-session.json` | `AXIOM_OPENCODE_SESSION_` |
| `context-stash` | `.opencode/config/context-stash.json` | `AXIOM_CONTEXT_STASH_` (Phase 4 complete — see `context-stash-config-adoption`) |
| `conductor` | `.opencode/config/conductor.json` | `AXIOM_CONDUCTOR_` (Phase 4 complete — see `conductor-phase4-config-adoption`) |

### Env var naming

Keys use double-underscore `__` to represent nesting:
- `limits.max_concurrent_agents` → `AXIOM_CONDUCTOR_LIMITS__MAX_CONCURRENT_AGENTS`
- Flat key `request_timeout_ms` → `AXIOM_OPENCODE_SESSION_REQUEST_TIMEOUT_MS`

> **⚠ Separator warning**: Use **double** underscore `__` for nested keys. A single `_` is treated
> as part of the key name and will be **silently ignored** (no error, no effect, no warning).
>
> ```bash
> # CORRECT: double underscore separates levels
> export AXIOM_CONDUCTOR_LIMITS__MAX_CONCURRENT_AGENTS=5
>
> # WRONG: single underscore — silently ignored, value has no effect
> export AXIOM_CONDUCTOR_LIMITS_MAX_CONCURRENT_AGENTS=5
> ```
>
> <!-- axiom:trace work_item=conductor-phase4-config-adoption spec=specs/112-Plugin-Config-Management.md#3.3 plan=phase-4/task-4-4/backlog-007 -->

All values are strings; booleans and numbers are coerced automatically (`"true"` → `true`, `"5000"` → `5000`).

### Example: tune session timeouts without editing code

```json
// .opencode/config/opencode-session.json
{
  "request_timeout_ms": 15000,
  "spawn_timeout_ms": 30000
}
```

Or via env var (e.g., in CI):
```sh
export AXIOM_OPENCODE_SESSION_REQUEST_TIMEOUT_MS=15000
```

### Example: move context stash to a custom directory

```json
// .opencode/config/context-stash.json
{
  "stash_dir": ".custom/stash"
}
```

### `.local.json` files (personal overrides)

Files named `*.local.json` are **automatically gitignored** (the repo-root `.gitignore` is updated on first write). Use them for personal overrides that should not be committed:
```json
// .opencode/config/opencode-session.local.json (NOT committed)
{
  "opencode_base_url": "http://localhost:5000"
}
```

### `codeops_config` MCP tool

The `codeops_config` tool lets agents inspect and update plugin config at runtime:

| Operation | Example | Returns |
|---|---|---|
| `get` | `codeops_config({"operation":"get","plugin":"opencode-session","key":"request_timeout_ms"})` | Current value + source layer |
| `set` | `codeops_config({"operation":"set","plugin":"opencode-session","key":"request_timeout_ms","value":"10000"})` | Confirmation + new value. _Note: `set` writes to disk. Changes take effect on next restart — the running plugin does not hot-reload config._ |
| `show` | `codeops_config({"operation":"show","plugin":"graph-harness"})` | Full effective config |
| `describe` | `codeops_config({"operation":"describe","plugin":"graph-harness"})` | Field description + default |
| `list` | `codeops_config({"operation":"list"})` | All configured plugins |
| `schema` | `codeops_config({"operation":"schema","plugin":"graph-harness"})` | Full JSON Schema |

### Schemas

JSON Schemas for all four plugins live in `.opencode/config/schemas/`. Use `codeops_config({"operation":"schema","plugin":"<name>"})` to inspect them at runtime.

Spec: `specs/112-Plugin-Config-Management.md`
