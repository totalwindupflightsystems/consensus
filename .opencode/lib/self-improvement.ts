/**
 * Self-Improvement Loop — proposal schema validation, inbox writing, and cap enforcement.
 *
 * REQ-HLU-019: proposal schema (type, rationale, proposed_diff, risk, reversible, trace_refs)
 * REQ-HLU-020: caps (max 3/run, max 10 pending, 14-day TTL, 0 auto-applied)
 * REQ-HLU-023: emits self_improvement.proposal_created event
 *
 * axiom:trace work_item=harness-levelup-01 spec=specs/101-Harness-Engineering.md#REQ-HLU-019 plan=phase-8/task-8-1/step-8-1-1
 * axiom:trace work_item=harness-levelup-01 spec=specs/101-Harness-Engineering.md#REQ-HLU-020 plan=phase-8/task-8-1/step-8-1-1
 * axiom:trace work_item=harness-levelup-01 spec=specs/101-Harness-Engineering.md#REQ-HLU-023 plan=phase-8/task-8-1/step-8-1-1
 */

import { mkdirSync, writeFileSync, readdirSync, existsSync, readFileSync, unlinkSync } from "node:fs";
import { join } from "node:path";
import { stringify as yamlStringify, parse as yamlParse } from "yaml";

// ─────────────────────────────────────────────────────────────────────────────
// Types (REQ-HLU-019)
// ─────────────────────────────────────────────────────────────────────────────

// axiom:trace work_item=harness-levelup-01 spec=specs/101-Harness-Engineering.md#REQ-HLU-019 plan=phase-8/task-8-1/step-8-1-1

/** REQ-HLU-019: The 6 valid proposal types */
export type ProposalType =
  | "finding"
  | "skill_patch"
  | "prompt_patch"
  | "spec_proposal"
  | "config_patch"
  | "command_proposal";

/** REQ-HLU-019: Risk levels */
export type RiskLevel = "Low" | "Medium" | "High";

/**
 * REQ-HLU-019: A self-improvement proposal written to the inbox.
 * All 6 fields are required for a valid proposal.
 */
export interface SelfImprovementProposal {
  type: ProposalType;
  rationale: string;
  proposed_diff: string;
  risk: RiskLevel;
  reversible: boolean;
  trace_refs: string[];
  /** Auto-set by writeProposal: ISO8601 timestamp */
  created_at?: string;
  /** Auto-set by writeProposal: unique proposal ID */
  proposal_id?: string;
  /** Optional — for spec_proposal, config_patch, skill_patch */
  target_path?: string;
}

/** Configuration for writeProposal */
export interface ProposalWriteConfig {
  /** Maximum pending proposals before cap_exceeded. Default: 10 */
  max_pending?: number;
  /** Maximum proposals per run (caller tracks this). Default: 3 */
  max_per_run?: number;
  /** Inbox directory for proposals. Default: .memory-bank/inbox/self-improvement */
  inbox_dir?: string;
}

// ─────────────────────────────────────────────────────────────────────────────
// Constants
// ─────────────────────────────────────────────────────────────────────────────

const VALID_TYPES = new Set<string>([
  "finding",
  "skill_patch",
  "prompt_patch",
  "spec_proposal",
  "config_patch",
  "command_proposal",
]);

const VALID_RISK_LEVELS = new Set<string>(["Low", "Medium", "High"]);

const DEFAULT_MAX_PENDING = 10;
const DEFAULT_MAX_PER_RUN = 3;
const DEFAULT_INBOX_SUBPATH = join(".memory-bank", "inbox", "self-improvement");

// ─────────────────────────────────────────────────────────────────────────────
// REQ-HLU-019: Proposal validation
// axiom:trace work_item=harness-levelup-01 spec=specs/101-Harness-Engineering.md#REQ-HLU-019 plan=phase-8/task-8-1/step-8-1-1
// ─────────────────────────────────────────────────────────────────────────────

/**
 * Validates all 6 required fields of a proposal.
 * Throws Error on missing or invalid fields.
 *
 * Required field rules:
 * - type: must be one of the 6 ProposalType values
 * - rationale: non-empty string
 * - proposed_diff: non-empty string
 * - risk: must be Low | Medium | High
 * - reversible: must be a boolean
 * - trace_refs: must be a non-empty array
 */
export function validateProposal(p: Partial<SelfImprovementProposal>): SelfImprovementProposal {
  // Validate type
  if (p.type === undefined || p.type === null) {
    throw new Error("missing required field: type");
  }
  if (!VALID_TYPES.has(p.type)) {
    throw new Error(
      `invalid type: "${p.type}" — must be one of: finding, skill_patch, prompt_patch, spec_proposal, config_patch, command_proposal`
    );
  }

  // Validate rationale
  if (typeof p.rationale !== "string" || p.rationale.trim().length === 0) {
    throw new Error("missing or empty required field: rationale");
  }

  // Validate proposed_diff
  if (typeof p.proposed_diff !== "string" || p.proposed_diff.trim().length === 0) {
    throw new Error("missing or empty required field: proposed_diff");
  }

  // Validate risk
  if (p.risk === undefined || p.risk === null) {
    throw new Error("missing required field: risk");
  }
  if (!VALID_RISK_LEVELS.has(p.risk)) {
    throw new Error(`invalid risk: "${p.risk}" — must be one of: Low, Medium, High`);
  }

  // Validate reversible
  if (typeof p.reversible !== "boolean") {
    throw new Error("missing or invalid required field: reversible (must be boolean)");
  }

  // Validate trace_refs
  if (!Array.isArray(p.trace_refs) || p.trace_refs.length === 0) {
    throw new Error("missing or empty required field: trace_refs (must be a non-empty array)");
  }

  // All valid — return as SelfImprovementProposal (passthrough + optional fields preserved)
  return p as SelfImprovementProposal;
}

// ─────────────────────────────────────────────────────────────────────────────
// REQ-HLU-020: Pending proposal count
// axiom:trace work_item=harness-levelup-01 spec=specs/101-Harness-Engineering.md#REQ-HLU-020 plan=phase-8/task-8-1/step-8-1-1
// ─────────────────────────────────────────────────────────────────────────────

/**
 * Counts .md and .yaml files in inboxDir.
 * Returns 0 if the directory does not exist.
 */
export async function countPendingProposals(inboxDir: string): Promise<number> {
  if (!existsSync(inboxDir)) {
    return 0;
  }
  let entries: string[];
  try {
    entries = readdirSync(inboxDir);
  } catch {
    return 0;
  }
  return entries.filter((f) => f.endsWith(".md") || f.endsWith(".yaml")).length;
}

// ─────────────────────────────────────────────────────────────────────────────
// REQ-HLU-019/023: Filename builder
// ─────────────────────────────────────────────────────────────────────────────

/**
 * Builds the proposal filename.
 * Format: <ISO_timestamp_safe>-<type>-<proposal_id>.md
 * Timestamp: new Date().toISOString().replace(/[:.]/g, "-").slice(0, 19)
 */
export function buildProposalFilename(proposal_id: string, type: ProposalType): string {
  const ts = new Date().toISOString().replace(/[:.]/g, "-").slice(0, 19);
  return `${ts}-${type}-${proposal_id}.md`;
}

// ─────────────────────────────────────────────────────────────────────────────
// REQ-HLU-019/020/023: Write proposal to inbox
// axiom:trace work_item=harness-levelup-01 spec=specs/101-Harness-Engineering.md#REQ-HLU-019 plan=phase-8/task-8-1/step-8-1-1
// axiom:trace work_item=harness-levelup-01 spec=specs/101-Harness-Engineering.md#REQ-HLU-020 plan=phase-8/task-8-1/step-8-1-1
// axiom:trace work_item=harness-levelup-01 spec=specs/101-Harness-Engineering.md#REQ-HLU-023 plan=phase-8/task-8-1/step-8-1-1
// ─────────────────────────────────────────────────────────────────────────────

/**
 * Validates and writes a proposal to the self-improvement inbox.
 *
 * Steps:
 *  1. Validates the proposal (throws on invalid)
 *  2. Counts pending proposals; throws cap_exceeded if >= max_pending (10)
 *  3. Generates proposal_id: "si-" + Date.now() + "-" + random 5-char base36
 *  4. Sets created_at to ISO8601 timestamp
 *  5. Writes YAML-formatted proposal file to <inboxDir>/<filename>
 *  6. Emits self_improvement.proposal_created event to process.stderr as NDJSON
 *  7. Returns { proposal_id, path }
 */
export async function writeProposal(
  proposal: SelfImprovementProposal,
  repoRoot: string,
  config?: ProposalWriteConfig
): Promise<{ proposal_id: string; path: string }> {
  const maxPending = config?.max_pending ?? DEFAULT_MAX_PENDING;
  const inboxDir = config?.inbox_dir ?? join(repoRoot, DEFAULT_INBOX_SUBPATH);

  // Step 1: Validate proposal (throws on invalid)
  const validated = validateProposal(proposal);

  // Step 2: Count pending proposals; throw if at or above cap
  const pending = await countPendingProposals(inboxDir);
  if (pending >= maxPending) {
    throw new Error(`cap_exceeded: max ${maxPending} pending proposals`);
  }

  // Step 3: Generate proposal_id
  const proposal_id = "si-" + Date.now() + "-" + Math.random().toString(36).slice(2, 7);

  // Step 4: Set created_at
  const created_at = new Date().toISOString();

  // Assemble full proposal document
  const doc: SelfImprovementProposal & { created_at: string; proposal_id: string } = {
    ...validated,
    proposal_id,
    created_at,
  };

  // Step 5: Ensure inbox dir exists and write YAML file
  mkdirSync(inboxDir, { recursive: true });
  const filename = buildProposalFilename(proposal_id, validated.type);
  const filePath = join(inboxDir, filename);
  const yamlContent = yamlStringify(doc);
  writeFileSync(filePath, yamlContent, "utf8");

  // Step 6: Emit REQ-HLU-023 structured event to stderr as NDJSON
  const event = {
    event: "self_improvement.proposal_created",
    level: "INFO",
    proposal_id,
    type: validated.type,
    target_path: validated.target_path ?? "",
    risk: validated.risk,
    rationale_summary: validated.rationale.slice(0, 200),
    timestamp: created_at,
  };
  process.stderr.write(JSON.stringify(event) + "\n");

  // Step 7: Return
  return { proposal_id, path: filePath };
}

// ─────────────────────────────────────────────────────────────────────────────
// REQ-HLU-021/022/023: Consumer process
// axiom:trace work_item=harness-levelup-01 spec=specs/101-Harness-Engineering.md#REQ-HLU-021 plan=phase-8/task-8-2/step-8-2-1
// axiom:trace work_item=harness-levelup-01 spec=specs/101-Harness-Engineering.md#REQ-HLU-022 plan=phase-8/task-8-2/step-8-2-1
// axiom:trace work_item=harness-levelup-01 spec=specs/101-Harness-Engineering.md#REQ-HLU-023 plan=phase-8/task-8-2/step-8-2-1
// ─────────────────────────────────────────────────────────────────────────────

/** Result returned by consumeSelfImprovementInbox(). */
export interface ConsumeResult {
  /** Total number of proposal files processed (expired + routed). */
  processed: number;
  /** Number of proposals expired (age > ttl_days). */
  expired: number;
  /** Number of non-expired proposals routed to human review. */
  routed_to_review: number;
  /** True if pending count was at or above max_pending at time of processing. */
  cap_at_limit: boolean;
  /** Parse or I/O errors per file (non-fatal — consumer continues). */
  errors: string[];
}

/** Configuration for consumeSelfImprovementInbox(). */
export interface ConsumeConfig {
  /** Inbox directory. Default: <repoRoot>/.memory-bank/inbox/self-improvement */
  inbox_dir?: string;
  /** Proposal TTL in days. Default: 14 */
  ttl_days?: number;
  /** Maximum pending proposals before cap_at_limit is set. Default: 10 */
  max_pending?: number;
}

const DEFAULT_TTL_DAYS = 14;
const MS_PER_DAY = 24 * 60 * 60 * 1000;

/**
 * REQ-HLU-021: Consumer process — runs at least once per day.
 *
 * For each .md/.yaml in the inbox:
 *  - Parses YAML frontmatter to extract created_at and proposal_id.
 *  - If age > ttl_days: writes a rejection record, deletes the pending file,
 *    emits self_improvement.proposal_expired to stderr (REQ-HLU-023).
 *  - Otherwise: logs to stderr as "awaiting human review" and increments
 *    routed_to_review.
 *  - Unparseable files are recorded in errors without throwing.
 *
 * REQ-HLU-022: Proposals with type "finding" follow the findings flow
 * (written to .memory-bank/findings/self-improvement/ by writeProposal callers;
 * consumer honours them identically — they go through the same TTL/review gate).
 */
export async function consumeSelfImprovementInbox(
  repoRoot: string,
  config?: ConsumeConfig
): Promise<ConsumeResult> {
  const inboxDir = config?.inbox_dir ?? join(repoRoot, DEFAULT_INBOX_SUBPATH);
  const ttlDays = config?.ttl_days ?? DEFAULT_TTL_DAYS;
  const maxPending = config?.max_pending ?? DEFAULT_MAX_PENDING;

  const result: ConsumeResult = {
    processed: 0,
    expired: 0,
    routed_to_review: 0,
    cap_at_limit: false,
    errors: [],
  };

  // If the inbox dir doesn't exist, there's nothing to process.
  if (!existsSync(inboxDir)) {
    return result;
  }

  let entries: string[];
  try {
    entries = readdirSync(inboxDir);
  } catch (err) {
    result.errors.push(`Failed to read inbox dir: ${String(err)}`);
    return result;
  }

  const proposalFiles = entries.filter(
    (f) => (f.endsWith(".md") || f.endsWith(".yaml")) && !f.startsWith(".")
  );

  // Check cap
  if (proposalFiles.length >= maxPending) {
    result.cap_at_limit = true;
  }

  const now = Date.now();

  for (const filename of proposalFiles) {
    const filePath = join(inboxDir, filename);

    // Parse YAML frontmatter using statically-imported yamlParse and readFileSync
    let doc: Record<string, unknown>;
    try {
      const raw = readFileSync(filePath, "utf8");
      doc = (yamlParse(raw) ?? {}) as Record<string, unknown>;
    } catch (err) {
      result.errors.push(`Failed to parse ${filename}: ${String(err)}`);
      continue;
    }

    const proposal_id = String(doc["proposal_id"] ?? filename);
    const created_at_raw = doc["created_at"];

    if (!created_at_raw) {
      result.errors.push(`Missing created_at in ${filename}`);
      continue;
    }

    const createdAt = new Date(String(created_at_raw));
    if (isNaN(createdAt.getTime())) {
      result.errors.push(`Invalid created_at "${String(created_at_raw)}" in ${filename}`);
      continue;
    }

    const ageMs = now - createdAt.getTime();
    const ageDays = ageMs / MS_PER_DAY;

    result.processed += 1;

    if (ageDays > ttlDays) {
      // ── EXPIRED ────────────────────────────────────────────────────────────

      // Write rejection record using statically-imported functions
      const rejectedDir = join(inboxDir, "rejected");
      try {
        mkdirSync(rejectedDir, { recursive: true });
        const expiredAt = new Date().toISOString();
        const rejectionDoc = {
          reason: "ttl_expired",
          expired_at: expiredAt,
          original_proposal_id: proposal_id,
          original_filename: filename,
          age_days: Math.round(ageDays * 10) / 10,
          ttl_days: ttlDays,
        };
        const rejectionPath = join(rejectedDir, `${proposal_id}-ttl-expired.yaml`);
        writeFileSync(rejectionPath, yamlStringify(rejectionDoc), "utf8");

        // Delete the pending file
        unlinkSync(filePath);
      } catch (err) {
        result.errors.push(`Failed to write rejection record for ${filename}: ${String(err)}`);
        // Don't increment expired — the file wasn't cleaned up successfully
        continue;
      }

      // Emit REQ-HLU-023 observability event to stderr as NDJSON
      const event = {
        event: "self_improvement.proposal_expired",
        level: "INFO",
        proposal_id,
        ttl_days: ttlDays,
        reason: "ttl_expired",
        timestamp: new Date().toISOString(),
      };
      process.stderr.write(JSON.stringify(event) + "\n");

      result.expired += 1;
    } else {
      // ── ROUTE TO REVIEW ────────────────────────────────────────────────────
      process.stderr.write(
        `[self-improvement] Proposal ${proposal_id} awaiting human review\n`
      );
      result.routed_to_review += 1;
    }
  }

  return result;
}

// OpenCode plugin loader no-op — this file is a utility module, not a plugin.
// OpenCode auto-discovers all .ts files in plugins/ and tries to load them.
// This export prevents "Plugin export is not a function" errors.
export default async () => ({ tool: {} });
