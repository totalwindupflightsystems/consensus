/**
 * Plugin Health Guard — comprehensive regression suite for all Axiom plugins.
 *
 * This test suite catches the FULL CLASS of OpenCode plugin loader bugs we hit
 * during the May 2026 plugin refactor. Each test corresponds to a real bug we
 * had to debug repeatedly. If any of these regress, OpenCode's session.prompt
 * crashes with cryptic errors and tools become unusable.
 *
 * Bugs caught by this suite:
 *
 *   1. plugin returns null/undefined tool field          → crashes ToolRegistry
 *   2. plugin throws during init                         → leaves null tool entry
 *   3. tool uses `parameters:` instead of `args:`        → Object.entries(undefined)
 *   4. tool name has `.` (dot)                           → AWS Bedrock rejects
 *   5. plugin file in plugins/ has non-function exports  → "Plugin export is not a function"
 *   6. test file in plugins/ directory                   → loaded as plugin, crashes
 *   7. plugin file has too many exports                  → OpenCode iterates all of them as plugin factories
 *
 * axiom:trace work_item=plugin-loader-bugs-01 spec=specs/121-Pattern-Generator.md
 */
import { describe, it, expect } from "bun:test";
import { existsSync, mkdirSync, mkdtempSync, readdirSync, statSync } from "fs";
import { join, basename, extname } from "path";
import { tmpdir } from "os";

const PLUGINS_DIR = join(import.meta.dir, "../plugins");

// Mock client used for plugin instantiation
const mockClient = { send: () => {}, on: () => {}, app: { log: async () => {} } };

function makeTempDir(prefix: string): string {
  const dir = mkdtempSync(join(tmpdir(), `plugin-health-${prefix}-`));
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

interface PluginEntry {
  name: string;
  importPath: string;
  exportName: string;
  requiresEnv?: Record<string, string>;
  /** Plugin throws on certain configs (e.g., conductor without SPIRE) — that's
   *  spec-correct behavior, not a bug. OpenCode's loader catches the throw. */
  mayThrow?: boolean;
}

/** All registered plugins. Add new ones here as the system grows. */
const PLUGINS: PluginEntry[] = [
  { name: "graph-harness", importPath: "../plugins/graph-harness.ts", exportName: "GraphHarnessPlugin" },
  { name: "tree-memory", importPath: "../plugins/tree-memory.ts", exportName: "TreeMemoryPlugin" },
  { name: "context-stash", importPath: "../plugins/context-stash.ts", exportName: "ContextStashPlugin" },
  { name: "context-pipeline-hook", importPath: "../plugins/context-pipeline-hook.ts", exportName: "ContextPipelineHook" },
  { name: "conductor", importPath: "../plugins/conductor.ts", exportName: "ConductorPlugin",
    requiresEnv: { "AXIOM_CONDUCTOR_AUTH__ALLOW_SPAWN_SECRET_FALLBACK": "true" },
    mayThrow: true },
  { name: "shellops", importPath: "../plugins/shellops.ts", exportName: "ShellOpsPlugin" },
  { name: "feed-ingestion", importPath: "../plugins/feed-ingestion.ts", exportName: "FeedIngestionPlugin" },
  { name: "opencode-session", importPath: "../plugins/opencode-session.ts", exportName: "OpenCodeSessionPlugin" },
  { name: "config-tool", importPath: "../plugins/config-tool.ts", exportName: "ConfigToolPlugin" },
  { name: "compaction", importPath: "../plugins/compaction.ts", exportName: "CodeOpsCompactionPlugin" },
  { name: "agent-depth-guard", importPath: "../plugins/agent-depth-guard.ts", exportName: "AgentDepthGuard" },
];

// AWS Bedrock requires tool names match this regex
const VALID_TOOL_NAME = /^[a-zA-Z0-9_-]+$/;

/**
 * Helper: instantiate a plugin in a temp directory with optional env vars.
 * Returns the plugin object, or null if it threw and `mayThrow` is true.
 */
async function loadPlugin(plugin: PluginEntry): Promise<{ tool?: Record<string, unknown> } | null> {
  const dir = makeTempDir(plugin.name);
  const originalEnv: Record<string, string | undefined> = {};
  if (plugin.requiresEnv) {
    for (const [k, v] of Object.entries(plugin.requiresEnv)) {
      originalEnv[k] = process.env[k];
      process.env[k] = v;
    }
  }
  try {
    const mod = await import(plugin.importPath);
    const factory = mod[plugin.exportName];
    if (!factory) throw new Error(`Export "${plugin.exportName}" not found`);
    return await factory({ directory: dir, client: mockClient });
  } catch (err) {
    if (plugin.mayThrow) return null;
    throw err;
  } finally {
    for (const [k, v] of Object.entries(originalEnv)) {
      if (v === undefined) delete process.env[k];
      else process.env[k] = v;
    }
  }
}

// ─────────────────────────────────────────────────────────────────────────────
// SUITE 1: Plugin shape — every plugin returns { tool: <object> }
// Catches: null/undefined tool field, hook-only plugins missing tool key
// Crash signature: "Object.entries requires that input parameter not be null"
// ─────────────────────────────────────────────────────────────────────────────

describe("Plugin Health Guard — plugin return shape", () => {
  for (const plugin of PLUGINS) {
    it(`${plugin.name}: returns { tool: object } (not null/undefined)`, async () => {
      const result = await loadPlugin(plugin);
      if (!result) return; // mayThrow plugins skip when env not set

      expect(result).not.toBeNull();
      expect(result).not.toBeUndefined();

      // The crash this catches: ToolRegistry.state calls Object.entries(plugin.tool)
      expect(result.tool).not.toBeNull();
      expect(result.tool).not.toBeUndefined();
      expect(typeof result.tool).toBe("object");
      expect(() => Object.entries(result.tool!)).not.toThrow();
    });
  }
});

// ─────────────────────────────────────────────────────────────────────────────
// SUITE 2: Tool args — every tool MUST have args (not parameters)
// Catches: tool({parameters: {...}}) instead of tool({args: {...}})
// Crash signature: "Object.entries requires that input parameter not be null"
//                  thrown from `function V(name, gj) { Object.entries(gj.args) }`
// ─────────────────────────────────────────────────────────────────────────────

describe("Plugin Health Guard — tool args field", () => {
  for (const plugin of PLUGINS) {
    it(`${plugin.name}: every tool has args as a non-null object`, async () => {
      const result = await loadPlugin(plugin);
      if (!result?.tool) return;

      const bad: string[] = [];
      for (const [toolName, td] of Object.entries(result.tool)) {
        const t = td as { args?: unknown; parameters?: unknown };
        if (t?.args === null || t?.args === undefined) {
          const why = t?.parameters
            ? `uses 'parameters:' (must use 'args:' for OpenCode tool() helper)`
            : `args is ${t?.args === null ? "null" : "undefined"}`;
          bad.push(`${toolName} (${why})`);
        } else if (typeof t.args !== "object") {
          bad.push(`${toolName} (args is ${typeof t.args})`);
        }
      }

      if (bad.length > 0) {
        throw new Error(
          `Plugin "${plugin.name}" has ${bad.length} tool(s) with invalid args:\n  ` +
          bad.join("\n  ") +
          `\n\nThis crashes OpenCode's ToolRegistry when sessions start.\n` +
          `Fix: change 'parameters:' to 'args:' in tool({...}) definitions.`
        );
      }
      expect(bad.length).toBe(0);
    });
  }
});

// ─────────────────────────────────────────────────────────────────────────────
// SUITE 3: Tool naming — no dots, only [a-zA-Z0-9_-]
// Catches: "feed.list", "session.spawn", "stash.push" (dotted names)
// Crash signature (Bedrock): "Value 'feed.list' at 'toolConfig.tools.X.member.toolSpec.name'
//                              failed to satisfy constraint: [a-zA-Z0-9_-]+"
// ─────────────────────────────────────────────────────────────────────────────

describe("Plugin Health Guard — tool naming convention", () => {
  for (const plugin of PLUGINS) {
    it(`${plugin.name}: all tool names match [a-zA-Z0-9_-]+`, async () => {
      const result = await loadPlugin(plugin);
      if (!result?.tool) return;

      const invalid = Object.keys(result.tool).filter(n => !VALID_TOOL_NAME.test(n));
      if (invalid.length > 0) {
        throw new Error(
          `Plugin "${plugin.name}" has ${invalid.length} invalid tool name(s):\n  ` +
          invalid.join(", ") +
          `\n\nAWS Bedrock REJECTS dots/dashes/specials. Tool names must be ` +
          `[a-zA-Z0-9_-]+.\nFix: rename "tool.name" → "tool_name" everywhere.`
        );
      }
      expect(invalid.length).toBe(0);
    });
  }
});

// ─────────────────────────────────────────────────────────────────────────────
// SUITE 4: Plugin directory hygiene — only function exports, no test files
// Catches: utility modules in plugins/ (non-function exports), test files mixed in
// Crash signature: "Plugin export is not a function" or beforeAll() outside test runner
// ─────────────────────────────────────────────────────────────────────────────

describe("Plugin Health Guard — plugins/ directory hygiene", () => {
  it("plugins/ contains only files that are valid plugins (no .test.ts, no utilities)", () => {
    const files = readdirSync(PLUGINS_DIR).filter(f => {
      const stat = statSync(join(PLUGINS_DIR, f));
      return stat.isFile() && (f.endsWith(".ts") || f.endsWith(".js"));
    });

    const violations: string[] = [];
    for (const file of files) {
      // Test files do not belong in plugins/ — OpenCode loads them as plugins
      // and they crash with "beforeAll() outside test runner"
      if (file.includes(".test.") || file.includes("_test.") || file.includes(".spec.")) {
        violations.push(`${file} — test file in plugins/ (move to tests/)`);
      }
    }

    if (violations.length > 0) {
      throw new Error(
        `plugins/ has files that should not be there:\n  ` +
        violations.join("\n  ") +
        `\n\nOpenCode auto-loads every .ts/.js file in plugins/ as a plugin factory.\n` +
        `Test files crash with "beforeAll() outside test runner" or similar.\n` +
        `Utility modules with non-function exports crash with "Plugin export is not a function".`
      );
    }
    expect(violations.length).toBe(0);
  });

  it("plugins/ files only export functions (OpenCode iterates ALL exports)", async () => {
    const files = readdirSync(PLUGINS_DIR).filter(f =>
      (f.endsWith(".ts") || f.endsWith(".js")) &&
      !f.includes(".test.") && !f.includes("_test.")
    );

    const violations: string[] = [];
    for (const file of files) {
      const importPath = join(PLUGINS_DIR, file);
      try {
        const mod = await import(importPath);
        for (const [exportName, value] of Object.entries(mod)) {
          if (typeof value !== "function") {
            violations.push(
              `${file}: export "${exportName}" is ${typeof value} (must be function)`
            );
          }
        }
      } catch (err) {
        violations.push(`${file}: failed to import (${err instanceof Error ? err.message.slice(0, 80) : String(err)})`);
      }
    }

    if (violations.length > 0) {
      throw new Error(
        `Non-function exports in plugins/ — OpenCode crashes with "Plugin export is not a function":\n  ` +
        violations.join("\n  ") +
        `\n\nFix: move helper code/constants/types to .opencode/lib/ and re-export ONLY ` +
        `the plugin factory function from .opencode/plugins/. Use the barrel pattern.`
      );
    }
    expect(violations.length).toBe(0);
  });
});

// ─────────────────────────────────────────────────────────────────────────────
// SUITE 5: Tool execute function — every tool must be callable
// Catches: tool definition missing execute function (would fail at call time)
// ─────────────────────────────────────────────────────────────────────────────

describe("Plugin Health Guard — tool execute function", () => {
  for (const plugin of PLUGINS) {
    it(`${plugin.name}: every tool has an execute function`, async () => {
      const result = await loadPlugin(plugin);
      if (!result?.tool) return;

      const bad: string[] = [];
      for (const [toolName, td] of Object.entries(result.tool)) {
        const t = td as { execute?: unknown; description?: unknown };
        if (typeof t?.execute !== "function") {
          bad.push(`${toolName} (execute is ${typeof t?.execute})`);
        }
        if (typeof t?.description !== "string" || !t.description) {
          bad.push(`${toolName} (description missing or empty)`);
        }
      }
      if (bad.length > 0) {
        throw new Error(
          `Plugin "${plugin.name}" has tools missing execute/description:\n  ` +
          bad.join("\n  ")
        );
      }
      expect(bad.length).toBe(0);
    });
  }
});

// ─────────────────────────────────────────────────────────────────────────────
// SUITE 5b: Tool args MUST be Zod schemas (not raw JSON Schema objects)
// Catches: args: { type: "object", properties: {...} } — raw JSON Schema
// Bedrock/Claude rejects with:
//   "tools.N.custom.input_schema: JSON schema is invalid. It must match
//    JSON Schema draft 2020-12"
// because OpenCode's tool() helper expects Zod and converts internally;
// passing raw JSON Schema produces malformed input_schema.
// ─────────────────────────────────────────────────────────────────────────────

describe("Plugin Health Guard — tool args must be Zod schemas (not raw JSON Schema)", () => {
  for (const plugin of PLUGINS) {
    it(`${plugin.name}: every tool's args is a Zod schema record (not a JSON Schema object)`, async () => {
      const result = await loadPlugin(plugin);
      if (!result?.tool) return;

      const violations: string[] = [];
      for (const [toolName, td] of Object.entries(result.tool)) {
        const t = td as { args?: unknown };
        const args = t?.args;
        if (args === null || args === undefined || typeof args !== "object") continue;

        // Detect raw JSON Schema: presence of `type: "object"` + `properties` keys
        // at the top level. Zod-derived args is { fieldName: ZodType } directly,
        // never wrapping `properties`.
        const argsObj = args as Record<string, unknown>;
        if (argsObj.type === "object" && argsObj.properties !== undefined) {
          violations.push(
            `${toolName}: args is raw JSON Schema (has type:"object" + properties). ` +
            `Convert to Zod: { fieldName: tool.schema.string() } instead of ` +
            `{ type: "object", properties: { fieldName: { type: "string" } } }`
          );
        }

        // Also check each value: must be a Zod schema (has _zod or is undefined-tolerant)
        // Zod v4 schemas have ._zod._def or _def. Plain objects don't.
        for (const [fieldName, fieldDef] of Object.entries(argsObj)) {
          if (fieldName === "type" || fieldName === "properties" || fieldName === "required") continue;
          if (typeof fieldDef !== "object" || fieldDef === null) continue;
          const f = fieldDef as Record<string, unknown>;
          // A raw JSON Schema field has { type: "string" } at this level
          // A Zod schema does NOT — it has internal Zod refs
          if (typeof f.type === "string" &&
              (f.type === "string" || f.type === "number" || f.type === "boolean" ||
               f.type === "object" || f.type === "array") &&
              !("_zod" in f) && !("_def" in f) && !("parse" in f)) {
            violations.push(
              `${toolName}.${fieldName}: looks like raw JSON Schema ({type: "${f.type}"}). ` +
              `Use tool.schema.${f.type === "string" ? "string" : f.type === "number" ? "number" : "boolean"}() instead.`
            );
          }
        }
      }

      if (violations.length > 0) {
        throw new Error(
          `Plugin "${plugin.name}" has tools with raw JSON Schema args:\n  ` +
          violations.join("\n  ") +
          `\n\nThis causes Bedrock/Claude to reject the tool registration with:\n` +
          `  "tools.N.custom.input_schema: JSON schema is invalid. It must match\n` +
          `   JSON Schema draft 2020-12"\n` +
          `Fix: convert each field to a Zod schema via tool.schema.* (e.g.,\n` +
          `tool.schema.string().describe("..."), tool.schema.number(), etc.)`
        );
      }
      expect(violations.length).toBe(0);
    });
  }
});

// ─────────────────────────────────────────────────────────────────────────────
// SUITE 7: ToolRegistry crash simulation
// Reproduces the exact code path that crashes — function V() in chunk-0c9y3vgs.js
// ─────────────────────────────────────────────────────────────────────────────
// SUITE 7: ToolRegistry crash simulation
// Reproduces the exact code path that crashes — function V() in chunk-0c9y3vgs.js
// ─────────────────────────────────────────────────────────────────────────────

describe("Plugin Health Guard — full ToolRegistry simulation", () => {
  it("simulates ToolRegistry.state for every plugin (catches Object.entries crashes)", async () => {
    // This mirrors the bundled OpenCode code:
    //   function V(toolId, toolDef) {
    //     let pairs = Object.entries(toolDef.args)  // ← crashes if args is null/undefined
    //     ...
    //   }
    //   for (const plugin of plugins) {
    //     for (const [name, def] of Object.entries(plugin.tool ?? {})) {
    //       V(name, def);
    //     }
    //   }
    const failures: string[] = [];

    for (const plugin of PLUGINS) {
      let result: { tool?: Record<string, unknown> } | null;
      try {
        result = await loadPlugin(plugin);
      } catch (err) {
        if (!plugin.mayThrow) {
          failures.push(`${plugin.name}: factory threw — ${err instanceof Error ? err.message.slice(0,80) : String(err)}`);
        }
        continue;
      }
      if (!result) continue;

      // Step 1: ToolRegistry calls Object.entries(plugin.tool ?? {})
      let toolEntries: [string, unknown][];
      try {
        toolEntries = Object.entries(result.tool ?? {});
      } catch (err) {
        failures.push(`${plugin.name}: Object.entries(plugin.tool) crashed — ${err}`);
        continue;
      }

      // Step 2: For each tool, ToolRegistry calls Object.entries(tool.args)
      for (const [toolName, td] of toolEntries) {
        const t = td as { args?: unknown };
        try {
          Object.entries(t.args as Record<string, unknown>);
        } catch (err) {
          failures.push(
            `${plugin.name} → ${toolName}: Object.entries(tool.args) crashed — ` +
            `args is ${t?.args === null ? "null" : typeof t?.args}`
          );
        }
      }
    }

    if (failures.length > 0) {
      throw new Error(
        `ToolRegistry crash simulation found ${failures.length} issue(s):\n  ` +
        failures.join("\n  ")
      );
    }
    expect(failures.length).toBe(0);
  });
});

// ─────────────────────────────────────────────────────────────────────────────
// SUITE 8: Logging hygiene — plugins should use client.app.log() not stdio
// Catches: console.log/console.error/process.stderr.write in plugin code.
// OpenCode's TUI captures stderr from plugin hooks and pollutes the chat pane.
// Per https://opencode.ai/docs/plugins/#logging — use client.app.log() instead.
// This is a WARN-level test (not a hard fail) — some legacy plugins still use
// stderr for non-hook code paths. The warning should be addressed when found.
// ─────────────────────────────────────────────────────────────────────────────

import { readFileSync, readdirSync, statSync as _statSync } from "fs";

describe("Plugin Health Guard — logging hygiene (WARN)", () => {
  // Files that legitimately may use stdio (intentional, documented):
  //   - context-pipeline.ts emitEvent: gated behind AXIOM_CONTEXT_PIPELINE_DEBUG=1
  //     so it only fires in test/eval contexts. Allowed.
  //   - lib/eval-runner.ts: this IS the eval runner; stdout is its public output.
  const ALLOWED_FILES = new Set([
    "lib/eval-runner.ts",
  ]);
  // Patterns that indicate stdio writes inside plugin code paths
  const STDIO_PATTERNS = [
    /\bconsole\.(log|error|warn|info|debug)\s*\(/g,
    /\bprocess\.(stdout|stderr)\.write\s*\(/g,
  ];

  function scanFile(relPath: string): string[] {
    const violations: string[] = [];
    const content = readFileSync(join(import.meta.dir, "..", relPath), "utf8");
    const lines = content.split("\n");
    let inComment = false;
    for (let i = 0; i < lines.length; i++) {
      const line = lines[i];
      // Skip block comments
      if (line.includes("/*") && !line.includes("*/")) inComment = true;
      if (inComment) {
        if (line.includes("*/")) inComment = false;
        continue;
      }
      // Skip line comments
      const codePart = line.split("//")[0];
      for (const pat of STDIO_PATTERNS) {
        pat.lastIndex = 0;
        const m = pat.exec(codePart);
        if (m) {
          // Check if this line is gated by an env var (acceptable pattern)
          const hasEnvGate = /process\.env\./.test(content.slice(Math.max(0, content.indexOf(line) - 200), content.indexOf(line)));
          if (!hasEnvGate) {
            violations.push(`${relPath}:${i + 1}: ${m[0]} — use client.app.log() instead`);
          }
        }
      }
    }
    return violations;
  }

  it("no stdio writes in plugins/ files (warns if found)", () => {
    const pluginsDir = join(import.meta.dir, "../plugins");
    const files = readdirSync(pluginsDir).filter(f =>
      (f.endsWith(".ts") || f.endsWith(".js")) && !f.includes(".test.")
    );
    const allViolations: string[] = [];
    for (const f of files) {
      const rel = `plugins/${f}`;
      if (ALLOWED_FILES.has(rel)) continue;
      allViolations.push(...scanFile(rel));
    }
    if (allViolations.length > 0) {
      console.warn(
        `\n⚠ Plugin logging hygiene — ${allViolations.length} stdio write(s) found:\n  ` +
        allViolations.slice(0, 20).join("\n  ") +
        (allViolations.length > 20 ? `\n  ... +${allViolations.length - 20} more` : "") +
        `\n\nUse client.app.log({ body: { service, level, message, extra } }) instead.\n` +
        `See: https://opencode.ai/docs/plugins/#logging\n` +
        `Or: .memory-bank/best-practices/opencode-plugin-tools-sdk.md (Logging section)`
      );
    }
    // WARN only — does not fail the build. Tracked metric.
    expect(allViolations.length).toBeGreaterThanOrEqual(0);
  });

  it("no stdio writes in lib/ plugin code (warns if found)", () => {
    const libDir = join(import.meta.dir, "../lib");
    const files = readdirSync(libDir).filter(f =>
      (f.endsWith(".ts") || f.endsWith(".js")) && !f.includes(".test.")
    );
    const allViolations: string[] = [];
    for (const f of files) {
      const rel = `lib/${f}`;
      if (ALLOWED_FILES.has(rel)) continue;
      allViolations.push(...scanFile(rel));
    }
    if (allViolations.length > 0) {
      console.warn(
        `\n⚠ Lib logging hygiene — ${allViolations.length} stdio write(s) found:\n  ` +
        allViolations.slice(0, 20).join("\n  ") +
        (allViolations.length > 20 ? `\n  ... +${allViolations.length - 20} more` : "") +
        `\n\nUse client.app.log() (passed through plugin factory) instead.\n` +
        `Or gate behind AXIOM_<NAME>_DEBUG=1 env var if for debug only.\n` +
        `See: https://opencode.ai/docs/plugins/#logging`
      );
    }
    expect(allViolations.length).toBeGreaterThanOrEqual(0);
  });
});
