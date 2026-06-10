# Requests Index

Durable user request intake records live here before they become specs, PRDs, Jira tickets, or executable work items.

axiom:trace work_item=memory-bank-requests-01 spec=specs/08-Memory-Bank-Base-Prompt.md doc=.memory-bank/requests/_index.md

---

## What to Read First

1. [`_prompt.md`](./_prompt.md) — local request rules, lifecycle, and template.
2. Root [`../_prompt.md`](../_prompt.md) — global memory bank invariants.

## Purpose

Use this folder when a request should survive context compaction but is not yet ready for direct execution. Requests preserve what was asked, why it matters, what is still unclear, and where the request should be promoted next.

## Request Lifecycle Buckets

### Captured

No captured requests yet.

### Clarifying

No clarifying requests yet.

### Ready for Planning

No ready-for-planning requests yet.

### Promoted

No promoted requests yet.

### Deferred / Closed

No deferred or closed requests yet.

## Promotion Targets

- Work items: [`../work-items/`](../work-items/)
- PRDs: [`../prds/`](../prds/)
- Specs: [`../../specs/`](../../specs/)
- Captures: [`../captures/`](../captures/)
- Jira mapping: [`../jira-mapping.md`](../jira-mapping.md)

## Changelog

| Date | Change | Reason |
|---|---|---|
| 2026-04-30 | Created requests index | Add request intake as a first-class memory bank area |
