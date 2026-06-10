/**
 * Tests for .opencode/shared/sqlite.ts — the canonical shared SQLite utilities module.
 *
 * axiom:trace work_item=SWDE-62 spec=specs/102-Graph-Harness.md#4.1 plan=phase-1/task-1-1/step-1-1-2 jira_ref=SWDE-62
 */

import { test, expect, describe, afterEach } from "bun:test";
import {
  RETRYABLE_SQLITE_CODES,
  isRetryableSqliteError,
  sqliteWriteWithRetry,
  sqliteReadWithRetry,
  openDatabase,
} from "../shared/sqlite";
import { mkdtempSync, rmSync } from "node:fs";
import { join } from "node:path";
import { tmpdir } from "node:os";

// ─────────────────────────────────────────────────────────────────────────────
// RETRYABLE_SQLITE_CODES
// ─────────────────────────────────────────────────────────────────────────────

describe("RETRYABLE_SQLITE_CODES", () => {
  test("contains SQLITE_BUSY", () =>
    expect(RETRYABLE_SQLITE_CODES.has("SQLITE_BUSY")).toBe(true));

  test("contains SQLITE_LOCKED", () =>
    expect(RETRYABLE_SQLITE_CODES.has("SQLITE_LOCKED")).toBe(true));

  test("contains SQLITE_IOERR", () =>
    expect(RETRYABLE_SQLITE_CODES.has("SQLITE_IOERR")).toBe(true));

  test("contains SQLITE_IOERR_VNODE (macOS errno 6922)", () =>
    expect(RETRYABLE_SQLITE_CODES.has("SQLITE_IOERR_VNODE")).toBe(true));

  test("contains SQLITE_IOERR_SHMMAP", () =>
    expect(RETRYABLE_SQLITE_CODES.has("SQLITE_IOERR_SHMMAP")).toBe(true));

  test("contains SQLITE_PROTOCOL", () =>
    expect(RETRYABLE_SQLITE_CODES.has("SQLITE_PROTOCOL")).toBe(true));

  test("does NOT contain SQLITE_CONSTRAINT (non-retryable)", () =>
    expect(RETRYABLE_SQLITE_CODES.has("SQLITE_CONSTRAINT")).toBe(false));

  test("does NOT contain SQLITE_ERROR (generic error, not transient)", () =>
    expect(RETRYABLE_SQLITE_CODES.has("SQLITE_ERROR")).toBe(false));

  test("contains at least 14 error codes (regression guard)", () =>
    expect(RETRYABLE_SQLITE_CODES.size).toBeGreaterThanOrEqual(14));
});

// ─────────────────────────────────────────────────────────────────────────────
// isRetryableSqliteError
// ─────────────────────────────────────────────────────────────────────────────

describe("isRetryableSqliteError", () => {
  test("returns true for error with code SQLITE_BUSY", () =>
    expect(isRetryableSqliteError({ code: "SQLITE_BUSY" })).toBe(true));

  test("returns true for error with code SQLITE_LOCKED", () =>
    expect(isRetryableSqliteError({ code: "SQLITE_LOCKED" })).toBe(true));

  test("returns true for error with code SQLITE_IOERR_VNODE", () =>
    expect(isRetryableSqliteError({ code: "SQLITE_IOERR_VNODE" })).toBe(true));

  test("returns true for message containing SQLITE_IOERR (message-path fallback)", () =>
    expect(isRetryableSqliteError({ message: "SQLITE_IOERR: disk I/O error" })).toBe(true));

  test("returns true for message containing SQLITE_BUSY (no code property)", () =>
    expect(isRetryableSqliteError({ message: "SQLITE_BUSY: database is locked" })).toBe(true));

  test("returns true for message containing 'disk I/O error'", () =>
    expect(isRetryableSqliteError({ message: "disk I/O error occurred" })).toBe(true));

  test("returns false for non-retryable code SQLITE_CONSTRAINT", () =>
    expect(isRetryableSqliteError({ code: "SQLITE_CONSTRAINT" })).toBe(false));

  test("returns false for non-retryable code SQLITE_NOTADB", () =>
    expect(isRetryableSqliteError({ code: "SQLITE_NOTADB" })).toBe(false));

  test("returns false for non-object (string)", () =>
    expect(isRetryableSqliteError("error string")).toBe(false));

  test("returns false for null", () =>
    expect(isRetryableSqliteError(null)).toBe(false));

  test("returns false for undefined", () =>
    expect(isRetryableSqliteError(undefined)).toBe(false));

  test("returns false for empty object (no code or message)", () =>
    expect(isRetryableSqliteError({})).toBe(false));
});

// ─────────────────────────────────────────────────────────────────────────────
// sqliteWriteWithRetry
// ─────────────────────────────────────────────────────────────────────────────

describe("sqliteWriteWithRetry", () => {
  test("returns true when fn succeeds on first attempt", () => {
    let calls = 0;
    const result = sqliteWriteWithRetry(() => { calls++; }, "test-write-ok");
    expect(result).toBe(true);
    expect(calls).toBe(1);
  });

  test("retries on SQLITE_BUSY and returns true when fn eventually succeeds", () => {
    let calls = 0;
    const result = sqliteWriteWithRetry(
      () => {
        calls++;
        if (calls < 2) throw Object.assign(new Error("busy"), { code: "SQLITE_BUSY" });
      },
      "test-write-retry",
      3,
      1, // 1ms base delay for test speed
    );
    expect(result).toBe(true);
    expect(calls).toBe(2);
  });

  test("retries up to maxRetries then returns false on persistent SQLITE_BUSY", () => {
    let calls = 0;
    const result = sqliteWriteWithRetry(
      () => {
        calls++;
        throw Object.assign(new Error("busy"), { code: "SQLITE_BUSY" });
      },
      "test-write-exhaust",
      2, // maxRetries=2 → total 3 attempts
      1,
    );
    expect(result).toBe(false);
    expect(calls).toBe(3); // initial + 2 retries
  });

  test("does NOT retry on non-retryable SQLITE_CONSTRAINT", () => {
    let calls = 0;
    const result = sqliteWriteWithRetry(
      () => {
        calls++;
        throw Object.assign(new Error("constraint violation"), { code: "SQLITE_CONSTRAINT" });
      },
      "test-write-no-retry",
      3,
      1,
    );
    expect(result).toBe(false);
    expect(calls).toBe(1); // no retry
  });

  test("retries on SQLITE_IOERR_VNODE (message-path classification)", () => {
    let calls = 0;
    const result = sqliteWriteWithRetry(
      () => {
        calls++;
        if (calls < 3) throw new Error("SQLITE_IOERR: disk I/O error (vnode)");
      },
      "test-write-ioerr",
      3,
      1,
    );
    expect(result).toBe(true);
    expect(calls).toBe(3);
  });
});

// ─────────────────────────────────────────────────────────────────────────────
// sqliteReadWithRetry
// ─────────────────────────────────────────────────────────────────────────────

describe("sqliteReadWithRetry", () => {
  test("returns the function result on success", () => {
    const result = sqliteReadWithRetry(() => 42, "test-read-ok");
    expect(result).toBe(42);
  });

  test("retries on SQLITE_BUSY and returns value when fn eventually succeeds", () => {
    let calls = 0;
    const result = sqliteReadWithRetry(
      () => {
        calls++;
        if (calls < 2) throw Object.assign(new Error("busy"), { code: "SQLITE_BUSY" });
        return "success";
      },
      "test-read-retry",
      3,
      1,
    );
    expect(result).toBe("success");
    expect(calls).toBe(2);
  });

  test("returns null after exhausting retries on persistent SQLITE_BUSY", () => {
    let calls = 0;
    const result = sqliteReadWithRetry(
      () => {
        calls++;
        throw Object.assign(new Error("busy"), { code: "SQLITE_BUSY" });
        return "never";
      },
      "test-read-exhaust",
      2,
      1,
    );
    expect(result).toBeNull();
    expect(calls).toBe(3);
  });

  test("returns null without retrying on non-retryable error", () => {
    let calls = 0;
    const result = sqliteReadWithRetry(
      () => {
        calls++;
        throw Object.assign(new Error("constraint"), { code: "SQLITE_CONSTRAINT" });
        return "never";
      },
      "test-read-no-retry",
      3,
      1,
    );
    expect(result).toBeNull();
    expect(calls).toBe(1);
  });
});

// ─────────────────────────────────────────────────────────────────────────────
// openDatabase — PRAGMA assertions
// ─────────────────────────────────────────────────────────────────────────────

describe("openDatabase", () => {
  const tmpDirs: string[] = [];
  afterEach(() => {
    for (const d of tmpDirs.splice(0)) {
      try { rmSync(d, { recursive: true }); } catch { /* best-effort */ }
    }
  });

  test("sets journal_mode=WAL", () => {
    const dir = mkdtempSync(join(tmpdir(), "sqlite-shared-test-"));
    tmpDirs.push(dir);
    const db = openDatabase(join(dir, "test.db"));
    const row = db.prepare("PRAGMA journal_mode").get() as { journal_mode: string };
    expect(row.journal_mode).toBe("wal");
    db.close();
  });

  test("sets busy_timeout=10000 (default)", () => {
    const dir = mkdtempSync(join(tmpdir(), "sqlite-shared-test-"));
    tmpDirs.push(dir);
    const db = openDatabase(join(dir, "test.db"));
    // Note: bun:sqlite PRAGMA busy_timeout returns column named "timeout", not "busy_timeout"
    const row = db.prepare("PRAGMA busy_timeout").get() as { timeout: number };
    expect(row.timeout).toBe(10000);
    db.close();
  });

  test("sets synchronous=NORMAL (1)", () => {
    const dir = mkdtempSync(join(tmpdir(), "sqlite-shared-test-"));
    tmpDirs.push(dir);
    const db = openDatabase(join(dir, "test.db"));
    const row = db.prepare("PRAGMA synchronous").get() as { synchronous: number };
    expect(row.synchronous).toBe(1); // NORMAL = 1
    db.close();
  });

  test("sets foreign_keys=ON (1)", () => {
    const dir = mkdtempSync(join(tmpdir(), "sqlite-shared-test-"));
    tmpDirs.push(dir);
    const db = openDatabase(join(dir, "test.db"));
    const row = db.prepare("PRAGMA foreign_keys").get() as { foreign_keys: number };
    expect(row.foreign_keys).toBe(1);
    db.close();
  });

  test("sets wal_autocheckpoint=1000 (default)", () => {
    const dir = mkdtempSync(join(tmpdir(), "sqlite-shared-test-"));
    tmpDirs.push(dir);
    const db = openDatabase(join(dir, "test.db"));
    const row = db.prepare("PRAGMA wal_autocheckpoint").get() as { wal_autocheckpoint: number };
    expect(row.wal_autocheckpoint).toBe(1000);
    db.close();
  });

  test("respects custom busyTimeoutMs option", () => {
    const dir = mkdtempSync(join(tmpdir(), "sqlite-shared-test-"));
    tmpDirs.push(dir);
    const db = openDatabase(join(dir, "test.db"), { busyTimeoutMs: 3000 });
    const row = db.prepare("PRAGMA busy_timeout").get() as { timeout: number };
    expect(row.timeout).toBe(3000);
    db.close();
  });

  test("respects custom walAutocheckpoint option", () => {
    const dir = mkdtempSync(join(tmpdir(), "sqlite-shared-test-"));
    tmpDirs.push(dir);
    const db = openDatabase(join(dir, "test.db"), { walAutocheckpoint: 500 });
    const row = db.prepare("PRAGMA wal_autocheckpoint").get() as { wal_autocheckpoint: number };
    expect(row.wal_autocheckpoint).toBe(500);
    db.close();
  });

  test("startupCheckpoint:true does not throw", () => {
    const dir = mkdtempSync(join(tmpdir(), "sqlite-shared-test-"));
    tmpDirs.push(dir);
    expect(() => {
      const db = openDatabase(join(dir, "test.db"), { startupCheckpoint: true });
      db.close();
    }).not.toThrow();
  });
});

// ─────────────────────────────────────────────────────────────────────────────
// openDatabase + initConductorDB (production init path — regression guard)
// ─────────────────────────────────────────────────────────────────────────────

describe("openDatabase + initConductorDB (production path)", () => {
  // Import initConductorDB here so this test proves the actual production sequence.
  // If initConductorDB ever reintroduces a PRAGMA busy_timeout override, this test fails.
  test("busy_timeout remains 10000ms after openDatabase() + initConductorDB()", async () => {
    const { initConductorDB } = await import("../lib/conductor.ts");
    const dir = mkdtempSync(join(tmpdir(), "sqlite-conductor-prod-"));
    try {
      const db = openDatabase(join(dir, "conductor.db"));
      initConductorDB(db); // must NOT override busy_timeout
      // Note: bun:sqlite PRAGMA busy_timeout returns column named "timeout"
      const row = db.prepare("PRAGMA busy_timeout").get() as { timeout: number };
      expect(row.timeout).toBe(10000);
      db.close();
    } finally {
      try { rmSync(dir, { recursive: true }); } catch { /* best-effort */ }
    }
  });

  test("journal_mode remains WAL after openDatabase() + initConductorDB()", async () => {
    const { initConductorDB } = await import("../lib/conductor.ts");
    const dir = mkdtempSync(join(tmpdir(), "sqlite-conductor-mode-"));
    try {
      const db = openDatabase(join(dir, "conductor.db"));
      initConductorDB(db);
      const row = db.prepare("PRAGMA journal_mode").get() as { journal_mode: string };
      expect(row.journal_mode).toBe("wal");
      db.close();
    } finally {
      try { rmSync(dir, { recursive: true }); } catch { /* best-effort */ }
    }
  });
});
