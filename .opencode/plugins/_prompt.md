# OpenCode Plugins Folder Prompt

This folder contains the **plugin entry points** that OpenCode loads at startup. Every `.ts` and `.js` file here is auto-discovered and called as a plugin factory.

**Parent**: `.memory-bank/_prompt.md` (root rules apply unless overridden here)

---

## Critical Rules (HARD — break these and OpenCode crashes)

### MUST — barrel pattern only

Each `plugins/<name>.ts` file MUST be a thin barrel that exports ONLY the plugin factory function. All implementation lives in `.opencode/lib/<name>.ts`.

**Why**: OpenCode's `getLegacyPlugins` iterates `Object.values(module)` and calls EVERY exported value as a plugin factory with `({directory, client})`. Helper functions, constants, classes, interfaces — all crash with "Plugin export is not a function" or "paths[0] must be string, got object" when called with the wrong args.

```ts
// ✅ CORRECT — plugins/my-plugin.ts (barrel)
import { MyPlugin } from "../lib/my-plugin.ts";
export { MyPlugin };
```

```ts
// ❌ WRONG — multiple exports in plugins/
export const MY_CONST = 42;          // crashes loader
export interface MyConfig { ... }    // crashes loader
export function helper() { ... }     // crashes loader
export const MyPlugin = ...;
```

### MUST — return `{ tool: ... }` always

Every plugin factory MUST return an object with a `tool` field — even if empty. OpenCode's `ToolRegistry.state` calls `Object.entries(plugin.tool)` and crashes on `undefined` or `null`. Hook-only plugins still need `tool: {}`.

```ts
// ✅ CORRECT — hook-only plugin
return {
  tool: {},
  "experimental.session.compacting": async (...) => { ... }
};

// ❌ WRONG — missing tool field
return {
  "experimental.session.compacting": async (...) => { ... }
};
```

### MUST — use `args:` not `parameters:` in tool definitions

The OpenCode `tool()` helper expects Zod schemas in `args:`. Using `parameters:` leaves `args` undefined → `Object.entries(undefined)` crash.

```ts
// ✅ CORRECT
tool({
  description: "...",
  args: { foo: tool.schema.string() },
  execute: async (args) => { ... }
});

// ❌ WRONG
tool({
  description: "...",
  parameters: { foo: tool.schema.string() },  // crashes registry
  execute: async (args) => { ... }
});
```

### MUST — Zod schemas, not raw JSON Schema

Tool args MUST be Zod via `tool.schema.*`. Raw JSON Schema (`{ type: "object", properties: {...} }`) produces invalid JSON Schema 2020-12 output that Bedrock rejects at runtime.

```ts
// ✅ CORRECT
args: {
  feed_id: tool.schema.string().describe("..."),
  enabled: tool.schema.boolean().optional(),
}

// ❌ WRONG — raw JSON Schema
args: {
  type: "object",
  properties: {
    feed_id: { type: "string" },
    enabled: { type: "boolean" },
  },
  required: ["feed_id"],
}
```

### MUST — tool names use `[a-zA-Z0-9_-]+` only (no dots)

AWS Bedrock rejects tool names with dots. Use underscores: `feed_list`, not `feed.list`.

### MUST NOT — use `console.log`/`process.stderr.write` directly

OpenCode's TUI captures plugin stdio and floods the conversation pane. Use `client.app.log()` (when factory has client access) or the `pluginWarn`/`pluginError`/`pluginInfo` helpers from `lib/config-utils.ts`. These helpers are env-gated behind `AXIOM_<PLUGIN>_DEBUG=1`.

See: `.memory-bank/best-practices/opencode-plugin-tools-sdk.md` Bug 10.

### MUST NOT — put test files (`*.test.ts`) in `plugins/`

OpenCode loads them as plugins → `beforeAll() outside test runner` crash. Test files live in `.opencode/tests/`.

---

## When Adding a New Plugin

1. **Implementation in `lib/`**: Create `.opencode/lib/<name>.ts` with the plugin factory and all helpers/types/constants.
2. **Barrel in `plugins/`**: Create `.opencode/plugins/<name>.ts` with ~4 lines:
   ```ts
   import { <Name>Plugin } from "../lib/<name>.ts";
   export { <Name>Plugin };
   ```
3. **Tests in `tests/`**: Create `.opencode/tests/<name>.test.ts`. Import internals from `../lib/<name>.ts` (NOT from `../plugins/`).
4. **Update inventories**:
   - This folder's `_index.md` (add the new plugin)
   - `.opencode/skills/axiom-pattern-generator/REFERENCE.md` (add the tool API)
   - `.opencode/tests/plugin-null-guard.test.ts` PLUGINS list (add for regression coverage)
   - `.opencode/tests/plugin-tool-surface.test.ts` PLUGIN_SURFACES (add expected tools)

---

## When Modifying an Existing Plugin

1. Edit `lib/<name>.ts` (NOT `plugins/<name>.ts` unless changing the barrel itself).
2. If adding/removing tools, update `plugin-tool-surface.test.ts` `PLUGIN_SURFACES.<name>.expectedTools`.
3. Run regression suites BEFORE restarting OpenCode:
   ```bash
   cd .opencode && bun test tests/plugin-null-guard.test.ts tests/plugin-tool-surface.test.ts
   ```
4. If any console.log/process.std{out,err} added, the WARN test (Suite 8) will list it. Migrate to `pluginWarn`/`pluginError` from `lib/config-utils.ts`.

---

## Debugging Plugin Loader Crashes

**Read first**: `.memory-bank/best-practices/opencode-plugin-tools-sdk.md` — has the full 10-bug catalog with error signatures, root causes, exact fixes, and which test catches each one.

**5-minute triage** when OpenCode crashes:
1. `cd .opencode && bun test tests/plugin-null-guard.test.ts tests/plugin-tool-surface.test.ts` — catches 95% of plugin loader bugs
2. If those pass: `opencode serve --print-logs` → grep for `"failed to load plugin"` and `CRITICAL`
3. Match the error signature against the bug catalog — each bug has an exact fix

---

## Related Files and References

| File | Purpose |
|------|---------|
| `_index.md` | Plugin inventory (this folder) |
| `.opencode/lib/<name>.ts` | Plugin internals (where you actually edit) |
| `.opencode/lib/config-utils.ts` | Shared `pluginWarn`/`pluginError`/`pluginInfo` helpers |
| `.opencode/tests/plugin-null-guard.test.ts` | Regression suite — runtime crash protection |
| `.opencode/tests/plugin-tool-surface.test.ts` | Regression suite — surface drift protection |
| `.memory-bank/best-practices/opencode-plugin-tools-sdk.md` | **Full bug catalog and patterns — start here for any plugin issue** |
| `.opencode/skills/axiom-pattern-generator/REFERENCE.md` | Tool API quick reference for all plugins |
| `specs/70-OpenCode-Plugin.md` | Plugin spec |
| `specs/121-Pattern-Generator.md` | How to combine plugin tools into reusable patterns |
| https://opencode.ai/docs/plugins/ | Official OpenCode plugin docs |

---

## OpenCode Plugin Loader Behavior (How OpenCode Calls Us)

When OpenCode starts:

1. Discovers all `.ts`/`.js` files in `plugins/` (this folder).
2. For each file, dynamically imports the module.
3. Iterates `Object.values(module)` and calls each exported function as `factory({directory, client, project, worktree, $})`.
4. Each factory returns `{ tool: {...}, "hook.name": handler, ... }`.
5. ToolRegistry merges all returned tools into the global tool set.
6. Hooks fire on session lifecycle events.

**The barrel pattern works because step 3 only finds 1 export.** Anything more, and OpenCode tries to call it as a plugin factory.

axiom:trace work_item=plugin-loader-bugs-01 spec=specs/70-OpenCode-Plugin.md doc=.memory-bank/best-practices/opencode-plugin-tools-sdk.md
