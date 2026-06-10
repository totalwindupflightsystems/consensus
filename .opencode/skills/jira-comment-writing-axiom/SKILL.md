---
name: jira-comment-writing-axiom
description: Style guide for Jira comments, status updates, blocker notes, and clarification requests. Optimized for progress communication and action ownership.
version: "1.0"
tags:
  vertical: [writing, planning]
  category: writing
  core: false
---

# Jira Comment Writing

Use for progress notes, blocker updates, evidence summaries, and clarification questions inside Jira.

axiom:trace work_item=DEX-73 spec= plan= test= doc=.memory-bank/explorations/writing-style-skill-collection.md prompt=.opencode/skills/jira-comment-writing-axiom/SKILL.md evidence= commit= jira_ref=DEX-73

## Default Shape

```markdown
Status: <done | in progress | blocked>
Due: <YYYY-MM-DD (N days remaining) | YYYY-MM-DD (N days overdue) | not set>

What changed:
- item
- item

Evidence:
- <command/result/path>

Next step:
- owner -> action

Question/Blocker:
- <only if needed>
```

## Style Rules

- Lead with state, not backstory.
- Include the due date line in every progress and blocker comment when a due date is set.
- Keep the comment actionable within one screen.
- Use @mentions only when someone explicitly owns the next move.
- Separate evidence from requests so readers can skim quickly.
- If blocked, say what is blocked, why, and what unlocks it.
- If overdue, call it out explicitly — do not hide it in prose.

## Good Comment Types

- progress update
- blocker escalation
- evidence summary
- clarification request
- handoff note
- due date change notification

## Avoid

- diary-style play-by-play
- vague "working on it" comments
- mixing several unrelated asks into one note
- omitting the due date line when a due date is set
- silently ignoring overdue status
