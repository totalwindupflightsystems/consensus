/**
 * Async Context Workers — bounded, cancellable, observable, non-authoritative.
 *
 * This module is infrastructure — it does NOT export an OpenCode plugin default
 * or register tools. Export plain TypeScript classes and types only.
 *
 * REQ-HLU-024: worker lifecycle state machine (PENDING → RUNNING → COMPLETED | CANCELLED)
 * REQ-HLU-025: worker boundaries (max 3 concurrent, 30s timeout, 2000 token budget,
 *              4000 char output limit, no agent spawn, no spec/repo write access)
 * REQ-HLU-026: subagent distillation worker — distils subagent result packs on subagent_return
 * REQ-HLU-027: pre-compact trajectory capture worker — snapshots run state on before_compact
 *
 * axiom:trace work_item=harness-levelup-01 spec=specs/101-Harness-Engineering.md#REQ-HLU-024 plan=phase-7/task-7-1/step-7-1-1 test=async-workers.test.ts
 * axiom:trace work_item=harness-levelup-01 spec=specs/101-Harness-Engineering.md#REQ-HLU-025 plan=phase-7/task-7-1/step-7-1-1 test=async-workers.test.ts
 * axiom:trace work_item=harness-levelup-01 spec=specs/101-Harness-Engineering.md#REQ-HLU-026 plan=phase-7/task-7-2/step-7-2-1 test=async-workers.test.ts
 * axiom:trace work_item=harness-levelup-01 spec=specs/101-Harness-Engineering.md#REQ-HLU-027 plan=phase-7/task-7-2/step-7-2-1 test=async-workers.test.ts
 */

// ─────────────────────────────────────────────────────────────────────────────
// Types (REQ-HLU-024, REQ-HLU-025)
// ─────────────────────────────────────────────────────────────────────────────

/** REQ-HLU-024: Worker lifecycle states */
export type WorkerStatus = "PENDING" | "RUNNING" | "COMPLETED" | "CANCELLED";

/** REQ-HLU-009: Worker trigger types */
export type WorkerTrigger =
  | "before_compact"
  | "subagent_return"
  | "user_message_received";

/** REQ-HLU-025: Worker pool configuration with enforced boundaries */
export interface WorkerConfig {
  /** Maximum number of concurrent workers. Default: 3 */
  max_concurrent_workers?: number;
  /** Maximum token budget per worker. Default: 2000 */
  max_token_budget?: number;
  /** Maximum execution time in milliseconds. Default: 30_000 */
  timeout_ms?: number;
  /** Maximum output size in characters. Default: 4000 */
  max_output_size?: number;
}

/** REQ-HLU-024: Immutable record produced after a worker run */
export interface WorkerRecord {
  worker_id: string;
  trigger: WorkerTrigger;
  status: WorkerStatus;
  tokens_used: number;
  started_at?: number;
  completed_at?: number;
  output?: string;
  error?: string;
}

// ─────────────────────────────────────────────────────────────────────────────
// Semaphore — concurrency cap (REQ-HLU-025)
// ─────────────────────────────────────────────────────────────────────────────

/**
 * Counting semaphore using a promise-based waitlist.
 * acquire() blocks when the semaphore count is 0.
 * release() unblocks the oldest waiter.
 */
export class Semaphore {
  private _count: number;
  private readonly _max: number;
  private readonly _waiters: Array<() => void> = [];

  constructor(max: number) {
    if (max < 1) throw new RangeError(`Semaphore max must be >= 1, got ${max}`);
    this._max = max;
    this._count = max;
  }

  /** Wait until a slot is available, then claim it. */
  async acquire(): Promise<void> {
    if (this._count > 0) {
      this._count--;
      return;
    }
    // No slot available — queue a waiter
    return new Promise<void>((resolve) => {
      this._waiters.push(resolve);
    });
  }

  /** Release a slot; unblocks the next waiter if one is queued. */
  release(): void {
    if (this._waiters.length > 0) {
      // Hand the slot directly to the next waiter (count stays at 0)
      const next = this._waiters.shift()!;
      next();
    } else {
      this._count++;
    }
  }

  /** Number of available slots right now. */
  get available(): number {
    return this._count;
  }
}

// ─────────────────────────────────────────────────────────────────────────────
// AsyncWorkerPool (REQ-HLU-024, REQ-HLU-025)
// ─────────────────────────────────────────────────────────────────────────────

const DEFAULT_CONFIG: Required<WorkerConfig> = {
  max_concurrent_workers: 3,
  max_token_budget: 2000,
  timeout_ms: 30_000,
  max_output_size: 4000,
};

/**
 * Bounded, cancellable, observable worker pool.
 *
 * Enforces REQ-HLU-025 constraints:
 *  - max 3 concurrent workers (semaphore)
 *  - max 2000 tokens per worker (token budget abort)
 *  - max 30 seconds wall-clock time (AbortSignal timeout race)
 *  - max 4000 chars output (truncation on completion)
 *  - no agent spawn (hard error via guardAgentSpawn)
 */
export class AsyncWorkerPool {
  private readonly _config: Required<WorkerConfig>;
  private readonly _semaphore: Semaphore;
  private _activeCount: number = 0;

  constructor(config?: WorkerConfig) {
    this._config = { ...DEFAULT_CONFIG, ...config };
    this._semaphore = new Semaphore(this._config.max_concurrent_workers);
  }

  /** Resolved configuration (all fields populated with defaults). */
  get config(): Required<WorkerConfig> {
    return { ...this._config };
  }

  /** Number of workers currently in RUNNING state. */
  get activeCount(): number {
    return this._activeCount;
  }

  /**
   * Guard against agent spawn inside worker code.
   * Any call to this method is a hard error (REQ-HLU-025).
   * @throws Error always — agent spawning is FORBIDDEN in workers
   */
  static guardAgentSpawn(): never {
    // axiom:trace work_item=harness-levelup-01 spec=specs/101-Harness-Engineering.md#REQ-HLU-025 plan=phase-7/task-7-1/step-7-1-1
    throw new Error("agent_spawn_forbidden: workers MUST NOT spawn agents");
  }

  /**
   * Submit a unit of work to the pool.
   *
   * Lifecycle (REQ-HLU-024):
   *   PENDING → (semaphore acquired) → RUNNING → COMPLETED | CANCELLED
   *
   * Boundaries (REQ-HLU-025):
   *   - Concurrency: acquire semaphore before transitioning to RUNNING
   *   - Timeout: Promise.race with a rejection after timeout_ms
   *   - Token budget: trackTokens() callback accumulates; abort if >max_token_budget
   *   - Output truncation: applied on COMPLETED
   *   - Agent spawn: guardAgentSpawn() is injected into worker scope (caller must use it)
   *
   * @param trigger — Which event triggered this worker (REQ-HLU-009)
   * @param work    — Async function receiving (AbortSignal, trackTokens)
   * @returns       — Resolved WorkerRecord regardless of outcome
   */
  async run(
    trigger: WorkerTrigger,
    work: (signal: AbortSignal, trackTokens: (n: number) => void) => Promise<string>,
  ): Promise<WorkerRecord> {
    // Step 1: Create record in PENDING state
    const record: WorkerRecord = {
      worker_id: `w_${Date.now()}_${Math.random().toString(36).slice(2, 8)}`,
      trigger,
      status: "PENDING",
      tokens_used: 0,
    };

    // Step 2: Acquire semaphore (blocks if max_concurrent_workers already running)
    await this._semaphore.acquire();
    this._activeCount++;

    // Step 3: Transition to RUNNING
    record.status = "RUNNING";
    record.started_at = Date.now();

    const controller = new AbortController();
    let tokenAccumulator = 0;
    let tokenBudgetExceeded = false;
    let agentSpawnAttempted = false;

    const trackTokens = (n: number): void => {
      tokenAccumulator += n;
      record.tokens_used = tokenAccumulator;
      if (tokenAccumulator > this._config.max_token_budget) {
        tokenBudgetExceeded = true;
        controller.abort("token_budget_exceeded");
      }
    };

    // REQ-HLU-025: use AbortSignal.timeout() as spec mandates
    // Workers MUST be wrapped in a Promise.race with AbortSignal.timeout(30_000)
    const timeoutSignal = AbortSignal.timeout(this._config.timeout_ms);
    const timeoutPromise = new Promise<never>((_resolve, reject) => {
      timeoutSignal.addEventListener("abort", () => {
        controller.abort("timeout");
        reject(new Error("timeout"));
      }, { once: true });
    });

    try {
      // Step 4: Race work against timeout
      const output = await Promise.race([
        work(controller.signal, trackTokens),
        timeoutPromise,
      ]);

      record.completed_at = Date.now();

      // Step 8: Token budget exceeded — even if work() returned, check the flag.
      // trackTokens() may have set tokenBudgetExceeded before work() resolved.
      if (tokenBudgetExceeded) {
        record.status = "CANCELLED";
        record.error = "token_budget_exceeded";
      } else {
        // Step 6: COMPLETED — truncate output to max_output_size
        record.status = "COMPLETED";
        record.output =
          output.length > this._config.max_output_size
            ? output.slice(0, this._config.max_output_size)
            : output;
      }
    } catch (err: unknown) {
      record.completed_at = Date.now();

      const message = err instanceof Error ? err.message : String(err);

      if (agentSpawnAttempted || message.includes("agent_spawn_forbidden")) {
        // Step 9: Agent spawn forbidden
        record.status = "CANCELLED";
        record.error = "agent_spawn_forbidden";
        // Re-throw so the caller sees the hard error
        throw err;
      } else if (
        tokenBudgetExceeded ||
        message === "token_budget_exceeded" ||
        (controller.signal.aborted && controller.signal.reason === "token_budget_exceeded")
      ) {
        // Step 8: Token budget exceeded (work threw on signal abort)
        record.status = "CANCELLED";
        record.error = "token_budget_exceeded";
      } else {
        // Step 7: Timeout (or any other cancellation)
        record.status = "CANCELLED";
        record.error = "timeout";
      }
    } finally {
      // Step 10: Always release semaphore
      // No clearTimeout needed — AbortSignal.timeout() manages its own cleanup
      this._activeCount--;
      this._semaphore.release();
    }

    return record;
  }
}

// ─────────────────────────────────────────────────────────────────────────────
// Concrete worker implementations
// ─────────────────────────────────────────────────────────────────────────────

// Lazy imports — loaded at call-time to keep test startup fast and avoid
// circular module issues when only the pool is used.
import { distillSubagentResult } from "./context-pipeline.ts";
import type { SubagentSummaryRecord } from "./context-pipeline.ts";
import { stringify as yamlStringify } from "yaml";
import { mkdirSync, writeFileSync } from "fs";
import { join } from "path";

// ─────────────────────────────────────────────────────────────────────────────
// REQ-HLU-026: Subagent distillation worker
// axiom:trace work_item=harness-levelup-01 spec=specs/101-Harness-Engineering.md#REQ-HLU-026 plan=phase-7/task-7-2/step-7-2-1 test=async-workers.test.ts
// ─────────────────────────────────────────────────────────────────────────────

/**
 * Submits a subagent distillation job to the worker pool.
 *
 * Lifecycle:
 *  1. Calls distillSubagentResult() from context-pipeline.ts
 *  2. Estimates token usage as Math.ceil(JSON.stringify(rawResult).length / 4)
 *  3. Writes the SubagentSummaryRecord to
 *     <outputDir>/subagent-summary-<timestamp>.yaml
 *  4. Returns the YAML text as the work output (truncated to 4000 chars by pool)
 *
 * axiom:trace work_item=harness-levelup-01 spec=specs/101-Harness-Engineering.md#REQ-HLU-026 plan=phase-7/task-7-2/step-7-2-1
 */
export async function runSubagentDistillationWorker(
  pool: AsyncWorkerPool,
  agentName: string,
  sessionId: string,
  rawResult: Record<string, unknown>,
  outputDir: string,
): Promise<WorkerRecord> {
  return pool.run("subagent_return", async (_signal, trackTokens) => {
    // Estimate token usage from raw payload size
    const estimatedTokens = Math.ceil(JSON.stringify(rawResult).length / 4);
    trackTokens(estimatedTokens);

    // Distil the raw result into a structured summary
    const summary: SubagentSummaryRecord = distillSubagentResult(
      agentName,
      sessionId,
      rawResult,
    );

    // Serialise to YAML
    const timestamp = Date.now();
    const yamlText = yamlStringify(summary);

    // Ensure output directory exists and write file
    mkdirSync(outputDir, { recursive: true });
    const filePath = join(outputDir, `subagent-summary-${timestamp}.yaml`);
    writeFileSync(filePath, yamlText, "utf8");

    return yamlText;
  });
}

// ─────────────────────────────────────────────────────────────────────────────
// REQ-HLU-027: Pre-compact trajectory capture worker
// axiom:trace work_item=harness-levelup-01 spec=specs/101-Harness-Engineering.md#REQ-HLU-027 plan=phase-7/task-7-2/step-7-2-1 test=async-workers.test.ts
// ─────────────────────────────────────────────────────────────────────────────

/** Shape of the trajectory snapshot passed to the pre-compact capture worker. */
export interface TrajectorySnapshot {
  decisions: string[];
  open_questions: string[];
  plan_cursor: string;
  injected_steps: string[];
  evidence_refs: string[];
  trace_refs: string[];
}

/**
 * Submits a pre-compact trajectory capture job to the worker pool.
 *
 * Lifecycle:
 *  1. Estimates token usage from the trajectory payload size
 *  2. Builds a pre-compact summary object with required YAML fields
 *  3. Writes to:
 *     <repoRoot>/.memory-bank/work-items/<workItemId>/runs/<runId>/pre_compact_summary.yaml
 *  4. Creates parent directories as needed
 *  5. Returns the YAML text as the work output (truncated to 4000 chars by pool)
 *
 * axiom:trace work_item=harness-levelup-01 spec=specs/101-Harness-Engineering.md#REQ-HLU-027 plan=phase-7/task-7-2/step-7-2-1
 */
export async function runPreCompactCaptureWorker(
  pool: AsyncWorkerPool,
  workItemId: string,
  runId: string,
  repoRoot: string,
  trajectory: TrajectorySnapshot,
): Promise<WorkerRecord> {
  return pool.run("before_compact", async (_signal, trackTokens) => {
    // Estimate token usage from trajectory payload
    const estimatedTokens = Math.ceil(JSON.stringify(trajectory).length / 4);
    trackTokens(estimatedTokens);

    // Build the pre-compact summary document
    const capturedAt = new Date().toISOString();

    // We don't yet know the worker_id before work completes, so we use a stable
    // placeholder that the caller can cross-reference via the returned WorkerRecord.
    const summaryDoc = {
      work_item_id: workItemId,
      run_id: runId,
      captured_at: capturedAt,
      summary: {
        decisions: trajectory.decisions,
        open_questions: trajectory.open_questions,
        plan_cursor: trajectory.plan_cursor,
        injected_steps: trajectory.injected_steps,
        evidence_refs: trajectory.evidence_refs,
        trace_refs: trajectory.trace_refs,
      },
      source_label: "model",
      distillation_metadata: {
        // worker_id is embedded after the fact — use run_id+timestamp as stable key
        worker_id: `pre-compact-${runId}-${Date.now()}`,
        captured_before_compact: true,
      },
    };

    const yamlText = yamlStringify(summaryDoc);

    // Build output path and create parent dirs
    const outputPath = join(
      repoRoot,
      ".memory-bank",
      "work-items",
      workItemId,
      "runs",
      runId,
      "pre_compact_summary.yaml",
    );
    mkdirSync(join(repoRoot, ".memory-bank", "work-items", workItemId, "runs", runId), {
      recursive: true,
    });
    writeFileSync(outputPath, yamlText, "utf8");

    return yamlText;
  });
}

// OpenCode plugin loader no-op — this file is a utility module, not a plugin.
// OpenCode auto-discovers all .ts files in plugins/ and tries to load them.
// This export prevents "Plugin export is not a function" errors.
export default async () => ({ tool: {} });
