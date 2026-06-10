# Memory Bank Index

This is the inventory of Memory Bank files and what they contain.

## Core Context Files
- `.memory-bank/projectBrief.md` - Project scope, goals/non-goals, definitions of done.
- `.memory-bank/productContext.md` - Problem space, UX goals, domain terminology.
- `.memory-bank/systemPatterns.md` - Architecture, design patterns, data flow.
- `.memory-bank/techContext.md` - Languages, frameworks, build tools, deployment.
- `.memory-bank/decisionLog.md` - Key technical and product decisions (newest first).
- `.memory-bank/activeContext.md` - Current focus, open questions, active constraints.
- `.memory-bank/progress.md` - What works, what's next, known issues.
- `.memory-bank/TODO.md` - Working execution backlog.

## Work Items
- `.memory-bank/work-items/` - Per-ticket folders (created as needed).
  - `.memory-bank/work-items/_index.md`
  - `.memory-bank/work-items/_prompt.md`
  - `.memory-bank/work-items/_current.md` - Default loop pointer file

## Requests
- `.memory-bank/requests/` - Durable user requests, clarification threads, and intent snapshots before promotion to specs, PRDs, Jira tickets, or work items.
  - `.memory-bank/requests/_index.md`
  - `.memory-bank/requests/_prompt.md`

## Implementation Plans
- `.memory-bank/implementation-plans/` - Project-level phase plans aligned to `.memory-bank/TODO.md`.
  - `.memory-bank/implementation-plans/_index.md`
  - `.memory-bank/implementation-plans/_prompt.md`

## Optional Subfolders (installed with --memory-bank-full)

These folders are part of the recommended "full" Memory Bank topology. They may be absent in minimal installs.

- `.memory-bank/agents/` - Agent profiles and reflections.
  - `.memory-bank/agents/_index.md`
  - `.memory-bank/agents/_prompt.md`
- `.memory-bank/best-practices/` - Curated best-practices notes (portable guidance).
  - `.memory-bank/best-practices/_index.md`
  - `.memory-bank/best-practices/_prompt.md`
- `.memory-bank/inbox/` - Agent-to-agent messages (immutable once sent).
  - `.memory-bank/inbox/_index.md`
  - `.memory-bank/inbox/_prompt.md`
- `.memory-bank/known-gaps/` - Known gaps scorecards and gap TODOs.
  - `.memory-bank/known-gaps/_index.md`
  - `.memory-bank/known-gaps/_prompt.md`
  - `.memory-bank/known-gaps/TODO.md`
- `.memory-bank/projects/` - Project-specific durable context.
  - `.memory-bank/projects/_index.md`
  - `.memory-bank/projects/_prompt.md`
- `.memory-bank/topics/` - Cross-project evergreen knowledge.
  - `.memory-bank/topics/_index.md`
  - `.memory-bank/topics/_prompt.md`

## Indexing
- `.memory-bank/_index.md` - Root inventory (this file).
- `.memory-bank/_prompt.md` - Global Memory Bank rules for agents.
- Additional `_index.md` files may exist in subfolders.
