/**
 * Tests for async-workers.ts — REQ-HLU-024, REQ-HLU-025.
 *
 * REQ-HLU-024: worker lifecycle state machine (PENDING → RUNNING → COMPLETED | CANCELLED)
 * REQ-HLU-025: worker boundaries — max 3 concurrent, 30s timeout, 2000 token budget,
 *              4000 char output limit, no agent spawn, no spec/repo write access
 *
 * Run: cd .opencode && bun test tests/async-workers.test.ts
 *
 * axiom:trace work_item=harness-levelup-01 spec=specs/101-Harness-Engineering.md#REQ-HLU-024 plan=phase-7/task-7-1/step-7-1-1 test=async-workers.test.ts
 * axiom:trace work_item=harness-levelup-01 spec=specs/101-Harness-Engineering.md#REQ-HLU-025 plan=phase-7/task-7-1/step-7-1-1 test=async-workers.test.ts
 */

import { test, expect, describe } from "bun:test";
import { mkdtempSync, readFileSync, existsSync } from "fs";
import { tmpdir } from "os";
import { join } from "path";
import { parse as yamlParse } from "yaml";
import {
  Semaphore,
  AsyncWorkerPool,
  runSubagentDistillationWorker,
  runPreCompactCaptureWorker,
} from "../lib/async-workers.ts";
import type { WorkerRecord } from "../lib/async-workers.ts";

// ─────────────────────────────────────────────────────────────────────────────
// Helper: a small async sleep that respects an AbortSignal
// ─────────────────────────────────────────────────────────────────────────────
function sleep(ms: number, signal?: AbortSignal): Promise<void> {
  return new Promise<void>((resolve, reject) => {
    const handle = setTimeout(resolve, ms);
    signal?.addEventListener("abort", () => {
      clearTimeout(handle);
      reject(new Error(String(signal.reason ?? "aborted")));
    });
  });
}

// ─────────────────────────────────────────────────────────────────────────────
// REQ-HLU-024: Worker lifecycle
// ─────────────────────────────────────────────────────────────────────────────

describe("REQ-HLU-024: worker lifecycle", () => {
  test("worker status is COMPLETED after successful run", async () => {
    const pool = new AsyncWorkerPool({ timeout_ms: 1000 });
    const record = await pool.run("before_compact", async (_signal, _track) => {
      return "hello";
    });

    expect(record.status).toBe("COMPLETED");
    expect(record.output).toBe("hello");
    expect(record.error).toBeUndefined();
  });

  test("WorkerRecord contains worker_id, trigger, tokens_used, started_at, completed_at", async () => {
    const pool = new AsyncWorkerPool({ timeout_ms: 1000 });
    const record = await pool.run("subagent_return", async (_signal, _track) => {
      return "result";
    });

    expect(record.worker_id).toMatch(/^w_\d+_[a-z0-9]+$/);
    expect(record.trigger).toBe("subagent_return");
    expect(typeof record.tokens_used).toBe("number");
    expect(typeof record.started_at).toBe("number");
    expect(typeof record.completed_at).toBe("number");
    expect(record.completed_at!).toBeGreaterThanOrEqual(record.started_at!);
  });

  test("cancelled worker has status CANCELLED", async () => {
    // Token budget exhaustion is the easiest reliable cancellation path
    const pool = new AsyncWorkerPool({ max_token_budget: 10, timeout_ms: 1000 });
    const record = await pool.run("user_message_received", async (_signal, trackTokens) => {
      trackTokens(11); // Exceeds budget immediately
      return "should not reach here";
    });

    expect(record.status).toBe("CANCELLED");
  });

  test("worker started_at is set before work begins and completed_at after", async () => {
    const pool = new AsyncWorkerPool({ timeout_ms: 1000 });
    const before = Date.now();
    const record = await pool.run("before_compact", async (_signal, _track) => {
      await sleep(5);
      return "done";
    });
    const after = Date.now();

    expect(record.started_at!).toBeGreaterThanOrEqual(before);
    expect(record.completed_at!).toBeLessThanOrEqual(after);
    expect(record.completed_at!).toBeGreaterThan(record.started_at!);
  });
});

// ─────────────────────────────────────────────────────────────────────────────
// REQ-HLU-025: Concurrency cap (max 3 workers)
// ─────────────────────────────────────────────────────────────────────────────

describe("REQ-HLU-025: concurrency cap (max 3 workers)", () => {
  test("can run up to 3 workers simultaneously", async () => {
    const pool = new AsyncWorkerPool({ max_concurrent_workers: 3, timeout_ms: 2000 });
    let concurrentPeak = 0;
    let active = 0;

    const work = async (_signal: AbortSignal, _track: (n: number) => void) => {
      active++;
      concurrentPeak = Math.max(concurrentPeak, active);
      await sleep(30);
      active--;
      return "done";
    };

    // Launch 3 workers simultaneously
    const results = await Promise.all([
      pool.run("before_compact", work),
      pool.run("before_compact", work),
      pool.run("before_compact", work),
    ]);

    expect(concurrentPeak).toBe(3);
    expect(results.every((r) => r.status === "COMPLETED")).toBe(true);
  });

  test("4th worker waits until one of the 3 running workers completes", async () => {
    const pool = new AsyncWorkerPool({ max_concurrent_workers: 3, timeout_ms: 2000 });
    const completionOrder: number[] = [];

    const makeWork = (id: number, delay: number) =>
      async (_signal: AbortSignal, _track: (n: number) => void) => {
        await sleep(delay);
        completionOrder.push(id);
        return `worker-${id}`;
      };

    // Workers 1-3 each sleep 50ms; worker 4 sleeps 5ms but must wait for a slot
    const [r1, r2, r3, r4] = await Promise.all([
      pool.run("before_compact", makeWork(1, 50)),
      pool.run("before_compact", makeWork(2, 50)),
      pool.run("before_compact", makeWork(3, 50)),
      pool.run("before_compact", makeWork(4, 5)), // 4th — will be queued
    ]);

    // All should complete successfully
    expect(r1.status).toBe("COMPLETED");
    expect(r2.status).toBe("COMPLETED");
    expect(r3.status).toBe("COMPLETED");
    expect(r4.status).toBe("COMPLETED");

    // Worker 4 (5ms once it starts) should complete AFTER at least one of 1-3 finishes
    // It must appear at position >=1 in completionOrder (after 1, 2, or 3)
    expect(completionOrder.length).toBe(4);
    const pos4 = completionOrder.indexOf(4);
    expect(pos4).toBeGreaterThanOrEqual(1);
  });

  test("semaphore.available decrements while workers run", async () => {
    const sem = new Semaphore(3);
    expect(sem.available).toBe(3);

    await sem.acquire();
    expect(sem.available).toBe(2);

    await sem.acquire();
    expect(sem.available).toBe(1);

    await sem.acquire();
    expect(sem.available).toBe(0);

    sem.release();
    expect(sem.available).toBe(1);
  });

  test("semaphore.available returns to max after all workers complete", async () => {
    const pool = new AsyncWorkerPool({ max_concurrent_workers: 3, timeout_ms: 1000 });

    await Promise.all([
      pool.run("before_compact", async () => { await sleep(10); return "a"; }),
      pool.run("before_compact", async () => { await sleep(10); return "b"; }),
      pool.run("before_compact", async () => { await sleep(10); return "c"; }),
    ]);

    // After all workers complete, the semaphore should be fully available again
    expect(pool.config.max_concurrent_workers).toBe(3);
    expect(pool.activeCount).toBe(0);

    // Verify we can immediately run 3 more workers (all slots free)
    const results = await Promise.all([
      pool.run("before_compact", async () => "1"),
      pool.run("before_compact", async () => "2"),
      pool.run("before_compact", async () => "3"),
    ]);
    expect(results.every((r) => r.status === "COMPLETED")).toBe(true);
  });
});

// ─────────────────────────────────────────────────────────────────────────────
// REQ-HLU-025: Token budget enforcement (2000 token limit)
// ─────────────────────────────────────────────────────────────────────────────

describe("REQ-HLU-025: token budget enforcement (2000 token limit)", () => {
  test("worker with total tokens <= 2000 completes normally", async () => {
    const pool = new AsyncWorkerPool({ max_token_budget: 2000, timeout_ms: 1000 });
    const record = await pool.run("subagent_return", async (_signal, trackTokens) => {
      trackTokens(500);
      trackTokens(1000);
      trackTokens(499); // total = 1999 — under budget
      return "finished";
    });

    expect(record.status).toBe("COMPLETED");
    expect(record.tokens_used).toBe(1999);
    expect(record.output).toBe("finished");
  });

  test("worker cancelled after trackTokens exceeds 2000 tokens", async () => {
    const pool = new AsyncWorkerPool({ max_token_budget: 2000, timeout_ms: 1000 });
    const record = await pool.run("subagent_return", async (signal, trackTokens) => {
      trackTokens(2001); // immediately exceeds budget
      // Work should still try to read signal and abort
      await sleep(50, signal);
      return "should not reach here";
    });

    expect(record.status).toBe("CANCELLED");
    expect(record.error).toBe("token_budget_exceeded");
  });

  test("cancelled worker has status CANCELLED and error=token_budget_exceeded", async () => {
    const pool = new AsyncWorkerPool({ max_token_budget: 100, timeout_ms: 1000 });
    const record = await pool.run("before_compact", async (signal, trackTokens) => {
      for (let i = 0; i < 10; i++) {
        trackTokens(20); // 10 × 20 = 200 tokens — exceeds 100
        await sleep(5, signal);
      }
      return "never";
    });

    expect(record.status).toBe("CANCELLED");
    expect(record.error).toBe("token_budget_exceeded");
    expect(record.tokens_used).toBeGreaterThan(100);
  });
});

// ─────────────────────────────────────────────────────────────────────────────
// REQ-HLU-025: Time limit enforcement (30s timeout — but use short timeouts in tests)
// ─────────────────────────────────────────────────────────────────────────────

describe("REQ-HLU-025: time limit enforcement (30s timeout)", () => {
  test("fast worker (10ms sleep) completes within time limit", async () => {
    const pool = new AsyncWorkerPool({ timeout_ms: 500 });
    const record = await pool.run("user_message_received", async (signal, _track) => {
      await sleep(10, signal);
      return "fast";
    });

    expect(record.status).toBe("COMPLETED");
    expect(record.output).toBe("fast");
  });

  test("slow worker (5s sleep with 100ms timeout) is cancelled with error=timeout", async () => {
    const pool = new AsyncWorkerPool({ timeout_ms: 100 });
    const record = await pool.run("before_compact", async (signal, _track) => {
      await sleep(5000, signal); // 5 second sleep — far beyond 100ms timeout
      return "too slow";
    });

    expect(record.status).toBe("CANCELLED");
    expect(record.error).toBe("timeout");
  });

  test("timed-out worker has status CANCELLED", async () => {
    const pool = new AsyncWorkerPool({ timeout_ms: 50 });
    const record = await pool.run("subagent_return", async (_signal, _track) => {
      // Does NOT respect the abort signal — simulates unbounded sync work
      await new Promise((resolve) => setTimeout(resolve, 2000));
      return "never";
    });

    expect(record.status).toBe("CANCELLED");
  });

  test("AbortSignal.timeout() is the timeout mechanism (not setTimeout)", async () => {
    // REQ-HLU-025: spec mandates AbortSignal.timeout() not plain setTimeout.
    // This test proves the mechanism cancels workers even when they do NOT check
    // the abort signal themselves — the Promise.race with the AbortSignal.timeout()
    // listener still fires and rejects the race, producing error: "timeout".
    const pool = new AsyncWorkerPool({ timeout_ms: 50 });

    const start = Date.now();
    const record = await pool.run("before_compact", async (_signal, _track) => {
      // Intentionally ignores the signal — simulates a worker that cannot be
      // cooperatively cancelled. AbortSignal.timeout() must still terminate it.
      await new Promise((resolve) => setTimeout(resolve, 500));
      return "should never return";
    });
    const elapsed = Date.now() - start;

    // Worker must be CANCELLED with error "timeout" ...
    expect(record.status).toBe("CANCELLED");
    expect(record.error).toBe("timeout");
    // ... and it must have been cut off well before the 500ms the work requested
    expect(elapsed).toBeLessThan(200);
  });
});

// ─────────────────────────────────────────────────────────────────────────────
// REQ-HLU-025: No-spawn guard (hard error)
// ─────────────────────────────────────────────────────────────────────────────

describe("REQ-HLU-025: no-spawn guard (hard error)", () => {
  test("AsyncWorkerPool.guardAgentSpawn() throws Error with 'agent_spawn_forbidden'", () => {
    expect(() => AsyncWorkerPool.guardAgentSpawn()).toThrow("agent_spawn_forbidden");
  });

  test("calling guardAgentSpawn inside work() cancels the worker", async () => {
    const pool = new AsyncWorkerPool({ timeout_ms: 1000 });
    let caughtError: Error | undefined;

    try {
      await pool.run("subagent_return", async (_signal, _track) => {
        AsyncWorkerPool.guardAgentSpawn();
        return "never";
      });
    } catch (err) {
      caughtError = err as Error;
    }

    // guardAgentSpawn re-throws — the pool.run() promise rejects
    expect(caughtError).toBeDefined();
    expect(caughtError!.message).toContain("agent_spawn_forbidden");
  });
});

// ─────────────────────────────────────────────────────────────────────────────
// REQ-HLU-025: Output size truncation (4000 char limit)
// ─────────────────────────────────────────────────────────────────────────────

describe("REQ-HLU-025: output size truncation (4000 char limit)", () => {
  test("output larger than 4000 chars is truncated to 4000", async () => {
    const pool = new AsyncWorkerPool({ max_output_size: 4000, timeout_ms: 1000 });
    const bigOutput = "x".repeat(5000);
    const record = await pool.run("before_compact", async () => bigOutput);

    expect(record.status).toBe("COMPLETED");
    expect(record.output!.length).toBe(4000);
  });

  test("output <= 4000 chars is not truncated", async () => {
    const pool = new AsyncWorkerPool({ max_output_size: 4000, timeout_ms: 1000 });
    const smallOutput = "y".repeat(100);
    const record = await pool.run("before_compact", async () => smallOutput);

    expect(record.status).toBe("COMPLETED");
    expect(record.output).toBe(smallOutput);
    expect(record.output!.length).toBe(100);
  });
});

// ─────────────────────────────────────────────────────────────────────────────
// Semaphore — unit tests
// ─────────────────────────────────────────────────────────────────────────────

describe("Semaphore unit tests", () => {
  test("throws on max < 1", () => {
    expect(() => new Semaphore(0)).toThrow();
  });

  test("acquire/release cycle keeps available count correct", async () => {
    const sem = new Semaphore(2);
    expect(sem.available).toBe(2);

    await sem.acquire();
    await sem.acquire();
    expect(sem.available).toBe(0);

    sem.release();
    expect(sem.available).toBe(1);

    sem.release();
    expect(sem.available).toBe(2);
  });

  test("waiters are unblocked in FIFO order", async () => {
    const sem = new Semaphore(1);
    await sem.acquire(); // holds the single slot

    const order: number[] = [];
    const p1 = sem.acquire().then(() => { order.push(1); sem.release(); });
    const p2 = sem.acquire().then(() => { order.push(2); sem.release(); });
    const p3 = sem.acquire().then(() => { order.push(3); sem.release(); });

    sem.release(); // unblocks p1
    await Promise.all([p1, p2, p3]);
    expect(order).toEqual([1, 2, 3]);
  });
});

// ─────────────────────────────────────────────────────────────────────────────
// REQ-HLU-026: Subagent distillation worker
// axiom:trace work_item=harness-levelup-01 spec=specs/101-Harness-Engineering.md#REQ-HLU-026 plan=phase-7/task-7-2/step-7-2-1 test=async-workers.test.ts
// ─────────────────────────────────────────────────────────────────────────────

describe("REQ-HLU-026: subagent distillation worker", () => {
  test("runSubagentDistillationWorker writes subagent summary YAML to output dir", async () => {
    const tmpDir = mkdtempSync(join(tmpdir(), "hlu026-"));
    const pool = new AsyncWorkerPool({ timeout_ms: 5000 });

    const rawResult = {
      files_changed: ["src/foo.ts"],
      tests_added: ["tests/foo.test.ts"],
      status: "completed",
      confidence: 80,
    };

    const record = await runSubagentDistillationWorker(
      pool,
      "dev-axiom",
      "session-abc-123",
      rawResult,
      tmpDir,
    );

    expect(record.status).toBe("COMPLETED");

    // Verify a file was written in the output dir
    const { readdirSync } = await import("fs");
    const files = readdirSync(tmpDir);
    expect(files.length).toBe(1);
    expect(files[0]).toMatch(/^subagent-summary-\d+\.yaml$/);
  });

  test("subagent summary YAML contains all required SubagentSummaryRecord fields", async () => {
    const tmpDir = mkdtempSync(join(tmpdir(), "hlu026-fields-"));
    const pool = new AsyncWorkerPool({ timeout_ms: 5000 });

    const rawResult = {
      files_changed: ["src/bar.ts"],
      tests_added: ["tests/bar.test.ts"],
      status: "completed",
      confidence: 75,
      evidence: {
        commands_run: ["pytest tests/ -q"],
        results: "3 passed",
      },
      injected_steps: [{ title: "Fix lint", reason: "Lint failed" }],
      trace_updates: ["axiom:trace work_item=X"],
      open_questions: ["Is this approach safe?"],
    };

    await runSubagentDistillationWorker(
      pool,
      "qa-axiom",
      "session-xyz-456",
      rawResult,
      tmpDir,
    );

    const { readdirSync } = await import("fs");
    const files = readdirSync(tmpDir);
    const content = readFileSync(join(tmpDir, files[0]), "utf8");
    const parsed = yamlParse(content) as Record<string, unknown>;

    // Required top-level fields from SubagentSummaryRecord
    expect(parsed).toHaveProperty("agent", "qa-axiom");
    expect(parsed).toHaveProperty("session_id", "session-xyz-456");
    expect(parsed).toHaveProperty("status", "completed");
    expect(parsed).toHaveProperty("outputs");
    expect(parsed).toHaveProperty("evidence");
    expect(parsed).toHaveProperty("injected_steps");
    expect(parsed).toHaveProperty("trace_updates");
    expect(parsed).toHaveProperty("open_questions");
    expect(parsed).toHaveProperty("distillation_metadata");

    // Verify nested outputs fields
    const outputs = parsed["outputs"] as Record<string, unknown>;
    expect(outputs).toHaveProperty("files_changed");
    expect(outputs).toHaveProperty("tests_added");
    expect(outputs).toHaveProperty("trace_markers");

    // distillation_metadata must have source_label: "model"
    const meta = parsed["distillation_metadata"] as Record<string, unknown>;
    expect(meta["source_label"]).toBe("model");
  });

  test("distillation worker record has trigger=subagent_return and status=COMPLETED", async () => {
    const tmpDir = mkdtempSync(join(tmpdir(), "hlu026-trigger-"));
    const pool = new AsyncWorkerPool({ timeout_ms: 5000 });

    const record = await runSubagentDistillationWorker(
      pool,
      "dev-axiom",
      "session-trigger-test",
      { status: "completed" },
      tmpDir,
    );

    expect(record.trigger).toBe("subagent_return");
    expect(record.status).toBe("COMPLETED");
  });

  test("distillation worker tracks token usage", async () => {
    const tmpDir = mkdtempSync(join(tmpdir(), "hlu026-tokens-"));
    const pool = new AsyncWorkerPool({ timeout_ms: 5000 });

    const rawResult = { status: "completed", files_changed: ["a.ts", "b.ts", "c.ts"] };
    const expectedTokenEstimate = Math.ceil(JSON.stringify(rawResult).length / 4);

    const record = await runSubagentDistillationWorker(
      pool,
      "dev-axiom",
      "session-token-test",
      rawResult,
      tmpDir,
    );

    expect(record.tokens_used).toBeGreaterThan(0);
    expect(record.tokens_used).toBe(expectedTokenEstimate);
  });
});

// ─────────────────────────────────────────────────────────────────────────────
// REQ-HLU-027: Pre-compact trajectory capture worker
// axiom:trace work_item=harness-levelup-01 spec=specs/101-Harness-Engineering.md#REQ-HLU-027 plan=phase-7/task-7-2/step-7-2-1 test=async-workers.test.ts
// ─────────────────────────────────────────────────────────────────────────────

describe("REQ-HLU-027: pre-compact trajectory capture worker", () => {
  test("runPreCompactCaptureWorker writes pre_compact_summary.yaml to correct path", async () => {
    const repoRoot = mkdtempSync(join(tmpdir(), "hlu027-root-"));
    const pool = new AsyncWorkerPool({ timeout_ms: 5000 });

    const trajectory = {
      decisions: ["Use YAML for output"],
      open_questions: ["Token budget enough?"],
      plan_cursor: "phase-7/task-7-2/step-7-2-1",
      injected_steps: [],
      evidence_refs: [".memory-bank/work-items/WI-1/verification.md"],
      trace_refs: ["axiom:trace work_item=WI-1"],
    };

    const record = await runPreCompactCaptureWorker(
      pool,
      "harness-levelup-01",
      "run-2026-05-17T01",
      repoRoot,
      trajectory,
    );

    expect(record.status).toBe("COMPLETED");

    const expectedPath = join(
      repoRoot,
      ".memory-bank",
      "work-items",
      "harness-levelup-01",
      "runs",
      "run-2026-05-17T01",
      "pre_compact_summary.yaml",
    );
    expect(existsSync(expectedPath)).toBe(true);
  });

  test("pre_compact_summary.yaml contains work_item_id, run_id, decisions, open_questions", async () => {
    const repoRoot = mkdtempSync(join(tmpdir(), "hlu027-fields-"));
    const pool = new AsyncWorkerPool({ timeout_ms: 5000 });

    const trajectory = {
      decisions: ["Decision A", "Decision B"],
      open_questions: ["Q1", "Q2"],
      plan_cursor: "phase-3/task-3-1/step-3-1-1",
      injected_steps: ["Step X"],
      evidence_refs: ["evidence/run-1.md"],
      trace_refs: ["axiom:trace work_item=WI-99"],
    };

    await runPreCompactCaptureWorker(
      pool,
      "WI-99",
      "run-abc",
      repoRoot,
      trajectory,
    );

    const filePath = join(
      repoRoot,
      ".memory-bank",
      "work-items",
      "WI-99",
      "runs",
      "run-abc",
      "pre_compact_summary.yaml",
    );
    const content = readFileSync(filePath, "utf8");
    const parsed = yamlParse(content) as Record<string, unknown>;

    expect(parsed["work_item_id"]).toBe("WI-99");
    expect(parsed["run_id"]).toBe("run-abc");
    expect(parsed).toHaveProperty("captured_at");
    expect(parsed).toHaveProperty("source_label", "model");

    const summary = parsed["summary"] as Record<string, unknown>;
    expect(summary["decisions"]).toEqual(["Decision A", "Decision B"]);
    expect(summary["open_questions"]).toEqual(["Q1", "Q2"]);
    expect(summary["plan_cursor"]).toBe("phase-3/task-3-1/step-3-1-1");
    expect(summary["injected_steps"]).toEqual(["Step X"]);
    expect(summary["evidence_refs"]).toEqual(["evidence/run-1.md"]);
    expect(summary["trace_refs"]).toEqual(["axiom:trace work_item=WI-99"]);

    const meta = parsed["distillation_metadata"] as Record<string, unknown>;
    expect(meta["captured_before_compact"]).toBe(true);
  });

  test("pre-compact capture worker record has trigger=before_compact and status=COMPLETED", async () => {
    const repoRoot = mkdtempSync(join(tmpdir(), "hlu027-trigger-"));
    const pool = new AsyncWorkerPool({ timeout_ms: 5000 });

    const record = await runPreCompactCaptureWorker(
      pool,
      "WI-trigger",
      "run-trigger",
      repoRoot,
      {
        decisions: [],
        open_questions: [],
        plan_cursor: "",
        injected_steps: [],
        evidence_refs: [],
        trace_refs: [],
      },
    );

    expect(record.trigger).toBe("before_compact");
    expect(record.status).toBe("COMPLETED");
  });

  test("pre-compact capture creates parent directories if missing", async () => {
    // Use a deeply nested non-existent path as repoRoot
    const baseDir = mkdtempSync(join(tmpdir(), "hlu027-mkdir-"));
    // repoRoot itself exists but none of the subdirectories do
    const repoRoot = join(baseDir, "deeply", "nested", "repo");
    const pool = new AsyncWorkerPool({ timeout_ms: 5000 });

    const record = await runPreCompactCaptureWorker(
      pool,
      "WI-mkdir",
      "run-mkdir",
      repoRoot,
      {
        decisions: ["mkdir test"],
        open_questions: [],
        plan_cursor: "p1",
        injected_steps: [],
        evidence_refs: [],
        trace_refs: [],
      },
    );

    expect(record.status).toBe("COMPLETED");

    const expectedPath = join(
      repoRoot,
      ".memory-bank",
      "work-items",
      "WI-mkdir",
      "runs",
      "run-mkdir",
      "pre_compact_summary.yaml",
    );
    expect(existsSync(expectedPath)).toBe(true);
  });
});

// ─────────────────────────────────────────────────────────────────────────────
// REQ-HLU-025: Semaphore FIFO dispatch ordering
// axiom:trace work_item=harness-levelup-01 spec=specs/101-Harness-Engineering.md#REQ-HLU-025 plan=phase-7/task-7-1/step-7-1-1 test=async-workers.test.ts
// ─────────────────────────────────────────────────────────────────────────────

describe("REQ-HLU-025: semaphore FIFO dispatch ordering", () => {
  test("tasks are dispatched in submission order (FIFO) — not LIFO or random", async () => {
    const pool = new AsyncWorkerPool({ max_concurrent_workers: 2, timeout_ms: 5000 });
    
    const startOrder: number[] = [];
    const taskCount = 5;
    
    // Submit 5 tasks simultaneously. With concurrency=2:
    // Tasks 0 and 1 start immediately. Tasks 2, 3, 4 queue in the semaphore.
    // When a running task finishes, the next queued task (FIFO) should start.
    const records = await Promise.all(
      Array.from({ length: taskCount }, (_, i) =>
        pool.run("before_compact", async (_signal, _trackTokens) => {
          startOrder.push(i);
          // Short sleep so tasks don't complete before all are submitted
          await new Promise((r) => setTimeout(r, 20));
          return `result-${i}`;
        })
      )
    );
    
    // All tasks must complete
    expect(records).toHaveLength(taskCount);
    for (const record of records) {
      expect(record.status).toBe("COMPLETED");
    }
    
    // FIFO: the dispatch order must equal [0, 1, 2, 3, 4]
    // (first 2 start immediately, rest are dispatched in order they were submitted)
    expect(startOrder).toHaveLength(taskCount);
    expect(startOrder).toEqual([0, 1, 2, 3, 4]);
  });

  test("semaphore acquire/release preserves insertion order under concurrent pressure", async () => {
    const sem = new Semaphore(1);  // one-at-a-time
    const order: string[] = [];
    
    // Acquire once to fill the semaphore
    await sem.acquire();
    
    // Queue 3 waiters in order A, B, C
    const a = sem.acquire().then(() => { order.push("A"); sem.release(); });
    const b = sem.acquire().then(() => { order.push("B"); sem.release(); });
    const c = sem.acquire().then(() => { order.push("C"); sem.release(); });
    
    // Release the initial hold — A should go first
    sem.release();
    await Promise.all([a, b, c]);
    
    expect(order).toEqual(["A", "B", "C"]);
  });
});

