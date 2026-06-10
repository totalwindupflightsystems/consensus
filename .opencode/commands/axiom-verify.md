---
description: "4-phase verification: spec alignment, runtime completeness, adversarial review, and loop-closing plan updates."
agent: dispatch-axiom
---

You are orchestrating a multi-phase verification of Axiom work. You coordinate specialized subagents — you do NOT do all the work yourself. Delegate Phase 1 spec alignment to @spec-verifier-axiom, Phase 3 adversarial review to the three adversarial agents, and handle orchestration, bash execution, and file writes yourself.

Inputs:
- Work item id: $WORK_ITEM_ID
- Repo: $REPO
- Mode: $MODE (optional; "quick" = Phase 1 only, "full" = all 4 phases, "kiss" = full + KISS filter (only inject findings that block an AC), "structural" = Phase 1 + code-graph blast-radius analysis, "analysis" = Phase 1 + axiom-analyze health score, "profile" = Phase 1 + runtime profiler hotspot check, "deep" = all 4 phases + structural + analysis. Default: "quick" for routine step verification. Use "full" for milestone gates, pre-release, or when adversarial review is needed. Use "kiss" when you want adversarial review but don't want perfection-chasing findings injected — only findings that block an AC. Use "structural" when verifying changes to shared code. Use "analysis" for quality regression checks. Use "profile" for performance-sensitive changes.)
- Strategy: $STRATEGY (optional; controls how verification phases are dispatched. Default: "parallel")
- Arguments: $ARGUMENTS (optional: `auto_inject=true|false` to control whether findings are auto-injected into the plan, default: true. `inject_cap=N` to set a maximum number of injected steps per run, default: **unlimited** — all findings are injected in one pass. `--kiss` shorthand for `--mode kiss`)

### Verification Strategies

| Strategy | How it works | When to use |
|---|---|---|
| `sequential` | Phases run one at a time. Phase 2 waits for Phase 1. Phase 3 waits for Phase 2. Each phase's findings inform the next. | When you want findings to cascade (e.g., Phase 1 spec gaps should inform Phase 3 adversarial review). More thorough but slower. |
| `parallel` (default) | All applicable phases dispatch simultaneously. Phase 1 (spec), Phase 2 (runtime), Phase 3 (adversarial), Phase 4 (updates) run at the same time. Results merged at the end. | Most work. Faster. Each phase is independent anyway — spec alignment doesn't need runtime completeness results to run. |
| `batch` | Group phases into dependency waves. Phase 1+2 run in parallel (both are evidence-gathering). Phase 3 waits for 1+2 (adversarial agents need the evidence). Phase 4 waits for 3 (plan updates need findings). | Best of both — parallel where safe, sequential where findings cascade. Smart default for full mode. |
| `single-context` | All phases run in ONE subagent context window. The verifier agent does everything itself without dispatching sub-subagents. | Small work items, quick checks, when subagent dispatch overhead isn't worth it. |

**How `batch` works for verify:**

```
Wave 1 (evidence gathering — parallel):
  ├── Phase 1: Spec alignment (@spec-verifier-axiom)
  └── Phase 2: Runtime completeness gate

Wave 2 (adversarial review — parallel, uses Wave 1 evidence):
  ├── @assumption-buster-axiom (receives Phase 1+2 findings)
  ├── @devils-advocate-axiom (receives Phase 1+2 findings)
  ├── @ralph-wiggum-verify (receives Phase 1+2 findings)
  ├── @qa-axiom (receives Phase 1+2 findings)
  └── @frontend-dev (conditional, receives Phase 1+2 findings)

Wave 3 (plan updates — sequential, uses Wave 1+2 findings):
  └── Phase 4: Write findings, inject steps, update TODO
```

**How to choose**: The agent SHOULD auto-select:
- `$MODE=quick` → `single-context` (Phase 1 only, no need for subagents)
- `$MODE=full` → `batch` (parallel evidence gathering, then adversarial with evidence, then updates)
- User can override with `$STRATEGY`.

Skills (load on demand):
- `kiss-axiom` — Load when $MODE is "kiss". Scores the injected findings against the work item's acceptance criteria and cuts any finding that doesn't block an AC. Prevents perfection-chasing from inflating the plan.
- `research-and-recon-axiom` — **Load when a verify finding references an external standard, library, or API, or when the root cause of a failure isn't clear from the code alone.** Covers how to search for current documentation, error messages, and best practices. Always get the current date before searching. Don't assume — look it up. Available tools: `searxng_searxng_web_search`, `searxng_web_url_read`, `webfetch`.
- `runtime-completeness-gate-axiom` — Load when $MODE is "full" AND the work item touches CLI, API, UI, worker, or multi-path data surfaces. Skip for spec-only, doc-only, or config-only work items. Use the skill's own load-trigger table (lines 34-47) to decide which patterns to apply.
- `expected-output-axiom` — Load when verifying step outputs. Defines how to declare expected output BEFORE execution and compare actual vs expected. Critical for distinguishing "it ran" from "it produced correct results."
- `test-inspector-axiom` — Load during Phase 1 to grade the test suite: classify each test as KEEPER/WEAK/THEATER/DEAD WEIGHT, assign value scores, and surface coverage holes where code changed but no test exists. Always useful when the work item touches test files.
- `test-quality-gates-axiom` — Deep guidance on test quality (no assertionless tests, Tier-3+ required).
- `conformance-testing-loop` — For spec-vs-behavior conformance checking.
- `evidence-bundle-schema` — For understanding the evidence format and what's required.
- `enterprise-testing-standard` — For tiered verification depth (6-tier hierarchy).
- `axiom-confidence-scoring` — For scoring confidence based on evidence signals.
- `code-graph-intelligence-axiom` — Load when $MODE includes "structural" or "blast-radius". Provides call graph, callers/callees lookup, blast-radius analysis, and change-impact queries. Use to verify that a change doesn't break unexpected callers or cross-language dependencies.
- `code-analysis-axiom` — Load when $MODE includes "analysis" or "quality-score". Runs multi-language static analysis (`axiom analyze`) and produces a health score. Use to verify that code quality hasn't regressed after implementation.
- `runtime-profiler-axiom` — Load when $MODE includes "profile" or "performance". Attaches to running processes to find hotspots and produce flamegraph data. Use to verify performance requirements or detect regressions introduced by the work item. Requires the `axiom-profiler` binary built from `profiler/`.

**If $MODE is "quick" (default)**: Execute only Phase 1 (steps 1-4). Skip Phases 2-4. This is appropriate for routine step verification on small, low-risk changes.

**If $MODE is "full"**: Execute all 4 phases. This is appropriate for milestone gates, pre-release verification, and any work item where adversarial review adds value. Use this when: completing a work item, cutting a release, or when you suspect wiring gaps.

**If $MODE is "structural"**: Execute Phase 1 + load `code-graph-intelligence-axiom` skill. After Phase 1 spec alignment, run blast-radius analysis: `axiom-code-intel changes --base HEAD~1` (or equivalent) to identify all callers/callees affected by changed files. Write results to `evidence.blast_radius_report`. If unexpected callers found outside the expected scope, inject a HIGH finding. Set `evidence.structural_scope_violations` count. Then continue to Phase 2+ as normal.

**If $MODE is "analysis"**: Execute Phase 1 + load `code-analysis-axiom` skill. After Phase 1, run `axiom analyze --score --path <affected-paths>`. Compare score to baseline in `verification.md`. If score decreased by >5 points, inject a HIGH finding. Write score to `evidence.quality_score`. Then continue to Phase 2+ as normal.

**If $MODE is "profile"**: Execute Phase 1 + load `runtime-profiler-axiom` skill. **Prerequisite**: `profiler/target/release/axiom-profiler` must be built (`cd profiler && cargo build --release`). Requires sudo on macOS. If binary absent or sudo unavailable, record "profiler: not available" and skip this enrichment — do NOT fail the verify. When available, attach profiler during the Tier-3 runtime test, capture hotspot data, write to `evidence.profile_report`. Then continue to Phase 2+ as normal.

**If $MODE is "deep"**: Execute all 4 phases + all three enrichments (structural + analysis + profile). The most thorough verification mode. Use for critical changes, major refactors, or pre-release gates on performance-sensitive paths.

**If $MODE is "kiss"**: Execute all 4 phases (same as "full") with one critical difference: **load the `kiss-axiom` skill and dispatch `@kiss-axiom` as an additional Phase 3 agent.** The KISS agent scores every finding from the adversarial triad against the work item's acceptance criteria. Only findings that directly block an AC are injected into the plan. Findings that make the implementation "more perfect" but don't block any AC are dropped (or moved to findings-backlog.md with a `[WONT-FIX]` label). Use this when:
- The adversarial review keeps injecting polish steps that aren't required
- You want a full review but only want to act on what's actually broken
- The work item is "good enough" and you don't want scope creep from the review loop

**KISS mode Phase 3 addition**: After the adversarial triad and QA agent return findings, dispatch `@kiss-axiom` with:
- The work item's acceptance criteria (from plan.yaml)
- All findings from the adversarial agents
- Instruction: "Score each finding against the ACs. Return only findings that block an AC as injectable. Mark all others as [WONT-FIX] with rationale."

**KISS mode Phase 4 rule**: Only inject findings that `@kiss-axiom` marked as AC-blocking. All other findings go to findings-backlog.md with `[WONT-FIX]` label and the KISS rationale. The `status=fail` rule still applies for CRITICAL findings — KISS does not suppress genuine blockers, only perfection-chasing.

Do:

## Phase 1 — Spec Alignment and Evidence Check (delegate to @spec-verifier-axiom)

1) **Delegate to @spec-verifier-axiom** as a subagent: ask it to read `specs/` and `.memory-bank/work-items/$WORK_ITEM_ID/`, verify alignment to specs, and check that required evidence exists. Collect its findings.

2) **Run the test quality gate** (fail-closed):
   ```bash
   python3 .axiom/scripts/check_test_quality.py \
     --test-dir .axiom/tests \
     --config .axiom/axiom.config.yaml \
     --json > test-quality-report.json
   ```
   - If the script does not exist, record "test quality gate: script missing" as a HIGH finding and continue (do not hard-fail the entire verify on a missing script).
   - Exit code 1 (hard fail): set status=fail, inject steps to fix each violation.
   - Exit code 2 (soft fail): record in evidence, flag for follow-up.
   - Exit code 0: record score in evidence.

3) **Verify runtime tier evidence**:
   - Check that `evidence.runtime_tier_reached` exists in the step evidence.
   - Check that it meets or exceeds `verification.runtime.required_min_tier` from config (default: 3).
   - If absent or below threshold: set status=fail, inject step to run the Tier-3 command.

4) If gaps exist in spec alignment or evidence, propose injected steps/tasks.

**If $MODE is "quick"**: Stop here. Emit output and exit.

## Phase 2 — Runtime Completeness Gate (conditional)

5) **Determine if runtime completeness check applies** using the skill's load-trigger table:
   - Work item touches CLI, API, UI, worker, or multi-path data → load `runtime-completeness-gate-axiom` and apply.
   - Work item is spec-only, doc-only, or config-only → skip this phase, note "runtime completeness: not applicable (non-runtime work item)".

6) If applicable, **load `runtime-completeness-gate-axiom` skill** and apply:
   - Run the 5-pattern check (nil executor, unregistered route, stubbed adapter, demo-only path, mock-data trap).
   - For UI/frontend work: also run patterns 10-17 (navigation, API-to-UI, action feedback, environment, verification theater).
   - For multi-path data work: run the Data Path Matrix (Step 2b) and wiring gap patterns 6-9.
   - Record findings as structured evidence.
   - If any runtime completeness gap is found: set status=fail, inject specific fix steps.

## Phase 3 — Adversarial and Specialist Subagent Review (fan-out)

7) **Dispatch subagents for review**. Each receives the work item context, the evidence from phases 1-2, and the current plan/TODO state:

   **Always dispatch (adversarial triad):**

   a) **@assumption-buster-axiom** — Surface undocumented prerequisites, ambiguous specs, and non-verifiable acceptance criteria in the work item. Look at the skills in `.opencode/skills/` for process gaps. Return findings with severity and recommended fixes.

   b) **@devils-advocate-axiom** — Challenge the implementation: is this the simplest thing that could work? Where will it break first? Are there simpler alternatives? What tradeoffs were made implicitly? Return a challenge pack with pressure tests and risks.

   c) **@ralph-wiggum-verify** — Audit the builder's output against the governing spec and TODO/plan. Check for drift between what the plan says should exist and what actually exists in the repo. Return PASS/FAIL/BLOCKED with steering decision (continue|steer|stop) and corrective actions.

   **Always dispatch (quality evaluation):**

   d) **@qa-axiom** — Evaluate the work item's test coverage, test quality, and verification completeness. Check that acceptance criteria have mapped verification paths. Check that tests are high-signal (not assertionless, not tautological, not mock-only). Verify that runtime tier evidence exists at the required level. Return a QA evaluation with coverage gaps, test quality issues, and recommended test additions.

   **Conditionally dispatch (frontend work):**

   e) **@frontend-dev** (`.opencode/agents/frontend-dev.md`) — Dispatch ONLY when the work item touches UI/frontend surfaces (React components, CSS, HTML templates, browser-rendered pages, client-side JavaScript/TypeScript). This agent has chrome-devtools MCP access and will:
      - Navigate to affected pages/routes in a real browser
      - Capture screenshots for key states (loaded, empty, error, loading)
      - Inspect DOM for accessibility (ARIA labels, focus order, semantic HTML)
      - Check console for errors/warnings
      - Check network requests for correct API calls and response handling
      - Verify responsive behavior if applicable
      - Return browser-verified evidence with screenshots and network traces

8) **Merge all subagent findings**:
   - Collect all findings from adversarial agents (a-c), QA evaluation (d), and frontend verification (e, when dispatched).
   - Deduplicate overlapping findings (same root cause → keep the most specific one).
   - Classify by severity: CRITICAL (blocks PASS), HIGH (should fix before next milestone), MEDIUM (track), LOW (note).
   - Any CRITICAL finding from any subagent → set status=fail.
   - QA coverage gaps where acceptance criteria have no verification path → CRITICAL.
   - Frontend browser evidence showing broken UI, console errors, or failed network calls → CRITICAL.

## Phase 4 — Auto-Inject Findings Into Plan (close the loop)

9) **Auto-inject findings into plan** (when `auto_inject=true`, the default):
   - Classify all findings from Phases 1-3 by severity: CRITICAL, HIGH, MEDIUM, LOW.
   - **Skip** findings labeled `[IMPOSSIBLE]`, `[DEFERRED-INDEFINITELY]`, or `[WONT-FIX]`.
   - **Dedup** against existing plan steps (same objective text → skip injection).
   - **Inject ALL remaining findings** — there is no default cap. Every finding that passes the above filters gets injected in a single pass so you don't need multiple verify runs to get everything in.
   - If `inject_cap=N` is explicitly set in `$ARGUMENTS`, cap at N injections and write overflow to `findings-backlog.md`. Use this only when you intentionally want to throttle (e.g., a very large review where you want to tackle findings in batches).
   - For each injectable finding:
     a) Convert the finding into an executable plan step:
        - `id`: auto-generated (e.g., `fix-<finding-slug>`)
        - `objective`: derived from finding title
        - `actions`: the recommended fix
        - `verification`: acceptance criteria that proves the fix works
        - `source`: `"verify-auto-inject"`
        - `severity`: from the finding
     b) Add to `.memory-bank/work-items/$WORK_ITEM_ID/plan.yaml` (or `plan.md`) as a new step.
     c) Add to `.memory-bank/TODO.md` as an unchecked item under the active phase.
     d) **Priority insertion**: CRITICAL steps go BEFORE existing incomplete steps. HIGH steps go AFTER current step but BEFORE MEDIUM/LOW work.
   - **Backlog format** (only used when `inject_cap=N` is set and overflow occurs): Each finding in `findings-backlog.md` MUST include:
     - Title (one line)
     - Severity: CRITICAL | HIGH | MEDIUM | LOW
     - Labels: (optional) `[IMPOSSIBLE]`, `[DEFERRED-INDEFINITELY]`, `[WONT-FIX]` — these prevent auto-injection
     - Description: what's wrong
     - Recommended fix: what to do
     - Source: which phase/agent produced it

10) **Update work-item artifacts**:
    - Write findings to `.memory-bank/work-items/$WORK_ITEM_ID/verification.md` (append, don't overwrite previous runs). Include timestamp and run context.
    - Update `.memory-bank/work-items/_current.md` if the active step needs to change based on findings.
    - Record the verification run timestamp and result in the work item's evidence trail.
    - Log: "Auto-injected N findings into plan (M overflow to backlog, K skipped as impossible/deferred)"

11) **Emit structured summary** for the user:
    - What passed (spec alignment, test quality, runtime completeness, QA evaluation, adversarial review, frontend verification if applicable).
    - What failed (with specific fix steps that the next `/axiom-step` or `/axiom-step-loop` will AUTOMATICALLY execute).
    - **Key message**: "N findings auto-injected into plan. Run `/axiom-step-loop --work-item <id>` to execute fixes automatically."
    - QA coverage summary: how many acceptance criteria have verified paths vs gaps.
    - Frontend evidence summary (when applicable): screenshots captured, console/network status.
    - Confidence score based on evidence signals.
    - "The next `/axiom-step-loop` run will automatically execute the injected fix steps."

Fail-closed rules:
- MUST return `status=fail` if test quality gate hard-fails (exit code 1).
- MUST return `status=fail` if `evidence.runtime_tier_reached` is absent or below threshold.
- MUST return `status=fail` if runtime completeness gate finds any gap (when applicable).
- MUST return `status=fail` if any adversarial subagent returns a CRITICAL finding.
- MUST return `status=fail` if @qa-axiom finds acceptance criteria with no verification path.
- MUST return `status=fail` if @frontend-dev finds broken UI, severe console errors, or failed API calls (when dispatched for frontend work).
- MUST NOT return `status=ok` if any required evidence tag is missing.
- MUST auto-inject findings into plan when `auto_inject=true` (Phase 4) — this is what enables the automated fix loop.
- MUST inject ALL findings in a single pass by default (no cap). Only cap when `inject_cap=N` is explicitly set.
- MUST NOT inject findings labeled `[IMPOSSIBLE]`, `[DEFERRED-INDEFINITELY]`, or `[WONT-FIX]`.
- MUST deduplicate injected steps to prevent repeated-run bloat.
- When `inject_cap=N` is set: MUST cap at N injections; overflow goes to findings-backlog.md.
- MUST preserve backlog items labeled [IMPOSSIBLE] in findings-backlog.md indefinitely (never remove them).

Output:
- Emit verifier XML tags per `.opencode/skills/axiom-xml-protocol/SKILL.md` and required tags in `.axiom/command-registry.yaml`.
- Required tags include `evidence.runtime_tier_reached` and `evidence.files_changed`.
- Include `evidence.tests_quality.*` tags with values from the quality gate run.
- Include QA evaluation summary (coverage gaps, test quality issues) in `detailed_summary`.
- Include adversarial review summary in `detailed_summary`.
- Include frontend browser evidence (screenshots, console/network status) in `detailed_summary` when @frontend-dev was dispatched.

## Output Contract (what to return to the caller)

### For Human Consumption
- Summary: one sentence stating which phases ran, what passed, and what failed.
- Confidence: 0-100

### For Agent Consumption (MUST include)
- `evidence.files_changed`: list of ALL files created/modified (full paths, semicolon-separated)
  - Typically: `.memory-bank/work-items/$WORK_ITEM_ID/verification.md`, `.memory-bank/TODO.md` (if injected steps added), `plan.yaml` (if steps injected)
- `evidence.verification_path`: full path to the verification.md file for this run
- `evidence.runtime_tier_reached`: highest runtime tier achieved (0-5)
- `evidence.tests_quality.score`: test quality score (0-100)
- `evidence.injected_steps_count`: number of findings auto-injected into plan
- `evidence.injected_steps_paths`: list of paths to injected step artifacts (for downstream agents to read)
- `evidence.backlog_overflow_count`: number of findings that went to findings-backlog.md (only non-zero when `inject_cap=N` is explicitly set)
- `evidence.skipped_impossible_count`: number of findings skipped due to [IMPOSSIBLE]/[WONT-FIX] labels
- `evidence.adversarial_findings_paths`: list of finding file paths from adversarial agents (if Phase 3 ran)
- `related_commands`: suggested follow-up commands
  - "To execute injected fix steps automatically, run: `/axiom-step-loop --work-item <id>`" ← PRIMARY recommendation
  - "To execute one fix step, run: `/axiom-step --work-item <id>`"
  - "To run a full adversarial review, run: `/axiom-adversary --target <id>`"
  - "To view the verification report, read: `.memory-bank/work-items/$WORK_ITEM_ID/verification.md`"
  - "To disable auto-injection, run: `/axiom-verify --work-item <id> auto_inject=false`"

### Cross-References
- "Verification report is at: `.memory-bank/work-items/$WORK_ITEM_ID/verification.md`"
- "Test quality gate script: `.axiom/scripts/check_test_quality.py`"
- "Runtime completeness gate: `.opencode/skills/runtime-completeness-gate-axiom/SKILL.md`"

See: `specs/48-Test-Quality-Gates.md`, `.opencode/skills/test-quality-gates-axiom/SKILL.md`, `.opencode/skills/runtime-completeness-gate-axiom/SKILL.md`

axiom:trace spec=specs/48-Test-Quality-Gates.md work_item=command-quality-01
