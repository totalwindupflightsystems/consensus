/**
 * Conductor Plugin — Invisible Background Agent Orchestration (Phase 1).
 *
 * Transforms the primary agent into an orchestra director — spawning N background
 * agents (each in their own OpenCode session), each working independently, each
 * optionally writing findings to shared stash files, while the primary agent stays
 * focused without context pollution.
 *
 * Phase 1 tools: conductor.spawn, conductor.done, conductor.status,
 *                conductor.result, conductor.cancel, conductor.wait
 *
 * Storage: .conductor/conductor.db (default, overridable via AXIOM_CONDUCTOR_DATABASE_PATH)
 *
 * Security:
 *   REQ-COND-020  Spawn secret: 128-bit CSPRNG, base64url, SHA-256 hash in DB, NEVER logged
 *   REQ-COND-005b allow_spawn_secret_fallback=false default; fail-closed if SPIRE unavailable
 *
 * axiom:trace work_item=SWDE-45 spec=specs/107-Conductor.md plan=phase-0/task-0.1/step-1 jira_ref=SWDE-45
 */

import { Database } from "bun:sqlite";
import { openDatabase, sqliteWriteWithRetry } from "../shared/sqlite";
// axiom:trace work_item=SWDE-62 spec=specs/102-Graph-Harness.md#4.1 plan=phase-4/task-4-1/step-4-1-1 jira_ref=SWDE-62
import {
  existsSync,
  mkdirSync,
  readFileSync,
  readdirSync,
  rmSync,
} from "node:fs";
import * as fsPromises from "node:fs/promises";
import { join, resolve, dirname } from "node:path";
import { timingSafeEqual } from "node:crypto";
import { tool } from "@opencode-ai/plugin";
import { createOpencodeClient } from "@opencode-ai/sdk";
import {
  validateStashId,
  safePath,
  redactCredentials,
  yamlDoubleQuote,
  buildSuspendedMarkdown,
  parseFrontmatter,
  atomicWrite,
  slugify,
  parseIndex,
} from "./context-stash.ts";
import { loadPluginConfig, pluginWarn, pluginError } from "./config-utils.ts";

// ─────────────────────────────────────────────────────────────────────────────
// Configuration
// ─────────────────────────────────────────────────────────────────────────────

export interface ConductorConfig {
  enabled: boolean;
  database_path: string;
  opencode_base_url: string;
  limits: {
    max_concurrent_agents: number;
    max_agents_per_session: number;
    default_timeout_minutes: number;
  };
  polling: {
    completion_check_interval_seconds: number;
    cost_update_interval_seconds: number;
  };
  context_banner: {
    enabled: boolean;
    verbosity: "minimal" | "normal" | "verbose";
    show_cost: boolean;
    show_elapsed: boolean;
  };
  lifecycle: {
    cancel_on_session_end: boolean;
    retention_days: number;
  };
  stash: {
    auto_create: boolean;
    default_log_level: string;
  };
  auth: {
    spire_socket_path: string;
    allow_spawn_secret_fallback: boolean;
    fallback_log_level: string;
  };
}

// Module-local (not exported) — plugin loader crashes on non-function exports.
// For test access, import from .opencode/shared/conductor-constants.ts instead.
export const DEFAULT_CONFIG: ConductorConfig = { // axiom:trace work_item=plugin-config-management-01 spec=specs/112-Plugin-Config-Management.md#8 plan=phase-4/task-4-2/step-4-2-0
  enabled: true,
  database_path: ".conductor/conductor.db",  // overridable via AXIOM_CONDUCTOR_DATABASE_PATH or config file
  opencode_base_url: "",  // empty = use injected plugin client; set to override with a specific server URL
  limits: {
    max_concurrent_agents: 10,
    max_agents_per_session: 50,
    default_timeout_minutes: 60,
  },
  polling: {
    completion_check_interval_seconds: 5,
    cost_update_interval_seconds: 30,
  },
  context_banner: {
    enabled: true,
    verbosity: "normal",
    show_cost: true,
    show_elapsed: true,
  },
  lifecycle: {
    cancel_on_session_end: true,
    retention_days: 7,
  },
  stash: {
    auto_create: true,
    default_log_level: "decisions",
  },
  auth: {
    spire_socket_path: "/run/spire/sockets/agent.sock",
    allow_spawn_secret_fallback: false,  // REQ-COND-005b §1: fail-closed. For local dev, set true in .opencode/config/conductor.json
    fallback_log_level: "critical",
  },
};

// ─────────────────────────────────────────────────────────────────────────────
// DB schema
// ─────────────────────────────────────────────────────────────────────────────

/**
 * Initialize the conductor_agents table.
 * Uses IF NOT EXISTS so it's safe to call on a shared harness.db.
 * The spawn_secret_hash column stores SHA-256 of the spawn secret — never the secret itself.
 *
 * NOTE: PRAGMA settings (WAL mode, busy_timeout, synchronous, foreign_keys, wal_autocheckpoint)
 * are owned by openDatabase() in .opencode/shared/sqlite.ts. This function contains only DDL
 * (CREATE TABLE / CREATE INDEX). The journal_mode and foreign_keys lines below are idempotent
 * re-sets kept for backward compatibility when tests call initConductorDB(new Database())
 * directly without going through openDatabase().
 *
 * IMPORTANT: Do NOT add PRAGMA busy_timeout here — it would override the 10000ms default
 * set by openDatabase() because SQLite PRAGMAs are last-write-wins on a connection.
 *
 * axiom:trace work_item=SWDE-45 spec=specs/107-Conductor.md#4.1 plan=phase-0/task-0.1/step-1 jira_ref=SWDE-45
 * axiom:trace work_item=SWDE-62 spec=specs/102-Graph-Harness.md#4.1 plan=phase-1/task-1-2/step-1-2-1 jira_ref=SWDE-62
 */
export function initConductorDB(db: Database): void {
  db.exec(`
    PRAGMA journal_mode=WAL;
    PRAGMA foreign_keys=ON;

    CREATE TABLE IF NOT EXISTS conductor_agents (
      agent_id           TEXT PRIMARY KEY,
      name               TEXT NOT NULL,
      session_id         TEXT NOT NULL,
      stash_id           TEXT,
      status             TEXT NOT NULL DEFAULT 'running',
      task               TEXT NOT NULL,
      model              TEXT,
      spawned_by         TEXT NOT NULL,
      spawned_at         TEXT NOT NULL,
      completed_at       TEXT,
      timeout_at         TEXT,
      cost_usd           REAL NOT NULL DEFAULT 0,
      result_summary     TEXT,
      result_type        TEXT,
      error              TEXT,
      spawn_secret_hash  TEXT NOT NULL,
      detached           INTEGER NOT NULL DEFAULT 0
    );

    CREATE TABLE IF NOT EXISTS conductor_focused_stashes (
      stash_id           TEXT PRIMARY KEY,
      session_id         TEXT NOT NULL,
      focused_at         TEXT NOT NULL,
      pinned             INTEGER NOT NULL DEFAULT 0
    );

    CREATE INDEX IF NOT EXISTS idx_conductor_agents_status
      ON conductor_agents(status);
    CREATE INDEX IF NOT EXISTS idx_conductor_agents_spawned_by
      ON conductor_agents(spawned_by);
    CREATE INDEX IF NOT EXISTS idx_conductor_agents_stash_id
      ON conductor_agents(stash_id);
  `);

  // ── Schema migrations (idempotent) ────────────────────────────────────────
  // REQ-COND-036: detached column added in Phase 2
  const existingCols = (
    db.prepare("SELECT name FROM pragma_table_info('conductor_agents')").all() as { name: string }[]
  ).map((r) => r.name);
  if (!existingCols.includes("detached")) {
    db.exec("ALTER TABLE conductor_agents ADD COLUMN detached INTEGER NOT NULL DEFAULT 0");
  }
}

// ─────────────────────────────────────────────────────────────────────────────
// Cryptographic helpers (exported for testing)
// ─────────────────────────────────────────────────────────────────────────────

/**
 * Generate a 128-bit CSPRNG spawn secret encoded as base64url.
 * REQ-COND-020: cryptographically random, 128-bit entropy.
 *
 * axiom:trace work_item=SWDE-45 spec=specs/107-Conductor.md#REQ-COND-020 plan=phase-0/task-0.2/step-1 jira_ref=SWDE-45
 */
export function generateSpawnSecret(): string {
  const bytes = new Uint8Array(16); // 128 bits
  crypto.getRandomValues(bytes);
  // base64url (no padding)
  const b64 = btoa(String.fromCharCode(...bytes));
  return b64.replace(/\+/g, "-").replace(/\//g, "_").replace(/=/g, "");
}

/**
 * SHA-256 hash of a spawn secret (hex string, 64 chars).
 * REQ-COND-020: only the hash is stored in the database.
 *
 * axiom:trace work_item=SWDE-45 spec=specs/107-Conductor.md#REQ-COND-020 plan=phase-0/task-0.2/step-1 jira_ref=SWDE-45
 */
export async function hashSpawnSecret(secret: string): Promise<string> {
  const encoder = new TextEncoder();
  const data = encoder.encode(secret);
  const hashBuffer = await crypto.subtle.digest("SHA-256", data);
  const hashArray = Array.from(new Uint8Array(hashBuffer));
  return hashArray.map((b) => b.toString(16).padStart(2, "0")).join("");
}

/**
 * Constant-time comparison of two SHA-256 hex strings.
 * Prevents timing oracle attacks on secret verification.
 * REQ-COND-020: constant-time comparison required.
 *
 * axiom:trace work_item=SWDE-45 spec=specs/107-Conductor.md#REQ-COND-020 plan=phase-0/task-0.3/step-1 jira_ref=SWDE-45
 */
export function constantTimeCompareHex(a: string, b: string): boolean {
  // Allocate fixed 32-byte buffers (SHA-256 output size)
  // Using fixed size prevents timing oracle on length differences
  const aBuf = Buffer.alloc(32, 0);
  const bBuf = Buffer.alloc(32, 0);
  try {
    const aRaw = Buffer.from(a, "hex");
    const bRaw = Buffer.from(b, "hex");
    aRaw.copy(aBuf, 0, 0, Math.min(aRaw.length, 32));
    bRaw.copy(bBuf, 0, 0, Math.min(bRaw.length, 32));
  } catch {
    // Invalid hex — still run comparison to maintain constant time
    return timingSafeEqual(aBuf, bBuf) && false;
  }
  // Constant-time byte comparison
  const bytesEqual = timingSafeEqual(aBuf, bBuf);
  // Also check lengths (masked by the fixed-buffer comparison above)
  return bytesEqual && a.length === b.length;
}

// ─────────────────────────────────────────────────────────────────────────────
// Agent ID generation
// ─────────────────────────────────────────────────────────────────────────────

/** Generate a unique background agent ID (e.g., "bg_a3f9x7k2"). */
export function generateAgentId(): string {
  const chars = "abcdefghijklmnopqrstuvwxyz0123456789";
  const bytes = new Uint8Array(8);
  crypto.getRandomValues(bytes);
  let id = "bg_";
  for (const byte of bytes) {
    id += chars[byte % chars.length];
  }
  return id;
}

// ─────────────────────────────────────────────────────────────────────────────
// Conductor envelope (injected into spawned agent's initial message)
// ─────────────────────────────────────────────────────────────────────────────

/**
 * Build the conductor_envelope block injected at the start of the spawned agent's task.
 * REQ-COND-005a: spawn secret passed to background agent in structured envelope field.
 *
 * NOTE: The spawn_secret within this envelope is the PLAINTEXT secret.
 * It MUST NOT be logged, stashed, or stored. The context-stash redactCredentials()
 * function will catch it if it leaks into stash content via the high-entropy pattern.
 *
 * axiom:trace work_item=SWDE-45 spec=specs/107-Conductor.md#REQ-COND-005a plan=phase-0/task-0.2/step-2 jira_ref=SWDE-45
 */
export function buildConductorEnvelope(
  agentId: string,
  stashId: string | null,
  secret: string
): string {
  const lines = ["[conductor_envelope]", `agent_id: ${agentId}`];
  if (stashId) lines.push(`stash_id: ${stashId}`);
  lines.push(`spawn_secret: ${secret}`);
  lines.push("[/conductor_envelope]");
  return lines.join("\n");
}

/**
 * Extract conductor envelope fields from a task string.
 * Returns null if no envelope is present.
 */
export function extractEnvelope(
  taskText: string
): { agentId: string; stashId: string | null; secret: string } | null {
  const match = taskText.match(
    /\[conductor_envelope\]([\s\S]*?)\[\/conductor_envelope\]/
  );
  if (!match) return null;
  const body = match[1];
  const agentId = body.match(/agent_id:\s*(\S+)/)?.[1];
  const stashId = body.match(/stash_id:\s*(\S+)/)?.[1] ?? null;
  const secret = body.match(/spawn_secret:\s*(\S+)/)?.[1];
  if (!agentId || !secret) return null;
  return { agentId, stashId, secret };
}

// ─────────────────────────────────────────────────────────────────────────────
// SPIRE availability check
// ─────────────────────────────────────────────────────────────────────────────

/**
 * Check if the SPIRE agent socket is available.
 * Uses existsSync (socket file presence) as the availability signal.
 * REQ-COND-005b: fail-closed if SPIRE unavailable and fallback not allowed.
 *
 * axiom:trace work_item=SWDE-45 spec=specs/107-Conductor.md#REQ-COND-005b plan=phase-0/task-0.1/step-2 jira_ref=SWDE-45
 */
export function checkSpireAvailability(socketPath: string): boolean {
  try {
    return existsSync(socketPath);
  } catch {
    return false;
  }
}

// ─────────────────────────────────────────────────────────────────────────────
// Stash helpers (filesystem operations, not via stash tool API)
// ─────────────────────────────────────────────────────────────────────────────

/** Path to the conductor entries JSONL file for a stash. */
function entriesPath(stashRoot: string, stashId: string): string {
  return join(stashRoot, "entries", `${stashId}.jsonl`);
}

/**
 * Per-stash write serializer — prevents TOCTOU races in concurrent appendStashEntry calls.
 * Maps `${stashRoot}:${stashId}` → last queued write promise.
 * REQ-COND-029: fan-in writes must be serialized to prevent JSONL corruption.
 * axiom:trace work_item=SWDE-45 spec=specs/107-Conductor.md#REQ-COND-029 plan=verify2-injected/H2
 */
const _appendLocks = new Map<string, Promise<void>>();

/**
 * Create an empty stash file for a background agent.
 * Used when conductor.spawn is called with --stash.
 * Writes to .memory-bank/stash/suspended/<stash-id>.md using context-stash helpers.
 *
 * axiom:trace work_item=SWDE-45 spec=specs/107-Conductor.md#REQ-COND-005 plan=phase-0/task-0.2/step-2 jira_ref=SWDE-45
 */
export async function createConductorStash(
  stashRoot: string,
  stashId: string,
  name: string,
  agentId: string,
  agentName: string
): Promise<void> {
  validateStashId(stashId);
  const suspendedDir = join(stashRoot, "suspended");
  if (!existsSync(suspendedDir)) {
    mkdirSync(suspendedDir, { recursive: true });
  }
  const filePath = safePath(stashRoot, "suspended", `${stashId}.md`);
  if (existsSync(filePath)) return; // Already exists — reuse it

  const now = new Date().toISOString();
  const fm = {
    stash_id: stashId,
    name,
    state: "suspended" as const,
    created_by: agentId,
    created_at: now,
    suspended_at: now,
    session_id: agentId, // use agentId as placeholder session for traceability
    tags: ["conductor", "background-agent"],
    entries: 0,
    last_agent: agentName,
  };
  const content = buildSuspendedMarkdown(
    fm,
    `Background agent stash for ${agentName} (${agentId})`,
    `Task assigned to agent ${agentId}.`,
    `Review conductor.result to see results from ${agentName}.`
  );
  await atomicWrite(filePath, content);
  await updateConductorStashIndex(
    stashRoot,
    stashId,
    name,
    "suspended",
    ["conductor", "background-agent"],
    fm.created_at,
    agentName
  );
}

/**
 * Update the _index.md for a conductor-created stash.
 * Uses parseIndex from context-stash.ts to read the current index, then rebuilds it.
 *
 * Minimal _index.md update for conductor-created stashes (REQ-COND-026)
 * Uses parseIndex from context-stash.ts to read; rebuilds with a simple append.
 * axiom:trace work_item=SWDE-45 spec=specs/107-Conductor.md#REQ-COND-026 plan=verify2-injected/H4
 */
async function updateConductorStashIndex(
  stashRoot: string,
  stashId: string,
  name: string,
  state: string,
  tags: string[],
  createdAt: string,
  lastAgent: string
): Promise<void> {
  const indexPath = join(stashRoot, "_index.md");
  let entries: Array<{ stash_id: string; name: string; state: string; tags: string[]; created_at: string; last_agent?: string }> = [];
  if (existsSync(indexPath)) {
    try {
      const content = readFileSync(indexPath, "utf-8");
      entries = parseIndex(content) as typeof entries;
    } catch { /* start fresh */ }
  }
  const idx = entries.findIndex((e) => e.stash_id === stashId);
  const newEntry = { stash_id: stashId, name, state, tags, created_at: createdAt, last_agent: lastAgent };
  if (idx >= 0) {
    entries[idx] = newEntry;
  } else {
    entries.push(newEntry);
  }
  // Build minimal index markdown
  const lines = [
    "# Context Stash Index",
    "",
    `_Updated: ${new Date().toISOString()}_`,
    "",
    "| ID | Name | State | Tags | Created | Last Agent |",
    "|----|------|-------|------|---------|------------|",
  ];
  for (const e of entries) {
    const row = [e.stash_id, e.name, e.state, (e.tags ?? []).join(", "), e.created_at, e.last_agent ?? ""]
      .map((v) => v.replace(/\|/g, ""));
    lines.push(`| ${row.join(" | ")} |`);
  }
  lines.push("");
  const content = lines.join("\n");
  await atomicWrite(indexPath, content);
}

/**
 * Append a conductor result entry to the stash's entries JSONL file.
 * Used by conductor.done to record structured results.
 *
 * Uses a per-stash promise queue to serialize concurrent writes and prevent
 * TOCTOU corruption of the JSONL file (REQ-COND-029 fan-in safety).
 *
 * axiom:trace work_item=SWDE-45 spec=specs/107-Conductor.md#4.2 plan=phase-0/task-0.3/step-1 jira_ref=SWDE-45
 */
export async function appendStashEntry(
  stashRoot: string,
  stashId: string,
  entry: Record<string, unknown>
): Promise<void> {
  const lockKey = `${stashRoot}:${stashId}`;
  // Chain onto the last queued write for this stash (serialization via promise queue)
  const prev = _appendLocks.get(lockKey) ?? Promise.resolve();
  const next = prev.then(() => _doAppendStashEntry(stashRoot, stashId, entry));
  _appendLocks.set(lockKey, next.catch(() => { /* don't block future writes on error */ }));
  return next;
}

async function _doAppendStashEntry(
  stashRoot: string,
  stashId: string,
  entry: Record<string, unknown>
): Promise<void> {
  const entriesDir = join(stashRoot, "entries");
  if (!existsSync(entriesDir)) {
    mkdirSync(entriesDir, { recursive: true });
  }
  const targetPath = entriesPath(stashRoot, stashId);

  // Redact credentials in the summary/details fields before writing
  const safeEntry = { ...entry };
  if (typeof safeEntry.summary === "string") {
    safeEntry.summary = redactCredentials(safeEntry.summary);
  }
  if (typeof safeEntry.details === "string") {
    safeEntry.details = redactCredentials(safeEntry.details);
  }
  const newLine = JSON.stringify(safeEntry) + "\n";

  // Atomic append: read existing + append new line + write to unique tmp + rename
  // This prevents partial-write artifacts on crash (REQ-COND-029 fan-in)
  // Each write uses a unique tmp file suffix to avoid tmp-name collisions across processes.
  // axiom:trace work_item=SWDE-45 spec=specs/107-Conductor.md#REQ-COND-029 plan=verify2-injected/H2
  const uniqueSuffix = `${Date.now().toString(36)}_${Math.random().toString(36).slice(2, 8)}`;
  const tmpPath = `${targetPath}.${uniqueSuffix}.tmp`;
  const existing = existsSync(targetPath)
    ? await fsPromises.readFile(targetPath, "utf-8")
    : "";
  await fsPromises.writeFile(tmpPath, existing + newLine, "utf-8");
  await fsPromises.rename(tmpPath, targetPath);
}

/**
 * Read all conductor entries for a stash.
 * Returns parsed entries in chronological order.
 */
export function readStashEntries(
  stashRoot: string,
  stashId: string
): Record<string, unknown>[] {
  const path = entriesPath(stashRoot, stashId);
  if (!existsSync(path)) return [];
  try {
    return readFileSync(path, "utf-8")
      .trim()
      .split("\n")
      .filter((l) => l.trim())
      .map((l) => JSON.parse(l) as Record<string, unknown>);
  } catch {
    return [];
  }
}

/**
 * Peek at a stash: return frontmatter + body preview + latest result entry.
 */
export function peekStash(
  stashRoot: string,
  stashId: string
): {
  found: boolean;
  name?: string;
  state?: string;
  summary?: string;
  result_entry?: Record<string, unknown>;
} {
  // Try suspended then closed
  let filePath: string | null = null;
  for (const dir of ["suspended", "closed"]) {
    const candidate = join(stashRoot, dir, `${stashId}.md`);
    if (existsSync(candidate)) {
      filePath = candidate;
      break;
    }
  }
  if (!filePath) return { found: false };

  try {
    const content = readFileSync(filePath, "utf-8");
    const { fm, body } = parseFrontmatter(content);
    const entries = readStashEntries(stashRoot, stashId);
    const resultEntries = entries.filter((e) => e.type === "result");
    const latest = resultEntries[resultEntries.length - 1] ?? null;
    return {
      found: true,
      name: fm.name,
      state: fm.state,
      summary: body.trim().slice(0, 1000),
      result_entry: latest ?? undefined,
    };
  } catch {
    return { found: false };
  }
}

/**
 * Check if a stash has been closed (moved to closed/).
 * Used by the completion poller.
 */
export function isStashClosed(stashRoot: string, stashId: string): boolean {
  return existsSync(join(stashRoot, "closed", `${stashId}.md`));
}

/**
 * Check if a stash has a type:result entry.
 * Used by the completion poller.
 */
export function stashHasResultEntry(
  stashRoot: string,
  stashId: string
): boolean {
  return readStashEntries(stashRoot, stashId).some((e) => e.type === "result");
}

// ─────────────────────────────────────────────────────────────────────────────
// Elapsed time formatter
// ─────────────────────────────────────────────────────────────────────────────

/** Format elapsed milliseconds as "Xm Ys". */
function formatElapsed(ms: number): string {
  const secs = Math.floor(ms / 1000);
  if (secs < 60) return `${secs}s`;
  const mins = Math.floor(secs / 60);
  const remSecs = secs % 60;
  return remSecs > 0 ? `${mins}m ${remSecs}s` : `${mins}m`;
}

// ─────────────────────────────────────────────────────────────────────────────
// Plugin factory
// ─────────────────────────────────────────────────────────────────────────────

export const ConductorPlugin = async ({
  directory,
  client,
}: {
  directory: string;
  client: unknown;
}) => {
  // ── Config ─────────────────────────────────────────────────────────────────────
  // Guard: reject missing/empty directory before passing to loadPluginConfig.
  // Note: typeof string check does not handle new String() objects (String subclasses).
  // OpenCode always passes string primitives so this is not a practical risk.
  // axiom:trace work_item=conductor-phase4-config-adoption spec=specs/112-Plugin-Config-Management.md#4.1 plan=phase-4/task-4-6/bl-r2-005
  if (!directory || typeof directory !== "string" || !directory.trim()) {
    throw new Error("ConductorPlugin: directory argument is required and must be a non-empty string");
  }
  // axiom:trace work_item=conductor-phase4-config-adoption spec=specs/112-Plugin-Config-Management.md#4.1 plan=phase-4/task-4-3/inject-p4-high-02

  // Phase 4 adoption: load config via loadPluginConfig() three-layer system
  // (defaults + file + env vars) per specs/112-Plugin-Config-Management.md.
  // axiom:trace work_item=conductor-phase4-config-adoption spec=specs/112-Plugin-Config-Management.md#8 plan=phase-4/task-4-2/step-4-2-2
  let config: ConductorConfig;
  // NOTE: This catch handles validatePluginName()/resolve() failures only.
  // File-read and JSON-parse errors are caught INSIDE loadPluginConfig() in config-utils.ts
  // (lines 444-451) and never propagate here. For file-permission or parse errors,
  // you will see a [config-utils] warning, not a [conductor] warning.
  // See: specs/112-Plugin-Config-Management.md §12 (Recovery and Troubleshooting)
  // axiom:trace work_item=conductor-phase4-config-adoption spec=specs/112-Plugin-Config-Management.md#4.1 plan=phase-4/task-4-5/r2-med-001
  try {
    config = loadPluginConfig(
      "conductor",
      DEFAULT_CONFIG,
      directory,
    ) as ConductorConfig;
  } catch (err: unknown) {
    const msg = err instanceof Error ? err.message : String(err);
    pluginWarn("conductor", `loadPluginConfig failed (${msg}), using DEFAULT_CONFIG`);
    config = structuredClone(DEFAULT_CONFIG) as ConductorConfig;
  }
  // axiom:trace work_item=conductor-phase4-config-adoption spec=specs/112-Plugin-Config-Management.md#4.1 plan=phase-4/task-4-3/inject-p4-high-03
  // Startup config dump: log effective config at DEBUG level for operator diagnostics.
  // To view: set LOG_LEVEL=debug (or check OpenCode plugin console output).
  // axiom:trace work_item=conductor-phase4-config-adoption spec=specs/112-Plugin-Config-Management.md#12 plan=phase-4/task-4-4/backlog-006
  void (client as ReturnType<typeof createOpencodeClient>)?.app?.log({
    body: { service: "conductor", level: "debug", message: "Effective config loaded",
      extra: { limits: config.limits, polling: config.polling,
               lifecycle: config.lifecycle, context_banner: config.context_banner } },
  }).catch(() => {/* best-effort */});

  // ── SPIRE availability check (moved BEFORE database init) ──────────────────
  // REQ-COND-005b: fail-closed if SPIRE unavailable and fallback not allowed
  // This MUST run before database init to avoid resolve() crashes when
  // config.database_path is unexpectedly malformed in degraded environments.
  // axiom:trace work_item=plugin-bug-sweep-01 spec=specs/107-Conductor.md#REQ-COND-005b plan=phase-2/task-2/step-verify-001
  const spireAvailable = checkSpireAvailability(config.auth.spire_socket_path);
  const fallbackAllowed = config.auth.allow_spawn_secret_fallback;

  if (!spireAvailable && !fallbackAllowed) {
    const critMsg =
      "CONDUCTOR: SPIRE unavailable and spawn_secret_fallback disabled. " +
      "For local dev, set allow_spawn_secret_fallback: true in .opencode/config/conductor.json. " +
      "See specs/107-Conductor.md#REQ-COND-005b for the security contract.";
    // axiom:trace work_item=plugin-bug-sweep-01 spec=specs/107-Conductor.md#REQ-COND-005b plan=phase-2/task-2/step-verify-001
    pluginError("conductor", `CRITICAL: ${critMsg}`);
    // REQ-COND-005b §4: throw to fail-closed. The outer ConductorPlugin barrel
    // wrapper catches this and returns { tool: {} } so OpenCode's ToolRegistry
    // doesn't crash with Object.entries on null tools.
    throw new Error(critMsg);
  }

  if (!spireAvailable && fallbackAllowed) {
    pluginWarn("conductor", "WARNING: Using spawn secret fallback (SPIRE unavailable). " +
      "This should only be used in local dev environments.");
  }

  // ── Database ──────────────────────────────────────────────────────────────
  // database_path is fully authoritative. Set via AXIOM_CONDUCTOR_DATABASE_PATH env var
  // or .opencode/config/conductor.json.
  // axiom:trace work_item=conductor-phase4-config-adoption spec=specs/112-Plugin-Config-Management.md#8 plan=phase-4/task-4-4/backlog-005
  const dbPath = resolve(directory, config.database_path);
  const dbDir = dirname(dbPath);
  if (!existsSync(dbDir)) {
    mkdirSync(dbDir, { recursive: true });
  }

  const db = openDatabase(dbPath);
  initConductorDB(db);

  // NOTE: SPIRE check moved BEFORE database init (line ~661) to avoid resolve() crashes.
  // The check already ran and returned { tool: {} } if SPIRE unavailable + fallback disabled.

  // ── Stash root ────────────────────────────────────────────────────────────
  const stashRoot = join(directory, ".memory-bank", "stash");
  for (const dir of [stashRoot, join(stashRoot, "suspended"), join(stashRoot, "closed"), join(stashRoot, "entries")]) {
    if (!existsSync(dir)) mkdirSync(dir, { recursive: true });
  }

  // Clean up orphaned .tmp files from crashed atomic JSONL writes
  const entriesDir = join(stashRoot, "entries");
  if (existsSync(entriesDir)) {
    for (const f of readdirSync(entriesDir)) {
      if (f.endsWith(".tmp")) {
        try { rmSync(join(entriesDir, f)); } catch { /* ignore */ }
      }
    }
  }

  // ── SDK client helper ─────────────────────────────────────────────────────
  // Use the injected plugin client by default — it has an internal channel that
  // avoids HTTP reentrancy (can't call back into the server during a tool call).
  // Override: set opencode_base_url in conductor config or AXIOM_OPENCODE_BASE_URL
  // env var to route to a remote or custom OpenCode server.
  function getSdkClient() {
    const configuredUrl = config.opencode_base_url || process.env.AXIOM_OPENCODE_BASE_URL;
    if (configuredUrl) {
      return createOpencodeClient({ baseUrl: configuredUrl });
    }
    return client as ReturnType<typeof createOpencodeClient>;
  }

  // ── Session helpers ───────────────────────────────────────────────────────

  async function spawnSession(
    initialMessage?: string,
    model?: string
  ): Promise<string | null> {
    try {
      const sdk = getSdkClient();
      // 5s timeout on session.create() — spawn should return fast
      const sessResp = await sdk.session.create({
        body: model ? { model } : {},
        signal: AbortSignal.timeout(5000),
      } as Parameters<typeof sdk.session.create>[0]);
      const sessionId = sessResp.data?.id ?? null;
      if (sessionId && initialMessage) {
        // promptAsync fires and returns immediately (HTTP 204) — does NOT block.
        // Uses path: { id } + body: { parts } shape matching the SDK contract.
        sdk.session.promptAsync({
          path: { id: sessionId },
          body: { parts: [{ type: "text", text: initialMessage }] },
        } as Parameters<typeof sdk.session.promptAsync>[0])
          .catch(() => {/* best-effort */});
      }
      return sessionId;
    } catch (err) {
      // Write error to OpenCode structured log so we can see it without debug env
      void (client as ReturnType<typeof createOpencodeClient>)?.app?.log({
        body: { service: "conductor", level: "error", message: `spawnSession failed: ${String(err)}` }
      }).catch(() => process.stderr.write(`[conductor] spawnSession failed: ${String(err)}\n`));
    }
    // Fallback: generate a synthetic session ID
    return `cnd_${Date.now().toString(36)}_${Math.random()
      .toString(36)
      .slice(2, 8)}`;
  }

  async function interruptSession(sessionId: string): Promise<void> {
    try {
      await getSdkClient().session.delete({ path: { id: sessionId } });
    } catch {
      /* best-effort */
    }
  }

  async function readSessionLastMessage(
    sessionId: string
  ): Promise<string | null> {
    try {
      const sdk = getSdkClient();
      const resp = await sdk.session.messages({ path: { id: sessionId } });
      const messages = resp.data ?? [];
      // Walk backwards to find the last assistant text part
      for (let i = messages.length - 1; i >= 0; i--) {
        const msg = messages[i] as Record<string, unknown>;
        if (msg.info && (msg.info as Record<string, unknown>).role === "assistant") {
          const parts = msg.parts as Array<Record<string, unknown>> | undefined;
          const text = parts?.find(p => p.type === "text")?.text as string | undefined;
          if (text) return text;
        }
      }
    } catch {
      /* best-effort */
    }
    return null;
  }

  // ── Logging helpers ───────────────────────────────────────────────────────

  function emitLifecycleEvent(
    eventType: string,
    agentId: string,
    extra: Record<string, unknown> = {}
  ): void {
    // REQ-COND-037: emit structured log events for conductor lifecycle
    // axiom:trace work_item=SWDE-45 spec=specs/107-Conductor.md#REQ-COND-037 plan=phase-2/task-2.3/step-1 jira_ref=SWDE-45
    const event = {
      event: eventType,
      agent_id: agentId,
      ts: new Date().toISOString(),
      ...extra,
    };
    // Use client.app.log for structured event logging (surfaced in OpenCode logs, not stdout)
    void (client as ReturnType<typeof createOpencodeClient>)?.app?.log({
      body: { service: "conductor", level: "debug", message: JSON.stringify(event) },
    }).catch(() => {/* best-effort */});
  }

  // ── conductor.spawn ───────────────────────────────────────────────────────
  // REQ-COND-001, REQ-COND-005, REQ-COND-005a, REQ-COND-005b, REQ-COND-020
  // axiom:trace work_item=SWDE-45 spec=specs/107-Conductor.md#REQ-COND-001 plan=phase-0/task-0.2/step-2 jira_ref=SWDE-45

  const conductorSpawnTool = tool({
    description:
      "Spawn a background agent (in its own OpenCode session) with a task and optional stash. " +
      "The agent runs invisibly — its transcript never pollutes the primary context. " +
      "Returns { agent_id, session_id, stash_id, status: 'running', timeout_at }. " +
      "Background agents call conductor.done to signal completion.",
    args: {
      name: tool.schema
        .string()
        .min(1)
        .describe("Human-readable name (e.g., 'investigate-auth')"),
      task: tool.schema
        .string()
        .min(1)
        .describe("The task/instructions for the background agent"),
      stash: tool.schema
        .string()
        .optional()
        .describe(
          "Stash ID (or name) to assign — agent writes findings here. Created if it doesn't exist."
        ),
      model: tool.schema
        .string()
        .optional()
        .describe("Model override (e.g., 'claude-sonnet-4')"),
      timeout: tool.schema
        .string()
        .optional()
        .describe(
          "Timeout duration (e.g., '30m', '1h'). Default: 60m. Agent auto-cancelled when exceeded."
        ),
      detach: tool.schema
        .boolean()
        .optional()
        .describe(
          "If true, the agent is detached — it survives session.stop and is not auto-cancelled. REQ-COND-036."
        ),
    },
    async execute(args, context) {
      // axiom:trace work_item=SWDE-45 spec=specs/107-Conductor.md#REQ-COND-001 jira_ref=SWDE-45
      try {
        const ctx = context as Record<string, unknown>;
        const spawningSessionId =
          (ctx?.sessionID as string | undefined) ??
          `primary_${Date.now().toString(36)}`;

        // ── Max concurrent agents limit (REQ-COND-009) ──────────────────────
        const runningCount = (
          db
            .prepare(
              `SELECT COUNT(*) as cnt FROM conductor_agents WHERE status='running' AND spawned_by=?`
            )
            .get(spawningSessionId) as { cnt: number }
        ).cnt;
        if (runningCount >= config.limits.max_concurrent_agents) {
          return JSON.stringify({
            error: `Max concurrent agents limit reached (${config.limits.max_concurrent_agents}). ` +
              `Wait for agents to complete or call conductor.cancel to stop some.`,
            current_count: runningCount,
          });
        }

        // ── Generate agent ID + spawn secret ─────────────────────────────────
        const agentId = generateAgentId();
        const spawnSecret = generateSpawnSecret();

        // ── CRITICAL log if fallback active (REQ-COND-005b) ─────────────────
        // REQ-COND-005b §5: emit CRITICAL log on EVERY spawn when fallback active
        // axiom:trace work_item=SWDE-45 spec=specs/107-Conductor.md#REQ-COND-005b jira_ref=SWDE-45
        if (!spireAvailable && fallbackAllowed) {
          pluginError("conductor", "CRITICAL: spawn secret fallback active", {
            event: "conductor_spawn_secret_fallback_active",
            agent_id: agentId,
            reason: "spire_unavailable",
            ts: new Date().toISOString(),
          });
        }

        // SECURITY: spawnSecret must NOT appear in any logged output after this point
        const secretHash = await hashSpawnSecret(spawnSecret);
        // SECURITY: spawnSecret must NOT appear in any logged output after this point

        // ── Resolve stash ────────────────────────────────────────────────────
        let stashId: string | null = null;
        if (args.stash) {
          stashId = slugify(args.stash);
          if (config.stash.auto_create) {
            await createConductorStash(
              stashRoot,
              stashId,
              args.stash,
              agentId,
              args.name
            );
          }
        }

        // ── Parse timeout ────────────────────────────────────────────────────
        let timeoutAt: string | null = null;
        const timeoutStr =
          args.timeout ??
          `${config.limits.default_timeout_minutes}m`;
        const timeoutMs = parseTimeoutString(timeoutStr);
        if (timeoutMs > 0) {
          timeoutAt = new Date(Date.now() + timeoutMs).toISOString();
        }

        // ── Build initial message with conductor envelope ────────────────────
        // REQ-COND-005a: envelope injected at start of task, separate from human-readable part
        const envelope = buildConductorEnvelope(agentId, stashId, spawnSecret);
        const initialMessage = `${envelope}\n\nYour task: ${args.task}`;

        // ── Spawn OpenCode session ───────────────────────────────────────────
        const sessionId = await spawnSession(initialMessage, args.model);
        if (!sessionId) {
          return JSON.stringify({ error: "Failed to spawn background session" });
        }

        // ── Record in DB ─────────────────────────────────────────────────────
        // SECURITY: only spawn_secret_hash stored — never spawnSecret itself
        const now = new Date().toISOString();
        const insertOk = sqliteWriteWithRetry(() => {
          db.prepare(`
            INSERT INTO conductor_agents
              (agent_id, name, session_id, stash_id, status, task, model,
               spawned_by, spawned_at, timeout_at, cost_usd, spawn_secret_hash, detached)
            VALUES (?, ?, ?, ?, 'running', ?, ?, ?, ?, ?, 0, ?, ?)
          `).run(
            agentId,
            args.name,
            sessionId,
            stashId,
            args.task,
            args.model ?? null,
            spawningSessionId,
            now,
            timeoutAt,
            secretHash,
            args.detach ? 1 : 0
            // NOTE: spawnSecret is NOT stored — only secretHash
          );
        }, "conductor_spawn:insert_agent");
        if (!insertOk) {
          pluginWarn("conductor", `Failed to record agent ${agentId} in DB after retries — agent is running but untracked`);
        }

        emitLifecycleEvent("agent_spawned", agentId, {
          name: args.name,
          session_id: sessionId,
          stash_id: stashId,
          spawned_by: spawningSessionId,
        });

        return JSON.stringify({
          agent_id: agentId,
          session_id: sessionId,
          stash_id: stashId,
          status: "running",
          timeout_at: timeoutAt,
          detached: !!args.detach,
        });
      } catch (err) {
        return JSON.stringify({ error: String(err) });
      }
    },
  });

  // ── conductor.done ────────────────────────────────────────────────────────
  // REQ-COND-019, REQ-COND-020
  // axiom:trace work_item=SWDE-45 spec=specs/107-Conductor.md#REQ-COND-019 plan=phase-0/task-0.3/step-1 jira_ref=SWDE-45

  const conductorDoneTool = tool({
    description:
      "Background agents call this tool to signal task completion. " +
      "Requires the spawn_secret received in the conductor_envelope. " +
      "On success: writes a result entry to the assigned stash and marks the agent done. " +
      "Returns { status: 'done', agent_id } on success or { error, code: 403 } on auth failure.",
    args: {
      agent_id: tool.schema
        .string()
        .min(1)
        .describe("Your agent_id from the conductor_envelope"),
      secret: tool.schema
        .string()
        .min(1)
        .describe("The spawn_secret from the conductor_envelope"),
      summary: tool.schema
        .string()
        .min(1)
        .describe("Brief result summary (< 500 chars) for the dashboard"),
      result_type: tool.schema
        .enum(["finding", "decision", "summary", "error"])
        .optional()
        .default("summary")
        .describe("Type of result"),
      details: tool.schema
        .string()
        .optional()
        .describe("Full result details (optional)"),
      cost_usd: tool.schema
        .number()
        .optional()
        .describe("Approximate cost in USD for this agent's work"),
    },
    async execute(args) {
      // axiom:trace work_item=SWDE-45 spec=specs/107-Conductor.md#REQ-COND-020 jira_ref=SWDE-45
      try {
        // ── Look up agent ────────────────────────────────────────────────────
        const row = db
          .prepare(
            `SELECT agent_id, name, session_id, stash_id, status, spawn_secret_hash
             FROM conductor_agents WHERE agent_id=?`
          )
          .get(args.agent_id) as
          | {
              agent_id: string;
              name: string;
              session_id: string;
              stash_id: string | null;
              status: string;
              spawn_secret_hash: string;
            }
          | undefined;

        if (!row) {
          return JSON.stringify({
            error: `Unknown agent_id: ${args.agent_id}`,
            code: 404,
          });
        }

        if (row.status !== "running") {
          return JSON.stringify({
            error: `Agent ${args.agent_id} is already ${row.status}`,
            code: 409,
          });
        }

        // ── Verify spawn secret (constant-time) ──────────────────────────────
        // REQ-COND-020: constant-time comparison, stored hash only, never the secret
        const providedHash = await hashSpawnSecret(args.secret);
        const secretValid = constantTimeCompareHex(
          row.spawn_secret_hash,
          providedHash
        );

        if (!secretValid) {
          // SECURITY: do NOT include the provided secret or hash in the error response
          return JSON.stringify({
            error: "Invalid spawn secret — authorization denied",
            code: 403,
          });
        }

        // ── Update DB ────────────────────────────────────────────────────────
        const now = new Date().toISOString();
        const summaryTruncated = args.summary.slice(0, 500);
        sqliteWriteWithRetry(() => {
          db.prepare(`
            UPDATE conductor_agents
            SET status='done', completed_at=?, result_summary=?, result_type=?,
                cost_usd=COALESCE(?, cost_usd)
            WHERE agent_id=?
          `).run(
            now,
            summaryTruncated,
            args.result_type ?? "summary",
            args.cost_usd ?? null,
            args.agent_id
          );
        }, "conductor_done:update_status");

        // ── Write stash result entry ─────────────────────────────────────────
        if (row.stash_id) {
          await appendStashEntry(stashRoot, row.stash_id, {
            ts: now,
            agent: row.agent_id,
            agent_name: row.name,
            type: "result",
            result_type: args.result_type ?? "summary",
            summary: summaryTruncated,
            details: args.details,
            cost_usd: args.cost_usd,
          });
        }

        emitLifecycleEvent("agent_done", args.agent_id, {
          name: row.name,
          result_type: args.result_type,
          stash_id: row.stash_id,
        });

        return JSON.stringify({
          status: "done",
          agent_id: args.agent_id,
          name: row.name,
          stash_id: row.stash_id,
        });
      } catch (err) {
        return JSON.stringify({ error: String(err) });
      }
    },
  });

  // ── conductor.status ──────────────────────────────────────────────────────
  // REQ-COND-003
  // axiom:trace work_item=SWDE-45 spec=specs/107-Conductor.md#REQ-COND-003 plan=phase-1/task-1.1/step-1 jira_ref=SWDE-45

  const conductorStatusTool = tool({
    description:
      "Show a one-line dashboard of all background agents for the current session. " +
      "Non-verbose — doesn't pollute context. Shows agent name, status, elapsed time, and cost. " +
      "The spawn_secret_hash is NEVER included in the output. " +
      "Returns formatted dashboard text.",
    args: {
      all: tool.schema
        .boolean()
        .optional()
        .describe("Show agents from all sessions, not just the current one"),
    },
    async execute(args, context) {
      // axiom:trace work_item=SWDE-45 spec=specs/107-Conductor.md#REQ-COND-003 jira_ref=SWDE-45
      try {
        const ctx = context as Record<string, unknown>;
        const sessionId = ctx?.sessionID as string | undefined;

        let rows: {
          agent_id: string;
          name: string;
          status: string;
          spawned_at: string;
          completed_at: string | null;
          cost_usd: number;
          result_summary: string | null;
          stash_id: string | null;
        }[];

        if (args.all || !sessionId) {
          rows = db
            .prepare(
              `SELECT agent_id, name, status, spawned_at, completed_at, cost_usd, result_summary, stash_id
               FROM conductor_agents ORDER BY spawned_at DESC`
            )
            .all() as typeof rows;
        } else {
          rows = db
            .prepare(
              `SELECT agent_id, name, status, spawned_at, completed_at, cost_usd, result_summary, stash_id
               FROM conductor_agents WHERE spawned_by=? ORDER BY spawned_at DESC`
            )
            .all(sessionId) as typeof rows;
        }

        if (rows.length === 0) {
          return JSON.stringify({
            dashboard: "[Conductor Dashboard]\n  (no background agents)\n",
            count: 0,
          });
        }

        const lines = ["[Conductor Dashboard]"];
        let totalCost = 0;
        let runningCount = 0;
        let doneCount = 0;

        for (const row of rows) {
          totalCost += row.cost_usd;
          const now = Date.now();
          const spawnedMs = new Date(row.spawned_at).getTime();
          const elapsed = formatElapsed(now - spawnedMs);

          let statusLine: string;
          if (row.status === "running") {
            runningCount++;
            statusLine = `running ${elapsed}`;
          } else if (row.status === "done") {
            doneCount++;
            const summary = row.result_summary
              ? ` — ${row.result_summary.slice(0, 80)}`
              : "";
            statusLine = `DONE${summary}`;
          } else if (row.status === "cancelled") {
            statusLine = `CANCELLED (${elapsed})`;
          } else if (row.status === "failed") {
            statusLine = `FAILED (${elapsed})`;
          } else {
            statusLine = row.status;
          }

          const costStr = config.context_banner.show_cost
            ? ` [cost: $${row.cost_usd.toFixed(3)}]`
            : "";
          // SECURITY: spawn_secret_hash intentionally EXCLUDED from output (REQ-COND-020)
          lines.push(
            `  ${row.agent_id} (${row.name}): ${statusLine}${costStr}`
          );
        }

        const costStr = config.context_banner.show_cost
          ? ` | Total cost: $${totalCost.toFixed(3)}`
          : "";
        lines.push(
          `\nTotal: ${rows.length} agents | ${doneCount} done | ${runningCount} running${costStr}`
        );

        return JSON.stringify({
          dashboard: lines.join("\n") + "\n",
          count: rows.length,
          running: runningCount,
          done: doneCount,
          total_cost_usd: totalCost,
        });
      } catch (err) {
        return JSON.stringify({ error: String(err) });
      }
    },
  });

  // ── conductor.result ─────────────────────────────────────────────────────
  // REQ-COND-004
  // axiom:trace work_item=SWDE-45 spec=specs/107-Conductor.md#REQ-COND-004 plan=phase-1/task-1.2/step-1 jira_ref=SWDE-45

  const conductorResultTool = tool({
    description:
      "Retrieve the result from a background agent (opt-in, on demand). " +
      "If the agent was assigned a stash, returns the stash peek content. " +
      "Otherwise returns the last assistant message from the session. " +
      "Returns { agent_id, name, status, result_summary, stash_content?, session_message? }.",
    args: {
      id: tool.schema
        .string()
        .min(1)
        .describe("Agent ID (e.g., 'bg_abc123')"),
    },
    async execute(args) {
      // axiom:trace work_item=SWDE-45 spec=specs/107-Conductor.md#REQ-COND-004 jira_ref=SWDE-45
      try {
        const row = db
          .prepare(
            `SELECT agent_id, name, session_id, stash_id, status, result_summary, result_type, cost_usd
             FROM conductor_agents WHERE agent_id=?`
          )
          .get(args.id) as
          | {
              agent_id: string;
              name: string;
              session_id: string;
              stash_id: string | null;
              status: string;
              result_summary: string | null;
              result_type: string | null;
              cost_usd: number;
            }
          | undefined;

        if (!row) {
          return JSON.stringify({
            error: `Unknown agent_id: ${args.id}`,
          });
        }

        const result: Record<string, unknown> = {
          agent_id: row.agent_id,
          name: row.name,
          status: row.status,
          result_summary: row.result_summary,
          result_type: row.result_type,
          cost_usd: row.cost_usd,
        };

        if (row.stash_id) {
          // Stash path: use stash peek
          const peek = peekStash(stashRoot, row.stash_id);
          result.stash_id = row.stash_id;
          result.stash_found = peek.found;
          if (peek.found) {
            result.stash_name = peek.name;
            result.stash_summary = peek.summary;
            result.stash_result_entry = peek.result_entry ?? null;
          }
        } else {
          // Session path: read last assistant message
          const lastMsg = await readSessionLastMessage(row.session_id);
          result.session_message = lastMsg;
        }

        return JSON.stringify(result);
      } catch (err) {
        return JSON.stringify({ error: String(err) });
      }
    },
  });

  // ── conductor.cancel ─────────────────────────────────────────────────────
  // REQ-COND-018
  // axiom:trace work_item=SWDE-45 spec=specs/107-Conductor.md#REQ-COND-018 plan=phase-1/task-1.3/step-1 jira_ref=SWDE-45

  const conductorCancelTool = tool({
    description:
      "Stop a background agent. " +
      "Interrupts the OpenCode session and marks the agent as 'cancelled'. " +
      "Returns { cancelled: true, agent_id, name } on success.",
    args: {
      id: tool.schema.string().min(1).describe("Agent ID to cancel"),
      reason: tool.schema
        .string()
        .optional()
        .describe("Reason for cancellation (recorded in DB)"),
    },
    async execute(args) {
      // axiom:trace work_item=SWDE-45 spec=specs/107-Conductor.md#REQ-COND-018 jira_ref=SWDE-45
      try {
        const row = db
          .prepare(
            `SELECT agent_id, name, session_id, status FROM conductor_agents WHERE agent_id=?`
          )
          .get(args.id) as
          | { agent_id: string; name: string; session_id: string; status: string }
          | undefined;

        if (!row) {
          return JSON.stringify({ error: `Unknown agent_id: ${args.id}` });
        }

        if (row.status !== "running") {
          return JSON.stringify({
            cancelled: false,
            reason: `Agent is already ${row.status}`,
            agent_id: args.id,
          });
        }

        // Interrupt the session
        await interruptSession(row.session_id);

        // Update DB
        const now = new Date().toISOString();
        sqliteWriteWithRetry(() => {
          db.prepare(`
            UPDATE conductor_agents
            SET status='cancelled', completed_at=?, error=?
            WHERE agent_id=?
          `).run(now, args.reason ?? "cancelled by primary agent", args.id);
        }, "conductor_cancel:update_status");

        emitLifecycleEvent("agent_cancelled", args.id, {
          name: row.name,
          reason: args.reason,
        });

        return JSON.stringify({
          cancelled: true,
          agent_id: args.id,
          name: row.name,
        });
      } catch (err) {
        return JSON.stringify({ error: String(err) });
      }
    },
  });

  // ── conductor.wait ────────────────────────────────────────────────────────
  // REQ-COND-017
  // axiom:trace work_item=SWDE-45 spec=specs/107-Conductor.md#REQ-COND-017 plan=phase-1/task-1.3/step-1 jira_ref=SWDE-45

  const conductorWaitTool = tool({
    description:
      "Block the current turn until a specific background agent completes (or timeout). " +
      "Polls every 5 seconds. Returns { status, agent_id, result_summary } when done or timed out.",
    args: {
      id: tool.schema.string().min(1).describe("Agent ID to wait for"),
      timeout: tool.schema
        .string()
        .optional()
        .default("5m")
        .describe(
          "Maximum wait time (e.g., '5m', '30m'). Default: 5m."
        ),
    },
    async execute(args) {
      // axiom:trace work_item=SWDE-45 spec=specs/107-Conductor.md#REQ-COND-017 jira_ref=SWDE-45
      try {
        const timeoutMs = parseTimeoutString(args.timeout ?? "5m");
        const deadline = Date.now() + timeoutMs;
        const pollIntervalMs =
          config.polling.completion_check_interval_seconds * 1000;

        while (Date.now() < deadline) {
          const row = db
            .prepare(
              `SELECT agent_id, name, status, result_summary, result_type, cost_usd
               FROM conductor_agents WHERE agent_id=?`
            )
            .get(args.id) as
            | {
                agent_id: string;
                name: string;
                status: string;
                result_summary: string | null;
                result_type: string | null;
                cost_usd: number;
              }
            | undefined;

          if (!row) {
            return JSON.stringify({ error: `Unknown agent_id: ${args.id}` });
          }

          if (row.status !== "running") {
            return JSON.stringify({
              agent_id: row.agent_id,
              name: row.name,
              status: row.status,
              result_summary: row.result_summary,
              result_type: row.result_type,
              cost_usd: row.cost_usd,
            });
          }

          // Wait for next poll
          await new Promise<void>((resolve) =>
            setTimeout(resolve, Math.min(pollIntervalMs, deadline - Date.now()))
          );
        }

        // Timed out
        const row = db
          .prepare(
            `SELECT agent_id, name, status FROM conductor_agents WHERE agent_id=?`
          )
          .get(args.id) as
          | { agent_id: string; name: string; status: string }
          | undefined;
        return JSON.stringify({
          agent_id: args.id,
          name: row?.name,
          status: row?.status ?? "unknown",
          timed_out: true,
          message: `Wait timed out after ${args.timeout ?? "5m"}. Agent may still be running.`,
        });
      } catch (err) {
        return JSON.stringify({ error: String(err) });
      }
    },
  });

  // ── system.transform — context banner ────────────────────────────────────
  // REQ-COND-006
  // axiom:trace work_item=SWDE-45 spec=specs/107-Conductor.md#REQ-COND-006 plan=phase-2/task-2.1/step-1 jira_ref=SWDE-45

  const systemTransformHook = async (
    input: { sessionID?: string; model: unknown },
    output: { system: string[] }
  ): Promise<void> => {
    if (!config.context_banner.enabled) return;

    try {
      const sessionId = input.sessionID;
      if (!sessionId) return;

      // Query running agents spawned by this session
      const agents = db
        .prepare(
          `SELECT agent_id, name, status, spawned_at, cost_usd
           FROM conductor_agents WHERE spawned_by=? ORDER BY spawned_at DESC LIMIT 20`
        )
        .all(sessionId) as {
        agent_id: string;
        name: string;
        status: string;
        spawned_at: string;
        cost_usd: number;
      }[];

      if (agents.length === 0) return;

      const running = agents.filter((a) => a.status === "running");
      const done = agents.filter((a) => a.status === "done");
      const now = Date.now();

      let bannerParts = [`${agents.length} bg agents`];

      if (config.context_banner.verbosity !== "minimal") {
        // Add brief per-agent status
        for (const agent of agents.slice(0, 5)) {
          // cap at 5 in banner
          if (agent.status === "running") {
            const elapsed = formatElapsed(
              now - new Date(agent.spawned_at).getTime()
            );
            bannerParts.push(`${agent.name}: running ${elapsed}`);
          } else if (agent.status === "done") {
            bannerParts.push(`${agent.name}: DONE`);
          }
        }
      }

      if (config.context_banner.show_cost) {
        const totalCost = agents.reduce((s, a) => s + a.cost_usd, 0);
        bannerParts.push(`total cost: $${totalCost.toFixed(3)}`);
      }

      const banner = `[Conductor: ${bannerParts.join(" | ")}]`;
      output.system.push(banner);

      // REQ-COND-012: append Pinned stashes section when any stashes are pinned for this session
      try {
        const pinned = db.prepare(
          `SELECT stash_id FROM conductor_focused_stashes WHERE session_id=? AND pinned=1 ORDER BY focused_at`
        ).all(sessionId) as { stash_id: string }[];
        if (pinned.length > 0) {
          output.system.push(`[Pinned: ${pinned.map((p) => p.stash_id).join(", ")}]`);
        }
      } catch { /* non-fatal */ }
    } catch {
      /* non-fatal — banner is optional */
    }
  };

  // ── Completion polling ────────────────────────────────────────────────────
  // REQ-COND-002, REQ-COND-007
  // axiom:trace work_item=SWDE-45 spec=specs/107-Conductor.md#REQ-COND-002 plan=phase-2/task-2.2/step-1 jira_ref=SWDE-45

  let pollingTimer: ReturnType<typeof setInterval> | null = null;

  function startCompletionPolling(): void {
    const intervalMs =
      config.polling.completion_check_interval_seconds * 1000;

    pollingTimer = setInterval(async () => {
      try {
        // Check for running agents
        const runningAgents = db
          .prepare(
            `SELECT agent_id, name, stash_id, timeout_at FROM conductor_agents WHERE status='running'`
          )
          .all() as {
          agent_id: string;
          name: string;
          stash_id: string | null;
          timeout_at: string | null;
        }[];

        const now = new Date();

        for (const agent of runningAgents) {
          // Check for timeout
          if (agent.timeout_at) {
            const timeoutAt = new Date(agent.timeout_at);
            if (now > timeoutAt) {
              sqliteWriteWithRetry(() => {
                db.prepare(`
                  UPDATE conductor_agents
                  SET status='cancelled', completed_at=?, error='timeout'
                  WHERE agent_id=?
                `).run(now.toISOString(), agent.agent_id);
              }, "conductor_wait:update_completed");
              emitLifecycleEvent("agent_timeout", agent.agent_id, {
                name: agent.name,
              });
              continue;
            }
          }

          // Check stash for completion signals
          if (agent.stash_id) {
            const closed = isStashClosed(stashRoot, agent.stash_id);
            const hasResult = stashHasResultEntry(stashRoot, agent.stash_id);

            if (closed || hasResult) {
              sqliteWriteWithRetry(() => {
                db.prepare(`
                  UPDATE conductor_agents
                  SET status='done', completed_at=?
                  WHERE agent_id=? AND status='running'
                `).run(now.toISOString(), agent.agent_id);
              }, "conductor_wait:update_status");

              emitLifecycleEvent("agent_done", agent.agent_id, {
                name: agent.name,
                detected_via: closed ? "stash_closed" : "result_entry",
              });
            }
          }
        }
      } catch {
        /* non-fatal polling errors */
      }
    }, intervalMs);
  }

  startCompletionPolling();

  // ── Phase 2: conductor.collect ────────────────────────────────────────────
  // REQ-COND-015: wait for all agents in the current session to reach a terminal state
  const conductorCollectTool = tool({
    description:
      "Wait for all spawned agents in the current session to finish. " +
      "Returns { all_done, timed_out, agents, total }. " +
      "Terminal statuses: done, failed, cancelled. " +
      "Pass all:true to include agents from all sessions.",
    args: {
      timeout: tool.schema.string().optional().describe("Max wait time (e.g. '30s', '5m'). Default: 5m."),
      all: tool.schema.boolean().optional().describe("Include agents from all sessions, not just current."),
    },
    async execute(args, context) {
      try {
        const ctx = context as Record<string, unknown>;
        const sessionId = ctx?.sessionID as string | undefined;
        const timeoutMs = parseTimeoutString((args.timeout as string | undefined) ?? "5m") || (5 * 60 * 1000);
        const pollIntervalMs = 100;
        const deadline = Date.now() + timeoutMs;

        const getAgents = () => {
          if (args.all) {
            return db.prepare(
              `SELECT agent_id, name, status, stash_id, result_summary, cost_usd FROM conductor_agents`
            ).all() as { agent_id: string; name: string; status: string; stash_id: string | null; result_summary: string | null; cost_usd: number }[];
          }
          return db.prepare(
            `SELECT agent_id, name, status, stash_id, result_summary, cost_usd FROM conductor_agents WHERE spawned_by=?`
          ).all(sessionId ?? "") as { agent_id: string; name: string; status: string; stash_id: string | null; result_summary: string | null; cost_usd: number }[];
        };

        const isTerminal = (s: string) => s === "done" || s === "failed" || s === "cancelled";

        while (true) {
          const agents = getAgents();
          const allDone = agents.every((a) => isTerminal(a.status));
          if (allDone || Date.now() >= deadline) {
            return JSON.stringify({
              all_done: allDone,
              timed_out: !allDone && Date.now() >= deadline,
              agents,
              total: agents.length,
            });
          }
          await new Promise((r) => setTimeout(r, pollIntervalMs));
        }
      } catch (err) {
        return JSON.stringify({ error: String(err) });
      }
    },
  });

  // ── Phase 2: conductor.broadcast ─────────────────────────────────────────
  // REQ-COND-014: send a message to all running agents in the current session
  const conductorBroadcastTool = tool({
    description:
      "Send a message to all currently running agents. " +
      "Returns { sent, failed, agents: [{ agent_id, delivered }] }.",
    args: {
      message: tool.schema.string().min(1).describe("Message to deliver to each running agent."),
      all: tool.schema.boolean().optional().describe("Broadcast to agents from all sessions, not just current."),
    },
    async execute(args, context) {
      try {
        const ctx = context as Record<string, unknown>;
        const sessionId = ctx?.sessionID as string | undefined;

        const runningAgents = args.all
          ? (db.prepare(`SELECT agent_id, session_id FROM conductor_agents WHERE status='running'`).all() as { agent_id: string; session_id: string }[])
          : (db.prepare(`SELECT agent_id, session_id FROM conductor_agents WHERE status='running' AND spawned_by=?`).all(sessionId ?? "") as { agent_id: string; session_id: string }[]);

        const results: { agent_id: string; delivered: boolean }[] = [];
        let sent = 0;
        let failed = 0;

        await Promise.all(runningAgents.map(async (agent) => {
          try {
            const sdk = getSdkClient();
            // promptAsync — delivers the message and returns immediately without
            // blocking on the model response. Broadcast just needs delivery, not reply.
            await sdk.session.promptAsync({
              path: { id: agent.session_id },
              body: { parts: [{ type: "text", text: args.message }] },
            } as Parameters<typeof sdk.session.promptAsync>[0]);
            sent++;
            results.push({ agent_id: agent.agent_id, delivered: true });
          } catch {
            failed++;
            results.push({ agent_id: agent.agent_id, delivered: false });
          }
        }));

        return JSON.stringify({ sent, failed, agents: results });
      } catch (err) {
        return JSON.stringify({ error: String(err) });
      }
    },
  });

  // ── Phase 2: conductor.delegate ──────────────────────────────────────────
  // REQ-COND-013: spawn a background agent with an auto-created stash for context handoff
  const conductorDelegateTool = tool({
    description:
      "Spawn a background agent and hand off context via an auto-created stash. " +
      "Writes context as a JSONL 'context' entry. " +
      "Returns { agent_id, session_id, stash_id, status, delegated:true }.",
    args: {
      name: tool.schema.string().min(1).describe("Agent name."),
      task: tool.schema.string().min(1).describe("Task instructions for the agent."),
      context: tool.schema.string().optional().describe("Context to hand off (written to stash as a 'context' entry)."),
      stash: tool.schema.string().optional().describe("Explicit stash name. Auto-generated if omitted."),
      model: tool.schema.string().optional().describe("Model override."),
      timeout: tool.schema.string().optional().describe("Timeout (e.g. '30m')."),
    },
    async execute(args, context) {
      try {
        const ctx = context as Record<string, unknown>;
        const sessionId = ctx?.sessionID as string | undefined;

        // Concurrency check
        const runningCount = (db.prepare(`SELECT COUNT(*) as cnt FROM conductor_agents WHERE status='running'`).get() as { cnt: number }).cnt;
        if (runningCount >= config.limits.max_concurrent_agents) {
          return JSON.stringify({ error: `Max concurrent agents (${config.limits.max_concurrent_agents}) reached` });
        }

        const agentId = generateAgentId();
        const stashId = args.stash ? slugify(String(args.stash)) : `delegate-${agentId.replace(/_/g, "-")}`;

        // Create stash and write context entry
        await createConductorStash(stashRoot, stashId, args.stash ?? stashId, agentId, String(args.name));
        if (args.context) {
          const summary = String(args.context).slice(0, 500);
          const details = String(args.context).length > 500 ? String(args.context) : undefined;
          await appendStashEntry(stashRoot, stashId, {
            ts: new Date().toISOString(),
            agent: agentId,
            type: "context",
            summary,
            ...(details ? { details } : {}),
          });
        }

        const spawnSecret = generateSpawnSecret();
        const secretHash = await hashSpawnSecret(spawnSecret);
        const envelope = buildConductorEnvelope(agentId, stashId, spawnSecret);
        const contextSection = args.context ? `\n\nContext from delegating agent:\n${args.context}` : "";
        const initialMessage = `${envelope}\n\nYour task: ${args.task}${contextSection}`;

        const sessionIdSpawned = await spawnSession(initialMessage, args.model);
        if (!sessionIdSpawned) return JSON.stringify({ error: "Failed to spawn session" });

        const now = new Date().toISOString();
        const timeoutMs = parseTimeoutString((args.timeout as string | undefined) ?? `${config.limits.default_timeout_minutes}m`);
        const timeoutAt = timeoutMs > 0 ? new Date(Date.now() + timeoutMs).toISOString() : null;

        sqliteWriteWithRetry(() => {
          db.prepare(`
            INSERT INTO conductor_agents
              (agent_id, name, session_id, stash_id, status, task, model,
               spawned_by, spawned_at, timeout_at, cost_usd, spawn_secret_hash, detached)
            VALUES (?, ?, ?, ?, 'running', ?, ?, ?, ?, ?, 0, ?, 0)
          `).run(agentId, args.name, sessionIdSpawned, stashId, args.task, args.model ?? null, sessionId ?? "", now, timeoutAt, secretHash);
        }, "conductor_delegate:insert");

        return JSON.stringify({ agent_id: agentId, session_id: sessionIdSpawned, stash_id: stashId, status: "running", delegated: true });
      } catch (err) {
        return JSON.stringify({ error: String(err) });
      }
    },
  });

  // ── Phase 2: conductor.relay ─────────────────────────────────────────────
  // REQ-COND-016: spawn a new agent using the result of a completed source agent as context
  const conductorRelayTool = tool({
    description:
      "Spawn a new agent using the result of a completed source agent as context. " +
      "If source is not yet done, waits up to wait_timeout then proceeds anyway. " +
      "Returns { agent_id, session_id, status, relay_from, source_status }.",
    args: {
      from: tool.schema.string().min(1).describe("agent_id of the source agent to relay from."),
      name: tool.schema.string().min(1).describe("Name for the new agent."),
      task: tool.schema.string().min(1).describe("Task for the new agent."),
      wait_timeout: tool.schema.string().optional().describe("How long to wait for source to complete (e.g. '30s'). Default: 0s (proceed immediately)."),
      model: tool.schema.string().optional().describe("Model override."),
      timeout: tool.schema.string().optional().describe("Timeout for new agent."),
    },
    async execute(args, context) {
      try {
        const ctx = context as Record<string, unknown>;
        const sessionId = ctx?.sessionID as string | undefined;

        // Check source agent exists
        const source = db.prepare(`SELECT agent_id, status, result_summary, stash_id, name FROM conductor_agents WHERE agent_id=?`)
          .get(args.from) as { agent_id: string; status: string; result_summary: string | null; stash_id: string | null; name: string } | undefined;
        if (!source) return JSON.stringify({ error: `Unknown source agent_id: ${args.from}` });

        // Concurrency check
        const runningCount = (db.prepare(`SELECT COUNT(*) as cnt FROM conductor_agents WHERE status='running'`).get() as { cnt: number }).cnt;
        if (runningCount >= config.limits.max_concurrent_agents) {
          return JSON.stringify({ error: `Max concurrent agents (${config.limits.max_concurrent_agents}) reached` });
        }

        // Optionally wait for source to complete
        const waitMs = parseTimeoutString((args.wait_timeout as string | undefined) ?? "0s");
        if (waitMs > 0 && source.status === "running") {
          const deadline = Date.now() + waitMs;
          while (Date.now() < deadline) {
            await new Promise((r) => setTimeout(r, 100));
            const refreshed = db.prepare(`SELECT status FROM conductor_agents WHERE agent_id=?`).get(args.from) as { status: string } | undefined;
            if (refreshed && refreshed.status !== "running") break;
          }
        }

        // Re-read source after possible wait
        const sourceNow = db.prepare(`SELECT status, result_summary, stash_id FROM conductor_agents WHERE agent_id=?`)
          .get(args.from) as { status: string; result_summary: string | null; stash_id: string | null } | undefined;

        // Build context from source result or stash
        let contextSection = "";
        if (sourceNow?.result_summary) {
          contextSection = `\n\nContext from ${source.name} (${source.agent_id}):\n${sourceNow.result_summary}`;
        } else if (sourceNow?.stash_id) {
          const peek = peekStash(stashRoot, sourceNow.stash_id);
          if (peek.found && peek.result_entry) {
            contextSection = `\n\nContext from ${source.name} stash:\n${peek.result_entry.summary ?? ""}`;
          }
        }

        const agentId = generateAgentId();
        const spawnSecret = generateSpawnSecret();
        const secretHash = await hashSpawnSecret(spawnSecret);
        const envelope = buildConductorEnvelope(agentId, null, spawnSecret);
        const initialMessage = `${envelope}\n\nYour task: ${args.task}${contextSection}`;

        const sessionIdSpawned = await spawnSession(initialMessage, args.model);
        if (!sessionIdSpawned) return JSON.stringify({ error: "Failed to spawn session" });

        const now = new Date().toISOString();
        const timeoutMs = parseTimeoutString((args.timeout as string | undefined) ?? `${config.limits.default_timeout_minutes}m`);
        const timeoutAt = timeoutMs > 0 ? new Date(Date.now() + timeoutMs).toISOString() : null;

        sqliteWriteWithRetry(() => {
          db.prepare(`
            INSERT INTO conductor_agents
              (agent_id, name, session_id, stash_id, status, task, model,
               spawned_by, spawned_at, timeout_at, cost_usd, spawn_secret_hash, detached)
            VALUES (?, ?, ?, ?, 'running', ?, ?, ?, ?, ?, 0, ?, 0)
          `).run(agentId, args.name, sessionIdSpawned, null, args.task, args.model ?? null, sessionId ?? "", now, timeoutAt, secretHash);
        }, "conductor_relay:insert");

        return JSON.stringify({
          agent_id: agentId,
          session_id: sessionIdSpawned,
          status: "running",
          relay_from: args.from,
          source_status: sourceNow?.status ?? source.status,
        });
      } catch (err) {
        return JSON.stringify({ error: String(err) });
      }
    },
  });

  // ── Phase 2: conductor.focus / unfocus / pin ──────────────────────────────
  // REQ-COND-010/011/012: stash focus/pin management for context banner
  const conductorFocusTool = tool({
    description: "Mark a stash as focused for the current session. Focused stashes appear in the context banner. Returns { stash_id, focused:true, found }.",
    args: { stash: tool.schema.string().min(1).describe("Stash ID to focus.") },
    async execute(args, context) {
      try {
        const ctx = context as Record<string, unknown>;
        const sessionId = (ctx?.sessionID as string | undefined) ?? "";
        const stashId = slugify(String(args.stash));
        const found = existsSync(join(stashRoot, "suspended", `${stashId}.md`)) ||
                      existsSync(join(stashRoot, "active", `${stashId}.yaml`)) ||
                      existsSync(join(stashRoot, "closed", `${stashId}.md`));
        const now = new Date().toISOString();

        // Upsert: update focused_at if exists, insert if not
        const existing = db.prepare(`SELECT stash_id FROM conductor_focused_stashes WHERE session_id=? AND stash_id=?`).get(sessionId, stashId);
        if (existing) {
          db.prepare(`UPDATE conductor_focused_stashes SET focused_at=? WHERE session_id=? AND stash_id=?`).run(now, sessionId, stashId);
        } else {
          db.prepare(`INSERT INTO conductor_focused_stashes (stash_id, session_id, focused_at, pinned) VALUES (?, ?, ?, 0)`).run(stashId, sessionId, now);
        }

        const peek = found ? peekStash(stashRoot, stashId) : null;
        return JSON.stringify({ stash_id: stashId, focused: true, found, ...(peek?.found ? { peek: peek.result_entry } : {}) });
      } catch (err) {
        return JSON.stringify({ error: String(err) });
      }
    },
  });

  const conductorUnfocusTool = tool({
    description: "Remove focus from a stash for the current session. Idempotent. Returns { stash_id, unfocused:true }.",
    args: { stash: tool.schema.string().min(1).describe("Stash ID to unfocus.") },
    async execute(args, context) {
      try {
        const ctx = context as Record<string, unknown>;
        const sessionId = (ctx?.sessionID as string | undefined) ?? "";
        const stashId = slugify(String(args.stash));
        db.prepare(`DELETE FROM conductor_focused_stashes WHERE session_id=? AND stash_id=?`).run(sessionId, stashId);
        return JSON.stringify({ stash_id: stashId, unfocused: true });
      } catch (err) {
        return JSON.stringify({ error: String(err) });
      }
    },
  });

  const conductorPinTool = tool({
    description: "Pin a stash so it always appears in the context banner. Idempotent. Returns { stash_id, pinned:true }.",
    args: { stash: tool.schema.string().min(1).describe("Stash ID to pin.") },
    async execute(args, context) {
      try {
        const ctx = context as Record<string, unknown>;
        const sessionId = (ctx?.sessionID as string | undefined) ?? "";
        const stashId = slugify(String(args.stash));
        const now = new Date().toISOString();

        const existing = db.prepare(`SELECT stash_id FROM conductor_focused_stashes WHERE session_id=? AND stash_id=?`).get(sessionId, stashId);
        if (existing) {
          db.prepare(`UPDATE conductor_focused_stashes SET pinned=1, focused_at=? WHERE session_id=? AND stash_id=?`).run(now, sessionId, stashId);
        } else {
          db.prepare(`INSERT INTO conductor_focused_stashes (stash_id, session_id, focused_at, pinned) VALUES (?, ?, ?, 1)`).run(stashId, sessionId, now);
        }

        return JSON.stringify({ stash_id: stashId, pinned: true });
      } catch (err) {
        return JSON.stringify({ error: String(err) });
      }
    },
  });

  // ── Return plugin hooks ───────────────────────────────────────────────────
  return {
    tool: {
      "conductor_spawn": conductorSpawnTool,
      "conductor_done": conductorDoneTool,
      "conductor_status": conductorStatusTool,
      "conductor_result": conductorResultTool,
      "conductor_cancel": conductorCancelTool,
      "conductor_wait": conductorWaitTool,
      "conductor_collect": conductorCollectTool,
      "conductor_broadcast": conductorBroadcastTool,
      "conductor_delegate": conductorDelegateTool,
      "conductor_relay": conductorRelayTool,
      "conductor_focus": conductorFocusTool,
      "conductor_unfocus": conductorUnfocusTool,
      "conductor_pin": conductorPinTool,
    },
    "experimental.chat.system.transform": systemTransformHook,
    // Clean up polling on stop
    event: async (input: { event: { type?: string; properties?: Record<string, unknown> } }) => {
      const eventType = input.event?.type;
      if (eventType === "session.idle") {
        // future: heartbeat update
        return;
      }
      if (eventType === "session.stop" || eventType === "stop") {
        if (pollingTimer) {
          clearInterval(pollingTimer);
          pollingTimer = null;
        }
        // REQ-COND-035: cancel all running agents when primary session ends
        if (config.lifecycle.cancel_on_session_end) {
          const sessionId =
            (input.event as Record<string, unknown>)?.sessionID as string | undefined ??
            (input.event?.properties?.sessionID as string | undefined);
          if (sessionId) {
            try {
              const runningAgents = db.prepare(
                `SELECT agent_id, session_id FROM conductor_agents WHERE status='running' AND spawned_by=? AND detached=0`
              ).all(sessionId) as { agent_id: string; session_id: string }[];
              // REQ-COND-035: parallel cancellation — bounded by single 3s timeout, not N × 3s
              // axiom:trace work_item=SWDE-45 spec=specs/107-Conductor.md#REQ-COND-035 plan=verify3-injected/H2
              await Promise.all(
                runningAgents.map(async (agent) => {
                  try {
                    await interruptSession(agent.session_id);
                    sqliteWriteWithRetry(() => {
                      db.prepare(`UPDATE conductor_agents SET status='cancelled', completed_at=?, error='primary session ended' WHERE agent_id=?`)
                        .run(new Date().toISOString(), agent.agent_id);
                    }, "conductor_cleanup:update_cancelled");
                    emitLifecycleEvent("agent_cancelled", agent.agent_id, {
                      reason: "primary_session_ended",
                    });
                  } catch { /* non-fatal per agent */ }
                })
              );
            } catch {
              /* non-fatal — best effort on session end */
            }
          }
        }
      }
    },
  };
};

// ─────────────────────────────────────────────────────────────────────────────
// Timeout string parser (exported for testing)
// ─────────────────────────────────────────────────────────────────────────────

/**
 * Parse a timeout string like "30m", "1h", "90s" into milliseconds.
 * Returns 0 on parse failure (no timeout).
 */
export function parseTimeoutString(s: string): number {
  const match = s.trim().match(/^(\d+(?:\.\d+)?)\s*([smh]?)$/i);
  if (!match) return 0;
  const value = parseFloat(match[1]);
  const unit = (match[2] ?? "m").toLowerCase();
  switch (unit) {
    case "s":
      return value * 1000;
    case "h":
      return value * 60 * 60 * 1000;
    case "m":
    default:
      return value * 60 * 1000;
  }
}
