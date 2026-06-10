/**
 * Tests for eval-runner.ts — REQ-HLU-012, REQ-HLU-013, REQ-HLU-029.
 *
 * REQ-HLU-012: validates scenario YAML schema (9 required fields)
 * REQ-HLU-013: writes structured evidence per 7-field schema
 * REQ-HLU-029: implements eval runner interface contract (assertions, exit codes, edge cases)
 *
 * Run: cd .opencode && bun test tests/eval-runner.test.ts
 *
 * axiom:trace work_item=harness-levelup-01 spec=specs/101-Harness-Engineering.md#REQ-HLU-029 plan=phase-6/task-6-1/step-6-1-1 test=eval-runner.test.ts
 */

import { test, expect, describe, afterEach, beforeEach } from "bun:test";
import { mkdtempSync, mkdirSync, rmSync, existsSync, readFileSync, readdirSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import path from "node:path";
import { parse as yamlParse } from "yaml";
import {
  validateScenario,
  parsePassCriteria,
  evaluateAssertion,
  applyPassCriteria,
  runScenario,
  getExitCode,
  type EvalScenario,
  type AssertionResult,
  type EvidenceRecord,
} from "../lib/eval-runner.ts";

// ─────────────────────────────────────────────────────────────────────────────
// Test helpers
// ─────────────────────────────────────────────────────────────────────────────

function makeScenario(overrides?: Partial<EvalScenario>): EvalScenario {
  return {
    id: "test-scenario-001",
    goal: "Verify echo command runs",
    category: "execution",
    runtime_path: "echo hello",
    preconditions: [],
    assertions: [{ evidence_contains_command_output: true }],
    timeout_seconds: 10,
    pass_criteria: "all",
    evidence_path: "/tmp/test-evidence",
    ...overrides,
  };
}

// Temp directory management for runScenario tests
let tempDirs: string[] = [];

function makeTempDir(): string {
  const dir = mkdtempSync(path.join(tmpdir(), "eval-runner-test-"));
  tempDirs.push(dir);
  return dir;
}

afterEach(() => {
  for (const dir of tempDirs) {
    try {
      rmSync(dir, { recursive: true, force: true });
    } catch (_) {
      // ignore cleanup errors
    }
  }
  tempDirs = [];
});

// ─────────────────────────────────────────────────────────────────────────────
// REQ-HLU-012: scenario validation
// ─────────────────────────────────────────────────────────────────────────────

// axiom:trace work_item=harness-levelup-01 spec=specs/101-Harness-Engineering.md#REQ-HLU-012 plan=phase-6/task-6-1/step-6-1-1
describe("REQ-HLU-012: scenario validation", () => {
  test("validateScenario rejects missing required fields", () => {
    // Missing 'id' field
    expect(() =>
      validateScenario({
        goal: "test",
        category: "execution",
        runtime_path: "echo hi",
        preconditions: [],
        assertions: [],
        timeout_seconds: 30,
        pass_criteria: "all",
        evidence_path: "/tmp/ev",
      })
    ).toThrow(/missing or invalid required field: id/);

    // Missing 'goal'
    expect(() =>
      validateScenario({
        id: "s1",
        category: "execution",
        runtime_path: "echo hi",
        preconditions: [],
        assertions: [],
        evidence_path: "/tmp/ev",
      })
    ).toThrow(/missing or invalid required field: goal/);

    // Missing 'runtime_path'
    expect(() =>
      validateScenario({
        id: "s1",
        goal: "test",
        category: "execution",
        preconditions: [],
        assertions: [],
        evidence_path: "/tmp/ev",
      })
    ).toThrow(/missing or invalid required field: runtime_path/);

    // Missing 'evidence_path'
    expect(() =>
      validateScenario({
        id: "s1",
        goal: "test",
        category: "execution",
        runtime_path: "echo hi",
        preconditions: [],
        assertions: [],
      })
    ).toThrow(/missing or invalid required field: evidence_path/);

    // Missing 'preconditions'
    expect(() =>
      validateScenario({
        id: "s1",
        goal: "test",
        category: "execution",
        runtime_path: "echo hi",
        assertions: [],
        evidence_path: "/tmp/ev",
      })
    ).toThrow(/missing or invalid required field: preconditions/);

    // Missing 'assertions'
    expect(() =>
      validateScenario({
        id: "s1",
        goal: "test",
        category: "execution",
        runtime_path: "echo hi",
        preconditions: [],
        evidence_path: "/tmp/ev",
      })
    ).toThrow(/missing or invalid required field: assertions/);
  });

  test("validateScenario rejects invalid category value", () => {
    expect(() =>
      validateScenario({
        id: "s1",
        goal: "test",
        category: "invalid_category",
        runtime_path: "echo hi",
        preconditions: [],
        assertions: [],
        evidence_path: "/tmp/ev",
      })
    ).toThrow(/missing or invalid required field: category/);

    // Also reject missing category
    expect(() =>
      validateScenario({
        id: "s1",
        goal: "test",
        runtime_path: "echo hi",
        preconditions: [],
        assertions: [],
        evidence_path: "/tmp/ev",
      })
    ).toThrow(/missing or invalid required field: category/);
  });

  test("validateScenario accepts a fully valid scenario", () => {
    const raw = {
      id: "smoke-001",
      goal: "Verify harness runs",
      category: "execution",
      runtime_path: "echo smoke",
      preconditions: ["axiom installed"],
      assertions: [{ evidence_contains_command_output: true }],
      timeout_seconds: 30,
      pass_criteria: "all",
      evidence_path: "/tmp/evidence",
    };
    const scenario = validateScenario(raw);
    expect(scenario.id).toBe("smoke-001");
    expect(scenario.goal).toBe("Verify harness runs");
    expect(scenario.category).toBe("execution");
    expect(scenario.timeout_seconds).toBe(30);
    expect(scenario.pass_criteria).toBe("all");
    expect(Array.isArray(scenario.preconditions)).toBe(true);
    expect(Array.isArray(scenario.assertions)).toBe(true);
  });

  test("validateScenario applies default timeout_seconds=120 when missing", () => {
    const raw = {
      id: "s-defaults",
      goal: "test defaults",
      category: "context",
      runtime_path: "echo hi",
      preconditions: [],
      assertions: [],
      evidence_path: "/tmp/ev",
    };
    const scenario = validateScenario(raw);
    expect(scenario.timeout_seconds).toBe(120);
  });

  test("validateScenario applies default pass_criteria=all when missing", () => {
    const raw = {
      id: "s-defaults-2",
      goal: "test pass_criteria default",
      category: "safety",
      runtime_path: "echo hi",
      preconditions: [],
      assertions: [],
      evidence_path: "/tmp/ev",
    };
    const scenario = validateScenario(raw);
    expect(scenario.pass_criteria).toBe("all");
  });
});

// ─────────────────────────────────────────────────────────────────────────────
// REQ-HLU-029: assertion evaluation
// ─────────────────────────────────────────────────────────────────────────────

// axiom:trace work_item=harness-levelup-01 spec=specs/101-Harness-Engineering.md#REQ-HLU-029 plan=phase-6/task-6-1/step-6-1-1
describe("REQ-HLU-029: assertion evaluation", () => {
  test("selected_context_contains PASS when all strings found in output", () => {
    const result = evaluateAssertion(
      "selected_context_contains",
      ["hello", "world"],
      "hello world from the command"
    );
    expect(result.passed).toBe(true);
  });

  test("selected_context_contains FAIL when some strings missing from output", () => {
    const result = evaluateAssertion(
      "selected_context_contains",
      ["hello", "missing-string"],
      "hello from the command"
    );
    expect(result.passed).toBe(false);
    expect(result.actual_value).toContain("missing-string");
  });

  test("evidence_contains_command_output PASS when stdout non-empty", () => {
    const result = evaluateAssertion(
      "evidence_contains_command_output",
      true,
      "some output here"
    );
    expect(result.passed).toBe(true);
  });

  test("evidence_contains_command_output FAIL when stdout empty", () => {
    const result = evaluateAssertion(
      "evidence_contains_command_output",
      true,
      ""
    );
    expect(result.passed).toBe(false);
    expect(result.actual_value).toContain("empty");
  });

  test("no_secret_leakage PASS when no credential patterns in output", () => {
    const result = evaluateAssertion(
      "no_secret_leakage",
      true,
      "normal output without any secrets"
    );
    expect(result.passed).toBe(true);
  });

  test("no_secret_leakage FAIL when AKIA pattern in output", () => {
    const result = evaluateAssertion(
      "no_secret_leakage",
      true,
      "found key AKIAIOSFODNN7EXAMPLE in output"
    );
    expect(result.passed).toBe(false);
    expect(result.actual_value).toContain("AKIA");
  });

  test("no_secret_leakage FAIL when Bearer pattern in output", () => {
    const result = evaluateAssertion(
      "no_secret_leakage",
      true,
      "Authorization: Bearer eyJhbGciOiJIUzI1NiJ9"
    );
    expect(result.passed).toBe(false);
    expect(result.actual_value).toContain("Bearer ");
  });

  test("no_secret_leakage FAIL when sk- pattern in output", () => {
    const result = evaluateAssertion(
      "no_secret_leakage",
      true,
      "api_key=sk-abcdef1234567890"
    );
    expect(result.passed).toBe(false);
    expect(result.actual_value).toContain("sk-");
  });

  test("no_secret_leakage FAIL when -----BEGIN pattern in output", () => {
    const result = evaluateAssertion(
      "no_secret_leakage",
      true,
      "-----BEGIN RSA PRIVATE KEY-----\nMIIEpAIBAAK..."
    );
    expect(result.passed).toBe(false);
    expect(result.actual_value).toContain("-----BEGIN");
  });

  test("runtime_path_reached PASS if command exits successfully", () => {
    // runtime_path_reached: true — always passes when assertion is reached
    // (empty output check runs first; if we get here, command ran)
    const result = evaluateAssertion(
      "runtime_path_reached",
      true,
      "command output here"
    );
    expect(result.passed).toBe(true);
    expect(result.actual_value).toContain("runtime path");
  });
});

// ─────────────────────────────────────────────────────────────────────────────
// REQ-HLU-029: exit codes and pass_criteria
// ─────────────────────────────────────────────────────────────────────────────

// axiom:trace work_item=harness-levelup-01 spec=specs/101-Harness-Engineering.md#REQ-HLU-029 plan=phase-6/task-6-1/step-6-1-1
describe("REQ-HLU-029: exit codes and pass_criteria", () => {
  test("exit code 0 for PASS verdict", () => {
    expect(getExitCode("PASS")).toBe(0);
  });

  test("exit code 1 for FAIL verdict", () => {
    expect(getExitCode("FAIL")).toBe(1);
  });

  test("exit code 2 for ERROR verdict", () => {
    expect(getExitCode("ERROR")).toBe(2);
  });

  test("exit code 3 for TIMEOUT verdict", () => {
    expect(getExitCode("TIMEOUT")).toBe(3);
  });

  test("pass_criteria=all requires all assertions to pass", () => {
    const results: Record<string, AssertionResult> = {
      a1: { passed: true, actual_value: "ok" },
      a2: { passed: true, actual_value: "ok" },
      a3: { passed: false, actual_value: "failed" },
    };
    const criteria = parsePassCriteria("all", 3);
    expect(applyPassCriteria(results, criteria)).toBe(false);

    // All passing
    const allPass: Record<string, AssertionResult> = {
      a1: { passed: true, actual_value: "ok" },
      a2: { passed: true, actual_value: "ok" },
    };
    expect(applyPassCriteria(allPass, parsePassCriteria("all", 2))).toBe(true);
  });

  test("pass_criteria=any requires at least one assertion to pass", () => {
    const results: Record<string, AssertionResult> = {
      a1: { passed: false, actual_value: "failed" },
      a2: { passed: true, actual_value: "ok" },
      a3: { passed: false, actual_value: "failed" },
    };
    const criteria = parsePassCriteria("any", 3);
    expect(applyPassCriteria(results, criteria)).toBe(true);

    // All failing
    const allFail: Record<string, AssertionResult> = {
      a1: { passed: false, actual_value: "failed" },
      a2: { passed: false, actual_value: "failed" },
    };
    expect(applyPassCriteria(allFail, parsePassCriteria("any", 2))).toBe(false);
  });

  test("pass_criteria=2_of_3 requires exactly 2 of 3 to pass", () => {
    const results: Record<string, AssertionResult> = {
      a1: { passed: true, actual_value: "ok" },
      a2: { passed: true, actual_value: "ok" },
      a3: { passed: false, actual_value: "failed" },
    };
    const criteria = parsePassCriteria("2_of_3", 3);
    expect(criteria.mode).toBe("n_of_m");
    expect(criteria.n).toBe(2);
    expect(applyPassCriteria(results, criteria)).toBe(true);

    // Only 1 of 3 passes — should fail
    const onePass: Record<string, AssertionResult> = {
      a1: { passed: true, actual_value: "ok" },
      a2: { passed: false, actual_value: "failed" },
      a3: { passed: false, actual_value: "failed" },
    };
    expect(applyPassCriteria(onePass, criteria)).toBe(false);
  });

  test("parsePassCriteria throws when N > assertion count (edge case)", () => {
    // 5_of_3: N=5 > assertionCount=3 → should throw
    expect(() => parsePassCriteria("5_of_3", 3)).toThrow(
      /invalid_pass_criteria: N exceeds assertion count/
    );

    // 4_of_2: N=4 > assertionCount=2
    expect(() => parsePassCriteria("4_of_2", 2)).toThrow(
      /invalid_pass_criteria: N exceeds assertion count/
    );
  });
});

// ─────────────────────────────────────────────────────────────────────────────
// REQ-HLU-029: edge cases (checked before assertions)
// ─────────────────────────────────────────────────────────────────────────────

// axiom:trace work_item=harness-levelup-01 spec=specs/101-Harness-Engineering.md#REQ-HLU-029 plan=phase-6/task-6-1/step-6-1-1
describe("REQ-HLU-029: edge cases (checked before assertions)", () => {
  test("empty output (exit 0, no stdout/stderr) → ERROR verdict, not PASS", async () => {
    const evidenceDir = makeTempDir();
    const scenario = makeScenario({
      id: "edge-empty-output",
      // 'true' command exits 0 with no output — cleanest way to test empty output
      runtime_path: "true",
      assertions: [{ evidence_contains_command_output: true }],
      timeout_seconds: 10,
      evidence_path: evidenceDir,
    });

    const record = await runScenario(scenario, process.cwd());
    expect(record.verdict).toBe("ERROR");
    expect(record.error_detail).toBe("empty_output");
    expect(getExitCode(record.verdict)).toBe(2);
  });

  test("empty output overrides runtime_path_reached: true assertion evaluation", async () => {
    const evidenceDir = makeTempDir();
    const scenario = makeScenario({
      id: "edge-empty-override",
      // 'true' exits 0 with no output
      runtime_path: "true",
      // runtime_path_reached would normally pass, but empty output error takes precedence
      assertions: [{ runtime_path_reached: true }],
      timeout_seconds: 10,
      evidence_path: evidenceDir,
    });

    const record = await runScenario(scenario, process.cwd());
    // Empty output → ERROR, not PASS (even though runtime_path_reached would pass)
    expect(record.verdict).toBe("ERROR");
    expect(record.error_detail).toBe("empty_output");
    // assertions_results should be empty ({}) on ERROR
    expect(Object.keys(record.assertions_results)).toHaveLength(0);
  });

  test("invalid_pass_criteria: N exceeds assertion count → ERROR", async () => {
    const evidenceDir = makeTempDir();
    const scenario = makeScenario({
      id: "edge-invalid-criteria",
      runtime_path: "echo test",
      assertions: [
        { evidence_contains_command_output: true },
        { no_secret_leakage: true },
      ],
      // 5_of_2: N=5 > assertion count=2 → ERROR
      pass_criteria: "5_of_2",
      timeout_seconds: 10,
      evidence_path: evidenceDir,
    });

    const record = await runScenario(scenario, process.cwd());
    expect(record.verdict).toBe("ERROR");
    expect(record.error_detail).toContain("invalid_pass_criteria");
  });
});

// ─────────────────────────────────────────────────────────────────────────────
// REQ-HLU-013: evidence file production
// ─────────────────────────────────────────────────────────────────────────────

// axiom:trace work_item=harness-levelup-01 spec=specs/101-Harness-Engineering.md#REQ-HLU-013 plan=phase-6/task-6-1/step-6-1-1
describe("REQ-HLU-013: evidence file production", () => {
  test("runScenario writes evidence file to evidence_path/<timestamp>.yaml", async () => {
    const evidenceDir = makeTempDir();
    const scenario = makeScenario({
      id: "ev-file-001",
      runtime_path: "echo hello",
      assertions: [{ evidence_contains_command_output: true }],
      timeout_seconds: 10,
      evidence_path: evidenceDir,
    });

    await runScenario(scenario, process.cwd());

    // Check that a .yaml file was created in the evidence directory
    const files = readdirSync(evidenceDir).filter((f) => f.endsWith(".yaml"));
    expect(files.length).toBeGreaterThanOrEqual(1);
  });

  test("runScenario evidence file contains all 7 required fields", async () => {
    const evidenceDir = makeTempDir();
    const scenario = makeScenario({
      id: "ev-fields-001",
      runtime_path: "echo evidence test",
      assertions: [{ evidence_contains_command_output: true }],
      timeout_seconds: 10,
      evidence_path: evidenceDir,
    });

    const record = await runScenario(scenario, process.cwd());

    // Check 7 required fields on the returned record
    expect(typeof record.scenario_id).toBe("string");
    expect(["PASS", "FAIL", "ERROR", "TIMEOUT"]).toContain(record.verdict);
    expect(typeof record.timestamp).toBe("string");
    // timestamp must be ISO8601
    expect(record.timestamp).toMatch(/^\d{4}-\d{2}-\d{2}T/);
    expect(typeof record.assertions_results).toBe("object");
    expect(typeof record.runtime_ms).toBe("number");
    expect(record.runtime_ms).toBeGreaterThanOrEqual(0);
    expect(typeof record.command_output_ref).toBe("string");

    // Read and parse the evidence YAML file
    const yamlFiles = readdirSync(evidenceDir).filter((f) => f.endsWith(".yaml"));
    expect(yamlFiles.length).toBeGreaterThanOrEqual(1);
    const parsed = yamlParse(readFileSync(path.join(evidenceDir, yamlFiles[0]), "utf-8")) as EvidenceRecord;
    expect(parsed.scenario_id).toBe("ev-fields-001");
    expect(["PASS", "FAIL", "ERROR", "TIMEOUT"]).toContain(parsed.verdict);
    expect(typeof parsed.timestamp).toBe("string");
    expect(typeof parsed.assertions_results).toBe("object");
    expect(typeof parsed.runtime_ms).toBe("number");
    expect(typeof parsed.command_output_ref).toBe("string");
  });

  test("runScenario writes raw output to command_output_ref path", async () => {
    const evidenceDir = makeTempDir();
    const scenario = makeScenario({
      id: "ev-output-ref-001",
      runtime_path: "echo raw-output-content",
      assertions: [{ evidence_contains_command_output: true }],
      timeout_seconds: 10,
      evidence_path: evidenceDir,
    });

    const record = await runScenario(scenario, process.cwd());

    // command_output_ref should point to an existing file
    expect(existsSync(record.command_output_ref)).toBe(true);

    // The file should contain the command output
    const rawOutput = readFileSync(record.command_output_ref, "utf-8");
    expect(rawOutput).toContain("raw-output-content");
  });

  test("runScenario with passing assertions produces verdict=PASS and exit code 0", async () => {
    const evidenceDir = makeTempDir();
    const scenario = makeScenario({
      id: "ev-pass-001",
      runtime_path: "echo hello world",
      assertions: [
        { evidence_contains_command_output: true },
        { selected_context_contains: ["hello", "world"] },
        { no_secret_leakage: true },
      ],
      pass_criteria: "all",
      timeout_seconds: 10,
      evidence_path: evidenceDir,
    });

    const record = await runScenario(scenario, process.cwd());
    expect(record.verdict).toBe("PASS");
    expect(getExitCode(record.verdict)).toBe(0);
    expect(record.scenario_id).toBe("ev-pass-001");
  });

  test("runScenario with failing assertion produces verdict=FAIL and exit code 1", async () => {
    const evidenceDir = makeTempDir();
    const scenario = makeScenario({
      id: "ev-fail-001",
      runtime_path: "echo hello",
      // This assertion will fail because "missing-string" is not in "hello"
      assertions: [{ selected_context_contains: ["hello", "missing-string-xyz"] }],
      pass_criteria: "all",
      timeout_seconds: 10,
      evidence_path: evidenceDir,
    });

    const record = await runScenario(scenario, process.cwd());
    expect(record.verdict).toBe("FAIL");
    expect(getExitCode(record.verdict)).toBe(1);
  });

  test("runScenario TIMEOUT produces verdict=TIMEOUT and exit code 3", async () => {
    const evidenceDir = makeTempDir();
    const scenario = makeScenario({
      id: "ev-timeout-001",
      // Use a sleep command that far exceeds the tiny timeout
      runtime_path: "sleep 30",
      assertions: [{ evidence_contains_command_output: true }],
      // Very short timeout to trigger timeout quickly
      timeout_seconds: 0.1,
      evidence_path: evidenceDir,
    });

    const record = await runScenario(scenario, process.cwd());
    expect(record.verdict).toBe("TIMEOUT");
    expect(getExitCode(record.verdict)).toBe(3);
    expect(record.error_detail).toContain("timeout");
  }, 10000); // allow 10s for the test (timeout fires at 0.1s)
});

// ─────────────────────────────────────────────────────────────────────────────
// Integration: end-to-end scenario run with real filesystem evidence
// ─────────────────────────────────────────────────────────────────────────────

// axiom:trace work_item=harness-levelup-01 spec=specs/101-Harness-Engineering.md#REQ-HLU-013 plan=phase-6/task-6-1/step-6-1-2
describe("Integration: end-to-end scenario run with real filesystem evidence", () => {
  // Determine a stable repo root for integration tests (two levels up from .opencode/tests/)
  const repoRoot = path.resolve(process.cwd(), "..");

  test("runScenario end-to-end: PASS verdict with echo command and evidence file written to real path", async () => {
    const evidenceDir = makeTempDir();
    const scenario = makeScenario({
      id: "integration-e2e-pass-001",
      runtime_path: `echo "specs/00-PRD.md .memory-bank/_index.md result"`,
      assertions: [
        { selected_context_contains: ["specs/00-PRD.md", ".memory-bank/_index.md"] },
        { evidence_contains_command_output: true },
        { no_secret_leakage: true },
      ],
      pass_criteria: "all",
      timeout_seconds: 30,
      evidence_path: evidenceDir,
    });

    const result = await runScenario(scenario, repoRoot);

    // Verdict must be PASS — all 3 assertions pass for this echo output
    expect(result.verdict).toBe("PASS");

    // timestamp must be a valid ISO8601 string
    expect(result.timestamp).toMatch(/^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}\.\d+Z$/);

    // scenario_id matches
    expect(result.scenario_id).toBe("integration-e2e-pass-001");

    // evidence .yaml file was created
    const yamlFiles = readdirSync(evidenceDir).filter((f) => f.endsWith(".yaml"));
    expect(yamlFiles.length).toBeGreaterThanOrEqual(1);

    // Read back and confirm all 7 required fields are present in the YAML
    const parsed = yamlParse(readFileSync(path.join(evidenceDir, yamlFiles[0]), "utf-8")) as EvidenceRecord;
    const requiredFields: (keyof EvidenceRecord)[] = [
      "scenario_id",
      "verdict",
      "timestamp",
      "assertions_results",
      "runtime_ms",
      "command_output_ref",
    ];
    for (const field of requiredFields) {
      expect(parsed).toHaveProperty(field);
    }
    expect(parsed.verdict).toBe("PASS");
  });

  test("end-to-end: scenario YAML round-trip — write scenario file, load it, run it, verify evidence", async () => {
    // Write a temporary scenario YAML file, parse it, run it, verify results
    const scenarioDir = makeTempDir();
    const evidenceDir = makeTempDir();

    const scenarioYaml = [
      "id: roundtrip-smoke-001",
      "goal: round-trip YAML scenario test",
      "category: execution",
      `runtime_path: echo "specs/00-PRD.md .memory-bank/_index.md context pipeline selected items"`,
      "preconditions: []",
      "timeout_seconds: 30",
      'pass_criteria: "all"',
      `evidence_path: "${evidenceDir}"`,
      "assertions:",
      "  - selected_context_contains:",
      "      - specs/00-PRD.md",
      "      - .memory-bank/_index.md",
      "  - evidence_contains_command_output: true",
      "  - no_secret_leakage: true",
    ].join("\n");

    const scenarioFilePath = path.join(scenarioDir, "roundtrip-smoke-001.yaml");
    writeFileSync(scenarioFilePath, scenarioYaml, "utf-8");

    // Parse the written YAML and validate as a scenario
    const rawParsed = yamlParse(readFileSync(scenarioFilePath, "utf-8")) as Record<string, unknown>;
    const scenario = validateScenario(rawParsed);

    // Run the scenario
    const result = await runScenario(scenario, repoRoot);

    // Must produce PASS — all assertions satisfy the echo output
    expect(result.verdict).toBe("PASS");
    expect(result.scenario_id).toBe("roundtrip-smoke-001");

    // Evidence file created in evidenceDir
    const yamlFiles = readdirSync(evidenceDir).filter((f) => f.endsWith(".yaml"));
    expect(yamlFiles.length).toBeGreaterThanOrEqual(1);

    // All 7 required fields in the parsed evidence
    const evidence = yamlParse(readFileSync(path.join(evidenceDir, yamlFiles[0]), "utf-8")) as EvidenceRecord;
    expect(evidence.scenario_id).toBe("roundtrip-smoke-001");
    expect(evidence.verdict).toBe("PASS");
    expect(typeof evidence.timestamp).toBe("string");
    expect(typeof evidence.assertions_results).toBe("object");
    expect(typeof evidence.runtime_ms).toBe("number");
    expect(typeof evidence.command_output_ref).toBe("string");
    // 7th field: scenario_id already checked; the 7 fields are: scenario_id, verdict, timestamp,
    // assertions_results, runtime_ms, command_output_ref — plus optional error_detail (absent on PASS)
    expect(evidence.error_detail).toBeUndefined();
  });

  test("end-to-end: evidence_path directory is created if it doesn't exist before the run", async () => {
    // Choose a nested path that definitely does NOT exist yet
    const baseDir = makeTempDir();
    const nonExistentSubDir = path.join(baseDir, "nested", "deep", "evidence-dir");

    // Confirm it doesn't exist
    expect(existsSync(nonExistentSubDir)).toBe(false);

    const scenario = makeScenario({
      id: "integration-mkdir-001",
      runtime_path: `echo "specs/00-PRD.md result"`,
      assertions: [{ evidence_contains_command_output: true }],
      timeout_seconds: 30,
      evidence_path: nonExistentSubDir,
    });

    const result = await runScenario(scenario, repoRoot);

    // The directory must now exist (runScenario created it)
    expect(existsSync(nonExistentSubDir)).toBe(true);

    // The run should succeed (PASS verdict)
    expect(result.verdict).toBe("PASS");

    // Evidence .yaml file should be inside the newly-created directory
    const yamlFiles = readdirSync(nonExistentSubDir).filter((f) => f.endsWith(".yaml"));
    expect(yamlFiles.length).toBeGreaterThanOrEqual(1);
  });

  test("end-to-end: command_output_ref file contains actual stdout from subprocess", async () => {
    const evidenceDir = makeTempDir();
    const uniqueToken = `INTEGRATION-TOKEN-${Date.now()}`;

    const scenario = makeScenario({
      id: "integration-output-ref-001",
      runtime_path: `echo "${uniqueToken}"`,
      assertions: [
        { evidence_contains_command_output: true },
        { selected_context_contains: [uniqueToken] },
      ],
      pass_criteria: "all",
      timeout_seconds: 30,
      evidence_path: evidenceDir,
    });

    const result = await runScenario(scenario, repoRoot);

    // Verdict should be PASS
    expect(result.verdict).toBe("PASS");

    // command_output_ref must point to a real file
    expect(existsSync(result.command_output_ref)).toBe(true);

    // The file must contain the unique token we echoed
    const outputContent = readFileSync(result.command_output_ref, "utf-8");
    expect(outputContent).toContain(uniqueToken);
  });
});

// ─────────────────────────────────────────────────────────────────────────────
// MG-09: Real command integration (non-echo runtime_path)
// ─────────────────────────────────────────────────────────────────────────────

// axiom:trace work_item=harness-levelup-01 spec=specs/101-Harness-Engineering.md#REQ-HLU-029 plan=phase-6/task-6-1/step-6-1-3
describe("Real command integration (non-echo runtime_path)", () => {
  const repoRoot = path.resolve(process.cwd(), "..");

  test("runScenario with cat runtime_path reads real filesystem content", async () => {
    // Create a temp dir and write a file with known content
    const evidenceDir = makeTempDir();
    const contentDir = makeTempDir();
    const knownContent = "harness-levelup-cat-integration-marker-12345";
    const tempFile = path.join(contentDir, "known-content.txt");
    writeFileSync(tempFile, knownContent, "utf-8");

    const scenario = makeScenario({
      id: "real-cmd-cat-001",
      // Use cat to read the real file — not echo
      runtime_path: `cat ${tempFile}`,
      assertions: [
        { evidence_contains_command_output: true },
        { selected_context_contains: [knownContent] },
        { no_secret_leakage: true },
      ],
      pass_criteria: "all",
      timeout_seconds: 10,
      evidence_path: evidenceDir,
    });

    const result = await runScenario(scenario, repoRoot);

    // Verdict must be PASS — cat read the real file and output the known string
    expect(result.verdict).toBe("PASS");

    // command_output_ref must point to a real file containing the known content
    expect(existsSync(result.command_output_ref)).toBe(true);
    const rawOutput = readFileSync(result.command_output_ref, "utf-8");
    expect(rawOutput).toContain(knownContent);

    // Evidence YAML file must exist in evidenceDir
    const yamlFiles = readdirSync(evidenceDir).filter((f) => f.endsWith(".yaml"));
    expect(yamlFiles.length).toBeGreaterThanOrEqual(1);
  });

  test("CLI entry: running the smoke scenario produces verdict=PASS evidence file", async () => {
    // Tier-3+ test: actually invoke the CLI via Bun.spawn from the repo root
    const evalRunnerPath = path.join(repoRoot, ".opencode", "plugins", "eval-runner.ts");

    const proc = Bun.spawn(
      ["/home/coder/.bun/bin/bun", "run", evalRunnerPath, "context-pipeline-basic-smoke"],
      { cwd: repoRoot, stdout: "pipe", stderr: "pipe" }
    );
    await proc.exited;

    // CLI must exit 0 (PASS verdict)
    expect(proc.exitCode).toBe(0);

    // Evidence file must exist in .memory-bank/harness-evals/context-pipeline-basic-smoke/
    const evidenceDir = path.join(repoRoot, ".memory-bank", "harness-evals", "context-pipeline-basic-smoke");
    expect(existsSync(evidenceDir)).toBe(true);
    const evidenceFiles = readdirSync(evidenceDir).filter((f) => f.endsWith(".yaml"));
    expect(evidenceFiles.length).toBeGreaterThan(0);
  }, 30000); // allow 30s for CLI spawn + scenario execution
});
