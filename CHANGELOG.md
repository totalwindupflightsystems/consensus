# Changelog

All notable changes to this project are documented here.

---

## [0.1.0] — 2026-10-27

### Added

- **Tool registration on-ramp (DF-CONSENSUS-62)** — `POST /api/v1/tools`
  (admin key) and `consensus tool register <name>` create `tools_registry`
  rows, and migration `032_seed_builtin_tools.sql` ships two built-in tools on
  a fresh install (`list_sessions`, plus `demo_approval_gate` with
  `requires_approval=true`). The HITL approval gate was previously reachable
  only by writing SQL by hand: the registry shipped empty, the CLI `tool` verb
  was inspect-only, and no REST create route existed.

### Fixed

- **External tool execution could not enqueue (DF-CONSENSUS-62)** —
  `POST /api/v1/tools/{name}/execute` now records
  `tool_requests.iteration_id` (NOT NULL with no column default on both
  backends), so requests for `http_endpoint`/`subprocess` tools reach the
  executor instead of failing with `500 INTERNAL_ERROR: failed to enqueue
  tool`. The test fixture had a `DEFAULT 0` the production schema does not
  have, which is why the bug was invisible to the suite; the fixture now
  matches the production column.
- **Install frictions (DF-CONSENSUS-9)** — README documents the Go 1.26
  prerequisite with exact bare-Debian/Ubuntu tarball commands (and
  `docs/quickstart-cross-platform.md` no longer recommends the outdated
  `apt install golang-go`); a new "Bunker environment" section explains that
  rootless dockerd is already running and how to discover its socket
  (`DOCKER_HOST`, `/run/user/<uid>/docker.sock`) instead of deadlocking
  `systemctl --user start docker` against the held RootlessKit lock; the
  create-session model contract is documented as config-only runtime
  selection (`model_id` is stored/echoed but does not switch execution;
  there is no `model` field), pinned by tests.
- **Duplicate `user_message` rows (DF-CONSENSUS-9)** — an identical re-send
  to the same session within a 5-second window is dropped instead of
  double-stored (send racing the heartbeat-driven auto-resume, or a client
  retry without its `Idempotency-Key`). Wired into all three API-surface
  message paths (send, idempotent send, `Service.SendMessage`); duplicates
  return the same `200` shape as the original.
- **worktree.remove does not implement the declared input contract (DF-CONSENSUS-63)** —
  the request body field `directory` was silently ignored; the fix reads
  `WorktreeRemoveInput.directory` as the body-only target while retaining
  the query selector, and resolves remove/reset workspaces using the
  SPEC-017 fixed workspace precedence so servers launched outside a
  repository can operate on explicitly selected worktrees.

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
