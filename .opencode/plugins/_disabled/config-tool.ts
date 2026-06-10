/**
 * codeops_config MCP Tool Plugin — Phase 2 of Plugin Config Management.
 *
 * Provides runtime config inspection and mutation for Axiom OpenCode plugins
 * via the codeops_config tool (operations: get, set, show, describe, list, schema).
 *
 * Spec: specs/112-Plugin-Config-Management.md §5, §11.1, §11.2
 * ACs: AC-10a through AC-10f
 *
 * axiom:trace work_item=plugin-config-management-01 spec=specs/112-Plugin-Config-Management.md#5 plan=phase-2/task-2-1/step-2-1-1
 */

import { existsSync, readdirSync, readFileSync } from "node:fs";
import { join, resolve } from "node:path";
import { tool } from "@opencode-ai/plugin";
import {
  loadPluginConfig,
  writePluginConfig,
  getConfigInfo,
  coerceEnvValue,
  envPrefixForPlugin,
  pathToEnvSuffix,
  collectLeafPaths,
  getAtPath,
} from "../lib/config-utils.ts";

// ─────────────────────────────────────────────────────────────────────────────
// Types
// ─────────────────────────────────────────────────────────────────────────────

/**
 * Source layer enum (§5.1.1).
 * Indicates which config layer provided the effective value for a `get` operation.
 */
type SourceLayer = "defaults" | "file" | "local" | "env";

// ─────────────────────────────────────────────────────────────────────────────
// schemaToDefaults (§5.6)
// ─────────────────────────────────────────────────────────────────────────────

/**
 * Derive a defaults object from a JSON Schema v7 file.
 * Walks the schema's `properties` recursively and uses each property's `default`
 * field as the default value. Properties without a `default` field are omitted.
 * Arrays with `default` are included as-is. Nested objects are recursed.
 *
 * axiom:trace work_item=plugin-config-management-01 spec=specs/112-Plugin-Config-Management.md#5.6 plan=phase-2/task-2-1/step-2-1-2
 */
function schemaToDefaults(schema: Record<string, unknown>): Record<string, unknown> {
  const properties = schema["properties"];
  if (!properties || typeof properties !== "object" || Array.isArray(properties)) {
    return {};
  }

  const result: Record<string, unknown> = {};
  for (const [key, propSchema] of Object.entries(properties as Record<string, unknown>)) {
    if (!propSchema || typeof propSchema !== "object" || Array.isArray(propSchema)) {
      continue;
    }
    const prop = propSchema as Record<string, unknown>;

    // If a default value is set, use it — but recurse if default is {} and there are child properties.
    // axiom:trace work_item=plugin-config-management-01 spec=specs/112-Plugin-Config-Management.md#5.6 plan=phase-2/task-2-1/step-r17-03
    if ("default" in prop) {
      const defaultVal = prop["default"];
      // If default is {} but the property has nested children, recurse to get child defaults.
      // Returning {} directly would silently drop all child defaults.
      if (
        defaultVal !== null &&
        typeof defaultVal === "object" &&
        !Array.isArray(defaultVal) &&
        Object.keys(defaultVal as object).length === 0 &&
        (prop as Record<string, unknown>).properties
      ) {
        result[key] = schemaToDefaults(prop as Record<string, unknown>);
      } else {
        result[key] = defaultVal;
      }
      continue;
    }

    // If it's a nested object with properties, recurse
    if (prop["type"] === "object" && prop["properties"]) {
      const nested = schemaToDefaults(prop);
      if (Object.keys(nested).length > 0) {
        result[key] = nested;
      }
    }
    // Properties with no default and no children are omitted
  }

  return result;
}

// ─────────────────────────────────────────────────────────────────────────────
// Helpers
// ─────────────────────────────────────────────────────────────────────────────

/**
 * Validate plugin name — throws with the spec-mandated message shape on invalid names.
 * Error message matches §11.1: "Invalid plugin name '<name>': must match /^[a-zA-Z0-9_-]+$/"
 *
 * axiom:trace work_item=plugin-config-management-01 spec=specs/112-Plugin-Config-Management.md#11.1
 */
function validatePluginNameForTool(name: string): void {
  if (!/^[a-zA-Z0-9_-]+$/.test(name)) {
    throw new Error(
      `Invalid plugin name '${name}': must match /^[a-zA-Z0-9_-]+$/`,
    );
  }
}

/**
 * Load the JSON Schema file for a plugin.
 * Returns the parsed schema object or throws with the spec error message.
 *
 * axiom:trace work_item=plugin-config-management-01 spec=specs/112-Plugin-Config-Management.md#5.3
 */
function loadSchema(pluginName: string, repoRoot: string): Record<string, unknown> {
  const schemasDir = join(resolve(repoRoot), ".opencode", "config", "schemas");
  const schemaPath = join(schemasDir, `${pluginName}.schema.json`);
  if (!existsSync(schemaPath)) {
    throw new Error(
      `No schema file found for plugin '${pluginName}'. Expected: .opencode/config/schemas/${pluginName}.schema.json`,
    );
  }
  try {
    const raw = readFileSync(schemaPath, "utf-8");
    return JSON.parse(raw) as Record<string, unknown>;
  } catch (err) {
    throw new Error(
      `Failed to parse schema file for plugin '${pluginName}': ${err instanceof Error ? err.message : String(err)}`,
    );
  }
}

/**
 * Detect which source layer provided the effective value for a given key.
 *
 * Priority (highest-to-lowest): env → local → file → defaults
 *
 * axiom:trace work_item=plugin-config-management-01 spec=specs/112-Plugin-Config-Management.md#5.1.1
 */
function detectSourceLayer(
  pluginName: string,
  key: string,
  repoRoot: string,
  defaults: Record<string, unknown>,
): SourceLayer {
  const root = resolve(repoRoot);
  const configDir = join(root, ".opencode", "config");

  // 1. Check env var (highest priority)
  const envPrefix = envPrefixForPlugin(pluginName);
  const envKey = envPrefix + pathToEnvSuffix(key);
  if (process.env[envKey] !== undefined) {
    return "env";
  }

  // 2. Check local file
  const localPath = join(configDir, `${pluginName}.local.json`);
  if (existsSync(localPath)) {
    try {
      const raw = readFileSync(localPath, "utf-8");
      const parsed = JSON.parse(raw) as Record<string, unknown>;
      const val = getAtPath(parsed, key);
      if (val !== undefined) {
        return "local";
      }
    } catch {
      // Corrupt local file — ignore
    }
  }

  // 3. Check committed config file
  const configPath = join(configDir, `${pluginName}.json`);
  if (existsSync(configPath)) {
    try {
      const raw = readFileSync(configPath, "utf-8");
      const parsed = JSON.parse(raw) as Record<string, unknown>;
      const val = getAtPath(parsed, key);
      if (val !== undefined) {
        return "file";
      }
    } catch {
      // Corrupt config file — ignore
    }
  }

  // 4. Falls back to defaults
  return "defaults";
}

// ─────────────────────────────────────────────────────────────────────────────
// Plugin factory
// ─────────────────────────────────────────────────────────────────────────────

/**
 * ConfigToolPlugin — registers the codeops_config MCP tool.
 *
 * Receives `directory` from OpenCode's plugin loader. This is passed directly
 * as `repoRoot` to all config-utils functions (§5.4).
 *
 * axiom:trace work_item=plugin-config-management-01 spec=specs/112-Plugin-Config-Management.md#5.4 plan=phase-2/task-2-1/step-2-1-1
 */
export const ConfigToolPlugin = async ({
  directory,
  client,
}: {
  directory: string;
  client: unknown;
}) => {
  // `directory` is the repo root — pass as `repoRoot` to all config-utils functions (§5.4)
  const repoRoot = directory;

  const codeopsConfigTool = tool({
    description:
      "Inspect and mutate Axiom plugin configuration. " +
      "Operations: get (read a single key with source layer), set (write a value), " +
      "show (full effective config), describe (config metadata), list (all plugins), " +
      "schema (JSON Schema for a plugin). " +
      "Note: set writes to disk. The running plugin instance does not reload config — " +
      "restart OpenCode for changes to take effect on the live plugin.",
    args: {
      operation: tool.schema
        .enum(["get", "set", "show", "describe", "list", "schema"])
        .describe("Operation to perform"),
      plugin: tool.schema
        .string()
        .optional()
        .describe("Plugin name (e.g., 'graph-harness'). Required for get/set/show/describe/schema."),
      key: tool.schema
        .string()
        .optional()
        .describe("Dotted config path (e.g., 'harness.idle_evaluation_interval_ms'). Required for get/set."),
      value: tool.schema
        .string()
        .optional()
        .describe("Value to set (as string; type coercion is applied). Required for set."),
      local: tool.schema
        .boolean()
        .optional()
        .describe("If true, write to .local.json instead of .json. Applies to set only."),
    },

    async execute(args, _context) {
      // ── Error handler pattern (§5.7) ──────────────────────────────────────
      try {
        const { operation, plugin: pluginName, key, value, local } = args;

        // ── list ─────────────────────────────────────────────────────────────
        if (operation === "list") {
          // axiom:trace work_item=plugin-config-management-01 spec=specs/112-Plugin-Config-Management.md#5.3
          const schemasDir = join(resolve(repoRoot), ".opencode", "config", "schemas");
          if (!existsSync(schemasDir)) {
            return JSON.stringify({
              plugins: [],
              note: "No schemas directory found at .opencode/config/schemas/",
            });
          }

          const configDir = join(resolve(repoRoot), ".opencode", "config");
          let schemaFiles: string[];
          try {
            schemaFiles = readdirSync(schemasDir).filter((f) => f.endsWith(".schema.json"));
          } catch {
            return JSON.stringify({
              plugins: [],
              note: "No schemas directory found at .opencode/config/schemas/",
            });
          }

          const plugins = schemaFiles.map((filename) => {
            const name = filename.replace(/\.schema\.json$/, "");
            return {
              name,
              configExists: existsSync(join(configDir, `${name}.json`)),
              localExists: existsSync(join(configDir, `${name}.local.json`)),
            };
          });

          return JSON.stringify({ plugins });
        }

        // ── Operations that require a plugin name ──────────────────────────
        if (!pluginName) {
          throw new Error(`'plugin' argument is required for operation '${operation}'`);
        }

        // Validate plugin name (§11.1 — path traversal guard)
        validatePluginNameForTool(pluginName);

        // ── describe ─────────────────────────────────────────────────────────
        if (operation === "describe") {
          // axiom:trace work_item=plugin-config-management-01 spec=specs/112-Plugin-Config-Management.md#4.4
          const info = getConfigInfo(pluginName, repoRoot);
          return JSON.stringify(info);
        }

        // ── schema ───────────────────────────────────────────────────────────
        if (operation === "schema") {
          // axiom:trace work_item=plugin-config-management-01 spec=specs/112-Plugin-Config-Management.md#5.3
          const schema = loadSchema(pluginName, repoRoot);
          return JSON.stringify({ schema });
        }

        // ── show ─────────────────────────────────────────────────────────────
        if (operation === "show") {
          // axiom:trace work_item=plugin-config-management-01 spec=specs/112-Plugin-Config-Management.md#5.6
          const schema = loadSchema(pluginName, repoRoot);
          const defaults = schemaToDefaults(schema);
          const config = loadPluginConfig(pluginName, defaults, repoRoot);
          return JSON.stringify({ config });
        }

        // ── get ──────────────────────────────────────────────────────────────
        if (operation === "get") {
          if (!key) {
            throw new Error(`'key' argument is required for operation 'get'`);
          }

          // Validate plugin exists in schemas dir (best-effort; loadSchema throws if absent)
          let schema: Record<string, unknown>;
          try {
            schema = loadSchema(pluginName, repoRoot);
          } catch {
            // Plugin schema absent — return unknown plugin error (§11.1)
            throw new Error(
              `Unknown plugin: ${pluginName}. Use codeops_config list to see available plugins.`,
            );
          }

          // Derive defaults from schema
          const defaults = schemaToDefaults(schema);

          // Validate key is a known leaf path (§5.2.1)
          const leafPaths = collectLeafPaths(defaults);
          if (!leafPaths.includes(key)) {
            throw new Error(
              `Unknown key '${key}' for plugin '${pluginName}'. Use codeops_config show to see all keys.`,
            );
          }

          // Load effective config (all layers merged)
          const config = loadPluginConfig(pluginName, defaults, repoRoot);
          const effectiveValue = getAtPath(config as Record<string, unknown>, key);

          // Detect source layer
          const sourceLayer = detectSourceLayer(pluginName, key, repoRoot, defaults);

          // Check for legacy YAML config (graph-harness only) — §5.1.1 backward-compat note.
          // Only warn when sourceLayer === "defaults": if a JSON config or env var already overrides
          // the value, warning about YAML is misleading — those layers already take precedence.
          // axiom:trace work_item=plugin-config-management-01 spec=specs/112-Plugin-Config-Management.md#5.1.1 plan=phase-2/task-2-1/step-r17-01
          let yamlWarning: string | undefined;
          if (pluginName === "graph-harness" && sourceLayer === "defaults") {
            const yamlPath = join(resolve(repoRoot), ".graph-harness", "config.yaml");
            if (existsSync(yamlPath)) {
              yamlWarning =
                "DEPRECATED: .graph-harness/config.yaml detected. This value is coming from the YAML file (merged into defaults). Migrate to .opencode/config/graph-harness.json to use the three-layer config system.";
            }
          }

          // axiom:trace work_item=plugin-config-management-01 spec=specs/112-Plugin-Config-Management.md#11.2
          return JSON.stringify({
            value: effectiveValue,
            source_layer: sourceLayer,
            ...(yamlWarning ? { yaml_warning: yamlWarning } : {}),
          });
        }

        // ── set ──────────────────────────────────────────────────────────────
        if (operation === "set") {
          if (!key) {
            throw new Error(`'key' argument is required for operation 'set'`);
          }
          if (value === undefined || value === null) {
            throw new Error(`'value' argument is required for operation 'set'`);
          }

          // Load schema and defaults
          let schema: Record<string, unknown>;
          try {
            schema = loadSchema(pluginName, repoRoot);
          } catch {
            throw new Error(
              `Unknown plugin: ${pluginName}. Use codeops_config list to see available plugins.`,
            );
          }

          const defaults = schemaToDefaults(schema);

          // Validate key is a known leaf path (§5.2.1)
          const leafPaths = collectLeafPaths(defaults);
          if (!leafPaths.includes(key)) {
            throw new Error(
              `Unknown key '${key}' for plugin '${pluginName}'. Use codeops_config show to see all keys.`,
            );
          }

          // Get default value for type coercion
          const defaultValue = getAtPath(defaults, key) ?? "";

          // Coerce the string value to the appropriate type
          const coercedValue = coerceEnvValue(key, value, defaultValue);
          if (coercedValue === undefined) {
            // Coercion failed — produce spec-mandated error (§11.1)
            const typeName = Array.isArray(defaultValue)
              ? "array"
              : typeof defaultValue;
            throw new Error(
              `Cannot coerce '${value}' to type '${typeName}' for key '${key}'.`,
            );
          }

          // Build a patch object at the dotted path and write it
          const patch = buildPatch(key, coercedValue);

          // §5.5: always pass skipGitignore: true in MCP context
          writePluginConfig(pluginName, patch, repoRoot, {
            local: local ?? false,
            skipGitignore: true,
          });

          const sourceLayer: SourceLayer = (local ?? false) ? "local" : "file";

          // axiom:trace work_item=plugin-config-management-01 spec=specs/112-Plugin-Config-Management.md#11.2
          return JSON.stringify({
            status: "ok",
            key,
            value: coercedValue,
            source_layer: sourceLayer,
          });
        }

        // Unrecognized operation (should be caught by enum arg validation, but be safe)
        throw new Error(`Unknown operation: '${operation}'`);
      } catch (err) {
        // §5.7: wrap all errors in structured JSON
        return JSON.stringify({
          error: String(err instanceof Error ? err.message : err),
        });
      }
    },
  });

  return {
    tool: { codeops_config: codeopsConfigTool },
  };
};

// ─────────────────────────────────────────────────────────────────────────────
// Utility — build a nested patch object from a dotted key path
// ─────────────────────────────────────────────────────────────────────────────

/**
 * Build a nested patch object for a dotted key path.
 * e.g., buildPatch('limits.max_count', 42) → { limits: { max_count: 42 } }
 */
function buildPatch(key: string, value: unknown): Record<string, unknown> {
  const parts = key.split(".");
  let result: Record<string, unknown> = {};
  let current = result;
  for (let i = 0; i < parts.length - 1; i++) {
    const nested: Record<string, unknown> = {};
    current[parts[i]] = nested;
    current = nested;
  }
  current[parts[parts.length - 1]] = value;
  return result;
}
