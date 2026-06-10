---
name: context-stash-axiom
description: >-
  Context Stash plugin and the three-layer configuration system (defaults → file → env vars).
  Covers stash.store/peek/list/delete tools, the loadPluginConfig() pattern for all plugins,
  env var override convention (AXIOM_<PLUGIN>__<FIELD>), file-layer config, schema validation,
  and migration patterns. Load when working with context stash operations or when any plugin
  needs configuration loading guidance.
version: "1.0"
tags:
  vertical: [configuration, memory, data]
  category: configuration
  core: false
spec: specs/106-Context-Stash.md
---

# Context Stash + Plugin Configuration System

The Context Stash is a per-session key-value store for passing structured context between agents, tools, and lifecycle hooks. It also serves as the reference implementation for the three-layer plugin configuration system (`loadPluginConfig()`).

**Spec (stash)**: `specs/106-Context-Stash.md`
**Spec (config)**: `specs/112-Plugin-Config-Management.md`
**Plugin**: `.opencode/plugins/context-stash.ts`
**Config utils**: `.opencode/plugins/config-utils.ts`
**Schema**: `.opencode/config/schemas/context-stash.schema.json`

<!-- axiom:trace work_item=SWDE-44 spec=specs/106-Context-Stash.md -->

---

## Quick Start (Setup + First Use)

```bash
# The context-stash plugin is auto-loaded by OpenCode — no separate install needed.
# Verify it's working:
cd .opencode && bun test tests/context-stash-pcm.test.ts

# To configure (optional — works with defaults out of the box):
# Create .opencode/config/context-stash.json for file-layer overrides
# Or set env vars: AXIOM_CONTEXT_STASH_TTL__DEFAULT_SECONDS=3600
```

The stash is immediately available to agents via tools:

```typescript
// Store context that other agents in this session can read
await stash.store("analysis_result", { 
  health_score: 87, 
  issues: 3, 
  recommendation: "fix N+1 query in user_loader" 
});

// Later (same session) — another agent retrieves it
const result = await stash.peek("analysis_result");
// → { health_score: 87, issues: 3, recommendation: "..." }

// List what's stored
const keys = await stash.list();
// → ["analysis_result", "plan_cursor", "last_dispatch"]
```

For plugin config management (`loadPluginConfig`):

```typescript
import { loadPluginConfig } from "./config-utils.ts";

// Any plugin can use three-layer config (defaults → file → env vars)
const config = loadPluginConfig("my-plugin", MY_DEFAULT_CONFIG, directory);
// Reads: code defaults → .opencode/config/my-plugin.json → AXIOM_MY_PLUGIN__* env vars
```

---

## When to Load This Skill

Load when an agent needs to:
- Store/retrieve context between agent invocations within a session
- Configure any OpenCode plugin using the three-layer system
- Migrate a plugin from legacy config to `loadPluginConfig()`
- Debug config loading issues (env vars not taking effect, file not found, schema validation failing)
- Understand the `AXIOM_<PLUGIN>__<FIELD>` env var convention

---

## Context Stash — Tools

| Tool | Description |
|------|-------------|
| `stash.store` | Store a key-value pair for the current session |
| `stash.peek` | Read a value without consuming it |
| `stash.list` | List all stored keys |
| `stash.delete` | Remove a key |

### Usage Pattern

```typescript
// Store context from one agent
await stash.store("plan_summary", { phases: 3, steps: 12, cursor: "phase-1/task-1-1" });

// Retrieve in another agent/tool within the same session
const plan = await stash.peek("plan_summary");
```

---

## Three-Layer Plugin Configuration

All Axiom plugins use `loadPluginConfig()` from `config-utils.ts`. The system resolves config from three layers (highest wins):

```
Layer 3: Environment variables     ← AXIOM_<PLUGIN>__<FIELD>=value
Layer 2: File config               ← <plugin>.json or <plugin>.local.json in config dir
Layer 1: Code defaults             ← DEFAULT_CONFIG in the plugin source
```

### Env Var Convention

`AXIOM_<PLUGIN>__<FIELD>` with `__` separating nested paths:

```bash
# Set conductor max concurrent agents
export AXIOM_CONDUCTOR_LIMITS__MAX_CONCURRENT_AGENTS=5

# Set context-stash TTL
export AXIOM_CONTEXT_STASH_TTL__DEFAULT_SECONDS=3600
```

### File Config

```jsonc
// .opencode/config/context-stash.json
{
  "ttl": { "default_seconds": 3600 },
  "max_entries": 1000
}
```

Local overrides (gitignored):
```jsonc
// .opencode/config/context-stash.local.json
{
  "ttl": { "default_seconds": 60 }  // fast TTL for testing
}
```

### Using `loadPluginConfig()`

```typescript
import { loadPluginConfig } from "./config-utils.ts";

const DEFAULT_CONFIG = {
  ttl: { default_seconds: 3600 },
  max_entries: 1000,
  persistence: { enabled: true }
};

// Resolves Layer 1 → 2 → 3 (highest wins)
const config = loadPluginConfig("context-stash", DEFAULT_CONFIG, directory);
```

---

## Schema Validation

Each plugin's config schema lives at `.opencode/config/schemas/<plugin>.schema.json`. The `loadPluginConfig()` function validates the merged config against this schema. Invalid values produce clear error messages.

---

## Migration Guide (Adopting loadPluginConfig)

When migrating a plugin to the three-layer system:

1. **Define `DEFAULT_CONFIG`** — export it from the plugin source
2. **Create schema** — `.opencode/config/schemas/<plugin>.schema.json`
3. **Replace legacy loading** — swap bespoke config loading with `loadPluginConfig()`
4. **Update tests** — use env vars instead of test backdoors (see `conductor-phase4-config-adoption`)
5. **Add migration warning** — if the plugin had old-format config, warn and show the new format

### Plugins Already Adopted

| Plugin | Status | Config dir |
|--------|--------|-----------|
| `graph-harness.ts` | ✅ Adopted | `.opencode/config/graph-harness.json` |
| `conductor.ts` | ✅ Adopted | `.opencode/config/conductor.json` |
| `context-stash.ts` | ✅ Adopted | `.opencode/config/context-stash.json` |
| `opencode-session.ts` | ✅ Adopted | `.opencode/config/opencode-session.json` |

---

## Key Files

| Path | Purpose |
|------|---------|
| `.opencode/plugins/context-stash.ts` | Context stash plugin |
| `.opencode/plugins/config-utils.ts` | `loadPluginConfig()` + `writePluginConfig()` + `getConfigInfo()` |
| `.opencode/config/schemas/context-stash.schema.json` | Schema for stash config |
| `specs/106-Context-Stash.md` | Stash spec |
| `specs/112-Plugin-Config-Management.md` | Config system spec (three-layer, env vars, schemas) |

---

## ADRs

- `ADR-TM-001` — DuckDB v1 deviation (related to tree-memory, not stash)
- Conductor backdoor removal documented in `conductor-phase4-config-adoption` work item

axiom:trace work_item=SWDE-44 spec=specs/106-Context-Stash.md spec=specs/112-Plugin-Config-Management.md
