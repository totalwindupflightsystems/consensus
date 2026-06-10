/**
 * Context Stash Plugin — filesystem-backed suspend/resume system for agent working context.
 *
 * Tools:   stash.push, stash.pop, stash.apply, stash.list, stash.peek,
 *          stash.drop, stash.close, stash.create
 * Storage: .memory-bank/stash/suspended/*.md
 *          .memory-bank/stash/closed/*.md
 *          .memory-bank/stash/_index.md
 *
 * Security (spec §14):
 *   REQ-STASH-NEW-001  YAML content fields use double-quoted scalars (injection prevention)
 *   REQ-STASH-NEW-002  Stash ID regex + realpath boundary check
 *   REQ-STASH-NEW-003  Credential redaction before every write
 *   REQ-STASH-NEW-004  Session IDs hashed in _index.md (never raw)
 *   REQ-STASH-NEW-005  Atomic writes (write-to-temp then rename)
 *   REQ-STASH-NEW-007  Agent identity from ToolContext, NOT from tool args
 *
 * axiom:trace work_item=SWDE-44 spec=specs/106-Context-Stash.md plan=phase-0/task-0.1/step-0.1.1 jira_ref=SWDE-44
 */

import {
  existsSync,
  mkdirSync,
  readdirSync,
  readFileSync,
  realpathSync,
  rmSync,
} from "node:fs";
import * as fsPromises from "node:fs/promises";
import { dirname, join, resolve } from "node:path";
import { createHash } from "node:crypto";
import { parse as yamlParse } from "yaml";
import { tool } from "@opencode-ai/plugin";
import { loadPluginConfig } from "../lib/config-utils.ts";

// ─────────────────────────────────────────────────────────────────────────────
// Types
// ─────────────────────────────────────────────────────────────────────────────

export interface StashFrontmatter {
  stash_id: string;
  name: string;
  state: "suspended" | "closed" | "active";
  created_by: string;
  created_at: string;
  suspended_at?: string;
  closed_at?: string;
  /** Present in stash file for forensics linkage — NOT raw in _index.md */
  session_id?: string;
  tags: string[];
  entries?: number;
  last_agent?: string;
  resume_hint?: string;
  outcome?: string;
  log_level?: "all" | "decisions" | "summaries" | "off";  // REQ-STASH-031
  background?: boolean;  // REQ-STASH-037 — true = background context
}

// ─────────────────────────────────────────────────────────────────────────────
// Storage Backend Interface (REQ-STASH-NEW-013 through REQ-STASH-NEW-017)
// axiom:trace work_item=SWDE-55 spec=specs/106-Context-Stash.md#REQ-STASH-NEW-013
// ─────────────────────────────────────────────────────────────────────────────

export interface StashContent {
  stashId: string;
  state: "suspended" | "closed" | "active";
  raw: string;
}

export interface StashEntry {
  ts: string;
  agent: string;
  type: "observation" | "decision" | "tool_call" | "finding" | "summary" | "handoff" | "question" | "blocker";
  content: string;
  refs?: string[];
  severity?: "info" | "warn" | "error" | "critical";
  to_agent?: string;   // REQ-STASH-062: for handoff entries
  session_id?: string; // REQ-STASH-101: forensics linkage
}

export interface StashFilter {
  state?: "suspended" | "closed" | "active";
  tag?: string;
  agent?: string;
}

export interface StashSummary {
  stash_id: string;
  name: string;
  state: "suspended" | "closed" | "active";
  tags: string[];
  created_at: string;
  last_agent?: string;
  resume_hint?: string;
  content: string;
  filePath: string;
}

/**
 * REQ-STASH-NEW-013: Storage backend interface for Context Stash.
 * Implementations: LocalFileBackend (default), S3Backend, PostgresBackend.
 * axiom:trace work_item=SWDE-55 spec=specs/106-Context-Stash.md#REQ-STASH-NEW-013
 */
export interface StashStorageBackend {
  /** Read a stash. Checks suspended first, then closed, then active (if state not specified). */
  read(stashId: string, state?: "suspended" | "closed" | "active"): Promise<StashContent | null>;
  /** Write a stash file atomically. Updates index for local backend. */
  write(stashId: string, content: StashContent): Promise<void>;
  /**
   * Atomically close a stash: write closed content, delete suspended file, update index.
   * Returns { warning } if write succeeded but delete failed (dual-existence scenario).
   */
  moveToClose(stashId: string, closedContent: StashContent): Promise<{ warning?: string }>;
  /** List all stashes. Sorted by created_at desc. */
  list(filter?: StashFilter): Promise<StashSummary[]>;
  /** Delete a stash file. state param: delete only from that state. Updates index. */
  delete(stashId: string, state?: "suspended" | "closed" | "active"): Promise<void>;
  /** Check if a stash exists. state param: check only that state. */
  exists(stashId: string, state?: "suspended" | "closed" | "active"): Promise<boolean>;
  /**
   * Append to active stash (Phase 2).
   * Optional — callers MUST check `if (backend.append)` before calling.
   * All three concrete backends implement this method (throwing for non-active stashes
   * when no active stash exists), but future backends may omit it.
   * axiom:trace work_item=SWDE-55 spec=specs/106-Context-Stash.md#REQ-STASH-NEW-013 plan=phase-4/fix-swde55-append-interface
   * axiom:trace work_item=SWDE-59 spec=specs/106-Context-Stash.md#REQ-STASH-NEW-017 plan=phase-6-post-verify-run3/add-fallbackbackend-append-warning
    */
   append?(stashId: string, entry: StashEntry): Promise<{ warning?: string }>;
 }

// ─────────────────────────────────────────────────────────────────────────────
// Security helpers (exported for testing)
// ─────────────────────────────────────────────────────────────────────────────

/**
 * REQ-STASH-NEW-002: Stash ID must match ^[a-z0-9][a-z0-9-]{0,63}$.
 * Throws on invalid ID.
 */
export const STASH_ID_REGEX = /^[a-z0-9][a-z0-9-]{0,63}$/;

export function validateStashId(id: string): void {
  if (!STASH_ID_REGEX.test(id)) {
    throw new Error(
      `Invalid stash ID "${id}". Must match ^[a-z0-9][a-z0-9-]{0,63}$`
    );
  }
}

/**
 * REQ-STASH-NEW-002: Resolve path and verify it stays within storageRoot.
 * Uses realpathSync to follow symlinks and reject symlink-based traversal.
 *
 * For existing files: realpathSync(candidate) resolves all symlinks, then
 * we verify the result is within storageRoot.
 *
 * For new files (ENOENT on candidate): realpathSync(dirname(candidate)) resolves
 * the parent directory (which must exist), then we reconstruct the target path
 * from the validated parent + basename.
 *
 * If realpathSync throws for a reason other than ENOENT, we propagate the error.
 */
export function safePath(
  storageRoot: string,
  subdir: string,
  filename: string
): string {
  let resolvedRoot: string;
  try {
    resolvedRoot = realpathSync(storageRoot);
  } catch {
    resolvedRoot = resolve(storageRoot);
  }
  const candidate = join(resolvedRoot, subdir, filename);

  let resolvedCandidate: string;
  try {
    // Happy path: file exists — resolve all symlinks
    resolvedCandidate = realpathSync(candidate);
  } catch (err: unknown) {
    const nodeErr = err as NodeJS.ErrnoException;
    if (nodeErr.code === "ENOENT") {
      // File doesn't exist yet (write case) — resolve the parent directory instead
      // and reconstruct the path using the literal basename (no symlink possible for
      // a file that doesn't exist yet).
      const parentDir = dirname(candidate);
      const basename = candidate.slice(parentDir.length + 1);
      try {
        const resolvedParent = realpathSync(parentDir);
        resolvedCandidate = join(resolvedParent, basename);
      } catch {
        // Parent also doesn't exist — fall back to resolve() for the boundary check
        resolvedCandidate = resolve(candidate);
      }
    } else {
      throw err;
    }
  }

  if (
    !resolvedCandidate.startsWith(resolvedRoot + "/") &&
    resolvedCandidate !== resolvedRoot
  ) {
    throw new Error(
      `Path traversal rejected: "${candidate}" is outside storage root "${resolvedRoot}"`
    );
  }
  return resolvedCandidate;
}

/**
 * REQ-STASH-NEW-003: Redact credential patterns before any disk write.
 * Patterns: Bearer tokens, sk- keys, AWS AKIA keys, PEM blocks,
 *           GitHub tokens, password/token/secret= patterns, high-entropy strings.
 */
export function redactCredentials(input: string): string {
  let out = input;
  // Bearer tokens
  out = out.replace(/Bearer\s+[A-Za-z0-9\-._~+/]+=*/gi, "Bearer [REDACTED]");
  // OpenAI sk- keys
  out = out.replace(/sk-[A-Za-z0-9]{20,}/g, "[REDACTED]");
  // AWS AKIA access key IDs
  out = out.replace(/AKIA[A-Z0-9]{16}/g, "[REDACTED]");
  // PEM private/public key blocks
  out = out.replace(
    /-----BEGIN[^-]+-----[\s\S]*?-----END[^-]+-----/g,
    "[REDACTED]"
  );
  // GitHub personal/OAuth/refresh tokens
  out = out.replace(/gh[posrt]_[A-Za-z0-9]{36}/g, "[REDACTED]");
  // password/token/secret/api_key= patterns
  out = out.replace(
    /(?:password|token|secret|api_key|apikey|access_key)\s*[:=]\s*\S+/gi,
    (m) => {
      const eqIdx = m.search(/[:=]/);
      return eqIdx >= 0 ? m.slice(0, eqIdx + 1) + " [REDACTED]" : "[REDACTED]";
    }
  );
  // Connection string DSN patterns (postgresql, mysql, mongodb, redis, etc.)
  // e.g., postgresql://admin:mysecret@localhost/db → postgresql://admin:[REDACTED]@localhost/db
  // Also handles empty-user DSNs: redis://:secretkey@host → redis://:[REDACTED]@host
  // axiom:trace work_item=SWDE-55 spec=specs/106-Context-Stash.md#REQ-STASH-NEW-003 plan=P3/sec-001
  out = out.replace(/([a-zA-Z][a-zA-Z0-9+\-.]*:\/\/[^:@\s]*:)[^@\s]+(@[^\s]*)/g, "$1[REDACTED]$2");
  // High-entropy strings (mixed upper+lower+digit, 40+ chars)
  out = out.replace(/[A-Za-z0-9_\-]{40,}/g, (m) => {
    const hasUpper = /[A-Z]/.test(m);
    const hasLower = /[a-z]/.test(m);
    const hasDigit = /[0-9]/.test(m);
    if (hasUpper && hasLower && hasDigit) return "[REDACTED]";
    return m;
  });
  return out;
}

/**
 * REQ-STASH-NEW-001: Serialize a string as a YAML double-quoted scalar.
 * Escapes all characters that could cause YAML injection:
 *   - Newlines → \n  (prevents \n---\n document boundary injection)
 *   - Backslashes → \\
 *   - Double quotes → \"
 * Returns the value wrapped in double quotes: "escaped-content"
 */
export function yamlDoubleQuote(value: string): string {
  const escaped = value
    .replace(/\\/g, "\\\\")
    .replace(/"/g, '\\"')
    .replace(/\n/g, "\\n")
    .replace(/\r/g, "\\r")
    .replace(/\t/g, "\\t")
    .replace(/\x00/g, "\\0");
  return `"${escaped}"`;
}

/**
 * REQ-STASH-NEW-004: Hash a session ID for _index.md storage.
 * Returns first 8 hex chars of SHA-256. Sufficient for correlation
 * without exposing the raw session ID.
 */
export function hashSessionId(sessionId: string): string {
  return createHash("sha256").update(sessionId).digest("hex").slice(0, 8);
}

// ─────────────────────────────────────────────────────────────────────────────
// Utility helpers (exported for testing)
// ─────────────────────────────────────────────────────────────────────────────

/** Convert a human-readable stash name to a URL-safe stash ID. */
export function slugify(name: string): string {
  return name
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, "-")
    .replace(/^-+|-+$/g, "")
    .slice(0, 64);
}

/** Format an ISO date as a human-readable relative age (e.g., "5m ago"). */
export function formatAge(isoDate: string): string {
  if (!isoDate) return "unknown";
  try {
    const diff = Date.now() - new Date(isoDate).getTime();
    const mins = Math.floor(diff / 60000);
    if (mins < 60) return `${mins}m ago`;
    const hours = Math.floor(mins / 60);
    if (hours < 24) return `${hours}h ago`;
    const days = Math.floor(hours / 24);
    return `${days}d ago`;
  } catch {
    return "unknown";
  }
}

// ─────────────────────────────────────────────────────────────────────────────
// Stash file format (exported for testing)
// ─────────────────────────────────────────────────────────────────────────────

/**
 * Build the markdown content for a suspended or closed stash file.
 *
 * Security invariants:
 *   - All user-supplied string fields use yamlDoubleQuote() → no \n---\n injection
 *   - redactCredentials() applied to all user content before write
 *   - Fuzz validation: asserts frontmatter has exactly 2 --- boundaries
 *
 * axiom:trace work_item=SWDE-44 spec=specs/106-Context-Stash.md#REQ-STASH-NEW-001
 */
export function buildSuspendedMarkdown(
  fm: StashFrontmatter & { session_id: string },
  summary: string | undefined,
  detail?: string,
  resumeHint?: string
): string {
  // Null-safe guard: summary may arrive as undefined when called outside OpenCode schema validation
  // axiom:trace work_item=plugin-bug-sweep-01 spec=specs/106-Context-Stash.md#REQ-STASH-001 plan=phase-1/step-2
  summary = summary ?? "";
  // Redact credentials from all user-supplied content (REQ-STASH-NEW-003)
  const safeSummary = redactCredentials(summary);
  const safeDetail = detail ? redactCredentials(detail) : undefined;
  const safeResumeHint = resumeHint ? redactCredentials(resumeHint) : undefined;
  const safeName = redactCredentials(fm.name);
  const safeFmResumeHint = fm.resume_hint
    ? redactCredentials(fm.resume_hint)
    : undefined;
  const safeFmOutcome = fm.outcome ? redactCredentials(fm.outcome) : undefined;

  // Build YAML frontmatter — user-supplied fields use double-quoted scalars
  // to prevent \n---\n document boundary injection (REQ-STASH-NEW-001)
  const fmLines: string[] = ["---"];
  fmLines.push(`stash_id: ${fm.stash_id}`);
  fmLines.push(`name: ${yamlDoubleQuote(safeName)}`);
  fmLines.push(`state: ${fm.state}`);
  fmLines.push(`created_by: ${fm.created_by}`);
  fmLines.push(`created_at: ${fm.created_at}`);
  if (fm.suspended_at) fmLines.push(`suspended_at: ${fm.suspended_at}`);
  if (fm.closed_at) fmLines.push(`closed_at: ${fm.closed_at}`);
  // session_id in stash file for forensics — _index.md only gets the hash
  if (fm.session_id) fmLines.push(`session_id: ${fm.session_id}`);
  if (fm.tags && fm.tags.length > 0) {
    fmLines.push(`tags: [${fm.tags.join(", ")}]`);
  } else {
    fmLines.push(`tags: []`);
  }
  if (fm.entries !== undefined) fmLines.push(`entries: ${fm.entries}`);
  if (fm.last_agent) fmLines.push(`last_agent: ${fm.last_agent}`);
  // Preserve log_level across exit/enter cycles (F2 fix — REQ-STASH-031)
  if ((fm as any).log_level) fmLines.push(`log_level: ${(fm as any).log_level}`);
  // User-supplied optional fields → double-quoted
  if (safeFmResumeHint)
    fmLines.push(`resume_hint: ${yamlDoubleQuote(safeFmResumeHint)}`);
  if (safeFmOutcome)
    fmLines.push(`outcome: ${yamlDoubleQuote(safeFmOutcome)}`);
  fmLines.push("---");

  // Build markdown body
  const bodyLines: string[] = ["", `# ${safeName}`, ""];
  if (safeSummary) {
    bodyLines.push("## Summary", safeSummary, "");
  }
  if (safeDetail) {
    bodyLines.push("## Detail", safeDetail, "");
  }
  if (safeResumeHint) {
    bodyLines.push("## Resume Hint", safeResumeHint, "");
  }

  const markdown = fmLines.join("\n") + bodyLines.join("\n");

  // Defense-in-depth: verify the frontmatter section has no bare \n---
  // (should be impossible with yamlDoubleQuote, but belt-and-suspenders)
  const frontmatterClose = markdown.indexOf("\n---\n");
  if (frontmatterClose !== -1) {
    const frontmatterSection = markdown.slice(4, frontmatterClose); // skip opening ---\n
    if (/\n---/.test(frontmatterSection)) {
      throw new Error(
        "YAML injection detected in frontmatter — build aborted"
      );
    }
  }

  return markdown;
}

/**
 * Parse a single YAML stash entry block into a plain object.
 * Handles the `- ts: ... \n  agent: ... \n  type: ...` format.
 * Strips the leading "- " from the first line and dedents the rest (2-space indent).
 * axiom:trace work_item=SWDE-55 spec=specs/106-Context-Stash.md#REQ-STASH-024
 */
export function parseStashEntry(raw: string): Record<string, unknown> | null {
  try {
    const lines = raw.split("\n");
    // Remove leading "- " from first non-empty line, dedent 2 spaces from rest
    const cleaned = lines.map((line, idx) => {
      if (idx === 0) return line.replace(/^-\s{0,1}/, "");
      // Remove 2-space indent that was part of the YAML list item indentation
      return line.startsWith("  ") ? line.slice(2) : line;
    }).join("\n");
    const parsed = yamlParse(cleaned) as Record<string, unknown>;
    return parsed && parsed.ts ? parsed : null;
  } catch {
    return null;
  }
}

/**
 * Serialize a StashEntry as a YAML list item for appending to an active stash log.
 * Uses yamlDoubleQuote() for all string fields (REQ-STASH-NEW-001).
 * axiom:trace work_item=SWDE-55 spec=specs/106-Context-Stash.md#REQ-STASH-NEW-013
 */
export function buildEntryYaml(entry: StashEntry): string {
  const lines: string[] = [`\n- ts: ${yamlDoubleQuote(entry.ts)}`];
  lines.push(`  agent: ${yamlDoubleQuote(entry.agent)}`);
  lines.push(`  type: ${entry.type}`);
  lines.push(`  content: ${yamlDoubleQuote(redactCredentials(entry.content))}`);
  if (entry.refs && entry.refs.length > 0) {
    const refsStr = entry.refs.map((r) => yamlDoubleQuote(r)).join(", ");
    lines.push(`  refs: [${refsStr}]`);
  }
  if (entry.severity) {
    lines.push(`  severity: ${entry.severity}`);
  }
  if (entry.to_agent) {
    lines.push(`  to_agent: ${yamlDoubleQuote(entry.to_agent)}`);
  }
  if ((entry as any).session_id) {
    lines.push(`  session_id: ${yamlDoubleQuote(String((entry as any).session_id))}`);
  }
  return lines.join("\n") + "\n";
}

/**
 * Build the initial YAML content for an active stash (YAML append-log format, §2.4).
 * axiom:trace work_item=SWDE-55 spec=specs/106-Context-Stash.md#REQ-STASH-NEW-013
 */
export function buildActiveYaml(
  fm: StashFrontmatter & { session_id: string; entered_at: string }
): string {
  const fmLines = ["---"];
  fmLines.push(`stash_id: ${fm.stash_id}`);
  fmLines.push(`name: ${yamlDoubleQuote(fm.name)}`);
  fmLines.push(`state: active`);
  fmLines.push(`created_by: ${fm.created_by}`);
  fmLines.push(`created_at: ${fm.created_at}`);
  fmLines.push(`entered_at: ${fm.entered_at}`);
  if (fm.session_id) fmLines.push(`session_id: ${fm.session_id}`);
  if (fm.tags && fm.tags.length > 0) {
    fmLines.push(`tags: [${fm.tags.join(", ")}]`);
  } else {
    fmLines.push(`tags: []`);
  }
  if (fm.last_agent) fmLines.push(`last_agent: ${fm.last_agent}`);
  if (fm.resume_hint) fmLines.push(`resume_hint: ${yamlDoubleQuote(fm.resume_hint)}`);
  if (fm.log_level) fmLines.push(`log_level: ${fm.log_level}`);
  if (fm.background) fmLines.push(`background: true`);
  fmLines.push("---");
  return fmLines.join("\n") + "\n";
}

/**
 * Parse YAML frontmatter from a stash markdown file.
 * Returns { fm: parsed frontmatter object, body: markdown body }.
 */
export function parseFrontmatter(content: string): {
  fm: Partial<StashFrontmatter & { session_id: string }>;
  body: string;
} {
  const match = content.match(/^---\n([\s\S]*?)\n---\n?([\s\S]*)$/);
  if (!match) return { fm: {}, body: content };
  try {
    const fm = yamlParse(match[1]) as Partial<
      StashFrontmatter & { session_id: string }
    >;
    return { fm: fm ?? {}, body: match[2] };
  } catch {
    return { fm: {}, body: content };
  }
}

// ─────────────────────────────────────────────────────────────────────────────
// Atomic write (REQ-STASH-NEW-005)
// ─────────────────────────────────────────────────────────────────────────────

/**
 * Write content to targetPath atomically: write to <targetPath>.tmp then
 * rename to targetPath. Prevents partial-write artifacts on crash.
 * axiom:trace work_item=SWDE-44 spec=specs/106-Context-Stash.md#REQ-STASH-NEW-005
 */
export async function atomicWrite(
  targetPath: string,
  content: string
): Promise<void> {
  const tmpPath = `${targetPath}.tmp`;
  await fsPromises.writeFile(tmpPath, content, "utf-8");
  await fsPromises.rename(tmpPath, targetPath);
}

// ─────────────────────────────────────────────────────────────────────────────
// _index.md management (REQ-STASH-010, REQ-STASH-NEW-004)
// ─────────────────────────────────────────────────────────────────────────────

interface IndexEntry {
  stash_id: string;
  name: string;
  state: string;
  tags: string[];
  created_at: string;
  last_agent?: string;
  /** Hashed session ID — NEVER raw (REQ-STASH-NEW-004) */
  session_hash?: string;
}

function buildIndexMarkdown(
  entries: IndexEntry[],
  updatedAt: string
): string {
  const lines = [
    "# Context Stash Index",
    "",
    `_Updated: ${updatedAt}_`,
    "",
    "| ID | Name | State | Tags | Created | Last Agent |",
    "|----|------|-------|------|---------|------------|",
  ];
  for (const e of entries) {
    const tags = e.tags.join(", ");
    // Sanitize table cells: strip pipe chars to prevent table injection
    const row = [
      e.stash_id,
      e.name,
      e.state,
      tags,
      e.created_at,
      e.last_agent ?? "",
    ].map((v) => v.replace(/\|/g, ""));
    lines.push(`| ${row.join(" | ")} |`);
  }
  lines.push("");
  lines.push(
    "<!-- axiom:trace work_item=SWDE-44 spec=specs/106-Context-Stash.md#REQ-STASH-010 -->"
  );
  return lines.join("\n");
}

export function parseIndex(content: string): IndexEntry[] {
  const entries: IndexEntry[] = [];
  const tableLines = content
    .split("\n")
    .filter(
      (l) =>
        l.startsWith("| ") &&
        !l.startsWith("| ID") &&
        !l.startsWith("|-")
    );
  for (const line of tableLines) {
    const parts = line
      .split("|")
      .map((p) => p.trim())
      .filter((p) => p);
    if (parts.length >= 5) {
      entries.push({
        stash_id: parts[0],
        name: parts[1],
        state: parts[2],
        tags: parts[3]
          ? parts[3]
              .split(",")
              .map((t) => t.trim())
              .filter((t) => t)
          : [],
        created_at: parts[4],
        last_agent: parts[5] || undefined,
      });
    }
  }
  return entries;
}

async function updateIndex(
  storageRoot: string,
  stashId: string,
  fm: Partial<StashFrontmatter & { session_id: string }>
): Promise<void> {
  const indexPath = join(storageRoot, "_index.md");
  let entries: IndexEntry[] = [];
  if (existsSync(indexPath)) {
    const content = readFileSync(indexPath, "utf-8");
    entries = parseIndex(content);
  }
  const idx = entries.findIndex((e) => e.stash_id === stashId);
  const newEntry: IndexEntry = {
    stash_id: stashId,
    name: fm.name ?? stashId,
    state: fm.state ?? "suspended",
    tags: fm.tags ?? [],
    created_at: fm.created_at ?? new Date().toISOString(),
    last_agent: fm.last_agent,
    // REQ-STASH-NEW-004: hash session ID — never store raw value
    session_hash: fm.session_id ? hashSessionId(fm.session_id) : undefined,
  };
  if (idx >= 0) {
    entries[idx] = newEntry;
  } else {
    entries.push(newEntry);
  }
  await atomicWrite(
    indexPath,
    buildIndexMarkdown(entries, new Date().toISOString())
  );
}

async function removeFromIndex(
  storageRoot: string,
  stashId: string
): Promise<void> {
  const indexPath = join(storageRoot, "_index.md");
  if (!existsSync(indexPath)) return;
  const content = readFileSync(indexPath, "utf-8");
  const entries = parseIndex(content).filter((e) => e.stash_id !== stashId);
  await atomicWrite(
    indexPath,
    buildIndexMarkdown(entries, new Date().toISOString())
  );
}

// ─────────────────────────────────────────────────────────────────────────────
// Filesystem helpers
// ─────────────────────────────────────────────────────────────────────────────

interface StashRecord {
  stash_id: string;
  name: string;
  state: "suspended" | "closed" | "active";
  tags: string[];
  created_at: string;
  last_agent?: string;
  resume_hint?: string;
  content: string;
  filePath: string;
}

export function readAllStashes(storageRoot: string): StashRecord[] {
  const records: StashRecord[] = [];
  for (const dir of ["suspended", "closed", "active"]) {
    const dirPath = join(storageRoot, dir);
    if (!existsSync(dirPath)) continue;
    // active/ uses .yaml files; suspended/ and closed/ use .md files
    const ext = dir === "active" ? ".yaml" : ".md";
    for (const filename of readdirSync(dirPath)) {
      if (!filename.endsWith(ext)) continue;
      const filePath = join(dirPath, filename);
      try {
        const content = readFileSync(filePath, "utf-8");
        const { fm } = parseFrontmatter(content);
        if (fm.stash_id) {
          records.push({
            stash_id: fm.stash_id,
            name: fm.name ?? filename.replace(ext, ""),
            state: fm.state ?? (dir as "suspended" | "closed" | "active"),
            tags: fm.tags ?? [],
            created_at: fm.created_at ?? "",
            last_agent: fm.last_agent,
            resume_hint: fm.resume_hint,
            content,
            filePath,
          });
        }
      } catch {
        /* skip malformed files */
      }
    }
  }
  // Sort by created_at descending — most recent first
  return records.sort((a, b) => b.created_at.localeCompare(a.created_at));
}

// ─────────────────────────────────────────────────────────────────────────────
// ContextStashPlugin is declared at the BOTTOM of this file (after all backend
// classes) to avoid temporal dead zone errors on FallbackBackend, S3Backend,
// PostgresBackend, and LocalFileBackend class references.
// axiom:trace work_item=SWDE-44 spec=specs/106-Context-Stash.md#3.1 plan=phase-0/task-0.1/step-0.1.1
// Forward declaration note: see bottom of file for ContextStashPlugin export.
// ─────────────────────────────────────────────────────────────────────────────

// ─────────────────────────────────────────────────────────────────────────────
// §8 Config Reading (REQ-STASH: §8)
// axiom:trace work_item=SWDE-55 spec=specs/106-Context-Stash.md#8
// ─────────────────────────────────────────────────────────────────────────────

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
    track_suspended: false, // REQ-STASH-NEW-006: MUST default to false (credential safety)
  },
};

// axiom:trace work_item=conductor-phase4-config-adoption spec=specs/112-Plugin-Config-Management.md#8 plan=phase-4/task-4-4/backlog-002
// loadStashConfig() removed — replaced by loadPluginConfig("context-stash", DEFAULT_STASH_CONFIG, repoRoot).

// ─────────────────────────────────────────────────────────────────────────────
// REQ-STASH-093: Structured lifecycle log events
// axiom:trace work_item=SWDE-55 spec=specs/106-Context-Stash.md#REQ-STASH-093
// ─────────────────────────────────────────────────────────────────────────────

function logLifecycleEvent(event: { type: string; stash_id: string; state?: string; agent?: string; [key: string]: unknown }): void {
  // axiom:trace work_item=SWDE-55 spec=specs/106-Context-Stash.md#REQ-STASH-093
  console.log(JSON.stringify({ source: "context-stash", ts: new Date().toISOString(), ...event }));
}

// ─────────────────────────────────────────────────────────────────────────────
// REQ-STASH-043: Pre-compact snapshot helper
// axiom:trace work_item=SWDE-55 spec=specs/106-Context-Stash.md#REQ-STASH-043
// ─────────────────────────────────────────────────────────────────────────────

async function saveCompactSnapshot(stashId: string, content: string, storageRoot: string): Promise<void> {
  // axiom:trace work_item=SWDE-55 spec=specs/106-Context-Stash.md#REQ-STASH-043
  try {
    const ts = new Date().toISOString().replace(/[:.]/g, "-").slice(0, 19);
    const snapshotDir = join(storageRoot, "active", ".snapshots");
    if (!existsSync(snapshotDir)) mkdirSync(snapshotDir, { recursive: true });
    const snapshotPath = join(snapshotDir, `${stashId}-${ts}.yaml`);
    await atomicWrite(snapshotPath, content);
  } catch { /* never crash compact on snapshot failure */ }
}

// ─────────────────────────────────────────────────────────────────────────────
// LocalFileBackend (REQ-STASH-NEW-014 — wraps existing filesystem logic)
// axiom:trace work_item=SWDE-55 spec=specs/106-Context-Stash.md#REQ-STASH-NEW-014
// ─────────────────────────────────────────────────────────────────────────────

/**
 * REQ-STASH-NEW-014: Refactor local file implementation to StashStorageBackend.
 * Behavior-preserving wrapper around existing filesystem helpers.
 */
export class LocalFileBackend implements StashStorageBackend {
  constructor(private readonly storageRoot: string) {
    // Bootstrap directories
    for (const dir of [
      storageRoot,
      join(storageRoot, "suspended"),
      join(storageRoot, "closed"),
      join(storageRoot, "active"),
    ]) {
      if (!existsSync(dir)) mkdirSync(dir, { recursive: true });
    }
    // Clean up orphaned .tmp files (REQ-STASH-NEW-005)
    for (const dir of ["suspended", "closed", "active"]) {
      const dirPath = join(storageRoot, dir);
      if (!existsSync(dirPath)) continue;
      for (const f of readdirSync(dirPath)) {
        if (f.endsWith(".md.tmp") || f.endsWith(".yaml.tmp")) {
          try { rmSync(join(dirPath, f)); } catch { /* ignore */ }
        }
      }
    }
  }

  async read(stashId: string, state?: "suspended" | "closed" | "active"): Promise<StashContent | null> {
    const states = state ? [state] : (["suspended", "closed", "active"] as const);
    for (const s of states) {
      try {
        const ext = s === "active" ? ".yaml" : ".md";
        const filePath = safePath(this.storageRoot, s, `${stashId}${ext}`);
        if (existsSync(filePath)) {
          const raw = readFileSync(filePath, "utf-8");
          return { stashId, state: s, raw };
        }
      } catch { /* path traversal rejected — skip */ }
    }
    return null;
  }

  async write(stashId: string, content: StashContent): Promise<void> {
    const ext = content.state === "active" ? ".yaml" : ".md";
    const filePath = safePath(this.storageRoot, content.state, `${stashId}${ext}`);
    await atomicWrite(filePath, content.raw);
    // Only update index for non-active stashes (active stash index updates happen on enter/exit)
    if (content.state !== "active") {
      const { fm } = parseFrontmatter(content.raw);
      await updateIndex(this.storageRoot, stashId, fm);
    }
  }

  async moveToClose(stashId: string, closedContent: StashContent): Promise<{ warning?: string }> {
    const sourcePath = safePath(this.storageRoot, "suspended", `${stashId}.md`);
    const destPath = safePath(this.storageRoot, "closed", `${stashId}.md`);
    await atomicWrite(destPath, closedContent.raw);
    try {
      rmSync(sourcePath);
    } catch (rmErr) {
      const { fm } = parseFrontmatter(closedContent.raw);
      await updateIndex(this.storageRoot, stashId, fm);
      return {
        warning: `Stash closed successfully but the suspended copy could not be removed: ${
          rmErr instanceof Error ? rmErr.message : String(rmErr)
        }. Manually delete 'suspended/${stashId}.md' to resolve.`,
      };
    }
    const { fm } = parseFrontmatter(closedContent.raw);
    await updateIndex(this.storageRoot, stashId, fm);
    return {};
  }

  async list(filter?: StashFilter): Promise<StashSummary[]> {
    let records = readAllStashes(this.storageRoot);
    if (filter?.state) records = records.filter((r) => r.state === filter.state);
    if (filter?.tag) records = records.filter((r) => r.tags.includes(filter.tag!));
    if (filter?.agent) records = records.filter((r) => r.last_agent === filter.agent);
    return records;
  }

  async delete(stashId: string, state?: "suspended" | "closed" | "active"): Promise<void> {
    const states = state ? [state] : (["suspended", "closed", "active"] as const);
    for (const s of states) {
      try {
        const ext = s === "active" ? ".yaml" : ".md";
        const filePath = safePath(this.storageRoot, s, `${stashId}${ext}`);
        if (existsSync(filePath)) {
          rmSync(filePath);
          if (s !== "active") {
            await removeFromIndex(this.storageRoot, stashId);
          }
          return;
        }
      } catch { /* path traversal rejected */ }
    }
  }

  async exists(stashId: string, state?: "suspended" | "closed" | "active"): Promise<boolean> {
    const states = state ? [state] : (["suspended", "closed", "active"] as const);
    for (const s of states) {
      try {
        const ext = s === "active" ? ".yaml" : ".md";
        const filePath = safePath(this.storageRoot, s, `${stashId}${ext}`);
        if (existsSync(filePath)) return true;
      } catch { /* path traversal rejected */ }
    }
    return false;
  }

  async append(stashId: string, entry: StashEntry): Promise<{ warning?: string }> {
    const filePath = safePath(this.storageRoot, "active", `${stashId}.yaml`);
    if (!existsSync(filePath)) {
      throw new Error(`No active stash '${stashId}'. Use stash.enter to activate a stash first.`);
    }
    const entryYaml = buildEntryYaml(entry);
    await fsPromises.appendFile(filePath, entryYaml, "utf-8");
    return {};
  }
}

// ─────────────────────────────────────────────────────────────────────────────
// Backend factory (REQ-STASH-NEW-017)
// axiom:trace work_item=SWDE-55 spec=specs/106-Context-Stash.md#REQ-STASH-NEW-017
// ─────────────────────────────────────────────────────────────────────────────

// ─────────────────────────────────────────────────────────────────────────────
// PostgreSQL Backend (REQ-STASH-NEW-016)
// axiom:trace work_item=SWDE-55 spec=specs/106-Context-Stash.md#REQ-STASH-NEW-016
// ─────────────────────────────────────────────────────────────────────────────
//
// NOTE: FallbackBackend and StashClientFactory are defined further below
// (after S3Backend) because they reference LocalFileBackend, S3Backend,
// and PostgresBackend — all of which must be declared first.

export interface PGClientInterface {
  query(sql: string, params?: unknown[]): Promise<{ rows: Record<string, unknown>[]; rowCount?: number }>;
}

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

/**
 * REQ-STASH-NEW-016: PostgreSQL backend for Context Stash.
 * Uses injected PGClientInterface for testability.
 * axiom:trace work_item=SWDE-55 spec=specs/106-Context-Stash.md#REQ-STASH-NEW-016
 */
export class PostgresBackend implements StashStorageBackend {
  // axiom:trace work_item=SWDE-55 spec=specs/106-Context-Stash.md#REQ-STASH-NEW-016
  // Promise-based mutex: all concurrent callers await the same promise — CREATE TABLE runs once.
  private initPromise: Promise<void> | null = null;

  constructor(private readonly client: PGClientInterface) {}

  private async init(): Promise<void> {
    if (!this.initPromise) {
      this.initPromise = this.client.query(PG_CREATE_TABLE).then(() => {});
    }
    return this.initPromise;
  }

  async read(stashId: string, state?: "suspended" | "closed" | "active"): Promise<StashContent | null> {
    await this.init();
    const result = state
      ? await this.client.query(
          "SELECT state, content FROM stash_entries WHERE stash_id = $1 AND state = $2 LIMIT 1",
          [stashId, state]
        )
      : await this.client.query(
          "SELECT state, content FROM stash_entries WHERE stash_id = $1 ORDER BY CASE WHEN state = 'suspended' THEN 0 ELSE 1 END LIMIT 1",
          [stashId]
        );
    const row = result.rows[0];
    if (!row) return null;
    return { stashId, state: row.state as "suspended" | "closed" | "active", raw: row.content as string };
  }

  async write(stashId: string, content: StashContent): Promise<void> {
    await this.init();
    const { fm } = parseFrontmatter(content.raw);
    const createdAt = fm.created_at ?? new Date().toISOString();
    await this.client.query(
      `INSERT INTO stash_entries (stash_id, state, content, created_at, updated_at)
       VALUES ($1, $2, $3, $4, $5)
       ON CONFLICT (stash_id) DO UPDATE
         SET state = EXCLUDED.state,
             content = EXCLUDED.content,
             updated_at = EXCLUDED.updated_at`,
      [stashId, content.state, content.raw, createdAt, new Date().toISOString()]
    );
  }

  async moveToClose(stashId: string, closedContent: StashContent): Promise<{ warning?: string }> {
    await this.init();
    const result = await this.client.query(
      "UPDATE stash_entries SET state = 'closed', content = $2, updated_at = $3 WHERE stash_id = $1 AND state = 'suspended'",
      [stashId, closedContent.raw, new Date().toISOString()]
    );
    if ((result.rowCount ?? 0) === 0) {
      // Either not found or already closed — try upsert
      await this.write(stashId, closedContent);
    }
    return {};
  }

  async list(filter?: StashFilter): Promise<StashSummary[]> {
    await this.init();
    const conditions: string[] = [];
    const params: unknown[] = [];
    if (filter?.state) { params.push(filter.state); conditions.push(`state = $${params.length}`); }
    const where = conditions.length > 0 ? `WHERE ${conditions.join(" AND ")}` : "";
    const result = await this.client.query(
      `SELECT stash_id, state, content, created_at FROM stash_entries ${where} ORDER BY created_at DESC`,
      params
    );
    const summaries: StashSummary[] = [];
    for (const row of result.rows) {
      const raw = row.content as string;
      const { fm } = parseFrontmatter(raw);
      const s: StashSummary = {
        stash_id: row.stash_id as string,
        name: fm.name ?? (row.stash_id as string),
        state: row.state as "suspended" | "closed" | "active",
        tags: fm.tags ?? [],
        created_at: row.created_at as string,
        last_agent: fm.last_agent,
        resume_hint: fm.resume_hint,
        content: raw,
        filePath: `pg:${row.stash_id}`,
      };
      if (filter?.tag && !s.tags.includes(filter.tag)) continue;
      if (filter?.agent && s.last_agent !== filter.agent) continue;
      summaries.push(s);
    }
    return summaries;
  }

  async delete(stashId: string, state?: "suspended" | "closed" | "active"): Promise<void> {
    await this.init();
    if (state) {
      await this.client.query(
        "DELETE FROM stash_entries WHERE stash_id = $1 AND state = $2",
        [stashId, state]
      );
    } else {
      await this.client.query("DELETE FROM stash_entries WHERE stash_id = $1", [stashId]);
    }
  }

  async exists(stashId: string, state?: "suspended" | "closed" | "active"): Promise<boolean> {
    await this.init();
    const result = state
      ? await this.client.query(
          "SELECT 1 FROM stash_entries WHERE stash_id = $1 AND state = $2 LIMIT 1",
          [stashId, state]
        )
      : await this.client.query(
          "SELECT 1 FROM stash_entries WHERE stash_id = $1 LIMIT 1",
          [stashId]
        );
    return result.rows.length > 0;
  }

  async append(stashId: string, entry: StashEntry): Promise<{ warning?: string }> {
    await this.init();
    const result = await this.client.query(
      "SELECT content FROM stash_entries WHERE stash_id = $1 AND state = 'active' LIMIT 1",
      [stashId]
    );
    if (result.rows.length === 0) {
      throw new Error(`No active stash '${stashId}'. Use stash.enter to activate a stash first.`);
    }
    const currentContent = result.rows[0].content as string;
    const entryYaml = buildEntryYaml(entry);
    const newContent = currentContent + entryYaml;
    await this.client.query(
      "UPDATE stash_entries SET content = $2, updated_at = $3 WHERE stash_id = $1 AND state = 'active'",
      [stashId, newContent, new Date().toISOString()]
    );
    return {};
  }
}

// ─────────────────────────────────────────────────────────────────────────────
// S3 Backend (REQ-STASH-NEW-015)
// axiom:trace work_item=SWDE-55 spec=specs/106-Context-Stash.md#REQ-STASH-NEW-015
// ─────────────────────────────────────────────────────────────────────────────

// axiom:trace work_item=SWDE-55 spec=specs/106-Context-Stash.md#REQ-STASH-NEW-015 plan=phase-4/wave-3
export interface S3ClientInterface {
  getObject(bucket: string, key: string): Promise<{ body: string; etag?: string; metadata?: Record<string, string> } | null>;
  putObject(bucket: string, key: string, body: string, options?: { ifMatch?: string; ifNoneMatch?: string; metadata?: Record<string, string> }): Promise<{ etag: string }>;
  deleteObject(bucket: string, key: string): Promise<void>;
  listObjects(bucket: string, prefix: string): Promise<Array<{ key: string; etag?: string; metadata?: Record<string, string> }>>;
  headObject(bucket: string, key: string): Promise<{ etag: string } | null>;
}

export interface S3BackendConfig {
  bucket: string;
  prefix: string;
  region?: string;
}

/**
 * REQ-STASH-NEW-015: S3 backend for Context Stash.
 * Uses ETag-based conditional PutObject for optimistic locking (ADR-STASH-S3-LOCK).
 * Uses injected S3ClientInterface for testability.
 * axiom:trace work_item=SWDE-55 spec=specs/106-Context-Stash.md#REQ-STASH-NEW-015
 */
export class S3Backend implements StashStorageBackend {
  private readonly bucket: string;
  private readonly prefix: string;

  constructor(config: S3BackendConfig, private readonly client: S3ClientInterface) {
    this.bucket = config.bucket;
    this.prefix = config.prefix.endsWith("/") ? config.prefix : `${config.prefix}/`;
  }

  private key(state: "suspended" | "closed" | "active", stashId: string): string {
    const ext = state === "active" ? ".yaml" : ".md";
    return `${this.prefix}${state}/${stashId}${ext}`;
  }

  async read(stashId: string, state?: "suspended" | "closed" | "active"): Promise<StashContent | null> {
    const states = state ? [state] : (["suspended", "closed", "active"] as const);
    for (const s of states) {
      const obj = await this.client.getObject(this.bucket, this.key(s, stashId));
      if (obj !== null) return { stashId, state: s, raw: obj.body };
    }
    return null;
  }

  async write(stashId: string, content: StashContent): Promise<void> {
    // axiom:trace work_item=SWDE-55 spec=specs/106-Context-Stash.md#REQ-STASH-NEW-015 plan=fix-swde55-etag-retry
    // Retry up to 10 times on 412 PreconditionFailed (concurrent writer).
    // headObject is fetched at the START of each attempt so a concurrent writer's
    // update is always visible — fixes the stale-ETag bug where all retries used
    // the same ETag fetched once before the loop (ADR-STASH-S3-LOCK Phase 4 fix).
    for (let attempt = 0; attempt < 10; attempt++) {
      // Fetch fresh ETag on every attempt — not just the first — so concurrent
      // updates between retries are picked up correctly.
      const existing = await this.client.headObject(this.bucket, this.key(content.state, stashId));
      const options = existing?.etag
        ? { ifMatch: existing.etag }      // update existing: must match current version
        : { ifNoneMatch: "*" };           // new object: must not already exist
      try {
        // Phase 4 (ADR-STASH-S3-LIST-PERF): store frontmatter fields as S3 metadata
        // so list() can read summaries in O(1) without fetching object bodies.
        // axiom:trace work_item=SWDE-55 spec=specs/106-Context-Stash.md#REQ-STASH-NEW-015 plan=phase-4/wave-3
        const { fm: fmMeta } = parseFrontmatter(content.raw);
        await this.client.putObject(
          this.bucket,
          this.key(content.state, stashId),
          content.raw,
          {
            ...options,
            metadata: {
              stash_id: fmMeta.stash_id ?? stashId,
              name: (fmMeta.name ?? stashId).slice(0, 256),
              state: content.state,
              created_at: fmMeta.created_at ?? "",
              tags: (fmMeta.tags ?? []).join(",").slice(0, 256),
              last_agent: (fmMeta.last_agent ?? "").slice(0, 128),
              resume_hint: (fmMeta.resume_hint ?? "").slice(0, 256),
            },
          }
        );
        return;
      } catch (err: unknown) {
        const e = err as { code?: string; statusCode?: number };
        if (e.code === "PreconditionFailed" || e.statusCode === 412) {
          if (attempt < 9) {
            await new Promise((r) => setTimeout(r, 100));
          }
          continue;
        }
        throw err;
      }
    }
    throw new Error(`S3Backend.write: failed after 10 retries (concurrent write conflict on ${this.key(content.state, stashId)})`);
  }

  async moveToClose(stashId: string, closedContent: StashContent): Promise<{ warning?: string }> {
    // Write closed/ object first (no precondition — new key)
    await this.client.putObject(this.bucket, this.key("closed", stashId), closedContent.raw);
    // Then delete suspended/ object
    try {
      await this.client.deleteObject(this.bucket, this.key("suspended", stashId));
    } catch (delErr) {
      return {
        warning: `S3 stash closed but suspended object could not be deleted: ${
          delErr instanceof Error ? delErr.message : String(delErr)
        }. The closed/ object is authoritative.`,
      };
    }
    return {};
  }

  // Phase 4 (ADR-STASH-S3-LIST-PERF): metadata-first list() — O(1) for Phase 4+ objects.
  // Falls back to GetObject for legacy objects that lack metadata tags.
  // axiom:trace work_item=SWDE-55 spec=specs/106-Context-Stash.md#REQ-STASH-NEW-015 plan=phase-4/wave-3
  async list(filter?: StashFilter): Promise<StashSummary[]> {
    const states: Array<"suspended" | "closed" | "active"> = filter?.state
      ? [filter.state]
      : ["suspended", "closed", "active"];
    const summaries: StashSummary[] = [];

    for (const state of states) {
      const ext = state === "active" ? ".yaml" : ".md";
      const objects = await this.client.listObjects(this.bucket, `${this.prefix}${state}/`);
      for (const obj of objects) {
        if (!obj.key.endsWith(ext)) continue;

        // Phase 4: use metadata tags if available (O(1) path — no GetObject needed)
        // Fall back to GetObject for objects written before Phase 4 (O(N) legacy path)
        let stashId: string;
        let name: string;
        let tags: string[];
        let created_at: string;
        let last_agent: string | undefined;
        let resume_hint: string | undefined;
        let rawContent: string | null = null;

        if (obj.metadata?.stash_id) {
          // Fast path: use metadata (Phase 4+ objects)
          stashId = obj.metadata.stash_id;
          name = obj.metadata.name ?? stashId;
          tags = obj.metadata.tags ? obj.metadata.tags.split(",").filter(Boolean) : [];
          created_at = obj.metadata.created_at ?? "";
          last_agent = obj.metadata.last_agent || undefined;
          resume_hint = obj.metadata.resume_hint || undefined;
        } else {
          // Slow path: fetch object content (Phase 3 legacy objects without metadata)
          const result = await this.client.getObject(this.bucket, obj.key);
          if (!result) continue;
          rawContent = result.body;
          const { fm } = parseFrontmatter(result.body);
          if (!fm.stash_id) continue;
          stashId = fm.stash_id;
          name = fm.name ?? stashId;
          tags = fm.tags ?? [];
          created_at = fm.created_at ?? "";
          last_agent = fm.last_agent;
          resume_hint = fm.resume_hint;
        }

        const s: StashSummary = {
          stash_id: stashId,
          name,
          state,
          tags,
          created_at,
          last_agent,
          resume_hint,
          content: rawContent ?? `(metadata-only — stash_id: ${stashId})`,
          filePath: obj.key,
        };

        if (filter?.tag && !s.tags.includes(filter.tag)) continue;
        if (filter?.agent && s.last_agent !== filter.agent) continue;
        summaries.push(s);
      }
    }

    return summaries.sort((a, b) => b.created_at.localeCompare(a.created_at));
  }

  async delete(stashId: string, state?: "suspended" | "closed" | "active"): Promise<void> {
    const states = state ? [state] : (["suspended", "closed", "active"] as const);
    for (const s of states) {
      const exists = await this.client.headObject(this.bucket, this.key(s, stashId));
      if (exists) {
        await this.client.deleteObject(this.bucket, this.key(s, stashId));
        return;
      }
    }
  }

  async exists(stashId: string, state?: "suspended" | "closed" | "active"): Promise<boolean> {
    const states = state ? [state] : (["suspended", "closed", "active"] as const);
    for (const s of states) {
      if (await this.client.headObject(this.bucket, this.key(s, stashId))) return true;
    }
    return false;
  }

  async append(stashId: string, entry: StashEntry): Promise<{ warning?: string }> {
    const key = this.key("active", stashId);
    // ETag-based read-modify-write (same pattern as write() — ADR-STASH-S3-LOCK)
    for (let attempt = 0; attempt < 10; attempt++) {
      const existing = await this.client.getObject(this.bucket, key);
      if (!existing) {
        throw new Error(`No active stash '${stashId}'. Use stash.enter to activate a stash first.`);
      }
      const entryYaml = buildEntryYaml(entry);
      const newContent = existing.body + entryYaml;
      try {
        await this.client.putObject(this.bucket, key, newContent, {
          ifMatch: existing.etag,
        });
        return {};
      } catch (err: unknown) {
        const e = err as { code?: string; statusCode?: number };
        if (e.code === "PreconditionFailed" || e.statusCode === 412) {
          if (attempt < 9) await new Promise((r) => setTimeout(r, 100));
          continue;
        }
        throw err;
      }
    }
    throw new Error(`S3Backend.append: failed after 10 retries (${key})`);
  }
}

// ─────────────────────────────────────────────────────────────────────────────
// StashClientFactory type (REQ-STASH-NEW-017 Phase 4 — clientFactory)
// axiom:trace work_item=SWDE-55 spec=specs/106-Context-Stash.md#REQ-STASH-NEW-017
// ─────────────────────────────────────────────────────────────────────────────

/**
 * REQ-STASH-NEW-017 Phase 4: Factory function that creates an S3 or Postgres
 * client on demand. Enables STASH_BACKEND env-var activation without manual
 * backendOverride injection.
 * axiom:trace work_item=SWDE-55 spec=specs/106-Context-Stash.md#REQ-STASH-NEW-017
 */
export type StashClientFactory = (
  backendType: "s3" | "postgres",
  config: { bucket?: string; prefix?: string; region?: string; dsn?: string }
) => S3ClientInterface | PGClientInterface | null;

// ─────────────────────────────────────────────────────────────────────────────
// FallbackBackend decorator (REQ-STASH-NEW-017 — runtime fallback)
// axiom:trace work_item=SWDE-55 spec=specs/106-Context-Stash.md#REQ-STASH-NEW-017
// ─────────────────────────────────────────────────────────────────────────────

/**
 * REQ-STASH-NEW-017: Wraps any StashStorageBackend and catches runtime errors,
 * falling back to a LocalFileBackend transparently. The warning MUST NOT contain
 * credentials — only the backend class name and error code.
 */
export class FallbackBackend implements StashStorageBackend {
  private readonly fallback: LocalFileBackend;

  constructor(
    private readonly primary: StashStorageBackend,
    fallbackStorageRoot: string
  ) {
    this.fallback = new LocalFileBackend(fallbackStorageRoot);
  }

  async read(stashId: string, state?: "suspended" | "closed" | "active"): Promise<StashContent | null> {
    try { return await this.primary.read(stashId, state); }
    catch { return this.fallback.read(stashId, state); }
  }

  async write(stashId: string, content: StashContent): Promise<void> {
    try { return await this.primary.write(stashId, content); }
    catch (err) {
      console.warn(`[ContextStash] Backend write error — falling back. Error: ${err instanceof Error ? err.message.slice(0, 80) : String(err).slice(0, 80)}`);
      return this.fallback.write(stashId, content);
    }
  }

  async moveToClose(stashId: string, closedContent: StashContent): Promise<{ warning?: string }> {
    try { return await this.primary.moveToClose(stashId, closedContent); }
    catch (err) {
      console.warn(`[ContextStash] Backend moveToClose error — falling back. Error: ${err instanceof Error ? err.message.slice(0, 80) : String(err).slice(0, 80)}`);
      return this.fallback.moveToClose(stashId, closedContent);
    }
  }

  async list(filter?: StashFilter): Promise<StashSummary[]> {
    try { return await this.primary.list(filter); }
    catch { return this.fallback.list(filter); }
  }

  async delete(stashId: string, state?: "suspended" | "closed" | "active"): Promise<void> {
    try { return await this.primary.delete(stashId, state); }
    catch { return this.fallback.delete(stashId, state); }
  }

  async exists(stashId: string, state?: "suspended" | "closed" | "active"): Promise<boolean> {
    try { return await this.primary.exists(stashId, state); }
    catch { return this.fallback.exists(stashId, state); }
  }

  /**
   * Delegates append to the primary backend. If primary.append throws (or primary
   * does not implement append), falls back to the local fallback backend.
   *
   * Note: `this.fallback.append?.()` uses optional chaining defensively.
   * LocalFileBackend.append is always implemented, but future backends that omit
   * the optional append() method per StashStorageBackend interface contract will
   * be handled gracefully (no-op rather than throw).
   *
   * axiom:trace work_item=SWDE-55 spec=specs/106-Context-Stash.md#REQ-STASH-NEW-013 plan=phase-5-followup/bl-r10-2-fallback-jsdoc
   */
  async append(stashId: string, entry: StashEntry): Promise<{ warning?: string }> {
    // axiom:trace work_item=SWDE-55 spec=specs/106-Context-Stash.md#REQ-STASH-NEW-013 plan=phase-4/fix-swde55-append-interface
    // axiom:trace work_item=SWDE-59 spec=specs/106-Context-Stash.md#REQ-STASH-NEW-017 plan=phase-6-post-verify-run3/add-fallbackbackend-append-warning
    try {
      if (this.primary.append) {
        return await this.primary.append(stashId, entry);
      }
    } catch (err) {
      console.warn(`[context-stash] Backend append error — falling back. Error: ${err instanceof Error ? err.message.slice(0, 80) : String(err).slice(0, 80)}`);
      await this.fallback.append?.(stashId, entry);
      return { warning: "primary backend failed on append, fell back to local storage" };
    }
    await this.fallback.append?.(stashId, entry);
    return {};
  }
}

/**
 * REQ-STASH-NEW-017: Backend selection and fallback.
 * Phase 4: clientFactory enables env-var activation without backendOverride injection.
 * axiom:trace work_item=SWDE-55 spec=specs/106-Context-Stash.md#REQ-STASH-NEW-017
 */
function createDefaultBackend(
  storageRoot: string,
  clientFactory?: StashClientFactory
): StashStorageBackend {
  const backendType = process.env.STASH_BACKEND ?? "local";

  if (backendType === "s3") {
    const bucket = process.env.STASH_S3_BUCKET;
    const prefix = process.env.STASH_S3_PREFIX ?? "stash/";
    const region = process.env.STASH_S3_REGION ?? "us-east-1";
    if (bucket && clientFactory) {
      const s3Client = clientFactory("s3", { bucket, prefix, region });
      if (s3Client) {
        console.log(`[ContextStash] Using S3Backend (bucket: ${bucket}, prefix: ${prefix})`);
        return new FallbackBackend(
          new S3Backend({ bucket, prefix, region }, s3Client as S3ClientInterface),
          storageRoot
        );
      }
    }
    // axiom:trace work_item=SWDE-59 spec=specs/106-Context-Stash.md#REQ-STASH-NEW-017 plan=phase-5/fix-clientfactory-production-path
    throw new Error(
      `STASH_BACKEND=${backendType} requires a clientFactory. ` +
      `Pass clientFactory as a separate parameter to ContextStashPlugin(), or set STASH_BACKEND=local. ` +
      `See specs/106-Context-Stash.md §16 REQ-STASH-NEW-017.`
    );
  } else if (backendType === "postgres") {
    const dsn = process.env.STASH_PG_DSN;
    if (dsn && clientFactory) {
      const pgClient = clientFactory("postgres", { dsn });
      if (pgClient) {
        console.log(`[ContextStash] Using PostgresBackend`);
        return new FallbackBackend(
          new PostgresBackend(pgClient as PGClientInterface),
          storageRoot
        );
      }
    }
    // axiom:trace work_item=SWDE-59 spec=specs/106-Context-Stash.md#REQ-STASH-NEW-017 plan=phase-5/fix-clientfactory-production-path
    throw new Error(
      `STASH_BACKEND=${backendType} requires a clientFactory. ` +
      `Pass clientFactory as a separate parameter to ContextStashPlugin(), or set STASH_BACKEND=local. ` +
      `See specs/106-Context-Stash.md §16 REQ-STASH-NEW-017.`
    );
  } else if (backendType !== "local") {
    console.error(
      `[ContextStash] Unknown STASH_BACKEND='${backendType}' — falling back to local file storage.`
    );
  }
  return new LocalFileBackend(storageRoot);
}

// ─────────────────────────────────────────────────────────────────────────────
// Plugin export
//
// OpenCode's getLegacyPlugins iterates Object.values(module) and calls each
// exported function as a plugin factory. The factory receives { client, directory }
// and returns a hooks object with { tool: { [toolName]: tool({...}) } }.
//
// PLACEMENT NOTE: ContextStashPlugin is declared here (end of file) so that
// FallbackBackend, S3Backend, PostgresBackend, and LocalFileBackend class
// references are all in scope (no temporal dead zone errors).
//
// axiom:trace work_item=SWDE-44 spec=specs/106-Context-Stash.md#3.1 plan=phase-0/task-0.1/step-0.1.1
// axiom:trace work_item=SWDE-55 spec=specs/106-Context-Stash.md#REQ-STASH-NEW-017 plan=phase-4/wave-2
// ─────────────────────────────────────────────────────────────────────────────

export const ContextStashPlugin = async ({
  directory,
  client: _client,
  backendOverride,
  clientFactory,
}: {
  directory: string;
  client: unknown;
  backendOverride?: StashStorageBackend;
  clientFactory?: StashClientFactory;
}) => {
  // Security: guard against empty directory (resolve("") = process.cwd() bypasses traversal check)
  // axiom:trace work_item=plugin-config-management-01 spec=specs/112-Plugin-Config-Management.md#8 plan=phase-4/task-4-4/step-4-4-3
  if (!directory || !directory.trim()) {
    throw new Error("[ContextStash] directory must be non-empty");
  }
  const repoRoot = directory;

  // §8: Load stash config via loadPluginConfig (context-stash-config-adoption).
  // axiom:trace work_item=context-stash-config-adoption spec=specs/112-Plugin-Config-Management.md#8 plan=phase-2/task-2-1/step-cs-02
  const stashConfig = loadPluginConfig("context-stash", DEFAULT_STASH_CONFIG, repoRoot) as StashConfig;

  // Option C migration warning: if .axiom/axiom.config.yaml has a non-empty stash: section
  // AND .opencode/config/context-stash.json does NOT exist, warn the user to migrate.
  // Note: this reads axiom.config.yaml independently from the removed loadStashConfig().
  // There is no double-read in normal use; the migration warning path is the only YAML reader here.
  // axiom:trace work_item=context-stash-config-adoption spec=specs/112-Plugin-Config-Management.md#8 plan=phase-2/task-2-1/step-cs-02
  const yamlConfigPath = join(repoRoot, ".axiom", "axiom.config.yaml");
  const jsonConfigPath = join(repoRoot, ".opencode", "config", "context-stash.json");
  if (existsSync(yamlConfigPath) && !existsSync(jsonConfigPath)) {
    try {
      const raw = readFileSync(yamlConfigPath, "utf-8");
      const parsed = yamlParse(raw) as Record<string, unknown>;
      if (parsed.stash && Object.keys(parsed.stash as object).length > 0) {
        const oldSettings = redactCredentials(JSON.stringify(parsed.stash as Record<string, unknown>, null, 2));
        console.warn(
          "[ContextStash] MIGRATION REQUIRED: Your stash config in .axiom/axiom.config.yaml " +
          "is no longer read. Create .opencode/config/context-stash.json with your settings instead. " +
          "See specs/112-Plugin-Config-Management.md §12 for migration instructions. " +
          `Old YAML settings: ${oldSettings}. ` +
          "Until migrated, default values are in effect."
        );
      }
    } catch { /* ignore — if we can't read the YAML, no warning needed */ }
  }
  const storageRoot = join(repoRoot, stashConfig.storage_path);

  // Security: guard storage_path against path traversal (REQ-STASH-NEW-002 boundary extension)
  // axiom:trace work_item=plugin-config-management-01 spec=specs/112-Plugin-Config-Management.md#8 plan=phase-4/task-4-4/step-4-4-3
  const resolvedStorageRoot = resolve(storageRoot);
  const resolvedRepoRoot = resolve(repoRoot);
  if (
    resolvedStorageRoot !== resolvedRepoRoot &&
    !resolvedStorageRoot.startsWith(resolvedRepoRoot + "/")
  ) {
    throw new Error(
      `[ContextStash] storage_path '${stashConfig.storage_path}' resolves outside repo root — path traversal rejected`
    );
  }

  // Use injected backend (wrapped in FallbackBackend for automatic local fallback),
  // or create default backend (env-var selected, with optional clientFactory for Phase 4).
  // REQ-STASH-NEW-017: axiom:trace work_item=SWDE-55 spec=specs/106-Context-Stash.md#REQ-STASH-NEW-017
  const backend: StashStorageBackend = backendOverride
    ? new FallbackBackend(backendOverride, storageRoot)
    : createDefaultBackend(storageRoot, clientFactory);

  console.log(`[ContextStash] Initialized — storage: ${storageRoot}`);

  // ─── Auto-logging state ───────────────────────────────────────────────────
  // Tracks which stash is the primary active context for auto-logging.
  // Updated by stash.enter and stash.exit.
  // axiom:trace work_item=SWDE-55 spec=specs/106-Context-Stash.md#REQ-STASH-031
  const autoLogState = {
    primaryStashId: null as string | null,
    logLevel: "decisions" as "all" | "decisions" | "summaries" | "off",
    toolCallCount: 0, // REQ-STASH-NEW-012: background stash polling counter
  };

  // Advisory lock registry — in-memory, TTL enforced (REQ-STASH-065, REQ-STASH-NEW-008)
  // NOTE: In-memory only — this advisory lock is NOT enforced across separate OpenCode
  // sessions or processes. Two agents in different sessions will each have their own
  // lockRegistry and will not see each other's locks. For cross-process enforcement,
  // use a shared lock store (e.g., filesystem lock file, Redis) in Phase 4.
  // This is sufficient for single-session use cases (compaction, exclusive writes within one session).
  // axiom:trace work_item=SWDE-55 spec=specs/106-Context-Stash.md#REQ-STASH-065
  const lockRegistry: Map<string, { owner: string; acquired: number; ttl: number; refreshTimer?: ReturnType<typeof setInterval> }> = new Map();

  return {
    tool: {
      // ──────────────────── stash.push ──────────────────────────────────────
      // REQ-STASH-001: Save current context to a new suspended stash.
      // axiom:trace work_item=SWDE-44 spec=specs/106-Context-Stash.md#REQ-STASH-001
      "stash.push": tool({
        description:
          "Save the current working context to a new suspended stash. " +
          "Like 'git stash' — park your current thought and come back later. " +
          "Returns { stash_id, name, state, file, message }.",
        args: {
          name: tool.schema
            .string()
            .describe(
              "Human-readable name (e.g., 'investigate auth bypass')"
            ),
          summary: tool.schema
            .string()
            .describe("What this context is about (1-3 sentences)"),
          tags: tool.schema
            .string()
            .optional()
            .describe("Comma-separated tags (e.g., 'security,auth')"),
          detail: tool.schema
            .string()
            .optional()
            .describe("Full preserved context (structured or prose)"),
          resume_hint: tool.schema
            .string()
            .optional()
            .describe("What to do when returning to this stash"),
        },
        async execute({ name, summary, tags, detail, resume_hint }, context) {
          try {
            // Guard: required params may arrive as undefined when called outside OpenCode schema validation
            // axiom:trace work_item=plugin-bug-sweep-01 spec=specs/106-Context-Stash.md#REQ-STASH-001 plan=phase-1/step-2
            if (!name) {
              return JSON.stringify({ error: "name is required" });
            }
            if (!summary) {
              return JSON.stringify({ error: "summary is required — provide a 1-3 sentence description of this context" });
            }
            const stashId = slugify(name);
            validateStashId(stashId);
            const ctx = context as { agent?: string; sessionID?: string };
            const agent = ctx?.agent ?? "unknown-agent";
            const sessionId = ctx?.sessionID ?? `local-${Date.now()}`;
            const now = new Date().toISOString();
            const tagList = tags
              ? tags.split(",").map((t) => t.trim()).filter((t) => t)
              : [];
            const fm: StashFrontmatter & { session_id: string } = {
              stash_id: stashId, name, state: "suspended", created_by: agent,
              created_at: now, suspended_at: now, session_id: sessionId,
              tags: tagList, entries: 0, last_agent: agent, resume_hint,
            };
            const content = buildSuspendedMarkdown(fm, summary, detail, resume_hint);
            // REQ-STASH-001: guard against silent overwrite
            if (await backend.exists(stashId, "suspended")) {
              return JSON.stringify({
                error: `Stash '${stashId}' already exists. Use stash.drop to delete it first, stash.pop to resume it, or choose a different name.`,
              });
            }
            // Enforce active_size_limit_kb (0 = unlimited)
            // axiom:trace work_item=plugin-config-management-01 spec=specs/112-Plugin-Config-Management.md#8 plan=phase-4/task-4-4/step-4-4-3
            if (stashConfig.active_size_limit_kb > 0) {
              const contentBytes = Buffer.byteLength(content, "utf8");
              const limitBytes = stashConfig.active_size_limit_kb * 1024;
              if (contentBytes > limitBytes) {
                return JSON.stringify({
                  error: `Stash content exceeds active_size_limit_kb (${contentBytes} bytes > ${limitBytes} bytes / ${stashConfig.active_size_limit_kb} KB). Reduce the size of 'summary', 'detail', or 'resume_hint'.`,
                });
              }
            }
            await backend.write(stashId, { stashId, state: "suspended", raw: content });
            logLifecycleEvent({ type: "stash.created", stash_id: stashId, state: "suspended", agent });
            return JSON.stringify({
              stash_id: stashId, name, state: "suspended",
              file: `suspended/${stashId}.md`,
              message: `Stash '${stashId}' created. Resume with: stash.pop --id ${stashId}`,
            });
          } catch (err) {
            return JSON.stringify({ error: err instanceof Error ? err.message : String(err) });
          }
        },
      }),

      // ──────────────────── stash.pop ────────────────────────────────────────
      // REQ-STASH-002: Resume most recent (or named) stash; remove from suspended.
      // axiom:trace work_item=SWDE-44 spec=specs/106-Context-Stash.md#REQ-STASH-002
      "stash.pop": tool({
        description:
          "Resume the most recent (or named) suspended stash and remove it from the list. " +
          "Like 'git stash pop'. Returns { stash_id, name, state:'popped', summary, resume_hint, ... }.",
        args: {
          id: tool.schema
            .string()
            .optional()
            .describe("Stash ID to pop (default: most recent suspended)"),
        },
        async execute({ id }) {
          try {
            const summaries = await backend.list({ state: "suspended" });
            if (summaries.length === 0) {
              return JSON.stringify({ error: "No suspended stashes found." });
            }
            let target: typeof summaries[0];
            if (id) {
              validateStashId(id);
              const found = summaries.find((s) => s.stash_id === id);
              if (!found) {
                return JSON.stringify({ error: `No suspended stash with ID '${id}'.` });
              }
              target = found;
            } else {
              target = summaries[0]; // most recent (sorted by created_at desc)
            }
            const { fm, body } = parseFrontmatter(target.content);
            await backend.delete(target.stash_id, "suspended");
            return JSON.stringify({
              stash_id: target.stash_id, name: target.name, state: "popped",
              summary: body.trim(), resume_hint: fm.resume_hint,
              tags: target.tags, created_at: target.created_at,
              message: `Stash '${target.stash_id}' resumed and removed. Context injected above.`,
            });
          } catch (err) {
            return JSON.stringify({ error: err instanceof Error ? err.message : String(err) });
          }
        },
      }),

      // ──────────────────── stash.apply ──────────────────────────────────────
      // REQ-STASH-003: Resume a stash WITHOUT removing it.
      // axiom:trace work_item=SWDE-44 spec=specs/106-Context-Stash.md#REQ-STASH-003
      "stash.apply": tool({
        description:
          "Resume a stash WITHOUT removing it from the list. " +
          "Like 'git stash apply'. State unchanged. Use stash.pop to remove after applying.",
        args: {
          id: tool.schema.string().describe("Stash ID to apply"),
        },
        async execute({ id }) {
          try {
            validateStashId(id);
            const content = await backend.read(id);
            if (!content) {
              return JSON.stringify({
                error: `No stash with ID '${id}' found (use stash.list to see available stashes).`,
              });
            }
            const { fm, body } = parseFrontmatter(content.raw);
            return JSON.stringify({
              stash_id: id, name: fm.name, state: fm.state,
              summary: body.trim(), resume_hint: fm.resume_hint,
              tags: fm.tags, created_at: fm.created_at,
              message: `Stash '${id}' context loaded (not removed). Use stash.pop to remove.`,
            });
          } catch (err) {
            return JSON.stringify({ error: err instanceof Error ? err.message : String(err) });
          }
        },
      }),

      // ──────────────────── stash.peek ───────────────────────────────────────
      // REQ-STASH-005: Show stash summary without activating or modifying state.
      // axiom:trace work_item=SWDE-44 spec=specs/106-Context-Stash.md#REQ-STASH-005
      "stash.peek": tool({
        description:
          "Show the summary and metadata of a stash without activating or modifying state. " +
          "Works on both suspended and closed stashes.",
        args: {
          id: tool.schema.string().describe("Stash ID to peek at"),
        },
        async execute({ id }) {
          try {
            validateStashId(id);
            const content = await backend.read(id);
            if (!content) {
              return JSON.stringify({ error: `No stash with ID '${id}' found.` });
            }
            const { fm, body } = parseFrontmatter(content.raw);
            return JSON.stringify({
              stash_id: id, name: fm.name, state: fm.state,
              tags: fm.tags, created_at: fm.created_at,
              suspended_at: fm.suspended_at, last_agent: fm.last_agent,
              resume_hint: fm.resume_hint,
              summary_preview: body.trim().slice(0, 500),
              message: "Peek complete — no state changes made.",
            });
          } catch (err) {
            return JSON.stringify({ error: err instanceof Error ? err.message : String(err) });
          }
        },
      }),

      // ──────────────────── stash.list ───────────────────────────────────────
      // REQ-STASH-004: List all stashes with state, name, tags, age, last agent.
      // axiom:trace work_item=SWDE-44 spec=specs/106-Context-Stash.md#REQ-STASH-004
      "stash.list": tool({
        description:
          "List all stashes with state, name, tags, age, and last agent. " +
          "Supports filtering by state, tag, and agent.",
        args: {
          state: tool.schema
            .string()
            .optional()
            .describe("Filter by state: suspended | closed"),
          tag: tool.schema
            .string()
            .optional()
            .describe("Filter by tag (exact match)"),
          agent: tool.schema
            .string()
            .optional()
            .describe("Filter by creating/last agent"),
        },
        async execute({ state, tag, agent }, context) {
          try {
            const filter: StashFilter = {};
            if (state) filter.state = state as "suspended" | "closed";
            if (tag) filter.tag = tag;
            if (agent) filter.agent = agent;
            const summaries = await backend.list(filter);
            const items = summaries.map((s) => {
              // Parse background flag from content frontmatter (not in StashSummary base fields)
              const { fm: sfm } = parseFrontmatter(s.content ?? "");
              const isBackground = (sfm as any).background === true;
              return {
                stash_id: s.stash_id, name: s.name, state: s.state,
                tags: s.tags, age: formatAge(s.created_at),
                created_at: s.created_at, last_agent: s.last_agent, resume_hint: s.resume_hint,
                // Phase 4 Phase 2: background context tracking
                background: isBackground,
                is_primary: s.state === "active" && !isBackground,
              };
            });

            // REQ-STASH-063: surface stashes with handoff entries addressed to this agent
            const ctxAgent = (context as { agent?: string })?.agent;
            if (ctxAgent) {
              for (const item of items as any[]) {
                if (item.state !== "active") continue;
                const content = await backend.read(item.stash_id, "active" as any);
                if (!content) continue;
                const { body } = parseFrontmatter(content.raw);
                const rawEntries = body.trim() ? body.trim().split(/\n(?=- ts:)/).filter(Boolean) : [];
                for (const raw of rawEntries) {
                  const entry = parseStashEntry(raw);
                  if (entry?.type === "handoff" && (entry as any).to_agent === ctxAgent) {
                    (item as any).has_handoff_for_me = true;
                    break;
                  }
                }
              }
              // Sort: stashes with handoff for me first
              (items as any[]).sort((a: any, b: any) => (b.has_handoff_for_me ? 1 : 0) - (a.has_handoff_for_me ? 1 : 0));
            }

            return JSON.stringify({ count: items.length, stashes: items });
          } catch (err) {
            return JSON.stringify({ error: err instanceof Error ? err.message : String(err) });
          }
        },
      }),

      // ──────────────────── stash.drop ───────────────────────────────────────
      // REQ-STASH-006: Delete a stash permanently.
      // axiom:trace work_item=SWDE-44 spec=specs/106-Context-Stash.md#REQ-STASH-006
      "stash.drop": tool({
        description: "Delete a stash permanently. This cannot be undone.",
        args: {
          id: tool.schema.string().describe("Stash ID to delete"),
        },
        async execute({ id }) {
          try {
            validateStashId(id);
            if (!(await backend.exists(id))) {
              return JSON.stringify({ error: `No stash with ID '${id}' found.` });
            }
            await backend.delete(id);
            logLifecycleEvent({ type: "stash.dropped", stash_id: id });
            return JSON.stringify({
              stash_id: id, state: "dropped",
              message: `Stash '${id}' deleted permanently.`,
            });
          } catch (err) {
            return JSON.stringify({ error: err instanceof Error ? err.message : String(err) });
          }
        },
      }),

      // ──────────────────── stash.close ──────────────────────────────────────
      // REQ-STASH-007: Mark stash completed; move to closed/ with outcome.
      // Uses atomic write (REQ-STASH-NEW-005).
      // axiom:trace work_item=SWDE-44 spec=specs/106-Context-Stash.md#REQ-STASH-007
      "stash.close": tool({
        description:
          "Mark a stash as completed and move it to closed/ with an outcome recorded. " +
          "Uses atomic write-then-rename to prevent partial state on crash.",
        args: {
          id: tool.schema.string().describe("Stash ID to close"),
          outcome: tool.schema
            .string()
            .optional()
            .describe(
              "What was accomplished (e.g., 'Fixed in commit abc123')"
            ),
        },
        async execute({ id, outcome }, context) {
          try {
            validateStashId(id);
            const suspended = await backend.read(id, "suspended");
            if (!suspended) {
              return JSON.stringify({ error: `No suspended stash with ID '${id}'.` });
            }
            const { fm, body } = parseFrontmatter(suspended.raw);
            const ctx = context as { agent?: string; sessionID?: string };
            const agent = ctx?.agent ?? "unknown-agent";
            const now = new Date().toISOString();
            const closedFm: StashFrontmatter & { session_id: string } = {
              stash_id: id, name: fm.name ?? id, state: "closed",
              created_by: fm.created_by ?? "unknown", created_at: fm.created_at ?? now,
              closed_at: now, session_id: fm.session_id ?? "",
              tags: fm.tags ?? [], last_agent: agent, outcome,
            };
            const closedContent = buildSuspendedMarkdown(closedFm, body.trim(), undefined, undefined);
            const { warning } = await backend.moveToClose(id, { stashId: id, state: "closed", raw: closedContent });
            logLifecycleEvent({ type: "stash.closed", stash_id: id, state: "closed", agent });
            if (warning) {
              return JSON.stringify({
                stash_id: id, state: "closed", outcome, file: `closed/${id}.md`, warning,
              });
            }
            return JSON.stringify({
              stash_id: id, state: "closed", outcome, file: `closed/${id}.md`,
              message: `Stash '${id}' closed.`,
            });
          } catch (err) {
            return JSON.stringify({ error: err instanceof Error ? err.message : String(err) });
          }
        },
      }),

      // ──────────────────── stash.create ─────────────────────────────────────
      // REQ-STASH-012: Create an empty stash without pushing current context.
      // axiom:trace work_item=SWDE-44 spec=specs/106-Context-Stash.md#REQ-STASH-012
      "stash.create": tool({
        description:
          "Create an empty stash without pushing current context. " +
          "Useful for starting a fresh investigation or queuing a task.",
        args: {
          name: tool.schema
            .string()
            .describe("Human-readable name for this stash"),
          tags: tool.schema
            .string()
            .optional()
            .describe("Comma-separated tags"),
          summary: tool.schema
            .string()
            .optional()
            .describe("Initial summary (optional — defaults to empty)"),
        },
        async execute({ name, tags, summary }, context) {
          try {
            const stashId = slugify(name);
            validateStashId(stashId);
            const ctx = context as { agent?: string; sessionID?: string };
            const agent = ctx?.agent ?? "unknown-agent";
            const sessionId = ctx?.sessionID ?? `local-${Date.now()}`;
            const now = new Date().toISOString();
            const tagList = tags
              ? tags.split(",").map((t) => t.trim()).filter((t) => t)
              : [];
            const fm: StashFrontmatter & { session_id: string } = {
              stash_id: stashId, name, state: "suspended", created_by: agent,
              created_at: now, suspended_at: now, session_id: sessionId,
              tags: tagList, entries: 0, last_agent: agent,
            };
            const content = buildSuspendedMarkdown(fm, summary ?? "(empty stash)", undefined, undefined);
            if (await backend.exists(stashId, "suspended")) {
              return JSON.stringify({
                error: `Stash '${stashId}' already exists. Use stash.drop to delete it first, stash.pop to resume it, or choose a different name.`,
              });
            }
            await backend.write(stashId, { stashId, state: "suspended", raw: content });
            return JSON.stringify({
              stash_id: stashId, name, state: "suspended",
              file: `suspended/${stashId}.md`,
              message: `Empty stash '${stashId}' created.`,
            });
          } catch (err) {
            return JSON.stringify({ error: err instanceof Error ? err.message : String(err) });
          }
        },
      }),

      // ──────────────────── stash.enter ──────────────────────────────────────
      // Enter (activate) a stash for active work. Transitions suspended→active.
      // axiom:trace work_item=SWDE-55 spec=specs/106-Context-Stash.md#2.3
      "stash.enter": tool({
        description:
          "Enter a stash to start active work. Transitions it from suspended to active state, " +
          "creating a YAML append-log at active/{id}.yaml. If no ID given and no suspended stash " +
          "exists, creates a fresh active stash. Returns { stash_id, state, name, resume_hint }.",
        args: {
          id: tool.schema
            .string()
            .optional()
            .describe("Stash ID to enter (default: most recent suspended, or create new)"),
          name: tool.schema
            .string()
            .optional()
            .describe("Name for new stash (required if creating fresh; ignored if entering existing)"),
          summary: tool.schema
            .string()
            .optional()
            .describe("Initial summary if creating a new active stash"),
          log_level: tool.schema
            .enum(["all", "decisions", "summaries", "off"])
            .optional()
            .describe("Auto-log level: all | decisions | summaries | off (default: decisions)"),
          background: tool.schema
            .boolean()
            .optional()
            .describe("Run as background context — receives periodic summaries but is not primary (REQ-STASH-037)"),
        },
        async execute({ id, name, summary, log_level, background }, context) {
          try {
            const ctx = context as { agent?: string; sessionID?: string };
            const agent = ctx?.agent ?? "unknown-agent";
            const sessionId = ctx?.sessionID ?? `local-${Date.now()}`;
            const now = new Date().toISOString();

            let stashId: string;
            let stashName: string;
            let tags: string[] = [];
            let resumeHint: string | undefined;
            let existingEntries = "";

            if (id) {
              validateStashId(id);
              // Check not already active FIRST (before reading suspended)
              if (await backend.exists(id, "active")) {
                return JSON.stringify({ error: `Stash '${id}' is already active. Use stash.append to add entries or stash.exit to close.` });
              }
              // Enter an existing suspended stash
              const suspended = await backend.read(id, "suspended");
              if (!suspended) {
                return JSON.stringify({ error: `No suspended stash '${id}'. Use stash.list to see available stashes.` });
              }
              const { fm, body } = parseFrontmatter(suspended.raw);
              stashId = id;
              stashName = fm.name ?? id;
              tags = fm.tags ?? [];
              resumeHint = fm.resume_hint;
              // Restore log_level from previous session (F2 fix — REQ-STASH-031)
              const restoredLogLevel = (fm as any).log_level as typeof activeFm.log_level | undefined;
              // Preserve the summary as the first entry in the log
              if (body.trim()) {
                existingEntries = buildEntryYaml({
                  ts: now,
                  agent,
                  type: "summary",
                  content: `[Context restored from suspended state] ${body.trim()}`,
                });
              }
              // Delete suspended file
              await backend.delete(id, "suspended");

              // Build active YAML — restoring log_level from suspended frontmatter
              const activeFm: StashFrontmatter & { session_id: string; entered_at: string } = {
                stash_id: stashId, name: stashName, state: "active",
                created_by: agent, created_at: now, entered_at: now,
                session_id: sessionId, tags, last_agent: agent, resume_hint: resumeHint,
                log_level: (log_level ?? restoredLogLevel ?? "decisions") as StashFrontmatter["log_level"],
                background: background ?? false,
              };
              const activeYaml = buildActiveYaml(activeFm) + existingEntries;
              await backend.write(stashId, { stashId, state: "active", raw: activeYaml });

              if (!(background ?? false)) {
                autoLogState.primaryStashId = stashId;
                autoLogState.logLevel = (activeFm.log_level ?? "decisions") as typeof autoLogState.logLevel;
              }

              logLifecycleEvent({ type: "stash.activated", stash_id: stashId, state: "active", agent });
              return JSON.stringify({
                stash_id: stashId, name: stashName, state: "active",
                file: `active/${stashId}.yaml`,
                resume_hint: resumeHint,
                message: `Stash '${stashId}' is now active. Use stash.append to log entries, stash.exit to suspend.`,
              });
            } else {
              // Create new active stash or enter most recent suspended
              const suspended = await backend.list({ state: "suspended" });
              if (suspended.length > 0 && !name) {
                // Enter most recent suspended stash
                const target = suspended[0];
                const content = await backend.read(target.stash_id, "suspended");
                if (!content) return JSON.stringify({ error: "Failed to read suspended stash." });
                const { fm, body } = parseFrontmatter(content.raw);
                stashId = target.stash_id;
                stashName = fm.name ?? stashId;
                tags = fm.tags ?? [];
                resumeHint = fm.resume_hint;
                // Restore log_level from previous session (F2 fix — REQ-STASH-031)
                const restoredLogLevelNoId = (fm as any).log_level as StashFrontmatter["log_level"] | undefined;
                if (body.trim()) {
                  existingEntries = buildEntryYaml({
                    ts: now, agent, type: "summary",
                    content: `[Context restored] ${body.trim()}`,
                  });
                }
                await backend.delete(stashId, "suspended");

                // Build active YAML — restoring log_level from suspended frontmatter
                const activeFmNoId: StashFrontmatter & { session_id: string; entered_at: string } = {
                  stash_id: stashId, name: stashName, state: "active",
                  created_by: agent, created_at: now, entered_at: now,
                  session_id: sessionId, tags, last_agent: agent, resume_hint: resumeHint,
                  log_level: (log_level ?? restoredLogLevelNoId ?? "decisions") as StashFrontmatter["log_level"],
                  background: background ?? false,
                };
                const activeYamlNoId = buildActiveYaml(activeFmNoId) + existingEntries;
                await backend.write(stashId, { stashId, state: "active", raw: activeYamlNoId });

                if (!(background ?? false)) {
                  autoLogState.primaryStashId = stashId;
                  autoLogState.logLevel = (activeFmNoId.log_level ?? "decisions") as typeof autoLogState.logLevel;
                }

                logLifecycleEvent({ type: "stash.activated", stash_id: stashId, state: "active", agent });
                return JSON.stringify({
                  stash_id: stashId, name: stashName, state: "active",
                  file: `active/${stashId}.yaml`,
                  resume_hint: resumeHint,
                  message: `Stash '${stashId}' is now active. Use stash.append to log entries, stash.exit to suspend.`,
                });
              } else {
                // Create fresh active stash
                if (!name) return JSON.stringify({ error: "Provide id (to enter existing) or name (to create new active stash)." });
                stashId = slugify(name);
                validateStashId(stashId);
                stashName = name;
                if (summary) {
                  existingEntries = buildEntryYaml({ ts: now, agent, type: "summary", content: summary });
                }
              }
            }

            // Build active YAML
            const activeFm: StashFrontmatter & { session_id: string; entered_at: string } = {
              stash_id: stashId, name: stashName, state: "active",
              created_by: agent, created_at: now, entered_at: now,
              session_id: sessionId, tags, last_agent: agent, resume_hint: resumeHint,
              log_level: (log_level ?? "decisions") as StashFrontmatter["log_level"],
              background: background ?? false,
            };
            const activeYaml = buildActiveYaml(activeFm) + existingEntries;

            await backend.write(stashId, { stashId, state: "active", raw: activeYaml });

            // REQ-STASH-037 / REQ-STASH-031: update auto-log state for the primary stash
            if (!(background ?? false)) {
              autoLogState.primaryStashId = stashId;
              autoLogState.logLevel = (log_level ?? "decisions") as typeof autoLogState.logLevel;
            }

            logLifecycleEvent({ type: "stash.activated", stash_id: stashId, state: "active", agent });
            return JSON.stringify({
              stash_id: stashId, name: stashName, state: "active",
              file: `active/${stashId}.yaml`,
              resume_hint: resumeHint,
              message: `Stash '${stashId}' is now active. Use stash.append to log entries, stash.exit to suspend.`,
            });
          } catch (err) {
            return JSON.stringify({ error: err instanceof Error ? err.message : String(err) });
          }
        },
      }),

      // ──────────────────── stash.exit ───────────────────────────────────────
      // Exit (deactivate) an active stash. Transitions active→suspended with a summary.
      // axiom:trace work_item=SWDE-55 spec=specs/106-Context-Stash.md#2.3
      "stash.exit": tool({
        description:
          "Exit an active stash, transitioning it back to suspended state. " +
          "All log entries are preserved in the body; resume_hint helps future agents pick up. " +
          "Returns { stash_id, state, entries_count, file }.",
        args: {
          id: tool.schema.string().describe("Stash ID to exit"),
          resume_hint: tool.schema
            .string()
            .optional()
            .describe("What to do when returning to this stash"),
          outcome_summary: tool.schema
            .string()
            .optional()
            .describe("Brief summary of what was accomplished in this session"),
        },
        async execute({ id, resume_hint, outcome_summary }, context) {
          try {
            validateStashId(id);
            const ctx = context as { agent?: string; sessionID?: string };
            const agent = ctx?.agent ?? "unknown-agent";
            const now = new Date().toISOString();

            const activeContent = await backend.read(id, "active");
            if (!activeContent) {
              return JSON.stringify({ error: `No active stash '${id}'. Use stash.list to see active stashes.` });
            }

            // Parse frontmatter and count entries
            const { fm, body } = parseFrontmatter(activeContent.raw);
            const entries = body.trim().split(/\n(?=- ts:)/).filter(Boolean);
            const entriesCount = entries.length;

            // Build suspended markdown with the log as body
            const suspendedFm: StashFrontmatter & { session_id: string } = {
              stash_id: id,
              name: fm.name ?? id,
              state: "suspended",
              created_by: fm.created_by ?? agent,
              created_at: fm.created_at ?? now,
              suspended_at: now,
              session_id: (fm as any).session_id ?? "",
              tags: fm.tags ?? [],
              entries: entriesCount,
              last_agent: agent,
              resume_hint: resume_hint ?? fm.resume_hint,
              // Preserve log_level across exit/enter cycles (F2 fix — REQ-STASH-031)
              log_level: (fm as any).log_level ?? "decisions",
            };

            const bodyContent = [
              outcome_summary ? `## Session Summary\n${outcome_summary}\n` : "",
              entriesCount > 0 ? `## Log Entries (${entriesCount})\n\`\`\`yaml${body}\`\`\`` : "",
            ].filter(Boolean).join("\n");

            const suspendedMarkdown = buildSuspendedMarkdown(
              suspendedFm,
              outcome_summary ?? `Active stash with ${entriesCount} log entries.`,
              bodyContent || undefined,
              resume_hint
            );

            // Atomic transition: write suspended, delete active
            await backend.write(id, { stashId: id, state: "suspended", raw: suspendedMarkdown });
            await backend.delete(id, "active");

            // REQ-STASH-031: clear auto-log state when primary stash exits
            if (autoLogState.primaryStashId === id) {
              autoLogState.primaryStashId = null;
            }

            logLifecycleEvent({ type: "stash.suspended", stash_id: id, state: "suspended", agent });
            return JSON.stringify({
              stash_id: id, state: "suspended",
              entries_count: entriesCount,
              file: `suspended/${id}.md`,
              resume_hint: resume_hint ?? fm.resume_hint,
              message: `Stash '${id}' suspended with ${entriesCount} log entries. Resume with: stash.enter --id ${id}`,
            });
          } catch (err) {
            return JSON.stringify({ error: err instanceof Error ? err.message : String(err) });
          }
        },
      }),

      // ──────────────────── stash.append ─────────────────────────────────────
      // Manually append a log entry to an active stash.
      // axiom:trace work_item=SWDE-55 spec=specs/106-Context-Stash.md#REQ-STASH-NEW-013
      "stash.append": tool({
        description:
          "Append a log entry to an active stash. " +
          "The stash must be active (use stash.enter first). " +
          "Returns { stash_id, entry_type, ts, warning? }. " +
          "If warning is present, the primary backend failed and entries were written to local fallback storage — surface this to the user.",
        args: {
          id: tool.schema.string().describe("Stash ID to append to"),
          type: tool.schema
            .enum(["observation", "decision", "tool_call", "finding", "summary", "handoff", "question", "blocker"])
            .describe("Entry type"),
          content: tool.schema.string().describe("Entry content"),
          refs: tool.schema
            .string()
            .optional()
            .describe("Comma-separated references (files, specs, etc.)"),
          severity: tool.schema
            .enum(["info", "warn", "error", "critical"])
            .optional()
            .describe("Severity (for finding/decision types)"),
          to_agent: tool.schema
            .string()
            .optional()
            .describe("Target agent (for handoff entries — REQ-STASH-062)"),
        },
        async execute({ id, type, content, refs, severity, to_agent }, context) {
          try {
            validateStashId(id);
            const ctx = context as { agent?: string; sessionID?: string };
            const agent = ctx?.agent ?? "unknown-agent";
            const sessionId = ctx?.sessionID;
            const now = new Date().toISOString();

            const entry: StashEntry = {
              ts: now, agent, type,
              content,
              refs: refs ? refs.split(",").map((r) => r.trim()).filter((r) => r) : undefined,
              severity: severity as StashEntry["severity"],
              ...(to_agent ? { to_agent } : {}),
              // REQ-STASH-101: include session_id when available and not default local
              ...(sessionId && !sessionId.startsWith("local-") ? { session_id: sessionId } : {}),
            };

            // axiom:trace work_item=SWDE-59 spec=specs/106-Context-Stash.md#REQ-STASH-NEW-017 plan=phase-7-post-verify-run4/wire-append-warning-to-tool-layer
            const appendResult = await backend.append?.(id, entry);

            // REQ-STASH-094: check size after append, trigger auto-compaction if over limit
            try {
              const afterContent = await backend.read(id, "active" as any);
              if (afterContent && stashConfig.compaction.auto) {
                const sizeKb = afterContent.raw.length / 1024;
                if (sizeKb > stashConfig.active_size_limit_kb) {
                  // Announce compaction first (REQ-STASH-044)
                  await backend.append?.(id, {
                    ts: new Date().toISOString(), agent: "auto-compact",
                    type: "summary",
                    content: `[Auto-compaction triggered: stash size ${sizeKb.toFixed(1)}KB exceeds ${stashConfig.active_size_limit_kb}KB limit]`,
                  });
                  // Compact using config keep_types and keep_recent
                  const compactContent = await backend.read(id, "active" as any);
                  if (compactContent) {
                    const { fm: cfm, body: cbody } = parseFrontmatter(compactContent.raw);
                    const rawEntries2 = cbody.trim() ? cbody.trim().split(/\n(?=- ts:)/).filter(Boolean) : [];
                    const keepTypes2 = stashConfig.compaction.keep_types;
                    const keepLast2 = stashConfig.compaction.keep_recent;
                    const keepSet2 = new Set<number>();
                    rawEntries2.forEach((_, i2) => {
                      const e2 = parseStashEntry(rawEntries2[i2]);
                      if (e2 && keepTypes2.includes(String(e2.type))) keepSet2.add(i2);
                      if (e2?.refs && Array.isArray(e2.refs) && e2.refs.length > 0) keepSet2.add(i2);
                    });
                    for (let k = rawEntries2.length - keepLast2; k < rawEntries2.length; k++) { if (k >= 0) keepSet2.add(k); }
                    const keptEntries = rawEntries2.filter((_, i2) => keepSet2.has(i2)).join("\n");
                    const autoActiveFm: StashFrontmatter & { session_id: string; entered_at: string } = {
                      stash_id: cfm.stash_id ?? id, name: cfm.name ?? id, state: "active" as any,
                      created_by: cfm.created_by ?? "auto-compact", created_at: cfm.created_at ?? now,
                      entered_at: (cfm as any).entered_at ?? now, session_id: cfm.session_id ?? "",
                      tags: cfm.tags ?? [], last_agent: cfm.last_agent,
                    };
                    const newContent2 = buildActiveYaml(autoActiveFm) + keptEntries;
                    await backend.write(id, { stashId: id, state: "active" as any, raw: newContent2 });
                  }
                }
              }
            } catch { /* never crash append on size check */ }

            return JSON.stringify({
              stash_id: id, entry_type: type, ts: now, agent,
              ...(appendResult?.warning ? { warning: appendResult.warning } : {}),
              message: `Entry appended to active stash '${id}'.`,
            });
          } catch (err) {
            return JSON.stringify({ error: err instanceof Error ? err.message : String(err) });
          }
        },
      }),

      // ──────────────────── stash.migrate ────────────────────────────────────      // axiom:trace work_item=SWDE-55 spec=specs/106-Context-Stash.md#REQ-STASH-NEW-013
      "stash.migrate": tool({
        description:
          "Migrate all stashes from local file storage to the configured remote backend " +
          "(S3 or PostgreSQL). Idempotent — skips stashes already present in the target. " +
          "Use dry_run=true to preview without writing. Returns { migrated, skipped, errors }.",
        args: {
          dry_run: tool.schema
            .boolean()
            .optional()
            .describe("Preview migration without writing (default: false)"),
          state_filter: tool.schema
            .string()
            .optional()
            .describe("Only migrate stashes in this state: suspended | closed (default: both)"),
        },
        async execute({ dry_run = false, state_filter }) {
          try {
            // Source: always LocalFileBackend (reads from .memory-bank/stash/)
            const source = new LocalFileBackend(storageRoot);
            const filter: StashFilter = {};
            if (state_filter) filter.state = state_filter as "suspended" | "closed";

            const stashes = await source.list(filter);
            if (stashes.length === 0) {
              return JSON.stringify({
                migrated: 0, skipped: 0, errors: 0,
                message: "No stashes found in local storage.",
              });
            }

            const results = { migrated: 0, skipped: 0, errors: 0, details: [] as string[] };

            for (const stash of stashes) {
              try {
                // Check if already in target backend (idempotency)
                const exists = await backend.exists(stash.stash_id, stash.state);
                if (exists) {
                  results.skipped++;
                  results.details.push(`SKIP: ${stash.stash_id} (already in target)`);
                  continue;
                }

                if (!dry_run) {
                  await backend.write(stash.stash_id, {
                    stashId: stash.stash_id,
                    state: stash.state,
                    raw: stash.content,
                  });
                }

                results.migrated++;
                results.details.push(`${dry_run ? "WOULD MIGRATE" : "MIGRATED"}: ${stash.stash_id} (${stash.state})`);
              } catch (err) {
                results.errors++;
                results.details.push(`ERROR: ${stash.stash_id} — ${err instanceof Error ? err.message : String(err)}`);
              }
            }

            return JSON.stringify({
              migrated: results.migrated,
              skipped: results.skipped,
              errors: results.errors,
              dry_run,
              total_found: stashes.length,
              details: results.details,
              message: dry_run
                ? `Dry run: would migrate ${results.migrated}, skip ${results.skipped}.`
                : `Migration complete: migrated ${results.migrated}, skipped ${results.skipped}, errors ${results.errors}.`,
            });
          } catch (err) {
            return JSON.stringify({
              error: err instanceof Error ? err.message : String(err),
            });
          }
        },
      }),

      // ──────────────────── stash.log ────────────────────────────────────────
      // Read and filter entries from an active stash's YAML log (REQ-STASH-024)
      // axiom:trace work_item=SWDE-55 spec=specs/106-Context-Stash.md#REQ-STASH-024
      "stash.log": tool({
        description:
          "Show recent entries from an active stash's YAML log. " +
          "Supports filtering by last N entries, type, agent, and severity. " +
          "Returns { stash_id, entries, total_entries }.",
        args: {
          id: tool.schema.string().describe("Stash ID to read log from"),
          last: tool.schema
            .number()
            .optional()
            .describe("Show only the last N entries (default: all)"),
          type: tool.schema
            .string()
            .optional()
            .describe("Filter by entry type (observation | decision | finding | etc.)"),
          agent: tool.schema
            .string()
            .optional()
            .describe("Filter by agent name"),
          since: tool.schema
            .string()
            .optional()
            .describe("Show entries since this time (ISO or relative like '2h ago')"),
        },
        async execute({ id, last, type: typeFilter, agent: agentFilter }) {
          try {
            validateStashId(id);
            const content = await backend.read(id, "active" as any);
            if (!content) {
              return JSON.stringify({
                error: `No active stash '${id}'. Use stash.list to see active stashes.`,
              });
            }

            const { fm, body } = parseFrontmatter(content.raw);

            // Parse YAML entries from the body
            const rawEntries = body.trim()
              ? body.trim().split(/\n(?=- ts:)/).filter(Boolean)
              : [];

            // Parse each entry into a structured object
            const entries: Array<Record<string, unknown>> = [];
            for (const raw of rawEntries) {
              try {
                const parsed = parseStashEntry(raw);
                if (parsed) entries.push(parsed);
              } catch {
                // skip malformed entries
              }
            }

            // Apply filters
            let filtered = entries;
            if (typeFilter) filtered = filtered.filter((e) => e.type === typeFilter);
            if (agentFilter) filtered = filtered.filter((e) => e.agent === agentFilter);
            if (last !== undefined && last > 0) {
              filtered = filtered.slice(-last);
            }

            return JSON.stringify({
              stash_id: id,
              name: fm.name,
              state: "active",
              total_entries: entries.length,
              shown: filtered.length,
              entries: filtered,
            });
          } catch (err) {
            return JSON.stringify({ error: err instanceof Error ? err.message : String(err) });
          }
        },
      }),

      // ──────────────────── stash.tag ────────────────────────────────────────
      // Add or remove tags from a stash (REQ-STASH-026)
      // axiom:trace work_item=SWDE-55 spec=specs/106-Context-Stash.md#REQ-STASH-026
      "stash.tag": tool({
        description:
          "Add or remove tags from a stash. Works on suspended, closed, and active stashes. " +
          "Returns { stash_id, tags, added, removed }.",
        args: {
          id: tool.schema.string().describe("Stash ID to tag"),
          add: tool.schema
            .string()
            .optional()
            .describe("Comma-separated tags to add"),
          remove: tool.schema
            .string()
            .optional()
            .describe("Comma-separated tags to remove"),
        },
        async execute({ id, add, remove }) {
          try {
            validateStashId(id);
            if (!add && !remove) {
              return JSON.stringify({ error: "Provide --add or --remove (or both)." });
            }

            // Find the stash in any state
            const content = await backend.read(id);
            if (!content) {
              return JSON.stringify({ error: `No stash with ID '${id}' found.` });
            }

            const { fm, body } = parseFrontmatter(content.raw);
            const currentTags: string[] = fm.tags ?? [];

            const toAdd = add ? add.split(",").map((t) => t.trim()).filter((t) => t) : [];
            const toRemove = remove ? remove.split(",").map((t) => t.trim()).filter((t) => t) : [];

            const newTags = [...new Set([...currentTags, ...toAdd])].filter(
              (t) => !toRemove.includes(t)
            );

            // Rebuild the raw content with updated tags
            let updatedRaw: string;
            if (content.state === ("active" as any)) {
              const activeFm: StashFrontmatter & { session_id: string; entered_at: string } = {
                stash_id: fm.stash_id ?? id,
                name: fm.name ?? id,
                state: "active" as any,
                created_by: fm.created_by ?? "unknown-agent",
                created_at: fm.created_at ?? new Date().toISOString(),
                entered_at: (fm as any).entered_at ?? new Date().toISOString(),
                session_id: fm.session_id ?? "",
                tags: newTags,
                last_agent: fm.last_agent,
                resume_hint: fm.resume_hint,
              };
              updatedRaw = buildActiveYaml(activeFm) + body;
            } else {
              const updatedFm: StashFrontmatter & { session_id: string } = {
                stash_id: fm.stash_id ?? id,
                name: fm.name ?? id,
                state: content.state as "suspended" | "closed",
                created_by: fm.created_by ?? "unknown-agent",
                created_at: fm.created_at ?? new Date().toISOString(),
                suspended_at: fm.suspended_at,
                closed_at: fm.closed_at,
                session_id: fm.session_id ?? "",
                tags: newTags,
                entries: fm.entries,
                last_agent: fm.last_agent,
                resume_hint: fm.resume_hint,
                outcome: fm.outcome,
              };
              updatedRaw = buildSuspendedMarkdown(
                updatedFm,
                body.trim() || "(stash)",
                undefined,
                fm.resume_hint
              );
            }

            await backend.write(id, { stashId: id, state: content.state, raw: updatedRaw });

            return JSON.stringify({
              stash_id: id,
              state: content.state,
              tags: newTags,
              added: toAdd.filter((t) => !currentTags.includes(t)),
              removed: toRemove.filter((t) => currentTags.includes(t)),
              message: `Tags updated for stash '${id}'.`,
            });
          } catch (err) {
            return JSON.stringify({ error: err instanceof Error ? err.message : String(err) });
          }
        },
      }),

      // ──────────────────── stash.switch ─────────────────────────────────────
      // Exit the current active stash and enter another (REQ-STASH-033)
      // axiom:trace work_item=SWDE-55 spec=specs/106-Context-Stash.md#REQ-STASH-033
      "stash.switch": tool({
        description:
          "Exit the current active stash (suspend it) and enter another. " +
          "Returns { exited, entered }.",
        args: {
          to: tool.schema.string().describe("Stash ID to switch to"),
          resume_hint: tool.schema.string().optional().describe("Resume hint for the stash being exited"),
          outcome_summary: tool.schema.string().optional().describe("Summary of what was done in the exited stash"),
        },
        async execute({ to, resume_hint, outcome_summary }, context) {
          try {
            validateStashId(to);
            const ctx = context as { agent?: string; sessionID?: string };
            const agent = ctx?.agent ?? "unknown-agent";
            const now = new Date().toISOString();

            // Find and exit current active stash(es)
            const activeStashes = await backend.list({ state: "active" as any });
            let exited: string | null = null;

            for (const active of activeStashes) {
              if (active.stash_id === to) continue; // don't exit the target
              const activeContent = await backend.read(active.stash_id, "active" as any);
              if (!activeContent) continue;

              const { fm, body } = parseFrontmatter(activeContent.raw);
              const entries = body.trim() ? body.trim().split(/\n(?=- ts:)/).filter(Boolean) : [];

              const suspendedFm: StashFrontmatter & { session_id: string } = {
                stash_id: active.stash_id,
                name: fm.name ?? active.stash_id,
                state: "suspended",
                created_by: fm.created_by ?? agent,
                created_at: fm.created_at ?? now,
                suspended_at: now,
                session_id: fm.session_id ?? "",
                tags: fm.tags ?? [],
                entries: entries.length,
                last_agent: agent,
                resume_hint: resume_hint ?? fm.resume_hint,
              };

              const bodyContent = outcome_summary
                ? `## Session Summary\n${outcome_summary}\n\n${entries.length > 0 ? `## Log Entries (${entries.length})\n\`\`\`yaml${body}\`\`\`` : ""}`
                : entries.length > 0
                ? `## Log Entries (${entries.length})\n\`\`\`yaml${body}\`\`\``
                : "";

              const suspendedMarkdown = buildSuspendedMarkdown(
                suspendedFm,
                outcome_summary ?? `Active session with ${entries.length} entries.`,
                bodyContent || undefined,
                resume_hint
              );

              await backend.write(active.stash_id, { stashId: active.stash_id, state: "suspended", raw: suspendedMarkdown });
              await backend.delete(active.stash_id, "active" as any);
              exited = active.stash_id;
              break; // only exit the first active stash
            }

            // Now enter target stash
            const sessionId = ctx?.sessionID ?? `local-${Date.now()}`;
            let stashName: string;
            let tags: string[] = [];
            let targetResumeHint: string | undefined;
            let existingEntries = "";

            // Check if target is already active
            if (await backend.exists(to, "active" as any)) {
              return JSON.stringify({
                exited,
                entered: to,
                state: "active",
                message: exited
                  ? `Switched from '${exited}' to '${to}' (already active).`
                  : `Stash '${to}' is already active.`,
              });
            }

            const targetSuspended = await backend.read(to, "suspended");
            if (targetSuspended) {
              const { fm: targetFm, body: targetBody } = parseFrontmatter(targetSuspended.raw);
              stashName = targetFm.name ?? to;
              tags = targetFm.tags ?? [];
              targetResumeHint = targetFm.resume_hint;
              if (targetBody.trim()) {
                existingEntries = buildEntryYaml({
                  ts: now, agent, type: "summary",
                  content: `[Context restored] ${targetBody.trim()}`,
                });
              }
              await backend.delete(to, "suspended");
            } else {
              return JSON.stringify({ error: `No stash '${to}' found. Use stash.push to create it first.` });
            }

            const activeFm: StashFrontmatter & { session_id: string; entered_at: string } = {
              stash_id: to, name: stashName, state: "active" as any,
              created_by: agent, created_at: now, entered_at: now,
              session_id: sessionId, tags, last_agent: agent, resume_hint: targetResumeHint,
            };
            await backend.write(to, { stashId: to, state: "active" as any, raw: buildActiveYaml(activeFm) + existingEntries });

            return JSON.stringify({
              exited,
              entered: to,
              state: "active",
              resume_hint: targetResumeHint,
              message: exited ? `Switched from '${exited}' to '${to}'.` : `Entered '${to}'.`,
            });
          } catch (err) {
            return JSON.stringify({ error: err instanceof Error ? err.message : String(err) });
          }
        },
      }),

      // ──────────────────── stash.compact ────────────────────────────────────
      // Compact an active stash's YAML log (REQ-STASH-040)
      // axiom:trace work_item=SWDE-55 spec=specs/106-Context-Stash.md#REQ-STASH-040
      "stash.compact": tool({
        description:
          "Compact an active stash's YAML log to reduce size. " +
          "Preserves all finding/decision entries, the last 10 entries of any type, " +
          "and all entries with refs (REQ-STASH-041). " +
          "Returns { stash_id, entries_before, entries_after, bytes_saved }.",
        args: {
          id: tool.schema.string().describe("Stash ID to compact"),
          keep_types: tool.schema
            .string()
            .optional()
            .describe("Comma-separated entry types to always keep (default: finding,decision)"),
          keep_last: tool.schema
            .number()
            .optional()
            .describe("Always keep the last N entries (default: 10)"),
        },
        async execute({ id, keep_types, keep_last = 10 }) {
          try {
            validateStashId(id);
            const content = await backend.read(id, "active" as any);
            if (!content) {
              return JSON.stringify({ error: `No active stash '${id}'.` });
            }

            const { fm, body } = parseFrontmatter(content.raw);
            const rawEntries = body.trim() ? body.trim().split(/\n(?=- ts:)/).filter(Boolean) : [];

            if (rawEntries.length === 0) {
              return JSON.stringify({
                stash_id: id, entries_before: 0, entries_after: 0, bytes_saved: 0,
                message: "Nothing to compact.",
              });
            }

            const alwaysKeepTypes = keep_types
              ? keep_types.split(",").map((t) => t.trim())
              : ["finding", "decision"];

            // Parse entries
            const parsed = rawEntries.map((raw) => ({
              raw,
              entry: parseStashEntry(raw),
            }));

            // Determine which entries to keep (REQ-STASH-041)
            const keepSet = new Set<number>();

            // Always keep: finding, decision types
            parsed.forEach(({ entry }, i) => {
              if (entry && alwaysKeepTypes.includes(String(entry.type))) {
                keepSet.add(i);
              }
            });

            // Always keep: entries with refs
            parsed.forEach(({ entry }, i) => {
              if (entry && entry.refs && Array.isArray(entry.refs) && (entry.refs as unknown[]).length > 0) {
                keepSet.add(i);
              }
            });

            // Always keep: last N entries
            const lastN = Math.min(keep_last, parsed.length);
            for (let i = parsed.length - lastN; i < parsed.length; i++) {
              keepSet.add(i);
            }

            const kept = parsed.filter((_, i) => keepSet.has(i));
            const compactedBody = kept.map((e) => e.raw).join("\n");

            // Add a compaction summary entry
            const compactionNote = buildEntryYaml({
              ts: new Date().toISOString(),
              agent: "stash.compact",
              type: "summary",
              content: `Compacted: kept ${kept.length}/${rawEntries.length} entries (${alwaysKeepTypes.join("/")} + last ${lastN}).`,
            });

            const activeFm: StashFrontmatter & { session_id: string; entered_at: string } = {
              stash_id: fm.stash_id ?? id,
              name: fm.name ?? id,
              state: "active" as any,
              created_by: fm.created_by ?? "stash.compact",
              created_at: fm.created_at ?? new Date().toISOString(),
              entered_at: (fm as any).entered_at ?? fm.suspended_at ?? new Date().toISOString(),
              session_id: fm.session_id ?? "",
              tags: fm.tags ?? [],
              last_agent: fm.last_agent,
              resume_hint: fm.resume_hint,
            };

            const newContent = buildActiveYaml(activeFm) + compactedBody + compactionNote;
            const bytesBefore = content.raw.length;
            // REQ-STASH-043: save pre-compact snapshot
            if (stashConfig.compaction.snapshot_before && backend instanceof LocalFileBackend) {
              await saveCompactSnapshot(id, content.raw, (backend as any).storageRoot);
            }
            await backend.write(id, { stashId: id, state: "active" as any, raw: newContent });
            const bytesAfter = newContent.length;

            return JSON.stringify({
              stash_id: id,
              entries_before: rawEntries.length,
              entries_after: kept.length + 1, // +1 for compaction note
              bytes_saved: Math.max(0, bytesBefore - bytesAfter),
              kept_types: alwaysKeepTypes,
              message: `Compacted '${id}': ${rawEntries.length} → ${kept.length + 1} entries.`,
            });
          } catch (err) {
            return JSON.stringify({ error: err instanceof Error ? err.message : String(err) });
          }
        },
      }),

      // ──────────────────── stash.summarize ──────────────────────────────────────
      // Preview what compact would produce without writing (REQ-STASH-045)
      // axiom:trace work_item=SWDE-55 spec=specs/106-Context-Stash.md#REQ-STASH-045
      "stash.summarize": tool({
        description:
          "Preview compaction output without writing. " +
          "Returns what stash.compact would produce, including which entries would be kept/dropped. " +
          "Returns { stash_id, entries_before, entries_after, entries_kept, entries_dropped, preview }.",
        args: {
          id: tool.schema.string().describe("Stash ID to preview compaction for"),
          keep_types: tool.schema.string().optional().describe("Comma-separated types to always keep (default: finding,decision)"),
          keep_last: tool.schema.number().optional().describe("Always keep last N entries (default: 10)"),
        },
        async execute({ id, keep_types, keep_last = 10 }) {
          try {
            validateStashId(id);
            const content = await backend.read(id, "active" as any);
            if (!content) {
              return JSON.stringify({ error: `No active stash '${id}'.` });
            }

            const { body } = parseFrontmatter(content.raw);
            const rawEntries = body.trim() ? body.trim().split(/\n(?=- ts:)/).filter(Boolean) : [];

            if (rawEntries.length === 0) {
              return JSON.stringify({ stash_id: id, entries_before: 0, entries_after: 0, entries_kept: [], entries_dropped: [], preview: "Nothing to compact." });
            }

            const alwaysKeepTypes = keep_types
              ? keep_types.split(",").map((t) => t.trim())
              : ["finding", "decision"];

            const parsed = rawEntries.map((raw) => ({ raw, entry: parseStashEntry(raw) }));
            const keepSet = new Set<number>();

            parsed.forEach(({ entry }, i) => {
              if (entry && alwaysKeepTypes.includes(String(entry.type))) keepSet.add(i);
            });
            parsed.forEach(({ entry }, i) => {
              if (entry && entry.refs && Array.isArray(entry.refs) && entry.refs.length > 0) keepSet.add(i);
            });
            const lastN = Math.min(keep_last, parsed.length);
            for (let i = parsed.length - lastN; i < parsed.length; i++) keepSet.add(i);

            const kept = parsed.filter((_, i) => keepSet.has(i));
            const dropped = parsed.filter((_, i) => !keepSet.has(i));

            return JSON.stringify({
              stash_id: id,
              entries_before: rawEntries.length,
              entries_after: kept.length + 1, // +1 for compaction note
              entries_kept: kept.map((e) => ({ type: e.entry?.type, ts: e.entry?.ts, content: String(e.entry?.content ?? "").slice(0, 100) })),
              entries_dropped: dropped.map((e) => ({ type: e.entry?.type, ts: e.entry?.ts, content: String(e.entry?.content ?? "").slice(0, 60) })),
              kept_types: alwaysKeepTypes,
              preview: `Would compact to ${kept.length + 1} entries (from ${rawEntries.length}). ` +
                `Keeping ${alwaysKeepTypes.join("/")} types, entries with refs, and last ${lastN}. ` +
                `Dropping ${dropped.length} entries.`,
            });
          } catch (err) {
            return JSON.stringify({ error: err instanceof Error ? err.message : String(err) });
          }
        },
      }),

      // ──────────────────── stash.context ────────────────────────────────────
      // Return the active context banner for injection into system prompt (REQ-STASH-034)
      // axiom:trace work_item=SWDE-55 spec=specs/106-Context-Stash.md#REQ-STASH-034
      "stash.context": tool({
        description:
          "Return a context banner showing the current active stash. " +
          "Agents can call this at session start to orient themselves. " +
          "Returns a formatted banner string ready for injection into context.",
        args: {},
        async execute() {
          try {
            const activeStashes = await backend.list({ state: "active" as any });
            if (activeStashes.length === 0) {
              return JSON.stringify({ banner: null, message: "No active stash context." });
            }

            const banners = await Promise.all(
              activeStashes.map(async (s) => {
                const content = await backend.read(s.stash_id, "active" as any);
                const { fm, body } = content ? parseFrontmatter(content.raw) : { fm: {}, body: "" };
                const entries = body.trim() ? body.trim().split(/\n(?=- ts:)/).filter(Boolean) : [];
                const lastEntry = entries[entries.length - 1];
                const lastAgent = lastEntry
                  ? parseStashEntry(lastEntry)?.agent ?? s.last_agent
                  : s.last_agent;
                const age = formatAge(s.created_at);
                const label = (s as any).background ? " [background]" : " [primary]";
                return `[ACTIVE CONTEXT: ${s.stash_id}${label} | ${entries.length} entries | last: ${lastAgent} ${age}]`;
              })
            );

            return JSON.stringify({
              banner: banners.join("\n"),
              active_count: activeStashes.length,
              primary: activeStashes.find((s) => !(s as any).background)?.stash_id ?? null,
            });
          } catch (err) {
            return JSON.stringify({ error: err instanceof Error ? err.message : String(err) });
          }
        },
      }),

      // ──────────────────── stash.search ─────────────────────────────────────
      // REQ-STASH-050: Cross-stash keyword, tag, type, agent search.
      // axiom:trace work_item=SWDE-55 spec=specs/106-Context-Stash.md#REQ-STASH-050
      "stash.search": tool({
        description:
          "Search for entries across all stashes by keyword, tag, type, agent, or time range. " +
          "Searches active YAML logs (entry content) and suspended/closed markdown bodies. " +
          "Returns { results: [{ stash_id, state, ts, type, agent, preview, refs }], total }. " +
          "Phase 1 search: O(N) backend reads for active stashes (one read per active stash). " +
          "For S3 backends with many active stashes, this is O(N) GetObject calls per search. " +
          "See adr-s3-list-perf.md for Phase 4 optimization plan.",
        args: {
          query: tool.schema.string().optional().describe("Keyword to search in entry content"),
          tag: tool.schema.string().optional().describe("Filter by stash tag"),
          type: tool.schema.string().optional().describe("Filter by entry type (finding, decision, etc.)"),
          agent: tool.schema.string().optional().describe("Filter by agent name"),
          state: tool.schema.string().optional().describe("Only search stashes in this state (active|suspended|closed)"),
          limit: tool.schema.number().optional().describe("Max results to return (default: 20)"),
        },
        async execute({ query, tag, type: typeFilter, agent: agentFilter, state: stateFilter, limit = 20 }) {
          try {
            const results: Array<{
              stash_id: string; state: string; name: string; ts?: string;
              type?: string; agent?: string; preview: string; refs?: unknown[];
            }> = [];

            // Get all stashes to search
            const filter: StashFilter = {};
            if (stateFilter) filter.state = stateFilter as "suspended" | "closed" | "active";
            const allStashes = await backend.list(filter as any);

            for (const stash of allStashes) {
              if (results.length >= limit) break;

              // Filter by tag
              if (tag && !stash.tags.includes(tag)) continue;

              if (stash.state === ("active" as any)) {
                // Search active YAML entries
                const content = await backend.read(stash.stash_id, "active" as any);
                if (!content) continue;
                const { body } = parseFrontmatter(content.raw);
                const rawEntries = body.trim() ? body.trim().split(/\n(?=- ts:)/).filter(Boolean) : [];

                for (const raw of rawEntries) {
                  if (results.length >= limit) break;
                  const entry = parseStashEntry(raw);
                  if (!entry) continue;
                  if (typeFilter && entry.type !== typeFilter) continue;
                  if (agentFilter && entry.agent !== agentFilter) continue;
                  const contentStr = String(entry.content ?? "");
                  if (query && !contentStr.toLowerCase().includes(query.toLowerCase())) continue;
                  results.push({
                    stash_id: stash.stash_id, state: "active", name: stash.name,
                    ts: String(entry.ts ?? ""), type: String(entry.type ?? ""),
                    agent: String(entry.agent ?? ""), preview: contentStr.slice(0, 200),
                    refs: Array.isArray(entry.refs) ? entry.refs : undefined,
                  });
                }
              } else {
                // Search suspended/closed markdown body (REQ-STASH-054)
                if (typeFilter || agentFilter) continue; // no per-entry metadata in markdown
                const bodyText = stash.content ?? "";
                if (query && !bodyText.toLowerCase().includes(query.toLowerCase())) continue;
                const preview = bodyText.trim().slice(0, 200);
                if (!preview && query) continue;
                results.push({
                  stash_id: stash.stash_id, state: stash.state, name: stash.name,
                  preview: preview || `[${stash.state} stash: ${stash.name}]`,
                });
              }
            }

            return JSON.stringify({ results, total: results.length });
          } catch (err) {
            return JSON.stringify({ error: err instanceof Error ? err.message : String(err) });
          }
        },
      }),

      // ──────────────────── stash.headers ────────────────────────────────────
      // REQ-STASH-052: Structural overview of a stash.
      // axiom:trace work_item=SWDE-55 spec=specs/106-Context-Stash.md#REQ-STASH-052
      "stash.headers": tool({
        description:
          "Show a structural overview of a stash: entry types, agents involved, tags, timeline. " +
          "Works on active stashes (parses YAML log) and suspended/closed (parses markdown). " +
          "Returns { stash_id, state, name, tags, agents, entry_types, timeline, entry_count, size_bytes }.",
        args: {
          id: tool.schema.string().describe("Stash ID to inspect"),
        },
        async execute({ id }) {
          try {
            validateStashId(id);
            const content = await backend.read(id);
            if (!content) return JSON.stringify({ error: `No stash '${id}'.` });

            const { fm, body } = parseFrontmatter(content.raw);
            const agents = new Set<string>();
            const entryTypes: Record<string, number> = {};
            const timeline: Array<{ ts: string; type: string; agent: string }> = [];

            if (content.state === ("active" as any)) {
              const rawEntries = body.trim() ? body.trim().split(/\n(?=- ts:)/).filter(Boolean) : [];
              for (const raw of rawEntries) {
                const entry = parseStashEntry(raw);
                if (!entry) continue;
                const ag = String(entry.agent ?? "unknown");
                const tp = String(entry.type ?? "unknown");
                agents.add(ag);
                entryTypes[tp] = (entryTypes[tp] ?? 0) + 1;
                timeline.push({ ts: String(entry.ts ?? ""), type: tp, agent: ag });
              }
            }

            if (fm.created_by) agents.add(fm.created_by);
            if (fm.last_agent) agents.add(fm.last_agent);

            return JSON.stringify({
              stash_id: id, state: content.state, name: fm.name ?? id,
              tags: fm.tags ?? [], agents: [...agents],
              entry_types: entryTypes, entry_count: timeline.length,
              timeline: timeline.slice(-20), // last 20 for timeline
              created_at: fm.created_at, last_agent: fm.last_agent,
              resume_hint: fm.resume_hint,
              size_bytes: content.raw.length,
            });
          } catch (err) {
            return JSON.stringify({ error: err instanceof Error ? err.message : String(err) });
          }
        },
      }),

      // ──────────────────── stash.related ────────────────────────────────────
      // REQ-STASH-055: Find related stashes by tag overlap + shared refs.
      // axiom:trace work_item=SWDE-55 spec=specs/106-Context-Stash.md#REQ-STASH-055
      "stash.related": tool({
        description:
          "Find stashes related to the given stash (or current active) by tag overlap and shared refs. " +
          "Phase 1 implementation: tag overlap + shared refs only (no embeddings). " +
          "Returns { related: [{ stash_id, name, score, shared_tags, shared_refs }] }.",
        args: {
          to: tool.schema.string().optional().describe("Stash ID to find related stashes for (default: current active)"),
          limit: tool.schema.number().optional().describe("Max results (default: 5)"),
        },
        async execute({ to, limit = 5 }) {
          try {
            // Resolve target stash
            const targetId = to ?? autoLogState.primaryStashId;
            if (!targetId) return JSON.stringify({ error: "No stash specified and no active stash. Use --to <stash-id>." });

            validateStashId(targetId);
            const targetContent = await backend.read(targetId);
            if (!targetContent) return JSON.stringify({ error: `No stash '${targetId}'.` });

            const { fm: targetFm, body: targetBody } = parseFrontmatter(targetContent.raw);
            const targetTags = new Set(targetFm.tags ?? []);

            // Collect refs from target entries
            const targetRefs = new Set<string>();
            if (targetContent.state === ("active" as any)) {
              const rawEntries = targetBody.trim() ? targetBody.trim().split(/\n(?=- ts:)/).filter(Boolean) : [];
              for (const raw of rawEntries) {
                const entry = parseStashEntry(raw);
                if (entry?.refs && Array.isArray(entry.refs)) {
                  for (const ref of entry.refs) targetRefs.add(String(ref));
                }
              }
            }

            // Score all other stashes
            const allStashes = await backend.list();
            const scored: Array<{ stash_id: string; name: string; score: number; shared_tags: string[]; shared_refs: string[] }> = [];

            for (const stash of allStashes) {
              if (stash.stash_id === targetId) continue;

              const sharedTags = stash.tags.filter((t) => targetTags.has(t));
              const sharedRefs: string[] = [];

              // Check refs in active stash entries
              if (stash.state === ("active" as any)) {
                const sc = await backend.read(stash.stash_id, "active" as any);
                if (sc) {
                  const { body } = parseFrontmatter(sc.raw);
                  const rawEntries = body.trim() ? body.trim().split(/\n(?=- ts:)/).filter(Boolean) : [];
                  for (const raw of rawEntries) {
                    const entry = parseStashEntry(raw);
                    if (entry?.refs && Array.isArray(entry.refs)) {
                      for (const ref of entry.refs) {
                        if (targetRefs.has(String(ref))) sharedRefs.push(String(ref));
                      }
                    }
                  }
                }
              }

              const score = sharedTags.length * 2 + sharedRefs.length;
              if (score > 0) scored.push({ stash_id: stash.stash_id, name: stash.name, score, shared_tags: sharedTags, shared_refs: [...new Set(sharedRefs)] });
            }

            scored.sort((a, b) => b.score - a.score);

            return JSON.stringify({ target: targetId, related: scored.slice(0, limit) });
          } catch (err) {
            return JSON.stringify({ error: err instanceof Error ? err.message : String(err) });
          }
        },
      }),

      // ──────────────────── stash.lock ───────────────────────────────────────
      // REQ-STASH-065: Advisory lock acquisition.
      // axiom:trace work_item=SWDE-55 spec=specs/106-Context-Stash.md#REQ-STASH-065
      "stash.lock": tool({
        description:
          "Acquire an advisory lock on a stash for exclusive operations (e.g., compaction). " +
          "Lock is advisory — does not block reads. TTL defaults to 300s (REQ-STASH-NEW-008). " +
          "Returns { stash_id, owner, acquired_at, ttl_seconds } or { error } if lock held.",
        args: {
          id: tool.schema.string().describe("Stash ID to lock"),
          ttl_seconds: tool.schema.number().optional().describe("Lock TTL in seconds (default: 300)"),
        },
        async execute({ id, ttl_seconds = 300 }, context) {
          try {
            // axiom:trace work_item=SWDE-55 spec=specs/106-Context-Stash.md#REQ-STASH-NEW-008 plan=phase-4-backlog/bl-r9-5
            if (ttl_seconds < 30) {
              return JSON.stringify({ error: `ttl_seconds must be >= 30 (got ${ttl_seconds}). Use LOCK_TTL_MS/1000 (300s) for the spec-compliant default.` });
            }
            validateStashId(id);
            const ctx = context as { agent?: string };
            const owner = ctx?.agent ?? "unknown-agent";
            const now = Date.now();

            // Check existing lock
            const existing = lockRegistry.get(id);
            if (existing) {
              const elapsed = (now - existing.acquired) / 1000;
              if (elapsed < existing.ttl) {
                return JSON.stringify({
                  error: `Stash '${id}' is locked by '${existing.owner}' (${Math.ceil(existing.ttl - elapsed)}s remaining). Retry later.`,
                });
              }
              // Expired lock — clear it
              if (existing.refreshTimer) clearInterval(existing.refreshTimer);
              lockRegistry.delete(id);
            }

            // Acquire lock with auto-refresh every 60s (REQ-STASH-NEW-008)
            const lockEntry = { owner, acquired: now, ttl: ttl_seconds };
            const refreshTimer = setInterval(() => {
              const entry = lockRegistry.get(id);
              if (entry?.owner === owner) {
                entry.acquired = Date.now(); // refresh
              } else {
                clearInterval(refreshTimer);
              }
            }, 60_000);
            lockRegistry.set(id, { ...lockEntry, refreshTimer });

            // Auto-expire after TTL
            setTimeout(() => {
              const entry = lockRegistry.get(id);
              if (entry?.owner === owner) {
                if (entry.refreshTimer) clearInterval(entry.refreshTimer);
                lockRegistry.delete(id);
              }
            }, ttl_seconds * 1000);

            return JSON.stringify({
              stash_id: id, owner, acquired_at: new Date(now).toISOString(),
              ttl_seconds, message: `Lock acquired on '${id}'. Release with stash.unlock when done.`,
            });
          } catch (err) {
            return JSON.stringify({ error: err instanceof Error ? err.message : String(err) });
          }
        },
      }),

      // ──────────────────── stash.unlock ─────────────────────────────────────
      // REQ-STASH-065: Advisory lock release.
      // axiom:trace work_item=SWDE-55 spec=specs/106-Context-Stash.md#REQ-STASH-065
      "stash.unlock": tool({
        description: "Release an advisory lock on a stash. Only the lock owner can release it.",
        args: {
          id: tool.schema.string().describe("Stash ID to unlock"),
        },
        async execute({ id }, context) {
          try {
            validateStashId(id);
            const ctx = context as { agent?: string };
            const agent = ctx?.agent ?? "unknown-agent";
            const entry = lockRegistry.get(id);
            if (!entry) return JSON.stringify({ stash_id: id, message: `No lock held on '${id}'.` });
            if (entry.owner !== agent) {
              return JSON.stringify({ error: `Cannot release lock: held by '${entry.owner}', not '${agent}'.` });
            }
            if (entry.refreshTimer) clearInterval(entry.refreshTimer);
            lockRegistry.delete(id);
            return JSON.stringify({ stash_id: id, message: `Lock on '${id}' released.` });
          } catch (err) {
            return JSON.stringify({ error: err instanceof Error ? err.message : String(err) });
          }
        },
      }),

      // ──────────────────── stash.archive ────────────────────────────────────
      // REQ-STASH-091, REQ-STASH-092: Archive a closed stash.
      // axiom:trace work_item=SWDE-55 spec=specs/106-Context-Stash.md#REQ-STASH-092
      "stash.archive": tool({
        description:
          "Archive a closed stash to .memory-bank/stash/archive/. " +
          "Archived stashes are preserved but not shown in stash.list by default. " +
          "Returns { stash_id, archived_at, archive_path }.",
        args: {
          id: tool.schema.string().describe("Stash ID to archive (must be closed)"),
        },
        async execute({ id }) {
          try {
            validateStashId(id);
            // Only archive closed stashes for now (LocalFileBackend only — remote backends store in content)
            const content = await backend.read(id, "closed");
            if (!content) return JSON.stringify({ error: `No closed stash '${id}'. Only closed stashes can be archived.` });

            const now = new Date().toISOString();
            // LocalFileBackend: move to archive/
            if (backend instanceof LocalFileBackend) {
              const lb = backend as LocalFileBackend;
              const archiveDir = join((lb as any).storageRoot, "archive");
              if (!existsSync(archiveDir)) mkdirSync(archiveDir, { recursive: true });
              const archivePath = safePath((lb as any).storageRoot, "archive", `${id}.md`);
              await atomicWrite(archivePath, content.raw);
              await backend.delete(id, "closed");
            } else {
              // For remote backends: write to archive "state"
              await backend.write(id, { stashId: id, state: "archived" as any, raw: content.raw });
              await backend.delete(id, "closed");
            }

            logLifecycleEvent({ type: "stash.archived", stash_id: id });
            return JSON.stringify({
              stash_id: id, archived_at: now,
              archive_path: `archive/${id}.md`,
              message: `Stash '${id}' archived. Not shown in stash.list by default.`,
            });
          } catch (err) {
            return JSON.stringify({ error: err instanceof Error ? err.message : String(err) });
          }
        },
      }),

      // ──────────────────── stash.cleanup ────────────────────────────────────
      // REQ-STASH-090: TTL-based stash review.
      // axiom:trace work_item=SWDE-55 spec=specs/106-Context-Stash.md#REQ-STASH-090
      "stash.cleanup": tool({
        description:
          "Review and flag stale stashes. Suspended stashes older than 30 days are flagged for review. " +
          "Closed stashes older than 90 days are flagged for archival. " +
          "Returns { stale_suspended, ready_to_archive, message }.",
        args: {
          suspend_ttl_days: tool.schema.number().optional().describe("Days before suspended stash is stale (default: 30)"),
          closed_ttl_days: tool.schema.number().optional().describe("Days before closed stash is ready to archive (default: 90)"),
          dry_run: tool.schema.boolean().optional().describe("Preview only, no changes (default: true)"),
        },
        async execute({ suspend_ttl_days = 30, closed_ttl_days = 90, dry_run = true }) {
          try {
            const now = Date.now();
            const suspendThresholdMs = suspend_ttl_days * 24 * 60 * 60 * 1000;
            const closedThresholdMs = closed_ttl_days * 24 * 60 * 60 * 1000;

            const allStashes = await backend.list();
            const staleSuspended: Array<{ stash_id: string; name: string; age_days: number }> = [];
            const readyToArchive: Array<{ stash_id: string; name: string; age_days: number }> = [];

            for (const stash of allStashes) {
              if (!stash.created_at) continue;
              const age = now - new Date(stash.created_at).getTime();
              if (stash.state === "suspended" && age > suspendThresholdMs) {
                staleSuspended.push({ stash_id: stash.stash_id, name: stash.name, age_days: Math.floor(age / 86400000) });
              } else if (stash.state === "closed" && age > closedThresholdMs) {
                readyToArchive.push({ stash_id: stash.stash_id, name: stash.name, age_days: Math.floor(age / 86400000) });
              }
            }

            const message = [
              staleSuspended.length > 0 ? `${staleSuspended.length} suspended stash(es) older than ${suspend_ttl_days} days — consider closing or resuming.` : "",
              readyToArchive.length > 0 ? `${readyToArchive.length} closed stash(es) older than ${closed_ttl_days} days — ready to archive.` : "",
              staleSuspended.length === 0 && readyToArchive.length === 0 ? "All stashes are within TTL limits." : "",
            ].filter(Boolean).join(" ");

            return JSON.stringify({ stale_suspended: staleSuspended, ready_to_archive: readyToArchive, dry_run, message });
          } catch (err) {
            return JSON.stringify({ error: err instanceof Error ? err.message : String(err) });
          }
        },
      }),

      // ──────────────────── stash.ref ────────────────────────────────────────
      // REQ-STASH-080, REQ-STASH-081, REQ-STASH-082: Expert Platform stash sharing
      // axiom:trace work_item=SWDE-55 spec=specs/106-Context-Stash.md#REQ-STASH-080
      "stash.ref": tool({
        description:
          "Get a portable stash reference (stash_id + summary content) for passing to another agent or expert. " +
          "REQ-STASH-080: stash references in expert request context. " +
          "REQ-STASH-081: stash_id included in response metadata. " +
          "Returns { stash_id, name, state, summary, tags, resume_hint } for embedding in request envelopes.",
        args: {
          id: tool.schema.string().describe("Stash ID to get reference for"),
        },
        async execute({ id }) {
          try {
            validateStashId(id);
            const content = await backend.read(id);
            if (!content) return JSON.stringify({ error: `No stash '${id}'.` });
            const { fm, body } = parseFrontmatter(content.raw);
            // REQ-STASH-082: return content (not path) for cross-expert sharing
            return JSON.stringify({
              stash_id: id, name: fm.name ?? id, state: content.state,
              tags: fm.tags ?? [], resume_hint: fm.resume_hint,
              summary: body.trim().slice(0, 1000),
              entries: content.state === ("active" as any) ? (content.raw.match(/^- ts:/gm) ?? []).length : (fm.entries ?? 0),
              // REQ-STASH-081: stash_id in response metadata
              _metadata: { stash_id: id, source_repo: "local" },
            });
          } catch (err) {
            return JSON.stringify({ error: err instanceof Error ? err.message : String(err) });
          }
        },
      }),

      // ──────────────────── stash.ingest ─────────────────────────────────────
      // REQ-STASH-083: Feed Ingestion → stash
      // axiom:trace work_item=SWDE-55 spec=specs/106-Context-Stash.md#REQ-STASH-083
      "stash.ingest": tool({
        description:
          "Write a feed item or external content to a stash as an observation entry. " +
          "REQ-STASH-083: Feed Ingestion pipeline items can be written to a designated stash. " +
          "Stash must be active. Returns { stash_id, entry_type, ts }.",
        args: {
          id: tool.schema.string().describe("Active stash ID to ingest into"),
          content: tool.schema.string().describe("Feed item content to append"),
          source: tool.schema.string().optional().describe("Source of the feed item (URL, feed name, etc.)"),
          type: tool.schema
            .enum(["observation", "finding", "summary"])
            .optional()
            .describe("Entry type (default: observation)"),
        },
        async execute({ id, content, source, type = "observation" }, context) {
          try {
            validateStashId(id);
            const ctx = context as { agent?: string };
            const agent = ctx?.agent ?? "feed-ingestion";
            const now = new Date().toISOString();
            await backend.append?.(id, {
              ts: now, agent, type: type as StashEntry["type"],
              content: redactCredentials(content),
              refs: source ? [source] : undefined,
            });
            return JSON.stringify({ stash_id: id, entry_type: type, ts: now, source, message: `Feed item ingested into stash '${id}'.` });
          } catch (err) {
            return JSON.stringify({ error: err instanceof Error ? err.message : String(err) });
          }
        },
      }),

      // ──────────────────── stash.node.enter ─────────────────────────────────
      // REQ-STASH-070, REQ-STASH-071: Graph Harness node entry
      // axiom:trace work_item=SWDE-55 spec=specs/106-Context-Stash.md#REQ-STASH-070
      "stash.node.enter": tool({
        description:
          "Called by Graph Harness when a node with a stash: field begins execution (REQ-STASH-070, -071). " +
          "Auto-creates or attaches to the named stash and enters managed context. " +
          "Returns { stash_id, state, node_id }.",
        args: {
          stash_id: tool.schema.string().describe("Stash name/ID from graph node stash: field"),
          node_id: tool.schema.string().describe("Graph node ID"),
          graph_id: tool.schema.string().optional().describe("Graph ID for tracing"),
        },
        async execute({ stash_id, node_id, graph_id }, context) {
          try {
            validateStashId(stash_id);
            const ctx = context as { agent?: string; sessionID?: string };
            const agent = ctx?.agent ?? "graph-node";
            const now = new Date().toISOString();

            // If stash is already active, attach without overwriting (REQ-STASH-071)
            // Multiple graph nodes can share the same active stash — entries are preserved.
            if (await backend.exists(stash_id, "active" as any)) {
              // Append a node-entry note without overwriting existing entries
              await backend.append?.(stash_id, {
                ts: now, agent, type: "summary",
                content: `[Graph node ${node_id} attached${graph_id ? ` from graph ${graph_id}` : ""}]`,
              });
              autoLogState.primaryStashId = stash_id;
              autoLogState.logLevel = "decisions";
              return JSON.stringify({ stash_id, state: "active", node_id, graph_id, message: `Graph node '${node_id}' attached to existing active stash '${stash_id}'.` });
            }

            // Create stash if it doesn't exist
            if (!(await backend.exists(stash_id))) {
              const fm = {
                stash_id, name: stash_id, state: "suspended" as const,
                created_by: agent, created_at: now, suspended_at: now,
                session_id: ctx?.sessionID ?? `graph-${Date.now()}`,
                tags: ["graph", graph_id ?? ""].filter(Boolean) as string[], entries: 0, last_agent: agent,
                resume_hint: `Graph node ${node_id}${graph_id ? ` (graph: ${graph_id})` : ""}`,
              };
              await backend.write(stash_id, { stashId: stash_id, state: "suspended", raw: buildSuspendedMarkdown(fm, `Graph node ${node_id} context stash`) });
            }
            // Enter managed context (REQ-STASH-071)
            const suspended = await backend.read(stash_id, "suspended");
            const existingBody = suspended ? buildEntryYaml({ ts: now, agent, type: "summary", content: `[Graph node ${node_id} entered${graph_id ? ` from graph ${graph_id}` : ""}]` }) : "";
            if (suspended) await backend.delete(stash_id, "suspended");
            const activeFm: StashFrontmatter & { session_id: string; entered_at: string } = {
              stash_id, name: stash_id, state: "active" as any,
              created_by: agent, created_at: now, entered_at: now,
              session_id: ctx?.sessionID ?? `graph-${Date.now()}`,
              tags: ["graph", graph_id ?? ""].filter(Boolean) as string[], last_agent: agent,
              log_level: "decisions" as const,
            };
            await backend.write(stash_id, { stashId: stash_id, state: "active" as any, raw: buildActiveYaml(activeFm) + existingBody });
            autoLogState.primaryStashId = stash_id;
            autoLogState.logLevel = "decisions";
            return JSON.stringify({ stash_id, state: "active", node_id, graph_id, message: `Graph node '${node_id}' entered stash context '${stash_id}'.` });
          } catch (err) {
            return JSON.stringify({ error: err instanceof Error ? err.message : String(err) });
          }
        },
      }),

      // ──────────────────── stash.node.complete ──────────────────────────────
      // REQ-STASH-072, REQ-STASH-075: Graph Harness node completion
      // axiom:trace work_item=SWDE-55 spec=specs/106-Context-Stash.md#REQ-STASH-072
      "stash.node.complete": tool({
        description:
          "Called by Graph Harness when a node completes (REQ-STASH-072). " +
          "Appends a summary entry with the node's outcome and exits managed context. " +
          "Returns { stash_id, state, entries_added }.",
        args: {
          stash_id: tool.schema.string().describe("Stash ID from graph node"),
          node_id: tool.schema.string().describe("Graph node ID"),
          outcome: tool.schema.string().optional().describe("Node outcome summary"),
          close_stash: tool.schema.boolean().optional().describe("Whether to close the stash after node completes (REQ-STASH-075)"),
        },
        async execute({ stash_id, node_id, outcome, close_stash = false }, context) {
          try {
            validateStashId(stash_id);
            const ctx = context as { agent?: string };
            const agent = ctx?.agent ?? "graph-harness";
            const now = new Date().toISOString();
            // Append completion summary (REQ-STASH-072)
            try {
              await backend.append?.(stash_id, {
                ts: now, agent, type: "summary",
                content: `[Node ${node_id} completed${outcome ? `: ${outcome}` : ""}]`,
              });
            } catch { /* stash might not be active */ }
            if (close_stash) {
              // REQ-STASH-075: auto-close stash when graph completes
              const active = await backend.read(stash_id, "active" as any);
              if (active) {
                const { fm, body } = parseFrontmatter(active.raw);
                const closedFm: StashFrontmatter & { session_id: string } = {
                  stash_id, name: fm.name ?? stash_id, state: "closed",
                  created_by: fm.created_by ?? agent, created_at: fm.created_at ?? now,
                  closed_at: now, session_id: fm.session_id ?? "",
                  tags: fm.tags ?? [], last_agent: agent, outcome,
                };
                await backend.write(stash_id, { stashId: stash_id, state: "closed", raw: buildSuspendedMarkdown(closedFm, body.trim()) });
                await backend.delete(stash_id, "active" as any);
                if (autoLogState.primaryStashId === stash_id) autoLogState.primaryStashId = null;
              }
            } else {
              // Just exit managed context
              const active = await backend.read(stash_id, "active" as any);
              if (active) {
                const { fm, body } = parseFrontmatter(active.raw);
                const suspendedFm: StashFrontmatter & { session_id: string } = {
                  stash_id, name: fm.name ?? stash_id, state: "suspended",
                  created_by: fm.created_by ?? agent, created_at: fm.created_at ?? now,
                  suspended_at: now, session_id: fm.session_id ?? "",
                  tags: fm.tags ?? [], last_agent: agent,
                };
                await backend.write(stash_id, { stashId: stash_id, state: "suspended", raw: buildSuspendedMarkdown(suspendedFm, body.trim()) });
                await backend.delete(stash_id, "active" as any);
                if (autoLogState.primaryStashId === stash_id) autoLogState.primaryStashId = null;
              }
            }
            return JSON.stringify({ stash_id, state: close_stash ? "closed" : "suspended", node_id, message: `Graph node '${node_id}' completed.` });
          } catch (err) {
            return JSON.stringify({ error: err instanceof Error ? err.message : String(err) });
          }
        },
      }),
    },

    // REQ-STASH-031: Auto-log tool calls to active stash
    // axiom:trace work_item=SWDE-55 spec=specs/106-Context-Stash.md#REQ-STASH-031
    "tool.execute.after": async (input: { tool: string; args?: Record<string, unknown> }, output: unknown) => {
      if (!autoLogState.primaryStashId) return;
      if (autoLogState.logLevel === "off") return;

      // Skip stash.* tool calls to avoid logging logging
      if (String(input.tool).startsWith("stash.")) return;

      const stashId = autoLogState.primaryStashId;
      const now = new Date().toISOString();

      try {
        if (autoLogState.logLevel === "all") {
          // Log every tool call
          const resultSnippet = typeof output === "string"
            ? output.slice(0, 200)
            : JSON.stringify(output).slice(0, 200);
          await backend.append?.(stashId, {
            ts: now,
            agent: "auto-log",
            type: "tool_call",
            content: `Tool: ${input.tool} → ${resultSnippet}${String(JSON.stringify(output)).length > 200 ? "…" : ""}`,
          });
        } else if (autoLogState.logLevel === "decisions") {
          // Only log tool calls that look like decisions/findings (write, edit, bash with content)
          const writeLike = ["write", "edit", "bash", "create"];
          if (writeLike.some((t) => String(input.tool).includes(t))) {
            const argStr = input.args ? Object.entries(input.args).map(([k, v]) => `${k}=${String(v).slice(0, 80)}`).join(", ") : "";
            await backend.append?.(stashId, {
              ts: now,
              agent: "auto-log",
              type: "decision",
              content: `Used ${input.tool}${argStr ? `: ${argStr}` : ""}`,
            });
          }
        }
        // "summaries" mode is handled by session.idle
      } catch {
        // Never let auto-logging crash the tool execution
      }

      // REQ-STASH-NEW-012: background stash polling — summary every 20th tool call
      autoLogState.toolCallCount++;
      if (autoLogState.toolCallCount % 20 === 0) {
        try {
          const backgroundStashes = await backend.list({ state: "active" as any } as any);
          for (const bs of backgroundStashes) {
            if ((bs as any).background && bs.stash_id !== autoLogState.primaryStashId) {
              await backend.append?.(bs.stash_id, {
                ts: new Date().toISOString(),
                agent: "auto-log",
                type: "summary",
                content: `[Background checkpoint — ${autoLogState.toolCallCount} tool calls since session start]`,
              });
            }
          }
        } catch { /* never crash */ }
      }
    },

    // REQ-STASH-034: Inject active stash context into compaction prompt
    // This ensures active investigation context survives session compaction.
    // axiom:trace work_item=SWDE-55 spec=specs/106-Context-Stash.md#REQ-STASH-034
    "experimental.session.compacting": async (_input: unknown, output: { context: string[]; prompt?: string }) => {
      if (!autoLogState.primaryStashId) return;
      try {
        const content = await backend.read(autoLogState.primaryStashId, "active" as any);
        if (!content) return;
        const { fm, body } = parseFrontmatter(content.raw);
        const entries = body.trim() ? body.trim().split(/\n(?=- ts:)/).filter(Boolean) : [];
        const recentEntries = entries.slice(-5); // last 5 entries for compaction context

        output.context.push(
          `## Active Stash Context: ${autoLogState.primaryStashId}\n` +
          `Name: ${fm.name ?? autoLogState.primaryStashId}\n` +
          `Log level: ${autoLogState.logLevel}\n` +
          `Total entries: ${entries.length}\n` +
          (fm.resume_hint ? `Resume hint: ${fm.resume_hint}\n` : "") +
          (recentEntries.length > 0
            ? `\nRecent log entries:\n\`\`\`yaml\n${recentEntries.join("\n")}\n\`\`\``
            : "")
        );
      } catch {
        // Never let compaction context injection crash the compaction
      }
    },

    // REQ-STASH-031 "summaries" mode: append summary entry when session goes idle
    // axiom:trace work_item=SWDE-55 spec=specs/106-Context-Stash.md#REQ-STASH-031
    "session.idle": async (_event: unknown) => {
      if (!autoLogState.primaryStashId) return;
      if (autoLogState.logLevel !== "summaries") return;
      try {
        await backend.append?.(autoLogState.primaryStashId, {
          ts: new Date().toISOString(),
          agent: "auto-log",
          type: "summary",
          content: "[Session idle — periodic summary checkpoint]",
        });
      } catch {
        // Never crash on auto-log
      }
    },

    // ─────────────────────────────────────────────────────────────────────────
    // Comprehensive hook coverage — all OpenCode plugin event types (SWDE-55)
    // axiom:trace work_item=SWDE-55 spec=specs/106-Context-Stash.md#REQ-STASH-031
    // axiom:trace work_item=SWDE-55 spec=specs/106-Context-Stash.md#REQ-STASH-034
    // ─────────────────────────────────────────────────────────────────────────

    // Log tool INTENT before execution (only in "all" mode to avoid noise)
    "tool.execute.before": async (input: { tool: string; args?: Record<string, unknown> }, _output: unknown) => {
      // axiom:trace work_item=SWDE-55 spec=specs/106-Context-Stash.md#REQ-STASH-031
      if (!autoLogState.primaryStashId) return;
      if (autoLogState.logLevel === "off") return;
      if (String(input.tool).startsWith("stash.")) return;
      if (autoLogState.logLevel !== "all") return; // before-hook only for "all" mode
      try {
        const argSummary = input.args
          ? Object.entries(input.args)
              .slice(0, 3)
              .map(([k, v]) => `${k}=${String(v).slice(0, 60)}`)
              .join(", ")
          : "";
        await backend.append?.(autoLogState.primaryStashId, {
          ts: new Date().toISOString(),
          agent: "auto-log",
          type: "tool_call",
          content: `→ ${input.tool}(${argSummary})`,
        });
      } catch { /* never crash tool execution */ }
    },

    // Inject active stash banner into every TUI prompt (REQ-STASH-034)
    "tui.prompt.append": async (_input: unknown, output: { text?: string; append?: string }) => {
      // axiom:trace work_item=SWDE-55 spec=specs/106-Context-Stash.md#REQ-STASH-034
      if (!autoLogState.primaryStashId) return;
      try {
        const content = await backend.read(autoLogState.primaryStashId, "active" as any);
        if (!content) return;
        const { fm, body } = parseFrontmatter(content.raw);
        const entries = body.trim() ? body.trim().split(/\n(?=- ts:)/).filter(Boolean) : [];
        const age = formatAge(fm.created_at ?? new Date().toISOString());
        // REQ-STASH-034: banner must include "last: <agent> <age>" and use "ACTIVE CONTEXT" label
        const lastEntry = entries.length > 0 ? parseStashEntry(entries[entries.length - 1]) : null;
        const lastAgent = lastEntry?.agent ? String(lastEntry.agent) : (fm.last_agent ?? "unknown");
        // REQ-STASH-NEW-009: size guard — only inject summary + resume_hint + last 10 if stash is large
        const CONTEXT_WINDOW_TOKENS = 100_000;
        const MAX_INJECT_CHARS = CONTEXT_WINDOW_TOKENS * 0.5 * 4; // ~50% of context, ~4 chars/token
        const rawSize = content.raw.length;
        let bannerExtra = "";
        if (rawSize > MAX_INJECT_CHARS) {
          const resumeHint = fm.resume_hint ? `Resume: ${fm.resume_hint}` : "";
          bannerExtra = ` [large stash: summary+hints only]${resumeHint ? ` | ${resumeHint}` : ""}`;
        }
        const banner = `[ACTIVE CONTEXT: ${autoLogState.primaryStashId} | ${entries.length} entries | last: ${lastAgent} ${age}]${bannerExtra}`;
        if (typeof output.append === "string") {
          output.append = banner + (output.append ? "\n" + output.append : "");
        } else {
          output.append = banner;
        }
      } catch { /* never crash prompts */ }
    },

    // Log LLM assistant responses in "all" mode
    "message.updated": async ({ event }: { event: { type: string; [key: string]: unknown } }) => {
      // axiom:trace work_item=SWDE-55 spec=specs/106-Context-Stash.md#REQ-STASH-031
      if (!autoLogState.primaryStashId) return;
      if (autoLogState.logLevel === "off") return;
      if (autoLogState.logLevel !== "all") return;
      try {
        const msg = event.message as { role?: string; parts?: Array<{ type: string; text?: string }> } | undefined;
        if (!msg || msg.role !== "assistant") return;
        const text = msg.parts?.map((p) => p.text ?? "").join("").trim().slice(0, 300);
        if (!text) return;
        await backend.append?.(autoLogState.primaryStashId, {
          ts: new Date().toISOString(),
          agent: "auto-log",
          type: "observation",
          content: `[AI response] ${text}${text.length >= 300 ? "…" : ""}`,
        });
      } catch { /* never crash */ }
    },

    // Log file edits as decisions
    "file.edited": async ({ event }: { event: { type: string; [key: string]: unknown } }) => {
      // axiom:trace work_item=SWDE-55 spec=specs/106-Context-Stash.md#REQ-STASH-031
      if (!autoLogState.primaryStashId) return;
      if (autoLogState.logLevel === "off") return;
      try {
        const filePath = String(event.path ?? event.file ?? "unknown");
        if (filePath.includes(".memory-bank/stash/")) return;
        await backend.append?.(autoLogState.primaryStashId, {
          ts: new Date().toISOString(),
          agent: "auto-log",
          type: "decision",
          content: `Edited: ${filePath}`,
          refs: [filePath],
        });
      } catch { /* never crash */ }
    },

    // Log session errors as blockers
    "session.error": async ({ event }: { event: { type: string; [key: string]: unknown } }) => {
      // axiom:trace work_item=SWDE-55 spec=specs/106-Context-Stash.md#REQ-STASH-031
      if (!autoLogState.primaryStashId) return;
      if (autoLogState.logLevel === "off") return;
      try {
        const errMsg = String(event.error ?? event.message ?? "unknown error").slice(0, 200);
        await backend.append?.(autoLogState.primaryStashId, {
          ts: new Date().toISOString(),
          agent: "auto-log",
          type: "blocker",
          content: `Session error: ${errMsg}`,
          severity: "error",
        });
      } catch { /* never crash */ }
    },

    // Log compaction event as summary
    "session.compacted": async (_event: unknown) => {
      // axiom:trace work_item=SWDE-55 spec=specs/106-Context-Stash.md#REQ-STASH-031
      if (!autoLogState.primaryStashId) return;
      if (autoLogState.logLevel === "off") return;
      try {
        await backend.append?.(autoLogState.primaryStashId, {
          ts: new Date().toISOString(),
          agent: "auto-log",
          type: "summary",
          content: "[Session compacted — context summarized by LLM]",
        });
      } catch { /* never crash */ }
    },

    // Log file diffs as decisions (decisions + all modes)
    "session.diff": async ({ event }: { event: { type: string; [key: string]: unknown } }) => {
      // axiom:trace work_item=SWDE-55 spec=specs/106-Context-Stash.md#REQ-STASH-031
      if (!autoLogState.primaryStashId) return;
      if (autoLogState.logLevel === "off") return;
      if (autoLogState.logLevel !== "all" && autoLogState.logLevel !== "decisions") return;
      try {
        const files = (event.files as string[] | undefined) ?? [];
        if (files.length === 0) return;
        const filtered = files.filter((f) => !f.includes(".memory-bank/stash/"));
        if (filtered.length === 0) return;
        await backend.append?.(autoLogState.primaryStashId, {
          ts: new Date().toISOString(),
          agent: "auto-log",
          type: "decision",
          content: `Session diff: ${filtered.slice(0, 5).join(", ")}${filtered.length > 5 ? ` +${filtered.length - 5} more` : ""}`,
          refs: filtered.slice(0, 5),
        });
      } catch { /* never crash */ }
    },

    // Log permission requests as questions
    "permission.asked": async ({ event }: { event: { type: string; [key: string]: unknown } }) => {
      // axiom:trace work_item=SWDE-55 spec=specs/106-Context-Stash.md#REQ-STASH-031
      if (!autoLogState.primaryStashId) return;
      if (autoLogState.logLevel === "off") return;
      try {
        const title = String(event.title ?? event.action ?? event.tool ?? "permission request").slice(0, 150);
        await backend.append?.(autoLogState.primaryStashId, {
          ts: new Date().toISOString(),
          agent: "auto-log",
          type: "question",
          content: `Permission requested: ${title}`,
        });
      } catch { /* never crash */ }
    },

    // Log permission grant/deny as decisions
    "permission.replied": async ({ event }: { event: { type: string; [key: string]: unknown } }) => {
      // axiom:trace work_item=SWDE-55 spec=specs/106-Context-Stash.md#REQ-STASH-031
      if (!autoLogState.primaryStashId) return;
      if (autoLogState.logLevel === "off") return;
      try {
        const granted = Boolean(event.granted ?? event.approved ?? event.allowed);
        const action = String(event.title ?? event.action ?? event.tool ?? "").slice(0, 100);
        await backend.append?.(autoLogState.primaryStashId, {
          ts: new Date().toISOString(),
          agent: "auto-log",
          type: "decision",
          content: `Permission ${granted ? "granted" : "denied"}${action ? `: ${action}` : ""}`,
          severity: granted ? undefined : "warn",
        });
      } catch { /* never crash */ }
    },

    // Log LSP errors as findings (only in "all" mode — too noisy otherwise)
    "lsp.client.diagnostics": async ({ event }: { event: { type: string; [key: string]: unknown } }) => {
      // axiom:trace work_item=SWDE-55 spec=specs/106-Context-Stash.md#REQ-STASH-031
      if (!autoLogState.primaryStashId) return;
      if (autoLogState.logLevel === "off") return;
      if (autoLogState.logLevel !== "all") return;
      try {
        const diagnostics = (event.diagnostics as Array<{ severity?: number; message?: string; file?: string }> | undefined) ?? [];
        const errors = diagnostics.filter((d) => d.severity === 1);
        if (errors.length === 0) return;
        const summary = errors.slice(0, 3).map((d) => `${d.file ?? "?"}:${d.message ?? "error"}`).join("; ");
        await backend.append?.(autoLogState.primaryStashId, {
          ts: new Date().toISOString(),
          agent: "auto-log",
          type: "finding",
          content: `LSP errors (${errors.length}): ${summary}${errors.length > 3 ? " …" : ""}`,
          severity: "error",
        });
      } catch { /* never crash */ }
    },

    // Auto-enter configured stash when a new session starts (REQ-STASH-030)
    "session.created": async (_event: unknown) => {
      // axiom:trace work_item=SWDE-55 spec=specs/106-Context-Stash.md#REQ-STASH-030
      // NOTE: If two sessions start simultaneously with the same STASH_AUTO_ENTER,
      // both will read the suspended stash and the second write will overwrite the first
      // session's entered_at/session_id. This is accepted behavior (last writer wins)
      // for the rare concurrent-session startup scenario.
      const autoEnterId = process.env.STASH_AUTO_ENTER;
      if (!autoEnterId) return;
      try {
        const suspended = await backend.read(autoEnterId, "suspended");
        if (!suspended) return;
        const { fm } = parseFrontmatter(suspended.raw);
        const now = new Date().toISOString();
        const logLevel = (process.env.STASH_AUTO_LOG_LEVEL ?? "decisions") as typeof autoLogState.logLevel;
        const activeFm = {
          stash_id: autoEnterId,
          name: fm.name ?? autoEnterId,
          state: "active" as any,
          created_by: "auto-log",
          created_at: now,
          entered_at: now,
          session_id: `auto-${Date.now()}`,
          tags: fm.tags ?? [],
          last_agent: "auto-log",
          resume_hint: fm.resume_hint,
          log_level: logLevel,
        };
        const existingEntries = fm.resume_hint
          ? buildEntryYaml({ ts: now, agent: "auto-log", type: "summary", content: `[Auto-entered on session start] ${fm.resume_hint}` })
          : "";
        await backend.delete(autoEnterId, "suspended");
        await backend.write(autoEnterId, {
          stashId: autoEnterId,
          state: "active" as any,
          raw: buildActiveYaml(activeFm) + existingEntries,
        });
        autoLogState.primaryStashId = autoEnterId;
        autoLogState.logLevel = logLevel;
        console.log(`[ContextStash] Auto-entered stash '${autoEnterId}' (log_level: ${logLevel})`);
      } catch { /* never crash session creation */ }
    },

    // Log TODO completions as decisions (only in "all" mode)
    "todo.updated": async ({ event }: { event: { type: string; [key: string]: unknown } }) => {
      // axiom:trace work_item=SWDE-55 spec=specs/106-Context-Stash.md#REQ-STASH-031
      if (!autoLogState.primaryStashId) return;
      if (autoLogState.logLevel !== "all") return;
      try {
        const todos = event.todos as Array<{ content?: string; status?: string }> | undefined;
        if (!todos || todos.length === 0) return;
        const completed = todos.filter((t) => t.status === "completed");
        if (completed.length === 0) return;
        await backend.append?.(autoLogState.primaryStashId, {
          ts: new Date().toISOString(),
          agent: "auto-log",
          type: "decision",
          content: `TODO completed: ${completed.map((t) => t.content?.slice(0, 80)).join("; ")}`,
        });
      } catch { /* never crash */ }
    },

    // Log commands executed in "all" mode
    "command.executed": async ({ event }: { event: { type: string; [key: string]: unknown } }) => {
      // axiom:trace work_item=SWDE-55 spec=specs/106-Context-Stash.md#REQ-STASH-031
      if (!autoLogState.primaryStashId) return;
      if (autoLogState.logLevel !== "all") return;
      try {
        const cmd = String(event.command ?? "").slice(0, 100);
        if (!cmd) return;
        await backend.append?.(autoLogState.primaryStashId, {
          ts: new Date().toISOString(),
          agent: "auto-log",
          type: "tool_call",
          content: `Command: ${cmd}`,
        });
      } catch { /* never crash */ }
    },

    // Clear auto-log state when session ends
    "session.deleted": async (_event: unknown) => {
      autoLogState.primaryStashId = null;
    },

    // ── Harmless pass-through stubs — present but intentionally minimal ───────
    "file.watcher.updated": async (_event: unknown) => { /* handled by file.edited */ },
    "lsp.updated": async (_event: unknown) => { /* no-op */ },
    "session.status": async (_event: unknown) => { /* no-op — too noisy */ },
    "session.updated": async (_event: unknown) => { /* no-op — too noisy */ },
    "message.removed": async (_event: unknown) => { /* no-op */ },
    "message.part.updated": async (_event: unknown) => { /* no-op */ },
    "message.part.removed": async (_event: unknown) => { /* no-op */ },
    "installation.updated": async (_event: unknown) => { /* no-op */ },
    "server.connected": async (_event: unknown) => { /* no-op */ },
  };
};
