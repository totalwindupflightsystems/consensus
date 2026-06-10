---
name: adr-writing-axiom
description: Style guide for architecture decision records. Optimized for one-decision scope, rationale, timestamps, immutability, and downstream consequences.
version: "1.0"
tags:
  vertical: [writing, planning]
  category: writing
  core: false
---

# ADR Writing

Use for architectural decisions that need durable rationale and lifecycle tracking.

axiom:trace work_item=DEX-73 spec= plan= test= doc=.memory-bank/explorations/writing-style-skill-collection.md prompt=.opencode/skills/adr-writing-axiom/SKILL.md evidence= commit= jira_ref=DEX-73

## Core Rules

- One ADR = one decision.
- Record context, decision, rationale, and consequences.
- Timestamp the record.
- Do not rewrite history; supersede with a new ADR instead.

## Default Structure

```markdown
# ADR-<id>: <decision title>

## Status
## Context
## Decision
## Rationale
## Consequences
## Alternatives considered
## Follow-up
```

## Writing Style

- Write the decision in plain language, not puzzle language.
- Explain why this option won.
- Make consequences concrete: cost, complexity, migration, ownership, new ADRs.

## Avoid

- multiple decisions in one ADR
- missing consequences section
- editing old ADRs to hide a reversal
