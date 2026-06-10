# WI-001: Remove Mock LLM Fallback + Wire Real Clients — Plan

**Mission**: Stop the production binary from silently succeeding with fake LLM outputs. Fail fast when LLM client init fails. Ship a working Anthropic client. Enforce structured outputs with `json_schema`. Enable mock mode via explicit opt-in.

## Acceptance Criteria → Verification

| AC | How to Verify |
|----|---------------|
| AC1: No mock fallback | Inspect `main.go` lines 111-114 — error must be fatal |
| AC2: Real Anthropic client | `go build ./...`, inspect `internal/llm/anthropic_client.go` |
| AC3: OpenAI `json_schema` strict mode | Inspect request payload in `openai_client.go` |
| AC4: `CONSCIENCE_MOCK_LLM=1` opt-in | Run tests that set env var and check mock returned |
| AC5: Anthropic cache_control | Inspect request payload for `cache_control` on system messages |
| AC6: All tests pass | `go test ./...` |

## Phases

### Phase 1: Fail Fast + Mock Opt-In
1. Remove mock fallback in `main.go` — return error instead
2. Add `CONSCIENCE_MOCK_LLM=1` env var check in `NewClient()` factory
3. Update tests that depend on mock behavior

### Phase 2: Anthropic HTTP Client
1. Create `internal/llm/anthropic_client.go` — direct HTTP implementation
2. Implement Messages API request/response types
3. Wire `cache_control` breakpoints on system messages

### Phase 3: OpenAI Structured Outputs
1. Add `ResponseFormat` config field (`json_object` | `json_schema`)
2. Wire `json_schema` with `strict: true` when configured

### Phase 4: Verify & Commit
1. `go build ./...` passes
2. `go test ./...` passes
3. Commit with conventional commit + co-author

## Rollback
- Each phase is independently revertible
- Phase 2 (Anthropic) doesn't affect OpenAI users
- Phase 3 (json_schema) is backward-compatible with existing users

axiom:trace work_item=WI-001-remove-mock-llm spec=specs/007-json-schema.md,specs/012-prompt-engineering.md plan=plan.md
