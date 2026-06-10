# WI-001: Remove Mock LLM Fallback + Wire Real Clients — Verification

## Phase 1: Fail Fast + Mock Opt-In ✅

### Phase 1 Task 1: Remove mock fallback in main.go
- Changed `cmd/conscience/main.go:111-115` from Warn+fallback to `fmt.Fprintf(os.Stderr, ...)` + `os.Exit(1)`
- **Evidence**: `grep NewMockClient cmd/conscience/main.go` → no longer used as fallback
- `go build ./...` ✅

### Phase 1 Task 2: Add CONSCIENCE_MOCK_LLM env var in NewClient factory
- Added `os` import to `internal/llm/client.go`
- Added `CONSCIENCE_MOCK_LLM=1` check in `NewClient()` — returns error if env var not set
- Added `TestNewClient_MockRejectsWithoutEnv` test
- Updated `TestNewClient_Mock` to set env var
- `go test ./internal/llm/...` ✅

## Phase 2: Anthropic HTTP Client ✅

### Phase 2 Task 1: Real Anthropic client
- Created `internal/llm/anthropic_client.go` with real HTTP implementation
- Implements Anthropic Messages API (POST /v1/messages)
- Properly separates system messages into `system` parameter vs conversation `messages`
- Adds `cache_control: {type: "ephemeral"}` breakpoints on last system block when cache enabled
- Maps Anthropic usage fields to `harness.LLMUsage` including cache tokens
- Compile-time interface check: `var _ harness.LLMClient = (*anthropicClient)(nil)`
- Added `TestAnthropicClient_CreatedWithoutPanic` and `TestAnthropicClientSatisfiesInterface`

### Phase 2 Task 2: Remove stub
- Removed `stubClient` type and old `NewAnthropicClient` stub from `client.go`
- `go build ./...` ✅
- `go test ./...` ✅

## Phase 3: OpenAI Structured Outputs ✅

### Phase 3 Task 1: json_schema with strict:true
- Added `ResponseFormat` type and field to `llm.Config` (`json_object` | `json_schema`)
- Added `agentOutputJSONSchema()` — full JSON schema for `AgentOutput` with `additionalProperties: false`
- Updated `openaiClient.Call()` to use `json_schema` with `strict: true` when configured
- Default remains `json_object` for backward compatibility
- `go build ./...` ✅
- `go test ./...` ✅
- `go vet ./...` ✅

## Summary
- **5 tasks completed across 3 phases**
- **17 LLM tests passing**, all 22+ packages clean
- Ready to commit

axiom:trace work_item=WI-001-remove-mock-llm spec=specs/007-json-schema.md,specs/012-prompt-engineering.md,specs/022-library-research.md plan=phases-1-2-3 evidence=verification.md
