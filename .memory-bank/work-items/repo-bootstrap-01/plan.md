---
work_item_id: repo-bootstrap-01
status: complete
repo: wojons/conscientiousness
updated: 2026-05-03
---

# Plan — Repository + Architecture Bootstrap

Create only the skeleton needed for follow-on work: module file, directories, package stubs, and trace markers near architectural boundaries. Keep behavior minimal until schema and harness work items are ready.

axiom:trace work_item=repo-bootstrap-01 spec=specs/000-north-star.md,specs/001-architecture.md,specs/021-repository-layout.md,specs/022-library-research.md plan=phase-1/task-1/step-1 evidence=.memory-bank/work-items/repo-bootstrap-01/verification.md

## AC → Verification

| AC | Criterion | Verification Path | Status |
|---|---|---|---|
| AC-1 | Full SPEC-021 package skeleton | `go test ./...` + directory audit against SPEC-021 §2 tree | PASS |
| AC-2 | SPEC-001 architecture seam interfaces | `go vet ./internal/db/...` + interface review | PASS |
| AC-3 | SPEC-022 dependency decisions documented | Manual review: library-decisions file cites SPEC-022 rationale | PASS |
| AC-4 | North-star traceability (SPEC-000) | Trace note maps each principle to package/artifact | PASS |
| AC-5 | Baseline build and test | `go build ./... && go test ./...` with `CGO_ENABLED=0` | PASS |

## Phases

1. **Step 1** — Create full SPEC-021 package/directory skeleton (all 22 dirs + go.mod, Makefile, Dockerfile, consensus.yaml)
2. **Step 2** — Document SPEC-022 dependency research decisions with rationale for each chosen library
3. **Step 3** — Define SPEC-001 architecture seam interfaces (db.DB, db.Tx, dual-backend dispatch)
4. **Step 4** — Baseline build verification with CGO_ENABLED=0 and SPEC-000 north-star trace note
