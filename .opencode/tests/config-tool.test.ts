/**
 * Phase 2 TDD Gate — codeops_config MCP Tool.
 *
 * These tests cover AC-10a through AC-10f and the error contracts in §11.1.
 *
 * Governing spec: specs/112-Plugin-Config-Management.md §5, §11.1, §11.2
 * ACs: AC-10a through AC-10f
 *
 * Run: cd .opencode && bun test tests/config-tool.test.ts
 *
 * axiom:trace work_item=plugin-config-management-01 spec=specs/112-Plugin-Config-Management.md#AC-10a plan=phase-2/task-2-1/step-2-1-5
 */

import { test, expect, describe, beforeEach, afterEach } from "bun:test";
import {
  existsSync,
  mkdtempSync,
  rmSync,
  writeFileSync,
  mkdirSync,
  readFileSync,
} from "node:fs";
import { join } from "node:path";
import { tmpdir } from "node:os";
import { ConfigToolPlugin } from "../plugins/config-tool.ts";
import { writePluginConfig } from "../lib/config-utils.ts";

// ─── Helpers ──────────────────────────────────────────────────────────────────

function makeTmpDir(): string {
  return mkdtempSync(join(tmpdir(), "config-tool-test-"));
}

/** Write a minimal test schema into <repoRoot>/.opencode/config/schemas/<plugin>.schema.json */
function writeTestSchema(repoRoot: string, pluginName: string): void {
  const schemasDir = join(repoRoot, ".opencode", "config", "schemas");
  mkdirSync(schemasDir, { recursive: true });
  writeFileSync(
    join(schemasDir, `${pluginName}.schema.json`),
    JSON.stringify({
      $schema: "http://json-schema.org/draft-07/schema#",
      $id: `${pluginName}.schema.json`,
      title: "Test Plugin Config",
      type: "object",
      properties: {
        enabled: { type: "boolean", default: true },
        label: { type: "string", default: "default-label" },
        limits: {
          type: "object",
          properties: {
            max_count: { type: "integer", default: 5 },
          },
        },
      },
    }),
  );
}

/** Write a committed config file (.json) with the given contents */
function writeCommittedConfig(
  repoRoot: string,
  pluginName: string,
  data: Record<string, unknown>,
): void {
  const configDir = join(repoRoot, ".opencode", "config");
  mkdirSync(configDir, { recursive: true });
  writeFileSync(join(configDir, `${pluginName}.json`), JSON.stringify(data));
}

/** Write a local config file (.local.json) with the given contents */
function writeLocalConfig(
  repoRoot: string,
  pluginName: string,
  data: Record<string, unknown>,
): void {
  const configDir = join(repoRoot, ".opencode", "config");
  mkdirSync(configDir, { recursive: true });
  writeFileSync(
    join(configDir, `${pluginName}.local.json`),
    JSON.stringify(data),
  );
}

/** Call the codeops_config tool and parse the JSON result */
async function callTool(
  plugin: Awaited<ReturnType<typeof ConfigToolPlugin>>,
  args: Record<string, unknown>,
): Promise<Record<string, unknown>> {
  const result = await plugin.tool["codeops_config"].execute(
    args as Parameters<typeof plugin.tool["codeops_config"]["execute"]>[0],
    { sessionID: "test" } as Parameters<typeof plugin.tool["codeops_config"]["execute"]>[1],
  );
  return JSON.parse(result as string) as Record<string, unknown>;
}

/**
 * Save and restore env vars around a test callback.
 * Any keys set inside the callback are cleaned up after.
 */
async function withEnv(
  vars: Record<string, string>,
  fn: () => Promise<void>,
): Promise<void> {
  const saved: Record<string, string | undefined> = {};
  for (const [k, v] of Object.entries(vars)) {
    saved[k] = process.env[k];
    process.env[k] = v;
  }
  try {
    await fn();
  } finally {
    for (const [k, orig] of Object.entries(saved)) {
      if (orig === undefined) {
        delete process.env[k];
      } else {
        process.env[k] = orig;
      }
    }
  }
}

// ─── AC-10a: get ─────────────────────────────────────────────────────────────

describe("AC-10a: codeops_config get", () => {
  let tmpDir: string;
  let plugin: Awaited<ReturnType<typeof ConfigToolPlugin>>;

  beforeEach(async () => {
    tmpDir = makeTmpDir();
    writeTestSchema(tmpDir, "testplugin");
    plugin = await ConfigToolPlugin({ directory: tmpDir, client: {} });
  });

  afterEach(() => {
    rmSync(tmpDir, { recursive: true, force: true });
  });

  test("AC-10a: get returns { value, source_layer: 'defaults' } for a known key with no config file", async () => {
    const result = await callTool(plugin, {
      operation: "get",
      plugin: "testplugin",
      key: "enabled",
    });
    expect(result.value).toBe(true);
    expect(result.source_layer).toBe("defaults");
  });

  test("AC-10a: get returns { value, source_layer: 'file' } when key is overridden in committed config", async () => {
    writeCommittedConfig(tmpDir, "testplugin", { label: "from-file" });
    const result = await callTool(plugin, {
      operation: "get",
      plugin: "testplugin",
      key: "label",
    });
    expect(result.value).toBe("from-file");
    expect(result.source_layer).toBe("file");
  });

  test("AC-10a: get returns { value, source_layer: 'env' } when key is overridden by env var", async () => {
    await withEnv({ AXIOM_TESTPLUGIN_LABEL: "from-env" }, async () => {
      const result = await callTool(plugin, {
        operation: "get",
        plugin: "testplugin",
        key: "label",
      });
      expect(result.value).toBe("from-env");
      expect(result.source_layer).toBe("env");
    });
  });

  test("AC-10a: get returns { error: '...' } for unknown key (key not in schema defaults)", async () => {
    const result = await callTool(plugin, {
      operation: "get",
      plugin: "testplugin",
      key: "nonexistent.key",
    });
    expect(typeof result.error).toBe("string");
    expect(result.error as string).toContain("Unknown key");
    expect(result.error as string).toContain("nonexistent.key");
  });

  test("AC-10a: get returns { error: '...' } for unknown plugin name", async () => {
    // No schema file for "noplugin" in tmpDir
    const result = await callTool(plugin, {
      operation: "get",
      plugin: "noplugin",
      key: "enabled",
    });
    expect(typeof result.error).toBe("string");
    expect(result.error as string).toContain("Unknown plugin");
  });
});

// ─── AC-10b: set ─────────────────────────────────────────────────────────────

describe("AC-10b: codeops_config set", () => {
  let tmpDir: string;
  let plugin: Awaited<ReturnType<typeof ConfigToolPlugin>>;

  beforeEach(async () => {
    tmpDir = makeTmpDir();
    writeTestSchema(tmpDir, "testplugin");
    plugin = await ConfigToolPlugin({ directory: tmpDir, client: {} });
  });

  afterEach(() => {
    rmSync(tmpDir, { recursive: true, force: true });
  });

  test("AC-10b: set writes value to committed config file; get reads it back with source_layer 'file'", async () => {
    // Set the label
    await callTool(plugin, {
      operation: "set",
      plugin: "testplugin",
      key: "label",
      value: "written-by-set",
    });

    // Get it back
    const result = await callTool(plugin, {
      operation: "get",
      plugin: "testplugin",
      key: "label",
    });
    expect(result.value).toBe("written-by-set");
    expect(result.source_layer).toBe("file");
  });

  test("AC-10b: set with local=true writes to .local.json; get reads it back with source_layer 'local'", async () => {
    // Set with local=true
    await callTool(plugin, {
      operation: "set",
      plugin: "testplugin",
      key: "label",
      value: "local-value",
      local: true,
    });

    const localPath = join(tmpDir, ".opencode", "config", "testplugin.local.json");
    expect(existsSync(localPath)).toBe(true);

    const result = await callTool(plugin, {
      operation: "get",
      plugin: "testplugin",
      key: "label",
    });
    expect(result.value).toBe("local-value");
    expect(result.source_layer).toBe("local");
  });

  test("AC-10b: set success returns { status: 'ok', key, value, source_layer }", async () => {
    const result = await callTool(plugin, {
      operation: "set",
      plugin: "testplugin",
      key: "label",
      value: "new-label",
    });
    expect(result.status).toBe("ok");
    expect(result.key).toBe("label");
    expect(result.value).toBe("new-label");
    expect(result.source_layer).toBe("file");
  });

  test("AC-10b: set type coercion — '42' for a number field writes 42 (number) not '42' (string)", async () => {
    const result = await callTool(plugin, {
      operation: "set",
      plugin: "testplugin",
      key: "limits.max_count",
      value: "42",
    });
    expect(result.status).toBe("ok");
    expect(result.value).toBe(42);
    expect(typeof result.value).toBe("number");

    // Verify it reads back as a number
    const getResult = await callTool(plugin, {
      operation: "get",
      plugin: "testplugin",
      key: "limits.max_count",
    });
    expect(getResult.value).toBe(42);
    expect(typeof getResult.value).toBe("number");
  });

  test("AC-10b: set returns { error: '...' } for type coercion failure", async () => {
    // "notanumber" cannot be coerced to number (default is 5 for max_count)
    const result = await callTool(plugin, {
      operation: "set",
      plugin: "testplugin",
      key: "limits.max_count",
      value: "notanumber",
    });
    expect(typeof result.error).toBe("string");
    expect(result.error as string).toContain("Cannot coerce");
  });
});

// ─── AC-10c: show ─────────────────────────────────────────────────────────────

describe("AC-10c: codeops_config show", () => {
  let tmpDir: string;
  let plugin: Awaited<ReturnType<typeof ConfigToolPlugin>>;

  beforeEach(async () => {
    tmpDir = makeTmpDir();
    writeTestSchema(tmpDir, "testplugin");
    plugin = await ConfigToolPlugin({ directory: tmpDir, client: {} });
  });

  afterEach(() => {
    rmSync(tmpDir, { recursive: true, force: true });
  });

  test("AC-10c: show returns { config: <object> } with schema-derived defaults merged with file layer", async () => {
    // Write a partial override in the committed config file
    writeCommittedConfig(tmpDir, "testplugin", { label: "overridden" });

    const result = await callTool(plugin, {
      operation: "show",
      plugin: "testplugin",
    });

    expect(typeof result.config).toBe("object");
    const config = result.config as Record<string, unknown>;

    // Schema defaults
    expect(config.enabled).toBe(true);

    // File override
    expect(config.label).toBe("overridden");

    // Nested schema default
    const limits = config.limits as Record<string, unknown>;
    expect(limits).toBeDefined();
    expect(limits.max_count).toBe(5);
  });

  test("AC-10c / step-r17-03: group with 'default: {}' still populates child defaults", async () => {
    // Write a schema where 'limits' has default: {} but has child properties with defaults.
    // The bug: schemaToDefaults short-circuits on "default" key and returns {} for limits,
    // dropping max_connections and timeout_ms. The fix: recurse when default is empty {}.
    // axiom:trace work_item=plugin-config-management-01 spec=specs/112-Plugin-Config-Management.md#5.6 plan=phase-2/task-2-1/step-r17-03
    const schemasDir = join(tmpDir, ".opencode", "config", "schemas");
    mkdirSync(schemasDir, { recursive: true });
    writeFileSync(
      join(schemasDir, "testplugin2.schema.json"),
      JSON.stringify({
        $schema: "http://json-schema.org/draft-07/schema#",
        type: "object",
        properties: {
          limits: {
            type: "object",
            default: {},
            properties: {
              max_connections: { type: "integer", default: 10 },
              timeout_ms: { type: "integer", default: 5000 },
            },
          },
        },
      }),
    );

    const plugin2 = await ConfigToolPlugin({ directory: tmpDir, client: {} });
    const result = await callTool(plugin2, {
      operation: "show",
      plugin: "testplugin2",
    });

    expect(result.config).toBeDefined();
    const config = result.config as Record<string, unknown>;
    const limits = config.limits as Record<string, unknown>;

    // Must NOT be an empty object — child defaults must be present
    expect(limits).toBeDefined();
    expect(limits.max_connections).toBe(10);
    expect(limits.timeout_ms).toBe(5000);
    // Confirm it is not the empty-object bug
    expect(Object.keys(limits).length).toBeGreaterThan(0);
  });

  test("AC-10c: show includes explicitly-written keys even if absent from schema (file layer included)", async () => {
    // Write a config file with an extra key NOT in the schema
    writeCommittedConfig(tmpDir, "testplugin", {
      label: "hello",
      extra_undocumented_key: "from-file",
    });

    const result = await callTool(plugin, {
      operation: "show",
      plugin: "testplugin",
    });

    const config = result.config as Record<string, unknown>;

    // Schema default present
    expect(config.enabled).toBe(true);
    // File override present
    expect(config.label).toBe("hello");
    // File-written key IS included: loadPluginConfig deep-merges the file layer over
    // schema defaults, so keys absent from the schema but present in the config file
    // WILL appear in the show output (§5.6 Known Limitation clarification).
    // axiom:trace work_item=plugin-config-management-01 spec=specs/112-Plugin-Config-Management.md#5.6 plan=phase-2/task-2-1/polish test=tests/config-tool.test.ts#AC-10c
    expect((result.config as Record<string, unknown>).extra_undocumented_key).toBe("from-file");
  });
});

// ─── AC-10d: describe ─────────────────────────────────────────────────────────

describe("AC-10d: codeops_config describe", () => {
  let tmpDir: string;
  let plugin: Awaited<ReturnType<typeof ConfigToolPlugin>>;

  beforeEach(async () => {
    tmpDir = makeTmpDir();
    writeTestSchema(tmpDir, "testplugin");
    plugin = await ConfigToolPlugin({ directory: tmpDir, client: {} });
  });

  afterEach(() => {
    rmSync(tmpDir, { recursive: true, force: true });
  });

  test("AC-10d: describe returns ConfigInfo shape { pluginName, configPath, localPath, configExists, localExists, envPrefix, activeEnvVars }", async () => {
    const result = await callTool(plugin, {
      operation: "describe",
      plugin: "testplugin",
    });

    // Verify all required ConfigInfo fields
    expect(result.pluginName).toBe("testplugin");
    expect(typeof result.configPath).toBe("string");
    expect(result.configPath as string).toContain("testplugin.json");
    expect(typeof result.localPath).toBe("string");
    expect(result.localPath as string).toContain("testplugin.local.json");
    expect(typeof result.configExists).toBe("boolean");
    expect(typeof result.localExists).toBe("boolean");
    expect(result.envPrefix).toBe("AXIOM_TESTPLUGIN_");
    expect(Array.isArray(result.activeEnvVars)).toBe(true);

    // No config files exist yet
    expect(result.configExists).toBe(false);
    expect(result.localExists).toBe(false);
  });
});

// ─── AC-10e: list ─────────────────────────────────────────────────────────────

describe("AC-10e: codeops_config list", () => {
  let tmpDir: string;
  let plugin: Awaited<ReturnType<typeof ConfigToolPlugin>>;

  beforeEach(async () => {
    tmpDir = makeTmpDir();
    plugin = await ConfigToolPlugin({ directory: tmpDir, client: {} });
  });

  afterEach(() => {
    rmSync(tmpDir, { recursive: true, force: true });
  });

  test("AC-10e: list returns { plugins: [{ name, configExists, localExists }, ...] } — one entry per schema file", async () => {
    // Create two schema files
    writeTestSchema(tmpDir, "plugin-alpha");
    writeTestSchema(tmpDir, "plugin-beta");

    // Create a committed config for plugin-alpha
    writeCommittedConfig(tmpDir, "plugin-alpha", { enabled: false });

    const result = await callTool(plugin, { operation: "list" });
    expect(Array.isArray(result.plugins)).toBe(true);

    const plugins = result.plugins as Array<{
      name: string;
      configExists: boolean;
      localExists: boolean;
    }>;
    expect(plugins).toHaveLength(2);

    const alpha = plugins.find((p) => p.name === "plugin-alpha");
    const beta = plugins.find((p) => p.name === "plugin-beta");

    expect(alpha).toBeDefined();
    expect(alpha!.configExists).toBe(true);
    expect(alpha!.localExists).toBe(false);

    expect(beta).toBeDefined();
    expect(beta!.configExists).toBe(false);
    expect(beta!.localExists).toBe(false);
  });

  test("AC-10e: list returns { plugins: [], note: '...' } when schemas directory does not exist", async () => {
    // No schemas directory created
    const result = await callTool(plugin, { operation: "list" });
    expect(Array.isArray(result.plugins)).toBe(true);
    expect((result.plugins as unknown[]).length).toBe(0);
    expect(typeof result.note).toBe("string");
    expect(result.note as string).toContain("No schemas directory found");
  });

  test("AC-10e: list does NOT filter by Phase 4 adoption status (schema-only discovery)", async () => {
    // Create schemas for two "theoretical" plugins that haven't adopted loadPluginConfig yet
    writeTestSchema(tmpDir, "legacy-plugin");
    writeTestSchema(tmpDir, "future-plugin");

    const result = await callTool(plugin, { operation: "list" });
    const plugins = result.plugins as Array<{ name: string }>;
    expect(plugins).toHaveLength(2);

    const names = plugins.map((p) => p.name);
    expect(names).toContain("legacy-plugin");
    expect(names).toContain("future-plugin");
  });

  test("AC-10e: configExists is false and localExists is true when only .local.json exists", async () => {
    // Only write a .local.json — no committed .json
    writePluginConfig("testplugin", { label: "local-only" }, tmpDir, { local: true, skipGitignore: true });
    writeTestSchema(tmpDir, "testplugin");

    const result = await callTool(plugin, { operation: "list" });
    expect(result.plugins).toBeDefined();
    const entry = (result.plugins as Array<{ name: string; configExists: boolean; localExists: boolean }>)
      .find((p) => p.name === "testplugin");
    expect(entry).toBeDefined();
    expect(entry!.configExists).toBe(false);   // .json doesn't exist
    expect(entry!.localExists).toBe(true);     // .local.json DOES exist

    // Verify show still returns the local value despite configExists=false
    const showResult = await callTool(plugin, { operation: "show", plugin: "testplugin" });
    expect(showResult.config).toBeDefined();
    expect((showResult.config as Record<string, unknown>).label).toBe("local-only");
  });
});

// ─── AC-10f: schema ─────────────────────────────────────────────────────────

describe("AC-10f: codeops_config schema", () => {
  let tmpDir: string;
  let plugin: Awaited<ReturnType<typeof ConfigToolPlugin>>;

  beforeEach(async () => {
    tmpDir = makeTmpDir();
    plugin = await ConfigToolPlugin({ directory: tmpDir, client: {} });
  });

  afterEach(() => {
    rmSync(tmpDir, { recursive: true, force: true });
  });

  test("AC-10f: schema returns { schema: <parsed-object> } for a known plugin", async () => {
    writeTestSchema(tmpDir, "testplugin");

    const result = await callTool(plugin, {
      operation: "schema",
      plugin: "testplugin",
    });

    expect(typeof result.schema).toBe("object");
    const schema = result.schema as Record<string, unknown>;
    expect(schema.$schema).toBeDefined();
    expect(schema.properties).toBeDefined();
  });

  test("AC-10f: schema returns { error: '...' } when schema file is absent", async () => {
    // No schema file for "noplugin"
    const result = await callTool(plugin, {
      operation: "schema",
      plugin: "noplugin",
    });
    expect(typeof result.error).toBe("string");
    expect(result.error as string).toContain("No schema file found");
    expect(result.error as string).toContain("noplugin");
  });
});

// ─── Error contracts (§11.1) ─────────────────────────────────────────────────

describe("Error contracts (§11.1)", () => {
  let tmpDir: string;
  let plugin: Awaited<ReturnType<typeof ConfigToolPlugin>>;

  beforeEach(async () => {
    tmpDir = makeTmpDir();
    writeTestSchema(tmpDir, "testplugin");
    plugin = await ConfigToolPlugin({ directory: tmpDir, client: {} });
  });

  afterEach(() => {
    rmSync(tmpDir, { recursive: true, force: true });
  });

  test("Invalid plugin name (path traversal) returns { error: 'Invalid plugin name...' }", async () => {
    const result = await callTool(plugin, {
      operation: "get",
      plugin: "../../../etc/passwd",
      key: "enabled",
    });
    expect(typeof result.error).toBe("string");
    expect(result.error as string).toContain("Invalid plugin name");
  });

  test("skipGitignore MCP default: set with local=true does NOT modify .gitignore by default (§5.5)", async () => {
    // Perform a set with local=true
    await callTool(plugin, {
      operation: "set",
      plugin: "testplugin",
      key: "label",
      value: "local-test",
      local: true,
    });

    // The .gitignore should NOT exist (skipGitignore: true is the default in MCP context)
    const gitignorePath = join(tmpDir, ".gitignore");
    expect(existsSync(gitignorePath)).toBe(false);
  });
});

// ─── Real-schema regression tests (Phase 3 completeness guard) ───────────────
// axiom:trace work_item=plugin-config-management-01 spec=specs/112-Plugin-Config-Management.md#AC-10c plan=phase-3/polish test=tests/config-tool.test.ts

describe("AC-10c / AC-10a regression: graph-harness schema completeness", () => {
  let tmpDir: string;
  let plugin: Awaited<ReturnType<typeof ConfigToolPlugin>>;

  beforeEach(async () => {
    tmpDir = makeTmpDir();
    // Copy the real graph-harness schema into the tmpDir schemas directory
    const schemasDir = join(tmpDir, ".opencode", "config", "schemas");
    mkdirSync(schemasDir, { recursive: true });
    const realSchemaPath = join(
      import.meta.dir,  // tests/ directory
      "..",             // .opencode/
      "config",
      "schemas",
      "graph-harness.schema.json",
    );
    writeFileSync(
      join(schemasDir, "graph-harness.schema.json"),
      readFileSync(realSchemaPath, "utf-8"),
    );
    plugin = await ConfigToolPlugin({ directory: tmpDir, client: {} });
  });

  afterEach(() => {
    rmSync(tmpDir, { recursive: true, force: true });
  });

  // step-qa-schema-01
  test("AC-10c: show graph-harness includes command_policy from real schema", async () => {
    const result = await callTool(plugin, {
      operation: "show",
      plugin: "graph-harness",
    });
    expect(result.config).toBeDefined();
    const cfg = result.config as Record<string, unknown>;
    // command_policy group must be present (was missing in 8-group stub)
    expect(cfg.command_policy).toBeDefined();
    expect((cfg.command_policy as Record<string, unknown>).mode).toBe("permissive");
    // schedule_defaults group must be present
    expect(cfg.schedule_defaults).toBeDefined();
    expect((cfg.schedule_defaults as Record<string, unknown>).max_repeat_count).toBe(100);
    // templates group must be present
    expect(cfg.templates).toBeDefined();
    // api_policy group must be present
    expect(cfg.api_policy).toBeDefined();
    // lifecycle group must be present
    expect(cfg.lifecycle).toBeDefined();
    expect((cfg.lifecycle as Record<string, unknown>).archive_after_days).toBe(7);
  });

  // step-qa-schema-02
  test("AC-10a: get graph-harness command_policy.mode returns defaults", async () => {
    const result = await callTool(plugin, {
      operation: "get",
      plugin: "graph-harness",
      key: "command_policy.mode",
    });
    expect(result.value).toBe("permissive");
    expect(result.source_layer).toBe("defaults");
  });

  test("AC-10a: get graph-harness command_policy.blocklist returns empty array default", async () => {
    const result = await callTool(plugin, {
      operation: "get",
      plugin: "graph-harness",
      key: "command_policy.blocklist",
    });
    expect(Array.isArray(result.value)).toBe(true);
    expect((result.value as unknown[]).length).toBe(0);
    expect(result.source_layer).toBe("defaults");
  });

  test("AC-10a: get graph-harness lifecycle.archive_after_days returns 7 (default)", async () => {
    const result = await callTool(plugin, {
      operation: "get",
      plugin: "graph-harness",
      key: "lifecycle.archive_after_days",
    });
    expect(result.value).toBe(7);
    expect(result.source_layer).toBe("defaults");
  });

  // step-r16-01: yaml_warning surfaced when .graph-harness/config.yaml exists
  // step-r17-01: yaml_warning gated on sourceLayer === "defaults" to avoid misleading warnings
  // axiom:trace work_item=plugin-config-management-01 spec=specs/112-Plugin-Config-Management.md#5.1.1 plan=phase-2/task-2-1/step-r17-01
  test("AC-10a: get for graph-harness with legacy config.yaml includes yaml_warning when source_layer is defaults", async () => {
    // Create .graph-harness/config.yaml in the tmpDir to simulate legacy YAML config.
    // No .opencode/config/graph-harness.json is present, so source_layer will be "defaults".
    const yamlDir = join(tmpDir, ".graph-harness");
    mkdirSync(yamlDir, { recursive: true });
    writeFileSync(
      join(yamlDir, "config.yaml"),
      "graph_harness:\n  harness:\n    idle_evaluation_interval_ms: 77777\n",
    );

    const result = await callTool(plugin, {
      operation: "get",
      plugin: "graph-harness",
      key: "harness.idle_evaluation_interval_ms",
    });

    // source_layer must be "defaults" (precondition: no JSON config overriding this key)
    expect(result.source_layer).toBe("defaults");

    // yaml_warning must be present and reference config.yaml
    expect(result).toHaveProperty("yaml_warning");
    expect(typeof result.yaml_warning).toBe("string");
    expect(result.yaml_warning as string).toContain("config.yaml");

    // value and source_layer still present
    expect(result).toHaveProperty("value");
    expect(result).toHaveProperty("source_layer");

    // NEW-2: value must be the schema-defaults value (30000), not the YAML value (77777).
    // The YAML file is detected (triggering yaml_warning) but is NOT loaded — the tool only
    // reads JSON config layers.  Confirming value=30000 proves the defaults path is active.
    // axiom:trace work_item=plugin-config-management-01 spec=specs/112-Plugin-Config-Management.md#5.1.1 plan=phase-final/step-final-04
    expect(result.value).toBe(30000);
    expect(result.source_layer).toBe("defaults");
  });

  test("AC-10a: get for graph-harness WITHOUT config.yaml does NOT include yaml_warning", async () => {
    // No .graph-harness/config.yaml in tmpDir
    const result = await callTool(plugin, {
      operation: "get",
      plugin: "graph-harness",
      key: "harness.idle_evaluation_interval_ms",
    });

    // yaml_warning must NOT be present when config.yaml is absent
    expect(result).not.toHaveProperty("yaml_warning");
    expect(result).toHaveProperty("value");
    expect(result).toHaveProperty("source_layer");
  });

  // step-r17-01: yaml_warning must NOT fire when source_layer is "file" or "env"
  // axiom:trace work_item=plugin-config-management-01 spec=specs/112-Plugin-Config-Management.md#5.1.1 plan=phase-2/task-2-1/step-r17-01
  test("AC-10a: get for graph-harness does NOT include yaml_warning when source_layer is file (JSON config overrides value)", async () => {
    // Create .graph-harness/config.yaml — it exists, but a JSON config also sets the key,
    // so source_layer will be "file". The yaml_warning must NOT appear.
    const yamlDir = join(tmpDir, ".graph-harness");
    mkdirSync(yamlDir, { recursive: true });
    writeFileSync(
      join(yamlDir, "config.yaml"),
      "graph_harness:\n  harness:\n    idle_evaluation_interval_ms: 77777\n",
    );

    // Write a committed JSON config overriding the same key
    writeCommittedConfig(tmpDir, "graph-harness", {
      harness: { idle_evaluation_interval_ms: 55555 },
    });

    const result = await callTool(plugin, {
      operation: "get",
      plugin: "graph-harness",
      key: "harness.idle_evaluation_interval_ms",
    });

    // source_layer must be "file" (JSON config takes precedence over YAML/defaults)
    expect(result.source_layer).toBe("file");

    // yaml_warning must NOT be present — the JSON layer already overrides the YAML
    expect(result).not.toHaveProperty("yaml_warning");

    // value must come from the JSON config, not the YAML
    expect(result.value).toBe(55555);
  });

  // R-5: schemaToDefaults structural spot-check
  // axiom:trace work_item=plugin-config-management-01 spec=specs/112-Plugin-Config-Management.md#5.6 plan=phase-5-complete/backlog
  test("R-5: schemaToDefaults produces non-empty defaults object for real graph-harness schema", async () => {
    // R-5: Structural check that schemaToDefaults can read the real schema and produce defaults.
    // A full deep-equal against DEFAULT_CONFIG would be brittle (schema is subset of DEFAULT_CONFIG).
    // This confirms: (1) schema file is valid JSON, (2) show returns an object, (3) it has keys.
    const result = await callTool(plugin, {
      operation: "show",
      plugin: "graph-harness",
    });
    // show must return { config: <object> }
    expect(typeof result).toBe("object");
    expect(result).not.toBeNull();
    expect(result.config).toBeDefined();
    const cfg = result.config as Record<string, unknown>;
    // Should have at least one group key (harness, lifecycle, command_policy, etc.)
    expect(Object.keys(cfg).length).toBeGreaterThan(0);
    // harness group must be present with defaults
    expect(cfg.harness).toBeDefined();
    expect(typeof (cfg.harness as Record<string, unknown>).idle_evaluation_interval_ms).toBe("number");
  });
});
