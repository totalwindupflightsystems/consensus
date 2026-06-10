/**
 * Tree Memory Plugin — Git-backed shared agent memory with query layer.
 *
 * Tools: tree.branch, tree.commit, tree.query, tree.merge, tree.promote,
 *        tree.log, tree.state, tree.peers, tree.init, tree.spawn, tree.status
 *
 * Storage: .tree-memory/repo/ (git repo)
 *          .tree-memory/config.yaml (plugin config)
 *
 * Spec: specs/113-Tree-Memory.md
 *
 * axiom:trace work_item=tree-memory-01 spec=specs/113-Tree-Memory.md plan=phase-1/task-1-1/step-1-1-1
 */

import {
  appendFileSync,
  existsSync,
  lstatSync,
  mkdirSync,
  readdirSync,
  readFileSync,
  realpathSync,
  writeFileSync,
  rmSync,
  statSync,
} from "node:fs";
import { join, resolve, relative } from "node:path";
import { execSync } from "node:child_process";
import { randomUUID } from "node:crypto";
import { createRequire } from "node:module";
import { tool } from "@opencode-ai/plugin";

// ─────────────────────────────────────────────────────────────────────────────
// Types
// ─────────────────────────────────────────────────────────────────────────────

export interface TreeMemoryConfig {
  security: {
    duckdb_sandbox: "strict" | "permissive" | "off";
    secret_scanning: {
      enabled: boolean;
      additional_patterns: string[];
      ignore_paths: string[];
    };
  };
  duckdb: {
    eager_start: boolean;
    memory_limit_mb: number;
    threads: number;
  };
  query_surfaces: Record<string, { enabled: boolean; scope?: string; description?: string }>;
  roles: Record<string, { surfaces: string[] }>;
  spawn_budget: {
    max_active_children: number;
    max_total_active: number;
    cooldown_seconds: number;
    cost_ceiling_usd: number;
    stale_timeout_minutes: number;
  };
  // axiom:trace work_item=tree-memory-01 spec=specs/113-Tree-Memory.md#3.2.1 plan=phase-H2/task-H2-1/step-H2-1-1
  merge?: {
    timeout_seconds?: number;  // default 60 — seconds before merge lock expires
    max_retries?: number;      // default 3 (already exists as hardcoded)
  };
  // BL-05: Global query result size cap — prevents unbounded reads from large repos
  // axiom:trace work_item=tree-memory-01 spec=specs/113-Tree-Memory.md#4 plan=phase-BL/task-BL-05/step-BL-05-1
  result_limit?: number;  // default 1000 — max results returned by any query surface
}

export interface Finding {
  id: string;
  agent: string;
  timestamp: string;
  topic: string;
  type: "finding" | "hypothesis" | "dead_end" | "observation";
  summary: string;
  confidence: "high" | "medium" | "low";
  evidence?: string;
  promoted: boolean;
  refs: string[];
  merge_conflict?: boolean;
}

export interface AgentState {
  agent: string;
  branch: string;
  status: "active" | "done" | "stale" | "merged";
  assignment: string;
  started_at: string;
  updated_at: string;
  tool_calls: number;
  findings_count: number;
  current_focus: string;
  next_step: string;
  log_frame_uuid?: string;
}

// ─────────────────────────────────────────────────────────────────────────────
// Secret Scanning (REQ-TM-SEC-03)
// axiom:trace work_item=tree-memory-01 spec=specs/113-Tree-Memory.md#3.4 plan=phase-1/task-1-3/step-1-3-1
// ─────────────────────────────────────────────────────────────────────────────

const DEFAULT_SECRET_PATTERNS: Array<{ name: string; pattern: RegExp }> = [
  { name: "aws_access_key", pattern: /AKIA[0-9A-Z]{16}/ },
  { name: "aws_secret_key", pattern: /aws_secret(?:_key)?[_\s]*[=:]\s*\S{20,}/ },
  { name: "github_pat", pattern: /ghp_[a-zA-Z0-9]{36}/ },
  { name: "github_oauth", pattern: /gho_[a-zA-Z0-9]{36}/ },
  { name: "openai_key", pattern: /sk-[a-zA-Z0-9]{48}/ },
  { name: "private_key", pattern: /-----BEGIN.*PRIVATE KEY-----/ },
  { name: "generic_token", pattern: /token["\s:=]+[a-zA-Z0-9_\-]{20,}/ },
];

export function scanForSecrets(
  content: string,
  additionalPatterns: string[] = [],
): { matched: boolean; patterns_matched: string[] } {
  const patterns = [...DEFAULT_SECRET_PATTERNS];
  for (const p of additionalPatterns) {
    try {
      patterns.push({ name: `custom:${p}`, pattern: new RegExp(p) });
    } catch { /* skip invalid regex */ }
  }
  const matched: string[] = [];
  for (const { name, pattern } of patterns) {
    if (pattern.test(content)) {
      matched.push(name);
    }
  }
  return { matched: matched.length > 0, patterns_matched: matched };
}

// ─────────────────────────────────────────────────────────────────────────────
// PII Exclusion via .treeignore (REQ-TM-COM-01)
// axiom:trace work_item=tree-memory-01 spec=specs/113-Tree-Memory.md#3.5 plan=phase-1/task-1-8/step-1-8-1
// ─────────────────────────────────────────────────────────────────────────────

/**
 * Load PII content patterns from .tree-memory/repo/.treeignore
 * Each non-comment, non-empty line is treated as a regex pattern.
 * REQ-TM-COM-01: axiom:trace work_item=tree-memory-01 spec=specs/113-Tree-Memory.md#3.5 plan=phase-1/task-1-8/step-1-8-1
 */
export function loadTreeIgnorePatterns(repoPath: string): Array<{name: string; pattern: RegExp}> {
  const treeignorePath = `${repoPath}/.treeignore`;
  if (!existsSync(treeignorePath)) return [];

  const lines = readFileSync(treeignorePath, 'utf-8').split('\n');
  const patterns: Array<{name: string; pattern: RegExp}> = [];

  for (const line of lines) {
    const trimmed = line.trim();
    if (!trimmed || trimmed.startsWith('#')) continue;

    // Format: pattern_name=regex OR just regex (name defaults to line index)
    const eqIdx = trimmed.indexOf('=');
    if (eqIdx > 0 && !trimmed.slice(0, eqIdx).includes(' ')) {
      const name = trimmed.slice(0, eqIdx);
      const regex = trimmed.slice(eqIdx + 1);
      try { patterns.push({ name, pattern: new RegExp(regex, 'gi') }); } catch { /* skip invalid */ }
    } else {
      try { patterns.push({ name: `pattern_${patterns.length}`, pattern: new RegExp(trimmed, 'gi') }); } catch { /* skip */ }
    }
  }
  return patterns;
}

/**
 * Redact PII content patterns in file content. Returns redacted content and audit entries.
 * REQ-TM-COM-01: axiom:trace work_item=tree-memory-01 spec=specs/113-Tree-Memory.md#3.5
 */
export function redactPiiContent(
  content: string,
  filePath: string,
  patterns: Array<{name: string; pattern: RegExp}>
): { redacted: string; auditEntries: Array<{pattern: string; file: string; lineCount: number}> } {
  if (patterns.length === 0) return { redacted: content, auditEntries: [] };

  const auditEntries: Array<{pattern: string; file: string; lineCount: number}> = [];
  let redacted = content;

  for (const { name, pattern } of patterns) {
    pattern.lastIndex = 0; // reset global flag
    const matches = redacted.match(pattern);
    if (matches && matches.length > 0) {
      auditEntries.push({ pattern: name, file: filePath, lineCount: matches.length });
      pattern.lastIndex = 0;
      redacted = redacted.replace(pattern, `[PII-REDACTED:${name}]`);
    }
  }

  return { redacted, auditEntries };
}

// ─────────────────────────────────────────────────────────────────────────────
// Git Engine (write operations)
// axiom:trace work_item=tree-memory-01 spec=specs/113-Tree-Memory.md#3 plan=phase-1/task-1-2/step-1-2-1
// ─────────────────────────────────────────────────────────────────────────────

export class GitEngine {
  private repoPath: string;
  constructor(repoPath: string) {
    this.repoPath = repoPath;
  }

  // Validate git binary exists and meets minimum version
  // axiom:trace work_item=tree-memory-01 spec=specs/113-Tree-Memory.md#11
  static validateGitEnvironment(): void {
    try {
      const result = execSync('git --version', { encoding: 'utf-8', timeout: 5000 });
      const match = result.match(/git version (\d+)\.(\d+)/);
      if (!match) throw new Error('Cannot parse git version');
      const major = parseInt(match[1]);
      const minor = parseInt(match[2]);
      if (major < 2 || (major === 2 && minor < 28)) {
        throw new Error(`git version ${major}.${minor} is below minimum required 2.28 (needed for 'git init -b')`);
      }
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : String(err);
      if (msg.includes('below minimum') || msg.includes('Cannot parse')) throw err;
      throw new Error(`git binary not found or not executable. tree-memory requires git >= 2.28. Error: ${msg}`);
    }
  }

  private git(cmd: string): string {
    try {
      return execSync(`git ${cmd}`, {
        cwd: this.repoPath,
        encoding: "utf-8",
        timeout: 10000,
        stdio: ["pipe", "pipe", "pipe"],
      }).trim();
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : String(err);
      throw new Error(`git error: ${msg}`);
    }
  }

  /** Initialize a bare-ish git repo for tree memory */
  init(): void {
    if (!existsSync(this.repoPath)) {
      mkdirSync(this.repoPath, { recursive: true });
    }
    if (!existsSync(join(this.repoPath, ".git"))) {
      this.git("init -b main");
      // Configure a default git user for tree memory commits
      this.git('config user.email "tree-memory@axiom.local"');
      this.git('config user.name "Tree Memory"');
      // Create initial commit on main
      const metaPath = join(this.repoPath, ".tree-memory.json");
      writeFileSync(metaPath, JSON.stringify({
        schema_version: "1.0.0",
        created_at: new Date().toISOString(),
      }, null, 2));
      this.git("add .tree-memory.json");
      this.git('commit -m "Initialize tree memory"');
    }
  }

  /** Check if the repo is initialized */
  isInitialized(): boolean {
    return existsSync(join(this.repoPath, ".git"));
  }

  /** Create a new branch from main (or another branch) */
  createBranch(name: string, fromBranch?: string): void {
    const base = fromBranch ?? "main";
    this.git(`branch ${name} ${base}`);
    this.git(`checkout ${name}`);
  }

  /** List all branches */
  listBranches(): string[] {
    const output = this.git("branch --list --format='%(refname:short)'");
    return output.split("\n").filter(Boolean).map(b => b.replace(/'/g, ""));
  }

  /** Get current branch */
  currentBranch(): string {
    return this.git("rev-parse --abbrev-ref HEAD");
  }

  /** Checkout a branch */
  checkout(branch: string): void {
    this.git(`checkout ${branch}`);
  }

  /** Write a file and commit */
  commitFile(filePath: string, content: string, message: string): void {
    const fullPath = join(this.repoPath, filePath);
    const dir = join(this.repoPath, filePath.split("/").slice(0, -1).join("/"));
    if (!existsSync(dir)) {
      mkdirSync(dir, { recursive: true });
    }
    writeFileSync(fullPath, content);
    this.git(`add "${filePath}"`);
    this.git(`commit -m "${message.replace(/"/g, '\\"')}"`);
  }

  /** Merge current branch to target (default: main) with first-wins queue (REQ-TM-ASS-01) */
  // axiom:trace work_item=tree-memory-01 spec=specs/113-Tree-Memory.md#3.2.1 plan=phase-H2/task-H2-2/step-H2-2-1
  merge(targetBranch: string = "main", timeoutMs: number = 60_000): { success: boolean; error?: string; conflicting_files?: string[] } {
    const currentBr = this.currentBranch();
    // axiom:trace work_item=tree-memory-01 spec=specs/113-Tree-Memory.md#3.2.1 plan=phase-1/task-1-2/step-1-2-2
    try {
      this.git(`checkout ${targetBranch}`);
      try {
        execSync(`git merge ${currentBr} --no-edit`, {
          cwd: this.repoPath,
          timeout: timeoutMs,
          encoding: "utf-8",
          stdio: ["pipe", "pipe", "pipe"],
        });
      } catch (err: unknown) {
        const msg = err instanceof Error ? err.message : String(err);
        if (msg.includes("ETIMEDOUT") || (err as NodeJS.ErrnoException)?.code === "ETIMEDOUT") {
          return { success: false, error: "merge_timeout" };
        }
        throw err;
      }
      return { success: true };
    } catch (err: unknown) {
      const topMsg = err instanceof Error ? err.message : String(err);
      if (topMsg.includes("merge_timeout") || topMsg === "merge_timeout") {
        return { success: false, error: "merge_timeout" };
      }
      // Attempt rebase-retry (REQ-TM-ASS-01)
      try {
        this.git("merge --abort");
        this.git(`checkout ${currentBr}`);
        try {
          execSync(`git rebase ${targetBranch}`, {
            cwd: this.repoPath,
            timeout: timeoutMs,
            encoding: "utf-8",
            stdio: ["pipe", "pipe", "pipe"],
          });
        } catch (err: unknown) {
          const msg = err instanceof Error ? err.message : String(err);
          if (msg.includes("ETIMEDOUT") || (err as NodeJS.ErrnoException)?.code === "ETIMEDOUT") {
            try { this.git("rebase --abort"); } catch { /* ignore */ }
            try { this.git(`checkout ${currentBr}`); } catch { /* ignore */ }
            return { success: false, error: "merge_timeout" };
          }
          throw err;
        }
        this.git(`checkout ${targetBranch}`);
        try {
          execSync(`git merge ${currentBr} --no-edit`, {
            cwd: this.repoPath,
            timeout: timeoutMs,
            encoding: "utf-8",
            stdio: ["pipe", "pipe", "pipe"],
          });
        } catch (err: unknown) {
          const msg = err instanceof Error ? err.message : String(err);
          if (msg.includes("ETIMEDOUT") || (err as NodeJS.ErrnoException)?.code === "ETIMEDOUT") {
            try { this.git("merge --abort"); } catch { /* ignore */ }
            try { this.git(`checkout ${currentBr}`); } catch { /* ignore */ }
            return { success: false, error: "merge_timeout" };
          }
          throw err;
        }
        return { success: true };
      } catch {
        // Retries exhausted — collect conflicting files
        let conflictingFiles: string[] = [];
        try {
          const statusOutput = this.git("status --porcelain");
          conflictingFiles = statusOutput
            .split("\n")
            .filter(line => line.startsWith("UU") || line.startsWith("AA") || line.startsWith("DD"))
            .map(line => line.slice(3).trim());
        } catch { /* best-effort conflict file detection */ }
        try { this.git("merge --abort"); } catch { /* ignore */ }
        try { this.git("rebase --abort"); } catch { /* ignore */ }
        try { this.git(`checkout ${currentBr}`); } catch { /* ignore */ }
        return {
          success: false,
          error: "merge_conflict",
          conflicting_files: conflictingFiles,
        };
      }
    }
  }

  /** Delete a branch */
  deleteBranch(name: string): void {
    const current = this.currentBranch();
    if (current === name) {
      this.git("checkout main");
    }
    this.git(`branch -D ${name}`);
  }

  /** Get git log */
  log(branch?: string, limit: number = 20): string {
    const branchArg = branch ? ` ${branch}` : "";
    return this.git(`log --oneline -${limit}${branchArg}`);
  }

  /** Get diff between two branches */
  diff(branchA: string, branchB: string): string {
    return this.git(`diff ${branchA}..${branchB}`);
  }

  /** Read a file from the repo */
  readFile(filePath: string): string | null {
    const fullPath = join(this.repoPath, filePath);
    if (!existsSync(fullPath)) return null;
    return readFileSync(fullPath, "utf-8");
  }

  /** List JSON files matching a glob pattern (simplified: supports *) */
  listFiles(pattern: string): string[] {
    const parts = pattern.split("/");
    return this._matchFiles(this.repoPath, parts, "");
  }

  private _matchFiles(basePath: string, parts: string[], prefix: string): string[] {
    if (parts.length === 0) return [];
    const [current, ...rest] = parts;

    if (!existsSync(basePath)) return [];

    let entries: string[];
    try {
      entries = readdirSync(basePath);
    } catch { return []; }

    const results: string[] = [];

    if (current === "*" || current === "**") {
      for (const entry of entries) {
        const entryPath = join(basePath, entry);
        const relPath = prefix ? `${prefix}/${entry}` : entry;
        try {
          const stat = statSync(entryPath);
          if (stat.isDirectory()) {
            // Match rest from this directory
            results.push(...this._matchFiles(entryPath, rest, relPath));
            // For **, also recurse with same pattern
            if (current === "**") {
              results.push(...this._matchFiles(entryPath, parts, relPath));
            }
          } else if (rest.length === 0) {
            // Leaf file matching wildcard
            results.push(relPath);
          }
        } catch { /* skip broken entries */ }
      }
    } else if (current.includes("*")) {
      // Pattern like "*.json"
      const regex = new RegExp("^" + current.replace(/\*/g, ".*").replace(/\?/g, ".") + "$");
      for (const entry of entries) {
        if (!regex.test(entry)) continue;
        const entryPath = join(basePath, entry);
        const relPath = prefix ? `${prefix}/${entry}` : entry;
        try {
          const stat = statSync(entryPath);
          if (stat.isDirectory() && rest.length > 0) {
            results.push(...this._matchFiles(entryPath, rest, relPath));
          } else if (stat.isFile() && rest.length === 0) {
            results.push(relPath);
          }
        } catch { /* skip */ }
      }
    } else {
      // Literal path segment
      const entryPath = join(basePath, current);
      const relPath = prefix ? `${prefix}/${current}` : current;
      if (existsSync(entryPath)) {
        try {
          const stat = statSync(entryPath);
          if (stat.isDirectory() && rest.length > 0) {
            results.push(...this._matchFiles(entryPath, rest, relPath));
          } else if (stat.isFile() && rest.length === 0) {
            results.push(relPath);
          }
        } catch { /* skip */ }
      }
    }

    return results;
  }
}

// ─────────────────────────────────────────────────────────────────────────────
// DuckDB Adapter (in-process query engine)
// Two-path architecture: DuckDB when installed, filesystem scanner as fallback.
// See ADR-TM-004 and specs/113-Tree-Memory.md §4.1
// axiom:trace work_item=tree-memory-duckdb-native spec=specs/113-Tree-Memory.md#4 plan=phase-2/task-2-1/step-2-1-1
// ─────────────────────────────────────────────────────────────────────────────

/** Minimal interface for the @duckdb/node-api DuckDBConnection returned by our loader */
interface DuckDBConn {
  run(sql: string): Promise<DuckDBResult>;
  closeSync(): void;
}
interface DuckDBResult {
  columnCount: number;
  columnName(i: number): string;
  getRows(): Promise<Array<Array<unknown>>>;
}
interface DuckDBInstanceApi {
  create(path: string, opts?: Record<string, string>): Promise<{
    connect(): Promise<DuckDBConn>;
    closeSync(): void;
  }>;
}

/**
 * Convert a raw DuckDB result value to a plain JS primitive.
 * Handles bigint, DuckDB timestamp objects, and any other custom types.
 */
function duckdbToJs(v: unknown): unknown {
  if (v === null || v === undefined) return null;
  if (typeof v === "bigint") return Number(v);
  if (typeof v === "string" || typeof v === "number" || typeof v === "boolean") return v;
  // DuckDB value objects (DuckDBTimestampValue, etc.) have a toString()
  if (v && typeof (v as Record<string, unknown>).toString === "function") {
    return String(v);
  }
  return v;
}

/**
 * Load the DuckDB adapter from the installed native directory.
 *
 * Detection path (set by the installer):
 *   .opencode/native/duckdb/node_modules/@duckdb/node-api/lib/duckdb.js
 *
 * Override path (for testing / CI with DuckDB):
 *   TREE_MEMORY_NATIVE_PATH=<path-to-duckdb.js>
 *
 * Returns a DuckdbAdapter on success, null on any failure (non-fatal).
 * Logs a single warn line if load fails.
 *
 * axiom:trace work_item=tree-memory-duckdb-native spec=specs/113-Tree-Memory.md#4 plan=phase-2/task-2-1/step-2-1-1
 */
export async function loadDuckdbIfAvailable(projectRoot: string): Promise<DuckdbAdapter | null> {
  // Honor explicit override (useful for CI and tests)
  const envPath = process.env["TREE_MEMORY_NATIVE_PATH"];
  const entryPath = envPath
    ? envPath
    : join(projectRoot, ".opencode", "native", "duckdb", "node_modules", "@duckdb", "node-api", "lib", "duckdb.js");

  if (!existsSync(entryPath)) {
    return null; // DuckDB not installed — silent (normal state for new installs)
  }

  try {
    const req = createRequire(entryPath);
    const duckdbModule = req(entryPath) as { DuckDBInstance: DuckDBInstanceApi };
    const { DuckDBInstance } = duckdbModule;
    if (typeof DuckDBInstance?.create !== "function") {
      console.warn("[tree-memory] WARN: DuckDB load failed (unexpected module shape), falling back to filesystem scanner");
      return null;
    }

    const dbInstance = await DuckDBInstance.create(":memory:");
    const conn = await dbInstance.connect();

    // Restrict extension loading (defense-in-depth)
    try {
      await conn.run("SET autoinstall_known_extensions=false");
      await conn.run("SET autoload_known_extensions=false");
    } catch { /* ignore — these settings may not exist in all versions */ }

    // Get DuckDB version
    let version = "unknown";
    try {
      const vRes = await conn.run("PRAGMA version");
      const vRows = await vRes.getRows();
      version = String(vRows[0]?.[0] ?? "unknown");
    } catch { /* best-effort */ }

    const adapter = new DuckdbAdapter(conn, dbInstance, version);
    return adapter;
  } catch (err: unknown) {
    const msg = err instanceof Error ? err.message : String(err);
    console.warn(`[tree-memory] WARN: DuckDB load failed (${msg}), falling back to filesystem scanner`);
    return null;
  }
}

/**
 * DuckdbAdapter — wraps an active DuckDB connection and implements query methods
 * matching QueryEngine's interface. Used when .opencode/native/duckdb/ is installed.
 *
 * All query methods return the same types as their QueryEngine counterparts.
 * axiom:trace work_item=tree-memory-duckdb-native spec=specs/113-Tree-Memory.md#4 plan=phase-2/task-2-1/step-2-1-1
 */
export class DuckdbAdapter {
  private readonly conn: DuckDBConn;
  private readonly instance: { closeSync(): void };
  private readonly _version: string;

  constructor(conn: DuckDBConn, instance: { closeSync(): void }, version: string) {
    this.conn = conn;
    this.instance = instance;
    this._version = version;
  }

  get version(): string { return this._version; }

  /**
   * Block dangerous SQL keywords in any SQL string before execution.
   * Per spec §4.3 blocked_functions list (also enforced for DuckDB parameterized queries as
   * defense-in-depth — all paths through DuckdbAdapter go through this check).
   * axiom:trace work_item=tree-memory-duckdb-native spec=specs/113-Tree-Memory.md#4.3 plan=phase-2/task-2-1/step-2-1-2
   */
  private validateSql(sql: string): void {
    // Case-insensitive search for blocked DuckDB functions/statements
    const blocked = ["INSTALL", "LOAD", "ATTACH", "httpfs", "COPY TO", "EXPORT DATABASE"];
    const sqlUpper = sql.toUpperCase();
    for (const kw of blocked) {
      if (sqlUpper.includes(kw.toUpperCase())) {
        throw new Error(`DuckDB sandbox violation: blocked keyword "${kw}" in SQL`);
      }
    }
  }

  /** Execute a SQL query and return rows as plain JS objects keyed by column name */
  private async query<T>(sql: string): Promise<T[]> {
    this.validateSql(sql);
    const res = await this.conn.run(sql);
    const cols: string[] = Array.from({ length: res.columnCount }, (_, i) => res.columnName(i));
    const rows = await res.getRows();
    return rows.map(row =>
      Object.fromEntries(cols.map((c, i) => [c, duckdbToJs(row[i])])),
    ) as T[];
  }

  /**
   * Validate that a path is within the sandbox before using in SQL.
   * Pre-check mirrors QueryEngine.validatePath() to prevent path escapes.
   * axiom:trace work_item=tree-memory-duckdb-native spec=specs/113-Tree-Memory.md#4.2 plan=phase-2/task-2-1/step-2-1-2
   */
  private validateGlobPath(pattern: string, repoPath: string, sandboxMode: "strict" | "permissive" | "off", projectRoot: string): void {
    // In strict mode: validate that repoPath itself is inside projectRoot
    // (prevents calling adapter with a foreign repoPath that escapes project boundaries)
    if (sandboxMode === "strict" && !repoPath.startsWith(projectRoot)) {
      throw new Error(`DuckDB sandbox violation: repoPath "${repoPath}" is outside project root`);
    }

    // Resolve the base directory of the glob (strip wildcard segments)
    const baseDir = pattern.replace(/\/?\*.*$/, "");
    const resolved = resolve(repoPath, baseDir);

    if (sandboxMode === "strict") {
      if (!resolved.startsWith(repoPath)) {
        throw new Error(`DuckDB sandbox violation: "${pattern}" escapes .tree-memory/repo/`);
      }
    } else if (sandboxMode === "permissive") {
      if (!resolved.startsWith(projectRoot)) {
        throw new Error(`DuckDB sandbox violation: "${pattern}" escapes project root`);
      }
    }
    // "off" mode — no restriction (operator opted in)
  }

  /** Escape a literal string for safe inclusion in SQL (used for WHERE clause values only, never paths) */
  private escLit(s: string): string {
    return s.replace(/'/g, "''");
  }

  async queryFindings(
    repoPath: string,
    sandboxMode: "strict" | "permissive" | "off",
    projectRoot: string,
    params: { topic?: string; agent?: string; confidence?: string; type?: string; since?: string; limit?: number },
  ): Promise<Finding[]> {
    // Two patterns: main-branch findings + agent-branch findings
    const mainPattern = join(repoPath, "agents", "*", "findings", "*.json").replace(/\\/g, "/");
    const branchPattern = join(repoPath, "findings", "*.json").replace(/\\/g, "/");

    this.validateGlobPath(mainPattern, repoPath, sandboxMode, projectRoot);
    this.validateGlobPath(branchPattern, repoPath, sandboxMode, projectRoot);

    let whereClause = "1=1";
    if (params.topic) whereClause += ` AND lower(topic::VARCHAR) LIKE '%${this.escLit(params.topic.toLowerCase())}%'`;
    if (params.agent) whereClause += ` AND agent::VARCHAR = '${this.escLit(params.agent)}'`;
    if (params.confidence) whereClause += ` AND confidence::VARCHAR = '${this.escLit(params.confidence)}'`;
    if (params.type) whereClause += ` AND type::VARCHAR = '${this.escLit(params.type)}'`;
    if (params.since) whereClause += ` AND strftime(timestamp, '%Y-%m-%dT%H:%M:%SZ') >= '${this.escLit(params.since)}'`;

    const limit = params.limit ?? 50;

    const selectCols = `
      id::VARCHAR AS id, agent::VARCHAR AS agent,
      topic::VARCHAR AS topic, type::VARCHAR AS type,
      summary::VARCHAR AS summary, confidence::VARCHAR AS confidence,
      strftime(timestamp, '%Y-%m-%dT%H:%M:%SZ') AS timestamp,
      CAST(promoted AS VARCHAR) AS promoted
    `;

    // Query each pattern separately to handle "No files found" gracefully
    const isNoFilesError = (e: unknown) => String(e).includes("No files found");
    const mapFindings = (rs: Finding[]) => rs.map(r => ({
      ...r,
      promoted: r.promoted === "true" || r.promoted === true,
      refs: Array.isArray((r as Record<string, unknown>)["refs"]) ? (r as Record<string, unknown>)["refs"] as string[] : [],
    }));

    let mainResults: Finding[] = [];
    try {
      mainResults = mapFindings(await this.query<Finding>(
        `SELECT ${selectCols} FROM read_json_auto('${mainPattern}') WHERE ${whereClause} ORDER BY timestamp DESC LIMIT ${limit}`,
      ));
    } catch (e) {
      if (!isNoFilesError(e)) throw e;
    }

    let branchResults: Finding[] = [];
    try {
      branchResults = mapFindings(await this.query<Finding>(
        `SELECT ${selectCols} FROM read_json_auto('${branchPattern}') WHERE ${whereClause} ORDER BY timestamp DESC LIMIT ${limit}`,
      ));
    } catch (e) {
      if (!isNoFilesError(e)) throw e;
    }

    // Merge, deduplicate by id, sort, and limit
    const seen = new Set<string>();
    const combined: Finding[] = [];
    for (const f of [...mainResults, ...branchResults]) {
      if (!seen.has(f.id)) {
        seen.add(f.id);
        combined.push(f);
      }
    }
    combined.sort((a, b) => (b.timestamp ?? "").localeCompare(a.timestamp ?? ""));
    return combined.slice(0, limit);
  }

  async queryPeers(
    repoPath: string,
    sandboxMode: "strict" | "permissive" | "off",
    projectRoot: string,
    params: { status?: string; agent?: string },
  ): Promise<AgentState[]> {
    const mainPattern = join(repoPath, "agents", "*", "state.json").replace(/\\/g, "/");
    const branchFile = join(repoPath, "state.json").replace(/\\/g, "/");

    this.validateGlobPath(mainPattern, repoPath, sandboxMode, projectRoot);

    let whereClause = "1=1";
    if (params.status) whereClause += ` AND status::VARCHAR = '${this.escLit(params.status)}'`;
    if (params.agent) whereClause += ` AND agent::VARCHAR = '${this.escLit(params.agent)}'`;

    const selectCols = `
      agent::VARCHAR AS agent, branch::VARCHAR AS branch,
      status::VARCHAR AS status, assignment::VARCHAR AS assignment,
      strftime(started_at, '%Y-%m-%dT%H:%M:%SZ') AS started_at,
      strftime(updated_at, '%Y-%m-%dT%H:%M:%SZ') AS updated_at,
      tool_calls::INTEGER AS tool_calls, findings_count::INTEGER AS findings_count,
      current_focus::VARCHAR AS current_focus, next_step::VARCHAR AS next_step
    `;

    const isNoFilesError = (e: unknown) => String(e).includes("No files found");
    const results: AgentState[] = [];

    // Main pattern (agents/*/state.json)
    try {
      const main = await this.query<AgentState>(
        `SELECT ${selectCols} FROM read_json_auto('${mainPattern}') WHERE ${whereClause}`,
      );
      results.push(...main);
    } catch (e) {
      if (!isNoFilesError(e)) throw e;
    }

    // Root state.json (agent branch state — only if file exists)
    if (existsSync(join(repoPath, "state.json"))) {
      try {
        const branch = await this.query<AgentState>(
          `SELECT ${selectCols} FROM read_json_auto('${branchFile}') WHERE ${whereClause}`,
        );
        results.push(...branch);
      } catch (e) {
        if (!isNoFilesError(e)) throw e;
      }
    }

    return results;
  }

  async queryWatches(
    repoPath: string,
    sandboxMode: "strict" | "permissive" | "off",
    projectRoot: string,
    params: { watch_name?: string; since?: string; question?: "last_match" | "count" | "frequency" | "trend" | "last_n"; limit?: number },
  ): Promise<unknown> {
    const pattern = join(repoPath, "background", "*", "matches", "*.json").replace(/\\/g, "/");
    this.validateGlobPath(pattern, repoPath, sandboxMode, projectRoot);

    let whereClause = "1=1";
    if (params.watch_name) whereClause += ` AND watch::VARCHAR = '${this.escLit(params.watch_name)}'`;
    if (params.since) whereClause += ` AND hour::VARCHAR >= '${this.escLit(params.since)}'`;

    try {
      switch (params.question) {
        case "count": {
          const rows = await this.query<{ total: number }>(`SELECT SUM(count) AS total FROM read_json_auto('${pattern}') WHERE ${whereClause}`);
          return { total_matches: Number(rows[0]?.total ?? 0) };
        }
        default: {
          const rows = await this.query<Record<string, unknown>>(`SELECT * FROM read_json_auto('${pattern}') WHERE ${whereClause} LIMIT ${params.limit ?? 20}`);
          return rows;
        }
      }
    } catch {
      return [];
    }
  }

  async queryEvents(
    repoPath: string,
    sandboxMode: "strict" | "permissive" | "off",
    projectRoot: string,
    params: { source?: string; type?: string; severity?: string; since?: string; limit?: number },
  ): Promise<unknown[]> {
    const pattern = join(repoPath, "background", "*", "events", "*.json").replace(/\\/g, "/");
    this.validateGlobPath(pattern, repoPath, sandboxMode, projectRoot);

    let whereClause = "1=1";
    if (params.source) whereClause += ` AND source::VARCHAR LIKE '%${this.escLit(params.source)}%'`;
    if (params.type) whereClause += ` AND type::VARCHAR = '${this.escLit(params.type)}'`;
    if (params.severity) whereClause += ` AND severity::VARCHAR = '${this.escLit(params.severity)}'`;
    if (params.since) whereClause += ` AND timestamp::VARCHAR >= '${this.escLit(params.since)}'`;

    try {
      return await this.query<Record<string, unknown>>(
        `SELECT * FROM read_json_auto('${pattern}') WHERE ${whereClause} LIMIT ${params.limit ?? 50}`,
      );
    } catch {
      return [];
    }
  }

  destroy(): void {
    try { this.conn.closeSync(); } catch { /* ignore */ }
    try { this.instance.closeSync(); } catch { /* ignore */ }
  }
}

// ─────────────────────────────────────────────────────────────────────────────
// Query Engine (read operations)
// Two-path: DuckDB when available (via DuckdbAdapter), filesystem scanner fallback.
// axiom:trace work_item=tree-memory-duckdb-native spec=specs/113-Tree-Memory.md#4 plan=phase-2/task-2-1/step-2-1-1
// ─────────────────────────────────────────────────────────────────────────────

export class QueryEngine {
  private initialized = false;
  private sandboxMode: "strict" | "permissive" | "off" = "strict";
  private repoPath: string;
  private projectRoot: string;
  // axiom:trace work_item=tree-memory-duckdb-native spec=specs/113-Tree-Memory.md#4 plan=phase-2/task-2-1/step-2-1-1
  private duckdbAdapter: DuckdbAdapter | null = null;

  constructor(
    repoPath: string,
    projectRoot: string,
  ) {
    this.repoPath = repoPath;
    this.projectRoot = projectRoot;
  }

  init(sandbox: "strict" | "permissive" | "off" = "strict", duckdb?: DuckdbAdapter | null): void {
    this.sandboxMode = sandbox;
    this.initialized = true;
    this.duckdbAdapter = duckdb ?? null;
  }

  isReady(): boolean {
    return this.initialized;
  }

  /** Returns which query engine is active */
  get engine(): "duckdb" | "filesystem-scanner" {
    return this.duckdbAdapter ? "duckdb" : "filesystem-scanner";
  }

  /** Returns the DuckDB version string when DuckDB is active */
  get engineVersion(): string | null {
    return this.duckdbAdapter?.version ?? null;
  }

  private ensureInit(): void {
    if (!this.initialized) {
      this.init();
    }
  }

  /** Validate a path is within sandbox bounds (REQ-TM-SEC-02) */
  private validatePath(filePath: string): string {
    const resolved = resolve(this.repoPath, filePath);
    if (this.sandboxMode === "strict") {
      if (!resolved.startsWith(this.repoPath)) {
        throw new Error(`Path sandbox violation: "${filePath}" escapes .tree-memory/repo/`);
      }
    } else if (this.sandboxMode === "permissive") {
      if (!resolved.startsWith(this.projectRoot)) {
        throw new Error(`Path sandbox violation: "${filePath}" escapes project root`);
      }
    }
    // "off" mode allows anything — operator opted in

    // Check for symlinks that escape the sandbox (H3 - symlink sandbox bypass fix)
    // axiom:trace work_item=tree-memory-01 spec=specs/113-Tree-Memory.md#4.2
    try {
      const stat = lstatSync(resolved);
      if (stat.isSymbolicLink()) {
        const linkTarget = realpathSync(resolved);
        if (!linkTarget.startsWith(this.repoPath)) {
          throw new Error(`sandbox violation: symlink at ${resolved} resolves outside repository`);
        }
      }
    } catch (err: unknown) {
      if (err instanceof Error && err.message.includes('sandbox violation')) throw err;
      // File doesn't exist yet — that's ok for writes
    }

    return resolved;
  }

  /** Read all JSON files matching a scope pattern and parse them */
  private readJsonFiles(scope: string): unknown[] {
    this.ensureInit();
    const git = new GitEngine(this.repoPath);
    const files = git.listFiles(scope);
    const results: unknown[] = [];
    for (const f of files) {
      if (!f.endsWith(".json")) continue;
      this.validatePath(f);
      const content = git.readFile(f);
      if (!content) continue;
      try {
        results.push(JSON.parse(content));
      } catch { /* skip invalid JSON */ }
    }
    return results;
  }

  /** Query findings across all branches */
  queryFindings(params: {
    topic?: string;
    agent?: string;
    confidence?: string;
    type?: string;
    since?: string;
    limit?: number;
  }): Finding[] {
    this.ensureInit();
    const scope = "agents/*/findings/*.json";
    let results = this.readJsonFiles(scope) as Finding[];

    // Also check top-level findings/ (on agent branches)
    const branchFindings = this.readJsonFiles("findings/*.json") as Finding[];
    results = [...results, ...branchFindings];

    if (params.topic) {
      results = results.filter(f => f.topic?.toLowerCase().includes(params.topic!.toLowerCase()));
    }
    if (params.agent) {
      results = results.filter(f => f.agent === params.agent);
    }
    if (params.confidence) {
      results = results.filter(f => f.confidence === params.confidence);
    }
    if (params.type) {
      results = results.filter(f => f.type === params.type);
    }
    if (params.since) {
      results = results.filter(f => f.timestamp >= params.since!);
    }

    // Sort by timestamp descending
    results.sort((a, b) => (b.timestamp ?? "").localeCompare(a.timestamp ?? ""));

    const limit = params.limit ?? 50;
    return results.slice(0, limit);
  }

  /** Query agent states (peers) */
  queryPeers(params: { status?: string; agent?: string }): AgentState[] {
    this.ensureInit();
    // Check agents/*/state.json on main
    let results = this.readJsonFiles("agents/*/state.json") as AgentState[];
    // Also check state.json at root (on agent branches)
    const rootState = this.readJsonFiles("state.json") as AgentState[];
    results = [...results, ...rootState];

    if (params.status) {
      results = results.filter(s => s.status === params.status);
    }
    if (params.agent) {
      results = results.filter(s => s.agent === params.agent);
    }

    return results;
  }

  /** Query watch matches */
  queryWatches(params: {
    watch_name?: string;
    since?: string;
    question?: "last_match" | "count" | "frequency" | "trend" | "last_n";
    limit?: number;
  }): unknown {
    this.ensureInit();
    const scope = "background/*/matches/*.json";
    let results = this.readJsonFiles(scope) as Array<{
      watch: string;
      file: string;
      pattern: string;
      hour: string;
      matches: Array<{ line: number; content: string; at: string }>;
      count: number;
    }>;

    if (params.watch_name) {
      results = results.filter(r => r.watch === params.watch_name);
    }
    if (params.since) {
      results = results.filter(r => r.hour >= params.since!);
    }

    // Handle question types
    switch (params.question) {
      case "count":
        return { total_matches: results.reduce((sum, r) => sum + (r.count ?? 0), 0) };
      case "last_match": {
        const allMatches = results.flatMap(r => r.matches ?? []);
        allMatches.sort((a, b) => (b.at ?? "").localeCompare(a.at ?? ""));
        return allMatches[0] ?? null;
      }
      case "last_n": {
        const all = results.flatMap(r => r.matches ?? []);
        all.sort((a, b) => (b.at ?? "").localeCompare(a.at ?? ""));
        return all.slice(0, params.limit ?? 10);
      }
      case "frequency": {
        const total = results.reduce((sum, r) => sum + (r.count ?? 0), 0);
        const hours = results.length || 1;
        return { total, hours, rate_per_hour: total / hours };
      }
      case "trend": {
        const sorted = [...results].sort((a, b) => a.hour.localeCompare(b.hour));
        return sorted.map(r => ({ hour: r.hour, count: r.count }));
      }
      default:
        return results.slice(0, params.limit ?? 20);
    }
  }

  /** Query events */
  queryEvents(params: {
    source?: string;
    type?: string;
    severity?: string;
    since?: string;
    limit?: number;
  }): unknown[] {
    this.ensureInit();
    const scope = "background/*/events/*.json";
    let results = this.readJsonFiles(scope) as Array<Record<string, unknown>>;

    if (params.source) {
      results = results.filter(r => String(r.source ?? "").includes(params.source!));
    }
    if (params.type) {
      results = results.filter(r => r.type === params.type);
    }
    if (params.severity) {
      results = results.filter(r => r.severity === params.severity);
    }
    if (params.since) {
      results = results.filter(r => String(r.timestamp ?? r.hour ?? "") >= params.since!);
    }

    return results.slice(0, params.limit ?? 50);
  }

  /** Query trends (time-series aggregation over surfaces) */
  queryTrends(params: {
    target_surface: string;
    metric?: string;
    window?: string;
    group_by?: string;
    since?: string;
    limit?: number;
  }): unknown {
    this.ensureInit();
    // Determine the scope from the target surface
    let data: Array<Record<string, unknown>> = [];

    switch (params.target_surface) {
      case "findings": {
        const findings = this.queryFindings({ since: params.since });
        data = findings.map(f => ({
          timestamp: f.timestamp,
          topic: f.topic,
          agent: f.agent,
          confidence: f.confidence,
          type: f.type,
        }));
        break;
      }
      case "watches": {
        const watches = this.readJsonFiles("background/*/matches/*.json") as Array<{
          watch: string; hour: string; count: number; matches: Array<{ at: string }>;
        }>;
        for (const w of watches) {
          for (const m of (w.matches ?? [])) {
            data.push({ timestamp: m.at, watch: w.watch, count: 1 });
          }
        }
        break;
      }
      case "events": {
        const events = this.queryEvents({ since: params.since });
        data = events as Array<Record<string, unknown>>;
        break;
      }
      default:
        return { error: `Unknown target surface for trends: ${params.target_surface}` };
    }

    // Time-bucket the data
    const windowMs = parseWindowToMs(params.window ?? "1h");
    const buckets: Record<string, { bucket: string; count: number; groups: Record<string, number> }> = {};

    for (const item of data) {
      const ts = String(item.timestamp ?? item.hour ?? "");
      if (!ts) continue;
      const time = new Date(ts).getTime();
      if (isNaN(time)) continue;
      const bucketStart = Math.floor(time / windowMs) * windowMs;
      const bucketKey = new Date(bucketStart).toISOString();

      if (!buckets[bucketKey]) {
        buckets[bucketKey] = { bucket: bucketKey, count: 0, groups: {} };
      }
      buckets[bucketKey].count++;

      if (params.group_by && item[params.group_by]) {
        const groupVal = String(item[params.group_by]);
        buckets[bucketKey].groups[groupVal] = (buckets[bucketKey].groups[groupVal] ?? 0) + 1;
      }
    }

    const sorted = Object.values(buckets).sort((a, b) => a.bucket.localeCompare(b.bucket));
    const limit = params.limit ?? 100;
    return sorted.slice(0, limit);
  }

  destroy(): void {
    this.initialized = false;
    if (this.duckdbAdapter) {
      this.duckdbAdapter.destroy();
      this.duckdbAdapter = null;
    }
  }
}

// ─────────────────────────────────────────────────────────────────────────────
// Time Window Parser (for trends)
// ─────────────────────────────────────────────────────────────────────────────

function parseWindowToMs(window: string): number {
  const match = window.match(/^(\d+)(s|m|h|d)$/);
  if (!match) return 3600000; // default 1h
  const [, num, unit] = match;
  const n = parseInt(num, 10);
  switch (unit) {
    case "s": return n * 1000;
    case "m": return n * 60 * 1000;
    case "h": return n * 3600 * 1000;
    case "d": return n * 86400 * 1000;
    default: return 3600000;
  }
}

// ─────────────────────────────────────────────────────────────────────────────
// Config Loader
// axiom:trace work_item=tree-memory-01 spec=specs/113-Tree-Memory.md#11 plan=phase-1/task-1-5/step-1-5-1
// ─────────────────────────────────────────────────────────────────────────────

const DEFAULT_CONFIG: TreeMemoryConfig = {
  security: {
    duckdb_sandbox: "strict",
    secret_scanning: {
      enabled: true,
      additional_patterns: [],
      ignore_paths: [],
    },
  },
  duckdb: {
    eager_start: false,
    memory_limit_mb: 256,
    threads: 2,
  },
  query_surfaces: {
    findings: { enabled: true, scope: "agents/*/findings/*.json" },
    peers: { enabled: true, scope: "*/state.json" },
    watches: { enabled: true, scope: "background/*/matches/*.json" },
    events: { enabled: true, scope: "background/*/events/*.json" },
    trends: { enabled: true },
    raw_sql: { enabled: false },
  },
  roles: {
    monitor: { surfaces: ["findings", "peers", "watches", "events", "trends"] },
    fix_agent: { surfaces: ["findings", "peers"] },
    synthesizer: { surfaces: ["findings", "peers", "watches", "events", "trends"] },
    background_watcher: { surfaces: ["watches", "events"] },
    admin: { surfaces: ["findings", "peers", "watches", "events", "trends", "raw_sql"] },
  },
  spawn_budget: {
    max_active_children: 5,
    max_total_active: 20,
    cooldown_seconds: 30,
    cost_ceiling_usd: 50.0,
    stale_timeout_minutes: 15,
  },
  // axiom:trace work_item=tree-memory-01 spec=specs/113-Tree-Memory.md#3.2.1 plan=phase-H2/task-H2-1/step-H2-1-2
  merge: {
    timeout_seconds: 60,
    max_retries: 3,
  },
};

export function loadTreeMemoryConfig(treeMemoryRoot: string): TreeMemoryConfig {
  const configPath = join(treeMemoryRoot, "config.yaml");
  if (!existsSync(configPath)) {
    return { ...DEFAULT_CONFIG };
  }
  try {
    const raw = readFileSync(configPath, "utf-8");
    // Simple YAML parsing for key-value pairs (no dependency on yaml package)
    // For a full implementation, use the yaml package. For v1, we support the
    // most common config keys via a lightweight parser.
    const parsed = parseSimpleYaml(raw);
    return deepMerge(DEFAULT_CONFIG, parsed) as TreeMemoryConfig;
  } catch {
    return { ...DEFAULT_CONFIG };
  }
}

/** Simple YAML-like config parser (handles nested objects and simple values) */
function parseSimpleYaml(raw: string): Record<string, unknown> {
  const result: Record<string, unknown> = {};
  const lines = raw.split("\n");
  const stack: Array<{ indent: number; obj: Record<string, unknown> }> = [
    { indent: -1, obj: result },
  ];

  for (const line of lines) {
    if (line.trim() === "" || line.trim().startsWith("#")) continue;
    const indent = line.length - line.trimStart().length;
    const trimmed = line.trim();

    // Pop stack to find parent
    while (stack.length > 1 && stack[stack.length - 1].indent >= indent) {
      stack.pop();
    }
    const parent = stack[stack.length - 1].obj;

    const colonIdx = trimmed.indexOf(":");
    if (colonIdx === -1) continue;
    const key = trimmed.slice(0, colonIdx).trim();
    const rawValue = trimmed.slice(colonIdx + 1).trim();

    if (rawValue === "" || rawValue === "|") {
      // Nested object
      const child: Record<string, unknown> = {};
      parent[key] = child;
      stack.push({ indent, obj: child });
    } else {
      // Simple value
      parent[key] = parseYamlValue(rawValue);
    }
  }

  return result;
}

function parseYamlValue(raw: string): unknown {
  // Remove inline comments
  const val = raw.replace(/#.*$/, "").trim();
  if (val === "true") return true;
  if (val === "false") return false;
  if (val === "null") return null;
  if (/^-?\d+$/.test(val)) return parseInt(val, 10);
  if (/^-?\d+\.\d+$/.test(val)) return parseFloat(val);
  if (val.startsWith("[") && val.endsWith("]")) {
    // Simple array
    return val.slice(1, -1).split(",").map(s => parseYamlValue(s.trim())).filter(v => v !== "");
  }
  // Strip quotes
  if ((val.startsWith('"') && val.endsWith('"')) || (val.startsWith("'") && val.endsWith("'"))) {
    return val.slice(1, -1);
  }
  return val;
}

function deepMerge(target: Record<string, unknown>, source: Record<string, unknown>): Record<string, unknown> {
  const result = { ...target };
  for (const [key, val] of Object.entries(source)) {
    if (val && typeof val === "object" && !Array.isArray(val) && key in result && typeof result[key] === "object" && !Array.isArray(result[key])) {
      result[key] = deepMerge(result[key] as Record<string, unknown>, val as Record<string, unknown>);
    } else {
      result[key] = val;
    }
  }
  return result;
}

// ─────────────────────────────────────────────────────────────────────────────
// UUID-Framed Log Markers (REQ-TM-SEC-01)
// axiom:trace work_item=tree-memory-01 spec=specs/113-Tree-Memory.md#14.1 plan=phase-1/task-1-6/step-1-6-1
// ─────────────────────────────────────────────────────────────────────────────

export function frameLogContent(content: string, sessionUuid: string): string {
  // Escape any existing LOG_MATCH markers in the content
  const escaped = content
    .replace(/LOG_MATCH:/g, "\\x00LOG_MATCH:")
    .replace(/END_LOG_MATCH:/g, "\\x00END_LOG_MATCH:");
  return `───── LOG_MATCH:${sessionUuid} ─────\n${escaped}\n───── END_LOG_MATCH:${sessionUuid} ─────`;
}

// ─────────────────────────────────────────────────────────────────────────────
// Plugin Export
// axiom:trace work_item=tree-memory-01 spec=specs/113-Tree-Memory.md#5 plan=phase-1/task-1-1/step-1-1-2
// ─────────────────────────────────────────────────────────────────────────────

export const TreeMemoryPlugin = async ({
  directory,
  client: _client,
}: {
  directory: string;
  client: unknown;
}) => {
  if (!directory || !directory.trim()) {
    throw new Error("[TreeMemory] directory must be non-empty");
  }

  const projectRoot = directory;
  const treeMemoryRoot = join(projectRoot, ".tree-memory");
  const repoPath = join(treeMemoryRoot, "repo");

  // Load config
  const config = loadTreeMemoryConfig(treeMemoryRoot);

  // Initialize git engine (lazy — only if repo exists)
  let git: GitEngine | null = null;
  function ensureGit(): GitEngine {
    if (!git) {
      git = new GitEngine(repoPath);
      if (!git.isInitialized()) {
        throw new Error("Tree memory not initialized. Call tree.init first.");
      }
    }
    return git;
  }

  // Query engine (lazy init per REQ-TM-ASS-02)
  // Pre-load DuckDB adapter once at plugin init (non-fatal if unavailable)
  // axiom:trace work_item=tree-memory-duckdb-native spec=specs/113-Tree-Memory.md#4 plan=phase-2/task-2-1/step-2-1-1
  let duckdbAdapter: DuckdbAdapter | null = null;
  try {
    duckdbAdapter = await loadDuckdbIfAvailable(projectRoot);
  } catch { /* loadDuckdbIfAvailable is already non-fatal; belt-and-suspenders */ }

  let queryEngine: QueryEngine | null = null;
  function ensureQuery(): QueryEngine {
    if (!queryEngine) {
      queryEngine = new QueryEngine(repoPath, projectRoot);
      queryEngine.init(config.security.duckdb_sandbox, duckdbAdapter);
    }
    return queryEngine;
  }

  // Session UUID for log framing (REQ-TM-SEC-01)
  const sessionUuid = randomUUID();

  // Eager DuckDB start if configured
  if (config.duckdb.eager_start && existsSync(repoPath)) {
    ensureQuery();
  }

  // ─── Tool: tree.init ────────────────────────────────────────────────────────
  const treeInit = tool({
    description:
      "Initialize a new tree memory instance. Creates the .tree-memory/repo/ git repository " +
      "and .tree-memory/config.yaml with default settings.",
    args: {
      name: tool.schema.string().optional().describe("Optional name for the tree memory instance"),
    },
    async execute({ name }) {
      try {
        if (existsSync(join(repoPath, ".git"))) {
          return JSON.stringify({ status: "already_initialized", path: repoPath });
        }

        // Create directory structure
        if (!existsSync(treeMemoryRoot)) {
          mkdirSync(treeMemoryRoot, { recursive: true });
        }

        // Write default config
        const configPath = join(treeMemoryRoot, "config.yaml");
        if (!existsSync(configPath)) {
          writeFileSync(configPath, [
            "# Tree Memory Configuration",
            `# Instance: ${name ?? "default"}`,
            `# Created: ${new Date().toISOString()}`,
            "",
            "security:",
            "  duckdb_sandbox: strict",
            "  secret_scanning:",
            "    enabled: true",
            "    additional_patterns: []",
            "    ignore_paths: []",
            "",
            "duckdb:",
            "  eager_start: false",
            "  memory_limit_mb: 256",
            "  threads: 2",
            "",
            "spawn_budget:",
            "  max_active_children: 5",
            "  max_total_active: 20",
            "  cooldown_seconds: 30",
            "  cost_ceiling_usd: 50.00",
            "  stale_timeout_minutes: 15",
          ].join("\n"));
        }

        // Initialize git repo
        GitEngine.validateGitEnvironment();
        git = new GitEngine(repoPath);
        git.init();

        return JSON.stringify({
          status: "initialized",
          path: repoPath,
          name: name ?? "default",
          session_uuid: sessionUuid,
        });
      } catch (err) {
        return JSON.stringify({ error: err instanceof Error ? err.message : String(err) });
      }
    },
  });

  // ─── Tool: tree.branch ──────────────────────────────────────────────────────
  const treeBranch = tool({
    description:
      "Create, list, or delete branches in tree memory. " +
      "Each agent gets its own branch for write isolation.",
    args: {
      action: tool.schema.enum(["create", "list", "delete"]).describe("Branch operation"),
      name: tool.schema.string().optional().describe("Branch name (required for create/delete)"),
      from_branch: tool.schema.string().optional().describe("Branch to create from (default: main)"),
    },
    async execute({ action, name, from_branch }) {
      try {
        const g = ensureGit();
        switch (action) {
          case "create": {
            if (!name) throw new Error("Branch name required for create");
            // BL-04: Enforce spawn_budget on tree.branch create (not just tree.spawn)
            // axiom:trace work_item=tree-memory-01 spec=specs/113-Tree-Memory.md#7 plan=phase-BL/task-BL-04/step-BL-04-1
            const currentBranches = g.listBranches();
            const maxBranches = config.spawn_budget?.max_total_active ?? 50;
            if (maxBranches > 0 && currentBranches.length >= maxBranches) {
              return JSON.stringify({
                error: "spawn_budget_exhausted",
                message: `Cannot create branch: spawn budget exhausted (${currentBranches.length}/${maxBranches} active branches)`,
                active: currentBranches.length,
                max: maxBranches,
              });
            }
            g.createBranch(name, from_branch);
            return JSON.stringify({ action: "created", branch: name, from: from_branch ?? "main" });
          }
          case "list": {
            const branches = g.listBranches();
            return JSON.stringify({ branches });
          }
          case "delete": {
            if (!name) throw new Error("Branch name required for delete");
            g.deleteBranch(name);
            return JSON.stringify({ action: "deleted", branch: name });
          }
          default:
            throw new Error(`Unknown action: ${action}`);
        }
      } catch (err) {
        return JSON.stringify({ error: err instanceof Error ? err.message : String(err) });
      }
    },
  });

  // ─── Tool: tree.commit ──────────────────────────────────────────────────────
  const treeCommit = tool({
    description:
      "Write a structured JSON file and commit to the agent's branch. " +
      "Runs pre-commit secret scanning (REQ-TM-SEC-03). " +
      "Returns error if secrets detected.",
    args: {
      file: tool.schema.string().describe("File path relative to repo root (e.g., findings/001-network.json)"),
      content: tool.schema.string().describe("JSON content to write"),
      message: tool.schema.string().describe("Commit message"),
    },
    async execute({ file, content, message }) {
      try {
        const g = ensureGit();

        // REQ-TM-SEC-03: Pre-commit secret scanning
        if (config.security.secret_scanning.enabled) {
          const isIgnored = config.security.secret_scanning.ignore_paths.some(
            p => file.startsWith(p.replace("*", ""))
          );
          if (!isIgnored) {
            const scan = scanForSecrets(content, config.security.secret_scanning.additional_patterns);
            if (scan.matched) {
              // Log the block (pattern name only, NOT content)
              const auditDir = join(repoPath, ".audit");
              if (!existsSync(auditDir)) mkdirSync(auditDir, { recursive: true });
              const auditFile = join(auditDir, "secret-blocks.jsonl");
              const entry = JSON.stringify({
                timestamp: new Date().toISOString(),
                file,
                patterns_matched: scan.patterns_matched,
              });
              try {
                const existing = existsSync(auditFile) ? readFileSync(auditFile, "utf-8") : "";
                writeFileSync(auditFile, existing + entry + "\n");
              } catch { /* audit logging is best-effort */ }

              return JSON.stringify({
                error: "secret_detected",
                patterns_matched: scan.patterns_matched,
                action: "redact_and_retry",
              });
            }
          }
        }

        // Path validation
        if (file.includes("..") || file.startsWith("/")) {
          throw new Error("Path traversal rejected");
        }

        // REQ-TM-COM-01: Apply .treeignore PII redaction
        // axiom:trace work_item=tree-memory-01 spec=specs/113-Tree-Memory.md#3.5 plan=phase-1/task-1-8/step-1-8-2
        const treeignorePatterns = loadTreeIgnorePatterns(repoPath);
        if (treeignorePatterns.length > 0) {
          const { redacted, auditEntries } = redactPiiContent(
            typeof content === 'string' ? content : JSON.stringify(content),
            file,
            treeignorePatterns
          );
          if (auditEntries.length > 0) {
            content = redacted;
            // Write audit entries (no matched content — just pattern name + file + count)
            const auditPath = join(repoPath, ".audit", "pii-redactions.jsonl");
            mkdirSync(join(repoPath, ".audit"), { recursive: true });
            const auditLine = JSON.stringify({
              timestamp: new Date().toISOString(),
              patterns_matched: auditEntries.map(e => e.pattern),
              file,
              redaction_count: auditEntries.reduce((sum, e) => sum + e.lineCount, 0)
            }) + '\n';
            appendFileSync(auditPath, auditLine);
          }
        }

        g.commitFile(file, content, message);
        return JSON.stringify({ status: "committed", file, branch: g.currentBranch() });
      } catch (err) {
        return JSON.stringify({ error: err instanceof Error ? err.message : String(err) });
      }
    },
  });

  // ─── Tool: tree.promote ─────────────────────────────────────────────────────
  const treePromote = tool({
    description:
      "Commit a promoted finding — a high-signal finding visible to all peers immediately " +
      "via tree.query(). Sets promoted=true in the finding.",
    args: {
      summary: tool.schema.string().describe("Finding summary"),
      topic: tool.schema.string().describe("Topic category"),
      confidence: tool.schema.enum(["high", "medium", "low"]).describe("Confidence level"),
      type: tool.schema.enum(["finding", "hypothesis", "dead_end", "observation"]).optional().describe("Finding type (default: finding)"),
      evidence: tool.schema.string().optional().describe("Evidence supporting the finding"),
    },
    async execute({ summary, topic, confidence, type, evidence }, context) {
      try {
        const g = ensureGit();
        const agent = (context as { agent?: string })?.agent ?? "unknown";
        const branch = g.currentBranch();

        // Count existing findings to generate ID
        const existingFindings = g.listFiles("findings/*.json");
        const nextNum = String(existingFindings.length + 1).padStart(3, "0");
        const slug = topic.toLowerCase().replace(/[^a-z0-9]+/g, "-").slice(0, 30);
        const fileName = `findings/${nextNum}-${slug}.json`;

        const finding: Finding = {
          id: `f-${nextNum}`,
          agent,
          timestamp: new Date().toISOString(),
          topic,
          type: type ?? "finding",
          summary,
          confidence,
          evidence: evidence ?? "",
          promoted: true,
          refs: [],
        };

        // Secret scan
        const content = JSON.stringify(finding, null, 2);
        if (config.security.secret_scanning.enabled) {
          const scan = scanForSecrets(content);
          if (scan.matched) {
            return JSON.stringify({ error: "secret_detected", patterns_matched: scan.patterns_matched, action: "redact_and_retry" });
          }
        }

        g.commitFile(fileName, content, `promote: ${summary.slice(0, 50)}`);
        return JSON.stringify({ status: "promoted", file: fileName, finding_id: finding.id, branch });
      } catch (err) {
        return JSON.stringify({ error: err instanceof Error ? err.message : String(err) });
      }
    },
  });

  // ─── Tool: tree.merge ───────────────────────────────────────────────────────
  const treeMerge = tool({
    description:
      "Merge the agent's branch to main (or parent). Uses first-wins merge queue " +
      "with rebase-retry (REQ-TM-ASS-01). Returns success or merge_conflict error.",
    args: {
      target: tool.schema.string().optional().describe("Target branch (default: main)"),
      delete_branch: tool.schema.boolean().optional().describe("Delete source branch after merge (default: true)"),
    },
    async execute({ target, delete_branch }) {
      try {
        const g = ensureGit();
        const sourceBranch = g.currentBranch();
        if (sourceBranch === "main") {
          throw new Error("Cannot merge main into itself");
        }

        // axiom:trace work_item=tree-memory-01 spec=specs/113-Tree-Memory.md#3.2.1 plan=phase-H2/task-H2-2/step-H2-2-2
        const timeoutMs = (config.merge?.timeout_seconds ?? 60) * 1000;
        const result = g.merge(target ?? "main", timeoutMs);
        if (!result.success) {
          if (result.error === "merge_timeout") {
            return JSON.stringify({
              error: "merge_timeout",
              branch: sourceBranch,
              message: "Merge exceeded timeout limit; lock released to prevent queue blockage.",
            });
          }
          return JSON.stringify({
            error: "merge_conflict",
            branch: sourceBranch,
            conflicting_files: result.conflicting_files ?? [],
            retries_exhausted: true,
            action: "notify_orchestrator",
          });
        }

        // Delete source branch if requested
        if (delete_branch !== false) {
          g.deleteBranch(sourceBranch);
        }

        return JSON.stringify({ status: "merged", source: sourceBranch, target: target ?? "main" });
      } catch (err) {
        return JSON.stringify({ error: err instanceof Error ? err.message : String(err) });
      }
    },
  });

  // ─── Tool: tree.state ───────────────────────────────────────────────────────
  const treeState = tool({
    description:
      "Read or update the agent's state.json. State is committed to the agent's branch " +
      "and queryable by peers via tree.peers.",
    args: {
      update: tool.schema.string().optional().describe("JSON string with fields to update. If omitted, returns current state."),
    },
    async execute({ update }, context) {
      try {
        const g = ensureGit();
        const agent = (context as { agent?: string })?.agent ?? "unknown";
        const branch = g.currentBranch();

        if (!update) {
          // Read current state
          const stateContent = g.readFile("state.json");
          if (!stateContent) {
            return JSON.stringify({ state: null, message: "No state.json on current branch" });
          }
          return stateContent;
        }

        // Parse update
        let updateObj: Record<string, unknown>;
        try {
          updateObj = JSON.parse(update);
        } catch {
          throw new Error("Invalid JSON for state update");
        }

        // Read existing or create new
        const existing = g.readFile("state.json");
        const currentState: AgentState = existing
          ? JSON.parse(existing)
          : {
              agent,
              branch,
              status: "active",
              assignment: "",
              started_at: new Date().toISOString(),
              updated_at: new Date().toISOString(),
              tool_calls: 0,
              findings_count: 0,
              current_focus: "",
              next_step: "",
              log_frame_uuid: sessionUuid,
            };

        // Merge update
        const newState = {
          ...currentState,
          ...updateObj,
          updated_at: new Date().toISOString(),
          log_frame_uuid: sessionUuid,
        };

        const content = JSON.stringify(newState, null, 2);
        g.commitFile("state.json", content, `state: update ${agent}`);
        return JSON.stringify({ status: "updated", state: newState });
      } catch (err) {
        return JSON.stringify({ error: err instanceof Error ? err.message : String(err) });
      }
    },
  });

  // ─── Tool: tree.query ───────────────────────────────────────────────────────
  const treeQuery = tool({
    description:
      "Query tree memory data using the active query engine (DuckDB when installed via 'axiom install', " +
      "filesystem scanner otherwise). Agents never write raw SQL — parameters are used as filters only. " +
      "See tree.status for the active engine. See ADR-TM-004 for the two-path architecture.",
    args: {
      surface: tool.schema.enum(["findings", "peers", "watches", "events", "trends"]).describe("Query surface to use"),
      topic: tool.schema.string().optional().describe("Filter by topic (findings)"),
      agent: tool.schema.string().optional().describe("Filter by agent name"),
      confidence: tool.schema.string().optional().describe("Filter by confidence level (findings)"),
      type: tool.schema.string().optional().describe("Filter by type"),
      status: tool.schema.string().optional().describe("Filter by status (peers)"),
      since: tool.schema.string().optional().describe("Filter by timestamp (ISO 8601)"),
      watch_name: tool.schema.string().optional().describe("Filter by watch name (watches)"),
      question: tool.schema.string().optional().describe("Watch question: last_match, count, frequency, trend, last_n"),
      source: tool.schema.string().optional().describe("Filter by source (events)"),
      severity: tool.schema.string().optional().describe("Filter by severity (events)"),
      target_surface: tool.schema.string().optional().describe("Target surface for trends aggregation (findings, watches, events)"),
      metric: tool.schema.string().optional().describe("Metric to aggregate for trends (default: count)"),
      window: tool.schema.string().optional().describe("Time window for trends bucketing (e.g., 5m, 1h, 1d)"),
      group_by: tool.schema.string().optional().describe("Field to group by in trends"),
      limit: tool.schema.number().optional().describe("Max results (default: 50)"),
    },
    async execute(params) {
      try {
        let result: unknown;

        // axiom:trace work_item=tree-memory-duckdb-native spec=specs/113-Tree-Memory.md#4 plan=phase-2/task-2-1/step-2-1-1
        // DuckDB path (async): use adapter when available; filesystem scanner fallback via QueryEngine
        if (duckdbAdapter) {
          switch (params.surface) {
            case "findings":
              result = await duckdbAdapter.queryFindings(repoPath, config.security.duckdb_sandbox, projectRoot, {
                topic: params.topic, agent: params.agent, confidence: params.confidence,
                type: params.type, since: params.since, limit: params.limit,
              });
              break;
            case "peers":
              result = await duckdbAdapter.queryPeers(repoPath, config.security.duckdb_sandbox, projectRoot, {
                status: params.status, agent: params.agent,
              });
              break;
            case "watches":
              result = await duckdbAdapter.queryWatches(repoPath, config.security.duckdb_sandbox, projectRoot, {
                watch_name: params.watch_name, since: params.since,
                question: params.question as "last_match" | "count" | "frequency" | "trend" | "last_n" | undefined,
                limit: params.limit,
              });
              break;
            case "events":
              result = await duckdbAdapter.queryEvents(repoPath, config.security.duckdb_sandbox, projectRoot, {
                source: params.source, type: params.type, severity: params.severity,
                since: params.since, limit: params.limit,
              });
              break;
            case "trends":
              // Intentional: trends ALWAYS uses the TypeScript time-bucketing engine, even when
              // DuckDB is active. DuckDB-native trends (GROUP BY / window functions) is deferred
              // to v3. See specs/113-Tree-Memory.md §4.3 and ADR-TM-005 seed.
              // axiom:trace work_item=tree-memory-duckdb-native spec=specs/113-Tree-Memory.md#4.3
              result = ensureQuery().queryTrends({
                target_surface: params.target_surface ?? "findings",
                metric: params.metric, window: params.window,
                group_by: params.group_by, since: params.since, limit: params.limit,
              });
              break;
            default:
              // axiom:trace work_item=plugin-bug-sweep-01 spec=specs/113-Tree-Memory.md#5 plan=phase-1/task-1-1/step-5
              throw new Error(`Unknown surface: "${params.surface ?? "(not provided)"}". Valid surfaces: findings, peers, watches, events, trends`);
          }
        } else {
          // Filesystem scanner path (sync)
          const qe = ensureQuery();
          switch (params.surface) {
            case "findings":
              result = qe.queryFindings({
                topic: params.topic,
              agent: params.agent,
              confidence: params.confidence,
              type: params.type,
              since: params.since,
              limit: params.limit,
            });
            break;
          case "peers":
            result = qe.queryPeers({
              status: params.status,
              agent: params.agent,
            });
            break;
          case "watches":
            result = qe.queryWatches({
              watch_name: params.watch_name,
              since: params.since,
              question: params.question as "last_match" | "count" | "frequency" | "trend" | "last_n" | undefined,
              limit: params.limit,
            });
            break;
          case "events":
            result = qe.queryEvents({
              source: params.source,
              type: params.type,
              severity: params.severity,
              since: params.since,
              limit: params.limit,
            });
            break;
          case "trends":
            result = qe.queryTrends({
              target_surface: params.target_surface ?? "findings",
              metric: params.metric,
              window: params.window,
              group_by: params.group_by,
              since: params.since,
              limit: params.limit,
            });
            break;
          default:
            // axiom:trace work_item=plugin-bug-sweep-01 spec=specs/113-Tree-Memory.md#5 plan=phase-1/task-1-1/step-5
            throw new Error(`Unknown surface: "${params.surface ?? "(not provided)"}". Valid surfaces: findings, peers, watches, events, trends`);
        }
        } // end filesystem scanner path

        // BL-05: Apply global result_limit cap — truncate oversized result sets
        // axiom:trace work_item=tree-memory-01 spec=specs/113-Tree-Memory.md#4 plan=phase-BL/task-BL-05/step-BL-05-2
        const resultLimit = config.result_limit ?? 1000;
        if (Array.isArray(result) && result.length > resultLimit) {
          const total_count = result.length;
          result = {
            results: result.slice(0, resultLimit),
            has_more: true,
            total_count,
            result_limit: resultLimit,
          };
        }

        // Frame any log content with UUID markers (REQ-TM-SEC-01)
        const resultStr = JSON.stringify(result, null, 2);
        return frameLogContent(resultStr, sessionUuid);
      } catch (err) {
        return JSON.stringify({ error: err instanceof Error ? err.message : String(err) });
      }
    },
  });

  // ─── Tool: tree.peers ───────────────────────────────────────────────────────
  const treePeers = tool({
    description:
      "Query active agent states. Shorthand for tree.query(surface='peers').",
    args: {
      status: tool.schema.string().optional().describe("Filter by status: active, done, stale, merged"),
      agent: tool.schema.string().optional().describe("Filter by specific agent name"),
    },
    async execute({ status, agent }) {
      try {
        // axiom:trace work_item=tree-memory-duckdb-native spec=specs/113-Tree-Memory.md#4 plan=phase-2/task-2-1/step-2-1-1
        const results = duckdbAdapter
          ? await duckdbAdapter.queryPeers(repoPath, config.security.duckdb_sandbox, projectRoot, { status, agent })
          : ensureQuery().queryPeers({ status, agent });
        return JSON.stringify(results, null, 2);
      } catch (err) {
        return JSON.stringify({ error: err instanceof Error ? err.message : String(err) });
      }
    },
  });

  // ─── Tool: tree.log ─────────────────────────────────────────────────────────
  const treeLog = tool({
    description:
      "Get git log for a branch — history of commits (what happened, when, who).",
    args: {
      branch: tool.schema.string().optional().describe("Branch to show log for (default: current)"),
      limit: tool.schema.number().optional().describe("Max entries (default: 20)"),
    },
    async execute({ branch, limit }) {
      try {
        const g = ensureGit();
        const logOutput = g.log(branch, limit ?? 20);
        return JSON.stringify({ branch: branch ?? g.currentBranch(), log: logOutput });
      } catch (err) {
        return JSON.stringify({ error: err instanceof Error ? err.message : String(err) });
      }
    },
  });

  // ─── Tool: tree.spawn ───────────────────────────────────────────────────────
  const treeSpawn = tool({
    description:
      "Create a new branch + state file for a child agent. Supports branching from " +
      "a parent agent's branch (§12 Dynamic Branching Pattern).",
    args: {
      agent_name: tool.schema.string().describe("Name/ID for the new agent"),
      assignment: tool.schema.string().describe("What the agent should do"),
      from_branch: tool.schema.string().optional().describe("Branch to spawn from (default: main). Enables context inheritance."),
    },
    async execute({ agent_name, assignment, from_branch }, context) {
      try {
        const g = ensureGit();
        const parentAgent = (context as { agent?: string })?.agent ?? "unknown";

        // Check spawn budget
        const branches = g.listBranches();
        const activeBranches = branches.filter(b => b !== "main");
        if (activeBranches.length >= config.spawn_budget.max_total_active) {
          return JSON.stringify({
            error: "spawn_budget_exhausted",
            active: activeBranches.length,
            max: config.spawn_budget.max_total_active,
            message: `Spawn budget full (${activeBranches.length}/${config.spawn_budget.max_total_active} active branches)`,
          });
        }

        // Create branch
        const branchName = `agent-${agent_name}`;
        g.createBranch(branchName, from_branch);

        // Create initial state
        const state: AgentState = {
          agent: agent_name,
          branch: branchName,
          status: "active",
          assignment,
          started_at: new Date().toISOString(),
          updated_at: new Date().toISOString(),
          tool_calls: 0,
          findings_count: 0,
          current_focus: assignment,
          next_step: "Begin investigation",
          log_frame_uuid: randomUUID(), // Each spawned agent gets its own UUID
        };

        g.commitFile("state.json", JSON.stringify(state, null, 2), `spawn: ${agent_name} - ${assignment.slice(0, 50)}`);

        return JSON.stringify({
          status: "spawned",
          branch: branchName,
          agent: agent_name,
          from: from_branch ?? "main",
          assignment,
          spawned_by: parentAgent,
        });
      } catch (err) {
        return JSON.stringify({ error: err instanceof Error ? err.message : String(err) });
      }
    },
  });

  // ─── Tool: tree.status ──────────────────────────────────────────────────────
  const treeStatus = tool({
    description:
      "Overview of tree memory state: branches, active agents, last merge, enabled surfaces.",
    args: {},
    async execute() {
      try {
        if (!existsSync(join(repoPath, ".git"))) {
          return JSON.stringify({ initialized: false, message: "Tree memory not initialized. Call tree.init." });
        }
        const g = ensureGit();
        const branches = g.listBranches();
        const activeBranches = branches.filter(b => b !== "main");

        // Check for active agents
        const agents: string[] = [];
        for (const branch of activeBranches) {
          agents.push(branch.replace(/^agent-/, "").replace(/^bg-/, ""));
        }

        const enabledSurfaces = Object.entries(config.query_surfaces)
          .filter(([_, v]) => v.enabled)
          .map(([k]) => k);

        return JSON.stringify({
          initialized: true,
          branches: branches.length,
          active_agents: activeBranches.length,
          agent_branches: activeBranches,
          current_branch: g.currentBranch(),
          enabled_surfaces: enabledSurfaces,
          session_uuid: sessionUuid,
          // axiom:trace work_item=tree-memory-duckdb-native spec=specs/113-Tree-Memory.md#4 plan=phase-2/task-2-2/step-2-2-1
          engine: duckdbAdapter ? "duckdb" : "filesystem-scanner",
          engine_version: duckdbAdapter?.version ?? null,
          spawn_budget: {
            used: activeBranches.length,
            max: config.spawn_budget.max_total_active,
          },
        });
      } catch (err) {
        return JSON.stringify({ error: err instanceof Error ? err.message : String(err) });
      }
    },
  });

  // ─── Tool: tree.diff ─────────────────────────────────────────────────────────
  // axiom:trace work_item=plugin-bug-sweep-01 spec=specs/113-Tree-Memory.md#5.3 plan=phase-1/task-1-2/step-5
  const treeDiff = tool({
    description:
      "Show the diff between two branches — what's different in files and content. " +
      "Pass branch_a and branch_b to compare any two branches. " +
      "Pass only branch_a to diff it against main.",
    args: {
      branch_a: tool.schema.string().describe("Branch to compare (required)"),
      branch_b: tool.schema.string().optional().describe("Branch to compare against (default: main)"),
    },
    async execute({ branch_a, branch_b }) {
      try {
        const g = ensureGit();
        if (!branch_a) {
          return JSON.stringify({ error: "branch_a is required. Pass branch_a and optionally branch_b (defaults to main)." });
        }
        const b = branch_b ?? "main";
        const diffOutput = g.diff(branch_a, b);
        return JSON.stringify({ branch_a, branch_b: b, diff: diffOutput || "(no differences)" });
      } catch (err) {
        return JSON.stringify({ error: err instanceof Error ? err.message : String(err) });
      }
    },
  });

  // ─── Return plugin object ───────────────────────────────────────────────────
  return {
    tool: {
      "tree.init": treeInit,
      "tree.branch": treeBranch,
      "tree.commit": treeCommit,
      "tree.promote": treePromote,
      "tree.merge": treeMerge,
      "tree.state": treeState,
      "tree.query": treeQuery,
      "tree.peers": treePeers,
      "tree.log": treeLog,
      "tree.diff": treeDiff,
      "tree.spawn": treeSpawn,
      "tree.status": treeStatus,
    },
  };
};
