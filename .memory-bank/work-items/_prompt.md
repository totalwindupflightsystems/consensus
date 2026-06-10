# Work Items Folder Prompt

This folder contains per-work-item plans, verification evidence, and run history.

If this repository also has `specs/` with real contracts, cite and follow those specs.
If `specs/` is missing or only contains stubs, treat the referenced `.opencode/skills/*/SKILL.md` documents as the portable contracts.

Portable structure
```
.memory-bank/work-items/
  <WORK_ITEM_ID>/
    meta-planning.md
    plan.md
    plan.yaml
    verification.md
    runs/
      <RUN_ID>/
        verification.md
        outputs.md
```

## Authoritative References (Portable)

Use these sources when creating or updating work-item artifacts.

| Reference | What it governs | Required citation in |
|---|---|---|
| `.opencode/skills/meta-plan-axiom/SKILL.md` | Work-item meta-plan workflow (`/axiom-meta-plan`) | `meta-planning.md` |
| `.opencode/skills/axiom-meta-planning-contract/SKILL.md` | Required sections + fail-closed meta-planning/plan contract | `meta-planning.md`, `plan.md` |
| `.opencode/skills/axiom-plan-schema/SKILL.md` | Machine-readable `plan.yaml` schema (parseable) | `plan.yaml` |
| `.opencode/skills/implementation-plan-history/SKILL.md` | Rolling vs immutable run snapshots; history rules | `verification.md`, `runs/<RUN_ID>/verification.md` |
| `.opencode/skills/evidence-bundle-schema/SKILL.md` | Evidence bundle shape + “never invent evidence” | `verification.md`, `runs/<RUN_ID>/verification.md` |
| `.opencode/skills/axiom-confidence-scoring/SKILL.md` | Confidence signals and how to record/explain them | `verification.md` |
| `.opencode/skills/traceability-doctrine/SKILL.md` | Trace marker format + minimum link requirements | all work-item notes |
| `.opencode/skills/baby-steps-methodology/SKILL.md` | Smallest meaningful step + validate-after-every-step | `plan.md`, `verification.md` |
| `.opencode/skills/axiom-xml-protocol/SKILL.md` | Required tags when a slash command demands structured output | wherever slash commands are used |
| `.opencode/skills/axiom-todo/SKILL.md` | Updating `.memory-bank/TODO.md` roadmap | `plan.md` (only when TODO is edited) |
| `.opencode/skills/axiom-implementation-plans/SKILL.md` | Updating `.memory-bank/implementation-plans/` aligned to TODO | `plan.md` (only when implementation plans are edited) |

---

## Fail-Closed Writing Rules

1) Source-cite or do not write.
- Behavioral claims MUST cite one of: a spec path (if present), a skill path, command output, or test output.

2) Separate verified facts from assumptions.
- Use explicit labels:
  - `[FACT]` backed by evidence (spec/skill/command/test output)
  - `[ASSUMPTION]` includes "How to verify" and "Impact if wrong"

3) Never invent evidence.
- If a test or command was not run, write "not run".

4) Use explicit progress states.
- `done`, `in-progress`, `not-started`, `blocked`, `deferred`

5) Trace-link or block.
- Every work item MUST include at least one `axiom:trace ... work_item=<WORK_ITEM_ID> ...` line.

Example (fill the fields that apply):
`axiom:trace work_item=<WORK_ITEM_ID> spec=<REF> plan=<REF> impl=<REF> test=<REF> doc=<REF> ops=<REF> prompt=<REF> evidence=<REF> commit=<REF>`

6) Rolling vs immutable evidence.
- Rolling: `<WORK_ITEM_ID>/verification.md` (mutable; cumulative)
- Immutable snapshots: `runs/<RUN_ID>/verification.md` (never edit; corrections go in a new run)

---

## Required Files (Per Work Item)

Each `.memory-bank/work-items/<WORK_ITEM_ID>/` SHOULD contain:
- `meta-planning.md`
- `plan.md`
- `plan.yaml`
- `verification.md` (rolling)
- `runs/<RUN_ID>/verification.md` (immutable snapshots)

Optional:
- `pr.md`
- `runs/<RUN_ID>/outputs.md`
- `runs/<RUN_ID>/trace-audit.md`
- `runs/<RUN_ID>/qa-report.md`

---

## Operator-Brief Style (Recommended Default)

New work items SHOULD be written as an operator brief first and an archive second. The goal is that `/axiom-step-loop` and a fresh subagent can quickly answer: what are we trying to prove, where is the cursor, what is the next smallest step, and what evidence closes it.

Use this style unless the work item is purely archival:

1. Lead with a plain-language mission paragraph in `meta-planning.md` and `plan.md`.
2. Keep `In scope` / `Out of scope` lists crisp.
3. Map every acceptance criterion to a concrete verification path before implementation starts.
4. Prefer 3–5 phases and 1–3 steps per task.
5. Put the current cursor near the top of `plan.yaml` as `execution.cursor` when supported.
6. Make each `plan.yaml` step self-contained: short title, command/action when applicable, spec/skill reference, verification list, and evidence path.
7. Keep long history, reconciliation notes, and prior commits in `verification.md`, `runs/`, or a linked note rather than the active step queue.

axiom:trace work_item=platform-stability-01 spec=specs/03-Plan-Schema.md,specs/09-Baby-Steps-Methodology.md prompt=.memory-bank/work-items/_prompt.md evidence=.memory-bank/work-items/platform-stability-01/plan.yaml

---

## Minimal Templates (Portable)

### `meta-planning.md`
- Operator-brief summary, scope fences (in/out), acceptance criteria (testable), decision points, assumptions + how to verify, open questions.
- Cite the skills used.

### `plan.md`
- One short mission paragraph.
- AC -> verification mapping table.
- Compact phases/tasks with verification + evidence path + rollback note.

### `plan.yaml`
- Must validate against `.opencode/skills/axiom-plan-schema/SKILL.md`.
- Each step SHOULD include a `spec_ref` if specs exist; otherwise cite the relevant skill.
- Put `execution.cursor` near the top and make each step self-contained enough for an isolated subagent.

### `verification.md`
- Must follow `.opencode/skills/evidence-bundle-schema/SKILL.md`.
- Must list commands actually run and their results.

---

## Trigger Rules

Create a new work item folder when:
- A new work request arrives (ticketed or not).
- A TODO/plan track begins and there is no matching work item yet.

Create a new `runs/<RUN_ID>/` snapshot when:
- You execute work (even if it fails).
- You retry a failed or blocked run.
