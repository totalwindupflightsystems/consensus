---
name: prd-spec-merge-axiom
description: Merge a finalized PRD (and its Spec-Merge Appendix) into the repo's specs/ directory. Produces updated or new spec files with traceable requirements, realized-by links, and L0 prd→spec trace markers.
version: "1.1"
tags:
  vertical: [writing, planning]
  category: writing
  core: false
---

# prd-spec-merge-axiom

## What I do

I take a finalized PRD (produced by `prd-generator-axiom`) and its **Spec-Merge Appendix** and merge them into the repo's `specs/` directory.

My job is to bridge the gap between PM intent (PRD) and engineering contracts (specs) — without losing traceability, without inventing facts, and without leaking implementation instructions into the wrong layer.

## When to use me

- A PRD has been finalized and stored in `.memory-bank/prds/`
- You need to create or update `specs/*.md` files to reflect the PRD's requirements
- You want to establish the L0 `prd → spec` trace link so the full trace graph is navigable
- You are preparing for `@pm-axiom` to produce a plan

Do NOT use me to:
- Generate a PRD from scratch → use `prd-generator-axiom`
- Write implementation code → use `@dev-axiom`
- Produce a plan → use `@pm-axiom`

## Inputs

Required:
- Path to the PRD file (e.g., `.memory-bank/prds/billing/csv-export.md`)
- The Spec-Merge Appendix JSON block from the PRD (the `<spec_merge_appendix>` block)

Optional but strongly preferred:
- Existing spec files that may be affected (from `specs/README.md` inventory)
- Baseline repo state (commit hash, branch) from the appendix
- Work item ID for trace markers

## Outputs

1. **Updated or new `specs/*.md` files** with:
   - New REQ/NFR sections for each requirement from the PRD
   - Acceptance criteria (testable, not vague)
   - "Realized by" placeholders (to be filled by `@dev-axiom`)
   - `axiom:trace` markers with `prd=` field pointing back to the PRD in `.memory-bank/prds/`

2. **Updated `specs/README.md`** with new spec entries

3. **Updated `.memory-bank/prds/_index.md`** inventory row linking PRD → spec(s)

4. **Merge note** at `.memory-bank/prds/<area>/<feature>-merge.md` (or alongside the PRD) recording the merge decision

## Non-negotiables

1. Never invent codebase facts, endpoints, schemas, or test results.
2. Hints labeled `INFERRED` or `SUGGESTED` in the appendix must be treated as proposals, not facts. Surface them as "Open decisions" or "Key questions for engineers" in the spec — not as requirements.
3. Only hints labeled `OBSERVED` (with evidence) may be treated as confirmed facts.
4. Specs are contracts. Every requirement added must be testable (has acceptance criteria + negative cases).
5. Never write implementation instructions into specs. If you find them, move them to "Key Questions for Engineers" or an ADR.
6. Preserve existing spec content. Append new sections; do not overwrite existing requirements.
7. Add `axiom:trace` markers near every new requirement section.

## Workflow

### Step 1 — Preflight

Read:
- `specs/README.md` (spec inventory — what exists)
- `specs/00-PRD.md` (product intent — alignment check)
- `specs/21-Traceability-Doctrine.md` (trace rules)
- The PRD file at `.memory-bank/prds/<area>/<feature>.md`
- The Spec-Merge Appendix JSON

Identify:
- Which existing spec files are affected (from `impact_analysis.existing_flows_touched` and `code_touchpoints`)
- Whether a new spec file is needed or existing ones should be extended
- Conflicts or tensions flagged in `impact_analysis.spec_conflicts_or_tensions`

### Step 2 — Conflict and tension scan

For each item in `spec_conflicts_or_tensions`:
- Read the referenced spec section
- Determine if the PRD requirement contradicts, extends, or replaces it
- If contradiction: draft an ADR stub and mark as "Open decision" — do NOT silently overwrite
- If extension: add new section with clear "extends REQ-XXX" note
- If replacement: mark old requirement as deprecated with pointer to new one

### Step 3 — Requirements extraction

From the PRD's `requirements_trace` array:
- For each `req_id` (R1, R2, ...):
  - Map to a spec section (new or existing)
  - Write a REQ-NNN block with:
    - Requirement statement (from PRD, product-level language)
    - Acceptance criteria (from `acceptance_tests` in the trace)
    - Negative cases (from `test_plan.edge_cases` where relevant)
    - Priority (P0/P1/P2 from the trace)
    - "Realized by" placeholder: `Realized by: TBD`
    - `axiom:trace` marker: `axiom:trace work_item=<ID> prd=.memory-bank/prds/<area>/<file>.md spec=specs/<file>.md#req-nnn plan= impl= test= evidence=`

### Step 4 — NFR extraction

From the PRD's `performance_or_sla_sensitivity` and `integration_touchpoints`:
- Add NFR sections for latency, availability, throughput where specified
- Add integration contract stubs for each touchpoint with `unknowns` surfaced as open decisions

### Step 5 — Telemetry and rollout

From `telemetry` and `rollout`:
- Add a "Telemetry" section to the spec listing required events and properties
- Add a "Rollout" section with feature flags, migration notes, and rollback plan (product-level)

### Step 6 — HINTS processing

For each hint in `hints_for_spec_merge_agent`:
- `OBSERVED` + high confidence → include as a confirmed note in the spec (cite evidence)
- `OBSERVED` + medium/low confidence → include as a note with "How to verify" caveat
- `INFERRED` → surface as "Open decision" or "Key question for engineers"
- `SUGGESTED` → surface as "Open decision" or "Key question for engineers"

Never promote `INFERRED` or `SUGGESTED` hints to requirements without human confirmation.

### Step 7 — Trace links

After writing all spec sections:
1. Add a `prd=.memory-bank/prds/<area>/<file>.md` field to every new `axiom:trace` marker
2. Update `.memory-bank/prds/_index.md` inventory row:
   `| .memory-bank/prds/<area>/<file>.md | <feature> | merged | specs/<file>.md | <work_item> | <date> |`
3. Create a merge note at `.memory-bank/prds/<area>/<feature>-merge.md` (see template below)

### Step 8 — Spec README update

Add new spec files to `specs/README.md` with:
- File path
- One-line description
- Status: `draft | active | deprecated`
- PRD source: `.memory-bank/prds/<area>/<file>.md`

### Step 9 — Quality gate

Before declaring done, verify:
- [ ] Every PRD requirement (R1, R2, ...) maps to at least one spec REQ-NNN
- [ ] Every REQ-NNN has testable acceptance criteria (not vague)
- [ ] Every REQ-NNN has at least one negative case
- [ ] No implementation instructions leaked into spec body
- [ ] All `INFERRED`/`SUGGESTED` hints are surfaced as open decisions, not requirements
- [ ] `axiom:trace` markers present near every new REQ section with `prd=.memory-bank/prds/...`
- [ ] `.memory-bank/prds/_index.md` inventory row updated
- [ ] `specs/README.md` updated
- [ ] Merge note created at `.memory-bank/prds/<area>/<feature>-merge.md`

## Merge Note Template

Create `.memory-bank/prds/<area>/<feature>-merge.md`:

```markdown
---
mb:
  type: note
  title: "PRD Merge: <feature>"
  created: YYYY-MM-DD
  updated: YYYY-MM-DD
  tags: [prd, spec-merge]
  links:
    up: "../_index.md"
    related:
      - "./<feature>.md"
      - "../../../specs/<spec-file>.md"
  source:
    type: doc
    ref: ".memory-bank/prds/<area>/<feature>.md"
  git:
    commit: ""
    paths: []
---

# PRD Merge: <feature>

## Summary
Brief description of what was merged and why.

## PRD Source
- File: `.memory-bank/prds/<area>/<feature>.md`
- Baseline commit: <from appendix or "unknown">
- Work item: <ID>

## Specs Affected
- `specs/<file>.md` — sections added/updated: REQ-NNN, REQ-NNN+1, ...

## Open Decisions (from INFERRED/SUGGESTED hints)
- [ ] <decision 1> — surfaced in `specs/<file>.md#open-decisions`
- [ ] <decision 2>

## Conflicts Resolved
- <conflict 1>: resolved as <ADR stub / deferred / accepted>

## Merge Status
- [ ] Requirements extracted
- [ ] NFRs extracted
- [ ] Telemetry/rollout sections added
- [ ] Trace markers placed (prd=.memory-bank/prds/...)
- [ ] .memory-bank/prds/_index.md updated
- [ ] specs/README.md updated

## Links
- [Up: PRDs Index](../_index.md)
- [PRD File](./<feature>.md)
- [Spec File](../../../specs/<spec-file>.md)

## Traceability
- Sources: `.memory-bank/prds/<area>/<feature>.md`, Spec-Merge Appendix
- Git: commit / paths (leave blank if unavailable)
```

## Failure Handling

- **Conflicting requirements**: draft ADR stub; mark as "Open decision"; do NOT silently overwrite existing requirements.
- **Missing acceptance criteria in PRD**: surface as "Key question for engineers" in the spec; do not invent criteria.
- **INFERRED hint with no evidence**: treat as a question, not a fact.
- **Spec file doesn't exist yet**: create a new `specs/NN-Feature-Name.md` following the naming convention from `specs/README.md`.
- **PRD appendix missing**: ask for it; do not proceed without it (the appendix is the machine-readable contract for this merge).

## Related

- `prd-generator-axiom/` — upstream skill that produces the PRD + appendix
- `spec-kickoff-axiom/` — alternative for spec creation without a PRD
- `.memory-bank/prds/` — canonical PRD storage (with `_index.md` inventory)
- `specs/README.md` — spec inventory
- `specs/21-Traceability-Doctrine.md` — traceability doctrine (L0 prd→spec link)
