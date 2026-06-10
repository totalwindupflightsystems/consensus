# Verification — real-llm-integration-tests-01

## Phase 1 Verification

### Commands Run
```bash
cd ~/conscientiousness

# Verify test compiles
go test -run TestRealLLMIntegration -count=1 -list '.*' ./internal/harness/ 2>&1

# Run with real LLM
go test -run TestRealLLMIntegration -count=1 -v ./internal/harness/ 2>&1

# Verify short skip
go test -short -run TestRealLLMIntegration ./internal/harness/ 2>&1
```

### Expected Results
- Test appears in test listing
- With LM Studio running: test passes, logs show real LLM response
- With -short: test skips cleanly
- No mock LLM involved — real HTTP to LM Studio

### Limitations
- Requires LM Studio to be running on :1234
- Requires `conscience` binary to be built (`go build -o conscience ./cmd/conscience`)
- Test uses SQLite (local), not Postgres

## Trace Refs
- axiom:trace work_item=real-llm-integration-tests-01 spec=specs/008-harness.md,specs/015-api-and-mcp.md plan=phase-1/task-1-1/step-1-1-1
