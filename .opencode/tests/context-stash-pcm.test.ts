/**
 * Phase 4 Adoption Gate — context-stash plugin config integration.
 *
 * Verifies that context-stash loads its config via loadPluginConfig() (context-stash-config-adoption),
 * that security guards work (path traversal, empty directory), that size limits are enforced,
 * and that the Option C migration warning fires for legacy YAML configs.
 *
 * axiom:trace work_item=context-stash-config-adoption spec=specs/112-Plugin-Config-Management.md#8 plan=phase-2/task-2-1/step-cs-03
 */

import { test, expect, describe, beforeEach, afterEach } from "bun:test";
import { mkdirSync, mkdtempSync, rmSync, existsSync, writeFileSync } from "node:fs";
import { mkdir, writeFile, rm } from "node:fs/promises";
import { join } from "node:path";
import { tmpdir } from "node:os";

import { ContextStashPlugin, DEFAULT_STASH_CONFIG } from "../lib/context-stash.ts";
import { loadPluginConfig } from "../lib/config-utils.ts";

function makeTmpDir(): string {
  return mkdtempSync(join(tmpdir(), "stash-pcm-test-"));
}

describe("Phase 4: context-stash security guards + config loading", () => {
  let tmpDir: string;

  beforeEach(() => {
    tmpDir = makeTmpDir();
  });

  afterEach(() => {
    rmSync(tmpDir, { recursive: true, force: true });
  });

  test("Plugin creates storageRoot from default storage_path (.memory-bank/stash)", async () => {
    const plugin = await ContextStashPlugin({ directory: tmpDir, client: {} });
    expect(plugin).toBeDefined();
    const expectedRoot = join(tmpDir, ".memory-bank", "stash");
    expect(existsSync(expectedRoot)).toBe(true);
  });

  test(
    "step-pcm-r10-01: path traversal in storage_path is rejected",
    async () => {
      // loadPluginConfig reads from .opencode/config/context-stash.json (not YAML)
      const configDir = join(tmpDir, ".opencode", "config");
      mkdirSync(configDir, { recursive: true });
      writeFileSync(join(configDir, "context-stash.json"), JSON.stringify({ storage_path: "../../tmp" }));
      await expect(
        ContextStashPlugin({ directory: tmpDir, client: {} })
      ).rejects.toThrow(/path traversal/i);
    },
  );

  test(
    "active_size_limit_kb enforcement rejects oversized stash.push",
    async () => {
      // loadPluginConfig reads from .opencode/config/context-stash.json
      const configDir = join(tmpDir, ".opencode", "config");
      mkdirSync(configDir, { recursive: true });
      writeFileSync(join(configDir, "context-stash.json"), JSON.stringify({ active_size_limit_kb: 1 }));
      const plugin = await ContextStashPlugin({ directory: tmpDir, client: {} });
      const result = JSON.parse(
        await plugin.tool["stash_push"].execute(
          {
            name: "test-stash",
            summary: "A".repeat(2000), // ~2 KB > 1 KB limit
          },
          {}
        ) as string
      );
      expect(result.error).toMatch(/active_size_limit_kb/i);
    },
  );

  test(
    "active_size_limit_kb=0 allows unlimited stash.push content",
    async () => {
      const configDir = join(tmpDir, ".opencode", "config");
      mkdirSync(configDir, { recursive: true });
      writeFileSync(join(configDir, "context-stash.json"), JSON.stringify({ active_size_limit_kb: 0 }));
      const plugin = await ContextStashPlugin({ directory: tmpDir, client: {} });
      const result = JSON.parse(
        await plugin.tool["stash_push"].execute(
          {
            name: "big-stash",
            summary: "A".repeat(5000), // 5 KB — passes with unlimited, fast enough for test
          },
          {}
        ) as string
      );
      expect(result.error).toBeUndefined();
      expect(result.stash_id).toBeDefined();
    },
  );
});

test(
  "step-pcm-r11-04: empty directory throws clear error",
  async () => {
    await expect(
      ContextStashPlugin({ directory: "", client: {} })
    ).rejects.toThrow(/directory must be non-empty/i);
  },
);

test(
  "step-pcm-r11-04: whitespace-only directory throws clear error",
  async () => {
    await expect(
      ContextStashPlugin({ directory: "   ", client: {} })
    ).rejects.toThrow(/directory must be non-empty/i);
  },
);

// ─────────────────────────────────────────────────────────────────────────────
// NEW: Phase 4 adoption tests — mirrors conductor-pcm.test.ts pattern
// axiom:trace work_item=context-stash-config-adoption spec=specs/112-Plugin-Config-Management.md#8 plan=phase-2/task-2-1/step-cs-03
// ─────────────────────────────────────────────────────────────────────────────

describe("AC-CS: context-stash loads config via loadPluginConfig()", () => {
  test("env var override: AXIOM_CONTEXT_STASH_ACTIVE_SIZE_LIMIT_KB takes effect", () => {
    // axiom:trace work_item=context-stash-config-adoption spec=specs/112-Plugin-Config-Management.md#3.3 plan=phase-2/task-2-1/step-cs-03
    const tmpDir = mkdtempSync(join(tmpdir(), "stash-pcm-env-"));
    const envKey = "AXIOM_CONTEXT_STASH_ACTIVE_SIZE_LIMIT_KB";
    const prev = process.env[envKey];
    process.env[envKey] = "4096";
    try {
      const loaded = loadPluginConfig("context-stash", DEFAULT_STASH_CONFIG, tmpDir);
      expect((loaded as typeof DEFAULT_STASH_CONFIG).active_size_limit_kb).toBe(4096);
    } finally {
      if (prev === undefined) delete process.env[envKey];
      else process.env[envKey] = prev;
      rmSync(tmpDir, { recursive: true, force: true });
    }
  });

  test("Layer 2: reads overrides from .opencode/config/context-stash.json", async () => {
    // axiom:trace work_item=context-stash-config-adoption spec=specs/112-Plugin-Config-Management.md#3.2 plan=phase-2/task-2-1/step-cs-03
    const tmpDir = mkdtempSync(join(tmpdir(), "stash-pcm-layer2-"));
    try {
      const configDir = join(tmpDir, ".opencode", "config");
      await mkdir(configDir, { recursive: true });
      await writeFile(join(configDir, "context-stash.json"), JSON.stringify({
        active_size_limit_kb: 8192
      }));
      const loaded = loadPluginConfig("context-stash", DEFAULT_STASH_CONFIG, tmpDir);
      expect((loaded as typeof DEFAULT_STASH_CONFIG).active_size_limit_kb).toBe(8192);
    } finally {
      await rm(tmpDir, { recursive: true, force: true });
    }
  });

  test("Layer 3 (.local.json) overrides Layer 2 (.json)", async () => {
    // axiom:trace work_item=context-stash-config-adoption spec=specs/112-Plugin-Config-Management.md#3.1 plan=phase-2/task-2-1/step-cs-03
    const tmpDir = mkdtempSync(join(tmpdir(), "stash-pcm-layer3-"));
    try {
      const configDir = join(tmpDir, ".opencode", "config");
      await mkdir(configDir, { recursive: true });
      await writeFile(join(configDir, "context-stash.json"), JSON.stringify({
        active_size_limit_kb: 8192
      }));
      await writeFile(join(configDir, "context-stash.local.json"), JSON.stringify({
        active_size_limit_kb: 16384
      }));
      const loaded = loadPluginConfig("context-stash", DEFAULT_STASH_CONFIG, tmpDir);
      expect((loaded as typeof DEFAULT_STASH_CONFIG).active_size_limit_kb).toBe(16384);
    } finally {
      await rm(tmpDir, { recursive: true, force: true });
    }
  });

  test("migration warning fires when axiom.config.yaml has stash: section and no context-stash.json", async () => {
    // axiom:trace work_item=context-stash-config-adoption spec=specs/112-Plugin-Config-Management.md#8 plan=phase-2/task-2-1/step-cs-03
    const tmpDir = mkdtempSync(join(tmpdir(), "stash-pcm-migwarn-"));
    try {
      // Write legacy YAML config with a stash: section
      const codeopsDir = join(tmpDir, ".axiom");
      await mkdir(codeopsDir, { recursive: true });
      await writeFile(
        join(codeopsDir, "axiom.config.yaml"),
        "stash:\n  active_size_limit_kb: 512\n"
      );
      // Deliberately do NOT create .opencode/config/context-stash.json

      // Capture pluginWarn output to stderr (env-gated)
      const warned: string[] = [];
      const origWrite = process.stderr.write.bind(process.stderr);
      const prevDebug = process.env.AXIOM_CONTEXT_STASH_DEBUG;
      process.env.AXIOM_CONTEXT_STASH_DEBUG = "1";
      process.stderr.write = ((chunk: string | Uint8Array): boolean => {
        warned.push(typeof chunk === "string" ? chunk : new TextDecoder().decode(chunk));
        return true;
      }) as typeof process.stderr.write;

      try {
        await ContextStashPlugin({ directory: tmpDir, client: {} });
      } finally {
        process.stderr.write = origWrite;
        if (prevDebug === undefined) delete process.env.AXIOM_CONTEXT_STASH_DEBUG;
        else process.env.AXIOM_CONTEXT_STASH_DEBUG = prevDebug;
      }

      expect(warned.some(m => m.includes("MIGRATION REQUIRED"))).toBe(true);
      expect(warned.some(m => m.includes("axiom.config.yaml"))).toBe(true);
      expect(warned.some(m => m.includes("context-stash.json"))).toBe(true);
      expect(warned.some(m => m.includes("512") || m.includes("active_size_limit_kb"))).toBe(true);
      // test-warning-message-format: §12 reference must appear so users can find migration docs
      // axiom:trace work_item=context-stash-config-adoption spec=specs/112-Plugin-Config-Management.md#8 plan=phase-5/task-5-2/test-warning-message-format
      expect(warned.some(m => m.includes("specs/112"))).toBe(true);
    } finally {
      await rm(tmpDir, { recursive: true, force: true });
    }
  });

  test("migration warning does NOT fire when context-stash.json exists", async () => {
    // axiom:trace work_item=context-stash-config-adoption spec=specs/112-Plugin-Config-Management.md#8 plan=phase-2/task-2-1/step-cs-03
    // Option C: suppress warning once the user has migrated to JSON config.
    const tmpDir = mkdtempSync(join(tmpdir(), "stash-pcm-nomigwarn-"));
    try {
      // Write legacy YAML config with a stash: section
      const codeopsDir = join(tmpDir, ".axiom");
      await mkdir(codeopsDir, { recursive: true });
      await writeFile(
        join(codeopsDir, "axiom.config.yaml"),
        "stash:\n  active_size_limit_kb: 512\n"
      );
      // Also create context-stash.json — user has migrated
      const configDir = join(tmpDir, ".opencode", "config");
      await mkdir(configDir, { recursive: true });
      await writeFile(join(configDir, "context-stash.json"), JSON.stringify({ active_size_limit_kb: 512 }));

      const warned: string[] = [];
      const origWrite = process.stderr.write.bind(process.stderr);
      const prevDebug = process.env.AXIOM_CONTEXT_STASH_DEBUG;
      process.env.AXIOM_CONTEXT_STASH_DEBUG = "1";
      process.stderr.write = ((chunk: string | Uint8Array): boolean => {
        warned.push(typeof chunk === "string" ? chunk : new TextDecoder().decode(chunk));
        return true;
      }) as typeof process.stderr.write;

      try {
        await ContextStashPlugin({ directory: tmpDir, client: {} });
      } finally {
        process.stderr.write = origWrite;
        if (prevDebug === undefined) delete process.env.AXIOM_CONTEXT_STASH_DEBUG;
        else process.env.AXIOM_CONTEXT_STASH_DEBUG = prevDebug;
      }

      expect(warned.some(m => m.includes("MIGRATION REQUIRED"))).toBe(false);
    } finally {
      await rm(tmpDir, { recursive: true, force: true });
    }
  });

  test("DEFAULT_STASH_CONFIG.git.track_suspended is false (REQ-STASH-NEW-006 security default)", () => {
    // axiom:trace work_item=context-stash-config-adoption spec=specs/106-Context-Stash.md#REQ-STASH-NEW-006 plan=phase-2/task-2-1/step-cs-03
    expect(DEFAULT_STASH_CONFIG.git.track_suspended).toBe(false);
  });

  test("DEFAULT_STASH_CONFIG is JSON-serializable (safe for structuredClone)", () => {
    // axiom:trace work_item=context-stash-config-adoption spec=specs/112-Plugin-Config-Management.md#8 plan=phase-2/task-2-1/step-cs-03
    const roundTripped = JSON.parse(JSON.stringify(DEFAULT_STASH_CONFIG));
    expect(roundTripped).toEqual(DEFAULT_STASH_CONFIG);
  });

  test("malformed context-stash.json is ignored — default values used", async () => {
    // axiom:trace work_item=context-stash-config-adoption spec=specs/112-Plugin-Config-Management.md#3.2 plan=phase-5/task-5-1/test-invalid-json-fallback
    const tmpDir = mkdtempSync(join(tmpdir(), "stash-pcm-malformed-"));
    try {
      const configDir = join(tmpDir, ".opencode", "config");
      await mkdir(configDir, { recursive: true });
      await writeFile(join(configDir, "context-stash.json"), "{ invalid json — not parseable");

      const warned: string[] = [];
      const origWrite = process.stderr.write.bind(process.stderr);
      const prevEnv = process.env.AXIOM_CONFIG_UTILS_DEBUG;
      process.env.AXIOM_CONFIG_UTILS_DEBUG = "1";
      process.stderr.write = ((chunk: string | Uint8Array): boolean => {
        warned.push(typeof chunk === "string" ? chunk : new TextDecoder().decode(chunk));
        return true;
      }) as typeof process.stderr.write;

      let loaded: typeof DEFAULT_STASH_CONFIG;
      try {
        loaded = loadPluginConfig("context-stash", DEFAULT_STASH_CONFIG, tmpDir) as typeof DEFAULT_STASH_CONFIG;
      } finally {
        process.stderr.write = origWrite;
        if (prevEnv === undefined) delete process.env.AXIOM_CONFIG_UTILS_DEBUG;
        else process.env.AXIOM_CONFIG_UTILS_DEBUG = prevEnv;
      }

      expect(loaded.active_size_limit_kb).toBe(DEFAULT_STASH_CONFIG.active_size_limit_kb);
      expect(warned.some(m => m.includes("Failed to parse"))).toBe(true);
    } finally {
      await rm(tmpDir, { recursive: true, force: true });
    }
  });

  test("empty stash:{} in axiom.config.yaml does NOT trigger migration warning", async () => {
    // test-empty-stash-suppression: Object.keys guard prevents spurious warnings for empty sections
    // axiom:trace work_item=context-stash-config-adoption spec=specs/112-Plugin-Config-Management.md#8 plan=phase-5/task-5-2/test-empty-stash-suppression
    const tmpDir = mkdtempSync(join(tmpdir(), "stash-pcm-emptystash-"));
    try {
      const codeopsDir = join(tmpDir, ".axiom");
      await mkdir(codeopsDir, { recursive: true });
      await writeFile(join(codeopsDir, "axiom.config.yaml"), "stash: {}\n");
      // Deliberately no context-stash.json

      const warned: string[] = [];
      const origWarn = console.warn;
      console.warn = (...args: unknown[]) => { warned.push(String(args[0])); };
      try {
        await ContextStashPlugin({ directory: tmpDir, client: {} });
      } finally {
        console.warn = origWarn;
      }

      expect(warned.some(m => m.includes("MIGRATION REQUIRED"))).toBe(false);
    } finally {
      await rm(tmpDir, { recursive: true, force: true });
    }
  });

  test("four-layer priority: env > .local.json > .json > defaults (end-to-end)", async () => {
    // test-four-layer-priority: all four layers active simultaneously, priority verified by progressive removal
    // axiom:trace work_item=context-stash-config-adoption spec=specs/112-Plugin-Config-Management.md#3.1 plan=phase-5/task-5-2/test-four-layer-priority
    const tmpDir = mkdtempSync(join(tmpdir(), "stash-pcm-4layer-"));
    const envKey = "AXIOM_CONTEXT_STASH_ACTIVE_SIZE_LIMIT_KB";
    const prevEnv = process.env[envKey];
    try {
      const configDir = join(tmpDir, ".opencode", "config");
      await mkdir(configDir, { recursive: true });
      await writeFile(join(configDir, "context-stash.json"), JSON.stringify({ active_size_limit_kb: 100 }));
      await writeFile(join(configDir, "context-stash.local.json"), JSON.stringify({ active_size_limit_kb: 200 }));
      process.env[envKey] = "300";

      // All 4 layers: env (300) wins
      const withEnv = loadPluginConfig("context-stash", DEFAULT_STASH_CONFIG, tmpDir) as typeof DEFAULT_STASH_CONFIG;
      expect(withEnv.active_size_limit_kb).toBe(300);

      // Remove env: .local.json (200) wins
      delete process.env[envKey];
      const withLocal = loadPluginConfig("context-stash", DEFAULT_STASH_CONFIG, tmpDir) as typeof DEFAULT_STASH_CONFIG;
      expect(withLocal.active_size_limit_kb).toBe(200);

      // Remove .local.json: .json (100) wins
      await rm(join(configDir, "context-stash.local.json"));
      const withFile = loadPluginConfig("context-stash", DEFAULT_STASH_CONFIG, tmpDir) as typeof DEFAULT_STASH_CONFIG;
      expect(withFile.active_size_limit_kb).toBe(100);

      // Remove .json: defaults (2048) win
      await rm(join(configDir, "context-stash.json"));
      const withDefaults = loadPluginConfig("context-stash", DEFAULT_STASH_CONFIG, tmpDir) as typeof DEFAULT_STASH_CONFIG;
      expect(withDefaults.active_size_limit_kb).toBe(DEFAULT_STASH_CONFIG.active_size_limit_kb);
    } finally {
      if (prevEnv === undefined) delete process.env[envKey];
      else process.env[envKey] = prevEnv;
      await rm(tmpDir, { recursive: true, force: true });
    }
  });

  test("unparseable YAML in axiom.config.yaml suppresses migration warning (catch-and-ignore)", async () => {
    // test-unparseable-yaml: catch block silences YAML parse errors; no warning should fire
    // axiom:trace work_item=context-stash-config-adoption spec=specs/112-Plugin-Config-Management.md#8 plan=phase-5/task-5-2/test-unparseable-yaml
    const tmpDir = mkdtempSync(join(tmpdir(), "stash-pcm-badyaml-"));
    try {
      const codeopsDir = join(tmpDir, ".axiom");
      await mkdir(codeopsDir, { recursive: true });
      // Write genuinely invalid YAML that will fail to parse
      await writeFile(join(codeopsDir, "axiom.config.yaml"), "stash: {\n  invalid: yaml: : : {{{\n");
      // No context-stash.json

      const warned: string[] = [];
      const origWarn = console.warn;
      console.warn = (...args: unknown[]) => { warned.push(String(args[0])); };
      try {
        await ContextStashPlugin({ directory: tmpDir, client: {} });
      } finally {
        console.warn = origWarn;
      }

      expect(warned.some(m => m.includes("MIGRATION REQUIRED"))).toBe(false);
    } finally {
      await rm(tmpDir, { recursive: true, force: true });
    }
  });
});
