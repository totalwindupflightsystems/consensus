---
description: Generate a deterministic sitrep report.
agent: dispatch-axiom
---

Produce a sitrep report.

Inputs
- `$ARGUMENTS` optional: sitrep mode hints (e.g., `daily`, `weekly`, `delta`, `debrief`) and scope.

Skills (load on demand):
- `axiom-copilot` — If the user is new and just wants to understand where things stand, load this for a conversational walkthrough of the sitrep output.
- `sitrep-ascii-graphs` — Always loaded (see step 1). Provides ASCII progress rendering.
- `axiom-gap-analysis` — For deeper multi-agent gap analysis beyond basic sitrep.

Do
1) Load skill `.opencode/skills/sitrep-ascii-graphs/SKILL.md` to enable ASCII progress graph rendering.
2) Call `@sitrep-axiom` with `$ARGUMENTS` as mode/scope hints.
3) Use deterministic ordering.
4) Prefer evidence-backed statements.
5) Include injected follow-ups for any gaps.
6) Ensure `progress_graphs` section (6.5) is present in the output for modes: sitrep_now, daily_sitrep, weekly_sitrep, release_readiness_report.

Output (machine-consumable)
- Include the sitrep report in the response.
- Also emit a `<axiom>` XML envelope (per `.opencode/skills/axiom-xml-protocol/SKILL.md`) with:
  - `<command>/axiom-sitrep</command>`
  - `<status>ok|fail|blocked</status>`
  - `<summary>` one sentence
  - `<diagnostics>` for warnings/errors

## Output Contract (what to return to the caller)

### For Human Consumption
- Summary: one sentence stating the sitrep mode and overall health status.
- Confidence: 0-100

### For Agent Consumption (MUST include)
- `evidence.files_changed`: list of ALL files created/modified (full paths, semicolon-separated)
  - Typically empty (sitrep is read-only) unless a sitrep report was written to memory bank
- `evidence.sitrep_mode`: the mode used (now|daily|weekly|delta|debrief)
- `evidence.gaps_found`: count of gaps/blockers identified in the sitrep
- `evidence.injected_steps_count`: count of follow-up steps injected
- `related_commands`: suggested follow-up commands based on sitrep findings
  - "To execute the highest-priority gap, run: `/axiom-step --work-item <id>`"
  - "To run a full adversarial review on the active work item, run: `/axiom-adversary --target current`"
  - "To refresh the roadmap, run: `/axiom-roadmap-refresh`"

### Cross-References
- "Sitrep methodology is in: `.opencode/skills/sitrep-axiom/SKILL.md`"
- "Active work item is at: `.memory-bank/work-items/_current.md`"
- "TODO roadmap is at: `.memory-bank/TODO.md`"

axiom:trace spec=specs/13-Command-Registry.md work_item=command-quality-01
