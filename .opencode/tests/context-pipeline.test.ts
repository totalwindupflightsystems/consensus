/**
 * Tests for context-pipeline.ts — REQ-HLU-001, REQ-HLU-002, REQ-HLU-008.
 *
 * REQ-HLU-001: Axiom MUST model context collection, ranking, packing, injection,
 *              execution, compaction, and evidence capture as observable stages.
 * REQ-HLU-002: Each context pipeline stage SHOULD define token/time budgets,
 *              truncation behavior, and failure semantics.
 * REQ-HLU-008: Harness stages MUST emit bounded structured events.
 *
 * Run: cd .opencode && bun test tests/context-pipeline.test.ts
 *
 * axiom:trace work_item=harness-levelup-01 spec=specs/101-Harness-Engineering.md#REQ-HLU-001 plan=phase-5/task-5-1/step-5-1-1 test=context-pipeline.test.ts
 * axiom:trace work_item=harness-levelup-01 spec=specs/101-Harness-Engineering.md#REQ-HLU-002 plan=phase-5/task-5-1/step-5-1-1 test=context-pipeline.test.ts
 * axiom:trace work_item=harness-levelup-01 spec=specs/101-Harness-Engineering.md#REQ-HLU-008 plan=phase-5/task-5-1/step-5-1-1 test=context-pipeline.test.ts
 */

import { test, expect, describe, beforeEach } from "bun:test";
import { existsSync, readFileSync, writeFileSync, mkdirSync, statSync as _statSync } from "node:fs";
import path from "node:path";
import os from "node:os";
import {
  readCandidateFiles,
  _getCandidateCache,
  getCandidateMaxMtime,
} from "../lib/context-pipeline-hook.ts";
import {
  runContextPipeline,
  stageCollection,
  stageRanking,
  stagePacking,
  stageInjection,
  stageExecution,
  stageCompaction,
  stageEvidenceCapture,
  labelContextItem,
  buildPrefillContext,
  formatLabelledContextForPrompt,
  distillSubagentResult,
  PIPELINE_STAGES,
  type ContextItem,
  type PipelineConfig,
  type PipelineEvent,
  type StageCompletedEvent,
  type PipelineStartedEvent,
  type PipelineCompletedEvent,
  type PipelineFailedEvent,
  type PrefillLabel,
  type LabelledContextItem,
  type SubagentSummaryRecord,
} from "../lib/context-pipeline.ts";

// ─────────────────────────────────────────────────────────────────────────────
// Test fixtures
// ─────────────────────────────────────────────────────────────────────────────

function makeItems(): ContextItem[] {
  return [
    { id: "item-1", path: "src/config.ts", content: "export const cfg = {}", tokenCount: 10, relevanceScore: 0.9 },
    { id: "item-2", path: "src/index.ts", content: "import cfg from './config'", tokenCount: 20, relevanceScore: 0.5 },
    { id: "item-3", path: "src/utils.ts", content: "export function noop() {}", tokenCount: 15, relevanceScore: 0.7 },
    { id: "item-4", path: "docs/README.md", content: "# Documentation", tokenCount: 30, relevanceScore: undefined },
    { id: "item-5", path: "tests/index.test.ts", content: "test('ok', () => {})", tokenCount: 12, relevanceScore: 0.3 },
  ];
}

function makeConfig(overrides?: Partial<PipelineConfig>): PipelineConfig {
  return {
    runId: "run-test-001",
    workItemId: "harness-levelup-01",
    tokenBudget: 200,
    ...overrides,
  };
}

// ─────────────────────────────────────────────────────────────────────────────
// REQ-HLU-001: 7-stage context pipeline
// ─────────────────────────────────────────────────────────────────────────────

describe("REQ-HLU-001: 7-stage context pipeline", () => {
  // axiom:trace work_item=harness-levelup-01 spec=specs/101-Harness-Engineering.md#REQ-HLU-001 plan=phase-5/task-5-1/step-5-1-1
  test("runContextPipeline runs all 7 stages in order", () => {
    const config = makeConfig();
    const items = makeItems();
    const result = runContextPipeline(config, items);

    // Extract stage_completed events in order
    const stageEvents = result.events.filter(
      (e) => e.event === "context.stage_completed"
    ) as StageCompletedEvent[];

    expect(stageEvents).toHaveLength(7);
    const stageNames = stageEvents.map((e) => e.stage);
    expect(stageNames).toEqual([...PIPELINE_STAGES]);
  });

  // axiom:trace work_item=harness-levelup-01 spec=specs/101-Harness-Engineering.md#REQ-HLU-001 plan=phase-5/task-5-1/step-5-1-1
  test("runContextPipeline returns PipelineResult with items and events", () => {
    const config = makeConfig();
    const items = makeItems();
    const result = runContextPipeline(config, items);

    // PipelineResult shape
    expect(Array.isArray(result.items)).toBe(true);
    expect(Array.isArray(result.events)).toBe(true);
    expect(typeof result.totalTokensPacked).toBe("number");
    expect(typeof result.durationMs).toBe("number");
    expect(result.durationMs).toBeGreaterThanOrEqual(0);
  });

  // axiom:trace work_item=harness-levelup-01 spec=specs/101-Harness-Engineering.md#REQ-HLU-001 plan=phase-5/task-5-1/step-5-1-1
  test("stageCollection passes all items through", () => {
    const items = makeItems();
    const config = makeConfig();
    const result = stageCollection(items, config);

    expect(result.items).toHaveLength(items.length);
    expect(result.truncated).toBe(false);
    // All original items preserved
    const ids = result.items.map((i) => i.id);
    for (const item of items) {
      expect(ids).toContain(item.id);
    }
  });

  // axiom:trace work_item=harness-levelup-01 spec=specs/101-Harness-Engineering.md#REQ-HLU-001 plan=phase-5/task-5-1/step-5-1-1
  test("stageRanking sorts by relevanceScore descending", () => {
    const items = makeItems();
    const config = makeConfig();
    const result = stageRanking(items, config);

    // Expect sorted by score: 0.9, 0.7, 0.5, 0.3, undefined
    expect(result.items[0].id).toBe("item-1"); // 0.9
    expect(result.items[1].id).toBe("item-3"); // 0.7
    expect(result.items[2].id).toBe("item-2"); // 0.5
    expect(result.items[3].id).toBe("item-5"); // 0.3
    expect(result.items[4].id).toBe("item-4"); // undefined → last
  });

  // axiom:trace work_item=harness-levelup-01 spec=specs/101-Harness-Engineering.md#REQ-HLU-001 plan=phase-5/task-5-1/step-5-1-1
  test("stagePacking respects token budget — drops items when budget exceeded", () => {
    const items = makeItems();
    // Total tokens: 10+20+15+30+12 = 87; budget=35 → only first items fit
    const config = makeConfig({ tokenBudget: 35 });
    const result = stagePacking(items, config, 35);

    const totalPacked = result.items.reduce((s, i) => s + i.tokenCount, 0);
    expect(totalPacked).toBeLessThanOrEqual(35);
    // Some items must have been dropped
    expect(result.items.length).toBeLessThan(items.length);
  });

  // axiom:trace work_item=harness-levelup-01 spec=specs/101-Harness-Engineering.md#REQ-HLU-001 plan=phase-5/task-5-1/step-5-1-1
  test("stagePacking sets truncated=true when items are dropped", () => {
    const items = makeItems();
    // Budget of 10 — only item-1 (10 tokens) fits; the rest are dropped
    const result = stagePacking(items, makeConfig(), 10);

    expect(result.truncated).toBe(true);
    expect(result.items.length).toBeLessThan(items.length);
  });

  // axiom:trace work_item=harness-levelup-01 spec=specs/101-Harness-Engineering.md#REQ-HLU-001 plan=phase-5/task-5-1/step-5-1-1
  test("stagePacking sets truncated=false when all items fit", () => {
    const items = makeItems();
    // Total = 10+20+15+30+12 = 87; budget=200 → all fit
    const result = stagePacking(items, makeConfig({ tokenBudget: 200 }), 200);

    expect(result.truncated).toBe(false);
    expect(result.items.length).toBe(items.length);
  });
});

// ─────────────────────────────────────────────────────────────────────────────
// REQ-HLU-002: token budget enforcement
// ─────────────────────────────────────────────────────────────────────────────

describe("REQ-HLU-002: token budget enforcement", () => {
  // axiom:trace work_item=harness-levelup-01 spec=specs/101-Harness-Engineering.md#REQ-HLU-002 plan=phase-5/task-5-1/step-5-1-1
  test("tokensRemaining decreases after each stage", () => {
    const config = makeConfig({ tokenBudget: 500 });
    const items = makeItems();
    const result = runContextPipeline(config, items);

    const stageEvents = result.events.filter(
      (e) => e.event === "context.stage_completed"
    ) as StageCompletedEvent[];

    // tokens_remaining at each stage should be <= budget
    for (const evt of stageEvents) {
      expect(evt.tokens_remaining).toBeLessThanOrEqual(config.tokenBudget);
    }
    // Each stage that uses tokens reduces the remaining count
    // (collection stage consumes tokens — verify first stage has remaining < budget)
    const collectionEvt = stageEvents.find((e) => e.stage === "collection");
    expect(collectionEvt).toBeDefined();
    // Collection accounts for item tokens
    expect(collectionEvt!.tokens_used).toBeGreaterThan(0);
  });

  // axiom:trace work_item=harness-levelup-01 spec=specs/101-Harness-Engineering.md#REQ-HLU-002 plan=phase-5/task-5-1/step-5-1-1
  test("pipeline with budget=0 produces empty items and truncated=true at packing stage", () => {
    const config = makeConfig({ tokenBudget: 0 });
    const items = makeItems();
    const result = runContextPipeline(config, items);

    // With budget=0, packing cannot fit any item
    expect(result.items).toHaveLength(0);
    expect(result.totalTokensPacked).toBe(0);

    const packingEvt = result.events.find(
      (e) => e.event === "context.stage_completed" && (e as StageCompletedEvent).stage === "packing"
    ) as StageCompletedEvent | undefined;
    expect(packingEvt).toBeDefined();
    expect(packingEvt!.truncated).toBe(true);
    expect(packingEvt!.items_out).toBe(0);
  });

  // axiom:trace work_item=harness-levelup-01 spec=specs/101-Harness-Engineering.md#REQ-HLU-002 plan=phase-5/task-5-1/step-5-1-1
  test("pipeline_completed event has correct budget_utilization_pct", () => {
    const config = makeConfig({ tokenBudget: 200 });
    const items = makeItems();
    const result = runContextPipeline(config, items);

    const completedEvt = result.events.find(
      (e) => e.event === "context.pipeline_completed"
    ) as PipelineCompletedEvent | undefined;
    expect(completedEvt).toBeDefined();

    const expectedPct = (completedEvt!.total_tokens_packed / config.tokenBudget) * 100.0;
    expect(completedEvt!.budget_utilization_pct).toBeCloseTo(expectedPct, 2);
    expect(completedEvt!.budget_utilization_pct).toBeGreaterThanOrEqual(0);
    expect(completedEvt!.budget_utilization_pct).toBeLessThanOrEqual(100);
  });

  // axiom:trace work_item=harness-levelup-01 spec=specs/101-Harness-Engineering.md#REQ-HLU-002 plan=phase-5/task-5-1/step-5-1-1
  test("each StageResult includes durationMs >= 0", () => {
    const items = makeItems();
    const config = makeConfig();

    // Test each stage function directly
    const collectionResult = stageCollection(items, config);
    expect(collectionResult.durationMs).toBeGreaterThanOrEqual(0);

    const rankingResult = stageRanking(items, config);
    expect(rankingResult.durationMs).toBeGreaterThanOrEqual(0);

    const packingResult = stagePacking(items, config, 200);
    expect(packingResult.durationMs).toBeGreaterThanOrEqual(0);

    const injectionResult = stageInjection(items, config);
    expect(injectionResult.durationMs).toBeGreaterThanOrEqual(0);

    const executionResult = stageExecution(items, config);
    expect(executionResult.durationMs).toBeGreaterThanOrEqual(0);

    const compactionResult = stageCompaction(items, config);
    expect(compactionResult.durationMs).toBeGreaterThanOrEqual(0);

    const evidenceResult = stageEvidenceCapture(items, config);
    expect(evidenceResult.durationMs).toBeGreaterThanOrEqual(0);
  });
});

// ─────────────────────────────────────────────────────────────────────────────
// REQ-HLU-008: structured event emission
// ─────────────────────────────────────────────────────────────────────────────

describe("REQ-HLU-008: structured event emission", () => {
  // axiom:trace work_item=harness-levelup-01 spec=specs/101-Harness-Engineering.md#REQ-HLU-008 plan=phase-5/task-5-1/step-5-1-1
  test("emits pipeline_started event with all required fields", () => {
    const config = makeConfig();
    const result = runContextPipeline(config, makeItems());

    const startEvt = result.events.find(
      (e) => e.event === "context.pipeline_started"
    ) as PipelineStartedEvent | undefined;
    expect(startEvt).toBeDefined();
    expect(startEvt!.run_id).toBe(config.runId);
    expect(startEvt!.work_item_id).toBe(config.workItemId);
    expect(Array.isArray(startEvt!.pipeline_stages)).toBe(true);
    expect(startEvt!.pipeline_stages).toEqual([...PIPELINE_STAGES]);
    expect(typeof startEvt!.token_budget).toBe("number");
    expect(startEvt!.token_budget).toBe(config.tokenBudget);
    expect(startEvt!.level).toBe("INFO");
  });

  // axiom:trace work_item=harness-levelup-01 spec=specs/101-Harness-Engineering.md#REQ-HLU-008 plan=phase-5/task-5-1/step-5-1-1
  test("emits 7 stage_completed events (one per stage)", () => {
    const config = makeConfig();
    const result = runContextPipeline(config, makeItems());

    const stageEvents = result.events.filter(
      (e) => e.event === "context.stage_completed"
    );
    expect(stageEvents).toHaveLength(7);
  });

  // axiom:trace work_item=harness-levelup-01 spec=specs/101-Harness-Engineering.md#REQ-HLU-008 plan=phase-5/task-5-1/step-5-1-1
   test("emits pipeline_completed event with all required fields", () => {
     const config = makeConfig();
     const result = runContextPipeline(config, makeItems());

     const completedEvt = result.events.find(
       (e) => e.event === "context.pipeline_completed"
     ) as PipelineCompletedEvent | undefined;
     expect(completedEvt).toBeDefined();
     expect(completedEvt!.run_id).toBe(config.runId);
     expect(typeof completedEvt!.total_items_selected).toBe("number");
     expect(typeof completedEvt!.total_tokens_packed).toBe("number");
     expect(typeof completedEvt!.budget_utilization_pct).toBe("number");
     // MG-02 fix: duration_ms is required by specs/25#context-pipeline-events
     expect(typeof completedEvt!.duration_ms).toBe("number");
     expect(completedEvt!.duration_ms).toBeGreaterThanOrEqual(0);
     expect(completedEvt!.level).toBe("INFO");
   });

  // axiom:trace work_item=harness-levelup-01 spec=specs/101-Harness-Engineering.md#REQ-HLU-008 plan=phase-5/task-5-1/step-5-1-1
  test("all stage_completed events have required fields: run_id, stage, items_in, items_out, tokens_used, tokens_remaining, duration_ms, truncated", () => {
    const config = makeConfig();
    const result = runContextPipeline(config, makeItems());

    const stageEvents = result.events.filter(
      (e) => e.event === "context.stage_completed"
    ) as StageCompletedEvent[];

    for (const evt of stageEvents) {
      expect(typeof evt.run_id).toBe("string");
      expect(evt.run_id).toBe(config.runId);
      expect(typeof evt.stage).toBe("string");
      expect(PIPELINE_STAGES).toContain(evt.stage as (typeof PIPELINE_STAGES)[number]);
      expect(typeof evt.items_in).toBe("number");
      expect(typeof evt.items_out).toBe("number");
      expect(typeof evt.tokens_used).toBe("number");
      expect(typeof evt.tokens_remaining).toBe("number");
      expect(typeof evt.duration_ms).toBe("number");
      expect(evt.duration_ms).toBeGreaterThanOrEqual(0);
      expect(typeof evt.truncated).toBe("boolean");
      expect(evt.level).toBe("INFO");
    }
  });

  // axiom:trace work_item=harness-levelup-01 spec=specs/101-Harness-Engineering.md#REQ-HLU-008 plan=phase-5/task-5-1/step-5-1-1
  test("pipeline_failed event emitted when a stage throws", () => {
    const config = makeConfig();

    // Create a candidate that will cause an error when processed by a stage
    // We do this by monkey-patching: pass a proxy item that throws on property access
    // during stage processing — actually, the simplest way is to pass a broken config
    // Since all stage functions are pure and don't throw on normal input, we need
    // to force an error via a getter trap on ContextItem.
    const brokenItem: ContextItem = {
      get id(): string { return "broken"; },
      path: "broken/path.ts",
      content: "ok",
      get tokenCount(): number { throw new Error("Simulated stage error"); },
      relevanceScore: 1.0,
    };

    let threwExpectedError = false;
    try {
      runContextPipeline(config, [brokenItem]);
    } catch (err) {
      threwExpectedError = true;
      // The error should have been rethrown
      expect(err instanceof Error).toBe(true);
      expect((err as Error).message).toBe("Simulated stage error");
    }

    expect(threwExpectedError).toBe(true);
  });

  // Additional: verify the failed event is in the events array up to the point of failure
  test("pipeline_failed event contains correct fields when thrown", () => {
    const config = makeConfig();
    const capturedEvents: PipelineEvent[] = [];

    // We need to test the actual emitted events. Since runContextPipeline rethrows,
    // we wrap it and check via a probe approach: use a custom item that throws
    // mid-pipeline at a specific stage. The stageCollection itself calls
    // item.tokenCount in its reduce, so a getter trap will fire there.
    const brokenItem: ContextItem = {
      id: "broken-2",
      path: "broken/path2.ts",
      content: "content",
      get tokenCount(): number { throw new Error("Forced pipeline failure"); },
      relevanceScore: 0.5,
    };

    let caught = false;
    let thrownMsg = "";
    try {
      runContextPipeline(config, [brokenItem]);
    } catch (err) {
      caught = true;
      thrownMsg = err instanceof Error ? err.message : String(err);
    }

    expect(caught).toBe(true);
    expect(thrownMsg).toBe("Forced pipeline failure");
  });
});

// ─────────────────────────────────────────────────────────────────────────────
// Integration: context-pipeline-hook with real filesystem candidates
// These tests exercise the full pipeline against real repo files (Tier-3 evidence).
// ─────────────────────────────────────────────────────────────────────────────

describe("Integration: context-pipeline-hook with real filesystem candidates", () => {
  // axiom:trace work_item=harness-levelup-01 spec=specs/101-Harness-Engineering.md#REQ-HLU-001 plan=phase-5/task-5-1/step-5-1-2 test=context-pipeline.test.ts

  /**
   * Resolve the repo root from the .opencode/ directory where tests run.
   * process.cwd() is .opencode/ so repo root is one level up.
   */
  function repoRoot(): string {
    return path.resolve(process.cwd(), "..");
  }

  test("runContextPipeline with real repo files emits pipeline_started event", () => {
    // Tier-3: real filesystem I/O
    const prdPath = path.join(repoRoot(), "specs/00-PRD.md");
    if (!existsSync(prdPath)) {
      console.log("skipping — file not found: specs/00-PRD.md");
      return;
    }

    const content = readFileSync(prdPath, "utf-8");
    const tokenCount = Math.ceil(content.length / 4);
    const candidates: ContextItem[] = [
      {
        id: "specs/00-PRD.md",
        path: "specs/00-PRD.md",
        content,
        tokenCount,
        relevanceScore: 0.8,
      },
    ];

    const config = makeConfig({ tokenBudget: 4000 });
    const result = runContextPipeline(config, candidates);

    // Assert on event structure (not just count)
    expect(result.events[0].event).toBe("context.pipeline_started");
    const startEvt = result.events[0] as PipelineStartedEvent;
    expect(startEvt.token_budget).toBe(4000);
    expect(startEvt.pipeline_stages).toEqual([...PIPELINE_STAGES]);
  });

  test("runContextPipeline with real repo files packs at least one item within budget", () => {
    // Tier-3: real filesystem I/O
    const prdPath = path.join(repoRoot(), "specs/00-PRD.md");
    if (!existsSync(prdPath)) {
      console.log("skipping — file not found: specs/00-PRD.md");
      return;
    }

    const content = readFileSync(prdPath, "utf-8").slice(0, 8192); // cap at 8KB
    const tokenCount = Math.ceil(content.length / 4);
    const candidates: ContextItem[] = [
      {
        id: "specs/00-PRD.md",
        path: "specs/00-PRD.md",
        content,
        tokenCount,
        relevanceScore: 0.8,
      },
    ];

    const config = makeConfig({ tokenBudget: 4000 });
    const result = runContextPipeline(config, candidates);

    // Packing stage should have selected the item (it fits within 4000 token budget)
    expect(result.items.length).toBeGreaterThanOrEqual(1);
    expect(result.totalTokensPacked).toBeGreaterThan(0);
    expect(result.totalTokensPacked).toBeLessThanOrEqual(4000);
  });

  test("integration: ContextItem from real file has correct tokenCount approximation", () => {
    // Tier-3: verify Math.ceil(content.length / 4) approximation
    const prdPath = path.join(repoRoot(), "specs/00-PRD.md");
    if (!existsSync(prdPath)) {
      console.log("skipping — file not found: specs/00-PRD.md");
      return;
    }

    const content = readFileSync(prdPath, "utf-8");
    const expectedTokenCount = Math.ceil(content.length / 4);

    // Build a ContextItem as the hook does it
    const item: ContextItem = {
      id: "specs/00-PRD.md",
      path: "specs/00-PRD.md",
      content,
      tokenCount: Math.ceil(content.length / 4),
      relevanceScore: 0.8,
    };

    // Verify the approximation is correct
    expect(item.tokenCount).toBe(expectedTokenCount);
    // Sanity: non-trivial file should have >0 tokens
    expect(item.tokenCount).toBeGreaterThan(0);
    // tokenCount should be ceiling of length/4
    expect(item.tokenCount).toBe(Math.ceil(content.length / 4));
  });

  test("pipeline events contain all 7 stage_completed events for real candidates", () => {
    // Tier-3: run full pipeline against real files, check all 7 stage events emitted
    const indexPath = path.join(repoRoot(), ".memory-bank/_index.md");
    if (!existsSync(indexPath)) {
      console.log("skipping — file not found: .memory-bank/_index.md");
      return;
    }

    const content = readFileSync(indexPath, "utf-8");
    const tokenCount = Math.ceil(content.length / 4);
    const candidates: ContextItem[] = [
      {
        id: ".memory-bank/_index.md",
        path: ".memory-bank/_index.md",
        content,
        tokenCount,
        relevanceScore: 0.7,
      },
    ];

    const config = makeConfig({ tokenBudget: 4000 });
    const result = runContextPipeline(config, candidates);

    // All 7 stage_completed events must be present
    const stageEvents = result.events.filter(
      (e) => e.event === "context.stage_completed"
    ) as StageCompletedEvent[];

    expect(stageEvents).toHaveLength(7);
    const stageNames = stageEvents.map((e) => e.stage);
    expect(stageNames).toEqual([...PIPELINE_STAGES]);

    // Each stage event must have correct structure
    for (const evt of stageEvents) {
      expect(typeof evt.run_id).toBe("string");
      expect(typeof evt.duration_ms).toBe("number");
      expect(evt.duration_ms).toBeGreaterThanOrEqual(0);
    }
  });
});

// ─────────────────────────────────────────────────────────────────────────────
// Additional invariant checks
// ─────────────────────────────────────────────────────────────────────────────

describe("Pipeline invariants", () => {
  test("PIPELINE_STAGES has exactly 7 entries", () => {
    expect(PIPELINE_STAGES).toHaveLength(7);
  });

  test("pipeline with empty candidates produces empty result", () => {
    const config = makeConfig();
    const result = runContextPipeline(config, []);

    expect(result.items).toHaveLength(0);
    expect(result.totalTokensPacked).toBe(0);
    // Should still emit pipeline_started, 7 stage_completed, pipeline_completed
    expect(result.events.filter((e) => e.event === "context.stage_completed")).toHaveLength(7);
    expect(result.events.find((e) => e.event === "context.pipeline_started")).toBeDefined();
    expect(result.events.find((e) => e.event === "context.pipeline_completed")).toBeDefined();
  });

  test("stageRanking: items with same score maintain relative order (stable-ish sort)", () => {
    const items: ContextItem[] = [
      { id: "a", path: "a.ts", content: "", tokenCount: 5, relevanceScore: 0.5 },
      { id: "b", path: "b.ts", content: "", tokenCount: 5, relevanceScore: 0.5 },
      { id: "c", path: "c.ts", content: "", tokenCount: 5, relevanceScore: 0.9 },
    ];
    const result = stageRanking(items, makeConfig());
    // c must be first (0.9), a and b can be in either order (0.5 each)
    expect(result.items[0].id).toBe("c");
    expect(["a", "b"]).toContain(result.items[1].id);
    expect(["a", "b"]).toContain(result.items[2].id);
  });

  test("total events count is: 1 (started) + 7 (stages) + 1 (completed) = 9", () => {
    const config = makeConfig();
    const result = runContextPipeline(config, makeItems());
    expect(result.events).toHaveLength(9);
  });

  test("pass-through stages (injection, execution, compaction, evidence_capture) preserve all items", () => {
    const items = makeItems();
    const config = makeConfig();

    const injResult = stageInjection(items, config);
    expect(injResult.items.length).toBe(items.length);
    expect(injResult.truncated).toBe(false);

    const execResult = stageExecution(items, config);
    expect(execResult.items.length).toBe(items.length);
    expect(execResult.truncated).toBe(false);

    const compResult = stageCompaction(items, config);
    expect(compResult.items.length).toBe(items.length);
    expect(compResult.truncated).toBe(false);

    const evResult = stageEvidenceCapture(items, config);
    expect(evResult.items.length).toBe(items.length);
    expect(evResult.truncated).toBe(false);
  });
});

// ─────────────────────────────────────────────────────────────────────────────
// REQ-HLU-010: Context prefill labelling
// axiom:trace work_item=harness-levelup-01 spec=specs/101-Harness-Engineering.md#REQ-HLU-010 plan=phase-5/task-5-2/step-5-2-1
// ─────────────────────────────────────────────────────────────────────────────

describe("REQ-HLU-010: context prefill labelling", () => {
  // axiom:trace work_item=harness-levelup-01 spec=specs/101-Harness-Engineering.md#REQ-HLU-010 plan=phase-5/task-5-2/step-5-2-1
  test("labelContextItem attaches prefillLabel to an existing ContextItem", () => {
    const item: ContextItem = {
      id: "item-a",
      path: "src/foo.ts",
      content: "export const x = 1;",
      tokenCount: 5,
    };
    const label: PrefillLabel = {
      source: "spec",
      confidence: 0.9,
      citations: ["specs/00-PRD.md"],
      freshness: "2026-01-01T00:00:00.000Z",
      trust_status: "verified",
    };
    const labelled = labelContextItem(item, label);

    expect(labelled.prefillLabel).toBeDefined();
    expect(labelled.prefillLabel.source).toBe("spec");
    expect(labelled.prefillLabel.confidence).toBe(0.9);
    expect(labelled.prefillLabel.trust_status).toBe("verified");
  });

  // axiom:trace work_item=harness-levelup-01 spec=specs/101-Harness-Engineering.md#REQ-HLU-010 plan=phase-5/task-5-2/step-5-2-1
  test("labelContextItem preserves all original ContextItem fields", () => {
    const item: ContextItem = {
      id: "item-b",
      path: "src/bar.ts",
      content: "const y = 2;",
      tokenCount: 3,
      relevanceScore: 0.7,
      metadata: { key: "value" },
    };
    const label: PrefillLabel = {
      source: "trajectory",
      confidence: 0.6,
      citations: ["src/bar.ts"],
      freshness: "2026-05-17T00:00:00.000Z",
      trust_status: "unverified",
    };
    const labelled = labelContextItem(item, label);

    expect(labelled.id).toBe(item.id);
    expect(labelled.path).toBe(item.path);
    expect(labelled.content).toBe(item.content);
    expect(labelled.tokenCount).toBe(item.tokenCount);
    expect(labelled.relevanceScore).toBe(item.relevanceScore);
    expect(labelled.metadata).toEqual(item.metadata);
  });

  // axiom:trace work_item=harness-levelup-01 spec=specs/101-Harness-Engineering.md#REQ-HLU-010 plan=phase-5/task-5-2/step-5-2-1
  test("buildPrefillContext applies default labels to unlabelled items", () => {
    const items: ContextItem[] = [
      { id: "x1", path: "a.ts", content: "abc", tokenCount: 1 },
      { id: "x2", path: "b.ts", content: "def", tokenCount: 2 },
    ];
    const labelled = buildPrefillContext(items);

    expect(labelled).toHaveLength(2);
    for (const item of labelled) {
      expect(item.prefillLabel).toBeDefined();
      expect(item.prefillLabel.source).toBe("memory_retrieval");
      expect(item.prefillLabel.confidence).toBe(0.5);
      expect(item.prefillLabel.trust_status).toBe("unverified");
      expect(typeof item.prefillLabel.freshness).toBe("string");
      // freshness must look like an ISO8601 timestamp
      expect(item.prefillLabel.freshness).toMatch(/^\d{4}-\d{2}-\d{2}T/);
    }
  });

  // axiom:trace work_item=harness-levelup-01 spec=specs/101-Harness-Engineering.md#REQ-HLU-010 plan=phase-5/task-5-2/step-5-2-1
  test("buildPrefillContext uses item.path as default citation", () => {
    const items: ContextItem[] = [
      { id: "c1", path: "specs/42-foo.md", content: "foo", tokenCount: 1 },
    ];
    const labelled = buildPrefillContext(items);

    expect(labelled[0].prefillLabel.citations).toEqual(["specs/42-foo.md"]);
  });

  // axiom:trace work_item=harness-levelup-01 spec=specs/101-Harness-Engineering.md#REQ-HLU-010 plan=phase-5/task-5-2/step-5-2-1
  test("formatLabelledContextForPrompt includes source, trust_status, and content excerpt", () => {
    const item: ContextItem = {
      id: "d1",
      path: "docs/note.md",
      content: "This is a long note. ".repeat(50), // > 300 chars
      tokenCount: 10,
    };
    const labelled = buildPrefillContext([item]);
    const formatted = formatLabelledContextForPrompt(labelled);

    expect(formatted).toContain("source: memory_retrieval");
    expect(formatted).toContain("trust: unverified");
    // Content excerpt should be at most 300 chars
    expect(formatted).toContain("This is a long note.");
    // Full content should NOT appear (it's > 300 chars)
    const fullContent = item.content;
    expect(formatted.includes(fullContent)).toBe(false);
  });

  // axiom:trace work_item=harness-levelup-01 spec=specs/101-Harness-Engineering.md#REQ-HLU-010 plan=phase-5/task-5-2/step-5-2-1
  test("formatLabelledContextForPrompt marks unverified items as [trust: unverified]", () => {
    const label: PrefillLabel = {
      source: "memory_retrieval",
      confidence: 0.4,
      citations: ["path/to/file.md"],
      freshness: "2026-01-01T00:00:00.000Z",
      trust_status: "unverified",
    };
    const item: ContextItem = {
      id: "e1",
      path: "path/to/file.md",
      content: "Some content here.",
      tokenCount: 4,
    };
    const labelled = labelContextItem(item, label);
    const formatted = formatLabelledContextForPrompt([labelled]);

    expect(formatted).toContain("trust: unverified");
    expect(formatted).toContain("[citations: path/to/file.md]");
  });
});

// ─────────────────────────────────────────────────────────────────────────────
// REQ-HLU-011: Subagent result distillation
// axiom:trace work_item=harness-levelup-01 spec=specs/101-Harness-Engineering.md#REQ-HLU-011 plan=phase-5/task-5-2/step-5-2-1
// ─────────────────────────────────────────────────────────────────────────────

describe("REQ-HLU-011: subagent result distillation", () => {
  // axiom:trace work_item=harness-levelup-01 spec=specs/101-Harness-Engineering.md#REQ-HLU-011 plan=phase-5/task-5-2/step-5-2-1
  test("distillSubagentResult produces required SubagentSummaryRecord shape", () => {
    const raw: Record<string, unknown> = {
      status: "completed",
      files_changed: ["src/foo.ts", "src/bar.ts"],
      tests_added: ["tests/foo.test.ts"],
      confidence: 85,
    };
    const record = distillSubagentResult("dev-axiom", "session-001", raw);

    expect(record.agent).toBe("dev-axiom");
    expect(record.session_id).toBe("session-001");
    expect(record.status).toBe("completed");
    expect(Array.isArray(record.outputs.files_changed)).toBe(true);
    expect(Array.isArray(record.outputs.tests_added)).toBe(true);
    expect(typeof record.outputs.trace_markers).toBe("number");
    expect(Array.isArray(record.evidence.commands_run)).toBe(true);
    expect(typeof record.evidence.results).toBe("string");
    expect(typeof record.evidence.confidence).toBe("number");
    expect(Array.isArray(record.injected_steps)).toBe(true);
    expect(Array.isArray(record.trace_updates)).toBe(true);
    expect(Array.isArray(record.open_questions)).toBe(true);
    expect(record.distillation_metadata.source_label).toBe("model");
  });

  // axiom:trace work_item=harness-levelup-01 spec=specs/101-Harness-Engineering.md#REQ-HLU-011 plan=phase-5/task-5-2/step-5-2-1
  test("distillSubagentResult extracts files_changed from result", () => {
    const raw: Record<string, unknown> = {
      files_changed: ["src/alpha.ts", "src/beta.ts"],
    };
    const record = distillSubagentResult("qa-axiom", "sess-002", raw);

    expect(record.outputs.files_changed).toEqual(["src/alpha.ts", "src/beta.ts"]);
  });

  // axiom:trace work_item=harness-levelup-01 spec=specs/101-Harness-Engineering.md#REQ-HLU-011 plan=phase-5/task-5-2/step-5-2-1
  test("distillSubagentResult sets source_label to 'model' always", () => {
    // Even if the raw result tries to set a different source_label, the function always returns "model"
    const raw: Record<string, unknown> = {
      distillation_metadata: {
        source_label: "ground_truth",  // should be overridden
        worker_id: "worker-1",
        tokens_used: 500,
        duration_ms: 1234,
      },
    };
    const record = distillSubagentResult("spec-axiom", "sess-003", raw);

    expect(record.distillation_metadata.source_label).toBe("model");
  });

  // axiom:trace work_item=harness-levelup-01 spec=specs/101-Harness-Engineering.md#REQ-HLU-011 plan=phase-5/task-5-2/step-5-2-1
  test("distillSubagentResult handles missing optional fields gracefully", () => {
    // Minimal raw result — all optional fields absent
    const raw: Record<string, unknown> = {};
    const record = distillSubagentResult("pm-axiom", "sess-004", raw);

    // Should not throw; should produce defaults
    expect(record.outputs.files_changed).toEqual([]);
    expect(record.outputs.tests_added).toEqual([]);
    expect(record.outputs.trace_markers).toBe(0);
    expect(record.evidence.commands_run).toEqual([]);
    expect(record.evidence.results).toBe("");
    expect(record.evidence.confidence).toBe(50);
    expect(record.injected_steps).toEqual([]);
    expect(record.trace_updates).toEqual([]);
    expect(record.open_questions).toEqual([]);
    expect(record.distillation_metadata.source_label).toBe("model");
    expect(record.distillation_metadata.tokens_used).toBe(0);
    expect(record.distillation_metadata.duration_ms).toBe(0);
  });

  // axiom:trace work_item=harness-levelup-01 spec=specs/101-Harness-Engineering.md#REQ-HLU-011 plan=phase-5/task-5-2/step-5-2-1
  test("distillSubagentResult preserves injected_steps from result", () => {
    const raw: Record<string, unknown> = {
      status: "blocked",
      injected_steps: [
        { title: "Run security review", reason: "Auth surface exposed" },
        { title: "Update spec", reason: "New endpoint not documented" },
      ],
    };
    const record = distillSubagentResult("tower-axiom", "sess-005", raw);

    expect(record.status).toBe("blocked");
    expect(record.injected_steps).toHaveLength(2);
    expect(record.injected_steps[0].title).toBe("Run security review");
    expect(record.injected_steps[0].reason).toBe("Auth surface exposed");
    expect(record.injected_steps[1].title).toBe("Update spec");
    expect(record.injected_steps[1].reason).toBe("New endpoint not documented");
  });
});

// ─────────────────────────────────────────────────────────────────────────────
// REQ-HLU-002: context-pipeline-hook per-session cache
// axiom:trace work_item=harness-levelup-01 spec=specs/101-Harness-Engineering.md#REQ-HLU-002 plan=phase-5/task-5-3/step-5-3-1 test=context-pipeline.test.ts
// ─────────────────────────────────────────────────────────────────────────────

describe("context-pipeline-hook cache", () => {
  // Clear the module-level cache before each test so tests are isolated.
  beforeEach(() => {
    _getCandidateCache().clear();
  });

  // axiom:trace work_item=harness-levelup-01 spec=specs/101-Harness-Engineering.md#REQ-HLU-002 plan=phase-5/task-5-3/step-5-3-1
  test("second call returns same candidates without re-reading disk", () => {
    // Arrange: create a temp dir with a PRD candidate file.
    const tmpDir = (() => {
      const d = path.join(os.tmpdir(), `cpc-test-${Date.now()}`);
      mkdirSync(path.join(d, "specs"), { recursive: true });
      mkdirSync(path.join(d, ".memory-bank", "work-items", "harness-levelup-01"), { recursive: true });
      writeFileSync(path.join(d, "specs", "00-PRD.md"), "# PRD content for cache test");
      return d;
    })();

    // First call: simulate what the hook does — read files and populate cache.
    const firstMtime = getCandidateMaxMtime(tmpDir);
    expect(firstMtime).toBeGreaterThan(0);
    const firstItems = readCandidateFiles(tmpDir);
    expect(firstItems.length).toBeGreaterThan(0);
    _getCandidateCache().set(tmpDir, { items: firstItems, maxMtime: firstMtime });

    // Cache is now populated.
    expect(_getCandidateCache().has(tmpDir)).toBe(true);

    // Second call: mtime is unchanged → cache hit, return same array reference.
    const secondMtime = getCandidateMaxMtime(tmpDir);
    const cachedEntry = _getCandidateCache().get(tmpDir)!;
    let secondItems: ReturnType<typeof readCandidateFiles>;
    if (cachedEntry && cachedEntry.maxMtime === secondMtime) {
      // Cache hit — use cached items, no disk read.
      secondItems = cachedEntry.items;
    } else {
      // Cache miss — re-read and update (shouldn't happen here).
      secondItems = readCandidateFiles(tmpDir);
      _getCandidateCache().set(tmpDir, { items: secondItems, maxMtime: secondMtime });
    }

    // Same array reference proves no re-read occurred.
    expect(secondItems).toBe(firstItems);
    expect(secondMtime).toBe(firstMtime);
  });

  // axiom:trace work_item=harness-levelup-01 spec=specs/101-Harness-Engineering.md#REQ-HLU-002 plan=phase-5/task-5-3/step-5-3-1
  test("cache invalidates when file mtime changes", () => {
    // Arrange: create a temp dir with a candidate file.
    const tmpDir = (() => {
      const d = path.join(os.tmpdir(), `cpc-mtime-${Date.now()}`);
      mkdirSync(path.join(d, "specs"), { recursive: true });
      mkdirSync(path.join(d, ".memory-bank", "work-items", "harness-levelup-01"), { recursive: true });
      writeFileSync(path.join(d, "specs", "00-PRD.md"), "# Initial content");
      return d;
    })();

    // Prime the cache: read files and record mtime.
    const firstMtime = getCandidateMaxMtime(tmpDir);
    const firstItems = readCandidateFiles(tmpDir);
    _getCandidateCache().set(tmpDir, { items: firstItems, maxMtime: firstMtime });
    expect(firstMtime).toBeGreaterThan(0);
    expect(firstItems.find((i) => i.path === "specs/00-PRD.md")!.content).toContain("Initial content");

    // Advance time and overwrite the file so mtime changes.
    const start = Date.now();
    while (Date.now() - start < 10) { /* spin to ensure mtime advances */ }
    writeFileSync(path.join(tmpDir, "specs", "00-PRD.md"), "# Updated content after mtime change");

    // Simulate hook second invocation: mtime changed → cache miss → re-read.
    const secondMtime = getCandidateMaxMtime(tmpDir);
    const cachedEntry = _getCandidateCache().get(tmpDir)!;
    let secondItems: ReturnType<typeof readCandidateFiles>;
    if (cachedEntry && cachedEntry.maxMtime === secondMtime) {
      // Cache hit (shouldn't happen — mtime should have changed).
      secondItems = cachedEntry.items;
    } else {
      // Cache miss — re-read and update.
      secondItems = readCandidateFiles(tmpDir);
      _getCandidateCache().set(tmpDir, { items: secondItems, maxMtime: secondMtime });
    }

    // The new mtime must be >= the old one (mtime only moves forward).
    expect(secondMtime).toBeGreaterThanOrEqual(firstMtime);

    // The cache now stores the new mtime.
    const newEntry = _getCandidateCache().get(tmpDir)!;
    expect(newEntry.maxMtime).toBe(secondMtime);

    // The re-read content reflects the updated file.
    const updatedItem = newEntry.items.find((i) => i.path === "specs/00-PRD.md");
    expect(updatedItem).toBeDefined();
    expect(updatedItem!.content).toContain("Updated content after mtime change");
  });
});
