# OpenCode Lib Folder Prompt

This folder contains the **plugin internals** — the actual implementation of every Axiom plugin. Each file in `.opencode/plugins/` is a thin barrel that re-exports from here. This separation exists because OpenCode's plugin loader iterates `Object.values(module)` on every file in `plugins/` and crashes on non-function exports.

**Parent**: `.memory-bank/_prompt.md` (root rules apply unless overridden here)
**Sibling**: `.opencode/plugins/_prompt.md` (plugin barrel rules)

---

## Critical Rules

### MUST — barrel pattern is sacred

Files in this folder are imported BY their barrel in `plugins/`. They CAN have many exports (helpers, types, constants, classes) without crashing OpenCode, because OpenCode never directly imports from `lib/` — only the barrel does.

**However**: tests import directly from here (NOT from `plugins/`) to access internals. If you change an export here, update both the barrel (if needed) AND any tests that imported it.

### MUST — use the shared logging helpers

`lib/config-utils.ts` exports `pluginWarn`, `pluginError`, `pluginInfo`. These are env-gated structured loggers. Use them instead of `console.warn`/`console.error`/`process.stderr.write`. OpenCode's TUI captures stdio from plugin code paths and floods the chat pane.

```ts
import { pluginWarn, pluginError } from "./config-utils.ts";

pluginWarn("my-plugin", "config field missing", { field: "name" });
pluginError("my-plugin", "DB write failed after retries", { error: errMsg });
```

To enable logging in dev:
```bash
export AXIOM_PLUGIN_DEBUG=1                 # all plugins
export AXIOM_<PLUGIN_NAME>_DEBUG=1          # specific plugin (uppercase, dashes→underscores)
```

### MUST — use Zod schemas for tool args (not raw JSON Schema)

When defining tool args, use `tool.schema.*` from `@opencode-ai/plugin`:

```ts
import { tool } from "@opencode-ai/plugin";

const myTool = tool({
  description: "...",
  args: {
    name: tool.schema.string().describe("..."),
    count: tool.schema.number().optional(),
    mode: tool.schema.enum(["fast", "slow"]),
  },
  execute: async (args) => { ... }
});
```

**Never** use raw JSON Schema like `{ type: "object", properties: {...} }`. It produces invalid JSON Schema 2020-12 and Bedrock rejects it at runtime (Bug 9 in the plugin tools SDK best practice).

### MUST — `args:` not `parameters:`

The OpenCode `tool()` helper expects `args:`. Using `parameters:` makes args undefined → crashes the registry on `Object.entries(undefined)`.

### MUST NOT — `z.any()`, `z.unknown()`, `z.record(z.any())`

OpenCode's bundled Zod v4 `toJSONSchema` crashes on these (the conversion fails because `_zod.def` is undefined on those types). For map-like values, use `tool.schema.record(tool.schema.string(), tool.schema.string())`. For arbitrary JSON, accept a string and parse internally.

---

## When Adding a New Plugin

1. Create `lib/<name>.ts` with:
   - Plugin factory: `export const <Name>Plugin = async ({ directory, client }) => { return { tool: {...}, ...hooks }; }`
   - Internal helpers, types, constants (no restrictions on count or shape)
   - Use `pluginWarn`/`pluginError`/`pluginInfo` for diagnostics

2. Create `plugins/<name>.ts` (barrel) with ONLY:
   ```ts
   import { <Name>Plugin } from "../lib/<name>.ts";
   export { <Name>Plugin };
   ```

3. Create `tests/<name>.test.ts` with:
   ```ts
   import { <Name>Plugin, <other internals> } from "../lib/<name>.ts";
   ```

4. Update inventories:
   - `lib/_index.md` (this folder's index)
   - `plugins/_index.md`
   - `tests/plugin-null-guard.test.ts` PLUGINS list
   - `tests/plugin-tool-surface.test.ts` PLUGIN_SURFACES list
   - `.opencode/skills/axiom-pattern-generator/REFERENCE.md`

---

## Common Patterns in This Folder

### Pattern: Plugin factory with config loading

```ts
import { loadPluginConfig, pluginWarn } from "./config-utils.ts";

const DEFAULT_CONFIG = { /* ... */ };

export const MyPlugin = async ({ directory, client }) => {
  let config;
  try {
    config = loadPluginConfig("my-plugin", DEFAULT_CONFIG, directory);
  } catch (err) {
    pluginWarn("my-plugin", `loadPluginConfig failed (${err}), using defaults`);
    config = structuredClone(DEFAULT_CONFIG);
  }
  return { tool: { /* ... */ } };
};
```

### Pattern: SDK structured logging via client.app.log

When the plugin factory has access to `client` (always for new plugins), prefer the OpenCode SDK logger:

```ts
export const MyPlugin = async ({ client }) => {
  const log = async (level, message, extra) => {
    try {
      await client?.app?.log?.({ body: { service: "my-plugin", level, message, extra } });
    } catch { /* swallow — never break runtime */ }
  };
  
  await log("info", "plugin initialized");
  return { tool: {} };
};
```

For deep utility code that doesn't have client, use the env-gated `pluginWarn` etc. instead.

### Pattern: Zod-converted tools

```ts
"my_tool": tool({
  description: "What this tool does",
  args: {
    required_field: tool.schema.string().describe("Required: ..."),
    optional_field: tool.schema.boolean().optional().describe("Optional: ..."),
    enum_field: tool.schema.enum(["a", "b", "c"]),
    list_field: tool.schema.array(tool.schema.string()).optional(),
  },
  async execute(args: Record<string, unknown>) {
    // ...
    return JSON.stringify({ result: ... });
  },
}),
```

---

## Anti-Patterns (DON'T)

- ❌ Don't put utility modules in `plugins/` — they crash OpenCode's loader. Put them here.
- ❌ Don't `console.log` for normal operation — use `pluginInfo` (debug-gated) or omit entirely.
- ❌ Don't accept arbitrary JSON objects as tool args — accept JSON strings and parse internally (Bedrock can't validate `{type: "object"}` without properties).
- ❌ Don't import internals into the barrel — barrels should ONLY re-export the plugin function.
- ❌ Don't break the `client.app.log` pattern by falling back to console.* on failure — the original problem was stdio leaking to the UI.

---

## Related Files and References

| File | Purpose |
|------|---------|
| `_index.md` | Lib inventory (this folder) |
| `.opencode/plugins/_prompt.md` | Plugin barrel rules |
| `.opencode/plugins/_index.md` | Plugin inventory |
| `lib/config-utils.ts` | `loadPluginConfig` + `pluginWarn`/`pluginError`/`pluginInfo` helpers |
| `.opencode/tests/` | Test directory (imports from here) |
| `.memory-bank/best-practices/opencode-plugin-tools-sdk.md` | **Full bug catalog and patterns** |
| `.opencode/skills/axiom-pattern-generator/REFERENCE.md` | Tool API quick reference |
| `specs/70-OpenCode-Plugin.md` | Plugin spec |
| `specs/121-Pattern-Generator.md` | Pattern generator spec |
| https://opencode.ai/docs/plugins/ | Official OpenCode plugin docs |

axiom:trace work_item=plugin-loader-bugs-01 spec=specs/70-OpenCode-Plugin.md doc=.memory-bank/best-practices/opencode-plugin-tools-sdk.md
