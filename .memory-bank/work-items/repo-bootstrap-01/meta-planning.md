---
work_item_id: repo-bootstrap-01
status: not-started
repo: wojons/conscientiousness
created: 2026-05-03
updated: 2026-05-03
---

# Meta-Planning — Repository + Architecture Bootstrap

Mission: create the implementation skeleton that lets future work land in the right place. This work item owns the Go module, directory layout, architectural seams, and library decisions.

axiom:trace work_item=repo-bootstrap-01 spec=specs/000-north-star.md,specs/001-architecture.md,specs/021-repository-layout.md,specs/022-library-research.md plan=phase-1/task-1/step-1 evidence=.memory-bank/work-items/repo-bootstrap-01/verification.md prompt=.memory-bank/work-items/_prompt.md

## Scope

In scope: Go module scaffold, package layout, placeholder interfaces, library decision notes.
Out of scope: full schema, harness, API, CLI implementation.

## Acceptance Criteria

### AC-1: Full SPEC-021 package skeleton (SPEC-021)
The repo contains a Go module and the complete directory/package structure from SPEC-021 §2, including:
- `cmd/consensus/main.go` (binary entry point)
- `internal/config/`, `internal/db/` (with `postgres/` and `sqlite/` sub-packages), `internal/harness/`, `internal/api/`, `internal/mcp/`
- `internal/cli/`, `internal/security/`, `internal/secrets/`, `internal/memory/`, `internal/session/`, `internal/tools/`, `internal/billing/`, `internal/llm/`, `internal/webhook/`, `internal/hitl/`, `internal/subagent/`, `internal/shim/`, `internal/migrate/`
- `migrations/`, `Makefile`, `Dockerfile`, `consensus.yaml`

### AC-2: SPEC-001 architecture seam interfaces (SPEC-001)
- `internal/db/db.go` defines the `DB` and `Tx` interfaces per SPEC-021 §3.2
- `internal/db/postgres/` contains a pgx-based driver stub with `SET LOCAL` session context
- `internal/db/sqlite/` contains a `modernc.org/sqlite` driver stub
- Architecture note documents the single-Go-binary, two-backend dispatch via `--db` flag (from SPEC-001 §2.3)

### AC-3: SPEC-022 dependency decisions documented (SPEC-022)
- Every external dependency choice is recorded with SPEC-022 §2-§8 rationale: pgx/v5, modernc.org/sqlite (and why not mattn/go-sqlite3), openai-go/v3, anthropic-sdk-go, mcp-go, chi/v5 (and why not gin/echo), cobra, go.yaml.in/yaml/v3 (and why not Viper), goose/v3, stdlib SSE (and why no library)
- `CGO_ENABLED=0` purity constraint is documented

### AC-4: North-star traceability (SPEC-000)
- Plan confirms layout aligns with SPEC-000 core principles: Database-as-Runtime, Atomic Cognition, Write Once/Deploy Anywhere, Agent as Microservice, Unbypassable Constraints
- A trace note maps each principle to its corresponding package/artifact

### AC-5: Baseline build and test
- `go build ./...` passes with `CGO_ENABLED=0`
- `go test ./...` baseline is recorded
