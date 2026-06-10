# Memory Bank Rules

This file defines global invariants for the Memory Bank. All agents must follow these rules.

## Canonical Path
The Memory Bank lives at `.memory-bank/`. This is the only supported path.

## Non-Negotiables
- No secrets (API keys, tokens, credentials, PII). Redact as `[REDACTED]`.
- Prefer crisp, verifiable statements over narratives.
- Always link to canonical artifacts (Jira key, PR URL, run id, spec path).
- Update the Memory Bank immediately when state changes.

## Reading Strategy
- Start with `.memory-bank/_index.md` to discover what exists.
- Read only the files relevant to your current task.
- For work items, navigate to `.memory-bank/work-items/<WORK_ITEM_ID>/`.
- For durable user requests that are not yet executable, navigate to `.memory-bank/requests/`.

## Writing Rules
- When creating or updating a file, update the relevant `_index.md`.
- Include YAML frontmatter when creating work-item files (recommended).
- Separate facts from assumptions; label assumptions.
- Write so a different agent can take over immediately.

## Findings & Self-Improvement
- Write findings to `.memory-bank/findings/` — NOT into `AGENTS.md`.
- `AGENTS.md` only points to `.memory-bank/findings/_index.md`.
- When an adversarial agent surfaces a finding, write it to `findings/adversarial/`.
- Create subfolders when 3+ findings of the same type exist.

## Folder Structure
Subfolders are created as needed:
- `work-items/` - Per-ticket folders with plans, evidence, and run snapshots.
- `requests/` - Durable user asks and clarification threads before promotion to specs/PRDs/Jira/work-items.
- `implementation-plans/` - Phase-level plans aligned to `.memory-bank/TODO.md`.
- `agents/` - Agent-specific operating knowledge and reflections.
- `inbox/` - Agent-to-agent communication (immutable once sent).
- `projects/` - Durable project-specific context.
- `topics/` - Cross-project evergreen knowledge.
- `best-practices/` - Reusable engineering playbooks.
- `known-gaps/` - Repo quality evaluations and gap fix backlog.
- `findings/` - Agent-discovered findings, anti-patterns, adversarial results, self-improvement notes.

Each subfolder MUST contain its own `_index.md` (inventory) and `_prompt.md` (local rules) when it grows beyond a few files.
