# real-llm-integration-tests-01: Real LLM Integration Tests

## Goal
Add integration tests that prove Consensus works end-to-end with a real LLM (LM Studio's qwen/qwen3.6-35b-a3b). Currently all 1,028 tests mock the LLM — the core consciousness loop (LLM → JSON → SQL → commit) has never been verified with a real model in the test suite.

## Plan

### Phase 1: Write the integration test

Create `internal/harness/real_llm_integration_test.go` with:

```go
func TestRealLLMIntegration(t *testing.T) {
    if testing.Short() {
        t.Skip("skipping real LLM integration test in short mode")
    }
    
    // 1. Compile and start consensus binary on random port
    // 2. Hit /health until ready
    // 3. Create session via POST /api/v1/sessions
    // 4. Send message via POST /api/v1/sessions/{id}/message  
    // 5. Wait for harness loop to process (poll session status)
    // 6. Verify memory events were inserted (GET /api/v1/sessions/{id}/memory)
    // 7. Verify response contains valid AgentOutput JSON
    // 8. Shutdown binary
}
```

### Phase 2: Verify with LM Studio

Run `go test -run TestRealLLMIntegration -count=1 -v ./internal/harness/` while LM Studio is running.

### Phase 3: Verify -short skip

Run `go test -short -run TestRealLLMIntegration ./internal/harness/` — should skip cleanly.

## Acceptance Criteria
- Test proves full stack works with real LLM (not mock)
- Binary starts, responds to health check, creates sessions, processes messages
- Memory events committed to SQLite after harness loop
- Test skips when LM Studio unreachable or -short flag set
- Zero changes to production code (test only)
