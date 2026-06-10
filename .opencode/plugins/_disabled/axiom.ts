/**
 * Axiom Plugin — local plugin wrapper for OpenCode.
 *
 * This file is the local plugin entry point for `.opencode/plugins/`.
 * OpenCode's plugin loader (getLegacyPlugins) iterates Object.values(module)
 * and calls each exported function as a plugin. It throws if any export is
 * not a function.
 *
 * IMPORT FROM DIST (not src): We import from `.axiom/plugin/dist/index.js`
 * (compiled JavaScript) rather than `.axiom/plugin/src/index.ts` (TypeScript).
 * This eliminates the hidden Bun TypeScript transpiler runtime dependency that
 * causes ERR_MODULE_NOT_FOUND when running under Node.js v25.6.0 or other
 * environments without Bun's TS transpilation. Using dist/ ensures the plugin
 * works in any OpenCode distribution (Bun, Node.js, Deno).
 *
 * Spec: specs/70-OpenCode-Plugin.md#REQ-PLG-002
 * axiom:trace work_item=DEX-310 spec=specs/70-OpenCode-Plugin.md#REQ-PLG-002 jira_ref=DEX-310
 */

import { CodeOpsPlugin } from "../../.axiom/plugin/dist/index.js";

// Re-export ONLY the plugin function.
// OpenCode's getLegacyPlugins iterates Object.values(module) and throws
// "Plugin export is not a function" for any non-function export.
// Exporting only CodeOpsPlugin ensures the loader finds exactly one plugin.
export { CodeOpsPlugin };
