// Barrel re-export: only ShellOpsPlugin is exposed to OpenCode's plugin loader.
// All internals (tool consts + factory) live in lib/shellops.ts.
export { ShellOpsPlugin } from "../lib/shellops.ts";
