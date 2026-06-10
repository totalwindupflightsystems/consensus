/**
 * Shared SQLite utilities for Axiom OpenCode plugins.
 *
 * Provides canonical SQLite initialization (WAL mode, busy_timeout, wal_autocheckpoint,
 * synchronous=NORMAL, foreign_keys=ON) and retry helpers for transient I/O errors.
 *
 * All plugins that open SQLite databases MUST use openDatabase() to ensure consistent
 * behaviour across graph-harness, conductor, and opencode-session.
 *
 * Background: Multiple OpenCode sessions share the same .graph-harness/harness.db.
 * Transient SQLITE_IOERR_VNODE (errno 6922) and SQLITE_BUSY errors are common in
 * this scenario. The retry helpers wrap operations with exponential backoff so
 * individual tool calls survive brief lock contention without surfacing errors.
 *
 * axiom:trace work_item=SWDE-62 spec=specs/102-Graph-Harness.md#4.1 plan=phase-1/task-1-1/step-1-1-1 jira_ref=SWDE-62
 */

import { Database } from "bun:sqlite";

// ─────────────────────────────────────────────────────────────────────────────
// Error classification
// ─────────────────────────────────────────────────────────────────────────────

/** Error codes that are retryable (transient I/O or lock contention). */
export const RETRYABLE_SQLITE_CODES = new Set([
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
 * Checks both `.code` and `.message` because bun:sqlite sometimes only sets the message.
 */
export function isRetryableSqliteError(err: unknown): boolean {
  if (!err || typeof err !== "object") return false;
  const code = (err as { code?: string }).code;
  if (code && RETRYABLE_SQLITE_CODES.has(code)) return true;
  const msg = (err as { message?: string }).message ?? "";
  if (
    msg.includes("SQLITE_IOERR") ||
    msg.includes("SQLITE_BUSY") ||
    msg.includes("SQLITE_LOCKED") ||
    msg.includes("disk I/O error")
  ) {
    return true;
  }
  return false;
}

// ─────────────────────────────────────────────────────────────────────────────
// Retry helpers
// ─────────────────────────────────────────────────────────────────────────────

// Module-level singleton for synchronous retry sleep (Atomics.wait).
// Allocated once to avoid repeated SharedArrayBuffer OS-level allocation under
// concurrent retry load. Safe for concurrent use — Atomics.wait on a zeroed
// buffer is a sleep-only operation; the value is never written.
// axiom:trace work_item=SWDE-62 spec=specs/102-Graph-Harness.md#4.1 jira_ref=SWDE-62
const _retryBuf = new SharedArrayBuffer(4);
const _retryView = new Int32Array(_retryBuf);

/**
 * Execute a SQLite write operation with retry + exponential backoff.
 *
 * @param fn        The synchronous write operation to execute.
 * @param label     Human-readable label for error logging.
 * @param maxRetries  Max retry attempts (default: 3).
 * @param baseDelayMs Base delay between retries in ms (default: 50).
 * @returns true if the operation succeeded, false if all retries were exhausted.
 */
export function sqliteWriteWithRetry(
  fn: () => void,
  label: string,
  maxRetries: number = 3,
  baseDelayMs: number = 50,
): boolean {
  for (let attempt = 0; attempt <= maxRetries; attempt++) {
    try {
      fn();
      return true;
    } catch (err) {
      if (!isRetryableSqliteError(err) || attempt === maxRetries) {
        if (attempt > 0) {
          console.warn(`[sqlite] ${label}: failed after ${attempt + 1} attempts:`, err);
        } else {
          console.warn(`[sqlite] ${label}: non-retryable error:`, err);
        }
        return false;
      }
      // Exponential backoff with jitter: baseDelay * 2^attempt + random(0..baseDelay)
      const delay = baseDelayMs * Math.pow(2, attempt) + Math.random() * baseDelayMs;
      // Synchronous sleep using Atomics.wait (Bun supports SharedArrayBuffer)
      Atomics.wait(_retryView, 0, 0, Math.ceil(delay));
    }
  }
  return false;
}

/**
 * Execute a SQLite read operation with retry + exponential backoff.
 * Same as sqliteWriteWithRetry but returns the result or null on failure.
 *
 * @param fn        The synchronous read operation.
 * @param label     Human-readable label for error logging.
 * @param maxRetries  Max retry attempts (default: 3).
 * @param baseDelayMs Base delay between retries in ms (default: 50).
 * @returns The result of fn, or null if all retries were exhausted.
 */
export function sqliteReadWithRetry<T>(
  fn: () => T,
  label: string,
  maxRetries: number = 3,
  baseDelayMs: number = 50,
): T | null {
  for (let attempt = 0; attempt <= maxRetries; attempt++) {
    try {
      return fn();
    } catch (err) {
      if (!isRetryableSqliteError(err) || attempt === maxRetries) {
        if (attempt > 0) {
          console.warn(`[sqlite] ${label}: read failed after ${attempt + 1} attempts:`, err);
        } else {
          console.warn(`[sqlite] ${label}: non-retryable read error:`, err);
        }
        return null;
      }
      const delay = baseDelayMs * Math.pow(2, attempt) + Math.random() * baseDelayMs;
      Atomics.wait(_retryView, 0, 0, Math.ceil(delay));
    }
  }
  return null;
}

// ─────────────────────────────────────────────────────────────────────────────
// Database initialisation
// ─────────────────────────────────────────────────────────────────────────────

/** Options for openDatabase(). */
export interface OpenDatabaseOptions {
  /**
   * busy_timeout in milliseconds.
   * Default: 10000 (10 s). Handles concurrent session contention.
   * For network filesystem deployments, increase further.
   */
  busyTimeoutMs?: number;

  /**
   * WAL auto-checkpoint threshold in pages (1 page ≈ 4 KB).
   * Default: 1000 (~4 MB). Prevents unbounded WAL file growth.
   */
  walAutocheckpoint?: number;

  /**
   * Whether to run a PASSIVE WAL checkpoint on open.
   * Default: false. Set to true for the primary DB writer (graph-harness)
   * to merge any WAL data left by previously crashed sessions.
   */
  startupCheckpoint?: boolean;
}

/**
 * Open a SQLite database with canonical Axiom settings:
 *   - WAL journal mode
 *   - busy_timeout (default 10 000 ms)
 *   - synchronous = NORMAL
 *   - foreign_keys = ON
 *   - wal_autocheckpoint (default 1 000 pages)
 *   - optional startup PASSIVE WAL checkpoint
 *
 * All plugins that use SQLite MUST open their database through this function
 * to ensure consistent behaviour across shared databases.
 *
 * axiom:trace work_item=SWDE-62 spec=specs/102-Graph-Harness.md#4.1 jira_ref=SWDE-62
 */
export function openDatabase(dbPath: string, opts: OpenDatabaseOptions = {}): Database {
  const {
    busyTimeoutMs = 10000,
    walAutocheckpoint = 1000,
    startupCheckpoint = false,
  } = opts;

  const db = new Database(dbPath);

  db.exec("PRAGMA journal_mode = WAL;");
  db.exec(`PRAGMA busy_timeout = ${busyTimeoutMs};`);
  db.exec("PRAGMA synchronous = NORMAL;");
  db.exec("PRAGMA foreign_keys = ON;");
  db.exec(`PRAGMA wal_autocheckpoint = ${walAutocheckpoint};`);

  if (startupCheckpoint) {
    try {
      db.exec("PRAGMA wal_checkpoint(PASSIVE)");
    } catch (cpErr) {
      console.warn(
        "[sqlite] Startup WAL checkpoint skipped (likely another session holds lock):",
        cpErr,
      );
    }
  }

  return db;
}
