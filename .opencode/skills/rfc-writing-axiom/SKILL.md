---
name: rfc-writing-axiom
description: Style guide for RFCs and major design proposals. Optimized for problem alignment, structured options, reviewability, and durable proposal history.
version: "1.0"
tags:
  vertical: [writing, planning]
  category: writing
  core: false
---

# RFC Writing

Use for larger cross-team proposals, major changes, policy shifts, and review-heavy design documents.

axiom:trace work_item=DEX-73 spec= plan= test= doc=.memory-bank/explorations/writing-style-skill-collection.md prompt=.opencode/skills/rfc-writing-axiom/SKILL.md evidence= commit= jira_ref=DEX-73

## Core Rules

- Start from a stable template.
- Align on the problem before polishing the solution.
- Present options and tradeoffs, not just the preferred answer.
- Keep rejected RFCs as historical context.

## Default Structure

```markdown
## Summary
## Problem
## Goals
## Non-goals
## Proposal
## Alternatives
## Risks
## Rollout / Migration
## Open Questions
## Review Ask
```

## Writing Style

- State the problem in terms the reviewers can agree or disagree with.
- Be explicit about the kind of feedback needed.
- Use tables for option comparisons.
- Use short narrative sections for rationale.

## Avoid

- solution-first RFCs with weak problem framing
- pretending there is only one possible approach
- deleting rejected proposals
