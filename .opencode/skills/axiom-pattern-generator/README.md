# axiom-pattern-generator

Meta-skill for observing, extracting, and formalizing reusable multi-tool workflow patterns. Patterns are to agents what flight procedures are to pilots — standardized, repeatable protocols where everyone knows the rules just by reading the words.

## When to Load

Load when:
- You're about to do a multi-tool task and want to formalize it as a pattern
- You notice you're repeating the same tool sequence across different tasks
- Another agent asks "how do I combine tool X with tool Y?"
- You want to build a new workflow that doesn't have a pattern yet
- You need to teach an agent a reusable approach to a recurring task type

## What's a Pattern?

A pattern is a **published procedure** — like an ILS approach to a runway. When you say "execute the investigation pattern," every agent knows:
- What tools to call (the tool chain)
- In what order (the flow)
- With what inputs (the data table)
- How to know it's working (on-track signals)
- What to do when it drifts (adjustment protocol)
- When to abort (off-track threshold)

## Files

| File | Purpose |
|------|---------|
| `SKILL.md` | Full pattern generation instructions — the 6-step loop |
| `REFERENCE.md` | Tool API quick-reference for all 7 plugin systems |
| `README.md` | This file |

## Quick Start

1. Load this skill
2. Identify a recurring multi-tool task
3. Execute it while tracking your tool calls
4. Extract the pattern (tool chain + pivot points + signals)
5. Formalize using the template in SKILL.md §4
6. Register in the skill map

## Related

- **Spec**: `specs/121-Pattern-Generator.md` (the contract — 80+ requirements)
- **Skill Map**: `.opencode/skills/axiom-skill-map/SKILL.md` (where patterns are registered)
- **Example patterns**: `pattern-investigate-changes`, `pattern-explore-codebase`, etc.

<!-- axiom:trace work_item=pattern-generator-01 spec=specs/121-Pattern-Generator.md -->
