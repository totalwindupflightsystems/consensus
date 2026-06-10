---
name: spec-writing-axiom
description: Style guide for technical specs and behavior contracts. Optimized for scope fences, requirements clarity, verification paths, and explicit non-goals.
version: "1.0"
tags:
  vertical: [writing, planning, coding]
  category: writing
  core: false
---

# Spec Writing

Use for product specs, system contracts, acceptance-criteria docs, and requirement-heavy design artifacts.

axiom:trace work_item=DEX-73 spec= plan= test= doc=.memory-bank/explorations/writing-style-skill-collection.md prompt=.opencode/skills/spec-writing-axiom/SKILL.md evidence= commit= jira_ref=DEX-73

## Core Rules

- Treat the spec as system truth, not brainstorming scratchpad.
- Define scope, non-goals, and invariants explicitly.
- Make every acceptance criterion testable.
- Separate requirements from implementation notes.
- Mark open decisions instead of faking certainty.

## Recommended Sections

```markdown
## Summary
## Problem / Motivation
## Scope
## Non-goals
## Requirements
## Acceptance Criteria
## Verification
## Risks / Open Decisions
```

## Writing Style

- Prefer SHALL / MUST style only when the repo contract uses normative language.
- Use tables for requirement inventories and AC-to-verification mappings.
- Use bullets for scope fences and risks.
- Use short paragraphs for rationale and tradeoffs.

## Avoid

- mixing future ideas into current contract text
- ambiguous terms like "fast" or "intuitive" without measurable meaning
- acceptance criteria with no verification path
