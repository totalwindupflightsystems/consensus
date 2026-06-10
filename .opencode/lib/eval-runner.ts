/**
 * Harness Eval Runner — executes eval scenarios and produces evidence files.
 *
 * REQ-HLU-012: validates scenario YAML schema (9 required fields)
 * REQ-HLU-013: writes structured evidence per 7-field schema
 * REQ-HLU-029: implements eval runner interface contract
 *
 * axiom:trace work_item=harness-levelup-01 spec=specs/101-Harness-Engineering.md#REQ-HLU-029 plan=phase-6/task-6-1/step-6-1-1
 */

import { existsSync, mkdirSync, readFileSync, writeFileSync } from "node:fs";
import path, { join } from "node:path";
import { parse as yamlParse, stringify as yamlStringify } from "yaml";

// ─────────────────────────────────────────────────────────────────────────────
// Types (REQ-HLU-012, REQ-HLU-013)
// ─────────────────────────────────────────────────────────────────────────────

// axiom:trace work_item=harness-levelup-01 spec=specs/101-Harness-Engineering.md#REQ-HLU-012 plan=phase-6/task-6-1/step-6-1-1
export interface EvalScenario {
  id: string;
  goal: string;
  category: "context" | "execution" | "evidence" | "safety";
  runtime_path: string;
  preconditions: string[];
  assertions: Record<string, unknown>[];
  timeout_seconds: number;
  pass_criteria: string;
  evidence_path: string;
}

export interface AssertionResult {
  passed: boolean;
  actual_value: string;
}

// axiom:trace work_item=harness-levelup-01 spec=specs/101-Harness-Engineering.md#REQ-HLU-013 plan=phase-6/task-6-1/step-6-1-1
export interface EvidenceRecord {
  scenario_id: string;
  verdict: "PASS" | "FAIL" | "ERROR" | "TIMEOUT";
  timestamp: string;
  assertions_results: Record<string, AssertionResult> | Record<string, never>;
  runtime_ms: number;
  command_output_ref: string;
  error_detail?: string;
}

export type EvalVerdict = "PASS" | "FAIL" | "ERROR" | "TIMEOUT";

// ─────────────────────────────────────────────────────────────────────────────
// Valid category values
// ─────────────────────────────────────────────────────────────────────────────

const VALID_CATEGORIES = new Set<string>(["context", "execution", "evidence", "safety"]);

// ─────────────────────────────────────────────────────────────────────────────
// REQ-HLU-012: Scenario validation
// ─────────────────────────────────────────────────────────────────────────────

/**
 * Validates all 9 required fields of a scenario object.
 * Applies defaults: timeout_seconds=120, pass_criteria="all".
 * Throws Error on missing or invalid field.
 *
 * axiom:trace work_item=harness-levelup-01 spec=specs/101-Harness-Engineering.md#REQ-HLU-012 plan=phase-6/task-6-1/step-6-1-1
 */
export function validateScenario(raw: Record<string, unknown>): EvalScenario {
  // Required string fields
  const requiredStrings = ["id", "goal", "runtime_path", "evidence_path"] as const;
  for (const field of requiredStrings) {
    if (typeof raw[field] !== "string" || (raw[field] as string).length === 0) {
      throw new Error(`missing or invalid required field: ${field}`);
    }
  }

  // category: must be one of the 4 valid values
  if (typeof raw["category"] !== "string" || !VALID_CATEGORIES.has(raw["category"] as string)) {
    throw new Error(
      `missing or invalid required field: category (must be one of: context, execution, evidence, safety)`
    );
  }

  // preconditions: must be an array (can be empty)
  if (!Array.isArray(raw["preconditions"])) {
    throw new Error(`missing or invalid required field: preconditions (must be an array)`);
  }

  // assertions: must be an array (can be empty)
  if (!Array.isArray(raw["assertions"])) {
    throw new Error(`missing or invalid required field: assertions (must be an array)`);
  }

  // timeout_seconds: optional, defaults to 120, must be a positive number if present
  let timeout_seconds = 120;
  if (raw["timeout_seconds"] !== undefined) {
    if (typeof raw["timeout_seconds"] !== "number" || raw["timeout_seconds"] <= 0) {
      throw new Error(`invalid field: timeout_seconds (must be a positive number)`);
    }
    timeout_seconds = raw["timeout_seconds"] as number;
  }

  // pass_criteria: optional, defaults to "all"
  let pass_criteria = "all";
  if (raw["pass_criteria"] !== undefined) {
    if (typeof raw["pass_criteria"] !== "string") {
      throw new Error(`invalid field: pass_criteria (must be a string)`);
    }
    pass_criteria = raw["pass_criteria"] as string;
  }

  return {
    id: raw["id"] as string,
    goal: raw["goal"] as string,
    category: raw["category"] as EvalScenario["category"],
    runtime_path: raw["runtime_path"] as string,
    preconditions: raw["preconditions"] as string[],
    assertions: raw["assertions"] as Record<string, unknown>[],
    timeout_seconds,
    pass_criteria,
    evidence_path: raw["evidence_path"] as string,
  };
}

// ─────────────────────────────────────────────────────────────────────────────
// REQ-HLU-029: Pass criteria parsing
// ─────────────────────────────────────────────────────────────────────────────

/**
 * Parses "all", "any", or "N_of_M" format.
 * Throws on N > assertionCount (edge case: invalid_pass_criteria).
 *
 * axiom:trace work_item=harness-levelup-01 spec=specs/101-Harness-Engineering.md#REQ-HLU-029 plan=phase-6/task-6-1/step-6-1-1
 */
export function parsePassCriteria(
  criteria: string,
  assertionCount: number
): { mode: "all" | "any" | "n_of_m"; n?: number } {
  if (criteria === "all") {
    return { mode: "all" };
  }
  if (criteria === "any") {
    return { mode: "any" };
  }

  // N_of_M pattern: e.g., "2_of_3"
  const match = criteria.match(/^(\d+)_of_(\d+)$/);
  if (match) {
    const n = parseInt(match[1], 10);
    const m = parseInt(match[2], 10);
    if (n > assertionCount) {
      throw new Error(`invalid_pass_criteria: N exceeds assertion count`);
    }
    return { mode: "n_of_m", n };
  }

  throw new Error(`invalid_pass_criteria: unrecognized format "${criteria}"`);
}

// ─────────────────────────────────────────────────────────────────────────────
// REQ-HLU-029: Assertion evaluation
// ─────────────────────────────────────────────────────────────────────────────

/**
 * Evaluates a single assertion against captured output.
 * Supported assertion types:
 *   - selected_context_contains: string[]  — PASS if all strings appear in output
 *   - evidence_contains_command_output: true — PASS if output is non-empty
 *   - no_secret_leakage: true — PASS if no credential patterns found
 *   - runtime_path_reached: true — PASS always (called only when command succeeded)
 *
 * axiom:trace work_item=harness-levelup-01 spec=specs/101-Harness-Engineering.md#REQ-HLU-029 plan=phase-6/task-6-1/step-6-1-1
 */
export function evaluateAssertion(
  key: string,
  expectedValue: unknown,
  output: string
): AssertionResult {
  switch (key) {
    case "selected_context_contains": {
      // PASS if all strings in the list appear in output
      const list = Array.isArray(expectedValue) ? (expectedValue as unknown[]) : [expectedValue];
      const missing = list.filter((s) => !output.includes(String(s)));
      if (missing.length === 0) {
        return { passed: true, actual_value: `all ${list.length} strings found in output` };
      } else {
        return {
          passed: false,
          actual_value: `missing strings: ${missing.map(String).join(", ")}`,
        };
      }
    }

    case "evidence_contains_command_output": {
      // PASS if stdout is non-empty
      const nonEmpty = output.trim().length > 0;
      return {
        passed: nonEmpty,
        actual_value: nonEmpty ? "output is non-empty" : "output is empty",
      };
    }

    case "no_secret_leakage": {
      // PASS if none of the credential patterns appear in output
      const secretPatterns = ["Bearer ", "sk-", "AKIA", "-----BEGIN"];
      const found = secretPatterns.filter((p) => output.includes(p));
      if (found.length === 0) {
        return { passed: true, actual_value: "no secret patterns detected" };
      } else {
        return {
          passed: false,
          actual_value: `secret patterns detected: ${found.join(", ")}`,
        };
      }
    }

    case "runtime_path_reached": {
      // PASS if the command exited without a runner-level error (any exit code)
      // When this assertion is evaluated, we know the command ran — so always PASS
      return { passed: true, actual_value: "command reached runtime path" };
    }

    default: {
      // Unknown assertion type — fail with explanation
      return {
        passed: false,
        actual_value: `unknown assertion type: ${key}`,
      };
    }
  }
}

// ─────────────────────────────────────────────────────────────────────────────
// REQ-HLU-029: Apply pass criteria to assertion results
// ─────────────────────────────────────────────────────────────────────────────

/**
 * Returns true if the results satisfy the pass criteria.
 *
 * axiom:trace work_item=harness-levelup-01 spec=specs/101-Harness-Engineering.md#REQ-HLU-029 plan=phase-6/task-6-1/step-6-1-1
 */
export function applyPassCriteria(
  results: Record<string, AssertionResult>,
  criteria: ReturnType<typeof parsePassCriteria>
): boolean {
  const resultValues = Object.values(results);
  const passCount = resultValues.filter((r) => r.passed).length;
  const total = resultValues.length;

  switch (criteria.mode) {
    case "all":
      return passCount === total;
    case "any":
      return passCount >= 1;
    case "n_of_m":
      return passCount >= (criteria.n ?? 0);
    default:
      return false;
  }
}

// ─────────────────────────────────────────────────────────────────────────────
// REQ-HLU-029: Exit code mapping
// ─────────────────────────────────────────────────────────────────────────────

/**
 * Maps EvalVerdict to exit code.
 * PASS → 0, FAIL → 1, ERROR → 2, TIMEOUT → 3
 *
 * axiom:trace work_item=harness-levelup-01 spec=specs/101-Harness-Engineering.md#REQ-HLU-029 plan=phase-6/task-6-1/step-6-1-1
 */
export function getExitCode(verdict: EvalVerdict): number {
  switch (verdict) {
    case "PASS":
      return 0;
    case "FAIL":
      return 1;
    case "ERROR":
      return 2;
    case "TIMEOUT":
      return 3;
    default:
      return 2;
  }
}

// ─────────────────────────────────────────────────────────────────────────────
// REQ-HLU-029: Full scenario execution
// ─────────────────────────────────────────────────────────────────────────────

/**
 * Full scenario execution:
 *  1. Create evidence_path directory (mkdir -p)
 *  2. Spawn runtime_path subprocess with timeout
 *  3. Check empty-output edge case AFTER execution
 *  4. Evaluate assertions
 *  5. Write evidence YAML to evidence_path/<timestamp>.yaml
 *  6. Return EvidenceRecord
 *
 * axiom:trace work_item=harness-levelup-01 spec=specs/101-Harness-Engineering.md#REQ-HLU-029 plan=phase-6/task-6-1/step-6-1-1
 * axiom:trace work_item=harness-levelup-01 spec=specs/101-Harness-Engineering.md#REQ-HLU-013 plan=phase-6/task-6-1/step-6-1-1
 */
export async function runScenario(
  scenario: EvalScenario,
  repoRoot: string
): Promise<EvidenceRecord> {
  const startMs = Date.now();
  const timestamp = new Date().toISOString();
  const fileTimestamp = timestamp.replace(/[:.]/g, "-");

  // 1. Create evidence_path directory (mkdir -p)
  const evidenceDir = path.isAbsolute(scenario.evidence_path)
    ? scenario.evidence_path
    : path.join(repoRoot, scenario.evidence_path);

  try {
    mkdirSync(evidenceDir, { recursive: true });
  } catch (mkdirErr) {
    const runtime_ms = Date.now() - startMs;
    const record: EvidenceRecord = {
      scenario_id: scenario.id,
      verdict: "ERROR",
      timestamp,
      assertions_results: {},
      runtime_ms,
      command_output_ref: "none",
      error_detail: `failed to create evidence_path: ${mkdirErr instanceof Error ? mkdirErr.message : String(mkdirErr)}`,
    };
    return record;
  }

  const outputRef = path.join(evidenceDir, `output-${fileTimestamp}.txt`);
  const evidenceFilePath = path.join(evidenceDir, `${fileTimestamp}.yaml`);

  // 2. Spawn runtime_path subprocess with timeout
  // Parse the command string into words (simple whitespace split — handles basic shell commands)
  // MG-04 fix: use ["sh", "-c", cmd] instead of naive whitespace split.
  // This correctly handles quoted arguments like:
  //   axiom run --intent "summarize work item" --repo .
  // The previous split(/\s+/) would break quoted strings with spaces.
  const cmdWords = ["sh", "-c", scenario.runtime_path];
  const timeoutMs = scenario.timeout_seconds * 1000;

  let stdout = "";
  let stderr = "";
  let verdict: EvalVerdict = "PASS";
  let errorDetail: string | undefined;
  let assertionsResults: Record<string, AssertionResult> | Record<string, never> = {};

  try {
    // Spawn with Bun
    const proc = Bun.spawn(cmdWords, {
      stdout: "pipe",
      stderr: "pipe",
      cwd: repoRoot,
    });

    // Race subprocess completion against timeout
    const timeoutPromise = new Promise<"timeout">((resolve) =>
      setTimeout(() => resolve("timeout"), timeoutMs)
    );

    const completionPromise = proc.exited.then(() => "done" as const);
    const race = await Promise.race([completionPromise, timeoutPromise]);

    if (race === "timeout") {
      // Kill the process
      proc.kill("SIGTERM");
      // Give it a moment to die then forcefully kill
      await new Promise((resolve) => setTimeout(resolve, 100));
      proc.kill("SIGKILL");

      const runtime_ms = Date.now() - startMs;

      // Write empty output ref
      try {
        writeFileSync(outputRef, "(timeout — no output captured)");
      } catch (_) {
        // ignore write errors for output file on timeout
      }

      const record: EvidenceRecord = {
        scenario_id: scenario.id,
        verdict: "TIMEOUT",
        timestamp,
        assertions_results: {},
        runtime_ms,
        command_output_ref: outputRef,
        error_detail: `timeout after ${scenario.timeout_seconds}s`,
      };

      // Write evidence YAML
      try {
        writeFileSync(evidenceFilePath, yamlStringify(record));
      } catch (_) {
        // ignore evidence write errors
      }

      return record;
    }

    // Completed — read stdout and stderr
    stdout = await new Response(proc.stdout).text();
    stderr = await new Response(proc.stderr).text();

  } catch (spawnErr) {
    // Runner-level error: command not found, permission denied, etc.
    const runtime_ms = Date.now() - startMs;
    const detail = spawnErr instanceof Error ? spawnErr.message : String(spawnErr);

    try {
      writeFileSync(outputRef, `spawn error: ${detail}`);
    } catch (_) {
      // ignore
    }

    const record: EvidenceRecord = {
      scenario_id: scenario.id,
      verdict: "ERROR",
      timestamp,
      assertions_results: {},
      runtime_ms,
      command_output_ref: outputRef,
      error_detail: `spawn error: ${detail}`,
    };

    try {
      writeFileSync(evidenceFilePath, yamlStringify(record));
    } catch (_) {
      // ignore
    }

    return record;
  }

  const runtime_ms = Date.now() - startMs;
  const output = stdout + stderr;

  // 3. Write raw output to command_output_ref
  try {
    writeFileSync(outputRef, output);
  } catch (writeErr) {
    const record: EvidenceRecord = {
      scenario_id: scenario.id,
      verdict: "ERROR",
      timestamp,
      assertions_results: {},
      runtime_ms,
      command_output_ref: "none",
      error_detail: `failed to write output file: ${writeErr instanceof Error ? writeErr.message : String(writeErr)}`,
    };
    try {
      writeFileSync(evidenceFilePath, yamlStringify(record));
    } catch (_) {
      // ignore
    }
    return record;
  }

  // 4. Edge case check: empty output (exit 0 but stdout+stderr both empty)
  // Precedence: empty-output ERROR wins over runtime_path_reached: true PASS
  if (output.trim().length === 0) {
    verdict = "ERROR";
    errorDetail = "empty_output";
    const record: EvidenceRecord = {
      scenario_id: scenario.id,
      verdict,
      timestamp,
      assertions_results: {},
      runtime_ms,
      command_output_ref: outputRef,
      error_detail: errorDetail,
    };
    try {
      writeFileSync(evidenceFilePath, yamlStringify(record));
    } catch (_) {
      // ignore
    }
    return record;
  }

  // 5. Validate pass_criteria before evaluating assertions
  let parsedCriteria: ReturnType<typeof parsePassCriteria>;
  try {
    parsedCriteria = parsePassCriteria(scenario.pass_criteria, scenario.assertions.length);
  } catch (criteriaErr) {
    const detail = criteriaErr instanceof Error ? criteriaErr.message : String(criteriaErr);
    const record: EvidenceRecord = {
      scenario_id: scenario.id,
      verdict: "ERROR",
      timestamp,
      assertions_results: {},
      runtime_ms,
      command_output_ref: outputRef,
      error_detail: detail,
    };
    try {
      writeFileSync(evidenceFilePath, yamlStringify(record));
    } catch (_) {
      // ignore
    }
    return record;
  }

  // 6. Evaluate assertions
  const results: Record<string, AssertionResult> = {};
  for (const assertionObj of scenario.assertions) {
    for (const [key, expectedValue] of Object.entries(assertionObj)) {
      results[key] = evaluateAssertion(key, expectedValue, output);
    }
  }
  assertionsResults = results;

  // 7. Apply pass criteria to determine verdict
  const overallPass = applyPassCriteria(results, parsedCriteria);
  verdict = overallPass ? "PASS" : "FAIL";

  if (!overallPass) {
    const failedKeys = Object.entries(results)
      .filter(([, r]) => !r.passed)
      .map(([k]) => k);
    errorDetail = `assertions failed: ${failedKeys.join(", ")}`;
  }

  // 8. Write evidence YAML
  const record: EvidenceRecord = {
    scenario_id: scenario.id,
    verdict,
    timestamp,
    assertions_results: assertionsResults,
    runtime_ms,
    command_output_ref: outputRef,
    ...(errorDetail !== undefined ? { error_detail: errorDetail } : {}),
  };

  try {
    writeFileSync(evidenceFilePath, yamlStringify(record));
  } catch (evErr) {
    // Record the write failure but still return the record
    record.error_detail = `evidence write failed: ${evErr instanceof Error ? evErr.message : String(evErr)}`;
    record.verdict = "ERROR";
  }

  return record;
}

// ─────────────────────────────────────────────────────────────────────────────
// CLI entry point — REQ-HLU-029 item 1
// Usage: bun run .opencode/plugins/eval-runner.ts <scenario-id>
// axiom:trace work_item=harness-levelup-01 spec=specs/101-Harness-Engineering.md#REQ-HLU-029 plan=phase-6/task-6-1/step-6-1-3
// ─────────────────────────────────────────────────────────────────────────────
if (import.meta.main) {
  const scenarioId = process.argv[2];
  if (!scenarioId) {
    process.stderr.write("Usage: bun run eval-runner.ts <scenario-id>\n");
    process.exit(2);
  }

  const cwd = process.cwd();
  const scenarioPath = join(cwd, ".memory-bank", "harness-evals", "scenarios", `${scenarioId}.yaml`);

  if (!existsSync(scenarioPath)) {
    process.stderr.write(`Error: scenario file not found: ${scenarioPath}\n`);
    process.exit(2);
  }

  try {
    const raw = yamlParse(readFileSync(scenarioPath, "utf-8")) as Record<string, unknown>;
    const scenario = validateScenario(raw);
    const record = await runScenario(scenario, cwd);
    process.stderr.write(JSON.stringify({ verdict: record.verdict, evidence_path: record.command_output_ref }, null, 2) + "\n");
    process.exit(getExitCode(record.verdict));
  } catch (err) {
    process.stderr.write(`Error: ${(err as Error).message}\n`);
    process.exit(2);
  }
}

// OpenCode plugin loader no-op — this file is a utility module, not a plugin.
// OpenCode auto-discovers all .ts files in plugins/ and tries to load them.
// This export prevents "Plugin export is not a function" errors.
export default async () => ({ tool: {} });
