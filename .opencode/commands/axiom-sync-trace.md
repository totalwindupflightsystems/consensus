---
description: Audit and repair traceability markers and links.
agent: tower-axiom
---

Audit and repair `axiom:trace ...` coverage across specs/plans/docs so traceability stays navigable.

Inputs
- Optional: `$ARGUMENTS` = scope hints (paths, work item id, or keywords).

Skills (load on demand):
- `traceability-doctrine` — Canonical trace marker format, required fields, validation checks, and commit/PR templates. Always load this skill before auditing or repairing trace markers.
- `axiom-xml-protocol` — XML envelope format and required tag set.

Do
1) Determine scope:
   - If `$ARGUMENTS` is non-empty, constrain to those paths/ids.
   - Else default to:
     - `specs/`
     - `.memory-bank/work-items/`

2) Audit:
   - Identify missing trace markers near behavior boundaries (spec sections, plan phases/tasks, docs/runbooks).
   - Identify broken links: work_item -> spec -> plan -> evidence.

3) Repair:
   - Add missing `axiom:trace ...` lines.
   - Fix broken file path references.
   - Prefer the canonical trace marker format from `.opencode/skills/traceability-doctrine/SKILL.md`.

4) Verification:
   - Run a quick grep-based spot-check that the expected trace markers exist.

Output (machine-consumable)
- Emit a `<axiom>` XML envelope.
- Use:
  - `<command>/axiom-sync-trace</command>`
  - `<status>ok|fail|blocked</status>`
  - `<summary>` one sentence
  - `<evidence><files_changed>` paths updated (semicolon-separated)
  - `<diagnostics>` for warnings/errors

## Output Contract (what to return to the caller)

### For Human Consumption
- Summary: one sentence stating how many trace markers were added/repaired.
- Confidence: 0-100

### For Agent Consumption (MUST include)
- `evidence.files_changed`: list of ALL files where trace markers were added/repaired (full paths, semicolon-separated)
- `evidence.markers_added`: count of new trace markers added
- `evidence.markers_repaired`: count of broken links fixed
- `evidence.broken_links_found`: count of broken trace links detected (before repair)
- `related_commands`: suggested follow-up commands
  - "To run a full trace audit, call: `@trace-auditor-axiom` with the work item ID"
  - "To run the full sync suite, run: `/axiom-sync-all`"

### Cross-References
- "Trace marker format is defined in: `.opencode/skills/traceability-doctrine/SKILL.md`"
- "Spec: `specs/21-Traceability-Doctrine.md`"
- "To verify trace completeness for a work item, run: `/axiom-verify --work-item <id>`"

axiom:trace spec=specs/21-Traceability-Doctrine.md work_item=command-quality-01
