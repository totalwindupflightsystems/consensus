/**
 * Context Pipeline — 7-stage context collection, ranking, packing, injection,
 * execution, compaction, and evidence capture with structured event emission.
 *
 * This module is infrastructure — it does NOT export an OpenCode plugin default
 * or register tools. Export plain TypeScript functions and types only.
 *
 * REQ-HLU-001: models all 7 stages as observable pipeline stages
 * REQ-HLU-002: each stage defines token budgets, truncation behavior, and failure semantics
 * REQ-HLU-008: stages emit bounded structured events per specs/25-Structured-Logging-Events.md#context-pipeline-events
 *
 * axiom:trace work_item=harness-levelup-01 spec=specs/101-Harness-Engineering.md#REQ-HLU-001 plan=phase-5/task-5-1/step-5-1-1 test=context-pipeline.test.ts
 */

// ─────────────────────────────────────────────────────────────────────────────
// Types (REQ-HLU-001)
// ─────────────────────────────────────────────────────────────────────────────

export interface ContextItem {
  id: string;
  path: string;
  content: string;
  tokenCount: number;
  relevanceScore?: number;
  metadata?: Record<string, unknown>;
}

export interface PipelineConfig {
  runId: string;
  workItemId: string;
  tokenBudget: number;
}

export interface StageResult {
  items: ContextItem[];
  tokensUsed: number;
  truncated: boolean;
  durationMs: number;
}

export interface PipelineResult {
  items: ContextItem[];
  events: PipelineEvent[];
  totalTokensPacked: number;
  durationMs: number;
}

// ─────────────────────────────────────────────────────────────────────────────
// Event Types (REQ-HLU-008, specs/25-Structured-Logging-Events.md#context-pipeline-events)
// ─────────────────────────────────────────────────────────────────────────────

export interface PipelineStartedEvent {
  event: "context.pipeline_started";
  level: "INFO";
  run_id: string;
  work_item_id: string;
  pipeline_stages: string[];
  token_budget: number;
}

export interface StageCompletedEvent {
  event: "context.stage_completed";
  level: "INFO";
  run_id: string;
  stage: string;
  items_in: number;
  items_out: number;
  tokens_used: number;
  tokens_remaining: number;
  duration_ms: number;
  truncated: boolean;
}

export interface PipelineCompletedEvent {
  event: "context.pipeline_completed";
  level: "INFO";
  run_id: string;
  total_items_selected: number;
  total_tokens_packed: number;
  budget_utilization_pct: number;
  duration_ms: number;  // REQ-HLU-008: required by specs/25#context-pipeline-events
}

export interface PipelineFailedEvent {
  event: "context.pipeline_failed";
  level: "ERROR";
  run_id: string;
  stage: string;
  error: string;
  fallback_used: boolean;
}

// Union of all 4 event types
export type PipelineEvent =
  | PipelineStartedEvent
  | StageCompletedEvent
  | PipelineCompletedEvent
  | PipelineFailedEvent;

// ─────────────────────────────────────────────────────────────────────────────
// Stage name constants (REQ-HLU-001)
// ─────────────────────────────────────────────────────────────────────────────

// axiom:trace work_item=harness-levelup-01 spec=specs/101-Harness-Engineering.md#REQ-HLU-001 plan=phase-5/task-5-1/step-5-1-1
export const PIPELINE_STAGES = [
  "collection",
  "ranking",
  "packing",
  "injection",
  "execution",
  "compaction",
  "evidence_capture",
] as const;

export type StageName = (typeof PIPELINE_STAGES)[number];

// ─────────────────────────────────────────────────────────────────────────────
// Event emission (REQ-HLU-008)
// Writes to stderr as newline-delimited JSON; also returns the event for tests.
// ─────────────────────────────────────────────────────────────────────────────

// axiom:trace work_item=harness-levelup-01 spec=specs/101-Harness-Engineering.md#REQ-HLU-008 plan=phase-5/task-5-1/step-5-1-1
function emitEvent(event: PipelineEvent): void {
  // Gate event emission behind AXIOM_CONTEXT_PIPELINE_DEBUG=1 — without this,
  // OpenCode's UI captures stderr from plugin hooks and floods the conversation
  // pane with NDJSON lines on every session.prompt. Tests/eval-runner can opt in.
  if (process.env.AXIOM_CONTEXT_PIPELINE_DEBUG !== "1") return;
  process.stderr.write(
    JSON.stringify({ ...event, timestamp: new Date().toISOString() }) + "\n"
  );
}

// ─────────────────────────────────────────────────────────────────────────────
// Individual stage functions (REQ-HLU-002)
// Each stage is pure and testable — same inputs always produce same outputs.
// ─────────────────────────────────────────────────────────────────────────────

/**
 * Collection stage: accepts all candidates, sums their token counts.
 * No filtering at this stage — all items pass through.
 * axiom:trace work_item=harness-levelup-01 spec=specs/101-Harness-Engineering.md#REQ-HLU-002 plan=phase-5/task-5-1/step-5-1-1
 */
export function stageCollection(
  items: ContextItem[],
  _config: PipelineConfig
): StageResult {
  const start = Date.now();
  const tokensUsed = items.reduce((sum, item) => sum + item.tokenCount, 0);
  const durationMs = Date.now() - start;
  return {
    items: [...items],
    tokensUsed,
    truncated: false,
    durationMs,
  };
}

/**
 * Ranking stage: sorts items by relevanceScore descending.
 * Items without a relevanceScore sort to the end.
 * axiom:trace work_item=harness-levelup-01 spec=specs/101-Harness-Engineering.md#REQ-HLU-002 plan=phase-5/task-5-1/step-5-1-1
 */
export function stageRanking(
  items: ContextItem[],
  _config: PipelineConfig
): StageResult {
  const start = Date.now();
  const sorted = [...items].sort((a, b) => {
    const aScore = a.relevanceScore ?? -Infinity;
    const bScore = b.relevanceScore ?? -Infinity;
    return bScore - aScore;
  });
  const tokensUsed = sorted.reduce((sum, item) => sum + item.tokenCount, 0);
  const durationMs = Date.now() - start;
  return {
    items: sorted,
    tokensUsed,
    truncated: false,
    durationMs,
  };
}

/**
 * Packing stage: greedily selects items from the ranked list until the token
 * budget is exhausted. Sets truncated=true if any items were dropped.
 * axiom:trace work_item=harness-levelup-01 spec=specs/101-Harness-Engineering.md#REQ-HLU-002 plan=phase-5/task-5-1/step-5-1-1
 */
export function stagePacking(
  items: ContextItem[],
  _config: PipelineConfig,
  tokensRemaining: number
): StageResult {
  const start = Date.now();
  const selected: ContextItem[] = [];
  let tokensUsed = 0;

  for (const item of items) {
    if (tokensUsed + item.tokenCount <= tokensRemaining) {
      selected.push(item);
      tokensUsed += item.tokenCount;
    }
    // If item doesn't fit, skip it (greedy — don't try smaller items later)
  }

  const truncated = selected.length < items.length;
  const durationMs = Date.now() - start;
  return {
    items: selected,
    tokensUsed,
    truncated,
    durationMs,
  };
}

/**
 * Injection stage: pass-through for now.
 * Injection formatting is performed by the caller (OpenCode session).
 * axiom:trace work_item=harness-levelup-01 spec=specs/101-Harness-Engineering.md#REQ-HLU-002 plan=phase-5/task-5-1/step-5-1-1
 */
export function stageInjection(
  items: ContextItem[],
  _config: PipelineConfig
): StageResult {
  const start = Date.now();
  const tokensUsed = items.reduce((sum, item) => sum + item.tokenCount, 0);
  const durationMs = Date.now() - start;
  return {
    items: [...items],
    tokensUsed,
    truncated: false,
    durationMs,
  };
}

/**
 * Execution stage: pass-through observation point.
 * The actual LLM call is the OpenCode runtime; this stage observes what was used.
 * axiom:trace work_item=harness-levelup-01 spec=specs/101-Harness-Engineering.md#REQ-HLU-002 plan=phase-5/task-5-1/step-5-1-1
 */
export function stageExecution(
  items: ContextItem[],
  _config: PipelineConfig
): StageResult {
  const start = Date.now();
  const tokensUsed = items.reduce((sum, item) => sum + item.tokenCount, 0);
  const durationMs = Date.now() - start;
  return {
    items: [...items],
    tokensUsed,
    truncated: false,
    durationMs,
  };
}

/**
 * Compaction stage: pass-through for now.
 * Context compaction is handled at the OpenCode level.
 * axiom:trace work_item=harness-levelup-01 spec=specs/101-Harness-Engineering.md#REQ-HLU-002 plan=phase-5/task-5-1/step-5-1-1
 */
export function stageCompaction(
  items: ContextItem[],
  _config: PipelineConfig
): StageResult {
  const start = Date.now();
  const tokensUsed = items.reduce((sum, item) => sum + item.tokenCount, 0);
  const durationMs = Date.now() - start;
  return {
    items: [...items],
    tokensUsed,
    truncated: false,
    durationMs,
  };
}

/**
 * Evidence capture stage: pass-through; records what context was used.
 * Durable evidence writing is handled by the caller or harness integration.
 * axiom:trace work_item=harness-levelup-01 spec=specs/101-Harness-Engineering.md#REQ-HLU-002 plan=phase-5/task-5-1/step-5-1-1
 */
export function stageEvidenceCapture(
  items: ContextItem[],
  _config: PipelineConfig
): StageResult {
  const start = Date.now();
  const tokensUsed = items.reduce((sum, item) => sum + item.tokenCount, 0);
  const durationMs = Date.now() - start;
  return {
    items: [...items],
    tokensUsed,
    truncated: false,
    durationMs,
  };
}

// ─────────────────────────────────────────────────────────────────────────────
// REQ-HLU-010: Context prefill labelling
// Agents MUST label source, confidence, citations, freshness, and trust status
// so downstream agents do not treat retrieved context as higher authority than
// specs or repo evidence.
// axiom:trace work_item=harness-levelup-01 spec=specs/101-Harness-Engineering.md#REQ-HLU-010 plan=phase-5/task-5-2/step-5-2-1
// ─────────────────────────────────────────────────────────────────────────────

export interface PrefillLabel {
  source: string;          // e.g., "memory_retrieval", "spec", "trajectory"
  confidence: number;      // 0.0–1.0
  citations: string[];     // file paths or IDs of source documents
  freshness: string;       // ISO8601 timestamp of last update
  trust_status: "verified" | "unverified" | "stale";
}

export interface LabelledContextItem extends ContextItem {
  prefillLabel: PrefillLabel;
}

/**
 * Attaches a PrefillLabel to a ContextItem, producing a LabelledContextItem.
 * Original item fields are preserved unchanged.
 * axiom:trace work_item=harness-levelup-01 spec=specs/101-Harness-Engineering.md#REQ-HLU-010 plan=phase-5/task-5-2/step-5-2-1
 */
export function labelContextItem(
  item: ContextItem,
  label: PrefillLabel
): LabelledContextItem {
  return { ...item, prefillLabel: { ...label } };
}

/**
 * Applies labels to a list of ContextItems.
 * Items that already have a prefillLabel (LabelledContextItem) keep it.
 * Items without a label get the defaultLabel merged with per-item defaults:
 *   { source: "memory_retrieval", confidence: 0.5, citations: [item.path],
 *     freshness: new Date().toISOString(), trust_status: "unverified" }
 * axiom:trace work_item=harness-levelup-01 spec=specs/101-Harness-Engineering.md#REQ-HLU-010 plan=phase-5/task-5-2/step-5-2-1
 */
export function buildPrefillContext(
  items: ContextItem[],
  defaultLabel?: Partial<PrefillLabel>
): LabelledContextItem[] {
  const now = new Date().toISOString();
  return items.map((item) => {
    // If already labelled, preserve existing label
    if ("prefillLabel" in item && item.prefillLabel !== undefined) {
      return item as LabelledContextItem;
    }
    const perItemDefault: PrefillLabel = {
      source: "memory_retrieval",
      confidence: 0.5,
      citations: [item.path],
      freshness: now,
      trust_status: "unverified",
    };
    const merged: PrefillLabel = { ...perItemDefault, ...defaultLabel };
    // Ensure citations falls back to item.path if defaultLabel didn't provide it
    if (!defaultLabel?.citations) {
      merged.citations = [item.path];
    }
    return { ...item, prefillLabel: merged };
  });
}

/**
 * Formats a list of LabelledContextItems into a string suitable for injection
 * into a system prompt. Each item appears as:
 *
 *   [source: <source> | confidence: <confidence> | trust: <trust_status>]
 *   <first 300 chars of content>
 *   [citations: <citations joined by ", ">]
 *
 * axiom:trace work_item=harness-levelup-01 spec=specs/101-Harness-Engineering.md#REQ-HLU-010 plan=phase-5/task-5-2/step-5-2-1
 */
export function formatLabelledContextForPrompt(items: LabelledContextItem[]): string {
  return items
    .map((item) => {
      const { source, confidence, citations, trust_status } = item.prefillLabel;
      const excerpt = item.content.slice(0, 300);
      const citationStr = citations.join(", ");
      return (
        `[source: ${source} | confidence: ${confidence} | trust: ${trust_status}]\n` +
        `${excerpt}\n` +
        `[citations: ${citationStr}]`
      );
    })
    .join("\n\n");
}

// ─────────────────────────────────────────────────────────────────────────────
// REQ-HLU-011: Subagent result distillation
// The harness SHOULD distill subagent result packs into durable summaries.
// axiom:trace work_item=harness-levelup-01 spec=specs/101-Harness-Engineering.md#REQ-HLU-011 plan=phase-5/task-5-2/step-5-2-1
// ─────────────────────────────────────────────────────────────────────────────

export interface SubagentSummaryRecord {
  agent: string;
  session_id: string;
  status: "completed" | "failed" | "blocked";
  outputs: {
    files_changed: string[];
    tests_added: string[];
    trace_markers: number;
  };
  evidence: {
    commands_run: string[];
    results: string;
    confidence: number;
  };
  injected_steps: Array<{ title: string; reason: string }>;
  trace_updates: string[];
  open_questions: string[];
  distillation_metadata: {
    worker_id: string;
    tokens_used: number;
    duration_ms: number;
    source_label: "model";  // always "model" — model-generated, not ground truth
  };
}

/**
 * Distils a raw subagent result pack into a structured SubagentSummaryRecord.
 *
 * Extraction rules:
 *  - files_changed: result.files_changed or result.evidence?.files_changed or []
 *  - tests_added: result.tests_added or []
 *  - trace_markers: count of "axiom:trace" occurrences in all string values of result
 *  - confidence: result.confidence (number) or 50
 *  - injected_steps: result.injected_steps or []
 *  - trace_updates: result.trace_updates or []
 *  - open_questions: result.open_questions or []
 *  - distillation_metadata.source_label is always "model"
 *
 * axiom:trace work_item=harness-levelup-01 spec=specs/101-Harness-Engineering.md#REQ-HLU-011 plan=phase-5/task-5-2/step-5-2-1
 */
export function distillSubagentResult(
  agentName: string,
  sessionId: string,
  rawResult: Record<string, unknown>
): SubagentSummaryRecord {
  // ── Helper: safely extract a string array from a result field ──────────────
  function extractStringArray(value: unknown): string[] {
    if (Array.isArray(value)) {
      return value.filter((v): v is string => typeof v === "string");
    }
    return [];
  }

  // ── Helper: count "axiom:trace" occurrences across all string values ──────
  function countTraceMarkers(obj: Record<string, unknown>): number {
    let count = 0;
    const search = (v: unknown): void => {
      if (typeof v === "string") {
        // Count all non-overlapping matches in this string
        const matches = v.match(/axiom:trace/g);
        if (matches) count += matches.length;
      } else if (Array.isArray(v)) {
        v.forEach(search);
      } else if (v !== null && typeof v === "object") {
        Object.values(v as Record<string, unknown>).forEach(search);
      }
    };
    Object.values(obj).forEach(search);
    return count;
  }

  // ── files_changed ─────────────────────────────────────────────────────────
  const filesChanged: string[] =
    extractStringArray(rawResult["files_changed"]).length > 0
      ? extractStringArray(rawResult["files_changed"])
      : extractStringArray(
          (rawResult["evidence"] as Record<string, unknown> | undefined)?.["files_changed"]
        );

  // ── tests_added ───────────────────────────────────────────────────────────
  const testsAdded = extractStringArray(rawResult["tests_added"]);

  // ── trace markers ─────────────────────────────────────────────────────────
  const traceMarkers = countTraceMarkers(rawResult);

  // ── status ────────────────────────────────────────────────────────────────
  const rawStatus = rawResult["status"];
  const status: "completed" | "failed" | "blocked" =
    rawStatus === "completed" || rawStatus === "failed" || rawStatus === "blocked"
      ? rawStatus
      : "completed";

  // ── evidence ──────────────────────────────────────────────────────────────
  const evidenceObj = rawResult["evidence"];
  const commandsRun =
    evidenceObj !== null && typeof evidenceObj === "object"
      ? extractStringArray((evidenceObj as Record<string, unknown>)["commands_run"])
      : [];
  const results =
    evidenceObj !== null && typeof evidenceObj === "object"
      ? String((evidenceObj as Record<string, unknown>)["results"] ?? "")
      : "";
  const confidence =
    typeof rawResult["confidence"] === "number" ? rawResult["confidence"] : 50;

  // ── injected_steps ────────────────────────────────────────────────────────
  const rawInjected = rawResult["injected_steps"];
  const injectedSteps: Array<{ title: string; reason: string }> = Array.isArray(rawInjected)
    ? rawInjected
        .filter((s): s is Record<string, unknown> => s !== null && typeof s === "object")
        .map((s) => ({
          title: String(s["title"] ?? ""),
          reason: String(s["reason"] ?? ""),
        }))
    : [];

  // ── trace_updates / open_questions ────────────────────────────────────────
  const traceUpdates = extractStringArray(rawResult["trace_updates"]);
  const openQuestions = extractStringArray(rawResult["open_questions"]);

  // ── distillation_metadata ─────────────────────────────────────────────────
  const metaObj = rawResult["distillation_metadata"];
  const meta =
    metaObj !== null && typeof metaObj === "object"
      ? (metaObj as Record<string, unknown>)
      : {};
  const workerId = typeof meta["worker_id"] === "string" ? meta["worker_id"] : agentName;
  const tokensUsed = typeof meta["tokens_used"] === "number" ? meta["tokens_used"] : 0;
  const durationMs = typeof meta["duration_ms"] === "number" ? meta["duration_ms"] : 0;

  return {
    agent: agentName,
    session_id: sessionId,
    status,
    outputs: {
      files_changed: filesChanged,
      tests_added: testsAdded,
      trace_markers: traceMarkers,
    },
    evidence: {
      commands_run: commandsRun,
      results,
      confidence,
    },
    injected_steps: injectedSteps,
    trace_updates: traceUpdates,
    open_questions: openQuestions,
    distillation_metadata: {
      worker_id: workerId,
      tokens_used: tokensUsed,
      duration_ms: durationMs,
      source_label: "model",
    },
  };
}

// ─────────────────────────────────────────────────────────────────────────────
// Main orchestrator (REQ-HLU-001, REQ-HLU-008)
// ─────────────────────────────────────────────────────────────────────────────

/**
 * runContextPipeline: orchestrates all 7 stages in order, emitting structured
 * events at each boundary. Returns the final packed items and all events emitted.
 *
 * axiom:trace work_item=harness-levelup-01 spec=specs/101-Harness-Engineering.md#REQ-HLU-001 plan=phase-5/task-5-1/step-5-1-1 test=context-pipeline.test.ts
 */
export function runContextPipeline(
  config: PipelineConfig,
  candidates: ContextItem[]
): PipelineResult {
  const pipelineStart = Date.now();
  const events: PipelineEvent[] = [];

  // Helper: collect and emit in one call
  function emit(event: PipelineEvent): void {
    events.push(event);
    emitEvent(event);
  }

  // 1. Emit pipeline_started
  emit({
    event: "context.pipeline_started",
    level: "INFO",
    run_id: config.runId,
    work_item_id: config.workItemId,
    pipeline_stages: [...PIPELINE_STAGES],
    token_budget: config.tokenBudget,
  });

  let currentItems = candidates;
  let tokensRemaining = config.tokenBudget;
  let currentStage: StageName = "collection";

  // Stage runner helper
  function runStage(
    stageName: StageName,
    stageFn: (items: ContextItem[], cfg: PipelineConfig, ...args: number[]) => StageResult,
    extraArgs: number[] = []
  ): ContextItem[] {
    currentStage = stageName;
    const itemsIn = currentItems.length;
    const stageResult = stageFn(currentItems, config, ...extraArgs);
    // Track token consumption (subtract used tokens from remaining)
    tokensRemaining -= stageResult.tokensUsed;

    emit({
      event: "context.stage_completed",
      level: "INFO",
      run_id: config.runId,
      stage: stageName,
      items_in: itemsIn,
      items_out: stageResult.items.length,
      tokens_used: stageResult.tokensUsed,
      tokens_remaining: tokensRemaining,
      duration_ms: stageResult.durationMs,
      truncated: stageResult.truncated,
    });

    return stageResult.items;
  }

  try {
    // Run all 7 stages in order — output of each becomes input for the next
    currentItems = runStage("collection", stageCollection);
    currentItems = runStage("ranking", stageRanking);
    // Packing uses the remaining budget after collection+ranking consumed tokens
    const packingBudget = config.tokenBudget;
    currentItems = runStage("packing", (items, cfg) => stagePacking(items, cfg, packingBudget), []);
    currentItems = runStage("injection", stageInjection);
    currentItems = runStage("execution", stageExecution);
    currentItems = runStage("compaction", stageCompaction);
    currentItems = runStage("evidence_capture", stageEvidenceCapture);
  } catch (err) {
    const errorMsg = err instanceof Error ? err.message : String(err);
    emit({
      event: "context.pipeline_failed",
      level: "ERROR",
      run_id: config.runId,
      stage: currentStage,
      error: errorMsg,
      fallback_used: false,
    });
    throw err;
  }

  // Compute total tokens packed from final items
  const totalTokensPacked = currentItems.reduce(
    (sum, item) => sum + item.tokenCount,
    0
  );
  const budgetUtilizationPct =
    config.tokenBudget > 0
      ? Math.min(100.0, (totalTokensPacked / config.tokenBudget) * 100.0)
      : 0.0;

  const durationMs = Date.now() - pipelineStart;

  emit({
    event: "context.pipeline_completed",
    level: "INFO",
    run_id: config.runId,
    total_items_selected: currentItems.length,
    total_tokens_packed: totalTokensPacked,
    budget_utilization_pct: budgetUtilizationPct,
    duration_ms: durationMs,  // MG-02 fix: was missing, required by specs/25
  });
  return {
    items: currentItems,
    events,
    totalTokensPacked,
    durationMs,
  };
}

// OpenCode plugin loader no-op — this file is a utility module, not a plugin.
// OpenCode auto-discovers all .ts files in plugins/ and tries to load them.
// This export prevents "Plugin export is not a function" errors.
export default async () => ({ tool: {} });
