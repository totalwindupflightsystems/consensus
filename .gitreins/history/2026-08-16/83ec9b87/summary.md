# Verdict: dogfood-103

**Task:** OpenAPI contract served from embedded spec — /openapi.json 200 from any CWD + /doc shadowing fix
**Evaluated:** 2026-08-16T09:49:43.432400
**Result:** ✓ PASS

## Pipeline Stages

- ✓ **tier1**
  -   ✓ secrets: 
  ✓ lint: 
  ✓ tests: ?   	github.com/wojons/consensus/chronicle	[no test files]
?   	github.com/wojons/consensus/cmd/cons
- ✓ **tier2**
  - COMPLETE
  ✓ openapi.json and openapi.yaml return 200 from ANY CWD (embedded spec fallback); docker run image serves /openapi.json 200 without copying specs/: specs/embed.go:16 go:embed openapi/bundled.yaml; internal/api/openapi.go:71-77 loadSpec() falls back to specs.BundledYAML when no on-disk file; Dockerfile copies only the binary (no specs/); TestOpenAPISpecServedFromEmbeddedSpec (openapi_test.go:60) chdirs to TempDir and asserts 200 for /openapi.yaml and /openapi.json — PASSED (exit 0)
  ✓ REST Swagger UI served at distinct path /doc/api, no longer shadowed by opencode shim /doc; servers URL derived from request Host: openapi.go:36-37 registers /doc/api and /doc/api/* (NOT /doc); handleSwaggerUI (openapi.go:143-146) derives serversURL from r.Host; opencode shim server.go:145 mounts /doc; TestDocAPISwaggerUIEndpoint asserts /doc/api 200 + Host-derived URL, TestBareDocNotServedByAPI asserts /doc 404 on API-only server — both PASSED
  ✓ README and docs/API.md describe what /doc actually is (opencode shim Swagger UI) and do not promise spec paths that 404: README.md:251 describes /doc as opencode shim's own Swagger UI and /doc/api as REST UI with embedded spec from any CWD/Docker; docs/API.md:81-91 lists /openapi.json, /openapi.yaml, /doc/api and explicitly notes /doc is NOT the REST API explorer; no 404-promising spec paths in README/docs/API.md
  ✓ regression test proves /openapi.json 200 with cwd outside repo root; go build/vet/gofmt clean; go test -short green: TestOpenAPISpecServedFromEmbeddedSpec (openapi_test.go:60) chdirs to TempDir and proves /openapi.json 200 outside repo root — PASSED; go build ./... exit 0; go vet ./... exit 0; gofmt -l clean; go test -short -count=1 ./... exit 0 (all packages ok)
All four DOGFOOD-103 criteria verified: embedded-spec fallback serves /openapi.json|yaml from any CWD and in Docker, REST Swagger UI moved to /doc/api with Host-derived servers URL, README/docs/API.md correctly describe /doc, and the regression test plus build/vet/gofmt/test all pass.

## Summary

Judge Result: dogfood-103

Stage tier1: PASS
    ✓ secrets: 
  ✓ lint: 
  ✓ tests: ?   	github.com/wojons/consensus/chronicle	[no test files]
?   	github.com/wojons/consensus/cmd/cons

Stage tier2: PASS
  COMPLETE
  ✓ openapi.json and openapi.yaml return 200 from ANY CWD (embedded spec fallback); docker run image serves /openapi.json 200 without copying specs/: specs/embed.go:16 go:embed openapi/bundled.yaml; internal/api/openapi.go:71-77 loadSpec() falls back to specs.BundledYAML when no on-disk file; Dockerfile copies only the binary (no specs/); TestOpenAPISpecServedFromEmbeddedSpec (openapi_test.go:60) chdirs to TempDir and asserts 200 for /openapi.yaml and /openapi.json — PASSED (exit 0)
  ✓ REST Swagger UI served at distinct path /doc/api, no longer shadowed by opencode shim /doc; servers URL derived from request Host: openapi.go:36-37 registers /doc/api and /doc/api/* (NOT /doc); handleSwaggerUI (openapi.go:143-146) derives serversURL from r.Host; opencode shim server.go:145 mounts /doc; TestDocAPISwaggerUIEndpoint asserts /doc/api 200 + Host-derived URL, TestBareDocNotServedByAPI asserts /doc 404 on API-only server — both PASSED
  ✓ README and docs/API.md describe what /doc actually is (opencode shim Swagger UI) and do not promise spec paths that 404: README.md:251 describes /doc as opencode shim's own Swagger UI and /doc/api as REST UI with embedded spec from any CWD/Docker; docs/API.md:81-91 lists /openapi.json, /openapi.yaml, /doc/api and explicitly notes /doc is NOT the REST API explorer; no 404-promising spec paths in README/docs/API.md
  ✓ regression test proves /openapi.json 200 with cwd outside repo root; go build/vet/gofmt clean; go test -short green: TestOpenAPISpecServedFromEmbeddedSpec (openapi_test.go:60) chdirs to TempDir and proves /openapi.json 200 outside repo root — PASSED; go build ./... exit 0; go vet ./... exit 0; gofmt -l clean; go test -short -count=1 ./... exit 0 (all packages ok)
All four DOGFOOD-103 criteria verified: embedded-spec fallback serves /openapi.json|yaml from any CWD and in Docker, REST Swagger UI moved to /doc/api with Host-derived servers URL, README/docs/API.md correctly describe /doc, and the regression test plus build/vet/gofmt/test all pass.

Overall: PASS ✓
