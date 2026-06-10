---
description: Reconstruct why a decision was made — trace choices back through specs, ADRs, git history, and agent outputs.
agent: dispatch-axiom
---

Perform decision archaeology — understand why a choice was made and whether it still applies.

Inputs
- `$ARGUMENTS` required: the decision or artifact to investigate (e.g., "why do we use in-process mode?", "why was Postgres chosen?", "why does /v1/alerts retry 5 times?", or a file path like "src/api/alerts.py:47").

Skills (load on demand):
- `decision-archaeology-axiom` — Always load. Core method for tracing decisions through layers.
- `adr-manager-axiom` — Load when the investigation reveals a decision that should have an ADR but doesn't.
- `contradiction-detection-axiom` — Load when the investigation reveals conflicting rationales.
- `hypothetical-alternatives-axiom` — Load when the user wants to explore alternatives after understanding the original decision.
- `axiom-copilot` — If the user needs help framing their question.

Do
1) Load skill `.opencode/skills/decision-archaeology-axiom/SKILL.md`.
2) Parse `$ARGUMENTS` to identify the decision point or artifact under investigation.
3) Execute the 4-phase archaeology method from the skill:
   a) **Identify** the decision point (artifact, question, scope)
   b) **Dig** through the layers (ADRs → specs → memory bank → git → code comments → external)
   c) **Reconstruct** the decision chain (context, alternatives, rationale, tradeoffs)
   d) **Assess** current validity (does the original context still apply?)
4) If the decision was made by an agent, also trace the agent's input context and prompt.
5) If no rationale is found after searching all layers, report "rationale not documented" and recommend creating a retroactive ADR.
6) If the investigation reveals the decision may no longer be valid, flag it for review and optionally invoke `/axiom-what-if` to explore alternatives.

Output (machine-consumable)
- Emit a `<axiom>` XML envelope (per `.opencode/skills/axiom-xml-protocol/SKILL.md`) with:
  - `<command>/axiom-why</command>`
  - `<status>ok|fail|blocked</status>` — `ok` if rationale was found; `fail` if rationale is missing or contradictory; `blocked` if artifacts are inaccessible
  - `<summary>` one sentence: the reconstructed rationale
  - `<detailed_summary>` full decision archaeology report (decision chain, alternatives considered, current validity)
  - `<evidence>` sources consulted and what was found at each layer
  - `<diagnostics>` for warnings (e.g., "no ADR exists for this decision")
  - `<review.risk>` risk if the decision is changed without understanding the original context
  - `<review.assumptions>` assumptions made during reconstruction

## Output Contract (what to return to the caller)

### For Human Consumption
- Summary: one sentence stating the reconstructed rationale and whether it's still valid.
- Confidence: 0-100

### For Agent Consumption (MUST include)
- `evidence.files_changed`: list of ALL files created/modified (full paths, semicolon-separated)
  - Typically empty (read-only archaeology) unless an ADR was created
- `evidence.rationale_found`: true|false — whether a documented rationale was found
- `evidence.decision_still_valid`: true|false|unknown — whether the original context still applies
- `evidence.adr_created`: path to ADR file if one was created (or null)
- `evidence.sources_consulted`: list of artifact paths that were searched
- `related_commands`: suggested follow-up commands
  - "To explore alternatives to this decision, run: `/axiom-what-if <decision-text>`"
  - "To record a missing rationale as an ADR, run: `/axiom-spec-request <rationale-text>`"
  - "To check for contradictions with this decision, run: `/axiom-contradict`"

### Cross-References
- "Decision archaeology method is in: `.opencode/skills/decision-archaeology-axiom/SKILL.md`"
- "ADR management is in: `.opencode/skills/adr-manager-axiom/SKILL.md`"

axiom:trace spec=specs/13-Command-Registry.md work_item=command-quality-01
