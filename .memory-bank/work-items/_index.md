# Work Items Index

Inventory of work items under `.memory-bank/work-items/`.

Conventions
- Folder name is the `WORK_ITEM_ID` (Jira key when available, otherwise a stable id).
- Each work item folder typically contains:
  - `meta-planning.md`
  - `plan.md`
  - `plan.yaml`
  - `verification.md` (rolling)
  - `runs/<RUN_ID>/verification.md` (immutable snapshots)

Work items
- `bootstrap-output-stream-01` — Normalize first-admin-key output streams (completed 2026-05-28)
- `WI-001-remove-mock-llm` — Remove Mock LLM Fallback + Wire Real Clients (completed 2026-05-29, commits ec91bf4)
- `WI-002-migrate-pgx` — Migrate lib/pq → pgx/v5 (completed 2026-05-29, commit 2a119a2)
- `WI-003-json-schema` — JSON Schema enforcement for dynamic tables (completed 2026-05-29, commit ec19e93)
- `WI-004-quarantine-scanner` — Cognitive Firewall quarantine scanner (completed 2026-05-29, commits 1b3cfae, 95b5254)
- `WI-005-tool-sandbox` — External Tool Execution Sandbox (completed 2026-05-29, commit fd4bde9)
- `WI-006-three-tier-sql` — Three-Tier SQL Execution Model (completed 2026-05-29)
