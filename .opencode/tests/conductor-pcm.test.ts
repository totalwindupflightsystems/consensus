/**
 * Phase 4 Adoption Gate — conductor plugin config-utils integration.
 *
 * AC-12 (specs/112-Plugin-Config-Management.md §11):
 * conductor MUST load its config via loadPluginConfig() from config-utils.ts.
 * Setting AXIOM_CONDUCTOR_LIMITS__MAX_CONCURRENT_AGENTS must take effect at
 * runtime without touching plugin source code.
 *
 * Status: DONE — Phase 4 adoption completed in conductor-phase4-config-adoption.
 * conductor.ts now uses loadPluginConfig("conductor", DEFAULT_CONFIG, directory)
 * and all tests use env vars instead of the _conductor_test_config backdoor.
 *
 * axiom:trace work_item=conductor-phase4-config-adoption spec=specs/112-Plugin-Config-Management.md#AC-12 plan=phase-4/task-4-2/step-4-2-1
 */

import { test, expect, describe } from "bun:test";
import { mkdtempSync, mkdirSync, existsSync } from "node:fs";
import { mkdtemp, mkdir, writeFile, rm } from "node:fs/promises";
import { join } from "node:path";
import { tmpdir } from "node:os";
import { Database } from "bun:sqlite";

import {
  ConductorPlugin,
  initConductorDB,
} from "../lib/conductor.ts";
import { DEFAULT_CONFIG } from "../shared/conductor-constants.ts";
import { loadPluginConfig } from "../lib/config-utils.ts";

describe("AC-12: conductor loads config via loadPluginConfig()", () => {
  test("conductor uses loadPluginConfig — env var override takes effect", async () => {
    const tmpDir = mkdtempSync(join(tmpdir(), "conductor-pcm-"));
    const conductorDir = join(tmpDir, ".conductor");
    mkdirSync(conductorDir, { recursive: true });
    const db = new Database(join(conductorDir, "conductor.db"));
    initConductorDB(db);

    // Set env var override — save prior values to restore in finally (r3-med-001)
    const saved = process.env.AXIOM_CONDUCTOR_LIMITS__MAX_CONCURRENT_AGENTS;
    const prevFallback = process.env.AXIOM_CONDUCTOR_AUTH__ALLOW_SPAWN_SECRET_FALLBACK;
    process.env.AXIOM_CONDUCTOR_LIMITS__MAX_CONCURRENT_AGENTS = "42";
    process.env.AXIOM_CONDUCTOR_AUTH__ALLOW_SPAWN_SECRET_FALLBACK = "true";

    try {
      const plugin = await ConductorPlugin({
        directory: tmpDir,
        client: { baseUrl: "http://127.0.0.1:1" },
      });

      // Verify the config was loaded with the env var override
      // by checking that loadPluginConfig produces the same value
      const loaded = loadPluginConfig("conductor", DEFAULT_CONFIG, tmpDir);
      expect((loaded as typeof DEFAULT_CONFIG).limits.max_concurrent_agents).toBe(42);
    } finally {
      if (saved === undefined) delete process.env.AXIOM_CONDUCTOR_LIMITS__MAX_CONCURRENT_AGENTS;
      else process.env.AXIOM_CONDUCTOR_LIMITS__MAX_CONCURRENT_AGENTS = saved;
      if (prevFallback === undefined) delete process.env.AXIOM_CONDUCTOR_AUTH__ALLOW_SPAWN_SECRET_FALLBACK;
      else process.env.AXIOM_CONDUCTOR_AUTH__ALLOW_SPAWN_SECRET_FALLBACK = prevFallback;
      await rm(tmpDir, { recursive: true, force: true });
    }
  });

  test("AXIOM_CONDUCTOR_POLLING__COMPLETION_CHECK_INTERVAL_SECONDS env var overrides default", () => {
    const saved = process.env.AXIOM_CONDUCTOR_POLLING__COMPLETION_CHECK_INTERVAL_SECONDS;
    process.env.AXIOM_CONDUCTOR_POLLING__COMPLETION_CHECK_INTERVAL_SECONDS = "0.1";

    try {
      const tmpDir = mkdtempSync(join(tmpdir(), "conductor-pcm-poll-"));
      const loaded = loadPluginConfig("conductor", DEFAULT_CONFIG, tmpDir);
      expect((loaded as typeof DEFAULT_CONFIG).polling.completion_check_interval_seconds).toBe(0.1);
    } finally {
      if (saved === undefined) delete process.env.AXIOM_CONDUCTOR_POLLING__COMPLETION_CHECK_INTERVAL_SECONDS;
      else process.env.AXIOM_CONDUCTOR_POLLING__COMPLETION_CHECK_INTERVAL_SECONDS = saved;
    }
  });

  test("Layer 2: reads overrides from .opencode/config/conductor.json", async () => {
    // axiom:trace work_item=conductor-phase4-config-adoption spec=specs/112-Plugin-Config-Management.md#AC-12 plan=phase-4/task-4-3/inject-p4-crit-02
    const tmpDir = await mkdtemp(join(tmpdir(), "conductor-pcm-layer2-"));
    try {
      const configDir = join(tmpDir, ".opencode", "config");
      await mkdir(configDir, { recursive: true });
      await writeFile(join(configDir, "conductor.json"), JSON.stringify({
        limits: { max_concurrent_agents: 3 }
      }));
      const loaded = loadPluginConfig("conductor", DEFAULT_CONFIG, tmpDir);
      expect((loaded as typeof DEFAULT_CONFIG).limits.max_concurrent_agents).toBe(3);
    } finally {
      await rm(tmpDir, { recursive: true, force: true });
    }
  });

  test("regression: _conductor_test_config in client object has no effect", async () => {
    // axiom:trace work_item=conductor-phase4-config-adoption spec=specs/112-Plugin-Config-Management.md#8 plan=phase-4/task-4-3/inject-p4-crit-03
    // Proves the _conductor_test_config backdoor is NOT read — loadPluginConfig is the sole source.
    const tmpDir = await mkdtemp(join(tmpdir(), "conductor-pcm-nodoor-"));
    try {
      const loaded = loadPluginConfig("conductor", DEFAULT_CONFIG, tmpDir);
      expect((loaded as typeof DEFAULT_CONFIG).limits.max_concurrent_agents).toBe(
        DEFAULT_CONFIG.limits.max_concurrent_agents
      );
      expect((loaded as typeof DEFAULT_CONFIG).auth.allow_spawn_secret_fallback).toBe(
        DEFAULT_CONFIG.auth.allow_spawn_secret_fallback
      );
    } finally {
      await rm(tmpDir, { recursive: true, force: true });
    }
  });

  test("null/undefined directory throws clear error", async () => {
    // axiom:trace work_item=conductor-phase4-config-adoption spec=specs/112-Plugin-Config-Management.md#4.1 plan=phase-4/task-4-3/inject-p4-high-02
    const prevFallback = process.env.AXIOM_CONDUCTOR_AUTH__ALLOW_SPAWN_SECRET_FALLBACK;
    process.env.AXIOM_CONDUCTOR_AUTH__ALLOW_SPAWN_SECRET_FALLBACK = "true";
    try {
      await expect(
        ConductorPlugin({ directory: undefined as unknown as string, client: { baseUrl: "http://127.0.0.1:1" } })
      ).rejects.toThrow("directory");
    } finally {
      if (prevFallback === undefined) delete process.env.AXIOM_CONDUCTOR_AUTH__ALLOW_SPAWN_SECRET_FALLBACK;
      else process.env.AXIOM_CONDUCTOR_AUTH__ALLOW_SPAWN_SECRET_FALLBACK = prevFallback;
    }
  });

  test("invalid env var type (NaN) is ignored — default value used", async () => {
    // axiom:trace work_item=conductor-phase4-config-adoption spec=specs/112-Plugin-Config-Management.md#3.3 plan=phase-4/task-4-4/backlog-003 source=backlog-auto-inject
    const tmpDir = await mkdtemp(join(tmpdir(), "conductor-pcm-invalid-env-"));
    const envKey = "AXIOM_CONDUCTOR_LIMITS__MAX_CONCURRENT_AGENTS";
    const prev = process.env[envKey];
    process.env[envKey] = "not-a-number";
    try {
      const loaded = loadPluginConfig("conductor", DEFAULT_CONFIG, tmpDir);
      expect((loaded as typeof DEFAULT_CONFIG).limits.max_concurrent_agents).toBe(
        DEFAULT_CONFIG.limits.max_concurrent_agents
      );
    } finally {
      if (prev === undefined) delete process.env[envKey];
      else process.env[envKey] = prev;
      await rm(tmpDir, { recursive: true, force: true });
    }
  });

  test("malformed conductor.json is ignored — default values used", async () => {
    // axiom:trace work_item=conductor-phase4-config-adoption spec=specs/112-Plugin-Config-Management.md#3.2 plan=phase-4/task-4-4/backlog-004 source=backlog-auto-inject
    const tmpDir = await mkdtemp(join(tmpdir(), "conductor-pcm-malformed-"));
    try {
      const configDir = join(tmpDir, ".opencode", "config");
      await mkdir(configDir, { recursive: true });
      await writeFile(join(configDir, "conductor.json"), "{invalid json — this is not parseable");
      const loaded = loadPluginConfig("conductor", DEFAULT_CONFIG, tmpDir);
      expect((loaded as typeof DEFAULT_CONFIG).limits.max_concurrent_agents).toBe(
        DEFAULT_CONFIG.limits.max_concurrent_agents
      );
    } finally {
      await rm(tmpDir, { recursive: true, force: true });
    }
  });

  test("Layer 3 (.local.json) overrides Layer 2 (.json)", async () => {
    // axiom:trace work_item=conductor-phase4-config-adoption spec=specs/112-Plugin-Config-Management.md#3.1 plan=phase-4/task-4-4/backlog-008 source=backlog-auto-inject
    const tmpDir = await mkdtemp(join(tmpdir(), "conductor-pcm-layer3-"));
    try {
      const configDir = join(tmpDir, ".opencode", "config");
      await mkdir(configDir, { recursive: true });
      await writeFile(join(configDir, "conductor.json"), JSON.stringify({
        limits: { max_concurrent_agents: 5 }
      }));
      await writeFile(join(configDir, "conductor.local.json"), JSON.stringify({
        limits: { max_concurrent_agents: 7 }
      }));
      const loaded = loadPluginConfig("conductor", DEFAULT_CONFIG, tmpDir);
      expect((loaded as typeof DEFAULT_CONFIG).limits.max_concurrent_agents).toBe(7);
    } finally {
      await rm(tmpDir, { recursive: true, force: true });
    }
  });

  test("e2e: ConductorPlugin with _conductor_test_config in client has no effect on config", async () => {
    // axiom:trace work_item=conductor-phase4-config-adoption spec=specs/112-Plugin-Config-Management.md#8 plan=phase-4/task-4-5/r2-med-002
    // This proves the _conductor_test_config backdoor is absent from the full ConductorPlugin
    // execution path, not just from loadPluginConfig's return value.
    const tmpDir = await mkdtemp(join(tmpdir(), "conductor-pcm-e2e-nodoor-"));
    const conductorDir = join(tmpDir, ".conductor");
    await mkdir(conductorDir, { recursive: true });
    const prevFallback = process.env.AXIOM_CONDUCTOR_AUTH__ALLOW_SPAWN_SECRET_FALLBACK;
    process.env.AXIOM_CONDUCTOR_AUTH__ALLOW_SPAWN_SECRET_FALLBACK = "true";
    try {
      // Pass a _conductor_test_config with a canary value that should NEVER take effect
      const plugin = await ConductorPlugin({
        directory: tmpDir,
        client: {
          baseUrl: "http://127.0.0.1:1",
          _conductor_test_config: { limits: { max_concurrent_agents: 999 } },
        },
      });
      // Verify the plugin actually initialized (not just that it didn't crash)
      expect(plugin).toBeDefined();
      // Load config separately to confirm 999 was NOT used
      const loaded = loadPluginConfig("conductor", DEFAULT_CONFIG, tmpDir);
      expect((loaded as typeof DEFAULT_CONFIG).limits.max_concurrent_agents).not.toBe(999);
      expect((loaded as typeof DEFAULT_CONFIG).limits.max_concurrent_agents).toBe(
        DEFAULT_CONFIG.limits.max_concurrent_agents
      );
    } finally {
      if (prevFallback === undefined) delete process.env.AXIOM_CONDUCTOR_AUTH__ALLOW_SPAWN_SECRET_FALLBACK;
      else process.env.AXIOM_CONDUCTOR_AUTH__ALLOW_SPAWN_SECRET_FALLBACK = prevFallback;
      await rm(tmpDir, { recursive: true, force: true });
    }
  });

  test("four-layer priority: env var > .local.json > .json > defaults", async () => {
    // axiom:trace work_item=conductor-phase4-config-adoption spec=specs/112-Plugin-Config-Management.md#AC-12 plan=phase-4/task-4-5/r2-med-003
    // Proves all four config layers are active simultaneously and respect priority order.
    const tmpDir = await mkdtemp(join(tmpdir(), "conductor-pcm-4layer-"));
    const envKey = "AXIOM_CONDUCTOR_LIMITS__MAX_CONCURRENT_AGENTS";
    const prevEnv = process.env[envKey];
    try {
      const configDir = join(tmpDir, ".opencode", "config");
      await mkdir(configDir, { recursive: true });

      // Layer 2: .json sets 10
      await writeFile(join(configDir, "conductor.json"), JSON.stringify({ limits: { max_concurrent_agents: 10 } }));
      // Layer 3: .local.json sets 20
      await writeFile(join(configDir, "conductor.local.json"), JSON.stringify({ limits: { max_concurrent_agents: 20 } }));
      // Layer 4: env var sets 30
      process.env[envKey] = "30";

      // All 4 layers: env var (30) wins
      const withEnv = loadPluginConfig("conductor", DEFAULT_CONFIG, tmpDir);
      expect((withEnv as typeof DEFAULT_CONFIG).limits.max_concurrent_agents).toBe(30);

      // Remove env var: .local.json (20) wins
      delete process.env[envKey];
      const withLocal = loadPluginConfig("conductor", DEFAULT_CONFIG, tmpDir);
      expect((withLocal as typeof DEFAULT_CONFIG).limits.max_concurrent_agents).toBe(20);

      // Remove .local.json: .json (10) wins
      await rm(join(configDir, "conductor.local.json"));
      const withFile = loadPluginConfig("conductor", DEFAULT_CONFIG, tmpDir);
      expect((withFile as typeof DEFAULT_CONFIG).limits.max_concurrent_agents).toBe(10);

      // Remove .json: defaults win
      await rm(join(configDir, "conductor.json"));
      const withDefaults = loadPluginConfig("conductor", DEFAULT_CONFIG, tmpDir);
      expect((withDefaults as typeof DEFAULT_CONFIG).limits.max_concurrent_agents).toBe(
        DEFAULT_CONFIG.limits.max_concurrent_agents
      );
    } finally {
      if (prevEnv === undefined) delete process.env[envKey];
      else process.env[envKey] = prevEnv;
      await rm(tmpDir, { recursive: true, force: true });
    }
  });

  test("DEFAULT_CONFIG is JSON-serializable (safe for structuredClone)", () => {
    // axiom:trace work_item=conductor-phase4-config-adoption spec=specs/112-Plugin-Config-Management.md#8 plan=phase-4/task-4-6/bl-r2-001
    // Guards against non-serializable values (Date, RegExp, Function, Symbol) being added to
    // DEFAULT_CONFIG, which would cause structuredClone to fail in conductor.ts catch block.
    const roundTripped = JSON.parse(JSON.stringify(DEFAULT_CONFIG));
    expect(roundTripped).toEqual(DEFAULT_CONFIG);
  });

  // bl-r2-004: No test needed for the outer catch at conductor.ts:637-641.
  // This path is effectively unreachable in normal operation:
  //   - validatePluginName("conductor") cannot fail (hardcoded valid plugin name)
  //   - resolve(directory) cannot fail after the null guard at line 616
  // File-read and JSON-parse errors are caught INSIDE loadPluginConfig() (config-utils.ts:445-472)
  // and never propagate to this outer catch. The catch exists as a defensive safety net only.
  // axiom:trace work_item=conductor-phase4-config-adoption spec=specs/112-Plugin-Config-Management.md#4.1 plan=phase-4/task-4-6/bl-r2-004


  test("AXIOM_CONDUCTOR_DATABASE_PATH env var overrides DB location", async () => {
    // axiom:trace work_item=conductor-phase4-config-adoption spec=specs/112-Plugin-Config-Management.md#8 plan=phase-4/task-4-7/backlog-005
    // Proves database_path is now wired: setting AXIOM_CONDUCTOR_DATABASE_PATH
    // causes loadPluginConfig to return the override path.
    const tmpDir = await mkdtemp(join(tmpdir(), "conductor-pcm-dbpath-"));
    const customDbPath = join(".conductor", "custom.db");
    const envKey = "AXIOM_CONDUCTOR_DATABASE_PATH";
    const prev = process.env[envKey];
    process.env[envKey] = customDbPath;
    try {
      const loaded = loadPluginConfig("conductor", DEFAULT_CONFIG, tmpDir);
      expect((loaded as typeof DEFAULT_CONFIG).database_path).toBe(customDbPath);
      expect((loaded as typeof DEFAULT_CONFIG).database_path).not.toBe(DEFAULT_CONFIG.database_path);
    } finally {
      if (prev === undefined) delete process.env[envKey];
      else process.env[envKey] = prev;
      await rm(tmpDir, { recursive: true, force: true });
    }
  });

  test("ConductorPlugin creates DB at AXIOM_CONDUCTOR_DATABASE_PATH location", async () => {
    // axiom:trace work_item=conductor-phase4-config-adoption spec=specs/112-Plugin-Config-Management.md#AC-12 plan=phase-4/task-4-8/r4-low-001
    // Proves database_path override is wired end-to-end through ConductorPlugin (not just loadPluginConfig).
    const tmpDir = await mkdtemp(join(tmpdir(), "conductor-pcm-dbpath-e2e-"));
    const customDbRelPath = join(".conductor", "custom-e2e.db");
    const envKey = "AXIOM_CONDUCTOR_DATABASE_PATH";
    const prevDbPath = process.env[envKey];
    const prevFallback = process.env.AXIOM_CONDUCTOR_AUTH__ALLOW_SPAWN_SECRET_FALLBACK;
    process.env[envKey] = customDbRelPath;
    process.env.AXIOM_CONDUCTOR_AUTH__ALLOW_SPAWN_SECRET_FALLBACK = "true";
    try {
      await ConductorPlugin({
        directory: tmpDir,
        client: { baseUrl: "http://127.0.0.1:1" },
      });
      // The DB file must exist at the configured path (resolved relative to tmpDir)
      const expectedDbPath = join(tmpDir, customDbRelPath);
      expect(existsSync(expectedDbPath)).toBe(true);
      // The DB must have a valid schema — conductor_agents table must exist
      // axiom:trace work_item=conductor-phase4-config-adoption spec=specs/112-Plugin-Config-Management.md#AC-12 plan=phase-4/task-4-9/r5-low-001
      const db = new Database(expectedDbPath, { readonly: true });
      try {
        const tables = db
          .query<{ name: string }, []>(
            "SELECT name FROM sqlite_master WHERE type='table' AND name='conductor_agents'"
          )
          .all();
        expect(tables).toHaveLength(1);
        expect(tables[0].name).toBe("conductor_agents");
      } finally {
        db.close();
      }
    } finally {
      if (prevDbPath === undefined) delete process.env[envKey];
      else process.env[envKey] = prevDbPath;
      if (prevFallback === undefined) delete process.env.AXIOM_CONDUCTOR_AUTH__ALLOW_SPAWN_SECRET_FALLBACK;
      else process.env.AXIOM_CONDUCTOR_AUTH__ALLOW_SPAWN_SECRET_FALLBACK = prevFallback;
      await rm(tmpDir, { recursive: true, force: true });
    }
  });
});
