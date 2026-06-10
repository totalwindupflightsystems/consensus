---
description: Execute one plan step and report evidence.
agent: dispatch-axiom
---

You are executing a single Axiom plan step.

Inputs:
- Work item id: $WORK_ITEM_ID
- Repo: $REPO
- Phase id: $PHASE_ID
- Task id: $TASK_ID
- Step id: $STEP_ID
- Arguments: $ARGUMENTS (optional: `auto_inject=true|false` to control backlog injection before execution, default: true)

Skills (load on demand):
- `axiom-copilot` — If the user is new to step execution, load this to explain what's happening and why.
- `test-quality-gates-axiom` — Deep guidance on test quality requirements.
- `baby-steps-methodology` — Reinforces the smallest-meaningful-change approach.
- `evidence-bundle-schema` — For understanding what evidence to capture.
- `jira-workflow-axiom` — Jira operating model (load when the work item is Jira-sourced).
- `code-graph-intelligence-axiom` — Load **during planning** (before writing code) when the step changes shared code; run blast-radius to identify all callers/callees. Do NOT load mid-execution as a substitute for prior planning.
- `code-analysis-axiom` — Load after implementation when the step's acceptance criteria include a specific health score target; run `axiom analyze --score` to verify no quality regression.

Do:
1) Read the current plan for $WORK_ITEM_ID.
2) **Backlog Injection** (when `auto_inject=true`, which is the default, AND no explicit $STEP_ID is provided):
   - Read `.memory-bank/work-items/$WORK_ITEM_ID/findings-backlog.md` if it exists.
   - For each finding NOT labeled `[IMPOSSIBLE]`, `[DEFERRED-INDEFINITELY]`, or `[WONT-FIX]`:
     a) Skip if a step with the same objective already exists in the plan (dedup).
     b) Inject as a new plan step (same rules as step-loop Phase 1, step 2).
     c) Remove injected findings from backlog.
   - If new steps were injected and no explicit $STEP_ID was provided, execute the highest-priority injected step.
   - If an explicit $STEP_ID is provided, execute THAT step regardless of backlog (backlog injection still happens for future iterations).
3) **Jira context**: If `plan.yaml` has a non-null `jira_key`, note it for trace markers and evidence posting.
4) Execute only the specified step (or the first auto-injected step if no $STEP_ID).
5) Run the minimum required validations for that step.
5) **Run the test quality gate** (required before marking status=ok):
   ```bash
   python3 .axiom/scripts/check_test_quality.py \
     --test-dir .axiom/tests \
     --config .axiom/axiom.config.yaml \
     --json > test-quality-report.json
   ```
   - If exit code 1 (hard fail): set status=fail, inject corrective steps to fix assertionless/tautology violations.
   - If exit code 2 (soft fail): record in evidence, proceed but flag for follow-up.
6) **Run the runtime tier command** for the configured `required_min_tier` (default: Tier 3):
   ```bash
   axiom run --work-item smoke-test --repo . --in-process
   ```
   Record the highest tier reached as `evidence.runtime_tier_reached`.
   - If tier reached < required_min_tier: set status=fail, inject corrective step.
7) **Jira evidence posting** (when `plan.yaml` has a non-null `jira_key` and Atlassian MCP is available):
   a) Post a progress comment to Jira summarizing the step outcome (pass/fail, key evidence).
   b) Include `jira_ref=<KEY>` in all `axiom:trace` markers for this step.
   c) If the step is blocked, post a blocker comment to Jira.
   d) If MCP is unavailable, note "Jira sync deferred" in evidence and proceed.
8) Update the Memory Bank work-item files as required.

Output:
- Emit the required XML envelope per `.opencode/skills/axiom-xml-protocol/SKILL.md` and required tags in `.axiom/command-registry.yaml`.
- Required tags include `evidence.runtime_tier_reached`.
- Include `evidence.tests_quality.score`, `evidence.tests_quality.assertionless`, `evidence.tests_quality.tautology` when tests were touched.
- A step MUST NOT be marked `status=ok` if:
  - The test quality gate hard-fails (assertionless or tautology violations > 0).
  - `evidence.runtime_tier_reached` is below `verification.runtime.required_min_tier` in config.

## Output Contract (what to return to the caller)

### For Human Consumption
- Summary: one sentence stating which step ran and whether it passed.
- Confidence: 0-100

### For Agent Consumption (MUST include)
- `evidence.files_changed`: list of ALL files created/modified (full paths, semicolon-separated)
  - Code files changed, test files changed, evidence bundle files
- `evidence.run_path`: full path to the run bundle (`.memory-bank/work-items/<id>/runs/<RUN_ID>/`)
- `evidence.verification_path`: full path to `verification.md` for this run
- `evidence.runtime_tier_reached`: highest runtime tier achieved (0-5)
- `evidence.tests_quality.score`: test quality score (0-100)
- `evidence.step_id`: the step ID that was executed (e.g., `phase-1/task-1-1/step-1-1-1`)
- `evidence.step_source`: where the step came from — `"plan"` (original plan) or `"backlog-auto-inject"` (from findings-backlog.md)
- `evidence.backlog_items_injected`: count of findings auto-injected from backlog during this run
- `related_commands`: suggested follow-up commands
  - "To verify the step, run: `/axiom-verify --work-item <id>`"
  - "To execute the next step, run: `/axiom-step --work-item <id>` (with next step IDs)"
  - "To run all remaining steps, run: `/axiom-step-loop --work-item <id>`"
  - "To skip backlog injection, run: `/axiom-step --work-item <id> auto_inject=false`"

### Cross-References
- "Run evidence is at: `.memory-bank/work-items/<id>/runs/<RUN_ID>/`"
- "Test quality gate: `.axiom/scripts/check_test_quality.py`"
- "Spec: `specs/48-Test-Quality-Gates.md`"

See: `specs/48-Test-Quality-Gates.md`, `.opencode/skills/test-quality-gates-axiom/SKILL.md`

axiom:trace spec=specs/48-Test-Quality-Gates.md work_item=command-quality-01
