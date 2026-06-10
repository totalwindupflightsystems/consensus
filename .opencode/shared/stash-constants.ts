/**
 * Shared constants for Context Stash — extracted from plugins/context-stash.ts
 * so the plugin loader doesn't crash on non-function exports.
 *
 * These are used by tests and by the plugin implementation.
 *
 * axiom:trace work_item=plugin-tool-registration-fix spec=specs/106-Context-Stash.md
 */

// Type inlined here to avoid circular import (context-stash.ts → shared → context-stash.ts)
export interface StashConfig {
  enabled: boolean;
  storage_path: string;
  active_size_limit_kb: number;
  compaction: {
    auto: boolean;
    keep_types: string[];
    keep_recent: number;
    snapshot_before: boolean;
  };
  lifecycle: {
    suspend_ttl_days: number;
    closed_ttl_days: number;
    archive_retain_days: number;
  };
  managed_context: {
    default_log_level: string;
    log_tool_categories: string[];
    max_simultaneous_active: number;
  };
  git: {
    track_active: boolean;
    track_suspended: boolean;
  };
}

/**
 * REQ-STASH-NEW-002: Stash ID must match ^[a-z0-9][a-z0-9-]{0,63}$.
 */
export const STASH_ID_REGEX = /^[a-z0-9][a-z0-9-]{0,63}$/;

export const DEFAULT_STASH_CONFIG: StashConfig = {
  enabled: true,
  storage_path: ".memory-bank/stash",
  active_size_limit_kb: 2048,
  compaction: {
    auto: true,
    keep_types: ["finding", "decision", "handoff", "blocker"],
    keep_recent: 10,
    snapshot_before: true,
  },
  lifecycle: {
    suspend_ttl_days: 30,
    closed_ttl_days: 90,
    archive_retain_days: 365,
  },
  managed_context: {
    default_log_level: "decisions",
    log_tool_categories: ["write", "edit", "bash"],
    max_simultaneous_active: 5,
  },
  git: {
    track_active: false,
    track_suspended: false,
  },
};

/**
 * Advisory lock TTL in milliseconds. Must be ≥300000 (300s) per REQ-STASH-NEW-008.
 * This default applies when ttl_seconds is not provided to stash_lock.
 * axiom:trace work_item=SWDE-55 spec=specs/106-Context-Stash.md#REQ-STASH-NEW-008
 */
export const LOCK_TTL_MS = 300_000; // 5 minutes

export const PG_CREATE_TABLE = `
CREATE TABLE IF NOT EXISTS stash_entries (
  stash_id   TEXT PRIMARY KEY,
  state      TEXT NOT NULL CHECK (state IN ('suspended', 'closed', 'active')),
  content    TEXT NOT NULL,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_stash_entries_state
  ON stash_entries (state);

CREATE INDEX IF NOT EXISTS idx_stash_entries_created_at
  ON stash_entries (created_at DESC)`;
