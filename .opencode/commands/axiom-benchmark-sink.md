---
description: Bootstrap, score, check status, or compare Benchmark Sink subscriptions
agent: tower-axiom
---

Run the OpenCode command surface for `/axiom-benchmark-sink`.

Inputs:
- `--subscription <id>` (required): subscription id from `.axiom/axiom.config.yaml`.
- `--bootstrap` (optional): run Behavior item 1 bootstrap workflow.
- `--score` (optional): run Behavior item 2 scoring (read-only, no changes).
- `--status` (optional): run Behavior item 3 status display (latest scores, trend, divergence).
- `--compare` (optional): run Behavior item 4 comparison of two benchmark runs.
- `--run-a <timestamp>` (optional, with `--compare`): first run to compare.
- `--run-b <timestamp>` (optional, with `--compare`): second run to compare.
- `--model <name>` (optional): override model metadata in scorecard.
- `--dry-run` (optional): stop after analysis and report planned actions.

Skills (load on demand):
- `axiom-xml-protocol` — XML envelope format and required tag set.
- `enterprise-testing-standard` — When `--score` is used, apply tiered verification to benchmark evidence.
- `evidence-bundle-schema` — For structuring benchmark run evidence (verification.md + outputs.md).

Do:
1) Read `.axiom/axiom.config.yaml` and find `upstream_tracking.subscriptions[<id>]`.
2) Validate the subscription exists and `mode=benchmark_sink`.
3) Based on the flag provided, call the appropriate handler:
   - `--bootstrap`: `axiom.cli.benchmark_sink.BenchmarkSinkBootstrapHandler.run(...)`
   - `--score`: `axiom.cli.benchmark_sink.BenchmarkSinkScoreHandler.run(...)`
   - `--status`: `axiom.cli.benchmark_sink.BenchmarkSinkStatusHandler.run(...)`
   - `--compare`: `axiom.cli.benchmark_sink.BenchmarkSinkCompareHandler.run(...)`
4) Format and return output via the corresponding `format_*_result(...)` function.

Spec ref: `specs/42-Upstream-Tracking-And-Fork-Management.md#local-cli-commands`

## Output Contract (what to return to the caller)

### For Human Consumption
- Summary: one sentence stating which operation ran and whether it succeeded.
- Confidence: 0-100

### For Agent Consumption (MUST include)
- `evidence.files_changed`: list of ALL files created/modified (full paths, semicolon-separated)
  - For `--bootstrap`: scorecard file path, run bundle path
  - For `--score`: latest scorecard path, run evidence path
  - For `--status`: no files changed (read-only)
  - For `--compare`: comparison report path
- `evidence.run_path`: path to the run bundle (`.memory-bank/work-items/<id>/runs/<RUN_ID>/`)
- `evidence.scorecard_path`: path to the latest scorecard file
- `related_commands`: suggested follow-up commands
  - "To view status after scoring, run: `/axiom-benchmark-sink --subscription <id> --status`"
  - "To compare two runs, run: `/axiom-benchmark-sink --subscription <id> --compare --run-a <ts1> --run-b <ts2>`"

### Cross-References
- "To file a benchmark regression as a Jira ticket, run: `/axiom-report-issue title=... type=bug`"
- "To verify the benchmark subscription config, check: `.axiom/axiom.config.yaml`"

axiom:trace work_item=benchmark-sink-01 spec=specs/42-Upstream-Tracking-And-Fork-Management.md#local-cli-commands plan=phase-24/task-24-14 test=.axiom/tests/cli/test_agent_cmd_benchmark_compare.py doc=.opencode/commands/axiom-benchmark-sink.md prompt= evidence= commit=
axiom:trace work_item=command-quality-01 spec=specs/13-Command-Registry.md
