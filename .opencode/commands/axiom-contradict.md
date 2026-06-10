---
description: Detect contradictions and misalignments across specs, plans, code, config, and agent outputs.
agent: dispatch-axiom
---

Scan for contradictions — places where two or more things in the system are not aligned or are fighting each other.

Inputs
- `$ARGUMENTS` optional: scope to scan (e.g., a spec file, a work item, a plan, "all specs", "recent changes"). If omitted, scan specs/ and any active work item context.

Skills (load on demand):
- `contradiction-detection-axiom` — Always load. Core detection method, taxonomy, and output format.
- `decision-archaeology-axiom` — Load when a contradiction's origin is unclear and you need to trace why the conflicting claims exist.
- `adr-manager-axiom` — Load when a contradiction requires an ADR to resolve.
- `axiom-copilot` — If the user is new and needs help understanding what contradictions mean.

Do
1) Load skill `.opencode/skills/contradiction-detection-axiom/SKILL.md`.
2) Determine scope from `$ARGUMENTS`. Default: scan `specs/` for spec-vs-spec contradictions and any active work item for plan-vs-constraint and spec-vs-implementation contradictions.
3) Execute the 5-step detection method from the skill:
   a) Gather claims from artifacts in scope
   b) Normalize claims (subject, predicate, source, confidence)
   c) Cross-reference claims pairwise for incompatibilities
   d) Classify each contradiction by type and severity
   e) Produce the contradiction report
4) For each critical or high severity contradiction, produce resolution recommendations with injected steps.
5) If contradictions involve agent outputs, call `@devils-advocate-axiom` for arbitration.
6) If contradictions require a decision, recommend an ADR via `@specwriter-axiom`.

Output (machine-consumable)
- Emit a `<axiom>` XML envelope (per `.opencode/skills/axiom-xml-protocol/SKILL.md`) with:
  - `<command>/axiom-contradict</command>`
  - `<status>ok|fail|blocked</status>` — `ok` if no critical contradictions found; `fail` if critical contradictions exist; `blocked` if scope cannot be scanned
  - `<summary>` one sentence: count and max severity of contradictions found
  - `<detailed_summary>` contradiction inventory with types and severities
  - `<evidence>` include `<files_changed>` if any artifacts were updated during resolution
  - `<diagnostics>` for warnings/errors
  - `<review.risk>` overall risk assessment based on contradiction severity
  - `<review.assumptions>` assumptions made during detection

## Output Contract (what to return to the caller)

### For Human Consumption
- Summary: one sentence stating how many contradictions were found and at what severity.
- Confidence: 0-100

### For Agent Consumption (MUST include)
- `evidence.files_changed`: list of ALL files created/modified (full paths, semicolon-separated)
  - Typically empty (read-only scan) unless ADR was created or artifacts were updated
- `evidence.contradictions_count`: total contradictions found
- `evidence.findings_by_severity`: XML sub-elements — `<critical>N</critical><high>N</high><medium>N</medium><low>N</low>` (parsed as dotted keys: `evidence.findings_by_severity.critical`)
- `evidence.adr_created`: path to ADR file if one was created (or null)
- `related_commands`: suggested follow-up commands
  - "To resolve a contradiction via an ADR, run: `/axiom-spec-request <resolution-text>`"
  - "To run a full adversarial review, run: `/axiom-adversary --target current`"
  - "To trace the origin of a contradiction, run: `/axiom-why <artifact-path>`"

### Cross-References
- "Contradiction detection method is in: `.opencode/skills/contradiction-detection-axiom/SKILL.md`"
- "ADR management is in: `.opencode/skills/adr-manager-axiom/SKILL.md`"

axiom:trace spec=specs/13-Command-Registry.md work_item=command-quality-01
