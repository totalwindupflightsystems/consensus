---
name: runbook-writing-axiom
description: Style guide for incident and operational runbooks. Optimized for command-first execution, verification, rollback, and escalation.
version: "1.0"
tags:
  vertical: [sre, writing]
  category: writing
  core: false
---

# Runbook Writing

Use for operational procedures, incident guides, troubleshooting flows, and recovery playbooks.

axiom:trace work_item=DEX-73 spec= plan= test= doc=.memory-bank/explorations/writing-style-skill-collection.md prompt=.opencode/skills/runbook-writing-axiom/SKILL.md evidence= commit= jira_ref=DEX-73

## Core Rules

- Write for a stressed operator at an inconvenient hour.
- Prefer commands and checks over narrative paragraphs.
- Include verification after the main path and after rollback.
- State escalation conditions explicitly.
- Preserve a working record of what was tried.

## Default Structure

```markdown
## Symptom
## Scope
## Prerequisites
## Steps
## Verification
## Rollback
## Escalation
```

## Style Rules

- Keep each step atomic.
- Show expected output when useful.
- Use numbered steps for execution order.
- Use tables for escalation matrices or symptom triage.

## Avoid

- paragraphs with several actions hidden inside
- no rollback section
- "contact someone" without naming who and when
