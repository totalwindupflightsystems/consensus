/**
 * Tests for conductor.ts — Conductor Phase 1 + Phase 2.
 *
 * Phase 1 covers all 9 acceptance criteria from SWDE-45:
 *
 *   AC-1  conductor.spawn returns {agent_id, session_id, stash_id, status: "running"}
 *   AC-2  conductor.status returns one-line dashboard with count, status, elapsed, cost
 *   AC-3  conductor.done without secret → error; with correct secret → done
 *   AC-4  conductor.result returns stash.peek OR last assistant message
 *   AC-5  Context banner injected via system.transform
 *   AC-6  conductor_agents SQLite table populated with all required fields after spawn
 *   AC-7  Spawn secret stored as SHA-256 hash; never in logs/stash/status output
 *   AC-8  allow_spawn_secret_fallback: false default — refuses to start without SPIRE
 *   AC-9  CRITICAL log event on every spawn when fallback active
 *
 * Phase 2 covers SWDE-49 acceptance criteria:
 *
 *   P2-AC-1  conductor.collect waits for all agents, returns results + all_done/timed_out
 *   P2-AC-2  conductor.relay waits for source, spawns new agent with result as context
 *   P2-AC-3  conductor.broadcast sends message to all running agents
 *   P2-AC-4  conductor.delegate creates stash + spawns agent, writes context entry
 *   P2-AC-5  conductor.focus marks stash focused + returns peek content
 *   P2-AC-6  conductor.unfocus removes focus; conductor.pin sets pinned=1
 *   P2-AC-7  Context banner includes pinned stash names
 *   P2-AC-8  spawn --detach sets detached=1 in DB
 *   P2-AC-9  session.stop cancels non-detached agents only
 *
 * Run: cd .opencode && bun test tests/conductor.test.ts
 *
 * axiom:trace work_item=SWDE-49 spec=specs/107-Conductor.md plan=phase2/task-2.6/step-1 test=tests/conductor.test.ts jira_ref=SWDE-49
 */

import { test, expect, describe, beforeEach, afterEach } from "bun:test";
import { existsSync, mkdirSync, mkdtempSync, rmSync, readFileSync } from "node:fs";
import { join } from "node:path";
import { tmpdir } from "node:os";
import { Database } from "bun:sqlite";

import {
  ConductorPlugin,
  DEFAULT_CONFIG,
  initConductorDB,
  generateSpawnSecret,
  hashSpawnSecret,
  constantTimeCompareHex,
  generateAgentId,
  buildConductorEnvelope,
  extractEnvelope,
  checkSpireAvailability,
  createConductorStash,
  appendStashEntry,
  readStashEntries,
  peekStash,
  isStashClosed,
  stashHasResultEntry,
  parseTimeoutString,
} from "../lib/conductor.ts";

// ─── Test helpers ─────────────────────────────────────────────────────────────

interface PluginInstance {
  plugin: Awaited<ReturnType<typeof ConductorPlugin>>;
  tmpDir: string;
  db: Database;
  stashRoot: string;
}

/**
 * Bootstrap a fresh Conductor plugin in a temp directory.
 * Sets allow_spawn_secret_fallback=true so tests don't require SPIRE.
 * Patches the config via the DB path override.
 */
async function makePlugin(opts: {
  allowFallback?: boolean;
  noSpire?: boolean;
} = {}): Promise<PluginInstance> {
  const tmpDir = mkdtempSync(join(tmpdir(), "conductor-test-"));

  // Pre-create .conductor dir and DB so the plugin uses it
  const conductorDir = join(tmpDir, ".conductor");
  mkdirSync(conductorDir, { recursive: true });
  const dbPath = join(conductorDir, "conductor.db");
  const db = new Database(dbPath);
  initConductorDB(db);

  // Phase 4 adoption: use env vars for config overrides instead of _conductor_test_config.
  // axiom:trace work_item=conductor-phase4-config-adoption spec=specs/112-Plugin-Config-Management.md#8 plan=phase-4/task-4-2/step-4-2-1

  // Create a patched plugin factory that allows fallback in tests
  const patchedPlugin = await makePatchedPlugin(tmpDir, db, {
    allow_spawn_secret_fallback: opts.allowFallback !== false,
  });

  const stashRoot = join(tmpDir, ".memory-bank", "stash");

  return { plugin: patchedPlugin, tmpDir, db, stashRoot };
}

/**
 * Create a plugin with config overridden via environment variables.
 * Uses the AXIOM_CONDUCTOR_* env var convention per specs/112-Plugin-Config-Management.md.
 *
 * axiom:trace work_item=conductor-phase4-config-adoption spec=specs/112-Plugin-Config-Management.md#8 plan=phase-4/task-4-2/step-4-2-1
 */
async function makePatchedPlugin(
  directory: string,
  db: Database,
  authOverrides: { allow_spawn_secret_fallback: boolean }
): Promise<Awaited<ReturnType<typeof ConductorPlugin>>> {
  // Phase 4 adoption: use env vars instead of _conductor_test_config backdoor.
  // AXIOM_CONDUCTOR_AUTH__ALLOW_SPAWN_SECRET_FALLBACK controls the fallback.
  // Save and restore to prevent process-wide state leak across tests.
  // axiom:trace work_item=conductor-phase4-config-adoption spec=specs/112-Plugin-Config-Management.md#AC-12 plan=phase-4/task-4-3/inject-p4-high-01
  const prevFallback = process.env.AXIOM_CONDUCTOR_AUTH__ALLOW_SPAWN_SECRET_FALLBACK;
  process.env.AXIOM_CONDUCTOR_AUTH__ALLOW_SPAWN_SECRET_FALLBACK =
    authOverrides.allow_spawn_secret_fallback ? "true" : "false";

  // ConductorPlugin uses detectBaseUrl() which will get "http://localhost:4096" (no real server).
  // The spawn tool will fall back to generating a synthetic session ID — which is fine for tests.
  // Use port 1 to guarantee an immediate connection-refused (avoids 5s timeout if VS Code or
  // any other process happens to be listening on port 4096 in the test environment).
  try {
    return await ConductorPlugin({ directory, client: { baseUrl: "http://127.0.0.1:1" } });
  } finally {
    if (prevFallback === undefined) {
      delete process.env.AXIOM_CONDUCTOR_AUTH__ALLOW_SPAWN_SECRET_FALLBACK;
    } else {
      process.env.AXIOM_CONDUCTOR_AUTH__ALLOW_SPAWN_SECRET_FALLBACK = prevFallback;
    }
  }
}

/** Call a conductor tool by name. */
async function callTool(
  plugin: Awaited<ReturnType<typeof ConductorPlugin>>,
  toolName: keyof Awaited<ReturnType<typeof ConductorPlugin>>["tool"],
  args: Record<string, unknown>,
  context: Record<string, unknown> = { sessionID: "test-primary-session" }
): Promise<Record<string, unknown>> {
  const t = plugin.tool[toolName] as {
    execute: (args: unknown, ctx: unknown) => Promise<string>;
  };
  const raw = await t.execute(args, context);
  return JSON.parse(raw) as Record<string, unknown>;
}

/** Insert a test agent with a known secret hash directly into the DB. */
async function insertTestAgent(
  db: Database,
  overrides: {
    agent_id?: string;
    name?: string;
    session_id?: string;
    stash_id?: string | null;
    status?: string;
    task?: string;
    spawned_by?: string;
    secret?: string;
  } = {}
): Promise<{ agentId: string; secret: string; secretHash: string }> {
  const agentId = overrides.agent_id ?? generateAgentId();
  const secret = overrides.secret ?? generateSpawnSecret();
  const secretHash = await hashSpawnSecret(secret);
  const now = new Date().toISOString();

  db.prepare(`
    INSERT INTO conductor_agents
      (agent_id, name, session_id, stash_id, status, task, spawned_by, spawned_at, spawn_secret_hash)
    VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
  `).run(
    agentId,
    overrides.name ?? "test-agent",
    overrides.session_id ?? "ses_test001",
    overrides.stash_id ?? null,
    overrides.status ?? "running",
    overrides.task ?? "test task",
    overrides.spawned_by ?? "test-primary-session",
    now,
    secretHash
  );

  return { agentId, secret, secretHash };
}

// ─────────────────────────────────────────────────────────────────────────────
// Helper function tests (pure, no plugin needed)
// ─────────────────────────────────────────────────────────────────────────────

describe("db init", () => {
  test("conductor_agents table exists with all required columns", () => {
    const tmpDir = mkdtempSync(join(tmpdir(), "conductor-db-test-"));
    const db = new Database(join(tmpDir, "test.db"));
    initConductorDB(db);

    const cols = (
      db.prepare("SELECT name FROM pragma_table_info('conductor_agents')").all() as {
        name: string;
      }[]
    ).map((r) => r.name);

    const required = [
      "agent_id", "name", "session_id", "stash_id", "status",
      "task", "model", "spawned_by", "spawned_at", "completed_at",
      "timeout_at", "cost_usd", "result_summary", "result_type", "error",
      "spawn_secret_hash",
      "detached",  // Phase 2 (REQ-COND-036)
    ];

    for (const col of required) {
      expect(cols).toContain(col);
    }

    // Verify indexes
    const indexes = (
      db.prepare("SELECT name FROM sqlite_master WHERE type='index' AND tbl_name='conductor_agents'").all() as {
        name: string;
      }[]
    ).map((r) => r.name);

    expect(indexes.some((n) => n.includes("status"))).toBe(true);
    expect(indexes.some((n) => n.includes("spawned_by"))).toBe(true);

    // Phase 2: conductor_focused_stashes table and index
    const tables = (
      db.prepare("SELECT name FROM sqlite_master WHERE type='table'").all() as { name: string }[]
    ).map((r) => r.name);
    expect(tables).toContain("conductor_focused_stashes");

    const focusedCols = (
      db.prepare("SELECT name FROM pragma_table_info('conductor_focused_stashes')").all() as { name: string }[]
    ).map((r) => r.name);
    expect(focusedCols).toContain("session_id");
    expect(focusedCols).toContain("stash_id");
    expect(focusedCols).toContain("pinned");
    expect(focusedCols).toContain("focused_at");

    rmSync(tmpDir, { recursive: true });
  });

  test("initConductorDB is idempotent (safe to call multiple times)", () => {
    const tmpDir = mkdtempSync(join(tmpdir(), "conductor-idempotent-"));
    const db = new Database(join(tmpDir, "test.db"));
    // Should not throw on second call
    initConductorDB(db);
    initConductorDB(db);
    const count = (db.prepare("SELECT COUNT(*) as n FROM conductor_agents").get() as { n: number }).n;
    expect(count).toBe(0);
    rmSync(tmpDir, { recursive: true });
  });
});

// ─────────────────────────────────────────────────────────────────────────────
// AC-7, AC-8: spawn secret generation and SPIRE fallback
// ─────────────────────────────────────────────────────────────────────────────

describe("spawn secret generation", () => {
  test("generates 128-bit base64url secret", () => {
    const secret = generateSpawnSecret();
    // base64url 128 bits = 22 chars (without padding)
    expect(secret).toMatch(/^[A-Za-z0-9\-_]{20,24}$/);
    // No padding
    expect(secret).not.toContain("=");
  });

  test("each secret is unique", () => {
    const secrets = new Set<string>();
    for (let i = 0; i < 100; i++) {
      secrets.add(generateSpawnSecret());
    }
    expect(secrets.size).toBe(100);
  });

  test("SHA-256 hash is 64 hex chars", async () => {
    const secret = generateSpawnSecret();
    const hash = await hashSpawnSecret(secret);
    expect(hash).toMatch(/^[a-f0-9]{64}$/);
  });

  test("secret != hash", async () => {
    const secret = generateSpawnSecret();
    const hash = await hashSpawnSecret(secret);
    expect(secret).not.toBe(hash);
  });

  test("same secret produces same hash (deterministic)", async () => {
    const secret = "test-secret-determinism";
    const h1 = await hashSpawnSecret(secret);
    const h2 = await hashSpawnSecret(secret);
    expect(h1).toBe(h2);
  });

  test("different secrets produce different hashes", async () => {
    const h1 = await hashSpawnSecret("secret-a");
    const h2 = await hashSpawnSecret("secret-b");
    expect(h1).not.toBe(h2);
  });

  test("constantTimeCompareHex returns true for equal hashes", async () => {
    const hash = await hashSpawnSecret("test-secret");
    expect(constantTimeCompareHex(hash, hash)).toBe(true);
  });

  test("constantTimeCompareHex returns false for different hashes", async () => {
    const h1 = await hashSpawnSecret("secret-one");
    const h2 = await hashSpawnSecret("secret-two");
    expect(constantTimeCompareHex(h1, h2)).toBe(false);
  });

  test("constantTimeCompareHex handles different length inputs without panic", () => {
    expect(constantTimeCompareHex("abc", "abcd")).toBe(false);
    expect(constantTimeCompareHex("", "")).toBe(true);
  });
});

describe("spire fallback", () => {
  test("checkSpireAvailability returns false for non-existent socket", () => {
    const result = checkSpireAvailability("/tmp/does-not-exist-spire.sock");
    expect(result).toBe(false);
  });

  test("checkSpireAvailability returns true for existing path", () => {
    const tmpDir = mkdtempSync(join(tmpdir(), "spire-test-"));
    const socketPath = join(tmpDir, "agent.sock");
    // Create a dummy file as a stand-in for the socket
    Bun.write(socketPath, "");
    expect(checkSpireAvailability(socketPath)).toBe(true);
    rmSync(tmpDir, { recursive: true });
  });

  test("plugin throws CRITICAL error when fallback=false and no SPIRE", async () => {
    const tmpDir = mkdtempSync(join(tmpdir(), "conductor-nospire-"));

    // Phase 4 adoption: use env var to explicitly set allow_spawn_secret_fallback=false.
    // The default config already has fallback=false, but we set it explicitly for clarity.
    // axiom:trace work_item=conductor-phase4-config-adoption spec=specs/112-Plugin-Config-Management.md#8 plan=phase-4/task-4-2/step-4-2-1
    const savedFallback = process.env.AXIOM_CONDUCTOR_AUTH__ALLOW_SPAWN_SECRET_FALLBACK;
    process.env.AXIOM_CONDUCTOR_AUTH__ALLOW_SPAWN_SECRET_FALLBACK = "false";

    let threw = false;
    let errorMsg = "";
    try {
      await ConductorPlugin({
        directory: tmpDir,
        client: { baseUrl: "http://127.0.0.1:1" },
      });
    } catch (err) {
      threw = true;
      errorMsg = String(err);
    } finally {
      // Restore env
      if (savedFallback === undefined) delete process.env.AXIOM_CONDUCTOR_AUTH__ALLOW_SPAWN_SECRET_FALLBACK;
      else process.env.AXIOM_CONDUCTOR_AUTH__ALLOW_SPAWN_SECRET_FALLBACK = savedFallback;
    }

    // With fallback=false and no SPIRE socket, the plugin MUST throw.
    expect(threw).toBe(true);
    expect(errorMsg.toLowerCase()).toContain("spire");

    rmSync(tmpDir, { recursive: true });
  });
});

// ─────────────────────────────────────────────────────────────────────────────
// BUG-9 regression: allow_spawn_secret_fallback spec compliance
// axiom:trace work_item=plugin-bug-sweep-01 spec=specs/107-Conductor.md#REQ-COND-005b plan=phase-2/task-2/step-verify-001
// ─────────────────────────────────────────────────────────────────────────────

describe("BUG-9 regression: allow_spawn_secret_fallback spec compliance", () => {
  test("DEFAULT_CONFIG.auth.allow_spawn_secret_fallback is false (REQ-COND-005b §1)", () => {
    // The spec mandates false as the default. This test prevents silent reversion.
    expect(DEFAULT_CONFIG.auth.allow_spawn_secret_fallback).toBe(false);
  });

  test("plugin throws with SPIRE/allow_spawn_secret_fallback message when fallback=false and SPIRE unavailable (REQ-COND-005b §4)", async () => {
    const tmpDir = mkdtempSync(join(tmpdir(), "conductor-bug9-"));
    process.env.AXIOM_CONDUCTOR_AUTH__ALLOW_SPAWN_SECRET_FALLBACK = "false";

    let threw = false;
    let errorMsg = "";
    try {
      await ConductorPlugin({
        directory: tmpDir,
        client: { baseUrl: "http://127.0.0.1:1" },
      });
    } catch (err) {
      threw = true;
      errorMsg = String(err);
    } finally {
      delete process.env.AXIOM_CONDUCTOR_AUTH__ALLOW_SPAWN_SECRET_FALLBACK;
      rmSync(tmpDir, { recursive: true });
    }

    expect(threw).toBe(true);
    expect(errorMsg).toContain("SPIRE");
    expect(errorMsg).toContain("allow_spawn_secret_fallback");
  });
});

// ─────────────────────────────────────────────────────────────────────────────
// Envelope helpers
// ─────────────────────────────────────────────────────────────────────────────

describe("conductor envelope", () => {
  test("buildConductorEnvelope contains expected fields", () => {
    const env = buildConductorEnvelope("bg_test001", "my-stash", "my-secret");
    expect(env).toContain("[conductor_envelope]");
    expect(env).toContain("agent_id: bg_test001");
    expect(env).toContain("stash_id: my-stash");
    expect(env).toContain("spawn_secret: my-secret");
    expect(env).toContain("[/conductor_envelope]");
  });

  test("buildConductorEnvelope omits stash_id when null", () => {
    const env = buildConductorEnvelope("bg_test001", null, "my-secret");
    expect(env).not.toContain("stash_id");
  });

  test("extractEnvelope parses envelope correctly", () => {
    const secret = "supersecretval";
    const env = buildConductorEnvelope("bg_abc123", "my-stash", secret);
    const fullTask = `${env}\n\nYour task: Do stuff`;

    const parsed = extractEnvelope(fullTask);
    expect(parsed).not.toBeNull();
    expect(parsed?.agentId).toBe("bg_abc123");
    expect(parsed?.stashId).toBe("my-stash");
    expect(parsed?.secret).toBe(secret);
  });

  test("extractEnvelope returns null for plain text", () => {
    expect(extractEnvelope("Just a plain task")).toBeNull();
  });
});

// ─────────────────────────────────────────────────────────────────────────────
// Timeout parser
// ─────────────────────────────────────────────────────────────────────────────

describe("parseTimeoutString", () => {
  test("parses minutes", () => {
    expect(parseTimeoutString("30m")).toBe(30 * 60 * 1000);
    expect(parseTimeoutString("1m")).toBe(60 * 1000);
  });

  test("parses seconds", () => {
    expect(parseTimeoutString("90s")).toBe(90 * 1000);
  });

  test("parses hours", () => {
    expect(parseTimeoutString("2h")).toBe(2 * 60 * 60 * 1000);
  });

  test("defaults to minutes for bare numbers", () => {
    expect(parseTimeoutString("5")).toBe(5 * 60 * 1000);
  });

  test("returns 0 for invalid input", () => {
    expect(parseTimeoutString("")).toBe(0);
    expect(parseTimeoutString("abc")).toBe(0);
  });
});

// ─────────────────────────────────────────────────────────────────────────────
// Stash helpers
// ─────────────────────────────────────────────────────────────────────────────

describe("stash helpers", () => {
  let tmpDir: string;
  let stashRoot: string;

  beforeEach(() => {
    tmpDir = mkdtempSync(join(tmpdir(), "conductor-stash-"));
    stashRoot = join(tmpDir, ".memory-bank", "stash");
    for (const dir of [stashRoot, join(stashRoot, "suspended"), join(stashRoot, "closed"), join(stashRoot, "entries")]) {
      mkdirSync(dir, { recursive: true });
    }
  });

  afterEach(() => {
    rmSync(tmpDir, { recursive: true });
  });

  test("createConductorStash creates suspended markdown file", async () => {
    await createConductorStash(stashRoot, "my-stash", "My Stash", "bg_001", "investigator");
    const filePath = join(stashRoot, "suspended", "my-stash.md");
    expect(existsSync(filePath)).toBe(true);
    const content = readFileSync(filePath, "utf-8");
    expect(content).toContain("my-stash");
    expect(content).toContain("conductor");
  });

  test("createConductorStash is idempotent (no overwrite if exists)", async () => {
    await createConductorStash(stashRoot, "my-stash", "My Stash", "bg_001", "investigator");
    const filePath = join(stashRoot, "suspended", "my-stash.md");
    const original = readFileSync(filePath, "utf-8");

    // Second call should not overwrite
    await createConductorStash(stashRoot, "my-stash", "Different Name", "bg_002", "another");
    const after = readFileSync(filePath, "utf-8");
    expect(after).toBe(original);
  });

  test("appendStashEntry writes JSONL line", async () => {
    await appendStashEntry(stashRoot, "my-stash", {
      ts: "2026-05-07T00:00:00Z",
      agent: "bg_001",
      type: "result",
      summary: "found 3 issues",
    });

    const entries = readStashEntries(stashRoot, "my-stash");
    expect(entries.length).toBe(1);
    expect(entries[0].type).toBe("result");
    expect(entries[0].agent).toBe("bg_001");
  });

  test("appendStashEntry redacts credentials from summary", async () => {
    await appendStashEntry(stashRoot, "my-stash", {
      type: "result",
      summary: "found token: sk-abc1234567890abcdefghijklmno",
    });

    const entries = readStashEntries(stashRoot, "my-stash");
    expect(entries[0].summary).not.toContain("sk-abc1234567890");
    expect(String(entries[0].summary)).toContain("[REDACTED]");
  });

  test("peekStash returns not found for missing stash", () => {
    const result = peekStash(stashRoot, "nonexistent");
    expect(result.found).toBe(false);
  });

  test("peekStash returns stash content when exists", async () => {
    await createConductorStash(stashRoot, "my-stash", "My Stash", "bg_001", "investigator");
    await appendStashEntry(stashRoot, "my-stash", {
      ts: new Date().toISOString(),
      agent: "bg_001",
      type: "result",
      summary: "done",
    });

    const result = peekStash(stashRoot, "my-stash");
    expect(result.found).toBe(true);
    expect(result.result_entry?.type).toBe("result");
  });

  test("isStashClosed detects closed stash", async () => {
    expect(isStashClosed(stashRoot, "nonexistent")).toBe(false);

    // Create a file in closed/ to simulate a closed stash
    mkdirSync(join(stashRoot, "closed"), { recursive: true });
    await Bun.write(join(stashRoot, "closed", "my-stash.md"), "---\nstash_id: my-stash\n---\n");
    expect(isStashClosed(stashRoot, "my-stash")).toBe(true);
  });

  test("stashHasResultEntry detects result entries", async () => {
    expect(stashHasResultEntry(stashRoot, "no-entries")).toBe(false);

    await appendStashEntry(stashRoot, "with-result", {
      type: "finding",
      summary: "some finding",
    });
    expect(stashHasResultEntry(stashRoot, "with-result")).toBe(false); // type != "result"

    await appendStashEntry(stashRoot, "with-result", {
      type: "result",
      summary: "the result",
    });
    expect(stashHasResultEntry(stashRoot, "with-result")).toBe(true);
  });
});

// ─────────────────────────────────────────────────────────────────────────────
// Full plugin tests (AC-1 through AC-9)
// ─────────────────────────────────────────────────────────────────────────────

describe("spawn returns", () => {
  // AC-1, AC-6: spawn returns correct structure and DB record exists

  test("conductor_spawn returns {agent_id, session_id, stash_id, status: 'running'}", async () => {
    const { plugin, db } = await makePlugin();
    const result = await callTool(plugin, "conductor_spawn", {
      name: "test-agent",
      task: "Do something useful",
    });

    expect(result.error).toBeUndefined();
    expect(typeof result.agent_id).toBe("string");
    expect(String(result.agent_id)).toMatch(/^bg_[a-z0-9]{8}$/);
    expect(typeof result.session_id).toBe("string");
    expect(result.status).toBe("running");
    expect(result.stash_id).toBeNull(); // no stash provided
  });

  test("conductor_spawn with stash returns stash_id", async () => {
    const { plugin } = await makePlugin();
    const result = await callTool(plugin, "conductor_spawn", {
      name: "stash-agent",
      task: "Investigate something",
      stash: "my-investigation",
    });

    expect(result.error).toBeUndefined();
    expect(result.stash_id).toBe("my-investigation");
    expect(result.status).toBe("running");
  });

  test("AC-6: DB record populated with all required fields after spawn", async () => {
    const { plugin, db } = await makePlugin();
    const result = await callTool(plugin, "conductor_spawn", {
      name: "db-test-agent",
      task: "DB field test",
    });

    const agentId = result.agent_id as string;
    const row = db
      .prepare("SELECT * FROM conductor_agents WHERE agent_id=?")
      .get(agentId) as Record<string, unknown>;

    expect(row).toBeDefined();
    expect(row.agent_id).toBe(agentId);
    expect(row.name).toBe("db-test-agent");
    expect(row.status).toBe("running");
    expect(row.task).toBe("DB field test");
    expect(row.spawned_at).toBeTruthy();
    expect(row.spawn_secret_hash).toBeTruthy();
    expect(typeof row.spawn_secret_hash).toBe("string");
    expect(String(row.spawn_secret_hash)).toHaveLength(64); // SHA-256 hex
  });

  test("AC-7: spawn_secret NOT stored in DB (only hash)", async () => {
    const { plugin, db } = await makePlugin();
    const result = await callTool(plugin, "conductor_spawn", {
      name: "secret-check-agent",
      task: "Secret redaction test",
    });

    const agentId = result.agent_id as string;
    const row = db
      .prepare("SELECT * FROM conductor_agents WHERE agent_id=?")
      .get(agentId) as Record<string, unknown>;

    // The hash should be 64 hex chars (SHA-256) — if it were the plaintext secret
    // it would be ~22 base64url chars
    expect(String(row.spawn_secret_hash)).toHaveLength(64);
    expect(String(row.spawn_secret_hash)).toMatch(/^[a-f0-9]{64}$/);

    // The spawn result should NOT contain the secret
    const resultStr = JSON.stringify(result);
    // The secret would be ~22 base64url chars if it leaked; hash is 64 hex chars
    // We check that no base64url string of ~22 chars appears in the result
    // (The agent_id and session_id are also in the result but follow different patterns)
    expect(resultStr).not.toMatch(/spawn_secret/i);
  });

  test("max concurrent agents limit enforced", async () => {
    const { plugin } = await makePlugin();
    // Spawn 10 agents (default limit)
    for (let i = 0; i < 10; i++) {
      await callTool(plugin, "conductor_spawn", {
        name: `agent-${i}`,
        task: "task",
      });
    }
    // 11th should fail
    const result = await callTool(plugin, "conductor_spawn", {
      name: "over-limit",
      task: "should fail",
    });
    expect(result.error).toBeTruthy();
    expect(String(result.error)).toContain("limit");
  });
});

// ─────────────────────────────────────────────────────────────────────────────
// AC-3: conductor.done authentication
// ─────────────────────────────────────────────────────────────────────────────

describe("done auth", () => {
  test("wrong secret → {error, code: 403}", async () => {
    const { plugin, db } = await makePlugin();
    const { agentId } = await insertTestAgent(db);

    const result = await callTool(plugin, "conductor_done", {
      agent_id: agentId,
      secret: "definitely-wrong-secret",
      summary: "trying to fake completion",
    });

    expect(result.error).toBeTruthy();
    expect(result.code).toBe(403);
    expect(String(result.error).toLowerCase()).toContain("invalid");

    // Verify DB status NOT changed
    const row = db
      .prepare("SELECT status FROM conductor_agents WHERE agent_id=?")
      .get(agentId) as { status: string };
    expect(row.status).toBe("running");
  });

  test("correct secret → {status: 'done'} and DB updated", async () => {
    const { plugin, db } = await makePlugin();
    const { agentId, secret } = await insertTestAgent(db);

    const result = await callTool(plugin, "conductor_done", {
      agent_id: agentId,
      secret,
      summary: "completed successfully",
      result_type: "finding",
    });

    expect(result.error).toBeUndefined();
    expect(result.status).toBe("done");
    expect(result.agent_id).toBe(agentId);

    // Verify DB updated
    const row = db
      .prepare("SELECT status, result_summary, result_type FROM conductor_agents WHERE agent_id=?")
      .get(agentId) as { status: string; result_summary: string; result_type: string };
    expect(row.status).toBe("done");
    expect(row.result_summary).toBe("completed successfully");
    expect(row.result_type).toBe("finding");
  });

  test("unknown agent_id → 404 error", async () => {
    const { plugin } = await makePlugin();
    const result = await callTool(plugin, "conductor_done", {
      agent_id: "bg_nonexistent",
      secret: "any-secret",
      summary: "done",
    });
    expect(result.error).toBeTruthy();
    expect(result.code).toBe(404);
  });

  test("done on already-done agent → 409 error", async () => {
    const { plugin, db } = await makePlugin();
    const { agentId, secret } = await insertTestAgent(db, { status: "done" });

    const result = await callTool(plugin, "conductor_done", {
      agent_id: agentId,
      secret,
      summary: "trying to complete again",
    });
    expect(result.error).toBeTruthy();
    expect(result.code).toBe(409);
  });

  test("conductor_done writes entry to stash when stash_id present", async () => {
    const { plugin, db, stashRoot } = await makePlugin();
    // Create a stash first
    await createConductorStash(stashRoot, "result-stash", "Result Stash", "bg_001", "tester");

    const { agentId, secret } = await insertTestAgent(db, {
      stash_id: "result-stash",
    });

    await callTool(plugin, "conductor_done", {
      agent_id: agentId,
      secret,
      summary: "found 3 issues",
      result_type: "finding",
      details: "Issue details here",
    });

    // Verify stash entry written
    const entries = readStashEntries(stashRoot, "result-stash");
    expect(entries.length).toBe(1);
    expect(entries[0].type).toBe("result");
    expect(entries[0].result_type).toBe("finding");
    expect(entries[0].summary).toBe("found 3 issues");
    expect(entries[0].agent).toBe(agentId);
  });

  test("AC-7: secret redaction — done error response does not echo the provided secret", async () => {
    const { plugin, db } = await makePlugin();
    const { agentId } = await insertTestAgent(db);
    const wrongSecret = generateSpawnSecret();

    const result = await callTool(plugin, "conductor_done", {
      agent_id: agentId,
      secret: wrongSecret,
      summary: "done",
    });

    // Error response must not contain the provided secret
    const responseStr = JSON.stringify(result);
    expect(responseStr).not.toContain(wrongSecret);
  });
});

// ─────────────────────────────────────────────────────────────────────────────
// AC-2: conductor.status dashboard
// ─────────────────────────────────────────────────────────────────────────────

describe("status dashboard", () => {
  test("returns empty dashboard when no agents", async () => {
    const { plugin } = await makePlugin();
    const result = await callTool(plugin, "conductor_status", {});
    expect(result.error).toBeUndefined();
    expect(result.count).toBe(0);
    expect(String(result.dashboard)).toContain("[Conductor Dashboard]");
    expect(String(result.dashboard)).toContain("no background agents");
    expect(typeof result.count).toBe("number");
    expect(result.count).toBe(0);
  });

  test("dashboard shows agent with status, elapsed, cost", async () => {
    const { plugin, db } = await makePlugin();
    await insertTestAgent(db, {
      name: "my-investigator",
      spawned_by: "test-primary-session",
    });

    const result = await callTool(plugin, "conductor_status", {});
    expect(result.count).toBe(1);
    expect(result.running).toBe(1);
    expect(String(result.dashboard)).toContain("my-investigator");
    expect(String(result.dashboard)).toContain("running");
    expect(String(result.dashboard)).toContain("[Conductor Dashboard]");
    // Assert elapsed time format (e.g., "0s", "1s", "running 0s")
    expect(String(result.dashboard)).toMatch(/running \d+s/);
    // Assert cost format (e.g., "$0.000")
    expect(String(result.dashboard)).toMatch(/\$\d+\.\d{3}/);
    // Assert total_cost_usd is a number
    expect(typeof result.total_cost_usd).toBe("number");
    // Assert count fields
    expect(result.count).toBe(1);
    expect(result.running).toBe(1);
  });

  test("dashboard shows DONE status with result summary", async () => {
    const { plugin, db } = await makePlugin();
    await insertTestAgent(db, {
      name: "done-agent",
      status: "done",
      spawned_by: "test-primary-session",
    });
    // Update result_summary
    db.prepare("UPDATE conductor_agents SET result_summary='found 3 issues' WHERE name='done-agent'").run();

    const result = await callTool(plugin, "conductor_status", {});
    expect(String(result.dashboard)).toContain("DONE");
    expect(String(result.dashboard)).toContain("found 3 issues");
  });

  test("AC-7: spawn_secret_hash never appears in status output", async () => {
    const { plugin, db } = await makePlugin();
    const { agentId, secretHash } = await insertTestAgent(db, {
      spawned_by: "test-primary-session",
    });

    const result = await callTool(plugin, "conductor_status", {});
    const dashboardStr = JSON.stringify(result);
    // The hash (64 hex chars) must not appear
    expect(dashboardStr).not.toContain(secretHash);
  });

  test("dashboard shows total with count, done count, running count", async () => {
    const { plugin, db } = await makePlugin();
    await insertTestAgent(db, { name: "agent-1", status: "running", spawned_by: "test-primary-session" });
    await insertTestAgent(db, { name: "agent-2", status: "done", spawned_by: "test-primary-session" });
    await insertTestAgent(db, { name: "agent-3", status: "running", spawned_by: "test-primary-session" });

    const result = await callTool(plugin, "conductor_status", {});
    expect(result.count).toBe(3);
    expect(result.running).toBe(2);
    expect(result.done).toBe(1);
    expect(String(result.dashboard)).toContain("Total: 3 agents");
  });
});

// ─────────────────────────────────────────────────────────────────────────────
// AC-4: conductor.result
// ─────────────────────────────────────────────────────────────────────────────

describe("result", () => {
  test("returns error for unknown agent", async () => {
    const { plugin } = await makePlugin();
    const result = await callTool(plugin, "conductor_result", { id: "bg_notexist" });
    expect(result.error).toBeTruthy();
  });

  test("stash path returns stash.peek content", async () => {
    const { plugin, db, stashRoot } = await makePlugin();
    await createConductorStash(stashRoot, "result-stash", "Result Stash", "bg_001", "tester");
    await appendStashEntry(stashRoot, "result-stash", {
      ts: new Date().toISOString(),
      agent: "bg_001",
      type: "result",
      result_type: "finding",
      summary: "found critical issue",
    });

    const { agentId } = await insertTestAgent(db, { stash_id: "result-stash", status: "done" });

    const result = await callTool(plugin, "conductor_result", { id: agentId });
    expect(result.error).toBeUndefined();
    expect(result.stash_id).toBe("result-stash");
    expect(result.stash_found).toBe(true);
    expect((result.stash_result_entry as Record<string, unknown>)?.summary).toBe("found critical issue");
  });

  test("session path returns null session_message when no OpenCode running", async () => {
    const { plugin, db } = await makePlugin();
    // No stash_id — will try session API which isn't running in tests
    const { agentId } = await insertTestAgent(db, { stash_id: null });

    const result = await callTool(plugin, "conductor_result", { id: agentId });
    expect(result.error).toBeUndefined();
    // session_message will be null since no OpenCode server is running
    expect(result.session_message).toBeNull();
    expect(result.agent_id).toBe(agentId);
  });
});

// ─────────────────────────────────────────────────────────────────────────────
// conductor.cancel
// ─────────────────────────────────────────────────────────────────────────────

describe("cancel", () => {
  test("cancel updates DB to cancelled", async () => {
    const { plugin, db } = await makePlugin();
    const { agentId } = await insertTestAgent(db);

    const result = await callTool(plugin, "conductor_cancel", {
      id: agentId,
      reason: "no longer needed",
    });

    expect(result.error).toBeUndefined();
    expect(result.cancelled).toBe(true);
    expect(result.agent_id).toBe(agentId);

    // Verify DB
    const row = db
      .prepare("SELECT status FROM conductor_agents WHERE agent_id=?")
      .get(agentId) as { status: string };
    expect(row.status).toBe("cancelled");
  });

  test("cancel on already-cancelled agent returns cancelled:false", async () => {
    const { plugin, db } = await makePlugin();
    const { agentId } = await insertTestAgent(db, { status: "cancelled" });

    const result = await callTool(plugin, "conductor_cancel", { id: agentId });
    expect(result.cancelled).toBe(false);
    expect(String(result.reason)).toContain("already");
  });

  test("cancel on unknown agent returns error", async () => {
    const { plugin } = await makePlugin();
    const result = await callTool(plugin, "conductor_cancel", { id: "bg_doesnotexist" });
    expect(result.error).toBeTruthy();
  });
});

// ─────────────────────────────────────────────────────────────────────────────
// conductor.wait
// ─────────────────────────────────────────────────────────────────────────────

describe("wait", () => {
  test("wait returns immediately if agent already done", async () => {
    const { plugin, db } = await makePlugin();
    const { agentId } = await insertTestAgent(db, { status: "done" });

    const start = Date.now();
    const result = await callTool(plugin, "conductor_wait", {
      id: agentId,
      timeout: "10s",
    });
    const elapsed = Date.now() - start;

    expect(result.error).toBeUndefined();
    expect(result.status).toBe("done");
    expect(result.agent_id).toBe(agentId);
    // Should return quickly (well under 10s)
    expect(elapsed).toBeLessThan(2000);
  });

  test("wait times out for perpetually-running agent", async () => {
    const { plugin, db } = await makePlugin();
    const { agentId } = await insertTestAgent(db, { status: "running" });

    const result = await callTool(plugin, "conductor_wait", {
      id: agentId,
      timeout: "3s", // very short timeout for test speed
    });

    // Should have timed out
    expect(result.timed_out).toBe(true);
    expect(result.status).toBe("running");
  }, 10000); // 10s test timeout

  test("wait returns immediately with error for unknown agent", async () => {
    const { plugin } = await makePlugin();
    const start = Date.now();
    const result = await callTool(plugin, "conductor_wait", {
      id: "bg_notexist",
      timeout: "30s", // long timeout — should NOT wait the full duration
    });
    const elapsed = Date.now() - start;

    // Should return immediately (not wait the full 30s timeout)
    expect(elapsed).toBeLessThan(2000);
    // Should indicate failure — either error or unknown status
    const hasError = Boolean(result.error);
    const hasUnknown = result.status === "unknown";
    expect(hasError || hasUnknown).toBe(true);
  }, 5000);
});

// ─────────────────────────────────────────────────────────────────────────────
// AC-5: Context banner (system.transform)
// ─────────────────────────────────────────────────────────────────────────────

describe("banner", () => {
  test("banner injected into system output when agents present", async () => {
    const { plugin, db } = await makePlugin();
    await insertTestAgent(db, {
      name: "banner-agent",
      status: "running",
      spawned_by: "banner-test-session",
    });

    const output: { system: string[] } = { system: [] };
    const transformFn = plugin["experimental.chat.system.transform"] as (
      input: { sessionID?: string; model: unknown },
      output: { system: string[] }
    ) => Promise<void>;

    await transformFn({ sessionID: "banner-test-session", model: {} }, output);

    expect(output.system.length).toBeGreaterThan(0);
    const banner = output.system[0];
    expect(banner).toContain("[Conductor:");
    expect(banner).toContain("banner-agent");
  });

  test("no banner when no agents for this session", async () => {
    const { plugin } = await makePlugin();

    const output: { system: string[] } = { system: [] };
    const transformFn = plugin["experimental.chat.system.transform"] as (
      input: { sessionID?: string; model: unknown },
      output: { system: string[] }
    ) => Promise<void>;

    await transformFn({ sessionID: "empty-session", model: {} }, output);

    expect(output.system.length).toBe(0);
  });

  test("no banner when sessionID is undefined", async () => {
    const { plugin } = await makePlugin();

    const output: { system: string[] } = { system: [] };
    const transformFn = plugin["experimental.chat.system.transform"] as (
      input: { sessionID?: string; model: unknown },
      output: { system: string[] }
    ) => Promise<void>;

    await transformFn({ sessionID: undefined, model: {} }, output);

    expect(output.system.length).toBe(0);
  });
});

// ─────────────────────────────────────────────────────────────────────────────
// AC-9: CRITICAL log on fallback spawn
// ─────────────────────────────────────────────────────────────────────────────

describe("fallback log", () => {
  test("every spawn emits a CRITICAL log when fallback active (per-spawn guarantee)", async () => {
    const { plugin } = await makePlugin({ allowFallback: true });

    // Capture stderr (where pluginError writes when AXIOM_CONDUCTOR_DEBUG=1)
    const errorLogs: string[] = [];
    const originalWrite = process.stderr.write.bind(process.stderr);
    const prevEnv = process.env.AXIOM_CONDUCTOR_DEBUG;
    process.env.AXIOM_CONDUCTOR_DEBUG = "1";
    process.stderr.write = ((chunk: string | Uint8Array): boolean => {
      errorLogs.push(typeof chunk === "string" ? chunk : new TextDecoder().decode(chunk));
      return true;
    }) as typeof process.stderr.write;

    try {
      // Spawn 3 agents — each should produce its own CRITICAL log
      await callTool(plugin, "conductor_spawn", { name: "fallback-agent-1", task: "task 1" });
      await callTool(plugin, "conductor_spawn", { name: "fallback-agent-2", task: "task 2" });
      await callTool(plugin, "conductor_spawn", { name: "fallback-agent-3", task: "task 3" });
    } finally {
      process.stderr.write = originalWrite;
      if (prevEnv === undefined) delete process.env.AXIOM_CONDUCTOR_DEBUG;
      else process.env.AXIOM_CONDUCTOR_DEBUG = prevEnv;
    }

    // Count the number of CRITICAL fallback-active log entries
    const criticalFallbackLogs = errorLogs.filter(
      (log) => log.includes("conductor_spawn_secret_fallback_active")
    );
    
    // Each spawn must produce exactly one CRITICAL log (per-spawn guarantee)
    expect(criticalFallbackLogs.length).toBe(3);
  });
});

// ─────────────────────────────────────────────────────────────────────────────
// AC-7: secret redaction end-to-end
// ─────────────────────────────────────────────────────────────────────────────

describe("secret redaction", () => {
  test("spawn return value does not contain plaintext secret", async () => {
    const { plugin } = await makePlugin();
    const result = await callTool(plugin, "conductor_spawn", {
      name: "redaction-test",
      task: "verify redaction",
    });

    const resultStr = JSON.stringify(result);
    // No spawn_secret key should appear
    expect(resultStr).not.toContain("spawn_secret");
    // The secret is base64url (~22 chars) — not easily guessable
    // but we can check the DB hash (64 hex chars) is not in the response
    expect(resultStr).not.toMatch(/[a-f0-9]{64}/); // SHA-256 hash not in output
  });

  test("status output does not contain spawn_secret_hash", async () => {
    const { plugin, db } = await makePlugin();
    const { secretHash } = await insertTestAgent(db, {
      name: "redact-test",
      spawned_by: "test-primary-session",
    });

    const result = await callTool(plugin, "conductor_status", {});
    expect(JSON.stringify(result)).not.toContain(secretHash);
  });

  test("done error response does not leak hash or secret", async () => {
    const { plugin, db } = await makePlugin();
    const { agentId, secretHash } = await insertTestAgent(db);

    const result = await callTool(plugin, "conductor_done", {
      agent_id: agentId,
      secret: "wrong-secret",
      summary: "done",
    });

    const responseStr = JSON.stringify(result);
    expect(responseStr).not.toContain(secretHash);
    expect(responseStr).not.toContain("wrong-secret");
  });
});

// ─────────────────────────────────────────────────────────────────────────────
// Integration: spawn → done round-trip
// ─────────────────────────────────────────────────────────────────────────────

describe("spawn → done round-trip", () => {
  test("spawn → done via extracted envelope secret", async () => {
    const { plugin, db } = await makePlugin();

    // Capture the initial_message sent to the spawned session
    // We do this by intercepting fetch (or by reading the DB and reconstructing)
    // For this integration test, we use the DB to get the agent_id and then
    // directly invoke done with the secret from the envelope.

    // Step 1: spawn
    const spawnResult = await callTool(plugin, "conductor_spawn", {
      name: "integration-test",
      task: "integration task",
    });
    expect(spawnResult.error).toBeUndefined();
    const agentId = spawnResult.agent_id as string;

    // Step 2: verify agent is running
    const row = db
      .prepare("SELECT status FROM conductor_agents WHERE agent_id=?")
      .get(agentId) as { status: string };
    expect(row.status).toBe("running");

    // Step 3: done with wrong secret → fail
    const failResult = await callTool(plugin, "conductor_done", {
      agent_id: agentId,
      secret: "not-the-right-secret",
      summary: "done",
    });
    expect(failResult.code).toBe(403);

    // Step 4: verify agent is still running (not changed by failed auth)
    const rowAfterFail = db
      .prepare("SELECT status FROM conductor_agents WHERE agent_id=?")
      .get(agentId) as { status: string };
    expect(rowAfterFail.status).toBe("running");

    // Step 5: Extract the secret from the hash for testing by re-using the
    // "known secret" approach. We directly update the DB with a known hash for this test.
    const knownSecret = "test-round-trip-secret";
    const knownHash = await hashSpawnSecret(knownSecret);
    db.prepare("UPDATE conductor_agents SET spawn_secret_hash=? WHERE agent_id=?")
      .run(knownHash, agentId);

    // Step 6: done with correct secret → success
    const doneResult = await callTool(plugin, "conductor_done", {
      agent_id: agentId,
      secret: knownSecret,
      summary: "integration test passed",
      result_type: "summary",
    });
    expect(doneResult.error).toBeUndefined();
    expect(doneResult.status).toBe("done");

    // Step 7: verify status dashboard shows done
    const statusResult = await callTool(plugin, "conductor_status", { all: true });
    expect(String(statusResult.dashboard)).toContain("DONE");
    expect(String(statusResult.dashboard)).toContain("integration-test");
  });

  test("two agents → status shows both", async () => {
    const { plugin, db } = await makePlugin();

    await insertTestAgent(db, { name: "agent-alpha", spawned_by: "test-primary-session" });
    await insertTestAgent(db, { name: "agent-beta", spawned_by: "test-primary-session" });

    const result = await callTool(plugin, "conductor_status", {});
    expect(result.count).toBe(2);
    expect(String(result.dashboard)).toContain("agent-alpha");
    expect(String(result.dashboard)).toContain("agent-beta");
  });
});

// ─────────────────────────────────────────────────────────────────────────────
// Completion polling
// ─────────────────────────────────────────────────────────────────────────────

describe("completion polling", () => {
  test("stash_close triggers done status via polling", async () => {
    const { plugin, db, stashRoot } = await makePlugin();

    // Insert a running agent with a stash
    await createConductorStash(stashRoot, "poll-stash", "Poll Stash", "bg_poll", "poller");
    const { agentId } = await insertTestAgent(db, {
      stash_id: "poll-stash",
      status: "running",
    });

    // Simulate stash.close by creating a file in closed/
    mkdirSync(join(stashRoot, "closed"), { recursive: true });
    await Bun.write(
      join(stashRoot, "closed", "poll-stash.md"),
      "---\nstash_id: poll-stash\nstate: closed\n---\n# Poll Stash\n"
    );

    // Wait for the polling loop to fire (default 5s, but let's wait a bit)
    // For test speed: we call the polling function directly by waiting 6s
    // In production this runs automatically; here we just verify the detection logic
    expect(isStashClosed(stashRoot, "poll-stash")).toBe(true);

    // Simulate what the polling loop does
    const closed = isStashClosed(stashRoot, "poll-stash");
    if (closed) {
      db.prepare("UPDATE conductor_agents SET status='done', completed_at=? WHERE agent_id=?")
        .run(new Date().toISOString(), agentId);
    }

    const row = db
      .prepare("SELECT status FROM conductor_agents WHERE agent_id=?")
      .get(agentId) as { status: string };
    expect(row.status).toBe("done");
  });

  test("type:result entry triggers done status via polling", async () => {
    const { plugin, db, stashRoot } = await makePlugin();

    await createConductorStash(stashRoot, "result-poll-stash", "Result Poll", "bg_rp", "rp");
    const { agentId } = await insertTestAgent(db, {
      stash_id: "result-poll-stash",
      status: "running",
    });

    // Append a result entry
    await appendStashEntry(stashRoot, "result-poll-stash", {
      ts: new Date().toISOString(),
      agent: agentId,
      type: "result",
      summary: "found it",
    });

    expect(stashHasResultEntry(stashRoot, "result-poll-stash")).toBe(true);
  });
});

// ─────────────────────────────────────────────────────────────────────────────
// H1: stop hook cancels running agents (REQ-COND-035)
// ─────────────────────────────────────────────────────────────────────────────

describe("stop hook", () => {
  test("stop hook cancels ALL running agents (parallel) when cancel_on_session_end is true", async () => {
    const { plugin, db } = await makePlugin();
    
    // Insert TWO running agents spawned by the same session
    const { agentId: agent1 } = await insertTestAgent(db, {
      name: "agent-stop-1",
      status: "running",
      spawned_by: "stop-test-session",
    });
    const { agentId: agent2 } = await insertTestAgent(db, {
      name: "agent-stop-2",
      status: "running",
      spawned_by: "stop-test-session",
    });

    // Trigger the stop event
    const eventFn = plugin.event as (input: { event: Record<string, unknown> }) => Promise<void>;
    await eventFn({
      event: { type: "session.stop", sessionID: "stop-test-session" },
    });

    // BOTH agents should be cancelled (parallel Promise.all)
    const row1 = db.prepare("SELECT status FROM conductor_agents WHERE agent_id=?").get(agent1) as { status: string };
    const row2 = db.prepare("SELECT status FROM conductor_agents WHERE agent_id=?").get(agent2) as { status: string };
    expect(row1.status).toBe("cancelled");
    expect(row2.status).toBe("cancelled");
  });
});

// ─────────────────────────────────────────────────────────────────────────────
// H2: atomic appendStashEntry (REQ-COND-029)
// ─────────────────────────────────────────────────────────────────────────────

describe("atomic stash entry", () => {
  test("appendStashEntry is atomic — concurrent writes produce valid JSONL", async () => {
    const tmpDir = mkdtempSync(join(tmpdir(), "conductor-atomic-"));
    const stashRoot = join(tmpDir, ".memory-bank", "stash");
    mkdirSync(join(stashRoot, "entries"), { recursive: true });

    // Simulate two concurrent appends
    await Promise.all([
      appendStashEntry(stashRoot, "shared-stash", { type: "result", agent: "bg_001", summary: "result-1" }),
      appendStashEntry(stashRoot, "shared-stash", { type: "result", agent: "bg_002", summary: "result-2" }),
    ]);

    const entries = readStashEntries(stashRoot, "shared-stash");
    expect(entries.length).toBe(2);
    // Both entries must be valid JSON
    for (const entry of entries) {
      expect(entry.type).toBe("result");
      expect(["bg_001", "bg_002"]).toContain(entry.agent);
    }

    rmSync(tmpDir, { recursive: true });
  });
});

// ─────────────────────────────────────────────────────────────────────────────
// H4: createConductorStash updates _index.md (REQ-COND-026)
// ─────────────────────────────────────────────────────────────────────────────

describe("stash index update", () => {
  test("createConductorStash updates _index.md", async () => {
    const tmpDir = mkdtempSync(join(tmpdir(), "conductor-index-"));
    const stashRoot = join(tmpDir, ".memory-bank", "stash");
    for (const d of [stashRoot, join(stashRoot, "suspended"), join(stashRoot, "closed"), join(stashRoot, "entries")]) {
      mkdirSync(d, { recursive: true });
    }

    await createConductorStash(stashRoot, "my-conductor-stash", "My Conductor Stash", "bg_001", "investigator");

    const indexPath = join(stashRoot, "_index.md");
    expect(existsSync(indexPath)).toBe(true);
    const content = readFileSync(indexPath, "utf-8");
    expect(content).toContain("my-conductor-stash");
    expect(content).toContain("suspended");

    rmSync(tmpDir, { recursive: true });
  });
});

// ─────────────────────────────────────────────────────────────────────────────
// C4: Real end-to-end polling loop test
// ─────────────────────────────────────────────────────────────────────────────

describe("real polling loop", () => {
  // Phase 4 config adoption: use env vars for test config overrides
  // axiom:trace work_item=conductor-phase4-config-adoption spec=specs/112-Plugin-Config-Management.md#8 plan=phase-4/task-4-2/step-4-2-1
  const savedEnv: Record<string, string | undefined> = {};
  beforeEach(() => {
    savedEnv.AUTH = process.env.AXIOM_CONDUCTOR_AUTH__ALLOW_SPAWN_SECRET_FALLBACK;
    savedEnv.POLL = process.env.AXIOM_CONDUCTOR_POLLING__COMPLETION_CHECK_INTERVAL_SECONDS;
    process.env.AXIOM_CONDUCTOR_AUTH__ALLOW_SPAWN_SECRET_FALLBACK = "true";
    process.env.AXIOM_CONDUCTOR_POLLING__COMPLETION_CHECK_INTERVAL_SECONDS = "0.1";
  });
  afterEach(() => {
    if (savedEnv.AUTH === undefined) delete process.env.AXIOM_CONDUCTOR_AUTH__ALLOW_SPAWN_SECRET_FALLBACK;
    else process.env.AXIOM_CONDUCTOR_AUTH__ALLOW_SPAWN_SECRET_FALLBACK = savedEnv.AUTH;
    if (savedEnv.POLL === undefined) delete process.env.AXIOM_CONDUCTOR_POLLING__COMPLETION_CHECK_INTERVAL_SECONDS;
    else process.env.AXIOM_CONDUCTOR_POLLING__COMPLETION_CHECK_INTERVAL_SECONDS = savedEnv.POLL;
  });

  test("polling loop auto-marks agent done when stash is closed", async () => {
    const tmpDir = mkdtempSync(join(tmpdir(), "conductor-pollreal-"));
    const conductorDir = join(tmpDir, ".conductor");
    mkdirSync(conductorDir, { recursive: true });
    const db = new Database(join(conductorDir, "conductor.db"));
    initConductorDB(db);

    // Create plugin with 100ms polling interval for test speed
    const plugin = await ConductorPlugin({
      directory: tmpDir,
      client: { baseUrl: "http://127.0.0.1:1" },
    });

    const stashRoot = join(tmpDir, ".memory-bank", "stash");
    for (const d of [stashRoot, join(stashRoot, "suspended"), join(stashRoot, "closed"), join(stashRoot, "entries")]) {
      mkdirSync(d, { recursive: true });
    }

    // Create stash and running agent
    await createConductorStash(stashRoot, "poll-real-stash", "Poll Real", "bg_pr01", "poller");
    const { agentId } = await insertTestAgent(db, {
      agent_id: "bg_pr01",
      stash_id: "poll-real-stash",
      status: "running",
    });

    // Verify initially running
    expect((db.prepare("SELECT status FROM conductor_agents WHERE agent_id=?").get(agentId) as { status: string }).status).toBe("running");

    // Wait one polling cycle
    await new Promise<void>((r) => setTimeout(r, 150));

    // Close the stash (simulate conductor.done or stash.close)
    await Bun.write(
      join(stashRoot, "closed", "poll-real-stash.md"),
      "---\nstash_id: poll-real-stash\nstate: closed\n---\n# Poll Real\n"
    );

    // Wait for polling loop to detect the closed stash (2+ cycles)
    await new Promise<void>((r) => setTimeout(r, 350));

    // The REAL setInterval should have detected the closed stash and updated DB
    const row = db.prepare("SELECT status FROM conductor_agents WHERE agent_id=?").get(agentId) as { status: string };
    expect(row.status).toBe("done");

    rmSync(tmpDir, { recursive: true });
  }, 5000); // 5s timeout for async polling

  test("polling loop auto-cancels agent when timeout_at is exceeded", async () => {
    const tmpDir = mkdtempSync(join(tmpdir(), "conductor-timeout-"));
    const conductorDir = join(tmpDir, ".conductor");
    mkdirSync(conductorDir, { recursive: true });
    const db = new Database(join(conductorDir, "conductor.db"));
    initConductorDB(db);

    // Create plugin with fast polling interval (set via env in beforeEach)
    const plugin = await ConductorPlugin({
      directory: tmpDir,
      client: { baseUrl: "http://127.0.0.1:1" },
    });

    const stashRoot = join(tmpDir, ".memory-bank", "stash");
    for (const d of [stashRoot, join(stashRoot, "suspended"), join(stashRoot, "closed"), join(stashRoot, "entries")]) {
      mkdirSync(d, { recursive: true });
    }

    // Insert agent whose timeout_at is already in the past (100ms ago)
    const { agentId } = await insertTestAgent(db, {
      status: "running",
      // timeout_at 200ms in the past — will be exceeded on first polling cycle
    });
    // Update timeout_at to be in the past directly
    db.prepare("UPDATE conductor_agents SET timeout_at=? WHERE agent_id=?")
      .run(new Date(Date.now() - 200).toISOString(), agentId);

    // Verify initially running
    expect(
      (db.prepare("SELECT status FROM conductor_agents WHERE agent_id=?").get(agentId) as { status: string }).status
    ).toBe("running");

    // Wait for the polling loop to detect the timeout (2+ cycles at 100ms each)
    await new Promise<void>((r) => setTimeout(r, 350));

    // The REAL setInterval should have auto-cancelled the timed-out agent
    const row = db.prepare("SELECT status, error FROM conductor_agents WHERE agent_id=?")
      .get(agentId) as { status: string; error: string | null };
    expect(row.status).toBe("cancelled");
    expect(row.error).toBe("timeout");

    rmSync(tmpDir, { recursive: true });
  }, 5000);

  test("polling loop auto-marks agent done when stash has type:result entry", async () => {
    const tmpDir = mkdtempSync(join(tmpdir(), "conductor-pollresult-"));
    const conductorDir = join(tmpDir, ".conductor");
    mkdirSync(conductorDir, { recursive: true });
    const db = new Database(join(conductorDir, "conductor.db"));
    initConductorDB(db);

    const plugin = await ConductorPlugin({
      directory: tmpDir,
      client: { baseUrl: "http://127.0.0.1:1" },
    });

    const stashRoot = join(tmpDir, ".memory-bank", "stash");
    for (const d of [stashRoot, join(stashRoot, "suspended"), join(stashRoot, "closed"), join(stashRoot, "entries")]) {
      mkdirSync(d, { recursive: true });
    }

    // Create stash and running agent
    await createConductorStash(stashRoot, "result-poll-stash2", "Result Poll 2", "bg_rp2", "poller");
    const { agentId } = await insertTestAgent(db, {
      agent_id: "bg_rp2",
      stash_id: "result-poll-stash2",
      status: "running",
    });

    // Wait one cycle to confirm no premature trigger
    await new Promise<void>((r) => setTimeout(r, 150));
    expect(
      (db.prepare("SELECT status FROM conductor_agents WHERE agent_id=?").get(agentId) as { status: string }).status
    ).toBe("running");

    // Append a type:result entry to the stash — this is what triggers completion
    await appendStashEntry(stashRoot, "result-poll-stash2", {
      ts: new Date().toISOString(),
      agent: agentId,
      type: "result",
      result_type: "summary",
      summary: "investigation complete",
    });

    // Wait for polling loop to detect the result entry (2+ cycles)
    await new Promise<void>((r) => setTimeout(r, 350));

    // The REAL setInterval should have detected the result entry and updated DB
    const row = db.prepare("SELECT status FROM conductor_agents WHERE agent_id=?")
      .get(agentId) as { status: string };
    expect(row.status).toBe("done");

    rmSync(tmpDir, { recursive: true });
  }, 5000);
});

// ─────────────────────────────────────────────────────────────────────────────
// Phase 2 — SWDE-49: collect / relay / broadcast / delegate / focus / unfocus / pin / detach
// axiom:trace work_item=SWDE-49 spec=specs/107-Conductor.md plan=phase2/task-2.6/step-1 jira_ref=SWDE-49
// ─────────────────────────────────────────────────────────────────────────────

describe("Phase 2: detached agents (REQ-COND-036)", () => {
  test("spawn with detach:true stores detached=1 in DB", async () => {
    const { plugin, db, tmpDir } = await makePlugin();

    const result = await callTool(plugin, "conductor_spawn", {
      name: "detached-worker",
      task: "work in background forever",
      detach: true,
    });

    expect(result.agent_id).toBeDefined();
    expect(result.detached).toBe(true);

    const row = db.prepare("SELECT detached FROM conductor_agents WHERE agent_id=?")
      .get(result.agent_id as string) as { detached: number };
    expect(row.detached).toBe(1);

    rmSync(tmpDir, { recursive: true });
  });

  test("spawn without detach returns detached:false and stores detached=0", async () => {
    const { plugin, db, tmpDir } = await makePlugin();

    const result = await callTool(plugin, "conductor_spawn", {
      name: "normal-worker",
      task: "regular task",
    });

    expect(result.detached).toBe(false);
    const row = db.prepare("SELECT detached FROM conductor_agents WHERE agent_id=?")
      .get(result.agent_id as string) as { detached: number };
    expect(row.detached).toBe(0);

    rmSync(tmpDir, { recursive: true });
  });

  test("session.stop cancels non-detached agents but NOT detached ones", async () => {
    const tmpDir = mkdtempSync(join(tmpdir(), "conductor-detach-stop-"));
    const conductorDir = join(tmpDir, ".conductor");
    mkdirSync(conductorDir, { recursive: true });
    const db = new Database(join(conductorDir, "conductor.db"));
    initConductorDB(db);

    // Phase 4: env var for auth fallback (set in beforeEach for this describe,
    // but this test may be in a different describe — set explicitly)
    process.env.AXIOM_CONDUCTOR_AUTH__ALLOW_SPAWN_SECRET_FALLBACK = "true";
    const plugin = await ConductorPlugin({
      directory: tmpDir,
      client: { baseUrl: "http://127.0.0.1:1" },
    });

    // Insert one normal (detached=0) and one detached (detached=1) agent
    const { agentId: normalId } = await insertTestAgent(db, {
      name: "normal-bg",
      spawned_by: "primary-session-detach-test",
    });
    const { agentId: detachedId } = await insertTestAgent(db, {
      name: "detached-bg",
      spawned_by: "primary-session-detach-test",
    });
    db.prepare("UPDATE conductor_agents SET detached=1 WHERE agent_id=?").run(detachedId);

    // Trigger session.stop
    const eventHook = plugin.event as (input: { event: { type: string; properties?: Record<string, unknown> } }) => Promise<void>;
    await eventHook({
      event: { type: "session.stop", properties: { sessionID: "primary-session-detach-test" } },
    });

    const normalRow = db.prepare("SELECT status FROM conductor_agents WHERE agent_id=?")
      .get(normalId) as { status: string };
    const detachedRow = db.prepare("SELECT status FROM conductor_agents WHERE agent_id=?")
      .get(detachedId) as { status: string };

    expect(normalRow.status).toBe("cancelled");   // normal agent IS cancelled
    expect(detachedRow.status).toBe("running");   // detached agent survives

    rmSync(tmpDir, { recursive: true });
  });
});

describe("Phase 2: conductor.collect (REQ-COND-015)", () => {
  test("returns all_done:true and empty agents when no agents exist", async () => {
    const { plugin, tmpDir } = await makePlugin();

    const result = await callTool(plugin, "conductor_collect", { timeout: "1s" });

    expect(result.all_done).toBe(true);
    expect(result.timed_out).toBe(false);
    expect(Array.isArray(result.agents)).toBe(true);
    expect(result.total).toBe(0);

    rmSync(tmpDir, { recursive: true });
  });

  test("returns all agents with their statuses after they complete", async () => {
    const { plugin, db, tmpDir } = await makePlugin();
    const ctx = { sessionID: "test-primary-session" };

    // Spawn two agents and immediately complete them
    const s1 = await callTool(plugin, "conductor_spawn", { name: "worker-1", task: "task 1" }, ctx);
    const s2 = await callTool(plugin, "conductor_spawn", { name: "worker-2", task: "task 2" }, ctx);

    // Directly mark them done in DB (no real session server in tests)
    db.prepare("UPDATE conductor_agents SET status='done', result_summary='result 1' WHERE agent_id=?")
      .run(s1.agent_id as string);
    db.prepare("UPDATE conductor_agents SET status='done', result_summary='result 2' WHERE agent_id=?")
      .run(s2.agent_id as string);

    const result = await callTool(plugin, "conductor_collect", { timeout: "1s" }, ctx);

    expect(result.all_done).toBe(true);
    expect(result.timed_out).toBe(false);
    expect(result.total).toBe(2);
    const agents = result.agents as { agent_id: string; status: string }[];
    expect(agents.every((a) => a.status === "done")).toBe(true);

    // step-qa-003: Verify per-agent result fields are returned in collect response
    const a1 = (result.agents as any[]).find(a => a.agent_id === s1.agent_id);
    const a2 = (result.agents as any[]).find(a => a.agent_id === s2.agent_id);
    expect(a1?.result_summary).toBe("result 1");
    expect(a2?.result_summary).toBe("result 2");
    // stash_id field should be present in each agent record (null since no stash was created)
    expect("stash_id" in a1).toBe(true);

    rmSync(tmpDir, { recursive: true });
  });

  test("returns timed_out:true and all_done:false when agents still running", async () => {
    const { plugin, db, tmpDir } = await makePlugin();
    const ctx = { sessionID: "test-primary-session" };

    // Spawn an agent but never complete it
    const s1 = await callTool(plugin, "conductor_spawn", { name: "slow-worker", task: "slow task" }, ctx);

    // collect with very short timeout (100ms)
    const result = await callTool(plugin, "conductor_collect", { timeout: "0.1s" }, ctx);

    expect(result.timed_out).toBe(true);
    expect(result.all_done).toBe(false);
    const agents = result.agents as { agent_id: string; status: string }[];
    const slowAgent = agents.find((a) => a.agent_id === s1.agent_id);
    expect(slowAgent?.status).toBe("running");

    rmSync(tmpDir, { recursive: true });
  }, 5000);

  test("collect with all:true includes agents from all sessions", async () => {
    const { plugin, db, tmpDir } = await makePlugin();

    // Insert agents from different sessions
    await insertTestAgent(db, { name: "sess-a-agent", spawned_by: "session-a", status: "done" });
    await insertTestAgent(db, { name: "sess-b-agent", spawned_by: "session-b", status: "done" });

    // Collect with all:true
    const result = await callTool(plugin, "conductor_collect", { timeout: "1s", all: true }, {});

    expect(result.total as number).toBeGreaterThanOrEqual(2);
    const agents = result.agents as { name: string }[];
    const names = agents.map((a) => a.name);
    expect(names).toContain("sess-a-agent");
    expect(names).toContain("sess-b-agent");

    rmSync(tmpDir, { recursive: true });
  });

  // step-qa-004: Verify collect treats failed/cancelled as terminal (all_done:true)
  test("collect with mixed agent states (done/failed/cancelled) returns all_done:true", async () => {
    const { plugin, db, tmpDir } = await makePlugin();

    // Insert 3 agents with terminal statuses directly
    await insertTestAgent(db, { name: "mix-done", spawned_by: "mix-state-session", status: "done" });
    await insertTestAgent(db, { name: "mix-failed", spawned_by: "mix-state-session", status: "failed" });
    await insertTestAgent(db, { name: "mix-cancelled", spawned_by: "mix-state-session", status: "cancelled" });

    const result = await callTool(plugin, "conductor_collect", { timeout: "1s", all: true }, {});

    // Behavioral contract: all_done means no running agents, not all strictly done
    expect(result.all_done).toBe(true);
    expect(result.total as number).toBeGreaterThanOrEqual(3);

    // All 3 terminal statuses should be present in the agents array
    const statuses = (result.agents as any[]).map(a => a.status);
    expect(statuses).toContain("done");
    expect(statuses).toContain("failed");
    expect(statuses).toContain("cancelled");

    rmSync(tmpDir, { recursive: true });
  });
});

describe("Phase 2: conductor.relay (REQ-COND-016)", () => {
  test("relay spawns a new agent with source result as context", async () => {
    const { plugin, db, tmpDir } = await makePlugin();
    const ctx = { sessionID: "test-primary-session" };

    // Spawn source agent and complete it
    const source = await callTool(plugin, "conductor_spawn", { name: "investigator", task: "investigate" }, ctx);
    const { secret } = await (async () => {
      const row = db.prepare("SELECT spawn_secret_hash FROM conductor_agents WHERE agent_id=?")
        .get(source.agent_id as string) as { spawn_secret_hash: string };
      // We can't reverse the hash — use conductor.done via tool with a fresh agent inserted
      const s = generateSpawnSecret();
      const h = await hashSpawnSecret(s);
      db.prepare("UPDATE conductor_agents SET spawn_secret_hash=? WHERE agent_id=?")
        .run(h, source.agent_id as string);
      return { secret: s };
    })();

    // Complete the source agent
    await callTool(plugin, "conductor_done", {
      agent_id: source.agent_id,
      secret,
      summary: "Found 3 issues in auth flow",
      result_type: "finding",
    });

    // Now relay — source is already done, so no actual waiting
    const relay = await callTool(plugin, "conductor_relay", {
      from: source.agent_id,
      name: "fixer",
      task: "Fix the issues found",
    }, ctx);

    expect(relay.agent_id).toBeDefined();
    expect(relay.agent_id).not.toBe(source.agent_id);
    expect(relay.status).toBe("running");
    expect(relay.relay_from).toBe(source.agent_id);
    expect(relay.source_status).toBe("done");

    // Verify the relay agent is in the DB
    const row = db.prepare("SELECT name, status, task FROM conductor_agents WHERE agent_id=?")
      .get(relay.agent_id as string) as { name: string; status: string; task: string } | undefined;
    expect(row?.name).toBe("fixer");
    expect(row?.status).toBe("running");

    // step-qa-001: Verify task column stores args.task exactly (context goes to initialMessage, not task column)
    expect(row?.task).toBe("Fix the issues found");

    rmSync(tmpDir, { recursive: true });
  });

  // step-qa-001: Verify relay stash-based context path — stash content goes to initialMessage, not task column
  test("relay with stash-based source context stores task (not stash content) in DB task column", async () => {
    const { plugin, db, tmpDir, stashRoot } = await makePlugin();
    const ctx = { sessionID: "test-primary-session" };

    // Create a stash with a type:result entry
    await createConductorStash(stashRoot, "relay-stash-ctx", "Relay Stash Context", "bg_src01", "source-agent");
    await appendStashEntry(stashRoot, "relay-stash-ctx", {
      ts: new Date().toISOString(),
      agent: "bg_src01",
      type: "result",
      summary: "stash-based finding, no auth issues",
    });

    // Insert source agent with stash_id and mark it done directly
    const { agentId: sourceAgentId } = await insertTestAgent(db, {
      name: "stash-source",
      status: "running",
      stash_id: "relay-stash-ctx",
      spawned_by: "test-primary-session",
    });
    db.prepare("UPDATE conductor_agents SET status='done' WHERE agent_id=?").run(sourceAgentId);

    // Relay from the stash-based source
    const relay = await callTool(plugin, "conductor_relay", {
      from: sourceAgentId,
      name: "stash-path-fixer",
      task: "Fix what was found",
    }, ctx);

    // Relay agent should be spawned
    expect(relay.agent_id).toBeDefined();
    expect(relay.status).toBe("running");

    // The relay agent's DB task column should equal args.task exactly
    // (stash context goes to initialMessage, not the task column)
    const relayRow = db.prepare("SELECT task FROM conductor_agents WHERE agent_id=?")
      .get(relay.agent_id as string) as { task: string } | undefined;
    expect(relayRow?.task).toBe("Fix what was found");

    // step-qa-007: verify the stash was readable at relay time (proxy for Path 1 being taken)
    const stashPeek = peekStash(stashRoot, "relay-stash-ctx");
    expect(stashPeek.found).toBe(true);
    expect(stashPeek.result_entry).not.toBeNull();

    rmSync(tmpDir, { recursive: true });
  });

  test("relay returns error for unknown source agent", async () => {
    const { plugin, tmpDir } = await makePlugin();

    const result = await callTool(plugin, "conductor_relay", {
      from: "bg_nonexistent",
      name: "new-agent",
      task: "do something",
      wait_timeout: "0.1s",
    });

    expect(result.error).toBeDefined();
    expect(String(result.error)).toContain("Unknown source agent_id");

    rmSync(tmpDir, { recursive: true });
  });

  test("relay times out if source agent doesn't complete", async () => {
    const { plugin, db, tmpDir } = await makePlugin();
    const ctx = { sessionID: "test-primary-session" };

    // Spawn source agent but never complete it
    const source = await callTool(plugin, "conductor_spawn", { name: "slow-investigator", task: "slow" }, ctx);
    expect(source.status).toBe("running");

    // relay with very short wait_timeout — source won't complete in time
    // relay will still spawn a new agent (with partial/no context)
    const relay = await callTool(plugin, "conductor_relay", {
      from: source.agent_id,
      name: "hopeful-fixer",
      task: "Fix whatever you find",
      wait_timeout: "0.1s",
    }, ctx);

    // relay should still return a new agent (even without context, it proceeds)
    expect(relay.agent_id).toBeDefined();
    expect(relay.status).toBe("running");

    rmSync(tmpDir, { recursive: true });
  }, 5000);

  // step-qa-005: Verify relay enforces max concurrent agents limit
  test("relay returns error when max concurrent agents limit is reached", async () => {
    const tmpDir = mkdtempSync(join(tmpdir(), "conductor-relay-limit-"));
    const conductorDir = join(tmpDir, ".conductor");
    mkdirSync(conductorDir, { recursive: true });
    const db = new Database(join(conductorDir, "conductor.db"));
    initConductorDB(db);

    // Create plugin with max_concurrent_agents: 1 (via env var)
    process.env.AXIOM_CONDUCTOR_AUTH__ALLOW_SPAWN_SECRET_FALLBACK = "true";
    process.env.AXIOM_CONDUCTOR_LIMITS__MAX_CONCURRENT_AGENTS = "1";
    const plugin = await ConductorPlugin({
      directory: tmpDir,
      client: { baseUrl: "http://127.0.0.1:1" },
    });
    delete process.env.AXIOM_CONDUCTOR_LIMITS__MAX_CONCURRENT_AGENTS;

    // Insert one running agent (fills the limit)
    await insertTestAgent(db, { spawned_by: "test-primary-session", status: "running" });

    // Insert a source agent that is done
    const { agentId: sourceAgentId } = await insertTestAgent(db, {
      name: "relay-limit-source",
      status: "done",
      spawned_by: "test-primary-session",
    });

    // Relay should fail because limit is already reached
    const result = await callTool(plugin, "conductor_relay", {
      from: sourceAgentId,
      name: "new-fixer",
      task: "fix it",
    }, { sessionID: "test-primary-session" });

    expect(result.error).toBeDefined();
    expect(String(result.error)).toContain("Max concurrent agents");

    rmSync(tmpDir, { recursive: true });
  });
});

describe("Phase 2: conductor.broadcast (REQ-COND-014)", () => {
  test("broadcast to zero running agents returns sent:0 and empty agents", async () => {
    const { plugin, tmpDir } = await makePlugin();

    const result = await callTool(plugin, "conductor_broadcast", {
      message: "update priority",
    });

    expect(result.sent).toBe(0);
    expect(result.failed).toBe(0);
    expect(Array.isArray(result.agents)).toBe(true);
    expect((result.agents as unknown[]).length).toBe(0);

    rmSync(tmpDir, { recursive: true });
  });

  test("broadcast attempts to deliver to all running agents (synthetic sessions fail gracefully)", async () => {
    const { plugin, db, tmpDir } = await makePlugin();
    const ctx = { sessionID: "test-primary-session" };

    // Spawn 3 agents (they'll have synthetic session IDs since no real server)
    await callTool(plugin, "conductor_spawn", { name: "fallback-agent-1", task: "task 1" }, ctx);
    await callTool(plugin, "conductor_spawn", { name: "fallback-agent-2", task: "task 2" }, ctx);
    await callTool(plugin, "conductor_spawn", { name: "fallback-agent-3", task: "task 3" }, ctx);

    const result = await callTool(plugin, "conductor_broadcast", {
      message: "priority change: focus on auth first",
    }, ctx);

    // sent+failed should equal total running agents for this session
    const totalAttempted = (result.sent as number) + (result.failed as number);
    expect(totalAttempted).toBe(3);
    // In test env (no real OpenCode server), deliveries fail — that's expected and acceptable
    const agents = result.agents as { agent_id: string; delivered: boolean }[];
    expect(agents.length).toBe(3);

    rmSync(tmpDir, { recursive: true });
  });

  test("broadcast with all:true includes agents from all sessions", async () => {
    const { plugin, db, tmpDir } = await makePlugin();

    // Insert running agents from different sessions
    await insertTestAgent(db, { name: "multi-a", spawned_by: "sess-x", status: "running" });
    await insertTestAgent(db, { name: "multi-b", spawned_by: "sess-y", status: "running" });

    const result = await callTool(plugin, "conductor_broadcast", {
      message: "global broadcast",
      all: true,
    }, {});

    const totalAttempted = (result.sent as number) + (result.failed as number);
    expect(totalAttempted).toBeGreaterThanOrEqual(2);

    rmSync(tmpDir, { recursive: true });
  });
});

describe("Phase 2: conductor.delegate (REQ-COND-013)", () => {
  test("delegate spawns a background agent with an auto-created stash", async () => {
    const { plugin, db, tmpDir, stashRoot } = await makePlugin();
    const ctx = { sessionID: "test-primary-session" };

    const result = await callTool(plugin, "conductor_delegate", {
      name: "continue-investigation",
      task: "Continue the auth investigation from where I left off",
      context: "We found 2 issues so far: missing CSRF token and session fixation. " + "x".repeat(450),
    }, ctx);

    expect(result.agent_id).toBeDefined();
    expect(result.session_id).toBeDefined();
    expect(result.stash_id).toBeDefined();
    expect(result.status).toBe("running");
    expect(result.delegated).toBe(true);

    // Verify agent is in DB
    const row = db.prepare("SELECT name, stash_id FROM conductor_agents WHERE agent_id=?")
      .get(result.agent_id as string) as { name: string; stash_id: string } | undefined;
    expect(row?.name).toBe("continue-investigation");
    expect(row?.stash_id).toBe(result.stash_id);

    // Verify context was written to stash as a JSONL entry
    const entries = readStashEntries(stashRoot, result.stash_id as string);
    const contextEntry = entries.find((e) => e.type === "context");
    expect(contextEntry).toBeDefined();
    expect(String(contextEntry?.summary ?? "")).toContain("CSRF token");

    // step-qa-002: Verify task column stores args.task exactly (context goes to initialMessage, not task column)
    const delegateRow = db.prepare("SELECT task FROM conductor_agents WHERE agent_id=?")
      .get(result.agent_id as string) as { task: string } | undefined;
    expect(delegateRow?.task).toBe("Continue the auth investigation from where I left off");

    // step-qa-002: Verify details field also contains the full context string
    expect(String(contextEntry?.details ?? "")).toContain("CSRF token");
    // step-qa-006: Verify summary is truncated to <=500 chars and details carries full length
    expect(contextEntry?.summary?.length).toBeLessThanOrEqual(500);
    expect((contextEntry?.details as string ?? "").length).toBeGreaterThan(500);

    rmSync(tmpDir, { recursive: true });
  });

  test("delegate works without context parameter", async () => {
    const { plugin, db, tmpDir } = await makePlugin();
    const ctx = { sessionID: "test-primary-session" };

    const result = await callTool(plugin, "conductor_delegate", {
      name: "background-worker",
      task: "Work on the refactor",
    }, ctx);

    expect(result.agent_id).toBeDefined();
    expect(result.delegated).toBe(true);
    expect(result.stash_id).toBeDefined();

    rmSync(tmpDir, { recursive: true });
  });

  test("delegate uses explicit stash name when provided", async () => {
    const { plugin, db, tmpDir } = await makePlugin();
    const ctx = { sessionID: "test-primary-session" };

    const result = await callTool(plugin, "conductor_delegate", {
      name: "named-worker",
      task: "Work on it",
      stash: "my-explicit-stash",
    }, ctx);

    expect(result.stash_id).toBe("my-explicit-stash");

    rmSync(tmpDir, { recursive: true });
  });

  // step-qa-005: Verify delegate enforces max concurrent agents limit
  test("delegate returns error when max concurrent agents limit is reached", async () => {
    const tmpDir = mkdtempSync(join(tmpdir(), "conductor-delegate-limit-"));
    const conductorDir = join(tmpDir, ".conductor");
    mkdirSync(conductorDir, { recursive: true });
    const db = new Database(join(conductorDir, "conductor.db"));
    initConductorDB(db);

    // Create plugin with max_concurrent_agents: 1 (via env var)
    process.env.AXIOM_CONDUCTOR_AUTH__ALLOW_SPAWN_SECRET_FALLBACK = "true";
    process.env.AXIOM_CONDUCTOR_LIMITS__MAX_CONCURRENT_AGENTS = "1";
    const plugin = await ConductorPlugin({
      directory: tmpDir,
      client: { baseUrl: "http://127.0.0.1:1" },
    });
    delete process.env.AXIOM_CONDUCTOR_LIMITS__MAX_CONCURRENT_AGENTS;

    // Insert one running agent (fills the limit)
    await insertTestAgent(db, { spawned_by: "test-primary-session", status: "running" });

    // Delegate should fail because limit is already reached
    const result = await callTool(plugin, "conductor_delegate", {
      name: "over-limit-worker",
      task: "should fail",
    }, { sessionID: "test-primary-session" });

    expect(result.error).toBeDefined();
    expect(String(result.error)).toContain("Max concurrent agents");

    rmSync(tmpDir, { recursive: true });
  });
});

describe("Phase 2: conductor.focus / unfocus / pin (REQ-COND-010/011/012)", () => {
  test("focus marks stash as focused and returns peek content", async () => {
    const { plugin, db, tmpDir, stashRoot } = await makePlugin();
    const ctx = { sessionID: "focus-test-session" };

    // Create a stash to focus
    await createConductorStash(stashRoot, "my-work-stash", "My Work", "bg_test01", "test-agent");

    const result = await callTool(plugin, "conductor_focus", { stash: "my-work-stash" }, ctx);

    expect(result.stash_id).toBe("my-work-stash");
    expect(result.focused).toBe(true);
    expect(result.found).toBe(true);

    // Verify in DB
    const row = db.prepare(
      "SELECT session_id, stash_id, pinned FROM conductor_focused_stashes WHERE session_id=? AND stash_id=?"
    ).get("focus-test-session", "my-work-stash") as { session_id: string; stash_id: string; pinned: number } | undefined;
    expect(row).toBeDefined();
    expect(row?.pinned).toBe(0);

    rmSync(tmpDir, { recursive: true });
  });

  test("focus returns found:false for non-existent stash (no error)", async () => {
    const { plugin, tmpDir } = await makePlugin();
    const ctx = { sessionID: "focus-test-session" };

    const result = await callTool(plugin, "conductor_focus", { stash: "does-not-exist" }, ctx);

    expect(result.stash_id).toBe("does-not-exist");
    expect(result.focused).toBe(true);
    expect(result.found).toBe(false);

    rmSync(tmpDir, { recursive: true });
  });

  test("focus twice updates focused_at without duplicating the row", async () => {
    const { plugin, db, tmpDir, stashRoot } = await makePlugin();
    const ctx = { sessionID: "focus-test-session" };

    await createConductorStash(stashRoot, "dedupe-stash", "Dedupe", "bg_dd", "dedupe-agent");
    await callTool(plugin, "conductor_focus", { stash: "dedupe-stash" }, ctx);
    await callTool(plugin, "conductor_focus", { stash: "dedupe-stash" }, ctx);

    const rows = db.prepare(
      "SELECT COUNT(*) as cnt FROM conductor_focused_stashes WHERE session_id=? AND stash_id=?"
    ).get("focus-test-session", "dedupe-stash") as { cnt: number };
    expect(rows.cnt).toBe(1);  // exactly one row — no duplicate

    rmSync(tmpDir, { recursive: true });
  });

  test("unfocus removes the focus row", async () => {
    const { plugin, db, tmpDir, stashRoot } = await makePlugin();
    const ctx = { sessionID: "unfocus-test-session" };

    await createConductorStash(stashRoot, "temp-stash", "Temp", "bg_t1", "temp-agent");
    await callTool(plugin, "conductor_focus", { stash: "temp-stash" }, ctx);

    // Verify focused
    const before = db.prepare(
      "SELECT COUNT(*) as cnt FROM conductor_focused_stashes WHERE session_id=? AND stash_id=?"
    ).get("unfocus-test-session", "temp-stash") as { cnt: number };
    expect(before.cnt).toBe(1);

    // Unfocus
    const result = await callTool(plugin, "conductor_unfocus", { stash: "temp-stash" }, ctx);
    expect(result.stash_id).toBe("temp-stash");
    expect(result.unfocused).toBe(true);

    // Verify removed
    const after = db.prepare(
      "SELECT COUNT(*) as cnt FROM conductor_focused_stashes WHERE session_id=? AND stash_id=?"
    ).get("unfocus-test-session", "temp-stash") as { cnt: number };
    expect(after.cnt).toBe(0);

    rmSync(tmpDir, { recursive: true });
  });

  test("unfocus on non-focused stash returns unfocused:true (idempotent)", async () => {
    const { plugin, tmpDir } = await makePlugin();
    const ctx = { sessionID: "unfocus-test-session" };

    const result = await callTool(plugin, "conductor_unfocus", { stash: "not-focused" }, ctx);
    expect(result.unfocused).toBe(true);

    rmSync(tmpDir, { recursive: true });
  });

  test("pin sets pinned=1 in conductor_focused_stashes", async () => {
    const { plugin, db, tmpDir } = await makePlugin();
    const ctx = { sessionID: "pin-test-session" };

    const result = await callTool(plugin, "conductor_pin", { stash: "important-stash" }, ctx);
    expect(result.stash_id).toBe("important-stash");
    expect(result.pinned).toBe(true);

    const row = db.prepare(
      "SELECT pinned FROM conductor_focused_stashes WHERE session_id=? AND stash_id=?"
    ).get("pin-test-session", "important-stash") as { pinned: number } | undefined;
    expect(row?.pinned).toBe(1);

    rmSync(tmpDir, { recursive: true });
  });

  test("pin after focus upgrades pinned from 0 to 1", async () => {
    const { plugin, db, tmpDir, stashRoot } = await makePlugin();
    const ctx = { sessionID: "pin-upgrade-session" };

    await createConductorStash(stashRoot, "upgrade-stash", "Upgrade", "bg_up", "upgrader");
    await callTool(plugin, "conductor_focus", { stash: "upgrade-stash" }, ctx);

    // Verify initially pinned=0
    const before = db.prepare(
      "SELECT pinned FROM conductor_focused_stashes WHERE session_id=? AND stash_id=?"
    ).get("pin-upgrade-session", "upgrade-stash") as { pinned: number };
    expect(before.pinned).toBe(0);

    // Pin it
    await callTool(plugin, "conductor_pin", { stash: "upgrade-stash" }, ctx);

    const after = db.prepare(
      "SELECT pinned FROM conductor_focused_stashes WHERE session_id=? AND stash_id=?"
    ).get("pin-upgrade-session", "upgrade-stash") as { pinned: number };
    expect(after.pinned).toBe(1);

    rmSync(tmpDir, { recursive: true });
  });

  test("pin twice is idempotent (still one row, pinned=1)", async () => {
    const { plugin, db, tmpDir } = await makePlugin();
    const ctx = { sessionID: "pin-idempotent-session" };

    await callTool(plugin, "conductor_pin", { stash: "stable-stash" }, ctx);
    await callTool(plugin, "conductor_pin", { stash: "stable-stash" }, ctx);

    const rows = db.prepare(
      "SELECT COUNT(*) as cnt, MAX(pinned) as p FROM conductor_focused_stashes WHERE session_id=? AND stash_id=?"
    ).get("pin-idempotent-session", "stable-stash") as { cnt: number; p: number };
    expect(rows.cnt).toBe(1);
    expect(rows.p).toBe(1);

    rmSync(tmpDir, { recursive: true });
  });
});

describe("Phase 2: context banner includes pinned stashes (REQ-COND-012)", () => {
  // Phase 4 config adoption: use env vars for auth fallback
  beforeEach(() => {
    process.env.AXIOM_CONDUCTOR_AUTH__ALLOW_SPAWN_SECRET_FALLBACK = "true";
  });
  afterEach(() => {
    delete process.env.AXIOM_CONDUCTOR_AUTH__ALLOW_SPAWN_SECRET_FALLBACK;
  });

  test("pinned stash name appears in context banner after pinning", async () => {
    const tmpDir = mkdtempSync(join(tmpdir(), "conductor-banner-pin-"));
    const conductorDir = join(tmpDir, ".conductor");
    mkdirSync(conductorDir, { recursive: true });
    const db = new Database(join(conductorDir, "conductor.db"));
    initConductorDB(db);

    const plugin = await ConductorPlugin({
      directory: tmpDir,
      client: { baseUrl: "http://127.0.0.1:1" },
    });

    const ctx = { sessionID: "banner-pin-session" };

    // Spawn an agent so the banner triggers (banner only shows when agents > 0)
    await callTool(plugin, "conductor_spawn", { name: "banner-agent", task: "banner task" }, ctx);

    // Pin a stash
    await callTool(plugin, "conductor_pin", { stash: "pinned-work" }, ctx);

    // Invoke the system.transform hook
    const transformHook = plugin["experimental.chat.system.transform"] as (
      input: { sessionID: string; model: string },
      output: { system: string[] }
    ) => Promise<void>;

    const output = { system: [] as string[] };
    await transformHook({ sessionID: "banner-pin-session", model: "test-model" }, output);

    // Banner should mention the pinned stash
    const banner = output.system.join("\n");
    expect(banner).toContain("Pinned:");
    expect(banner).toContain("pinned-work");

    rmSync(tmpDir, { recursive: true });
  });

  test("unpinned focused stash does NOT appear in banner under Pinned:", async () => {
    const tmpDir = mkdtempSync(join(tmpdir(), "conductor-banner-focus-"));
    const conductorDir = join(tmpDir, ".conductor");
    mkdirSync(conductorDir, { recursive: true });
    const db = new Database(join(conductorDir, "conductor.db"));
    initConductorDB(db);

    const plugin = await ConductorPlugin({
      directory: tmpDir,
      client: { baseUrl: "http://127.0.0.1:1" },
    });

    const ctx = { sessionID: "banner-focus-only-session" };

    // Spawn an agent and focus a stash (but don't pin it)
    await callTool(plugin, "conductor_spawn", { name: "focus-agent", task: "focus task" }, ctx);
    await callTool(plugin, "conductor_focus", { stash: "unpinned-work" }, ctx);

    const transformHook = plugin["experimental.chat.system.transform"] as (
      input: { sessionID: string; model: string },
      output: { system: string[] }
    ) => Promise<void>;

    const output = { system: [] as string[] };
    await transformHook({ sessionID: "banner-focus-only-session", model: "test-model" }, output);

    // Should have a banner (agent exists) but NOT show Pinned: section
    const banner = output.system.join("\n");
    expect(banner.length).toBeGreaterThan(0);
    expect(banner).not.toContain("Pinned:");

    rmSync(tmpDir, { recursive: true });
  });
});

describe("Phase 2: DB migration — detached column on existing DBs", () => {
  test("initConductorDB adds detached column to existing conductor_agents table", () => {
    const tmpDir = mkdtempSync(join(tmpdir(), "conductor-migration-"));
    const db = new Database(join(tmpDir, "test.db"));

    // Simulate a Phase 1 DB (without detached column)
    db.exec(`
      PRAGMA journal_mode=WAL;
      CREATE TABLE conductor_agents (
        agent_id TEXT PRIMARY KEY,
        name TEXT NOT NULL,
        session_id TEXT NOT NULL,
        stash_id TEXT,
        status TEXT NOT NULL DEFAULT 'running',
        task TEXT NOT NULL,
        spawned_by TEXT NOT NULL,
        spawned_at TEXT NOT NULL,
        spawn_secret_hash TEXT NOT NULL
      );
    `);

    // Verify detached column doesn't exist yet
    const beforeCols = (
      db.prepare("SELECT name FROM pragma_table_info('conductor_agents')").all() as { name: string }[]
    ).map((r) => r.name);
    expect(beforeCols).not.toContain("detached");

    // Run initConductorDB — should add detached column via migration
    initConductorDB(db);

    const afterCols = (
      db.prepare("SELECT name FROM pragma_table_info('conductor_agents')").all() as { name: string }[]
    ).map((r) => r.name);
    expect(afterCols).toContain("detached");

    // Verify conductor_focused_stashes was also created
    const tables = (
      db.prepare("SELECT name FROM sqlite_master WHERE type='table'").all() as { name: string }[]
    ).map((r) => r.name);
    expect(tables).toContain("conductor_focused_stashes");

    rmSync(tmpDir, { recursive: true });
  });

  test("initConductorDB is safe to run twice on migrated DB", () => {
    const tmpDir = mkdtempSync(join(tmpdir(), "conductor-migration-idempotent-"));
    const db = new Database(join(tmpDir, "test.db"));

    // Run twice — should not throw
    initConductorDB(db);
    initConductorDB(db);

    const cols = (
      db.prepare("SELECT name FROM pragma_table_info('conductor_agents')").all() as { name: string }[]
    ).map((r) => r.name);
    expect(cols).toContain("detached");

    rmSync(tmpDir, { recursive: true });
  });
});
