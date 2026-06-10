/**
 * Axiom Plugin — stub (source plugin not yet built).
 * 
 * This is a temporary stub. The real plugin will be built from
 * .axiom/plugin/src/ when the plugin source is available.
 * 
 * Spec: specs/70-OpenCode-Plugin.md#REQ-PLG-002
 */
export function CodeOpsPlugin(input) {
  return {
    name: "codeops-stub",
    async init() {
      // Plugin source not yet built — no-op stub
    },
    async tool(toolcall) {
      return { result: "axiom-plugin-stub: tool execution unavailable (plugin source not built)" };
    }
  };
}
