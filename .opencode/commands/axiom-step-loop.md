---
description: Execute plan steps with configurable parallelism — sequential in one context, sequential across contexts, or fully parallel.
agent: dispatch-axiom
---

You are executing a multi-step Axiom plan loop. You support three execution strategies that control how steps are dispatched to subagents.

Inputs:
- Work item id: $WORK_ITEM_ID
- Repo: $REPO (default: `.`)
- Strategy: $STRATEGY (optional; controls how steps are dispatched. Default: "sequential-isolated")
- Arguments: $ARGUMENTS (optional: `from=<phase/task/step>` to start from a specific step, `max_steps=N` to cap iterations, `dry_run=true` to list steps without executing, `auto_inject=true|false` to control backlog injection, default: true, `mode=kiss` to enable KISS filtering on backlog injection, `mode=full-queue` to load and execute the entire step queue autonomously without stopping for human input between phases, `--kiss` shorthand for `mode=kiss`)

### Execution Strategies

| Strategy | How it works | When to use |
|---|---|---|
| `sequential-single` | All steps execute in ONE subagent context window, one at a time. The subagent keeps full context across steps. | Small plans (<5 steps), tightly coupled steps that share context, when you want the agent to "remember" what it did in previous steps. |
| `sequential-isolated` (default) | Each step gets its OWN fresh subagent context window, one at a time. Previous step's result is summarized and passed as context. | Most work. Prevents context pollution. Each step starts clean. Default because it's safest. |
| `parallel` | All remaining steps dispatch to SEPARATE subagent context windows simultaneously. Results are collected and merged. | Independent steps that don't depend on each other (e.g., "write docs" + "add tests" + "update config" can all run at once). Steps with dependencies MUST NOT use this mode. |
| `batch` | Analyze step dependencies, group into **waves** of independent steps, execute each wave in parallel, then advance to the next wave. Respects ordering constraints while maximizing parallelism. | Plans with a mix of dependent and independent steps. The smart default for large plans. |

**How `batch` works:**

1. **Dependency analysis**: Read `plan.yaml` and identify dependencies between steps:
   - Explicit `depends_on` fields
   - Shared file paths (step A writes `foo.go`, step B reads `foo.go` → B depends on A)
   - Ordering constraints (step B references step A's output)

2. **Wave grouping**: Group steps into waves where all steps within a wave are independent:
   ```
   Wave 1: [step-1, step-3, step-5]  ← no dependencies between these
   Wave 2: [step-2, step-4]          ← depend on wave 1 results
   Wave 3: [step-6]                  ← depends on wave 2 results
   ```

3. **Execute wave by wave**: For each wave, dispatch all steps in parallel (like `parallel` mode). Wait for the entire wave to complete. Evaluate results. If any step in the wave fails, stop — do not advance to the next wave.

4. **Commit between waves**: After each wave completes successfully, commit all changes. This ensures the next wave starts from a clean, verified state.

**How to choose**: The agent SHOULD auto-select the strategy based on the plan structure:
- If steps have `depends_on` fields with a mix of dependent and independent → `batch`
- If steps are all independent (no shared files, no ordering requirement) → `parallel`
- If steps are all sequential (each depends on the previous) → `sequential-isolated`
- If the plan is small and steps build on each other → `sequential-single`
- If unsure → `sequential-isolated` (safest default)

The user can override with `$STRATEGY` to force a specific mode.

**`mode=full-queue`**: When this argument is set, the loop loads the **entire step queue** — across all phases, all tasks, all injected steps — and executes it autonomously from start to finish without pausing for human input between phases. This is the "run it all" mode. The loop:
1. Builds the complete step queue (all incomplete steps, all phases, respecting priority ordering).
2. Auto-selects the best execution strategy for the full queue (defaults to `batch` to maximise parallelism while respecting dependencies).
3. Executes every wave/step in sequence, committing between waves.
4. After each wave completes, **automatically runs `/axiom-verify --mode quick`** on the work item to catch regressions before advancing. If verify fails, the loop stops and reports — it does NOT continue blindly.
5. On completion of all steps, automatically runs a final `/axiom-verify --mode full` to produce the milestone gate evidence.
6. Reports a single consolidated summary at the end covering all steps executed.

**When to use `full-queue`**: Use when the plan is well-understood, the steps are well-defined, and you want the agent to drive the entire work item to completion without you pressing "next" after each phase. Not recommended for exploratory or high-risk work where human review between phases is valuable.

**Safety guardrails in `full-queue` mode**:
- A step that fails its quality gate still stops the loop — `full-queue` does not bypass fail-closed rules.
- The inter-wave verify check catches regressions early so the loop doesn't compound failures across phases.
- `max_steps=N` still applies as a hard cap if set.
- If a step is blocked (missing prerequisite, environment issue), the loop stops and reports rather than skipping.

Inputs:
- Work item id: $WORK_ITEM_ID
- Repo: $REPO (default: `.`)
- Arguments: $ARGUMENTS (optional: `from=<phase/task/step>` to start from a specific step, `max_steps=N` to cap iterations, `dry_run=true` to list steps without executing, `mode=kiss` to filter backlog injection to AC-blocking findings only, `mode=full-queue` to run the entire queue autonomously)

Skills (load on demand):
- `kiss-axiom` — Load when `mode=kiss` is set in $ARGUMENTS. Filters backlog injection: only inject findings that map to an AC. Prevents perfection-chasing steps from entering the plan.
- `research-and-recon-axiom` — **Load when a step is blocked, failing, or requires knowledge of an external library/API/protocol.** Covers when and how to search the web, read documentation, and investigate errors. Key rule: always get the current date before searching to avoid stale results. Don't guess or rely solely on training data — search first. Available tools: `searxng_searxng_web_search`, `searxng_web_url_read`, `webfetch`.
- `axiom-copilot` — If the user is new to step execution, load this to explain what's happening.
- `test-quality-gates-axiom` — Deep guidance on test quality requirements.
- `baby-steps-methodology` — Reinforces the smallest-meaningful-change approach.
- `evidence-bundle-schema` — For understanding what evidence to capture.
- `expected-output-axiom` — Load when evaluating step results. Defines how to declare what output you expect from verification commands BEFORE running them, then compare expected vs actual. Prevents "it didn't crash" from being confused with "it produced correct results."
- `jira-workflow-axiom` — Jira operating model (load when the work item is Jira-sourced).
- `runtime-completeness-gate-axiom` — Load when a step claims "done" to verify the real operator path works.
- `code-graph-intelligence-axiom` — Optional. Load when stuck or when changes touch shared code with many consumers. Helps identify blast radius and callers/callees you might be missing. Not required for every step.
- `code-analysis-axiom` — Optional. Load when the loop produces code and you want to verify health score hasn't regressed. Run `axiom analyze --score` between steps for quality confidence.

Do:

### Phase 1: Plan Discovery and Backlog Injection

> **Pre-Execution Gate (MANDATORY — from `axiom-meta-planning-contract`):**
> Before executing ANY step, verify these files exist at `.memory-bank/work-items/$WORK_ITEM_ID/`:
> - `meta-planning.md` — If missing: STOP. Run `/axiom-meta-plan --work-item $WORK_ITEM_ID` first.
> - `plan.md` — If missing: STOP. Run `/axiom-meta-plan --work-item $WORK_ITEM_ID` first.
> - `plan.yaml` — If missing: STOP. Run `/axiom-meta-plan --work-item $WORK_ITEM_ID` first.
>
> If any file is absent, do NOT proceed to step execution. Report the gap and either:
> (a) Create the missing artifacts yourself using the `axiom-meta-planning-contract` skill, OR
> (b) Return status=blocked with the instruction to run `/axiom-meta-plan` first.
>
> **Why**: Production use showed 5 of 6 work items executing without plans, leaving future agents with no structured context.

> **git.branching config**: When `git.branching.mode: auto-branch` is set in
> `.axiom/axiom.config.yaml`, branch creation and on-complete actions are
> handled automatically. The step-loop does not read this config directly —
> instead, each subagent dispatched by the loop calls `axiom run` or the
> HTTP API, which reads the config and passes `skip_branch_setup=False` to the
> orchestrator. The orchestrator then calls `_ensure_codeops_branch()` at run
> start and `execute_on_complete()` at run end. This is the correct indirect
> path — the step-loop itself does not need to be config-aware.
> axiom:trace work_item=branch-management-02 spec=specs/67-Go-Agent-Orchestration-Engine.md jira_ref=DEX-62 plan=phase-3/task-3-1/step-3-1-2

1) Read the plan for `$WORK_ITEM_ID`:
   - `.memory-bank/work-items/$WORK_ITEM_ID/plan.yaml`
   - `.memory-bank/TODO.md` (for current status of steps)

2) **Backlog Injection** (when `auto_inject=true`, which is the default):
   - Read `.memory-bank/work-items/$WORK_ITEM_ID/findings-backlog.md` if it exists.
   - For each finding in the backlog:
     a) **Skip** if the finding is labeled `[IMPOSSIBLE]`, `[DEFERRED-INDEFINITELY]`, or `[WONT-FIX]`. These are explicitly excluded from automatic execution.
     b) **Skip** if a step with the same objective text already exists in the plan (dedup check).
     c) **If `mode=kiss`**: Before injecting, load the `kiss-axiom` skill and check whether the finding maps to an acceptance criterion in `plan.yaml`. If it does NOT map to an AC — i.e., it's a polish/hardening/perfection finding — skip injection and add `[WONT-FIX]` label to the finding in findings-backlog.md with rationale: "KISS: does not block an AC." Only inject findings that directly block an AC.
     d) **Inject** as a new plan step: convert the finding into an executable step with:
        - `objective`: derived from the finding title/description
        - `actions`: the recommended fix from the finding
        - `verification`: the acceptance criteria from the finding (or "verify the fix resolves the original finding")
        - `source`: `"backlog-auto-inject"` (marks this as auto-injected for traceability)
        - `priority`: derived from the finding severity (CRITICAL → first, HIGH → next, MEDIUM → after existing steps)
   - Write injected steps to `plan.yaml` and add to `.memory-bank/TODO.md` as unchecked items.
   - Remove successfully injected findings from `findings-backlog.md` (they now live in the plan).
   - Log: "Auto-injected N findings from backlog into plan (skipped M impossible/deferred items, K KISS-filtered)"

3) Build the **step queue**: an ordered list of all steps that are not yet `status=ok`.
   - If `from=` is specified in `$ARGUMENTS`, start from that step.
   - If the plan has an `execution.cursor`, start from the cursor position.
   - Otherwise start from the first incomplete step.
   - **If `mode=full-queue`**: Load ALL incomplete steps across ALL phases and tasks into the queue — do not stop at phase boundaries. The queue spans the entire plan.
   - **Priority ordering**: CRITICAL-sourced steps first, then HIGH, then the natural plan order.
4) If `max_steps=N` is specified, cap the queue at N steps.
5) If `dry_run=true`, output the step queue (including any auto-injected steps) and stop.
6) **Jira context**: If `plan.yaml` has a non-null `jira_key`, note it for trace markers.

### Phase 2: Step Execution (strategy-dependent)

**If $STRATEGY is "sequential-single":**

6a) Dispatch ONE subagent with ALL remaining steps in the queue. The subagent prompt includes:
   - The full step queue (all steps, in order)
   - Instruction: "Execute each step in order. After each step, run validations. If any step fails, stop and report. Keep full context across steps."
   - The subagent executes all steps in its single context window.
   - After the subagent returns, evaluate ALL results at once (step 7).

**If $STRATEGY is "sequential-isolated" (default):**

6b) For each step in the queue, dispatch a SEPARATE subagent:
   - Each subagent gets a fresh context window.
   - The subagent prompt includes:
     - The work item id, phase id, task id, and step id
     - The step's objective, actions, and verification criteria from plan.yaml
     - A summary of what previous steps accomplished (not the full context)
     - The instruction: "Execute this single step. Run validations. Report what you did, what files changed, and what evidence you captured. Do NOT proceed to the next step."
   - After each subagent returns, evaluate the result (step 7) before dispatching the next.

**If $STRATEGY is "parallel":**

6c) Dispatch ALL remaining steps simultaneously, each to a SEPARATE subagent:
   - Each subagent gets a fresh context window with its specific step.
   - All subagents run in parallel (use multiple Task tool calls in one message).
   - **IMPORTANT**: Only use this when steps are truly independent. If step B depends on step A's output, they MUST NOT run in parallel.
   - After ALL subagents return, evaluate ALL results (step 7).
   - If any step fails, the others' results are still valid (they ran independently).

**If $STRATEGY is "batch":**

6d) Analyze dependencies and execute in waves:
   1. Build dependency graph from `plan.yaml` (`depends_on` fields + shared file analysis).
   2. Topological sort into waves (steps with no unmet dependencies form the next wave).
   3. For each wave:
      a. Dispatch all steps in the wave simultaneously (like `parallel` mode).
      b. Wait for ALL steps in the wave to complete.
      c. Evaluate ALL results (step 7).
      d. If any step in the wave fails → STOP. Do not advance to the next wave.
      e. If all steps pass → commit all changes (`git add -A && git commit`), then advance to next wave.
   4. Between waves: summarize what the previous wave accomplished and pass as context to the next wave's subagents.
   5. Report progress: "Wave 1 complete (3/3 steps). Wave 2 starting (2 steps)..."

For all strategies:
7) **Evaluate the subagent result(s)**:
   - If the subagent reports success with evidence:
     a) Run the **test quality gate** yourself (tower):
        ```bash
        python3 .axiom/scripts/check_test_quality.py \
          --test-dir .axiom/tests \
          --config .axiom/axiom.config.yaml \
          --json
        ```
     b) Run the **runtime tier command** if applicable:
        ```bash
        axiom run --work-item smoke-test --repo . --in-process
        ```
     c) If both gates pass: mark the step `status=ok` in TODO.md, update the plan cursor, create a run bundle.
     d) If a gate fails: mark `status=fail`, record why, and **stop the loop** (do not proceed to the next step).
   - If the subagent reports failure or is blocked:
     a) Record the failure in the evidence bundle.
     b) **Stop the loop.** Do not proceed to the next step.
     c) Report what failed and what the injected next steps are.

8) **Jira evidence posting** (when `plan.yaml` has a non-null `jira_key` and Atlassian MCP is available):
    - Post a progress comment after each step completion.
    - Post a blocker comment if the loop stops on failure.

9) **Memory bank update** (after EACH step, NOT optional):
    - Update `.memory-bank/work-items/$WORK_ITEM_ID/verification.md` with step evidence
    - Record any decisions made during the step
    - If patterns/findings discovered, write to `.memory-bank/findings/`
    - **Preferred:** Call `@memory-bank-axiom` to handle structure and indexes
    - **Acceptable:** Write directly following `.memory-bank/_prompt.md` rules
    - This ensures that if the loop is interrupted, progress is not lost

10) **Advance to the next step** and repeat from step 6.

**If `mode=full-queue`**: After each wave (or after each step in sequential modes), run a quick verify check before advancing:
```
/axiom-verify --work-item $WORK_ITEM_ID --mode quick
```
If the quick verify fails, stop the loop and report. Do not advance to the next wave/step. This prevents compounding failures across phases.

### Phase 3: Loop Completion

10) When all steps in the queue are done (or `max_steps` reached):
    - Update `.memory-bank/TODO.md` with all completed steps.
    - Update the plan cursor to the next incomplete step (or mark the phase complete).
    - Create a summary run bundle with all step evidence.
    - Post final Jira comment if applicable.
    - **If `mode=full-queue`**: Automatically run `/axiom-verify --work-item $WORK_ITEM_ID --mode full` to produce the final milestone gate evidence. Include the verify result in the consolidated summary.

### Subagent Dispatch Rules

- **Strategy determines parallelism.** `sequential-single` = one subagent for all steps. `sequential-isolated` = one subagent per step, serially. `parallel` = one subagent per step, all at once.
- **Use `dev-axiom`** as the default subagent type for implementation steps.
- **Use `qa-axiom`** for steps that are primarily about testing/verification.
- **Use `docs-runbooks-axiom`** for steps that are primarily about documentation.
- **Use `db-architect-axiom`** for steps involving database schema/migration work.
- The subagent prompt must be self-contained — include all context the subagent needs to execute without reading the full plan.
- **Do not re-dispatch a failed step** unless you have a corrective action to include in the prompt.
- **For parallel mode**: verify steps are independent before dispatching. Check for shared file paths, ordering dependencies, and data dependencies.

### Stop Conditions

- Any step fails its quality gate → stop, report, inject corrective steps.
- Any subagent reports blocked → stop, report what's blocking.
- `max_steps` reached → stop, report progress.
- All steps complete → stop, report success.
- **`mode=full-queue` only**: Inter-wave quick verify fails → stop, report which step caused the regression, do not advance.

### When Stuck — Search Before Stopping

If a step fails 2+ times with the same error, or a subagent reports blocked on an unfamiliar problem, **load `research-and-recon-axiom` and search before stopping the loop**. Common situations where searching resolves the block:

- Error message from a library or runtime that isn't in the codebase
- API shape or behaviour that may have changed since training
- Compatibility issue between two dependencies
- A spec requirement that references an external standard or RFC

Get the current date first (`date -u +"%Y-%m-%d"`), then search with a year-anchored query. Record what you found in the step evidence. Only stop the loop if searching doesn't resolve the block.

Output:
- Emit the required XML envelope per `.opencode/skills/axiom-xml-protocol/SKILL.md`.
- Required tags:
  - `<command>/axiom-step-loop</command>`
  - `<status>ok|fail|blocked</status>`
  - `<summary>` — "Completed N/M steps for $WORK_ITEM_ID" or "Stopped at step X: <reason>"
  - `<evidence>`:
    - `<steps_completed>` — count
    - `<steps_remaining>` — count
    - `<steps_failed>` — count
    - `<last_step>` — the last step id attempted
    - `<runtime_tier_reached>` — highest tier from any step
    - `<files_changed>` — semicolon-separated aggregate
  - `<diagnostics>` for warnings/errors
  - `<next_steps>` — what to do next (either the next step id, or corrective actions if stopped on failure)

## Output Contract (what to return to the caller)

### For Human Consumption
- Summary: one sentence stating how many steps completed and whether the loop stopped cleanly.
- Confidence: 0-100

### For Agent Consumption (MUST include)
- `evidence.files_changed`: aggregate of ALL files created/modified across all steps (full paths, semicolon-separated)
- `evidence.run_paths`: list of run bundle paths created (one per step)
- `evidence.steps_completed`: count of steps that passed
- `evidence.steps_remaining`: count of steps still pending
- `evidence.steps_injected_from_backlog`: count of steps auto-injected from findings-backlog.md
- `evidence.last_step`: the last step ID attempted
- `evidence.runtime_tier_reached`: highest runtime tier achieved across all steps
- `related_commands`: suggested follow-up commands
  - "To verify the completed work, run: `/axiom-verify --work-item <id>`"
  - "To continue from where the loop stopped, run: `/axiom-step-loop --work-item <id> from=<last-step>`"
  - "To run the entire queue autonomously, run: `/axiom-step-loop --work-item <id> mode=full-queue`"
  - "To run an adversarial review, run: `/axiom-adversary --target <id>`"
  - "To skip backlog injection, run: `/axiom-step-loop --work-item <id> auto_inject=false`"

### Cross-References
- "Run evidence is at: `.memory-bank/work-items/<id>/runs/`"
- "Plan cursor is in: `.memory-bank/work-items/<id>/plan.yaml`"
- "Spec: `specs/48-Test-Quality-Gates.md`"

See: `specs/48-Test-Quality-Gates.md`, `specs/09-Baby-Steps-Methodology.md`, `.opencode/skills/runtime-completeness-gate-axiom/SKILL.md`

axiom:trace spec=specs/48-Test-Quality-Gates.md work_item=command-quality-01
