---
description: Produce/update meta-planning and an implementation plan for a work item.
agent: pm-axiom
---

You are generating a meta-plan and implementation plan for Axiom.

Inputs:
- Work item id: $WORK_ITEM_ID
- Goal / request summary: $GOAL

Optional inputs:
- Repo: $REPO
- Extra context (links/constraints/notes): $CONTEXT
- `--kiss` flag: enable KISS mode — do full planning and meta-planning to understand the request, but when generating steps apply the KISS filter: only generate steps that map to an acceptance criterion. Do not generate hardening, polish, or perfection steps that aren't required to meet the stated ACs. Load `kiss-axiom` skill when this flag is set.

Skills (load on demand):
- `axiom-meta-planning-contract` — Detailed meta-planning rules and required sections.
- `axiom-plan-schema` — Plan YAML structure reference.
- `kiss-axiom` — Load when `--kiss` flag is set. Score every generated step against the ACs. Cut steps that don't map to an AC. Prevents over-engineering and scope creep in the plan.
- `jira-workflow-axiom` — Jira operating model (load when `$WORK_ITEM_ID` is a Jira key).

Do:
1) Read `specs/README.md` and the relevant spec files.
2) Read `.memory-bank/_index.md` and relevant Memory Bank context.
3) **Jira ticket context** (when `$WORK_ITEM_ID` is a Jira key like `PROJ-123`):
   a) If Atlassian MCP is available, read the Jira ticket (description, acceptance criteria, comments, priority, labels, linked issues).
   b) Check intake confidence; follow pushback protocol if below threshold (per `specs/05-Jira-Integration.md#intake-confidence-pushback-protocol`).
   c) Identify parent/child/blocking Jira relationships.
   d) If Atlassian MCP is unavailable, proceed with intake data and note "Jira sync deferred — MCP unavailable".
4) Write or update:
   - `.memory-bank/work-items/$WORK_ITEM_ID/meta-planning.md`
   - `.memory-bank/work-items/$WORK_ITEM_ID/plan.md`
   - `.memory-bank/work-items/$WORK_ITEM_ID/plan.yaml`
5) Follow the portable contract in `.opencode/skills/axiom-meta-planning-contract/SKILL.md`.
6) **Jira status management** (when Jira-sourced):
   a) Include `jira_key` in `plan.yaml` `work_item` section.
   b) Include `jira_ref=$WORK_ITEM_ID` in all `axiom:trace` markers.
   c) Post a progress comment to Jira summarizing the meta-plan (per `specs/05-Jira-Integration.md#comment-format`).
   d) Transition ticket to "AI Handoff (Plan)" (per `specs/10-Lifecycle-State-Machine.md`).
   e) If questions arise, post them as a Jira comment so the ticket author can respond in-context.

Output:
- Emit the required XML envelope per `.opencode/skills/axiom-xml-protocol/SKILL.md`.

## Output Contract (what to return to the caller)

### For Human Consumption
- Summary: one sentence stating the work item ID and how many phases/tasks/steps were planned.
- Confidence: 0-100

### For Agent Consumption (MUST include)
- `evidence.files_changed`: list of ALL files created/modified (full paths, semicolon-separated)
  - Typically: `.memory-bank/work-items/$WORK_ITEM_ID/meta-planning.md`, `plan.md`, `plan.yaml`
- `evidence.work_item_id`: the stable work item ID
- `evidence.meta_planning_path`: full path to `meta-planning.md`
- `evidence.plan_path`: full path to `plan.yaml`
- `evidence.phases_count`: number of phases in the plan
- `evidence.steps_count`: total number of steps across all phases
- `related_commands`: suggested follow-up commands
  - "To execute the first step, run: `/axiom-step --work-item <id>`"
  - "To run all steps in sequence, run: `/axiom-step-loop --work-item <id>`"
  - "To view the plan, read: `.memory-bank/work-items/<id>/plan.md`"

### Cross-References
- "Meta-planning contract is in: `.opencode/skills/axiom-meta-planning-contract/SKILL.md`"
- "Plan schema is in: `.opencode/skills/axiom-plan-schema/SKILL.md`"
- "Work item artifacts are at: `.memory-bank/work-items/$WORK_ITEM_ID/`"

References:
- `specs/20-Meta-Planning.md` (meta-planning contract, Jira tracking section)
- `specs/05-Jira-Integration.md` (ticket-as-work-unit, comment format, pushback)
- `specs/10-Lifecycle-State-Machine.md` (Jira status transitions)
- `specs/21-Traceability-Doctrine.md#external-reference-fields` (jira_ref in trace markers)

axiom:trace spec=specs/20-Meta-Planning.md work_item=command-quality-01
