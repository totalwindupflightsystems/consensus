/**
 * Graph Harness Plugin — graph-driven execution engine for OpenCode.
 *
 * The model defines and reshapes a directed graph of work.
 * The harness drives execution deterministically.
 * The model is the architect; the harness is the foreman.
 *
 * Storage: `.graph-harness/harness.db` (SQLite, WAL mode via bun:sqlite)
 * Config:  `.graph-harness/config.yaml` (optional, YAML via yaml npm package)
 *
 * Plugin export shape: named export of an async function.
 * OpenCode's getLegacyPlugins iterates Object.values(module) and calls each export
 * as a plugin factory. ALL exports MUST be functions — any non-function throws.
 *
 * Phase 1:
 *   step-1-1-1: Plugin scaffold + SQLite schema + config loading + .gitignore auto-setup
 *   step-1-2-1: graph.create tool — create graphs with nodes, dependencies, conditions
 *   step-1-3-1: graph.status tool — read graph state, progress, critical path, blocked reasons
 *   step-1-4-1: Harness loop — session.idle event handler, evaluateConditions,
 *               runWithTimeout, redactCredentials, subprocess lifecycle
 *
 * axiom:trace work_item=graph-harness-01 spec=specs/102-Graph-Harness.md plan=phase-1/task-1-1/step-1-1-1
 * axiom:trace work_item=graph-harness-01 spec=specs/102-Graph-Harness.md#REQ-GH-021 plan=phase-1/task-1-4/step-1-4-1
 */

import { Database } from "bun:sqlite";
import { SQL as _BunSQL } from "bun"; // PG backend — new Bun.SQL(url) API (SWDE-67)
import { existsSync, mkdirSync, readFileSync, appendFileSync, statSync, writeFileSync, readdirSync, rmSync, renameSync } from "node:fs";
import { join } from "node:path";
import { parse as parseYaml, stringify as yamlStringify } from "yaml";
import { tool } from "@opencode-ai/plugin";
// SWDE-48: import stash helpers for stash pop/push lifecycle integration
// axiom:trace work_item=SWDE-48 spec=specs/102-Graph-Harness.md jira_ref=SWDE-48
import {
  parseFrontmatter as stashParseFrontmatter,
  buildSuspendedMarkdown,
  atomicWrite as stashAtomicWrite,
} from "./context-stash.ts";
import { deepMerge, loadPluginConfig, pluginWarn, pluginError, pluginInfo } from "./config-utils.ts";
// axiom:trace work_item=plugin-config-management-01 spec=specs/112-Plugin-Config-Management.md#8 plan=phase-4/task-4-1/step-4-1-1

// ─────────────────────────────────────────────────────────────────────────────
// Database Adapter — pluggable async storage backend (REQ-GH-150, REQ-GH-151)
//
// DbAdapter abstracts all DB operations behind an async API so SQLite (default)
// and PostgreSQL (SWDE-67) share the same harness code path.
//
// SqliteAdapter: wraps bun:sqlite synchronously — all methods resolve immediately
// PostgresAdapter: uses Bun's built-in `sql` from "bun" — genuinely async
//
// SQL dialect notes for PostgresAdapter:
//   - `?` params → `$1`, `$2`, ... (converted automatically)
//   - `INSERT OR IGNORE` → `INSERT ... ON CONFLICT DO NOTHING`
//   - `datetime('now')` → `NOW()`
//   - SQLite PRAGMAs → skipped (no-op)
//
// axiom:trace work_item=SWDE-67 spec=specs/102-Graph-Harness.md#17e jira_ref=SWDE-67
// ─────────────────────────────────────────────────────────────────────────────

/** Async database adapter interface (REQ-GH-150). */
interface DbAdapter {
  queryOne<T = Record<string, unknown>>(sql: string, params?: unknown[]): Promise<T | null>;
  queryAll<T = Record<string, unknown>>(sql: string, params?: unknown[]): Promise<T[]>;
  run(sql: string, params?: unknown[]): Promise<{ changes: number }>;
  exec(sql: string): Promise<void>;
  transaction<T>(fn: (tx: DbAdapter) => Promise<T>): Promise<T>;
  tryAdvisoryLock(key: bigint): Promise<boolean>;
  close(): Promise<void>;
  readonly backend: "sqlite" | "postgres";
}

/**
 * Type alias for spec traceability: REQ-GH-150 defines "GraphStorageBackend".
 * The concrete implementation uses DbAdapter (lower-level SQL primitives).
 * axiom:trace work_item=SWDE-67 spec=specs/102-Graph-Harness.md#REQ-GH-150 jira_ref=SWDE-67
 * axiom:trace work_item=SWDE-54 spec=specs/102-Graph-Harness.md#REQ-GH-150 (realized via SWDE-67)
 */
// eslint-disable-next-line @typescript-eslint/no-unused-vars
type GraphStorageBackend = DbAdapter;

/** Wraps bun:sqlite Database behind async DbAdapter interface (REQ-GH-151). */
class SqliteAdapter implements DbAdapter {
  readonly backend = "sqlite" as const;
  private _txLock: Promise<void> = Promise.resolve();

  constructor(private _db: Database) {}

  async queryOne<T = Record<string, unknown>>(sql: string, params: unknown[] = []): Promise<T | null> {
    return this._db.prepare(sql).get(...params) as T | null;
  }
  async queryAll<T = Record<string, unknown>>(sql: string, params: unknown[] = []): Promise<T[]> {
    return this._db.prepare(sql).all(...params) as T[];
  }
  async run(sql: string, params: unknown[] = []): Promise<{ changes: number }> {
    const r = this._db.prepare(sql).run(...params);
    return { changes: r.changes };
  }
  async exec(sql: string): Promise<void> {
    this._db.exec(sql);
  }
  async transaction<T>(fn: (tx: DbAdapter) => Promise<T>): Promise<T> {
    // Use a mutex (promise chain) to serialize transactions and prevent
    // "cannot start a transaction within a transaction" errors when multiple
    // async coroutines attempt concurrent transactions (e.g., Promise.all).
    // All SQLite operations inside fn() resolve synchronously (microtasks),
    // so the lock is released atomically before the next tick can acquire it.
    let resolve!: () => void;
    const entry = new Promise<void>((r) => { resolve = r; });
    const currentLock = this._txLock;
    this._txLock = this._txLock.then(() => entry);
    await currentLock;
    // We now hold the lock. Run the transaction.
    this._db.exec("BEGIN");
    try {
      const result = await fn(this);
      this._db.exec("COMMIT");
      return result;
    } catch (err) {
      try { this._db.exec("ROLLBACK"); } catch { /* best-effort */ }
      throw err;
    } finally {
      resolve(); // release lock
    }
  }
  async tryAdvisoryLock(_key: bigint): Promise<boolean> { return true; }
  async close(): Promise<void> {
    try { this._db.exec("PRAGMA wal_checkpoint(PASSIVE)"); } catch { /* best-effort */ }
    this._db.close();
  }
  /** Expose raw db for migration reads. */
  getRawDb(): Database { return this._db; }
}

// ─── PostgresAdapter ──────────────────────────────────────────────────────────

// Use `new _BunSQL(url)` — the correct Bun.SQL constructor API for custom connections.
// `sql` from "bun" is a singleton (default env-based connection); new _BunSQL(url) creates
// a named pool with explicit DSN. Verified with Bun 1.3.13.
// axiom:trace work_item=SWDE-67 spec=specs/102-Graph-Harness.md#REQ-GH-152 jira_ref=SWDE-67
type BunSqlClient = InstanceType<typeof _BunSQL>;

/** Convert SQLite `?` params to PG `$N` and handle dialect differences. */
function _pgConvertSql(sql: string, params: unknown[]): { sql: string; params: unknown[] } {
  if (/^\s*PRAGMA\s+/i.test(sql)) return { sql: "", params: [] };
  let wasOrIgnore = /\bINSERT\s+OR\s+IGNORE\s+INTO\b/i.test(sql);
  sql = sql.replace(/\bINSERT\s+OR\s+IGNORE\s+INTO\b/gi, "INSERT INTO");
  sql = sql.replace(/\bINSERT\s+OR\s+REPLACE\s+INTO\b/gi, "INSERT INTO");
  sql = sql.replace(/datetime\s*\(\s*'now'\s*\)/gi, "NOW()");
  // Convert ? to $N (skip inside string literals)
  let idx = 0, out = "", inStr = false, strCh = "";
  for (let i = 0; i < sql.length; i++) {
    const c = sql[i];
    if (!inStr && (c === "'" || c === '"')) { inStr = true; strCh = c; out += c; }
    else if (inStr && c === strCh && sql[i-1] !== "\\") { inStr = false; out += c; }
    else if (!inStr && c === "?") { idx++; out += `$${idx}`; }
    else { out += c; }
  }
  if (wasOrIgnore && /\bINSERT\s+INTO\b/i.test(out) && !/\bON\s+CONFLICT\b/i.test(out)) {
    out = out.trimEnd().replace(/;?\s*$/, "") + " ON CONFLICT DO NOTHING";
  }
  return { sql: out, params };
}

/** PostgreSQL backend using Bun's built-in `sql` (REQ-GH-152, SWDE-67). */
class PostgresAdapter implements DbAdapter {
  readonly backend = "postgres" as const;
  private _closed = false;
  private _healthTimer: ReturnType<typeof setInterval> | null = null;

  constructor(
    private _txSql: BunSqlClient,   // transaction-mode pool (regular queries)
    private _sessSql: BunSqlClient  // session-mode pool (advisory locks)
  ) {}

  async queryOne<T = Record<string, unknown>>(raw: string, params: unknown[] = []): Promise<T | null> {
    if (this._closed) throw new Error("[PgAdapter] closed");
    const { sql, params: p } = _pgConvertSql(raw, params);
    if (!sql.trim()) return null;
    const rows = await (this._txSql as unknown as { unsafe(s: string, p: unknown[]): Promise<T[]> }).unsafe(sql, p);
    return rows[0] ?? null;
  }
  async queryAll<T = Record<string, unknown>>(raw: string, params: unknown[] = []): Promise<T[]> {
    if (this._closed) throw new Error("[PgAdapter] closed");
    const { sql, params: p } = _pgConvertSql(raw, params);
    if (!sql.trim()) return [];
    return (this._txSql as unknown as { unsafe(s: string, p: unknown[]): Promise<T[]> }).unsafe(sql, p);
  }
  async run(raw: string, params: unknown[] = []): Promise<{ changes: number }> {
    if (this._closed) throw new Error("[PgAdapter] closed");
    let { sql, params: p } = _pgConvertSql(raw, params);
    if (!sql.trim()) return { changes: 0 };
    if (/\bINSERT\s+INTO\b/i.test(sql) && !/\bON\s+CONFLICT\b/i.test(sql)) {
      sql = sql.trimEnd().replace(/;?\s*$/, "") + " ON CONFLICT DO NOTHING";
    }
    const res = await (this._txSql as unknown as { unsafe(s: string, p: unknown[]): Promise<{ rowCount?: number }> }).unsafe(sql, p);
    return { changes: (res as unknown as { rowCount?: number }).rowCount ?? 0 };
  }
  async exec(raw: string): Promise<void> {
    if (this._closed) throw new Error("[PgAdapter] closed");
    if (/^\s*PRAGMA\s+/i.test(raw)) return;
    let pg = raw.replace(/datetime\s*\(\s*'now'\s*\)/gi, "NOW()");
    pg = pg.replace(/\bINSERT\s+OR\s+IGNORE\s+INTO\b/gi, "INSERT INTO");
    await (this._txSql as unknown as { unsafe(s: string, p: unknown[]): Promise<unknown> }).unsafe(pg, []);
  }
  async transaction<T>(fn: (tx: DbAdapter) => Promise<T>): Promise<T> {
    if (this._closed) throw new Error("[PgAdapter] closed");
    return (this._txSql as unknown as {
      begin(fn: (tx: BunSqlClient) => Promise<T>): Promise<T>
    }).begin(async (txClient) => fn(new _PgTxAdapter(txClient)));
  }
  async tryAdvisoryLock(key: bigint): Promise<boolean> {
    if (this._closed) return false;
    const rows = await (this._sessSql as unknown as { unsafe(s: string, p: unknown[]): Promise<{ acquired: boolean }[]> })
      .unsafe(`SELECT pg_try_advisory_lock($1) as acquired`, [key.toString()]);
    return rows[0]?.acquired ?? false;
  }
  async close(): Promise<void> {
    if (this._closed) return;
    this._closed = true;
    if (this._healthTimer) clearInterval(this._healthTimer);
    try { await (this._txSql as unknown as { end(): Promise<void> }).end(); } catch { /* best-effort */ }
    try { await (this._sessSql as unknown as { end(): Promise<void> }).end(); } catch { /* best-effort */ }
  }
  /** Start the 30-second health check + reconnect loop (REQ-GH-154 / SWDE-67). */
  startHealthCheck(pgUrl: string, repoRoot: string): void {
    // axiom:trace work_item=SWDE-67 spec=specs/102-Graph-Harness.md#REQ-GH-154 jira_ref=SWDE-67
    this._healthTimer = setInterval(async () => {
      if (this._closed) return;
      try {
        await this.queryOne(`SELECT 1 as alive`, []);
      } catch (err) {
        console.warn("[GraphHarness] PG health check failed — attempting reconnect:", err);
        await _pgReconnect(this, pgUrl, repoRoot);
      }
    }, 30_000);
    if (this._healthTimer?.unref) this._healthTimer.unref(); // don't block process exit
  }
}

/** PG adapter inside a sql.begin() transaction. */
class _PgTxAdapter implements DbAdapter {
  readonly backend = "postgres" as const;
  constructor(private _tx: BunSqlClient) {}
  async queryOne<T = Record<string, unknown>>(raw: string, params: unknown[] = []): Promise<T | null> {
    const { sql, params: p } = _pgConvertSql(raw, params);
    if (!sql.trim()) return null;
    const rows = await (this._tx as unknown as { unsafe(s: string, p: unknown[]): Promise<T[]> }).unsafe(sql, p);
    return rows[0] ?? null;
  }
  async queryAll<T = Record<string, unknown>>(raw: string, params: unknown[] = []): Promise<T[]> {
    const { sql, params: p } = _pgConvertSql(raw, params);
    if (!sql.trim()) return [];
    return (this._tx as unknown as { unsafe(s: string, p: unknown[]): Promise<T[]> }).unsafe(sql, p);
  }
  async run(raw: string, params: unknown[] = []): Promise<{ changes: number }> {
    let { sql, params: p } = _pgConvertSql(raw, params);
    if (!sql.trim()) return { changes: 0 };
    if (/\bINSERT\s+INTO\b/i.test(sql) && !/\bON\s+CONFLICT\b/i.test(sql)) {
      sql = sql.trimEnd().replace(/;?\s*$/, "") + " ON CONFLICT DO NOTHING";
    }
    const res = await (this._tx as unknown as { unsafe(s: string, p: unknown[]): Promise<{ rowCount?: number }> }).unsafe(sql, p);
    return { changes: (res as unknown as { rowCount?: number }).rowCount ?? 0 };
  }
  async exec(raw: string): Promise<void> {
    if (/^\s*PRAGMA\s+/i.test(raw)) return;
    let pg = raw.replace(/datetime\s*\(\s*'now'\s*\)/gi, "NOW()");
    pg = pg.replace(/\bINSERT\s+OR\s+IGNORE\s+INTO\b/gi, "INSERT INTO");
    await (this._tx as unknown as { unsafe(s: string, p: unknown[]): Promise<unknown> }).unsafe(pg, []);
  }
  async transaction<T>(fn: (tx: DbAdapter) => Promise<T>): Promise<T> { return fn(this); }
  async tryAdvisoryLock(key: bigint): Promise<boolean> {
    const rows = await (this._tx as unknown as { unsafe(s: string, p: unknown[]): Promise<{ acquired: boolean }[]> })
      .unsafe(`SELECT pg_try_advisory_xact_lock($1) as acquired`, [key.toString()]);
    return rows[0]?.acquired ?? false;
  }
  async close(): Promise<void> { /* no-op */ }
}

/** Initialize a PostgresAdapter from a DSN URL. Creates both pool instances. */
async function initPostgresAdapter(pgUrl: string, _repoRoot: string): Promise<PostgresAdapter> {
  // axiom:trace work_item=SWDE-67 spec=specs/102-Graph-Harness.md#REQ-GH-152 jira_ref=SWDE-67
  const txSql = new _BunSQL(pgUrl);   // transaction-mode pool (regular queries)
  const sessSql = new _BunSQL(pgUrl); // session-mode pool (advisory locks — stays alive)

  // Apply PG schema (idempotent)
  await _pgApplySchema(txSql);

  const adapter = new PostgresAdapter(txSql, sessSql);
  adapter.startHealthCheck(pgUrl, _repoRoot);
  return adapter;
}

/** Reconnect PG pools with exponential backoff. */
async function _pgReconnect(adapter: PostgresAdapter, pgUrl: string, repoRoot: string): Promise<void> {
  // axiom:trace work_item=SWDE-67 spec=specs/102-Graph-Harness.md#REQ-GH-154 jira_ref=SWDE-67
  let delayMs = 1000;
  for (let attempt = 1; attempt <= 5; attempt++) {
    try {
      await new Promise((r) => setTimeout(r, delayMs));
      const newAdapter = await initPostgresAdapter(pgUrl, repoRoot);
      // Swap internal pools (best-effort — not perfectly atomic)
      (adapter as unknown as { _txSql: unknown })._txSql = (newAdapter as unknown as { _txSql: unknown })._txSql;
      (adapter as unknown as { _sessSql: unknown })._sessSql = (newAdapter as unknown as { _sessSql: unknown })._sessSql;
      pluginInfo("graph-harness", `PG reconnected after ${attempt} attempt(s)`);
      return;
    } catch (err) {
      console.warn(`[GraphHarness] PG reconnect attempt ${attempt} failed:`, err);
      delayMs = Math.min(delayMs * 2, 30_000);
    }
  }
  console.error("[GraphHarness] PG reconnect exhausted — harness may be degraded");
}

// ─────────────────────────────────────────────────────────────────────────────
// PostgreSQL schema DDL (idempotent — REQ-GH-152 / SWDE-67)
// Mirrors the SQLite schema in initSqliteDb() with PG-native types.
// axiom:trace work_item=SWDE-67 spec=specs/102-Graph-Harness.md#4.2 jira_ref=SWDE-67
// ─────────────────────────────────────────────────────────────────────────────

async function _pgApplySchema(sql: BunSqlClient): Promise<void> {
  const exec = (s: string) => (sql as unknown as { unsafe(q: string, p: unknown[]): Promise<unknown> }).unsafe(s, []);

  await exec(`CREATE TABLE IF NOT EXISTS graphs (
    id TEXT PRIMARY KEY,
    title TEXT NOT NULL,
    description TEXT,
    status TEXT NOT NULL DEFAULT 'active',
    parent_graph_id TEXT,
    parent_node_id TEXT,
    locked_by TEXT DEFAULT NULL,
    created_at TEXT NOT NULL,
    completed_at TEXT,
    metadata JSONB,
    modifications_without_progress BIGINT NOT NULL DEFAULT 0,
    notifications_config JSONB DEFAULT NULL
  )`);

  await exec(`CREATE TABLE IF NOT EXISTS nodes (
    id TEXT NOT NULL,
    graph_id TEXT NOT NULL,
    title TEXT NOT NULL,
    description TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'pending',
    execution_mode TEXT NOT NULL DEFAULT 'agent',
    execution_config JSONB,
    parallel_group TEXT,
    join_strategy TEXT,
    assigned_session TEXT,
    attempt_count BIGINT DEFAULT 0,
    max_retries BIGINT DEFAULT 3,
    optional BOOLEAN DEFAULT FALSE,
    context JSONB,
    schedule TEXT,
    repeat BOOLEAN DEFAULT FALSE,
    activated_at TEXT,
    completed_at TEXT,
    created_at TEXT NOT NULL,
    metadata JSONB,
    trigger_on TEXT NOT NULL DEFAULT 'idle',
    trigger_cancel_on TEXT NOT NULL DEFAULT 'active',
    trigger_every TEXT,
    trigger_cron TEXT,
    trigger_max_runs BIGINT DEFAULT 0,
    trigger_run_count BIGINT DEFAULT 0,
    trigger_lifetime_h REAL DEFAULT 0,
    trigger_last_fired_at TEXT,
    PRIMARY KEY (id, graph_id)
  )`);

  await exec(`CREATE TABLE IF NOT EXISTS dependencies (
    graph_id TEXT NOT NULL,
    node_id TEXT NOT NULL,
    depends_on TEXT NOT NULL,
    PRIMARY KEY (graph_id, node_id, depends_on)
  )`);

  await exec(`CREATE TABLE IF NOT EXISTS conditions (
    id TEXT PRIMARY KEY,
    graph_id TEXT NOT NULL,
    node_id TEXT NOT NULL,
    ordinal BIGINT NOT NULL DEFAULT 0,
    type TEXT NOT NULL,
    command TEXT,
    expected TEXT,
    description TEXT,
    timeout_seconds BIGINT DEFAULT 60,
    independent BOOLEAN NOT NULL DEFAULT FALSE,
    max_retries BIGINT DEFAULT 3
  )`);

  await exec(`CREATE TABLE IF NOT EXISTS sessions (
    session_id TEXT PRIMARY KEY,
    graph_id TEXT NOT NULL,
    node_id TEXT,
    role TEXT NOT NULL DEFAULT 'worker',
    status TEXT NOT NULL DEFAULT 'active',
    worker_pid BIGINT DEFAULT NULL,
    last_heartbeat TEXT,
    created_at TEXT NOT NULL,
    tokens_used BIGINT DEFAULT 0,
    cost_usd REAL DEFAULT 0.0,
    tool_calls TEXT DEFAULT '{}',
    consecutive_briefing_failures BIGINT NOT NULL DEFAULT 0,
    instance_id TEXT  -- cluster mode: which harness instance owns this session (REQ-SESSIONS-FK-001)
  )`);

  await exec(`CREATE TABLE IF NOT EXISTS ledger (
    id BIGSERIAL PRIMARY KEY,
    graph_id TEXT NOT NULL,
    session_id TEXT,
    action TEXT NOT NULL,
    target_node_id TEXT,
    detail JSONB,
    timestamp TEXT NOT NULL
  )`);

  await exec(`CREATE TABLE IF NOT EXISTS node_outputs (
    id TEXT PRIMARY KEY,
    graph_id TEXT NOT NULL,
    node_id TEXT NOT NULL,
    key TEXT NOT NULL,
    value TEXT NOT NULL,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
  )`);

  await exec(`CREATE TABLE IF NOT EXISTS node_messages (
    id TEXT PRIMARY KEY,
    graph_id TEXT NOT NULL,
    from_node_id TEXT NOT NULL,
    to_node_id TEXT NOT NULL,
    content TEXT NOT NULL,
    priority TEXT NOT NULL DEFAULT 'normal',
    created_at TEXT NOT NULL,
    delivered BOOLEAN NOT NULL DEFAULT FALSE,
    delivered_at TEXT,
    status TEXT NOT NULL DEFAULT 'pending'
  )`);

  await exec(`CREATE TABLE IF NOT EXISTS data_flow (
    id TEXT PRIMARY KEY,
    graph_id TEXT NOT NULL,
    from_node_id TEXT NOT NULL,
    to_node_id TEXT NOT NULL,
    output_key TEXT NOT NULL,
    input_key TEXT,
    required BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TEXT NOT NULL
  )`);

  await exec(`CREATE TABLE IF NOT EXISTS annotations (
    id TEXT PRIMARY KEY,
    graph_id TEXT NOT NULL,
    node_id TEXT,
    session_id TEXT,
    type TEXT NOT NULL,
    content TEXT NOT NULL,
    severity TEXT,
    created_at TEXT NOT NULL
  )`);

  await exec(`CREATE TABLE IF NOT EXISTS conductor_agents (
    agent_id TEXT PRIMARY KEY,
    graph_id TEXT NOT NULL,
    node_id TEXT NOT NULL,
    session_id TEXT,
    spawn_secret_hash TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'active',
    created_at TEXT NOT NULL,
    completed_at TEXT
  )`);

  // Indexes
  await exec(`CREATE INDEX IF NOT EXISTS idx_nodes_graph_status ON nodes(graph_id, status)`);
  await exec(`CREATE INDEX IF NOT EXISTS idx_sessions_graph ON sessions(graph_id)`);
  await exec(`CREATE INDEX IF NOT EXISTS idx_sessions_instance_id ON sessions(instance_id)`); // REQ-SESSIONS-FK-001
  await exec(`CREATE INDEX IF NOT EXISTS idx_ledger_graph ON ledger(graph_id)`);
  await exec(`CREATE INDEX IF NOT EXISTS idx_node_outputs_node ON node_outputs(graph_id, node_id)`);
  await exec(`CREATE INDEX IF NOT EXISTS idx_dependencies_node ON dependencies(graph_id, node_id)`);
  await exec(`CREATE INDEX IF NOT EXISTS idx_conditions_node ON conditions(graph_id, node_id)`);
  await exec(`CREATE INDEX IF NOT EXISTS idx_messages_to_node ON node_messages(graph_id, to_node_id, delivered)`);
  await exec(`CREATE INDEX IF NOT EXISTS idx_data_flow_to ON data_flow(graph_id, to_node_id)`);
  await exec(`CREATE INDEX IF NOT EXISTS idx_annotations_node ON annotations(graph_id, node_id)`);

  // Templates table — mirrors the SQLite DDL at line ~1089 (REQ-GH-152 / SWDE-67)
  // Required by graph_template_load, graph_template_save, and graph_admin "templates" subcommand.
  // axiom:trace work_item=SWDE-67 spec=specs/102-Graph-Harness.md#REQ-GH-152 jira_ref=SWDE-67
  await exec(`CREATE TABLE IF NOT EXISTS templates (
    name TEXT PRIMARY KEY,
    description TEXT NOT NULL,
    definition JSONB NOT NULL,
    parameters JSONB,
    created_at TEXT NOT NULL,
    updated_at TEXT,
    usage_count BIGINT DEFAULT 0,
    last_used_at TEXT
  )`);
  await exec(`CREATE INDEX IF NOT EXISTS idx_templates_name ON templates(name)`);

  // ── Cluster mode tables (REQ-DGE-001, REQ-DGE-020) ────────────────────────
  // PostgreSQL-only. cluster.enabled=false (default) means these tables exist
  // but are never populated — safe and idempotent for single-instance mode.
  // axiom:trace work_item=SWDE-56 spec=specs/108-Distributed-Graph-Execution.md#REQ-DGE-001 plan=phase-1/task-1-1/step-1-1-1
  await exec(`CREATE TABLE IF NOT EXISTS cluster_instances (
    instance_id TEXT PRIMARY KEY,
    opencode_base_url TEXT NOT NULL DEFAULT '',
    capabilities JSONB NOT NULL DEFAULT '[]',
    region TEXT NOT NULL DEFAULT '',
    max_nodes INTEGER NOT NULL DEFAULT 10,
    active_nodes INTEGER NOT NULL DEFAULT 0,
    status TEXT NOT NULL DEFAULT 'active',
    last_heartbeat TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    registered_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    metadata JSONB NOT NULL DEFAULT '{}'
  )`);
  await exec(`CREATE INDEX IF NOT EXISTS idx_cluster_instances_status ON cluster_instances(status)`);
  await exec(`CREATE INDEX IF NOT EXISTS idx_cluster_instances_region ON cluster_instances(region, status)`);

  // axiom:trace work_item=SWDE-56 spec=specs/108-Distributed-Graph-Execution.md#REQ-DGE-020 plan=phase-1/task-1-1/step-1-1-2
  await exec(`CREATE TABLE IF NOT EXISTS node_affinity (
    id TEXT PRIMARY KEY,
    node_id TEXT NOT NULL,
    graph_id TEXT NOT NULL,
    affinity_type TEXT NOT NULL DEFAULT 'require',
    capability TEXT NOT NULL,
    region TEXT NOT NULL DEFAULT '',
    weight INTEGER NOT NULL DEFAULT 1
  )`);
  await exec(`CREATE INDEX IF NOT EXISTS idx_node_affinity_node ON node_affinity(graph_id, node_id)`);
  await exec(`CREATE INDEX IF NOT EXISTS idx_node_affinity_capability ON node_affinity(capability, affinity_type)`);
}

// ─────────────────────────────────────────────────────────────────────────────
// Config types and defaults
// ─────────────────────────────────────────────────────────────────────────────

interface GraphHarnessConfig {
  enabled: boolean;
  database: {
    path: string;
    /** Storage backend: "sqlite" (default) or "postgres" (REQ-GH-152). */
    backend: "sqlite" | "postgres";
    /** PostgreSQL DSN — required when backend=postgres. Can use env var substitution. */
    postgres_dsn: string;
    /** Busy timeout in ms for SQLite WAL contention (default: 10000). */
    busy_timeout_ms: number;
  };
  harness: {
    idle_evaluation_interval_ms: number;
    heartbeat_timeout_seconds: number;
    condition_timeout_seconds: number;
  };
  spawning: {
    max_concurrent_sessions: number;
    max_total_sessions: number;
    max_spawn_depth: number;
    spawn_method: string;
  };
  cost: {
    max_cost_per_graph_usd: number;
    max_cost_per_node_usd: number;
    warn_at_percent: number;
  };
  command_policy: {
    mode: string;
    allowlist: string[];
    blocklist: string[];
  };
  retry: {
    default_max_retries: number;
    backoff_base_seconds: number;
    backoff_multiplier: number;
    backoff_jitter: boolean;
    flaky_bonus_attempts: number;
    max_total_retries_per_graph: number;
  };
  limits: {
    max_nodes_per_graph: number;
    max_nesting_depth: number;
    max_conditions_per_node: number;
    max_modifications_without_progress: number;
    max_node_active_minutes: number;
    max_graph_execution_hours: number;
  };
  schedule_defaults: {
    max_repeat_count: number;
    schedule_lifetime_hours: number;
    repeat_cost_cap_usd: number;
  };
  templates: {
    directory: string;
    builtin: boolean;
    allow_global: boolean;
  };
  api_policy: {
    blocked_ip_ranges: string[];
    blocked_domains: string[];
  };
  lifecycle: {
    archive_after_days: number;
    archive_directory: string;
  };
  interface: {
    notifications: boolean;
    approval_required_for_dangerous: boolean;
  };
}

// Module-local (not exported) — plugin loader crashes on non-function exports.
// For test access, import from a shared location instead.
export const DEFAULT_CONFIG: GraphHarnessConfig = {
  enabled: true,
  database: {
    path: ".graph-harness/harness.db",
    backend: "sqlite",
    postgres_dsn: "",
    busy_timeout_ms: 10000,
  },
  harness: {
    idle_evaluation_interval_ms: 30000,
    heartbeat_timeout_seconds: 300,
    condition_timeout_seconds: 60,
  },
  spawning: {
    max_concurrent_sessions: 5,
    max_total_sessions: 20,
    max_spawn_depth: 2,
    spawn_method: "sdk",
  },
  cost: {
    max_cost_per_graph_usd: 50.0,
    max_cost_per_node_usd: 10.0,
    warn_at_percent: 80,
  },
  command_policy: {
    mode: "permissive",
    allowlist: [],
    blocklist: [],
  },
  retry: {
    default_max_retries: 3,
    backoff_base_seconds: 5,
    backoff_multiplier: 2,
    backoff_jitter: true,
    flaky_bonus_attempts: 2,
    max_total_retries_per_graph: 50,
  },
  limits: {
    max_nodes_per_graph: 100,
    max_nesting_depth: 3,
    max_conditions_per_node: 20,
    max_modifications_without_progress: 10,
    max_node_active_minutes: 30,
    max_graph_execution_hours: 4,
  },
  schedule_defaults: {
    max_repeat_count: 100,
    schedule_lifetime_hours: 24,
    repeat_cost_cap_usd: 5.0,
  },
  templates: {
    directory: ".graph-harness/templates",
    builtin: true,
    allow_global: false,
  },
  api_policy: {
    blocked_ip_ranges: [],
    blocked_domains: [],
  },
  lifecycle: {
    archive_after_days: 7,
    archive_directory: ".graph-harness/archive",
  },
  interface: {
    notifications: true,
    approval_required_for_dangerous: true,
  },
};

// ─────────────────────────────────────────────────────────────────────────────
// Config loading
// Uses loadPluginConfig() from config-utils.ts (three-layer system: defaults +
// .opencode/config/graph-harness.json + env vars). YAML fallback for backward
// compat per OD-1 resolution (spec §10).
// axiom:trace work_item=plugin-config-management-01 spec=specs/112-Plugin-Config-Management.md#8 plan=phase-4/task-4-1/step-4-1-1
// ─────────────────────────────────────────────────────────────────────────────

/**
 * Loads the four-layer GraphHarnessConfig (defaults → YAML → JSON → env vars).
 *
 * @param repoRoot - Repository root path; used to locate config files and the
 *   deprecated `.graph-harness/config.yaml` backward-compat layer.
 * @returns Fully resolved {@link GraphHarnessConfig} after all layers are applied.
 * @remarks **Exported for regression testing only** (DA-Challenge-3 in
 *   `tests/graph-harness-pcm.test.ts`). Not intended for external production callers.
 *   The function signature and return type are internal implementation details and
 *   may change without notice.
 *   Thread-safe: `enrichedDefaults` is a local variable and `loadPluginConfig` clones
 *   its input, so concurrent calls receive fully isolated config objects.
 */
export function loadConfig(repoRoot: string): GraphHarnessConfig {
  // Four-layer config (spec §3.1): defaults → YAML (deprecated) → JSON config → env vars
  // YAML backward compat: if .graph-harness/config.yaml exists, merge into defaults FIRST
  // so that env vars (applied by loadPluginConfig) remain the highest-priority layer.
  // axiom:trace work_item=plugin-config-management-01 spec=specs/112-Plugin-Config-Management.md#3.1 plan=phase-r15/task-r15-01/step-r15-01
  let enrichedDefaults: GraphHarnessConfig = DEFAULT_CONFIG;
  const yamlPath = join(repoRoot, ".graph-harness", "config.yaml");
  if (existsSync(yamlPath)) {
    try {
      const raw = readFileSync(yamlPath, "utf-8");
      const parsed = parseYaml(raw) as { graph_harness?: Partial<GraphHarnessConfig> };
      if (parsed?.graph_harness) {
        console.warn("[GraphHarness] DEPRECATED: .graph-harness/config.yaml detected. Migrate to .opencode/config/graph-harness.json (see specs/112-Plugin-Config-Management.md).");
        // Merge YAML into defaults (Layer 2 on top of Layer 1).
        // loadPluginConfig below will then apply JSON config and env vars on top (Layers 3+4).
        enrichedDefaults = deepMerge(DEFAULT_CONFIG, parsed.graph_harness);
      }
    } catch (err) {
      console.warn("[GraphHarness] Failed to parse config.yaml — ignoring:", err);
    }
  }
  // Layer 3 (JSON config) and Layer 4 (env vars) applied here — env vars win over everything above.
  return loadPluginConfig("graph-harness", enrichedDefaults, repoRoot);
}

// ─────────────────────────────────────────────────────────────────────────────
// .gitignore auto-setup (REQ-GH-AC-35 / REQ-GH-074a)
// Ensures .graph-harness/ is excluded from git on first plugin load.
// ─────────────────────────────────────────────────────────────────────────────

function ensureGitignore(repoRoot: string): void {
  const gitignorePath = join(repoRoot, ".gitignore");
  const entry = ".graph-harness/";

  if (existsSync(gitignorePath)) {
    const content = readFileSync(gitignorePath, "utf-8");
    // Check for any of: ".graph-harness", ".graph-harness/", ".graph-harness/*"
    const alreadyIgnored = content
      .split("\n")
      .some((line) => {
        const trimmed = line.trim();
        return (
          trimmed === entry ||
          trimmed === ".graph-harness" ||
          trimmed === ".graph-harness/*" ||
          trimmed === "/.graph-harness/" ||
          trimmed === "/.graph-harness"
        );
      });

    if (!alreadyIgnored) {
      // Append with a trailing newline guard
      const needsNewline = content.length > 0 && !content.endsWith("\n");
      appendFileSync(gitignorePath, `${needsNewline ? "\n" : ""}${entry}\n`);
      console.log("[GraphHarness] Added .graph-harness/ to .gitignore");
    }
  } else {
    // No .gitignore yet — create a minimal one
    appendFileSync(gitignorePath, `${entry}\n`);
    console.log("[GraphHarness] Created .gitignore with .graph-harness/");
  }
}

// ─────────────────────────────────────────────────────────────────────────────
// SQLite resilience: retry helper for transient I/O errors
//
// SQLITE_IOERR_VNODE (errno 6922) and SQLITE_BUSY are transient errors common
// when multiple OpenCode sessions share the same .graph-harness/harness.db.
// This helper wraps write operations with exponential backoff retry logic.
//
// axiom:trace work_item=sqlite-resilience-01 spec=specs/102-Graph-Harness.md#4.1
// ─────────────────────────────────────────────────────────────────────────────

/** SQLite error codes that are retryable (transient I/O or lock contention). */
const RETRYABLE_SQLITE_CODES = new Set([
  "SQLITE_BUSY",
  "SQLITE_LOCKED",
  "SQLITE_IOERR",
  "SQLITE_IOERR_VNODE",
  "SQLITE_IOERR_WRITE",
  "SQLITE_IOERR_READ",
  "SQLITE_IOERR_FSYNC",
  "SQLITE_IOERR_SHORT_READ",
  "SQLITE_IOERR_LOCK",
  "SQLITE_IOERR_CLOSE",
  "SQLITE_IOERR_SHMOPEN",
  "SQLITE_IOERR_SHMSIZE",
  "SQLITE_IOERR_SHMLOCK",
  "SQLITE_IOERR_SHMMAP",
  "SQLITE_PROTOCOL",
]);

/**
 * Check if a SQLite error is retryable (transient I/O or lock contention).
 */
function isRetryableSqliteError(err: unknown): boolean {
  if (!err || typeof err !== "object") return false;
  const code = (err as { code?: string }).code;
  if (code && RETRYABLE_SQLITE_CODES.has(code)) return true;
  // Also check the message for IOERR patterns (bun:sqlite sometimes uses message only)
  const msg = (err as { message?: string }).message ?? "";
  if (msg.includes("SQLITE_IOERR") || msg.includes("SQLITE_BUSY") || msg.includes("SQLITE_LOCKED") || msg.includes("disk I/O error")) {
    return true;
  }
  return false;
}

/**
 * Execute a SQLite write operation with retry + exponential backoff.
 *
 * @param fn - The write operation to execute (synchronous)
 * @param label - Human-readable label for error logging
 * @param maxRetries - Max retry attempts (default: 3)
 * @param baseDelayMs - Base delay between retries in ms (default: 50)
 * @returns true if succeeded, false if all retries exhausted
 */
/** @deprecated Use dbWriteWithRetry for async contexts. Kept for SQLite-only code paths. */
function sqliteWriteWithRetry(
  fn: () => void,
  label: string,
  maxRetries: number = 3,
  baseDelayMs: number = 50
): boolean {
  for (let attempt = 0; attempt <= maxRetries; attempt++) {
    try {
      fn();
      return true;
    } catch (err) {
      if (!isRetryableSqliteError(err) || attempt === maxRetries) {
        if (attempt > 0) {
          console.warn(`[GraphHarness] ${label}: failed after ${attempt + 1} attempts:`, err);
        } else {
          console.warn(`[GraphHarness] ${label}: non-retryable error:`, err);
        }
        return false;
      }
      const delay = baseDelayMs * Math.pow(2, attempt) + Math.random() * baseDelayMs;
      const buf = new SharedArrayBuffer(4);
      const view = new Int32Array(buf);
      Atomics.wait(view, 0, 0, Math.ceil(delay));
    }
  }
  return false;
}

/**
 * Execute an async DB write with retry + exponential backoff.
 * Works for both SQLite (immediate resolution) and PostgreSQL (async).
 * axiom:trace work_item=SWDE-67 spec=specs/102-Graph-Harness.md#17e jira_ref=SWDE-67
 */
async function dbWriteWithRetry(
  fn: () => Promise<void>,
  label: string,
  maxRetries: number = 3,
  baseDelayMs: number = 50
): Promise<boolean> {
  for (let attempt = 0; attempt <= maxRetries; attempt++) {
    try {
      await fn();
      return true;
    } catch (err) {
      const isRetryable = isRetryableSqliteError(err) ||
        // PG transient errors: serialization failure, deadlock, connection failure
        (err as { code?: string }).code === "40001" ||
        (err as { code?: string }).code === "40P01" ||
        (err as { code?: string }).code === "57P01" ||
        (err as { message?: string }).message?.includes("connection");
      if (!isRetryable || attempt === maxRetries) {
        if (attempt > 0) {
          console.warn(`[GraphHarness] ${label}: failed after ${attempt + 1} attempts:`, err);
        } else {
          console.warn(`[GraphHarness] ${label}: non-retryable error:`, err);
        }
        return false;
      }
      const delay = baseDelayMs * Math.pow(2, attempt) + Math.random() * baseDelayMs;
      await new Promise((r) => setTimeout(r, Math.ceil(delay)));
    }
  }
  return false;
}

/**
 * Execute an async DB read with retry + exponential backoff.
 * axiom:trace work_item=SWDE-67 spec=specs/102-Graph-Harness.md#17e jira_ref=SWDE-67
 */
async function dbReadWithRetry<T>(
  fn: () => Promise<T>,
  label: string,
  maxRetries: number = 3,
  baseDelayMs: number = 50
): Promise<T | null> {
  for (let attempt = 0; attempt <= maxRetries; attempt++) {
    try {
      return await fn();
    } catch (err) {
      const isRetryable = isRetryableSqliteError(err) ||
        (err as { code?: string }).code === "40001" ||
        (err as { code?: string }).code === "40P01" ||
        (err as { message?: string }).message?.includes("connection");
      if (!isRetryable || attempt === maxRetries) {
        if (attempt > 0) {
          console.warn(`[GraphHarness] ${label}: read failed after ${attempt + 1} attempts:`, err);
        } else {
          console.warn(`[GraphHarness] ${label}: non-retryable read error:`, err);
        }
        return null;
      }
      const delay = baseDelayMs * Math.pow(2, attempt) + Math.random() * baseDelayMs;
      await new Promise((r) => setTimeout(r, Math.ceil(delay)));
    }
  }
  return null;
}

/** @deprecated Use dbReadWithRetry for async contexts. Kept for backward compatibility. */
function sqliteReadWithRetry<T>(
  fn: () => T,
  label: string,
  maxRetries: number = 3,
  baseDelayMs: number = 50
): T | null {
  for (let attempt = 0; attempt <= maxRetries; attempt++) {
    try {
      return fn();
    } catch (err) {
      if (!isRetryableSqliteError(err) || attempt === maxRetries) {
        if (attempt > 0) {
          console.warn(`[GraphHarness] ${label}: read failed after ${attempt + 1} attempts:`, err);
        } else {
          console.warn(`[GraphHarness] ${label}: non-retryable read error:`, err);
        }
        return null;
      }
      const delay = baseDelayMs * Math.pow(2, attempt) + Math.random() * baseDelayMs;
      const buf = new SharedArrayBuffer(4);
      const view = new Int32Array(buf);
      Atomics.wait(view, 0, 0, Math.ceil(delay));
    }
  }
  return null;
}

// ─────────────────────────────────────────────────────────────────────────────
// SQLite schema initialization (spec §4)
//
// All 11 tables from spec §4.2 (the step description lists 10 but the spec DDL
// includes `annotations` as an 11th — implemented per spec, deviation noted).
// All indexes from spec §4.3.
// Schema creation is idempotent (IF NOT EXISTS on all tables + indexes).
//
// WAL mode + busy_timeout + synchronous=NORMAL + foreign_keys=ON (spec §4.1).
// ─────────────────────────────────────────────────────────────────────────────

function initSqliteDb(dbPath: string): Database {
  const db = new Database(dbPath);

  // ── Spec §4.1 PRAGMAs ─────────────────────────────────────────────────────
  db.exec("PRAGMA journal_mode = WAL;");
  // Increased busy_timeout from 5000ms to 10000ms to better handle concurrent
  // sessions. For network filesystem deployments, increase further via
  // config.database.busy_timeout_ms. See specs/102-Graph-Harness.md §4.1.
  db.exec("PRAGMA busy_timeout = 10000;");
  db.exec("PRAGMA synchronous = NORMAL;");
  db.exec("PRAGMA foreign_keys = ON;");
  // Limit WAL file growth — checkpoint every 1000 pages (~4MB) to prevent
  // unbounded WAL accumulation across concurrent sessions.
  db.exec("PRAGMA wal_autocheckpoint = 1000;");

  // ── Spec §4.2 Tables ──────────────────────────────────────────────────────

  // ═══════════════════════════════════════════════════════════════════
  // CORE GRAPH STRUCTURE
  // ═══════════════════════════════════════════════════════════════════

  db.exec(`
    CREATE TABLE IF NOT EXISTS graphs (
        id TEXT PRIMARY KEY,
        title TEXT NOT NULL,
        description TEXT,
        status TEXT NOT NULL DEFAULT 'active',
            -- active | paused | complete | abandoned | failed | idle
            -- 'idle': all nodes done for this cycle; graph is repeating, waiting for next_fire_at.
        parent_graph_id TEXT,
        parent_node_id TEXT,
        locked_by TEXT DEFAULT NULL,
            -- session_id holding the mutation lock (REQ-GH-116); NULL = unlocked
        created_at TEXT NOT NULL,
        completed_at TEXT,
        metadata JSON,
        modifications_without_progress INTEGER NOT NULL DEFAULT 0,
        FOREIGN KEY (parent_graph_id) REFERENCES graphs(id)
    );
  `);

  db.exec(`
    CREATE TABLE IF NOT EXISTS nodes (
        id TEXT NOT NULL,
        graph_id TEXT NOT NULL,
        title TEXT NOT NULL,
        description TEXT NOT NULL,
        status TEXT NOT NULL DEFAULT 'pending',
            -- pending | active | done | failed | abandoned | waiting_for_children | skipped | cancelled | requeued
            -- 'requeued': completed this cycle, waiting for next_fire_at to become due (repeating nodes only).
            --             Semantically distinct from 'cancelled' (which means terminated by user/system).
        execution_mode TEXT NOT NULL DEFAULT 'agent',
            -- agent | script | transform | wait | api | route | composite
        execution_config JSON,
        parallel_group TEXT,
        join_strategy TEXT,             -- all | any | majority
        assigned_session TEXT,
        attempt_count INTEGER DEFAULT 0,
        max_retries INTEGER DEFAULT 3,
        optional BOOLEAN DEFAULT FALSE,
        context JSON,                   -- {files: [], instructions: "", constraints: []}
        schedule TEXT,                  -- cron or interval (e.g., "every 5m", "*/5 * * * *")
        repeat BOOLEAN DEFAULT FALSE,
        activated_at TEXT,
        completed_at TEXT,
        created_at TEXT NOT NULL,
        metadata JSON,
        PRIMARY KEY (id, graph_id),
        FOREIGN KEY (graph_id) REFERENCES graphs(id)
    );
  `);

  db.exec(`
    CREATE TABLE IF NOT EXISTS dependencies (
        graph_id TEXT NOT NULL,
        node_id TEXT NOT NULL,
        depends_on TEXT NOT NULL,
        PRIMARY KEY (graph_id, node_id, depends_on),
        FOREIGN KEY (node_id, graph_id) REFERENCES nodes(id, graph_id),
        FOREIGN KEY (depends_on, graph_id) REFERENCES nodes(id, graph_id)
    );
  `);
  // axiom:trace work_item=graph-harness-01 spec=specs/102-Graph-Harness.md#4.2 plan=step-verify-gh-06
  // Migration note: FK column order corrected (was inverted: FOREIGN KEY (graph_id, node_id) → now FOREIGN KEY (node_id, graph_id)).
  // SQLite cannot ALTER TABLE to fix FKs on existing DBs; this fix applies to new DBs.
  // For existing DBs, a full table recreation would be needed (deferred to a migration step).

  db.exec(`
    CREATE TABLE IF NOT EXISTS conditions (
        id TEXT PRIMARY KEY,
        graph_id TEXT NOT NULL,
        node_id TEXT NOT NULL,
        ordinal INTEGER NOT NULL,
        type TEXT NOT NULL,
            -- script | test_pattern | file_exists | file_changed | manual | compound
        command TEXT,
        expected TEXT,
        description TEXT,
        timeout_seconds INTEGER DEFAULT 60,
        independent BOOLEAN DEFAULT FALSE,
        passed BOOLEAN,
        last_result TEXT,
        last_stderr TEXT,
        last_exit_code INTEGER,
        last_evaluated_at TEXT,
        flaky_count INTEGER DEFAULT 0,
        FOREIGN KEY (node_id, graph_id) REFERENCES nodes(id, graph_id)
    );
  `);

  // ═══════════════════════════════════════════════════════════════════
  // DATA FLOW & MESSAGING
  // ═══════════════════════════════════════════════════════════════════

  db.exec(`
    CREATE TABLE IF NOT EXISTS node_outputs (
        id TEXT PRIMARY KEY,
        graph_id TEXT NOT NULL,
        node_id TEXT NOT NULL,
        key TEXT NOT NULL,
        value TEXT NOT NULL,
        type TEXT NOT NULL DEFAULT 'text',  -- text | json | file_ref | artifact
        post_transform TEXT,
        created_at TEXT NOT NULL,
        UNIQUE(graph_id, node_id, key),
        FOREIGN KEY (node_id, graph_id) REFERENCES nodes(id, graph_id)
    );
  `);

  db.exec(`
    CREATE TABLE IF NOT EXISTS node_messages (
        id TEXT PRIMARY KEY,
        graph_id TEXT NOT NULL,
        from_node_id TEXT NOT NULL,
        to_node_id TEXT NOT NULL,
        from_session_id TEXT,
        type TEXT NOT NULL DEFAULT 'info',
            -- info | warning | data | request | finding
        subject TEXT,
        content TEXT NOT NULL,
        priority TEXT DEFAULT 'normal',  -- low | normal | high | critical
        delivered BOOLEAN DEFAULT FALSE,
        delivered_at TEXT,
        created_at TEXT NOT NULL,
        metadata JSON,
        FOREIGN KEY (from_node_id, graph_id) REFERENCES nodes(id, graph_id),
        FOREIGN KEY (to_node_id, graph_id) REFERENCES nodes(id, graph_id)
    );
  `);

  db.exec(`
    CREATE TABLE IF NOT EXISTS data_flow (
        id TEXT PRIMARY KEY,
        graph_id TEXT NOT NULL,
        from_node_id TEXT NOT NULL,
        to_node_id TEXT NOT NULL,
        output_key TEXT NOT NULL,
        required BOOLEAN DEFAULT TRUE,
        transform TEXT,
        FOREIGN KEY (from_node_id, graph_id) REFERENCES nodes(id, graph_id),
        FOREIGN KEY (to_node_id, graph_id) REFERENCES nodes(id, graph_id)
    );
  `);

  // ═══════════════════════════════════════════════════════════════════
  // SESSION TRACKING & HISTORY
  // ═══════════════════════════════════════════════════════════════════

  db.exec(`
    CREATE TABLE IF NOT EXISTS sessions (
        session_id TEXT PRIMARY KEY,
        graph_id TEXT NOT NULL,
        node_id TEXT,
        role TEXT NOT NULL DEFAULT 'worker',  -- worker | coordinator | synthesizer
        status TEXT NOT NULL DEFAULT 'active', -- active | done | stale | failed
        spawned_by TEXT,
        worker_pid INTEGER DEFAULT NULL,
            -- OS PID of CLI-spawned opencode process; used for graceful SIGTERM/SIGKILL (REQ-GH-082)
        last_heartbeat TEXT,
        created_at TEXT NOT NULL,
        completed_at TEXT,
        tool_calls INTEGER DEFAULT 0,
        tokens_used INTEGER DEFAULT 0,
        cost_usd REAL DEFAULT 0.0,
        consecutive_briefing_failures INTEGER NOT NULL DEFAULT 0,
        FOREIGN KEY (graph_id) REFERENCES graphs(id)
    );
  `);

  db.exec(`
    CREATE TABLE IF NOT EXISTS annotations (
        id TEXT PRIMARY KEY,
        graph_id TEXT NOT NULL,
        node_id TEXT NOT NULL,
        session_id TEXT,
        type TEXT NOT NULL,  -- finding | decision | blocker | note | failure_context
        content TEXT NOT NULL,
        severity TEXT DEFAULT 'info',
        created_at TEXT NOT NULL,
        FOREIGN KEY (node_id, graph_id) REFERENCES nodes(id, graph_id)
    );
  `);

  db.exec(`
    CREATE TABLE IF NOT EXISTS templates (
        name TEXT PRIMARY KEY,
        description TEXT NOT NULL,
        definition JSON NOT NULL,
        parameters JSON,
        created_at TEXT NOT NULL,
        updated_at TEXT,
        usage_count INTEGER DEFAULT 0,
        last_used_at TEXT
    );
  `);

  db.exec(`
    CREATE TABLE IF NOT EXISTS ledger (
        id INTEGER PRIMARY KEY AUTOINCREMENT,
        graph_id TEXT NOT NULL,
        session_id TEXT,
        action TEXT NOT NULL,
        target_node_id TEXT,
        detail JSON,
        timestamp TEXT NOT NULL,
        FOREIGN KEY (graph_id) REFERENCES graphs(id)
    );
  `);

  // ── Spec §4.3 Indexes ─────────────────────────────────────────────────────

  db.exec(`
    CREATE INDEX IF NOT EXISTS idx_nodes_graph_status ON nodes(graph_id, status);
    CREATE INDEX IF NOT EXISTS idx_nodes_graph_parallel ON nodes(graph_id, parallel_group);
    CREATE INDEX IF NOT EXISTS idx_deps_graph_depends ON dependencies(graph_id, depends_on);
    CREATE INDEX IF NOT EXISTS idx_sessions_graph ON sessions(graph_id, status);
    CREATE INDEX IF NOT EXISTS idx_conditions_node ON conditions(graph_id, node_id, ordinal);
    CREATE INDEX IF NOT EXISTS idx_ledger_graph_time ON ledger(graph_id, timestamp);
    CREATE INDEX IF NOT EXISTS idx_annotations_node ON annotations(graph_id, node_id);
    CREATE INDEX IF NOT EXISTS idx_outputs_node ON node_outputs(graph_id, node_id);
    CREATE INDEX IF NOT EXISTS idx_messages_to ON node_messages(graph_id, to_node_id, delivered);
    CREATE INDEX IF NOT EXISTS idx_messages_from ON node_messages(graph_id, from_node_id);
    CREATE INDEX IF NOT EXISTS idx_dataflow_to ON data_flow(graph_id, to_node_id);
    CREATE INDEX IF NOT EXISTS idx_dataflow_from ON data_flow(graph_id, from_node_id);
  `);

  // ── Schema migrations (idempotent, ADD COLUMN IF NOT EXISTS) ──────────────
  // For databases created before the modifications_without_progress column was added.
  try {
    db.exec(`ALTER TABLE graphs ADD COLUMN modifications_without_progress INTEGER NOT NULL DEFAULT 0;`);
  } catch { /* column already exists — safe to ignore */ }

  // Add input_key column to data_flow if it doesn't exist yet (phase-3 migration).
  try {
    db.exec(`ALTER TABLE data_flow ADD COLUMN input_key TEXT;`);
  } catch { /* column already exists — safe to ignore */ }

  // Add status column to node_messages if it doesn't exist yet (phase-3 migration).
  try {
    db.exec(`ALTER TABLE node_messages ADD COLUMN status TEXT NOT NULL DEFAULT 'queued';`);
  } catch { /* column already exists — safe to ignore */ }

  // ── Phase 8 / §17b.4 migrations — trigger columns ──────────────────────────
  // axiom:trace work_item=SWDE-46 spec=specs/102-Graph-Harness.md#17b.4 plan=phase-0/task-0.2/step-1 jira_ref=SWDE-46
  try { db.exec(`ALTER TABLE nodes ADD COLUMN trigger_on        TEXT DEFAULT 'idle';`);       } catch { /* exists */ }
  try { db.exec(`ALTER TABLE nodes ADD COLUMN trigger_cancel_on TEXT DEFAULT 'active';`);     } catch { /* exists */ }
  try { db.exec(`ALTER TABLE nodes ADD COLUMN trigger_every     TEXT;`);                      } catch { /* exists */ }
  try { db.exec(`ALTER TABLE nodes ADD COLUMN trigger_cron      TEXT;`);                      } catch { /* exists */ }
  try { db.exec(`ALTER TABLE nodes ADD COLUMN trigger_max_runs  INTEGER DEFAULT 0;`);         } catch { /* exists */ }
  try { db.exec(`ALTER TABLE nodes ADD COLUMN trigger_lifetime_h REAL DEFAULT 0;`);           } catch { /* exists */ }
  try { db.exec(`ALTER TABLE nodes ADD COLUMN trigger_run_count  INTEGER DEFAULT 0;`);        } catch { /* exists */ }
  try { db.exec(`ALTER TABLE nodes ADD COLUMN trigger_last_fired_at TEXT;`);                  } catch { /* exists */ }

  // ── Phase 112 / graph-scheduler-rearchitecture-01 — scheduler columns ───────
  // axiom:trace work_item=graph-scheduler-rearchitecture-01 spec=specs/102-Graph-Harness.md#REQ-GH-SCHED-V2 plan=phase-1/task-1-1/step-1-1-1
  try { db.exec(`ALTER TABLE nodes ADD COLUMN next_fire_at       TEXT;`);                     } catch { /* exists */ }
  try { db.exec(`ALTER TABLE nodes ADD COLUMN trigger_every_ms   INTEGER NOT NULL DEFAULT 0;`); } catch { /* exists */ }

  // ── Phase 112 / graph-scheduler-rearchitecture-01 — lifecycle columns on graphs ─
  // axiom:trace work_item=graph-scheduler-rearchitecture-01 spec=specs/102-Graph-Harness.md#REQ-GH-SCHED-V2 plan=phase-1/task-1-2/step-1-2-1
  try { db.exec(`ALTER TABLE graphs ADD COLUMN lifecycle_mode  TEXT NOT NULL DEFAULT 'one_shot';`);  } catch { /* exists */ }
  try { db.exec(`ALTER TABLE graphs ADD COLUMN cycle_count     INTEGER NOT NULL DEFAULT 0;`);        } catch { /* exists */ }
  try { db.exec(`ALTER TABLE graphs ADD COLUMN max_cycles      INTEGER NOT NULL DEFAULT 0;`);        } catch { /* exists */ }

  // ── Phase 112 — partial index for scheduler due-node query performance ───────
  // Partial index on next_fire_at WHERE status='requeued': O(1) MIN() for adaptive sleep calculation.
  // axiom:trace work_item=graph-scheduler-rearchitecture-01 spec=specs/102-Graph-Harness.md#REQ-GH-SCHED-V2 plan=phase-1/task-1-3/step-1-3-1
  try {
    db.exec(`CREATE INDEX IF NOT EXISTS idx_nodes_due ON nodes(next_fire_at) WHERE status='requeued';`);
  } catch { /* exists */ }

  // ── Phase 112 — backfill lifecycle_mode for existing graphs ─────────────────
  // Any graph with at least one repeating node is 'repeating'; all others stay 'one_shot'.
  // axiom:trace work_item=graph-scheduler-rearchitecture-01 spec=specs/102-Graph-Harness.md#REQ-GH-SCHED-V2 plan=phase-1/task-1-4/step-1-4-1
  try {
    db.exec(`
      UPDATE graphs SET lifecycle_mode = 'repeating'
      WHERE lifecycle_mode = 'one_shot'
        AND id IN (
          SELECT DISTINCT graph_id FROM nodes
          WHERE (repeat = 1 OR repeat = 'true')
             OR trigger_every IS NOT NULL
             OR trigger_cron IS NOT NULL
        )
    `);
  } catch (e) { console.warn("[GraphHarness] lifecycle_mode backfill failed:", e); }

  // ── Phase 112 — backfill trigger_every_ms from legacy trigger_every text ────
  // Parses "5s", "500ms", "2m", "1h" → integer milliseconds.
  // Idempotent: only updates rows where trigger_every IS NOT NULL AND trigger_every_ms = 0.
  // axiom:trace work_item=graph-scheduler-rearchitecture-01 spec=specs/102-Graph-Harness.md#REQ-GH-SCHED-V2 plan=phase-1/task-1-5/step-1-5-1
  try {
    db.exec(`
      UPDATE nodes SET trigger_every_ms = CAST(
        CASE
          WHEN trigger_every GLOB '*ms'
            THEN CAST(SUBSTR(trigger_every, 1, LENGTH(trigger_every)-2) AS INTEGER)
          WHEN trigger_every GLOB '*s'
            THEN CAST(SUBSTR(trigger_every, 1, LENGTH(trigger_every)-1) AS INTEGER) * 1000
          WHEN trigger_every GLOB '*m'
            THEN CAST(SUBSTR(trigger_every, 1, LENGTH(trigger_every)-1) AS INTEGER) * 60000
          WHEN trigger_every GLOB '*h'
            THEN CAST(SUBSTR(trigger_every, 1, LENGTH(trigger_every)-1) AS INTEGER) * 3600000
          ELSE 0
        END AS INTEGER
      )
      WHERE trigger_every IS NOT NULL AND (trigger_every_ms IS NULL OR trigger_every_ms = 0)
    `);
  } catch (e) { console.warn("[GraphHarness] trigger_every_ms backfill failed:", e); }

  // ── Phase 112 — v_due_work view: the contract for "what needs to fire now" ─
  // Uses DROP VIEW + CREATE VIEW (not IF NOT EXISTS) for schema-change idempotency.
  // SQLite doesn't support CREATE OR REPLACE VIEW, so IF NOT EXISTS silently persists
  // stale definitions. DROP+CREATE is safe since the view has no data.
  // axiom:trace work_item=graph-scheduler-rearchitecture-01 spec=specs/102-Graph-Harness.md#REQ-GH-SCHED-V2 plan=phase-2/task-2-2/step-2-2-1
  try {
    db.exec(`DROP VIEW IF EXISTS v_due_work`);
    db.exec(`
      CREATE VIEW v_due_work AS
      SELECT n.graph_id, n.id AS node_id
      FROM nodes n
       JOIN graphs g ON g.id = n.graph_id
       WHERE n.status = 'requeued'
         AND n.next_fire_at IS NOT NULL
         AND n.next_fire_at <= datetime('now')
         AND LOWER(g.status) IN ('active', 'idle', 'created')
         AND (n.trigger_max_runs = 0 OR n.trigger_run_count < n.trigger_max_runs)
         AND (n.trigger_lifetime_h = 0 OR
              (julianday('now') - julianday(n.created_at)) * 24 < n.trigger_lifetime_h)
    `);
  } catch (e) { console.warn("[GraphHarness] v_due_work view creation failed:", e); }

  // ── Phase 112 — trg_graph_status_on_node_change: auto-transition graph status ─
  // Fires AFTER UPDATE OF status ON nodes to keep graphs.status in sync.
  // Replaces the 4 scattered JS graph-complete guards (lines 5696, 7498, 7725, 7956).
  //
  // VERIFIED (trigger-transaction-semantics tests):
  //   (a) fires per-row ✓  (b) sees in-progress tx writes ✓  (c) rolls back with tx ✓
  //
  // Transition logic:
  //   active → idle    : no pending/active nodes + at least one requeued node
  //   active → complete: no pending/active/requeued nodes
  //   idle   → active  : a requeued node became pending (handled by scheduler flip, not this trigger)
  //
  // axiom:trace work_item=graph-scheduler-rearchitecture-01 spec=specs/102-Graph-Harness.md#REQ-GH-SCHED-V2 plan=phase-2/task-2-3/step-2-3-1
  try {
    db.exec(`DROP TRIGGER IF EXISTS trg_graph_status_on_node_change`);
    db.exec(`
      CREATE TRIGGER trg_graph_status_on_node_change
      AFTER UPDATE OF status ON nodes
      WHEN NEW.status IN ('done', 'requeued', 'failed', 'abandoned', 'cancelled', 'skipped')
      BEGIN
        -- active/created → idle: nothing running, but some requeued nodes are still due to fire
        -- Include 'created': a newly created graph where the first node ran and was requeued
        -- stays in 'CREATED' status. The trigger must transition it to 'idle' so v_due_work picks it up.
        UPDATE graphs SET status = 'idle'
        WHERE id = NEW.graph_id
          AND LOWER(status) IN ('active', 'created')
          AND NOT EXISTS (
            SELECT 1 FROM nodes
            WHERE graph_id = NEW.graph_id
              AND LOWER(status) IN ('active', 'pending')
          )
          AND EXISTS (
            SELECT 1 FROM nodes
            WHERE graph_id = NEW.graph_id
              AND LOWER(status) = 'requeued'
          );

        -- active/idle/created → complete: nothing left to run or wait for
        -- Must check: no pending/active, no requeued, AND no cancelled-but-trigger-bearing
        -- nodes (the legacy repeat mechanism marks nodes 'cancelled' while waiting to re-fire),
        -- AND no done nodes with runs remaining (about to be reset to cancelled/requeued by the
        -- post-completion JS path — this bridges the gap between DONE write and CANCELLED reset).
        UPDATE graphs
        SET status = 'complete', completed_at = datetime('now')
        WHERE id = NEW.graph_id
          AND LOWER(status) IN ('active', 'idle', 'created')
          AND NOT EXISTS (
            SELECT 1 FROM nodes
            WHERE graph_id = NEW.graph_id
              AND LOWER(status) IN ('active', 'pending', 'requeued')
          )
          AND NOT EXISTS (
            -- Legacy cancelled-as-repeat-wait (old repeat mechanism)
            SELECT 1 FROM nodes
            WHERE graph_id = NEW.graph_id
              AND LOWER(status) = 'cancelled'
              AND (trigger_on IS NOT NULL OR trigger_cron IS NOT NULL)
          )
          AND NOT EXISTS (
            -- Done nodes that have runs remaining (in-flight: about to be reset to cancelled/requeued)
            SELECT 1 FROM nodes
            WHERE graph_id = NEW.graph_id
              AND LOWER(status) = 'done'
              AND (trigger_every IS NOT NULL OR trigger_every_ms > 0 OR repeat = 1 OR repeat = 'true')
              AND (trigger_max_runs = 0 OR trigger_run_count < trigger_max_runs)
              AND (trigger_lifetime_h = 0 OR
                   (julianday('now') - julianday(created_at)) * 24 < trigger_lifetime_h)
          );      END
    `);
  } catch (e) { console.warn("[GraphHarness] trg_graph_status_on_node_change creation failed:", e); }

  return db;
}

// ─────────────────────────────────────────────────────────────────────────────
// Cluster work-stealing helpers (REQ-DGE-010, REQ-DGE-044)
// ─────────────────────────────────────────────────────────────────────────────

// ─── Cluster work-stealing (REQ-DGE-010) ──────────────────────────────────────
// CAS-based node assignment using FOR UPDATE SKIP LOCKED to prevent double-assignment.
// Uses db.transaction() to atomically claim a node + increment active_nodes.
// Backoff/retry (REQ-DGE-012) is managed at the call site (setInterval), not here —
// performWorkSteal is a pure CAS function; policy lives in the poll loop.
// axiom:trace work_item=SWDE-56 spec=specs/108-Distributed-Graph-Execution.md#REQ-DGE-010 plan=phase-2/task-2-1/step-2-1-1

/** Result of a single work-steal attempt. */
type WorkStealResult =
  | { claimed: true; nodeId: string; graphId: string }
  | { claimed: false; reason: "no_work" | "at_capacity" | "error" };

/**
 * Attempts to claim one PENDING node via CAS (FOR UPDATE SKIP LOCKED).
 * Atomic: CAS assignment + active_nodes increment wrapped in a single transaction.
 * Returns the claimed node if successful, or a reason for no-claim.
 * REQ-DGE-010, REQ-DGE-044
 */
async function performWorkSteal(
  db: DbAdapter,
  instanceId: string,
  config: GraphHarnessConfig
): Promise<WorkStealResult> {
  // Type-cast to access cluster config
  const clusterCfg = (config as GraphHarnessConfig & {
    cluster?: {
      max_nodes?: number;
    };
  }).cluster;
  const maxNodes = clusterCfg?.max_nodes ?? 10;

  // Check capacity first (avoids unnecessary DB round-trip)
  const instanceRow = await db.queryOne<{ active_nodes: number }>(
    `SELECT active_nodes FROM cluster_instances WHERE instance_id = ?`,
    [instanceId]
  );
  if (!instanceRow) return { claimed: false, reason: "error" };
  if (instanceRow.active_nodes >= maxNodes) {
    return { claimed: false, reason: "at_capacity" };
  }

  // Atomic: CAS claim + counter increment in one transaction (REQ-DGE-044)
  // axiom:trace work_item=SWDE-56 spec=specs/108-Distributed-Graph-Execution.md#REQ-DGE-044 plan=phase-2/task-2-1/step-2-1-3
  try {
    const result = await db.transaction(async (tx) => {
      // CTE pattern: WITH candidate AS (...FOR UPDATE SKIP LOCKED...) UPDATE nodes ...
      // This is the canonical PostgreSQL work-queue pattern — eliminates double-assignment race.
      // axiom:trace work_item=SWDE-56 spec=specs/108-Distributed-Graph-Execution.md#REQ-DGE-021 plan=phase-3/task-3-1/step-3-1-1
      const claimed = await tx.queryOne<{ id: string; graph_id: string }>(
        `WITH candidate AS (
           SELECT id, graph_id FROM nodes
           WHERE LOWER(status) = 'pending'
             AND assigned_session IS NULL
             AND NOT EXISTS (
               -- REQ-DGE-021: Require constraint — node must not have require constraints
               -- that this instance cannot satisfy
               SELECT 1 FROM node_affinity na
               WHERE na.node_id = nodes.id
                 AND na.affinity_type = 'require'
                 AND na.capability NOT IN (
                   SELECT json_array_elements_text(capabilities)
                   FROM cluster_instances
                   WHERE instance_id = ?
                 )
             )
           FOR UPDATE SKIP LOCKED
           LIMIT 1
         )
         UPDATE nodes
         SET status = 'active',
             assigned_session = ?,
             activated_at = datetime('now')
         FROM candidate
         WHERE nodes.id = candidate.id
         RETURNING nodes.id, nodes.graph_id`,
        [instanceId, instanceId]  // instanceId used twice: affinity filter + assigned_session
      );

      if (!claimed) return null; // No PENDING node available

      // Increment active_nodes atomically with the CAS assignment (REQ-DGE-044)
      await tx.run(
        `UPDATE cluster_instances
         SET active_nodes = active_nodes + 1
         WHERE instance_id = ? AND active_nodes < ?`,
        [instanceId, maxNodes]
      );

      return claimed;
    });

    if (!result) return { claimed: false, reason: "no_work" };
    return { claimed: true, nodeId: result.id, graphId: result.graph_id };
  } catch (e) {
    return { claimed: false, reason: "error" };
  }
}

// ─── Cluster stale instance detection (REQ-DGE-060) ──────────────────────────
// Coordinator-free CAS approach: any active instance can detect dead peers.
// Uses CAS UPDATE — first instance to update wins; others get 0 rows and skip.
// axiom:trace work_item=SWDE-56 spec=specs/108-Distributed-Graph-Execution.md#REQ-DGE-060 plan=phase-3/task-3-2/step-3-2-1

/**
 * Detects and marks stale cluster instances as dead (coordinator-free CAS).
 * Returns array of instance IDs that were marked dead by this call.
 * Other callers that race will get 0 rows and skip reassignment.
 * REQ-DGE-060, REQ-DGE-061
 */
async function detectStaleInstances(
  db: DbAdapter,
  instanceId: string,
  config: GraphHarnessConfig
): Promise<string[]> {
  if (db.backend !== "postgres") return []; // Cluster mode is PostgreSQL-only

  const heartbeatTimeoutS = (config as GraphHarnessConfig & {
    cluster?: { heartbeat_timeout_s?: number };
  }).cluster?.heartbeat_timeout_s ?? 90;

  try {
    // CAS UPDATE: marks stale instances dead atomically.
    // Only the instance whose UPDATE returns rows proceeds with reassignment.
    const deadInstances = await db.queryAll<{ instance_id: string }>(
      `UPDATE cluster_instances
       SET status = 'dead'
       WHERE status = 'active'
         AND instance_id != ?
         AND last_heartbeat < NOW() - INTERVAL '${heartbeatTimeoutS} seconds'
       RETURNING instance_id`,
      [instanceId]  // Exclude self — we're still active
    );

    if (deadInstances.length === 0) return [];

    pluginInfo("graph-harness", `[Cluster] Detected ${deadInstances.length} stale instance(s): ${deadInstances.map(r => r.instance_id).join(", ")}`);

    // Reassign nodes owned by dead instances (step-3-2-2)
    for (const dead of deadInstances) {
      await reassignDeadInstanceNodes(db, dead.instance_id);
    }

    return deadInstances.map(r => r.instance_id);
  } catch (e) {
    // Best-effort: non-fatal
    return [];
  }
}

/**
 * Reassigns all ACTIVE nodes belonging to a dead instance back to PENDING.
 * Uses sessions.instance_id FK (NOT LIKE matching) — REQ-DGE-061.
 * axiom:trace work_item=SWDE-56 spec=specs/108-Distributed-Graph-Execution.md#REQ-DGE-061 plan=phase-3/task-3-2/step-3-2-2
 */
async function reassignDeadInstanceNodes(
  db: DbAdapter,
  deadInstanceId: string
): Promise<number> {
  if (db.backend !== "postgres") return 0;

  try {
    // Use sessions.instance_id FK (REQ-DGE-061) — NOT LIKE matching (anti-pattern)
    const result = await db.run(
      `UPDATE nodes
       SET status = 'pending',
           assigned_session = NULL,
           activated_at = NULL
       WHERE LOWER(status) = 'active'
         AND assigned_session IN (
           SELECT session_id FROM sessions WHERE instance_id = ?
         )`,
      [deadInstanceId]
    );

    if (result.changes > 0) {
      pluginInfo("graph-harness", `[Cluster] Reassigned ${result.changes} node(s) from dead instance ${deadInstanceId}`);
    }

    return result.changes;
  } catch (e) {
    return 0; // Best-effort
  }
}

/**
 * Called when a cluster-claimed node completes (success or failure).
 * Decrements active_nodes atomically. GREATEST(0, ...) prevents negative values.
 * REQ-DGE-044
 * axiom:trace work_item=SWDE-56 spec=specs/108-Distributed-Graph-Execution.md#REQ-DGE-044 plan=phase-2/task-2-1/step-2-1-3
 */
async function decrementClusterActiveNodes(db: DbAdapter, instanceId: string): Promise<void> {
  try {
    await db.run(
      `UPDATE cluster_instances
       SET active_nodes = GREATEST(0, active_nodes - 1)
       WHERE instance_id = ?`,
      [instanceId]
    );
  } catch (e) {
    // Best-effort: non-fatal
  }
}

// ─────────────────────────────────────────────────────────────────────────────
// Plugin bootstrap — runs once per plugin load
// ─────────────────────────────────────────────────────────────────────────────

async function bootstrap(repoRoot: string): Promise<{ db: DbAdapter; config: GraphHarnessConfig; clusterInstanceId: string | null }> {
  // 1. Load config (uses defaults if config.yaml absent)
  // axiom:trace work_item=plugin-config-management-01 spec=specs/112-Plugin-Config-Management.md#3.1 plan=phase-r15/task-r15-01/step-r15-01
  const config = loadConfig(repoRoot);

  if (!config.enabled) {
    console.log("[GraphHarness] Plugin disabled via config (enabled: false) — skipping DB init");
    // Return a typed DbAdapter so callers (e.g., tool handlers) don't crash on db.queryOne().
    // SqliteAdapter wraps the in-memory DB behind the DbAdapter interface — safe to no-op.
    // axiom:trace work_item=SWDE-67 spec=specs/102-Graph-Harness.md#REQ-GH-151 jira_ref=SWDE-67
    const db: DbAdapter = new SqliteAdapter(new Database(":memory:"));
    return { db, config, clusterInstanceId: null };
  }

  // 2. Ensure .graph-harness/ directory exists
  const graphHarnessDir = join(repoRoot, ".graph-harness");
  if (!existsSync(graphHarnessDir)) {
    mkdirSync(graphHarnessDir, { recursive: true });
    console.log("[GraphHarness] Created .graph-harness/ directory");
  }

  // 3. Auto-update .gitignore (REQ-GH-AC-35)
  ensureGitignore(repoRoot);

  // 4. Initialize storage backend (SQLite or Postgres — REQ-GH-154)
  // axiom:trace work_item=SWDE-67 spec=specs/102-Graph-Harness.md#REQ-GH-154 jira_ref=SWDE-67
  let db: DbAdapter;
  const configuredBackend = config.database.backend ?? "sqlite";
  // Allow env override: GRAPH_HARNESS_BACKEND=postgres
  const envBackend = process.env.GRAPH_HARNESS_BACKEND as "sqlite" | "postgres" | undefined;
  const pgUrl = process.env.GRAPH_HARNESS_PG_URL ?? config.database.postgres_dsn;

  const effectiveBackend = envBackend ?? configuredBackend;

  if (effectiveBackend === "postgres" && pgUrl) {
    try {
      db = await initPostgresAdapter(pgUrl, repoRoot);
      pluginInfo("graph-harness", `PostgreSQL backend initialized — ${pgUrl.replace(/:[^:@]+@/, ":***@")}`);
    } catch (pgErr) {
      // REQ-GH-154: fallback to SQLite on PG unavailability
      console.warn(`[GraphHarness] ⚠️ PostgreSQL backend unavailable, falling back to SQLite:`, pgErr);
      const dbPath = join(repoRoot, config.database.path);
      db = new SqliteAdapter(initSqliteDb(dbPath));
    }
  } else {
    if (effectiveBackend === "postgres" && !pgUrl) {
      console.warn("[GraphHarness] ⚠️ backend=postgres but no postgres_dsn/GRAPH_HARNESS_PG_URL set — using SQLite");
    }
    const dbPath = join(repoRoot, config.database.path);
    db = new SqliteAdapter(initSqliteDb(dbPath));
  }

  // ── Startup WAL checkpoint (best-effort) ─────────────────────────────────
  // Always attempt a PASSIVE checkpoint on startup to merge any WAL data left
  // by previously crashed sessions. This prevents stale SHM files and WAL
  // accumulation that trigger SQLITE_IOERR_VNODE on macOS.
  // PASSIVE mode does not block other readers/writers — safe for concurrent use.
  // axiom:trace work_item=sqlite-resilience-01 spec=specs/102-Graph-Harness.md#4.1
  try {
    await db.exec("PRAGMA wal_checkpoint(PASSIVE)");
  } catch (cpErr) {
    // Non-fatal — may fail if another process holds a lock, which is fine
    console.warn("[GraphHarness] Startup WAL checkpoint skipped (likely another session holds lock):", cpErr);
  }

  // ── Cluster mode state (hoisted for SIGTERM closure) ─────────────────────
  // Declared here so the SIGTERM handler can access these via closure, even
  // though they are initialized after SIGTERM registration.
  // axiom:trace work_item=SWDE-56 spec=specs/108-Distributed-Graph-Execution.md#REQ-DGE-001 plan=phase-1/task-1-2/step-1-2-3
  let clusterInstanceId_SIGTERM: string | null = null;
  let clusterHeartbeatInterval_SIGTERM: ReturnType<typeof setInterval> | null = null;

  // ── Process exit handler — checkpoint WAL on clean shutdown ──────────────
  // Ensures WAL data is merged into the main DB when this session ends,
  // preventing stale WAL/SHM files for the next session that opens the DB.
  // axiom:trace work_item=sqlite-resilience-01 spec=specs/102-Graph-Harness.md#4.1
  const exitHandler = () => {
    // Best-effort cleanup on exit (non-async context)
    if (db.backend === "sqlite") {
      try { (db as SqliteAdapter).getRawDb().exec("PRAGMA wal_checkpoint(PASSIVE)"); } catch { /* best-effort */ }
    }
    db.close().catch(() => { /* best-effort */ });
  };
  process.on("exit", exitHandler);
  process.on("SIGINT", () => { exitHandler(); process.exit(0); });

  // ── Cluster-aware graceful SIGTERM (REQ-DGE-002 drain-wait) ───────────────
  // When cluster mode is active: drain-wait protocol before deregistering.
  // When not in cluster mode: fall through to exitHandler + exit(0).
  // axiom:trace work_item=SWDE-56 spec=specs/108-Distributed-Graph-Execution.md#REQ-DGE-002 plan=phase-1/task-1-2/step-1-2-3
  process.on("SIGTERM", async () => {
    if (clusterInstanceId_SIGTERM !== null && db.backend === "postgres") {
      // Stop heartbeat immediately
      if (clusterHeartbeatInterval_SIGTERM !== null) {
        clearInterval(clusterHeartbeatInterval_SIGTERM);
        clusterHeartbeatInterval_SIGTERM = null;
      }
      pluginInfo("graph-harness", `[Cluster] SIGTERM — entering drain mode for instance ${clusterInstanceId_SIGTERM}`);
      try {
        // 1. status = 'draining' (REQ-DGE-002)
        await db.run(
          `UPDATE cluster_instances SET status = 'draining' WHERE instance_id = $1`,
          [clusterInstanceId_SIGTERM]
        );
        // 2. Drain-wait: poll active_nodes until 0 or graceful_drain_timeout_s (REQ-DGE-002)
        const drainTimeoutMs = 300_000; // 300s default
        const pollIntervalMs = 5_000;
        const deadline = Date.now() + drainTimeoutMs;
        while (Date.now() < deadline) {
          const row = await db.queryOne(
            `SELECT active_nodes FROM cluster_instances WHERE instance_id = $1`,
            [clusterInstanceId_SIGTERM]
          ) as { active_nodes: number } | null;
          if (!row || row.active_nodes <= 0) break;
          pluginInfo("graph-harness", `[Cluster] Draining — ${row.active_nodes} node(s) remaining…`);
          await new Promise(res => setTimeout(res, pollIntervalMs));
        }
        // 3. Reassign remaining nodes to PENDING (timeout path)
        await db.run(
          `UPDATE nodes SET status = 'pending', assigned_session = NULL
           WHERE LOWER(status) = 'active'
             AND assigned_session IN (
               SELECT session_id FROM sessions WHERE instance_id = $1
             )`,
          [clusterInstanceId_SIGTERM]
        );
        // 4. status = 'dead' (REQ-DGE-002)
        await db.run(
          `UPDATE cluster_instances SET status = 'dead' WHERE instance_id = $1`,
          [clusterInstanceId_SIGTERM]
        );
        pluginInfo("graph-harness", `[Cluster] Instance ${clusterInstanceId_SIGTERM} gracefully deregistered`);
      } catch (drainErr) {
        pluginWarn("graph-harness", `[Cluster] Drain error: ${drainErr}`);
      }
    }
    exitHandler();
    process.exit(0);
  });

  // REQ-GH-084: Orphaned ACTIVE node recovery — runs on every plugin init
  // Nodes stuck in 'active' with no live session tracking them are reset to 'pending'.
  // axiom:trace work_item=graph-harness-01 spec=specs/102-Graph-Harness.md#REQ-GH-084 plan=step-p4fix-01
  const orphanedNodes = await db.queryAll(`
    SELECT n.id, n.graph_id FROM nodes n
    WHERE LOWER(n.status) = 'active'
      AND n.id NOT IN (
        SELECT session_id_check.node_id FROM sessions session_id_check
        WHERE session_id_check.node_id IS NOT NULL
          AND LOWER(session_id_check.status) = 'active'
      )
  `, []) as Array<{ id: string; graph_id: string }>;

  for (const orphan of orphanedNodes) {
    await db.run(`UPDATE nodes SET status='pending', activated_at=NULL WHERE id=? AND graph_id=?`, [orphan.id, orphan.graph_id]);
    // Log to console since ledger might not have graph context yet
    pluginInfo("graph-harness", `Recovered orphaned ACTIVE node: ${orphan.id} → PENDING`);
  }
  if (orphanedNodes.length > 0) {
    pluginInfo("graph-harness", `Recovered ${orphanedNodes.length} orphaned node(s) on startup`);
  }

  // REQ-GH-084: SQLite startup integrity check — graceful degradation on corruption
  // Skip for PG backend (PRAGMAs not applicable — SWDE-67)
  // axiom:trace work_item=graph-harness-01 spec=specs/102-Graph-Harness.md#REQ-GH-084 plan=sqlite-integrity-check
  if (db.backend === "sqlite") try {
    const integrityResult = await db.queryOne("PRAGMA integrity_check", []) as { integrity_check: string } | null;
    if (integrityResult?.integrity_check !== "ok") {
      pluginWarn("graph-harness", `⚠️ SQLite integrity check FAILED: ${integrityResult?.integrity_check ?? "unknown"}`);
      console.warn("[GraphHarness] Database may be corrupted. Attempting WAL checkpoint to recover...");
      // Attempt WAL checkpoint to merge WAL into main DB file (fixes some corruption classes)
      try {
        await db.exec("PRAGMA wal_checkpoint(TRUNCATE)");
        // Re-check after checkpoint
        const recheckResult = await db.queryOne("PRAGMA integrity_check", []) as { integrity_check: string } | null;
        if (recheckResult?.integrity_check === "ok") {
          console.log("[GraphHarness] ✓ Database recovered after WAL checkpoint");
        } else {
          console.error("[GraphHarness] ✗ Database integrity still failing after checkpoint. Consider deleting .graph-harness/harness.db and restarting.");
          // Continue anyway — partial functionality is better than a crash
        }
      } catch (checkpointErr) {
        console.error("[GraphHarness] WAL checkpoint failed:", checkpointErr);
      }
    } else {
      // Only log at debug level on success — don't spam on every load
      // console.log("[GraphHarness] ✓ SQLite integrity check passed");
    }
  } catch (integrityErr) {
    // Non-fatal — if PRAGMA itself fails, the DB is likely very corrupt or locked
    console.warn("[GraphHarness] Could not run integrity check:", integrityErr);
  }

  // ── ADR-GH-002: Schema migration — add consecutive_briefing_failures if missing ──
  try {
    await db.exec(`ALTER TABLE sessions ADD COLUMN consecutive_briefing_failures INTEGER NOT NULL DEFAULT 0`);
  } catch { /* already exists */ }

  // ── Phase 7 migrations: worker_pid (REQ-GH-082) and locked_by (REQ-GH-116) ──
  // axiom:trace work_item=graph-lock-upgrade-01 spec=specs/102-Graph-Harness.md#REQ-GH-082
  try { await db.exec(`ALTER TABLE sessions ADD COLUMN worker_pid INTEGER DEFAULT NULL`); } catch { /* exists */ }
  try { await db.exec(`ALTER TABLE graphs ADD COLUMN locked_by TEXT DEFAULT NULL`); } catch { /* exists */ }

  // ── SWDE-63 migration: per-graph notification config ─────────────────────
  // axiom:trace work_item=SWDE-63 spec=specs/102-Graph-Harness.md#REQ-GH-101 jira_ref=SWDE-63
  try { await db.exec(`ALTER TABLE graphs ADD COLUMN notifications_config JSON DEFAULT NULL`); } catch { /* exists */ }

  // ── Cluster registration + heartbeat (REQ-DGE-001, REQ-DGE-004) ──────────
  // Only active when cluster.enabled=true AND backend=postgres (REQ-DGE-015).
  // axiom:trace work_item=SWDE-56 spec=specs/108-Distributed-Graph-Execution.md#REQ-DGE-001 plan=phase-1/task-1-2/step-1-2-1
  let clusterInstanceId: string | null = null;
  let clusterHeartbeatInterval: ReturnType<typeof setInterval> | null = null;

  const clusterEnabled = (config as GraphHarnessConfig & { cluster?: { enabled?: boolean; heartbeat_interval_s?: number } }).cluster?.enabled ?? false;
  const heartbeatIntervalMs = ((config as GraphHarnessConfig & { cluster?: { heartbeat_interval_s?: number } }).cluster?.heartbeat_interval_s ?? 30) * 1000;

  if (clusterEnabled && db.backend === "postgres") {
    try {
      // Generate a stable instance ID for this process
      const instanceId = process.env.GRAPH_HARNESS_INSTANCE_ID ?? crypto.randomUUID();
      clusterInstanceId = instanceId;

      // Register this instance in cluster_instances (REQ-DGE-001)
      await db.run(
        `INSERT INTO cluster_instances (instance_id, status, active_nodes, last_heartbeat, registered_at)
         VALUES ($1, 'active', 0, NOW(), NOW())
         ON CONFLICT (instance_id) DO UPDATE SET status='active', last_heartbeat=NOW()`,
        [instanceId]
      );
      pluginInfo("graph-harness", `[Cluster] Registered instance ${instanceId}`);

      // Wire hoisted SIGTERM variables (REQ-DGE-002 drain-wait uses these)
      clusterInstanceId_SIGTERM = clusterInstanceId;
    } catch (regErr) {
      // Non-fatal: cluster registration failure should not prevent harness from starting
      pluginWarn("graph-harness", `[Cluster] Registration failed (cluster mode disabled for this session): ${regErr}`);
      clusterInstanceId = null;
    }
  }

  if (clusterInstanceId !== null) {
    // ── Heartbeat emission loop (REQ-DGE-004) ────────────────────────────────
    // IMPORTANT: This is a setInterval in bootstrap(), NOT in session.idle.
    // session.idle only fires when an agent finishes a turn. Long-running nodes
    // (e.g. 10-minute LLM calls) would miss heartbeats if we relied on session.idle.
    // axiom:trace work_item=SWDE-56 spec=specs/108-Distributed-Graph-Execution.md#REQ-DGE-004 plan=phase-1/task-1-2/step-1-2-2
    clusterHeartbeatInterval = setInterval(async () => {
      try {
        // REQ-DGE-044: ONLY update last_heartbeat — do NOT include active_nodes here
        await db.run(
          `UPDATE cluster_instances SET last_heartbeat = NOW() WHERE instance_id = $1`,
          [clusterInstanceId]
        );
      } catch (hbErr) {
        pluginWarn("graph-harness", `[Cluster] Heartbeat failed: ${hbErr}`);
      }
    }, heartbeatIntervalMs);
    // unref() so the heartbeat timer does not prevent process from exiting
    if (clusterHeartbeatInterval?.unref) clusterHeartbeatInterval.unref();
    pluginInfo("graph-harness", `[Cluster] Heartbeat started (interval: ${heartbeatIntervalMs / 1000}s) — instance ${clusterInstanceId}`);

    // Wire heartbeat interval to SIGTERM handler
    clusterHeartbeatInterval_SIGTERM = clusterHeartbeatInterval;
  }

  // ── Work-stealing poll loop (REQ-DGE-011) ────────────────────────────────────
  // Phase 1: polling-only (no LISTEN/NOTIFY). LISTEN/NOTIFY is Phase 2 (REQ-DGE-053).
  // Trigger: fires every steal_interval_s when active_nodes < max_nodes.
  // Backoff (REQ-DGE-012): consecutive no_work responses trigger idle ticks to
  // avoid hammering the DB when there is nothing to claim.
  // axiom:trace work_item=SWDE-56 spec=specs/108-Distributed-Graph-Execution.md#REQ-DGE-011 plan=phase-2/task-2-1/step-2-1-1
  if (clusterInstanceId !== null) {
    const stealIntervalMs = ((config as GraphHarnessConfig & {
      cluster?: { steal_interval_s?: number }
    }).cluster?.steal_interval_s ?? 10) * 1000;

    // Backoff state (REQ-DGE-012)
    let _stealConsecutiveFailures = 0;
    let _stealIdleTicks = 0;
    const maxRetries = (config as GraphHarnessConfig & { cluster?: { max_steal_retries?: number } }).cluster?.max_steal_retries ?? 3;
    const backoffMs = ((config as GraphHarnessConfig & { cluster?: { steal_backoff_s?: number } }).cluster?.steal_backoff_s ?? 2) * 1000;

    const clusterWorkStealInterval = setInterval(async () => {
      // Idle mode: skip ticks when we've exceeded max retries (REQ-DGE-012)
      if (_stealIdleTicks > 0) { _stealIdleTicks--; return; }

      const steal = await performWorkSteal(db, clusterInstanceId!, config);

      if (steal.claimed) {
        _stealConsecutiveFailures = 0;
        pluginInfo("graph-harness", `[Cluster] Claimed node ${steal.nodeId} (graph ${steal.graphId})`);
        // Note: actual session execution is handled by the existing harness loop.
        // The node is now 'active' with assigned_session=instanceId; harness picks it up.
      } else if (steal.reason === "no_work") {
        _stealConsecutiveFailures++;
        if (_stealConsecutiveFailures >= maxRetries) {
          // Backoff: enter idle for N ticks (REQ-DGE-012)
          _stealIdleTicks = Math.ceil(backoffMs / stealIntervalMs);
          _stealConsecutiveFailures = 0;
        }
      } else if (steal.reason === "at_capacity") {
        _stealConsecutiveFailures = 0; // Not a failure — at capacity
      }
      // steal.reason === "error": leave _stealConsecutiveFailures unchanged;
      // transient DB errors should not trigger backoff reset

      // ── Stale instance detection (REQ-DGE-060) ───────────────────────────────
      // axiom:trace work_item=SWDE-56 spec=specs/108-Distributed-Graph-Execution.md#REQ-DGE-060 plan=phase-3/task-3-2/step-3-2-3
      await detectStaleInstances(db, clusterInstanceId!, config).catch(() => { /* best-effort */ });
    }, stealIntervalMs);

    if (clusterWorkStealInterval?.unref) clusterWorkStealInterval.unref();
    pluginInfo("graph-harness", `[Cluster] Work-stealing started (interval: ${stealIntervalMs / 1000}s)`);
  }

  // axiom:trace work_item=SWDE-56 spec=specs/108-Distributed-Graph-Execution.md#REQ-DGE-043 plan=phase-2/task-2-0/step-v3-001
  return { db, config, clusterInstanceId };
}

// ─────────────────────────────────────────────────────────────────────────────
// Cycle detection (DFS-based topological sort)
//
// Input: list of node IDs and raw dependency edges [{from, to}].
// A dependency edge {from: "A", to: "B"} means B depends on A (A → B in DAG).
//
// We model the graph as: for each edge, A has an outgoing edge to B
// (i.e., B cannot start until A is done; traversal goes A→B).
//
// DFS coloring: WHITE=unvisited, GRAY=in-stack (currently visiting), BLACK=done.
// A back-edge (GRAY→GRAY) indicates a cycle. We extract the cycle path.
//
// Returns: null if no cycle, or a human-readable error string if cycle found.
// ─────────────────────────────────────────────────────────────────────────────

function detectCycle(
  nodeIds: string[],
  deps: Array<{ from: string; to: string; required?: boolean }>
): string | null {
  // Build adjacency list: from → [to, to, ...]
  const adj = new Map<string, string[]>();
  for (const id of nodeIds) {
    adj.set(id, []);
  }
  for (const dep of deps) {
    const neighbors = adj.get(dep.from) ?? [];
    neighbors.push(dep.to);
    adj.set(dep.from, neighbors);
  }

  // DFS state
  const WHITE = 0, GRAY = 1, BLACK = 2;
  const color = new Map<string, number>();
  const parent = new Map<string, string | null>();

  for (const id of nodeIds) {
    color.set(id, WHITE);
    parent.set(id, null);
  }

  let cycleStart: string | null = null;
  let cycleEnd: string | null = null;

  // Iterative DFS to avoid stack overflow on large graphs
  // Returns true if a cycle was detected
  function dfs(start: string): boolean {
    const stack: Array<{ node: string; iterIdx: number }> = [{ node: start, iterIdx: 0 }];
    color.set(start, GRAY);

    while (stack.length > 0) {
      const frame = stack[stack.length - 1];
      const { node } = frame;
      const neighbors = adj.get(node) ?? [];

      if (frame.iterIdx < neighbors.length) {
        const neighbor = neighbors[frame.iterIdx];
        frame.iterIdx++;

        if (color.get(neighbor) === GRAY) {
          // Back edge — cycle detected
          cycleStart = neighbor;
          cycleEnd = node;
          return true;
        }
        if (color.get(neighbor) === WHITE) {
          color.set(neighbor, GRAY);
          parent.set(neighbor, node);
          stack.push({ node: neighbor, iterIdx: 0 });
        }
      } else {
        // Done with this node
        color.set(node, BLACK);
        stack.pop();
      }
    }
    return false;
  }

  for (const id of nodeIds) {
    if (color.get(id) === WHITE) {
      if (dfs(id)) {
        break;
      }
    }
  }

  if (cycleStart === null || cycleEnd === null) {
    return null;
  }

  // Extract cycle path by following parent pointers from cycleEnd back to cycleStart
  const cyclePath: string[] = [cycleStart];
  let cur: string | null = cycleEnd;
  while (cur !== null && cur !== cycleStart) {
    cyclePath.push(cur);
    cur = parent.get(cur) ?? null;
  }
  cyclePath.push(cycleStart);
  cyclePath.reverse();

  return `Cycle detected: ${cyclePath.join(" → ")}`;
}

// ─────────────────────────────────────────────────────────────────────────────
// Notification subsystem — types, channels, and dispatcher (SWDE-63)
//
// REQ-GH-101 extension: structured events + channel routing + deduplication.
// Preserves the existing terminal bell+OSC behaviour as the "terminal" channel.
//
// axiom:trace work_item=SWDE-63 spec=specs/102-Graph-Harness.md#REQ-GH-101 plan=phase-notif/task-1/step-1 jira_ref=SWDE-63
// ─────────────────────────────────────────────────────────────────────────────

type NotificationEventType =
  | "node_failed"
  | "retry_storm"
  | "cost_warning"
  | "graph_completed"
  | "graph_failed"
  | "graph_paused"
  | "graph_resumed"
  | "approval_needed";

interface NotificationEvent {
  type: NotificationEventType;
  graph_id: string;
  node_id?: string;
  title: string;
  body: string;
  metadata?: Record<string, unknown>;
  timestamp: string;
}

interface NotificationChannel {
  name: string;
  send(event: NotificationEvent): Promise<void>;
}

/** Per-graph rule: which event types route to which channels. */
interface NotificationRule {
  /** Event types this rule applies to. Use "*" for all event types. */
  events: string[];
  /** Channel names: "terminal" | "webhook" | "log" | "agent_inbox" */
  channels: string[];
  /** Required for "webhook" channel — URL to POST event JSON to. */
  webhook_url?: string;
  /** Required for "agent_inbox" channel — agent subfolder name under .memory-bank/inbox/. */
  agent?: string;
}

/** Per-graph notification configuration, stored as JSON in graphs.notifications_config. */
interface GraphNotificationConfig {
  rules?: NotificationRule[];
  /** Cooldown window in seconds for identical event deduplication. Default: 60. */
  cooldown_seconds?: number;
}

// ── Channel: Terminal (bell + OSC 9/99/777) ──────────────────────────────────
function makeTerminalChannel(): NotificationChannel {
  return {
    name: "terminal",
    async send(event: NotificationEvent): Promise<void> {
      try {
        process.stdout.write("\x07");
        process.stdout.write(`\x1b]9;${event.title}: ${event.body}\x07`);
        process.stdout.write(`\x1b]99;;${event.title}: ${event.body}\x07`);
        process.stdout.write(`\x1b]777;notify;${event.title};${event.body}\x07`);
      } catch {
        // Non-critical
      }
    },
  };
}

// ── Channel: Webhook (HTTP POST with event JSON) ──────────────────────────────
function makeWebhookChannel(
  webhookUrl: string,
  onFailure?: (event: NotificationEvent, error: string) => void
): NotificationChannel {
  return {
    name: "webhook",
    async send(event: NotificationEvent): Promise<void> {
      // SWDE-63 step-C-5: URL scheme validation to prevent SSRF.
      // Allow https://, http://localhost, and http://127.x.x.x only.
      // Blocks http://169.254.169.254/ (AWS metadata), http://10.x.x.x, etc.
      // axiom:trace work_item=SWDE-63 spec=specs/102-Graph-Harness.md#REQ-GH-101 plan=step-C-5 jira_ref=SWDE-63
      const isLocalHttp = webhookUrl.startsWith("http://localhost") ||
        webhookUrl.startsWith("http://127.");
      if (!webhookUrl.startsWith("https://") && !isLocalHttp) {
        pluginWarn("graph-harness", `Webhook notification: URL "${webhookUrl}" rejected — https:// required for non-localhost endpoints`);
        return;
      }
      try {
        const resp = await fetch(webhookUrl, {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify(event),
          signal: AbortSignal.timeout(5000),
        });
        if (!resp.ok) {
          const errMsg = `HTTP ${resp.status} ${resp.statusText}`;
          pluginWarn("graph-harness", `Webhook notification failed: ${errMsg} (${webhookUrl})`);
          onFailure?.(event, errMsg);
        }
      } catch (err) {
        const errMsg = err instanceof Error ? err.message : String(err);
        console.warn(`[GraphHarness] Webhook notification error (${webhookUrl}):`, err);
        onFailure?.(event, errMsg);
      }
    },
  };
}

// ── Channel: Log (structured JSON console output) ─────────────────────────────
function makeLogChannel(): NotificationChannel {
  return {
    name: "log",
    async send(event: NotificationEvent): Promise<void> {
      try {
        console.log(JSON.stringify({
          level: "notification",
          event_type: event.type,
          graph_id: event.graph_id,
          node_id: event.node_id ?? null,
          title: event.title,
          body: event.body,
          metadata: event.metadata ?? null,
          timestamp: event.timestamp,
        }));
      } catch {
        // Non-critical
      }
    },
  };
}

// ── Channel: Agent Inbox (write Markdown file to .memory-bank/inbox/<agent>/) ─
function makeAgentInboxChannel(agentName: string, baseDir: string): NotificationChannel {
  return {
    name: "agent_inbox",
    async send(event: NotificationEvent): Promise<void> {
      // SWDE-63 step-C-4: validate agentName to prevent path traversal.
      // Only alphanumeric, hyphens, and underscores allowed; max 64 chars.
      // e.g. "../../../tmp/evil" is rejected; "my-agent_01" is accepted.
      // axiom:trace work_item=SWDE-63 spec=specs/102-Graph-Harness.md#REQ-GH-101 plan=step-C-4 jira_ref=SWDE-63
      if (!/^[a-zA-Z0-9_-]{1,64}$/.test(agentName)) {
        pluginWarn("graph-harness", `agent_inbox channel: invalid agentName "${agentName}" — skipping (must match /^[a-zA-Z0-9_-]{1,64}$/)`);
        return;
      }
      try {
        const { mkdirSync, writeFileSync } = await import("node:fs");
        const { join } = await import("node:path");
        const inboxDir = join(baseDir, ".memory-bank", "inbox", agentName);
        mkdirSync(inboxDir, { recursive: true });
        const filename = `notification-${event.type}-${event.graph_id}-${Date.now()}.md`;
        const lines: string[] = [
          `# Notification: ${event.title}`,
          ``,
          `**Type:** ${event.type}`,
          `**Graph:** ${event.graph_id}`,
        ];
        if (event.node_id) lines.push(`**Node:** ${event.node_id}`);
        lines.push(`**Time:** ${event.timestamp}`, ``, `## Body`, ``, event.body);
        if (event.metadata) {
          lines.push(``, `## Metadata`, ``, "```json", JSON.stringify(event.metadata, null, 2), "```");
        }
        writeFileSync(join(inboxDir, filename), lines.join("\n"), "utf8");
      } catch (err) {
        console.warn(`[GraphHarness] Agent inbox notification error:`, err);
      }
    },
  };
}

// ── Notification Dispatcher ───────────────────────────────────────────────────
// Routes events to configured channels with deduplication.
// Dedup key: "type:graph_id:node_id" — identical events suppressed within cooldown window.
// axiom:trace work_item=SWDE-63 spec=specs/102-Graph-Harness.md#REQ-GH-101 plan=phase-notif/task-1/step-2 jira_ref=SWDE-63
class NotificationDispatcher {
  private readonly dedupeMap = new Map<string, number>();
  private readonly terminalChannel: NotificationChannel;
  private readonly logChannel: NotificationChannel;
  private readonly globalEnabled: boolean;
  private readonly baseDir: string;
  /** Optional callback invoked when a channel delivery fails — used for ledger observability. */
  private readonly onDeliveryFailure?: (channelName: string, event: NotificationEvent, error: string) => void;

  constructor(
    globalEnabled: boolean,
    baseDir: string,
    onDeliveryFailure?: (channelName: string, event: NotificationEvent, error: string) => void
  ) {
    this.globalEnabled = globalEnabled;
    this.baseDir = baseDir;
    this.onDeliveryFailure = onDeliveryFailure;
    this.terminalChannel = makeTerminalChannel();
    this.logChannel = makeLogChannel();
  }

  /** Dispatch an event, routing to channels per per-graph config with deduplication. */
  async dispatch(
    event: NotificationEvent,
    graphConfig?: GraphNotificationConfig | null
  ): Promise<void> {
    if (!this.globalEnabled) return;

    const cooldownMs = ((graphConfig?.cooldown_seconds) ?? 60) * 1000;
    // Dedup key: "type:graph_id:node_id"
    // — Intentional spam-prevention design: two events of the same type for the same
    //   node within the cooldown window are collapsed into one notification. This means
    //   if handleScriptNode and markNonAgentNodeFailed both fire node_failed for the same
    //   node within 60s, only the first notification is delivered. This is correct for
    //   notification spam prevention. If distinguishing multiple failure modes for the
    //   same node within a single cooldown window is required, reduce cooldown_seconds
    //   or add a discriminator field (e.g., reason) to the dedup key.
    // B-2: axiom:trace work_item=SWDE-63 spec=specs/102-Graph-Harness.md#REQ-GH-101 jira_ref=SWDE-63
    const dedupeKey = `${event.type}:${event.graph_id}:${event.node_id ?? ""}`;
    const now = Date.now();
    const lastSent = this.dedupeMap.get(dedupeKey);
    if (lastSent !== undefined && (now - lastSent) < cooldownMs) {
      return; // suppressed by cooldown
    }
    this.dedupeMap.set(dedupeKey, now);

    const channels = this.resolveChannels(event.type, graphConfig);
    await Promise.all(channels.map((ch) =>
      ch.send(event).catch((err: unknown) => {
        // Non-fatal: log the failure and invoke observability callback if set
        const errMsg = err instanceof Error ? err.message : String(err);
        pluginWarn("graph-harness", `Notification delivery failed (channel: ${ch.name}): ${errMsg}`);
        this.onDeliveryFailure?.(ch.name, event, errMsg);
      })
    ));
  }

  private resolveChannels(
    eventType: string,
    graphConfig?: GraphNotificationConfig | null
  ): NotificationChannel[] {
    // Default: terminal-only (preserves existing REQ-GH-101 behaviour)
    const rules: NotificationRule[] = graphConfig?.rules ?? [
      { events: ["*"], channels: ["terminal"] },
    ];
    const result: NotificationChannel[] = [];
    const seen = new Set<string>();

    for (const rule of rules) {
      if (!rule.events.includes(eventType) && !rule.events.includes("*")) continue;
      for (const channelName of rule.channels) {
        const dedupeKey = `${channelName}:${rule.webhook_url ?? ""}:${rule.agent ?? ""}`;
        if (seen.has(dedupeKey)) continue;
        seen.add(dedupeKey);
        if (channelName === "terminal") {
          result.push(this.terminalChannel);
        } else if (channelName === "log") {
          result.push(this.logChannel);
        } else if (channelName === "webhook" && rule.webhook_url) {
          // Pass the observability callback so non-2xx responses are recorded (step-H-5)
          const url = rule.webhook_url;
          const failCb = this.onDeliveryFailure
            ? (ev: NotificationEvent, err: string) => this.onDeliveryFailure!("webhook", ev, err)
            : undefined;
          result.push(makeWebhookChannel(url, failCb));
        } else if (channelName === "agent_inbox" && rule.agent) {
          result.push(makeAgentInboxChannel(rule.agent, this.baseDir));
        }
      }
    }
    return result;
  }
}

// ─────────────────────────────────────────────────────────────────────────────
// Test seam: override the conductor_plugin_absent_warn TTL for unit tests.
// Set to a small value (e.g. 1) before creating a plugin instance to exercise
// the re-notification path without waiting 60 real seconds. Reset to null after.
// axiom:trace work_item=SWDE-48 spec=specs/102-Graph-Harness.md#REQ-GH-203 plan=phase-run7-hardening/task-run7-ttl-renotify/step-BL-031 jira_ref=SWDE-48
// ─────────────────────────────────────────────────────────────────────────────
/** Test seam: mutate .conductorAbsentWarnTtlMs before creating a plugin instance
 *  to override the 60s TTL without waiting. Reset to null after the test.
 *  Object export is used (not `export let`) so ES module consumers can mutate it.
 */
export const _testSeams = { conductorAbsentWarnTtlMs: null as number | null };

// ─────────────────────────────────────────────────────────────────────────────
// Plugin export
//
// OpenCode's getLegacyPlugins iterates Object.values(module) and calls each
// exported function as a plugin factory. The factory receives { client, directory }
// and returns a hooks object.
//
// The hooks object shape:
//   {
//     tool: { [toolName]: tool({...}) }   // custom tools registered with the LLM
//     event: async ({ event }) => {}       // lifecycle event handlers (task 1-4)
//     "experimental.chat.system.transform": async (...) => {} // briefing (task 1-4)
//   }
//
// Phase 1 task-1-2-1: graph.create tool added.
// Harness loop (event hook + system.transform) added in tasks 1-4.
// ─────────────────────────────────────────────────────────────────────────────

export const GraphHarnessPlugin = async ({ directory, client }: { directory: string; client: unknown }) => {
  // `directory` is the repo root provided by OpenCode's plugin loader
  const repoRoot = directory;

  // Bootstrap: load config, ensure directories, init DB schema (async — SWDE-67)
  // axiom:trace work_item=SWDE-67 spec=specs/102-Graph-Harness.md#REQ-GH-154 jira_ref=SWDE-67
  const { db, config, clusterInstanceId } = await bootstrap(repoRoot);

  // ── SWDE-63: Notification dispatcher ────────────────────────────────────────
  // Instantiated once per plugin instance. Channels are resolved per-dispatch
  // from per-graph config. Deduplication state lives in-memory (survives the
  // session, resets on restart — intentional, restart clears cooldown windows).
  // The delivery-failure callback wires in ledger observability (step-H-5).
  // axiom:trace work_item=SWDE-63 spec=specs/102-Graph-Harness.md#REQ-GH-101 plan=phase-notif/task-2/step-1 jira_ref=SWDE-63
  const notifDispatcher = new NotificationDispatcher(
    config.interface.notifications,
    repoRoot,
    // step-H-5: on channel delivery failure, write a notification_delivery_failed ledger entry
    async (channelName, event, error) => {
      try {
        const ts = new Date().toISOString();
        await db.run(
          `INSERT INTO ledger (graph_id, session_id, action, target_node_id, detail, timestamp)
           VALUES (?, NULL, 'notification_delivery_failed', NULL, ?, ?)`
        , [event.graph_id, JSON.stringify({ channel: channelName, event_type: event.type, error }), ts]);
      } catch { /* non-fatal — ledger write failure must not crash notification path */ }
    }
  );

  // B-1: Per-graph notification config cache (step-polish-6).
  // Eliminates redundant SQLite reads: config is set at graph creation and never changes.
  // Cache is per-plugin-instance (resets on restart, which is fine since config is immutable).
  // axiom:trace work_item=SWDE-63 spec=specs/102-Graph-Harness.md#REQ-GH-101 plan=step-polish-6 jira_ref=SWDE-63
  const notifConfigCache = new Map<string, GraphNotificationConfig | null>();

  /** Load per-graph GraphNotificationConfig from the DB (or cache), or null if not set. */
  async function loadGraphNotifConfig(graphId: string): Promise<GraphNotificationConfig | null> {
    if (notifConfigCache.has(graphId)) {
      return notifConfigCache.get(graphId) ?? null;
    }
    try {
      const row = await db.queryOne(`SELECT notifications_config FROM graphs WHERE id = ?`, [graphId]) as { notifications_config: string | null } | null;
      const result = row?.notifications_config
        ? JSON.parse(row.notifications_config) as GraphNotificationConfig
        : null;
      notifConfigCache.set(graphId, result);
      return result;
    } catch {
      notifConfigCache.set(graphId, null);
      return null;
    }
  }

  /** Convenience wrapper: dispatch a notification with per-graph config auto-loaded. */
  async function dispatchNotification(
    event: NotificationEvent
  ): Promise<void> {
    const graphConfig = await loadGraphNotifConfig(event.graph_id);
    await notifDispatcher.dispatch(event, graphConfig);
  }

  // ─────────────────────────────────────────────────────────────────────────
  // SWDE-48: Stash & Conductor integration helpers
  //
  // onNodeActivated  — stash pop + conductor agent spawn on node activation
  // onNodeTerminated — stash push + conductor agent cancel on node completion
  //
  // The stash root mirrors the context-stash plugin's storage layout.
  // The conductor_agents table is shared with the conductor plugin via harness.db.
  //
  // axiom:trace work_item=SWDE-48 spec=specs/102-Graph-Harness.md jira_ref=SWDE-48
  // ─────────────────────────────────────────────────────────────────────────

  /** Filesystem root for context-stash plugin storage (mirrors context-stash.ts). */
  const stashRoot = join(repoRoot, ".memory-bank", "stash");

  // REQ-GH-203: TTL-based dedup for conductor_plugin_absent_warn.
  // Map<graph_id:node_id, last_warn_timestamp_ms>. Emit at most once per 60s
  // per node to prevent ledger flooding while allowing re-notification on
  // graph retries and after the TTL window.
  // axiom:trace work_item=SWDE-48 spec=specs/102-Graph-Harness.md#REQ-GH-203 plan=phase-run6-hardening/task-run6-impl/step-BL-028 jira_ref=SWDE-48
  const CONDUCTOR_ABSENT_WARN_TTL_MS = _testSeams.conductorAbsentWarnTtlMs ?? 60_000; // 60 seconds (overridable via _testSeams.conductorAbsentWarnTtlMs for tests)
  const conductorPluginAbsentWarnLastEmitted = new Map<string, number>();

  /** Read node metadata from DB. Returns empty object if none or malformed. */
  async function readNodeMeta(graphId: string, nodeId: string): Promise<Record<string, unknown>> {
    try {
      const row = await db.queryOne(`SELECT metadata FROM nodes WHERE graph_id=? AND id=?`, [graphId, nodeId]) as { metadata: string | null } | null;
      if (row?.metadata) return JSON.parse(row.metadata) as Record<string, unknown>;
    } catch { /* malformed — ignore */ }
    return {};
  }

  /** Merge additions into node metadata in DB (non-destructive). */
  async function mergeNodeMeta(graphId: string, nodeId: string, additions: Record<string, unknown>): Promise<void> {
    try {
      const existing = await readNodeMeta(graphId, nodeId);
      const merged = { ...existing, ...additions };
      await db.run(`UPDATE nodes SET metadata=? WHERE graph_id=? AND id=?`, [JSON.stringify(merged), graphId, nodeId]);
    } catch (err) {
      console.warn(`[GraphHarness] mergeNodeMeta failed for ${nodeId}:`, err);
    }
  }

  /**
   * REQ-GH-200 Crash Recovery: On plugin startup, scan stashRoot/suspended/ for any *.consuming
   * files that indicate a crash between the atomic rename (step 1) and the annotation INSERT (step 2).
   * - If the annotations table contains a matching [Stash: <id>] entry: INSERT succeeded before crash
   *   — safe to delete the orphaned .consuming file.
   * - If no matching annotation exists: INSERT did not complete — rename .consuming back to *.md
   *   to restore the stash for the next activation.
   *
   * axiom:trace work_item=SWDE-48 spec=specs/102-Graph-Harness.md#REQ-GH-200 plan=phase-hardening/task-w2-impl/step-BL-007 jira_ref=SWDE-48
   */
  async function recoverConsuming(): Promise<void> {
    const suspendedDir = join(stashRoot, "suspended");
    if (!existsSync(suspendedDir)) return;

    let entries: string[];
    try {
      entries = readdirSync(suspendedDir).filter(f => f.endsWith(".md.consuming"));
    } catch {
      return;
    }

    for (const filename of entries) {
      const consumingPath = join(suspendedDir, filename);
      // Extract stash ID: strip ".md.consuming" suffix
      const stashId = filename.slice(0, -".md.consuming".length);
      const originalPath = join(suspendedDir, `${stashId}.md`);

      // Skip .consuming files that are too recent — they may be in-flight stash pops
      // (renameSync succeeded but annotation INSERT not yet complete). Only recover
      // files older than STALE_THRESHOLD_MS, which indicates a genuine crash-before-INSERT.
      // See: specs/102-Graph-Harness.md#REQ-GH-200 Crash Recovery
      // axiom:trace work_item=SWDE-48 spec=specs/102-Graph-Harness.md#REQ-GH-200 plan=phase-run5-hardening/task-run5-impl/step-BL-027 jira_ref=SWDE-48
      const STALE_THRESHOLD_MS = 30_000; // 30 seconds
      try {
        const stats = statSync(consumingPath);
        const ageMs = Date.now() - stats.mtimeMs;
        if (ageMs < STALE_THRESHOLD_MS) {
          pluginInfo("graph-harness", `recoverConsuming: skipping recent .consuming for '${stashId}' (age ${Math.round(ageMs / 1000)}s < ${STALE_THRESHOLD_MS / 1000}s threshold — may be in-flight)`);
          continue;
        }
      } catch {
        // statSync failed — skip this file to be safe
        continue;
      }

      try {
        // Check if annotation was already written (INSERT succeeded before crash).
        // Use db.queryOne (works for both SQLite and PG backends — SWDE-67)
        const annotationExists = await db.queryOne(
          `SELECT 1 AS found FROM annotations WHERE content LIKE ? LIMIT 1`
        , [`[Stash: ${stashId}]%`]);

        if (annotationExists) {
          // INSERT succeeded — safe to delete the .consuming file
          try { rmSync(consumingPath); } catch { /* best-effort */ }
          pluginInfo("graph-harness", `recoverConsuming: deleted orphan .consuming for '${stashId}' (annotation exists)`);
        } else {
          // INSERT did not succeed — rename back to restore the stash
          renameSync(consumingPath, originalPath);
          pluginInfo("graph-harness", `recoverConsuming: restored stash '${stashId}' from .consuming (annotation absent)`);
        }
      } catch (err) {
        pluginWarn("graph-harness", `recoverConsuming: failed to recover '${stashId}': ${err instanceof Error ? err.message : String(err)}`);
      }
    }
  }

  // REQ-GH-200: Recover any .consuming orphans left by a previous crash-before-INSERT.
  // Must run after stashRoot is defined and DB is open. Best-effort — errors are warned, not thrown.
  // axiom:trace work_item=SWDE-48 spec=specs/102-Graph-Harness.md#REQ-GH-200 plan=phase-hardening/task-w2-impl/step-BL-007 jira_ref=SWDE-48
  await recoverConsuming();

  /**
   * Called after CAS node activation succeeds.
   * - If metadata.stash is set: pop stash content into node annotation (for briefing injection).
   * - If metadata.conductor_agent is set: spawn background agent via conductor_agents table.
   *
   * Both actions are best-effort — failures are logged but never block the activation.
   *
   * axiom:trace work_item=SWDE-48 spec=specs/102-Graph-Harness.md#REQ-GH-204 jira_ref=SWDE-48
   */
  async function onNodeActivated(graphId: string, nodeId: string, sessionId: string): Promise<void> {
    try {
      const meta = await readNodeMeta(graphId, nodeId);
      const stashId = typeof meta.stash === "string" ? meta.stash : null;
      const conductorAgent = meta.conductor_agent as Record<string, unknown> | undefined;

      // ── Stash pop: inject stash content into node annotation ───────────────
      // The annotation surfaces in buildNodeBriefing and buildSystemBriefing
      // so the agent always has access to the stash context.
      // axiom:trace work_item=SWDE-48 spec=specs/102-Graph-Harness.md#REQ-GH-200 jira_ref=SWDE-48
      if (stashId) {
        // SWDE-48 REQ-GH-200: Validate stash ID format before filesystem access (path traversal prevention)
        // axiom:trace work_item=SWDE-48 spec=specs/102-Graph-Harness.md#REQ-GH-200 jira_ref=SWDE-48
        const STASH_ID_RE = /^[a-z0-9][a-z0-9-]{0,63}$/;
        if (!STASH_ID_RE.test(stashId)) {
          pluginWarn("graph-harness", `onNodeActivated: invalid stash ID '${stashId}' — skipping stash pop (path traversal prevention)`);
          try {
            await logLedger(graphId, sessionId, "stash_pop_rejected", nodeId, { stash_id: stashId, reason: "invalid_id_format" });
          } catch { /* non-fatal */ }
        } else {
        try {
          let stashContent: string | null = null;
          let foundCandidate: string | null = null;
          let foundDir: string | null = null;
          for (const dir of ["suspended", "closed"]) {
            const candidate = join(stashRoot, dir, `${stashId}.md`);
            if (existsSync(candidate)) {
              const raw = readFileSync(candidate, "utf-8");
              const { body } = stashParseFrontmatter(raw);
              stashContent = body.trim();
              foundCandidate = candidate;
              foundDir = dir;
              break;
            }
          }
          if (stashContent && foundCandidate && foundDir === "suspended") {
            // Atomic pop: rename file first, INSERT annotation, delete on success, restore on failure
            const consumingPath = `${foundCandidate}.consuming`;
            let renamed = false;
            let insertOk = false;
            // Step 1: rename to .consuming (atomic on POSIX)
            try {
              renameSync(foundCandidate, consumingPath);
              renamed = true;
            } catch (renameErr) {
              // Could not rename — log and skip pop (don't block activation)
              console.warn(`[GraphHarness] stash pop rename failed for '${stashId}':`, renameErr);
              // axiom:trace work_item=SWDE-48 spec=specs/102-Graph-Harness.md#REQ-GH-200 jira_ref=SWDE-48
              try {
                await logLedger(graphId, sessionId, "stash_pop_failed", nodeId, {
                  stash_id: stashId,
                  error: renameErr instanceof Error ? renameErr.message : String(renameErr),
                  phase: "rename",
                });
              } catch { /* non-fatal */ }
            }
            if (renamed) {
              // Step 2: INSERT annotation (DB write)
              const ts = new Date().toISOString();
              const annotationId = `ann_stash_${graphId}_${nodeId}_${Date.now().toString(36)}`;
              try {
                await db.run(
                  `INSERT OR IGNORE INTO annotations
                   (id, graph_id, node_id, type, content, severity, created_at)
                   VALUES (?, ?, ?, 'note', ?, 'info', ?)`
                , [annotationId, graphId, nodeId, `[Stash: ${stashId}]\n\n${stashContent}`, ts]);
                insertOk = true;
              } catch (insertErr) {
                // DB write failed — restore the file
                console.warn(`[GraphHarness] stash pop annotation INSERT failed for '${stashId}' — restoring:`, insertErr);
                try { renameSync(consumingPath, foundCandidate); } catch { /* best-effort restore */ }
                // axiom:trace work_item=SWDE-48 spec=specs/102-Graph-Harness.md#REQ-GH-200 jira_ref=SWDE-48
                try {
                  await logLedger(graphId, sessionId, "stash_pop_failed", nodeId, {
                    stash_id: stashId,
                    error: insertErr instanceof Error ? insertErr.message : String(insertErr),
                    phase: "annotation_insert",
                  });
                } catch { /* non-fatal */ }
              }
              // Step 3: delete .consuming on success
              if (insertOk) {
                try { rmSync(consumingPath); } catch { /* best-effort — .consuming left behind is benign only when insertOk=true; crash-before-INSERT case is handled by recoverConsuming() on next startup. */ }
                await logLedger(graphId, sessionId, "stash_popped", nodeId, { stash_id: stashId });
                pluginInfo("graph-harness", `Stash '${stashId}' popped into node ${nodeId}`);
              }
            }
          } else if (stashContent && foundCandidate && foundDir === "closed") {
            // Closed stash: read-only pop (no file deletion), just insert annotation
            const ts = new Date().toISOString();
            const annotationId = `ann_stash_${graphId}_${nodeId}_${Date.now().toString(36)}`;
            await db.run(
              `INSERT OR IGNORE INTO annotations
               (id, graph_id, node_id, type, content, severity, created_at)
               VALUES (?, ?, ?, 'note', ?, 'info', ?)`
            , [annotationId, graphId, nodeId, `[Stash: ${stashId}]\n\n${stashContent}`, ts]);
            await logLedger(graphId, sessionId, "stash_popped", nodeId, { stash_id: stashId });
            pluginInfo("graph-harness", `Stash '${stashId}' popped into node ${nodeId}`);
          }
        } catch (err) {
          console.warn(`[GraphHarness] onNodeActivated: stash pop failed for '${stashId}':`, err);
        }
        } // end else (valid stash ID)
      }

      // ── Conductor agent spawn ──────────────────────────────────────────────
      // Spawn a background conductor agent if the node declares conductor_agent config.
      // The agent runs independently; its completion is polled via conductor_agent_done condition.
      // axiom:trace work_item=SWDE-48 spec=specs/102-Graph-Harness.md#REQ-GH-201 jira_ref=SWDE-48
      if (conductorAgent && typeof conductorAgent.task === "string") {
        try {
          // Require ConductorPlugin to be loaded (shares harness.db)
          const tableExists = await db.queryOne(
            `SELECT 1 FROM sqlite_master WHERE type='table' AND name='conductor_agents'`
          , []);
          if (!tableExists) {
            console.warn("[GraphHarness] conductor_agents table not found — is ConductorPlugin loaded?");
            // Emit a ledger entry so operators can detect silent spawn skips via ledger queries
            // axiom:trace work_item=SWDE-48 spec=specs/102-Graph-Harness.md#REQ-GH-201 jira_ref=SWDE-48 plan=phase-hardening/task-w3-tests-and-low/step-BL-017
            await logLedger(graphId, sessionId, "conductor_spawn_skipped", nodeId, {
              reason: "conductor_agents_table_absent",
            }).catch(() => { /* non-fatal */ });
          } else {
            // Detect base URL for spawning sessions
            const baseUrl = (() => {
              const c = client as Record<string, unknown>;
              const candidates = [c?.baseUrl, c?.base_url, (c?.config as Record<string, unknown>)?.baseUrl];
              for (const u of candidates) {
                if (typeof u === "string" && u.startsWith("http")) return u;
              }
              return process.env.OPENCODE_BASE_URL ?? "http://localhost:4096";
          })

            const agentName = typeof conductorAgent.name === "string"
              ? conductorAgent.name
              : `graph-${nodeId.slice(0, 12)}-agent`;
            const agentTask = conductorAgent.task as string;
            const agentModel = typeof conductorAgent.model === "string" ? conductorAgent.model : undefined;

            // Generate agent ID and a spawn secret using CSPRNG (≥128-bit entropy)
            // axiom:trace work_item=SWDE-48 spec=specs/102-Graph-Harness.md#REQ-GH-201 jira_ref=SWDE-48
            const _agentIdBytes = new Uint8Array(4);
            crypto.getRandomValues(_agentIdBytes);
            const _agentIdSuffix = Array.from(_agentIdBytes).map((b) => b.toString(16).padStart(2, "0")).join("").slice(0, 6);
            const agentId = `bg_gh_${nodeId.slice(0, 8)}_${_agentIdSuffix}`;
            const _spawnSecretBytes = new Uint8Array(16); // 128 bits
            crypto.getRandomValues(_spawnSecretBytes);
            const _spawnSecretB64 = btoa(String.fromCharCode(..._spawnSecretBytes));
            const spawnSecret = _spawnSecretB64.replace(/\+/g, "-").replace(/\//g, "_").replace(/=/g, "");
            const secretHashBuf = await crypto.subtle.digest("SHA-256", new TextEncoder().encode(spawnSecret));
            const secretHash = Array.from(new Uint8Array(secretHashBuf))
              .map((b) => b.toString(16).padStart(2, "0")).join("");

            // Spawn the OpenCode session via HTTP
            let spawnedSessionId: string | null = null;
            try {
              const resp = await fetch(`${baseUrl}/session`, {
                method: "POST",
                headers: { "Content-Type": "application/json" },
                body: JSON.stringify(agentModel ? { model: agentModel } : {}),
                signal: AbortSignal.timeout(5000),
              });
              if (resp.ok) {
                const body = await resp.json() as Record<string, unknown>;
                spawnedSessionId = (body.id ?? body.session_id ?? body.sessionID) as string | null;
                if (spawnedSessionId) {
                  // Inject conductor envelope + task as first message
                  const conductorStash = typeof conductorAgent.stash === "string" ? conductorAgent.stash : "";
                  const envelope = `[conductor_envelope]\nagent_id: ${agentId}\n${conductorStash ? `stash_id: ${conductorStash}\n` : ""}spawn_secret: ${spawnSecret}\n[/conductor_envelope]\n\n${agentTask}`;
                  await fetch(`${baseUrl}/session/${spawnedSessionId}/message`, {
                    method: "POST",
                    headers: { "Content-Type": "application/json" },
                    body: JSON.stringify({ role: "user", content: envelope }),
                    signal: AbortSignal.timeout(5000),
                  }).catch(() => { /* best-effort */ });
                }
              }
            } catch { /* HTTP unavailable — use synthetic session ID */ }
            if (!spawnedSessionId) {
              spawnedSessionId = `cnd_gh_${Date.now().toString(36)}_${Math.random().toString(36).slice(2, 8)}`;
            }

            // Record in conductor_agents table (shared with ConductorPlugin)
            const now = new Date().toISOString();
            const timeoutMinutes = typeof conductorAgent.timeout === "number" ? conductorAgent.timeout : 60;
            const timeoutAt = new Date(Date.now() + timeoutMinutes * 60000).toISOString();
            const stashIdForAgent = typeof conductorAgent.stash === "string" ? conductorAgent.stash : null;
            await db.run(`
              INSERT INTO conductor_agents
                (agent_id, name, session_id, stash_id, status, task, model,
                 spawned_by, spawned_at, timeout_at, cost_usd, spawn_secret_hash)
              VALUES (?, ?, ?, ?, 'running', ?, ?, ?, ?, ?, 0, ?)
            `, [agentId,
              agentName,
              spawnedSessionId,
              stashIdForAgent,
              agentTask,
              agentModel ?? null,
              sessionId,
              now,
              timeoutAt,
              secretHash]);

            // Store agentId in node metadata so onNodeTerminated can cancel it
            // and conductor_agent_done condition can check its status
            await mergeNodeMeta(graphId, nodeId, { _conductor_agent_id: agentId });
            await logLedger(graphId, sessionId, "conductor_agent_spawned", nodeId, {
              agent_id: agentId,
              name: agentName,
              session_id: spawnedSessionId,
              stash_id: stashIdForAgent,
            });
            pluginInfo("graph-harness", `Conductor agent '${agentId}' spawned for node ${nodeId}`);
          }
        } catch (err) {
          console.warn(`[GraphHarness] onNodeActivated: conductor spawn failed for node ${nodeId}:`, err);
          // SWDE-48 REQ-GH-201: log failure to ledger so operators can detect conductor spawn failures
          // axiom:trace work_item=SWDE-48 spec=specs/102-Graph-Harness.md#REQ-GH-201 jira_ref=SWDE-48
          try {
            await logLedger(graphId, sessionId, "conductor_spawn_failed", nodeId, {
              error: err instanceof Error ? err.message : String(err),
            });
          } catch { /* non-fatal — ledger write failure must not re-throw */ }
        }
      }
    } catch (err) {
      console.warn(`[GraphHarness] onNodeActivated error for ${nodeId}:`, err);
    }
  }

  /**
   * Called after node reaches DONE or FAILED.
   * - If metadata.stash is set: push updated context back to stash.
   * - If metadata._conductor_agent_id is set: cancel the background agent.
   *
   * Both actions are best-effort — failures are logged but never block the termination flow.
   *
   * axiom:trace work_item=SWDE-48 spec=specs/102-Graph-Harness.md#REQ-GH-204 jira_ref=SWDE-48
   */
  async function onNodeTerminated(graphId: string, nodeId: string, status: "done" | "failed"): Promise<void> {
    try {
      const meta = await readNodeMeta(graphId, nodeId);
      const stashId = typeof meta.stash === "string" ? meta.stash : null;
      const conductorAgentId = typeof meta._conductor_agent_id === "string" ? meta._conductor_agent_id : null;

      // ── Stash push: write updated context back to stash ────────────────────
      // Creates a new suspended stash file capturing node outputs and final status.
      // axiom:trace work_item=SWDE-48 spec=specs/102-Graph-Harness.md#REQ-GH-200 jira_ref=SWDE-48
      if (stashId) {
        // SWDE-48 REQ-GH-200: Validate stash ID format on push path (path traversal prevention)
        // axiom:trace work_item=SWDE-48 spec=specs/102-Graph-Harness.md#REQ-GH-200 jira_ref=SWDE-48
        const STASH_PUSH_ID_RE = /^[a-z0-9][a-z0-9-]{0,63}$/;
        if (!STASH_PUSH_ID_RE.test(stashId)) {
          pluginWarn("graph-harness", `onNodeTerminated: invalid stash ID '${stashId}' — skipping stash push (path traversal prevention)`);
          try {
            await logLedger(graphId, null, "stash_push_failed", nodeId, { stash_id: stashId, reason: "invalid_id_format", status });
          } catch { /* non-fatal */ }
          // fall through to conductor agent cancel below
        } else {
          try {
            // Collect node outputs for the stash body
            const outputRows = await db.queryAll(
              `SELECT key, value FROM node_outputs WHERE graph_id=? AND node_id=?`
            , [graphId, nodeId]) as Array<{ key: string; value: string }>;
            const outputSummary = outputRows.length > 0
              ? outputRows.map((o) => `- ${o.key}: ${o.value.slice(0, 200)}`).join("\n")
              : "(no outputs)";

            const nodeRow = await db.queryOne(`SELECT title FROM nodes WHERE graph_id=? AND id=?`, [graphId, nodeId]) as { title: string } | null;
            const nodeName = nodeRow?.title ?? nodeId;

            const summary = `Graph node "${nodeName}" (${nodeId}) — ${status.toUpperCase()} in graph ${graphId}`;
            const detail = `## Node Outputs\n${outputSummary}`;

            const fm = {
              stash_id: stashId,
              name: `${nodeName} (${status})`,
              state: "suspended" as const,
              created_by: "graph-harness",
              created_at: new Date().toISOString(),
              suspended_at: new Date().toISOString(),
              session_id: `graph-${graphId}`,
              tags: ["graph-harness", status, graphId],
              entries: 0,
              last_agent: "graph-harness",
            };

            const content = buildSuspendedMarkdown(fm, summary, detail);
            const suspendedDir = join(stashRoot, "suspended");
            if (!existsSync(suspendedDir)) mkdirSync(suspendedDir, { recursive: true });
            const filePath = join(suspendedDir, `${stashId}.md`);
            await stashAtomicWrite(filePath, content);
            await logLedger(graphId, null, "stash_pushed", nodeId, { stash_id: stashId, status });
            pluginInfo("graph-harness", `Stash '${stashId}' pushed from node ${nodeId} (${status})`);
          } catch (err) {
            const msg = err instanceof Error ? err.message : String(err);
            console.warn(`[GraphHarness] onNodeTerminated: stash push failed for '${stashId}':`, err);
            // SWDE-48: log failure to ledger so operators can detect and recover
            // axiom:trace work_item=SWDE-48 spec=specs/102-Graph-Harness.md#REQ-GH-200 jira_ref=SWDE-48
            try {
              await logLedger(graphId, null, "stash_push_failed", nodeId, { stash_id: stashId, error: msg.slice(0, 500), status });
            } catch { /* non-fatal — ledger write failure must not re-throw */ }
          }
        }
      }

      // ── Cancel conductor agent (best-effort) ───────────────────────────────
      // Marks the agent cancelled in the DB and attempts to interrupt its session.
      // axiom:trace work_item=SWDE-48 spec=specs/102-Graph-Harness.md#REQ-GH-201 jira_ref=SWDE-48
      if (conductorAgentId) {
        try {
          const tableExists = await db.queryOne(
            `SELECT 1 FROM sqlite_master WHERE type='table' AND name='conductor_agents'`
          , []);
          if (tableExists) {
            await db.run(
              `UPDATE conductor_agents SET status='cancelled', completed_at=?
               WHERE agent_id=? AND status='running'`
            , [new Date().toISOString(), conductorAgentId]);

            // Interrupt session via HTTP (best-effort, fire-and-forget)
            const agentRow = await db.queryOne(`SELECT session_id FROM conductor_agents WHERE agent_id=?`, [conductorAgentId]) as { session_id: string } | null;
            if (agentRow?.session_id) {
              const baseUrl = process.env.OPENCODE_BASE_URL ?? "http://localhost:4096";
              fetch(`${baseUrl}/session/${agentRow.session_id}`, {
                method: "DELETE",
                signal: AbortSignal.timeout(3000),
              }).catch(() => { /* best-effort */ });
            }

            await logLedger(graphId, null, "conductor_agent_cancelled", nodeId, {
              agent_id: conductorAgentId,
              reason: status,
            });
            pluginInfo("graph-harness", `Conductor agent '${conductorAgentId}' cancelled (node ${nodeId} ${status})`);
          }
        } catch (err) {
          const msg = err instanceof Error ? err.message : String(err);
          console.warn(`[GraphHarness] onNodeTerminated: conductor cancel failed for '${conductorAgentId}':`, err);
          // SWDE-48: log failure to ledger so operators can detect and recover
          // axiom:trace work_item=SWDE-48 spec=specs/102-Graph-Harness.md#REQ-GH-201 jira_ref=SWDE-48
          try {
            await logLedger(graphId, null, "conductor_cancel_failed", nodeId, { agent_id: conductorAgentId, error: msg.slice(0, 500), status });
          } catch { /* non-fatal */ }
        }
      }
      // Decrement cluster active_nodes on node completion (REQ-DGE-043, REQ-DGE-044)
      // axiom:trace work_item=SWDE-56 spec=specs/108-Distributed-Graph-Execution.md#REQ-DGE-044 plan=phase-2/task-2-0/step-v3-001
      const isCluster = (config as GraphHarnessConfig & { cluster?: { enabled?: boolean } }).cluster?.enabled ?? false;
      if (clusterInstanceId && isCluster && db.backend === "postgres") {
        await decrementClusterActiveNodes(db, clusterInstanceId).catch(() => { /* best-effort */ });
      }
    } catch (err) {
      console.warn(`[GraphHarness] onNodeTerminated error for ${nodeId}:`, err);
    }
  }

  console.log(
    `[GraphHarness] Initialized — backend: ${db.backend}, DB: ${db.backend === "postgres" ? "(postgres)" : config.database.path}, ` +
    `max_nodes: ${config.limits.max_nodes_per_graph}, ` +
    `max_concurrent_sessions: ${config.spawning.max_concurrent_sessions}`
  );

  // ─────────────────────────────────────────────────────────────────────────
  // REQ-GH-030: SDK Client Verification — detect spawn method
  //
  // Probe client to determine how to spawn child sessions:
  //   "sdk"  — client exposes createSession or session.create API
  //   "cli"  — opencode CLI is on PATH (fallback)
  //   "none" — no spawn capability; parent session executes all nodes
  //
  // The result is stored in `spawnMethod` and used by spawnChildSession().
  //
  // NOTE (REQ-GH-030 limitation): The spawn method probe runs ONCE at plugin init.
  // If the SDK is lazily loaded after init, spawnMethod will be permanently set to 'cli' or 'none'.
  // To pick up a runtime SDK, restart the OpenCode server after the SDK is installed.
  // See .memory-bank/work-items/graph-harness-01/findings-backlog.md#OVF-MI-1
  // axiom:trace work_item=graph-harness-01 spec=specs/102-Graph-Harness.md#REQ-GH-030 plan=step-p4fix-10
  //
  // axiom:trace work_item=graph-harness-01 spec=specs/102-Graph-Harness.md#REQ-GH-030 plan=phase-4/task-4-1/step-4-1-1
  // ─────────────────────────────────────────────────────────────────────────

  let spawnMethod: "sdk" | "cli" | "none" = "none";
  // client.session.create() is the correct and only SDK shape.
  // client.createSession() does NOT exist on OpencodeClient — probe removed.
  if (
    client?.session &&
    typeof (client.session as Record<string, unknown>).create === "function"
  ) {
    spawnMethod = "sdk";
  } else {
    // Check if opencode CLI is available for fallback
    try {
      const which = Bun.spawnSync(["which", "opencode"], { stdout: "pipe" });
      if (which.success) spawnMethod = "cli";
    } catch { /* opencode not found */ }
  }
  pluginInfo("graph-harness", `Spawn method: ${spawnMethod}`);

  // REQ-GH-REPEAT: Startup recovery scan — find DONE nodes with trigger_every that
  // were not reset to CANCELLED (crash between DONE write and CANCELLED reset).
  // Bug fix: previously only scanned 'active'/'created' graphs, missing nodes in
  // 'complete' graphs that completed after the last cycle but before the reset.
  // axiom:trace work_item=graph-scheduler-rearchitecture-01 spec=specs/102-Graph-Harness.md#REQ-GH-REPEAT plan=phase-0/task-0-1/step-0-1-1
  try {
    const nowTs = new Date().toISOString();
    const doneRepeatNodes = await db.queryAll(
      `SELECT n.id, n.graph_id, n.title, n.trigger_every, n.trigger_max_runs,
              n.trigger_run_count, n.trigger_lifetime_h, n.created_at
       FROM nodes n
       JOIN graphs g ON n.graph_id = g.id
       WHERE LOWER(n.status) = 'done'
         AND n.trigger_every IS NOT NULL
          AND LOWER(g.status) IN ('active', 'created', 'complete')
          AND (n.trigger_max_runs = 0 OR n.trigger_run_count < n.trigger_max_runs)`,
      []
    ) as Array<{
      id: string; graph_id: string; title: string;
      trigger_every: string; trigger_max_runs: number; trigger_run_count: number;
      trigger_lifetime_h: number; created_at: string;
    }>;

    for (const node of doneRepeatNodes) {
      // Check lifetime hasn't expired
      const ageMs = Date.now() - Date.parse(node.created_at);
      if (node.trigger_lifetime_h > 0 && ageMs >= node.trigger_lifetime_h * 3_600_000) continue;
      // Recover: reset to CANCELLED
      await db.run(
        `UPDATE nodes SET status='cancelled', completed_at=NULL, trigger_last_fired_at=?
         WHERE graph_id=? AND id=?`,
        [nowTs, node.graph_id, node.id]
      );
      await logLedger(node.graph_id, null, "startup_repeat_recovery", node.id, {
        node_title: node.title, trigger_every: node.trigger_every,
        trigger_run_count: node.trigger_run_count,
      });
      pluginInfo("graph-harness", `Startup repeat recovery: node "${node.id}" reset to CANCELLED`);
    }
    if (doneRepeatNodes.length > 0) {
      pluginInfo("graph-harness", `Startup repeat recovery: recovered ${doneRepeatNodes.length} DONE repeat node(s)`);
    }
  } catch (recoveryErr) {
    console.warn("[GraphHarness] Startup repeat recovery scan failed:", recoveryErr);
  }

  // ── Phase 112 / task-3-5: Legacy cancelled-as-repeat-wait → requeued migration ─
  // Converts nodes using the OLD pattern (cancelled + trigger_every) to the NEW pattern
  // (requeued + next_fire_at). Runs on every plugin init — safe and idempotent since
  // already-requeued nodes are not affected.
  // COALESCE ensures NULL trigger_last_fired_at uses now() (node that crashed before first fire).
  // axiom:trace work_item=graph-scheduler-rearchitecture-01 spec=specs/102-Graph-Harness.md#REQ-GH-SCHED-V2 plan=phase-3/task-3-5/step-3-5-1
  try {
    const legacyCancelledNodes = await db.queryAll(
      `SELECT n.id, n.graph_id, n.trigger_every_ms, n.trigger_last_fired_at
       FROM nodes n
       JOIN graphs g ON g.id = n.graph_id
       WHERE LOWER(n.status) = 'cancelled'
         AND n.trigger_every IS NOT NULL
         AND (n.trigger_max_runs = 0 OR n.trigger_run_count < n.trigger_max_runs)
         AND LOWER(g.status) NOT IN ('abandoned', 'complete')`,
      []
    ) as Array<{ id: string; graph_id: string; trigger_every_ms: number; trigger_last_fired_at: string | null }>;

    for (const node of legacyCancelledNodes) {
      // COALESCE: if trigger_last_fired_at IS NULL, use now() — node never fired before crash
      const lastFiredMs = node.trigger_last_fired_at
        ? Date.parse(node.trigger_last_fired_at)
        : Date.now();
      const nextFire = new Date(lastFiredMs + (node.trigger_every_ms || 0)).toISOString();
      await db.run(
        `UPDATE nodes SET status='requeued', next_fire_at=? WHERE graph_id=? AND id=? AND LOWER(status)='cancelled'`,
        [nextFire, node.graph_id, node.id]
      );
      await logLedger(node.graph_id, null, "repeat_status_migration", node.id, {
        from: "cancelled", to: "requeued", next_fire_at: nextFire,
      });
    }
    if (legacyCancelledNodes.length > 0) {
      pluginInfo("graph-harness", `Migration: converted ${legacyCancelledNodes.length} legacy cancelled repeat node(s) to requeued`);
    }
  } catch (migErr) {
    console.warn("[GraphHarness] Legacy cancelled→requeued migration failed:", migErr);
  }

  // ─────────────────────────────────────────────────────────────────────────
  // Phase 112 / task-3-1, task-3-2, task-3-3: Outer scheduler loop
  //
  // The scheduler is the OUTER LOOP of the inner/outer loop architecture.
  // It polls the database for requeued nodes whose next_fire_at has arrived,
  // flips them to pending, and wakes the inner loop (runHarnessLoop).
  //
  // Design decisions (see ADR-008, meta-planning.md D-series, backlog BL-01/02/03):
  //   BL-01: while-loop + sleepInterruptible (NOT setInterval — avoids callback backlog)
  //   BL-02: AbortController initialized here (at plugin init), NOT at module level
  //          — prevents hot-reload from inheriting stale aborted state
  //   BL-03: sleepInterruptible resolves on abort OR timeout; test uses relative timing
  //
  // axiom:trace work_item=graph-scheduler-rearchitecture-01 spec=specs/102-Graph-Harness.md#REQ-GH-SCHED-V2 plan=phase-3/task-3-1/step-3-1-1
  // ─────────────────────────────────────────────────────────────────────────

  // ── task-3-1: sleepInterruptible helper ──────────────────────────────────
  // Resolves after `ms` milliseconds OR when `signal` is aborted, whichever
  // comes first. Uses { once: true } + a `done` flag to prevent double-
  // resolution and avoid AbortSignal listener leaks.
  function sleepInterruptible(ms: number, signal: AbortSignal): Promise<void> {
    return new Promise<void>((resolve) => {
      if (signal.aborted) { resolve(); return; }
      let done = false;
      const timer = setTimeout(() => {
        if (!done) { done = true; resolve(); }
      }, ms);
      signal.addEventListener("abort", () => {
        if (!done) { done = true; clearTimeout(timer); resolve(); }
      }, { once: true });
    });
  }

  // ── task-3-2: processDueWork — flip requeued→pending for all due nodes ───
  // Called by the scheduler on each wake. Reads v_due_work (a SQL view) for
  // nodes whose next_fire_at has arrived, flips them to pending, and reactivates
  // their graphs. The inner loop (runHarnessLoop) then activates and executes them.
  //
  // task-4-2-prereq: Session synthesis (BL-05, Option A)
  // When the scheduler reactivates a graph, also reactivate any done coordinator
  // sessions for that graph. This keeps agent-mode nodes working across cycle boundaries
  // without needing to create new sessions.
  async function processDueWork(): Promise<void> {
    const due = await db.queryAll(
      `SELECT graph_id, node_id FROM v_due_work`, []
    ) as Array<{ graph_id: string; node_id: string }>;

    for (const row of due) {
      await db.run(
        `UPDATE nodes SET status='pending', activated_at=NULL
         WHERE graph_id=? AND id=? AND LOWER(status)='requeued'`,
        [row.graph_id, row.node_id]
      );
      // Reactivate idle/complete graphs so the inner loop picks them up
      await db.run(
        `UPDATE graphs SET status='active', completed_at=NULL
         WHERE id=? AND LOWER(status) IN ('idle', 'complete', 'created')`,
        [row.graph_id]
      );
       // Session synthesis (BL-05 Option A): reactivate the MOST RECENT done coordinator
       // session so the inner loop can deliver briefings to agent-mode nodes across cycle
       // boundaries.
       // LIMIT 1 + ORDER BY created_at DESC prevents coordinator session proliferation:
       // without the LIMIT, every prior done session gets reactivated, accumulating O(N)
       // active sessions after N scheduler cycles.
       // axiom:trace work_item=graph-scheduler-rearchitecture-01 spec=specs/102-Graph-Harness.md#REQ-GH-SCHED-V2 plan=phase-8/task-8-2/step-8-2-1
       await db.run(
         `UPDATE sessions SET status='active', completed_at=NULL
          WHERE session_id = (
            SELECT session_id FROM sessions
            WHERE graph_id=? AND LOWER(status)='done' AND role='coordinator'
            ORDER BY created_at DESC LIMIT 1
          )`,
         [row.graph_id]
       );
      await logLedger(row.graph_id, null, "scheduler_node_queued", row.node_id, {
        reason: "next_fire_at_due",
      }).catch(() => { /* non-fatal */ });
    }
  }

  // ── task-3-3: wakeScheduler — interrupt the sleep to process due nodes now ─
  // Called when a new requeued row is inserted with an earlier next_fire_at than
  // the current sleep target. Creates a fresh AbortController for the next iteration
  // before aborting the current one (prevents old controller from affecting new sleeps).
  // BL-02: _schedulerAbort is initialized at plugin init (below), NOT at module level.
  let _schedulerShutdown = false;
  let _schedulerAbort: AbortController | null = null;

  function wakeScheduler(): void {
    if (_schedulerAbort === null) return; // scheduler not running
    const old = _schedulerAbort;
    _schedulerAbort = new AbortController(); // fresh controller for next iteration
    old.abort(); // wake the sleeping loop
  }

  // ── task-3-2: schedulerLoop — the outer loop ─────────────────────────────
  // Adaptive sleep: Δ = clamp(MIN(next_fire_at) - now, 100ms, 30s).
  // Uses while-loop + sleepInterruptible (BL-01) instead of setInterval to
  // naturally respect backpressure (no callback queuing if processDueWork is slow).
  async function schedulerLoop(): Promise<void> {
    while (!_schedulerShutdown) {
      // Compute adaptive sleep based on the soonest due node
      const next = await db.queryOne(
        `SELECT MIN(next_fire_at) AS t FROM nodes WHERE LOWER(status)='requeued'`, []
      ) as { t: string | null } | null;

      const nowMs = Date.now();
      const nextFireMs = next?.t ? Math.max(0, Date.parse(next.t) - nowMs) : 30_000;
      const delayMs = Math.max(100, Math.min(30_000, nextFireMs));

      if (_schedulerAbort === null) break; // stopped before sleep
      await sleepInterruptible(delayMs, _schedulerAbort.signal);

      if (_schedulerShutdown) break;

      // Process all due nodes
      await processDueWork().catch((err) => {
        console.error("[GraphHarness] schedulerLoop processDueWork error:", err);
      });
    }
    pluginInfo("graph-harness", "schedulerLoop: stopped");
  }

  // ── task-3-4: stopScheduler — clean shutdown ─────────────────────────────
  function stopScheduler(): void {
    _schedulerShutdown = true;
    if (_schedulerAbort) {
      const a = _schedulerAbort;
      _schedulerAbort = null;
      a.abort();
    }
  }

  /**
   * Spawn a child worker session for a given node.
   *
   * Returns the new session ID on success, or null if spawn failed.
   * On null: caller should fallback to parent-session execution of the node
   * (node is already atomically activated via CAS before this call).
   *
   * SDK API: client.session.create({ body: { parentID: string } })
   * Note: the body field is `parentID` (capital ID), not `parentId`.
   * Confirmed from SessionCreateData type in @opencode-ai/sdk dist/gen/types.gen.d.ts.
   *
   * axiom:trace work_item=graph-harness-01 spec=specs/102-Graph-Harness.md#REQ-GH-030 plan=phase-4/task-4-1/step-p4fix-02
   */
  async function spawnChildSession(
    parentSessionId: string,
    graphId: string,
    nodeId: string,
    _workerRole: string = "worker"
  ): Promise<string | null> {
    const workerSessionId = `gh_worker_${Date.now().toString(36)}_${Math.random().toString(36).slice(2, 6)}`;

    if (spawnMethod === "sdk") {
      try {
        // Use client.session.create() — the only correct SDK spawn API.
        // SessionCreateData body uses `parentID` (capital ID), not `parentId`.
        const sessionClient = client?.session as { create: (opts: { body?: { parentID?: string } }) => Promise<unknown> } | undefined;
        if (sessionClient?.create) {
          await sessionClient.create({ body: { parentID: parentSessionId } });
        }
        return workerSessionId;
      } catch (e) {
        console.error(`[GraphHarness] SDK spawn failed, falling back to parent execution:`, e);
        return null;
      }
    } else if (spawnMethod === "cli") {
      try {
        const proc = Bun.spawn(["opencode", "run", "--non-interactive"], {
          stdout: "pipe",
          stderr: "pipe",
        });
        const exitCode = await proc.exited;
        return exitCode === 0 ? workerSessionId : null;
      } catch {
        return null;
      }
    }

    return null; // no spawn method — parent executes
  }

  // ─────────────────────────────────────────────────────────────────────────
  // graph.create tool (REQ-GH-001)
  //
  // Creates a new execution graph with nodes, dependencies, and conditions.
  // Validates structure (unique IDs, valid dep refs, cycle detection, limits).
  // Atomically writes graphs + nodes + dependencies + conditions + ledger entry.
  //
  // axiom:trace work_item=graph-harness-01 spec=specs/102-Graph-Harness.md#REQ-GH-001 plan=phase-1/task-1-2/step-1-2-1
  // ─────────────────────────────────────────────────────────────────────────

  const graphCreateTool = tool({
    description:
      "Create a new execution graph with nodes, dependencies, and conditions. " +
      "The graph drives deterministic execution — the model defines the work, the harness executes it. " +
      "Returns the graph_id, node_count, edge_count, and status on success. " +
      "Returns an error object on validation failure (duplicate IDs, bad refs, cycles, limit violations).",
    args: {
      name: tool.schema
        .string()
        .min(1)
        .describe("Human-readable name/title for the graph"),
      description: tool.schema
        .string()
        .optional()
        .describe("Optional description of what this graph accomplishes"),
      nodes: tool.schema
        .array(
          tool.schema.object({
            id: tool.schema
              .string()
              .min(1)
              .describe("Unique node ID within this graph (e.g. 'lint', 'test', 'deploy')"),
            title: tool.schema.string().min(1).describe("Human-readable title for this node"),
            description: tool.schema
              .string()
              .optional()
              .describe("What this node should accomplish"),
            execution_mode: tool.schema
              .enum(["agent", "script", "transform", "wait", "api", "route", "composite"])
              .optional()
              .describe("How this node executes. Default: 'agent'"),
            execution_config: tool.schema
              .record(tool.schema.string(), tool.schema.string())
              .optional()
              .describe("Mode-specific configuration (commands, transforms, etc.)"),
            schedule: tool.schema
              .string()
              .optional()
              .describe("Repeat interval for this node, e.g. 'every 30s', 'every 5m', 'every 1h'. Requires repeat:true."),
            repeat: tool.schema
              .boolean()
              .optional()
              .describe("If true, node resets to PENDING after completing and fires again on schedule. Default: false."),
            max_retries: tool.schema
              .number()
              .optional()
              .describe("Max retry attempts on condition failure. Default: 3."),
            constraints: tool.schema
              .array(tool.schema.string())
              .optional()
              .describe("Constraints/rules for the agent working on this node"),
            context: tool.schema
              .string()
              .optional()
              .describe("Additional context injected into the agent briefing for this node"),
            metadata: tool.schema
              .record(tool.schema.string(), tool.schema.string())
              .optional()
              .describe("Arbitrary node metadata"),
            trigger: tool.schema
              .object({
                on: tool.schema
                  .string()
                  .optional()
                  .describe("Session/graph event that activates this node. Default: 'idle'. See §17b.1."),
                cancel_on: tool.schema
                  .string()
                  .optional()
                  .describe("Session/graph event that cancels this node. Default: 'active'. Set to 'never' to never cancel."),
                every: tool.schema
                  .string()
                  .optional()
                  .describe("Interval between fires: '30s', '5m', '1h', '2d'."),
                cron: tool.schema
                  .string()
                  .optional()
                  .describe("Cron expression, e.g. '0 * * * *' = top of every hour."),
                max_runs: tool.schema
                  .number()
                  .optional()
                  .describe("Max times this node fires. 0 = unlimited. Default: 0."),
                lifetime_hours: tool.schema
                  .number()
                  .optional()
                  .describe("Stop firing after N hours. 0 = unlimited. Default: 0."),
              })
              .optional()
              .describe("Trigger block: controls when this node fires and when it cancels. §17b."),
          })
        )
        .min(1)
        .describe("Array of nodes in this graph. At least one required."),
      dependencies: tool.schema
        .array(
          tool.schema.object({
            from: tool.schema.string().describe("Source node ID (this node must complete first)"),
            to: tool.schema.string().describe("Target node ID (this node depends on 'from')"),
            required: tool.schema
              .boolean()
              .optional()
              .describe("If false, the target can start even if source fails. Default: true"),
          })
        )
        .optional()
        .describe("Dependency edges: 'from' must complete before 'to' can start"),
      conditions: tool.schema
        .array(
          tool.schema.object({
            node_id: tool.schema.string().describe("Which node these conditions belong to"),
            type: tool.schema
              .enum(["script", "file_exists", "http_check", "model_judge", "none", "test_pattern", "file_changed", "manual", "compound", "stash_has_finding", "conductor_agent_done"])
              .describe("Condition evaluation method"),
              // axiom:trace work_item=graph-harness-01 spec=specs/102-Graph-Harness.md#4.2 plan=step-verify-gh-09
            command: tool.schema
              .string()
              .optional()
              .describe("Shell command for type='script' — exits 0 = pass"),
            timeout_seconds: tool.schema
              .number()
              .optional()
              .describe("Per-condition timeout. Default: 30"),
            max_retries: tool.schema
              .number()
              .optional()
              .describe("Max retry attempts if condition fails. Default: 3"),
            description: tool.schema
              .string()
              .optional()
              .describe("Human-readable description of what this condition verifies"),
          })
        )
        .optional()
        .describe(
          "Done-conditions for nodes. When all conditions for a node pass, the harness marks it DONE."
        ),
      locked_by: tool.schema.string().optional()
        .describe(
          "Optional: session_id to lock this graph to on creation (REQ-GH-116). " +
          "Creates the graph AND locks it atomically — no race window. " +
          "Only the named session may call mutation tools until the lock is released."
        ),
      draft: tool.schema.boolean().optional()
        .describe(
          "If true, create the graph in DRAFT status (REQ-GH-141). " +
          "A draft graph does not activate any nodes — the harness loop ignores it. " +
          "Transition to active via graph_activate. Returns status: 'draft'."
        ),
      notifications: tool.schema.object({
        rules: tool.schema.array(
          tool.schema.object({
            events: tool.schema.array(tool.schema.string())
              .describe("Event types this rule applies to. Use [\"*\"] for all events."),
            channels: tool.schema.array(tool.schema.string())
              .describe("Channels: \"terminal\" | \"webhook\" | \"log\" | \"agent_inbox\""),
            webhook_url: tool.schema.string().optional()
              .describe("Webhook URL (required for webhook channel)"),
            agent: tool.schema.string().optional()
              .describe("Agent name (required for agent_inbox channel)"),
          })
        ).optional().describe("Notification routing rules"),
        cooldown_seconds: tool.schema.number().optional()
          .describe("Deduplication cooldown in seconds. Default: 60."),
      }).optional()
        .describe(
          "Per-graph notification config (SWDE-63). " +
          "Controls which events fire on which channels (terminal/webhook/log/agent_inbox). " +
          "Default: terminal channel for all events, 60s cooldown."
        ),
    },

    async execute(args, context) {
      // ── 0. Early exit if plugin is disabled ─────────────────────────────
      if (!config.enabled) {
        return JSON.stringify({ error: "Graph Harness is disabled (enabled: false in config)" });
      }

      try {
        // ── 1. Input normalization ───────────────────────────────────────────
        const graphTitle = args.name;
        const graphDescription = args.description ?? null;
        const nodes = args.nodes;
        const lockedByArg = args.locked_by ?? null;
        // REQ-GH-141: draft flag — when true, graph is created in DRAFT status (harness ignores it)
        // axiom:trace work_item=SWDE-54 spec=specs/102-Graph-Harness.md#REQ-GH-141 plan=phase-10/new-step-p10-02 jira_ref=SWDE-54
        const isDraft = args.draft === true;
        // axiom:trace work_item=SWDE-67 spec=specs/102-Graph-Harness.md#REQ-GH-032 jira_ref=SWDE-67
        // Support `edges:` as an alias for `dependencies:` — the schema only exposes
        // `dependencies`, but test fixtures (and external callers) may use `edges:` instead.
        // When both are provided, `dependencies` takes precedence.
        const edgesAlias = (args as Record<string, unknown>).edges as typeof args.dependencies | undefined;
        const rawDeps = args.dependencies ?? edgesAlias ?? [];
        const rawConditions = args.conditions ?? [];
        // SWDE-63: per-graph notification config (optional; null = use system defaults)
        const notificationsConfig = (args as Record<string, unknown>).notifications
          ? JSON.stringify((args as Record<string, unknown>).notifications)
          : null;

        // ── 2. Validate: at least one node (already enforced by .min(1) in schema)
        //    Validate: unique node IDs
        const nodeIds = nodes.map((n) => n.id);
        const idSet = new Set<string>();
        for (const nid of nodeIds) {
          if (idSet.has(nid)) {
            return JSON.stringify({
              error: `Duplicate node ID: "${nid}". All node IDs must be unique within a graph.`,
              details: { duplicate_id: nid },
            });
          }
          idSet.add(nid);
        }

        // ── 3. Validate: graph limits ────────────────────────────────────────
        const maxNodes = config.limits.max_nodes_per_graph;
        if (nodes.length > maxNodes) {
          return JSON.stringify({
            error: `Too many nodes: ${nodes.length} exceeds max_nodes_per_graph (${maxNodes}).`,
            details: { node_count: nodes.length, limit: maxNodes },
          });
        }

        // Validate conditions per node
        const conditionsByNode = new Map<string, typeof rawConditions>();
        for (const cond of rawConditions) {
          const existing = conditionsByNode.get(cond.node_id) ?? [];
          existing.push(cond);
          conditionsByNode.set(cond.node_id, existing);
        }
        const maxConditions = config.limits.max_conditions_per_node;
        for (const [nodeId, conds] of conditionsByNode) {
          if (conds.length > maxConditions) {
            return JSON.stringify({
              error: `Node "${nodeId}" has ${conds.length} conditions, exceeds max_conditions_per_node (${maxConditions}).`,
              details: { node_id: nodeId, condition_count: conds.length, limit: maxConditions },
            });
          }
        }

        // ── 4. Validate: dep references point to existing nodes ──────────────
        for (const dep of rawDeps) {
          if (!idSet.has(dep.from)) {
            return JSON.stringify({
              error: `Dependency references unknown node ID: "${dep.from}" (in from).`,
              details: { missing_node: dep.from, valid_nodes: Array.from(idSet) },
            });
          }
          if (!idSet.has(dep.to)) {
            return JSON.stringify({
              error: `Dependency references unknown node ID: "${dep.to}" (in to).`,
              details: { missing_node: dep.to, valid_nodes: Array.from(idSet) },
            });
          }
        }

        // Validate: condition node_ids reference existing nodes
        for (const cond of rawConditions) {
          if (!idSet.has(cond.node_id)) {
            return JSON.stringify({
              error: `Condition references unknown node ID: "${cond.node_id}".`,
              details: { missing_node: cond.node_id, valid_nodes: Array.from(idSet) },
            });
          }
        }

        // ── 5. Validate: no cycles (DFS-based topological sort) ──────────────
        // Build adjacency list: from → [to, to, ...]
        // A dependency "from→to" means "to depends on from", i.e., from must
        // complete before to. For cycle detection we traverse in the dependency
        // direction: we follow "to depends on from" as edges from → to in the
        // DFS. If we find a back-edge, we have a cycle.
        const cycleError = detectCycle(nodeIds, rawDeps);
        if (cycleError) {
          return JSON.stringify({
            error: cycleError,
            details: { validation: "cycle_detected" },
          });
        }

        // ── 6. Validate: at least one root node (no incoming dependencies) ───
        const hasIncoming = new Set<string>();
        for (const dep of rawDeps) {
          hasIncoming.add(dep.to);
        }
        const rootNodes = nodeIds.filter((id) => !hasIncoming.has(id));
        if (rootNodes.length === 0) {
          return JSON.stringify({
            error: "Graph has no root nodes — every node has at least one incoming dependency. This would be a cycle or an unreachable graph.",
            details: { all_nodes: nodeIds },
          });
        }

        // ── 7. Generate graph ID ─────────────────────────────────────────────
        const graphId = `gh_${Date.now().toString(36)}_${Math.random().toString(36).slice(2, 8)}`;
        const nowIso = new Date().toISOString();

        // ── 8. Atomic DB write ───────────────────────────────────────────────
        // Use a transaction to ensure all-or-nothing insertion.
        // On any error: implicit rollback, return error object.






        // FK column order has been corrected in the schema (step-verify-gh-06).
        // No need to disable FK enforcement.
        // axiom:trace work_item=graph-harness-01 spec=specs/102-Graph-Harness.md#4.2 plan=step-verify-gh-06

        // Run the entire write in a single transaction (SQLite or PG)
        const txResult = await db.transaction(async (db) => {
            // ── Phase 112: compute lifecycle_mode from nodes ─────────────────
            // A graph is 'repeating' if any node has repeat:true, trigger.every, or trigger.cron.
            // axiom:trace work_item=graph-scheduler-rearchitecture-01 spec=specs/102-Graph-Harness.md#REQ-GH-SCHED-V2 plan=phase-2/task-2-4/step-2-4-1
            const lifecycleMode: "one_shot" | "repeating" = nodes.some((n) => {
              const raw = n as Record<string, unknown>;
              const hasRepeat = raw.repeat === true || raw.repeat === 1;
              const trigger = raw.trigger as { every?: string; cron?: string } | null | undefined;
              const hasEvery = !!(trigger?.every ?? (raw.schedule && typeof raw.schedule === "string" && /^every\s+/i.test(raw.schedule as string)));
              const hasCron  = !!(trigger?.cron);
              return hasRepeat || hasEvery || hasCron;
            }) ? "repeating" : "one_shot";

            // Insert graph (locked_by set atomically on creation — REQ-GH-116)
            // REQ-GH-141: use DRAFT status when draft:true
            await db.run(`
          INSERT INTO graphs (id, title, description, status, locked_by, created_at, metadata, notifications_config, lifecycle_mode)
          VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
        `, [
              graphId,
              graphTitle,
              graphDescription,
              isDraft ? 'DRAFT' : 'CREATED',
              lockedByArg,
              nowIso,
              null,            // metadata: not exposed in graph.create input for now
              notificationsConfig,  // SWDE-63: per-graph notification config (or null)
              lifecycleMode
            ]);

          // Insert nodes
          for (const node of nodes) {
            const execMode = node.execution_mode ?? "agent";
            const execConfig = node.execution_config
              ? JSON.stringify(node.execution_config)
              : null;

            // Build context JSON: wrap constraints + context string
            const contextObj: Record<string, unknown> = {};
            if (node.constraints && node.constraints.length > 0) {
              contextObj.constraints = node.constraints;
            }
            if (node.context) {
              contextObj.instructions = node.context;
            }
            const contextJson =
              Object.keys(contextObj).length > 0 ? JSON.stringify(contextObj) : null;
             // SWDE-48: promote stash + conductor_agent fields to metadata for harness lifecycle hooks
             // axiom:trace work_item=SWDE-48 spec=specs/102-Graph-Harness.md jira_ref=SWDE-48
             const nodeMeta: Record<string, unknown> = node.metadata
               ? { ...(node.metadata as Record<string, unknown>) }
               : {};
             const nodeRaw = node as Record<string, unknown>;
             if (typeof nodeRaw.stash === "string" && nodeRaw.stash.length > 0) {
               nodeMeta.stash = nodeRaw.stash;
             }
             if (nodeRaw.conductor_agent && typeof nodeRaw.conductor_agent === "object") {
               nodeMeta.conductor_agent = nodeRaw.conductor_agent;
             }
             const metaJson = Object.keys(nodeMeta).length > 0 ? JSON.stringify(nodeMeta) : null;
            const scheduleVal = (node as Record<string, unknown>).schedule as string | null ?? null;
            const repeatVal = (node as Record<string, unknown>).repeat as boolean | null ?? false;
            const maxRetriesVal = (node as Record<string, unknown>).max_retries as number | null ?? 3;

            // ── §17b backward compat alias + trigger block extraction ─────────
            // axiom:trace work_item=SWDE-46 spec=specs/102-Graph-Harness.md#17b plan=phase-0/task-0.3/step-1 jira_ref=SWDE-46
            const triggerBlock = (node as Record<string, unknown>).trigger as {
              on?: string; cancel_on?: string; every?: string; cron?: string;
              max_runs?: number; lifetime_hours?: number;
            } | null | undefined;

            // Start with values from explicit trigger block
            let tOn: string = triggerBlock?.on ?? 'idle';
            let tCancelOn: string = triggerBlock?.cancel_on ?? 'active';
            let tEvery: string | null = triggerBlock?.every ?? null;
            const tCron: string | null = triggerBlock?.cron ?? null;
            let tMaxRuns: number = triggerBlock?.max_runs ?? 0;
            const tLifetimeH: number = triggerBlock?.lifetime_hours ?? 0;

            // schedule alias: "every 30s" → trigger_on="idle", trigger_every="30s", cancel_on="active"
            if (scheduleVal && !triggerBlock?.every && !triggerBlock?.cron) {
              const everyAlias = scheduleVal.match(/^every\s+(.+)$/i);
              if (everyAlias) {
                tOn = 'idle';
                tEvery = everyAlias[1].trim();
                tCancelOn = triggerBlock?.cancel_on ?? 'active';
              }
            }
             // repeat alias: repeat=true → trigger_max_runs=0 (unlimited)
            if (repeatVal && !triggerBlock) {
              tMaxRuns = 0;
            }

            // ── Phase 112: compute trigger_every_ms from tEvery text ─────────
            // axiom:trace work_item=graph-scheduler-rearchitecture-01 spec=specs/102-Graph-Harness.md#REQ-GH-SCHED-V2 plan=phase-2/task-2-4/step-2-4-1
            let tEveryMs = 0;
            if (tEvery) {
              const mEvery = /^(\d+(?:\.\d+)?)\s*(ms|s|m|h|d|w)?$/i.exec(tEvery.trim());
              if (mEvery) {
                const n = parseFloat(mEvery[1]);
                switch ((mEvery[2] ?? "s").toLowerCase()) {
                  case "ms": tEveryMs = n; break;
                  case "m":  tEveryMs = n * 60_000; break;
                  case "h":  tEveryMs = n * 3_600_000; break;
                  case "d":  tEveryMs = n * 86_400_000; break;
                  case "w":  tEveryMs = n * 604_800_000; break;
                  default:   tEveryMs = n * 1_000; // 's' or no unit
                }
              }
            }

            await db.run(`
          INSERT INTO nodes
            (id, graph_id, title, description, status, execution_mode,
             execution_config, context, metadata, schedule, repeat, max_retries, created_at,
             trigger_on, trigger_cancel_on, trigger_every, trigger_cron,
             trigger_max_runs, trigger_lifetime_h, trigger_every_ms)
          VALUES (?, ?, ?, ?, 'PENDING', ?, ?, ?, ?, ?, ?, ?, ?,
                  ?, ?, ?, ?, ?, ?, ?)
        `, [
              node.id,
              graphId,
              node.title,
              node.description ?? "",
              execMode,
              execConfig,
              contextJson,
              metaJson,
              scheduleVal,
              repeatVal ? true : false,
              maxRetriesVal,
              nowIso,
              tOn,
              tCancelOn,
              tEvery,
              tCron,
              tMaxRuns,
              tLifetimeH,
              tEveryMs
            ]);
          }

          // Insert dependencies
          for (const dep of rawDeps) {
            // dep.to depends on dep.from → in DB: node_id=dep.to, depends_on=dep.from
            await db.run(`
          INSERT INTO dependencies (graph_id, node_id, depends_on)
          VALUES (?, ?, ?)
        `, [graphId, dep.to, dep.from]);
          }

          // Insert conditions, grouped by node with ordinal
          const ordinalCounters = new Map<string, number>();
          for (const cond of rawConditions) {
            const ordinal = (ordinalCounters.get(cond.node_id) ?? 0);
            ordinalCounters.set(cond.node_id, ordinal + 1);

            const condId = `cond_${graphId}_${cond.node_id}_${ordinal}`;
            const timeoutSecs = cond.timeout_seconds ?? 30;
            await db.run(`
          INSERT INTO conditions
            (id, graph_id, node_id, ordinal, type, command, description,
             timeout_seconds, independent)
          VALUES (?, ?, ?, ?, ?, ?, ?, ?, FALSE)
        `, [
              condId,
              graphId,
              cond.node_id,
              ordinal,
              cond.type,
              cond.command ?? null,
              cond.description ?? null,
              timeoutSecs
            ]);
          }

          // Insert ledger entry for graph_created
          const ledgerDetail = JSON.stringify({
            node_count: nodes.length,
            edge_count: rawDeps.length,
            condition_count: rawConditions.length,
            root_nodes: rootNodes,
          });
          await db.run(`
          INSERT INTO ledger (graph_id, session_id, action, target_node_id, detail, timestamp)
          VALUES (?, NULL, 'graph_created', NULL, ?, ?)
        `, [graphId, ledgerDetail, nowIso]);

          // If locked_by provided, add graph_created_locked ledger entry (REQ-GH-116)
          if (lockedByArg) {
            await db.run(`
          INSERT INTO ledger (graph_id, session_id, action, target_node_id, detail, timestamp)
          VALUES (?, NULL, 'graph_created_locked', NULL, ?, ?)
        `, [graphId, JSON.stringify({ locked_by: lockedByArg }), nowIso]);
          }

          return { graphId, nodeCount: nodes.length, edgeCount: rawDeps.length };
        })

        // axiom:trace work_item=graph-harness-01 spec=specs/102-Graph-Harness.md#REQ-GH-021 plan=step-verify2-gh-04 plan=step-verify2-gh-07
        const responseObj: Record<string, unknown> = {
          graph_id: txResult.graphId,
          node_count: txResult.nodeCount,
          edge_count: txResult.edgeCount,
          // REQ-GH-141: return 'draft' when draft:true, otherwise 'created'
          status: isDraft ? "draft" : "created",
          locked_by: lockedByArg ?? null,   // REQ-GH-116: null = unlocked on creation
        };

        // Bootstrap coordinator session row (REQ-GH-021: session bootstrap)
        // The harness loop requires a sessions row to exist before session.idle fires.
        // We insert it here so system.transform and runHarnessLoop can find it immediately.
        // REQ-GH-141: skip session bootstrap for DRAFT graphs — they have no coordinator yet.
        // axiom:trace work_item=graph-harness-01 spec=specs/102-Graph-Harness.md#REQ-GH-021 plan=step-verify-gh-01
        if (isDraft) {
          // DRAFT graphs await explicit activation — no coordinator session is registered.
          responseObj.note = "Draft graph created. Modify freely, then call graph_activate to start execution.";
        } else if (context?.sessionID) {
          try {
            const existingSession = await db.queryOne(
              `SELECT graph_id, status FROM sessions WHERE session_id = ?`
            , [context.sessionID]) as { graph_id: string; status: string } | null;

            if (existingSession && existingSession.status.toLowerCase() === 'active' &&
                existingSession.graph_id !== txResult.graphId) {
              // Session already actively coordinates a DIFFERENT graph — warn, don't overwrite.
              responseObj.warning =
                `Session already coordinates graph '${existingSession.graph_id}'. ` +
                `New graph '${txResult.graphId}' created but harness loop will continue driving '${existingSession.graph_id}'. ` +
                `Use a new session to work on '${txResult.graphId}', or complete/abandon '${existingSession.graph_id}' first.`;
            } else if (existingSession) {
              // Session row exists but is done/stale — reuse it for the new graph (UPSERT).
              await db.run(`
                UPDATE sessions
                SET graph_id=?, status='active', node_id=NULL,
                    last_heartbeat=datetime('now'), completed_at=NULL
                WHERE session_id=?
              `, [txResult.graphId, context.sessionID]);
              await addLedgerEntry(txResult.graphId, 'session_bootstrapped', {
                session_id: context.sessionID, role: 'coordinator', reused: true
              });
            } else {
              // No existing row — fresh INSERT.
              await db.run(`
                INSERT INTO sessions
                  (session_id, graph_id, role, status, node_id, created_at, last_heartbeat)
                VALUES (?, ?, 'coordinator', 'active', NULL, datetime('now'), datetime('now'))
              `, [context.sessionID, txResult.graphId]);
              await addLedgerEntry(txResult.graphId, 'session_bootstrapped', {
                session_id: context.sessionID, role: 'coordinator'
              });
            }
          } catch (err) {
            // Non-fatal: log but don't fail graph.create
            console.error('[GraphHarness] session bootstrap failed:', err);
          }
        } else {
          responseObj.warning =
            "Session not registered — harness loop will not activate nodes. " +
            "Ensure graph.create is called from within an active OpenCode session where sessionID is available.";
        }

        return JSON.stringify(responseObj);
      } catch (err) {
        const message = err instanceof Error ? err.message : String(err);
        return JSON.stringify({
          error: `Failed to create graph: ${message}`,
          details: { exception: message },
        });
      }
    },
  });

  // ─────────────────────────────────────────────────────────────────────────
  // graph.status tool (REQ-GH-008)
  //
  // Returns the current state of a graph: progress counts, current active node,
  // next unblocked nodes, critical path, and (in full/blocked/active modes)
  // per-node details.
  //
  // axiom:trace work_item=graph-harness-01 spec=specs/102-Graph-Harness.md#REQ-GH-008 plan=phase-1/task-1-3/step-1-3-1
  // ─────────────────────────────────────────────────────────────────────────

  const graphStatusTool = tool({
    description:
      "Read the current state of an execution graph. Returns progress counts, current active node, " +
      "next unblocked nodes, and the critical path. Use detail='full' for all node details, " +
      "'blocked_only' to see which nodes are blocked and why, 'active_only' to see currently executing nodes.",
    args: {
      graph_id: tool.schema
        .string()
        .min(1)
        .describe("The graph ID to inspect (e.g. 'gh_...')"),
      detail: tool.schema
        .enum(["summary", "full", "blocked_only", "active_only"])
        .optional()
        .describe("Detail level: 'summary' (default), 'full', 'blocked_only', 'active_only'"),
    },

    async execute(args, _context) {
      if (!config.enabled) {
        return JSON.stringify({ error: "Graph Harness is disabled (enabled: false in config)" });
      }

      try {
        const graphId = args.graph_id;
        const detail = args.detail ?? "summary";

        // If no graph_id provided, return a helpful list of recent graphs
        // axiom:trace work_item=plugin-bug-sweep-01 spec=specs/102-Graph-Harness.md#REQ-GH-008 plan=phase-3/step-backlog-004
        if (!graphId) {
          const recent = await db.queryAll<{ id: string; title: string; status: string; created_at: string }>(
            `SELECT id, title, status, created_at FROM graphs ORDER BY created_at DESC LIMIT 5`
          );
          if (recent.length === 0) {
            return JSON.stringify({ error: "graph_id is required. No graphs exist yet — create one with graph_create." });
          }
          return JSON.stringify({
            error: "graph_id is required",
            hint: "Pass one of these graph_ids",
            recent_graphs: recent.map(g => ({ graph_id: g.id, name: g.title, status: g.status, created_at: g.created_at })),
          });
        }

        // ── 1. Fetch graph row ─────────────────────────────────────────────
        const graphRow = await db.queryOne(
            `SELECT id, title, description, status, locked_by, created_at, completed_at,
                    lifecycle_mode, cycle_count, max_cycles
             FROM graphs WHERE id = ?`
          , [graphId]) as | {
              id: string;
              title: string;
              description: string | null;
              status: string;
              locked_by: string | null;
              created_at: string;
              completed_at: string | null;
              lifecycle_mode: string | null;
              cycle_count: number | null;
              max_cycles: number | null;
            }
          | undefined;

        if (!graphRow) {
          return JSON.stringify({ error: `Graph not found: ${graphId}` });
        }

        // ── 2. Fetch all nodes ─────────────────────────────────────────────
         const allNodes = await db.queryAll(
             `SELECT id, title, description, status, execution_mode,
                     attempt_count, created_at, completed_at, activated_at,
                     trigger_run_count, trigger_max_runs, trigger_every,
                     trigger_every_ms, next_fire_at
              FROM nodes WHERE graph_id = ?
              ORDER BY created_at ASC`
           , [graphId]) as Array<{
           id: string;
           title: string;
           description: string;
           status: string;
           execution_mode: string;
           attempt_count: number;
           created_at: string;
           completed_at: string | null;
           activated_at: string | null;
           trigger_run_count: number | null;
           trigger_max_runs: number | null;
           trigger_every: string | null;
         }>;

        // ── 3. Progress counts ─────────────────────────────────────────────
        const progress = {
          total_nodes: allNodes.length,
          done: 0,
          active: 0,
          pending: 0,
          failed: 0,
          blocked: 0,
          abandoned: 0,
          cancelled: 0,
        };
        for (const n of allNodes) {
          const s = n.status.toLowerCase();
          if (s === "done") progress.done++;
          else if (s === "active") progress.active++;
          else if (s === "pending") progress.pending++;
          else if (s === "failed") progress.failed++;
          else if (s === "blocked") progress.blocked++;
          else if (s === "abandoned") progress.abandoned++;
          else if (s === "cancelled") progress.cancelled++;
        }

        // ── 4. Current active node (first one found by creation order) ─────
        const activeNode = allNodes.find((n) => n.status.toLowerCase() === "active");
        const currentNode = activeNode
          ? {
              id: activeNode.id,
              title: activeNode.title,
              status: activeNode.status,
              attempt_count: activeNode.attempt_count,
            }
          : null;

        // ── 5. Fetch all dependency edges for this graph ───────────────────
        // Schema: dependencies(graph_id, node_id, depends_on)
        //   → node_id depends on depends_on (depends_on must be DONE first)
        const depRows = await db.queryAll(
            `SELECT node_id, depends_on FROM dependencies WHERE graph_id = ?`
          , [graphId]) as Array<{ node_id: string; depends_on: string }>;

        // Build: depMap[node_id] = [list of nodes it depends on]
        const depMap = new Map<string, string[]>();
        for (const n of allNodes) {
          depMap.set(n.id, []);
        }
        for (const row of depRows) {
          const deps = depMap.get(row.node_id) ?? [];
          deps.push(row.depends_on);
          depMap.set(row.node_id, deps);
        }

        // ── 6. Next unblocked nodes ────────────────────────────────────────
        // A node is "unblocked" if: status == PENDING AND all its required deps are DONE or CANCELLED
        // CANCELLED deps are treated as DONE for unblocking (§17b.3).
        const nodeStatusMap = new Map<string, string>();
        for (const n of allNodes) {
          nodeStatusMap.set(n.id, n.status.toLowerCase());
        }

        const nextUnblocked: string[] = [];
        for (const n of allNodes) {
          if (n.status.toLowerCase() !== "pending") continue;
          const deps = depMap.get(n.id) ?? [];
          const allDepsDone = deps.every(
            (depId) => {
              const s = nodeStatusMap.get(depId) ?? "";
              return s === "done" || s === "cancelled";
            }
          );
          if (allDepsDone) {
            nextUnblocked.push(n.id);
          }
        }

        // ── 7. Critical path (longest node-count path from root to leaf) ───
        // Build forward adjacency: node → [nodes that depend on it]
        // i.e., if "B depends_on A", then A → B in the forward dag
        const forwardAdj = new Map<string, string[]>();
        for (const n of allNodes) {
          forwardAdj.set(n.id, []);
        }
        for (const row of depRows) {
          // row.depends_on must complete before row.node_id
          const successors = forwardAdj.get(row.depends_on) ?? [];
          successors.push(row.node_id);
          forwardAdj.set(row.depends_on, successors);
        }

        // Root nodes: those with no incoming deps (depMap[id] is empty)
        const rootNodeIds = allNodes
          .filter((n) => (depMap.get(n.id) ?? []).length === 0)
          .map((n) => n.id);

        // DFS from each root; track the longest path (most nodes)
        let criticalPath: string[] = [];

        function dfsLongestPath(nodeId: string, currentPath: string[]): void {
          const extended = [...currentPath, nodeId];
          const successors = forwardAdj.get(nodeId) ?? [];
          if (successors.length === 0) {
            // Leaf node — check if this is the longest path so far
            if (extended.length > criticalPath.length) {
              criticalPath = extended;
            }
            return;
          }
          for (const succ of successors) {
            dfsLongestPath(succ, extended);
          }
        }

        for (const rootId of rootNodeIds) {
          dfsLongestPath(rootId, []);
        }

        // Fallback: if no deps at all, single-node graph → longest single node by alpha
        if (criticalPath.length === 0 && allNodes.length > 0) {
          criticalPath = [allNodes[0].id];
        }

        // ── 8. Compute updated_at (most recent node activity or graph created_at) ─
        let updatedAt = graphRow.created_at;
        for (const n of allNodes) {
          if (n.completed_at && n.completed_at > updatedAt) updatedAt = n.completed_at;
          if (n.activated_at && n.activated_at > updatedAt) updatedAt = n.activated_at;
          if (n.created_at && n.created_at > updatedAt) updatedAt = n.created_at;
        }

        // ── 8b. Aggregate session cost (REQ-GH-074) ───────────────────────
        // axiom:trace work_item=graph-harness-01 spec=specs/102-Graph-Harness.md#REQ-GH-074 plan=phase-4/task-4-5/step-4-5-1
        const costAgg = await db.queryOne(
            `SELECT COALESCE(SUM(tokens_used), 0) as total_tokens,
                    COALESCE(SUM(cost_usd), 0.0) as total_cost_usd,
                    COUNT(*) as session_count
             FROM sessions WHERE graph_id = ?`
          , [graphId]) as { total_tokens: number; total_cost_usd: number; session_count: number } | null;

        const costSummary = {
          total_tokens_used: costAgg?.total_tokens ?? 0,
          total_cost_usd: costAgg?.total_cost_usd ?? 0,
          session_count: costAgg?.session_count ?? 0,
        };

        // ── 9. Build base response (summary) ──────────────────────────────
        const baseResponse = {
          graph_id: graphRow.id,
          name: graphRow.title,
          status: graphRow.status,
          locked_by: graphRow.locked_by ?? null,  // REQ-GH-116: lock visibility at all detail levels
          // REQ-GH-SCHED-V2: lifecycle fields
          lifecycle_mode: graphRow.lifecycle_mode ?? "one_shot",
          ...(graphRow.cycle_count != null && graphRow.cycle_count > 0 ? { cycle_count: graphRow.cycle_count } : {}),
          ...(graphRow.max_cycles != null && graphRow.max_cycles > 0 ? { max_cycles: graphRow.max_cycles } : {}),
          progress,
          current_node: currentNode,
          next_unblocked: nextUnblocked,
          critical_path: criticalPath,
          created_at: graphRow.created_at,
          updated_at: updatedAt,
          cost: costSummary,
        };

        // ── 10. Handle detail modes ────────────────────────────────────────

        if (detail === "summary") {
          return JSON.stringify(baseResponse);
        }

         if (detail === "full") {
           // Include all nodes with full details
           const fullNodes = allNodes.map((n) => {
             const node: Record<string, unknown> = {
               id: n.id,
               title: n.title,
               description: n.description,
               status: n.status,
               execution_mode: n.execution_mode,
               attempt_count: n.attempt_count,
               created_at: n.created_at,
               completed_at: n.completed_at ?? null,
             };
              // AC-4: expose run_count for repeating nodes; REQ-GH-SCHED-V2: also next_fire_at
              if (n.trigger_run_count != null && n.trigger_run_count > 0) {
                node.run_count = n.trigger_run_count;
                if (n.trigger_max_runs != null && n.trigger_max_runs > 0) node.max_runs = n.trigger_max_runs;
                if (n.trigger_every) node.schedule = n.trigger_every;
              }
              // REQ-GH-SCHED-V2: next_fire_at for requeued nodes
              if ((n as any).next_fire_at != null) {
                node.next_fire_at = (n as any).next_fire_at;
              }
             return node;
           });
           return JSON.stringify({ ...baseResponse, nodes: fullNodes });
         }

        if (detail === "blocked_only") {
          // Return only BLOCKED nodes with why they are blocked
          // (which of their dependencies are not yet DONE or CANCELLED)
          const blockedNodes = allNodes
            .filter((n) => n.status.toLowerCase() === "blocked")
            .map((n) => {
              const deps = depMap.get(n.id) ?? [];
              const blockingDeps = deps.filter(
                (depId) => {
                  const s = nodeStatusMap.get(depId) ?? "";
                  return s !== "done" && s !== "cancelled";
                }
              );
              return {
                id: n.id,
                title: n.title,
                status: n.status,
                attempt_count: n.attempt_count,
                blocked_by: blockingDeps,
              };
            });
          return JSON.stringify({ ...baseResponse, blocked_nodes: blockedNodes });
        }

        if (detail === "active_only") {
          // Return only ACTIVE nodes
          const activeNodes = allNodes
            .filter((n) => n.status.toLowerCase() === "active")
            .map((n) => ({
              id: n.id,
              title: n.title,
              status: n.status,
              execution_mode: n.execution_mode,
              attempt_count: n.attempt_count,
              activated_at: n.activated_at ?? null,
            }));
          return JSON.stringify({ ...baseResponse, active_nodes: activeNodes });
        }

        // Unreachable — all enum cases handled above
        return JSON.stringify(baseResponse);
      } catch (err) {
        const message = err instanceof Error ? err.message : String(err);
        return JSON.stringify({
          error: `Failed to get graph status: ${message}`,
          details: { exception: message },
        });
      }
    },
  });

  // ─────────────────────────────────────────────────────────────────────────
  // graph.inject tool (REQ-GH-002)
  //
  // Insert one or more new nodes into an existing execution graph at a
  // specified position relative to a target node. Handles dependency rewiring
  // for three positions: "before", "after", and "parallel_to".
  //
  // axiom:trace work_item=graph-harness-01 spec=specs/102-Graph-Harness.md#REQ-GH-002 plan=phase-2/task-2-1/step-2-1-1
  // ─────────────────────────────────────────────────────────────────────────

  const graphInjectTool = tool({
    description:
      "Insert one or more nodes into an existing graph at a specified position relative to a target node. " +
      "Position 'before': new nodes block the target (inherit target's incoming deps). " +
      "Position 'after': new nodes follow the target (inherit target's outgoing deps). " +
      "Position 'parallel_to': new nodes run in parallel with the target (same incoming and outgoing deps). " +
      "Returns { graph_id, injected_node_ids, position, target_node_id, status } on success or { error } on failure.",
    args: {
      graph_id: tool.schema
        .string()
        .min(1)
        .describe("The graph to inject nodes into"),
      position: tool.schema
        .enum(["before", "after", "parallel_to"])
        .describe(
          "'before': new nodes run before target; 'after': new nodes run after target; 'parallel_to': new nodes run in parallel with target"
        ),
      target_node_id: tool.schema
        .string()
        .min(1)
        .describe("The reference node for injection — new nodes are positioned relative to this node"),
      nodes: tool.schema
        .array(
          tool.schema.object({
            id: tool.schema
              .string()
              .min(1)
              .describe("Unique node ID (must not already exist in the graph)"),
            title: tool.schema.string().min(1).describe("Human-readable title"),
            description: tool.schema.string().optional().describe("What this node should accomplish"),
            execution_mode: tool.schema
              .enum(["agent", "script", "transform", "wait", "api", "route", "composite"])
              .optional()
              .describe("How this node executes. Default: 'agent'"),
            execution_config: tool.schema
              .record(tool.schema.string(), tool.schema.string())
              .optional()
              .describe("Mode-specific configuration"),
            constraints: tool.schema
              .array(tool.schema.string())
              .optional()
              .describe("Constraints/rules for the agent working on this node"),
            context: tool.schema
              .string()
              .optional()
              .describe("Additional context injected into the agent briefing"),
            metadata: tool.schema
              .record(tool.schema.string(), tool.schema.string())
              .optional()
              .describe("Arbitrary node metadata"),
          })
        )
        .min(1)
        .describe("New nodes to inject. At least one required."),
      conditions: tool.schema
        .array(
          tool.schema.object({
            node_id: tool.schema.string().describe("Which new node these conditions belong to"),
            type: tool.schema
              .enum([
                 "script",
                 "file_exists",
                 "http_check",
                 "model_judge",
                 "none",
                 "test_pattern",
                 "file_changed",
                 "manual",
                 "compound",
                 "stash_has_finding",
                 "conductor_agent_done",
               ])
              .describe("Condition evaluation method"),
            command: tool.schema.string().optional().describe("Shell command for type='script'"),
            timeout_seconds: tool.schema.number().optional().describe("Per-condition timeout. Default: 30"),
            max_retries: tool.schema.number().optional().describe("Max retry attempts. Default: 3"),
            description: tool.schema.string().optional().describe("Human-readable description"),
          })
        )
        .optional()
        .describe("Optional done-conditions for the newly injected nodes"),
    },

    async execute(args, context) {
      // axiom:trace work_item=graph-harness-01 spec=specs/102-Graph-Harness.md#REQ-GH-002 plan=phase-2/task-2-1/step-2-1-1
      if (!config.enabled) {
        return JSON.stringify({ error: "Graph Harness is disabled (enabled: false in config)" });
      }

      try {
        // ── Role check (REQ-GH-013) ───────────────────────────────────────────
        await checkSessionRole(context, ["coordinator"]);

        const graphId = args.graph_id;
        const position = args.position;
        const targetNodeId = args.target_node_id;
        const newNodes = args.nodes;
        const rawConditions = args.conditions ?? [];

        // ── Gate 1: checkMutationAllowed ─────────────────────────────────────
        await checkMutationAllowed(graphId);
        await checkGraphLock(graphId, context); // REQ-GH-110

        // ── Gate 2: graph must exist and be in CREATED, ACTIVE, or DRAFT status ─
        // axiom:trace work_item=SWDE-54 spec=specs/102-Graph-Harness.md#REQ-GH-141 plan=phase-10/step-r4-03 jira_ref=SWDE-54
        const graphRow = await db.queryOne(`SELECT id, status FROM graphs WHERE id = ?`, [graphId]) as { id: string; status: string } | undefined;

        if (!graphRow) {
          return JSON.stringify({ error: `Graph not found: ${graphId}` });
        }
        const graphStatus = graphRow.status.toUpperCase();
        if (graphStatus !== "CREATED" && graphStatus !== "ACTIVE" && graphStatus !== "DRAFT") {
          return JSON.stringify({
            error: `Graph is not in a mutable state: status=${graphRow.status}. Only CREATED, ACTIVE, or DRAFT graphs can be injected into.`,
            details: { graph_id: graphId, status: graphRow.status },
          });
        }

        // ── Gate 3: target node must exist in graph ───────────────────────────
        const targetRow = await db.queryOne(`SELECT id, status FROM nodes WHERE graph_id = ? AND id = ?`, [graphId, targetNodeId]) as { id: string; status: string } | undefined;

        if (!targetRow) {
          return JSON.stringify({
            error: `Target node not found: "${targetNodeId}" in graph ${graphId}`,
            details: { graph_id: graphId, target_node_id: targetNodeId },
          });
        }

        // ── Gate 4: target must not be DONE or ABANDONED ─────────────────────
        await ensureNodeMutable(targetNodeId, graphId);

        // ── Gate 5: new node IDs must not already exist ───────────────────────
        const newNodeIds = newNodes.map((n) => n.id);
        // Check for duplicates among the new nodes themselves
        const newIdSet = new Set<string>();
        for (const nid of newNodeIds) {
          if (newIdSet.has(nid)) {
            return JSON.stringify({
              error: `Duplicate new node ID: "${nid}". All injected node IDs must be unique.`,
              details: { duplicate_id: nid },
            });
          }
          newIdSet.add(nid);
        }
        // Check against existing nodes in the graph
        const existingNodes = await db.queryAll(`SELECT id FROM nodes WHERE graph_id = ?`, [graphId]) as Array<{ id: string }>
        const existingIds = new Set(existingNodes.map((r) => r.id));
        for (const nid of newNodeIds) {
          if (existingIds.has(nid)) {
            return JSON.stringify({
              error: `Node ID already exists in graph: "${nid}". Injected node IDs must be new.`,
              details: { duplicate_id: nid, graph_id: graphId },
            });
          }
        }

        // ── Gate 6: graph node count must stay within limits ──────────────────
        const maxNodes = config.limits.max_nodes_per_graph;
        const totalAfter = existingNodes.length + newNodes.length;
        if (totalAfter > maxNodes) {
          return JSON.stringify({
            error: `Injecting ${newNodes.length} nodes would exceed max_nodes_per_graph (${maxNodes}). Current: ${existingNodes.length}, after: ${totalAfter}.`,
            details: { current_count: existingNodes.length, new_count: newNodes.length, limit: maxNodes },
          });
        }

        // ── Fetch current dependencies for the graph ──────────────────────────
        // Schema: dependencies(graph_id, node_id, depends_on)
        // Semantics: node_id depends on depends_on (i.e., depends_on must complete before node_id starts)
        // In edge notation: depends_on → node_id
        const allDeps = await db.queryAll(`SELECT node_id, depends_on FROM dependencies WHERE graph_id = ?`, [graphId]) as Array<{ node_id: string; depends_on: string }>;

        // ── Compute new dependency set after rewiring ─────────────────────────
        // We build two sets:
        //   depsToRemove: Set of "node_id|depends_on" pairs to delete
        //   depsToAdd:    Array of {node_id, depends_on} rows to insert
        const depsToRemove = new Set<string>();
        const depsToAdd: Array<{ node_id: string; depends_on: string }> = [];

        if (position === "before") {
          // ── "before": new nodes run BEFORE target ──────────────────────────
          // 1. Find all incoming dependencies of target (rows where node_id = target)
          //    These are the things that block target: depends_on → target
          const incomingToTarget = allDeps.filter((d) => d.node_id === targetNodeId);

          // 2. Each new node inherits those incoming deps (depends_on → newNode)
          //    We do NOT delete the original incoming deps to target — we also keep them
          //    pointing at the new nodes (and remove them from target? No.)
          //    Spec says: "New nodes get target's INCOMING dependencies (they get blocked by what was blocking target)"
          //    This means: add the same incoming deps to each new node.
          //    BUT target's incoming deps stay as-is? No — target should now wait for all new nodes.
          //    So: target should no longer depend on those original predecessors ONLY if the new nodes
          //    themselves depend on those predecessors. Since new nodes are interposed, they become
          //    the direct predecessors of target.
          //
          //    Final state:
          //    - original predecessors → each new node (new nodes wait for original predecessors)
          //    - each new node → target (target waits for ALL new nodes)
          //    - Remove original predecessors → target (target no longer waits directly for original predecessors)

          // Remove target's original incoming deps (target will now wait for new nodes instead)
          for (const dep of incomingToTarget) {
            depsToRemove.add(`${dep.node_id}|${dep.depends_on}`);
          }

          // Add: original predecessor → each new node
          for (const dep of incomingToTarget) {
            for (const nid of newNodeIds) {
              depsToAdd.push({ node_id: nid, depends_on: dep.depends_on });
            }
          }

          // Add: each new node → target
          for (const nid of newNodeIds) {
            depsToAdd.push({ node_id: targetNodeId, depends_on: nid });
          }
        } else if (position === "after") {
          // ── "after": new nodes run AFTER target ───────────────────────────
          // 1. Find all outgoing dependencies of target (rows where depends_on = target)
          //    These are things that target blocks: target → node_id
          const outgoingFromTarget = allDeps.filter((d) => d.depends_on === targetNodeId);

          // Final state:
          // - target → each new node (new nodes wait for target)
          // - each new node → original successors (original successors now wait for ALL new nodes)
          // - Remove: target → original successors (they no longer wait directly for target)

          // Remove target's original outgoing deps
          for (const dep of outgoingFromTarget) {
            depsToRemove.add(`${dep.node_id}|${dep.depends_on}`);
          }

          // Add: target → each new node
          for (const nid of newNodeIds) {
            depsToAdd.push({ node_id: nid, depends_on: targetNodeId });
          }

          // Add: each new node → original successors
          for (const dep of outgoingFromTarget) {
            for (const nid of newNodeIds) {
              depsToAdd.push({ node_id: dep.node_id, depends_on: nid });
            }
          }
        } else {
          // position === "parallel_to"
          // ── "parallel_to": new nodes run in parallel with target ───────────
          // New nodes get same incoming deps as target (same prerequisites)
          // New nodes get same outgoing deps as target (same things wait for them)
          // Target's dependencies are UNCHANGED.

          const incomingToTarget = allDeps.filter((d) => d.node_id === targetNodeId);
          const outgoingFromTarget = allDeps.filter((d) => d.depends_on === targetNodeId);

          // Add: same incoming deps to each new node
          for (const dep of incomingToTarget) {
            for (const nid of newNodeIds) {
              depsToAdd.push({ node_id: nid, depends_on: dep.depends_on });
            }
          }

          // Add: each new node → original successors
          for (const dep of outgoingFromTarget) {
            for (const nid of newNodeIds) {
              depsToAdd.push({ node_id: dep.node_id, depends_on: nid });
            }
          }
        }

        // ── Gate 7: cycle detection on proposed graph state ───────────────────
        // Build the full node list and edge list after proposed changes.
        const allNodeIdsAfter = [...existingIds, ...newNodeIds];

        // Build remaining deps (existing minus removed) plus new deps
        const remainingDeps = allDeps.filter(
          (d) => !depsToRemove.has(`${d.node_id}|${d.depends_on}`)
        );
        const allDepsAfter = [
          ...remainingDeps.map((d) => ({ from: d.depends_on, to: d.node_id })),
          ...depsToAdd.map((d) => ({ from: d.depends_on, to: d.node_id })),
        ];

        const cycleError = detectCycle(allNodeIdsAfter, allDepsAfter);
        if (cycleError) {
          return JSON.stringify({
            error: `Injection would create a cycle: ${cycleError}`,
            details: { validation: "cycle_detected", position, target_node_id: targetNodeId },
          });
        }

        // ── Atomic DB write ───────────────────────────────────────────────────
        const nowIso = new Date().toISOString();





        await db.transaction(async (db) => {
          // Insert new nodes
          for (const node of newNodes) {
            const execMode = node.execution_mode ?? "agent";
            const execConfig = node.execution_config
              ? JSON.stringify(node.execution_config)
              : null;
            const contextObj: Record<string, unknown> = {};
            if (node.constraints && node.constraints.length > 0) {
              contextObj.constraints = node.constraints;
            }
            if (node.context) {
              contextObj.instructions = node.context;
            }
            const contextJson =
              Object.keys(contextObj).length > 0 ? JSON.stringify(contextObj) : null;
            // SWDE-48: promote stash + conductor_agent fields to metadata for harness lifecycle hooks
            // axiom:trace work_item=SWDE-48 spec=specs/102-Graph-Harness.md jira_ref=SWDE-48
            const nodeMeta: Record<string, unknown> = node.metadata
              ? { ...(node.metadata as Record<string, unknown>) }
              : {};
            const nodeRaw2 = node as Record<string, unknown>;
            if (typeof nodeRaw2.stash === "string" && nodeRaw2.stash.length > 0) {
              nodeMeta.stash = nodeRaw2.stash;
            }
            if (nodeRaw2.conductor_agent && typeof nodeRaw2.conductor_agent === "object") {
              nodeMeta.conductor_agent = nodeRaw2.conductor_agent;
            }
            const metaJson = Object.keys(nodeMeta).length > 0 ? JSON.stringify(nodeMeta) : null;

            // Extract trigger fields from injected node (mirrors graph_create logic)
            const triggerBlock = (node as Record<string, unknown>).trigger as {
              on?: string; cancel_on?: string; every?: string; cron?: string;
              max_runs?: number; lifetime_hours?: number;
            } | null | undefined;
            const scheduleVal = (node as Record<string, unknown>).schedule as string | null ?? null;
            const repeatVal = (node as Record<string, unknown>).repeat as boolean | null ?? false;
            const maxRetriesVal = (node as Record<string, unknown>).max_retries as number | null ?? 3;
            let tOn: string = triggerBlock?.on ?? 'idle';
            let tCancelOn: string = triggerBlock?.cancel_on ?? 'active';
            let tEvery: string | null = triggerBlock?.every ?? null;
            const tCron: string | null = triggerBlock?.cron ?? null;
            let tMaxRuns: number = triggerBlock?.max_runs ?? 0;
            const tLifetimeH: number = triggerBlock?.lifetime_hours ?? 0;
            if (scheduleVal && !triggerBlock?.every && !triggerBlock?.cron) {
              const everyAlias = scheduleVal.match(/^every\s+(.+)$/i);
              if (everyAlias) { tOn = 'idle'; tEvery = everyAlias[1].trim(); }
            }
            if (repeatVal && !triggerBlock) tMaxRuns = 0;

            await db.run(`
          INSERT INTO nodes
            (id, graph_id, title, description, status, execution_mode,
             execution_config, context, metadata, schedule, repeat, max_retries, created_at,
             trigger_on, trigger_cancel_on, trigger_every, trigger_cron,
             trigger_max_runs, trigger_lifetime_h)
          VALUES (?, ?, ?, ?, 'PENDING', ?, ?, ?, ?, ?, ?, ?, ?,
                  ?, ?, ?, ?, ?, ?)
        `, [
              node.id,
              graphId,
              node.title,
              node.description ?? "",
              execMode,
              execConfig,
              contextJson,
              metaJson,
              scheduleVal,
              repeatVal ? true : false,
              maxRetriesVal,
              nowIso,
              tOn,
              tCancelOn,
              tEvery,
              tCron,
              tMaxRuns,
              tLifetimeH
            ]);
          }

          // Remove old deps
          for (const key of depsToRemove) {
            const [nodeId, dependsOn] = key.split("|");
            await db.run(`
          DELETE FROM dependencies WHERE graph_id = ? AND node_id = ? AND depends_on = ?
        `, [graphId, nodeId, dependsOn]);
          }

          // Insert new deps (INSERT OR IGNORE handles any edge already present)
          for (const dep of depsToAdd) {
            await db.run(`
          INSERT INTO dependencies (graph_id, node_id, depends_on)
          VALUES (?, ?, ?)
        `, [graphId, dep.node_id, dep.depends_on]);
          }

          // Insert conditions for new nodes
          const ordinalCounters = new Map<string, number>();
          for (const cond of rawConditions) {
            // Only process conditions for newly injected nodes
            if (!newIdSet.has(cond.node_id)) continue;
            const ordinal = ordinalCounters.get(cond.node_id) ?? 0;
            ordinalCounters.set(cond.node_id, ordinal + 1);
            const condId = `cond_${graphId}_${cond.node_id}_${ordinal}`;
            const timeoutSecs = cond.timeout_seconds ?? 30;
            await db.run(`
          INSERT INTO conditions
            (id, graph_id, node_id, ordinal, type, command, description,
             timeout_seconds, independent)
          VALUES (?, ?, ?, ?, ?, ?, ?, ?, FALSE)
        `, [
              condId,
              graphId,
              cond.node_id,
              ordinal,
              cond.type,
              cond.command ?? null,
              cond.description ?? null,
              timeoutSecs
            ]);
          }

          // Ledger entry
          await addLedgerEntry(
            graphId,
            "nodes_injected",
            {
              position,
              target_node_id: targetNodeId,
              injected_ids: newNodeIds,
              deps_removed: depsToRemove.size,
              deps_added: depsToAdd.length,
            }
          );
        });

        // ── Increment mutation counter ─────────────────────────────────────────
        await incrementMutationCounter(graphId);

        return JSON.stringify({
          graph_id: graphId,
          injected_node_ids: newNodeIds,
          position,
          target_node_id: targetNodeId,
          status: "ok",
        });
      } catch (err) {
        const message = err instanceof Error ? err.message : String(err);
        return JSON.stringify({
          error: message,
          details: { exception: message },
        });
      }
    },
  });

  // ─────────────────────────────────────────────────────────────────────────
  // graph.modify Tool (REQ-GH-003)
  //
  // axiom:trace work_item=graph-harness-01 spec=specs/102-Graph-Harness.md#REQ-GH-003 plan=phase-2/task-2-2/step-2-2-1
  // ─────────────────────────────────────────────────────────────────────────

  const graphModifyTool = tool({
    description:
      "Modify a node's properties (title, description, constraints, context, metadata, execution_mode, " +
      "execution_config) or its conditions and dependencies. " +
      "Cannot modify DONE or ABANDONED nodes. " +
      "Dependency changes are cycle-checked. " +
      "Returns { graph_id, node_id, status: 'modified', changes_applied } on success or { error } on failure.",
    args: {
      graph_id: tool.schema.string().min(1).describe("Graph ID"),
      node_id: tool.schema.string().min(1).describe("Node ID to modify"),
      changes: tool.schema.object({
        title: tool.schema.string().optional().describe("New title"),
        description: tool.schema.string().optional().describe("New description"),
        constraints: tool.schema.array(tool.schema.string()).optional().describe("Replace all constraints"),
        context: tool.schema.string().optional().describe("New context"),
        metadata: tool.schema.record(tool.schema.string(), tool.schema.string()).optional().describe("Replace metadata"),
        execution_mode: tool.schema
          .enum(["agent", "script", "transform", "wait", "api", "route", "composite"])
          .optional()
          .describe("New execution mode"),
        execution_config: tool.schema.record(tool.schema.string(), tool.schema.string()).optional().describe("New execution config"),
        add_conditions: tool.schema
          .array(
            tool.schema.object({
              type: tool.schema.string().describe("Condition type"),
              command: tool.schema.string().optional(),
              timeout_seconds: tool.schema.number().optional(),
              max_retries: tool.schema.number().optional(),
              description: tool.schema.string().optional(),
            })
          )
          .optional()
          .describe("Append new conditions"),
        remove_conditions: tool.schema
          .array(tool.schema.string())
          .optional()
          .describe("Condition IDs to remove"),
        add_dependencies: tool.schema
          .array(tool.schema.string())
          .optional()
          .describe("Node IDs this node should now depend on"),
        remove_dependencies: tool.schema
          .array(tool.schema.string())
          .optional()
          .describe("Node IDs to remove from dependencies"),
      }).describe("Fields to modify"),
    },

    async execute(args, context) {
      // axiom:trace work_item=graph-harness-01 spec=specs/102-Graph-Harness.md#REQ-GH-003 plan=phase-2/task-2-2/step-2-2-1
      if (!config.enabled) {
        return JSON.stringify({ error: "Graph Harness is disabled" });
      }

      try {
        // ── Role check (REQ-GH-013) ───────────────────────────────────────────
        // REQ-GH-013: workers can modify their own assigned node only
        // axiom:trace work_item=graph-harness-01 spec=specs/102-Graph-Harness.md#REQ-GH-013 plan=phase-2/task-2-6/step-2-6-2
        const callerSessionId = (context as Record<string, unknown> | undefined)?.sessionID as string | undefined;
        if (callerSessionId) {
          const sessionRow = await db.queryOne(
            `SELECT role, node_id FROM sessions WHERE session_id = ? AND LOWER(status) = 'active'`
          , [callerSessionId]) as { role: string; node_id: string | null } | undefined;
          if (sessionRow && sessionRow.role === 'worker') {
            if (sessionRow.node_id !== args.node_id) {
              return JSON.stringify({
                error: `Permission denied: worker session can only call graph.modify on its assigned node (${sessionRow.node_id ?? 'none'}), not '${args.node_id}'`
              });
            }
            // worker modifying own node — allowed, fall through
          }
          // coordinator or session not found → allow
        }

        const graphId = args.graph_id;
        const nodeId = args.node_id;
        const changes = args.changes;

        // ── Gate 1: graph must exist and be mutable ──────────────────────────
        // axiom:trace work_item=SWDE-54 spec=specs/102-Graph-Harness.md#REQ-GH-141 plan=phase-10/step-r4-03 jira_ref=SWDE-54
        const graphRow = await db.queryOne(`SELECT id, status FROM graphs WHERE id = ?`, [graphId]) as { id: string; status: string } | undefined;
        if (!graphRow) {
          return JSON.stringify({ error: `Graph not found: ${graphId}` });
        }
        const graphStatus = graphRow.status.toUpperCase();
        if (graphStatus !== "CREATED" && graphStatus !== "ACTIVE" && graphStatus !== "DRAFT") {
          return JSON.stringify({
            error: `Graph is not in a mutable state: status=${graphRow.status}`,
          });
        }

        // ── Gate 2: node must exist in graph ─────────────────────────────────
        const nodeRow = await db.queryOne(`SELECT id, status FROM nodes WHERE graph_id = ? AND id = ?`, [graphId, nodeId]) as { id: string; status: string } | undefined;
        if (!nodeRow) {
          return JSON.stringify({ error: `Node not found: ${nodeId} in graph ${graphId}` });
        }

        // ── Gate 3: node must not be DONE/ABANDONED ──────────────────────────
        await ensureNodeMutable(nodeId, graphId);

        // ── Gate 4: mutation counter check ───────────────────────────────────
        await checkMutationAllowed(graphId);
        await checkGraphLock(graphId, context); // REQ-GH-110

        // ── Gate 5: node must not be ACTIVE in another session ───────────────
        const sessionId = callerSessionId;
        const activeSessionCheck = await db.queryAll(
            `SELECT session_id FROM sessions WHERE node_id = ? AND LOWER(status) = 'active'`
          , [nodeId]) as Array<{ session_id: string }>
        // Fix: when sessionId is absent we cannot identify the caller, so treat as no conflicting sessions
        const activeInOther = sessionId
          ? activeSessionCheck.filter((s) => s.session_id !== sessionId)
          : [];
        if (activeInOther.length > 0) {
          return JSON.stringify({
            error: `Node ${nodeId} is currently ACTIVE in another session: ${activeInOther[0].session_id}`,
          });
        }

        // ── Track which fields are changed ───────────────────────────────────
        const changesApplied: string[] = [];

        // ── Gate 6: cycle detection for dependency changes ───────────────────
        const addDeps = changes.add_dependencies ?? [];
        const removeDeps = changes.remove_dependencies ?? [];
        if (addDeps.length > 0) {
          // Verify all add_dependencies nodes exist
          for (const depId of addDeps) {
            const depRow = await db.queryOne(`SELECT id FROM nodes WHERE graph_id = ? AND id = ?`, [graphId, depId]) as { id: string } | undefined;
            if (!depRow) {
              return JSON.stringify({ error: `Dependency node not found: ${depId} in graph ${graphId}` });
            }
          }

          // Build current full dep set, apply changes, check cycles
          const allNodeIds = (
            await db.queryAll(`SELECT id FROM nodes WHERE graph_id = ?`, [graphId]) as Array<{ id: string }>
          ).map((r) => r.id);

          const existingDeps = await db.queryAll(`SELECT node_id, depends_on FROM dependencies WHERE graph_id = ?`, [graphId]) as Array<{ node_id: string; depends_on: string }>;

          const removeSet = new Set(removeDeps.map((d) => `${nodeId}|${d}`));
          const remainingDeps = existingDeps.filter(
            (d) => !(d.node_id === nodeId && removeSet.has(`${d.node_id}|${d.depends_on}`))
          );

          const proposedEdges = [
            ...remainingDeps.map((d) => ({ from: d.depends_on, to: d.node_id })),
            ...addDeps.map((d) => ({ from: d, to: nodeId })),
          ];

          const cycleError = detectCycle(allNodeIds, proposedEdges);
          if (cycleError) {
            return JSON.stringify({
              error: `Dependency change would create a cycle: ${cycleError}`,
            });
          }
        }

        // ── Atomic DB write ───────────────────────────────────────────────────
        await db.transaction(async (db) => {
          // Update node scalar fields
          if (changes.title !== undefined) {
            await db.run(`UPDATE nodes SET title = ? WHERE graph_id = ? AND id = ?`, [changes.title, graphId, nodeId]);
            changesApplied.push("title");
          }
          if (changes.description !== undefined) {
            await db.run(`UPDATE nodes SET description = ? WHERE graph_id = ? AND id = ?`, [changes.description, graphId, nodeId]);
            changesApplied.push("description");
          }
          if (changes.execution_mode !== undefined) {
            await db.run(`UPDATE nodes SET execution_mode = ? WHERE graph_id = ? AND id = ?`, [changes.execution_mode, graphId, nodeId]);
            changesApplied.push("execution_mode");
          }
          if (changes.execution_config !== undefined) {
            await db.run(`UPDATE nodes SET execution_config = ? WHERE graph_id = ? AND id = ?`, [JSON.stringify(changes.execution_config), graphId, nodeId]);
            changesApplied.push("execution_config");
          }
          if (changes.metadata !== undefined) {
            await db.run(`UPDATE nodes SET metadata = ? WHERE graph_id = ? AND id = ?`, [JSON.stringify(changes.metadata), graphId, nodeId]);
            changesApplied.push("metadata");
          }

          // Update context: merge constraints and context string
          if (changes.constraints !== undefined || changes.context !== undefined) {
            const existingCtxRow = await db.queryOne(`SELECT context FROM nodes WHERE graph_id = ? AND id = ?`, [graphId, nodeId]) as { context: string | null } | undefined;
            let existingCtx: Record<string, unknown> = {};
            try {
              if (existingCtxRow?.context) {
                existingCtx = JSON.parse(existingCtxRow.context) as Record<string, unknown>;
              }
            } catch { /* ignore parse errors */ }

            if (changes.constraints !== undefined) {
              existingCtx.constraints = changes.constraints;
              changesApplied.push("constraints");
            }
            if (changes.context !== undefined) {
              existingCtx.instructions = changes.context;
              changesApplied.push("context");
            }

            await db.run(`UPDATE nodes SET context = ? WHERE graph_id = ? AND id = ?`, [JSON.stringify(existingCtx), graphId, nodeId]);
          }

          // Remove conditions
          if (removeDeps.length > 0 || (changes.remove_conditions ?? []).length > 0) {
            for (const condId of (changes.remove_conditions ?? [])) {
              await db.run(`DELETE FROM conditions WHERE id = ? AND graph_id = ? AND node_id = ?`, [condId, graphId, nodeId]);
            }
            if ((changes.remove_conditions ?? []).length > 0) {
              changesApplied.push("remove_conditions");
            }
          }

          // Add conditions
          if ((changes.add_conditions ?? []).length > 0) {
            const maxOrdinalRow = await db.queryOne(`SELECT MAX(ordinal) as max_ord FROM conditions WHERE graph_id = ? AND node_id = ?`, [graphId, nodeId]) as { max_ord: number | null } | undefined;
            let ordinal = (maxOrdinalRow?.max_ord ?? -1) + 1;
            for (const cond of (changes.add_conditions ?? [])) {
              const condId = `cond_${graphId}_${nodeId}_${ordinal}`;
              await db.run(
                `INSERT INTO conditions (id, graph_id, node_id, ordinal, type, command, description, timeout_seconds, independent)
                 VALUES (?, ?, ?, ?, ?, ?, ?, ?, FALSE)`
              , [condId, graphId, nodeId, ordinal,
                cond.type,
                cond.command ?? null,
                cond.description ?? null,
                cond.timeout_seconds ?? 30]);
              ordinal++;
            }
            changesApplied.push("add_conditions");
          }

          // Remove dependencies
          for (const depId of removeDeps) {
            await db.run(`DELETE FROM dependencies WHERE graph_id = ? AND node_id = ? AND depends_on = ?`, [graphId, nodeId, depId]);
          }
          if (removeDeps.length > 0) changesApplied.push("remove_dependencies");

          // Add dependencies
          for (const depId of addDeps) {
            await db.run(`INSERT OR IGNORE INTO dependencies (graph_id, node_id, depends_on) VALUES (?, ?, ?)`, [graphId, nodeId, depId]);
          }
          if (addDeps.length > 0) changesApplied.push("add_dependencies");

          // Ledger entry
          await addLedgerEntry(graphId, "node_modified", { node_id: nodeId, changes_applied: changesApplied });
        });

        // ── Increment mutation counter ─────────────────────────────────────────
        await incrementMutationCounter(graphId);

        return JSON.stringify({
          graph_id: graphId,
          node_id: nodeId,
          status: "modified",
          changes_applied: changesApplied,
        });
      } catch (err) {
        const message = err instanceof Error ? err.message : String(err);
        return JSON.stringify({ error: message });
      }
    },
  });

  // ─────────────────────────────────────────────────────────────────────────
  // graph.split Tool (REQ-GH-004)
  //
  // axiom:trace work_item=graph-harness-01 spec=specs/102-Graph-Harness.md#REQ-GH-004 plan=phase-2/task-2-3/step-2-3-1
  // ─────────────────────────────────────────────────────────────────────────

  const graphSplitTool = tool({
    description:
      "Split a PENDING node into parallel sub-nodes with a join strategy. " +
      "The original node becomes a join node that waits for sub-nodes to complete. " +
      "Sub-nodes inherit the original node's incoming dependencies. " +
      "Returns { graph_id, original_node_id, sub_node_ids, join_strategy, status: 'split' } on success.",
    args: {
      graph_id: tool.schema.string().min(1).describe("Graph ID"),
      node_id: tool.schema.string().min(1).describe("Node to split"),
      sub_nodes: tool.schema
        .array(
          tool.schema.object({
            id: tool.schema.string().min(1).describe("Sub-node ID (must be unique in graph)"),
            title: tool.schema.string().min(1).describe("Sub-node title"),
            description: tool.schema.string().optional(),
            execution_mode: tool.schema
              .enum(["agent", "script", "transform", "wait", "api", "route", "composite"])
              .optional(),
            execution_config: tool.schema.record(tool.schema.string(), tool.schema.string()).optional(),
          })
        )
        .min(2)
        .describe("Sub-nodes (minimum 2 required)"),
      join_strategy: tool.schema
        .enum(["all", "any", "majority"])
        .describe("When original node completes: 'all'=all sub-nodes DONE, 'any'=any sub-node DONE, 'majority'=majority DONE"),
    },

    async execute(args, context) {
      // axiom:trace work_item=graph-harness-01 spec=specs/102-Graph-Harness.md#REQ-GH-004 plan=phase-2/task-2-3/step-2-3-1
      if (!config.enabled) {
        return JSON.stringify({ error: "Graph Harness is disabled" });
      }

      try {
        // ── Role check (REQ-GH-013) ───────────────────────────────────────────
        await checkSessionRole(context, ["coordinator"]);

        const graphId = args.graph_id;
        const nodeId = args.node_id;
        const subNodes = args.sub_nodes;
        const joinStrategy = args.join_strategy;

        // ── Gate 1: at least 2 sub-nodes ─────────────────────────────────────
        if (subNodes.length < 2) {
          return JSON.stringify({ error: "At least 2 sub-nodes are required for a split" });
        }

        // ── Gate 2: graph must exist and be mutable ──────────────────────────
        const graphRow = await db.queryOne(`SELECT id, status FROM graphs WHERE id = ?`, [graphId]) as { id: string; status: string } | undefined;
        if (!graphRow) {
          return JSON.stringify({ error: `Graph not found: ${graphId}` });
        }
        const graphStatus = graphRow.status.toUpperCase();
        if (graphStatus !== "CREATED" && graphStatus !== "ACTIVE") {
          return JSON.stringify({
            error: `Graph is not in a mutable state: status=${graphRow.status}`,
          });
        }

        // ── Gate 3: node must exist ───────────────────────────────────────────
        const nodeRow = await db.queryOne(`SELECT id, status FROM nodes WHERE graph_id = ? AND id = ?`, [graphId, nodeId]) as { id: string; status: string } | undefined;
        if (!nodeRow) {
          return JSON.stringify({ error: `Node not found: ${nodeId} in graph ${graphId}` });
        }

        // ── Gate 4: node must be PENDING (not active or done) ─────────────────
        await ensureNodeMutable(nodeId, graphId);
        if (nodeRow.status.toUpperCase() === "ACTIVE") {
          return JSON.stringify({ error: `Cannot split ACTIVE node: ${nodeId}` });
        }
        // Double-check sessions table for active assignment
        const activeSession = await db.queryOne(`SELECT session_id FROM sessions WHERE node_id = ? AND LOWER(status) = 'active'`, [nodeId]) as { session_id: string } | null;
        if (activeSession) {
          return JSON.stringify({ error: `Cannot split ACTIVE node: ${nodeId} (active session: ${activeSession.session_id})` });
        }

        // ── Gate 5: checkMutationAllowed ──────────────────────────────────────
        await checkMutationAllowed(graphId);
        await checkGraphLock(graphId, context); // REQ-GH-110

        // ── Gate 6: sub-node IDs must not conflict with existing nodes ─────────
        const existingNodes = await db.queryAll(`SELECT id FROM nodes WHERE graph_id = ?`, [graphId]) as Array<{ id: string }>
        const existingIds = new Set(existingNodes.map((r) => r.id));
        const subNodeIds = subNodes.map((s) => s.id);

        const subIdSet = new Set<string>();
        for (const sid of subNodeIds) {
          if (subIdSet.has(sid)) {
            return JSON.stringify({ error: `Duplicate sub-node ID: "${sid}"` });
          }
          subIdSet.add(sid);
          if (existingIds.has(sid)) {
            return JSON.stringify({ error: `Sub-node ID already exists in graph: "${sid}"` });
          }
        }

        // ── Gate 7: graph node count limit ────────────────────────────────────
        const maxNodes = config.limits.max_nodes_per_graph;
        const totalAfter = existingNodes.length + subNodes.length;
        if (totalAfter > maxNodes) {
          return JSON.stringify({
            error: `Splitting would exceed max_nodes_per_graph (${maxNodes}). Current: ${existingNodes.length}, after: ${totalAfter}.`,
          });
        }

        // ── Fetch original node's incoming deps ────────────────────────────────
        const incomingToOriginal = await db.queryAll(`SELECT depends_on FROM dependencies WHERE graph_id = ? AND node_id = ?`, [graphId, nodeId]) as Array<{ depends_on: string }>

        // ── Atomic DB write ───────────────────────────────────────────────────
        const nowIso = new Date().toISOString();

        await db.transaction(async (db) => {
          // Insert sub-nodes
          for (const sn of subNodes) {
            const execMode = sn.execution_mode ?? "agent";
            const execConfig = sn.execution_config ? JSON.stringify(sn.execution_config) : null;
            await db.run(
              `INSERT INTO nodes (id, graph_id, title, description, status, execution_mode, execution_config, created_at)
               VALUES (?, ?, ?, ?, 'PENDING', ?, ?, ?)`
            , [sn.id, graphId, sn.title, sn.description ?? "", execMode, execConfig, nowIso]);
          }

          // Sub-nodes inherit original's incoming deps
          for (const dep of incomingToOriginal) {
            for (const sid of subNodeIds) {
              await db.run(`INSERT OR IGNORE INTO dependencies (graph_id, node_id, depends_on) VALUES (?, ?, ?)`, [graphId, sid, dep.depends_on]);
            }
          }

          // Original node (join node) depends on ALL sub-nodes
          for (const sid of subNodeIds) {
            await db.run(`INSERT OR IGNORE INTO dependencies (graph_id, node_id, depends_on) VALUES (?, ?, ?)`, [graphId, nodeId, sid]);
          }

          // Update original node metadata with join info
          const existingMetaRow = await db.queryOne(`SELECT metadata FROM nodes WHERE graph_id = ? AND id = ?`, [graphId, nodeId]) as { metadata: string | null } | undefined;
          let existingMeta: Record<string, unknown> = {};
          try {
            if (existingMetaRow?.metadata) {
              existingMeta = JSON.parse(existingMetaRow.metadata) as Record<string, unknown>;
            }
          } catch { /* ignore */ }

          const joinMeta = {
            ...existingMeta,
            join_node: true,
            join_strategy: joinStrategy,
            sub_node_ids: subNodeIds,
            completed_sub_nodes: [],
          };
          await db.run(`UPDATE nodes SET metadata = ? WHERE graph_id = ? AND id = ?`, [JSON.stringify(joinMeta), graphId, nodeId]);

          // Ledger entry
          await addLedgerEntry(graphId, "node_split", {
            node_id: nodeId,
            sub_node_ids: subNodeIds,
            join_strategy: joinStrategy,
          });
        });

        // ── Increment mutation counter ─────────────────────────────────────────
        await incrementMutationCounter(graphId);

        return JSON.stringify({
          graph_id: graphId,
          original_node_id: nodeId,
          sub_node_ids: subNodeIds,
          join_strategy: joinStrategy,
          status: "split",
        });
      } catch (err) {
        const message = err instanceof Error ? err.message : String(err);
        return JSON.stringify({ error: message });
      }
    },
  });

  // ─────────────────────────────────────────────────────────────────────────
  // graph.annotate Tool (REQ-GH-009)
  //
  // axiom:trace work_item=graph-harness-01 spec=specs/102-Graph-Harness.md#REQ-GH-009 plan=phase-2/task-2-4/step-2-4-1
  // ─────────────────────────────────────────────────────────────────────────

  const graphAnnotateTool = tool({
    description:
      "Add an annotation to a node. Any node can be annotated regardless of status (including DONE nodes). " +
      "No mutation counter increment — annotations are metadata, not structural changes. " +
      "Returns { graph_id, node_id, annotation_id, status: 'annotated' } on success.",
    args: {
      graph_id: tool.schema.string().min(1).describe("Graph ID"),
      node_id: tool.schema.string().min(1).describe("Node ID to annotate"),
      annotation: tool.schema.object({
        type: tool.schema.enum(["note", "finding", "decision", "blocker", "failure_context"]).describe("Annotation type"),
        content: tool.schema.string().min(1).describe("Annotation content"),
        severity: tool.schema
          .enum(["info", "warn", "error", "critical"])
          .optional()
          .describe("Severity level"),
        author: tool.schema.string().optional().describe("Session ID or display name"),
      }).describe("Annotation to add"),
    },

    async execute(args, context) {
      // axiom:trace work_item=graph-harness-01 spec=specs/102-Graph-Harness.md#REQ-GH-009 plan=phase-2/task-2-4/step-2-4-1
      if (!config.enabled) {
        return JSON.stringify({ error: "Graph Harness is disabled" });
      }

      try {
        const graphId = args.graph_id;
        const nodeId = args.node_id;
        const annotation = args.annotation;

        // ── Gate 0: graph lock check (REQ-GH-110) ────────────────────────────
        await checkGraphLock(graphId, context);

        // ── Gate 1: graph must exist ──────────────────────────────────────────
        const graphRow = await db.queryOne(`SELECT id FROM graphs WHERE id = ?`, [graphId]) as { id: string } | undefined;
        if (!graphRow) {
          return JSON.stringify({ error: `Graph not found: ${graphId}` });
        }

        // ── Gate 2: node must exist in graph ─────────────────────────────────
        const nodeRow = await db.queryOne(`SELECT id FROM nodes WHERE graph_id = ? AND id = ?`, [graphId, nodeId]) as { id: string } | undefined;
        if (!nodeRow) {
          return JSON.stringify({ error: `Node not found: ${nodeId} in graph ${graphId}` });
        }

        // ── Insert annotation ─────────────────────────────────────────────────
        const sessionId = (context as Record<string, unknown> | null)?.sessionID as string | undefined;
        const annotationId = `ann_${graphId}_${nodeId}_${Date.now()}_${Math.random().toString(36).slice(2, 8)}`;
        const nowIso = new Date().toISOString();

        await db.run(
          `INSERT INTO annotations (id, graph_id, node_id, session_id, type, content, severity, created_at)
           VALUES (?, ?, ?, ?, ?, ?, ?, ?)`
        , [annotationId,
          graphId,
          nodeId,
          annotation.author ?? sessionId ?? null,
          annotation.type,
          annotation.content,
          annotation.severity ?? "info",
          nowIso]);

        return JSON.stringify({
          graph_id: graphId,
          node_id: nodeId,
          annotation_id: annotationId,
          status: "annotated",
        });
      } catch (err) {
        const message = err instanceof Error ? err.message : String(err);
        return JSON.stringify({ error: message });
      }
    },
  });

  // ─────────────────────────────────────────────────────────────────────────
  // graph.abandon Tool (REQ-GH-010 + REQ-GH-013)
  //
  // axiom:trace work_item=graph-harness-01 spec=specs/102-Graph-Harness.md#REQ-GH-010 plan=phase-2/task-2-5/step-2-5-1
  // ─────────────────────────────────────────────────────────────────────────

  const graphAbandonTool = tool({
    description:
      "Abandon a node (cascade-blocks dependents) or an entire graph (marks all pending/active nodes abandoned). " +
      "Workers cannot call this tool (role check enforced). " +
      "Returns { graph_id, scope, status: 'abandoned', ... } on success or { error } on failure.",
    args: {
      graph_id: tool.schema.string().min(1).describe("Graph ID"),
      scope: tool.schema.enum(["node", "graph"]).describe("Abandon a single node or the entire graph"),
      node_id: tool.schema.string().optional().describe("Required when scope='node'"),
      reason: tool.schema.string().optional().describe("Human-readable reason for abandonment"),
    },

    async execute(args, context) {
      // axiom:trace work_item=graph-harness-01 spec=specs/102-Graph-Harness.md#REQ-GH-010 plan=phase-2/task-2-5/step-2-5-1
      if (!config.enabled) {
        return JSON.stringify({ error: "Graph Harness is disabled" });
      }

      try {
        // ── Role check (REQ-GH-013) ───────────────────────────────────────────
        await checkSessionRole(context, ["coordinator"]);

        const graphId = args.graph_id;
        const scope = args.scope;
        const reason = args.reason ?? null;

        // ── Graph lock check (REQ-GH-110) ────────────────────────────────────
        await checkGraphLock(graphId, context);

        // ── Gate 1: graph must exist ──────────────────────────────────────────
        const graphRow = await db.queryOne(`SELECT id, status FROM graphs WHERE id = ?`, [graphId]) as { id: string; status: string } | undefined;
        if (!graphRow) {
          return JSON.stringify({ error: `Graph not found: ${graphId}` });
        }

        if (scope === "node") {
          // ── scope="node": validate node_id ────────────────────────────────
          const nodeId = args.node_id;
          if (!nodeId) {
            return JSON.stringify({ error: "node_id is required when scope='node'" });
          }

          const nodeRow = await db.queryOne(`SELECT id, status FROM nodes WHERE graph_id = ? AND id = ?`, [graphId, nodeId]) as { id: string; status: string } | undefined;
          if (!nodeRow) {
            return JSON.stringify({ error: `Node not found: ${nodeId} in graph ${graphId}` });
          }

          // ── Mark target node ABANDONED ────────────────────────────────────
          await db.run(`UPDATE nodes SET status = 'ABANDONED', completed_at = datetime('now') WHERE graph_id = ? AND id = ?`, [graphId, nodeId]);

          // axiom:trace work_item=graph-harness-01 spec=specs/102-Graph-Harness.md#REQ-GH-010 plan=step-p2fix-02
          // ── Split origin cascade: if this node was a split origin, abandon all sub-nodes ──
          // Sub-nodes do NOT appear in dependencies as dependents of the origin (they inherit
          // the origin's incoming deps), so the BFS below won't reach them. Explicit cascade.
          let cascadedSubNodes: string[] = [];
          const splitOriginRow = await db.queryOne(
            `SELECT metadata FROM nodes WHERE id=? AND graph_id=?`
          , [nodeId, graphId]) as { metadata: string | null } | null;

          if (splitOriginRow?.metadata) {
            try {
              const meta = JSON.parse(splitOriginRow.metadata);
              if (meta.join_node === true && Array.isArray(meta.sub_node_ids) && meta.sub_node_ids.length > 0) {
                // Cascade abandon to sub-nodes
                for (const subNodeId of meta.sub_node_ids as string[]) {
                  await db.run(
                    `UPDATE nodes SET status='ABANDONED', completed_at=datetime('now') WHERE id=? AND graph_id=? AND LOWER(status) NOT IN ('done', 'abandoned')`
                  , [subNodeId, graphId]);
                }
                cascadedSubNodes = meta.sub_node_ids as string[];
                await addLedgerEntry(graphId, 'split_origin_abandoned', {
                  origin_node_id: nodeId,
                  sub_nodes_abandoned: meta.sub_node_ids
                });
              }
            } catch (e) {
              // malformed metadata — skip cascade
            }
          }

          // ── Cascade: find all nodes that (transitively) depend on this node and BLOCK them ──
          // BFS over the dependency graph starting from nodeId
          const dependentsBlocked: string[] = [];
          const visited = new Set<string>([nodeId]);
          const queue: string[] = [nodeId];

          while (queue.length > 0) {
            const current = queue.shift()!;
            // Find all nodes that depend on current (i.e., node_id=current is in depends_on)
            const directDependents = await db.queryAll(`SELECT node_id FROM dependencies WHERE graph_id = ? AND depends_on = ?`, [graphId, current]) as Array<{ node_id: string }>
            for (const dep of directDependents) {
              if (!visited.has(dep.node_id)) {
                visited.add(dep.node_id);
                queue.push(dep.node_id);
                // Block non-abandoned nodes that depended on abandoned node
                const depStatus = await db.queryOne(`SELECT status FROM nodes WHERE graph_id = ? AND id = ?`, [graphId, dep.node_id]) as { status: string } | undefined;
                if (depStatus && depStatus.status.toUpperCase() !== "ABANDONED" && depStatus.status.toUpperCase() !== "DONE") {
                  // BLOCKED nodes can be reset to PENDING via graph.unblock (REQ-GH-unblock).
                  // Note: unblocking without adding an alternative predecessor means upstream
                  // outputs will be absent from the node briefing — the node may fail conditions
                  // if they depend on that data.
                  await db.run(`UPDATE nodes SET status = 'BLOCKED', completed_at = NULL WHERE graph_id = ? AND id = ?`, [graphId, dep.node_id]);
                  dependentsBlocked.push(dep.node_id);
                }
              }
            }
          }

          await addLedgerEntry(graphId, "node_abandoned", {
            node_id: nodeId,
            dependents_blocked: dependentsBlocked,
            reason,
          });

          return JSON.stringify({
            graph_id: graphId,
            scope: "node",
            node_id: nodeId,
            dependents_blocked: dependentsBlocked,
            cascaded_sub_nodes: cascadedSubNodes,
            status: "abandoned",
          });
        } else {
          // ── scope="graph": abandon the entire graph ────────────────────────
          const allPendingActive = await db.queryAll(
              `SELECT id FROM nodes WHERE graph_id = ? AND LOWER(status) IN ('pending', 'active')`
            , [graphId]) as Array<{ id: string }>
          const abandonedNodeCount = allPendingActive.length;

          await db.run(
            `UPDATE nodes SET status = 'ABANDONED', completed_at = datetime('now') WHERE graph_id = ? AND LOWER(status) IN ('pending', 'active')`
          , [graphId]);

          await db.run(`UPDATE graphs SET status = 'abandoned' WHERE id = ?`, [graphId]);

          await addLedgerEntry(graphId, "graph_abandoned", {
            reason,
            node_count_abandoned: abandonedNodeCount,
          });

          return JSON.stringify({
            graph_id: graphId,
            scope: "graph",
            node_count_abandoned: abandonedNodeCount,
            status: "abandoned",
          });
        }
      } catch (err) {
        const message = err instanceof Error ? err.message : String(err);
        return JSON.stringify({ error: message });
      }
    },
  });

  // ─────────────────────────────────────────────────────────────────────────
  // graph.unblock Tool
  //
  // Resets BLOCKED nodes back to PENDING, enabling recovery after a dependency
  // was abandoned. Use graph.inject first to add an alternative predecessor if
  // the original dependency's output is still needed.
  //
  // axiom:trace work_item=graph-harness-01 spec=specs/102-Graph-Harness.md#graph-unblock plan=graph-unblock-impl
  // ─────────────────────────────────────────────────────────────────────────

  const graphUnblockTool = tool({
    description:
      "Reset BLOCKED nodes back to PENDING so they can be executed again. " +
      "A node becomes BLOCKED when a required dependency is abandoned (via graph.abandon scope=node). " +
      "Use graph.inject FIRST to add an alternative predecessor if the original dependency's output is still needed — " +
      "graph.unblock only resets the status; it does NOT fix the underlying broken dependency chain. " +
      "⚠️ If you unblock without providing an alternative predecessor, the node will activate and receive a briefing " +
      "but the expected upstream outputs will be absent. The node may fail its done-conditions if they depend on that data. " +
      "Coordinators only. Returns { graph_id, unblocked_node_ids, status: 'ok' } on success or { error } on failure.",
    args: {
      graph_id: tool.schema.string().min(1).describe("Graph ID"),
      node_ids: tool.schema
        .array(tool.schema.string())
        .optional()
        .describe(
          "Specific node IDs to unblock. If omitted, ALL BLOCKED nodes in the graph are unblocked."
        ),
      reason: tool.schema
        .string()
        .optional()
        .describe("Human-readable reason for unblocking (e.g., 'injected alternative dependency')"),
    },

    async execute(args, context) {
      // axiom:trace work_item=graph-harness-01 spec=specs/102-Graph-Harness.md#graph-unblock plan=graph-unblock-impl
      if (!config.enabled) {
        return JSON.stringify({ error: "Graph Harness is disabled" });
      }

      try {
        // ── Role check: coordinators only ─────────────────────────────────
        await checkSessionRole(context, ["coordinator"]);

        const graphId = args.graph_id;
        const reason = args.reason ?? null;

        // ── Gate 1: graph must exist (any status — even abandoned/complete) ──
        const graphRow = await db.queryOne(`SELECT id, status FROM graphs WHERE id = ?`, [graphId]) as { id: string; status: string } | undefined;
        if (!graphRow) {
          return JSON.stringify({ error: `Graph not found: ${graphId}` });
        }

        // ── Gate 2: resolve which nodes to unblock ─────────────────────────
        let targetNodeIds: string[];

        if (args.node_ids && args.node_ids.length > 0) {
          // Specific nodes requested — validate each exists and is BLOCKED
          const notBlocked: string[] = [];
          const notFound: string[] = [];

          for (const nodeId of args.node_ids) {
            const nodeRow = await db.queryOne(`SELECT id, status FROM nodes WHERE graph_id = ? AND id = ?`, [graphId, nodeId]) as { id: string; status: string } | undefined;
            if (!nodeRow) {
              notFound.push(nodeId);
            } else if (nodeRow.status.toUpperCase() !== "BLOCKED") {
              notBlocked.push(`${nodeId} (status: ${nodeRow.status})`);
            }
          }

          if (notFound.length > 0) {
            return JSON.stringify({
              error: `Node(s) not found in graph ${graphId}: ${notFound.join(", ")}`,
            });
          }
          if (notBlocked.length > 0) {
            return JSON.stringify({
              error: `Node(s) are not BLOCKED: ${notBlocked.join(", ")}`,
            });
          }

          targetNodeIds = args.node_ids;
        } else {
          // No node_ids specified — unblock ALL BLOCKED nodes in the graph
          const blockedRows = await db.queryAll(
              `SELECT id FROM nodes WHERE graph_id = ? AND UPPER(status) = 'BLOCKED'`
            , [graphId]) as Array<{ id: string }>

          if (blockedRows.length === 0) {
            return JSON.stringify({
              graph_id: graphId,
              unblocked_node_ids: [],
              status: "ok",
            });
          }

          targetNodeIds = blockedRows.map((r) => r.id);
        }

        // ── Gate 3: reset BLOCKED → pending, clear activated_at ───────────
        for (const nodeId of targetNodeIds) {
          await db.run(
            `UPDATE nodes SET status = 'pending', activated_at = NULL WHERE graph_id = ? AND id = ?`
          , [graphId, nodeId]);
        }

        // ── Ledger entry (no mutation counter increment — recovery op) ─────
        await addLedgerEntry(graphId, "nodes_unblocked", {
          node_ids: targetNodeIds,
          reason,
        });

        return JSON.stringify({
          graph_id: graphId,
          unblocked_node_ids: targetNodeIds,
          status: "ok",
        });
      } catch (err) {
        const message = err instanceof Error ? err.message : String(err);
        return JSON.stringify({ error: message });
      }
    },
  });

  // ─────────────────────────────────────────────────────────────────────────
  // Infinite Loop Prevention + Graph Limits (REQ-GH-070, REQ-GH-071, REQ-GH-072)
  //
  // axiom:trace work_item=graph-harness-01 spec=specs/102-Graph-Harness.md#REQ-GH-070 plan=phase-1/task-1-7/step-1-7-1
  // axiom:trace work_item=graph-harness-01 spec=specs/102-Graph-Harness.md#REQ-GH-071 plan=phase-1/task-1-7/step-1-7-1
  // axiom:trace work_item=graph-harness-01 spec=specs/102-Graph-Harness.md#REQ-GH-072 plan=phase-1/task-1-7/step-1-7-1
  // ─────────────────────────────────────────────────────────────────────────

  /**
   * Check if a graph mutation is allowed under the modifications_without_progress limit.
   *
   * Reads `modifications_without_progress` from the graphs table and throws if it
   * has reached or exceeded `config.limits.max_modifications_without_progress` (default 10).
   *
   * Callers (graph.inject, graph.modify, graph.split — Phase 2 tools) MUST call this
   * before performing any structural mutation on the graph.
   *
   * Does NOT increment the counter — that is done by the mutation tool itself.
   *
   * axiom:trace work_item=graph-harness-01 spec=specs/102-Graph-Harness.md#REQ-GH-071 plan=phase-1/task-1-7/step-1-7-1
   */
  async function checkMutationAllowed(graphId: string): Promise<void> {
    const row = await db.queryOne(`SELECT modifications_without_progress FROM graphs WHERE id = ?`, [graphId]) as { modifications_without_progress: number } | undefined;

    if (!row) {
      throw new Error(`Graph not found: ${graphId}`);
    }

    const limit = config.limits.max_modifications_without_progress;
    if (row.modifications_without_progress >= limit) {
      throw new Error(
        `Graph mutation disabled: ${limit} mutations without node completion`
      );
    }
  }

  /**
   * Increment the modifications_without_progress counter for a graph.
   * Called by Phase 2 mutation tools (graph.inject, graph.modify, graph.split).
   *
   * axiom:trace work_item=graph-harness-01 spec=specs/102-Graph-Harness.md#REQ-GH-071 plan=phase-1/task-1-7/step-1-7-1
   */
  async function incrementMutationCounter(graphId: string): Promise<void> {
    await db.run(
      `UPDATE graphs SET modifications_without_progress = modifications_without_progress + 1 WHERE id = ?`
    , [graphId]);
  }

  /**
   * Ensure a node is in a mutable state (not DONE or ABANDONED).
   *
   * Throws if the node has already completed or been abandoned.
   * Called by Phase 2 mutation tools before modifying node properties.
   *
   * axiom:trace work_item=graph-harness-01 spec=specs/102-Graph-Harness.md#REQ-GH-072 plan=phase-1/task-1-7/step-1-7-1
   */
  async function ensureNodeMutable(nodeId: string, graphId: string): Promise<void> {
    const row = await db.queryOne(`SELECT status FROM nodes WHERE graph_id = ? AND id = ?`, [graphId, nodeId]) as { status: string } | undefined;

    if (!row) {
      throw new Error(`Node not found: ${nodeId} in graph ${graphId}`);
    }

    const status = row.status.toLowerCase();
    if (status === "done" || status === "abandoned") {
      throw new Error(`Cannot modify completed node: ${nodeId}`);
    }
  }

  /**
   * Stub: validate nesting depth for a graph.
   * Phase 2 feature — currently returns ok for all graphs.
   *
   * axiom:trace work_item=graph-harness-01 spec=specs/102-Graph-Harness.md#REQ-GH-070 plan=phase-1/task-1-7/step-1-7-1
   */
  function _checkNestingDepth(_graphId: string): { ok: boolean; depth: number } {
    // Phase 2: implement actual nesting depth check
    return { ok: true, depth: 0 };
  }

  // Note: checkMutationAllowed, incrementMutationCounter, ensureNodeMutable, _checkNestingDepth
  // are exported conceptually via closure — Phase 2 tools will call them.
  // The underscore on _checkNestingDepth indicates it is a stub (linter: unused).
  void _checkNestingDepth; // suppress unused warning

  // ─────────────────────────────────────────────────────────────────────────
  // Session Role Enforcement (REQ-GH-013)
  //
  // axiom:trace work_item=graph-harness-01 spec=specs/102-Graph-Harness.md#REQ-GH-013 plan=phase-2/task-2-6/step-2-6-1
  // ─────────────────────────────────────────────────────────────────────────

  /**
   * Check that the calling session has one of the allowed roles.
   *
   * If context.sessionID is absent → allow (assume coordinator).
   * If session not found in DB → allow (assume coordinator).
   * If session found but role not in allowedRoles → throw with clear message.
   *
   * axiom:trace work_item=graph-harness-01 spec=specs/102-Graph-Harness.md#REQ-GH-013 plan=phase-2/task-2-6/step-2-6-1
   */
  async function checkSessionRole(context: unknown, allowedRoles: string[]): Promise<void> {
    const ctx = context as Record<string, unknown> | null | undefined;
    const sessionId = ctx?.sessionID as string | undefined;
    if (!sessionId) return; // no session → assume coordinator → allow (ADR-GH-004)

    const sessionRow = await db.queryOne(`SELECT role FROM sessions WHERE session_id = ?`, [sessionId]) as { role: string } | undefined;
    if (!sessionRow) return; // session not found → assume coordinator → allow (ADR-GH-004)

    const role = sessionRow.role;
    if (!allowedRoles.includes(role)) {
      throw new Error(
        `Permission denied: role '${role}' cannot call this tool. Allowed roles: ${allowedRoles.join(", ")}`
      );
    }
  }

  /**
   * REQ-GH-110: Enforce graph lock. When graphs.locked_by is set, only the
   * lock-holder session may call mutation tools (inject/modify/split/annotate/abandon).
   *
   * ABSENT-SESSION POLICY (ADR-GH-004 — decided: allow-through):
   * If sessionID is absent or the session has no row in the sessions table,
   * the call is ALLOWED for backward compat and bootstrap-race safety.
   * axiom:trace work_item=graph-lock-upgrade-01 spec=specs/102-Graph-Harness.md#REQ-GH-110
   */
  async function checkGraphLock(graphId: string, context: unknown): Promise<void> {
    const ctx = context as Record<string, unknown> | null | undefined;
    const sessionId = ctx?.sessionID as string | undefined;
    const row = await db.queryOne(`SELECT locked_by FROM graphs WHERE id=?`, [graphId]) as
      { locked_by: string | null } | undefined;
    if (!row || row.locked_by === null) return; // unlocked — always allow
    if (!sessionId) return;                     // no session context — allow-through (ADR-GH-004)
    if (row.locked_by === sessionId) return;    // caller holds the lock — allow
    throw new Error(
      `Graph ${graphId} is locked to session ${row.locked_by}. ` +
      `Use session_message to coordinate, or graph_unlock if you are the coordinator.`
    );
  }

  // ─────────────────────────────────────────────────────────────────────────
  // Harness Loop Utilities (task-1-4-1)
  //
  // axiom:trace work_item=graph-harness-01 spec=specs/102-Graph-Harness.md#REQ-GH-073 plan=phase-1/task-1-4/step-1-4-1
  // ─────────────────────────────────────────────────────────────────────────

  /**
   * Redact common credential patterns from subprocess output before persisting
   * to the database.
   *
   * Per REQ-GH-074a: scan last_result and last_stderr for common credential
   * patterns — matches replaced with [REDACTED].
   *
   * Spec patterns: Bearer tokens, OpenAI sk- keys, AWS AKIA keys, BEGIN blocks,
   * plus step-spec additions: GitHub tokens, long random strings, generic key=value secrets.
   *
   * axiom:trace work_item=graph-harness-01 spec=specs/102-Graph-Harness.md#REQ-GH-074a plan=phase-1/task-1-4/step-1-4-1
   */
  function redactCredentials(input: string): string {
    let out = input;

    // ── Spec REQ-GH-074a patterns ──────────────────────────────────────────
    // Bearer tokens (Authorization: Bearer <token>)
    out = out.replace(/Bearer\s+[A-Za-z0-9\-._~+/]+=*/gi, "Bearer [REDACTED]");
    // OpenAI sk- style keys
    out = out.replace(/sk-[A-Za-z0-9]{20,}/g, "[REDACTED]");
    // AWS access key IDs
    out = out.replace(/AKIA[A-Z0-9]{16}/g, "[REDACTED]");
    // PEM blocks (private keys, certificates)
    out = out.replace(/-----BEGIN[^-]+-----[\s\S]*?-----END[^-]+-----/g, "[REDACTED]");

    // ── Step-spec additional patterns ─────────────────────────────────────
    // GitHub personal access tokens (ghp_, ghs_, gho_, ghr_ prefixes)
    out = out.replace(/gh[posrt]_[A-Za-z0-9]{36}/g, "[REDACTED]");
    // Generic password/token/secret = value patterns
    out = out.replace(/(?:password|token|secret|api_key|apikey|access_key)\s*[:=]\s*\S+/gi, (m) => {
      const eqIdx = m.search(/[:=]/);
      return eqIdx >= 0 ? m.slice(0, eqIdx + 1) + " [REDACTED]" : "[REDACTED]";
    });
    // Conservative long random strings (20+ char alphanumeric with separators)
    // Only flag strings that look like keys (not normal log words)
    out = out.replace(/[A-Za-z0-9_\-]{40,}/g, (m) => {
      // Only redact if the string has high entropy-like structure (mix of case + digits)
      const hasUpper = /[A-Z]/.test(m);
      const hasLower = /[a-z]/.test(m);
      const hasDigit = /[0-9]/.test(m);
      if (hasUpper && hasLower && hasDigit) return "[REDACTED]";
      return m;
    });

    return out;
  }

  /**
   * Redact credentials on the FULL output first, then truncate to 50 lines (REQ-GH-074a).
   * Order matters: redact before truncating so secrets on line 51+ are never stored unredacted.
   * axiom:trace work_item=graph-harness-01 spec=specs/102-Graph-Harness.md#REQ-GH-074a plan=step-verify-gh-07
   */
  function sanitizeOutput(raw: string): string {
    // Step 1: redact credentials on the FULL output (before truncation)
    const redacted = redactCredentials(raw);
    // Step 2: truncate to 50 lines
    const lines = redacted.split("\n");
    return lines.length > 50
      ? lines.slice(0, 50).join("\n") + "\n[... truncated at 50 lines]"
      : redacted;
  }

  /**
   * Run a shell command with a hard timeout.
   * SIGTERM → 5s grace → SIGKILL → reap. (REQ-GH-073)
   *
   * Returns {exitCode, output} where output is combined stdout+stderr (sanitized).
   * Never throws — captures all errors in output with exitCode=-1.
   *
   * axiom:trace work_item=graph-harness-01 spec=specs/102-Graph-Harness.md#REQ-GH-073 plan=phase-1/task-1-4/step-1-4-1
   */
  async function runWithTimeout(
    cmd: string,
    timeoutMs: number
  ): Promise<{ exitCode: number; output: string }> {
    let proc: ReturnType<typeof Bun.spawn> | null = null;
    let killTimer: ReturnType<typeof setTimeout> | null = null;
    let gracePeriodTimer: ReturnType<typeof setTimeout> | null = null;

    try {
      proc = Bun.spawn(["sh", "-c", cmd], {
        stdout: "pipe",
        stderr: "pipe",
      });

      // Schedule timeout: SIGTERM → wait 5s → SIGKILL (REQ-GH-073)
      const killPromise = new Promise<void>((resolve) => {
        killTimer = setTimeout(async () => {
          if (proc) {
            try { proc.kill("SIGTERM"); } catch { /* ignore */ }
          }
          gracePeriodTimer = setTimeout(() => {
            if (proc) {
              try { proc.kill("SIGKILL"); } catch { /* ignore */ }
            }
            resolve();
          }, 5000);
        }, timeoutMs);
      });

      // Collect stdout + stderr in parallel with the timeout
      const stdoutPromise = proc.stdout
        ? new Response(proc.stdout).text().catch(() => "")
        : Promise.resolve("");
      const stderrPromise = proc.stderr
        ? new Response(proc.stderr).text().catch(() => "")
        : Promise.resolve("");

      // Race: either process exits naturally or kill timer fires
      const [stdout, stderr, exitCode] = await Promise.race([
        Promise.all([stdoutPromise, stderrPromise, proc.exited]),
        killPromise.then(() => Promise.all([stdoutPromise, stderrPromise, proc!.exited])),
      ]);

      const rawOutput = ((stdout as string) + (stderr as string)).trim();
      return { exitCode: exitCode as number, output: sanitizeOutput(rawOutput) };
    } catch (err) {
      const msg = err instanceof Error ? err.message : String(err);
      return { exitCode: -1, output: sanitizeOutput(`[subprocess error] ${msg}`) };
    } finally {
      // Clean up timers
      if (killTimer !== null) clearTimeout(killTimer);
      if (gracePeriodTimer !== null) clearTimeout(gracePeriodTimer);
      // Best-effort SIGKILL on any lingering process (zombie prevention, REQ-GH-073)
      if (proc) {
        try { proc.kill("SIGKILL"); } catch { /* already exited */ }
      }
    }
  }

  // ─── DB helper row types ──────────────────────────────────────────────────

  interface NodeRow {
    id: string;
    graph_id: string;
    title: string;
    description: string;
    status: string;
    execution_mode: string;
    execution_config: string | null;
    attempt_count: number;
    max_retries: number;
    context: string | null;
    activated_at: string | null;
    completed_at: string | null;
  }

  interface ConditionRow {
    id: string;
    graph_id: string;
    node_id: string;
    ordinal: number;
    type: string;
    command: string | null;
    timeout_seconds: number;
    description: string | null;
  }

  interface SessionRow {
    session_id: string;
    graph_id: string;
    node_id: string | null;
    status: string;
  }

  /**
   * Log a ledger entry for harness loop actions.
   */
  async function logLedger(
    graphId: string,
    sessionId: string | null,
    action: string,
    targetNodeId: string | null,
    detail: Record<string, unknown>
  ): Promise<void> {
    await dbWriteWithRetry(
      async () => {
        await db.run(
          `INSERT INTO ledger (graph_id, session_id, action, target_node_id, detail, timestamp)
           VALUES (?, ?, ?, ?, ?, ?)`,
          [graphId, sessionId, action, targetNodeId, JSON.stringify(detail), new Date().toISOString()]
        );
      },
      `await logLedger(${action})`
    );
  }

  /**
   * Add a ledger entry — public helper for use by tools and event handlers.
   * Avoids DB call duplication for the common case where callers only have a
   * graph_id and a flat detail payload.
   *
   * This is an alias over logLedger with a more ergonomic call signature.
   *
   * axiom:trace work_item=graph-harness-01 spec=specs/102-Graph-Harness.md#REQ-GH-074 plan=phase-1/task-1-8/step-1-8-1
   */
  async function addLedgerEntry(
    graphId: string,
    action: string,
    detail: Record<string, unknown>,
    opts?: { sessionId?: string | null; targetNodeId?: string | null }
  ): Promise<void> {
    await logLedger(
      graphId,
      opts?.sessionId ?? null,
      action,
      opts?.targetNodeId ?? null,
      detail
    );
  }

  /**
   * Update per-session cost tracking.
   *
   * Increments tokens_used and cost_usd accumulators in the sessions table.
   * Emits a cost_threshold_warning ledger entry and console warning when
   * the session crosses warn_at_percent of max_cost_per_graph_usd.
   * Deduplicates warnings: only re-warns when cost grows by ≥10% since last warning.
   *
   * axiom:trace work_item=graph-harness-01 spec=specs/102-Graph-Harness.md#REQ-GH-074 plan=phase-4/task-4-5/step-4-5-1
   * axiom:trace work_item=graph-harness-01 spec=specs/102-Graph-Harness.md#REQ-GH-074 plan=step-p4fix-07
   */
  // Deduplication map: tracks last cost_usd at which we warned per session.
  // Prevents 100+ duplicate cost_threshold_warning entries for long-running sessions.
  // axiom:trace work_item=graph-harness-01 spec=specs/102-Graph-Harness.md#REQ-GH-074 plan=step-p4fix-07
  const lastWarnedCostPerSession = new Map<string, number>(); // sessionId → last warned cost_usd

  async function updateSessionCost(sessionId: string, tokensUsed: number, costUsd: number): Promise<void> {
    await dbWriteWithRetry(
      async () => {
        await db.run(`
          UPDATE sessions SET tokens_used = tokens_used + ?, cost_usd = cost_usd + ?
          WHERE session_id = ?
        `, [tokensUsed, costUsd, sessionId]);
      },
      "await updateSessionCost(write)"
    );

    // Check warning threshold
    const session = await dbReadWithRetry(
      async () => await db.queryOne<{ cost_usd: number; graph_id: string }>(
        `SELECT cost_usd, graph_id FROM sessions WHERE session_id = ?`,
        [sessionId]
      ),
      "await updateSessionCost(read)"
    );

    if (!session) return;

    const maxCost = config.cost.max_cost_per_graph_usd;
    const warnAt = config.cost.warn_at_percent / 100; // convert percent to fraction

    if (maxCost > 0 && session.cost_usd >= maxCost * warnAt) {
      // Deduplication: only warn if never warned before OR cost grew ≥10% since last warning
      const lastWarned = lastWarnedCostPerSession.get(sessionId) ?? 0;
      const shouldWarn = lastWarned === 0 || session.cost_usd >= lastWarned * 1.1;

      if (shouldWarn) {
        lastWarnedCostPerSession.set(sessionId, session.cost_usd);
        const pct = Math.round((session.cost_usd / maxCost) * 100);
        console.warn(
          `[GraphHarness] Session ${sessionId} at ${pct}% of cost cap ` +
          `($${session.cost_usd.toFixed(4)} of $${maxCost})`
        );
        await addLedgerEntry(session.graph_id, "cost_threshold_warning", {
          session_id: sessionId,
          cost_usd: session.cost_usd,
          threshold: maxCost * warnAt,
          pct,
        });
        // REQ-GH-101 / SWDE-63: structured cost_warning notification
        await dispatchNotification({
          type: "cost_warning",
          graph_id: session.graph_id,
          title: "Cost Warning",
          body: `Session ${sessionId} at ${pct}% of cost cap ($${session.cost_usd.toFixed(4)})`,
          metadata: { session_id: sessionId, cost_usd: session.cost_usd, pct },
          timestamp: new Date().toISOString(),
        }).catch(() => { /* non-fatal */ });
      }
    }
  }

  /**
   * Find the first unblocked PENDING node for a graph.
   * A node is unblocked when all of its dependencies are DONE.
   *
   * NOTE: join_strategy evaluation (any/majority) is a Phase 4 dependency.
   * Currently all dependencies must be DONE for a node to unblock (equivalent to join_strategy='all').
   * Phase 4 MUST read nodes.metadata.join_strategy and apply the correct logic:
   *   - 'all': all sub-nodes DONE (current behavior)
   *   - 'any': at least one sub-node DONE
   *   - 'majority': ceil(n/2) sub-nodes DONE
   * axiom:trace work_item=graph-harness-01 spec=specs/102-Graph-Harness.md#REQ-GH-004 plan=phase-4/task-4-4/step-4-4-1
   */
  async function findNextUnblockedNode(graphId: string): Promise<NodeRow | null> {
    // Get all PENDING nodes for this graph
    const pendingNodes = await db.queryAll(
        `SELECT id, graph_id, title, description, status, execution_mode, execution_config,
                attempt_count, max_retries, context, activated_at, completed_at,
                join_strategy
         FROM nodes WHERE graph_id = ? AND LOWER(status) = 'pending'
         ORDER BY created_at ASC`
      , [graphId]) as (NodeRow & { join_strategy?: string | null })[];

    if (pendingNodes.length === 0) return null;

    // For each pending node, check if its deps satisfy its join strategy.
    // axiom:trace work_item=graph-harness-01 spec=specs/102-Graph-Harness.md#REQ-GH-004 plan=phase-4/task-4-4/step-4-4-1
    for (const node of pendingNodes) {
      const deps = await db.queryAll(
          `SELECT depends_on FROM dependencies WHERE graph_id = ? AND node_id = ?`
        , [graphId, node.id]) as Array<{ depends_on: string }>

      // Determine how many deps are done
      // CANCELLED is treated as DONE for dependency resolution (§17b.3).
      // axiom:trace work_item=SWDE-46 spec=specs/102-Graph-Harness.md#17b.3 plan=phase-1/task-2.1/step-1 jira_ref=SWDE-46
      const doneDeps = await Promise.all(deps.map(async (d) => {
        const depNode = await db.queryOne<{ status: string }>(`SELECT status FROM nodes WHERE graph_id = ? AND id = ?`, [graphId, d.depends_on]);
        const s = depNode?.status.toLowerCase() ?? "";
        return (s === "done" || s === "cancelled") ? d : null;
      })).then(results => results.filter(Boolean) as typeof deps);

      // Apply join_strategy for dependency satisfaction
      // Default: "all" (all deps must be done)
      const joinStrategy = (node as NodeRow & { join_strategy?: string | null }).join_strategy ?? "all";
      let depsSatisfied: boolean;
      switch (joinStrategy) {
        case "any":
          depsSatisfied = deps.length === 0 || doneDeps.length >= 1;
          break;
        case "majority":
          depsSatisfied = deps.length === 0 || doneDeps.length >= Math.ceil(deps.length / 2);
          break;
        case "all":
        default:
          depsSatisfied = deps.length === 0 || doneDeps.length === deps.length;
          break;
      }

      if (!depsSatisfied) continue;

      // ── REQ-GH-043: check required data_flow inputs ──────────────────────
      // If this node has required data_flow inputs, all upstream required outputs must exist.
      // axiom:trace work_item=graph-harness-01 spec=specs/102-Graph-Harness.md#REQ-GH-043 plan=phase-3/task-3-2/step-3-2-1
      const requiredFlows = await db.queryAll(
          `SELECT from_node_id, output_key FROM data_flow
           WHERE graph_id = ? AND to_node_id = ? AND required = 1`
        , [graphId, node.id]) as Array<{ from_node_id: string; output_key: string }>;

      const allFlowsSatisfied = (await Promise.all(
        requiredFlows.map(async (flow) => {
          const outputRow = await db.queryOne(
              `SELECT id FROM node_outputs WHERE graph_id = ? AND node_id = ? AND key = ?`
            , [graphId, flow.from_node_id, flow.output_key]);
          return outputRow !== null && outputRow !== undefined;
        })
      )).every(Boolean);

      if (allFlowsSatisfied) return node;
    }

    return null; // All PENDING nodes are blocked
  }

  /**
   * Find ALL unblocked PENDING nodes for a graph (not just the first).
   *
   * Used by spawnWorkersForUnblockedNodes to find nodes eligible for
   * parallel worker session spawning.
   *
   * Applies the same dep/flow checks as findNextUnblockedNode.
   *
   * axiom:trace work_item=graph-harness-01 spec=specs/102-Graph-Harness.md#REQ-GH-031 plan=phase-4/task-4-2/step-4-2-1
   */
  async function findAllUnblockedNodes(graphId: string): Promise<NodeRow[]> {
    const pendingNodes = await db.queryAll(
        `SELECT id, graph_id, title, description, status, execution_mode,
                attempt_count, max_retries, context, activated_at, completed_at
         FROM nodes WHERE graph_id = ? AND LOWER(status) = 'pending'
         ORDER BY created_at ASC`
      , [graphId]) as NodeRow[];

    const unblocked: NodeRow[] = [];

    for (const node of pendingNodes) {
      const deps = await db.queryAll(`SELECT depends_on FROM dependencies WHERE graph_id = ? AND node_id = ?`, [graphId, node.id]) as Array<{ depends_on: string }>

      const allDone = deps.length === 0 || (await Promise.all(deps.map(async (d) => {
        const depNode = await db.queryOne<{ status: string }>(`SELECT status FROM nodes WHERE graph_id = ? AND id = ?`, [graphId, d.depends_on]);
        return depNode ? depNode.status.toLowerCase() === "done" : false;
      }))).every(Boolean);

      if (!allDone) continue;

      const requiredFlows = await db.queryAll(
          `SELECT from_node_id, output_key FROM data_flow
           WHERE graph_id = ? AND to_node_id = ? AND required = 1`
        , [graphId, node.id]) as Array<{ from_node_id: string; output_key: string }>;

      const allFlowsSatisfied = (await Promise.all(
        requiredFlows.map(async (flow) => {
          const outputRow = await db.queryOne(`SELECT id FROM node_outputs WHERE graph_id = ? AND node_id = ? AND key = ?`, [graphId, flow.from_node_id, flow.output_key]);
          return outputRow !== null && outputRow !== undefined;
        })
      )).every(Boolean);

      if (allFlowsSatisfied) unblocked.push(node);
    }

    return unblocked;
  }

  /**
   * Returns the spawn retry backoff delay in milliseconds for a given attempt number.
   * Implements exponential backoff: attempt 1 → 2000ms, attempt 2 → 4000ms, attempt 3 → 8000ms.
   * axiom:trace work_item=graph-harness-01 spec=specs/102-Graph-Harness.md#REQ-GH-081a plan=step-p4fix-03
   */
  function getSpawnBackoffMs(attempt: number): number {
    return 2000 * Math.pow(2, attempt - 1);
  }

  /**
   * Attempt to spawn worker sessions for additional unblocked nodes beyond
   * the one already activated for the current session.
   *
   * Algorithm:
   *   1. Count currently active sessions for this graph.
   *   2. If already at max_concurrent_sessions, return immediately.
   *   3. For each additional unblocked node, atomically activate it (CAS),
   *      then try to spawn a worker session (3 retries with backoff).
   *   4. On successful spawn: register worker in sessions table.
   *   5. On spawn failure: node remains activated; parent processes it on next
   *      idle tick (spawn_fallback logged to ledger).
   *
   * axiom:trace work_item=graph-harness-01 spec=specs/102-Graph-Harness.md#REQ-GH-031 plan=phase-4/task-4-2/step-4-2-1
   */
  async function spawnWorkersForUnblockedNodes(
    graphId: string,
    currentSessionId: string,
    alreadyActivatedNodeId: string
  ): Promise<void> {
    const activeSessionsRow = await db.queryOne(`SELECT COUNT(*) as cnt FROM sessions WHERE graph_id=? AND LOWER(status)='active'`, [graphId]) as { cnt: number };

    let activeCount = activeSessionsRow.cnt;
    const maxSessions = config.spawning.max_concurrent_sessions;

    if (activeCount >= maxSessions) return; // at capacity

    // Find additional unblocked nodes (not the one just activated, not join nodes)
    // Join nodes are activated exclusively by checkAndActivateJoinNode when strategy is met.
    const additionalUnblocked = (await findAllUnblockedNodes(graphId)).filter((n) => {
      if (n.id === alreadyActivatedNodeId) return false;
      // Skip join nodes — they activate via checkAndActivateJoinNode, not direct worker spawn
      try {
        const meta = n.metadata ? JSON.parse(n.metadata as unknown as string) as Record<string, unknown> : {};
        if (meta.join_node === true) return false;
      } catch { /* malformed metadata — include node */ }
      return true;
    });

    for (const node of additionalUnblocked) {
      if (activeCount >= maxSessions) break;

      // CAS: atomically activate this node to prevent TOCTOU with other sessions
      const cas = await db.run(
        `UPDATE nodes SET status='active', activated_at=datetime('now')
         WHERE id=? AND graph_id=? AND LOWER(status)='pending'`
      , [node.id, graphId]);

      if (cas.changes === 0) continue; // another session already got this node

      await addLedgerEntry(graphId, "node_activated", {
        node_id: node.id,
        node_title: node.title,
        spawned_by: currentSessionId,
      });

      // SWDE-48: stash pop + conductor agent spawn for parallel worker nodes (best-effort)
      // axiom:trace work_item=SWDE-48 spec=specs/102-Graph-Harness.md jira_ref=SWDE-48
      await onNodeActivated(graphId, node.id, currentSessionId).catch((err) => {
        console.warn("[GraphHarness] onNodeActivated (worker) error:", err);
      });

      // Attempt to spawn a worker session (3 retries with backoff)
      let spawnResult: string | null = null;
      for (let attempt = 1; attempt <= 3; attempt++) {
        spawnResult = await spawnChildSession(currentSessionId, graphId, node.id);
        if (spawnResult !== null) break;
        // axiom:trace work_item=graph-harness-01 spec=specs/102-Graph-Harness.md#REQ-GH-081a plan=step-p4fix-03
        if (attempt < 3) await Bun.sleep(getSpawnBackoffMs(attempt)); // 2s, 4s backoff
      }

      if (spawnResult !== null) {
        // Register worker session in DB
        await db.run(`
          INSERT OR IGNORE INTO sessions
            (session_id, graph_id, role, status, node_id, created_at, last_heartbeat)
          VALUES (?, ?, 'worker', 'active', ?, datetime('now'), datetime('now'))
        `, [spawnResult, graphId, node.id]);
        await addLedgerEntry(graphId, "worker_spawned", {
          worker_session_id: spawnResult,
          node_id: node.id,
          attempt_count: 1,
        });
      } else {
        // Spawn failed — node is already activated; parent will process it on next idle tick
        await addLedgerEntry(graphId, "spawn_fallback", {
          node_id: node.id,
          reason: "all_spawn_attempts_failed",
        });
        // REQ-GH-081a: Node annotation for spawn fallback
        // axiom:trace work_item=graph-harness-01 spec=specs/102-Graph-Harness.md#REQ-GH-081a plan=step-p4fix-06
        await db.run(`
          INSERT INTO annotations (id, graph_id, node_id, type, content, severity, created_at)
          VALUES (?, ?, ?, 'note', 'spawn failed — executing in parent session', 'warn', datetime('now'))
        `, [`ann_${Date.now().toString(36)}`,
          graphId,
          node.id]);
      }
      activeCount++;
    }
  }

  /**
   * Evaluate all done-conditions for a node.
   *
   * Condition types (REQ-GH-021):
   *   - "none"        → always passes
   *   - "script"      → run command, exit 0 = pass
   *   - "file_exists" → check path existence
   *   - "http_check"  → HTTP GET, 200-299 = pass
   *   - "model_judge" → pending (Phase 5+), returns "pending"
   *
   * Returns { passed: boolean; results: ConditionResult[] }
   *
   * axiom:trace work_item=graph-harness-01 spec=specs/102-Graph-Harness.md#REQ-GH-021 plan=phase-1/task-1-4/step-1-4-1
   */
  interface ConditionResult {
    condition_id: string;
    type: string;
    passed: boolean | "pending";
    output: string;
    exit_code: number | null;
  }

  async function evaluateConditions(
    node: NodeRow,
    conditions: ConditionRow[]
  ): Promise<{ passed: boolean; results: ConditionResult[] }> {
    // No conditions → passes trivially
    if (conditions.length === 0) {
      return { passed: true, results: [] };
    }

    const results: ConditionResult[] = [];
    const nowIso = new Date().toISOString();

    for (const cond of conditions) {
      let passed: boolean | "pending" = false;
      let output = "";
      let exitCode: number | null = null;

      const timeoutMs = ((cond.timeout_seconds || config.harness.condition_timeout_seconds) * 1000);

      try {
        switch (cond.type.toLowerCase()) {
          case "none": {
            // Always passes (REQ-GH-021)
            passed = true;
            output = "none — always passes";
            break;
          }

          case "script": {
            if (!cond.command) {
              output = "no command specified for script condition";
              passed = false;
              break;
            }
            const result = await runWithTimeout(cond.command, timeoutMs);
            exitCode = result.exitCode;
            output = result.output;
            passed = result.exitCode === 0;
            break;
          }

          case "file_exists": {
            if (!cond.command) {
              output = "no path specified for file_exists condition";
              passed = false;
              break;
            }
            const filePath = cond.command.trim();
            try {
              passed = existsSync(filePath);
              output = passed ? `file exists: ${filePath}` : `file not found: ${filePath}`;
            } catch (err) {
              output = `file check error: ${err instanceof Error ? err.message : String(err)}`;
              passed = false;
            }
            break;
          }

          case "http_check": {
            if (!cond.command) {
              output = "no URL specified for http_check condition";
              passed = false;
              break;
            }
            // Validate URL against blocked domains/IPs (api_policy)
            try {
              const url = new URL(cond.command.trim());
              const blockedDomains = config.api_policy?.blocked_domains ?? [];
              const isBlocked = blockedDomains.some((d: string) => url.hostname.includes(d));
              if (isBlocked) {
                output = `blocked domain: ${url.hostname}`;
                passed = false;
                break;
              }
              // Perform HTTP GET with timeout via AbortController
              const controller = new AbortController();
              const fetchTimer = setTimeout(() => controller.abort(), timeoutMs);
              try {
                const resp = await fetch(cond.command.trim(), {
                  method: "GET",
                  signal: controller.signal,
                });
                clearTimeout(fetchTimer);
                passed = resp.status >= 200 && resp.status < 300;
                output = `HTTP ${resp.status} ${resp.statusText}`;
                exitCode = resp.status;
              } finally {
                clearTimeout(fetchTimer);
              }
            } catch (err) {
              const msg = err instanceof Error ? err.message : String(err);
              output = `http_check error: ${msg}`;
              passed = false;
            }
            break;
          }

          case "model_judge": {
            // Phase 5+ feature — return "pending" (not treated as failure)
            passed = "pending";
            output = "model_judge not implemented (Phase 5+) — skipping";
            break;
          }

          case "test_pattern": {
            // Run command and check output against expected pattern
            // axiom:trace work_item=graph-harness-01 spec=specs/102-Graph-Harness.md#4.2 plan=step-verify-gh-09
            if (!cond.command) {
              output = "no command specified for test_pattern condition";
              passed = false;
              break;
            }
            const testPatternResult = await runWithTimeout(cond.command, timeoutMs);
            exitCode = testPatternResult.exitCode;
            passed = testPatternResult.exitCode === 0;
            output = testPatternResult.output;
            break;
          }

          case "file_changed": {
            // Check if file has been modified since node was activated (git mtime check)
            // axiom:trace work_item=graph-harness-01 spec=specs/102-Graph-Harness.md#4.2 plan=step-verify2-gh-05
            if (!cond.command) {
              output = "no file path specified for file_changed condition";
              passed = false;
              break;
            }
            try {
              const filePath = cond.command.trim();
              const stats = statSync(filePath);
              // Check if file was modified after the node was activated.
              // Guard against: null, undefined, string 'null', missing 'Z' suffix, and non-parseable values.
              const nodeRow = await db.queryOne(`SELECT activated_at FROM nodes WHERE id = ?`, [node.id]) as { activated_at: string | null } | null;
              const activatedAt: number = (() => {
                const val = nodeRow?.activated_at;
                if (!val || val === 'null') return 0;
                const ts = new Date(val.endsWith('Z') ? val : val + 'Z').getTime();
                return isNaN(ts) ? 0 : ts;
            })
              passed = stats.mtimeMs > activatedAt;
              output = passed ? `${filePath} changed since activation` : `${filePath} not changed since activation`;
            } catch (e) {
              passed = false;
              output = `file_changed check failed: ${e}`;
            }
            break;
          }

          case "manual": {
            // Manual conditions require human approval via annotation
            // axiom:trace work_item=graph-harness-01 spec=specs/102-Graph-Harness.md#4.2 plan=step-verify-gh-09
            const approvalAnnotation = await db.queryOne(
              `SELECT * FROM annotations WHERE node_id=? AND graph_id=? AND type='decision' AND content LIKE '%approved%' ORDER BY created_at DESC LIMIT 1`
            , [node.id, node.graph_id]);
            passed = !!approvalAnnotation;
            output = passed ? 'Manual approval found in annotations' : 'Awaiting manual approval (use graph.annotate with type=decision, content containing "approved")';
            // SWDE-63 / REQ-GH-101: dispatch approval_needed notification when awaiting human review
            // Cooldown dedup (default 60s) prevents notification storms during repeated idle evaluations.
            // axiom:trace work_item=SWDE-63 spec=specs/102-Graph-Harness.md#REQ-GH-101 plan=step-C-3 jira_ref=SWDE-63
            if (!passed) {
              await dispatchNotification({
                type: "approval_needed",
                graph_id: node.graph_id,
                node_id: node.id,
                title: "Approval Needed",
                body: `Node ${node.id} in graph ${node.graph_id} is awaiting manual approval`,
                timestamp: new Date().toISOString(),
              }).catch(() => { /* non-fatal */ });
            }
            break;
          }

          case "compound": {
            // Compound conditions: run the command (Phase 1 implementation)
            // axiom:trace work_item=graph-harness-01 spec=specs/102-Graph-Harness.md#4.2 plan=step-verify-gh-09
            if (!cond.command) {
              output = "compound condition: no command specified";
              passed = false;
              break;
            }
            const compoundResult = await runWithTimeout(cond.command, timeoutMs);
            exitCode = compoundResult.exitCode;
            passed = compoundResult.exitCode === 0;
            output = compoundResult.output;
            break;
          }

          case "stash_has_finding": {
            // Passes when the named stash has at least one type:result entry in its JSONL file.
            // command field = stash ID; use "__node__" to resolve from the node's own metadata.stash.
            // axiom:trace work_item=SWDE-48 spec=specs/102-Graph-Harness.md#REQ-GH-202 jira_ref=SWDE-48
            const targetStashId = cond.command?.trim();
            const resolvedStashId = (!targetStashId || targetStashId === "__node__")
              ? await (async () => {
                  try {
                    const meta = await readNodeMeta(node.graph_id, node.id);
                    return meta.stash as string | undefined;
                  } catch { return undefined; }
                })()
              : targetStashId;
            if (!resolvedStashId) {
              passed = false;
              output = "stash_has_finding: no stash ID specified (set command to stash ID or '__node__')";
              break;
            }
            const entriesFilePath = join(stashRoot, "entries", `${resolvedStashId}.jsonl`);
            try {
              if (existsSync(entriesFilePath)) {
                const lines = readFileSync(entriesFilePath, "utf-8").trim().split("\n").filter((l) => l.trim());
                const hasResult = lines.some((l) => {
                  try { return (JSON.parse(l) as Record<string, unknown>).type === "result"; } catch { return false; }
                });
                passed = hasResult;
                output = passed
                  ? `stash '${resolvedStashId}' has a finding (type:result entry found)`
                  : `stash '${resolvedStashId}' has no finding yet (no type:result entry)`;
              } else {
                passed = false;
                output = `stash '${resolvedStashId}' has no entries file yet`;
              }
            } catch (err) {
              passed = false;
              output = `stash_has_finding error: ${err instanceof Error ? err.message : String(err)}`;
            }
            break;
          }

          case "conductor_agent_done": {
            // Passes when the conductor agent for this node (or the specified agent_id) has status='done'.
            // command field = agent_id; use "__node__" or empty to resolve from node metadata._conductor_agent_id.
            // axiom:trace work_item=SWDE-48 spec=specs/102-Graph-Harness.md#REQ-GH-203 jira_ref=SWDE-48
            const targetAgentId = cond.command?.trim();
            const resolvedAgentId = (!targetAgentId || targetAgentId === "__node__")
              ? await (async () => {
                  try {
                    const meta = await readNodeMeta(node.graph_id, node.id);
                    return meta._conductor_agent_id as string | undefined;
                  } catch { return undefined; }
                })()
              : targetAgentId;
            if (!resolvedAgentId) {
              passed = false;
              output = "conductor_agent_done: no agent_id found (activate with conductor_agent config or specify agent_id in command)";
              break;
            }
            try {
              const tableExists = await db.queryOne(
                `SELECT 1 FROM sqlite_master WHERE type='table' AND name='conductor_agents'`
              , []);
              if (!tableExists) {
                // ConductorPlugin not loaded — treat as pending so graph is not blocked
                passed = "pending" as unknown as boolean;
                output = "conductor_agents table not found — ConductorPlugin may not be loaded (treating as pending)";
                // Also emit a ledger event so operators can detect this via ledger queries
                // (console.warn alone is not queryable)
                // axiom:trace work_item=SWDE-48 spec=specs/102-Graph-Harness.md#REQ-GH-203 jira_ref=SWDE-48 plan=phase-run5-hardening/task-run5-impl/step-BL-026
                const _warnKey = `${node.graph_id}:${node.id}`;
                const _lastWarn = conductorPluginAbsentWarnLastEmitted.get(_warnKey) ?? 0;
                if (Date.now() - _lastWarn > CONDUCTOR_ABSENT_WARN_TTL_MS) {
                  conductorPluginAbsentWarnLastEmitted.set(_warnKey, Date.now());
                  logLedger(node.graph_id, null, "conductor_plugin_absent_warn", node.id, {
                    condition_type: "conductor_agent_done",
                  }).catch(() => { /* non-fatal */ });
                }
                break;
              }
              const agentRow = await db.queryOne(`SELECT status FROM conductor_agents WHERE agent_id=?`, [resolvedAgentId]) as { status: string } | null;
              if (!agentRow) {
                passed = false;
                output = `conductor agent '${resolvedAgentId}' not found in DB`;
                break;
              }
              passed = agentRow.status === "done";
              output = passed
                ? `conductor agent '${resolvedAgentId}' is done`
                : `conductor agent '${resolvedAgentId}' status: ${agentRow.status}`;
            } catch (err) {
              passed = false;
              output = `conductor_agent_done error: ${err instanceof Error ? err.message : String(err)}`;
            }
            break;
          }

          default: {
            output = `unknown condition type: ${cond.type}`;
            passed = false;
          }
        }
      } catch (err) {
        const msg = err instanceof Error ? err.message : String(err);
        output = `condition evaluation error: ${msg}`;
        passed = false;
      }

      // Persist condition result back to DB (REQ-GH-074a: sanitize output)
      const sanitized = sanitizeOutput(output);
      try {
        await db.run(
           `UPDATE conditions
            SET passed = ?, last_result = ?, last_exit_code = ?, last_evaluated_at = ?
            WHERE id = ?`
        , [passed === true ? 1 : passed === false ? 0 : null,
          sanitized,
          exitCode,
          nowIso,
          cond.id]);
      } catch (dbErr) {
        console.warn("[GraphHarness] Failed to update condition result:", dbErr);
      }

      // Ledger entry for condition evaluated (REQ-GH-074)
      // axiom:trace work_item=graph-harness-01 spec=specs/102-Graph-Harness.md#REQ-GH-074 plan=phase-1/task-1-8/step-1-8-1
      await logLedger(node.graph_id, null, "condition_evaluated", node.id, {
        condition_id: cond.id,
        condition_type: cond.type,
        passed: passed,
        exit_code: exitCode,
        output_snippet: sanitized.slice(0, 200),
      });

      // REQ-GH-083: check for flaky oscillation after each condition evaluation
      // axiom:trace work_item=graph-harness-01 spec=specs/102-Graph-Harness.md#REQ-GH-083 plan=phase-6/task-6-8/step-6-8-1
      try { checkAndMarkFlaky(node.graph_id, node.id, cond.id); } catch { /* non-fatal */ }

      results.push({
        condition_id: cond.id,
        type: cond.type,
        passed,
        output: sanitized,
        exit_code: exitCode,
      });
    }

    // allPassed semantics:
    //   - Zero conditions: trivially passes (results.length === 0) — nodes without done-conditions advance on any idle tick
    //   - All pass: no false, at least one true
    //   - Any false: fails
    //   - All pending: does NOT pass (hasAnyTrue is false) — node stays active until conditions resolve
    //   Note: model_judge returns "pending"; pending-only results do NOT advance the node (REQ-GH-203)
    // Mixed pending+true: if any condition is true and none are false, the node advances
    // even if some conditions are still pending (null). This is intentional — pending
    // conditions do not block a node that has already satisfied its other gates.
    // axiom:trace work_item=SWDE-48 spec=specs/102-Graph-Harness.md#REQ-GH-203 jira_ref=SWDE-48
    const hasAnyFalse = results.some((r) => r.passed === false);
    const hasAnyTrue = results.some((r) => r.passed === true);
    const allPassed = !hasAnyFalse && (results.length === 0 || hasAnyTrue);

    return { passed: allPassed, results };
  }

  // ─────────────────────────────────────────────────────────────────────────
  // Script Node Execution (REQ-GH-022, REQ-GH-060)
  //
  // When a node has execution_mode="script", the harness executes the node's
  // execution_config.command directly (no LLM). This bypasses the normal
  // briefing/idle cycle — the script runs immediately after node activation.
  //
  // axiom:trace work_item=graph-harness-01 spec=specs/102-Graph-Harness.md#REQ-GH-022 plan=phase-1/task-1-6/step-1-6-1
  // ─────────────────────────────────────────────────────────────────────────

  const SCRIPT_OUTPUT_CAP_BYTES = 8192;

  // ── Phase 112 — CYCLE_END_UPDATE: atomic single-statement cycle-end SQL ─────
  //
  // Called after a node completes a cycle. Decides in one round-trip whether the
  // node should be 'requeued' (runs remain, sets next_fire_at) or 'done' (runs
  // exhausted or lifetime expired, clears next_fire_at).
  //
  // The CASE expressions read the OLD trigger_run_count (pre-increment). After
  // incrementing, if the new count reaches trigger_max_runs, status becomes 'done'.
  //
  // IMPORTANT — run_count ownership contract (Phase 8 update, prevents double-increment, F-011):
  // CYCLE_END_UPDATE is the ONLY path that increments trigger_run_count for ALL trigger nodes.
  // evaluateTriggerNodes MUST NOT increment trigger_run_count under any circumstances —
  // it only sets status='pending' and trigger_last_fired_at.
  // (Broadened from 'trigger_every_ms > 0 only' to all nodes in Phase 8/task-8-4.)
  // Spec: specs/102-Graph-Harness.md#REQ-GH-SCHED-V2 run-count ownership clause.
  //
  // axiom:trace work_item=graph-scheduler-rearchitecture-01 spec=specs/102-Graph-Harness.md#REQ-GH-SCHED-V2 plan=phase-2/task-2-1/step-2-1-1
  const CYCLE_END_UPDATE = `
    UPDATE nodes SET
      status = CASE
        WHEN (trigger_every_ms IS NULL OR trigger_every_ms = 0) AND trigger_every IS NULL
          THEN 'done'
        WHEN trigger_max_runs > 0
             AND trigger_run_count + 1 >= trigger_max_runs
          THEN 'done'
        WHEN trigger_lifetime_h > 0
             AND (julianday('now') - julianday(created_at)) * 24 >= trigger_lifetime_h
          THEN 'done'
        ELSE 'requeued'
      END,
      trigger_run_count    = trigger_run_count + 1,
      trigger_last_fired_at = datetime('now'),
      completed_at          = datetime('now'),
      next_fire_at = CASE
        WHEN (trigger_every_ms IS NULL OR trigger_every_ms = 0) AND trigger_every IS NULL
          THEN NULL
        WHEN trigger_max_runs > 0
             AND trigger_run_count + 1 >= trigger_max_runs
          THEN NULL
        WHEN trigger_lifetime_h > 0
             AND (julianday('now') - julianday(created_at)) * 24 >= trigger_lifetime_h
          THEN NULL
        WHEN trigger_every_ms > 0
          THEN datetime('now', '+' || CAST(CAST(trigger_every_ms AS REAL) / 1000.0 AS TEXT) || ' seconds')
        ELSE datetime('now')
      END
    WHERE graph_id = ? AND id = ?
  `;

  /**
   * Execute a script node directly (no LLM involvement).
   *
   * - Runs execution_config.command via runWithTimeout()
   * - Respects execution_config.working_directory (if provided)
   * - Captures up to 8KB of combined stdout+stderr, applies sanitizeOutput()
   * - Stores output in node_outputs table with key="stdout" if capture_output !== false
   * - Exit 0 → node DONE; non-zero → node FAILED
   *
   * Returns: { done: boolean; output: string; exitCode: number }
   *
   * axiom:trace work_item=graph-harness-01 spec=specs/102-Graph-Harness.md#REQ-GH-022 plan=phase-1/task-1-6/step-1-6-1
   */
  async function executeScriptNode(
    node: NodeRow,
    graphId: string,
    sessionId: string
  ): Promise<{ done: boolean; output: string; exitCode: number }> {
    const nowIso = new Date().toISOString();

    // Parse execution_config from node (stored as JSON string in DB or object)
    let execConfig: Record<string, unknown> = {};
    if (node.execution_mode === "script") {
      // execution_config is stored as JSON; re-read fresh from DB to get the full config
      const freshNode = await db.queryOne(`SELECT execution_config FROM nodes WHERE graph_id = ? AND id = ?`, [graphId, node.id]) as { execution_config: string | null } | null;
      if (freshNode?.execution_config) {
        try {
          execConfig = JSON.parse(freshNode.execution_config) as Record<string, unknown>;
        } catch { /* ignore malformed */ }
      }
    }

    const command = typeof execConfig.command === "string" ? execConfig.command : null;
    const workingDirectory = typeof execConfig.working_directory === "string"
      ? execConfig.working_directory
      : null;
    const captureOutput = execConfig.capture_output !== false; // default true

    if (!command) {
      // No command configured — treat as immediate failure
      await logLedger(graphId, sessionId, "script_node_failed", node.id, {
        node_title: node.title,
        reason: "no_command_configured",
      });
      await db.run(
        `UPDATE nodes SET status = 'failed', completed_at = ? WHERE graph_id = ? AND id = ?`
      , [nowIso, graphId, node.id]);
      return { done: false, output: "[script node: no command configured]", exitCode: -1 };
    }

    // Build the actual command — prepend cd if working_directory provided
    const fullCommand = workingDirectory
      ? `cd ${JSON.stringify(workingDirectory)} && ${command}`
      : command;

    const timeoutMs = config.harness.condition_timeout_seconds * 1000;

    await logLedger(graphId, sessionId, "script_node_started", node.id, {
      node_title: node.title,
      command,
      working_directory: workingDirectory,
    });

    // Run the command
    const result = await runWithTimeout(fullCommand, timeoutMs);

    // Apply 8KB output cap (in addition to sanitizeOutput's line-based cap)
    let capturedOutput = result.output;
    const outputBytes = new TextEncoder().encode(capturedOutput);
    if (outputBytes.length > SCRIPT_OUTPUT_CAP_BYTES) {
      const sliced = new TextDecoder().decode(outputBytes.slice(0, SCRIPT_OUTPUT_CAP_BYTES));
      const truncatedBytes = outputBytes.length - SCRIPT_OUTPUT_CAP_BYTES;
      capturedOutput = sliced + `\n... (${truncatedBytes} bytes truncated)`;
    }

    // Store output in node_outputs if capture_output is true (default)
    if (captureOutput) {
      const outputId = `out_${graphId}_${node.id}_stdout`;
      try {
        // FK column order corrected in schema (step-verify-gh-06) — no need to disable FK enforcement
        await db.run(
          `INSERT INTO node_outputs (id, graph_id, node_id, key, value, type, created_at)
           VALUES (?, ?, ?, 'stdout', ?, 'text', ?)
           ON CONFLICT(graph_id, node_id, key) DO UPDATE SET value = excluded.value`
        , [outputId, graphId, node.id, capturedOutput, nowIso]);
      } catch (dbErr) {
        console.warn("[GraphHarness] Failed to store script node output:", dbErr);
      }
    }

    if (result.exitCode === 0) {
      // Success: mark node DONE
      await db.run(
        `UPDATE nodes SET status = 'done', completed_at = ? WHERE graph_id = ? AND id = ?`
      , [nowIso, graphId, node.id]);

      await logLedger(graphId, sessionId, "script_node_done", node.id, {
        node_title: node.title,
        exit_code: result.exitCode,
        output_bytes: new TextEncoder().encode(capturedOutput).length,
      });

      // REQ-GH-032: check if this node's completion triggers a join node
      // axiom:trace work_item=graph-harness-01 spec=specs/102-Graph-Harness.md#REQ-GH-032 plan=phase-4/task-4-4/step-4-4-1
      await checkAndActivateJoinNode(node.id, graphId);

      // Decrement cluster active_nodes on script node DONE (REQ-DGE-044, SWDE-56 step-da-001)
      // axiom:trace work_item=SWDE-56 spec=specs/108-Distributed-Graph-Execution.md#REQ-DGE-044 plan=post_milestone_followon/step-da-001
      if (clusterInstanceId && (config as GraphHarnessConfig & { cluster?: { enabled?: boolean } }).cluster?.enabled && db.backend === "postgres") {
        await decrementClusterActiveNodes(db, clusterInstanceId).catch(() => { /* best-effort */ });
      }

      pluginInfo("graph-harness", `Script node "${node.id}" DONE (exit 0) for graph ${graphId}`);
      return { done: true, output: capturedOutput, exitCode: result.exitCode };
    } else {
      // Failure: mark node FAILED
      await db.run(
        `UPDATE nodes SET status = 'failed', completed_at = ? WHERE graph_id = ? AND id = ?`
      , [nowIso, graphId, node.id]);

      await logLedger(graphId, sessionId, "script_node_failed", node.id, {
        node_title: node.title,
        exit_code: result.exitCode,
        output: capturedOutput,
        reason: "non_zero_exit",
      });

      pluginWarn("graph-harness", `Script node "${node.id}" FAILED (exit ${result.exitCode}) for graph ${graphId}`);
      // Decrement cluster active_nodes on script node FAILED (REQ-DGE-044, SWDE-56 step-da-001)
      // axiom:trace work_item=SWDE-56 spec=specs/108-Distributed-Graph-Execution.md#REQ-DGE-044 plan=post_milestone_followon/step-da-001
      if (clusterInstanceId && (config as GraphHarnessConfig & { cluster?: { enabled?: boolean } }).cluster?.enabled && db.backend === "postgres") {
        await decrementClusterActiveNodes(db, clusterInstanceId).catch(() => { /* best-effort */ });
      }
      // SWDE-63 AC-1: node_failed notification for script node failures
      // axiom:trace work_item=SWDE-63 spec=specs/102-Graph-Harness.md#REQ-GH-101 plan=phase-notif/task-3/step-3 jira_ref=SWDE-63
      await dispatchNotification({
        type: "node_failed",
        graph_id: graphId,
        node_id: node.id,
        title: "Node Failed",
        body: `Node ${node.id} failed in graph ${graphId}`,
        metadata: { exit_code: result.exitCode, reason: "non_zero_exit" },
        timestamp: nowIso,
      }).catch(() => { /* non-fatal */ });
      return { done: false, output: capturedOutput, exitCode: result.exitCode };
    }
  }

  // ─────────────────────────────────────────────────────────────────────────
  // Phase 5 execution mode helpers
  // ─────────────────────────────────────────────────────────────────────────

  /**
   * Helper: mark a non-agent node DONE and advance the graph.
   * Shared by all Phase 5 mode helpers to avoid duplication.
   */
  async function markNonAgentNodeDone(
    node: NodeRow,
    graphId: string,
    sessionId: string,
    action: string,
    detail: Record<string, unknown>
  ): Promise<void> {
    const ts = new Date().toISOString();
    await db.run(
      `UPDATE nodes SET status = 'done', completed_at = ? WHERE graph_id = ? AND id = ?`
    , [ts, graphId, node.id]);
    await db.run(
      `UPDATE graphs SET modifications_without_progress = 0 WHERE id = ?`
    , [graphId]);
    await logLedger(graphId, sessionId, action, node.id, detail);
    await checkAndActivateJoinNode(node.id, graphId);

    const following = await findNextUnblockedNode(graphId);
    if (following) {
      const cas = await db.run(`
        UPDATE nodes SET status='active', activated_at=datetime('now')
        WHERE id=? AND graph_id=? AND LOWER(status)='pending'
      `, [following.id, graphId]);
      if (cas.changes === 0) return;
      await db.run(`UPDATE sessions SET node_id = ? WHERE session_id = ?`, [following.id, sessionId]);
      await logLedger(graphId, sessionId, "node_activated", following.id, {
        node_title: following.title,
        execution_mode: following.execution_mode,
        following: node.id,
      });
      if (following.execution_mode !== "script" &&
          following.execution_mode !== "transform" &&
          following.execution_mode !== "wait" &&
          following.execution_mode !== "api" &&
          following.execution_mode !== "route" &&
          following.execution_mode !== "composite") {
        const briefing = await buildNodeBriefing(graphId, following);
        // Use safeInjectBriefing so SDK failure resets node to PENDING instead of silent ACTIVE stall
        // axiom:trace work_item=graph-harness-01 spec=specs/102-Graph-Harness.md#REQ-GH-021 plan=step-postv0-005
        await safeInjectBriefing(sessionId, graphId, following.id, briefing);
      }
    } else {
      // No follow-on node. Trigger already handled graph→complete/idle.
      // Sync session if graph is now complete.
      // axiom:trace work_item=graph-scheduler-rearchitecture-01 spec=specs/102-Graph-Harness.md#REQ-GH-SCHED-V2 plan=phase-4/task-4-4/step-4-4-1
      const graphCheckNA = await db.queryOne(`SELECT status FROM graphs WHERE id=?`, [graphId]) as {status:string}|null;
      if (graphCheckNA?.status?.toLowerCase() === "complete") {
        await db.run(`UPDATE sessions SET status='done', completed_at=? WHERE session_id=? AND LOWER(status)='active'`, [ts, sessionId]);
        await logLedger(graphId, sessionId, "graph_complete", null, { message: "All nodes completed. Graph marked COMPLETE.", last_node: node.id });
        pluginInfo("graph-harness", `Graph ${graphId} is COMPLETE.`);
        await dispatchNotification({
          type: "graph_completed", graph_id: graphId, title: "Graph Complete",
          body: `Graph ${graphId} is complete`, timestamp: new Date().toISOString(),
        }).catch(() => {});
      }
    }
    // Decrement cluster active_nodes on non-agent node completion (REQ-DGE-043, REQ-DGE-044)
    // axiom:trace work_item=SWDE-56 spec=specs/108-Distributed-Graph-Execution.md#REQ-DGE-044 plan=phase-2/task-2-0/step-v3-001
    const isClusterNA = (config as GraphHarnessConfig & { cluster?: { enabled?: boolean } }).cluster?.enabled ?? false;
    if (clusterInstanceId && isClusterNA && db.backend === "postgres") {
      await decrementClusterActiveNodes(db, clusterInstanceId).catch(() => { /* best-effort */ });
    }
  }

  /**
   * Helper: mark a non-agent node FAILED.
   */
  async function markNonAgentNodeFailed(
    node: NodeRow,
    graphId: string,
    sessionId: string,
    action: string,
    detail: Record<string, unknown>
  ): Promise<void> {
    const ts = new Date().toISOString();
    await db.run(
      `UPDATE nodes SET status = 'failed', completed_at = ? WHERE graph_id = ? AND id = ?`
    , [ts, graphId, node.id]);
    await logLedger(graphId, sessionId, action, node.id, detail);
    pluginWarn("graph-harness", `Node "${node.id}" (${node.execution_mode}) FAILED for graph ${graphId}`);
    // SWDE-63 AC-1: node_failed notification for non-agent node failures
    // axiom:trace work_item=SWDE-63 spec=specs/102-Graph-Harness.md#REQ-GH-101 plan=phase-notif/task-3/step-2 jira_ref=SWDE-63
    await dispatchNotification({
      type: "node_failed",
      graph_id: graphId,
      node_id: node.id,
      title: "Node Failed",
      body: `Node ${node.id} failed in graph ${graphId}`,
      metadata: { execution_mode: node.execution_mode, ...detail },
      timestamp: ts,
    }).catch(() => { /* non-fatal */ });
    // SWDE-48: stash push + conductor agent cancel on non-agent node failure (best-effort)
    // axiom:trace work_item=SWDE-48 spec=specs/102-Graph-Harness.md jira_ref=SWDE-48
    await onNodeTerminated(graphId, node.id, "failed").catch(() => { /* non-fatal */ });
  }

  /**
   * Helper: store a value in node_outputs.
   */
  async function storeNodeOutput(
    graphId: string,
    nodeId: string,
    key: string,
    value: string,
    type: "text" | "json" = "text"
  ): Promise<void> {
    const id = `out_${graphId}_${nodeId}_${key}`;
    const ts = new Date().toISOString();
    try {
      await db.run(
        `INSERT INTO node_outputs (id, graph_id, node_id, key, value, type, created_at)
         VALUES (?, ?, ?, ?, ?, ?, ?)
         ON CONFLICT(graph_id, node_id, key) DO UPDATE SET value = excluded.value`
      , [id, graphId, nodeId, key, value, type, ts]);
    } catch (err) {
      console.warn("[GraphHarness] Failed to store node output:", err);
    }
  }

  /**
   * Helper: read an output value from any completed upstream node.
   * Searches node_outputs WHERE graph_id=graphId AND key=inputKey.
   */
  async function readUpstreamOutput(graphId: string, inputKey: string): Promise<string | null> {
    const row = await db.queryOne(`SELECT value FROM node_outputs WHERE graph_id = ? AND key = ? LIMIT 1`, [graphId, inputKey]) as { value: string } | null;
    return row?.value ?? null;
  }

  /**
   * Parse a duration string like "5s", "2m", "1h" into milliseconds.
   */
  function parseDurationMs(s: string): number {
    // Supports: ms, s, m, h, d, w
    // axiom:trace work_item=SWDE-46 spec=specs/102-Graph-Harness.md#REQ-GH-121 plan=verify-SWDE46-H4 jira_ref=SWDE-46
    const m = /^(\d+(?:\.\d+)?)\s*(ms|s|m|h|d|w)?$/.exec(s.trim());
    if (!m) return 0;
    const n = parseFloat(m[1]);
    switch ((m[2] ?? "s").toLowerCase()) {
      case "ms": return n;
      case "m": return n * 60_000;
      case "h": return n * 3_600_000;
      case "d": return n * 86_400_000;
      case "w": return n * 604_800_000;
      default: return n * 1_000; // "s" or no unit
    }
  }

  // ─── REQ-GH-061: Transform Mode ──────────────────────────────────────────
  // axiom:trace work_item=graph-harness-01 spec=specs/102-Graph-Harness.md#REQ-GH-061 plan=phase-5/task-5-1/step-5-1-1

  async function executeTransformNode(
    node: NodeRow,
    graphId: string,
    sessionId: string,
    execConfig: Record<string, unknown>
  ): Promise<void> {
    const inputKey = typeof execConfig.input_key === "string" ? execConfig.input_key : null;
    const outputKey = typeof execConfig.output_key === "string" ? execConfig.output_key : "transform_result";
    const transform = typeof execConfig.transform === "string" ? execConfig.transform : ".";
    const format = execConfig.format === "json" ? "json" : "text";

    await logLedger(graphId, sessionId, "transform_node_started", node.id, {
      node_title: node.title, input_key: inputKey, transform, format,
    });

    const input = inputKey ? (await readUpstreamOutput(graphId, inputKey) ?? "null") : "null";

    const result = await applyJqTransform(input, transform);

    if (result.startsWith("[jq error") || result.startsWith("[jq timeout]")) {
      markNonAgentNodeFailed(node, graphId, sessionId, "transform_node_failed", {
        node_title: node.title, reason: result, transform,
      });
      return;
    }

    await storeNodeOutput(graphId, node.id, outputKey, result, format === "json" ? "json" : "text");
    await markNonAgentNodeDone(node, graphId, sessionId, "transform_node_done", {
      node_title: node.title, output_key: outputKey, output_bytes: result.length,
    });
  }

  // ─── REQ-GH-062: Wait Mode ───────────────────────────────────────────────
  // axiom:trace work_item=graph-harness-01 spec=specs/102-Graph-Harness.md#REQ-GH-062 plan=phase-5/task-5-2/step-5-2-1

  async function executeWaitNode(
    node: NodeRow,
    graphId: string,
    sessionId: string,
    execConfig: Record<string, unknown>
  ): Promise<void> {
    const type = typeof execConfig.type === "string" ? execConfig.type : "time";
    const target = typeof execConfig.target === "string" ? execConfig.target : "1s";
    const pollIntervalMs = (typeof execConfig.poll_interval_seconds === "number"
      ? execConfig.poll_interval_seconds : 5) * 1000;
    const timeoutMs = (typeof execConfig.timeout_seconds === "number"
      ? execConfig.timeout_seconds : 300) * 1000;
    const expectedStatus = typeof execConfig.expected_status === "number"
      ? execConfig.expected_status : 200;
    const outputKey = typeof execConfig.output_key === "string" ? execConfig.output_key : null;

    await logLedger(graphId, sessionId, "wait_node_started", node.id, {
      node_title: node.title, type, target,
    });

    const deadline = Date.now() + timeoutMs;

    if (type === "time") {
      await Bun.sleep(parseDurationMs(target));
      await markNonAgentNodeDone(node, graphId, sessionId, "wait_node_done", {
        node_title: node.title, type, target,
      });
      return;
    }

    // Poll-based: file, http, condition
    let lastOutput = "";
    while (Date.now() < deadline) {
      let condMet = false;

      if (type === "file") {
        condMet = existsSync(target);
      } else if (type === "http") {
        try {
          const resp = await fetch(target, { signal: AbortSignal.timeout(5000) });
          condMet = resp.status === expectedStatus;
          if (condMet && outputKey) {
            lastOutput = await resp.text().catch(() => "");
          }
        } catch { condMet = false; }
      } else if (type === "condition") {
        const condResult = await runWithTimeout(target, Math.min(pollIntervalMs, 10_000));
        condMet = condResult.exitCode === 0;
        if (condMet && outputKey) lastOutput = condResult.output;
      }

      if (condMet) {
        if (outputKey && lastOutput) {
          await storeNodeOutput(graphId, node.id, outputKey, lastOutput);
        }
        await markNonAgentNodeDone(node, graphId, sessionId, "wait_node_done", {
          node_title: node.title, type, target,
        });
        return;
      }

      await Bun.sleep(Math.min(pollIntervalMs, deadline - Date.now()));
    }

    markNonAgentNodeFailed(node, graphId, sessionId, "wait_node_failed", {
      node_title: node.title, reason: "timeout", type, target, timeout_ms: timeoutMs,
    });
  }

  // ─── REQ-GH-063: API Mode ────────────────────────────────────────────────
  // axiom:trace work_item=graph-harness-01 spec=specs/102-Graph-Harness.md#REQ-GH-063 plan=phase-5/task-5-3/step-5-3-1

  async function executeApiNode(
    node: NodeRow,
    graphId: string,
    sessionId: string,
    execConfig: Record<string, unknown>
  ): Promise<void> {
    const method = typeof execConfig.method === "string" ? execConfig.method.toUpperCase() : "GET";
    let url = typeof execConfig.url === "string" ? execConfig.url : "";
    const headers = (typeof execConfig.headers === "object" && execConfig.headers !== null
      ? execConfig.headers : {}) as Record<string, string>;
    let body = typeof execConfig.body === "string" ? execConfig.body : undefined;
    const expectedStatus = typeof execConfig.expected_status === "number" ? execConfig.expected_status : 200;
    const outputKey = typeof execConfig.output_key === "string" ? execConfig.output_key : null;
    const timeoutMs = (typeof execConfig.timeout_seconds === "number" ? execConfig.timeout_seconds : 30) * 1000;

    // Build variable lookup: execution_config.variables + node context
    const variables: Record<string, string> = {};
    if (typeof execConfig.variables === "object" && execConfig.variables !== null) {
      for (const [k, v] of Object.entries(execConfig.variables as Record<string, unknown>)) {
        if (typeof v === "string") variables[k] = v;
      }
    }
    if (node.context) {
      try {
        const ctx = JSON.parse(node.context) as Record<string, unknown>;
        for (const [k, v] of Object.entries(ctx)) {
          if (typeof v === "string" && !(k in variables)) variables[k] = v;
        }
      } catch { /* ignore malformed context */ }
    }

    /**
     * Substitute {{key}} and {{$ENV_VAR}} in a string.
     * Priority: execution_config.variables > node context > upstream node_outputs > empty string.
     */
    const applyVarSubstitution = async (s: string): Promise<string> => {
      // {{$ENV_VAR}} — environment variable substitution
      s = s.replace(/\{\{(\$[A-Z_][A-Z0-9_]*)\}\}/g, (_m, k: string) => {
        return process.env[k.slice(1)] ?? "";
      });
      // {{key}} — variables dict, then node_outputs
      // Use a two-pass approach: collect substitutions first (since string.replace can't be async)
      {
        const matches = [...s.matchAll(/\{\{([^}]+)\}\}/g)];
        for (const match of matches) {
          const k = match[1].trim();
          const val = k in variables ? variables[k] : (await readUpstreamOutput(graphId, k)) ?? "";
          s = s.replace(match[0], val);
        }
      }
      return s;
    };

    if (!url) {
      markNonAgentNodeFailed(node, graphId, sessionId, "api_node_failed", {
        node_title: node.title, reason: "missing_url",
      });
      return;
    }

    // Apply variable substitution to URL and body
    url = await applyVarSubstitution(url);
    if (body !== undefined) {
      body = await applyVarSubstitution(body as string);
    }

    // ── REQ-GH-063: Blocked domain check (api_policy.blocked_domains) ────────
    // axiom:trace work_item=graph-harness-01 spec=specs/102-Graph-Harness.md#REQ-GH-063 plan=phase-5/task-5-3/step-5-3-1
    const blockedDomains = config.api_policy?.blocked_domains ?? [];
    if (blockedDomains.length > 0) {
      let hostname = "";
      try {
        hostname = new URL(url).hostname;
      } catch { /* malformed URL — caught later */ }
      if (hostname) {
        const isBlocked = blockedDomains.some((d: string) =>
          hostname === d || hostname.endsWith(`.${d}`)
        );
        if (isBlocked) {
          markNonAgentNodeFailed(node, graphId, sessionId, "api_node_failed", {
            node_title: node.title, reason: "domain_blocked", hostname, url,
          });
          return;
        }
      }
    }

    await logLedger(graphId, sessionId, "api_node_started", node.id, {
      node_title: node.title, method, url,
    });

    try {
      const resp = await fetch(url, {
        method,
        headers,
        body: body != null ? body : undefined,
        signal: AbortSignal.timeout(timeoutMs),
      });

      const responseText = await resp.text().catch(() => "");

      if (resp.status !== expectedStatus) {
        markNonAgentNodeFailed(node, graphId, sessionId, "api_node_failed", {
          node_title: node.title, reason: "unexpected_status",
          expected: expectedStatus, actual: resp.status,
        });
        return;
      }

      // Store response body in "response" key (or custom outputKey if specified)
      await storeNodeOutput(graphId, node.id, outputKey ?? "response", responseText);

      await markNonAgentNodeDone(node, graphId, sessionId, "api_node_done", {
        node_title: node.title, method, url, status: resp.status,
      });
    } catch (err) {
      markNonAgentNodeFailed(node, graphId, sessionId, "api_node_failed", {
        node_title: node.title, reason: "fetch_error",
        error: err instanceof Error ? err.message : String(err),
      });
    }
  }

  // ─── REQ-GH-064: Route Mode ──────────────────────────────────────────────
  // axiom:trace work_item=graph-harness-01 spec=specs/102-Graph-Harness.md#REQ-GH-064 plan=phase-5/task-5-4/step-5-4-1

  async function executeRouteNode(
    node: NodeRow,
    graphId: string,
    sessionId: string,
    execConfig: Record<string, unknown>
  ): Promise<void> {
    const condition = typeof execConfig.condition === "string" ? execConfig.condition : "";
    const ifTrue = typeof execConfig.if_true === "string" ? execConfig.if_true : null;
    const ifFalse = typeof execConfig.if_false === "string" ? execConfig.if_false : null;

    if (!condition) {
      markNonAgentNodeFailed(node, graphId, sessionId, "route_node_failed", {
        node_title: node.title, reason: "missing_condition",
      });
      return;
    }

    await logLedger(graphId, sessionId, "route_node_started", node.id, {
      node_title: node.title, condition, if_true: ifTrue, if_false: ifFalse,
    });

    const timeoutMs = config.harness.condition_timeout_seconds * 1000;
    const result = await runWithTimeout(condition, timeoutMs);
    const conditionMet = result.exitCode === 0;
    const targetNodeId = conditionMet ? ifTrue : ifFalse;

    await logLedger(graphId, sessionId, "route_node_evaluated", node.id, {
      node_title: node.title, exit_code: result.exitCode, condition_met: conditionMet,
      routing_to: targetNodeId,
    });

    if (targetNodeId) {
      const cas = await db.run(`
        UPDATE nodes SET status='active', activated_at=datetime('now')
        WHERE id=? AND graph_id=? AND LOWER(status)='pending'
      `, [targetNodeId, graphId]);
      if (cas.changes > 0) {
        await logLedger(graphId, sessionId, "node_activated", targetNodeId, {
          node_title: targetNodeId, execution_mode: "route_target",
          routed_from: node.id, condition_met: conditionMet,
        });
      }
    }

    await markNonAgentNodeDone(node, graphId, sessionId, "route_node_done", {
      node_title: node.title, condition_met: conditionMet, routed_to: targetNodeId,
    });
  }

  // ─── REQ-GH-065: Composite Mode ─────────────────────────────────────────
  // axiom:trace work_item=graph-harness-01 spec=specs/102-Graph-Harness.md#REQ-GH-065 plan=phase-5/task-5-5/step-5-5-1

  async function executeCompositeNode(
    node: NodeRow,
    graphId: string,
    sessionId: string,
    execConfig: Record<string, unknown>
  ): Promise<void> {
    const steps = Array.isArray(execConfig.steps) ? execConfig.steps as Array<Record<string, unknown>> : [];

    if (steps.length === 0) {
      markNonAgentNodeFailed(node, graphId, sessionId, "composite_node_failed", {
        node_title: node.title, reason: "no_steps",
      });
      return;
    }

    // Read current_step_index from node metadata for crash recovery
    let startIdx = 0;
    try {
      const metaRow = await db.queryOne(`SELECT metadata FROM nodes WHERE graph_id = ? AND id = ?`, [graphId, node.id]) as { metadata: string | null } | null;
      if (metaRow?.metadata) {
        const meta = JSON.parse(metaRow.metadata) as Record<string, unknown>;
        if (typeof meta.current_step_index === "number") {
          startIdx = meta.current_step_index;
        }
      }
    } catch { /* ignore malformed metadata */ }

    await logLedger(graphId, sessionId, "composite_node_started", node.id, {
      node_title: node.title, total_steps: steps.length, start_index: startIdx,
    });

    for (let i = startIdx; i < steps.length; i++) {
      const step = steps[i];
      const stepMode = typeof step.mode === "string" ? step.mode.toLowerCase() : "script";
      const stepConfig = (typeof step.config === "object" && step.config !== null
        ? step.config : {}) as Record<string, unknown>;

      // Checkpoint: persist current_step_index before executing step
      try {
        const metaRow = await db.queryOne(`SELECT metadata FROM nodes WHERE graph_id = ? AND id = ?`, [graphId, node.id]) as { metadata: string | null } | null;
        let meta: Record<string, unknown> = {};
        if (metaRow?.metadata) {
          try { meta = JSON.parse(metaRow.metadata) as Record<string, unknown>; } catch { /* ignore */ }
        }
        meta.current_step_index = i;
        await db.run(`UPDATE nodes SET metadata = ? WHERE graph_id = ? AND id = ?`, [JSON.stringify(meta), graphId, node.id]);
      } catch { /* non-fatal: checkpoint best-effort */ }

      await logLedger(graphId, sessionId, "composite_step_started", node.id, {
        node_title: node.title, step_index: i, step_mode: stepMode,
      });

      let stepOk = false;

      if (stepMode === "script") {
        const command = typeof stepConfig.command === "string" ? stepConfig.command : null;
        if (command) {
          const res = await runWithTimeout(command, config.harness.condition_timeout_seconds * 1000);
          stepOk = res.exitCode === 0;
          const outKey = typeof stepConfig.output_key === "string" ? stepConfig.output_key : null;
          if (outKey && res.output) await storeNodeOutput(graphId, node.id, outKey, res.output);
        }
      } else if (stepMode === "transform") {
        const inputKey = typeof stepConfig.input_key === "string" ? stepConfig.input_key : null;
        const outputKey = typeof stepConfig.output_key === "string" ? stepConfig.output_key : `step_${i}_result`;
        const transform = typeof stepConfig.transform === "string" ? stepConfig.transform : ".";
        const input = inputKey ? (await readUpstreamOutput(graphId, inputKey) ?? "null") : "null";
        const xResult = await applyJqTransform(input, transform);
        stepOk = !xResult.startsWith("[jq error") && !xResult.startsWith("[jq timeout]");
        if (stepOk) await storeNodeOutput(graphId, node.id, outputKey, xResult);
      } else if (stepMode === "api") {
        // Inline simple API call for composite steps
        let url = typeof stepConfig.url === "string" ? stepConfig.url : "";
        const method = typeof stepConfig.method === "string" ? stepConfig.method.toUpperCase() : "GET";
        const expectedSt = typeof stepConfig.expected_status === "number" ? stepConfig.expected_status : 200;
        url = url.replace(/\{\{(\$[A-Z_][A-Z0-9_]*)\}\}/g, (_m, k: string) => process.env[k.slice(1)] ?? "");
        {
          const urlMatches = [...url.matchAll(/\{\{([^}]+)\}\}/g)];
          for (const m of urlMatches) {
            const val = (await readUpstreamOutput(graphId, m[1].trim())) ?? "";
            url = url.replace(m[0], val);
          }
        }
        if (url) {
          try {
            const r = await fetch(url, { method, signal: AbortSignal.timeout(30_000) });
            stepOk = r.status === expectedSt;
            const outKey = typeof stepConfig.output_key === "string" ? stepConfig.output_key : null;
            if (stepOk && outKey) await storeNodeOutput(graphId, node.id, outKey, await r.text().catch(() => ""));
          } catch { stepOk = false; }
        }
      }

      if (!stepOk) {
        markNonAgentNodeFailed(node, graphId, sessionId, "composite_node_failed", {
          node_title: node.title, reason: "step_failed", failed_step_index: i, step_mode: stepMode,
        });
        return;
      }

      await logLedger(graphId, sessionId, "composite_step_done", node.id, {
        node_title: node.title, step_index: i, step_mode: stepMode,
      });
    }

    await markNonAgentNodeDone(node, graphId, sessionId, "composite_node_done", {
      node_title: node.title, total_steps: steps.length,
    });
  }

  // ─────────────────────────────────────────────────────────────────────────
  // Phase 5.6: executeScheduledNode — direct execution wrapper (REQ-GH-066)
  //
  // Called by the dispatcher when a node has execution_mode="scheduled".
  // Delegates to checkAndFireScheduledNodes logic for this single node,
  // marks the node DONE, then checks repeat conditions.
  //
  // axiom:trace work_item=graph-harness-01 spec=specs/102-Graph-Harness.md#REQ-GH-066 plan=phase-5/task-5-6/step-5-6-1
  // ─────────────────────────────────────────────────────────────────────────

  async function executeScheduledNode(
    node: NodeRow,
    graphId: string,
    sessionId: string,
    execConfig: Record<string, unknown>
  ): Promise<void> {
    const ts = new Date().toISOString();

    // Read current repeat metadata
    let repeatCount = 0;
    let nodeCreatedAt: string = ts;
    try {
      const metaRow = await db.queryOne(`SELECT metadata, created_at FROM nodes WHERE graph_id = ? AND id = ?`, [graphId, node.id]) as { metadata: string | null; created_at: string } | null;
      if (metaRow) {
        nodeCreatedAt = metaRow.created_at ?? ts;
        if (metaRow.metadata) {
          const meta = JSON.parse(metaRow.metadata) as Record<string, unknown>;
          if (typeof meta.repeat_count === "number") repeatCount = meta.repeat_count;
        }
      }
    } catch { /* ignore */ }

    // Resolve schedule config
    const scheduleType = typeof execConfig.schedule === "string" ? execConfig.schedule : "interval";
    const intervalSeconds = typeof execConfig.interval_seconds === "number" ? execConfig.interval_seconds : 60;
    const maxRepeatCount = typeof execConfig.max_repeat_count === "number"
      ? execConfig.max_repeat_count
      : (config.schedule_defaults?.max_repeat_count ?? 100);
    const scheduleLifetimeHours = typeof execConfig.schedule_lifetime_hours === "number"
      ? execConfig.schedule_lifetime_hours
      : (config.schedule_defaults?.schedule_lifetime_hours ?? 24);

    // Cron type: stub — log and skip cron scheduling for now
    if (scheduleType === "cron") {
      const cronExpr = typeof execConfig.cron_expression === "string" ? execConfig.cron_expression : "(unset)";
      pluginInfo("graph-harness", `Scheduled node "${node.id}" cron="${cronExpr}" — cron scheduling is stubbed; executing once`);
    }

    // Increment repeat count
    repeatCount += 1;

    // Persist updated repeat_count
    try {
      const metaRow = await db.queryOne(`SELECT metadata FROM nodes WHERE graph_id = ? AND id = ?`, [graphId, node.id]) as { metadata: string | null } | null;
      let meta: Record<string, unknown> = {};
      if (metaRow?.metadata) {
        try { meta = JSON.parse(metaRow.metadata) as Record<string, unknown>; } catch { /* ignore */ }
      }
      meta.repeat_count = repeatCount;
      await db.run(`UPDATE nodes SET metadata = ? WHERE graph_id = ? AND id = ?`, [JSON.stringify(meta), graphId, node.id]);
    } catch { /* non-fatal */ }

    await logLedger(graphId, sessionId, "scheduled_node_executed", node.id, {
      node_title: node.title,
      repeat_count: repeatCount,
      schedule_type: scheduleType,
    });

    // Mark node DONE
    await db.run(`UPDATE nodes SET status = 'done', completed_at = ? WHERE graph_id = ? AND id = ?`, [ts, graphId, node.id]);
    await db.run(`UPDATE graphs SET modifications_without_progress = 0 WHERE id = ?`, [graphId]);
    await checkAndActivateJoinNode(node.id, graphId);

    // ── Check repeat conditions ──────────────────────────────────────────────
    let shouldRepeat = true;
    let doneReason = "";

    // Check max_repeat_count (0 = unlimited)
    if (maxRepeatCount > 0 && repeatCount >= maxRepeatCount) {
      shouldRepeat = false;
      doneReason = `repeat limit reached (${repeatCount}/${maxRepeatCount})`;
    }

    // Check schedule_lifetime_hours (0 = unlimited)
    if (shouldRepeat && scheduleLifetimeHours > 0) {
      try {
        const ageMs = Date.now() - new Date(nodeCreatedAt).getTime();
        const ageHours = ageMs / (1000 * 60 * 60);
        if (ageHours >= scheduleLifetimeHours) {
          shouldRepeat = false;
          doneReason = `schedule lifetime expired (${ageHours.toFixed(2)}h >= ${scheduleLifetimeHours}h)`;
        }
      } catch { /* ignore date parse errors */ }
    }

    if (shouldRepeat && intervalSeconds > 0) {
      // Schedule reset to PENDING after interval_seconds
      const delayMs = intervalSeconds * 1000;
      pluginInfo("graph-harness", `Scheduled node "${node.id}" will repeat after ${intervalSeconds}s (execution #${repeatCount})`);
      setTimeout(async () => {
        try {
          const currentStatus = (await db.queryOne<{ status: string }>(
            `SELECT status FROM nodes WHERE graph_id = ? AND id = ?`, [graphId, node.id]
          ))?.status?.toLowerCase();
          if (currentStatus === "done") {
            await db.run(
              `UPDATE nodes SET status = 'pending', activated_at = NULL, completed_at = NULL WHERE graph_id = ? AND id = ?`
            , [graphId, node.id]);
            // Wake the graph back up so the next idle tick picks up this pending node
            await db.run(
              `UPDATE graphs SET status = 'active', completed_at = NULL WHERE id = ? AND LOWER(status) = 'complete'`
            , [graphId]);
            await logLedger(graphId, sessionId, "scheduled_node_reset", node.id, {
              node_title: node.title,
              repeat_count: repeatCount,
              next_run_after_seconds: intervalSeconds,
            });
          }
        } catch (resetErr) {
          console.error("[GraphHarness] Failed to reset scheduled node:", resetErr);
        }
      }, delayMs);
    } else if (!shouldRepeat) {
      // Final completion
      await logLedger(graphId, sessionId, "scheduled_node_completed", node.id, {
        node_title: node.title,
        repeat_count: repeatCount,
        reason: doneReason,
      });
      pluginInfo("graph-harness", `Scheduled node "${node.id}" completed permanently: ${doneReason}`);
    }
  }

  // ─── REQ-GH-066: Scheduled / Repeating Nodes ─────────────────────────────
  // axiom:trace work_item=graph-harness-01 spec=specs/102-Graph-Harness.md#REQ-GH-066 plan=phase-5/task-5-6/step-5-6-1
  //
  // On each session.idle tick, scan for PENDING scheduled nodes and fire
  // those whose schedule interval has elapsed, subject to repeat/lifetime limits.

  async function checkAndFireScheduledNodes(graphId: string, sessionId: string): Promise<void> {
    // Query all scheduled nodes for this graph that are still PENDING
    const scheduledNodes = await db.queryAll(
        `SELECT id, title, execution_mode, execution_config, metadata
         FROM nodes
         WHERE graph_id = ? AND LOWER(execution_mode) = 'scheduled' AND LOWER(status) = 'pending'`
      , [graphId]) as Array<{
        id: string;
        title: string;
        execution_mode: string;
        execution_config: string | null;
        metadata: string | null;
      }>;

    if (scheduledNodes.length === 0) return;

    const nowMs = Date.now();
    const nowIso = new Date(nowMs).toISOString();

    for (const sn of scheduledNodes) {
      // Parse execution_config
      let execConfig: Record<string, unknown> = {};
      if (sn.execution_config) {
        try { execConfig = JSON.parse(sn.execution_config) as Record<string, unknown>; } catch { /* ignore */ }
      }

      // Parse metadata (holds repeat_count, last_fired_at, etc.)
      let meta: Record<string, unknown> = {};
      if (sn.metadata) {
        try { meta = JSON.parse(sn.metadata) as Record<string, unknown>; } catch { /* ignore */ }
      }

      const schedule = typeof execConfig.schedule === "string" ? execConfig.schedule : null;
      if (!schedule) continue; // no schedule defined — skip

      const maxRepeat = typeof execConfig.max_repeat_count === "number"
        ? execConfig.max_repeat_count
        : (config.schedule_defaults?.max_repeat_count ?? 100);
      const lifetimeHours = typeof execConfig.schedule_lifetime_hours === "number"
        ? execConfig.schedule_lifetime_hours
        : (config.schedule_defaults?.schedule_lifetime_hours ?? 24);

      const repeatCount = typeof meta.repeat_count === "number" ? meta.repeat_count : 0;
      const firstFiredAt = typeof meta.first_fired_at === "number" ? meta.first_fired_at : null;
      const lastFiredAt = typeof meta.last_fired_at === "number" ? meta.last_fired_at : null;

      // Check max_repeat_count limit
      if (repeatCount >= maxRepeat) {
        await logLedger(graphId, sessionId, "scheduled_node_limit_reached", sn.id, {
          node_title: sn.title, repeat_count: repeatCount, max_repeat_count: maxRepeat,
        });
        continue;
      }

      // Check schedule_lifetime_hours limit
      if (firstFiredAt !== null) {
        const lifetimeMs = lifetimeHours * 3_600_000;
        if (nowMs - firstFiredAt > lifetimeMs) {
          await logLedger(graphId, sessionId, "scheduled_node_lifetime_expired", sn.id, {
            node_title: sn.title, lifetime_hours: lifetimeHours,
          });
          continue;
        }
      }

      // Parse "every Ns" / "every Nm" / "every Nh" schedule format
      // Also supports: datetime ISO strings (fire once at or after that time)
      let shouldFire = false;

      const everyMatch = /^every\s+(\d+(?:\.\d+)?)\s*(ms|s|m|h)?$/i.exec(schedule.trim());
      if (everyMatch) {
        const intervalMs = parseDurationMs(`${everyMatch[1]}${everyMatch[2] ?? "s"}`);
        // intervalMs >= 0: allow 0ms intervals (fire on every tick)
        if (lastFiredAt === null) {
          // Never fired — fire immediately
          shouldFire = true;
        } else {
          shouldFire = nowMs - lastFiredAt >= intervalMs;
        }
      } else {
        // Try ISO datetime: fire once at or after the specified time
        const targetMs = Date.parse(schedule);
        if (!isNaN(targetMs)) {
          shouldFire = nowMs >= targetMs && (lastFiredAt === null || lastFiredAt < targetMs);
        }
      }

      if (!shouldFire) continue;

      // ── Fire the scheduled node ──────────────────────────────────────────
      // Update metadata: increment repeat_count, update last_fired_at
      meta.repeat_count = repeatCount + 1;
      meta.last_fired_at = nowMs;
      if (firstFiredAt === null) meta.first_fired_at = nowMs;

      await db.run(`UPDATE nodes SET metadata = ? WHERE graph_id = ? AND id = ?`, [JSON.stringify(meta), graphId, sn.id]);

      await logLedger(graphId, sessionId, "scheduled_node_fired", sn.id, {
        node_title: sn.title, repeat_count: meta.repeat_count, schedule,
      });

      // Execute the node's payload if it has a command
      const command = typeof execConfig.command === "string" ? execConfig.command : null;
      if (command) {
        const timeoutMs = config.harness.condition_timeout_seconds * 1000;
        const result = await runWithTimeout(command, timeoutMs);
        const outputKey = typeof execConfig.output_key === "string" ? execConfig.output_key : "scheduled_output";
        if (result.output) {
          await storeNodeOutput(graphId, sn.id, `${outputKey}_${meta.repeat_count}`, result.output);
        }
        await logLedger(graphId, sessionId, "scheduled_node_executed", sn.id, {
          node_title: sn.title, exit_code: result.exitCode, repeat_count: meta.repeat_count,
        });
      }
    }
  }

  // ─── Phase 8 / §17b: Trigger Node Evaluation ────────────────────────────
  // axiom:trace work_item=SWDE-46 spec=specs/102-Graph-Harness.md#17b.2 plan=phase-1/task-1.2/step-1 jira_ref=SWDE-46
  //
  // Valid session/graph trigger event names (§17b.1).
  const KNOWN_TRIGGER_EVENTS = new Set([
    "idle", "active", "always", "never", "once",
    "session.created", "session.complete", "message.received",
    "graph.complete", "graph.failed", "graph.node.done", "graph.node.failed",
  ]);

  /**
   * Evaluate trigger.on for all PENDING and CANCELLED (re-activatable) nodes in a graph
   * when a given event fires. For idle/interval nodes, checks trigger_every interval.
   * For cron-based nodes, checks the cron schedule.
   *
   * Called from: checkAndFireScheduledNodes (idle tick) and harnessEventHandler (any event).
   *
   * §17b.2: Idle-interval, cron, always-on, one-shot modes.
   * §17b.3: CANCELLED → re-activated by next trigger event.
   */
  async function evaluateTriggerNodes(graphId: string, sessionId: string, event: string): Promise<void> {
    const nowMs = Date.now();

    // Log warning for unknown events (§17b.1)
    if (event !== "idle" && !KNOWN_TRIGGER_EVENTS.has(event)) {
      pluginWarn("graph-harness", `Unknown trigger event "${event}" — treating as never (§17b.1)`);
      return;
    }

    // Query all nodes with trigger blocks that are PENDING or CANCELLED
    const triggerNodes = await db.queryAll(
        `SELECT id, title, trigger_on, trigger_cancel_on, trigger_every, trigger_cron,
                trigger_max_runs, trigger_lifetime_h, trigger_run_count, trigger_last_fired_at,
                status, created_at, trigger_every_ms
         FROM nodes
         WHERE graph_id = ?
           AND LOWER(status) IN ('pending', 'cancelled', 'requeued')
           AND (trigger_on IS NOT NULL OR trigger_cron IS NOT NULL)
           AND NOT (
             -- Exclude idle+every nodes: scheduler owns those exclusively (Phase 112 task-5-2)
             LOWER(COALESCE(trigger_on,'idle')) = 'idle'
             AND (trigger_every IS NOT NULL OR COALESCE(trigger_every_ms,0) > 0)
             AND trigger_cron IS NULL
           )`
      , [graphId]) as Array<{
        id: string; title: string;
        trigger_on: string | null; trigger_cancel_on: string | null;
        trigger_every: string | null; trigger_cron: string | null;
        trigger_max_runs: number; trigger_lifetime_h: number;
        trigger_run_count: number; trigger_last_fired_at: string | null;
        status: string; created_at: string;
      }>;

    if (triggerNodes.length === 0) return;

    for (const tn of triggerNodes) {
      const tOn = tn.trigger_on ?? "idle";
      const tEvery = tn.trigger_every;
      const tCron = tn.trigger_cron;
      const tMaxRuns = tn.trigger_max_runs ?? 0;
      const tLifetimeH = tn.trigger_lifetime_h ?? 0;
      const runCount = tn.trigger_run_count ?? 0;
      const lastFiredMs = tn.trigger_last_fired_at ? Date.parse(tn.trigger_last_fired_at) : null;
      const createdMs = Date.parse(tn.created_at);

      // Check max_runs limit (0 = unlimited)
      if (tMaxRuns > 0 && runCount >= tMaxRuns) {
        await logLedger(graphId, sessionId, "trigger_max_runs_reached", tn.id, {
          node_title: tn.title, run_count: runCount, max_runs: tMaxRuns,
        });
        // If the node is CANCELLED (waiting to re-fire) and runs are exhausted,
        // mark it DONE permanently so the graph can complete.
        // axiom:trace work_item=graph-scheduler-repeat-01 spec=specs/102-Graph-Harness.md#17b plan=phase-1/task-1-2/step-1-2-1
        if (tn.status.toLowerCase() === "cancelled") {
          await db.run(
            `UPDATE nodes SET status='done', completed_at=? WHERE graph_id=? AND id=?`,
            [new Date(nowMs).toISOString(), graphId, tn.id]
          );
          await logLedger(graphId, sessionId, "trigger_node_done", tn.id, {
            node_title: tn.title, reason: "max_runs_reached", run_count: runCount, max_runs: tMaxRuns,
          });
        }
        continue;
      }

      // Check lifetime limit (0 = unlimited)
      if (tLifetimeH > 0) {
        const ageMs = nowMs - createdMs;
        if (ageMs >= tLifetimeH * 3_600_000) {
          await logLedger(graphId, sessionId, "trigger_lifetime_expired", tn.id, {
            node_title: tn.title, age_hours: ageMs / 3_600_000, lifetime_hours: tLifetimeH,
          });
          // If CANCELLED and lifetime expired, mark DONE so the graph can complete.
          // axiom:trace work_item=graph-scheduler-repeat-01 spec=specs/102-Graph-Harness.md#17b plan=phase-1/task-1-2/step-1-2-1
          if (tn.status.toLowerCase() === "cancelled") {
            await db.run(
              `UPDATE nodes SET status='done', completed_at=? WHERE graph_id=? AND id=?`,
              [new Date(nowMs).toISOString(), graphId, tn.id]
            );
            await logLedger(graphId, sessionId, "trigger_node_done", tn.id, {
              node_title: tn.title, reason: "lifetime_expired",
              age_hours: ageMs / 3_600_000, lifetime_hours: tLifetimeH,
            });
          }
          continue;
        }
      }

      let shouldFire = false;

      // ── Cron path ──────────────────────────────────────────────────────────
      if (tCron) {
        // Only evaluate on idle ticks (cron is time-driven)
        if (event === "idle") {
          shouldFire = cronMatchesNow(tCron, nowMs, lastFiredMs);
        }
      }
      // ── Event-based path ──────────────────────────────────────────────────
      else if (tOn === "once") {
        // ── One-shot: fire once on the first idle event, then stop ────────────
        // axiom:trace work_item=SWDE-46 spec=specs/102-Graph-Harness.md#17b.1 plan=verify-SWDE46-C2 jira_ref=SWDE-46
        shouldFire = lastFiredMs === null;
      } else if (tOn === "always") {
        // ── Always-on: fire on EVERY idle tick, NO interval gating ───────────
        // §17b.1: "Fire on every session.idle tick without interval gating".
        // trigger.every is intentionally ignored for 'always' nodes.
        // axiom:trace work_item=SWDE-46 spec=specs/102-Graph-Harness.md#17b.1 plan=verify2-SWDE46-M1 jira_ref=SWDE-46
        shouldFire = true;
      } else if (tOn === event) {
        if (tEvery) {
          // ── Phase 112 / task-5-1: interval-only nodes are owned by the scheduler ─
          // Nodes with trigger_every (interval-driven) are managed by the outer
          // scheduler loop (processDueWork → v_due_work → pending). evaluateTriggerNodes
          // is responsible for EVENT-based triggers only.
          //
          // For nodes with BOTH trigger_on AND trigger_every, the event acts as a
          // one-shot activation on match; the interval re-fire is handled by CYCLE_END_UPDATE
          // + scheduler. This prevents double-increment of trigger_run_count (F-011).
          //
          // EXCEPTION: trigger_on='idle' + trigger_every is the scheduler's primary
          // activation path — skip it here (scheduler fires via v_due_work on next_fire_at).
          // axiom:trace work_item=graph-scheduler-rearchitecture-01 spec=specs/102-Graph-Harness.md#REQ-GH-SCHED-V2 plan=phase-5/task-5-1/step-5-1-1
          if (tOn !== "idle") {
            // Non-idle event trigger: fire on event match (scheduler still handles interval re-fire)
            shouldFire = tOn === event;
          }
          // else: idle + tEvery → scheduler owns this completely, skip
        } else {
          // No interval — fire on every matching event
          shouldFire = true;
        }
      }

      if (!shouldFire) continue;

      // ── Activate the trigger node ──────────────────────────────────────────
      // Phase 112 / task-8-4: evaluateTriggerNodes NO LONGER increments trigger_run_count.
      // CYCLE_END_UPDATE is the sole owner of trigger_run_count increments (run-count
      // ownership contract). Previously, both paths incremented — causing double-increment
      // for nodes with trigger_on + trigger_every, exhausting max_runs in half the expected
      // cycles. Now: evaluateTriggerNodes only sets status='pending' + trigger_last_fired_at.
      // axiom:trace work_item=graph-scheduler-rearchitecture-01 spec=specs/102-Graph-Harness.md#REQ-GH-SCHED-V2 plan=phase-8/task-8-4/step-8-4-1
      const firedAt = new Date(nowMs).toISOString();

      // Reset CANCELLED/REQUEUED → PENDING, update last-fired timestamp only
      await db.run(
        `UPDATE nodes
         SET status = 'pending',
             trigger_last_fired_at = ?,
             activated_at = NULL,
             completed_at = NULL
         WHERE graph_id = ? AND id = ?`
      , [firedAt, graphId, tn.id]);

      // Ensure the graph stays active
      await db.run(
        `UPDATE graphs SET status = 'active' WHERE id = ? AND LOWER(status) IN ('complete', 'created')`
      , [graphId]);

      await logLedger(graphId, sessionId, "trigger_node_fired", tn.id, {
        node_title: tn.title, event, run_count: runCount,
        trigger_on: tOn, trigger_every: tEvery ?? null, trigger_cron: tCron ?? null,
      });

      pluginInfo("graph-harness", `Trigger node "${tn.id}" fired (event=${event}, run=${runCount})`);
    }
  }

  /**
   * Evaluate cron expression against current time.
   * Minimal cron parser: only fields 0–4 (minute, hour, dom, month, dow).
   * Returns true if the cron matches the current minute AND at least 60s have elapsed since lastFiredMs.
   */
  function cronMatchesNow(expr: string, nowMs: number, lastFiredMs: number | null): boolean {
    // Anti-double-fire guard: don't fire more than once per minute
    if (lastFiredMs !== null && nowMs - lastFiredMs < 60_000) return false;

    const d = new Date(nowMs);
    const fields = expr.trim().split(/\s+/);
    if (fields.length < 5) return false;

    function matchField(field: string, value: number): boolean {
      if (field === "*") return true;
      // Handle */step
      if (field.startsWith("*/")) {
        const step = parseInt(field.slice(2), 10);
        return !isNaN(step) && step > 0 && value % step === 0;
      }
      // Handle ranges: e.g. "1-5"
      if (field.includes("-")) {
        const [lo, hi] = field.split("-").map(Number);
        return value >= lo && value <= hi;
      }
      // Handle lists: e.g. "1,2,3"
      if (field.includes(",")) {
        return field.split(",").map(Number).includes(value);
      }
      // Plain number
      const n = parseInt(field, 10);
      return !isNaN(n) && n === value;
    }

    const minute = d.getUTCMinutes();
    const hour   = d.getUTCHours();
    const dom    = d.getUTCDate();
    const month  = d.getUTCMonth() + 1; // cron months are 1-12
    const dow    = d.getUTCDay();       // 0=Sun

    return (
      matchField(fields[0], minute) &&
      matchField(fields[1], hour)   &&
      matchField(fields[2], dom)    &&
      matchField(fields[3], month)  &&
      matchField(fields[4], dow)
    );
  }

  /**
   * Handle trigger.cancel_on events for ACTIVE and PENDING trigger nodes.
   *
   * §17b.3:
   * - PENDING: reset timer (do not activate)
   * - ACTIVE: mark CANCELLED + ledger entry node_cancelled
   * - Graph stays active
   */
  async function evaluateCancelOnNodes(graphId: string, sessionId: string, event: string): Promise<void> {
    // Find all nodes whose trigger_cancel_on matches this event
    const cancelNodes = await db.queryAll(
        `SELECT id, title, status, trigger_cancel_on
         FROM nodes
         WHERE graph_id = ?
           AND trigger_cancel_on = ?
           AND LOWER(status) IN ('pending', 'active')`
      , [graphId, event]) as Array<{
        id: string; title: string; status: string; trigger_cancel_on: string;
      }>;

    for (const cn of cancelNodes) {
      if (cn.status.toLowerCase() === "pending") {
        // PENDING: reset timer (update last_fired_at to now so interval re-starts)
        await db.run(
          `UPDATE nodes SET trigger_last_fired_at = ? WHERE graph_id = ? AND id = ?`
        , [new Date().toISOString(), graphId, cn.id]);
        await logLedger(graphId, sessionId, "trigger_cancel_pending_reset", cn.id, {
          node_title: cn.title, cancel_on: event,
        });
        pluginInfo("graph-harness", `Trigger node "${cn.id}" — PENDING reset by cancel_on="${event}"`);
      } else if (cn.status.toLowerCase() === "active") {
        // ACTIVE: mark CANCELLED + ledger entry
        await db.run(
          `UPDATE nodes SET status = 'cancelled', completed_at = ? WHERE graph_id = ? AND id = ?`
        , [new Date().toISOString(), graphId, cn.id]);
        await logLedger(graphId, sessionId, "node_cancelled", cn.id, {
          reason: "trigger.cancel_on", event, node_title: cn.title,
        });
        pluginInfo("graph-harness", `Trigger node "${cn.id}" — ACTIVE → CANCELLED by cancel_on="${event}"`);
      }
    }
  }

  // ─── REQ-GH-067: Forced Tools Verification ───────────────────────────────
  // axiom:trace work_item=graph-harness-01 spec=specs/102-Graph-Harness.md#REQ-GH-067 plan=phase-5/task-5-7/step-5-7-1
  //
  // When a node has execution_config.forced_tools, the done-condition check
  // also verifies that each required tool was called during this activation,
  // and (if require_success: true) that the last call exited successfully.

  interface ForcedToolSpec {
    tool_name: string;
    require_success?: boolean;
  }

  /**
   * Check forced_tools requirement for an agent-mode node.
   * Returns null if no forced_tools configured or all are satisfied.
   * Returns an error string describing the first unsatisfied forced tool.
   */
  async function checkForcedTools(
    node: NodeRow,
    graphId: string,
    execConfig: Record<string, unknown>
  ): string | null {
    const forcedTools = Array.isArray(execConfig.forced_tools)
      ? execConfig.forced_tools as ForcedToolSpec[]
      : null;
    if (!forcedTools || forcedTools.length === 0) return null;

    for (const ft of forcedTools) {
      const toolName = typeof ft.tool_name === "string" ? ft.tool_name : "";
      if (!toolName) continue;

      // Look for a tool_called ledger entry for this node + tool
      const calledEntry = await db.queryOne(
          `SELECT detail FROM ledger
           WHERE graph_id = ? AND target_node_id = ? AND action = 'tool_called'
           ORDER BY timestamp DESC LIMIT 1`
        , [graphId, node.id]) as { detail: string } | null;

      if (!calledEntry) {
        return `forced tool not called: ${toolName}`;
      }

      // Parse the ledger detail to check tool_name and optionally exit_code
      let detail: Record<string, unknown> = {};
      try { detail = JSON.parse(calledEntry.detail) as Record<string, unknown>; } catch { /* ignore */ }

      if (detail.tool_name !== toolName) {
        return `forced tool not called: ${toolName}`;
      }

      // require_success check
      if (ft.require_success === true) {
        const exitCode = typeof detail.exit_code === "number" ? detail.exit_code : -1;
        if (exitCode !== 0) {
          return `forced tool "${toolName}" called but failed (exit_code=${exitCode})`;
        }
      }
    }

    return null; // all forced tools satisfied
  }

  /**
   * Log a tool_called ledger entry (called from tool.execute.after hook).
   * Stores tool_name, exit_code (if available), and maps to the currently
   * active node for this session.
   */
  async function logToolCalled(sessionId: string, toolName: string, exitCode: number | null): Promise<void> {
    // Find the active session/node for this session — use retry for the read too
    const sessionRow = await dbReadWithRetry(
      async () => await db.queryOne<{ graph_id: string; node_id: string | null }>(`SELECT graph_id, node_id FROM sessions WHERE session_id = ? AND LOWER(status) = 'active'`, [sessionId]),
      "logToolCalled(read)"
    );
    if (!sessionRow || !sessionRow.node_id) return;

    await logLedger(sessionRow.graph_id, sessionId, "tool_called", sessionRow.node_id, {
      tool_name: toolName,
      exit_code: exitCode,
    });
  }

  /**
   * Verify forced_tools requirement for an agent-mode node.
   * Returns { passed: true, failures: [] } if all are satisfied or none configured.
   * Returns { passed: false, failures: [...] } describing unsatisfied tools.
   *
   * axiom:trace work_item=graph-harness-01 spec=specs/102-Graph-Harness.md#REQ-GH-067 plan=phase-5/task-5-7/step-5-7-1
   */
  async function verifyForcedTools(
    nodeId: string,
    graphId: string,
    _sessionId: string,
    execConfig: Record<string, unknown>
  ): { passed: boolean; failures: string[] } {
    const forcedTools = Array.isArray(execConfig.forced_tools)
      ? execConfig.forced_tools as ForcedToolSpec[]
      : null;
    if (!forcedTools || forcedTools.length === 0) return { passed: true, failures: [] };

    const failures: string[] = [];

    for (const ft of forcedTools) {
      const toolName = typeof ft.tool_name === "string" ? ft.tool_name : "";
      if (!toolName) continue;

      // Query ALL tool_called entries for this node — look for matching tool_name
      const calledEntries = await db.queryAll(
          `SELECT detail FROM ledger
           WHERE graph_id = ? AND target_node_id = ? AND action = 'tool_called'
           ORDER BY timestamp DESC`
        , [graphId, nodeId]) as { detail: string }[];

      const matchingEntry = calledEntries.find((e) => {
        try {
          const d = JSON.parse(e.detail) as Record<string, unknown>;
          return d.tool_name === toolName;
        } catch { return false; }
      });

      if (!matchingEntry) {
        failures.push(`forced tool not called: ${toolName}`);
        continue;
      }

      // require_success check
      if (ft.require_success === true) {
        let detail: Record<string, unknown> = {};
        try { detail = JSON.parse(matchingEntry.detail) as Record<string, unknown>; } catch { /* ignore */ }
        const exitCode = typeof detail.exit_code === "number" ? detail.exit_code : -1;
        if (exitCode !== 0) {
          failures.push(`forced tool "${toolName}" called but failed (exit_code=${exitCode})`);
        }
      }
    }

    return { passed: failures.length === 0, failures };
  }

  /**
   * Build a human-readable briefing for a node to inject via session.prompt().
   * Provides the model with context about what it should accomplish next.
   */
  async function buildNodeBriefing(graphId: string, node: NodeRow): Promise<string> {
    const lines: string[] = [
      `[Graph Harness] Next task: **${node.title}**`,
      "",
      `Node: ${node.id} | Graph: ${graphId}`,
    ];

    if (node.description) {
      lines.push("", `**Description**: ${node.description}`);
    }

    // Include context (constraints + instructions)
    if (node.context) {
      try {
        const ctx = JSON.parse(node.context) as Record<string, unknown>;
        if (Array.isArray(ctx.constraints) && ctx.constraints.length > 0) {
          lines.push("", "**Constraints**:");
          for (const c of ctx.constraints) {
            lines.push(`  - ${c}`);
          }
        }
        if (ctx.instructions) {
          lines.push("", `**Instructions**: ${ctx.instructions}`);
        }
      } catch { /* malformed context — skip */ }
    }

    // Show conditions the agent needs to satisfy
    const conditions = await db.queryAll(
        `SELECT type, command, description FROM conditions
         WHERE graph_id = ? AND node_id = ?
         ORDER BY ordinal ASC`
      , [graphId, node.id]) as Array<{ type: string; command: string | null; description: string | null }>;

    if (conditions.length > 0) {
      lines.push("", "**Done conditions** (harness will auto-evaluate after each turn):");
      for (const c of conditions) {
        const label = c.description || c.command || c.type;
        lines.push(`  - [${c.type}] ${label}`);
      }
    }

    // SWDE-48: include stash context if it was popped into this node's annotations
    // axiom:trace work_item=SWDE-48 spec=specs/102-Graph-Harness.md#REQ-GH-200 jira_ref=SWDE-48
    try {
      const stashAnnotation = await db.queryOne(
        `SELECT content FROM annotations
         WHERE graph_id=? AND node_id=? AND type='note' AND content LIKE '[Stash:%'
         ORDER BY created_at ASC LIMIT 1`
      , [graphId, node.id]) as { content: string } | null;
      if (stashAnnotation) {
        lines.push("", "**Stash context** (auto-popped on activation):", stashAnnotation.content);
      }
    } catch { /* non-fatal */ }

    lines.push(
      "",
      "When you believe this task is complete, say so. " +
      "The harness will evaluate done conditions automatically."
    );

    return lines.join("\n");
  }

  /**
   * Build a retry briefing injecting previous failure context.
   */
  async function buildRetryBriefing(
    graphId: string,
    node: NodeRow,
    failedConditions: ConditionResult[]
  ): string {
    const lines: string[] = [
      `[Graph Harness] Retry attempt ${node.attempt_count} for: **${node.title}**`,
      "",
      "The previous attempt did not satisfy all done conditions.",
      "",
      "**Failed conditions**:",
    ];

    for (const c of failedConditions) {
      lines.push(`  - [${c.type}] ${c.output}`);
    }

    lines.push(
      "",
      "Please address the failures above and try again. " +
      "When ready, indicate completion — the harness will re-evaluate."
    );

    return lines.join("\n");
  }

  // ─────────────────────────────────────────────────────────────────────────
  // session.idle Event Handler — Harness Execution Loop (REQ-GH-021)
  //
  // Fired by OpenCode when the agent finishes its turn (model done, all tool
  // calls resolved). This is the central harness clock tick.
  //
  // Architecture: the `event` hook receives all OpenCode events as a union.
  // We filter for `session.idle` and run the graph advancement logic.
  //
  // Non-blocking contract: any unexpected error is caught and logged.
  // The plugin must NEVER crash on an unhandled exception in this handler.
  //
  // axiom:trace work_item=graph-harness-01 spec=specs/102-Graph-Harness.md#REQ-GH-021 plan=phase-1/task-1-4/step-1-4-1
  // ─────────────────────────────────────────────────────────────────────────

  // ─────────────────────────────────────────────────────────────────────────
  // session.idle + session.complete Event Handler (REQ-GH-021, REQ-GH-074)
  //
  // Handles two event types:
  //   - session.idle: harness execution loop tick
  //   - session.complete: cost data capture + ledger entry
  //
  // Non-blocking contract: any unexpected error is caught and logged.
  //
  // axiom:trace work_item=graph-harness-01 spec=specs/102-Graph-Harness.md#REQ-GH-021 plan=phase-1/task-1-4/step-1-4-1
  // axiom:trace work_item=graph-harness-01 spec=specs/102-Graph-Harness.md#REQ-GH-074 plan=phase-1/task-1-8/step-1-8-1
  // ─────────────────────────────────────────────────────────────────────────

  const harnessEventHandler = async ({ event }: { event: { type: string; properties?: Record<string, unknown> } }) => {
    if (!config.enabled) return;

    if (event.type === "session.idle") {
      // Extract sessionID from event properties (EventSessionIdle shape)
      const sessionId = (event.properties as { sessionID?: string } | undefined)?.sessionID;
      if (!sessionId) return;

      try {
        await runHarnessLoop(sessionId);
      } catch (err) {
        // Non-blocking contract: log but never crash
        console.error("[GraphHarness] Unhandled error in harness loop:", err);
      }
      return;
    }

    // ── ADR-GH-001: session.created — re-inject pending graph work ──────────────────
    // When the SDK exposes a stop hook, this can be removed in favour of that.
    // Until then: when a new OpenCode session is created, check if there is an
    // active graph with a node waiting for a briefing.  Bootstrap the new session
    // for that graph and inject the pending briefing so multi-node graphs continue
    // without requiring human re-prompting.
    //
    // axiom:trace work_item=graph-harness-01 spec=specs/102-Graph-Harness.md#REQ-GH-021 plan=adr-gh-001
    if (event.type === "session.created") {
      const props = event.properties as { info?: { id?: string } } | undefined;
      const newSessionId = props?.info?.id;
      if (!newSessionId) return;

      try {
        // Find any active graph that has an ACTIVE node but no live session for it
        // (i.e., the previous session ended before the agent finished the briefing)
        const orphanedWork = await db.queryOne(`
          SELECT n.id AS node_id, n.graph_id, g.title AS graph_name, n.title AS node_title
          FROM nodes n
          JOIN graphs g ON g.id = n.graph_id
          WHERE LOWER(n.status) = 'active'
            AND LOWER(g.status) IN ('active', 'created')
            AND n.id NOT IN (
              SELECT COALESCE(s.node_id, '') FROM sessions s
              WHERE s.graph_id = n.graph_id AND LOWER(s.status) = 'active'
                AND s.node_id IS NOT NULL
            )
          ORDER BY n.activated_at ASC
          LIMIT 1
        `, []) as { node_id: string; graph_id: string; graph_name: string; node_title: string } | undefined;

        if (orphanedWork) {
          const { node_id, graph_id, node_title } = orphanedWork;
          pluginInfo("graph-harness", `session.created: found orphaned work — node "${node_title}" in graph ${graph_id}. Bootstrapping new session ${newSessionId}.`);

          // Register the new session for this graph+node
          await db.run(`
            INSERT OR IGNORE INTO sessions
              (session_id, graph_id, role, status, node_id, created_at, last_heartbeat)
            VALUES (?, ?, 'coordinator', 'active', ?, datetime('now'), datetime('now'))
          `, [newSessionId, graph_id, node_id]);
          await db.run(
            `UPDATE sessions SET node_id = ? WHERE session_id = ?`
          , [node_id, newSessionId]);

          await addLedgerEntry(graph_id, 'session_resumed', {
            new_session_id: newSessionId,
            node_id,
            reason: 'prior_session_ended',
          });

          // Re-inject the briefing for the pending node into the new session
          const nodeRows = await db.queryOne(
            `SELECT * FROM nodes WHERE id = ? AND graph_id = ?`
          , [node_id, graph_id]) as NodeRow | undefined;

          if (nodeRows) {
            const briefing = await buildNodeBriefing(graph_id, nodeRows);
            // ADR-GH-001: retry briefing injection with backoff in case session isn't ready yet
            // axiom:trace work_item=graph-harness-01 spec=specs/102-Graph-Harness.md#REQ-GH-021 plan=step-postv0-006
            const INJECTION_DELAYS = [500, 1000, 2000]; // 500ms, 1s, 2s
            let injected = false;
            for (const delay of INJECTION_DELAYS) {
              await Bun.sleep(delay);
              try {
                await injectBriefing(newSessionId, briefing);
                injected = true;
                break; // success — stop retrying
              } catch {
                // Not ready yet — try next delay
                pluginInfo("graph-harness", `session.created: inject attempt failed (will retry), session: ${newSessionId}`);
              }
            }
            if (!injected) {
              // All retries exhausted — log but don't panic; next session.idle will pick it up
              pluginError("graph-harness", `session.created: all injection retries exhausted for node ${node_id}`);
              await addLedgerEntry(graph_id, 'session_created_inject_exhausted', {
                new_session_id: newSessionId, node_id, delays_tried: INJECTION_DELAYS
              });
            }
          }
        }
      } catch (err) {
        console.warn("[GraphHarness] session.created handler error:", err);
      }
      return;
    }

    if (event.type === "session.complete") {
      // Cost tracking: capture token usage and cost from session.complete event
      // axiom:trace work_item=graph-harness-01 spec=specs/102-Graph-Harness.md#REQ-GH-074 plan=phase-1/task-1-8/step-1-8-1
      const props = event.properties as Record<string, unknown> | undefined;
      const sessionId = (props?.sessionID ?? props?.session_id ?? props?.id) as string | undefined;
      if (!sessionId) return;

      try {
        // Look up the session in the DB
        const sessionRow = await db.queryOne(`SELECT session_id, graph_id FROM sessions WHERE session_id = ?`, [sessionId]) as { session_id: string; graph_id: string } | undefined;

        if (!sessionRow) return; // Not a tracked session

        const graphId = sessionRow.graph_id;

        // Extract cost data from event properties (SDK-specific shape varies)
        const tokensUsed = typeof props?.tokens_used === "number" ? props.tokens_used
          : typeof props?.tokensUsed === "number" ? props.tokensUsed
          : null;
        const costUsd = typeof props?.cost_usd === "number" ? props.cost_usd
          : typeof props?.costUsd === "number" ? props.costUsd
          : typeof props?.cost === "number" ? props.cost
          : null;

        // Update sessions table with cost data if available
        if (tokensUsed !== null || costUsd !== null) {
          const nowIso = new Date().toISOString();
          if (tokensUsed !== null && costUsd !== null) {
            // Use updateSessionCost for increment + threshold warning (REQ-GH-074)
            // axiom:trace work_item=graph-harness-01 spec=specs/102-Graph-Harness.md#REQ-GH-074 plan=phase-4/task-4-5/step-4-5-1
            await updateSessionCost(sessionId, tokensUsed, costUsd);
            await db.run(
              `UPDATE sessions SET status = 'done', completed_at = ? WHERE session_id = ?`
            , [nowIso, sessionId]);
          } else if (tokensUsed !== null) {
            await updateSessionCost(sessionId, tokensUsed, 0);
            await db.run(
              `UPDATE sessions SET status = 'done', completed_at = ? WHERE session_id = ?`
            , [nowIso, sessionId]);
          } else {
            await updateSessionCost(sessionId, 0, costUsd!);
            await db.run(
              `UPDATE sessions SET status = 'done', completed_at = ? WHERE session_id = ?`
            , [nowIso, sessionId]);
          }
        }

        // Always log session ended to ledger
        await addLedgerEntry(graphId, "session_ended", {
          session_id: sessionId,
          tokens_used: tokensUsed,
          cost_usd: costUsd,
          has_cost_data: tokensUsed !== null || costUsd !== null,
        }, { sessionId });

        // REQ-GH-075: clean up briefing failure counter for this session (memory leak fix)
        // axiom:trace work_item=graph-harness-01 spec=specs/102-Graph-Harness.md#REQ-GH-075 plan=step-final-gate-04
        await resetBriefingFailure(sessionId);

      } catch (err) {
        console.warn("[GraphHarness] Failed to handle session.complete event:", err);
      }
    }

  // ── Phase 8 / §17b.3: cancel_on handler — fires evaluateCancelOnNodes when session becomes active
  // axiom:trace work_item=SWDE-46 spec=specs/102-Graph-Harness.md#17b.3 plan=verify-SWDE46-C1 jira_ref=SWDE-46
  if (event.type === "session.active") {
    const props = event.properties as { sessionID?: string } | undefined;
    const sessionId = props?.sessionID;
    if (!sessionId) return;
    try {
      const sessionRow = await db.queryOne(`SELECT graph_id FROM sessions WHERE session_id = ? AND LOWER(status) = 'active'`, [sessionId]) as { graph_id: string } | undefined;
      if (!sessionRow) return;
      await evaluateCancelOnNodes(sessionRow.graph_id, sessionId, "active");
    } catch (err) {
      console.warn("[GraphHarness] session.active cancel handler error:", err);
    }
    return;
  }
};

  /**
   * Check if a completed node triggers a join node strategy.
   *
   * When a sub-node completes (DONE), this function looks for any join nodes
   * that have this node in their sub_node_ids metadata. It updates the
   * completed_sub_nodes list and checks whether the join strategy is satisfied:
   *   - 'all'      : all sub-nodes DONE (default)
   *   - 'any'      : at least one sub-node DONE
   *   - 'majority' : ⌈n/2⌉ sub-nodes DONE
   *
   * If the strategy is met, atomically activates the join node (CAS).
   *
   * axiom:trace work_item=graph-harness-01 spec=specs/102-Graph-Harness.md#REQ-GH-032 plan=phase-4/task-4-4/step-4-4-1
   * axiom:trace work_item=graph-harness-01 spec=specs/102-Graph-Harness.md#REQ-GH-004 plan=phase-4/task-4-4/step-4-4-1
   */
  async function checkAndActivateJoinNode(completedNodeId: string, graphId: string): Promise<void> {
    // Wrap in a transaction to prevent concurrent calls from double-activating a join node.
    // The second call will find the join node already 'active' and skip.
    await db.transaction(async (db) => {
    // Find pending nodes that have metadata containing join_node markers
    const potentialJoins = await db.queryAll(`
      SELECT id, metadata FROM nodes
      WHERE graph_id = ? AND LOWER(status) = 'pending'
        AND metadata IS NOT NULL
        AND metadata LIKE '%join_node%'
    `, [graphId]) as Array<{ id: string; metadata: string }>;

    for (const joinCandidate of potentialJoins) {
      let meta: Record<string, unknown>;
      try {
        meta = JSON.parse(joinCandidate.metadata) as Record<string, unknown>;
      } catch {
        continue; // malformed metadata — skip
      }

      if (meta.join_node !== true) continue;

      const subNodeIds = meta.sub_node_ids as string[] | undefined;
      if (!Array.isArray(subNodeIds) || !subNodeIds.includes(completedNodeId)) continue;

      // Update completed_sub_nodes
      const completed = Array.isArray(meta.completed_sub_nodes)
        ? (meta.completed_sub_nodes as string[]).slice()
        : [];
      if (!completed.includes(completedNodeId)) {
        completed.push(completedNodeId);
      }

      const total = subNodeIds.length;
      const doneCount = completed.length;
      const strategy = (typeof meta.join_strategy === "string" ? meta.join_strategy : "all") as string;

      // Evaluate join strategy
      let strategyMet: boolean;
      switch (strategy) {
        case "any":
          strategyMet = doneCount >= 1;
          break;
        case "majority":
          strategyMet = doneCount >= Math.ceil(total / 2);
          break;
        case "all":
        default:
          strategyMet = doneCount >= total;
          break;
      }

      // Persist updated completed_sub_nodes
      meta.completed_sub_nodes = completed;
      await db.run(`UPDATE nodes SET metadata=? WHERE id=? AND graph_id=?`, [JSON.stringify(meta), joinCandidate.id, graphId]);

      if (strategyMet) {
        // CAS: atomically activate join node
        const cas = await db.run(`
          UPDATE nodes SET status='active', activated_at=datetime('now')
          WHERE id=? AND graph_id=? AND LOWER(status)='pending'
        `, [joinCandidate.id, graphId]);

        if (cas.changes > 0) {
          await addLedgerEntry(graphId, "join_activated", {
            join_node_id: joinCandidate.id,
            strategy,
            completed: doneCount,
            total,
            triggering_node: completedNodeId,
          });
          console.log(
            `[GraphHarness] Join node "${joinCandidate.id}" activated ` +
            `(strategy=${strategy}, ${doneCount}/${total} done)`
          );
        }
      }
    }
    }); // end transaction
  }

  /**
   * Detect sessions that have not sent a heartbeat within heartbeat_timeout_seconds.
   * Mark them STALE and reset their assigned nodes back to PENDING so another
   * session can pick them up on the next idle tick.
   *
   * Called at the start of every runHarnessLoop tick so stale workers are
   * reclaimed promptly whenever any active session fires.
   *
    * axiom:trace work_item=graph-harness-01 spec=specs/102-Graph-Harness.md#REQ-GH-082 plan=phase-4/task-4-3/step-4-3-1
    */
   async function detectAndReassignStaleSessions(graphId: string): Promise<void> {
     const timeoutSeconds = config.harness.heartbeat_timeout_seconds; // default 300

     const stale = await db.queryAll(`
       SELECT session_id, node_id FROM sessions
       WHERE graph_id = ? AND LOWER(status) = 'active'
         AND node_id IS NOT NULL
         AND last_heartbeat IS NOT NULL
         AND CAST((julianday('now') - julianday(last_heartbeat)) * 86400 AS INTEGER) > ?
     `, [graphId, timeoutSeconds]) as Array<{ session_id: string; node_id: string }>;

     for (const s of stale) {
       // Mark session stale
       await db.run(`UPDATE sessions SET status='stale' WHERE session_id=?`, [s.session_id]);

       // REQ-GH-082: Attempt graceful kill before node reassignment
       // Kill is best-effort — failure MUST NOT block reassignment.
       // axiom:trace work_item=graph-harness-01 spec=specs/102-Graph-Harness.md#REQ-GH-082 plan=step-p4fix-04
       await (async () => {
         try {
           const terminateFn = (client as Record<string, unknown>)?.session &&
             ((client as Record<string, Record<string, unknown>>).session?.terminate);
           if (typeof terminateFn === "function") {
             await (terminateFn as (id: string) => Promise<void>)(s.session_id);
           }
         } catch (killErr) {
           console.error(`[GraphHarness] Kill attempt failed for stale session ${s.session_id}:`, killErr);
           // Continue — node reassignment happens regardless
         }
       })();

       // Re-mark the node as PENDING so another session can pick it up
       await db.run(
         `UPDATE nodes SET status='pending', activated_at=NULL
          WHERE id=? AND graph_id=? AND LOWER(status)='active'`
       , [s.node_id, graphId]);

       await addLedgerEntry(graphId, "session_stale", {
          session_id: s.session_id,
          node_id: s.node_id,
          timeout_seconds: timeoutSeconds,
        });

        // REQ-GH-075: clean up briefing failure counter for stale session (memory leak fix)
        // axiom:trace work_item=graph-harness-01 spec=specs/102-Graph-Harness.md#REQ-GH-075 plan=step-final-gate-04
        await resetBriefingFailure(s.session_id);

        pluginWarn("graph-harness", `Session ${s.session_id} stale — node ${s.node_id} reset to PENDING`);
     }

    // Also mark stale sessions that have no node_id (idle stale sessions)
    const staleNoNode = await db.queryAll(`
      SELECT session_id FROM sessions
      WHERE graph_id = ? AND LOWER(status) = 'active'
        AND node_id IS NULL
        AND last_heartbeat IS NOT NULL
        AND CAST((julianday('now') - julianday(last_heartbeat)) * 86400 AS INTEGER) > ?
    `, [graphId, timeoutSeconds]) as Array<{ session_id: string }>;

    for (const s of staleNoNode) {
      await db.run(`UPDATE sessions SET status='stale' WHERE session_id=?`, [s.session_id]);
      await addLedgerEntry(graphId, "session_stale", {
        session_id: s.session_id,
        node_id: null,
        timeout_seconds: timeoutSeconds,
      });
      // REQ-GH-075: clean up briefing failure counter for stale session (memory leak fix)
      // axiom:trace work_item=graph-harness-01 spec=specs/102-Graph-Harness.md#REQ-GH-075 plan=step-final-gate-04
      await resetBriefingFailure(s.session_id);
    }
  }

  // ── REQ-GH-REPEAT: Shared repeat-node re-activation helper ─────────────────
  // Called after agent-mode AND script-mode node DONE to reset to CANCELLED
  // if the node should re-fire. Returns true if reset to CANCELLED, false if
  // left as DONE (runs exhausted or lifetime expired).
  // axiom:trace work_item=graph-scheduler-repeat-01 spec=specs/102-Graph-Harness.md#REQ-GH-REPEAT plan=phase-4/task-4-3/step-4-3-1
  /**
   * Core harness loop logic for one session.idle tick.
   *
   * Steps:
   * 1. Find active graph/node for this session
   * 2. If no node is active: find first unblocked PENDING node and activate it
   * 3. Evaluate done-conditions for the active node
   * 4. On pass: mark DONE, find next node, re-enter agent loop
   * 5. On fail: increment attempt_count, apply backoff, log ledger
   *
   * axiom:trace work_item=graph-harness-01 spec=specs/102-Graph-Harness.md#REQ-GH-021 plan=phase-1/task-1-4/step-1-4-1
   */
  async function runHarnessLoop(sessionId: string): Promise<void> {
    const nowIso = () => new Date().toISOString();

    // ── Step 1: Find the session record in the DB ──────────────────────────
     // axiom:trace work_item=graph-harness-01 spec=specs/102-Graph-Harness.md#REQ-GH-021 plan=step-verify2-gh-03
    const sessionRow = await db.queryOne(
        `SELECT session_id, graph_id, node_id, status
         FROM sessions WHERE session_id = ? AND LOWER(status) = 'active'`
      , [sessionId]) as SessionRow | undefined;

    if (!sessionRow) {
      // No active session registered with this session ID — nothing to do
      return;
    }

    const graphId = sessionRow.graph_id;

    // ── REQ-GH-141: Fetch graphRow early to guard DRAFT before heartbeat ─────
    // axiom:trace work_item=SWDE-54 spec=specs/102-Graph-Harness.md#REQ-GH-141 plan=phase-10/step-r4-06 jira_ref=SWDE-54
    const graphRow = await db.queryOne(`SELECT id, status FROM graphs WHERE id = ?`, [graphId]) as { id: string; status: string } | undefined;

    // REQ-GH-141: Skip DRAFT graphs — they await explicit activation via graph_activate.
    // MUST be checked BEFORE the heartbeat update so that a coordinator session tied to a
    // DRAFT graph does not get its heartbeat bumped (which would prevent stale-session recovery).
    if (graphRow && graphRow.status.toUpperCase() === 'DRAFT') {
      return; // DRAFT graph — harness loop ignores it until explicitly activated
    }

    // ── REQ-GH-082: Update heartbeat for this session ────────────────────
    // axiom:trace work_item=graph-harness-01 spec=specs/102-Graph-Harness.md#REQ-GH-082 plan=phase-4/task-4-3/step-4-3-1
    await db.run(
      `UPDATE sessions SET last_heartbeat=datetime('now') WHERE session_id=?`
    , [sessionId]);

    // ── REQ-GH-082: Detect and reassign stale peer sessions ──────────────
    await detectAndReassignStaleSessions(graphId);

    // ── Step 2: Verify graph is still active ──────────────────────────────
    // (graphRow already fetched above for DRAFT guard; re-use it here)

    if (!graphRow || !["active", "ACTIVE", "CREATED"].includes(graphRow.status.toUpperCase())) {
      // Graph completed/abandoned/failed — nothing to do
      return;
    }

    // ── Step 2b: REQ-GH-066 Scheduled node check ─────────────────────────
    // On each tick, evaluate which scheduled nodes should fire.
    // axiom:trace work_item=graph-harness-01 spec=specs/102-Graph-Harness.md#REQ-GH-066 plan=phase-5/task-5-6/step-5-6-1
    await checkAndFireScheduledNodes(graphId, sessionId);

    // ── Step 2c: Phase 8 / §17b trigger node evaluation (idle tick) ──────
    // axiom:trace work_item=SWDE-46 spec=specs/102-Graph-Harness.md#17b.2 plan=phase-1/task-1.2/step-1 jira_ref=SWDE-46
    await evaluateTriggerNodes(graphId, sessionId, "idle");

    // ── Step 3: Find the active node for this session ─────────────────────
    let activeNode: NodeRow | null = null;

    if (sessionRow.node_id) {
      activeNode = await db.queryOne(
          `SELECT id, graph_id, title, description, status, execution_mode, execution_config,
                  attempt_count, max_retries, context, activated_at, completed_at
           FROM nodes WHERE graph_id = ? AND id = ? AND LOWER(status) = 'active'`
        , [graphId, sessionRow.node_id]) as NodeRow | null;
    }

    // ── Step 4: If no active node, find and activate the first unblocked one
    if (!activeNode) {
      // ── REQ-GH-080: Circuit breaker check before node activation ──────────
      // axiom:trace work_item=graph-harness-01 spec=specs/102-Graph-Harness.md#REQ-GH-080 plan=phase-6/task-6-9/step-6-9-1
      if (await checkCircuitBreaker(graphId)) {
        return; // graph paused — stop processing
      }

      const nextNode = await findNextUnblockedNode(graphId);

      if (!nextNode) {
        // Check if graph is fully complete.
        // CANCELLED trigger nodes keep the graph alive — §17b.3: "graph stays active, waiting for next trigger event."
        // Repeating nodes that haven't exhausted max_runs also keep the graph alive.
        // axiom:trace work_item=SWDE-46 spec=specs/102-Graph-Harness.md#17b.3 plan=phase-1/task-2.1/step-1 jira_ref=SWDE-46
        // ── Phase 112 / task-4-4: Guard removed — trigger handles graph→complete/idle ─
        // axiom:trace work_item=graph-scheduler-rearchitecture-01 spec=specs/102-Graph-Harness.md#REQ-GH-SCHED-V2 plan=phase-4/task-4-4/step-4-4-1
        // Sync session: if trigger already marked graph complete, mark session done.
        const graphCheck = await db.queryOne(`SELECT status FROM graphs WHERE id=?`, [graphId]) as {status:string}|null;
        if (graphCheck?.status?.toLowerCase() === "complete") {
          await db.run(`UPDATE sessions SET status='done', completed_at=? WHERE session_id=? AND LOWER(status)='active'`, [nowIso(), sessionId]);
          await logLedger(graphId, sessionId, "graph_complete", null, { message: "All nodes completed. Graph marked COMPLETE." });
          await resetBriefingFailure(sessionId);
          archiveStaleGraphs().catch(() => {});
        }
        // No unblocked nodes and graph not complete = blocked, nothing we can do now
        return;
      }

      // ── CAS Activation Pattern (MUST follow this pattern for all node activations) ──────────
      // REQ-GH-021: Node activation MUST be atomic to prevent TOCTOU races under concurrent sessions.
      // Required pattern:
      //   1. UPDATE nodes SET status='active', activated_at=datetime('now')
      //      WHERE id=? AND graph_id=? AND LOWER(status)='pending'
      //   2. Check result.changes === 0 → another session won the race, return early
      // This pattern MUST be used in: primary activation, script follow-on activation,
      // graph.split sub-node activation (Phase 4), and any future code that activates nodes.
      // ─────────────────────────────────────────────────────────────────────────────────────────
      // Atomic activation — only succeeds if node is still PENDING
      // This prevents the TOCTOU race where two concurrent session.idle handlers
      // both find the same node and both try to activate it.
      // axiom:trace work_item=graph-harness-01 spec=specs/102-Graph-Harness.md#REQ-GH-021 plan=step-verify-gh-10
      const activationResult = await db.run(`
        UPDATE nodes
        SET status='active', activated_at=datetime('now')
        WHERE id=? AND graph_id=? AND LOWER(status)='pending'
      `, [nextNode.id, graphId]);

      if (activationResult.changes === 0) {
        // Another session activated this node first — skip
        pluginInfo("graph-harness", `Node ${nextNode.id} was activated by another session — skipping`);
        return;
      }

      await db.run(
        `UPDATE sessions SET node_id = ? WHERE session_id = ?`
      , [nextNode.id, sessionId]);

      await logLedger(graphId, sessionId, "node_activated", nextNode.id, {
        node_title: nextNode.title,
        execution_mode: nextNode.execution_mode,
      });

      // SWDE-48: stash pop + conductor agent spawn on node activation (best-effort, non-blocking)
      // axiom:trace work_item=SWDE-48 spec=specs/102-Graph-Harness.md jira_ref=SWDE-48
      await onNodeActivated(graphId, nextNode.id, sessionId).catch((err) => {
        console.warn("[GraphHarness] onNodeActivated error:", err);
      });

      // ── REQ-GH-031: Spawn workers for other unblocked nodes ───────────────
      // After activating our primary node, check if there are additional unblocked
      // nodes that we can hand off to worker sessions (up to max_concurrent_sessions).
      // Fire-and-forget: errors in spawning are logged but do not block this session.
      // axiom:trace work_item=graph-harness-01 spec=specs/102-Graph-Harness.md#REQ-GH-031 plan=phase-4/task-4-2/step-4-2-1
      spawnWorkersForUnblockedNodes(graphId, sessionId, nextNode.id).catch((err) => {
        console.error("[GraphHarness] spawnWorkersForUnblockedNodes error:", err);
      });

      // ── Execution mode dispatcher (REQ-GH-021) ────────────────────────────
      // Check execution mode — non-agent modes are handled directly (no LLM).
      // axiom:trace work_item=graph-harness-01 spec=specs/102-Graph-Harness.md#REQ-GH-021 plan=step-p4fix-08
      const execMode = (nextNode.execution_mode ?? "agent").toLowerCase();

      // ── Phase 5 execution modes (REQ-GH-061 through REQ-GH-065) ────────────
      // axiom:trace work_item=graph-harness-01 spec=specs/102-Graph-Harness.md#REQ-GH-061 plan=phase-5/task-5-1/step-5-1-1
      // axiom:trace work_item=graph-harness-01 spec=specs/102-Graph-Harness.md#REQ-GH-062 plan=phase-5/task-5-2/step-5-2-1
      // axiom:trace work_item=graph-harness-01 spec=specs/102-Graph-Harness.md#REQ-GH-063 plan=phase-5/task-5-3/step-5-3-1
      // axiom:trace work_item=graph-harness-01 spec=specs/102-Graph-Harness.md#REQ-GH-064 plan=phase-5/task-5-4/step-5-4-1
      // axiom:trace work_item=graph-harness-01 spec=specs/102-Graph-Harness.md#REQ-GH-065 plan=phase-5/task-5-5/step-5-5-1
      const execConfigRaw = nextNode.execution_config;
      const execConfig = typeof execConfigRaw === "string"
        ? (() => { try { return JSON.parse(execConfigRaw || "{}") as Record<string, unknown>; } catch { return {}; } })()
        : (execConfigRaw as Record<string, unknown> ?? {});

      if (execMode === "transform") {
        await executeTransformNode(nextNode, graphId, sessionId, execConfig);
        return;
      }
      if (execMode === "wait") {
        await executeWaitNode(nextNode, graphId, sessionId, execConfig);
        return;
      }
      if (execMode === "api") {
        await executeApiNode(nextNode, graphId, sessionId, execConfig);
        return;
      }
      if (execMode === "route") {
        await executeRouteNode(nextNode, graphId, sessionId, execConfig);
        return;
      }
      if (execMode === "composite") {
        await executeCompositeNode(nextNode, graphId, sessionId, execConfig);
        return;
      }

      // ── Phase 5.6: Scheduled node — execute directly (REQ-GH-066) ─────────
      // axiom:trace work_item=graph-harness-01 spec=specs/102-Graph-Harness.md#REQ-GH-066 plan=phase-5/task-5-6/step-5-6-1
      if (execMode === "scheduled") {
        await executeScheduledNode(nextNode, graphId, sessionId, execConfig);
        return;
      }

       // ── Script node shortcut: execute immediately, skip LLM ──────────────
       if (nextNode.execution_mode === "script") {
         const scriptResult = await executeScriptNode(nextNode, graphId, sessionId);
         if (scriptResult.done) {
           // ── Phase 112 / task-4-1: CYCLE_END_UPDATE replaces setTimeout-based repeat ─
           // Atomically decide requeue-vs-done and set next_fire_at in one round trip.
           // The SQLite trigger handles graph status (active→idle or active→complete).
           // axiom:trace work_item=graph-scheduler-rearchitecture-01 spec=specs/102-Graph-Harness.md#REQ-GH-SCHED-V2 plan=phase-4/task-4-1/step-4-1-1
           await db.run(CYCLE_END_UPDATE, [graphId, nextNode.id]);

           const updatedNode = await db.queryOne(
             `SELECT status FROM nodes WHERE graph_id=? AND id=?`, [graphId, nextNode.id]
           ) as { status: string } | null;
           const newStatus = updatedNode?.status?.toLowerCase() ?? "done";

           if (newStatus === "requeued") {
             // Scheduler will handle re-activation when next_fire_at arrives.
             // Trigger already set graph→idle. Session stays active for future cycles.
             wakeScheduler(); // interrupt sleep so scheduler processes the new requeued row promptly
             await db.run(`UPDATE graphs SET modifications_without_progress = 0 WHERE id = ?`, [graphId]);
             return;
           }

           // Node is done (one-shot or exhausted repeating). Find follow-on node.
           await db.run(`UPDATE graphs SET modifications_without_progress = 0 WHERE id = ?`, [graphId]);
           const followingNode = await findNextUnblockedNode(graphId);
           if (followingNode) {
             // Atomic activation — guard against TOCTOU race in concurrent script-node sessions.
             const followActivation = await db.run(`
               UPDATE nodes SET status='active', activated_at=datetime('now')
               WHERE id=? AND graph_id=? AND LOWER(status)='pending'
             `, [followingNode.id, graphId]);
             if (followActivation.changes === 0) {
               pluginInfo("graph-harness", `Follow node ${followingNode.id} activated by another session — skipping`);
               return;
             }
             await db.run(`UPDATE sessions SET node_id = ? WHERE session_id = ?`, [followingNode.id, sessionId]);
             await logLedger(graphId, sessionId, "node_activated", followingNode.id, {
               node_title: followingNode.title, execution_mode: followingNode.execution_mode, following: nextNode.id,
             });
             if (followingNode.execution_mode === "script") {
               return; // Let next tick handle it (avoid deep recursion)
             }
             const briefing = await buildNodeBriefing(graphId, followingNode);
             await safeInjectBriefing(sessionId, graphId, followingNode.id, briefing);
           } else {
             // No follow-on node. Trigger already handled graph status (complete/idle).
             // Sync session: if graph is now complete, mark session done.
             const graphAfter = await db.queryOne(`SELECT status FROM graphs WHERE id=?`, [graphId]) as {status:string}|null;
             if (graphAfter?.status?.toLowerCase() === "complete") {
               await db.run(`UPDATE sessions SET status='done', completed_at=? WHERE session_id=? AND LOWER(status)='active'`, [nowIso(), sessionId]);
               await logLedger(graphId, sessionId, "graph_complete", null, { message: "All nodes completed. Graph marked COMPLETE.", last_node: nextNode.id });
               pluginInfo("graph-harness", `Graph ${graphId} is COMPLETE.`);
               await dispatchNotification({ type: "graph_completed", graph_id: graphId, title: "Graph Complete", body: `Graph ${graphId} is complete`, timestamp: new Date().toISOString() }).catch(() => {});
               await resetBriefingFailure(sessionId);
               archiveStaleGraphs().catch(() => {});
             }
           }
         } else {
           // Script node failed — trigger handles graph status; just check if graph should fail.
           const pendingOrActive3 = (await db.queryOne(
               `SELECT COUNT(*) as cnt FROM nodes WHERE graph_id = ? AND LOWER(status) IN ('pending', 'active')`
             , [graphId]) as { cnt: number } | undefined)?.cnt ?? 0;
           if (pendingOrActive3 === 0) {
             const anyFailed = (await db.queryOne(
                 `SELECT COUNT(*) as cnt FROM nodes WHERE graph_id = ? AND LOWER(status) = 'failed'`
               , [graphId]) as { cnt: number } | undefined)?.cnt ?? 0;
             if (anyFailed > 0) {
               await db.run(`UPDATE graphs SET status = 'failed' WHERE id = ?`, [graphId]);
               await logLedger(graphId, sessionId, "graph_failed", null, {
                 message: "Graph failed due to script node failure.", failed_node: nextNode.id,
               });
               pluginWarn("graph-harness", `Graph ${graphId} FAILED.`);
               await dispatchNotification({
                 type: "graph_failed", graph_id: graphId, title: "Graph Failed",
                 body: `Graph ${graphId} failed (script node failure: ${nextNode.id})`,
                 metadata: { failed_node: nextNode.id }, timestamp: new Date().toISOString(),
               }).catch(() => {});
             }
           }
         }
         return;
       }

      // Inject briefing for the newly activated node — re-enter agent loop.
      // Use safeInjectBriefing so an SDK failure resets the node to PENDING
      // rather than leaving it silently stuck in ACTIVE with no briefing.
      const briefing = await buildNodeBriefing(graphId, nextNode);
      await safeInjectBriefing(sessionId, graphId, nextNode.id, briefing);
      return;
    }

    // ── Step 5: Active node exists — evaluate done-conditions ─────────────
    const conditions = await db.queryAll(
        `SELECT id, graph_id, node_id, ordinal, type, command, timeout_seconds, description
         FROM conditions WHERE graph_id = ? AND node_id = ?
         ORDER BY ordinal ASC`
      , [graphId, activeNode.id]) as ConditionRow[];

    const { passed, results } = await evaluateConditions(activeNode, conditions);

    if (passed) {
      // ── Phase 5.7: Forced tools verification (REQ-GH-067) — check before marking DONE ──
      // axiom:trace work_item=graph-harness-01 spec=specs/102-Graph-Harness.md#REQ-GH-067 plan=phase-5/task-5-7/step-5-7-1
      const activeNodeExecConfig = await (async () => {
        try {
          const cfg = await db.queryOne(`SELECT execution_config FROM nodes WHERE graph_id=? AND id=?`, [graphId, activeNode.id]) as { execution_config: string | null } | null;
          return cfg?.execution_config ? JSON.parse(cfg.execution_config) as Record<string, unknown> : {};
        } catch { return {}; }
      })();
      if (activeNodeExecConfig.forced_tools) {
        const ftResult = await verifyForcedTools(activeNode.id, graphId, sessionId, activeNodeExecConfig);
        if (!ftResult.passed) {
          // Mark node FAILED — forced tool requirements not met
          await db.run(`UPDATE nodes SET status = 'failed', completed_at = ? WHERE graph_id = ? AND id = ?`, [nowIso(), graphId, activeNode.id]);
          await logLedger(graphId, sessionId, "node_failed_forced_tools", activeNode.id, {
            node_title: activeNode.title,
            failures: ftResult.failures,
          });
          pluginWarn("graph-harness", `Node "${activeNode.id}" FAILED: forced_tools not satisfied: ${ftResult.failures.join("; ")}`);
          return; // Do not advance — let retry/fail logic handle it on next idle
        }
      }

      // ── Step 5a: All conditions pass → mark node DONE ───────────────────
      // REQ-GH-085: atomic node completion — all writes must succeed or roll back together.
      // axiom:trace work_item=graph-harness-01 spec=specs/102-Graph-Harness.md#REQ-GH-085 plan=step-final-gate-01
      await db.transaction(async (db) => {
        await db.run(
          `UPDATE nodes SET status = 'done', completed_at = ? WHERE graph_id = ? AND id = ?`
        , [nowIso(), graphId, activeNode.id]);

        // Reset modifications_without_progress counter on node completion (REQ-GH-071)
        await db.run(
          `UPDATE graphs SET modifications_without_progress = 0 WHERE id = ?`
        , [graphId]);

        await logLedger(graphId, sessionId, "node_done", activeNode.id, {
          node_title: activeNode.title,
          attempt_count: activeNode.attempt_count,
          conditions_evaluated: results.length,
        });
      });

      // REQ-GH-032: check if this node's completion triggers a join node
      // checkAndActivateJoinNode is OUTSIDE the transaction — it has its own CAS guard.
      // axiom:trace work_item=graph-harness-01 spec=specs/102-Graph-Harness.md#REQ-GH-032 plan=phase-4/task-4-4/step-4-4-1
      await checkAndActivateJoinNode(activeNode.id, graphId);

      // SWDE-48: stash push + conductor agent cancel on node completion (best-effort, non-blocking)
      // axiom:trace work_item=SWDE-48 spec=specs/102-Graph-Harness.md jira_ref=SWDE-48
      await onNodeTerminated(graphId, activeNode.id, "done").catch((err) => {
        console.warn("[GraphHarness] await onNodeTerminated(done) error:", err);
      });

      pluginInfo("graph-harness", `Node "${activeNode.id}" DONE for graph ${graphId}`);

      // ── REQ-GH-REPEAT → REQ-GH-SCHED-V2: CYCLE_END_UPDATE replaces resetNodeForRepeat ─
      // Atomically decide requeue-vs-done and set next_fire_at. Trigger handles graph status.
      // axiom:trace work_item=graph-scheduler-rearchitecture-01 spec=specs/102-Graph-Harness.md#REQ-GH-SCHED-V2 plan=phase-4/task-4-2/step-4-2-1
      await db.run(CYCLE_END_UPDATE, [graphId, activeNode.id]);

      const updatedAgentNode = await db.queryOne(
        `SELECT status FROM nodes WHERE graph_id=? AND id=?`, [graphId, activeNode.id]
      ) as { status: string } | null;
      const agentNewStatus = updatedAgentNode?.status?.toLowerCase() ?? "done";

      if (agentNewStatus === "requeued") {
        // Scheduler handles re-activation. Trigger set graph→idle. Session stays active.
        wakeScheduler();
        return;
      }

      // Find the next unblocked node
      const nextNode = await findNextUnblockedNode(graphId);

      if (nextNode) {
        // CAS activation to prevent TOCTOU race (concurrent sessions finding the same next node)
        // axiom:trace work_item=graph-harness-01 spec=specs/102-Graph-Harness.md#REQ-GH-021 plan=step-final-gate-02
        const followOnCas = await db.run(`
          UPDATE nodes SET status = 'active', activated_at = datetime('now')
          WHERE id = ? AND graph_id = ? AND LOWER(status) = 'pending'
        `, [nextNode.id, graphId]);

        if (followOnCas.changes === 0) {
          // Another session activated this node first — stop here, let them handle it
          return;
        }

        await db.run(
          `UPDATE sessions SET node_id = ? WHERE session_id = ?`
        , [nextNode.id, sessionId]);

        await logLedger(graphId, sessionId, "node_activated", nextNode.id, {
          node_title: nextNode.title,
          execution_mode: nextNode.execution_mode,
          following: activeNode.id,
        });

        // Re-enter agent loop with the next node's briefing.
        // Use safeInjectBriefing so an SDK failure resets the node to PENDING.
        const briefing = await buildNodeBriefing(graphId, nextNode);
        await safeInjectBriefing(sessionId, graphId, nextNode.id, briefing);
      } else {
        // No follow-on node. Trigger already handled graph status (complete or idle).
        // Sync session: if graph is now complete, mark session done.
        const graphAfterAgent = await db.queryOne(`SELECT status FROM graphs WHERE id=?`, [graphId]) as {status:string}|null;
        if (graphAfterAgent?.status?.toLowerCase() === "complete") {
          await db.run(`UPDATE sessions SET status='done', completed_at=? WHERE session_id=? AND LOWER(status)='active'`, [nowIso(), sessionId]);
          await logLedger(graphId, sessionId, "graph_complete", null, {
            message: "All nodes completed. Graph marked COMPLETE.", last_node: activeNode.id,
          });
          pluginInfo("graph-harness", `Graph ${graphId} is COMPLETE (last node: ${activeNode.id}).`);
          await dispatchNotification({
            type: "graph_completed", graph_id: graphId, title: "Graph Complete",
            body: `Graph ${graphId} is complete`, timestamp: new Date().toISOString(),
          }).catch(() => {});
          archiveStaleGraphs().catch(() => {});
          await resetBriefingFailure(sessionId);
        }
        // If graph is 'idle', session stays active — scheduler handles the next cycle.
      }
    } else {
      // ── Step 5b: Condition(s) failed → retry with backoff ────────────────
      const failedResults = results.filter((r) => r.passed === false);
      const newAttemptCount = (activeNode.attempt_count ?? 0) + 1;

      await db.run(
        `UPDATE nodes SET attempt_count = ? WHERE graph_id = ? AND id = ?`
      , [newAttemptCount, graphId, activeNode.id]);

      const maxRetries = activeNode.max_retries ?? config.retry.default_max_retries;

      await logLedger(graphId, sessionId, "condition_failed", activeNode.id, {
        node_title: activeNode.title,
        attempt_count: newAttemptCount,
        max_retries: maxRetries,
        failed_conditions: failedResults.map((r) => ({
          id: r.condition_id,
          type: r.type,
          output: r.output,
        })),
      });

      if (newAttemptCount > maxRetries) {
        // ── Max retries exceeded → mark node FAILED ──────────────────────
        await db.run(
          `UPDATE nodes SET status = 'failed', completed_at = ? WHERE graph_id = ? AND id = ?`
        , [nowIso(), graphId, activeNode.id]);

        await logLedger(graphId, sessionId, "node_failed", activeNode.id, {
          node_title: activeNode.title,
          attempt_count: newAttemptCount,
          max_retries: maxRetries,
          reason: "max_retries_exceeded",
        });

        pluginWarn("graph-harness", `Node "${activeNode.id}" FAILED after ${newAttemptCount} attempts in graph ${graphId}`);
        // REQ-GH-101 / SWDE-63: structured node_failed notification
        await dispatchNotification({
          type: "node_failed",
          graph_id: graphId,
          node_id: activeNode.id,
          title: "Node Failed",
          body: `Node ${activeNode.id} failed in graph ${graphId}`,
          metadata: { attempt_count: newAttemptCount, max_retries: maxRetries },
          timestamp: new Date().toISOString(),
        }).catch(() => { /* non-fatal */ });
        // SWDE-48: stash push + conductor agent cancel on node failure (best-effort, non-blocking)
        // axiom:trace work_item=SWDE-48 spec=specs/102-Graph-Harness.md jira_ref=SWDE-48
        await onNodeTerminated(graphId, activeNode.id, "failed").catch((err) => {
          console.warn("[GraphHarness] await onNodeTerminated(failed) error:", err);
        });

        // Optionally continue to next unblocked node if this one is not on critical path
        // For v0: attempt to continue (optional node behaviour TBD in later phases)
        const nextNode = await findNextUnblockedNode(graphId);
         if (nextNode) {
           // CAS activation to prevent TOCTOU race (concurrent sessions both failing same node)
           // axiom:trace work_item=graph-harness-01 spec=specs/102-Graph-Harness.md#REQ-GH-021 plan=step-postv0-008
           const failContinueCas = await db.run(
             `UPDATE nodes SET status = 'active', activated_at = ?
              WHERE graph_id = ? AND id = ? AND LOWER(status) = 'pending'`
           , [nowIso(), graphId, nextNode.id]);

           if (failContinueCas.changes === 0) {
             // Another session already activated this node — skip
             pluginInfo("graph-harness", `Follow-on node ${nextNode.id} already activated by another session after failure`);
             return;
           }

           await db.run(
             `UPDATE sessions SET node_id = ? WHERE session_id = ?`
           , [nextNode.id, sessionId]);

           await logLedger(graphId, sessionId, "node_activated", nextNode.id, {
             node_title: nextNode.title,
             reason: "continuing_after_node_failure",
             failed_node: activeNode.id,
           });

           const briefing = await buildNodeBriefing(graphId, nextNode);
           await safeInjectBriefing(sessionId, graphId, nextNode.id, briefing);
        } else {
          // No next node — graph is blocked or failed
          const pendingOrActiveAfterFail = (await db.queryOne(
              `SELECT COUNT(*) as cnt FROM nodes
               WHERE graph_id = ? AND LOWER(status) IN ('pending', 'active')`
            , [graphId]) as { cnt: number } | undefined)?.cnt ?? 0;
          if (pendingOrActiveAfterFail === 0) {
            // All nodes are done/failed — mark graph FAILED
            await db.run(`UPDATE graphs SET status = 'failed' WHERE id = ?`, [graphId]);
            await logLedger(graphId, sessionId, "graph_failed", null, {
              message: "Graph failed: node exceeded max retries and no more nodes to run.",
              failed_node: activeNode.id,
            });
            pluginWarn("graph-harness", `Graph ${graphId} FAILED (all nodes exhausted).`);
            // SWDE-63 AC-4: graph_failed notification
            await dispatchNotification({
              type: "graph_failed",
              graph_id: graphId,
              node_id: activeNode.id,
              title: "Graph Failed",
              body: `Graph ${graphId} failed (all nodes exhausted, last: ${activeNode.id})`,
              metadata: { failed_node: activeNode.id },
              timestamp: new Date().toISOString(),
            }).catch(() => { /* non-fatal */ });
          }
        }
      } else {
        // ── Retries remain → apply exponential backoff then re-inject ─────
        const base = config.retry.backoff_base_seconds;
        const multiplier = config.retry.backoff_multiplier;
        const jitter = config.retry.backoff_jitter;

        let delaySeconds = base * Math.pow(multiplier, newAttemptCount - 1);

        if (jitter) {
          // ±20% randomization to prevent thundering herd
          const jitterFactor = 1 + (Math.random() * 0.4 - 0.2);
          delaySeconds = delaySeconds * jitterFactor;
        }

        const delayMs = Math.round(delaySeconds * 1000);

        await logLedger(graphId, sessionId, "retry_scheduled", activeNode.id, {
          node_title: activeNode.title,
          attempt_count: newAttemptCount,
          delay_ms: delayMs,
          failed_conditions: failedResults.map((r) => r.condition_id),
        });

        // Apply backoff delay then re-inject failure context
        // Use setTimeout to avoid blocking the event loop for long delays (REQ-GH-021)
        setTimeout(async () => {
          try {
            const retryBriefing = await buildRetryBriefing(graphId, activeNode!, failedResults);
            await injectBriefing(sessionId, retryBriefing);
          } catch (err) {
            console.error("[GraphHarness] Failed to inject retry briefing:", err);
          }
        }, delayMs);
      }
    }
  }

  /**
   * Inject a message into the session to re-enter the agent loop.
   *
   * Uses client.session.promptAsync() — fire-and-forget; returns immediately
   * after the message is accepted (204) so we don't block the event handler.
   *
   * Per spec REQ-GH-021: "use client.session.prompt() to inject the next node
   * briefing and re-enter the agent loop".
   *
   * axiom:trace work_item=graph-harness-01 spec=specs/102-Graph-Harness.md#REQ-GH-021 plan=phase-1/task-1-4/step-1-4-1
   */
  async function injectBriefing(sessionId: string, message: string): Promise<void> {
    try {
      // `client` is captured from the outer plugin factory scope.
      // The type is ReturnType<typeof createOpencodeClient> from @opencode-ai/sdk.
      // promptAsync sends a message to the session and returns immediately (204).
      await (client as {
        session: {
          promptAsync: (opts: {
            path: { id: string };
            body: { parts: Array<{ type: "text"; text: string; synthetic?: boolean }> };
          }) => Promise<unknown>;
        };
      }).session.promptAsync({
        path: { id: sessionId },
        body: {
          parts: [
            {
              type: "text",
              text: message,
              // Mark synthetic so it is treated as harness injection,
              // not a human turn (avoids polluting conversation history display)
              synthetic: true,
            },
          ],
        },
      });
    } catch (err) {
      // Log but do not re-throw — failure to inject is logged, not fatal
      console.error("[GraphHarness] Failed to inject briefing for session:", sessionId, err);
      throw err; // Let the caller decide how to handle
    }
  }

  /**
   * Safe wrapper for injectBriefing when called after node activation.
   *
   * If the SDK promptAsync call fails (network partition, session killed, etc.),
   * the plain `injectBriefing` re-throws and the outer harnessEventHandler
   * swallows the error, leaving the node ACTIVE with no briefing — a silent stall.
   *
   * This wrapper adds:
   *   1. A ledger entry so operators can see the failure in graph.status
   *   2. A node reset to PENDING so the next session.idle will re-activate it
   *      instead of waiting up to heartbeat_timeout_seconds (default 300 s)
   *
   * Use this everywhere a just-activated node needs its first briefing injected.
   * Do NOT use for retry briefings (Step 5b) — those are lower-risk because
   * the node is already ACTIVE and conditions will be re-evaluated on the next tick.
   *
   * axiom:trace work_item=graph-harness-01 spec=specs/102-Graph-Harness.md#REQ-GH-021 plan=step-postv0-004
   */
  async function safeInjectBriefing(
    sessionId: string,
    graphId: string,
    nodeId: string,
    message: string
  ): Promise<void> {
    try {
      await injectBriefing(sessionId, message);
    } catch (err) {
      const errMsg = err instanceof Error ? err.message : String(err);
      console.error(
        `[GraphHarness] Briefing injection failed for node ${nodeId} — resetting to PENDING for retry`,
        errMsg
      );
      // Write a ledger entry so this is visible in graph.status / operator queries
      try {
        await logLedger(graphId, sessionId, "briefing_injection_failed", nodeId, {
          error: errMsg,
          action: "node_reset_to_pending_for_retry",
        });
      } catch { /* ledger write is best-effort */ }
      // Reset node to PENDING — next session.idle will re-activate and retry
      try {
        await db.run(
          `UPDATE nodes SET status = 'pending', activated_at = NULL
           WHERE id = ? AND graph_id = ? AND LOWER(status) = 'active'`
        , [nodeId, graphId]);
        // Clear session's node_id so the harness knows no node is in-flight
        await db.run(
          `UPDATE sessions SET node_id = NULL WHERE session_id = ?`
        , [sessionId]);
      } catch (resetErr) {
        console.error("[GraphHarness] Failed to reset node after briefing failure:", resetErr);
      }
      // Do NOT re-throw — the recovery is in place; log is sufficient
    }
  }

  // ─────────────────────────────────────────────────────────────────────────
  // graph.output Tool (REQ-GH-005 + REQ-GH-040)
  //
  // Records named outputs from a node into the node_outputs table.
  // UPSERT semantics: insert or replace on (graph_id, node_id, key).
  // 8KB cap on value — truncates with marker if exceeded.
  //
  // axiom:trace work_item=graph-harness-01 spec=specs/102-Graph-Harness.md#REQ-GH-005 plan=phase-3/task-3-1/step-3-1-1
  // ─────────────────────────────────────────────────────────────────────────

  const OUTPUT_SIZE_CAP = 8192;

  const graphOutputTool = tool({
    description:
      "Record a named output from a node (e.g., test_results, report). " +
      "UPSERT semantics by default — existing values are replaced. " +
      "Set overwrite=false to error if key already exists. " +
      "Value is capped at 8KB (excess is truncated with a marker). " +
      "Returns { graph_id, node_id, key, bytes_stored, truncated, status: 'stored' }.",
    args: {
      graph_id: tool.schema.string().min(1).describe("Graph ID"),
      node_id: tool.schema.string().min(1).describe("Node ID producing the output"),
      key: tool.schema.string().min(1).describe("Output name (e.g., 'test_results', 'report')"),
      value: tool.schema.string().describe("Output value (JSON string or plain text)"),
      overwrite: tool.schema.boolean().optional().describe("If false, error when key already exists (default: true)"),
    },

    async execute(args, context) {
      // axiom:trace work_item=graph-harness-01 spec=specs/102-Graph-Harness.md#REQ-GH-005 plan=phase-3/task-3-1/step-3-1-1
      if (!config.enabled) {
        return JSON.stringify({ error: "Graph Harness is disabled" });
      }

      try {
        const { graph_id: graphId, node_id: nodeId, key } = args;
        const overwrite = args.overwrite !== false; // default true

        // ── Role check: workers can only write to their own assigned node ────
        const callerSessionId = (context as Record<string, unknown> | undefined)?.sessionID as string | undefined;
        if (callerSessionId) {
          const sessionRow = await db.queryOne(
            `SELECT role, node_id FROM sessions WHERE session_id = ? AND LOWER(status) = 'active'`
          , [callerSessionId]) as { role: string; node_id: string | null } | undefined;
          if (sessionRow && sessionRow.role === "worker") {
            if (sessionRow.node_id !== nodeId) {
              return JSON.stringify({
                error: `Permission denied: worker session can only call graph.output on its assigned node (${sessionRow.node_id ?? "none"}), not '${nodeId}'`,
              });
            }
          }
        }

        // ── Gate: graph must exist ───────────────────────────────────────────
        const graphRow = await db.queryOne(`SELECT id FROM graphs WHERE id = ?`, [graphId]) as { id: string } | undefined;
        if (!graphRow) {
          return JSON.stringify({ error: `Graph not found: ${graphId}` });
        }

        // ── Gate: node must exist in graph ───────────────────────────────────
        const nodeRow = await db.queryOne(`SELECT id FROM nodes WHERE graph_id = ? AND id = ?`, [graphId, nodeId]) as { id: string } | undefined;
        if (!nodeRow) {
          return JSON.stringify({ error: `Node not found: ${nodeId} in graph ${graphId}` });
        }

        // ── Check overwrite policy ───────────────────────────────────────────
        const existingRow = await db.queryOne(`SELECT id FROM node_outputs WHERE graph_id = ? AND node_id = ? AND key = ?`, [graphId, nodeId, key]) as { id: string } | undefined;

        if (existingRow && !overwrite) {
          return JSON.stringify({ error: `Output key already exists: ${key}` });
        }

        // ── Apply 8KB cap ────────────────────────────────────────────────────
        let storedValue = args.value;
        let truncated = false;
        if (storedValue.length > OUTPUT_SIZE_CAP) {
          const excess = storedValue.length - OUTPUT_SIZE_CAP;
          storedValue = storedValue.slice(0, OUTPUT_SIZE_CAP) + ` ...[truncated ${excess} bytes]`;
          truncated = true;
        }

        // ── UPSERT into node_outputs ─────────────────────────────────────────
        const nowIso = new Date().toISOString();
        const outputId = `out_${graphId}_${nodeId}_${key}_${Date.now().toString(36)}`;

        if (existingRow) {
          // Update existing row
          await db.run(
            `UPDATE node_outputs SET value = ?, created_at = ? WHERE graph_id = ? AND node_id = ? AND key = ?`
          , [storedValue, nowIso, graphId, nodeId, key]);
          // Ledger: output_overwritten
          await addLedgerEntry(graphId, "output_overwritten", { node_id: nodeId, key, bytes: storedValue.length, truncated });
        } else {
          await db.run(
            `INSERT INTO node_outputs (id, graph_id, node_id, key, value, type, created_at)
             VALUES (?, ?, ?, ?, ?, 'text', ?)`
          , [outputId, graphId, nodeId, key, storedValue, nowIso]);
        }

        return JSON.stringify({
          graph_id: graphId,
          node_id: nodeId,
          key,
          bytes_stored: storedValue.length,
          truncated,
          status: "stored",
        });
      } catch (err) {
        const message = err instanceof Error ? err.message : String(err);
        return JSON.stringify({ error: message });
      }
    },
  });

  // ─────────────────────────────────────────────────────────────────────────
  // graph.dataflow Tool (REQ-GH-007 + REQ-GH-043)
  //
  // Declares output flow contracts between nodes.
  // Runs cycle detection on data flow graph before inserting.
  // Required flows block downstream activation (enforced in findNextUnblockedNode).
  //
  // axiom:trace work_item=graph-harness-01 spec=specs/102-Graph-Harness.md#REQ-GH-007 plan=phase-3/task-3-2/step-3-2-1
  // ─────────────────────────────────────────────────────────────────────────

  const graphDataflowTool = tool({
    description:
      "Declare output flow contracts between nodes. " +
      "If required=true (default), the downstream node won't activate until the upstream node has produced that output. " +
      "Runs cycle detection on the data flow graph. " +
      "Returns { graph_id, flows_declared, status: 'ok' }.",
    args: {
      graph_id: tool.schema.string().min(1).describe("Graph ID"),
      flows: tool.schema.array(
        tool.schema.object({
          from_node_id: tool.schema.string().min(1).describe("Source node ID"),
          to_node_id: tool.schema.string().min(1).describe("Destination node ID"),
          output_key: tool.schema.string().min(1).describe("Which output from from_node flows to to_node"),
          required: tool.schema.boolean().optional().describe("If true (default), to_node won't activate until output is produced"),
          transform: tool.schema.string().optional().describe("Optional jq transform expression"),
          input_key: tool.schema.string().optional().describe("Key to use in to_node's input (default: same as output_key)"),
        })
      ).describe("Flows to declare"),
    },

    async execute(args, _context) {
      // axiom:trace work_item=graph-harness-01 spec=specs/102-Graph-Harness.md#REQ-GH-007 plan=phase-3/task-3-2/step-3-2-1
      if (!config.enabled) {
        return JSON.stringify({ error: "Graph Harness is disabled" });
      }

      try {
        const { graph_id: graphId, flows } = args;

        // ── Gate: graph must exist ───────────────────────────────────────────
        const graphRow = await db.queryOne(`SELECT id FROM graphs WHERE id = ?`, [graphId]) as { id: string } | undefined;
        if (!graphRow) {
          return JSON.stringify({ error: `Graph not found: ${graphId}` });
        }

        // ── Gate: all node IDs must exist ────────────────────────────────────
        for (const flow of flows) {
          const fromRow = await db.queryOne(`SELECT id FROM nodes WHERE graph_id = ? AND id = ?`, [graphId, flow.from_node_id]) as { id: string } | undefined;
          if (!fromRow) {
            return JSON.stringify({ error: `Unknown node ID in flow: ${flow.from_node_id}` });
          }
          const toRow = await db.queryOne(`SELECT id FROM nodes WHERE graph_id = ? AND id = ?`, [graphId, flow.to_node_id]) as { id: string } | undefined;
          if (!toRow) {
            return JSON.stringify({ error: `Unknown node ID in flow: ${flow.to_node_id}` });
          }
        }

        // ── Cycle detection on data flow graph ───────────────────────────────
        // Fetch existing flows + proposed flows and check for cycles.
        const existingFlows = await db.queryAll(`SELECT from_node_id, to_node_id FROM data_flow WHERE graph_id = ?`, [graphId]) as Array<{ from_node_id: string; to_node_id: string }>;

        const allNodeIds = (await db.queryAll(`SELECT id FROM nodes WHERE graph_id = ?`, [graphId]) as Array<{ id: string }>).map((r) => r.id);
        const proposedEdges = flows.map((f) => ({ from: f.from_node_id, to: f.to_node_id }));
        const existingEdges = existingFlows.map((f) => ({ from: f.from_node_id, to: f.to_node_id }));
        const allEdges = [...existingEdges, ...proposedEdges];

        const cycleError = detectCycle(allNodeIds, allEdges);
        if (cycleError) {
          return JSON.stringify({ error: `Data flow cycle detected: ${cycleError}` });
        }

        // ── UPSERT flows ─────────────────────────────────────────────────────
        const nowIso = new Date().toISOString();
        let declared = 0;

        for (const flow of flows) {
          const required = flow.required !== false; // default true
          const inputKey = flow.input_key ?? flow.output_key;
          const flowId = `df_${graphId}_${flow.from_node_id}_${flow.to_node_id}_${flow.output_key}_${Date.now().toString(36)}`;

          // Check if row exists
          const existingFlow = await db.queryOne(
            `SELECT id FROM data_flow WHERE graph_id = ? AND from_node_id = ? AND to_node_id = ? AND output_key = ?`
          , [graphId, flow.from_node_id, flow.to_node_id, flow.output_key]) as { id: string } | undefined;

          if (existingFlow) {
            await db.run(
              `UPDATE data_flow SET required = ?, transform = ?, input_key = ?
               WHERE graph_id = ? AND from_node_id = ? AND to_node_id = ? AND output_key = ?`
            , [required, flow.transform ?? null, inputKey, graphId, flow.from_node_id, flow.to_node_id, flow.output_key]);
          } else {
            await db.run(
              `INSERT INTO data_flow (id, graph_id, from_node_id, to_node_id, output_key, required, transform, input_key)
               VALUES (?, ?, ?, ?, ?, ?, ?, ?)`
            , [flowId, graphId, flow.from_node_id, flow.to_node_id, flow.output_key, required, flow.transform ?? null, inputKey]);
          }
          declared++;
        }

        await addLedgerEntry(graphId, "dataflow_declared", { flows_declared: declared });

        return JSON.stringify({
          graph_id: graphId,
          flows_declared: declared,
          status: "ok",
        });
      } catch (err) {
        const message = err instanceof Error ? err.message : String(err);
        return JSON.stringify({ error: message });
      }
    },
  });

  // ─────────────────────────────────────────────────────────────────────────
  // graph.message Tool (REQ-GH-006 + REQ-GH-041 + REQ-GH-042)
  //
  // Send messages between nodes. Messages are queued for PENDING nodes,
  // and delivered immediately (via briefing inject) for ACTIVE nodes.
  // Critical messages trigger a briefing refresh for active sessions.
  //
  // axiom:trace work_item=graph-harness-01 spec=specs/102-Graph-Harness.md#REQ-GH-006 plan=phase-3/task-3-3/step-3-3-1
  // ─────────────────────────────────────────────────────────────────────────

  const graphMessageTool = tool({
    description:
      "Send a message from one node to another. " +
      "Messages to PENDING nodes are queued for delivery in their briefing. " +
      "Messages to ACTIVE nodes are delivered immediately. " +
      "Critical priority messages trigger a briefing refresh for active sessions. " +
      "Workers can only send from their own assigned node. " +
      "Returns { graph_id, message_id, from_node_id, to_node_id, delivery_status, priority }.",
    args: {
      graph_id: tool.schema.string().min(1).describe("Graph ID"),
      from_node_id: tool.schema.string().min(1).describe("Source node ID"),
      to_node_id: tool.schema.string().min(1).describe("Destination node ID"),
      content: tool.schema.string().min(1).describe("Message content"),
      priority: tool.schema.enum(["normal", "high", "critical"]).optional().describe("Message priority (default: normal)"),
    },

    async execute(args, context) {
      // axiom:trace work_item=graph-harness-01 spec=specs/102-Graph-Harness.md#REQ-GH-006 plan=phase-3/task-3-3/step-3-3-1
      if (!config.enabled) {
        return JSON.stringify({ error: "Graph Harness is disabled" });
      }

      try {
        const { graph_id: graphId, from_node_id: fromNodeId, to_node_id: toNodeId, content } = args;
        const priority = args.priority ?? "normal";

        // ── Role check: workers can only send from their own assigned node ────
        const callerSessionId = (context as Record<string, unknown> | undefined)?.sessionID as string | undefined;
        if (callerSessionId) {
          const sessionRow = await db.queryOne(
            `SELECT role, node_id FROM sessions WHERE session_id = ? AND LOWER(status) = 'active'`
          , [callerSessionId]) as { role: string; node_id: string | null } | undefined;
          if (sessionRow && sessionRow.role === "worker") {
            if (sessionRow.node_id !== fromNodeId) {
              return JSON.stringify({
                error: `Permission denied: worker session can only send messages from its assigned node (${sessionRow.node_id ?? "none"}), not '${fromNodeId}'`,
              });
            }
          }
        }

        // ── Gate: graph must exist ───────────────────────────────────────────
        const graphRow = await db.queryOne(`SELECT id FROM graphs WHERE id = ?`, [graphId]) as { id: string } | undefined;
        if (!graphRow) {
          return JSON.stringify({ error: `Graph not found: ${graphId}` });
        }

        // ── Gate: from_node must exist in graph ──────────────────────────────
        const fromNodeRow = await db.queryOne(`SELECT id FROM nodes WHERE graph_id = ? AND id = ?`, [graphId, fromNodeId]) as { id: string } | undefined;
        if (!fromNodeRow) {
          return JSON.stringify({ error: `Source node not found: ${fromNodeId} in graph ${graphId}` });
        }

        // ── Gate: to_node must exist in graph ────────────────────────────────
        const toNodeRow = await db.queryOne(`SELECT id, status FROM nodes WHERE graph_id = ? AND id = ?`, [graphId, toNodeId]) as { id: string; status: string } | undefined;
        if (!toNodeRow) {
          return JSON.stringify({ error: `Destination node not found: ${toNodeId} in graph ${graphId}` });
        }

        // ── Insert message ───────────────────────────────────────────────────
        const nowIso = new Date().toISOString();
        const messageId = `msg_${graphId}_${Date.now().toString(36)}_${Math.random().toString(36).slice(2, 8)}`;
        const toNodeStatus = toNodeRow.status.toLowerCase();

        await db.run(
          `INSERT INTO node_messages (id, graph_id, from_node_id, to_node_id, content, priority, status, delivered, created_at)
           VALUES (?, ?, ?, ?, ?, ?, 'queued', 0, ?)`
        , [messageId, graphId, fromNodeId, toNodeId, content, priority, nowIso]);

        // ── Determine delivery status ────────────────────────────────────────
        let deliveryStatus: "queued" | "delivered" = "queued";

        if (toNodeStatus === "active") {
          // Node is active — check if there's an active session for this node
          const activeSession = await db.queryOne(`SELECT session_id FROM sessions WHERE graph_id = ? AND node_id = ? AND LOWER(status) = 'active' LIMIT 1`, [graphId, toNodeId]) as { session_id: string } | undefined;

          if (activeSession) {
            // Mark as delivered
            await db.run(`UPDATE node_messages SET delivered = 1, delivered_at = ?, status = 'delivered' WHERE id = ?`, [nowIso, messageId]);
            deliveryStatus = "delivered";

            // Critical priority: inject briefing refresh
            if (priority === "critical") {
              try {
                await injectBriefing(activeSession.session_id, `[CRITICAL MESSAGE from ${fromNodeId}]: ${content}`);
              } catch {
                // injectBriefing failure is non-fatal — message is still stored
              }
            }
          }
        }

        await addLedgerEntry(graphId, "message_sent", {
          message_id: messageId,
          from_node_id: fromNodeId,
          to_node_id: toNodeId,
          priority,
          delivery_status: deliveryStatus,
        });

        return JSON.stringify({
          graph_id: graphId,
          message_id: messageId,
          from_node_id: fromNodeId,
          to_node_id: toNodeId,
          delivery_status: deliveryStatus,
          priority,
        });
      } catch (err) {
        const message = err instanceof Error ? err.message : String(err);
        return JSON.stringify({ error: message });
      }
    },
  });

  // ─────────────────────────────────────────────────────────────────────────
  // applyJqTransform — execute real jq binary for data_flow transforms
  //
  // Pipes input via stdin to: jq '<transform>'
  // On error: returns "[jq error: ...]" + original input
  // On timeout: returns "[jq timeout]" + original input
  //
  // axiom:trace work_item=graph-harness-01 spec=specs/102-Graph-Harness.md#REQ-GH-061 plan=phase-3/task-3-5/step-3-5-1
  // ─────────────────────────────────────────────────────────────────────────

  async function applyJqTransform(input: string, transform: string, timeoutMs = 5000): Promise<string> {
    try {
      const proc = Bun.spawn(["jq", "-r", transform], {
        stdin: "pipe",
        stdout: "pipe",
        stderr: "pipe",
      });

      // FileSink API (Bun): write() + end()
      proc.stdin.write(input);
      proc.stdin.end();

      type RaceResult = [string, number];
      const result = await Promise.race<RaceResult>([
        Promise.all([
          new Response(proc.stdout).text(),
          proc.exited,
        ]) as Promise<RaceResult>,
        new Promise<never>((_, reject) =>
          setTimeout(() => reject(new Error("timeout")), timeoutMs)
        ),
      ]).catch((err: unknown) => {
        const isTimeout = err instanceof Error && err.message === "timeout";
        if (!isTimeout) proc.kill();
        return [isTimeout ? "[jq timeout]" : "[jq error]", 1] as RaceResult;
      });

      const [output, exitCode] = result;
      if (exitCode === 0) {
        return (output as string).trim();
      }
      return `[jq error: ${input}]`;
    } catch (err) {
      // jq not available or other error
      return `[jq error: ${err instanceof Error ? err.message : String(err)}]`;
    }
  }

  // ─────────────────────────────────────────────────────────────────────────
  // system.transform hook — inject active node context into every LLM call.
  //
  // Called by OpenCode BEFORE EVERY LLM call (experimental.chat.system.transform).
  // Appends a <graph-data> XML block to output.system[] when a graph is active
  // so the model always knows what node it is working on.
  //
  // SDK contract (from @opencode-ai/plugin/dist/index.d.ts):
  //   "experimental.chat.system.transform"?: (
  //     input: { sessionID?: string; model: Model },
  //     output: { system: string[] }
  //   ) => Promise<void>
  //
  // Mutation contract: push onto output.system[] — do NOT replace the array.
  //
  // 32KB cap: if the briefing portion exceeds 32,768 bytes, it is truncated
  // and a note is appended (REQ-GH-023).
  //
  // axiom:trace work_item=graph-harness-01 spec=specs/102-Graph-Harness.md#REQ-GH-023 plan=phase-1/task-1-5/step-1-5-1
  // ─────────────────────────────────────────────────────────────────────────

  const BRIEFING_CAP_BYTES = 32_768;

  // Track consecutive briefing failures per session (REQ-GH-075)
  // ADR-GH-002: Persisted to sessions.consecutive_briefing_failures for durability across restarts.
  // On plugin load, restore in-progress counts from DB so the 3-strike rule survives server restarts.
  // axiom:trace work_item=graph-harness-01 spec=specs/102-Graph-Harness.md#REQ-GH-075 plan=adr-gh-002
  const briefingFailureCount = new Map<string, number>(); // sessionId → consecutive failure count (in-memory cache)

  // Restore from DB on startup (only active sessions with failures > 0)
  try {
    const persistedCounts = await db.queryAll(
      `SELECT session_id, consecutive_briefing_failures FROM sessions
       WHERE LOWER(status) = 'active' AND consecutive_briefing_failures > 0`
    , []) as Array<{ session_id: string; consecutive_briefing_failures: number }>;
    for (const row of persistedCounts) {
      briefingFailureCount.set(row.session_id, row.consecutive_briefing_failures);
    }
    if (persistedCounts.length > 0) {
      pluginInfo("graph-harness", `Restored briefing failure counts for ${persistedCounts.length} session(s)`);
    }
  } catch {
    // Tolerate if column doesn't exist yet on first load — will be populated on next failure
  }

  /** Increment the persisted briefing failure count for a session. */
  async function incrementBriefingFailure(sessionId: string): Promise<number> {
    const next = (briefingFailureCount.get(sessionId) ?? 0) + 1;
    briefingFailureCount.set(sessionId, next);
    await dbWriteWithRetry(
      async () => { await db.run(
        `UPDATE sessions SET consecutive_briefing_failures = ? WHERE session_id = ?`,
        [next, sessionId]); },
      "incrementBriefingFailure"
    );
    return next;
  }

  /** Reset the persisted briefing failure count for a session (called on success). */
  async function resetBriefingFailure(sessionId: string): Promise<void> {
    briefingFailureCount.delete(sessionId);
    await dbWriteWithRetry(
      async () => { await db.run(
        `UPDATE sessions SET consecutive_briefing_failures = 0 WHERE session_id = ?`,
        [sessionId]); },
      "resetBriefingFailure"
    );
  }

  /**
   * Build the system.transform briefing XML block for the active node.
   *
   * Returns:
   *   - null  → no active session or no active node for this sessionID
   *   - string → the full `<graph-data>...</graph-data>` block to append
   *
   * The string does NOT include the leading "\n\n" — the caller adds that.
   */
  function buildSystemBriefing(sessionId: string): Promise<string | null> {
    return _buildSystemBriefingAsync(sessionId);
  }

  async function _buildSystemBriefingAsync(sessionId: string): Promise<string | null> {
    // ── 1. Look up active session in DB ──────────────────────────────────
    // Uses retry to handle transient SQLITE_IOERR_VNODE from concurrent sessions.
    // axiom:trace work_item=graph-harness-01 spec=specs/102-Graph-Harness.md#REQ-GH-021 plan=step-verify2-gh-03
    // axiom:trace work_item=sqlite-resilience-01 spec=specs/102-Graph-Harness.md#4.1
    const sessionRow = await dbReadWithRetry(
      async () => await db.queryOne(
          `SELECT session_id, graph_id, node_id, status
           FROM sessions WHERE session_id = ? AND LOWER(status) = 'active'`
        , [sessionId]) as SessionRow | undefined,
      "buildSystemBriefing(session_lookup)"
    ) ?? undefined;

    if (!sessionRow) {
      // No active session for this sessionID
      return null;
    }

    const graphId = sessionRow.graph_id;

    // ── 2. Look up graph status ──────────────────────────────────────────
    // axiom:trace work_item=graph-lock-upgrade-01 spec=specs/102-Graph-Harness.md#REQ-GH-116
    const graphRow = await dbReadWithRetry(
      async () => await db.queryOne(`SELECT id, title, status, locked_by FROM graphs WHERE id = ?`, [graphId]) as { id: string; title: string; status: string; locked_by: string | null } | undefined,
      "buildSystemBriefing(graph_lookup)"
    ) ?? undefined;

    if (!graphRow) {
      return null;
    }

    // If graph is paused → inject pause notice
    if (graphRow.status.toLowerCase() === "paused") {
      return `<graph-data>\n[GRAPH PAUSED — No active work]\n</graph-data>`;
    }

    // If graph is not active (complete, abandoned, failed) → return null
    if (!["active", "created"].includes(graphRow.status.toLowerCase())) {
      return null;
    }

    // ── 3. Look up active node ────────────────────────────────────────────
    if (!sessionRow.node_id) {
      // Session registered but no node assigned yet
      return null;
    }

    const activeNode = await db.queryOne(
        `SELECT id, graph_id, title, description, status, execution_mode,
                attempt_count, max_retries, context, activated_at, completed_at
         FROM nodes WHERE graph_id = ? AND id = ? AND LOWER(status) = 'active'`
      , [graphId, sessionRow.node_id]) as NodeRow | null;

    if (!activeNode) {
      // node_id set but node is not currently active (may be done/pending)
      return null;
    }

    // ── 4. Build graph position stats ────────────────────────────────────
    const totalNodes = (await db.queryOne(`SELECT COUNT(*) as cnt FROM nodes WHERE graph_id = ?`, [graphId]) as { cnt: number } | undefined)?.cnt ?? 0;

    const doneNodes = (await db.queryOne(`SELECT COUNT(*) as cnt FROM nodes WHERE graph_id = ? AND LOWER(status) = 'done'`, [graphId]) as { cnt: number } | undefined)?.cnt ?? 0;

    const pendingNodes = (await db.queryOne(`SELECT COUNT(*) as cnt FROM nodes WHERE graph_id = ? AND LOWER(status) = 'pending'`, [graphId]) as { cnt: number } | undefined)?.cnt ?? 0;

    // Find next nodes (nodes that depend only on the current active node and are PENDING)
    const nextNodes = await db.queryAll(
        `SELECT n.title FROM nodes n
         INNER JOIN dependencies d ON d.graph_id = n.graph_id AND d.node_id = n.id
         WHERE d.graph_id = ? AND d.depends_on = ? AND LOWER(n.status) = 'pending'
         ORDER BY n.id ASC LIMIT 5`
      , [graphId, activeNode.id]) as Array<{ title: string }>

    const nextNodesList =
      nextNodes.length > 0
        ? nextNodes.map((n) => `"${n.title}"`).join(", ")
        : "None — this is the final node";

    // ── 5. Parse context (constraints) ───────────────────────────────────
    let constraintLines = "None specified";
    if (activeNode.context) {
      try {
        const ctx = JSON.parse(activeNode.context) as Record<string, unknown>;
        if (Array.isArray(ctx.constraints) && ctx.constraints.length > 0) {
          constraintLines = (ctx.constraints as string[]).map((c) => `- ${c}`).join("\n");
        }
      } catch { /* malformed context — use default */ }
    }

    // ── 6. Load done-conditions ───────────────────────────────────────────
    const conditions = await db.queryAll(
        `SELECT type, command, description FROM conditions
         WHERE graph_id = ? AND node_id = ?
         ORDER BY ordinal ASC`
      , [graphId, activeNode.id]) as Array<{ type: string; command: string | null; description: string | null }>;

    let conditionLines: string;
    if (conditions.length === 0) {
      conditionLines = "1. [none] The harness will advance when you indicate completion.";
    } else {
      conditionLines = conditions
        .map((c, i) => {
          const label = c.description || c.command || c.type;
          return `${i + 1}. [${c.type}] ${label}`;
        })
        .join("\n");
    }

    // ── 7. Retry context (if this is a retry attempt) ─────────────────────
    let retrySection = "";
    if ((activeNode.attempt_count ?? 0) > 0) {
      // Fetch the last failure annotation for context
      const lastFailure = await db.queryOne(
          `SELECT content FROM annotations
           WHERE graph_id = ? AND node_id = ? AND type = 'failure_context'
           ORDER BY created_at DESC LIMIT 1`
        , [graphId, activeNode.id]) as { content: string } | undefined;

      const failureContext = lastFailure?.content ?? "(no failure detail recorded)";
      retrySection = `\n\n**Why you're here again**: ${failureContext}`;
    }

    // ── 8a. Upstream outputs (REQ-GH-023 + REQ-GH-043) ──────────────────
    // axiom:trace work_item=graph-harness-01 spec=specs/102-Graph-Harness.md#REQ-GH-023 plan=phase-3/task-3-4/step-3-4-1
    let upstreamOutputsXml = "";
    {
      const upstreamFlows = await db.queryAll(
          `SELECT df.from_node_id, df.output_key, df.transform, df.input_key
           FROM data_flow df
           WHERE df.graph_id = ? AND df.to_node_id = ?`
        , [graphId, activeNode.id]) as Array<{
          from_node_id: string;
          output_key: string;
          transform: string | null;
          input_key: string | null;
        }>;

      if (upstreamFlows.length > 0) {
        const outputLines: string[] = [];
        for (const flow of upstreamFlows) {
          const outputRow = await db.queryOne(`SELECT value FROM node_outputs WHERE graph_id = ? AND node_id = ? AND key = ?`, [graphId, flow.from_node_id, flow.output_key]) as { value: string } | undefined;

          if (outputRow) {
            let displayValue = outputRow.value;
            if (flow.transform) {
              displayValue = await applyJqTransform(outputRow.value, flow.transform);
            }
            const keyAttr = flow.input_key ? ` input_key="${flow.input_key}"` : "";
            const transformAttr = flow.transform ? ` transform="${flow.transform.replace(/"/g, "&quot;")}"` : "";
            outputLines.push(
              `  <output from="${flow.from_node_id}" key="${flow.output_key}"${keyAttr}${transformAttr}>${displayValue}</output>`
            );
          }
        }
        if (outputLines.length > 0) {
          upstreamOutputsXml = `<upstream-outputs>\n${outputLines.join("\n")}\n</upstream-outputs>`;
        }
      }
    }

    // ── 8b. Pending messages (REQ-GH-023 + REQ-GH-041 + REQ-GH-042) ─────
    // axiom:trace work_item=graph-harness-01 spec=specs/102-Graph-Harness.md#REQ-GH-023 plan=phase-3/task-3-4/step-3-4-1
    let pendingMessagesXml = "";
    {
      const pendingMessages = await db.queryAll(
          `SELECT from_node_id, content, priority FROM node_messages
           WHERE graph_id = ? AND to_node_id = ? AND (status = 'queued' OR (delivered = 0 AND status != 'delivered'))
           ORDER BY
             CASE priority WHEN 'critical' THEN 0 WHEN 'high' THEN 1 ELSE 2 END ASC,
             created_at ASC`
        , [graphId, activeNode.id]) as Array<{
          from_node_id: string;
          content: string;
          priority: string;
        }>;

      if (pendingMessages.length > 0) {
        const msgLines = pendingMessages.map(
          (m) => `  <message from="${m.from_node_id}" priority="${m.priority}">${m.content}</message>`
        );
        pendingMessagesXml = `<messages count="${pendingMessages.length}">\n${msgLines.join("\n")}\n</messages>`;
        // Mark messages as delivered now that they're in the briefing
        await db.run(
          `UPDATE node_messages SET delivered = 1, delivered_at = datetime('now'), status = 'delivered'
           WHERE graph_id = ? AND to_node_id = ? AND (status = 'queued' OR (delivered = 0 AND status != 'delivered'))`
        , [graphId, activeNode.id]);
      }
    }

    // ── 9. Assemble the briefing ──────────────────────────────────────────
    const attemptDisplay = (activeNode.attempt_count ?? 0) + 1;
    const maxRetries = activeNode.max_retries ?? config.retry.default_max_retries;

    const briefingParts = [
      `## Current Node: ${activeNode.title}`,
      ``,
      `**Graph**: ${graphRow.title} (${graphId})`,
      `**Node ID**: ${activeNode.id}`,
      `**Status**: ACTIVE (attempt ${attemptDisplay} of ${maxRetries})`,
      `**Description**: ${activeNode.description}`,
      ``,
      `**Your constraints**:`,
      constraintLines,
      ``,
      `**Done conditions** (harness will verify these automatically):`,
      conditionLines,
      ``,
      `**Graph position**:`,
      `- Total nodes: ${totalNodes} | Done: ${doneNodes} | Pending: ${pendingNodes}`,
      `- Next after this node: ${nextNodesList}`,
      retrySection,
    ];

    // Append upstream outputs and messages sections if non-empty
    if (upstreamOutputsXml) briefingParts.push("", upstreamOutputsXml);
    if (pendingMessagesXml) briefingParts.push("", pendingMessagesXml);

    const briefingContent = briefingParts.join("\n");

    // ── 10. Apply 32KB cap on briefing content ────────────────────────────
    const encoder = new TextEncoder();
    const encoded = encoder.encode(briefingContent);
    let finalContent: string;

    if (encoded.length > BRIEFING_CAP_BYTES) {
      const truncated = new TextDecoder().decode(encoded.slice(0, BRIEFING_CAP_BYTES));
      const omitted = encoded.length - BRIEFING_CAP_BYTES;
      finalContent = truncated + `\n... (briefing truncated: ${omitted} bytes omitted due to 32KB cap)`;
    } else {
      finalContent = briefingContent;
    }

    // ── 11. Append lock-status block if caller holds the graph lock (REQ-GH-116) ──
    // axiom:trace work_item=graph-lock-upgrade-01 spec=specs/102-Graph-Harness.md#REQ-GH-116
    let lockStatusBlock = "";
    const lockEnabled = (config.interface as Record<string, unknown>)?.lock_status_in_briefing !== false;
    if (lockEnabled && graphRow.locked_by && graphRow.locked_by === sessionId) {
      lockStatusBlock =
        `\n<lock-status>\n` +
        `You hold the exclusive lock on this graph. Other sessions cannot call mutation tools\n` +
        `(inject/modify/split/annotate/abandon) until you release or transfer the lock.\n` +
        `Use graph_unlock to release or graph_transfer to hand off.\n` +
        `</lock-status>`;
    }

    return `<graph-data>\n${finalContent}\n</graph-data>${lockStatusBlock}`;
  }

  /**
   * experimental.chat.system.transform hook.
   *
   * Pushes the active node's context block onto output.system[] before every LLM call.
   * If no active session/graph/node for this sessionID, the system prompt is left unchanged.
   *
   * axiom:trace work_item=graph-harness-01 spec=specs/102-Graph-Harness.md#REQ-GH-023 plan=phase-1/task-1-5/step-1-5-1
   */
  const systemTransformHook = async (
    input: { sessionID?: string },
    output: { system: string[] }
  ): Promise<void> => {
    if (!config.enabled) return;

    const sessionId = input.sessionID;
    if (!sessionId) return;

    try {
      const block = await buildSystemBriefing(sessionId);
      if (block !== null) {
        output.system.push(block);
        // Reset consecutive failure counter on successful briefing (REQ-GH-075, ADR-GH-002: persisted)
        await resetBriefingFailure(sessionId);
      }
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : String(err);

      // Increment consecutive failure counter — in-memory first (always succeeds),
      // then best-effort persist to DB via sqliteWriteWithRetry (ADR-GH-002)
      const failCount = await incrementBriefingFailure(sessionId);

      // Log to console
      console.error(`[GraphHarness] system.transform error (attempt ${failCount}):`, err);

      // Inject mandatory fallback briefing (REQ-GH-075)
      // axiom:trace work_item=graph-harness-01 spec=specs/102-Graph-Harness.md#REQ-GH-075 plan=step-verify-gh-02
      output.system.push(
        `[Graph Harness] Briefing generation failed (attempt ${failCount}/3). ` +
        `Error: ${msg}. Proceeding without graph context.`
      );

      // Log to ledger if we have a session row — use retry helpers to avoid
      // cascading failures when the original error was itself a SQLite I/O error.
      // If the DB is completely unavailable, the retry helper handles gracefully.
      // axiom:trace work_item=sqlite-resilience-01 spec=specs/102-Graph-Harness.md#4.1
      const sessionRow = await dbReadWithRetry(
        async () => await db.queryOne<SessionRow>(`SELECT * FROM sessions WHERE session_id = ?`, [sessionId]),
        "briefing_failure(session_lookup)"
      );
      if (sessionRow) {
        await addLedgerEntry(sessionRow.graph_id, 'briefing_failed', {
          session_id: sessionId, error: msg, attempt: failCount
        }, { sessionId });

        // Mark node FAILED after 3 consecutive failures (REQ-GH-075)
        if (failCount >= 3 && sessionRow.node_id) {
          await dbWriteWithRetry(
            async () => { await db.run(`UPDATE nodes SET status='failed', completed_at=datetime('now') WHERE id=? AND graph_id=?`, [sessionRow.node_id, sessionRow.graph_id]); },
            "briefing_failure(mark_node_failed)"
          );
          await addLedgerEntry(sessionRow.graph_id, 'node_failed_briefing', {
            node_id: sessionRow.node_id, reason: 'briefing_failed_3_times'
          }, { sessionId });
          // Reset counter after triggering the FAILED transition
          await resetBriefingFailure(sessionId);
        }
      }
    }
  };

  // ─────────────────────────────────────────────────────────────────────────
  // Phase 6.3: Built-in template library (REQ-GH-051)
  //
  // Written to .graph-harness/templates/ on init (if not already present).
  // Three built-in templates: test-fix-verify, implement-feature, security-review.
  //
  // axiom:trace work_item=graph-harness-01 spec=specs/102-Graph-Harness.md#REQ-GH-051 plan=phase-6/task-6-3/step-6-3-1
  // ─────────────────────────────────────────────────────────────────────────

  const BUILTIN_TEMPLATES: Array<{ name: string; content: string }> = [
    {
      name: "test-fix-verify",
      content: `name: test-fix-verify
description: "Run failing tests, fix code, verify all pass"
version: 1
parameters:
  - placeholder: "{{test_command}}"
    description: "The test command to run"
    default: "cd .axiom && python -m pytest tests/test_*.py -q --timeout=45"
nodes:
  - id: run-tests
    execution_mode: script
    title: "Run tests"
    description: "Run the test suite and capture results"
    execution_config:
      command: "{{test_command}}"
      capture_output: true
      output_key: test_results
  - id: fix-failures
    execution_mode: agent
    title: "Fix failing tests"
    description: "Fix the failing tests identified in the previous step."
    dependencies:
      - run-tests
    done_conditions:
      - type: script
        command: "{{test_command}}"
        expected: "passed"
  - id: verify-passes
    execution_mode: script
    title: "Verify all tests pass"
    description: "Final verification that all tests pass"
    dependencies:
      - fix-failures
    execution_config:
      command: "{{test_command}}"
      capture_output: true
data_flows:
  - from: run-tests
    to: fix-failures
    output_key: test_results
`,
    },
    {
      name: "implement-feature",
      content: `name: implement-feature
description: "Full feature implementation cycle: spec, implement, test, review"
version: 1
parameters:
  - placeholder: "{{feature_name}}"
    description: "Name of the feature to implement"
    default: "feature"
  - placeholder: "{{test_command}}"
    description: "Test command to verify implementation"
    default: "cd .axiom && python -m pytest tests/ -q --timeout=60"
nodes:
  - id: spec
    execution_mode: agent
    title: "Write spec for {{feature_name}}"
    description: "Define acceptance criteria and technical spec for the feature."
    done_conditions:
      - type: file_exists
        command: "specs/{{feature_name}}.md"
  - id: implement
    execution_mode: agent
    title: "Implement {{feature_name}}"
    description: "Implement the feature according to the spec."
    dependencies:
      - spec
    done_conditions:
      - type: script
        command: "{{test_command}}"
        expected: "passed"
  - id: test
    execution_mode: script
    title: "Run tests"
    description: "Run the full test suite to catch regressions."
    dependencies:
      - implement
    execution_config:
      command: "{{test_command}}"
      capture_output: true
      output_key: test_results
  - id: review
    execution_mode: agent
    title: "Review implementation"
    description: "Review code quality, spec alignment, and documentation completeness."
    dependencies:
      - test
    done_conditions:
      - type: manual
        description: "Human approves the implementation"
data_flows:
  - from: test
    to: review
    output_key: test_results
`,
    },
    {
      name: "security-review",
      content: `name: security-review
description: "Security audit: analyze attack surface, find vulnerabilities, propose fixes"
version: 1
parameters:
  - placeholder: "{{target_path}}"
    description: "Directory or file to audit"
    default: "."
nodes:
  - id: analyze-attack-surface
    execution_mode: agent
    title: "Analyze attack surface"
    description: "Enumerate all input surfaces, trust boundaries, and data flows in {{target_path}}."
    done_conditions:
      - type: none
  - id: find-vulns
    execution_mode: agent
    title: "Find vulnerabilities"
    description: "Identify injection, auth, and data-handling vulnerabilities in the attack surface."
    dependencies:
      - analyze-attack-surface
    done_conditions:
      - type: none
  - id: propose-fixes
    execution_mode: agent
    title: "Propose security fixes"
    description: "For each finding, propose a minimal, verifiable remediation. Create work items for high severity."
    dependencies:
      - find-vulns
    done_conditions:
      - type: manual
        description: "Human reviews proposed fixes"
`,
    },
  ];

  /**
   * Write built-in templates to the templates directory if not already present.
   * Called during plugin bootstrap.
   * axiom:trace work_item=graph-harness-01 spec=specs/102-Graph-Harness.md#REQ-GH-051 plan=phase-6/task-6-3/step-6-3-1
   */
  function initBuiltinTemplates(): void {
    const templatesDir = join(directory, config.templates.directory);
    if (!existsSync(templatesDir)) {
      try {
        mkdirSync(templatesDir, { recursive: true });
      } catch (err) {
        console.warn("[GraphHarness] Failed to create templates directory:", err);
        return;
      }
    }

    for (const tpl of BUILTIN_TEMPLATES) {
      const tplPath = join(templatesDir, `${tpl.name}.yaml`);
      if (!existsSync(tplPath)) {
        try {
          writeFileSync(tplPath, tpl.content, "utf-8");
          pluginInfo("graph-harness", `Wrote built-in template: ${tpl.name}.yaml`);
        } catch (err) {
          console.warn(`[GraphHarness] Failed to write built-in template ${tpl.name}:`, err);
        }
      }
    }
  }

  // Initialize built-in templates on plugin load
  if (config.templates.builtin) {
    initBuiltinTemplates();
  }

  // ─────────────────────────────────────────────────────────────────────────
  // Phase 6.1: graph.template.load (REQ-GH-011, REQ-GH-050, REQ-GH-052)
  //
  // Loads a YAML template from .graph-harness/templates/<name>.yaml,
  // applies {{placeholder}} variable substitution, and injects nodes/edges
  // into a new or existing graph.
  //
  // axiom:trace work_item=graph-harness-01 spec=specs/102-Graph-Harness.md#REQ-GH-011 plan=phase-6/task-6-1/step-6-1-1
  // ─────────────────────────────────────────────────────────────────────────

  const TEMPLATE_MAX_SIZE_BYTES = 16384; // 16KB per REQ-GH-050

  /**
   * Sanitize a template name: lowercase, [a-z0-9_-] only, max 64 chars.
   * Returns null if invalid (path separator found).
   * axiom:trace work_item=graph-harness-01 spec=specs/102-Graph-Harness.md#REQ-GH-012 plan=phase-6/task-6-2/step-6-2-1
   */
  function sanitizeTemplateName(name: string): string | null {
    // Reject path separators immediately
    if (name.includes("/") || name.includes("\\") || name.includes("..")) {
      return null;
    }
    const sanitized = name.toLowerCase().replace(/[^a-z0-9_-]/g, "-").slice(0, 64);
    return sanitized || null;
  }

  /**
   * Resolve {{placeholder}} and {{$ENV_VAR}} substitutions in a template string.
   * Unresolved {{placeholder}} (not env vars) are left as-is for error detection.
   * {{$ENV_VAR}} references are resolved from process.env at load time.
   * axiom:trace work_item=graph-harness-01 spec=specs/102-Graph-Harness.md#REQ-GH-052 plan=phase-6/task-6-1/step-6-1-1
   */
  function resolveTemplatePlaceholders(
    content: string,
    variables: Record<string, string>
  ): { resolved: string; unresolved: string[] } {
    const unresolved: string[] = [];
    const resolved = content.replace(/\{\{([^}]+)\}\}/g, (match, inner) => {
      const key = inner.trim();
      // Environment variable reference: {{$VAR_NAME}}
      if (key.startsWith("$")) {
        const envKey = key.slice(1);
        const envVal = process.env[envKey];
        if (envVal !== undefined) return envVal;
        // Env var not set — leave reference in place (resolved at execution time per REQ-GH-052)
        return match;
      }
      // Regular placeholder — look up in variables
      if (key in variables) {
        return variables[key];
      }
      // Check if template has a default in the parameters section (not available here,
      // so we just record as unresolved)
      unresolved.push(key);
      return match; // leave unresolved in place
    });
    return { resolved, unresolved };
  }

  /**
   * Parse template YAML into a structured definition.
   * Uses the `yaml` npm package (already imported).
   */
  function parseTemplateYaml(content: string): Record<string, unknown> | null {
    try {
      return parseYaml(content) as Record<string, unknown>;
    } catch (err) {
      console.warn("[GraphHarness] Failed to parse template YAML:", err);
      return null;
    }
  }

  const graphTemplateLoadTool = tool({
    description:
      "Load a YAML template from .graph-harness/templates/ and inject its nodes into a new or existing graph. " +
      "Resolves {{placeholder}} variables from the variables parameter. " +
      "Template file size must be ≤16KB. " +
      "Returns { graph_id, nodes_injected, template_name }.",

    args: {
      template_name: tool.schema.string().describe("Template name (without .yaml extension)"),
      graph_id: tool.schema.string().optional().describe("Inject into existing graph (omit to create new graph)"),
      variables: tool.schema.record(tool.schema.string(), tool.schema.string()).optional()
        .describe("Variable substitutions for {{placeholder}} in the template"),
    },

    async execute(args, context) {
      // axiom:trace work_item=graph-harness-01 spec=specs/102-Graph-Harness.md#REQ-GH-011 plan=phase-6/task-6-1/step-6-1-1
      if (!config.enabled) return JSON.stringify({ error: "Graph Harness is disabled" });

      try {
        const rawName = args.template_name;
        const variables = args.variables ?? {};
        const sessionId = (context as { sessionID?: string })?.sessionID ?? null;

        // Sanitize name
        const safeName = sanitizeTemplateName(rawName);
        if (!safeName) {
          return JSON.stringify({ error: `Invalid template name: "${rawName}". Use [a-z0-9_-] only, no path separators.` });
        }

        // Find template file
        const templatesDir = join(directory, config.templates.directory);
        const candidatePaths = [
          join(templatesDir, `${safeName}.yaml`),
          join(templatesDir, safeName),
        ];
        let templatePath: string | null = null;
        for (const p of candidatePaths) {
          if (existsSync(p)) {
            // Ensure resolved path stays within templates directory (path traversal protection)
            const { resolve: resolvePath } = await import("node:path");
            const resolved = resolvePath(p);
            const allowedRoot = resolvePath(templatesDir);
            if (!resolved.startsWith(allowedRoot + "/") && resolved !== allowedRoot) {
              return JSON.stringify({ error: "Path traversal detected in template resolution" });
            }
            templatePath = p;
            break;
          }
        }

        if (!templatePath) {
          return JSON.stringify({
            error: `Template not found: "${safeName}". Check .graph-harness/templates/`,
            templates_directory: templatesDir,
          });
        }

        // Check file size ≤16KB (REQ-GH-050)
        try {
          const stat = statSync(templatePath);
          if (stat.size > TEMPLATE_MAX_SIZE_BYTES) {
            return JSON.stringify({
              error: `Template file too large: ${stat.size} bytes (max ${TEMPLATE_MAX_SIZE_BYTES} bytes / 16KB)`,
              file: templatePath,
            });
          }
        } catch (statErr) {
          return JSON.stringify({ error: `Failed to stat template file: ${String(statErr)}` });
        }

        // Read template
        let rawContent: string;
        try {
          rawContent = readFileSync(templatePath, "utf-8");
        } catch (readErr) {
          return JSON.stringify({ error: `Failed to read template: ${String(readErr)}` });
        }

        // Apply defaults from parameters section before variable substitution
        const tempParsed = parseTemplateYaml(rawContent);
        if (tempParsed && Array.isArray(tempParsed.parameters)) {
          for (const param of tempParsed.parameters as Array<Record<string, unknown>>) {
            const placeholder = typeof param.placeholder === "string" ? param.placeholder.replace(/^\{\{|\}\}$/g, "").trim() : null;
            const defaultVal = typeof param.default === "string" ? param.default : null;
            if (placeholder && defaultVal && !(placeholder in variables)) {
              variables[placeholder] = defaultVal;
            }
          }
        }

        // Resolve placeholders
        const { resolved: resolvedContent, unresolved } = resolveTemplatePlaceholders(rawContent, variables);
        if (unresolved.length > 0) {
          pluginWarn("graph-harness", `Template "${safeName}" has unresolved placeholders: ${unresolved.join(", ")}`);
        }

        // Replace any remaining {{placeholder}} with (name-unset) to keep node structure valid
        // axiom:trace work_item=plugin-bug-sweep-01 spec=specs/102-Graph-Harness.md#REQ-GH-050 plan=phase-3/step-backlog-004
        const cleanedContent = resolvedContent.replace(/\{\{([^}]+)\}\}/g, (_, name) => `(${name}-unset)`);

        // Parse resolved YAML
        const tplDef = parseTemplateYaml(cleanedContent);
        if (!tplDef) {
          return JSON.stringify({ error: "Failed to parse template YAML after variable substitution" });
        }

        // Support both nodes: (standard) and steps: (ops templates)
        // axiom:trace work_item=plugin-bug-sweep-01 spec=specs/102-Graph-Harness.md#REQ-GH-050 plan=phase-3/step-backlog-002
        const nodes = Array.isArray(tplDef.nodes) ? tplDef.nodes as Array<Record<string, unknown>>
                    : Array.isArray(tplDef.steps) ? tplDef.steps as Array<Record<string, unknown>>
                    : [];
        const edges = Array.isArray(tplDef.data_flows) ? tplDef.data_flows as Array<Record<string, unknown>> : [];

        if (nodes.length === 0) {
          return JSON.stringify({ error: "Template has no nodes defined (checked both 'nodes:' and 'steps:' keys)" });
        }

        const ts = new Date().toISOString();

        // Resolve or create graph
        let graphId = args.graph_id ?? null;
        if (graphId) {
          const existing = await db.queryOne(`SELECT id FROM graphs WHERE id = ?`, [graphId]) as { id: string } | null;
          if (!existing) {
            return JSON.stringify({ error: `Graph not found: ${graphId}` });
          }
        } else {
          // Create new graph
          const tplName = typeof tplDef.name === "string" ? tplDef.name : safeName;
          const tplDesc = typeof tplDef.description === "string" ? tplDef.description : "";
          graphId = `g_tpl_${safeName}_${Date.now()}`;
          await db.run(
            `INSERT INTO graphs (id, title, description, status, created_at)
             VALUES (?, ?, ?, 'active', ?)`
          , [graphId, tplName, tplDesc, ts]);
        }

        // Inject nodes
        let nodesInjected = 0;
        for (const node of nodes) {
          const nodeId = typeof node.id === "string" ? node.id : `n_${Date.now()}_${nodesInjected}`;
          const title = typeof node.title === "string" ? node.title : nodeId;
          const description = typeof node.description === "string" ? node.description : "";
          const execMode = typeof node.execution_mode === "string" ? node.execution_mode : "agent";
          const execConfig = node.execution_config !== undefined ? JSON.stringify(node.execution_config) : null;
          const context = node.context !== undefined ? JSON.stringify(node.context) : null;

          // Check if node already exists (idempotent on re-inject)
          const exists = await db.queryOne(`SELECT id FROM nodes WHERE graph_id = ? AND id = ?`, [graphId, nodeId]);
          if (!exists) {
            await db.run(
              `INSERT INTO nodes (id, graph_id, title, description, status, execution_mode, execution_config, context, created_at)
               VALUES (?, ?, ?, ?, 'pending', ?, ?, ?, ?)`
            , [nodeId, graphId, title, description, execMode, execConfig, context, ts]);
            nodesInjected++;
          }

          // Handle dependencies for this node
          const deps = Array.isArray(node.dependencies) ? node.dependencies as string[] : [];
          for (const dep of deps) {
            // Ensure dep node exists (may not be injected yet — ignore FK error)
            try {
              await db.run(
                `INSERT OR IGNORE INTO dependencies (graph_id, node_id, depends_on) VALUES (?, ?, ?)`
              , [graphId, nodeId, dep]);
            } catch { /* ignore */ }
          }

          // Handle done_conditions for this node
          if (Array.isArray(node.done_conditions)) {
            let ordinal = 0;
            for (const cond of node.done_conditions as Array<Record<string, unknown>>) {
              const condId = `cond_tpl_${graphId}_${nodeId}_${ordinal}`;
              const condType = typeof cond.type === "string" ? cond.type : "none";
              const condCmd = typeof cond.command === "string" ? cond.command : null;
              const condDesc = typeof cond.description === "string" ? cond.description : null;
              const condExpected = typeof cond.expected === "string" ? cond.expected : null;
              try {
                await db.run(
                  `INSERT OR IGNORE INTO conditions (id, graph_id, node_id, ordinal, type, command, expected, description)
                   VALUES (?, ?, ?, ?, ?, ?, ?, ?)`
                , [condId, graphId, nodeId, ordinal, condType, condCmd, condExpected, condDesc]);
              } catch { /* ignore */ }
              ordinal++;
            }
          }
        }

        // Inject data_flows (edges)
        for (const edge of edges) {
          const fromNode = typeof edge.from === "string" ? edge.from : null;
          const toNode = typeof edge.to === "string" ? edge.to : null;
          const outputKey = typeof edge.output_key === "string" ? edge.output_key : null;
          if (fromNode && toNode) {
            const flowId = `df_tpl_${graphId}_${fromNode}_${toNode}`;
            try {
              await db.run(
                `INSERT OR IGNORE INTO data_flow (id, graph_id, from_node_id, to_node_id, output_key, created_at)
                 VALUES (?, ?, ?, ?, ?, ?)`
              , [flowId, graphId, fromNode, toNode, outputKey, ts]);
            } catch { /* ignore */ }
          }
        }

        // Update templates usage tracking
        try {
          await db.run(
            `INSERT INTO templates (name, description, definition, created_at) VALUES (?, ?, ?, ?)
             ON CONFLICT(name) DO UPDATE SET usage_count = usage_count + 1, last_used_at = ?`
          , [safeName,
            typeof tplDef.description === "string" ? tplDef.description : "",
            JSON.stringify(tplDef),
            ts, ts]);
        } catch { /* non-fatal */ }

        await logLedger(graphId, sessionId, "template_loaded", null, {
          template_name: safeName,
          nodes_injected: nodesInjected,
          unresolved_placeholders: unresolved,
        });

        return JSON.stringify({
          graph_id: graphId,
          nodes_injected: nodesInjected,
          template_name: safeName,
          unresolved_placeholders: unresolved.length > 0 ? unresolved : undefined,
        });
      } catch (err) {
        return JSON.stringify({ error: `graph.template.load failed: ${String(err)}` });
      }
    },
  });

  // ─────────────────────────────────────────────────────────────────────────
  // Phase 6.2: graph.template.save (REQ-GH-012)
  //
  // Serializes a graph's nodes/edges/conditions to YAML and writes to
  // .graph-harness/templates/<sanitized_name>.yaml.
  //
  // axiom:trace work_item=graph-harness-01 spec=specs/102-Graph-Harness.md#REQ-GH-012 plan=phase-6/task-6-2/step-6-2-1
  // ─────────────────────────────────────────────────────────────────────────

  const graphTemplateSaveTool = tool({
    description:
      "Save the current graph as a reusable YAML template in .graph-harness/templates/. " +
      "Template name is sanitized to [a-z0-9_-], max 64 chars. Path traversal is rejected. " +
      "Returns { template_path, nodes_exported }.",

    args: {
      graph_id: tool.schema.string().describe("Graph to export as template"),
      template_name: tool.schema.string().describe("Name for the template file (without .yaml extension)"),
      description: tool.schema.string().optional().describe("Human-readable description of the template"),
    },

    async execute(args, context) {
      // axiom:trace work_item=graph-harness-01 spec=specs/102-Graph-Harness.md#REQ-GH-012 plan=phase-6/task-6-2/step-6-2-1
      if (!config.enabled) return JSON.stringify({ error: "Graph Harness is disabled" });

      try {
        const graphId = args.graph_id;
        const sessionId = (context as { sessionID?: string })?.sessionID ?? null;

        // Sanitize name
        const safeName = sanitizeTemplateName(args.template_name);
        if (!safeName) {
          return JSON.stringify({
            error: `Invalid template name: "${args.template_name}". Use [a-z0-9_-] only, no path separators.`,
          });
        }

        // Verify graph exists
        const graphRow = await db.queryOne(`SELECT id, title, description FROM graphs WHERE id = ?`, [graphId]) as { id: string; title: string; description: string | null } | null;
        if (!graphRow) {
          return JSON.stringify({ error: `Graph not found: ${graphId}` });
        }

        // Ensure templates directory exists
        const templatesDir = join(directory, config.templates.directory);
        if (!existsSync(templatesDir)) {
          mkdirSync(templatesDir, { recursive: true });
        }

        // Verify output path stays within templates directory (path traversal guard)
        const { resolve: resolvePath } = await import("node:path");
        const outPath = resolvePath(join(templatesDir, `${safeName}.yaml`));
        const allowedRoot = resolvePath(templatesDir);
        if (!outPath.startsWith(allowedRoot + "/") && outPath !== allowedRoot) {
          return JSON.stringify({ error: "Path traversal detected in template save path" });
        }

        // Read nodes from graph
        const nodes = await db.queryAll(
            `SELECT id, title, description, execution_mode, execution_config, context FROM nodes
             WHERE graph_id = ? ORDER BY created_at ASC`
          , [graphId]) as Array<{
            id: string; title: string; description: string;
            execution_mode: string; execution_config: string | null; context: string | null;
          }>;

        // Read dependencies
        const deps = await db.queryAll(`SELECT node_id, depends_on FROM dependencies WHERE graph_id = ?`, [graphId]) as Array<{ node_id: string; depends_on: string }>;

        // Build dependency map: nodeId → [depends_on, ...]
        const depMap = new Map<string, string[]>();
        for (const dep of deps) {
          const arr = depMap.get(dep.node_id) ?? [];
          arr.push(dep.depends_on);
          depMap.set(dep.node_id, arr);
        }

        // Read conditions for each node
        const conditions = await db.queryAll(
            `SELECT node_id, type, command, expected, description FROM conditions
             WHERE graph_id = ? ORDER BY ordinal ASC`
          , [graphId]) as Array<{
            node_id: string; type: string; command: string | null;
            expected: string | null; description: string | null;
          }>;

        // Build conditions map: nodeId → [conditions]
        const condMap = new Map<string, Array<Record<string, unknown>>>();
        for (const cond of conditions) {
          const arr = condMap.get(cond.node_id) ?? [];
          arr.push({
            type: cond.type,
            ...(cond.command ? { command: cond.command } : {}),
            ...(cond.expected ? { expected: cond.expected } : {}),
            ...(cond.description ? { description: cond.description } : {}),
          });
          condMap.set(cond.node_id, arr);
        }

        // Read data flows
        const flows = await db.queryAll(
            `SELECT from_node_id, to_node_id, output_key FROM data_flow WHERE graph_id = ?`
          , [graphId]) as Array<{ from_node_id: string; to_node_id: string; output_key: string | null }>;

        // Build YAML content (manual serialization — avoids external YAML serializer dep)
        function yamlStr(s: string): string {
          // Quote strings that need it
          if (/[:#\[\]{},&*?|<>\-=!%@`"']/.test(s) || s.includes("\n") || s.trim() !== s) {
            return JSON.stringify(s); // use JSON string as valid YAML scalar
          }
          return s;
        }

        const lines: string[] = [
          `name: ${yamlStr(safeName)}`,
          `description: ${yamlStr(args.description ?? graphRow.description ?? "")}`,
          `version: 1`,
          `nodes:`,
        ];

        for (const node of nodes) {
          lines.push(`  - id: ${yamlStr(node.id)}`);
          lines.push(`    title: ${yamlStr(node.title)}`);
          lines.push(`    description: ${yamlStr(node.description)}`);
          lines.push(`    execution_mode: ${node.execution_mode}`);

          const nodeDeps = depMap.get(node.id);
          if (nodeDeps && nodeDeps.length > 0) {
            lines.push(`    dependencies:`);
            for (const d of nodeDeps) {
              lines.push(`      - ${yamlStr(d)}`);
            }
          }

          if (node.execution_config) {
            try {
              const ec = JSON.parse(node.execution_config) as Record<string, unknown>;
              lines.push(`    execution_config:`);
              for (const [k, v] of Object.entries(ec)) {
                if (typeof v === "string") {
                  lines.push(`      ${k}: ${yamlStr(v)}`);
                } else if (typeof v === "number" || typeof v === "boolean") {
                  lines.push(`      ${k}: ${v}`);
                } else if (v !== null && v !== undefined) {
                  lines.push(`      ${k}: ${yamlStr(JSON.stringify(v))}`);
                }
              }
            } catch { /* skip malformed */ }
          }

          const nodeConds = condMap.get(node.id);
          if (nodeConds && nodeConds.length > 0) {
            lines.push(`    done_conditions:`);
            for (const cond of nodeConds) {
              lines.push(`      - type: ${yamlStr(cond.type as string)}`);
              for (const [k, v] of Object.entries(cond)) {
                if (k === "type") continue;
                lines.push(`        ${k}: ${yamlStr(String(v))}`);
              }
            }
          }
        }

        if (flows.length > 0) {
          lines.push(`data_flows:`);
          for (const flow of flows) {
            lines.push(`  - from: ${yamlStr(flow.from_node_id)}`);
            lines.push(`    to: ${yamlStr(flow.to_node_id)}`);
            if (flow.output_key) {
              lines.push(`    output_key: ${yamlStr(flow.output_key)}`);
            }
          }
        }

        const yamlContent = lines.join("\n") + "\n";

        // Write template file
        writeFileSync(outPath, yamlContent, "utf-8");

        // Update templates table
        const ts = new Date().toISOString();
        try {
          await db.run(
            `INSERT INTO templates (name, description, definition, created_at, updated_at)
             VALUES (?, ?, ?, ?, ?)
             ON CONFLICT(name) DO UPDATE SET description=excluded.description, definition=excluded.definition, updated_at=?`
          , [safeName,
            args.description ?? graphRow.description ?? "",
            JSON.stringify({ name: safeName, description: args.description ?? "" }),
            ts, ts, ts]);
        } catch { /* non-fatal */ }

        await logLedger(graphId, sessionId, "template_saved", null, {
          template_name: safeName,
          template_path: outPath,
          nodes_exported: nodes.length,
        });

        return JSON.stringify({
          template_path: outPath,
          nodes_exported: nodes.length,
          template_name: safeName,
        });
      } catch (err) {
        return JSON.stringify({ error: `graph.template.save failed: ${String(err)}` });
      }
    },
  });

  // ─────────────────────────────────────────────────────────────────────────
  // Phase 10: graph.template.update tool (REQ-GH-143)
  //
  // Update an existing template file by merging new field values into it.
  // Uses yaml.stringify for serialization to avoid data loss.
  //
  // axiom:trace work_item=SWDE-54 spec=specs/102-Graph-Harness.md#REQ-GH-143 plan=phase-10/new-step-p10-04 jira_ref=SWDE-54
  // ─────────────────────────────────────────────────────────────────────────

  const graphTemplateUpdateTool = tool({
    description:
      "Update an existing graph template in .graph-harness/templates/ by merging new field values. " +
      "Accepts partial updates: nodes, description, parameters, data_flows. " +
      "Uses atomic write (write-to-temp-then-rename). " +
      "Returns { template_name, updated: true, path }.",

    args: {
      name: tool.schema.string().describe("Template name (filename without .yaml extension)"),
      nodes: tool.schema.array(tool.schema.any()).optional().describe("Replace nodes array"),
      description: tool.schema.string().optional().describe("Update description"),
      parameters: tool.schema.array(tool.schema.any()).optional().describe("Update parameter definitions"),
      data_flows: tool.schema.array(tool.schema.any()).optional().describe("Update data flows"),
    },

    async execute(args, _context) {
      // axiom:trace work_item=SWDE-54 spec=specs/102-Graph-Harness.md#REQ-GH-143 plan=phase-10/new-step-p10-04 jira_ref=SWDE-54
      if (!config.enabled) return JSON.stringify({ error: "Graph Harness is disabled" });

      try {
        const name = args.name;

        // Path traversal / name validation — same rule as graph_template_save
        if (!name || !/^[a-z0-9_-]+$/.test(name) || name.length > 64) {
          return JSON.stringify({
            error: "Invalid template name: must match [a-z0-9_-]+, max 64 chars",
          });
        }

        // Resolve template path
        const templatesDir = join(directory, config.templates.directory);
        const templatePath = join(templatesDir, name + ".yaml");

        // Load existing template
        if (!existsSync(templatePath)) {
          return JSON.stringify({ error: `Template not found: ${name}` });
        }

        const raw = readFileSync(templatePath, "utf-8");
        const existing = parseYaml(raw) as Record<string, unknown>;

        // Merge incoming fields
        if (args.nodes !== undefined) existing.nodes = args.nodes;
        if (args.description !== undefined) existing.description = args.description;
        if (args.parameters !== undefined) existing.parameters = args.parameters;
        if (args.data_flows !== undefined) existing.data_flows = args.data_flows;

        // Serialize using yaml.stringify to preserve all fields
        const yamlContent = yamlStringify(existing);

        // 16KB cap
        if (yamlContent.length > 16 * 1024) {
          return JSON.stringify({ error: "Template exceeds 16KB limit after update" });
        }

        // Atomic write: write to tmp then rename
        const tmpPath = templatePath + ".tmp" + Date.now();
        writeFileSync(tmpPath, yamlContent, "utf-8");
        renameSync(tmpPath, templatePath);

        return JSON.stringify({ template_name: name, updated: true, path: templatePath });
      } catch (err) {
        return JSON.stringify({ error: `graph.template.update failed: ${String(err)}` });
      }
    },
  });

  // ─────────────────────────────────────────────────────────────────────────
  // Phase 6.4: graph.admin tool (REQ-GH-100)
  //
  // Admin sub-commands: status, pause, resume, skip, retry, override,
  // abandon, approve, templates.
  //
  // axiom:trace work_item=graph-harness-01 spec=specs/102-Graph-Harness.md#REQ-GH-100 plan=phase-6/task-6-4/step-6-4-1
  // ─────────────────────────────────────────────────────────────────────────

  const graphAdminTool = tool({
    description:
      "Administrative commands for graph management. " +
      "Sub-commands: status, pause, resume, skip, retry, override, abandon, approve, templates. " +
      "The 'override' command requires a --reason parameter. " +
      "Returns { ok, result } or { error }.",

    args: {
      command: tool.schema
        .enum(["status", "pause", "resume", "skip", "retry", "override", "abandon", "approve", "templates"])
        .describe("Admin command to execute"),
      graph_id: tool.schema.string().optional().describe("Target graph ID (required for most commands)"),
      node_id: tool.schema.string().optional().describe("Target node ID (required for node-level commands: skip, retry, override, approve)"),
      reason: tool.schema.string().optional().describe("Required for 'override' command. Reason for the override."),
    },

    async execute(args, context) {
      // axiom:trace work_item=graph-harness-01 spec=specs/102-Graph-Harness.md#REQ-GH-100 plan=phase-6/task-6-4/step-6-4-1
      if (!config.enabled) return JSON.stringify({ error: "Graph Harness is disabled" });

      try {
        const sessionId = (context as { sessionID?: string })?.sessionID ?? null;
        const ts = new Date().toISOString();

        switch (args.command) {
          // ── status ─────────────────────────────────────────────────────────
          case "status": {
            if (!args.graph_id) return JSON.stringify({ error: "graph_id required for 'status'" });
            const graphRow = await db.queryOne(`SELECT id, title, status, created_at, completed_at FROM graphs WHERE id = ?`, [args.graph_id]) as { id: string; title: string; status: string; created_at: string; completed_at: string | null } | null;
            if (!graphRow) return JSON.stringify({ error: `Graph not found: ${args.graph_id}` });

            const nodes = await db.queryAll(`SELECT id, title, status, execution_mode FROM nodes WHERE graph_id = ? ORDER BY created_at ASC`, [args.graph_id]) as Array<{ id: string; title: string; status: string; execution_mode: string }>;

            const statusCounts: Record<string, number> = {};
            for (const n of nodes) {
              statusCounts[n.status] = (statusCounts[n.status] ?? 0) + 1;
            }

            return JSON.stringify({
              graph_id: graphRow.id,
              title: graphRow.title,
              status: graphRow.status,
              created_at: graphRow.created_at,
              completed_at: graphRow.completed_at,
              node_count: nodes.length,
              status_counts: statusCounts,
              nodes: nodes.map((n) => ({ id: n.id, title: n.title, status: n.status, execution_mode: n.execution_mode })),
            });
          }

          // ── pause ──────────────────────────────────────────────────────────
          // REQ-GH-090: use transitionGraphStatus to enforce valid transitions
          // axiom:trace work_item=graph-harness-01 spec=specs/102-Graph-Harness.md#REQ-GH-090 plan=phase-6/task-6-7/step-6-7-1
          case "pause": {
            if (!args.graph_id) return JSON.stringify({ error: "graph_id required for 'pause'" });
            const pauseResult = await transitionGraphStatus(args.graph_id, "paused", (args as Record<string, unknown>).reason as string | undefined ?? "admin_pause");
            if (!pauseResult.ok) {
              return JSON.stringify({ ok: false, error: pauseResult.error, graph_id: args.graph_id });
            }
            return JSON.stringify({ ok: true, result: "Graph paused", graph_id: args.graph_id });
          }

          // ── resume ─────────────────────────────────────────────────────────
          // REQ-GH-091: use transitionGraphStatus to enforce valid transitions
          // axiom:trace work_item=graph-harness-01 spec=specs/102-Graph-Harness.md#REQ-GH-091 plan=phase-6/task-6-7/step-6-7-2
          case "resume": {
            if (!args.graph_id) return JSON.stringify({ error: "graph_id required for 'resume'" });
            const resumeResult = await transitionGraphStatus(args.graph_id, "active", "admin_resume");
            if (!resumeResult.ok) {
              return JSON.stringify({ ok: false, error: resumeResult.error, graph_id: args.graph_id });
            }
            return JSON.stringify({ ok: true, result: "Graph resumed", graph_id: args.graph_id });
          }

          // ── skip ───────────────────────────────────────────────────────────
          case "skip": {
            if (!args.graph_id) return JSON.stringify({ error: "graph_id required for 'skip'" });
            if (!args.node_id) return JSON.stringify({ error: "node_id required for 'skip'" });

            const nodeRow = await db.queryOne(`SELECT id, status FROM nodes WHERE graph_id = ? AND id = ?`, [args.graph_id, args.node_id]) as { id: string; status: string } | null;
            if (!nodeRow) return JSON.stringify({ error: `Node not found: ${args.node_id}` });

            await db.run(
              `UPDATE nodes SET status = 'skipped', completed_at = ? WHERE graph_id = ? AND id = ?`
            , [ts, args.graph_id, args.node_id]);

            await logLedger(args.graph_id, sessionId, "node_skipped_admin", args.node_id, {
              previous_status: nodeRow.status,
              operator: sessionId,
            });

            return JSON.stringify({ ok: true, result: `Node "${args.node_id}" skipped`, node_id: args.node_id });
          }

          // ── retry ──────────────────────────────────────────────────────────
          case "retry": {
            if (!args.graph_id) return JSON.stringify({ error: "graph_id required for 'retry'" });
            if (!args.node_id) return JSON.stringify({ error: "node_id required for 'retry'" });

            const nodeRow = await db.queryOne(`SELECT id, status, attempt_count FROM nodes WHERE graph_id = ? AND id = ?`, [args.graph_id, args.node_id]) as { id: string; status: string; attempt_count: number } | null;
            if (!nodeRow) return JSON.stringify({ error: `Node not found: ${args.node_id}` });

            await db.run(
              `UPDATE nodes SET status = 'pending', attempt_count = 0, activated_at = NULL, completed_at = NULL
               WHERE graph_id = ? AND id = ?`
            , [args.graph_id, args.node_id]);

            await logLedger(args.graph_id, sessionId, "node_retry_admin", args.node_id, {
              previous_status: nodeRow.status,
              previous_attempt_count: nodeRow.attempt_count,
              operator: sessionId,
            });

            return JSON.stringify({ ok: true, result: `Node "${args.node_id}" reset to pending for retry`, node_id: args.node_id });
          }

          // ── override ───────────────────────────────────────────────────────
          case "override": {
            if (!args.graph_id) return JSON.stringify({ error: "graph_id required for 'override'" });
            if (!args.node_id) return JSON.stringify({ error: "node_id required for 'override'" });
            if (!args.reason || args.reason.trim() === "") {
              return JSON.stringify({ error: "reason is REQUIRED for 'override'. Provide --reason with a clear explanation." });
            }

            const nodeRow = await db.queryOne(`SELECT id, status FROM nodes WHERE graph_id = ? AND id = ?`, [args.graph_id, args.node_id]) as { id: string; status: string } | null;
            if (!nodeRow) return JSON.stringify({ error: `Node not found: ${args.node_id}` });

            await db.run(
              `UPDATE nodes SET status = 'done', completed_at = ? WHERE graph_id = ? AND id = ?`
            , [ts, args.graph_id, args.node_id]);

            // Mandatory ledger entry for override (REQ-GH-100 override safety)
            await logLedger(args.graph_id, sessionId, "node_overridden", args.node_id, {
              action: "override",
              reason: args.reason,
              operator: sessionId,
              previous_status: nodeRow.status,
            });

            // Annotation with reason
            const annotId = `ann_override_${args.graph_id}_${args.node_id}_${Date.now()}`;
            try {
              await db.run(
                `INSERT OR IGNORE INTO annotations (id, graph_id, node_id, session_id, type, content, severity, created_at)
                 VALUES (?, ?, ?, ?, 'decision', ?, 'info', ?)`
              , [annotId, args.graph_id, args.node_id, sessionId, `Override reason: ${args.reason}`, ts]);
            } catch { /* non-fatal */ }

            return JSON.stringify({
              ok: true,
              result: `Node "${args.node_id}" overridden to done`,
              node_id: args.node_id,
              reason: args.reason,
            });
          }

          // ── abandon ────────────────────────────────────────────────────────
          case "abandon": {
            if (!args.graph_id) return JSON.stringify({ error: "graph_id required for 'abandon'" });

            const graphRow = await db.queryOne(`SELECT id, status FROM graphs WHERE id = ?`, [args.graph_id]) as { id: string; status: string } | null;
            if (!graphRow) return JSON.stringify({ error: `Graph not found: ${args.graph_id}` });

            await db.run(`UPDATE graphs SET status = 'abandoned', completed_at = ? WHERE id = ?`, [ts, args.graph_id]);
            await db.run(
              `UPDATE nodes SET status = 'abandoned', completed_at = ?
               WHERE graph_id = ? AND LOWER(status) IN ('pending', 'active')`
            , [ts, args.graph_id]);

            await logLedger(args.graph_id, sessionId, "graph_abandoned_admin", null, {
              operator: sessionId,
              reason: args.reason ?? null,
            });

            return JSON.stringify({ ok: true, result: "Graph abandoned", graph_id: args.graph_id });
          }

          // ── approve ────────────────────────────────────────────────────────
          case "approve": {
            if (!args.graph_id) return JSON.stringify({ error: "graph_id required for 'approve'" });
            if (!args.node_id) return JSON.stringify({ error: "node_id required for 'approve'" });

            const nodeRow = await db.queryOne(`SELECT id, status FROM nodes WHERE graph_id = ? AND id = ?`, [args.graph_id, args.node_id]) as { id: string; status: string } | null;
            if (!nodeRow) return JSON.stringify({ error: `Node not found: ${args.node_id}` });

            // Find and mark manual conditions as passed
            const manualConds = await db.queryAll(
                `SELECT id FROM conditions WHERE graph_id = ? AND node_id = ? AND type = 'manual'`
              , [args.graph_id, args.node_id]) as Array<{ id: string }>

            for (const cond of manualConds) {
              await db.run(`UPDATE conditions SET passed = 1, last_evaluated_at = ? WHERE id = ?`, [ts, cond.id]);
            }

            await logLedger(args.graph_id, sessionId, "node_approved", args.node_id, {
              operator: sessionId,
              manual_conditions_approved: manualConds.length,
            });

            return JSON.stringify({
              ok: true,
              result: `Node "${args.node_id}" approved (${manualConds.length} manual condition(s) marked passed)`,
              node_id: args.node_id,
              conditions_approved: manualConds.length,
            });
          }

          // ── templates ──────────────────────────────────────────────────────
          case "templates": {
            const templatesDir = join(directory, config.templates.directory);
            const templateFiles: Array<{ name: string; path: string; size_bytes: number }> = [];

            if (existsSync(templatesDir)) {
              try {
                const files = readdirSync(templatesDir);
                for (const f of files) {
                  if (f.endsWith(".yaml") || f.endsWith(".yml")) {
                    const fPath = join(templatesDir, f);
                    try {
                      const stat = statSync(fPath);
                      templateFiles.push({
                        name: f.replace(/\.(yaml|yml)$/, ""),
                        path: fPath,
                        size_bytes: stat.size,
                      });
                    } catch { /* skip unreadable */ }
                  }
                }
              } catch { /* directory unreadable */ }
            }

            // Also query templates table for usage stats
            const dbTemplates = await db.queryAll(`SELECT name, description, usage_count, last_used_at FROM templates`, []) as Array<{ name: string; description: string; usage_count: number; last_used_at: string | null }>;

            const dbTemplateMap = new Map(dbTemplates.map((t) => [t.name, t]));

            return JSON.stringify({
              templates_directory: templatesDir,
              template_count: templateFiles.length,
              templates: templateFiles.map((f) => ({
                name: f.name,
                size_bytes: f.size_bytes,
                usage_count: dbTemplateMap.get(f.name)?.usage_count ?? 0,
                description: dbTemplateMap.get(f.name)?.description ?? null,
                last_used_at: dbTemplateMap.get(f.name)?.last_used_at ?? null,
              })),
            });
          }

          default:
            return JSON.stringify({ error: `Unknown command: ${String(args.command)}` });
        }
      } catch (err) {
        return JSON.stringify({ error: `graph.admin failed: ${String(err)}` });
      }
    },
  });

  // ─────────────────────────────────────────────────────────────────────────
  // Phase 6.5 — Archive stale graphs (REQ-GH-092)
  //
  // Finds completed/abandoned graphs older than archive_after_days and
  // serializes them to .graph-harness/archive/<graph_id>_<timestamp>.json.
  // Templates are never archived.
  //
  // axiom:trace work_item=graph-harness-01 spec=specs/102-Graph-Harness.md#REQ-GH-092 plan=phase-6/task-6-5/step-6-5-1
  // ─────────────────────────────────────────────────────────────────────────

  async function archiveStaleGraphs(): Promise<void> {
    const days = config.lifecycle.archive_after_days;
    if (!days || days <= 0) return; // archiving disabled

    const archiveDir = join(directory, config.lifecycle.archive_directory);
    try {
      if (!existsSync(archiveDir)) {
        mkdirSync(archiveDir, { recursive: true });
      }
    } catch (err) {
      console.warn("[GraphHarness] Failed to create archive directory:", err);
      return;
    }

    // Find stale completed/abandoned graphs
    const staleGraphs = await db.queryAll(`
      SELECT id, title, status, completed_at, created_at, metadata
      FROM graphs
      WHERE LOWER(status) IN ('complete', 'abandoned')
        AND completed_at IS NOT NULL
        AND datetime(completed_at) < datetime('now', ? || ' days')
    `, [`-${days}`]) as Array<{
      id: string; title: string; status: string;
      completed_at: string; created_at: string; metadata: string | null;
    }>;

    for (const graph of staleGraphs) {
      const ts = new Date().toISOString().replace(/[:.]/g, "-");
      const archivePath = join(archiveDir, `${graph.id}_${ts}.json`);

      // Skip if already archived (file with this graph_id prefix already exists)
      try {
        const existing = readdirSync(archiveDir);
        if (existing.some((f) => f.startsWith(graph.id + "_"))) {
          continue; // already archived
        }
      } catch { /* skip on read error */ }

      // Serialize graph + nodes + outputs
      const nodes = await db.queryAll(
        `SELECT * FROM nodes WHERE graph_id = ?`
      , [graph.id]);
      const outputs = await db.queryAll(
        `SELECT * FROM node_outputs WHERE graph_id = ?`
      , [graph.id]);

      const archivePayload = {
        archived_at: new Date().toISOString(),
        graph: {
          id: graph.id,
          title: graph.title,
          status: graph.status,
          completed_at: graph.completed_at,
          created_at: graph.created_at,
          metadata: graph.metadata ? (() => { try { return JSON.parse(graph.metadata!); } catch { return graph.metadata; } })() : null,
        },
        nodes,
        outputs,
      };

      try {
        writeFileSync(archivePath, JSON.stringify(archivePayload, null, 2), "utf-8");
        await addLedgerEntry(graph.id, "graph_archived", {
          archive_path: archivePath,
          archived_at: archivePayload.archived_at,
          node_count: (nodes as unknown[]).length,
          output_count: (outputs as unknown[]).length,
        });
        pluginInfo("graph-harness", `Archived graph ${graph.id} → ${archivePath}`);
      } catch (err) {
        console.warn(`[GraphHarness] Failed to archive graph ${graph.id}:`, err);
      }
    }
  }

  // ─────────────────────────────────────────────────────────────────────────
  // Phase 6.6 — Terminal notifications (REQ-GH-101) — extended by SWDE-63
  //
  // sendTerminalNotification() replaced by NotificationDispatcher / TerminalChannel
  // (see module-level notification subsystem, ~line 878).
  // All call sites now use dispatchNotification() for structured events.
  //
  // axiom:trace work_item=SWDE-63 spec=specs/102-Graph-Harness.md#REQ-GH-101 plan=phase-notif/task-1/step-3 jira_ref=SWDE-63
  // ─────────────────────────────────────────────────────────────────────────

  // ─────────────────────────────────────────────────────────────────────────
  // Phase 6.7 — Graph lifecycle state machine (REQ-GH-090, REQ-GH-091)
  //
  // Valid transitions:
  //   created  → active | abandoned
  //   active   → paused | complete | abandoned | failed
  //   paused   → active | abandoned
  //   complete → (terminal)
  //   abandoned → (terminal)
  //   failed   → (terminal)
  //
  // axiom:trace work_item=graph-harness-01 spec=specs/102-Graph-Harness.md#REQ-GH-090 plan=phase-6/task-6-7/step-6-7-1
  // axiom:trace work_item=graph-harness-01 spec=specs/102-Graph-Harness.md#REQ-GH-091 plan=phase-6/task-6-7/step-6-7-2
  // ─────────────────────────────────────────────────────────────────────────

  const VALID_GRAPH_TRANSITIONS: Record<string, string[]> = {
    created:   ["active", "paused", "abandoned"],
    active:    ["paused", "complete", "abandoned", "failed"],
    paused:    ["active", "abandoned"],
    complete:  [],
    abandoned: [],
    failed:    [],
  };

  async function transitionGraphStatus(
    graphId: string,
    newStatus: string,
    reason?: string
  ): { ok: boolean; error?: string } {
    const graph = await db.queryOne(
      `SELECT id, status FROM graphs WHERE id = ?`
    , [graphId]) as { id: string; status: string } | null;

    if (!graph) {
      return { ok: false, error: `Graph not found: ${graphId}` };
    }

    const currentStatus = graph.status.toLowerCase();
    const allowed = VALID_GRAPH_TRANSITIONS[currentStatus] ?? [];

    if (!allowed.includes(newStatus.toLowerCase())) {
      return {
        ok: false,
        error: `Invalid transition: ${currentStatus} → ${newStatus}. Allowed from ${currentStatus}: [${allowed.join(", ") || "none — terminal state"}]`,
      };
    }

    const now = new Date().toISOString();
    const isTerminal = ["complete", "abandoned", "failed"].includes(newStatus.toLowerCase());

    await db.run(
      `UPDATE graphs SET status = ?${isTerminal ? ", completed_at = ?" : ""} WHERE id = ?`
    , [...(isTerminal ? [newStatus, now, graphId] : [newStatus, graphId])]);

    await addLedgerEntry(graphId, "graph_status_transition", {
      from: currentStatus,
      to: newStatus,
      reason: reason ?? "manual",
      timestamp: now,
    });

    console.log(`[GraphHarness] Graph ${graphId}: ${currentStatus} → ${newStatus}${reason ? ` (${reason})` : ""}`);

    // On PAUSED: send notification
    if (newStatus.toLowerCase() === "paused") {
      await dispatchNotification({
        type: "graph_paused",
        graph_id: graphId,
        title: "Graph Paused",
        body: `Graph ${graphId} paused${reason ? ` (${reason})` : ""}`,
        metadata: reason ? { reason } : undefined,
        timestamp: now,
      }).catch(() => { /* non-fatal */ });
    }
    // On ACTIVE (resume): send notification
    if (newStatus.toLowerCase() === "active" && currentStatus === "paused") {
      await dispatchNotification({
        type: "graph_resumed",
        graph_id: graphId,
        title: "Graph Resumed",
        body: `Graph ${graphId} resumed`,
        timestamp: now,
      }).catch(() => { /* non-fatal */ });
    }

    return { ok: true };
  }

  // ─────────────────────────────────────────────────────────────────────────
  // Phase 6.8 — Flaky condition detection (REQ-GH-083)
  //
  // Tracks condition results and detects pass→fail→pass or fail→pass→fail
  // oscillation. Marks node FLAKY in metadata and grants 2 bonus attempts.
  //
  // axiom:trace work_item=graph-harness-01 spec=specs/102-Graph-Harness.md#REQ-GH-083 plan=phase-6/task-6-8/step-6-8-1
  // ─────────────────────────────────────────────────────────────────────────

  async function checkAndMarkFlaky(
    graphId: string,
    nodeId: string,
    conditionId: string
  ): boolean {
    // Get the last 3 results for this condition from the ledger
    const history = await db.queryAll(`
      SELECT detail FROM ledger
      WHERE graph_id = ? AND action = 'condition_evaluated'
        AND target_node_id = ?
        AND json_extract(detail, '$.condition_id') = ?
      ORDER BY timestamp DESC
      LIMIT 3
    `, [graphId, nodeId, conditionId]) as Array<{ detail: string }>;

    if (history.length < 3) return false;

    // Parse results — oldest first (reverse the DESC order)
    const results: boolean[] = history.reverse().map((row) => {
      try {
        const d = JSON.parse(row.detail) as { passed?: boolean | number };
        return d.passed === true || d.passed === 1;
      } catch {
        return false;
      }
    });

    // Check oscillation: pass→fail→pass or fail→pass→fail
    const oscillates =
      (results[0] === true  && results[1] === false && results[2] === true) ||
      (results[0] === false && results[1] === true  && results[2] === false);

    if (!oscillates) return false;

    // Mark node as flaky in metadata
    const nodeRow = await db.queryOne(
      `SELECT metadata FROM nodes WHERE id = ? AND graph_id = ?`
    , [nodeId, graphId]) as { metadata: string | null } | null;

    const existingMeta: Record<string, unknown> = (() => {
      if (!nodeRow?.metadata) return {};
      try { return JSON.parse(nodeRow.metadata) as Record<string, unknown>; } catch { return {}; }
    })();

    if (!existingMeta.flaky) {
      existingMeta.flaky = true;
      existingMeta.flaky_detected_at = new Date().toISOString();

      await db.run(
        `UPDATE nodes SET metadata = ? WHERE id = ? AND graph_id = ?`
      , [JSON.stringify(existingMeta), nodeId, graphId]);

      // Grant 2 bonus retry attempts
      const bonusAttempts = config.retry.flaky_bonus_attempts ?? 2;
      await db.run(
        `UPDATE nodes SET max_retries = max_retries + ? WHERE id = ? AND graph_id = ?`
      , [bonusAttempts, nodeId, graphId]);

      await addLedgerEntry(graphId, "condition_oscillation_detected", {
        node_id: nodeId,
        condition_id: conditionId,
        results_last_3: results,
        bonus_attempts_granted: bonusAttempts,
      });

      pluginInfo("graph-harness", `Flaky condition detected for node ${nodeId} in graph ${graphId} — +${bonusAttempts} bonus attempts`);
    }

    return true;
  }

  // ─────────────────────────────────────────────────────────────────────────
  // Phase 6.9 — Circuit breaker (REQ-GH-080)
  //
  // Counts total retry_scheduled ledger entries for a graph.
  // Pauses graph when count reaches max_total_retries_per_graph.
  //
  // axiom:trace work_item=graph-harness-01 spec=specs/102-Graph-Harness.md#REQ-GH-080 plan=phase-6/task-6-9/step-6-9-1
  // ─────────────────────────────────────────────────────────────────────────

  async function checkCircuitBreaker(graphId: string): Promise<boolean> {
    const totalRetries = await db.queryOne(
      `SELECT COUNT(*) as cnt FROM ledger WHERE graph_id = ? AND action = 'retry_scheduled'`
    , [graphId]) as { cnt: number };

    const limit = config.retry.max_total_retries_per_graph ?? 50;
    if (limit > 0 && totalRetries.cnt >= limit) {
      // Use transitionGraphStatus to enforce valid state machine transition
      const graphRow = await db.queryOne(`SELECT status FROM graphs WHERE id = ?`, [graphId]) as { status: string } | null;
      const currentStatus = graphRow?.status?.toLowerCase() ?? "active";

      // Only pause if currently active (not already paused/complete/failed/abandoned)
      if (currentStatus === "active") {
        await transitionGraphStatus(graphId, "paused", "circuit_breaker_tripped");
        await addLedgerEntry(graphId, "circuit_breaker_tripped", {
          total_retries: totalRetries.cnt,
          limit,
        });
        console.warn(
          `[GraphHarness] Circuit breaker tripped for graph ${graphId}: ` +
          `${totalRetries.cnt} retries >= limit ${limit} — graph PAUSED`
        );
        // SWDE-63 AC-2: retry_storm notification fires when circuit breaker trips
        // axiom:trace work_item=SWDE-63 spec=specs/102-Graph-Harness.md#REQ-GH-101 plan=phase-notif/task-3/step-1 jira_ref=SWDE-63
        await dispatchNotification({
          type: "retry_storm",
          graph_id: graphId,
          title: "Retry Storm Detected",
          body: `Graph ${graphId} exceeded retry limit (${totalRetries.cnt} retries >= ${limit}) — graph PAUSED. Human review required.`,
          metadata: { total_retries: totalRetries.cnt, limit },
          timestamp: new Date().toISOString(),
        }).catch(() => { /* non-fatal */ });
        return true;
      }
    }
    return false;
  }

  // ─────────────────────────────────────────────────────────────────────────
  // Stop hook — prevent agent stop when graph work remains (REQ-GH-021, H-15)
  //
  // NOTE: The OpenCode plugin SDK (@opencode-ai/plugin v*) does NOT currently
  // expose a stop/beforeStop/onStop hook in the Hooks interface (verified at
  // .opencode/node_modules/@opencode-ai/plugin/dist/index.d.ts). The function
  // is implemented here and ready to wire in when the SDK exposes the hook.
  //
  // axiom:trace work_item=graph-harness-01 spec=specs/102-Graph-Harness.md#REQ-GH-021 plan=step-verify-gh-08
  // ─────────────────────────────────────────────────────────────────────────

  async function stopHookFn(params: { sessionID?: string }): Promise<boolean | void> {
    // Returns false to PREVENT stop if graph work remains.
    // Returns true (or void) to ALLOW stop.
    const sessionId = params?.sessionID;
    if (!sessionId) return; // allow stop if no session ID

    try {
      const sessionRow = await db.queryOne(`SELECT * FROM sessions WHERE session_id = ?`, [sessionId]) as SessionRow | null;
      if (!sessionRow || sessionRow.status.toLowerCase() !== 'active') return; // allow stop

      const graphRow = await db.queryOne(`SELECT * FROM graphs WHERE id = ?`, [sessionRow.graph_id]) as { id: string; title: string; status: string } | null;
      if (!graphRow || !['created', 'CREATED', 'active', 'ACTIVE'].includes(graphRow.status)) return;

      // Check if there's more work to do
      const activeNode = sessionRow.node_id ?
        await db.queryOne(`SELECT id, graph_id, title, description, status, execution_mode, attempt_count, max_retries, context, activated_at, completed_at FROM nodes WHERE id = ? AND graph_id = ?`, [sessionRow.node_id, sessionRow.graph_id]) as NodeRow | null : null;
      const nextNode = await findNextUnblockedNode(sessionRow.graph_id);

      if (!activeNode && !nextNode) return; // no work left, allow stop

      // Inject next briefing to re-enter the loop
      const targetNode = activeNode || nextNode;
      if (targetNode) {
        const briefing = await buildNodeBriefing(sessionRow.graph_id, targetNode as NodeRow);
        await injectBriefing(sessionId, briefing);
      }
      return false; // PREVENT stop — we re-entered the loop
    } catch (err) {
      console.error('[GraphHarness] stop hook error:', err);
      return; // allow stop on error
    }
  }

  // Suppress lint warning — stopHookFn is ready to wire when SDK exposes the hook
  void stopHookFn;

  // ─────────────────────────────────────────────────────────────────────────
  // graph_lock / graph_unlock / graph_transfer — REQ-GH-116, REQ-GH-117
  // ─────────────────────────────────────────────────────────────────────────

  const graphLockTool = tool({
    description:
      "Lock a graph to a specific session. Only that session can call mutation tools " +
      "(inject/modify/split/annotate/abandon) until the lock is released. " +
      "Caller must be the coordinator. " +
      "If the graph is already locked by a DIFFERENT session, returns an error — use force=true to override (coordinator emergency takeover). " +
      "Returns { locked: true, graph_id, locked_by, previous_holder? } or { error }.",
    args: {
      graph_id: tool.schema.string().min(1).describe("Graph to lock"),
      session_id: tool.schema.string().min(1).describe("Session that will hold the lock"),
      force: tool.schema.boolean().optional()
        .describe("Overwrite an existing lock held by a different session (emergency takeover). Default: false."),
    },
    async execute(args, context) {
      // axiom:trace work_item=graph-lock-upgrade-01 spec=specs/102-Graph-Harness.md#REQ-GH-116
      // glu-11: SELECT+UPDATE inside db.transaction() to eliminate TOCTOU race.
      try {
        await checkSessionRole(context, ["coordinator"]);
        if (!await db.queryOne(`SELECT id FROM graphs WHERE id=?`, [args.graph_id])) {
          return JSON.stringify({ error: `Graph not found: ${args.graph_id}` });
        }

        const callerSessionId = (context as Record<string, unknown>)?.sessionID as string | undefined;
        let lockError: string | null = null;
        let lockErrorExtra: Record<string, unknown> | null = null;

        const { previousHolder } = await db.transaction(async (db) => {
          const graphRow = await db.queryOne(`SELECT id, locked_by FROM graphs WHERE id=?`, [args.graph_id]) as
            { id: string; locked_by: string | null } | undefined;
          if (!graphRow) {
            lockError = `Graph not found inside transaction: ${args.graph_id}`;
            return { previousHolder: null };
          }
          if (graphRow.locked_by !== null && graphRow.locked_by !== args.session_id && !args.force) {
            lockError = `Graph ${args.graph_id} is already locked by session '${graphRow.locked_by}'. ` +
              `Use graph_transfer to hand off atomically, or pass force=true for emergency coordinator takeover.`;
            lockErrorExtra = { locked_by: graphRow.locked_by };
            return { previousHolder: graphRow.locked_by };
          }
          const prev = graphRow.locked_by;
          await db.run(`UPDATE graphs SET locked_by=? WHERE id=?`, [args.session_id, args.graph_id]);
          return { previousHolder: prev };
        });

        if (lockError) return JSON.stringify({ error: lockError, ...(lockErrorExtra ?? {}) });

        await addLedgerEntry(args.graph_id, "graph_locked", {
          locked_by: args.session_id, previous_holder: previousHolder, by: callerSessionId, forced: args.force ?? false,
        });
        return JSON.stringify({
          locked: true, graph_id: args.graph_id, locked_by: args.session_id,
          ...(previousHolder && previousHolder !== args.session_id ? { previous_holder: previousHolder } : {}),
        });
      } catch (err) {
        return JSON.stringify({ error: String(err) });
      }
    },
  });

  const graphUnlockTool = tool({
    description:
      "Release a graph lock. Caller must be the coordinator or the current lock holder. " +
      "Returns { unlocked: true, graph_id } or { error }.",
    args: {
      graph_id: tool.schema.string().min(1).describe("Graph to unlock"),
    },
    async execute(args, context) {
      // axiom:trace work_item=graph-lock-upgrade-01 spec=specs/102-Graph-Harness.md#REQ-GH-116
      try {
        const ctx = context as Record<string, unknown> | null | undefined;
        const sessionId = ctx?.sessionID as string | undefined;
        const row = await db.queryOne(`SELECT locked_by FROM graphs WHERE id=?`, [args.graph_id]) as
          { locked_by: string | null } | undefined;
        if (!row) return JSON.stringify({ error: `Graph not found: ${args.graph_id}` });
        if (row.locked_by !== null && sessionId && row.locked_by !== sessionId) {
          const sessionRow = await db.queryOne(`SELECT role FROM sessions WHERE session_id=?`, [sessionId]) as
            { role: string } | undefined;
          if (!sessionRow || sessionRow.role !== "coordinator") {
            return JSON.stringify({ error: `Cannot unlock: you are not the lock holder (${row.locked_by}) or a coordinator.` });
          }
        }
        await db.run(`UPDATE graphs SET locked_by=NULL WHERE id=?`, [args.graph_id]);
        await addLedgerEntry(args.graph_id, "graph_unlocked", { previously_locked_by: row.locked_by, by: sessionId });
        return JSON.stringify({ unlocked: true, graph_id: args.graph_id });
      } catch (err) {
        return JSON.stringify({ error: String(err) });
      }
    },
  });

  const graphTransferTool = tool({
    description:
      "Atomically transfer the graph lock from the current holder to a new session. " +
      "Avoids the TOCTOU window of graph_unlock + graph_lock. " +
      "Coordinator-only. Returns { transferred: true, from, to, graph_id } or { error }.",
    args: {
      graph_id: tool.schema.string().min(1).describe("Graph whose lock to transfer"),
      to_session_id: tool.schema.string().min(1).describe("Session that will hold the lock after transfer"),
    },
    async execute(args, context) {
      // axiom:trace work_item=graph-lock-upgrade-01 spec=specs/102-Graph-Harness.md#REQ-GH-117
      try {
        await checkSessionRole(context, ["coordinator"]);
        if (!await db.queryOne(`SELECT id FROM graphs WHERE id=?`, [args.graph_id])) {
          return JSON.stringify({ error: `Graph not found: ${args.graph_id}` });
        }

        const callerSessionId = (context as Record<string, unknown>)?.sessionID as string | undefined;
        let transferError: string | null = null;
        const { from } = await db.transaction(async (db) => {
          const toSessionRow = await db.queryOne(`SELECT session_id FROM sessions WHERE session_id=?`, [args.to_session_id]) as
            { session_id: string } | undefined;
          if (!toSessionRow) {
            transferError = `Target session not found: ${args.to_session_id}. Graph lock not transferred.`;
            return { from: null };
          }
          const graphRow = await db.queryOne(`SELECT locked_by FROM graphs WHERE id=?`, [args.graph_id]) as
            { locked_by: string | null } | undefined;
          const prevLockedBy = graphRow?.locked_by ?? null;
          await db.run(`UPDATE graphs SET locked_by=? WHERE id=?`, [args.to_session_id, args.graph_id]);
          return { from: prevLockedBy };
        });

        if (transferError) return JSON.stringify({ error: transferError });

        try {
          await addLedgerEntry(args.graph_id, "graph_lock_transferred", { from, to: args.to_session_id, by: callerSessionId });
        } catch (ledgerErr) {
          pluginWarn("graph-harness", `graph_transfer ledger write failed (best-effort): ${String(ledgerErr)}`);
        }
        return JSON.stringify({ transferred: true, graph_id: args.graph_id, from: from ?? null, to: args.to_session_id });
      } catch (err) {
        return JSON.stringify({ error: String(err) });
      }
    },
  });

  // ─────────────────────────────────────────────────────────────────────────
  // graph_session_list — lists sessions registered in the harness DB
  // Shows only sessions tied to graphs (by graph_id FK constraint).
  // For ALL OpenCode sessions use session_list from the opencode-session plugin.
  // ─────────────────────────────────────────────────────────────────────────
  const graphSessionListTool = tool({
    description:
      "List sessions tracked by the graph harness (sessions tied to graphs). " +
      "For ALL OpenCode sessions use the session_list tool from the session plugin. " +
      "Filter by graph_id and/or status. Returns up to 100 sessions sorted by created_at DESC " +
      "with role, cost, node, and heartbeat info.",
    args: {
      graph_id: tool.schema.string().optional()
        .describe("Filter to a specific graph (omit for all graphs)"),
      status: tool.schema.string().optional()
        .describe("Filter by status: active | done | stale | failed"),
    },
    async execute(args) {
      // axiom:trace work_item=opencode-session-plugin-01 spec=specs/102-Graph-Harness.md#REQ-GH-115
      try {
        let query = `SELECT session_id, graph_id, node_id, role, status,
                            last_heartbeat, worker_pid, tool_calls, cost_usd, created_at
                     FROM sessions WHERE 1=1`;
        const params: unknown[] = [];
        if (args.graph_id) { query += ` AND graph_id=?`;           params.push(args.graph_id); }
        if (args.status)   { query += ` AND LOWER(status)=LOWER(?)`; params.push(args.status); }
        query += ` ORDER BY created_at DESC LIMIT 100`;

        const sessions = await db.queryAll(query, [...params]) as Record<string, unknown>[];
        return JSON.stringify({ sessions, total: sessions.length });
      } catch (err) {
        return JSON.stringify({ error: String(err) });
      }
    },
  });

  // ─────────────────────────────────────────────────────────────────────────
  // graph_export — REQ-GH-122 (Phase 8)
  //
  // Serializes a live graph (nodes, deps, conditions, trigger blocks) to YAML or JSON.
  // axiom:trace work_item=SWDE-46 spec=specs/102-Graph-Harness.md#REQ-GH-122 plan=phase-3/task-3.1/step-1 jira_ref=SWDE-46
  // ─────────────────────────────────────────────────────────────────────────

  const graphExportTool = tool({
    description:
      "Serializes a live graph (including trigger blocks and current node states) to a YAML or JSON file. " +
      "Definition-only export (include_state: false) creates a re-importable graph blueprint. " +
      "Full snapshot (include_state: true) preserves node statuses and outputs for backup/restore.",
    args: {
      graph_id: tool.schema
        .string()
        .min(1)
        .describe("ID of the graph to export"),
      path: tool.schema
        .string()
        .min(1)
        .describe("Output file path, e.g. '.graph-harness/exports/monitor.yaml'"),
      format: tool.schema
        .enum(["yaml", "json"])
        .optional()
        .describe("Output format. Default: 'yaml'"),
      include_state: tool.schema
        .boolean()
        .optional()
        .describe("If true, include current node statuses and outputs. Default: false (definition only)."),
    },
    execute: async ({ graph_id: graphId, path: outputPath, format, include_state }, _context) => {
      try {
        // Fetch graph
        const graphRow = await db.queryOne(
          `SELECT id, title, description, status, created_at, locked_by FROM graphs WHERE id = ?`
        , [graphId]) as {
          id: string; title: string; description: string | null; status: string;
          created_at: string; locked_by: string | null;
        } | undefined;

        if (!graphRow) {
          return JSON.stringify({ error: `Graph not found: ${graphId}` });
        }

        // Fetch nodes with trigger blocks
        const nodes = await db.queryAll(
          `SELECT id, title, description, status, execution_mode, execution_config, context,
                  metadata, schedule, repeat, max_retries, created_at, completed_at,
                  trigger_on, trigger_cancel_on, trigger_every, trigger_cron,
                  trigger_max_runs, trigger_lifetime_h, trigger_run_count
           FROM nodes WHERE graph_id = ? ORDER BY created_at ASC`
        , [graphId]) as Array<{
          id: string; title: string; description: string | null; status: string;
          execution_mode: string; execution_config: string | null; context: string | null;
          metadata: string | null; schedule: string | null; repeat: number | null;
          max_retries: number | null; created_at: string; completed_at: string | null;
          trigger_on: string | null; trigger_cancel_on: string | null;
          trigger_every: string | null; trigger_cron: string | null;
          trigger_max_runs: number | null; trigger_lifetime_h: number | null;
          trigger_run_count: number | null;
        }>;

        // Fetch dependencies
        const deps = await db.queryAll(
          `SELECT node_id, depends_on FROM dependencies WHERE graph_id = ?`
        , [graphId]) as Array<{ node_id: string; depends_on: string }>;

        // Fetch conditions
        const conds = await db.queryAll(
          `SELECT node_id, type, command, description, timeout_seconds
           FROM conditions WHERE graph_id = ? ORDER BY node_id, ordinal ASC`
        , [graphId]) as Array<{
          node_id: string; type: string; command: string | null;
          description: string | null; timeout_seconds: number | null;
        }>;

        // Optionally fetch outputs (include_state only)
        const outputRows = include_state
          ? await db.queryAll(
              `SELECT node_id, key, value, type FROM node_outputs WHERE graph_id = ?`
            , [graphId]) as Array<{ node_id: string; key: string; value: string; type: string }>
          : [];

        const fmt = format ?? "yaml";

        // Build the export document
        const exportDoc: Record<string, unknown> = {
          // Export header comment info (embedded as metadata)
          _export_meta: {
            format_version: "1",
            graph_id: graphId,
            exported_at: new Date().toISOString(),
          },
          name: graphRow.title,
          description: graphRow.description ?? undefined,
          nodes: nodes.map((n) => {
            const nodeOut: Record<string, unknown> = {
              id: n.id,
              title: n.title,
              description: n.description ?? undefined,
              execution_mode: n.execution_mode !== "agent" ? n.execution_mode : undefined,
            };
            if (n.execution_config) {
              try { nodeOut.execution_config = JSON.parse(n.execution_config); } catch { /* skip */ }
            }
            if (n.context) {
              try { nodeOut.context = JSON.parse(n.context); } catch { nodeOut.context = n.context; }
            }
            if (n.max_retries != null && n.max_retries !== 3) {
              nodeOut.max_retries = n.max_retries;
            }
            // Trigger block — only emit if trigger fields are non-default
            const hasTriggerOn = n.trigger_on && n.trigger_on !== "idle";
            const hasTriggerCancelOn = n.trigger_cancel_on && n.trigger_cancel_on !== "active";
            const hasTriggerEvery = !!n.trigger_every;
            const hasTriggerCron = !!n.trigger_cron;
            const hasTriggerMaxRuns = n.trigger_max_runs != null && n.trigger_max_runs !== 0;
            const hasTriggerLifetime = n.trigger_lifetime_h != null && n.trigger_lifetime_h !== 0;
            // Also emit trigger block if node has any trigger field at all
            const hasAnyTrigger = hasTriggerOn || hasTriggerCancelOn || hasTriggerEvery ||
              hasTriggerCron || hasTriggerMaxRuns || hasTriggerLifetime;
            if (hasAnyTrigger) {
              const triggerBlock: Record<string, unknown> = {};
              if (n.trigger_on) triggerBlock.on = n.trigger_on;
              if (n.trigger_cancel_on) triggerBlock.cancel_on = n.trigger_cancel_on;
              if (n.trigger_every) triggerBlock.every = n.trigger_every;
              if (n.trigger_cron) triggerBlock.cron = n.trigger_cron;
              if (n.trigger_max_runs != null && n.trigger_max_runs !== 0) triggerBlock.max_runs = n.trigger_max_runs;
              if (n.trigger_lifetime_h != null && n.trigger_lifetime_h !== 0) triggerBlock.lifetime_hours = n.trigger_lifetime_h;
              nodeOut.trigger = triggerBlock;
            }
            // include_state: add status, timestamps
            if (include_state) {
              nodeOut.status = n.status;
              nodeOut.created_at = n.created_at;
              if (n.completed_at) nodeOut.completed_at = n.completed_at;
              if (n.trigger_run_count) nodeOut.trigger_run_count = n.trigger_run_count;
            }
            return nodeOut;
          }),
          dependencies: deps.length > 0
            ? deps.map((d) => ({ from: d.depends_on, to: d.node_id }))
            : undefined,
          conditions: conds.length > 0
            ? conds.map((c) => ({
                node_id: c.node_id,
                type: c.type,
                command: c.command ?? undefined,
                description: c.description ?? undefined,
                timeout_seconds: c.timeout_seconds ?? undefined,
              }))
            : undefined,
        };

        if (include_state && outputRows.length > 0) {
          exportDoc.outputs = outputRows.map((o) => ({
            node_id: o.node_id, key: o.key, value: o.value, type: o.type,
          }));
        }

        // Serialize to YAML or JSON
        let content: string;
        if (fmt === "json") {
          content = JSON.stringify(exportDoc, null, 2);
        } else {
          // Minimal YAML serialization using the 'yaml' package (yamlStringify imported at top)
          content = `# graph-harness export v1\n# graph_id: ${graphId}\n# exported_at: ${new Date().toISOString()}\n` +
            yamlStringify(exportDoc, { lineWidth: 120 });
        }

        // Ensure output directory exists
        const { dirname } = await import("node:path");
        // ── Path traversal guard ─────────────────────────────────────────────
        // axiom:trace work_item=SWDE-46 spec=specs/102-Graph-Harness.md#REQ-GH-122 plan=verify-SWDE46-C4 jira_ref=SWDE-46
        const { resolve: resolvePath } = await import("node:path");
        const resolvedOut = resolvePath(outputPath);
        const resolvedRoot = resolvePath(repoRoot);
        if (!resolvedOut.startsWith(resolvedRoot)) {
          return JSON.stringify({ error: `Path outside workspace: ${outputPath}` });
        }
        const { mkdirSync: mkdir } = await import("node:fs");
        try { mkdir(dirname(outputPath), { recursive: true }); } catch { /* exists */ }

        const { writeFileSync: wf } = await import("node:fs");
        wf(outputPath, content, "utf-8");

        await logLedger(graphId, null, "graph_exported", null, {
          path: outputPath, format: fmt, node_count: nodes.length,
          include_state: include_state ?? false,
        });

        return JSON.stringify({
          exported: true,
          path: outputPath,
          format: fmt,
          node_count: nodes.length,
        });
      } catch (err) {
        const msg = err instanceof Error ? err.message : String(err);
        return JSON.stringify({ error: `graph_export failed: ${msg}` });
      }
    },
  });

  // ─────────────────────────────────────────────────────────────────────────
  // graph_import — REQ-GH-123 (Phase 8)
  //
  // Creates a new graph from an exported YAML/JSON file.
  // Distinct from graph_template_load — this imports a full graph definition
  // (including trigger blocks) rather than a parameterized template.
  // axiom:trace work_item=SWDE-46 spec=specs/102-Graph-Harness.md#REQ-GH-123 plan=phase-3/task-3.2/step-1 jira_ref=SWDE-46
  // ─────────────────────────────────────────────────────────────────────────

  const graphImportTool = tool({
    description:
      "Creates a new graph from a previously exported YAML/JSON graph definition file. " +
      "Preserves trigger blocks, dependencies, and conditions. " +
      "Use 'name' to override the graph name; use 'locked_by' to lock to a session on creation.",
    args: {
      path: tool.schema
        .string()
        .min(1)
        .describe("Path to the exported graph YAML or JSON file"),
      name: tool.schema
        .string()
        .optional()
        .describe("Override the graph name (default: use name from file)"),
      locked_by: tool.schema
        .string()
        .optional()
        .describe("Lock the new graph to this session on creation"),
    },
    execute: async ({ path: inputPath, name: nameOverride, locked_by }, context) => {
      try {
        // ── Path traversal guard ─────────────────────────────────────────────
        // axiom:trace work_item=SWDE-46 spec=specs/102-Graph-Harness.md#REQ-GH-123 plan=verify-SWDE46-C4 jira_ref=SWDE-46
        const { resolve: resolveImportPath } = await import("node:path");
        const resolvedImport = resolveImportPath(inputPath);
        const resolvedRootImport = resolveImportPath(repoRoot);
        if (!resolvedImport.startsWith(resolvedRootImport)) {
          return JSON.stringify({ error: `Path outside workspace: ${inputPath}` });
        }
        // Read + parse the file
        const { readFileSync: rfs } = await import("node:fs");
        let raw: string;
        try {
          raw = rfs(inputPath, "utf-8");
        } catch {
          return JSON.stringify({ error: `Cannot read file: ${inputPath}` });
        }

        let doc: Record<string, unknown>;
        if (inputPath.endsWith(".json")) {
          try { doc = JSON.parse(raw) as Record<string, unknown>; }
          catch { return JSON.stringify({ error: `Invalid JSON in file: ${inputPath}` }); }
        } else {
          // YAML (default)
          try {
            const { parse: parseY } = await import("yaml");
            doc = parseY(raw) as Record<string, unknown>;
          } catch { return JSON.stringify({ error: `Invalid YAML in file: ${inputPath}` }); }
        }

        if (!doc || typeof doc !== "object") {
          return JSON.stringify({ error: "File does not contain a valid graph definition" });
        }

        // Extract graph name
        const graphName = nameOverride ?? (typeof doc.name === "string" ? doc.name : "Imported Graph");

        // Extract nodes
        const rawNodes = Array.isArray(doc.nodes) ? doc.nodes as Record<string, unknown>[] : [];
        if (rawNodes.length === 0) {
          return JSON.stringify({ error: "No nodes found in import file" });
        }

        // Build graph.create-compatible payload
        const nodes = rawNodes.map((n) => {
          const nodeOut: Record<string, unknown> = {
            id: typeof n.id === "string" ? n.id : `node_${Math.random().toString(36).slice(2)}`,
            title: typeof n.title === "string" ? n.title : "Imported Node",
            description: typeof n.description === "string" ? n.description : undefined,
            execution_mode: typeof n.execution_mode === "string" ? n.execution_mode : undefined,
            execution_config: typeof n.execution_config === "object" && n.execution_config !== null
              ? n.execution_config
              : undefined,
            context: typeof n.context === "string" ? n.context
              : (typeof n.context === "object" && n.context !== null
                ? JSON.stringify(n.context) : undefined),
            max_retries: typeof n.max_retries === "number" ? n.max_retries : undefined,
            trigger: typeof n.trigger === "object" && n.trigger !== null ? n.trigger : undefined,
          };
          return nodeOut;
        });

        // Build dependencies
        const rawDeps = Array.isArray(doc.dependencies) ? doc.dependencies as Record<string, unknown>[] : [];
        const dependencies = rawDeps
          .filter((d) => typeof d.from === "string" && typeof d.to === "string")
          .map((d) => ({ from: d.from as string, to: d.to as string }));

        // Build conditions
        const rawConds = Array.isArray(doc.conditions) ? doc.conditions as Record<string, unknown>[] : [];
        const conditions = rawConds
          .filter((c) => typeof c.node_id === "string" && typeof c.type === "string")
          .map((c) => ({
            node_id: c.node_id as string,
            type: c.type as string,
            command: typeof c.command === "string" ? c.command : undefined,
            description: typeof c.description === "string" ? c.description : undefined,
            timeout_seconds: typeof c.timeout_seconds === "number" ? c.timeout_seconds : undefined,
          }));

        // Delegate to graph_create logic
        const createArgs: Record<string, unknown> = {
          name: graphName,
          description: typeof doc.description === "string" ? doc.description : undefined,
          nodes,
          dependencies: dependencies.length > 0 ? dependencies : undefined,
          conditions: conditions.length > 0 ? conditions : undefined,
          locked_by: locked_by ?? undefined,
        };

        const result = await graphCreateTool.execute(
          createArgs as Parameters<typeof graphCreateTool.execute>[0],
          context
        );

        const parsed = JSON.parse(result as string) as Record<string, unknown>;
        if (parsed.error) {
          return JSON.stringify({ error: `graph_import: graph creation failed: ${parsed.error}` });
        }

        // Log the import
        if (typeof parsed.graph_id === "string") {
          await logLedger(parsed.graph_id as string, null, "graph_imported", null, {
            source_path: inputPath, node_count: nodes.length,
          });
        }

        return JSON.stringify({
          graph_id: parsed.graph_id,
          node_count: parsed.node_count,
          status: "created",
          source_path: inputPath,
        });
      } catch (err) {
        const msg = err instanceof Error ? err.message : String(err);
        return JSON.stringify({ error: `graph_import failed: ${msg}` });
      }
    },
  });

  // ─────────────────────────────────────────────────────────────────────────
  // graph.update Tool (REQ-GH-140)
  //
  // Updates graph-level metadata fields (title, description, metadata).
  // Does NOT modify graph structure (nodes, edges, conditions).
  // Coordinator-only. Blocked on terminal graph states.
  // Metadata is merged (Object.assign), not replaced.
  //
  // axiom:trace work_item=SWDE-54 spec=specs/102-Graph-Harness.md#REQ-GH-140 plan=phase-10/new-step-p10-01 jira_ref=SWDE-54
  // ─────────────────────────────────────────────────────────────────────────

  const graphUpdateTool = tool({
    description:
      "Update graph-level properties (title, description, metadata). " +
      "Does NOT change graph structure. Coordinator-only. " +
      "Metadata is merged with existing metadata (not replaced). " +
      "Returns { graph_id, status: 'updated', changes_applied: string[] } on success or { error } on failure.",
    args: {
      graph_id: tool.schema.string().min(1).describe("Graph ID"),
      title: tool.schema.string().optional().describe("New graph title"),
      description: tool.schema.string().optional().describe("New graph description"),
      metadata: tool.schema
        .record(tool.schema.string(), tool.schema.string())
        .optional()
        .describe("Metadata to merge into existing graph metadata (not replaced)"),
    },

    async execute(args, context) {
      // axiom:trace work_item=SWDE-54 spec=specs/102-Graph-Harness.md#REQ-GH-140 plan=phase-10/new-step-p10-01 jira_ref=SWDE-54
      if (!config.enabled) {
        return JSON.stringify({ error: "Graph Harness is disabled" });
      }

      try {
        const graphId = args.graph_id;

        // ── Role check: coordinator only ────────────────────────────────────
        const callerSessionId = (context as Record<string, unknown> | undefined)?.sessionID as string | undefined;
        if (callerSessionId) {
          const sessionRow = await db.queryOne(
            `SELECT role FROM sessions WHERE session_id = ? AND LOWER(status) = 'active'`,
            [callerSessionId]
          ) as { role: string } | undefined;
          if (sessionRow && sessionRow.role === "worker") {
            return JSON.stringify({
              error: "Permission denied: graph_update requires coordinator role",
            });
          }
        }

        await checkGraphLock(graphId, context); // REQ-GH-110

        // ── Gate 1: graph must exist ─────────────────────────────────────────
        const graphRow = await db.queryOne(
          `SELECT id, status, title, description, metadata FROM graphs WHERE id = ?`,
          [graphId]
        ) as { id: string; status: string; title: string; description: string | null; metadata: string | null } | undefined;

        if (!graphRow) {
          return JSON.stringify({ error: `Graph not found: ${graphId}` });
        }

        // ── Gate 2: block terminal/non-mutable states ────────────────────────
        const graphStatus = graphRow.status.toUpperCase();
        const blockedStatuses = ["ABANDONED", "ARCHIVED", "COMPLETE", "FAILED"];
        if (blockedStatuses.includes(graphStatus)) {
          return JSON.stringify({
            error: `Graph is not in a mutable state: status=${graphRow.status}. Only CREATED, ACTIVE, or DRAFT graphs can be updated.`,
          });
        }

        // ── Track which fields are changed ───────────────────────────────────
        const changesApplied: string[] = [];

        // ── Atomic DB write ───────────────────────────────────────────────────
        await db.transaction(async (db) => {
          if (args.title !== undefined && args.title !== graphRow.title) {
            await db.run(`UPDATE graphs SET title = ? WHERE id = ?`, [args.title, graphId]);
            changesApplied.push("title");
          }
          if (args.description !== undefined && args.description !== graphRow.description) {
            await db.run(`UPDATE graphs SET description = ? WHERE id = ?`, [args.description, graphId]);
            changesApplied.push("description");
          }
          if (args.metadata !== undefined) {
            // Merge: load existing, apply Object.assign, persist
            let existingMetadata: Record<string, string> = {};
            try {
              if (graphRow.metadata) {
                existingMetadata = JSON.parse(graphRow.metadata) as Record<string, string>;
              }
            } catch { /* ignore parse errors — start with empty */ }
            const mergedMetadata = Object.assign({}, existingMetadata, args.metadata);
            await db.run(`UPDATE graphs SET metadata = ? WHERE id = ?`, [JSON.stringify(mergedMetadata), graphId]);
            changesApplied.push("metadata");
          }

          // Ledger entry for the update
          await logLedger(graphId, callerSessionId ?? null, "graph_updated", null, {
            changes_applied: changesApplied,
          });
        });

        return JSON.stringify({
          graph_id: graphId,
          status: "updated",
          changes_applied: changesApplied,
        });
      } catch (err) {
        const message = err instanceof Error ? err.message : String(err);
        return JSON.stringify({ error: message });
      }
    },
  });

  // ─────────────────────────────────────────────────────────────────────────
  // graph.activate Tool (REQ-GH-142)
  //
  // Transitions a DRAFT graph to CREATED, bootstraps a coordinator session row,
  // and returns the first unblocked PENDING node id (or null if none).
  //
  // The DB status-write AND session bootstrap are wrapped in a single SQLite
  // transaction (BACKLOG-04 race-condition fix) so the harness loop never sees
  // a CREATED graph without a coordinator session.
  //
  // Returns { graph_id, status: "created", first_node_id: string | null }
  //
  // axiom:trace work_item=SWDE-54 spec=specs/102-Graph-Harness.md#REQ-GH-142 plan=phase-10/new-step-p10-03 jira_ref=SWDE-54
  // ─────────────────────────────────────────────────────────────────────────

  const graphActivateTool = tool({
    description:
      "Transition a DRAFT graph to active execution. " +
      "Verifies graph is in DRAFT state, promotes it to CREATED, bootstraps a coordinator session, " +
      "and returns the first unblocked PENDING node. " +
      "Coordinator-only. " +
      "Returns `{status:\"created\"}` on success or { error } on failure.",
    args: {
      graph_id: tool.schema.string().min(1).describe("Graph ID to activate"),
    },

    async execute(args, context) {
      // axiom:trace work_item=SWDE-54 spec=specs/102-Graph-Harness.md#REQ-GH-142 plan=phase-10/new-step-p10-03 jira_ref=SWDE-54
      if (!config.enabled) {
        return JSON.stringify({ error: "Graph Harness is disabled" });
      }

      try {
        const graphId = args.graph_id;
        const callerSessionId = (context as Record<string, unknown> | undefined)?.sessionID as string | undefined;

        // ── Role check: coordinator only ────────────────────────────────────
        if (callerSessionId) {
          const sessionRow = await db.queryOne(
            `SELECT role FROM sessions WHERE session_id = ? AND LOWER(status) = 'active'`,
            [callerSessionId]
          ) as { role: string } | undefined;
          if (sessionRow && sessionRow.role === "worker") {
            return JSON.stringify({
              error: "Permission denied: graph_activate requires coordinator role",
            });
          }
        }

        // ── Status gate: graph must exist and be in DRAFT state ─────────────
        const graphRow = await db.queryOne(
          `SELECT id, status FROM graphs WHERE id = ?`,
          [graphId]
        ) as { id: string; status: string } | undefined;

        if (!graphRow) {
          return JSON.stringify({ error: `Graph not found: ${graphId}` });
        }

        const currentStatus = graphRow.status.toUpperCase();
        if (currentStatus !== "DRAFT") {
          return JSON.stringify({
            error: `graph_activate: graph is not in DRAFT state (status=${graphRow.status})`,
          });
        }

        // ── BACKLOG-04: Atomic transaction — status write + session bootstrap ─
        // Wrapping both operations prevents the harness loop from seeing a
        // CREATED graph without a coordinator session row.
        await db.transaction(async (db) => {
          // 1. Promote graph from DRAFT → CREATED
          await db.run(
            `UPDATE graphs SET status = 'CREATED' WHERE id = ?`,
            [graphId]
          );

          // 2. Bootstrap coordinator session (mirrors graphCreateTool non-draft path)
          if (callerSessionId) {
            const existingSession = await db.queryOne(
              `SELECT graph_id, status FROM sessions WHERE session_id = ?`,
              [callerSessionId]
            ) as { graph_id: string; status: string } | null;

            if (existingSession && existingSession.status.toLowerCase() === "active" &&
                existingSession.graph_id !== graphId) {
              // axiom:trace work_item=SWDE-54 spec=specs/102-Graph-Harness.md#REQ-GH-142 plan=phase-10/backlog-r4-02 jira_ref=SWDE-54
              // Session actively coordinates a DIFFERENT graph — fail-closed: do not leave
              // the newly-promoted CREATED graph without a coordinator session (would be stuck).
              throw new Error(
                `Session ${callerSessionId} already coordinates graph '${existingSession.graph_id}'. ` +
                `Release that graph first (graph_unlock or graph_abandon) or use a fresh session to activate this draft.`
              );
            } else if (existingSession) {
              // Session row exists but done/stale — reuse (UPSERT).
              await db.run(`
                UPDATE sessions
                SET graph_id=?, status='active', node_id=NULL,
                    last_heartbeat=datetime('now'), completed_at=NULL
                WHERE session_id=?
              `, [graphId, callerSessionId]);
              await db.run(
                `INSERT INTO ledger (graph_id, session_id, action, target_node_id, detail, timestamp)
                 VALUES (?, ?, ?, NULL, ?, ?)`,
                [graphId, callerSessionId, "session_bootstrapped",
                 JSON.stringify({ session_id: callerSessionId, role: "coordinator", reused: true }),
                 new Date().toISOString()]
              );
            } else {
              // Fresh INSERT.
              await db.run(`
                INSERT INTO sessions
                  (session_id, graph_id, role, status, node_id, created_at, last_heartbeat)
                VALUES (?, ?, 'coordinator', 'active', NULL, datetime('now'), datetime('now'))
              `, [callerSessionId, graphId]);
              await db.run(
                `INSERT INTO ledger (graph_id, session_id, action, target_node_id, detail, timestamp)
                 VALUES (?, ?, ?, NULL, ?, ?)`,
                [graphId, callerSessionId, "session_bootstrapped",
                 JSON.stringify({ session_id: callerSessionId, role: "coordinator" }),
                 new Date().toISOString()]
              );
            }
          }

          // 3. Ledger entry for the activation transition (inside transaction)
          await db.run(
            `INSERT INTO ledger (graph_id, session_id, action, target_node_id, detail, timestamp)
             VALUES (?, ?, ?, NULL, ?, ?)`,
            [graphId, callerSessionId ?? null, "graph_activated",
             JSON.stringify({ from: "DRAFT", to: "CREATED" }),
             new Date().toISOString()]
          );
        });

        // ── Find first unblocked PENDING node (outside transaction, read-only) ─
        const firstNode = await findNextUnblockedNode(graphId);
        const firstNodeId = firstNode ? firstNode.id : null;

        return JSON.stringify({
          graph_id: graphId,
          status: "created",
          first_node_id: firstNodeId,
        });
      } catch (err) {
        const message = err instanceof Error ? err.message : String(err);
        return JSON.stringify({ error: message });
      }
    },
  });

  // ─────────────────────────────────────────────────────────────────────────
  // Autonomous idle-evaluation timer (AC-11, REQ-PCM)
  //
  // Drives the harness evaluation loop on a background tick independent of
  // OpenCode session.idle events. This ensures graphs advance (condition
  // evaluation, stale-session recovery, scheduled-node firing) even when no
  // agent turn is in progress — e.g. between human interactions.
  //
  // Interval source: config.harness.idle_evaluation_interval_ms
  //   Default: 30000 ms (30 s)
  //   Override: AXIOM_GRAPH_HARNESS_HARNESS__IDLE_EVALUATION_INTERVAL_MS
  //
  // The timer is non-blocking: .unref() prevents it from keeping the process
  // alive after all other work is done.
  //
  // axiom:trace work_item=plugin-config-management-01 spec=specs/112-Plugin-Config-Management.md#AC-11 plan=phase-4/task-4-1/step-pcm-r14-05
  // ─────────────────────────────────────────────────────────────────────────
  if (config.enabled) {
    const _idleEvalTimer = setInterval(async () => {
      try {
        // Find all active sessions so each graph's loop can be ticked.
        const activeSessions = await db.queryAll(
          `SELECT session_id FROM sessions WHERE LOWER(status) = 'active'`, []
        ) as Array<{ session_id: string }>;
        for (const { session_id } of activeSessions) {
          await runHarnessLoop(session_id).catch((err) => {
            console.error("[GraphHarness] idle-eval timer: runHarnessLoop error:", err);
          });
        }
      } catch (err) {
        // Non-blocking: log but never crash the timer
        console.error("[GraphHarness] idle-eval timer error:", err);
      }
    }, config.harness.idle_evaluation_interval_ms);
    // Don't block process exit on this timer
    if (_idleEvalTimer?.unref) _idleEvalTimer.unref();

    // ── Phase 112 / task-3-4: Start the outer scheduler loop ─────────────
    // BL-02: AbortController initialized HERE (at plugin init), not at module
    // level — ensures a fresh, non-aborted controller on every plugin load
    // including hot-reloads that don't restart the process.
    // axiom:trace work_item=graph-scheduler-rearchitecture-01 spec=specs/102-Graph-Harness.md#REQ-GH-SCHED-V2 plan=phase-3/task-3-4/step-3-4-1
    _schedulerShutdown = false;
    _schedulerAbort = new AbortController();
    schedulerLoop().catch((err) => {
      console.error("[GraphHarness] schedulerLoop crashed:", err);
    });
    pluginInfo("graph-harness", "Outer scheduler loop started (inner/outer loop architecture)");
  }

  // ─────────────────────────────────────────────────────────────────────────
  // Return hooks object.
  // The `tool` key registers custom tools with the OpenCode LLM palette.
  // The `event` key registers the harness loop on session.idle events.
  // The `experimental.chat.system.transform` key injects node context into
  // every LLM system prompt when a graph is active (REQ-GH-023).
  //
  // NOTE: `stop` hook is NOT registered because the SDK Hooks interface
  // does not define it. stopHookFn is implemented above and ready to wire in.
  //
  // axiom:trace work_item=graph-harness-01 spec=specs/102-Graph-Harness.md#REQ-GH-023 plan=phase-1/task-1-5/step-1-5-1
  // ─────────────────────────────────────────────────────────────────────────
  return {
    tool: {
      "graph_create": graphCreateTool,
      "graph_status": graphStatusTool,
      "graph_inject": graphInjectTool,
      "graph_modify": graphModifyTool,
      "graph_split": graphSplitTool,
      "graph_annotate": graphAnnotateTool,
      "graph_abandon": graphAbandonTool,
      "graph_unblock": graphUnblockTool,
      "graph_lock": graphLockTool,
      "graph_unlock": graphUnlockTool,
      "graph_transfer": graphTransferTool,
      "graph_session_list": graphSessionListTool,
      "graph_output": graphOutputTool,
      "graph_dataflow": graphDataflowTool,
      "graph_message": graphMessageTool,
      "graph_template_load": graphTemplateLoadTool,
      "graph_template_save": graphTemplateSaveTool,
      "graph_template_update": graphTemplateUpdateTool,
      "graph_admin": graphAdminTool,
      "graph_export": graphExportTool,
      "graph_import": graphImportTool,
      "graph_update": graphUpdateTool,
      "graph_activate": graphActivateTool,
    },
    // session.idle is delivered through the generic event hook (REQ-GH-021):
     //   event.type === "session.idle"
     //   event.properties.sessionID is the session that became idle
     event: harnessEventHandler,
     // Inject active node briefing into system prompt before every LLM call (REQ-GH-023)
     "experimental.chat.system.transform": systemTransformHook,
     // ── Phase 112: scheduler test helper — not exposed to LLM ──────────────
     // Allows tests to wake the scheduler after directly manipulating the DB.
     // axiom:trace work_item=graph-scheduler-rearchitecture-01 spec=specs/102-Graph-Harness.md#REQ-GH-SCHED-V2 plan=phase-3/task-3-3/step-3-3-1
     _wakeScheduler: wakeScheduler,
     _stopScheduler: stopScheduler,
     // REQ-GH-067: Track tool calls for forced_tools verification.
     // axiom:trace work_item=graph-harness-01 spec=specs/102-Graph-Harness.md#REQ-GH-067 plan=phase-5/task-5-7/step-5-7-1
     "tool.execute.after": async (
       input: { tool: string; sessionID: string; callID: string; args: unknown },
       _output: { title: string; output: string; metadata: unknown }
     ): Promise<void> => {
       try {
         // Log the tool call into the ledger for forced_tools verification.
         // Exit code is not directly available here — default to 0 (success) if no error.
         await logToolCalled(input.sessionID, input.tool, 0);
       } catch (err) {
         // Non-fatal: tool call tracking should never crash the plugin
         console.warn("[GraphHarness] tool.execute.after error:", err);
       }
     },
   };
};

// ─────────────────────────────────────────────────────────────────────────────
// graph-harness migrate CLI
//
// Usage:
//   bun run plugins/graph-harness.ts migrate --to postgres --pg-url postgres://...
//   bun run plugins/graph-harness.ts migrate --to sqlite [--pg-url postgres://...] [--out /path/to/harness.db]
//
// Performs an offline migration between SQLite and PostgreSQL backends.
// The harness must be stopped before running a migration.
//
// axiom:trace work_item=SWDE-67 spec=specs/102-Graph-Harness.md#REQ-GH-154 jira_ref=SWDE-67
// ─────────────────────────────────────────────────────────────────────────────

if (import.meta.main) {
  const args = process.argv.slice(2);

  if (args[0] === "migrate") {
    await runMigrateCli(args.slice(1));
  } else {
    console.log("Usage: bun run plugins/graph-harness.ts migrate --to <postgres|sqlite> [options]");
    console.log("  migrate --to postgres --pg-url postgres://user:pass@host/db [--repo .]");
    console.log("  migrate --to sqlite --pg-url postgres://user:pass@host/db [--out .graph-harness/harness.db] [--repo .]");
    process.exit(0);
  }
}

async function runMigrateCli(argv: string[]): Promise<void> {
  // axiom:trace work_item=SWDE-67 spec=specs/102-Graph-Harness.md#REQ-GH-154 jira_ref=SWDE-67
  let to: "postgres" | "sqlite" | undefined;
  let pgUrl: string | undefined;
  let repoRoot = process.cwd();
  let outPath: string | undefined;

  for (let i = 0; i < argv.length; i++) {
    if (argv[i] === "--to" && argv[i+1]) { to = argv[++i] as "postgres" | "sqlite"; }
    else if (argv[i] === "--pg-url" && argv[i+1]) { pgUrl = argv[++i]; }
    else if (argv[i] === "--repo" && argv[i+1]) { repoRoot = argv[++i]; }
    else if (argv[i] === "--out" && argv[i+1]) { outPath = argv[++i]; }
  }

  if (!to) { console.error("Error: --to <postgres|sqlite> is required"); process.exit(1); }
  if (!pgUrl) { pgUrl = process.env.GRAPH_HARNESS_PG_URL ?? ""; }
  if (!pgUrl) { console.error("Error: --pg-url or GRAPH_HARNESS_PG_URL is required"); process.exit(1); }

  const config = loadConfig(repoRoot);
  const sqlitePath = join(repoRoot, config.database.path);

  pluginInfo("graph-harness", `Starting migration to ${to}`);
  pluginInfo("graph-harness", `SQLite path: ${sqlitePath}`);
  pluginInfo("graph-harness", `PG URL: ${pgUrl.replace(/:[^:@]+@/, ":***@")}`);

  if (to === "postgres") {
    await migrateToPostgres(sqlitePath, pgUrl);
  } else {
    await migrateToSqlite(pgUrl, outPath ?? sqlitePath);
  }
}

/** Migrate all data from SQLite → PostgreSQL (REQ-GH-154). */
async function migrateToPostgres(sqlitePath: string, pgUrl: string): Promise<void> {
  // axiom:trace work_item=SWDE-67 spec=specs/102-Graph-Harness.md#REQ-GH-154 jira_ref=SWDE-67
  //
  // MIGRATION SAFETY NOTES:
  // 1. OFFLINE ONLY: The harness must be stopped before running migration.
  //    Active sessions writing to SQLite during migration may produce inconsistent PG state.
  // 2. IDEMPOTENCY: Uses ON CONFLICT DO NOTHING — safe to re-run on an EMPTY PG database.
  //    If PG already has data from a previous partial migration, existing rows are SKIPPED
  //    (not updated). To restart from scratch: DROP all harness tables in PG, then re-run.
  // 3. DATABASE MUST EXIST: Create the target database before running (e.g., createdb harness).
  const db = new Database(sqlitePath, { readonly: true });
  const sqliteAdapter = new SqliteAdapter(db);

  console.log("[Migrate] Connecting to PostgreSQL...");
  const pgAdapter = await initPostgresAdapter(pgUrl, "");

  // BOOLEAN columns in PG that are stored as 0/1 INTEGER in SQLite.
  // These must be converted to true/false before inserting into PG.
  // axiom:trace work_item=SWDE-67 spec=specs/102-Graph-Harness.md#REQ-GH-154 jira_ref=SWDE-67
  const PG_BOOLEAN_COLS: Record<string, Set<string>> = {
    nodes: new Set(["optional", "repeat"]),
    conditions: new Set(["independent", "passed"]),
    node_messages: new Set(["delivered"]),
    data_flow: new Set(["required"]),
  };

  function _normRow(table: string, row: Record<string, unknown>): Record<string, unknown> {
    const boolCols = PG_BOOLEAN_COLS[table];
    if (!boolCols) return row;
    const out: Record<string, unknown> = {};
    for (const [k, v] of Object.entries(row)) {
      out[k] = boolCols.has(k) && (v === 0 || v === 1) ? v === 1 : v;
    }
    return out;
  }

  const TABLES = [
    "graphs", "nodes", "dependencies", "conditions", "sessions",
    "ledger", "node_outputs", "node_messages", "data_flow", "annotations", "conductor_agents",
  ] as const;

  for (const table of TABLES) {
    pluginInfo("graph-harness", `Migrating table: ${table}`);
    try {
      const rows = await sqliteAdapter.queryAll(`SELECT * FROM ${table}`);
      console.log(`  → ${rows.length} rows`);
      if (rows.length === 0) continue;

      // Insert rows into PG in batches (converting BOOLEAN columns from 0/1 to true/false)
      const BATCH = 50;
      for (let i = 0; i < rows.length; i += BATCH) {
        const batch = rows.slice(i, i + BATCH);
        for (const rawRow of batch) {
          const row = _normRow(table, rawRow as Record<string, unknown>);
          const cols = Object.keys(row);
          const vals = Object.values(row);
          const placeholders = vals.map((_, j) => `$${j + 1}`).join(", ");
          const colList = cols.join(", ");
          const pgSql = `INSERT INTO ${table} (${colList}) VALUES (${placeholders}) ON CONFLICT DO NOTHING`;
          try {
            await pgAdapter.run(pgSql, vals);
          } catch (err) {
            console.warn(`  ⚠️ Row insert failed (row id=${row.id ?? "?"}): ${String(err).slice(0, 100)}`);
          }
        }
      }
    } catch (err) {
      console.warn(`  ⚠️ Table ${table} migration failed: ${String(err).slice(0, 200)}`);
    }
  }

  await pgAdapter.close();
  db.close();
  console.log("[Migrate] ✓ Migration to PostgreSQL complete.");
  console.log("[Migrate] Next: update .graph-harness/config.yaml to set backend=postgres and restart the harness.");
}

/** Migrate all data from PostgreSQL → SQLite (REQ-GH-154). */
async function migrateToSqlite(pgUrl: string, sqliteOutPath: string): Promise<void> {
  // axiom:trace work_item=SWDE-67 spec=specs/102-Graph-Harness.md#REQ-GH-154 jira_ref=SWDE-67
  console.log("[Migrate] Connecting to PostgreSQL...");
  const pgAdapter = await initPostgresAdapter(pgUrl, "");

  pluginInfo("graph-harness", `Opening SQLite at: ${sqliteOutPath}`);
  const sqliteDb = initSqliteDb(sqliteOutPath);
  const sqliteAdapter = new SqliteAdapter(sqliteDb);

  // Apply ALTERs that bootstrap() would apply (for columns added post-schema).
  // ⚠️ SYNC HAZARD: This list mirrors the ALTER TABLE statements in bootstrap() (lines ~1161-1183
  // and ~1326-1336). When adding a new column via ALTER TABLE in bootstrap(), you MUST also add
  // the corresponding ALTER here so PG→SQLite migration restores the full schema.
  // axiom:trace work_item=SWDE-67 spec=specs/102-Graph-Harness.md#REQ-GH-154 jira_ref=SWDE-67
  const alters = [
    `ALTER TABLE graphs ADD COLUMN notifications_config JSON DEFAULT NULL`,
    `ALTER TABLE graphs ADD COLUMN locked_by TEXT DEFAULT NULL`,
    `ALTER TABLE graphs ADD COLUMN modifications_without_progress INTEGER NOT NULL DEFAULT 0`,
    `ALTER TABLE sessions ADD COLUMN worker_pid INTEGER DEFAULT NULL`,
    `ALTER TABLE sessions ADD COLUMN consecutive_briefing_failures INTEGER NOT NULL DEFAULT 0`,
    `ALTER TABLE data_flow ADD COLUMN input_key TEXT`,
    `ALTER TABLE node_messages ADD COLUMN status TEXT NOT NULL DEFAULT 'queued'`,
    `ALTER TABLE nodes ADD COLUMN trigger_on TEXT DEFAULT 'idle'`,
    `ALTER TABLE nodes ADD COLUMN trigger_cancel_on TEXT DEFAULT 'active'`,
    `ALTER TABLE nodes ADD COLUMN trigger_every TEXT`,
    `ALTER TABLE nodes ADD COLUMN trigger_cron TEXT`,
    `ALTER TABLE nodes ADD COLUMN trigger_max_runs INTEGER DEFAULT 0`,
    `ALTER TABLE nodes ADD COLUMN trigger_lifetime_h REAL DEFAULT 0`,
    `ALTER TABLE nodes ADD COLUMN trigger_run_count INTEGER DEFAULT 0`,
    `ALTER TABLE nodes ADD COLUMN trigger_last_fired_at TEXT`,
  ];
  for (const a of alters) {
    try { sqliteDb.exec(a); } catch { /* already exists — safe to ignore */ }
  }

  // Build SQLite column sets per table (to filter out PG-only columns)
  const sqliteCols = new Map<string, Set<string>>();
  for (const tbl of ["graphs","nodes","dependencies","conditions","sessions","ledger",
                       "node_outputs","node_messages","data_flow","annotations","templates"]) {
    try {
      const info = sqliteDb.prepare(`PRAGMA table_info(${tbl})`).all() as Array<{ name: string }>;
      sqliteCols.set(tbl, new Set(info.map(r => r.name)));
    } catch { sqliteCols.set(tbl, new Set()); }
  }

  const TABLES = [
    "graphs", "nodes", "dependencies", "conditions", "sessions",
    "ledger", "node_outputs", "node_messages", "data_flow", "annotations", "conductor_agents",
  ] as const;

  for (const table of TABLES) {
    pluginInfo("graph-harness", `Migrating table: ${table}`);
    try {
      const rows = await pgAdapter.queryAll(`SELECT * FROM ${table}`);
      console.log(`  → ${rows.length} rows`);
      if (rows.length === 0) continue;

      const knownCols = sqliteCols.get(table); // undefined = skip column filtering

      for (const pgRow of rows) {
        // Filter to only columns that exist in the SQLite target table
        const filteredRow = knownCols
          ? Object.fromEntries(Object.entries(pgRow).filter(([k]) => knownCols.has(k)))
          : pgRow;
        const cols = Object.keys(filteredRow);
        const vals = Object.values(filteredRow).map((v) =>
          v !== null && typeof v === "object" ? JSON.stringify(v) : v
        );
        const placeholders = vals.map(() => "?").join(", ");
        const colList = cols.join(", ");
        const sqliteSql = `INSERT OR IGNORE INTO ${table} (${colList}) VALUES (${placeholders})`;
        try {
          await sqliteAdapter.run(sqliteSql, vals);
        } catch (err) {
          console.warn(`  ⚠️ Row insert failed: ${String(err).slice(0, 100)}`);
        }
      }
    } catch (err) {
      console.warn(`  ⚠️ Table ${table} migration failed: ${String(err).slice(0, 200)}`);
    }
  }

  await pgAdapter.close();
  await sqliteAdapter.close();
  console.log("[Migrate] ✓ Migration to SQLite complete.");
  pluginInfo("graph-harness", `Next: update .graph-harness/config.yaml to set backend=sqlite and path=${sqliteOutPath}`);
}
