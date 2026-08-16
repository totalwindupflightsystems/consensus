# Verdict: dogfood-103

**Task:** OpenAPI contract served from embedded spec — /openapi.json 200 from any CWD + /doc shadowing fix
**Evaluated:** 2026-08-16T09:43:34.384248
**Result:** ✗ FAIL

## Pipeline Stages

- ✗ **tier1**
  -   ✓ secrets: 
  ✓ lint: 
  ✗ tests: Command timed out
- ✓ **tier2**
  - COMPLETE
  ✓ openapi.json and openapi.yaml return 200 from ANY CWD (embedded spec fallback); docker run image serves /openapi.json 200 without copying specs/: internal/api/openapi.go loadSpec() falls back to specs.BundledYAML when no on-disk file found; specs/embed.go go:embed openapi/bundled.yaml; Dockerfile copies only /bin/consensus (no specs/ dir); TestOpenAPISpecServedFromEmbeddedSpec chdirs to t.TempDir() and asserts 200 for both /openapi.yaml and /openapi.json — passes (ok internal/api 0.493s)
  ✓ REST Swagger UI served at distinct path /doc/api, no longer shadowed by opencode shim /doc; servers URL derived from request Host: registerOpenAPIRoutes (openapi.go:33-38) registers /doc/api and /doc/api/* only, not /doc; opencode shim serves /doc (internal/shim/opencode/server.go:145); TestBareDocNotServedByAPI asserts /doc returns 404 on API server; handleSwaggerUI derives serversURL from r.Host; TestDocAPISwaggerUIEndpoint verifies Host-derived URL — all pass
  ✓ README and docs/API.md describe what /doc actually is (opencode shim Swagger UI) and do not promise spec paths that 404: README.md:251 states /doc is the opencode shim's own Swagger UI and REST API explorer is at /doc/api; docs/API.md:73-91 documents /openapi.json, /openapi.yaml, /doc/api and explicitly notes GET /doc is the opencode shim's Swagger UI; no spec paths promised that 404
  ✗ regression test proves /openapi.json 200 with cwd outside repo root; go build/vet/gofmt clean; go test -short green: Regression test TestOpenAPISpecServedFromEmbeddedSpec passes; go build ./... exit 0; go vet ./... exit 0; gofmt -l clean. BUT go test -short -count=1 ./... is NOT green: FAIL in internal/config (TestApplyEnvOverrides_OpenRouterUnsetKeepsDeepSeek — env OPENROUTER_API_KEY set) and demo (TestDemo_FullAgentHarness). These failures are pre-existing/unrelated to dogfood-103 (from commits 19fbaaa/ac8d36a), but the criterion explicitly requires 'go test -short green' which is not met.
OpenAPI embedded-spec serving, /doc/api shadowing fix, and docs are correctly implemented and the regression test passes, but the full 'go test -short' suite is not green due to pre-existing unrelated failures in internal/config and demo.

## Summary

Judge Result: dogfood-103

Stage tier1: FAIL
    ✓ secrets: 
  ✓ lint: 
  ✗ tests: Command timed out

Stage tier2: PASS
  COMPLETE
  ✓ openapi.json and openapi.yaml return 200 from ANY CWD (embedded spec fallback); docker run image serves /openapi.json 200 without copying specs/: internal/api/openapi.go loadSpec() falls back to specs.BundledYAML when no on-disk file found; specs/embed.go go:embed openapi/bundled.yaml; Dockerfile copies only /bin/consensus (no specs/ dir); TestOpenAPISpecServedFromEmbeddedSpec chdirs to t.TempDir() and asserts 200 for both /openapi.yaml and /openapi.json — passes (ok internal/api 0.493s)
  ✓ REST Swagger UI served at distinct path /doc/api, no longer shadowed by opencode shim /doc; servers URL derived from request Host: registerOpenAPIRoutes (openapi.go:33-38) registers /doc/api and /doc/api/* only, not /doc; opencode shim serves /doc (internal/shim/opencode/server.go:145); TestBareDocNotServedByAPI asserts /doc returns 404 on API server; handleSwaggerUI derives serversURL from r.Host; TestDocAPISwaggerUIEndpoint verifies Host-derived URL — all pass
  ✓ README and docs/API.md describe what /doc actually is (opencode shim Swagger UI) and do not promise spec paths that 404: README.md:251 states /doc is the opencode shim's own Swagger UI and REST API explorer is at /doc/api; docs/API.md:73-91 documents /openapi.json, /openapi.yaml, /doc/api and explicitly notes GET /doc is the opencode shim's Swagger UI; no spec paths promised that 404
  ✗ regression test proves /openapi.json 200 with cwd outside repo root; go build/vet/gofmt clean; go test -short green: Regression test TestOpenAPISpecServedFromEmbeddedSpec passes; go build ./... exit 0; go vet ./... exit 0; gofmt -l clean. BUT go test -short -count=1 ./... is NOT green: FAIL in internal/config (TestApplyEnvOverrides_OpenRouterUnsetKeepsDeepSeek — env OPENROUTER_API_KEY set) and demo (TestDemo_FullAgentHarness). These failures are pre-existing/unrelated to dogfood-103 (from commits 19fbaaa/ac8d36a), but the criterion explicitly requires 'go test -short green' which is not met.
OpenAPI embedded-spec serving, /doc/api shadowing fix, and docs are correctly implemented and the regression test passes, but the full 'go test -short' suite is not green due to pre-existing unrelated failures in internal/config and demo.

Overall: FAIL ✗
