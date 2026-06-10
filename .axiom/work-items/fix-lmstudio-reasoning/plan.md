# Fix LM Studio reasoning_content fallback and 400 error parsing

## Problem
The qwen/qwen3.5-9b model returns output in `reasoning_content` field, not `content`. 
The Go LLM client in `internal/llm/openai.go` has a reasoning_content fallback but it may
not be working correctly. Additionally, LM Studio returns 400 errors as plain strings
but the Go code expects a structured `openaiError` type.

Test evidence:
```
llm: calling provider url=http://127.0.0.1:1234/v1/chat/completions model=qwen/qwen3.5-9b messages=2
planning: LLM call failed turn=1 error="llm: parse response (status 400): json: cannot unmarshal string..."
```

## Steps
1. Check `internal/llm/openai.go` - find where the response is parsed
2. Add tolerance for string-format error responses from LM Studio
3. Verify reasoning_content fallback works by testing with curl
4. If reasoning_content fallback is broken, fix it
5. Test with: `go test -run TestRealLLMIntegration -count=1 -v -timeout 300s ./internal/harness/`

## Files
- `internal/llm/openai.go` — LLM client, error parsing, reasoning_content
- `internal/llm/client.go` — may also need changes
