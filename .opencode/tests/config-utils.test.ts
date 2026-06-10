/**
 * Tests for config-utils.ts — Plugin Config Management Phase 1.
 *
 * Covers all 9 non-tool acceptance criteria from specs/112-Plugin-Config-Management.md:
 *
 *   AC-1  loadPluginConfig returns defaults when no file and no env vars
 *   AC-2  File layer overrides defaults via deep merge
 *   AC-3  .local.json overrides .json
 *   AC-4  Env var overrides all file layers
 *   AC-5  Type coercion: boolean string → boolean, number string → number
 *   AC-6  writePluginConfig writes atomically (temp+rename)
 *   AC-7  .local.json written with mode 0600
 *   AC-8  deepMerge does not mutate inputs
 *   AC-9  Arrays are replaced (not merged) on override
 *
 * Also covers security requirements:
 *   SEC-1  pluginName validation prevents path traversal
 *   ASM-1  repoRoot normalization
 *   ASM-3  configDir auto-created by writePluginConfig
 *
 * Run: cd .opencode && bun test tests/config-utils.test.ts
 *
 * axiom:trace work_item=plugin-config-management-01 spec=specs/112-Plugin-Config-Management.md plan=phase-1/task-1-2/step-1-2-1 test=tests/config-utils.test.ts
 */

import { test, expect, describe, beforeEach, afterEach } from "bun:test";
import { existsSync, mkdtempSync, rmSync, readFileSync, statSync, writeFileSync, mkdirSync, chmodSync, readdirSync } from "node:fs";
import { join } from "node:path";
import { tmpdir } from "node:os";

import {
  deepMerge,
  loadPluginConfig,
  writePluginConfig,
  applyEnvOverrides,
  coerceEnvValue,
  envPrefixForPlugin,
  pathToEnvSuffix,
  envSuffixToPath,
  getConfigInfo,
} from "../lib/config-utils.ts";

// ─── Test helpers ─────────────────────────────────────────────────────────────

interface TestConfig {
  enabled: boolean;
  database: {
    path: string;
    busy_timeout_ms: number;
  };
  limits: {
    max_concurrent: number;
    max_retries: number;
    tags: string[];
  };
  label: string;
}

const TEST_DEFAULTS: TestConfig = {
  enabled: true,
  database: {
    path: ".data/test.db",
    busy_timeout_ms: 5000,
  },
  limits: {
    max_concurrent: 5,
    max_retries: 3,
    tags: ["default"],
  },
  label: "test-plugin",
};

function makeTmpDir(): string {
  return mkdtempSync(join(tmpdir(), "config-utils-test-"));
}

function writeJson(dir: string, filename: string, data: unknown): void {
  writeFileSync(join(dir, filename), JSON.stringify(data, null, 2));
}

function configDir(repoRoot: string): string {
  return join(repoRoot, ".opencode", "config");
}

function ensureConfigDir(repoRoot: string): void {
  mkdirSync(configDir(repoRoot), { recursive: true });
}

// ─── Test fixtures with env var isolation ─────────────────────────────────────

const TEST_PREFIX = "AXIOM_TESTPLUGIN_";

function withEnv(vars: Record<string, string>, fn: () => void): void {
  const restore: Record<string, string | undefined> = {};
  for (const [k, v] of Object.entries(vars)) {
    restore[k] = process.env[k];
    process.env[k] = v;
  }
  try {
    fn();
  } finally {
    for (const [k, original] of Object.entries(restore)) {
      if (original === undefined) {
        delete process.env[k];
      } else {
        process.env[k] = original;
      }
    }
  }
}

// ─────────────────────────────────────────────────────────────────────────────
// AC-8: deepMerge
// ─────────────────────────────────────────────────────────────────────────────

describe("deepMerge", () => {
  test("AC-8: does not mutate base", () => {
    const base = { a: 1, b: { c: 2 } };
    const override = { b: { c: 99 } };
    const result = deepMerge(base, override);
    expect(result.b.c).toBe(99);
    expect(base.b.c).toBe(2); // base unchanged
  });

  test("AC-8: does not mutate override", () => {
    const base = { a: 1, b: { c: 2 } };
    const override = { b: { c: 99 } };
    deepMerge(base, override);
    expect(override.b.c).toBe(99); // override unchanged
  });

  test("merges nested objects recursively", () => {
    const base = { a: { x: 1, y: 2 }, b: 3 };
    const override = { a: { y: 99, z: 100 } };
    const result = deepMerge(base, override);
    expect(result.a.x).toBe(1);  // preserved from base
    expect(result.a.y).toBe(99); // overridden
    expect((result.a as Record<string, number>).z).toBe(100); // new key
    expect(result.b).toBe(3);    // untouched
  });

  test("AC-9: arrays are replaced, not merged", () => {
    const base = { tags: ["a", "b", "c"], name: "x" };
    const override = { tags: ["new"] };
    const result = deepMerge(base, override);
    expect(result.tags).toEqual(["new"]); // replaced
    expect(result.tags.length).toBe(1);
  });

  test("null in override sets field to null", () => {
    const base = { a: 1, b: "hello" } as Record<string, unknown>;
    const result = deepMerge(base, { b: null } as Record<string, unknown>);
    expect(result.b).toBeNull();
    expect(result.a).toBe(1);
  });

  test("undefined in override is ignored (base preserved)", () => {
    const base = { a: 1, b: 2 };
    const result = deepMerge(base, { b: undefined });
    expect(result.b).toBe(2);
  });

  test("works with empty override", () => {
    const base = { a: 1, b: { c: 2 } };
    const result = deepMerge(base, {});
    expect(result).toEqual(base);
    expect(result).not.toBe(base); // new object
  });

  test("works with full override", () => {
    const base = { a: 1 };
    const result = deepMerge(base, { a: 42 });
    expect(result.a).toBe(42);
  });

  test("is safe against __proto__ key injection attempts", () => {
    const base = { a: 1 } as Record<string, unknown>;
    // Attempt classic prototype pollution via __proto__ key in JSON payload
    const malicious = JSON.parse('{"__proto__": {"polluted": true}}') as Record<string, unknown>;
    // deepMerge must not throw and must not pollute Object.prototype
    expect(() => deepMerge(base, malicious)).not.toThrow();
    // isPlainObject() rejects objects with non-standard prototypes — Object.prototype must be clean
    expect((Object.prototype as Record<string, unknown>)["polluted"]).toBeUndefined();
    // Base object must not be mutated
    expect(base.a).toBe(1);
  });
});

// ─────────────────────────────────────────────────────────────────────────────
// coerceEnvValue
// ─────────────────────────────────────────────────────────────────────────────

describe("coerceEnvValue", () => {
  test("AC-5: 'true' string → boolean true", () => {
    expect(coerceEnvValue("TEST", "true", false)).toBe(true);
    expect(coerceEnvValue("TEST", "1", false)).toBe(true);
    expect(coerceEnvValue("TEST", "yes", false)).toBe(true);
    expect(coerceEnvValue("TEST", "TRUE", false)).toBe(true);
  });

  test("AC-5: 'false' string → boolean false", () => {
    expect(coerceEnvValue("TEST", "false", true)).toBe(false);
    expect(coerceEnvValue("TEST", "0", true)).toBe(false);
    expect(coerceEnvValue("TEST", "no", true)).toBe(false);
  });

  test("AC-5: number string → number", () => {
    expect(coerceEnvValue("TEST", "42", 0)).toBe(42);
    expect(coerceEnvValue("TEST", "3.14", 0)).toBeCloseTo(3.14);
    expect(coerceEnvValue("TEST", "0", 5)).toBe(0);
  });

  test("AC-5: invalid number string → undefined (with warning)", () => {
    expect(coerceEnvValue("TEST", "not-a-number", 0)).toBeUndefined();
  });

  test("string default → string as-is", () => {
    expect(coerceEnvValue("TEST", "hello world", "")).toBe("hello world");
  });

  test("array default: comma-split string", () => {
    expect(coerceEnvValue("TEST", "a,b,c", [])).toEqual(["a", "b", "c"]);
  });

  test("array default: JSON array string", () => {
    expect(coerceEnvValue("TEST", '["x","y"]', [])).toEqual(["x", "y"]);
  });

  // axiom:trace work_item=plugin-config-management-01 spec=specs/112-Plugin-Config-Management.md#3.3 plan=phase-r15/task-r15-03/step-r15-03
  test("coerceEnvValue: negative number is accepted (type coercion only, no schema minimum check)", () => {
    // Documents that coerceEnvValue does NOT enforce schema minimum constraints.
    // A caller that needs schema validation should use codeops_config validate.
    // AB-NEW-2: explicit not.toThrow() mutation guard
    expect(() => coerceEnvValue("max_sessions", "-5", 5)).not.toThrow();
    const result = coerceEnvValue("max_sessions", "-5", 5); // default is 5 (number)
    expect(result).toBe(-5); // -5 is returned, not rejected
    expect(typeof result).toBe("number");
  });

  test("AB-NEW-2: coerceEnvValue does not throw on negative number input", () => {
    // axiom:trace work_item=plugin-config-management-01 spec=specs/112-Plugin-Config-Management.md#3.3
    // Dedicated mutation-guard: ensures coerceEnvValue never throws for negative numeric strings.
    expect(() => coerceEnvValue("max_sessions", "-5", 5)).not.toThrow();
    expect(() => coerceEnvValue("limit", "-100", 10)).not.toThrow();
    expect(() => coerceEnvValue("offset", "-0", 0)).not.toThrow();
  });
});

// ─────────────────────────────────────────────────────────────────────────────
// Path/prefix utilities
// ─────────────────────────────────────────────────────────────────────────────

describe("envPrefixForPlugin", () => {
  test("converts plugin name to env prefix", () => {
    expect(envPrefixForPlugin("graph-harness")).toBe("AXIOM_GRAPH_HARNESS_");
    expect(envPrefixForPlugin("conductor")).toBe("AXIOM_CONDUCTOR_");
    expect(envPrefixForPlugin("opencode-session")).toBe("AXIOM_OPENCODE_SESSION_");
  });
});

describe("pathToEnvSuffix / envSuffixToPath", () => {
  test("flat key", () => {
    expect(pathToEnvSuffix("enabled")).toBe("ENABLED");
    expect(envSuffixToPath("ENABLED")).toBe("enabled");
  });

  test("nested key", () => {
    expect(pathToEnvSuffix("harness.idle_evaluation_interval_ms"))
      .toBe("HARNESS__IDLE_EVALUATION_INTERVAL_MS");
    expect(envSuffixToPath("HARNESS__IDLE_EVALUATION_INTERVAL_MS"))
      .toBe("harness.idle_evaluation_interval_ms");
  });

  test("deep nested key", () => {
    expect(pathToEnvSuffix("a.b.c_d")).toBe("A__B__C_D");
    expect(envSuffixToPath("A__B__C_D")).toBe("a.b.c_d");
  });

  test("single underscore in key name is correctly preserved (not treated as separator)", () => {
    // busy_timeout_ms has single underscores — they stay as underscores in the segment
    expect(pathToEnvSuffix("database.busy_timeout_ms")).toBe("DATABASE__BUSY_TIMEOUT_MS");
    expect(envSuffixToPath("DATABASE__BUSY_TIMEOUT_MS")).toBe("database.busy_timeout_ms");
    // Note: this is unambiguous only because key names must not contain __
  });

  test("collision: paths differing only in underscore placement produce DIFFERENT env vars (bijection under REQ-PCM-010)", () => {
    // 'a.b_c': segment 'a', segment 'b_c' → 'A__B_C'
    // 'a_b.c': segment 'a_b', segment 'c' → 'A_B__C'
    // These are DIFFERENT — the mapping is collision-free when REQ-PCM-010 is enforced.
    // This test documents the correct bijective behavior (single _ vs __ separator is unambiguous).
    const pathWithNestedUnderscore = pathToEnvSuffix("a.b_c");   // nesting: a → b_c
    const pathWithFlatUnderscore = pathToEnvSuffix("a_b.c");     // nesting: a_b → c
    expect(pathWithNestedUnderscore).toBe("A__B_C");
    expect(pathWithFlatUnderscore).toBe("A_B__C");
    // They are different — the encoding is bijective under REQ-PCM-010 (no __ in key names)
    expect(pathWithNestedUnderscore).not.toBe(pathWithFlatUnderscore);
  });
});

// ─────────────────────────────────────────────────────────────────────────────
// AC-1: loadPluginConfig — defaults only
// ─────────────────────────────────────────────────────────────────────────────

describe("loadPluginConfig — defaults only", () => {
  let tmpDir: string;

  beforeEach(() => {
    tmpDir = makeTmpDir();
  });
  afterEach(() => {
    rmSync(tmpDir, { recursive: true, force: true });
  });

  test("AC-1: returns a clone of defaults when no config file exists", () => {
    const result = loadPluginConfig("testplugin", TEST_DEFAULTS, tmpDir);
    expect(result).toEqual(TEST_DEFAULTS);
    expect(result).not.toBe(TEST_DEFAULTS); // new object
  });

  test("AC-1: does not mutate defaults", () => {
    const defaults = { value: 42 };
    loadPluginConfig("testplugin", defaults, tmpDir);
    expect(defaults.value).toBe(42);
  });
});

// ─────────────────────────────────────────────────────────────────────────────
// AC-2: File layer overrides defaults
// ─────────────────────────────────────────────────────────────────────────────

describe("loadPluginConfig — file layer", () => {
  let tmpDir: string;

  beforeEach(() => {
    tmpDir = makeTmpDir();
    ensureConfigDir(tmpDir);
  });
  afterEach(() => {
    rmSync(tmpDir, { recursive: true, force: true });
  });

  test("AC-2: file layer overrides flat keys", () => {
    writeJson(configDir(tmpDir), "testplugin.json", { enabled: false, label: "overridden" });
    const result = loadPluginConfig("testplugin", TEST_DEFAULTS, tmpDir);
    expect(result.enabled).toBe(false);
    expect(result.label).toBe("overridden");
    // Other keys preserved
    expect(result.database.path).toBe(TEST_DEFAULTS.database.path);
  });

  test("AC-2: file layer deep-merges nested objects", () => {
    writeJson(configDir(tmpDir), "testplugin.json", {
      database: { busy_timeout_ms: 9999 },
    });
    const result = loadPluginConfig("testplugin", TEST_DEFAULTS, tmpDir);
    expect(result.database.busy_timeout_ms).toBe(9999);
    expect(result.database.path).toBe(TEST_DEFAULTS.database.path); // preserved
  });

  test("AC-9: file layer replaces arrays", () => {
    writeJson(configDir(tmpDir), "testplugin.json", {
      limits: { tags: ["from-file"] },
    });
    const result = loadPluginConfig("testplugin", TEST_DEFAULTS, tmpDir);
    expect(result.limits.tags).toEqual(["from-file"]);
  });

  test("gracefully ignores invalid JSON file (falls back to defaults)", () => {
    writeFileSync(join(configDir(tmpDir), "testplugin.json"), "NOT VALID JSON");
    const result = loadPluginConfig("testplugin", TEST_DEFAULTS, tmpDir);
    expect(result).toEqual(TEST_DEFAULTS);
  });
});

// ─────────────────────────────────────────────────────────────────────────────
// AC-3: .local.json overrides .json
// ─────────────────────────────────────────────────────────────────────────────

describe("loadPluginConfig — local file layer", () => {
  let tmpDir: string;

  beforeEach(() => {
    tmpDir = makeTmpDir();
    ensureConfigDir(tmpDir);
  });
  afterEach(() => {
    rmSync(tmpDir, { recursive: true, force: true });
  });

  test("AC-3: .local.json overrides .json", () => {
    writeJson(configDir(tmpDir), "testplugin.json", { label: "from-json" });
    writeJson(configDir(tmpDir), "testplugin.local.json", { label: "from-local" });
    const result = loadPluginConfig("testplugin", TEST_DEFAULTS, tmpDir);
    expect(result.label).toBe("from-local");
  });

  test("AC-3: .local.json deep-merges over .json for nested keys", () => {
    writeJson(configDir(tmpDir), "testplugin.json", {
      database: { busy_timeout_ms: 100 },
    });
    writeJson(configDir(tmpDir), "testplugin.local.json", {
      database: { path: "local-override.db" },
    });
    const result = loadPluginConfig("testplugin", TEST_DEFAULTS, tmpDir);
    expect(result.database.path).toBe("local-override.db");
    expect(result.database.busy_timeout_ms).toBe(100); // from .json, preserved
  });

  test("allowLocal=false skips .local.json", () => {
    writeJson(configDir(tmpDir), "testplugin.json", { label: "from-json" });
    writeJson(configDir(tmpDir), "testplugin.local.json", { label: "from-local" });
    const result = loadPluginConfig("testplugin", TEST_DEFAULTS, tmpDir, { allowLocal: false });
    expect(result.label).toBe("from-json"); // .local.json skipped
  });
});

// ─────────────────────────────────────────────────────────────────────────────
// AC-4: Env var overrides all file layers
// ─────────────────────────────────────────────────────────────────────────────

describe("loadPluginConfig — env var layer (AC-4, AC-5)", () => {
  let tmpDir: string;

  beforeEach(() => {
    tmpDir = makeTmpDir();
    ensureConfigDir(tmpDir);
  });
  afterEach(() => {
    rmSync(tmpDir, { recursive: true, force: true });
  });

  test("AC-4: env var overrides .json and .local.json", () => {
    writeJson(configDir(tmpDir), "testplugin.json", { label: "from-json" });
    writeJson(configDir(tmpDir), "testplugin.local.json", { label: "from-local" });
    withEnv({ AXIOM_TESTPLUGIN_LABEL: "from-env" }, () => {
      const result = loadPluginConfig("testplugin", TEST_DEFAULTS, tmpDir);
      expect(result.label).toBe("from-env");
    });
  });

  test("AC-4 + AC-5: env var boolean coercion", () => {
    withEnv({ AXIOM_TESTPLUGIN_ENABLED: "false" }, () => {
      const result = loadPluginConfig("testplugin", TEST_DEFAULTS, tmpDir);
      expect(result.enabled).toBe(false);
    });
  });

  test("AC-4 + AC-5: env var number coercion for nested key", () => {
    withEnv({ AXIOM_TESTPLUGIN_DATABASE__BUSY_TIMEOUT_MS: "12345" }, () => {
      const result = loadPluginConfig("testplugin", TEST_DEFAULTS, tmpDir);
      expect(result.database.busy_timeout_ms).toBe(12345);
    });
  });

  test("AC-4 + AC-5: env var number for double-nested key", () => {
    withEnv({ AXIOM_TESTPLUGIN_LIMITS__MAX_CONCURRENT: "99" }, () => {
      const result = loadPluginConfig("testplugin", TEST_DEFAULTS, tmpDir);
      expect(result.limits.max_concurrent).toBe(99);
    });
  });

  test("AC-5: invalid env var number is ignored (defaults preserved)", () => {
    withEnv({ AXIOM_TESTPLUGIN_LIMITS__MAX_CONCURRENT: "not-a-number" }, () => {
      const result = loadPluginConfig("testplugin", TEST_DEFAULTS, tmpDir);
      expect(result.limits.max_concurrent).toBe(TEST_DEFAULTS.limits.max_concurrent);
    });
  });

  test("AC-9: env var replaces array", () => {
    withEnv({ AXIOM_TESTPLUGIN_LIMITS__TAGS: "x,y,z" }, () => {
      const result = loadPluginConfig("testplugin", TEST_DEFAULTS, tmpDir);
      expect(result.limits.tags).toEqual(["x", "y", "z"]);
    });
  });
});

// ─────────────────────────────────────────────────────────────────────────────
// AC-6: writePluginConfig — atomic write
// ─────────────────────────────────────────────────────────────────────────────

describe("writePluginConfig", () => {
  let tmpDir: string;

  beforeEach(() => {
    tmpDir = makeTmpDir();
  });
  afterEach(() => {
    rmSync(tmpDir, { recursive: true, force: true });
  });

  test("AC-6: writes config file", () => {
    writePluginConfig("testplugin", { label: "written" }, tmpDir);
    const path = join(configDir(tmpDir), "testplugin.json");
    expect(existsSync(path)).toBe(true);
    const content = JSON.parse(readFileSync(path, "utf-8"));
    expect(content.label).toBe("written");
  });

  test("AC-6: deep-merges into existing file", () => {
    ensureConfigDir(tmpDir);
    writeJson(configDir(tmpDir), "testplugin.json", { a: 1, b: { c: 2 } });
    writePluginConfig("testplugin", { b: { d: 3 } }, tmpDir);
    const path = join(configDir(tmpDir), "testplugin.json");
    const content = JSON.parse(readFileSync(path, "utf-8"));
    expect(content.a).toBe(1);
    expect(content.b.c).toBe(2); // preserved
    expect(content.b.d).toBe(3); // new key
  });

  test("ASM-3: creates config directory if it doesn't exist", () => {
    // tmpDir exists but .opencode/config/ does not
    const cdPath = configDir(tmpDir);
    expect(existsSync(cdPath)).toBe(false);
    writePluginConfig("testplugin", { x: 1 }, tmpDir);
    expect(existsSync(cdPath)).toBe(true);
  });

  test("AC-7: .local.json written with mode 0600", () => {
    writePluginConfig("testplugin", { local_key: "value" }, tmpDir, { local: true });
    const path = join(configDir(tmpDir), "testplugin.local.json");
    expect(existsSync(path)).toBe(true);
    const mode = statSync(path).mode & 0o777;
    // On Linux, mode should be 0600; on some systems umask may affect this.
    // We check that group and other bits are 0 (owner-only).
    const groupAndOtherBits = mode & 0o077;
    expect(groupAndOtherBits).toBe(0);
  });

  test("AC-7: .local.json content is correct", () => {
    writePluginConfig("testplugin", { secret_key: "abc" }, tmpDir, { local: true });
    const path = join(configDir(tmpDir), "testplugin.local.json");
    const content = JSON.parse(readFileSync(path, "utf-8"));
    expect(content.secret_key).toBe("abc");
  });

  test("leaves no .tmp file after successful write", () => {
    writePluginConfig("testplugin", { x: 1 }, tmpDir);
    const tmpFiles = (existsSync(configDir(tmpDir))
      ? require("node:fs").readdirSync(configDir(tmpDir)) as string[]
      : []
    ).filter((f: string) => f.endsWith(".tmp"));
    expect(tmpFiles.length).toBe(0);
  });

  test("REQ-PCM-001: .gitignore updated on first local write", () => {
    writePluginConfig("testplugin", { x: 1 }, tmpDir, { local: true });
    const gitignorePath = join(tmpDir, ".gitignore");
    expect(existsSync(gitignorePath)).toBe(true);
    const content = readFileSync(gitignorePath, "utf-8");
    expect(content).toContain(".opencode/config/*.local.json");
  });

  test("REQ-PCM-001: .gitignore not modified for non-local write", () => {
    writePluginConfig("testplugin", { x: 1 }, tmpDir);
    const gitignorePath = join(tmpDir, ".gitignore");
    // Non-local write must NOT create .gitignore at all
    expect(existsSync(gitignorePath)).toBe(false);
  });

  test("REQ-PCM-001: .gitignore idempotent — pattern not duplicated on second local write", () => {
    writePluginConfig("testplugin", { x: 1 }, tmpDir, { local: true });
    writePluginConfig("testplugin", { x: 2 }, tmpDir, { local: true });
    const content = readFileSync(join(tmpDir, ".gitignore"), "utf-8");
    // Pattern should appear exactly once
    const matches = content.split("\n").filter(l => l.trim() === ".opencode/config/*.local.json");
    expect(matches.length).toBe(1);
  });

  test("REQ-PCM-001: skipGitignore=true does NOT modify .gitignore", () => {
    // axiom:trace work_item=plugin-config-management-01 spec=specs/112-Plugin-Config-Management.md#4.2 plan=phase-2/task-2-1/step-2-1-1
    writePluginConfig("testplugin", { x: 1 }, tmpDir, { local: true, skipGitignore: true });
    const gitignorePath = join(tmpDir, ".gitignore");
    // .gitignore must NOT be created when skipGitignore is true
    expect(existsSync(gitignorePath)).toBe(false);
    // The .local.json file itself must still be written
    expect(existsSync(join(configDir(tmpDir), "testplugin.local.json"))).toBe(true);
  });

  test("REQ-PCM-001: ensureGitignored failure is swallowed with warning (does not block write)", () => {
    // axiom:trace work_item=plugin-config-management-01 spec=specs/112-Plugin-Config-Management.md#4.2 plan=phase-2/task-2-1/step-2-1-1
    // Simulate a read-only .gitignore by creating one with mode 0444
    const gitignorePath = join(tmpDir, ".gitignore");
    writeFileSync(gitignorePath, "# existing content\n");
    chmodSync(gitignorePath, 0o444); // read-only
    // writePluginConfig must NOT throw; it must log a warning and continue
    expect(() =>
      writePluginConfig("testplugin", { x: 1 }, tmpDir, { local: true }),
    ).not.toThrow();
    // The local config file must be written successfully
    expect(existsSync(join(configDir(tmpDir), "testplugin.local.json"))).toBe(true);
    // Cleanup: restore write permission
    chmodSync(gitignorePath, 0o644);
  });

  test("QA-4: writePluginConfig then loadPluginConfig round-trip preserves values", () => {
    // axiom:trace work_item=plugin-config-management-01 spec=specs/112-Plugin-Config-Management.md#AC-6
    const tmpDir2 = mkdtempSync(join(tmpdir(), "pcm-roundtrip-"));
    try {
      const data = { api_key: "round-trip-value", max_count: 99 };
      const defaults = { api_key: "", max_count: 0 };
      writePluginConfig("testplugin", data, tmpDir2);
      const loaded = loadPluginConfig("testplugin", defaults, tmpDir2);
      expect(loaded.api_key).toBe("round-trip-value");
      expect(loaded.max_count).toBe(99);
    } finally {
      rmSync(tmpDir2, { recursive: true, force: true });
    }
  });

  test("AC-6: no .tmp file remains after successful writePluginConfig", () => {
    // axiom:trace work_item=plugin-config-management-01 spec=specs/112-Plugin-Config-Management.md#AC-6
    const tmpDir3 = mkdtempSync(join(tmpdir(), "pcm-tmp-clean-"));
    try {
      writePluginConfig("testplugin", { key: "value" }, tmpDir3);
      const cd = join(tmpDir3, ".opencode", "config");
      if (existsSync(cd)) {
        const tmpFiles = readdirSync(cd).filter(f => f.endsWith(".tmp"));
        expect(tmpFiles.length).toBe(0);
      }
    } finally {
      rmSync(tmpDir3, { recursive: true, force: true });
    }
  });
});

// ─────────────────────────────────────────────────────────────────────────────
// SEC-1: pluginName validation
// ─────────────────────────────────────────────────────────────────────────────

describe("pluginName security validation (SEC-1)", () => {
  let tmpDir: string;

  beforeEach(() => {
    tmpDir = makeTmpDir();
  });
  afterEach(() => {
    rmSync(tmpDir, { recursive: true, force: true });
  });

  test("SEC-1: loadPluginConfig rejects path traversal", () => {
    expect(() =>
      loadPluginConfig("../../etc/passwd", TEST_DEFAULTS, tmpDir),
    ).toThrow(/path traversal/i);
  });

  test("SEC-1: writePluginConfig rejects path traversal", () => {
    expect(() =>
      writePluginConfig("../../etc/passwd", {}, tmpDir),
    ).toThrow(/path traversal/i);
  });

  test("SEC-1: rejects plugin names with spaces", () => {
    expect(() =>
      loadPluginConfig("bad plugin", TEST_DEFAULTS, tmpDir),
    ).toThrow();
  });

  test("SEC-1: accepts valid plugin names", () => {
    // Should not throw
    expect(() =>
      loadPluginConfig("graph-harness", TEST_DEFAULTS, tmpDir),
    ).not.toThrow();
    expect(() =>
      loadPluginConfig("my_plugin_01", TEST_DEFAULTS, tmpDir),
    ).not.toThrow();
  });
});

// ─────────────────────────────────────────────────────────────────────────────
// getConfigInfo
// ─────────────────────────────────────────────────────────────────────────────

describe("getConfigInfo", () => {
  let tmpDir: string;

  beforeEach(() => {
    tmpDir = makeTmpDir();
  });
  afterEach(() => {
    rmSync(tmpDir, { recursive: true, force: true });
  });

  test("returns correct paths and existence flags when no files exist", () => {
    const info = getConfigInfo("testplugin", tmpDir);
    expect(info.pluginName).toBe("testplugin");
    expect(info.configExists).toBe(false);
    expect(info.localExists).toBe(false);
    expect(info.envPrefix).toBe("AXIOM_TESTPLUGIN_");
    expect(info.configPath).toContain("testplugin.json");
    expect(info.localPath).toContain("testplugin.local.json");
    expect(Array.isArray(info.activeEnvVars)).toBe(true);
  });

  test("reflects config file existence after write", () => {
    writePluginConfig("testplugin", { x: 1 }, tmpDir);
    const info = getConfigInfo("testplugin", tmpDir);
    expect(info.configExists).toBe(true);
    expect(info.localExists).toBe(false);
  });

  test("lists active env vars matching prefix", () => {
    withEnv({ AXIOM_TESTPLUGIN_LABEL: "hello" }, () => {
      const info = getConfigInfo("testplugin", tmpDir);
      expect(info.activeEnvVars).toContain("AXIOM_TESTPLUGIN_LABEL");
    });
  });
});

// ─────────────────────────────────────────────────────────────────────────────
// applyEnvOverrides (unit tests for the exported helper)
// ─────────────────────────────────────────────────────────────────────────────

describe("applyEnvOverrides", () => {
  test("applies multiple overrides in one call", () => {
    withEnv({
      "AXIOM_TESTPLUGIN_ENABLED": "false",
      "AXIOM_TESTPLUGIN_LABEL": "new-label",
      "AXIOM_TESTPLUGIN_DATABASE__BUSY_TIMEOUT_MS": "999",
    }, () => {
      const result = applyEnvOverrides(
        TEST_DEFAULTS,
        "testplugin",
        TEST_DEFAULTS,
      );
      expect(result.enabled).toBe(false);
      expect(result.label).toBe("new-label");
      expect(result.database.busy_timeout_ms).toBe(999);
      expect(result.database.path).toBe(TEST_DEFAULTS.database.path); // untouched
    });
  });

  test("does not mutate the input config", () => {
    const input = structuredClone(TEST_DEFAULTS);
    withEnv({ "AXIOM_TESTPLUGIN_LABEL": "mutate-test" }, () => {
      applyEnvOverrides(input, "testplugin", TEST_DEFAULTS);
    });
    expect(input.label).toBe(TEST_DEFAULTS.label); // original unchanged
  });
});

// ─────────────────────────────────────────────────────────────────────────────
// REQ-PCM-010: collectLeafPaths constraint — double-underscore key name guard
// ─────────────────────────────────────────────────────────────────────────────

// axiom:trace work_item=plugin-config-management-01 spec=specs/112-Plugin-Config-Management.md#REQ-PCM-010 plan=phase-1/task-1-3/step-1-3-2 test=tests/config-utils.test.ts

describe("collectLeafPaths constraint — REQ-PCM-010", () => {
  let tmpDir: string;
  beforeEach(() => { tmpDir = makeTmpDir(); });
  afterEach(() => { rmSync(tmpDir, { recursive: true, force: true }); });

  test("REQ-PCM-010: loadPluginConfig throws if defaults have a key containing __", () => {
    const badDefaults = {
      valid_key: 1,
      nested: { bad__key: "value" },
    };
    expect(() =>
      loadPluginConfig("testplugin", badDefaults as unknown as typeof TEST_DEFAULTS, tmpDir),
    ).toThrow(/key names MUST NOT contain double underscore/i);
  });

  test("REQ-PCM-010: loadPluginConfig succeeds with keys containing single underscores", () => {
    // Single underscores are fine — only __ is forbidden
    const goodDefaults = {
      idle_evaluation_interval_ms: 1000,
      nested: { max_concurrent_sessions: 5 },
    };
    expect(() =>
      loadPluginConfig("testplugin", goodDefaults as unknown as typeof TEST_DEFAULTS, tmpDir),
    ).not.toThrow();
  });

  test("REQ-PCM-010: applyEnvOverrides throws if defaults have a __ key", () => {
    const badDefaults = { bad__key: 1 };
    expect(() =>
      applyEnvOverrides(badDefaults, "testplugin", badDefaults),
    ).toThrow(/key names MUST NOT contain double underscore/i);
  });
});

// ─────────────────────────────────────────────────────────────────────────────
// Custom options: configDir and envPrefix
// ─────────────────────────────────────────────────────────────────────────────

// axiom:trace work_item=plugin-config-management-01 spec=specs/112-Plugin-Config-Management.md#4.1 plan=phase-1/task-1-5/step-1-5-1 test=tests/config-utils.test.ts

describe("loadPluginConfig / writePluginConfig — custom options", () => {
  let tmpDir: string;

  beforeEach(() => {
    tmpDir = makeTmpDir();
  });
  afterEach(() => {
    rmSync(tmpDir, { recursive: true, force: true });
  });

  test("loadPluginConfig: custom configDir is used instead of default", () => {
    const customDir = join(tmpDir, "my-custom-config");
    mkdirSync(customDir, { recursive: true });
    writeFileSync(join(customDir, "testplugin.json"), JSON.stringify({ label: "custom-dir" }));
    // The default configDir (.opencode/config/) is empty — should not be consulted
    const result = loadPluginConfig("testplugin", TEST_DEFAULTS, tmpDir, {
      configDir: customDir,
    });
    expect(result.label).toBe("custom-dir");
    // Verify the default config path was NOT created
    expect(existsSync(configDir(tmpDir))).toBe(false);
  });

  test("loadPluginConfig: custom envPrefix is used instead of default prefix", () => {
    withEnv({ "MYAPP_LABEL": "custom-prefix-value" }, () => {
      const result = loadPluginConfig("testplugin", TEST_DEFAULTS, tmpDir, {
        envPrefix: "MYAPP_",
      });
      expect(result.label).toBe("custom-prefix-value");
    });
    // Default prefix (AXIOM_TESTPLUGIN_) should NOT match MYAPP_ vars
    const resultDefaultPrefix = loadPluginConfig("testplugin", TEST_DEFAULTS, tmpDir);
    expect(resultDefaultPrefix.label).toBe(TEST_DEFAULTS.label);
  });

  test("writePluginConfig: custom configDir is used instead of default", () => {
    const customDir = join(tmpDir, "write-custom");
    writePluginConfig("testplugin", { x: 42 }, tmpDir, { configDir: customDir });
    expect(existsSync(join(customDir, "testplugin.json"))).toBe(true);
    const content = JSON.parse(readFileSync(join(customDir, "testplugin.json"), "utf-8"));
    expect(content.x).toBe(42);
    // Verify default path was NOT written
    expect(existsSync(join(configDir(tmpDir), "testplugin.json"))).toBe(false);
  });

  test("getConfigInfo: prefix isolation — no false positives from other plugin prefixes", () => {
    withEnv({
      "AXIOM_TESTPLUGIN_LABEL": "mine",
      "AXIOM_OTHERPLUGIN_LABEL": "not-mine",
    }, () => {
      const info = getConfigInfo("testplugin", tmpDir);
      expect(info.activeEnvVars).toContain("AXIOM_TESTPLUGIN_LABEL");
      expect(info.activeEnvVars).not.toContain("AXIOM_OTHERPLUGIN_LABEL");
    });
  });
});

// ─────────────────────────────────────────────────────────────────────────────
// Error-path coverage
// ─────────────────────────────────────────────────────────────────────────────

describe("error-path coverage", () => {
  let tmpDir: string;

  beforeEach(() => {
    tmpDir = makeTmpDir();
    ensureConfigDir(tmpDir);
  });
  afterEach(() => {
    rmSync(tmpDir, { recursive: true, force: true });
  });

  // Catch path at loadPluginConfig local layer: corrupt .local.json file
  test("loadPluginConfig: corrupt .local.json is gracefully ignored (falls back to .json layer)", () => {
    writeJson(configDir(tmpDir), "testplugin.json", { label: "from-json" });
    writeFileSync(join(configDir(tmpDir), "testplugin.local.json"), "NOT VALID JSON");
    const result = loadPluginConfig("testplugin", TEST_DEFAULTS, tmpDir);
    // Should use .json layer value, not crash
    expect(result.label).toBe("from-json");
  });

  // Catch path at writePluginConfig: corrupt existing file
  test("writePluginConfig: starts fresh when existing .json file is corrupt", () => {
    writeFileSync(join(configDir(tmpDir), "testplugin.json"), "CORRUPT JSON");
    expect(() =>
      writePluginConfig("testplugin", { fresh_key: true }, tmpDir),
    ).not.toThrow();
    const content = JSON.parse(
      readFileSync(join(configDir(tmpDir), "testplugin.json"), "utf-8"),
    );
    expect(content.fresh_key).toBe(true);
  });

  // Catch path at coerceEnvValue: invalid JSON array prefix
  test("coerceEnvValue: invalid JSON with array prefix falls back to comma-split", () => {
    // Starts with "[" but is not valid JSON — should fall through to comma-split
    const result = coerceEnvValue("TEST", "[invalid", []);
    // Falls back to comma-split: "[invalid" → ["[invalid"]
    expect(Array.isArray(result)).toBe(true);
    expect((result as string[])[0]).toBe("[invalid");
  });

  // Deep nesting (3+ levels)
  test("deepMerge: handles 3+ levels of nesting correctly", () => {
    const base = { a: { b: { c: { d: 1, e: 2 } } } } as Record<string, unknown>;
    const override = { a: { b: { c: { d: 99 } } } } as Record<string, unknown>;
    const result = deepMerge(base, override);
    const inner = ((result.a as Record<string, unknown>).b as Record<string, unknown>).c as Record<string, unknown>;
    expect(inner.d).toBe(99);
    expect(inner.e).toBe(2);
    // Verify base not mutated
    const baseInner = ((base.a as Record<string, unknown>).b as Record<string, unknown>).c as Record<string, unknown>;
    expect(baseInner.d).toBe(1);
  });
});
