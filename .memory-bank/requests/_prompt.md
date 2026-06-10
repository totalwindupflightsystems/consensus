# Requests Folder Prompt

This folder stores durable user requests, clarification threads, and intent snapshots that are not yet ready to become specs, Jira tickets, work items, or PRDs.

**Parent**: `.memory-bank/_prompt.md` (root rules apply unless overridden here)

axiom:trace work_item=memory-bank-requests-01 spec=specs/08-Memory-Bank-Base-Prompt.md doc=.memory-bank/requests/_prompt.md

---

## Scope

Use `.memory-bank/requests/` for:

- User asks that should be remembered across sessions but are not yet an executable work item.
- Clarification transcripts and decision menus that may later seed specs, PRDs, Jira tickets, or work items.
- Lightweight request intake records from chat, Slack, email, Notion, Jira, or manual operator notes.
- Stable “why this was requested” context that should survive context compaction.

Do **not** use this folder for:

- Executable work plans or verification evidence; use `.memory-bank/work-items/<WORK_ITEM_ID>/`.
- Raw untriaged ideas or pasted notes; use `.memory-bank/captures/` first.
- Durable system behavior contracts; update `specs/`.
- Canonical PRDs; use `.memory-bank/prds/`.
- Inter-agent messages; use `.memory-bank/inbox/`.

## Lifecycle

Requests move through these states:

1. `captured` — request is recorded, but not yet normalized.
2. `clarifying` — open questions remain.
3. `ready-for-planning` — acceptance criteria and scope fence are clear enough to create a work item, Jira ticket, PRD, or spec stub.
4. `promoted` — request has been promoted; include links to the target artifact.
5. `deferred` — intentionally postponed; include trigger for revisiting.
6. `closed` — no further action needed; include rationale.

## Required Sections for Request Notes

Every request note MUST include:

1. YAML frontmatter with `mb.type: request`.
2. Summary — what was requested and why it matters.
3. Request Text — original or safely summarized request; redact secrets.
4. Source and Context — where it came from and relevant links.
5. Scope Fence — in scope, out of scope, and assumptions.
6. Acceptance Criteria Draft — testable bullets, or “not yet clear”.
7. Open Questions — max 7 active questions.
8. Promotion Plan — where this should go next and why.
9. Traceability — source refs, linked specs/tickets/work items, git context if available.

## Naming

Use stable, kebab-case filenames:

- `YYYY-MM-DD-short-request-title.md` for standalone requests.
- `<source-key>-short-title.md` when the source has a stable key.

## Changelog

| Date | Change | Reason |
|---|---|---|
| 2026-04-30 | Created requests folder prompt | Add durable request intake support to memory bank navigation |
