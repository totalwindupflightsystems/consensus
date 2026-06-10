/**
 * Plugin Config Utilities — three-layer config loader for Axiom OpenCode plugins.
 *
 * Layer order (later layers override earlier):
 *   1. Plugin defaults (typed TypeScript object, compiled into plugin)
 *   2. .opencode/config/<plugin>.json       (committed, shared, optional)
 *   3. .opencode/config/<plugin>.local.json  (gitignored, per-user, optional)
 *   4. Environment variables                 (AXIOM_<PLUGIN>_<PATH> format)
 *
 * Security (adversarial-review.md findings applied):
 *   SEC-1: pluginName validated against /^[a-zA-Z0-9_-]+$/ — path traversal prevention
 *   ASM-1: repoRoot is path.resolve()'d to normalize relative and trailing-slash paths
 *   ASM-3: configDir created with mkdirSync({recursive:true}) before any write
 *
 * All writes use atomic write (temp file → rename) to prevent partial-write corruption.
 * .local.json files are created with mode 0600 (user-only read/write).
 *
 * axiom:trace work_item=plugin-config-management-01 spec=specs/112-Plugin-Config-Management.md plan=phase-1/task-1-1/step-1-1-1
 */

import {
  appendFileSync,
  existsSync,
  mkdirSync,
  readFileSync,
  renameSync,
  unlinkSync,
  writeFileSync,
  chmodSync,
} from "node:fs";
import { join, resolve } from "node:path";

// ─────────────────────────────────────────────────────────────────────────────
// Types
// ─────────────────────────────────────────────────────────────────────────────

export interface LoadPluginConfigOptions {
  /** Config directory. Default: "<repoRoot>/.opencode/config" */
  configDir?: string;
  /** Env var prefix override. Default: "AXIOM_<PLUGIN_UPPER_SNAKE>_" */
  envPrefix?: string;
  /** Whether to load .local.json. Default: true */
  allowLocal?: boolean;
}

export interface WritePluginConfigOptions {
  /** Config directory. Default: "<repoRoot>/.opencode/config" */
  configDir?: string;
  /** If true, writes to .local.json (mode 0600). Default: false */
  local?: boolean;
  /**
   * If true, skips the automatic .gitignore update even when `local` is true.
   * Use in CI environments, test harnesses, or non-git directories.
   * Default: false (gitignore is updated by default per REQ-PCM-001).
   */
  skipGitignore?: boolean;
}

export interface ConfigInfo {
  pluginName: string;
  configPath: string;
  localPath: string;
  configExists: boolean;
  localExists: boolean;
  envPrefix: string;
  activeEnvVars: string[];
}

// ─────────────────────────────────────────────────────────────────────────────
// Validation
// ─────────────────────────────────────────────────────────────────────────────

/**
 * Validate a plugin name to prevent path traversal attacks (SEC-1).
 * Only alphanumeric, hyphens, and underscores are allowed.
 * Throws with a clear error message if invalid.
 */
function validatePluginName(name: string): void {
  if (!/^[a-zA-Z0-9_-]+$/.test(name)) {
    throw new Error(
      `[config-utils] Invalid pluginName "${name}" — must match /^[a-zA-Z0-9_-]+$/ ` +
      `(alphanumeric, hyphens, underscores only). Path traversal attempt?`,
    );
  }
}

// ─────────────────────────────────────────────────────────────────────────────
// Deep merge
// ─────────────────────────────────────────────────────────────────────────────

/**
 * Deep-merge `override` into `base`, returning a new object. Never mutates inputs.
 *
 * Rules:
 * - Merging is recursive for plain objects.
 * - Arrays are REPLACED (not merged).
 * - A null value in override sets the field to null.
 * - An undefined value in override is ignored (lower-layer value preserved).
 *
 * axiom:trace work_item=plugin-config-management-01 spec=specs/112-Plugin-Config-Management.md#3.4
 */
export function deepMerge<T extends Record<string, unknown>>(
  base: T,
  override: Partial<T>,
): T {
  // Clone base to avoid mutation (ASM-mutation safety)
  const result: Record<string, unknown> = { ...base };

  for (const key of Object.keys(override) as Array<keyof T>) {
    const overrideVal = override[key];
    const baseVal = base[key];

    // Undefined override → skip (preserve base)
    if (overrideVal === undefined) {
      continue;
    }

    // Null override → set to null
    if (overrideVal === null) {
      result[key as string] = null;
      continue;
    }

    // Both are plain (non-array, non-null) objects → recurse
    if (
      isPlainObject(overrideVal) &&
      isPlainObject(baseVal)
    ) {
      result[key as string] = deepMerge(
        baseVal as Record<string, unknown>,
        overrideVal as Record<string, unknown>,
      );
      continue;
    }

    // Everything else (arrays, primitives, mixed types) → replace
    result[key as string] = overrideVal;
  }

  return result as T;
}

/** Returns true if value is a plain object (not null, not array, not Date, etc.) */
function isPlainObject(val: unknown): val is Record<string, unknown> {
  // Security note: isPlainObject() rejects objects with non-standard prototypes
  // (__proto__ exploits, class instances, Date, RegExp, etc.) via the
  // Object.getPrototypeOf check. Such values fall through to the replace branch.
  // axiom:trace work_item=plugin-config-management-01 spec=specs/112-Plugin-Config-Management.md#3.4
  return (
    val !== null &&
    typeof val === "object" &&
    !Array.isArray(val) &&
    Object.getPrototypeOf(val) === Object.prototype
  );
}

// ─────────────────────────────────────────────────────────────────────────────
// Type coercion for env vars
// ─────────────────────────────────────────────────────────────────────────────

/**
 * Coerce a string env var value to match the type of `defaultValue`.
 *
 * Rules (spec §3.3):
 * - boolean default: "true"/"1"/"yes" → true; "false"/"0"/"no" → false
 * - number default: Number(value). If NaN, returns `undefined` (env var ignored with warning).
 * - string[] default: JSON-parsed if starts with "[", else comma-split.
 * - string default: kept as-is.
 * - object default: JSON-parsed. If invalid JSON, returns `undefined` with warning.
 *
 * Returns the coerced value, or `undefined` if coercion fails (caller ignores).
 *
 * @note Type coercion only — does NOT validate against JSON Schema constraints
 *       (minimum, maximum, enum). Schema-constraint violations are silently accepted.
 *       Use `codeops_config validate` for semantic validation.
 *
 * axiom:trace work_item=plugin-config-management-01 spec=specs/112-Plugin-Config-Management.md#3.3
 */
export function coerceEnvValue(
  envVarName: string,
  rawValue: string,
  defaultValue: unknown,
): unknown {
  if (typeof defaultValue === "boolean") {
    const lower = rawValue.toLowerCase().trim();
    if (lower === "true" || lower === "1" || lower === "yes") return true;
    if (lower === "false" || lower === "0" || lower === "no") return false;
    pluginWarn("config-utils", `Env var ${envVarName}="${rawValue}" is not a valid boolean — ignoring`);
    return undefined;
  }

  if (typeof defaultValue === "number") {
    const n = Number(rawValue);
    if (isNaN(n)) {
      pluginWarn("config-utils", `Env var ${envVarName}="${rawValue}" is not a valid number — ignoring`);
      return undefined;
    }
    return n;
  }

  if (Array.isArray(defaultValue)) {
    const trimmed = rawValue.trim();
    if (trimmed.startsWith("[")) {
      try {
        return JSON.parse(trimmed);
      } catch {
        // Fall through to comma-split
      }
    }
    // Comma-split: "a,b,c" → ["a", "b", "c"]
    return trimmed.split(",").map((s) => s.trim());
  }

  if (isPlainObject(defaultValue)) {
    try {
      const parsed = JSON.parse(rawValue);
      if (!isPlainObject(parsed)) {
        pluginWarn("config-utils", `Env var ${envVarName}: expected an object value, got ${typeof parsed} — ignoring`);
        return undefined;
      }
      return parsed;
    } catch {
      pluginWarn("config-utils", `Env var ${envVarName}="${rawValue}" is not valid JSON for object field — ignoring`);
      return undefined;
    }
  }

  // string (or unknown/other): return as-is
  return rawValue;
}

// ─────────────────────────────────────────────────────────────────────────────
// Env var prefix utilities
// ─────────────────────────────────────────────────────────────────────────────

/**
 * Derive the canonical env var prefix for a plugin.
 * `graph-harness` → `AXIOM_GRAPH_HARNESS_`
 * `conductor`     → `AXIOM_CONDUCTOR_`
 */
export function envPrefixForPlugin(pluginName: string): string {
  if (!pluginName || typeof pluginName !== "string") return "AXIOM_UNKNOWN_";
  return `AXIOM_${pluginName.toUpperCase().replace(/-/g, "_")}_`;
}

/**
 * Convert a dotted config path to an env var suffix using double-underscore as
 * nesting separator.
 *
 * `harness.idle_evaluation_interval_ms` → `HARNESS__IDLE_EVALUATION_INTERVAL_MS`
 * `enabled`                             → `ENABLED`
 *
 * The path segments are uppercased; single underscores within segment names are
 * preserved as-is (they are part of the key name, not separators).
 */
export function pathToEnvSuffix(path: string): string {
  return path
    .split(".")
    .map((seg) => seg.toUpperCase())
    .join("__");
}

/**
 * Convert an env var suffix back to a dotted config path.
 * `HARNESS__IDLE_EVALUATION_INTERVAL_MS` → `harness.idle_evaluation_interval_ms`
 */
export function envSuffixToPath(suffix: string): string {
  return suffix
    .split("__")
    .map((seg) => seg.toLowerCase())
    .join(".");
}

// ─────────────────────────────────────────────────────────────────────────────
// Env var application
// ─────────────────────────────────────────────────────────────────────────────

/**
 * Apply matching environment variable overrides to a config object.
 *
 * Walks all top-level and nested keys in `defaults` and checks for env vars
 * matching the pattern: `{prefix}{PATH_UPPER_DOUBLE_UNDERSCORE}`.
 *
 * Only keys present in `defaults` are considered. Unknown env var keys are ignored.
 * Env vars that fail type coercion are ignored with a warning.
 *
 * axiom:trace work_item=plugin-config-management-01 spec=specs/112-Plugin-Config-Management.md#3.3
 */
export function applyEnvOverrides<T extends Record<string, unknown>>(
  config: T,
  pluginName: string,
  defaults: T,
  opts?: { envPrefix?: string },
): T {
  const prefix = opts?.envPrefix ?? envPrefixForPlugin(pluginName);
  const env = process.env;

  // Collect all leaf paths from defaults
  const paths = collectLeafPaths(defaults);
  let result: Record<string, unknown> = { ...config };

  for (const dotPath of paths) {
    const envKey = prefix + pathToEnvSuffix(dotPath);
    const rawValue = env[envKey];
    if (rawValue === undefined) continue;

    // Get default value at this path to determine expected type
    const defaultValue = getAtPath(defaults, dotPath);

    // Warn if default is an object but env var provides a non-object (ASM-2)
    if (isPlainObject(defaultValue) && !rawValue.trim().startsWith("{")) {
      pluginWarn("config-utils", `Env var ${envKey}: path "${dotPath}" resolves to a nested object. Provide a JSON object value or use more specific paths. Ignoring.`);
      continue;
    }

    const coerced = coerceEnvValue(envKey, rawValue, defaultValue);
    if (coerced === undefined) continue;

    result = setAtPath(result, dotPath, coerced) as T;
  }

  return result as T;
}

/**
 * Collect all dotted leaf paths from an object (including nested paths).
 * e.g., `{ a: { b: 1 }, c: 2 }` → `["a.b", "c"]`
 *
 * REQ-PCM-010: Key names MUST NOT contain double underscore (`__`).
 * Double underscore is reserved as the nesting separator in environment variable
 * names (e.g., `AXIOM_PLUGIN_SECTION__KEY`). A key containing `__` would make
 * env var paths ambiguous and produce silent wrong mappings.
 *
 * Throws an error at config load time if any key in `obj` (at any depth) contains `__`.
 * This provides a clear startup-time error rather than silent misconfiguration.
 *
 * Spec: specs/112-Plugin-Config-Management.md#REQ-PCM-010
 *
 * axiom:trace work_item=plugin-config-management-01 spec=specs/112-Plugin-Config-Management.md#REQ-PCM-010 plan=phase-1/task-1-3/step-1-3-2
 */
export function collectLeafPaths(
  obj: Record<string, unknown>,
  prefix = "",
): string[] {
  const paths: string[] = [];
  for (const [key, val] of Object.entries(obj)) {
    const fullPath = prefix ? `${prefix}.${key}` : key;
    // REQ-PCM-010: key names MUST NOT contain __ (reserved as env var nesting separator)
    if (key.includes("__")) {
      throw new Error(
        `[config-utils] Invalid config key "${key}" at path "${fullPath}" — ` +
        `key names MUST NOT contain double underscore (__). ` +
        `This is reserved as the env var nesting separator. ` +
        `Spec: specs/112-Plugin-Config-Management.md#REQ-PCM-010`,
      );
    }
    if (isPlainObject(val)) {
      paths.push(...collectLeafPaths(val, fullPath));
    } else {
      paths.push(fullPath);
    }
  }
  return paths;
}

/**
 * Get the value at a dotted path in an object.
 * Returns `undefined` if the path does not exist.
 */
export function getAtPath(obj: Record<string, unknown>, path: string): unknown {
  const parts = path.split(".");
  let cur: unknown = obj;
  for (const part of parts) {
    if (!isPlainObject(cur)) return undefined;
    cur = (cur as Record<string, unknown>)[part];
  }
  return cur;
}

/**
 * Set the value at a dotted path in an object. Returns a new object (does not mutate).
 */
function setAtPath(
  obj: Record<string, unknown>,
  path: string,
  value: unknown,
): Record<string, unknown> {
  const parts = path.split(".");
  if (parts.length === 1) {
    return { ...obj, [parts[0]]: value };
  }
  const [head, ...rest] = parts;
  const nested = isPlainObject(obj[head]) ? (obj[head] as Record<string, unknown>) : {};
  return {
    ...obj,
    [head]: setAtPath(nested, rest.join("."), value),
  };
}

// ─────────────────────────────────────────────────────────────────────────────
// Config loading
// ─────────────────────────────────────────────────────────────────────────────

/**
 * Load and deep-merge all config layers for a plugin.
 *
 * Layer order (later overrides earlier):
 *   1. defaults (argument)
 *   2. <configDir>/<pluginName>.json
 *   3. <configDir>/<pluginName>.local.json  (if allowLocal !== false)
 *   4. Environment variables
 *
 * Returns a new object — never mutates `defaults`.
 *
 * @param pluginName — The plugin identifier (e.g., "graph-harness"). Must match
 *   /^[a-zA-Z0-9_-]+$/.
 * @param defaults — The plugin's default config. MUST be JSON-serializable (no Date,
 *   RegExp, Function, Symbol, or circular references). `structuredClone` is used
 *   internally. Call this function ONCE at plugin startup and cache the result.
 * @param repoRoot — Absolute or relative path to the repository root.
 * @param opts — Optional overrides for configDir, envPrefix, and allowLocal.
 * @note Uses synchronous `readFileSync` internally. Config files should be under 100KB
 *   to avoid blocking plugin initialization at startup.
 *
 * axiom:trace work_item=plugin-config-management-01 spec=specs/112-Plugin-Config-Management.md#4.1
 */
export function loadPluginConfig<T extends Record<string, unknown>>(
  pluginName: string,
  defaults: T,
  repoRoot: string,
  opts?: LoadPluginConfigOptions,
): T {
  // SEC-1: validate pluginName to prevent path traversal
  validatePluginName(pluginName);

  // ASM-1: normalize repoRoot
  const root = resolve(repoRoot);
  const configDir = opts?.configDir ?? join(root, ".opencode", "config");
  const allowLocal = opts?.allowLocal !== false;

  // Start with a deep clone of defaults
  let config: T = structuredClone(defaults);

  // Layer 2: committed config file
  const configPath = join(configDir, `${pluginName}.json`);
  if (existsSync(configPath)) {
    try {
      const raw = readFileSync(configPath, "utf-8");
      const parsed = JSON.parse(raw) as Partial<T>;
      config = deepMerge(config, parsed);
    } catch (err) {
      // Distinguish I/O errors (EACCES/ENOENT) from JSON parse errors for operator clarity
      const errCode = (err as NodeJS.ErrnoException).code;
      if (errCode === "EACCES" || errCode === "ENOENT" || errCode === "EISDIR") {
        pluginWarn("config-utils", `Cannot read ${configPath} (${errCode}) — using defaults`);
      } else {
        pluginWarn("config-utils", `Failed to parse ${configPath} — using defaults:`, err);
      }
    }
  }

  // Layer 3: local override file (gitignored, per-user)
  if (allowLocal) {
    const localPath = join(configDir, `${pluginName}.local.json`);
    if (existsSync(localPath)) {
      try {
        const raw = readFileSync(localPath, "utf-8");
        const parsed = JSON.parse(raw) as Partial<T>;
        config = deepMerge(config, parsed);
      } catch (err) {
        // Distinguish I/O errors (EACCES/ENOENT) from JSON parse errors for operator clarity
        const errCode = (err as NodeJS.ErrnoException).code;
        if (errCode === "EACCES" || errCode === "ENOENT" || errCode === "EISDIR") {
          pluginWarn("config-utils", `Cannot read ${localPath} (${errCode}) — ignoring`);
        } else {
          pluginWarn("config-utils", `Failed to parse ${localPath} — ignoring:`, err);
        }
      }
    }
  }

  // Layer 4: environment variable overrides
  config = applyEnvOverrides(config, pluginName, defaults, {
    envPrefix: opts?.envPrefix,
  });

  return config;
}

// ─────────────────────────────────────────────────────────────────────────────
// Gitignore management
// ─────────────────────────────────────────────────────────────────────────────

/**
 * Ensure the given pattern is present in `<repoRoot>/.gitignore`.
 *
 * Idempotent: if the pattern (or a close *.local.json variant) is already present,
 * the file is not modified. If `.gitignore` does not exist, it is created.
 *
 * axiom:trace work_item=plugin-config-management-01 spec=specs/112-Plugin-Config-Management.md#REQ-PCM-001 plan=phase-1/task-1-3/step-1-3-1
 */
function ensureGitignored(repoRoot: string, pattern: string): void {
  const gitignorePath = join(repoRoot, ".gitignore");

  if (existsSync(gitignorePath)) {
    const content = readFileSync(gitignorePath, "utf-8");
    const lines = content.split("\n").map((l) => l.trim());
    // Check for exact match or the generic *.local.json variant
    if (
      lines.includes(pattern) ||
      lines.includes("*.local.json")
    ) {
      return; // Already present — nothing to do
    }
    // Append the pattern (with a trailing newline to avoid joining to the last line)
    appendFileSync(
      gitignorePath,
      `\n# Plugin local config overrides (gitignored per REQ-PCM-001)\n${pattern}\n`,
      "utf-8",
    );
  } else {
    // Create .gitignore with the pattern
    writeFileSync(
      gitignorePath,
      `# Plugin local config overrides (gitignored per REQ-PCM-001)\n${pattern}\n`,
      "utf-8",
    );
  }
}

// ─────────────────────────────────────────────────────────────────────────────
// Config writing
// ─────────────────────────────────────────────────────────────────────────────

/**
 * Atomically write a config patch to disk.
 *
 * Reads the existing config file (if any), deep-merges `patch`, then writes
 * using the atomic temp-file → rename pattern to prevent partial-write corruption.
 *
 * If `opts.local` is true, writes to `<plugin>.local.json` with mode 0600.
 * Creates the config directory if it does not exist (ASM-3 mitigation).
 *
 * Side effect: when `opts.local` is true (and `opts.skipGitignore` is not set),
 * this function also appends `.opencode/config/*.local.json` to `<repoRoot>/.gitignore`
 * to prevent accidental commits (REQ-PCM-001). Pass `skipGitignore: true` to suppress
 * this behaviour in CI environments or test harnesses.
 *
 * axiom:trace work_item=plugin-config-management-01 spec=specs/112-Plugin-Config-Management.md#4.2
 *
 * @note Atomicity is guaranteed on POSIX systems only (Linux, macOS). On Windows,
 *       `renameSync` is a copy+delete and is NOT atomic. Windows is best-effort.
 */
export function writePluginConfig(
  pluginName: string,
  patch: Record<string, unknown>,
  repoRoot: string,
  opts?: WritePluginConfigOptions,
): void {
  // SEC-1: validate pluginName
  validatePluginName(pluginName);

  // ASM-1: normalize repoRoot
  const root = resolve(repoRoot);
  const configDir = opts?.configDir ?? join(root, ".opencode", "config");
  const isLocal = opts?.local === true;

  // ASM-3: ensure config directory exists
  mkdirSync(configDir, { recursive: true });

  // REQ-PCM-001: ensure .gitignore contains the local config pattern on first local write
  if (isLocal && !opts?.skipGitignore) {
    try {
      ensureGitignored(root, ".opencode/config/*.local.json");
    } catch (gitignoreErr) {
      pluginWarn("config-utils", "Could not update .gitignore (read-only filesystem or non-git directory?) — continuing anyway", { error: String(gitignoreErr) });
    }
  }

  const filename = isLocal ? `${pluginName}.local.json` : `${pluginName}.json`;
  const targetPath = join(configDir, filename);

  // Read existing config (if any) and merge
  let existing: Record<string, unknown> = {};
  if (existsSync(targetPath)) {
    try {
      const raw = readFileSync(targetPath, "utf-8");
      existing = JSON.parse(raw) as Record<string, unknown>;
    } catch {
      // Start fresh if existing file is corrupt
    }
  }

  const merged = deepMerge(existing, patch);
  const content = JSON.stringify(merged, null, 2) + "\n";

  // Atomic write: write to unique temp file, then rename
  const uniqueSuffix = `${Date.now().toString(36)}_${Math.random().toString(36).slice(2, 8)}`;
  const tmpPath = `${targetPath}.${uniqueSuffix}.tmp`;

  try {
    writeFileSync(tmpPath, content, "utf-8");
    if (isLocal) {
      // REQ-PCM-002: mode 0600 for local files
      chmodSync(tmpPath, 0o600);
    }
    renameSync(tmpPath, targetPath);
  } catch (err) {
    // Clean up tmp on failure
    try {
      if (existsSync(tmpPath)) {
        unlinkSync(tmpPath);
      }
    } catch {
      /* best-effort cleanup */
    }
    throw err;
  }
}

// ─────────────────────────────────────────────────────────────────────────────
// Config metadata
// ─────────────────────────────────────────────────────────────────────────────

/**
 * Return metadata about a plugin's config state.
 * Used by the `codeops_config describe` operation (Phase 2).
 *
 * axiom:trace work_item=plugin-config-management-01 spec=specs/112-Plugin-Config-Management.md#4.4
 */
export function getConfigInfo(
  pluginName: string,
  repoRoot: string,
  opts?: { configDir?: string; envPrefix?: string },
): ConfigInfo {
  validatePluginName(pluginName);

  const root = resolve(repoRoot);
  const configDir = opts?.configDir ?? join(root, ".opencode", "config");
  const envPrefix = opts?.envPrefix ?? envPrefixForPlugin(pluginName);

  const configPath = join(configDir, `${pluginName}.json`);
  const localPath = join(configDir, `${pluginName}.local.json`);

  // Find all env vars in the current environment that match our prefix
  const activeEnvVars = Object.keys(process.env)
    .filter((k) => k.startsWith(envPrefix))
    .sort();

  return {
    pluginName,
    configPath,
    localPath,
    configExists: existsSync(configPath),
    localExists: existsSync(localPath),
    envPrefix,
    activeEnvVars,
  };
}

// ─────────────────────────────────────────────────────────────────────────────
// Plugin logging helpers
//
// OpenCode's TUI captures stderr from plugin code paths and surfaces it in the
// conversation pane. Direct console.warn/error calls flood the UI on every
// session prompt or config load.
//
// These helpers gate logging behind environment variables:
//   - AXIOM_PLUGIN_DEBUG=1 — enable ALL plugin logs
//   - AXIOM_<PLUGIN_NAME>_DEBUG=1 — enable per-plugin logs
//
// Per https://opencode.ai/docs/plugins/#logging, plugin factories that have
// access to the OpenCode SDK `client` should prefer client.app.log() instead.
// These helpers are for utility/library code paths that don't have client.
//
// axiom:trace work_item=plugin-loader-bugs-01 spec=specs/70-OpenCode-Plugin.md
// ─────────────────────────────────────────────────────────────────────────────

/**
 * Check if logging is enabled for a plugin (via env vars).
 * Returns true when either AXIOM_PLUGIN_DEBUG=1 or
 * AXIOM_<UPPERCASE_PLUGIN>_DEBUG=1 is set.
 */
function isPluginLogEnabled(pluginName: string): boolean {
  if (process.env.AXIOM_PLUGIN_DEBUG === "1") return true;
  const envName = `AXIOM_${pluginName.toUpperCase().replace(/-/g, "_")}_DEBUG`;
  return process.env[envName] === "1";
}

/**
 * Env-gated structured warning. Writes to stderr only when the appropriate
 * debug env var is set. Replacement for console.warn in plugin lib/ files.
 *
 * Usage: pluginWarn("conductor", "loadPluginConfig failed", { error: msg });
 */
export function pluginWarn(pluginName: string, message: string, extra?: unknown): void {
  if (!isPluginLogEnabled(pluginName)) return;
  const fields: Record<string, unknown> = {
    level: "warn",
    service: pluginName,
    message,
    timestamp: new Date().toISOString(),
  };
  if (extra !== undefined) fields.extra = extra;
  process.stderr.write(JSON.stringify(fields) + "\n");
}

/**
 * Env-gated structured error. Same as pluginWarn but with level=error.
 */
export function pluginError(pluginName: string, message: string, extra?: unknown): void {
  if (!isPluginLogEnabled(pluginName)) return;
  const fields: Record<string, unknown> = {
    level: "error",
    service: pluginName,
    message,
    timestamp: new Date().toISOString(),
  };
  if (extra !== undefined) fields.extra = extra;
  process.stderr.write(JSON.stringify(fields) + "\n");
}

/**
 * Env-gated structured info log.
 */
export function pluginInfo(pluginName: string, message: string, extra?: unknown): void {
  if (!isPluginLogEnabled(pluginName)) return;
  const fields: Record<string, unknown> = {
    level: "info",
    service: pluginName,
    message,
    timestamp: new Date().toISOString(),
  };
  if (extra !== undefined) fields.extra = extra;
  process.stderr.write(JSON.stringify(fields) + "\n");
}

// OpenCode plugin loader no-op — this file is a utility module, not a plugin.
// OpenCode auto-discovers all .ts files in plugins/ and tries to load them.
// This export prevents "Plugin export is not a function" errors.
export default async () => ({ tool: {} });
