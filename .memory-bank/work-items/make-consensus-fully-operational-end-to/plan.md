# Plan: Make Consensus Fully Operational End-to-End

axiom:trace work_item=make-consensus-fully-operational-end-to spec=specs/000-north-star.md,specs/001-architecture.md,specs/005-security.md,specs/008-harness.md,specs/015-api-and-mcp.md,specs/021-repository-layout.md,specs/022-library-research.md plan=phase-0/task-0-1/step-0-1-1

**Work Item ID:** make-consensus-fully-operational-end-to
**Run ID:** 2026-05-28T07-51-20Z_4421b1
**Jira:** N/A — non-Jira work item

---

## Mission

Wire real LLM provider integration (OpenAI + Anthropic), align dependencies with specs, add trace markers to the MCP subsystem, update the file-inventory spec, and create baseline runbooks — so the Consensus platform runs end-to-end with a real LLM backend instead of a mock.

---

## AC to Verification Mapping

| AC ID | Source | Description | Verification Phase | Verifier |
|---|---|---|---|---|
| AC-01 | RT-001 | Real LLM client in production | Phase 2, Step 2-2-1 | @qa-axiom |
| AC-02 | RT-001 | OpenAI SDK with Structured Outputs | Phase 2, Step 2-2-2 | @qa-axiom |
| AC-03 | RT-001 | Anthropic SDK with prompt caching | Phase 2, Step 2-2-2 | @qa-axiom |
| AC-04 | RT-003 | go.mod aligned with SPEC-022 | Phase 1, Step 1-1-1 | @spec-verifier-axiom |
| AC-05 | RT-004 | MCP files have trace markers | Phase 3, Step 3-2-1 | @trace-auditor-axiom |
| AC-06 | RT-006 | SPEC-021 matches actual codebase | Phase 3, Step 3-1-1 | @specwriter-axiom |
| AC-07 | RT-007 | Deployment runbook | Phase 4, Step 4-1-1 | @docs-runbooks-axiom |
| AC-08 | RT-007 | Troubleshooting runbook | Phase 4, Step 4-1-1 | @docs-runbooks-axiom |
| AC-09 | E2E | Full cognition loop with real LLM | Phase 5, Step 5-1-1 | @qa-axiom |
| AC-10 | E2E | All existing tests pass | Phase 1-5 gates | @qa-axiom |

---

## Phases

### Phase 1: Dependency Alignment + Build Gate

**Goal:** Align go.mod with SPEC-022 runtime requirements and confirm build/CI stays green.

| Task | Steps |
|---|---|
| T1-1: Add missing deps | Step 1-1-1: Add LLM SDKs, chi, goose to go.mod; run `go mod tidy`; verify build + test + vet |
| T1-2: Verification gate | Step 1-2-1: Run full test suite; confirm no regressions |

### Phase 2: Real LLM Provider Integration

**Goal:** Replace mock LLM client with configurable real providers (OpenAI, Anthropic).

| Task | Steps |
|---|---|
| T2-1: LLM provider interface | Step 2-1-1: Define provider selection logic (env var + config file) |
| T2-2: OpenAI provider | Step 2-2-1: Implement OpenAI client wrapping go-openai SDK with Structured Outputs |
| T2-3: Anthropic provider | Step 2-3-1: Implement Anthropic client wrapping anthropic-sdk-go with prompt caching |
| T2-4: Wire into main.go | Step 2-4-1: Replace `llm.NewMockClient()` with provider-aware factory; wire env vars |
| T2-5: Verification gate | Step 2-5-1: Run tests; gate live tests behind `CONSENSUS_LIVE_LLM_TEST` |

### Phase 3: Trace Markers + Spec Alignment

**Goal:** Close RT-004 (MCP trace markers) and RT-006 (SPEC-021 file inventory).

| Task | Steps |
|---|---|
| T3-1: Update SPEC-021 | Step 3-1-1: Run `find` on actual codebase; diff against SPEC-021; update spec |
| T3-2: Add MCP trace markers | Step 3-2-1: Add `axiom:trace` lines to all 4 MCP source files |

### Phase 4: Operational Documentation

**Goal:** Create baseline deployment and troubleshooting runbooks (RT-007).

| Task | Steps |
|---|---|
| T4-1: Create runbooks | Step 4-1-1: Write `docs/runbooks/deployment.md` and `docs/runbooks/troubleshooting.md` |

### Phase 5: End-to-End Verification

**Goal:** Prove the platform runs end-to-end with a real LLM.

| Task | Steps |
|---|---|
| T5-1: Live LLM E2E test | Step 5-1-1: Run `CONSENSUS_LLM_PROVIDER=openai consensus serve`; confirm full cognition loop completes |
| T5-2: Final verification gate | Step 5-2-1: Run full test suite; confirm all 263+ tests pass; verify build + vet |

---

## Expected Touched Areas (Modules/Files)

| Phase | Files/Directories |
|---|---|
| Phase 1 | `go.mod`, `go.sum` |
| Phase 2 | `internal/llm/openai.go` (new), `internal/llm/anthropic.go` (new), `internal/llm/client.go` (modify), `cmd/consensus/main.go` (modify) |
| Phase 3 | `specs/021-repository-layout.md` (modify), `internal/mcp/*.go` (4 files — modify) |
| Phase 4 | `docs/runbooks/deployment.md` (new), `docs/runbooks/troubleshooting.md` (new) |
| Phase 5 | No new files — verification only |

---

## Commit / PR Template

```
feat: wire real LLM providers and make platform fully operational

- Add OpenAI and Anthropic provider implementations
- Align go.mod dependencies with SPEC-022
- Add trace markers to MCP subsystem
- Update SPEC-021 file inventory to match actual codebase
- Create baseline deployment and troubleshooting runbooks

axiom:trace work_item=make-consensus-fully-operational-end-to spec=specs/001-architecture.md,specs/005-security.md,specs/008-harness.md,specs/015-api-and-mcp.md,specs/021-repository-layout.md,specs/022-library-research.md plan=phase-1-through-5 evidence=verification.md

Closes: RT-001, RT-003, RT-004, RT-006, RT-007
```

---

## Rollback Plan

1. Revert go.mod changes: `git checkout HEAD -- go.mod go.sum`
2. Revert `main.go` LLM init: restore `llm.NewMockClient()`
3. Set `CONSENSUS_LLM_PROVIDER=mock` to fallback to existing mock
4. All structural changes are additive — no harness loop modification required

---

**Plan complete. Machine-readable version at `plan.yaml`.**
