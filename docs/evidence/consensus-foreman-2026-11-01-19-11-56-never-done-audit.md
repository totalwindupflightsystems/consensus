# NEVER-DONE audit evidence

Tested commit: `54cd2c013c001bca47f35b8641a255ad42bab7eb`

## Acceptance command 1

```text
COMMAND: go test ./... -count=1 -short
?   	github.com/wojons/consensus	[no test files]
?   	github.com/wojons/consensus/chronicle	[no test files]
ok  	github.com/wojons/consensus/cmd/consensus	0.017s
ok  	github.com/wojons/consensus/demo	5.700s
ok  	github.com/wojons/consensus/internal/api	2.107s
ok  	github.com/wojons/consensus/internal/billing	0.031s
ok  	github.com/wojons/consensus/internal/bootstrap	0.508s
ok  	github.com/wojons/consensus/internal/chronicle	0.461s
ok  	github.com/wojons/consensus/internal/cli	0.171s
ok  	github.com/wojons/consensus/internal/compression	0.087s
ok  	github.com/wojons/consensus/internal/config	0.546s
ok  	github.com/wojons/consensus/internal/crier	0.075s
ok  	github.com/wojons/consensus/internal/db	0.037s
ok  	github.com/wojons/consensus/internal/db/driver	0.154s
ok  	github.com/wojons/consensus/internal/db/dynamic	0.156s
ok  	github.com/wojons/consensus/internal/db/jsonschema	0.027s
ok  	github.com/wojons/consensus/internal/db/postgres	0.040s
ok  	github.com/wojons/consensus/internal/db/sqlite	0.095s
ok  	github.com/wojons/consensus/internal/harness	48.275s
ok  	github.com/wojons/consensus/internal/hitl	0.048s
ok  	github.com/wojons/consensus/internal/llm	0.292s
ok  	github.com/wojons/consensus/internal/llmcompat	0.019s
ok  	github.com/wojons/consensus/internal/mcp	0.219s
ok  	github.com/wojons/consensus/internal/memory	0.033s
ok  	github.com/wojons/consensus/internal/migrate	0.834s
ok  	github.com/wojons/consensus/internal/modelsync	0.134s
ok  	github.com/wojons/consensus/internal/quarantine	0.170s
ok  	github.com/wojons/consensus/internal/recoverydiag	0.177s
ok  	github.com/wojons/consensus/internal/secrets	0.045s
ok  	github.com/wojons/consensus/internal/security	0.059s
ok  	github.com/wojons/consensus/internal/session	0.084s
ok  	github.com/wojons/consensus/internal/shim/h3	0.052s
ok  	github.com/wojons/consensus/internal/shim/opencode	41.707s
ok  	github.com/wojons/consensus/internal/subagent	0.025s
ok  	github.com/wojons/consensus/internal/tools	0.241s
ok  	github.com/wojons/consensus/internal/web	0.009s
ok  	github.com/wojons/consensus/internal/webhook	0.144s
ok  	github.com/wojons/consensus/pkg/client	0.013s
ok  	github.com/wojons/consensus/scripts/loadprobe	0.006s
?   	github.com/wojons/consensus/specs	[no test files]
EXIT_CODE: 0
```

## Acceptance command 2

```text
COMMAND: go vet ./...
EXIT_CODE: 0
```

## Acceptance command 3

```text
COMMAND: grep -r 'func TestShimRealLLMSessionLifecycle' --include='*_test.go' .
./internal/chronicle/shim_real_llm_test.go:func TestShimRealLLMSessionLifecycle(t *testing.T) {
EXIT_CODE: 0
```

STATUS: PASS
TIER: T3 (direct acceptance commands invoked and observed)
EVIDENCE: docs/evidence/consensus-foreman-2026-11-01-19-11-56-never-done-audit.md

ch:trace row=NEVER-DONE spec=none:audit-fixture wave=none:serial test=internal/chronicle/shim_real_llm_test.go::TestShimRealLLMSessionLifecycle doc=docs/evidence/consensus-foreman-2026-11-01-19-11-56-never-done-audit.md evidence=docs/evidence/consensus-foreman-2026-11-01-19-11-56-never-done-audit.md witness=path@origin:internal/chronicle/shim_real_llm_test.go verdict=none:pending-gitreins-judge commit=none:pending-merge memory=none:pending-readback
