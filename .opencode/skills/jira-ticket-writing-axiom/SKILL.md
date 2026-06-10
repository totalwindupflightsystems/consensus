---
name: jira-ticket-writing-axiom
description: Style guide for Jira issue summaries and descriptions. Optimized for skim-friendly titles, factual context, scope clarity, and testable acceptance criteria.
version: "1.0"
tags:
  vertical: [writing, planning]
  category: writing
  core: false
---

# Jira Ticket Writing

Use for new Jira tickets, issue rewrites, and ticket hygiene passes.

axiom:trace work_item=DEX-73 spec= plan= test= doc=.memory-bank/explorations/writing-style-skill-collection.md prompt=.opencode/skills/jira-ticket-writing-axiom/SKILL.md evidence= commit= jira_ref=DEX-73

## Summary Line Rules

- Make the title skim-friendly.
- Start task-like tickets with a clear verb when possible.
- Describe bugs as observed behavior, not as "bug in X".
- Include only the minimum tag-like context needed.

## Description Structure

```markdown
## Context
<why this work exists>

## Problem
<current behavior or gap>

## Scope
- in scope
- out of scope

## Acceptance Criteria
1. <testable outcome>
2. <testable outcome>

## Timeline
- **Due date**: YYYY-MM-DD
- **Start date**: YYYY-MM-DD (if known)
- **Rationale**: <why this date — sprint boundary, dependency, stakeholder commitment, or estimate>

## References
- related issue/spec/doc
```

## Due Date Rules

- Every ticket MUST have a due date set at creation time.
- If the requester does not specify a due date, estimate one based on scope and complexity. Note the estimate as an assumption: `Due date is estimated; adjust if scope changes.`
- Use sprint end dates as natural due date boundaries when the ticket is sprint-assigned.
- For epics, set the due date to the expected completion of the last child ticket.
- When scope changes materially, update the due date and add a comment explaining why.

## Style Rules

- Be factual and concrete.
- Prefer acceptance criteria over vague success language.
- Keep external links supportive, not mandatory for basic understanding.
- Surface constraints, dependencies, and blockers explicitly.
- Always include the `repo_label` from `.axiom/axiom.config.yaml` in the ticket's labels when creating tickets (e.g., `axiom-backend`). This scopes the ticket to the correct repo in shared Jira projects.

## Avoid

- titles that require opening the ticket to understand them
- narrative rambling before the real ask
- non-testable acceptance criteria
- tickets without a due date
- due dates with no rationale (even a one-line estimate reason helps)
