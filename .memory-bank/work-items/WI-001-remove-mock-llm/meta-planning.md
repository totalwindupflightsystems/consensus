# WI-001: Remove Mock LLM Fallback + Wire Real Clients — Meta-Planning

**Operator Brief**: The production binary silently falls back to a mock LLM client when the real client fails to initialize. This means in production without real API keys, the system appears to run but produces deterministic "mock reasoning" outputs — a dangerous failure mode. Additionally, the Anthropic provider is a stub that always errors. We need to fail fast, implement real Anthropic HTTP client, add structured output enforcement via `json_schema`, and wire prompt caching.

**Specs**: 007 (JSON Schema), 012 (Prompt Engineering), 022 (Library Research)

**Gaps**: CS-RT-001, CS-GAP-007, CS-GAP-011, CS-GAP-016

**Verification Bar**: Standard (production-blocker fix)

---

## Scope Fences

**In scope**:
- Remove mock fallback at `cmd/conscience/main.go:112-114` — fail fast with error
- Implement real Anthropic HTTP client (direct HTTP, like `openai_client.go`)
- Wire `response_format: {type: "json_schema", json_schema: {...}, strict: true}` for OpenAI
- Add `CONSCIENCE_MOCK_LLM=1` env var opt-in for dev/testing
- Add Anthropic `cache_control` breakpoints on static prompt layers
- Update `internal/llm/client.go` factory to support mock-via-env

**Out of scope**:
- Anthropic SDK integration (direct HTTP is preferred for control)
- Third-party provider SDKs beyond OpenAI/Anthropic
- Prompt caching for OpenAI (OpenAI handles this server-side)
- `model_registry` table updates

---

## Acceptance Criteria

| # | Criterion | Verification Path |
|---|-----------|-------------------|
| AC1 | `main.go` does NOT fall back to mock when `NewClient` fails — it returns error and exits | `go build ./...` + inspect `main.go` lines 111-114 |
| AC2 | Real Anthropic client exists, implements `harness.LLMClient`, makes HTTP requests | `go build ./...`, inspect `internal/llm/anthropic_client.go` |
| AC3 | OpenAI client sends `response_format` with `type: "json_schema"` and `strict: true` when configured | Inspect request JSON in `openai_client.go` |
| AC4 | `CONSCIENCE_MOCK_LLM=1` env var causes factory to return `MockClient` | Unit test in `client_test.go` |
| AC5 | Anthropic client sends `cache_control` breakpoints on system messages | Inspect request JSON in `anthropic_client.go` |
| AC6 | All tests pass | `go test ./...` |

---

## Decision Points

| Decision | Options | Chosen | Rationale |
|----------|---------|--------|-----------|
| Anthropic SDK vs direct HTTP | a) `anthropic-sdk-go`, b) direct HTTP | **Direct HTTP** | SDK not in go.mod; direct HTTP follows established `openai_client.go` pattern; more control |
| `json_schema` vs `json_object` | a) `json_object` (current), b) `json_schema` strict | **Both** | Keep `json_object` for backward compat, add `json_schema` mode via config flag |
| Mock opt-in mechanism | a) env var only, b) config file, c) both | **Env var** | Simple, test-friendly, matches existing pattern (`CONSCIENCE_API_KEY`) |

---

## Assumptions

| # | Statement | How to Verify | Impact if Wrong |
|---|-----------|---------------|-----------------|
| A1 | Anthropic Messages API is OpenAI-compatible-ish (different endpoint, schema) | Check Anthropic API docs | May need different request/response shapes |
| A2 | `CONSCIENCE_MOCK_LLM=1` is sufficient opt-in for dev mode | Env var pattern used elsewhere in codebase | May also need config file support later |

---

## Open Questions

None — scope is well-defined by existing gaps.

---

axiom:trace work_item=WI-001-remove-mock-llm spec=specs/007-json-schema.md,specs/012-prompt-engineering.md,specs/022-library-research.md plan=meta-planning.md
