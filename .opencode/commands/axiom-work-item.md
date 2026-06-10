---
description: Create or refresh a work item (meta-plan + plan artifacts).
agent: dispatch-axiom
---

Create or refresh a work item using the portable meta-planning contract.

Inputs
- `$WORK_ITEM_ID` optional (preferred). If missing, derive a stable id.
  - When Jira-sourced, use the Jira key (e.g., `PROJ-123`).
  - Otherwise, use a stable kebab-case identifier (e.g., `bootstrapping-01`).
- `$ARGUMENTS` required: work request / objective text.
- `--kiss` flag (optional): enable KISS mode — do full planning and meta-planning to understand the request, but only generate steps that map to an acceptance criterion. Prevents over-engineering and scope creep. Recommended for automated/runner contexts.

Skills (load on demand):
- `kiss-axiom` — Load when `--kiss` flag is set. Score every generated step against the ACs. Cut steps that don't map to an AC. Prevents over-engineering and scope creep in the plan. Does NOT skip planning or meta-planning — those still run in full to understand the request.
- `axiom-copilot` — If the user is new, load this to explain what work items are and how they connect to specs, plans, and execution.
- `axiom-meta-planning-contract` — Detailed meta-planning rules. **Load this first** — it defines the required sections, light vs standard meta-plan, and the pre-execution gate that blocks step execution if artifacts are missing.
- `axiom-plan-schema` — Plan YAML structure reference. **Load this** to produce a `plan.yaml` that the step-loop can actually execute. The schema defines phases, tasks, steps, verification objects, `execution.cursor`, and `jira_key`.
- `middle-out-planning-axiom` — Load when the work item crosses system boundaries or has integration risk. Start at the critical boundary, prove it works, then expand outward. Prevents building disconnected components.
- `working-backwards-axiom` — Load when the work item has a user-visible surface (CLI, API, UI, worker output). Plan from the end-user experience backward. Every step must include how to verify it's connected end-to-end.
- `baby-steps-methodology` — Load to enforce smallest-meaningful-change discipline. Each step must be independently verifiable. No step should do more than one thing.
- `traceability-doctrine` — Load to ensure every step has a `axiom:trace` marker linking it to a spec, plan phase, and evidence path.
- `axiom-todo` — Load to understand how TODO.md entries must be structured and how they align to the plan.
- `jira-workflow-axiom` — Jira operating model (load when `$WORK_ITEM_ID` is a Jira key).

Do
1) Ensure `.memory-bank/work-items/<WORK_ITEM_ID>/` exists.

2) **Read the memory bank context** before planning:
   - `.memory-bank/work-items/_prompt.md` — authoritative references and fail-closed writing rules for all work-item artifacts.
   - `.memory-bank/work-items/_index.md` — existing work items (check for duplicates and related work).
   - `.memory-bank/_index.md` — overall memory bank map.
   - `.memory-bank/TODO.md` — current open steps (avoid duplicating existing work).

3) **Jira context** (when `$WORK_ITEM_ID` is a Jira key like `PROJ-123`):
   a) If Atlassian MCP is available, read the Jira ticket (description, AC, comments, priority, labels, linked issues).
   b) Carry the Jira key into all downstream artifacts (`plan.yaml` `jira_key`, `axiom:trace` markers with `jira_ref=`).
   c) If Atlassian MCP is unavailable, note "Jira sync deferred — MCP unavailable" and proceed with available context.

4) **Create the three required artifacts** (all three are required by the pre-execution gate — the step-loop will refuse to run without them):

   **a) `meta-planning.md`** — Load `axiom-meta-planning-contract` skill. Write the meta-plan with all required sections:
   - Intent (what changes and why, in-scope / out-of-scope)
   - Contract Reconciliation (Jira ref, AC summary, specs touched, conflicts)
   - Decision Points (product/policy decisions, defaults chosen)
   - Risks and Blast Radius (security-sensitive areas, rollback notes)
   - Verification Design (evidence that proves done, required vs optional checks)
   - Ambiguity Assessment (assumptions, rating, missing inputs)

   **b) `plan.md`** — Human-readable implementation plan with phases/tasks/steps. Load `working-backwards-axiom` and `middle-out-planning-axiom` as appropriate. Each step must have: objective, actions, verification, files expected.

   **c) `plan.yaml`** — Machine-readable plan that the step-loop executes. Load `axiom-plan-schema` skill. **This is the most important artifact** — the step-loop reads this, not plan.md. Required fields:
   ```yaml
   version: 1
   work_item:
     id: "<WORK_ITEM_ID>"
     jira_key: null  # or Jira key if applicable
   meta_planning:
     notes_md_path: ".memory-bank/work-items/<id>/meta-planning.md"
   plan:
     phases:
       - id: "phase-1"
         title: "..."
         tasks:
           - id: "task-1-1"
             title: "..."
             steps:
               - id: "step-1-1-1"
                 title: "..."
                 command: "/axiom-step"
                 spec_ref: "specs/..."
                 verification:
                   - type: "command"
                     command: "<verification command>"
                     required: true
   execution:
     status: "pending"
     cursor:
       phase_id: "phase-1"
       task_id: "task-1-1"
       step_id: "step-1-1-1"
   ```

5) **Design the step queue for maximum throughput** — this is the most important planning decision:
   - **Group independent steps into the same phase/task** so `mode=full-queue` and `batch` strategy can parallelise them.
   - **Identify the critical path** (the sequence of steps that cannot be parallelised) and make it as short as possible.
   - **Do NOT artificially split work** into more phases than necessary. A phase boundary is a synchronisation point — only add one when later steps genuinely depend on earlier steps completing.
   - **Do NOT cap the number of steps** — write every step needed to complete the work. The step-loop will execute them all with `mode=full-queue`. Artificial caps cause multiple loop invocations for no reason.
   - **Each step should be independently verifiable** (baby-steps principle) but steps within a task can run in parallel if they don't share files.
   - **Prefer `batch` strategy** for plans with a mix of dependent and independent steps — it maximises parallelism while respecting dependencies.

6) **Write TODO.md entries** — append unchecked items for every step:
   ```markdown
   ## <WORK_ITEM_ID> — <title>
   - [ ] `<WORK_ITEM_ID>` `phase-1/task-1-1/step-1-1-1` — <objective>. Spec: `<spec ref>`
   ```
   Check for duplicates before appending. Do not re-add steps that already exist.

7) Update indexes:
   - `.memory-bank/work-items/_index.md` — add entry for this work item
   - `.memory-bank/_index.md` — update if this is a new work item

8) **Jira post-creation** (when Jira-sourced and MCP available):
   a) Post a progress comment to Jira summarizing the work item creation.
   b) Transition ticket to "AI Handoff (Plan)" if not already there.

Output (machine-consumable)
- Emit a `<axiom>` XML envelope (per `.opencode/skills/axiom-xml-protocol/SKILL.md`).
- Use:
  - `<command>/axiom-work-item</command>`
  - `<status>ok|fail|blocked</status>`
  - `<summary>` one sentence
  - `<evidence>` include:
    - `<work_item_id>`
    - `<files_changed>` (semicolon-separated)
  - `<diagnostics>` for warnings/errors

## Output Contract (what to return to the caller)

### For Human Consumption
- Summary: one sentence stating the work item ID and whether it was created or refreshed.
- Confidence: 0-100

### For Agent Consumption (MUST include)
- `evidence.files_changed`: list of ALL files created/modified (full paths, semicolon-separated)
  - Always includes: `.memory-bank/work-items/<id>/meta-planning.md`, `plan.md`, `plan.yaml`, `_index.md` files, `.memory-bank/TODO.md`
- `evidence.work_item_id`: the stable work item ID
- `evidence.work_item_path`: full path to the work item folder (`.memory-bank/work-items/<id>/`)
- `evidence.plan_path`: full path to `plan.yaml`
- `evidence.meta_planning_path`: full path to `meta-planning.md`
- `evidence.step_count`: total number of steps across all phases (should be ALL steps needed, not capped)
- `evidence.phase_count`: number of phases
- `evidence.critical_path_length`: number of steps on the critical path (sequential minimum)
- `related_commands`: suggested follow-up commands
  - "To execute all steps autonomously, run: `/axiom-step-loop --work-item <id> mode=full-queue`" ← PRIMARY
  - "To execute the first step, run: `/axiom-step --work-item <id>`"
  - "To run all steps in sequence, run: `/axiom-step-loop --work-item <id>`"
  - "To verify the work item, run: `/axiom-verify --work-item <id>`"

### Cross-References
- "Work item artifacts are at: `.memory-bank/work-items/<id>/`"
- "Memory bank writing rules: `.memory-bank/work-items/_prompt.md`"
- "Plan schema is defined in: `.opencode/skills/axiom-plan-schema/SKILL.md`"
- "Meta-planning contract is in: `.opencode/skills/axiom-meta-planning-contract/SKILL.md`"
- "Step queue design: `.opencode/skills/middle-out-planning-axiom/SKILL.md`"
- "Working backwards planning: `.opencode/skills/working-backwards-axiom/SKILL.md`"

axiom:trace spec=specs/13-Command-Registry.md work_item=command-quality-01
