---
description: Resolve raw user intent into a work item with plan, TODO entries, and memory bank artifacts.
agent: dispatch-axiom
---

You are resolving a user's raw intent into an actionable work item for Axiom. Your job is NOT just to produce a plan — you must **write the work item artifacts** so the step-loop can immediately execute.

axiom:trace work_item=collaborative-intent-resolution-01 spec=specs/59-Collaborative-Intent-Resolution.md plan=P-79-collaborative-intent-resolution

## Inputs

- Intent: $INTENT
- Repo: $REPO
- Work item id: $WORK_ITEM_ID (optional — if not provided, generate one from the intent)
- `--kiss` flag (optional): enable KISS mode — do full planning and meta-planning to understand the request, but only generate steps that map to an acceptance criterion. Prevents over-engineering and scope creep. Recommended for automated/runner contexts.

Optional inputs:
- Context: $CONTEXT (enriched repo context — languages, commands, specs, memory bank summary)
- Answers: $ANSWERS (answers to previously asked questions, if this is a follow-up iteration)
- Flags:
  - `dry_run=true` — output XML plan only, do NOT write files to disk. Useful when another system consumes the XML output programmatically.
  - Default (no flag): write work item artifacts to disk AND output XML.

Skills (load on demand):
- `kiss-axiom` — Load when `--kiss` flag is set. Score every generated step against the ACs. Cut steps that don't map to an AC. Does NOT skip planning or meta-planning.
- `axiom-capability-surface` — Full catalog of Axiom capabilities (commands, agents, skills, spec categories). Load this skill when the intent is complex, vague, or you need to understand what Axiom can do to route the work correctly. See `specs/59-Collaborative-Intent-Resolution.md#REQ-CIR-009`.
- `axiom-meta-planning-contract` — **Load this before writing any artifacts.** Defines the required sections for `meta-planning.md`, the pre-execution gate (all three files must exist), and light vs standard meta-plan rules.
- `axiom-plan-schema` — **Load this before writing `plan.yaml`.** Defines the machine-readable schema that the step-loop executes. The step-loop reads `plan.yaml`, not `plan.md`.
- `middle-out-planning-axiom` — Load when the intent crosses system boundaries. Start at the critical integration point, prove it works, then expand outward.
- `working-backwards-axiom` — Load when the intent has a user-visible surface. Plan from the end-user experience backward to implementation. Every step must include how to verify it's connected end-to-end.
- `baby-steps-methodology` — Each step must be independently verifiable and do exactly one thing.
- `traceability-doctrine` — Every step needs a `axiom:trace` marker linking to a spec and evidence path.

## What This Command Does

1. **Reads the repo** — understands project structure, existing code, specs, memory bank.
2. **Resolves the intent** — turns the raw request into a structured plan with phases/tasks/steps.
3. **Creates the work item** — writes actual files to disk so the step-loop can execute immediately.

## Decision Tree

Follow this decision tree strictly:

1. **Read the repo** — understand project structure, existing code, specs, memory bank.
   - Check for `specs/README.md`, `.memory-bank/_index.md`, key config files.
   - Identify primary languages, frameworks, and existing patterns.
   - If the intent is complex, vague, or you need to understand what Axiom can do, load the `axiom-capability-surface` skill for a full catalog of commands, agents, skills, and spec categories.

2. **Assess the intent** — is it clear enough to act on?

   - **If YES** (clear enough to plan): produce a plan and write the work item artifacts.
   - **If NO** (ambiguous, missing info): make your best judgment and produce a plan. If truly impossible, return `<blocked_reason>`.
   - **If IMPOSSIBLE** (contradictory, infeasible, missing files referenced): return `<blocked_reason>`.

3. **Generate a work item ID** — if `$WORK_ITEM_ID` is not provided:
   - Derive from the intent: lowercase, hyphenated, max 40 chars
   - Example: "add dark mode toggle" → `dark-mode-toggle-01`
   - Check `.memory-bank/work-items/` to avoid collisions

4. **Write the work item artifacts** (this is the key step — you MUST do this):

   The pre-execution gate in `axiom-step-loop` requires **all three files** to exist before it will execute any steps. Missing any one of them blocks execution.

   ```
   .memory-bank/work-items/<work_item_id>/
   ├── meta-planning.md   ← REQUIRED by pre-execution gate
   ├── plan.md            ← REQUIRED by pre-execution gate (human-readable)
   └── plan.yaml          ← REQUIRED by pre-execution gate (machine-readable — what the step-loop actually executes)
   ```

   **Read `.memory-bank/work-items/_prompt.md` before writing** — it defines the authoritative references and fail-closed writing rules for all work-item artifacts.

   Also append to `.memory-bank/TODO.md`:
   ```markdown
   ## <work_item_id> — <title>

   - [ ] `<work_item_id>` `phase-N/task-N-M/step-N-M-K` — <step objective>. Spec: `<spec ref>`
   - [ ] `<work_item_id>` `phase-N/task-N-M/step-N-M-K` — <step objective>. Spec: `<spec ref>`
   ...
   ```

5. **Include context** — reference specific files, specs, or patterns found in the repo.

## Work Item Artifacts Format

### meta-planning.md

Load `axiom-meta-planning-contract` skill. Write all required sections:
- Intent (what changes and why, in-scope / out-of-scope)
- Contract Reconciliation (Jira ref, AC summary, specs touched)
- Decision Points (product/policy decisions, defaults chosen)
- Risks and Blast Radius
- Verification Design (evidence that proves done)
- Ambiguity Assessment (assumptions, rating, missing inputs)

### plan.md

Human-readable plan for context and review. Load `working-backwards-axiom` and `middle-out-planning-axiom` as appropriate.

```markdown
---
mb:
  type: plan
  title: "<Title derived from intent>"
  created: <today's date>
  work_item: <work_item_id>
  jira_ref: <jira key if known, else null>
---

# <Title> — Implementation Plan

## Context
<Brief description of what this work item accomplishes and why>

## Phase 1 — <Phase Title>

### Task 1.1 — <Task Title>

#### Step 1.1.1
- **Objective**: <what this step accomplishes>
- **Actions**: <concrete actions to take>
- **Verification**: <how to verify the step succeeded>
- **Files**: <files expected to be created/modified>

axiom:trace work_item=<work_item_id> spec=<governing spec> plan=phase-1/task-1-1/step-1-1-1
```

### plan.yaml — THE CRITICAL ARTIFACT

**This is what the step-loop actually executes.** Load `axiom-plan-schema` skill. The step-loop reads `plan.yaml`, not `plan.md`. Every step in `plan.md` must have a corresponding entry in `plan.yaml`.

```yaml
version: 1
work_item:
  id: "<work_item_id>"
  jira_key: null  # or Jira key if applicable
meta_planning:
  notes_md_path: ".memory-bank/work-items/<id>/meta-planning.md"
plan:
  phases:
    - id: "phase-1"
      title: "<Phase Title>"
      tasks:
        - id: "task-1-1"
          title: "<Task Title>"
          steps:
            - id: "step-1-1-1"
              title: "<Step Title>"
              command: "/axiom-step"
              spec_ref: "specs/<governing-spec>.md#anchor"
              inputs:
                objective: "<what this step accomplishes>"
                actions: "<concrete actions>"
              verification:
                - type: "command"
                  command: "<verification command>"
                  required: true
                  timeout_seconds: 300
execution:
  status: "pending"
  cursor:
    phase_id: "phase-1"
    task_id: "task-1-1"
    step_id: "step-1-1-1"
```

### Step Queue Design — Maximise Throughput

**Do NOT artificially limit the number of steps.** Write every step needed to complete the work. The step-loop with `mode=full-queue` will execute them all in one pass.

**Design for parallelism:**
- Steps within a task that don't share files can run in parallel (`batch` strategy)
- Group independent work into the same phase/task so it can be parallelised
- Only add a phase boundary when later steps genuinely depend on earlier steps completing
- Identify the critical path (the minimum sequential chain) and make it as short as possible
- Prefer `batch` strategy for plans with mixed dependencies — it maximises parallelism while respecting ordering

**Step language rules:**
- Each step title must be an imperative verb phrase: "Add X", "Fix Y", "Implement Z"
- `objective` must be a single sentence stating what the step accomplishes
- `actions` must be concrete and executable — not "improve the code" but "add `validate_input()` to `api/routes.rs` line 45"
- `verification` must be a runnable command or a specific observable outcome — not "check it works" but "`cargo test test_validate_input` passes"
- Every step must have a `spec_ref` linking to the governing spec section

### TODO.md entries

Each step becomes a checkbox in `.memory-bank/TODO.md`:
```markdown
- [ ] `<work_item_id>` `phase-N/task-N-M/step-N-M-K` — <objective>. Spec: `<spec ref>`
```

Before writing to TODO.md:
- Check for duplicate entries (same work_item_id + step) to prevent repeated-run bloat
- Append at the end of the file (or after the last entry for this work item if it already exists)

## Rules

- **MUST write all three files** — `meta-planning.md`, `plan.md`, and `plan.yaml`. Missing any one blocks step execution.
- **MUST create the work item directory** — `mkdir -p .memory-bank/work-items/<id>/`
- **MUST write plan.yaml** — the machine-readable plan the step-loop executes. `plan.md` alone is not enough.
- **MUST write meta-planning.md** — required by the pre-execution gate.
- **MUST append to TODO.md** — unchecked items for each step
- **MUST check for duplicates** — don't re-add steps that already exist in TODO.md
- **MUST NOT cap the number of steps** — write every step needed. The step-loop with `mode=full-queue` handles unlimited steps in one pass.
- Plans MUST follow the phase/task/step structure compatible with `specs/03-Plan-Schema.md` and `axiom-plan-schema` skill.
- Each step MUST have `title`, `command`, `spec_ref`, and `verification`.
- Step language MUST be imperative and concrete — not vague descriptions but executable instructions.
- Do NOT produce a plan with 0 steps — if you cannot produce meaningful steps, return `<blocked_reason>`.
- Reference specific files and patterns found in the repo to ground the plan in reality.
- The `<status>` tag MUST be `ok` (work item created) or `blocked` (cannot resolve).

## Output Format

Emit a `<axiom>` XML envelope per `specs/04-XML-Protocol.md` with the required tags from `.axiom/command-registry.yaml`.

The envelope MUST contain an `<intent_resolution>` compound tag with exactly ONE of:
- `<plan>` — a summary of the plan that was written to disk
- `<blocked_reason>` — why the intent cannot be resolved

### Success Response Example

```xml
<axiom>
  <run>
    <run_id>$RUN_ID</run_id>
    <work_item_id>$WORK_ITEM_ID</work_item_id>
    <repo>$REPO</repo>
  </run>
  <command>/axiom-intake</command>
  <status>ok</status>
  <confidence>85</confidence>
  <summary>Created work item 'dark-mode-toggle-01' with 2 phases, 5 steps. Ready for /axiom-step-loop.</summary>
  <detailed_summary>
    Analyzed repo structure and created work item artifacts:
    - .memory-bank/work-items/dark-mode-toggle-01/meta-planning.md
    - .memory-bank/work-items/dark-mode-toggle-01/plan.md (2 phases, 3 tasks, 5 steps)
    - .memory-bank/work-items/dark-mode-toggle-01/plan.yaml (machine-readable, step-loop ready)
    - .memory-bank/TODO.md updated with 5 unchecked items
    Run /axiom-step-loop --work-item dark-mode-toggle-01 mode=full-queue to execute all steps autonomously.
  </detailed_summary>
  <related_specs>specs/59-Collaborative-Intent-Resolution.md</related_specs>
  <memory_updates>.memory-bank/work-items/dark-mode-toggle-01/plan.md;.memory-bank/TODO.md</memory_updates>
  <modify_plan>false</modify_plan>
  <retry>false</retry>
  <intent_resolution>
    <plan>
      <work_item_id>dark-mode-toggle-01</work_item_id>
      <title>Add Dark Mode Toggle</title>
      <phases_count>2</phases_count>
      <steps_count>5</steps_count>
      <plan_path>.memory-bank/work-items/dark-mode-toggle-01/plan.md</plan_path>
    </plan>
  </intent_resolution>
</axiom>
```

### Blocked Response Example

```xml
<axiom>
  <run>
    <run_id>$RUN_ID</run_id>
    <work_item_id>$WORK_ITEM_ID</work_item_id>
    <repo>$REPO</repo>
  </run>
  <command>/axiom-intake</command>
  <status>blocked</status>
  <confidence>20</confidence>
  <summary>Cannot resolve intent — reason</summary>
  <detailed_summary>Detailed explanation of what was attempted and why it failed.</detailed_summary>
  <related_specs>specs/59-Collaborative-Intent-Resolution.md</related_specs>
  <memory_updates></memory_updates>
  <modify_plan>false</modify_plan>
  <retry>false</retry>
  <intent_resolution>
    <blocked_reason>
      <reason>Clear explanation of why the intent cannot be resolved.</reason>
      <attempted>What was tried before concluding it is blocked.</attempted>
      <suggestions>
        <suggestion>Concrete action the user can take to unblock.</suggestion>
      </suggestions>
    </blocked_reason>
  </intent_resolution>
</axiom>
```

## Output Contract (what to return to the caller)

### For Human Consumption
- Summary: one sentence stating the work item was created and how to execute it.
- Confidence: 0-100

### For Agent Consumption (MUST include)
- `evidence.files_changed`: list of ALL files created/modified (full paths, semicolon-separated)
  - Always includes: `.memory-bank/work-items/<id>/meta-planning.md`, `.memory-bank/work-items/<id>/plan.md`, `.memory-bank/work-items/<id>/plan.yaml`, `.memory-bank/TODO.md`
- `evidence.work_item_id`: the work item ID created
- `evidence.plan_path`: full path to the plan.yaml file (the machine-readable one the step-loop uses)
- `evidence.plan_phases_count`: number of phases in the produced plan (0 if blocked)
- `evidence.plan_steps_count`: total number of steps across all phases (0 if blocked) — should be ALL steps needed, not capped
- `evidence.critical_path_length`: number of steps on the critical path (minimum sequential chain)
- `related_commands`: suggested follow-up commands
  - "To execute all steps autonomously, run: `/axiom-step-loop --work-item <id> mode=full-queue`" ← PRIMARY
  - "To execute the plan step by step, run: `/axiom-step-loop --work-item <id>`"
  - "To verify the work, run: `/axiom-verify --work-item <id>`"

### Cross-References
- "Work item plan: `.memory-bank/work-items/<id>/plan.yaml`" (machine-readable, step-loop uses this)
- "Work item plan (human): `.memory-bank/work-items/<id>/plan.md`"
- "Work item meta-plan: `.memory-bank/work-items/<id>/meta-planning.md`"
- "Memory bank writing rules: `.memory-bank/work-items/_prompt.md`"
- "Intent resolution spec: `specs/59-Collaborative-Intent-Resolution.md`"
- "Plan schema: `specs/03-Plan-Schema.md`"
- "Plan schema skill: `.opencode/skills/axiom-plan-schema/SKILL.md`"
- "Meta-planning contract: `.opencode/skills/axiom-meta-planning-contract/SKILL.md`"
- "Step queue design: `.opencode/skills/middle-out-planning-axiom/SKILL.md`"
- "To execute all steps: `/axiom-step-loop --work-item <id> mode=full-queue`"

See: `specs/59-Collaborative-Intent-Resolution.md`, `specs/04-XML-Protocol.md`, `specs/03-Plan-Schema.md`

axiom:trace spec=specs/59-Collaborative-Intent-Resolution.md work_item=command-quality-01
