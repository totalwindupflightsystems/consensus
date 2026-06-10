/**
 * Tests for self-improvement.ts — REQ-HLU-019, REQ-HLU-020, REQ-HLU-023.
 *
 * REQ-HLU-019: proposal schema validation (6 required fields, 6 valid types)
 * REQ-HLU-020: cap enforcement (max 10 pending, max 3/run, 14-day TTL, 0 auto-applied)
 * REQ-HLU-023: emits self_improvement.proposal_created event to stderr
 *
 * Run: cd .opencode && bun test tests/self-improvement.test.ts
 *
 * axiom:trace work_item=harness-levelup-01 spec=specs/101-Harness-Engineering.md#REQ-HLU-019 plan=phase-8/task-8-1/step-8-1-1 test=self-improvement.test.ts
 * axiom:trace work_item=harness-levelup-01 spec=specs/101-Harness-Engineering.md#REQ-HLU-020 plan=phase-8/task-8-1/step-8-1-1 test=self-improvement.test.ts
 * axiom:trace work_item=harness-levelup-01 spec=specs/101-Harness-Engineering.md#REQ-HLU-023 plan=phase-8/task-8-1/step-8-1-1 test=self-improvement.test.ts
 */

import { test, expect, describe } from "bun:test";
import { mkdtempSync, mkdirSync, writeFileSync, existsSync, readdirSync, readFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { parse as yamlParse } from "yaml";
import {
  validateProposal,
  countPendingProposals,
  writeProposal,
  buildProposalFilename,
  consumeSelfImprovementInbox,
  type SelfImprovementProposal,
  type ProposalType,
} from "../lib/self-improvement.ts";

// ─────────────────────────────────────────────────────────────────────────────
// Test helpers
// ─────────────────────────────────────────────────────────────────────────────

function makeValidProposal(overrides?: Partial<SelfImprovementProposal>): SelfImprovementProposal {
  return {
    type: "finding",
    rationale: "The harness fails silently when config is missing; evidence: run-001/verification.md",
    proposed_diff: "--- a/config.ts\n+++ b/config.ts\n@@ -1 +1 @@\n+const DEFAULT_CONFIG = {};",
    risk: "Low",
    reversible: true,
    trace_refs: ["axiom:trace work_item=harness-levelup-01 spec=specs/101-Harness-Engineering.md"],
    ...overrides,
  };
}

// ─────────────────────────────────────────────────────────────────────────────
// REQ-HLU-019: Proposal schema validation
// ─────────────────────────────────────────────────────────────────────────────

describe("REQ-HLU-019: proposal schema validation", () => {
  test("validateProposal accepts a fully valid proposal", () => {
    const proposal = makeValidProposal();
    expect(() => validateProposal(proposal)).not.toThrow();
    const result = validateProposal(proposal);
    expect(result.type).toBe("finding");
    expect(result.risk).toBe("Low");
    expect(result.reversible).toBe(true);
  });

  test("validateProposal rejects missing type field", () => {
    const proposal = makeValidProposal({ type: undefined as unknown as ProposalType });
    expect(() => validateProposal(proposal)).toThrow("missing required field: type");
  });

  test("validateProposal rejects invalid type value", () => {
    const proposal = makeValidProposal({ type: "bad_type" as ProposalType });
    expect(() => validateProposal(proposal)).toThrow(/invalid type/);
  });

  test("validateProposal rejects empty rationale", () => {
    const proposal = makeValidProposal({ rationale: "" });
    expect(() => validateProposal(proposal)).toThrow(/rationale/);
  });

  test("validateProposal rejects empty proposed_diff", () => {
    const proposal = makeValidProposal({ proposed_diff: "" });
    expect(() => validateProposal(proposal)).toThrow(/proposed_diff/);
  });

  test("validateProposal rejects empty trace_refs array", () => {
    const proposal = makeValidProposal({ trace_refs: [] });
    expect(() => validateProposal(proposal)).toThrow(/trace_refs/);
  });

  test("validateProposal rejects invalid risk value", () => {
    const proposal = makeValidProposal({ risk: "Critical" as "Low" | "Medium" | "High" });
    expect(() => validateProposal(proposal)).toThrow(/invalid risk/);
  });

  test("validateProposal accepts all 6 valid ProposalType values", () => {
    const validTypes: ProposalType[] = [
      "finding",
      "skill_patch",
      "prompt_patch",
      "spec_proposal",
      "config_patch",
      "command_proposal",
    ];

    for (const type of validTypes) {
      const proposal = makeValidProposal({ type });
      expect(() => validateProposal(proposal)).not.toThrow();
      expect(validateProposal(proposal).type).toBe(type);
    }
  });
});

// ─────────────────────────────────────────────────────────────────────────────
// REQ-HLU-020: Cap enforcement
// ─────────────────────────────────────────────────────────────────────────────

describe("REQ-HLU-020: cap enforcement", () => {
  test("writeProposal succeeds when pending count is below cap (10)", async () => {
    const repoRoot = mkdtempSync(join(tmpdir(), "si-cap-below-"));
    const inbox = join(repoRoot, ".memory-bank", "inbox", "self-improvement");
    const { mkdirSync } = await import("node:fs");
    mkdirSync(inbox, { recursive: true });

    // Create 5 stub .md files — below the cap of 10
    for (let i = 0; i < 5; i++) {
      writeFileSync(join(inbox, `stub-${i}.md`), "stub");
    }

    const result = await writeProposal(makeValidProposal(), repoRoot);
    expect(result.proposal_id).toMatch(/^si-\d+-[a-z0-9]+$/);
    expect(result.path).toContain(".memory-bank");
  });

  test("writeProposal throws cap_exceeded when 10 proposals already pending", async () => {
    const repoRoot = mkdtempSync(join(tmpdir(), "si-cap-full-"));
    const inbox = join(repoRoot, ".memory-bank", "inbox", "self-improvement");
    const { mkdirSync } = await import("node:fs");
    mkdirSync(inbox, { recursive: true });

    // Create exactly 10 stub .md files to hit the cap
    for (let i = 0; i < 10; i++) {
      writeFileSync(join(inbox, `proposal-${i}.md`), "stub");
    }

    await expect(
      writeProposal(makeValidProposal(), repoRoot)
    ).rejects.toThrow("cap_exceeded: max 10 pending proposals");
  });

  test("countPendingProposals returns 0 for non-existent directory", async () => {
    const count = await countPendingProposals("/nonexistent/path/that/does/not/exist");
    expect(count).toBe(0);
  });

  test("countPendingProposals counts .md files in inbox dir", async () => {
    const inboxDir = mkdtempSync(join(tmpdir(), "si-count-"));

    // Create 3 .md files and 1 non-.md file
    writeFileSync(join(inboxDir, "p1.md"), "proposal 1");
    writeFileSync(join(inboxDir, "p2.md"), "proposal 2");
    writeFileSync(join(inboxDir, "p3.md"), "proposal 3");
    writeFileSync(join(inboxDir, "notes.txt"), "not a proposal");

    const count = await countPendingProposals(inboxDir);
    expect(count).toBe(3);
  });

  test("writeProposal creates inbox dir if it doesn't exist", async () => {
    const repoRoot = mkdtempSync(join(tmpdir(), "si-mkdir-"));
    const inbox = join(repoRoot, ".memory-bank", "inbox", "self-improvement");

    // Confirm inbox does not yet exist
    expect(existsSync(inbox)).toBe(false);

    await writeProposal(makeValidProposal(), repoRoot);

    // Inbox should now exist
    expect(existsSync(inbox)).toBe(true);
  });
});

// ─────────────────────────────────────────────────────────────────────────────
// REQ-HLU-019/023: Proposal writing and events
// ─────────────────────────────────────────────────────────────────────────────

describe("REQ-HLU-019/023: proposal writing and events", () => {
  test("writeProposal writes a YAML-formatted file to inbox_dir", async () => {
    const repoRoot = mkdtempSync(join(tmpdir(), "si-write-"));
    const inbox = join(repoRoot, ".memory-bank", "inbox", "self-improvement");

    await writeProposal(makeValidProposal(), repoRoot);

    expect(existsSync(inbox)).toBe(true);
    const files = readdirSync(inbox);
    expect(files.length).toBe(1);
    expect(files[0]).toMatch(/\.md$/);
  });

  test("writeProposal-produced file contains all 6 required fields", async () => {
    const repoRoot = mkdtempSync(join(tmpdir(), "si-fields-"));
    const inbox = join(repoRoot, ".memory-bank", "inbox", "self-improvement");

    const proposal = makeValidProposal({
      type: "spec_proposal",
      rationale: "Spec missing REQ-HLU-999",
      proposed_diff: "--- a/spec.md\n+++ b/spec.md\n+REQ-HLU-999: new requirement",
      risk: "Medium",
      reversible: false,
      trace_refs: ["axiom:trace work_item=harness-levelup-01", "run-002/verification.md"],
      target_path: "specs/101-Harness-Engineering.md",
    });

    await writeProposal(proposal, repoRoot);

    const files = readdirSync(inbox);
    const content = readFileSync(join(inbox, files[0]), "utf8");
    const parsed = yamlParse(content) as Record<string, unknown>;

    // All 6 required fields must be present
    expect(parsed).toHaveProperty("type", "spec_proposal");
    expect(parsed).toHaveProperty("rationale");
    expect(parsed).toHaveProperty("proposed_diff");
    expect(parsed).toHaveProperty("risk", "Medium");
    expect(parsed).toHaveProperty("reversible", false);
    expect(parsed).toHaveProperty("trace_refs");
    expect(Array.isArray(parsed["trace_refs"])).toBe(true);
  });

  test("writeProposal sets created_at to a valid ISO8601 timestamp", async () => {
    const repoRoot = mkdtempSync(join(tmpdir(), "si-ts-"));
    const inbox = join(repoRoot, ".memory-bank", "inbox", "self-improvement");

    const before = new Date().toISOString();
    await writeProposal(makeValidProposal(), repoRoot);
    const after = new Date().toISOString();

    const files = readdirSync(inbox);
    const content = readFileSync(join(inbox, files[0]), "utf8");
    const parsed = yamlParse(content) as Record<string, unknown>;

    const createdAt = parsed["created_at"] as string;
    expect(typeof createdAt).toBe("string");

    // Should be parseable as a date
    const ts = new Date(createdAt);
    expect(isNaN(ts.getTime())).toBe(false);

    // Should be within the test window
    expect(createdAt >= before).toBe(true);
    expect(createdAt <= after).toBe(true);
  });

  test("writeProposal emits self_improvement.proposal_created event to stderr", async () => {
    const repoRoot = mkdtempSync(join(tmpdir(), "si-event-"));

    // Capture stderr
    const stderrLines: string[] = [];
    const originalWrite = process.stderr.write.bind(process.stderr);
    let captured = "";
    process.stderr.write = ((chunk: string | Uint8Array, ...args: unknown[]) => {
      if (typeof chunk === "string") captured += chunk;
      return originalWrite(chunk, ...(args as Parameters<typeof originalWrite>).slice(1));
    }) as typeof process.stderr.write;

    try {
      await writeProposal(makeValidProposal({ type: "config_patch", risk: "High" }), repoRoot);
    } finally {
      process.stderr.write = originalWrite;
    }

    // Parse all NDJSON lines from stderr output
    const lines = captured.trim().split("\n").filter((l) => l.trim().length > 0);
    const eventLines = lines.filter((l) => {
      try {
        const parsed = JSON.parse(l) as Record<string, unknown>;
        return parsed["event"] === "self_improvement.proposal_created";
      } catch {
        return false;
      }
    });

    expect(eventLines.length).toBeGreaterThanOrEqual(1);

    const event = JSON.parse(eventLines[0]) as Record<string, unknown>;
    expect(event["event"]).toBe("self_improvement.proposal_created");
    expect(event["level"]).toBe("INFO");
    expect(event["type"]).toBe("config_patch");
    expect(event["risk"]).toBe("High");
    expect(typeof event["proposal_id"]).toBe("string");
    expect((event["proposal_id"] as string)).toMatch(/^si-\d+-[a-z0-9]+$/);
  });

  test("writeProposal returns correct proposal_id and path", async () => {
    const repoRoot = mkdtempSync(join(tmpdir(), "si-return-"));
    const inbox = join(repoRoot, ".memory-bank", "inbox", "self-improvement");

    const result = await writeProposal(makeValidProposal(), repoRoot);

    expect(result.proposal_id).toMatch(/^si-\d+-[a-z0-9]+$/);
    expect(result.path).toContain(inbox);
    expect(result.path).toMatch(/\.md$/);
    expect(existsSync(result.path)).toBe(true);
  });
});

// ─────────────────────────────────────────────────────────────────────────────
// REQ-HLU-021/022/023: Consumer process
//
// axiom:trace work_item=harness-levelup-01 spec=specs/101-Harness-Engineering.md#REQ-HLU-021 plan=phase-8/task-8-2/step-8-2-1 test=self-improvement.test.ts
// axiom:trace work_item=harness-levelup-01 spec=specs/101-Harness-Engineering.md#REQ-HLU-022 plan=phase-8/task-8-2/step-8-2-1 test=self-improvement.test.ts
// axiom:trace work_item=harness-levelup-01 spec=specs/101-Harness-Engineering.md#REQ-HLU-023 plan=phase-8/task-8-2/step-8-2-1 test=self-improvement.test.ts
// ─────────────────────────────────────────────────────────────────────────────

describe("REQ-HLU-021/022/023: consumer process", () => {
  // Helper: write a minimal valid proposal YAML file into an inbox directory.
  // Uses top-level node:fs imports (not require) to avoid ESM/CJS issues in bun test.
  function writeProposalFile(
    inboxDir: string,
    filename: string,
    proposal_id: string,
    created_at: string,
    type = "finding"
  ) {
    mkdirSync(inboxDir, { recursive: true });
    const content = [
      `proposal_id: "${proposal_id}"`,
      `type: "${type}"`,
      `created_at: "${created_at}"`,
      `rationale: "test rationale"`,
      `proposed_diff: "--- a/x\\n+++ b/x\\n+fix"`,
      `risk: Low`,
      `reversible: true`,
      `trace_refs:`,
      `  - "axiom:trace work_item=harness-levelup-01"`,
    ].join("\n");
    writeFileSync(join(inboxDir, filename), content, "utf8");
  }

  test("consumeSelfImprovementInbox processes pending proposals", async () => {
    const repoRoot = mkdtempSync(join(tmpdir(), "si-consume-basic-"));
    const inbox = join(repoRoot, ".memory-bank", "inbox", "self-improvement");
    const now = new Date().toISOString();

    writeProposalFile(inbox, "proposal-001.yaml", "si-001", now);
    writeProposalFile(inbox, "proposal-002.yaml", "si-002", now);

    const result = await consumeSelfImprovementInbox(repoRoot);

    expect(result.processed).toBe(2);
    expect(result.errors.length).toBe(0);
  });

  test("consumeSelfImprovementInbox expires proposals older than 14 days", async () => {
    const repoRoot = mkdtempSync(join(tmpdir(), "si-consume-expire-"));
    const inbox = join(repoRoot, ".memory-bank", "inbox", "self-improvement");

    // 20 days ago
    const twentyDaysAgo = new Date(Date.now() - 20 * 24 * 60 * 60 * 1000).toISOString();
    writeProposalFile(inbox, "old-proposal.yaml", "si-old-001", twentyDaysAgo);

    const result = await consumeSelfImprovementInbox(repoRoot);

    expect(result.expired).toBe(1);
    expect(result.routed_to_review).toBe(0);
    expect(result.processed).toBe(1);
  });

  test("expired proposals write rejection record with reason: ttl_expired", async () => {
    const repoRoot = mkdtempSync(join(tmpdir(), "si-consume-rejected-"));
    const inbox = join(repoRoot, ".memory-bank", "inbox", "self-improvement");

    const twentyDaysAgo = new Date(Date.now() - 20 * 24 * 60 * 60 * 1000).toISOString();
    const proposalId = "si-ttl-test-001";
    writeProposalFile(inbox, "old-proposal.yaml", proposalId, twentyDaysAgo);

    await consumeSelfImprovementInbox(repoRoot);

    // Rejection record must exist
    const rejectedDir = join(inbox, "rejected");
    expect(existsSync(rejectedDir)).toBe(true);
    const rejectedFiles = readdirSync(rejectedDir);
    expect(rejectedFiles.length).toBeGreaterThanOrEqual(1);

    const rejFile = rejectedFiles.find((f) => f.includes(proposalId));
    expect(rejFile).toBeDefined();

    const content = readFileSync(join(rejectedDir, rejFile!), "utf8");
    const parsed = yamlParse(content) as Record<string, unknown>;
    expect(parsed["reason"]).toBe("ttl_expired");
    expect(parsed["original_proposal_id"]).toBe(proposalId);

    // Original pending file must be deleted
    expect(existsSync(join(inbox, "old-proposal.yaml"))).toBe(false);
  });

  test("consumeSelfImprovementInbox emits proposal_expired event to stderr", async () => {
    const repoRoot = mkdtempSync(join(tmpdir(), "si-consume-event-"));
    const inbox = join(repoRoot, ".memory-bank", "inbox", "self-improvement");

    const twentyDaysAgo = new Date(Date.now() - 20 * 24 * 60 * 60 * 1000).toISOString();
    writeProposalFile(inbox, "old-event.yaml", "si-event-expire-001", twentyDaysAgo);

    // Capture stderr
    let captured = "";
    const originalWrite = process.stderr.write.bind(process.stderr);
    process.stderr.write = ((chunk: string | Uint8Array, ...args: unknown[]) => {
      if (typeof chunk === "string") captured += chunk;
      return originalWrite(chunk, ...(args as Parameters<typeof originalWrite>).slice(1));
    }) as typeof process.stderr.write;

    try {
      await consumeSelfImprovementInbox(repoRoot);
    } finally {
      process.stderr.write = originalWrite;
    }

    const lines = captured.trim().split("\n").filter((l) => l.trim().length > 0);
    const expiredEvents = lines.filter((l) => {
      try {
        const p = JSON.parse(l) as Record<string, unknown>;
        return p["event"] === "self_improvement.proposal_expired";
      } catch {
        return false;
      }
    });

    expect(expiredEvents.length).toBeGreaterThanOrEqual(1);

    const event = JSON.parse(expiredEvents[0]) as Record<string, unknown>;
    expect(event["event"]).toBe("self_improvement.proposal_expired");
    expect(event["reason"]).toBe("ttl_expired");
    expect(typeof event["proposal_id"]).toBe("string");
    expect(typeof event["ttl_days"]).toBe("number");
  });

  test("consumeSelfImprovementInbox routes non-expired proposals to review", async () => {
    const repoRoot = mkdtempSync(join(tmpdir(), "si-consume-route-"));
    const inbox = join(repoRoot, ".memory-bank", "inbox", "self-improvement");

    // 3 days old — well within 14-day TTL
    const threeDaysAgo = new Date(Date.now() - 3 * 24 * 60 * 60 * 1000).toISOString();
    writeProposalFile(inbox, "recent-proposal.yaml", "si-recent-001", threeDaysAgo);

    const result = await consumeSelfImprovementInbox(repoRoot);

    expect(result.routed_to_review).toBe(1);
    expect(result.expired).toBe(0);
    expect(result.processed).toBe(1);

    // Non-expired file should still exist
    expect(existsSync(join(inbox, "recent-proposal.yaml"))).toBe(true);
  });

  test("consumeSelfImprovementInbox returns correct ConsumeResult counts", async () => {
    const repoRoot = mkdtempSync(join(tmpdir(), "si-consume-counts-"));
    const inbox = join(repoRoot, ".memory-bank", "inbox", "self-improvement");

    const now = new Date().toISOString();
    const twentyDaysAgo = new Date(Date.now() - 20 * 24 * 60 * 60 * 1000).toISOString();

    // 2 fresh, 1 expired
    writeProposalFile(inbox, "fresh-1.yaml", "si-fresh-001", now);
    writeProposalFile(inbox, "fresh-2.yaml", "si-fresh-002", now);
    writeProposalFile(inbox, "old-1.yaml", "si-old-count-001", twentyDaysAgo);

    const result = await consumeSelfImprovementInbox(repoRoot);

    expect(result.processed).toBe(3);
    expect(result.expired).toBe(1);
    expect(result.routed_to_review).toBe(2);
    expect(result.errors.length).toBe(0);
  });

  test("consumeSelfImprovementInbox handles empty inbox gracefully", async () => {
    const repoRoot = mkdtempSync(join(tmpdir(), "si-consume-empty-"));
    // Do NOT create any inbox dir — it doesn't exist yet

    const result = await consumeSelfImprovementInbox(repoRoot);

    expect(result.processed).toBe(0);
    expect(result.expired).toBe(0);
    expect(result.routed_to_review).toBe(0);
    expect(result.cap_at_limit).toBe(false);
    expect(result.errors.length).toBe(0);
  });

  test("consumeSelfImprovementInbox handles unparseable files without throwing", async () => {
    const repoRoot = mkdtempSync(join(tmpdir(), "si-consume-badparse-"));
    const inbox = join(repoRoot, ".memory-bank", "inbox", "self-improvement");
    mkdirSync(inbox, { recursive: true });

    // Write an invalid YAML file that will fail to parse as a proposal
    // (valid YAML but missing created_at)
    writeFileSync(join(inbox, "bad-proposal.yaml"), ": invalid yaml: {unclosed", "utf8");

    // Also write a valid one to confirm processing continues
    const now = new Date().toISOString();
    writeProposalFile(inbox, "good-proposal.yaml", "si-good-001", now);

    const result = await consumeSelfImprovementInbox(repoRoot);

    // Should not throw; errors list should have the bad file; good one still processed
    expect(result.errors.length).toBeGreaterThanOrEqual(1);
    expect(result.processed).toBeGreaterThanOrEqual(1);
  });
});
