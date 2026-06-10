/**
 * Plugin Tool Surface Completeness — regression suite for ALL plugins.
 *
 * Companion to plugin-null-guard.test.ts. Where null-guard catches RUNTIME
 * crashes (tool registration failures), this catches SURFACE shrinkage:
 *   - A tool gets accidentally renamed or dropped during refactoring
 *   - A tool's args schema becomes empty
 *   - A new tool gets added without spec/test coverage
 *
 * Pattern follows shellops-plugin.test.ts SC-1 (Surface Completeness).
 * Each plugin declares its expected tool inventory. Tests fail if any tool
 * is missing or unexpected — which forces developers to update the inventory
 * deliberately when changing the surface.
 *
 * axiom:trace work_item=plugin-loader-bugs-01 spec=specs/121-Pattern-Generator.md
 */
import { describe, it, expect } from "bun:test";
import { mkdtempSync, mkdirSync } from "fs";
import { join } from "path";
import { tmpdir } from "os";

const mockClient = { send: () => {}, on: () => {}, app: { log: async () => {} } };

function makeTempDir(prefix: string): string {
  const dir = mkdtempSync(join(tmpdir(), `surface-${prefix}-`));
  for (const sub of [
    [".memory-bank", "stash"],
    [".graph-harness"],
    [".conductor"],
    [".opencode", "config"],
    [".tree-memory"],
    [".axiom", "feeds"],
    [".memory-bank", "feed-state"],
  ]) {
    mkdirSync(join(dir, ...sub), { recursive: true });
  }
  return dir;
}

interface PluginSurface {
  name: string;
  importPath: string;
  exportName: string;
  /** The full set of tools this plugin should register. */
  expectedTools: string[];
  /** Set if this plugin requires env vars to instantiate. */
  requiresEnv?: Record<string, string>;
  /** Allow extra tools beyond expectedTools (default: false — strict mode). */
  allowExtras?: boolean;
}

// ─────────────────────────────────────────────────────────────────────────────
// EXPECTED TOOL INVENTORY — update deliberately when adding/removing tools.
// Numbers in the comment are the expected count for each plugin.
// ─────────────────────────────────────────────────────────────────────────────

const PLUGIN_SURFACES: PluginSurface[] = [
  {
    name: "graph-harness",
    importPath: "../plugins/graph-harness.ts",
    exportName: "GraphHarnessPlugin",
    // 20 tools — full graph DAG execution surface
    expectedTools: [
      "graph_create", "graph_status", "graph_inject", "graph_modify", "graph_split",
      "graph_annotate", "graph_abandon", "graph_unblock", "graph_output", "graph_dataflow",
      "graph_message", "graph_template_load", "graph_template_save", "graph_admin",
      "graph_lock", "graph_unlock", "graph_transfer", "graph_session_list",
      "graph_export", "graph_import",
    ],
  },
  {
    name: "tree-memory",
    importPath: "../plugins/tree-memory.ts",
    exportName: "TreeMemoryPlugin",
    // 12 tools — branch + commit + query interface for tree-structured memory
    expectedTools: [
      "tree_init", "tree_branch", "tree_commit", "tree_promote", "tree_merge",
      "tree_state", "tree_query", "tree_peers", "tree_log", "tree_diff",
      "tree_spawn", "tree_status",
    ],
  },
  {
    name: "context-stash",
    importPath: "../plugins/context-stash.ts",
    exportName: "ContextStashPlugin",
    // 29 tools — comprehensive context-stash surface
    expectedTools: [
      "stash_append", "stash_apply", "stash_archive", "stash_cleanup",
      "stash_close", "stash_compact", "stash_context", "stash_create",
      "stash_drop", "stash_enter", "stash_exit", "stash_headers",
      "stash_ingest", "stash_list", "stash_lock", "stash_log",
      "stash_migrate", "stash_node_complete", "stash_node_enter",
      "stash_peek", "stash_pop", "stash_push", "stash_ref",
      "stash_related", "stash_search", "stash_summarize", "stash_switch",
      "stash_tag", "stash_unlock",
    ],
  },
  {
    name: "conductor",
    importPath: "../plugins/conductor.ts",
    exportName: "ConductorPlugin",
    // 13 tools — full conductor agent management surface
    expectedTools: [
      "conductor_spawn", "conductor_status", "conductor_done", "conductor_result",
      "conductor_cancel", "conductor_wait", "conductor_collect", "conductor_relay",
      "conductor_delegate", "conductor_broadcast", "conductor_focus",
      "conductor_unfocus", "conductor_pin",
    ],
    requiresEnv: { "AXIOM_CONDUCTOR_AUTH__ALLOW_SPAWN_SECRET_FALLBACK": "true" },
  },
  {
    name: "feed-ingestion",
    importPath: "../plugins/feed-ingestion.ts",
    exportName: "FeedIngestionPlugin",
    // 8 tools — feed surface (renamed from dotted in May 2026)
    expectedTools: [
      "feed_list", "feed_status", "feed_poll", "feed_webhook",
      "feed_email", "feed_analytics", "feed_subscribe", "feed_health",
    ],
  },
  {
    name: "shellops",
    importPath: "../plugins/shellops.ts",
    exportName: "ShellOpsPlugin",
    // 26 tools — full ShellOps MCP surface (Zod migration complete 2026-05-18)
    expectedTools: [
      "shellops_broadcast",
      "shellops_classify",
      "shellops_events_listen",
      "shellops_events_query",
      "shellops_events_stop",
      "shellops_exec",
      "shellops_health",
      "shellops_investigate",
      "shellops_logs_query",
      "shellops_logs_similar",
      "shellops_nohup_check",
      "shellops_nohup_list",
      "shellops_nohup_output",
      "shellops_profile_load",
      "shellops_profile_query",
      "shellops_status",
      "shellops_terminal_capture",
      "shellops_terminal_create",
      "shellops_terminal_destroy",
      "shellops_terminal_list",
      "shellops_terminal_run",
      "shellops_triage",
      "shellops_watch_list",
      "shellops_watch_query",
      "shellops_watch_start",
      "shellops_watch_stop",
    ],
  },
  {
    name: "config-tool",
    importPath: "../plugins/config-tool.ts",
    exportName: "ConfigToolPlugin",
    expectedTools: ["codeops_config"],
  },
];

async function loadPluginTools(p: PluginSurface): Promise<Record<string, unknown>> {
  const dir = makeTempDir(p.name);
  const originalEnv: Record<string, string | undefined> = {};
  if (p.requiresEnv) {
    for (const [k, v] of Object.entries(p.requiresEnv)) {
      originalEnv[k] = process.env[k];
      process.env[k] = v;
    }
  }
  try {
    const mod = await import(p.importPath);
    const factory = mod[p.exportName];
    const result = await factory({ directory: dir, client: mockClient });
    return (result?.tool as Record<string, unknown>) ?? {};
  } finally {
    for (const [k, v] of Object.entries(originalEnv)) {
      if (v === undefined) delete process.env[k];
      else process.env[k] = v;
    }
  }
}

// ─────────────────────────────────────────────────────────────────────────────
// SUITE 1: Every expected tool is registered (no surface shrinkage)
// ─────────────────────────────────────────────────────────────────────────────

describe("Tool Surface Completeness — expected tools are registered", () => {
  for (const plugin of PLUGIN_SURFACES) {
    it(`${plugin.name}: all ${plugin.expectedTools.length} expected tools are present`, async () => {
      const tools = await loadPluginTools(plugin);
      const actual = new Set(Object.keys(tools));
      const missing = plugin.expectedTools.filter(t => !actual.has(t));

      if (missing.length > 0) {
        throw new Error(
          `Plugin "${plugin.name}" is missing ${missing.length} expected tool(s):\n  ` +
          missing.join(", ") +
          `\n\nThis means a tool was renamed or removed without updating the inventory.\n` +
          `Either restore the tool, or update PLUGIN_SURFACES in this test deliberately.`
        );
      }
      expect(missing.length).toBe(0);
    });
  }
});

// ─────────────────────────────────────────────────────────────────────────────
// SUITE 2: No unexpected tools (strict mode for non-allowExtras plugins)
// Forces deliberate updates when surface grows.
// ─────────────────────────────────────────────────────────────────────────────

describe("Tool Surface Completeness — no unexpected tools (strict)", () => {
  for (const plugin of PLUGIN_SURFACES) {
    if (plugin.allowExtras) continue;
    it(`${plugin.name}: no tools beyond the expected ${plugin.expectedTools.length}`, async () => {
      const tools = await loadPluginTools(plugin);
      const expected = new Set(plugin.expectedTools);
      const extras = Object.keys(tools).filter(t => !expected.has(t));

      if (extras.length > 0) {
        throw new Error(
          `Plugin "${plugin.name}" has ${extras.length} unexpected tool(s):\n  ` +
          extras.join(", ") +
          `\n\nA new tool was added — update PLUGIN_SURFACES.${plugin.name}.expectedTools ` +
          `in this test to reflect the new surface deliberately.\n` +
          `If this tool is intentional, also add it to the spec, README, and any cross-references.`
        );
      }
      expect(extras.length).toBe(0);
    });
  }
});

// ─────────────────────────────────────────────────────────────────────────────
// SUITE 3: Every tool has complete metadata
// Catches: tools missing description, tools with empty args, tools without execute
// ─────────────────────────────────────────────────────────────────────────────

describe("Tool Surface Completeness — every tool has complete metadata", () => {
  for (const plugin of PLUGIN_SURFACES) {
    it(`${plugin.name}: every tool has description, args, execute`, async () => {
      const tools = await loadPluginTools(plugin);
      const violations: string[] = [];

      for (const [name, td] of Object.entries(tools)) {
        const t = td as { description?: unknown; args?: unknown; execute?: unknown };
        if (typeof t?.description !== "string" || !t.description.trim()) {
          violations.push(`${name}: missing or empty description`);
        }
        if (t?.args === null || t?.args === undefined || typeof t.args !== "object") {
          violations.push(`${name}: args is ${t?.args === null ? "null" : typeof t?.args} (must be object)`);
        }
        if (typeof t?.execute !== "function") {
          violations.push(`${name}: execute is ${typeof t?.execute} (must be function)`);
        }
      }

      if (violations.length > 0) {
        throw new Error(
          `Plugin "${plugin.name}" has tools with incomplete metadata:\n  ` +
          violations.join("\n  ")
        );
      }
      expect(violations.length).toBe(0);
    });
  }
});

// ─────────────────────────────────────────────────────────────────────────────
// SUITE 4: Total tool count check (lock the overall registry size)
// Helps catch surprise additions/removals at a glance.
// ─────────────────────────────────────────────────────────────────────────────

describe("Tool Surface Completeness — registry total", () => {
  it("sum of all expected tools matches the actual registry total", async () => {
    let expectedTotal = 0;
    let actualTotal = 0;
    const breakdown: string[] = [];

    for (const plugin of PLUGIN_SURFACES) {
      const tools = await loadPluginTools(plugin);
      const actualCount = Object.keys(tools).length;
      const expectedCount = plugin.expectedTools.length;
      expectedTotal += plugin.allowExtras ? actualCount : expectedCount;
      actualTotal += actualCount;
      breakdown.push(`  ${plugin.name}: expected ${expectedCount}${plugin.allowExtras ? "+" : ""}, actual ${actualCount}`);
    }

    // For visibility — the test passes; this just emits the inventory
    if (actualTotal !== expectedTotal) {
      console.warn(
        `Total tool count: expected ${expectedTotal}, actual ${actualTotal}\n` +
        breakdown.join("\n")
      );
    }
    // Lock total to exact expected count — catches silent tool additions/removals
    expect(actualTotal).toBe(expectedTotal);
  });
});
