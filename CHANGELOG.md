# Changelog

All notable changes to this project are documented here.

---

## [0.1.0] — 2026-09-29

First release of Consensus: a database-native cognitive architecture for AI
agents. One Go binary (`cmd/consensus`) carrying the full runtime against
either SQLite or PostgreSQL.

### Added

- **Runtime binary** — the harness execution loop with heartbeat task
  polling, wake signaling, and graceful SIGTERM/SIGINT drain; serves REST
  API, MCP, shims, and the dashboards from a single process.
- **REST API** — `/api/v1/*` (chi router) for sessions, approvals, memory,
  models, and health; SSE event streams; declared OpenAPI contract with a
  served-spec parity test; per-role rate limits.
- **MCP server** — tools, resources, and prompts over JSON-RPC, via SSE
  (`/mcp/sse`) or stdio (`consensus mcp-stdio`) transports.
- **CLI** — cobra-based management commands: serve, init, migrate
  (up/down/status), session, approve/reject, config, models, status,
  memory, tool, skill, completion, version; table/JSON/YAML output formats;
  identity verification against the target server before remote calls.
- **Protocol shims** — opencode server-protocol adapter (session/config/
  event translation onto the native API) and the H3 protocol shim
  (health/process/result/sessions/cancel) so existing agent frontends can
  drive Consensus unmodified.
- **Dual database backends** — SQLite and PostgreSQL behind one driver
  interface; Postgres uses LISTEN/NOTIFY for real-time events, SQLite uses
  a polling bridge; auto-migrations on startup with drift detection.
- **Bootstrap & auth** — first-run bootstrap admin key printed to stdout
  with a 90-day default TTL; agent-role scoping with row-level security;
  admin DB bypass path for migrations and bootstrap.
- **Cognitive firewall (quarantine)** — tool-output scanning with
  quarantine insertion and quarantine events surfaced on the event bus.
- **Webhooks** — external event ingestion at `/webhooks/*` with HMAC
  signature verification (no API key on the ingestion path) and a
  Go-level routing loop that matches rules and wakes target sessions.
- **Human-in-the-loop (HITL)** — approval requests that pause sessions,
  with an expiry cron for stale pending approvals.
- **Dashboards** — web admin UI at `/ui/` and the Chronicle investigation
  workbench at `/chronicle/`, both served by the same binary.
- **Release plumbing** — tag-triggered GitHub Actions release workflow
  (GoReleaser on v* tags: build matrix, archives, checksums, release
  assets); goreleaser build stamps (version/commit/date) observable via
  `consensus --version` and `consensus version`.

---
