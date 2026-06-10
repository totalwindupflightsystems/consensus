---
description: "DeeDee QA Harness — Gherkin-style scenario-based QA with diagnostic triage. Runs test scenarios, diagnoses failures, and produces structured triage reports."
agent: tower-axiom
---

Run DeeDee QA scenarios using the `deedee` binary.

Inputs:
- Subcommand: $SUBCOMMAND (required; one of: run, status, list, report, diff, baseline, stale, replay, convert)
- Scenario set: $SCENARIO_SET (optional; filter by tag or directory)
- Budget: $BUDGET (optional; e.g., 20-usd or 100-scenarios)
- Parallel: $PARALLEL (optional; number of parallel workers; default: 1)
- Diagnostic model: $DIAGNOSTIC_MODEL (optional; model for diagnostic stages)
- Executor model: $EXECUTOR_MODEL (optional; model for scenario execution)
- Auto fix: $AUTO_FIX_SCENARIOS (optional; true to apply scenario YAML fixes; default: false)
- Flaky retries: $FLAKY_RETRIES (optional; immediate retries for flakiness detection; default: 1)
- Fail fast: $FAIL_FAST (optional; true to stop on first failure; default: false)
- Run ID: $RUN_ID (optional; for status/report/diff/replay subcommands)
- Scenario: $SCENARIO (optional; for replay subcommand)
- Format: $FORMAT (optional; table or json; default: table)

Routes to the `deedee` binary (morty/cmd/deedee/).

REQ-DEEDEE-060: /axiom-qa slash command registered.
REQ-DEEDEE-066: Dual CLI surface (deedee binary + /axiom-qa slash command).

Skills (load on demand):
- `deedee-qa-harness-axiom` — Full QA harness configuration, scenario registry, diagnostic sub-stages.
- `test-quality-gates-axiom` — Test value doctrine and quality enforcement.
- `code-graph-intelligence-axiom` — Optional. Load when QA scenarios cover shared code and you need blast-radius analysis to determine if test scope is sufficient for the changed callers/callees.
- `code-analysis-axiom` — Optional. Load for health score comparison before/after QA runs.

axiom:trace work_item=deedee-qa-harness-01 spec=specs/99-DeeDee-QA-Harness.md#REQ-DEEDEE-060,REQ-DEEDEE-066 plan=phase-4/task-4-3/step-4-3-1 jira_ref=SWDE-37

Do:

1. Validate $SUBCOMMAND is one of: run, status, list, report, diff, baseline, stale, replay, convert.
2. Route to the appropriate `deedee` subcommand with the provided flags.
3. For `run`: execute `deedee run [--scenario-set $SCENARIO_SET] [--budget $BUDGET] [--parallel $PARALLEL] [--diagnostic-model $DIAGNOSTIC_MODEL] [--executor-model $EXECUTOR_MODEL] [--auto-fix-scenarios $AUTO_FIX_SCENARIOS] [--flaky-retries $FLAKY_RETRIES] [--fail-fast $FAIL_FAST]`
4. For `status`: execute `deedee status [--run $RUN_ID]`
5. For `report`: execute `deedee report [--run $RUN_ID]`
6. For `diff`: execute `deedee diff [--run $RUN_ID]`
7. For `replay`: execute `deedee replay [--run $RUN_ID] [--scenario $SCENARIO]`
8. For `convert`: execute `deedee convert <scenario-file>`
9. Return the output to the user.
