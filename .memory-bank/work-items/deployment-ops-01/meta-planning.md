---
work_item_id: deployment-ops-01
status: not-started
repo: wojons/conscientiousness
created: 2026-05-03
updated: 2026-05-03
---

# Meta-Planning — Deployment, HITL, Webhooks, and Subagents

Mission: make Conscience operable after the core runtime exists. This track owns deployment modes, webhook/event ingestion, human interrupt state, and subagent orchestration boundaries.

axiom:trace work_item=deployment-ops-01 spec=specs/004-subagents.md,specs/009-deployment.md,specs/013-webhooks-and-events.md,specs/014-hitl-interrupt-state.md plan=phase-1/task-1/step-1 evidence=.memory-bank/work-items/deployment-ops-01/verification.md prompt=.memory-bank/work-items/_prompt.md

## Acceptance Criteria

### SPEC-009 — Deployment

1. **AC-DEP-01** — Binary builds produce a single statically-linked Go binary with embedded migration SQL files (`//go:embed migrations/*.sql`).
2. **AC-DEP-02** — Embedded schema migrations auto-apply on startup for both Postgres and SQLite backends; drift detection pauses all agents until migration completes.
3. **AC-DEP-03** — Configuration parity: a single `conscience.yaml` (or equivalent flags/env vars) drives both Postgres and SQLite modes identically.
4. **AC-DEP-04** — Horizontal scaling: multiple binary instances sharing one Postgres backend use `FOR UPDATE SKIP LOCKED` for exclusive task claiming without coordination.
5. **AC-DEP-05** — Deployment mode scripts/documentation exist for all six topologies: local SQLite, local Postgres, Supabase Cloud, self-hosted Supabase, production Postgres+VM, and horizontal scaling.
6. **AC-DEP-06** — CLI management commands (`conscience init`, `conscience migrate status/up/down`, `conscience session`, `conscience approve`, `conscience config`) function correctly.

### SPEC-013 — Webhooks & External Events

7. **AC-EVT-01** — Webhook endpoints verify HMAC-SHA256 signatures using secrets stored per `webhook_registrations` row; `signature_valid` column reflects verification outcome.
8. **AC-EVT-02** — Event idempotency enforced via `ON CONFLICT (source, source_id) WHERE source_id IS NOT NULL DO NOTHING`; duplicate deliveries are ignored.
9. **AC-EVT-03** — Rate limiting applied per source IP: max 60 requests/minute; excess requests receive 429.
10. **AC-EVT-04** — Payload size limits: 1 MB body max, 64 KB headers max; oversized payloads rejected with 413.
11. **AC-EVT-05** — Events with `signature_valid = false` are routed to `external_quarantine` with `validation_status = 'pending'`; processing requires Alt-Mode approval.
12. **AC-EVT-06** — `routing_rules` match events on `source_pattern`, `event_type_pattern`, and `payload_pattern` (JSONB path expression), ordered by `priority ASC`; highest-priority match wins.
13. **AC-EVT-07** — `external_events` table is the universal inbox with correct status lifecycle: `pending` → `routed` → `processing` → `completed` / `failed` / `quarantined`.
14. **AC-EVT-08** — `webhook_registrations` define accepted webhook sources, URL paths, target sessions/workflows, enabled/disabled state, and allowed `event_types`.

### SPEC-014 — HITL Interrupt State

15. **AC-HITL-01** — `approval_requests` rows created for all six `request_type` values: `tool_execution`, `destructive_action`, `budget_override`, `schema_change`, `sub_agent_spawn`, `custom`.
16. **AC-HITL-02** — `hitl_configuration` supports `scope = 'global'` and `scope = 'session'` with cascading precedence (session overrides global).
17. **AC-HITL-03** — No auto-approval rule: every `pending` approval requires explicit human action; `expires_at` causes automatic `expired` status — never `approved`.
18. **AC-HITL-04** — Reviewer authorization: only users with `alt_mode_role` can call `review_approval()`; identity checked via `current_setting('conscience.user_id')`.
19. **AC-HITL-05** — Approval expiry cron: expired `pending` approval requests set themselves to `expired`, and sessions paused solely on expired approvals are transitioned to `failed`.
20. **AC-HITL-06** — Notification channels fire on approval request creation: dashboard SSE, email, Slack webhook, and generic webhook; each logged in `notification_log`.

### SPEC-004 — Subagent Orchestration

21. **AC-SUB-01** — Memory forking clones only `display_modes.mode = 'compressed'` memory events from parent to child in a single atomic `INSERT … SELECT`; child memory is fully isolated post-fork.
22. **AC-SUB-02** — RLS isolation enforces `session_id = current_setting('conscience.session_id')` on `memory_events`, `tasks`, and tool access; sub-agents cannot read or modify other agents' data.
23. **AC-SUB-03** — `wake_parent_on_completion` trigger transitions parent from `waiting_sub` to `idle` when any child session reaches `completed` status.
24. **AC-SUB-04** — Error propagation: failed sub-agent sets task `status = 'failed'`; parent receives error via `result` column; parent chooses retry, replace, or escalate.
25. **AC-SUB-05** — Sub-agent depth limit enforced at 5 (default, configurable via `system_settings`); `spawn_subagent()` rejects spawns exceeding the limit.

### Scope Fences

- **In scope:** deployment topologies, configuration surfaces, embedded migrations, webhook endpoint security & routing, event lifecycle, HITL approval flow, subagent forking/isolation/wake/error/depth.
- **Out of scope:** production infrastructure provisioning (IaC), CI/CD pipeline implementation, monitoring/alerting dashboards, UI for approval workflow (Alt-Mode dashboard only consumes the DB state).
- **Non-goals:** modifying the cited specs themselves; implementing features not described in SPEC-004/009/013/014.

### Decision Points

| Decision | Options | Recommended | Rationale |
|---|---|---|---|
| How to handle SQLite triggers (no native trigger support) | Go hooks mirroring trigger logic | Go hooks | SPEC-009 parity matrix specifies Go hooks for SQLite |
| Webhook secret storage | Vault (Supabase Cloud), env vars, or config file | Config-driven with Vault opt-in | SPEC-009 §Supabase Cloud specifics supports Supabase Vault optionally |
| Depth limit configurability | Hard-coded 5 only, or `system_settings` override | `system_settings` override | SPEC-004 explicitly says "configurable via `system_settings`" |
