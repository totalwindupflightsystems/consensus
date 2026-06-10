---
name: pull-request-writing-axiom
description: Style guide for pull request titles and bodies. Optimized for reviewer context, small-scope summaries, testing notes, and traceable review guidance.
version: "1.0"
tags:
  vertical: [coding]
  category: writing
  core: false
---

# Pull Request Writing

Use for PR titles, PR bodies, review summaries, and merge-ready descriptions.

axiom:trace work_item=DEX-73 spec= plan= test= doc=.memory-bank/explorations/writing-style-skill-collection.md prompt=.opencode/skills/pull-request-writing-axiom/SKILL.md evidence= commit= jira_ref=DEX-73

## Core Rules

- Keep the PR focused on one purpose.
- Explain why the change exists before listing files.
- Tell reviewers what changed, how to review it, and what was verified.
- Link tickets, specs, and evidence instead of repeating large context dumps.

## Recommended Structure

```markdown
## Why
<problem and intent>

## What Changed
- change 1
- change 2

## How To Review
1. Start at <file/area>
2. Then inspect <area>

## Verification
- <command/result>

## Risks
- <risk or "none noted">

## Trace
- Work item: <id>
- Specs: <refs>
- Evidence: <path>
```

## Title Style

- Use an action-oriented title.
- Prefer the user or system effect over the implementation detail.
- Keep it specific enough that reviewers can identify the scope from the PR list alone.

## Avoid

- "misc fixes"
- file-by-file changelogs in the opener
- no verification section
- hiding known risks
