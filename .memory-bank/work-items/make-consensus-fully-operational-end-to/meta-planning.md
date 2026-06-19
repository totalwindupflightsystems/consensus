# Meta-Planning: Make Consensus Fully Operational End-to-End

axiom:trace work_item=make-consensus-fully-operational-end-to spec=specs/000-north-star.md,specs/001-architecture.md,specs/005-security.md,specs/022-library-research.md plan=phase-0/task-0-1/step-0-1-1

**Run ID:** 2026-05-28T07-51-20Z_4421b1
**Repo:** wojons/conscientiousness
**Status:** Planning Complete

---

## 1) Intent

### What changes and why?

The Consensus platform has 23 Go packages that all build and test clean, but the platform is not *operationally real*. The production entry point (`cmd/consensus/main.go`) calls `llm.NewMockClient()` — there is no real LLM integration. The go.mod lacks key dependencies specified in SPEC-022. The MCP subsystem has zero trace markers. Critical runbooks and operational documentation are absent.

This work item wires real LLM provider integration (OpenAI, Anthropic), aligns go.mod with SPEC-022 where practical, adds trace markers to the MCP subsystem, and creates baseline operational documentation — producing a platform that can actually run end-to-end with a real LLM backend.

### In scope

- **RT-001** (Critical): Replace mock LLM client with real OpenAI and Anthropic SDKs
- **RT-003** (Critical): Add missing SPEC-022 runtime dependencies to go.mod
- **RT-004** (Major): Add trace markers to all MCP subsystem files (4 source files)
- **RT-006** (Major): Update SPEC-021 repository layout to match actual codebase
- **RT-007** (Major): Create baseline runbooks (deployment, troubleshooting, health-check)
- End-to-end verification: run `consensus serve` with a real LLM backend and confirm full loop

### Out of scope

- Real LLM integration in subagent spawn paths (uses existing mock fallback — deferred)
- Postgres-specific features (SET LOCAL, FOR UPDATE SKIP LOCKED) — already tracked separately
- OpenAPI contract drift fixes (noting for separate work)
- Alerting infrastructure (noting for follow-on)
- Ops dashboard creation (noting for follow-on)
- Full adversarial re-audit after changes (recommended follow-on)

## 2) Contract Reconciliation

### Jira ticket reference

N/A — non-Jira work item.

### Acceptance criteria summary (derived from red team audit RT-001 through RT-007)

| AC ID | Source | Description | Priority |
|---|---|---|---|
| AC-01 | RT-001 | Production `consensus serve` uses real LLM client (not mock). Configurable via env vars or config file. | Critical |
| AC-02 | RT-001 | OpenAI SDK integrated with Structured Outputs support for JSON response format. | Critical |
| AC-03 | RT-001 | Anthropic SDK integrated with prompt caching support. | Critical |
| AC-04 | RT-003 | go.mod includes all dependencies listed in SPEC-022 §2-11 that are used at runtime. | Critical |
| AC-05 | RT-004 | All 4 MCP source files contain `axiom:trace` markers referencing their spec and work item. | Major |
| AC-06 | RT-006 | SPEC-021 file inventory updated to reflect actual codebase (remove phantom files, add undocumented files). | Major |
| AC-07 | RT-007 | Baseline deployment runbook exists in `docs/runbooks/deployment.md`. | Major |
| AC-08 | RT-007 | Baseline troubleshooting runbook exists in `docs/runbooks/troubleshooting.md`. | Major |
| AC-09 | E2E | `consensus serve` starts, connects to a real LLM, and completes at least one full cognition loop. | Critical |
| AC-10 | E2E | All existing tests continue to pass after LLM wiring changes. | Critical |

### Contract/specs touched

- `specs/005-security.md` — LLM provider configuration, API key management
- `specs/008-harness.md` — LLM client interface, cognition loop
- `specs/021-repository-layout.md` — file inventory update
- `specs/022-library-research.md` — dependency alignment

### Conflicts discovered and resolution

| Conflict | Resolution |
|---|---|
| go.mod uses `lib/pq` but SPEC-022 recommends `pgx/v5` | Keep `lib/pq` (documented design choice per activeContext.md). Do not add pgx/v5. SPEC-022 §2.1 acknowledges both options. |
| SPEC-022 lists 10 recommended deps; only 5 are in go.mod | Add the 5 missing that are used at runtime (LLM SDKs, chi, goose). Skip `pgx/v5` (uses lib/pq), skip `schemathesis` (CI tool, not runtime), skip `golang-migrate` (custom migration runner in use). |
| MCP trace markers absent in 4 files | Add markers referencing `specs/015-api-and-mcp.md` |

### Related tickets

N/A — no Jira integration configured.

## 3) Decision Points

| Decision | Default chosen | What changes with other choices |
|---|---|---|
| OpenAI vs Anthropic as default provider | Both supported; default to OpenAI via `CONSENSUS_LLM_PROVIDER=openai` | Switch to anthropic via env var |
| Whether to add pgx/v5 alongside lib/pq | No; keep lib/pq only | Would increase binary size, add migration risk |
| API key storage mechanism | Environment variables (`OPENAI_API_KEY`, `ANTHROPIC_API_KEY`) | Could add vault/ESO integration later |
| Structured Outputs enforcement | Enforce via OpenAI `response_format` param. Anthropic: use system prompt instructions + JSON validation in harness | Could add stronger enforcement later |
| Mock LLM client preservation | Keep MockClient for testing; real client selected at startup via provider config | N/A |

## 4) Risks and Blast Radius

| Risk | Severity | Mitigation |
|---|---|---|
| API keys in logs or error messages | High | Add `[REDACTED]` to LLM error paths; never log full response bodies |
| Breaking existing 263+ harness tests | Medium | MockClient preserved; existing tests use mock. Only new tests use real client. |
| LLM latency breaking timeout assumptions | Medium | Add configurable timeout to real LLM client; default 120s |
| Anthropic SDK version compatibility | Low | Pin to latest stable; test before final commit |
| Runtime cost for real LLM calls in tests | Low | Real LLM tests gated behind `CONSENSUS_LIVE_LLM_TEST=1` env var |

### Rollback/containment

- Revert go.mod changes to restore pure-mock mode
- `CONSENSUS_LLM_PROVIDER=mock` falls back to existing MockClient
- All real-LLM code behind provider interface — no structural changes to harness loop

## 5) Verification Design

### Evidence that proves done

| Evidence | Format | Location |
|---|---|---|
| `go build ./...` passes with new dependencies | Build log | `verification.md` |
| `go test ./...` all 263+ tests pass | Test output | `verification.md` |
| `go vet ./...` clean | Vet output | `verification.md` |
| OpenAI live test: `CONSENSUS_LLM_PROVIDER=openai consensus serve` completes loop | Console log | `verification.md` |
| Anthropic live test: `CONSENSUS_LLM_PROVIDER=anthropic consensus serve` completes loop | Console log | `verification.md` |
| MCP files contain `axiom:trace` markers | grep output | `verification.md` |
| SPEC-021 matches `find` output for actual codebase | diff | `verification.md` |
| Runbooks exist and pass markdown lint | File check | `verification.md` |

### Required vs optional checks

| Check | Required | Notes |
|---|---|---|
| Build + test + vet | Yes | Gate 1 |
| OpenAI live test | Yes | Requires API key in env |
| Anthropic live test | No | Optional; requires API key |
| Trace marker grep | Yes | |
| SPEC-021 diff | Yes | |
| Runbooks exist | Yes | |

### Which verifiers must run

- `@dev-axiom` — implements LLM wiring, deps, trace markers, runbooks
- `@qa-axiom` — runs test suite, verifies build
- `@spec-verifier-axiom` — verifies SPEC-021 alignment, trace marker presence
- `@specwriter-axiom` — updates SPEC-021 file inventory
- `@trace-auditor-axiom` — validates trace graph completeness

#### 5a) Test Value Assessment

1. **Spec-to-test mapping**: Existing 263+ harness/integration tests verify cognition loop, transaction safety, error recovery. New LLM tests verify provider interface implementation.
2. **Real code path**: Real LLM client calls actual OpenAI/Anthropic APIs behind env-var gate.
3. **Deletion test**: If MockClient tests are deleted, harness loop verification breaks. If real LLM tests are deleted, nothing breaks (gated).
4. **Gap check**: No existing tests for LLM provider selection logic or API key validation.
5. **Anti-pattern check**: MockClient satisfies LLMClient interface — no fake executor bypass. Real LLM tests use real API calls (not raw HTTP).

## 6) Ambiguity Assessment

### Assumptions the plan depends on

| # | Assumption | How to verify | Impact if wrong |
|---|---|---|---|
| A1 | OpenAI Go SDK (`github.com/sashabaranov/go-openai`) is compatible with Go 1.25 | Run `go get` and build | May need alternative SDK or REST calls |
| A2 | Anthropic Go SDK exists and supports prompt caching | Run `go get github.com/anthropics/anthropic-sdk-go` | May need REST client implementation |
| A3 | `lib/pq` is acceptable alternative to `pgx/v5` (per activeContext.md decision) | Confirm with project owner | Would require larger go.mod overhaul |
| A4 | User has API keys available for testing | Attempt live test | Live tests skipped; verification deferred |
| A5 | `go get` will resolve dependency tree cleanly | Run `go get ./... && go mod tidy` | Manual conflict resolution needed |

### Ambiguity rating

**Medium.** The technical path is clear (wire a real LLM client, add deps, add trace markers, update docs). The main unknowns are (1) whether specific Go SDK versions are compatible and (2) whether API keys are available for testing. The plan handles both with progressive disclosure (build first, test with mock, then gate live tests).

### Missing inputs + compensations

| Input | Compensation |
|---|---|
| Preferred LLM provider | Support both; default to OpenAI |
| API key availability | Gate live tests behind env var |
| SPEC-022 §11 LLM SDK decisions (not yet written) | Make pragmatic choices (go-openai, anthropic-sdk-go) |

## 7) References

- Red Team Audit findings: `.memory-bank/activeContext.md` (RT-001 through RT-007)
- Architecture: `specs/001-architecture.md`
- Security: `specs/005-security.md`
- Library Research: `specs/022-library-research.md`
- Repository Layout: `specs/021-repository-layout.md`
- Harness Loop: `specs/008-harness.md`
- API & MCP: `specs/015-api-and-mcp.md`

---

**Meta-planning complete. Plan follows in `plan.md` and `plan.yaml`.**
