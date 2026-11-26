# Verdict: DF-CONSENSUS-081

**Task:** Harden migration 034 Postgres coverage and reserved builtin policy
**Evaluated:** 2026-11-26T10:06:36.897353
**Result:** ✗ FAIL (DEGRADED — skipped checks)

## Pipeline Stages

- ✓ **tier1**
  -   ✓ secrets: ok (no output)
  ✓ lint: ok (no output)
  ✓ spec_parity: check-spec-parity: building scratch binary at commit e9dddf1ab7d7b50f68a4f78ad4172de39d403300
  ✓ tests: ?   	github.com/wojons/consensus	[no test files]
- ✗ **tier2**
  - INCOMPLETE
  ✗ Coverage FAILED — ungraded: PROOF: go test ./internal/migrate/ -count=1 exercises SQLite migration 034 coverage and the Postgres integration test is present and runs when CONSENSUS_TEST_POSTGRES_URL is configured; git grep -n "reserved builtin" docs/ finds the policy; FAILIF: migration 034 remains covered only by SQLite or the reserved builtin override policy is undocumented.
Evaluator error: LLM call failed: LLM request failed after 3 attempts (provider=openai model=glm-5.3-flash url=https://api.z.ai/api/coding/paas/v4/chat/completions key=<GITREINS_LLM_API_KEY>) — last error: 429 Client Error: Too Many Requests for url: https://api.z.ai/api/coding/paas/v4/chat/completions

## Summary

Stage tier1: PASS
    ✓ secrets: ok (no output)
  ✓ lint: ok (no output)
  ✓ spec_parity: check-spec-parity: building scratch binary at commit e9dddf1ab7d7b50f68a4f78ad4172de39d403300
  ✓ tests: ?   	github.com/wojons/consensus	[no test files]

Stage tier2: FAIL
  INCOMPLETE
  ✗ Coverage FAILED — ungraded: PROOF: go test ./internal/migrate/ -count=1 exercises SQLite migration 034 coverage and the Postgres integration test is present and runs when CONSENSUS_TEST_POSTGRES_URL is configured; git grep -n "reserved builtin" docs/ finds the policy; FAILIF: migration 034 remains covered only by SQLite or the reserved builtin override policy is undocumented.
Evaluator error: LLM call failed: LLM request failed after 3 attempts (provider=openai model=glm-5.3-flash url=https://api.z.ai/api/coding/paas/v4/chat/completions key=<GITREINS_LLM_API_KEY>) — last error: 429 Client Error: Too Many Requests for url: https://api.z.ai/api/coding/paas/v4/chat/completions

Overall: FAIL ✗
