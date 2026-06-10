/**
 * OpenCode Session Control Plugin — standalone session management tools.
 *
 * Extracted from graph-harness.ts (REQ-GH-111 through REQ-GH-115).
 * Enhanced with Session Roster (SWDE-64) and Session Event Stream Cursors (SWDE-66).
 *
 * Tools registered:
 *   session_spawn     — REQ-GH-111 (+ SWDE-64 auto-roster)
 *   session_message   — REQ-GH-112 (+ SWDE-66 validateSessionId)
 *   session_interrupt — REQ-GH-113 (+ SWDE-66 validateSessionId)
 *   session_info      — REQ-GH-114 (+ SWDE-66 validateSessionId)
 *   session_list      — REQ-GH-115 (+ SWDE-66 active_only filter)
 *   session_read      — enhanced slicing/filtering + SWDE-66 cursor/budget enhancements
 *   session_config    — runtime config + SWDE-66 loopback allowlist (SEC-SWDE66-001 fix)
 *   session_stat      — REQ-OC-CURSOR-001 (SWDE-66: lightweight stats, no content)
 *   session_track     — REQ-OC-ROSTER-001 (SWDE-64)
 *   session_roster    — REQ-OC-ROSTER-002 (SWDE-64)
 *   session_tag       — REQ-OC-ROSTER-003 (SWDE-64)
 *   session_untag     — REQ-OC-ROSTER-003 (SWDE-64)
 *   session_untrack   — REQ-OC-ROSTER-007 (SWDE-64)
 *
 * axiom:trace work_item=SWDE-66 spec=specs/31-OpenCode-Integration-Contract.md#REQ-OC-CURSOR-001 jira_ref=SWDE-66
 * axiom:trace work_item=SWDE-64 spec=specs/31-OpenCode-Integration-Contract.md#REQ-OC-ROSTER-001 jira_ref=SWDE-64
 * axiom:trace work_item=opencode-session-plugin-01 spec=specs/102-Graph-Harness.md#REQ-GH-111
 */

import { Database } from "bun:sqlite";
import { openDatabase, sqliteWriteWithRetry } from "../shared/sqlite";
// axiom:trace work_item=SWDE-62 spec=specs/102-Graph-Harness.md#4.1 plan=phase-4/task-4-2/step-4-2-1 jira_ref=SWDE-62
import { existsSync, mkdirSync } from "node:fs";
import { join } from "node:path";
import { tool } from "@opencode-ai/plugin";
import { loadPluginConfig, pluginWarn } from "./config-utils.ts";
// axiom:trace work_item=plugin-config-management-01 spec=specs/112-Plugin-Config-Management.md#8 plan=phase-4/task-4-3/step-4-3-1

// ─────────────────────────────────────────────────────────────────────────────
// In-memory process handle registry (fix F2: PID reuse risk)
// ─────────────────────────────────────────────────────────────────────────────
const _processHandles = new Map<string, ReturnType<typeof Bun.spawn>>();

// ─────────────────────────────────────────────────────────────────────────────
// Security helpers
// axiom:trace work_item=SWDE-66 spec=specs/31-OpenCode-Integration-Contract.md#12a jira_ref=SWDE-66
// ─────────────────────────────────────────────────────────────────────────────

/**
 * Validates session IDs against ^[a-zA-Z0-9_-]{4,128}$.
 * MUST be called before any HTTP call or DB write using session_id (SEC-SWDE66-002).
 */
function validateSessionId(id: string): boolean {
  return /^[a-zA-Z0-9_-]{4,128}$/.test(id);
}

/**
 * Redacts known credential patterns from text before storage.
 * Applied to roster notes and tags before DB writes (REQ-OC-ROSTER-001).
 * Base64 threshold: 60+ consecutive alphanumeric chars (avoids false-positives on
 * 40-char session IDs; catches encoded secrets).
 */
function redactCredentials(text: string): string {
  // GitHub personal access token
  text = text.replace(/ghp_[A-Za-z0-9]{10,}/g, "[REDACTED:github-token]");
  // Atlassian API token
  text = text.replace(/ATATT3[A-Za-z0-9+/=]{10,}/g, "[REDACTED:atlassian-token]");
  // OpenAI-style API key
  text = text.replace(/sk-[A-Za-z0-9]{20,}/g, "[REDACTED:api-key]");
  // Bearer token (case-insensitive header value)
  text = text.replace(/Bearer\s+[A-Za-z0-9._-]{10,}/gi, "[REDACTED:bearer-token]");
  // JWT (three dot-separated base64url segments)
  text = text.replace(/eyJ[A-Za-z0-9._-]+\.[A-Za-z0-9._-]+\.[A-Za-z0-9._-]+/g, "[REDACTED:jwt]");
  // Generic key=value or key:value credential pair
  text = text.replace(
    /(token|password|secret|auth)\s*[=:]\s*[A-Za-z0-9_-]{8,}/gi,
    "[REDACTED:credential]",
  );
  // Long pure alphanumeric blob (60+ chars) — likely encoded secret / base64 payload
  text = text.replace(/[A-Za-z0-9]{60,}/g, "[REDACTED:encoded-secret]");
  return text;
}

/**
 * Matches a tool name against a filter pattern supporting prefix/suffix wildcards.
 * Pattern validation: ^[a-zA-Z0-9_*?-]{1,128}$
 * - "bash*"  → prefix match (starts with "bash")
 * - "*_edit" → suffix match (ends with "_edit")
 * - "bash"   → exact match
 */
function matchToolFilter(toolName: string, filter: string): boolean {
  const isPrefix = filter.endsWith("*") && !filter.startsWith("*");
  const isSuffix = filter.startsWith("*") && !filter.endsWith("*");
  if (isPrefix) return toolName.startsWith(filter.slice(0, -1));
  if (isSuffix) return toolName.endsWith(filter.slice(1));
  return toolName === filter;
}

/** Loopback-only allowlist for opencode_base_url (SEC-SWDE66-001 fix).
 * Includes both `::1` and `[::1]` to handle URL parsers that may or may not strip
 * IPv6 brackets from the hostname property. */
const LOOPBACK_HOSTS = new Set(["127.0.0.1", "localhost", "::1", "[::1]"]);

// ─────────────────────────────────────────────────────────────────────────────
// Plugin config
// axiom:trace work_item=plugin-config-management-01 spec=specs/112-Plugin-Config-Management.md#8 plan=phase-4/task-4-3/step-4-3-1
// ─────────────────────────────────────────────────────────────────────────────

export interface SessionPluginConfig {
  opencode_base_url: string;
  request_timeout_ms: number;
  spawn_timeout_ms: number;
  message_fetch_limit: number;
  stat_rate_limit_ms: number;
}

// Module-local (not exported) — plugin loader crashes on non-function exports.
export const DEFAULT_SESSION_CONFIG: SessionPluginConfig = {
  opencode_base_url: "http://localhost:4096",
  request_timeout_ms: 5000,
  spawn_timeout_ms: 10000,
  message_fetch_limit: 100,
  stat_rate_limit_ms: 500,
};

// Exported helpers for testability (step-pcm-r10-02/03)
// axiom:trace work_item=plugin-config-management-01 spec=specs/112-Plugin-Config-Management.md#8 plan=phase-4/task-4-3/step-4-3-3
export function applyLimitFn<T>(arr: T[], limit: number): T[] {
  return limit > 0 ? arr.slice(0, limit) : arr;
}
export function shouldRunWatchdog(timeoutMs: number): boolean {
  return timeoutMs > 0;
}

// ─────────────────────────────────────────────────────────────────────────────
// Plugin factory
// ─────────────────────────────────────────────────────────────────────────────

export const OpenCodeSessionPlugin = async ({
  directory,
  client,
}: {
  directory: string;
  client: unknown;
}) => {
  // ── Database initialisation ───────────────────────────────────────────────
  // Creates .graph-harness/harness.db if absent (roster-first: REQ-OC-ROSTER-006).
  // Always creates session_roster + tool_cursors tables (idempotent).
  // Runs startup TTL cleanup for both tables.
  // Schema-checks sessions table only if it exists; never nullifies db on mismatch
  // so that roster + cursor tools always work regardless of graph-harness version.
  const dbDir = join(directory, ".graph-harness");
  const dbPath = join(dbDir, "harness.db");

  let db: Database | null = null;
  // Tracks whether the graph-harness sessions table is schema-compatible.
  // Only session_info and session_interrupt (DB path) depend on this.
  let graphHarnessDbAvailable = false;

  try {
    // Create directory + DB — roster-first creation (REQ-OC-ROSTER-006)
    if (!existsSync(dbDir)) {
      mkdirSync(dbDir, { recursive: true });
    }
    db = openDatabase(dbPath);

    // ── Create session_roster table (SWDE-64, REQ-OC-ROSTER-001 through 008) ──
    db.exec(`
      CREATE TABLE IF NOT EXISTS session_roster (
        id         INTEGER PRIMARY KEY AUTOINCREMENT,
        session_id TEXT NOT NULL CHECK(length(session_id) >= 4 AND length(session_id) <= 128),
        origin     TEXT NOT NULL CHECK(origin IN ('adopted', 'spawned')),
        tags       TEXT NOT NULL DEFAULT '[]' CHECK(json_valid(tags)),
        added_by   TEXT NOT NULL DEFAULT 'unknown',
        added_at   TEXT NOT NULL DEFAULT (datetime('now')),
        notes      TEXT NOT NULL DEFAULT '' CHECK(length(notes) <= 4096),
        UNIQUE(session_id, added_by)
      );
      CREATE INDEX IF NOT EXISTS idx_roster_session_id ON session_roster(session_id);
      CREATE INDEX IF NOT EXISTS idx_roster_added_by   ON session_roster(added_by);
      CREATE INDEX IF NOT EXISTS idx_roster_added_at   ON session_roster(added_at);
    `);

    // ── Create tool_cursors table (SWDE-66, REQ-OC-CURSOR-003) ───────────────
    // axiom:trace work_item=SWDE-66 spec=specs/31-OpenCode-Integration-Contract.md#REQ-OC-CURSOR-003 jira_ref=SWDE-66
    db.exec(`
      CREATE TABLE IF NOT EXISTS tool_cursors (
        caller_id       TEXT NOT NULL,
        session_id      TEXT NOT NULL,
        last_message_id TEXT NOT NULL DEFAULT '',
        last_read_at    TEXT NOT NULL DEFAULT (datetime('now')),
        PRIMARY KEY (caller_id, session_id)
      );
    `);

    // ── Startup TTL cleanup ───────────────────────────────────────────────────
    // Roster: 30-day TTL (REQ-OC-ROSTER-008)
    db.exec(`DELETE FROM session_roster WHERE added_at < datetime('now', '-30 days')`);
    // Cursors: 1-day TTL (SWDE-66 lane rule; CHAOS-SWDE66-002 fix)
    // axiom:trace work_item=SWDE-66 spec=specs/31-OpenCode-Integration-Contract.md#REQ-OC-CURSOR-003 jira_ref=SWDE-66
    db.exec(`DELETE FROM tool_cursors WHERE last_read_at < datetime('now', '-1 day')`);

    // ── sessions table schema compat check (graph-harness interop) ────────────
    const sessionsExists =
      (
        db
          .prepare(`SELECT name FROM sqlite_master WHERE type='table' AND name='sessions'`)
          .get() as { name: string } | undefined
      ) !== undefined;

    if (sessionsExists) {
      const cols = (
        db.prepare("SELECT name FROM pragma_table_info('sessions')").all() as {
          name: string;
        }[]
      ).map((r) => r.name);
      const required = [
        "session_id",
        "graph_id",
        "role",
        "status",
        "worker_pid",
        "last_heartbeat",
      ];
      const missing = required.filter((c) => !cols.includes(c));
      if (missing.length > 0) {
        pluginWarn("opencode-session", `Schema mismatch: sessions table missing columns [${missing.join(", ")}]. Graph-harness DB features (session_info, session_interrupt DB path) are unavailable.`);
        // Do NOT set db = null — roster + cursor tools still work with this db instance
      } else {
        graphHarnessDbAvailable = true;
      }
    }
    // If sessions table absent: graphHarnessDbAvailable stays false; roster/cursor still work.
  } catch (initErr) {
    pluginWarn("opencode-session", "DB init error", { error: String(initErr) });
    db = null;
  }

  // ── Base URL detection ────────────────────────────────────────────────────
  function detectBaseUrl(): string {
    const c = client as Record<string, unknown>;
    const candidates = [
      c?.baseUrl,
      c?.base_url,
      c?._baseUrl,
      (c?.config as Record<string, unknown>)?.baseUrl,
      (c?.config as Record<string, unknown>)?.base_url,
      (c?.defaults as Record<string, unknown>)?.baseUrl,
    ];
    for (const url of candidates) {
      if (typeof url === "string" && url.startsWith("http")) return url;
    }
    return (process.env.OPENCODE_BASE_URL as string | undefined) ?? "http://localhost:4096";
  }

   // ── Runtime config — mutable, updated live via session_config ────────────
   // Priority order for opencode_base_url (ADR-OCS-001 resolved):
   //   1. AXIOM_OPENCODE_SESSION_OPENCODE_BASE_URL env var (explicit operator override) — wins
   //   2. SDK client's detected baseUrl (existing connection on the client object) — wins if no env var
   //   3. OPENCODE_BASE_URL (OpenCode's own env var, legacy fallback inside detectBaseUrl)
   //   4. Default http://localhost:4096
   // Rationale: env var override is the spec §3.1 Layer 4 contract. If an operator explicitly
   // sets the env var they expect it to win. When no env var is set, the SDK client already
   // knows its server URL — trust it. This closes DA-5/ADR-OCS-001.
   // Implementation note: we detect env var PRESENCE (not value) to correctly handle the edge case
   // where the env var is set to the same value as the default (e.g., http://localhost:4096).
   // axiom:trace work_item=plugin-config-management-01 spec=specs/112-Plugin-Config-Management.md#8 plan=phase-4/task-4-3/step-4-3-2
   const _loadedConfig = loadPluginConfig("opencode-session", DEFAULT_SESSION_CONFIG, directory);
   const _envUrlOverrideSet = process.env["AXIOM_OPENCODE_SESSION_OPENCODE_BASE_URL"] !== undefined;
   const runtimeConfig = {
     opencode_base_url:
       // Detect env var PRESENCE (not value) — covers the edge case where the env var is
       // set to the same value as the default (value-comparison would silently ignore it).
       _envUrlOverrideSet
         ? _loadedConfig.opencode_base_url   // env var explicitly set → wins (spec §3.1 Layer 4)
         : detectBaseUrl(),                   // no env var → SDK client URL (existing connection)
     request_timeout_ms: _loadedConfig.request_timeout_ms,
     spawn_timeout_ms: _loadedConfig.spawn_timeout_ms,
     message_fetch_limit: _loadedConfig.message_fetch_limit,
     stat_rate_limit_ms: _loadedConfig.stat_rate_limit_ms, // SWDE-66: min interval between session_stat calls per (caller_id, session_id)
   };

  function cfg() {
    return runtimeConfig;
  }

  // ── Rate limiter for session_stat (SWDE-66, CHAOS-SWDE66-001 prevention) ──
  // axiom:trace work_item=SWDE-66 spec=specs/31-OpenCode-Integration-Contract.md#REQ-OC-CURSOR-001 jira_ref=SWDE-66
  const _statRateLimit = new Map<string, number>();

  // ── Ledger helper (non-fatal; noop when DB absent) ────────────────────────
  function addLedgerEntry(
    graphId: string,
    eventType: string,
    data: Record<string, unknown>,
  ): void {
    if (!db) return;
    sqliteWriteWithRetry(() => {
      db!.prepare(
        `INSERT OR IGNORE INTO ledger (graph_id, event_type, data, created_at)
         VALUES (?, ?, ?, datetime('now'))`,
      ).run(graphId, eventType, JSON.stringify(data));
    }, "session.ledger:insert");
  }

  // ── Caller ID helper ──────────────────────────────────────────────────────
  // Returns context.sessionID if set; 'unknown' as fallback (spec §12a: added_by fallback).
  // 'unknown' also triggers the session_roster namespace warning (REQ-OC-ROSTER-002).
  function getCallerId(context: unknown): string {
    const sid = (context as Record<string, unknown>)?.sessionID as string | undefined;
    return sid && sid.length > 0 ? sid : "unknown";
  }

  // ── Raw message content extractor (for character budget counting) ─────────
  function extractMessageContent(msg: Record<string, unknown>): string {
    if (typeof msg.content === "string") return msg.content;
    if (typeof msg.text === "string") return msg.text;
    if (typeof msg.message === "string") return msg.message;
    // Handle OpenCode parts array
    const parts = msg.parts;
    if (Array.isArray(parts)) {
      return parts
        .map((p: unknown) => {
          const part = p as Record<string, unknown>;
          return typeof part.content === "string"
            ? part.content
            : typeof part.text === "string"
              ? part.text
              : "";
        })
        .join("");
    }
    return "";
  }

  // ── Fetch messages from OpenCode HTTP API ─────────────────────────────────
  // Returns raw message array or null on all-endpoint failure.
  async function fetchMessages(
    sessionId: string,
  ): Promise<Record<string, unknown>[] | null> {
    // axiom:trace work_item=plugin-config-management-01 spec=specs/112-Plugin-Config-Management.md#8 plan=phase-4/task-4-3/step-4-3-3
    const limit = cfg().message_fetch_limit;
    const applyLimit = <T>(arr: T[]): T[] => (limit > 0 ? arr.slice(0, limit) : arr);
    const endpoints = [
      `${cfg().opencode_base_url}/session/${encodeURIComponent(sessionId)}/message`,
      `${cfg().opencode_base_url}/session/${encodeURIComponent(sessionId)}/messages`,
      `${cfg().opencode_base_url}/session/${encodeURIComponent(sessionId)}`,
    ];
    for (const url of endpoints) {
      try {
        const resp = await fetch(url, {
          headers: { Accept: "application/json" },
          signal: AbortSignal.timeout(cfg().request_timeout_ms),
        });
        if (resp.ok) {
          const body = (await resp.json()) as unknown;
          if (Array.isArray(body)) return applyLimit(body as Record<string, unknown>[]);
          const b = body as Record<string, unknown>;
          const msgs = (b.messages ?? b.data ?? b.content ?? null) as
            | Record<string, unknown>[]
            | null;
          if (Array.isArray(msgs)) return applyLimit(msgs);
        }
      } catch {
        /* try next endpoint */
      }
    }
    return null;
  }

  // ─────────────────────────────────────────────────────────────────────────
  // REQ-GH-111: session_spawn  (+ SWDE-64 auto-roster + SWDE-66 tags param)
  // ─────────────────────────────────────────────────────────────────────────
  const sessionSpawnTool = tool({
    description:
      "Spawn a new OpenCode session and return its session_id. " +
      "Use this when you need to delegate work to a fresh agent context. " +
      "Optional: lock a graph to the new session so only it can mutate the graph (lock_graph_id). " +
      "Optional: send an initial message to the new session immediately after spawn (initial_message). " +
      "Returns { session_id, status: 'spawned' } on success or { error } on failure.",
    args: {
      initial_message: tool.schema
        .string()
        .optional()
        .describe("Message to send into the new session on its first turn"),
      lock_graph_id: tool.schema
        .string()
        .optional()
        .describe("Graph ID to lock to the new session after spawn (REQ-GH-110)"),
      model: tool.schema
        .string()
        .optional()
        .describe("Model override for the new session (passed as query param if supported)"),
      tags: tool.schema
        .array(tool.schema.string())
        .optional()
        .describe("Tags for auto-roster entry (SWDE-64, REQ-OC-ROSTER-004)"),
    },
    async execute(args, context) {
      // axiom:trace work_item=opencode-session-plugin-01 spec=specs/102-Graph-Harness.md#REQ-GH-111
      // axiom:trace work_item=SWDE-64 spec=specs/31-OpenCode-Integration-Contract.md#REQ-OC-ROSTER-004 jira_ref=SWDE-64
      try {
        let newSessionId: string | null = null;
        let workerPid: number | null = null;

        // Try HTTP API first (2-second timeout: fail fast for CLI fallback)
        try {
          const resp = await fetch(`${cfg().opencode_base_url}/session`, {
            method: "POST",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify(args.model ? { model: args.model } : {}),
            signal: AbortSignal.timeout(cfg().request_timeout_ms),
          });
          if (resp.ok) {
            const body = (await resp.json()) as Record<string, unknown>;
            newSessionId = (body.id ?? body.session_id ?? body.sessionID) as string | null;
          }
        } catch {
          /* HTTP API unavailable — fall through to CLI */
        }

        // CLI fallback
        if (!newSessionId) {
          newSessionId = `gh_session_${Date.now().toString(36)}_${Math.random()
            .toString(36)
            .slice(2, 6)}`;
          try {
            const cliArgs = ["opencode", "run", "--non-interactive"];
            if (args.model) cliArgs.push("--model", args.model);
            const proc = Bun.spawn(cliArgs, { stdout: "ignore", stderr: "ignore" });
            workerPid = proc.pid ?? null;
            _processHandles.set(newSessionId, proc);
            // Watchdog: kill spawned process if it exceeds spawn_timeout_ms
            // axiom:trace work_item=plugin-config-management-01 spec=specs/112-Plugin-Config-Management.md#8 plan=phase-4/task-4-3/step-4-3-3
            const spawnTimeout = cfg().spawn_timeout_ms;
            if (spawnTimeout > 0) {
              const killTimer = setTimeout(() => {
                if (proc.exitCode === null) {
                  proc.kill();
                  pluginWarn("opencode-session", `Spawn watchdog: killed session after ${spawnTimeout}ms`);
                }
              }, spawnTimeout);
              proc.exited.then(() => clearTimeout(killTimer)).catch(() => clearTimeout(killTimer));
            }
          } catch {
            return JSON.stringify({ error: "Failed to spawn session via CLI" });
          }
        }

        // Register session in graph-harness DB (if available and graph is being locked)
        if (db && graphHarnessDbAvailable) {
          const callerSessionId = (context as Record<string, unknown>)?.sessionID as
            | string
            | undefined;
          const nowIso = new Date().toISOString();
          if (args.lock_graph_id) {
            // axiom:trace work_item=SWDE-62 spec=specs/102-Graph-Harness.md#4.1 jira_ref=SWDE-62
            sqliteWriteWithRetry(() => {
              db!.prepare(`
                INSERT OR IGNORE INTO sessions
                  (session_id, graph_id, role, status, spawned_by, worker_pid, created_at, last_heartbeat)
                VALUES (?, ?, 'coordinator', 'active', ?, ?, ?, ?)
              `).run(
                newSessionId,
                args.lock_graph_id,
                callerSessionId ?? null,
                workerPid,
                nowIso,
                nowIso,
              );
            }, "session.spawn:insert_session");

            sqliteWriteWithRetry(() => {
              db!.prepare(`UPDATE graphs SET locked_by=? WHERE id=?`).run(
                newSessionId,
                args.lock_graph_id,
              );
            }, "session.spawn:lock_graph");
            addLedgerEntry(args.lock_graph_id, "graph_locked", {
              locked_by: newSessionId,
              locked_by_session: callerSessionId,
            });
          }
        }

        // Auto-track in session_roster (SWDE-64, REQ-OC-ROSTER-004)
        // Non-fatal: spawn still returns success even if roster INSERT fails.
        if (db && newSessionId) {
          const callerId = getCallerId(context);
          const rawTags = (args.tags ?? []).map((t: string) => redactCredentials(t));
          const tagsJson = JSON.stringify(rawTags);
          sqliteWriteWithRetry(() => {
            db!.prepare(`
              INSERT INTO session_roster (session_id, origin, tags, added_by, added_at, notes)
              VALUES (?, 'spawned', ?, ?, datetime('now'), '')
              ON CONFLICT(session_id, added_by) DO NOTHING
            `).run(newSessionId, tagsJson, callerId);
          }, "session_spawn:auto_roster");
        }

        // Send initial message if requested (REQ-GH-112 path)
        if (args.initial_message && newSessionId) {
          try {
            await fetch(
              `${cfg().opencode_base_url}/session/${encodeURIComponent(newSessionId)}/message`,
              {
                method: "POST",
                headers: { "Content-Type": "application/json" },
                body: JSON.stringify({ role: "user", content: args.initial_message }),
                signal: AbortSignal.timeout(cfg().request_timeout_ms),
              },
            );
          } catch {
            /* best-effort */
          }
        }

        return JSON.stringify({
          session_id: newSessionId,
          status: "spawned",
          locked_graph_id: args.lock_graph_id ?? null,
        });
      } catch (err) {
        return JSON.stringify({ error: String(err) });
      }
    },
  });

  // ─────────────────────────────────────────────────────────────────────────
  // REQ-GH-112: session_message  (+ SWDE-66 validateSessionId)
  // ─────────────────────────────────────────────────────────────────────────
  const sessionMessageTool = tool({
    description:
      "Send a message to an existing OpenCode session. " +
      "Use this to coordinate with sessions spawned via session.spawn, or to prompt child sessions. " +
      "Returns { sent: true, session_id } on success or { error, session_id } on failure.",
    args: {
      session_id: tool.schema.string().min(1).describe("Target session ID"),
      message: tool.schema.string().min(1).describe("Message content to deliver"),
      role: tool.schema
        .enum(["user", "assistant"])
        .optional()
        .default("user")
        .describe("Message role (default: user)"),
    },
    async execute(args) {
      // axiom:trace work_item=opencode-session-plugin-01 spec=specs/102-Graph-Harness.md#REQ-GH-112
      // axiom:trace work_item=SWDE-66 spec=specs/31-OpenCode-Integration-Contract.md#12a jira_ref=SWDE-66
      // SWDE-66 SEC-SWDE66-002: validate session_id before any HTTP call
      if (!validateSessionId(args.session_id)) {
        return JSON.stringify({
          error: `Invalid session_id format (must match ^[a-zA-Z0-9_-]{4,128}$): ${args.session_id}`,
          session_id: args.session_id,
        });
      }
      try {
        // Try SDK client.session.promptAsync first
        const promptFn =
          (client as Record<string, unknown>)?.session &&
          (client as Record<string, Record<string, unknown>>).session?.promptAsync;
        if (typeof promptFn === "function") {
          await (promptFn as (id: string, msg: string) => Promise<void>)(
            args.session_id,
            args.message,
          );
          return JSON.stringify({ sent: true, session_id: args.session_id, method: "sdk" });
        }

        // HTTP API fallback
        const resp = await fetch(
          `${cfg().opencode_base_url}/session/${encodeURIComponent(args.session_id)}/message`,
          {
            method: "POST",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify({ role: args.role ?? "user", content: args.message }),
            signal: AbortSignal.timeout(cfg().request_timeout_ms),
          },
        );
        if (!resp.ok) {
          const text = await resp.text();
          return JSON.stringify({
            error: `HTTP ${resp.status}: ${text}`,
            session_id: args.session_id,
          });
        }
        return JSON.stringify({ sent: true, session_id: args.session_id, method: "http" });
      } catch (err) {
        return JSON.stringify({ error: String(err), session_id: args.session_id });
      }
    },
  });

  // ─────────────────────────────────────────────────────────────────────────
  // REQ-GH-113: session_interrupt  (+ SWDE-66 validateSessionId)
  // ─────────────────────────────────────────────────────────────────────────
  const sessionInterruptTool = tool({
    description:
      "Gracefully interrupt and stop an active session. " +
      "Uses SIGTERM → 5s grace → SIGKILL if the session was CLI-spawned (worker_pid known). " +
      "Falls back to SDK terminate() or marks the session stale in the DB. " +
      "Returns { interrupted, method, session_id }.",
    args: {
      session_id: tool.schema.string().min(1).describe("Session ID to interrupt"),
      reason: tool.schema.string().optional().describe("Reason logged to ledger"),
    },
    async execute(args, context) {
      // axiom:trace work_item=opencode-session-plugin-01 spec=specs/102-Graph-Harness.md#REQ-GH-113
      // axiom:trace work_item=SWDE-66 spec=specs/31-OpenCode-Integration-Contract.md#12a jira_ref=SWDE-66
      // SWDE-66 SEC-SWDE66-002: validate before DB or process lookup
      if (!validateSessionId(args.session_id)) {
        return JSON.stringify({
          error: `Invalid session_id format (must match ^[a-zA-Z0-9_-]{4,128}$): ${args.session_id}`,
          session_id: args.session_id,
        });
      }
      try {
        // DB lookup only when graph-harness schema is available
        const sessionRow =
          db && graphHarnessDbAvailable
            ? (db
                .prepare(
                  `SELECT graph_id, worker_pid, status FROM sessions WHERE session_id=?`,
                )
                .get(args.session_id) as
                | { graph_id: string; worker_pid: number | null; status: string }
                | undefined)
            : undefined;

        if (db && graphHarnessDbAvailable && !sessionRow) {
          return JSON.stringify({
            error: `Session not found: ${args.session_id}`,
            session_id: args.session_id,
          });
        }
        if (sessionRow && sessionRow.status !== "active") {
          return JSON.stringify({
            interrupted: false,
            reason: `Session already ${sessionRow.status}`,
            session_id: args.session_id,
          });
        }

        let method = "noop";

        // Prefer stored process handle (fix F2: avoids PID-reuse risk)
        const procHandle = _processHandles.get(args.session_id);
        if (procHandle) {
          try {
            procHandle.kill("SIGTERM");
            await new Promise<void>((resolve) => {
              const killTimer = setTimeout(() => {
                try {
                  procHandle.kill("SIGKILL");
                } catch {
                  /* gone */
                }
                resolve();
              }, 5000);
              procHandle.exited
                .then(() => {
                  clearTimeout(killTimer);
                  resolve();
                })
                .catch(() => {
                  clearTimeout(killTimer);
                  resolve();
                });
            });
          } catch {
            /* process already gone */
          }
          _processHandles.delete(args.session_id);
          method = "handle";
        } else if (sessionRow?.worker_pid !== null && sessionRow?.worker_pid !== undefined) {
          // PID-based fallback (best-effort; PID reuse is possible)
          try {
            process.kill(sessionRow.worker_pid, "SIGTERM");
          } catch {
            /* already gone */
          }
          await new Promise<void>((resolve) => {
            const killTimer = setTimeout(() => {
              try {
                process.kill(sessionRow.worker_pid!, "SIGKILL");
              } catch {
                /* gone */
              }
              resolve();
            }, 5000);
            const poll = setInterval(() => {
              try {
                process.kill(sessionRow.worker_pid!, 0);
              } catch {
                clearTimeout(killTimer);
                clearInterval(poll);
                resolve();
              }
            }, 200);
          });
          method = "sigterm-pid";
        } else {
          // SDK terminate fallback
          const terminateFn =
            (client as Record<string, unknown>)?.session &&
            (client as Record<string, Record<string, unknown>>).session?.terminate;
          if (typeof terminateFn === "function") {
            try {
              await (terminateFn as (id: string) => Promise<void>)(args.session_id);
              method = "sdk";
            } catch {
              method = "noop";
            }
          }
        }

        // Mark stale in DB (only when graph-harness schema available)
        if (db && graphHarnessDbAvailable) {
          sqliteWriteWithRetry(() => {
            db!.prepare(
              `UPDATE sessions SET status='stale', completed_at=datetime('now') WHERE session_id=?`,
            ).run(args.session_id);
          }, "session.interrupt:update_status");

          const callerSessionId = (context as Record<string, unknown>)?.sessionID as
            | string
            | undefined;
          addLedgerEntry(sessionRow?.graph_id || "", "session_interrupted", {
            session_id: args.session_id,
            reason: args.reason ?? "manual",
            method,
            interrupted_by: callerSessionId,
          });
        }

        return JSON.stringify({ interrupted: true, method, session_id: args.session_id });
      } catch (err) {
        return JSON.stringify({ error: String(err), session_id: args.session_id });
      }
    },
  });

  // ─────────────────────────────────────────────────────────────────────────
  // REQ-GH-114: session_info  (+ SWDE-66 validateSessionId)
  // ─────────────────────────────────────────────────────────────────────────
  const sessionInfoTool = tool({
    description:
      "Get status information about a session registered with the graph harness. " +
      "Only sessions that were spawned by or registered with this harness are visible. " +
      "Returns { session_id, graph_id, node_id, role, status, last_heartbeat, worker_pid, " +
      "tool_calls, cost_usd, created_at } or { error } if not found.",
    args: {
      session_id: tool.schema.string().min(1).describe("Session ID to look up"),
    },
    async execute(args) {
      // axiom:trace work_item=opencode-session-plugin-01 spec=specs/102-Graph-Harness.md#REQ-GH-114
      // axiom:trace work_item=SWDE-66 spec=specs/31-OpenCode-Integration-Contract.md#12a jira_ref=SWDE-66
      if (!validateSessionId(args.session_id)) {
        return JSON.stringify({
          error: `Invalid session_id format (must match ^[a-zA-Z0-9_-]{4,128}$): ${args.session_id}`,
        });
      }
      try {
        if (!db || !graphHarnessDbAvailable) {
          return JSON.stringify({
            error: "Graph harness DB not available (harness.db not found or schema mismatch)",
          });
        }
        const row = db
          .prepare(
            `SELECT session_id, graph_id, node_id, role, status, last_heartbeat,
                    worker_pid, tool_calls, cost_usd, created_at, consecutive_briefing_failures
             FROM sessions WHERE session_id=?`,
          )
          .get(args.session_id) as Record<string, unknown> | undefined;

        if (!row) {
          return JSON.stringify({
            error: `Session not found in graph-harness DB: ${args.session_id}`,
          });
        }
        return JSON.stringify(row);
      } catch (err) {
        return JSON.stringify({ error: String(err) });
      }
    },
  });

  // ─────────────────────────────────────────────────────────────────────────
  // REQ-GH-115: session_list  (+ SWDE-66 active_only filter)
  // ─────────────────────────────────────────────────────────────────────────
  const sessionListTool = tool({
    description:
      "List ALL active OpenCode sessions by calling the OpenCode HTTP API. " +
      "This includes standalone sessions (not tied to a graph) as well as " +
      "graph-worker sessions. Use graph_session_list (in graph-harness) to see only harness-tracked sessions. " +
      "Returns { sessions, total } or { error }.",
    args: {
      active_only: tool.schema
        .boolean()
        .optional()
        .describe(
          "When true, return only sessions with activity within the last 60 seconds (REQ-OC-CURSOR-005)",
        ),
    },
    async execute(args) {
      // axiom:trace work_item=SWDE-66 spec=specs/31-OpenCode-Integration-Contract.md#REQ-OC-CURSOR-005 jira_ref=SWDE-66
      try {
        const resp = await fetch(`${cfg().opencode_base_url}/session`, {
          method: "GET",
          headers: { Accept: "application/json" },
          signal: AbortSignal.timeout(cfg().request_timeout_ms),
        });
        if (resp.ok) {
          const body = (await resp.json()) as unknown;
          let sessions: unknown[] = [];
          if (Array.isArray(body)) {
            sessions = body;
          } else {
            const b = body as Record<string, unknown>;
            sessions = Array.isArray(b.sessions) ? b.sessions : [b];
          }

          // active_only filter: keep sessions with last_activity within 60s
          if (args.active_only) {
            const cutoffMs = Date.now() - 60_000;
            sessions = sessions.filter((s: unknown) => {
              const sess = s as Record<string, unknown>;
              // Try common timestamp fields
              const ts =
                (sess.last_activity_at as string | undefined) ??
                (sess.updated_at as string | undefined) ??
                (sess.last_message_at as string | undefined) ??
                (sess.updatedAt as string | undefined);
              if (!ts) return true; // no timestamp — assume active (safe default)
              return new Date(ts).getTime() >= cutoffMs;
            });
          }

          return JSON.stringify({
            sessions,
            total: sessions.length,
            source: "opencode_api",
            ...(args.active_only ? { active_only: true } : {}),
          });
        }
        return JSON.stringify({
          error: `OpenCode API returned ${resp.status}`,
          url: `${cfg().opencode_base_url}/session`,
          note: "OpenCode may not expose a session list endpoint. Try session_info with a known session_id.",
        });
      } catch (err) {
        return JSON.stringify({
          error: String(err),
          url: `${cfg().opencode_base_url}/session`,
          note: "Could not reach OpenCode API. Check that OpenCode is running and OPENCODE_BASE_URL is set.",
        });
      }
    },
  });

  // ─────────────────────────────────────────────────────────────────────────
  // session_read — slice/filter/poll access  (+ SWDE-66 enhancements)
  //
  // SWDE-66 additions (REQ-OC-CURSOR-002 through REQ-OC-CURSOR-003):
  //   filter_role      — allowlist-validated role filter ('assistant','user','tool','system')
  //   filter_tool      — prefix/suffix tool name filter (^[a-zA-Z0-9_*?-]{1,128}$)
  //   character_budget — stop after ~N chars of content (soft limit; last msg included in full)
  //   activity_check   — fast-path: return stats only (no content) — {active, new_since_cursor, stalled_seconds}
  //   stats_only       — return count + metadata only, no content
  //   auto_cursor      — persist last-read position across calls (BEGIN IMMEDIATE txn)
  //   include_tool_name — opt-in: include last_tool_name in stats response (COMPLY-SWDE66-001 fix)
  //   include_filter_stats — opt-in: include filtered_out count (COMPLY-SWDE66-003 mitigation)
  //
  // Existing args unchanged (since_id, tail, offset, limit, reverse, role, contains).
  // axiom:trace work_item=SWDE-66 spec=specs/31-OpenCode-Integration-Contract.md#REQ-OC-CURSOR-002 jira_ref=SWDE-66
  // ─────────────────────────────────────────────────────────────────────────
  const sessionReadTool = tool({
    description:
      "Read messages from an OpenCode session with rich slice/filter/poll access. " +
      "Use tail:1 to get the latest reply. Use since_id to poll for new messages. " +
      "Use role to filter by message type (user/assistant/tool/system). " +
      "Use offset+limit+step for slice-style access (like Python arr[offset::step][:limit]). " +
      "Use auto_cursor:true to automatically track your read position across calls. " +
      "Use character_budget to limit content size. Use activity_check for a fast stats-only check. " +
      "Returns { messages, total, has_more, session_id }.",
    args: {
      session_id: tool.schema.string().min(1).describe("Session to read messages from"),
      // ── Original slicing/filtering args (unchanged) ──
      offset: tool.schema
        .number()
        .optional()
        .describe("Skip first N messages (default 0). Like arr[offset:]"),
      limit: tool.schema
        .number()
        .optional()
        .describe("Max messages to return after filtering/slicing (default 50)"),
      step: tool.schema
        .number()
        .optional()
        .describe("Take every Nth message (default 1). Like arr[::step]"),
      tail: tool.schema
        .number()
        .optional()
        .describe("Take last N messages — overrides offset. Like arr[-tail:]"),
      role: tool.schema
        .string()
        .optional()
        .describe("Filter by role: user | assistant | tool | system (legacy; prefer filter_role)"),
      contains: tool.schema
        .string()
        .optional()
        .describe("Case-insensitive substring filter on message content"),
      since_id: tool.schema
        .string()
        .optional()
        .describe("Only return messages after this message ID (for polling new replies)"),
      reverse: tool.schema
        .boolean()
        .optional()
        .describe("Return newest-first (default false = chronological order)"),
      // ── SWDE-66 new args ──
      filter_role: tool.schema
        .string()
        .optional()
        .describe(
          "SWDE-66: Role filter with allowlist validation: assistant | user | tool | system",
        ),
      filter_tool: tool.schema
        .string()
        .optional()
        .describe(
          "SWDE-66: Tool name filter — prefix glob (bash*), suffix glob (*_edit), or exact. Pattern: ^[a-zA-Z0-9_*?-]{1,128}$",
        ),
      character_budget: tool.schema
        .number()
        .optional()
        .describe(
          "SWDE-66: Stop after approx N chars of message content (1..1,000,000). Last msg included in full. Sets character_budget_hit:true.",
        ),
      activity_check: tool.schema
        .boolean()
        .optional()
        .describe(
          "SWDE-66: Fast-path — return {active, new_since_cursor, stalled_seconds, last_event_role} without content",
        ),
      stats_only: tool.schema
        .boolean()
        .optional()
        .describe("SWDE-66: Return count + stats only, no message content"),
      auto_cursor: tool.schema
        .boolean()
        .optional()
        .describe(
          "SWDE-66: Auto-track read position — resumes from last message read by this caller. Cursor advances only when messages returned. When caller_id cannot be determined (no context.sessionID), auto_cursor falls back to caller_id='unknown', which is shared by all anonymous callers monitoring the same session. In multi-agent environments, ensure context.sessionID is populated to avoid cursor namespace collisions.",
        ),
      include_tool_name: tool.schema
        .boolean()
        .optional()
        .describe("SWDE-66: Opt-in — include last_tool_name in stats response (default false)"),
      include_filter_stats: tool.schema
        .boolean()
        .optional()
        .describe(
          "SWDE-66: Opt-in — include filtered_out count (requires extra processing, default false)",
        ),
    },
    async execute(args, context) {
      // axiom:trace work_item=SWDE-66 spec=specs/31-OpenCode-Integration-Contract.md#REQ-OC-CURSOR-002 jira_ref=SWDE-66
      // SWDE-66 SEC-SWDE66-002: validate before HTTP call
      if (!validateSessionId(args.session_id)) {
        return JSON.stringify({
          error: `Invalid session_id format (must match ^[a-zA-Z0-9_-]{4,128}$): ${args.session_id}`,
          session_id: args.session_id,
        });
      }

      // Validate filter_role allowlist (CHAOS-SWDE66-003 + MI-6 fix)
      const VALID_ROLES = new Set(["assistant", "user", "tool", "system"]);
      const effectiveRole = args.filter_role ?? args.role;
      if (effectiveRole && !VALID_ROLES.has(effectiveRole.toLowerCase())) {
        return JSON.stringify({
          error: `filter_role must be one of: assistant, user, tool, system. Got: '${effectiveRole}'`,
          session_id: args.session_id,
        });
      }

      // Validate filter_tool pattern
      if (args.filter_tool && !/^[a-zA-Z0-9_*?-]{1,128}$/.test(args.filter_tool)) {
        return JSON.stringify({
          error: `filter_tool must match ^[a-zA-Z0-9_*?-]{1,128}$. Got: '${args.filter_tool}'`,
          session_id: args.session_id,
        });
      }

      // Validate character_budget range
      if (
        args.character_budget !== undefined &&
        (args.character_budget < 1 || args.character_budget > 1_000_000)
      ) {
        return JSON.stringify({
          error: `character_budget must be between 1 and 1,000,000. Got: ${args.character_budget}`,
          session_id: args.session_id,
        });
      }

      const callerId = getCallerId(context);

      try {
        // ── Resolve auto_cursor (BEGIN IMMEDIATE transaction for race safety) ──
        // CHAOS-SWDE66-005 fix: wrap cursor read-write in a single BEGIN IMMEDIATE txn
        let effectiveSinceId = args.since_id;

        if (args.auto_cursor && db && !effectiveSinceId) {
          // Read cursor within a transaction so concurrent readers don't race
          try {
            db.exec("BEGIN IMMEDIATE");
            const cursorRow = db
              .prepare(
                `SELECT last_message_id FROM tool_cursors WHERE caller_id=? AND session_id=?`,
              )
              .get(callerId, args.session_id) as
              | { last_message_id: string }
              | undefined;
            db.exec("COMMIT");
            if (cursorRow && cursorRow.last_message_id) {
              effectiveSinceId = cursorRow.last_message_id;
            }
          } catch {
            try {
              db.exec("ROLLBACK");
            } catch {
              /* ignore */
            }
          }
        }

        // ── activity_check fast path ──────────────────────────────────────
        // Returns lightweight stats without returning message content
        if (args.activity_check) {
          const msgs = await fetchMessages(args.session_id);
          if (!msgs) {
            return JSON.stringify({
              error: "Could not reach OpenCode API",
              session_id: args.session_id,
            });
          }
          const totalEvents = msgs.length;

          // Compute new_since_cursor
          let newSinceCursor = totalEvents;
          if (effectiveSinceId) {
            const idx = msgs.findIndex(
              (m) => String(m.id ?? m.message_id ?? "") === effectiveSinceId,
            );
            newSinceCursor = idx === -1 ? totalEvents : msgs.length - (idx + 1);
          }

          // Compute stalled_seconds from last message
          let stalledSeconds: number | null = null;
          let lastEventRole: string | null = null;
          let lastToolName: string | null = null;
          if (msgs.length > 0) {
            const lastMsg = msgs[msgs.length - 1];
            lastEventRole = String(lastMsg.role ?? lastMsg.type ?? "") || null;
            const ts =
              (lastMsg.created_at as string | undefined) ??
              (lastMsg.timestamp as string | undefined);
            if (ts) {
              const t = new Date(ts).getTime();
              if (!isNaN(t)) stalledSeconds = Math.floor((Date.now() - t) / 1000);
            }
            if (args.include_tool_name) {
              lastToolName =
                (String(lastMsg.tool ?? lastMsg.tool_name ?? lastMsg.name ?? "") || null);
            }
          }

          // is_active: false when no messages (session not yet started); true when last event was within 60s
          // axiom:trace work_item=SWDE-66 spec=specs/31-OpenCode-Integration-Contract.md#REQ-OC-CURSOR-001 plan=phase-4/task-4.1/step-1 jira_ref=SWDE-66
          const active = stalledSeconds === null ? false : stalledSeconds < 60;
          return JSON.stringify({
            session_id: args.session_id,
            active,
            new_since_cursor: newSinceCursor,
            stalled_seconds: stalledSeconds,
            last_event_role: lastEventRole,
            ...(args.include_tool_name ? { last_tool_name: lastToolName } : {}),
            total_events: totalEvents,
          });
        }

        // ── 1. Fetch raw messages from OpenCode ───────────────────────────
        let rawMessages: Record<string, unknown>[] = [];
        const fetched = await fetchMessages(args.session_id);
        if (fetched !== null) {
          rawMessages = fetched;
        }

        const totalFetched = rawMessages.length;

        // ── 2. Apply since_id / auto_cursor filter ────────────────────────
        // CHAOS-SWDE66-003 fix: apply since_id BEFORE role filter to avoid event loss
        if (effectiveSinceId) {
          const idx = rawMessages.findIndex(
            (m) => String(m.id ?? m.message_id ?? "") === effectiveSinceId,
          );
          if (idx !== -1) {
            rawMessages = rawMessages.slice(idx + 1);
          }
        }

        const afterCursorCount = rawMessages.length;

        // ── stats_only path ───────────────────────────────────────────────
        if (args.stats_only) {
          const lastMsg = rawMessages.length > 0 ? rawMessages[rawMessages.length - 1] : null;
          const lastEventRole = lastMsg
            ? String(lastMsg.role ?? lastMsg.type ?? "") || null
            : null;
          let stalledSeconds: number | null = null;
          if (lastMsg) {
            const ts =
              (lastMsg.created_at as string | undefined) ??
              (lastMsg.timestamp as string | undefined);
            if (ts) {
              const t = new Date(ts).getTime();
              if (!isNaN(t)) stalledSeconds = Math.floor((Date.now() - t) / 1000);
            }
          }
          const lastToolName = args.include_tool_name && lastMsg
            ? String(lastMsg.tool ?? lastMsg.tool_name ?? lastMsg.name ?? "") || null
            : undefined;
          return JSON.stringify({
            session_id: args.session_id,
            total_events: totalFetched,
            new_since_cursor: afterCursorCount,
            stalled_seconds: stalledSeconds,
            last_event_role: lastEventRole,
            ...(args.include_tool_name ? { last_tool_name: lastToolName } : {}),
          });
        }

        // ── 3. Apply filter_role / role filter ────────────────────────────
        if (effectiveRole) {
          const roleFilter = effectiveRole.toLowerCase();
          rawMessages = rawMessages.filter(
            (m) => String(m.role ?? m.type ?? "").toLowerCase() === roleFilter,
          );
        }

        // ── 4. Apply filter_tool ──────────────────────────────────────────
        if (args.filter_tool) {
          rawMessages = rawMessages.filter((m) => {
            const toolName = String(m.tool ?? m.tool_name ?? m.name ?? "");
            return matchToolFilter(toolName, args.filter_tool!);
          });
        }

        // ── 5. Apply contains filter ──────────────────────────────────────
        if (args.contains) {
          const needle = args.contains.toLowerCase();
          rawMessages = rawMessages.filter((m) =>
            extractMessageContent(m).toLowerCase().includes(needle),
          );
        }

        const totalAfterFilters = rawMessages.length;
        const filteredOut = afterCursorCount - totalAfterFilters;

        // ── 6. Apply character_budget (soft limit) ────────────────────────
        // Include the message that crosses the budget in full; stop after it.
        let characterBudgetHit = false;
        if (args.character_budget !== undefined) {
          const budget = Math.min(1_000_000, Math.max(1, args.character_budget));
          let charCount = 0;
          const budgeted: Record<string, unknown>[] = [];
          for (const m of rawMessages) {
            const content = extractMessageContent(m);
            charCount += content.length;
            budgeted.push(m);
            if (charCount >= budget) {
              characterBudgetHit = true;
              break;
            }
          }
          rawMessages = budgeted;
        }

        // ── 7. Apply tail (last N) ────────────────────────────────────────
        if (args.tail !== undefined && args.tail > 0) {
          rawMessages = rawMessages.slice(-args.tail);
        } else {
          // ── 8. Apply offset ───────────────────────────────────────────
          const offset = args.offset ?? 0;
          if (offset > 0) rawMessages = rawMessages.slice(offset);
        }

        // ── 9. Apply step (every Nth) ─────────────────────────────────────
        const step = args.step ?? 1;
        if (step > 1) {
          rawMessages = rawMessages.filter((_, i) => i % step === 0);
        }

        // ── 10. Apply limit ───────────────────────────────────────────────
        const limit = args.limit ?? 50;
        const hasMore = rawMessages.length > limit;
        if (hasMore) rawMessages = rawMessages.slice(0, limit);

        // ── 11. Reverse if requested ──────────────────────────────────────
        if (args.reverse) rawMessages = [...rawMessages].reverse();

        // ── 12. Update auto_cursor (only if messages returned) ────────────
        // CHAOS-SWDE66-006 fix: cursor only advances on non-empty results.
        // CHAOS-SWDE66-005 fix: BEGIN IMMEDIATE prevents concurrent overwrite.
        if (args.auto_cursor && db && rawMessages.length > 0) {
          const lastMsg = rawMessages[args.reverse ? 0 : rawMessages.length - 1];
          const lastId = String(lastMsg.id ?? lastMsg.message_id ?? "");
          if (lastId) {
            sqliteWriteWithRetry(() => {
              db!.exec("BEGIN IMMEDIATE");
              try {
                db!.prepare(`
                  INSERT INTO tool_cursors (caller_id, session_id, last_message_id, last_read_at)
                  VALUES (?, ?, ?, datetime('now'))
                  ON CONFLICT(caller_id, session_id) DO UPDATE SET
                    last_message_id = excluded.last_message_id,
                    last_read_at    = datetime('now')
                `).run(callerId, args.session_id, lastId);
                db!.exec("COMMIT");
              } catch (innerErr) {
                try {
                  db!.exec("ROLLBACK");
                } catch {
                  /* ignore rollback error */
                }
                throw innerErr;
              }
            }, "session_read:auto_cursor_upsert");
          }
        }

        // ── Build response ────────────────────────────────────────────────
        const response: Record<string, unknown> = {
          messages: rawMessages,
          total: rawMessages.length,
          total_before_limit: totalAfterFilters,
          has_more: hasMore,
          session_id: args.session_id,
          ...(characterBudgetHit ? { character_budget_hit: true } : {}),
          ...(args.include_filter_stats ? { filtered_out: filteredOut } : {}),
        };

        return JSON.stringify(response);
      } catch (err) {
        return JSON.stringify({ error: String(err), session_id: args.session_id });
      }
    },
  });

  // ─────────────────────────────────────────────────────────────────────────
  // session_config — read/update runtime config  (+ SWDE-66 loopback allowlist)
  // axiom:trace work_item=SWDE-66 spec=specs/31-OpenCode-Integration-Contract.md#12b jira_ref=SWDE-66
  // ─────────────────────────────────────────────────────────────────────────
  const sessionConfigTool = tool({
    description:
      "Get or update runtime configuration for the OpenCode session plugin. " +
      "Changes take effect immediately — no restart needed. " +
      "Call with no args to see all current values. " +
      "Pass key+value to update a single setting. " +
      "Config keys: opencode_base_url, request_timeout_ms, spawn_timeout_ms, message_fetch_limit. " +
      "Note: Changes made via session_config are in-memory only and are NOT visible to codeops_config show. For persistent configuration, use codeops_config set and restart OpenCode.",
    args: {
      key: tool.schema.string().optional().describe("Config key to get or set"),
      value: tool.schema
        .string()
        .optional()
        .describe("New value for the key (omit to just read current value)"),
    },
    async execute(args) {
      try {
        if (!args.key) {
          return JSON.stringify({ config: cfg(), note: "Pass key+value to update a setting" });
        }

        const k = args.key as keyof typeof runtimeConfig;
        if (!(k in runtimeConfig)) {
          return JSON.stringify({
            error: `Unknown config key: '${args.key}'`,
            valid_keys: Object.keys(runtimeConfig),
          });
        }

        if (args.value === undefined) {
          return JSON.stringify({ key: args.key, value: runtimeConfig[k] });
        }

        // SWDE-66 SEC-SWDE66-001 CRITICAL fix: enforce loopback-only allowlist for opencode_base_url
        // Prevents SSRF via session_config redirection to internal metadata services.
        if (k === "opencode_base_url") {
          const urlStr = String(args.value);
          let parsedHost: string;
          try {
            const u = new URL(urlStr);
            // Only http/https schemes
            if (u.protocol !== "http:" && u.protocol !== "https:") {
              return JSON.stringify({
                error: `opencode_base_url must use http or https scheme. Got: '${u.protocol}'`,
              });
            }
            parsedHost = u.hostname;
          } catch {
            return JSON.stringify({
              error: `opencode_base_url is not a valid URL: '${urlStr}'`,
            });
          }
          if (!LOOPBACK_HOSTS.has(parsedHost)) {
            return JSON.stringify({
              error:
                `opencode_base_url must use a loopback address (127.0.0.1, localhost, ::1). ` +
                `Got hostname: '${parsedHost}'. Non-loopback URLs are rejected to prevent SSRF.`,
              allowed_hosts: [...LOOPBACK_HOSTS],
            });
          }
        }

        // Coerce to the correct type
        const existing = runtimeConfig[k];
        if (typeof existing === "number") {
          const n = Number(args.value);
          if (isNaN(n))
            return JSON.stringify({
              error: `'${args.key}' requires a number, got '${args.value}'`,
            });
          (runtimeConfig as Record<string, unknown>)[k] = n;
        } else {
          (runtimeConfig as Record<string, unknown>)[k] = args.value;
        }

        return JSON.stringify({ key: args.key, value: runtimeConfig[k], updated: true });
      } catch (err) {
        return JSON.stringify({ error: String(err) });
      }
    },
  });

  // ─────────────────────────────────────────────────────────────────────────
  // REQ-OC-CURSOR-001: session_stat — lightweight session stats, no content
  //
  // Returns event count, new_since_cursor, stalled_seconds, is_active, and
  // optionally last_tool_name (opt-in: include_tool_name: true).
  // Rate-limited: 500ms minimum per (caller_id, session_id) to prevent busy-polling.
  //
  // axiom:trace work_item=SWDE-66 spec=specs/31-OpenCode-Integration-Contract.md#REQ-OC-CURSOR-001 jira_ref=SWDE-66
  // ─────────────────────────────────────────────────────────────────────────
  const sessionStatTool = tool({
    description:
      "Lightweight session stats tool — returns event count, new-since-cursor, stalled_seconds, " +
      "is_active WITHOUT returning message content. " +
      "Use this to monitor multiple sessions cheaply before deciding which ones to session_read. " +
      "Rate-limited: 500ms minimum between calls per (caller_id, session_id). " +
      "Returns { session_id, total_events, new_since_cursor, stalled_seconds, is_active, cursor }.",
    args: {
      session_id: tool.schema.string().min(1).describe("Session ID to stat"),
      include_tool_name: tool.schema
        .boolean()
        .optional()
        .describe(
          "Opt-in: include last_tool_name in response (default false — privacy control, COMPLY-SWDE66-001)",
        ),
    },
    async execute(args, context) {
      // axiom:trace work_item=SWDE-66 spec=specs/31-OpenCode-Integration-Contract.md#REQ-OC-CURSOR-001 jira_ref=SWDE-66
      // Validate session_id (SEC-SWDE66-002)
      if (!validateSessionId(args.session_id)) {
        return JSON.stringify({
          error: `Invalid session_id format (must match ^[a-zA-Z0-9_-]{4,128}$): ${args.session_id}`,
          session_id: args.session_id,
        });
      }

      const callerId = getCallerId(context);

      // Rate limit: 500ms minimum per (caller_id, session_id)
      const rateLimitKey = `${callerId}:${args.session_id}`;
      const lastCallTs = _statRateLimit.get(rateLimitKey);
      const nowMs = Date.now();
      if (lastCallTs !== undefined && nowMs - lastCallTs < cfg().stat_rate_limit_ms) {
        const retryAfterMs = cfg().stat_rate_limit_ms - (nowMs - lastCallTs);
        return JSON.stringify({
          error: `Rate limited: wait ${retryAfterMs}ms before calling session_stat again`,
          rate_limited: true,
          retry_after_ms: retryAfterMs,
          session_id: args.session_id,
        });
      }
      _statRateLimit.set(rateLimitKey, nowMs);

      try {
        // Fetch all messages (content counted but NOT returned — REQ-OC-CURSOR-001)
        const msgs = await fetchMessages(args.session_id);
        if (msgs === null) {
          return JSON.stringify({
            error: "Could not reach OpenCode API to fetch session stats",
            session_id: args.session_id,
          });
        }

        const totalEvents = msgs.length;

        // Look up caller's cursor for new_since_cursor computation
        let newSinceCursor = totalEvents; // first call: all events are "new"
        let cursorId: string | null = null;
        if (db) {
          const cursorRow = db
            .prepare(
              `SELECT last_message_id FROM tool_cursors WHERE caller_id=? AND session_id=?`,
            )
            .get(callerId, args.session_id) as { last_message_id: string } | undefined;

          if (cursorRow && cursorRow.last_message_id) {
            cursorId = cursorRow.last_message_id;
            const idx = msgs.findIndex(
              (m) => String(m.id ?? m.message_id ?? "") === cursorId,
            );
            newSinceCursor = idx === -1 ? totalEvents : msgs.length - (idx + 1);
          }
        }

        // Compute stalled_seconds, last event metadata
        let stalledSeconds: number | null = null;
        let lastEventAt: string | null = null;
        let lastEventRole: string | null = null;
        let lastToolName: string | null = null;
        let currentCursor: string | null = null;

        if (msgs.length > 0) {
          const lastMsg = msgs[msgs.length - 1];
          lastEventRole = String(lastMsg.role ?? lastMsg.type ?? "") || null;
          currentCursor = String(lastMsg.id ?? lastMsg.message_id ?? "") || null;

          const ts =
            (lastMsg.created_at as string | undefined) ??
            (lastMsg.timestamp as string | undefined) ??
            (lastMsg.updatedAt as string | undefined);
          if (ts) {
            lastEventAt = ts;
            const t = new Date(ts).getTime();
            if (!isNaN(t)) stalledSeconds = Math.floor((Date.now() - t) / 1000);
          }

          // last_tool_name: OPT-IN only (COMPLY-SWDE66-001 fix)
          if (args.include_tool_name) {
            lastToolName =
              String(lastMsg.tool ?? lastMsg.tool_name ?? lastMsg.name ?? "") || null;
          }
        }

        // is_active: false when no messages (session not yet started); true when last event was within 60s
        const isActive = stalledSeconds === null ? false : stalledSeconds < 60;

        const response: Record<string, unknown> = {
          session_id: args.session_id,
          total_events: totalEvents,
          new_since_cursor: newSinceCursor,
          last_event_at: lastEventAt,
          last_event_role: lastEventRole,
          stalled_seconds: stalledSeconds,
          is_active: isActive,
          cursor: currentCursor,
        };

        // last_tool_name only included when opted in (COMPLY-SWDE66-001)
        if (args.include_tool_name) {
          response.last_tool_name = lastToolName;
        }

        return JSON.stringify(response);
      } catch (err) {
        return JSON.stringify({ error: String(err), session_id: args.session_id });
      }
    },
  });

  // ─────────────────────────────────────────────────────────────────────────
  // SWDE-64: REQ-OC-ROSTER-001 — session_track
  // ─────────────────────────────────────────────────────────────────────────
  const sessionTrackTool = tool({
    description:
      "Adopt an existing session into the roster for tracking. " +
      "Does NOT validate that the session exists in OpenCode (fire-and-forget tracking). " +
      "Re-tracking the same session by the same caller is an upsert (updates tags, notes). " +
      "Returns { tracked: true, entry } on success or { error } on failure.",
    args: {
      session_id: tool.schema.string().min(1).describe("Session ID to track (must be 4-128 chars, ^[a-zA-Z0-9_-]+)"),
      tags: tool.schema
        .array(tool.schema.string())
        .optional()
        .describe("Tags to associate with this session (max 20, max 64 chars each)"),
      notes: tool.schema
        .string()
        .optional()
        .describe("Human-readable notes (max 4096 chars; credentials are redacted)"),
    },
    async execute(args, context) {
      // axiom:trace work_item=SWDE-64 spec=specs/31-OpenCode-Integration-Contract.md#REQ-OC-ROSTER-001 jira_ref=SWDE-64
      if (!validateSessionId(args.session_id)) {
        return JSON.stringify({
          error: `Invalid session_id format (must match ^[a-zA-Z0-9_-]{4,128}$): ${args.session_id}`,
        });
      }
      if (!db) {
        return JSON.stringify({ error: "DB not available" });
      }

      const callerId = getCallerId(context);
      const rawTags = args.tags ?? [];

      // Validate tag limits
      if (rawTags.length > 20) {
        return JSON.stringify({ error: `Too many tags: max 20, got ${rawTags.length}` });
      }
      for (const t of rawTags) {
        if (t.length > 64) {
          return JSON.stringify({
            error: `Tag too long (max 64 chars): '${t.slice(0, 20)}...'`,
          });
        }
      }

      // Redact credentials from tags and notes
      const redactedTags = rawTags.map((t) => redactCredentials(t));
      const redactedNotes = redactCredentials(args.notes ?? "");
      const tagsJson = JSON.stringify(redactedTags);

      try {
        const success = sqliteWriteWithRetry(() => {
          db!.prepare(`
            INSERT INTO session_roster (session_id, origin, tags, added_by, added_at, notes)
            VALUES (?, 'adopted', ?, ?, datetime('now'), ?)
            ON CONFLICT(session_id, added_by) DO UPDATE SET
              tags     = excluded.tags,
              notes    = excluded.notes,
              added_at = datetime('now')
          `).run(args.session_id, tagsJson, callerId, redactedNotes);
        }, "session_track:upsert");

        if (!success) {
          return JSON.stringify({ error: "DB write failed after retries" });
        }

        const entry = db
          .prepare(
            `SELECT id, session_id, origin, tags, added_by, added_at, notes
             FROM session_roster WHERE session_id=? AND added_by=?`,
          )
          .get(args.session_id, callerId) as Record<string, unknown> | undefined;

        if (!entry) {
          return JSON.stringify({ error: "DB read-back failed after insert" });
        }

        // Deserialize tags JSON for the response
        try {
          entry.tags = JSON.parse(entry.tags as string);
        } catch {
          /* leave as string */
        }

        return JSON.stringify({ tracked: true, entry });
      } catch (err) {
        return JSON.stringify({ error: String(err) });
      }
    },
  });

  // ─────────────────────────────────────────────────────────────────────────
  // SWDE-64: REQ-OC-ROSTER-002 — session_roster
  // ─────────────────────────────────────────────────────────────────────────
  const sessionRosterTool = tool({
    description:
      "List tracked sessions. Default scope: caller's own entries. " +
      "Use show_all:true for all callers' entries. " +
      "Returns { entries, total, total_in_db, has_more, warning? }.",
    args: {
      show_all: tool.schema
        .boolean()
        .optional()
        .describe("Include entries from all callers (default: caller-scoped)"),
      tag_filter: tool.schema
        .string()
        .optional()
        .describe("Return only entries whose tags array contains this value"),
      enrich: tool.schema
        .boolean()
        .optional()
        .describe("Fetch live session status from OpenCode API for each entry (3s timeout per entry)"),
      limit: tool.schema
        .number()
        .optional()
        .describe("Max entries to return (default 50, max 50)"),
    },
    async execute(args, context) {
      // axiom:trace work_item=SWDE-64 spec=specs/31-OpenCode-Integration-Contract.md#REQ-OC-ROSTER-002 jira_ref=SWDE-64
      if (!db) {
        return JSON.stringify({ error: "DB not available" });
      }

      const callerId = getCallerId(context);
      const isUnknownCaller = callerId === "unknown";
      const limit = Math.min(50, Math.max(1, args.limit ?? 50));

      try {
        // Build WHERE clause
        let whereClause = args.show_all ? "1=1" : "added_by = ?";
        const whereParams: unknown[] = args.show_all ? [] : [callerId];

        if (args.tag_filter) {
          // JSON array contains check using parameterized binding
          whereClause += ` AND EXISTS (SELECT 1 FROM json_each(tags) WHERE value = ?)`;
          whereParams.push(args.tag_filter);
        }

        // Count for pagination
        const countRow = db
          .prepare(`SELECT COUNT(*) as cnt FROM session_roster WHERE ${whereClause}`)
          .get(...whereParams) as { cnt: number };
        const totalInDb = countRow.cnt;

        // Fetch with limit
        const rows = db
          .prepare(
            `SELECT id, session_id, origin, tags, added_by, added_at, notes
             FROM session_roster WHERE ${whereClause}
             ORDER BY added_at DESC
             LIMIT ?`,
          )
          .all(...whereParams, limit) as Record<string, unknown>[];

        // Deserialize tags JSON
        const entries = rows.map((r) => {
          let tags: unknown = r.tags;
          try {
            tags = JSON.parse(r.tags as string);
          } catch {
            /* leave as string */
          }
          return { ...r, tags };
        });

        // Optional enrichment (parallel, 3s timeout each)
        let enriched = false;
        if (args.enrich) {
          enriched = true;
          await Promise.all(
            entries.map(async (entry) => {
              try {
                const resp = await fetch(
                  `${cfg().opencode_base_url}/session/${encodeURIComponent(entry.session_id as string)}`,
                  {
                    headers: { Accept: "application/json" },
                    signal: AbortSignal.timeout(3000),
                  },
                );
                entry.live_status = resp.ok
                  ? ((await resp.json()) as Record<string, unknown>).status ?? null
                  : null;
              } catch {
                entry.live_status = null; // timeout or unreachable — never propagate
              }
            }),
          );
        }

        const response: Record<string, unknown> = {
          entries,
          total: entries.length,
          total_in_db: totalInDb,
          has_more: totalInDb > entries.length,
          ...(enriched ? { enriched: true } : {}),
        };

        // Warning when caller identity is unknown (REQ-OC-ROSTER-002 namespace note)
        if (isUnknownCaller && !args.show_all) {
          response.warning =
            "caller_id is 'unknown' (no sessionID in context). " +
            "All 'unknown'-added entries share the same scope. Use show_all:true in shared environments.";
        }

        return JSON.stringify(response);
      } catch (err) {
        return JSON.stringify({ error: String(err) });
      }
    },
  });

  // ─────────────────────────────────────────────────────────────────────────
  // SWDE-64: REQ-OC-ROSTER-003 — session_tag
  // ─────────────────────────────────────────────────────────────────────────
  const sessionTagTool = tool({
    description:
      "Add tags to a tracked session (caller's own entry only). " +
      "Merges with existing tags; deduplicates. Max 20 total tags. " +
      "Returns { updated: true, session_id, tags }.",
    args: {
      session_id: tool.schema.string().min(1).describe("Session ID to tag"),
      tags: tool.schema.array(tool.schema.string()).describe("Tags to add"),
    },
    async execute(args, context) {
      // axiom:trace work_item=SWDE-64 spec=specs/31-OpenCode-Integration-Contract.md#REQ-OC-ROSTER-003 jira_ref=SWDE-64
      if (!validateSessionId(args.session_id)) {
        return JSON.stringify({
          error: `Invalid session_id format: ${args.session_id}`,
        });
      }
      if (!db) return JSON.stringify({ error: "DB not available" });

      const callerId = getCallerId(context);

      try {
        const row = db
          .prepare(
            `SELECT tags FROM session_roster WHERE session_id=? AND added_by=?`,
          )
          .get(args.session_id, callerId) as { tags: string } | undefined;

        if (!row) {
          return JSON.stringify({
            error: `Entry not found for session_id='${args.session_id}' added_by='${callerId}'. Call session_track first.`,
          });
        }

        let existing: string[] = [];
        try {
          existing = JSON.parse(row.tags);
        } catch {
          /* treat as empty */
        }

        // Merge and deduplicate; apply redactCredentials to new tags
        const newRedacted = args.tags.map((t: string) => redactCredentials(t));
        const merged = [...new Set([...existing, ...newRedacted])];

        if (merged.length > 20) {
          return JSON.stringify({ error: `Tag limit exceeded: max 20, would be ${merged.length}` });
        }

        const mergedJson = JSON.stringify(merged);
        sqliteWriteWithRetry(() => {
          db!.prepare(
            `UPDATE session_roster SET tags=? WHERE session_id=? AND added_by=?`,
          ).run(mergedJson, args.session_id, callerId);
        }, "session_tag:update");

        return JSON.stringify({ updated: true, session_id: args.session_id, tags: merged });
      } catch (err) {
        return JSON.stringify({ error: String(err) });
      }
    },
  });

  // ─────────────────────────────────────────────────────────────────────────
  // SWDE-64: REQ-OC-ROSTER-003 — session_untag
  // ─────────────────────────────────────────────────────────────────────────
  const sessionUntagTool = tool({
    description:
      "Remove tags from a tracked session (caller's own entry only). " +
      "Returns { updated: true, session_id, tags, removed }.",
    args: {
      session_id: tool.schema.string().min(1).describe("Session ID to untag"),
      tags: tool.schema.array(tool.schema.string()).describe("Tags to remove"),
    },
    async execute(args, context) {
      // axiom:trace work_item=SWDE-64 spec=specs/31-OpenCode-Integration-Contract.md#REQ-OC-ROSTER-003 jira_ref=SWDE-64
      if (!validateSessionId(args.session_id)) {
        return JSON.stringify({ error: `Invalid session_id format: ${args.session_id}` });
      }
      if (!db) return JSON.stringify({ error: "DB not available" });

      const callerId = getCallerId(context);

      try {
        const row = db
          .prepare(
            `SELECT tags FROM session_roster WHERE session_id=? AND added_by=?`,
          )
          .get(args.session_id, callerId) as { tags: string } | undefined;

        if (!row) {
          return JSON.stringify({
            error: `Entry not found for session_id='${args.session_id}' added_by='${callerId}'.`,
          });
        }

        let existing: string[] = [];
        try {
          existing = JSON.parse(row.tags);
        } catch {
          /* treat as empty */
        }

        const toRemove = new Set(args.tags);
        const remaining = existing.filter((t) => !toRemove.has(t));
        const actuallyRemoved = existing.filter((t) => toRemove.has(t));

        sqliteWriteWithRetry(() => {
          db!.prepare(
            `UPDATE session_roster SET tags=? WHERE session_id=? AND added_by=?`,
          ).run(JSON.stringify(remaining), args.session_id, callerId);
        }, "session_untag:update");

        return JSON.stringify({
          updated: true,
          session_id: args.session_id,
          tags: remaining,
          removed: actuallyRemoved,
        });
      } catch (err) {
        return JSON.stringify({ error: String(err) });
      }
    },
  });

  // ─────────────────────────────────────────────────────────────────────────
  // SWDE-64: REQ-OC-ROSTER-007 — session_untrack
  // ─────────────────────────────────────────────────────────────────────────
  const sessionUntrackTool = tool({
    description:
      "Remove a roster entry. Default: removes caller's entry only. " +
      "remove_all:true removes ALL callers' entries for this session_id. " +
      "Returns { removed: <count>, session_id, remove_all }.",
    args: {
      session_id: tool.schema.string().min(1).describe("Session ID to untrack"),
      remove_all: tool.schema
        .boolean()
        .optional()
        .describe(
          "Remove all callers' entries for this session_id (privileged cross-caller operation — risk accepted for v1, DA-008)",
        ),
    },
    async execute(args, context) {
      // axiom:trace work_item=SWDE-64 spec=specs/31-OpenCode-Integration-Contract.md#REQ-OC-ROSTER-007 jira_ref=SWDE-64
      if (!validateSessionId(args.session_id)) {
        return JSON.stringify({ error: `Invalid session_id format: ${args.session_id}` });
      }
      if (!db) return JSON.stringify({ error: "DB not available" });

      const callerId = getCallerId(context);

      try {
        let removedCount = 0;
        if (args.remove_all) {
          const result = db
            .prepare(`DELETE FROM session_roster WHERE session_id=?`)
            .run(args.session_id);
          removedCount = result.changes;
        } else {
          const result = db
            .prepare(`DELETE FROM session_roster WHERE session_id=? AND added_by=?`)
            .run(args.session_id, callerId);
          removedCount = result.changes;
        }

        return JSON.stringify({
          removed: removedCount,
          session_id: args.session_id,
          remove_all: args.remove_all ?? false,
        });
      } catch (err) {
        return JSON.stringify({ error: String(err) });
      }
    },
  });

  // ─────────────────────────────────────────────────────────────────────────
  // Return plugin hooks — 13 tools total
  // ─────────────────────────────────────────────────────────────────────────
  return {
    tool: {
      // Original tools (REQ-GH-111 through REQ-GH-115)
      session_spawn: sessionSpawnTool,
      session_message: sessionMessageTool,
      session_interrupt: sessionInterruptTool,
      session_info: sessionInfoTool,
      session_list: sessionListTool,
      // Enhanced read/config tools
      session_read: sessionReadTool,
      session_config: sessionConfigTool,
      // SWDE-66: new cursor/stats tools
      session_stat: sessionStatTool,
      // SWDE-64: roster tools
      session_track: sessionTrackTool,
      session_roster: sessionRosterTool,
      session_tag: sessionTagTool,
      session_untag: sessionUntagTool,
      session_untrack: sessionUntrackTool,
    },
  };
};
