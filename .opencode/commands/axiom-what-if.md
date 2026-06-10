---
description: Generate and evaluate hypothetical alternatives and what-if scenarios for decisions, designs, or plans.
agent: dispatch-axiom
---

Explore alternatives — generate, evaluate, and compare options for a decision or scenario.

Inputs
- `$ARGUMENTS` required: the decision or scenario to explore (e.g., "what if we used Redis instead of Postgres?", "what if the API goes down for 24h?", "alternatives for retry strategy", or a decision frame).

Skills (load on demand):
- `hypothetical-alternatives-axiom` — Always load. Core method for generating and evaluating alternatives.
- `decision-archaeology-axiom` — Load when exploring alternatives for an existing decision (understand the original choice first).
- `contradiction-detection-axiom` — Load when alternatives are being explored to resolve a contradiction.
- `adr-manager-axiom` — Load when the evaluation produces a decision that should be recorded as an ADR.
- `axiom-copilot` — If the user needs help framing their what-if question.

Do
1) Load skill `.opencode/skills/hypothetical-alternatives-axiom/SKILL.md`.
2) Parse `$ARGUMENTS` to determine the mode:
   - **Decision mode**: "alternatives for X" or "what if we used Y instead of Z?" → run the 5-step alternatives method
   - **Scenario mode**: "what if X happens?" → run the scenario analysis template
3) For **decision mode**:
   a) Frame the decision (question, context, constraints, evaluation criteria)
   b) Generate at least 3 alternatives using the generation strategies (constraint relaxation, technology swap, architecture shift, scope adjustment, inversion, precedent)
   c) Check hard constraints — eliminate violating alternatives
   d) Evaluate remaining alternatives against criteria with weighted scoring
   e) Produce comparison matrix and recommendation
   f) If the decision is significant, recommend creating an ADR
4) For **scenario mode**:
   a) Define the scenario (name, type, assumptions)
   b) Analyze impact across system areas
   c) Identify current mitigations and gaps
   d) Recommend actions to address gaps
5) If `$ARGUMENTS` references an existing decision, first run decision archaeology (load `decision-archaeology-axiom`) to understand the original choice before generating alternatives.
6) Optionally invoke `@devils-advocate-axiom` to stress-test the recommendation.
7) Optionally invoke `@assumption-buster-axiom` to challenge the decision frame assumptions.

Output (machine-consumable)
- Emit a `<axiom>` XML envelope (per `.opencode/skills/axiom-xml-protocol/SKILL.md`) with:
  - `<command>/axiom-what-if</command>`
  - `<status>ok|fail|blocked</status>` — `ok` if alternatives were generated and evaluated; `fail` if the decision frame is too ambiguous; `blocked` if critical context is missing
  - `<summary>` one sentence: recommended alternative and why
  - `<detailed_summary>` full comparison matrix, evaluation scores, and recommendation rationale
  - `<evidence>` sources consulted for precedent and constraint validation
  - `<diagnostics>` for warnings (e.g., "unknown scores for criterion X — data needed")
  - `<review.risk>` risk assessment of the recommended alternative
  - `<review.assumptions>` assumptions in the decision frame and evaluation
  - `<modify_plan>` true if the recommendation changes the current plan; false for pure exploration

## Output Contract (what to return to the caller)

### For Human Consumption
- Summary: one sentence stating the recommended alternative and the key tradeoff.
- Confidence: 0-100

### For Agent Consumption (MUST include)
- `evidence.files_changed`: list of ALL files created/modified (full paths, semicolon-separated)
  - Typically empty (analysis only) unless an ADR was created
- `evidence.alternatives_count`: number of alternatives evaluated
- `evidence.recommended_alternative`: the recommended option (short name)
- `evidence.adr_created`: path to ADR file if one was created (or null)
- `related_commands`: suggested follow-up commands
  - "To record this decision as an ADR, run: `/axiom-spec-request <decision-text>`"
  - "To trace the history of the original decision, run: `/axiom-why <artifact-path>`"
  - "To check for contradictions with the recommendation, run: `/axiom-contradict`"

### Cross-References
- "Alternatives methodology is in: `.opencode/skills/hypothetical-alternatives-axiom/SKILL.md`"
- "ADR management is in: `.opencode/skills/adr-manager-axiom/SKILL.md`"

axiom:trace spec=specs/13-Command-Registry.md work_item=command-quality-01
