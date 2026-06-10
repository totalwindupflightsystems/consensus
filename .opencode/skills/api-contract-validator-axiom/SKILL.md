---
name: api-contract-validator-axiom
description: >
  OpenAPI/AsyncAPI spec generation, contract drift detection, negative testing with schemathesis,
  API versioning discipline, and contract validation verdicts. Load this skill when working with
  HTTP APIs, event-driven systems, or any surface that exposes a machine-readable contract.
  Enforces the Axiom requirement that openapi.json must stay in sync with implementation.
license: MIT
compatibility: opencode
metadata:
  version: "1.0"
  created: "2026-02-27"
  primary_spec: specs/30-External-API-And-Realtime.md
  secondary_specs:
    - specs/31-OpenCode-Integration-Contract.md
    - specs/00-PRD.md
    - AGENTS.md
tags:
  vertical: [coding, ops]
  category: testing
  core: false
---

# API Contract Validator Skill (Portable)

> **"API work is not complete if `openapi.json` is stale versus live API behavior."**
>
> **"Never claim contract compliance without runtime evidence."**

This skill provides portable, production-grade guidance for API contract validation,
drift detection, negative testing, and versioning discipline. It is the contract-enforcement
companion to `protocol-testing` (tool-driven test patterns) and `enterprise-testing-standard`
(tier hierarchy).

---

## Activation

Load this skill when:
- Generating or updating an OpenAPI or AsyncAPI specification
- Implementing or modifying HTTP API endpoints, SSE streams, or WebSocket handlers
- Running contract drift detection between spec and live server
- Writing negative API tests (schemathesis, dredd, custom)
- Reviewing API versioning and breaking change discipline
- Preparing an API-affecting PR for review
- Onboarding a new service that exposes any HTTP/event-driven API

---

## Non-Negotiables

1. **`openapi.json` sync is mandatory.** If routes, request/response schemas, status codes,
   auth requirements, or error models change, update API specs and regenerate/update
   `openapi.json` in the same change. Never claim API work complete if `openapi.json` is
   stale versus implementation. (Source: `AGENTS.md` OpenAPI Contract Sync section)

2. **Fail-closed on breaking changes without version bump.** Any breaking change (removed
   endpoint, changed required field, narrowed response schema, changed status code semantics)
   MUST be accompanied by a major version bump (`/api/v1/` -> `/api/v2/`) or a documented
   deprecation path. Unversioned breaking changes are BLOCKED.

3. **Runtime verification required.** Contract validation MUST include runtime checks against
   a live server (`axiom serve` + HTTP requests). Static spec validation alone is insufficient.
   (Source: `specs/30-External-API-And-Realtime.md`)

4. **Every endpoint needs a negative test.** No endpoint is contract-compliant without at
   least one negative test (auth failure, invalid input, not-found). Status-code-only checks
   are not contract tests.

5. **Never invent test results.** All contract validation claims must be backed by captured
   command output. "I checked and it matches" without evidence is unverified.

---

## OpenAPI Spec Generation

### From Code (Auto-Generation)

| Framework | Tool | Command |
|-----------|------|---------|
| FastAPI (Python) | Built-in | `curl http://127.0.0.1:8100/openapi.json > openapi.json` |
| Express/Koa (Node) | `swagger-jsdoc` + `swagger-ui-express` | `npx swagger-jsdoc -d swaggerDef.js -o openapi.json` |
| Go (gin/echo) | `swag` | `swag init -g main.go -o docs/` |
| Spring Boot | `springdoc-openapi` | `curl http://localhost:8080/v3/api-docs > openapi.json` |
| Generic | `openapi-generator` | `openapi-generator generate -i spec.yaml -g openapi-yaml` |

### Axiom-Specific Generation

For Axiom repos with `axiom serve`:

```bash
# Start the server
axiom serve --port 8100 &
sleep 3

# Capture the live OpenAPI spec (if auto-generated)
curl -sf http://127.0.0.1:8100/openapi.json > openapi.json

# Or from OpenCode server (internal API)
curl -sf http://127.0.0.1:4096/doc > opencode-openapi.json

# Kill server
kill %1
```

### Spec Validation

```bash
# Validate OpenAPI spec syntax and semantics
openapi-spec-validator openapi.json

# Validate with spectral (custom rules)
spectral lint openapi.json --ruleset .spectral.yaml

# Validate AsyncAPI spec
asyncapi validate asyncapi.yaml
```

---

## AsyncAPI for Event-Driven Systems

### When to Use AsyncAPI

Use AsyncAPI (not OpenAPI) for:
- Server-Sent Events (SSE) streams
- WebSocket bidirectional channels
- Message queue topics (MQTT, NATS, Kafka)
- Any publish/subscribe or event-driven interface

### Axiom SSE Contract

The Axiom SSE stream (`GET /api/v1/events/stream`) is defined in
`specs/30-External-API-And-Realtime.md`. An AsyncAPI spec for this stream should define:

```yaml
asyncapi: '2.6.0'
info:
  title: Axiom Event Stream
  version: '1.0.0'
channels:
  /api/v1/events/stream:
    subscribe:
      message:
        oneOf:
          - $ref: '#/components/messages/StepStarted'
          - $ref: '#/components/messages/StepCompleted'
          - $ref: '#/components/messages/VerificationPassed'
          - $ref: '#/components/messages/VerificationFailed'
          - $ref: '#/components/messages/RunCompleted'
          - $ref: '#/components/messages/RunFailed'
          - $ref: '#/components/messages/RunCancelled'
```

### Validation

```bash
# Validate AsyncAPI spec
asyncapi validate asyncapi.yaml

# Generate documentation
asyncapi generate fromTemplate asyncapi.yaml @asyncapi/html-template -o docs/events/
```

---

## Contract Drift Detection

Contract drift occurs when the live server behavior diverges from the spec file. This is
the most common source of API bugs in production.

### Detection Methods

#### Method 1: Schemathesis (Automated, Recommended)

```bash
# Run schemathesis against live server using the spec
schemathesis run openapi.json \
  --url http://127.0.0.1:8100 \
  --checks all \
  --hypothesis-max-examples 100 \
  --auth "Bearer $API_TOKEN" \
  2>&1 | tee /tmp/schemathesis-drift.txt
```

Schemathesis auto-generates requests from the spec and validates responses match the spec.
Any mismatch is a drift.

#### Method 2: Dredd (Contract-First)

```bash
# Run dredd against live server
dredd openapi.json http://127.0.0.1:8100 \
  --header "Authorization: Bearer $API_TOKEN" \
  2>&1 | tee /tmp/dredd-drift.txt
```

#### Method 3: Manual Comparison (Fallback)

When automated tools are unavailable:

```bash
# 1. Start server
axiom serve --port 8100 &
sleep 3

# 2. For each endpoint in openapi.json, make a request and compare
curl -sf http://127.0.0.1:8100/health | python3 -m json.tool > /tmp/health-actual.json

# 3. Compare actual response schema against openapi.json definition
# Check: all required fields present, types match, no extra undocumented fields

# 4. Kill server
kill %1
```

#### Method 4: OpenAPI Diff (Spec-to-Spec)

```bash
# Compare two versions of the spec
openapi-diff openapi-old.json openapi-new.json --output /tmp/api-diff.txt

# Or use oasdiff for breaking change detection
oasdiff breaking openapi-old.json openapi-new.json
```

### Axiom Drift Detection Workflow

```bash
# Full drift detection for Axiom API
axiom serve --port 8100 &
sleep 3

# 1. Validate spec syntax
openapi-spec-validator openapi.json

# 2. Run schemathesis for drift
schemathesis run openapi.json \
  --url http://127.0.0.1:8100 \
  --checks all \
  --hypothesis-max-examples 50 \
  2>&1 | tee /tmp/drift-check.txt

# 3. Spot-check critical endpoints
curl -sf http://127.0.0.1:8100/health | python3 -c "
import json, sys
data = json.load(sys.stdin)
required = ['status', 'version', 'mode', 'uptime_seconds']
missing = [f for f in required if f not in data]
if missing:
    print(f'DRIFT: Missing fields: {missing}')
    sys.exit(1)
print('OK: Health endpoint matches spec')
"

kill %1
```

---

## Negative Testing Patterns

### Schemathesis Auto-Negative Testing

```bash
# Auto-generate negative cases from OpenAPI spec
schemathesis run openapi.json \
  --url http://127.0.0.1:8100 \
  --checks all \
  --hypothesis-max-examples 200 \
  --stateful=links \
  2>&1 | tee /tmp/schemathesis-negative.txt
```

### Contract Test Templates

```python
# tests/contract/test_api_contract.py
"""API contract tests: verify live server matches openapi.json."""

class TestErrorResponses:
    """Every error response must match the error schema in openapi.json."""

    def test_error_shape_on_404(self, http_client):
        resp = http_client.get("/api/v1/runs/nonexistent-run-id")
        assert resp.status_code == 404
        body = resp.json()
        assert "error" in body
        assert "code" in body["error"]
        assert "message" in body["error"]
        assert body["error"]["code"] == "run_not_found"

    def test_error_shape_on_400(self, http_client):
        resp = http_client.post("/api/v1/runs", json={})
        assert resp.status_code == 400
        body = resp.json()
        assert "error" in body
        assert body["error"]["code"] == "invalid_request"

    def test_error_shape_on_409(self, http_client):
        """Duplicate run creation returns 409 with correct error code."""
        # Create first run
        payload = {"intent": "test", "idempotency_token": "dup-test-001"}
        resp1 = http_client.post("/api/v1/runs", json=payload)
        assert resp1.status_code == 201
        # Attempt duplicate
        resp2 = http_client.post("/api/v1/runs", json=payload)
        assert resp2.status_code in (200, 409)  # idempotent accept or conflict

class TestAuthGates:
    """Auth gates must return correct status codes per spec."""

    def test_missing_auth_returns_401(self, unauthenticated_client):
        resp = unauthenticated_client.get("/api/v1/runs")
        assert resp.status_code == 401

    def test_insufficient_scope_returns_403(self, readonly_client):
        resp = readonly_client.post("/api/v1/runs", json={"intent": "test"})
        assert resp.status_code == 403
```

---

## API Versioning Discipline

### Semantic Versioning for APIs

| Change Type | Version Impact | Example |
|-------------|---------------|---------|
| New endpoint added | Minor (non-breaking) | `GET /api/v1/metrics` added |
| New optional field in response | Minor (non-breaking) | `duration_ms` added to run response |
| New optional field in request | Minor (non-breaking) | `tags` field added to run creation |
| New required field in request | **Major (breaking)** | `repo` now required in local mode |
| Field removed from response | **Major (breaking)** | `status` alias removed |
| Status code changed | **Major (breaking)** | 200 -> 201 for creation |
| Field type changed | **Major (breaking)** | `confidence` from int to object |
| Endpoint removed | **Major (breaking)** | `DELETE /api/v1/runs/{id}` removed |

### Breaking Change Detection

```bash
# Detect breaking changes between spec versions
oasdiff breaking openapi-old.json openapi-new.json

# Or with openapi-diff
openapi-diff openapi-old.json openapi-new.json --fail-on-incompatible
```

### Deprecation Path

When deprecating an API element:

1. **Announce**: Add `deprecated: true` in the OpenAPI spec for the element
2. **Warn**: Return `Deprecation` header in responses: `Deprecation: true`
3. **Document**: Add deprecation notice with removal timeline to changelog
4. **Compatibility window**: Maintain deprecated element for at least 2 minor versions
5. **Remove**: Remove in the next major version after the compatibility window

### Axiom Compatibility Aliases

Axiom uses time-based compatibility cutoffs (see `specs/30-External-API-And-Realtime.md`):

- `api.compat_alias_until`: UTC timestamp for response field aliases (`status` -> `run_status`)
- `api.local_api_provenance_compat_until`: UTC timestamp for provenance field handling

These are the deprecation mechanism for field renames. After the cutoff, aliases are removed.

---

## Contract Validation Verdict

After running contract validation, produce a verdict using this scoring model:

### Verdict Scale

| Verdict | Score | Meaning |
|---------|-------|---------|
| **PASS** | 80-100 | Spec and implementation are in sync; all negative tests pass |
| **WARN** | 50-79 | Minor drift detected (new undocumented fields, optional field mismatches) |
| **FAIL** | 1-49 | Significant drift (missing endpoints, wrong status codes, schema violations) |
| **BLOCKED** | 0 | Cannot validate (server won't start, spec is invalid, tools unavailable) |

### Scoring Rubric

| Check | Weight | Pass Criteria |
|-------|--------|---------------|
| Spec syntax valid | 10 | `openapi-spec-validator` passes |
| Server starts and health OK | 10 | `GET /health` returns 200 |
| All documented endpoints respond | 25 | Every endpoint in spec returns expected status |
| Response schemas match spec | 25 | Schemathesis or manual schema check passes |
| Negative tests pass | 15 | Auth gates, input validation, not-found all correct |
| No undocumented endpoints | 5 | No routes exist that aren't in the spec |
| Breaking change discipline | 10 | No unversioned breaking changes detected |

### Verdict Template

```markdown
## API Contract Validation Verdict

**Verdict**: PASS | WARN | FAIL | BLOCKED
**Score**: <0-100>
**Date**: <ISO 8601>
**Spec**: openapi.json (version <X>)
**Server**: <URL>

### Checks
| Check | Score | Status | Notes |
|-------|-------|--------|-------|
| Spec syntax | 10/10 | PASS | openapi-spec-validator clean |
| Server health | 10/10 | PASS | GET /health -> 200 |
| Endpoint coverage | 25/25 | PASS | All 12 endpoints respond |
| Schema match | 20/25 | WARN | 1 undocumented field in /runs response |
| Negative tests | 15/15 | PASS | 8/8 negative tests pass |
| No undocumented routes | 5/5 | PASS | No extra routes found |
| Breaking changes | 10/10 | PASS | No breaking changes |

**Total**: 95/100

### Evidence
- Schemathesis output: <path>
- Negative test output: <path>
- Server logs: <path>

### Drift Items (if any)
1. Undocumented field `metadata` in GET /api/v1/runs response (WARN, non-breaking)
```

---

## Integration

### Works With

| Skill/Agent | Integration Point |
|-------------|-------------------|
| `protocol-testing` | Tool-driven test patterns; this skill adds contract-specific validation |
| `enterprise-testing-standard` | Contract tests map to Tier 3-5 |
| `test-quality-gates-axiom` | Contract test quality gates |
| `enterprise-release-quality` | API contract validation is a release gate |
| `@security-review-axiom` | Auth gate testing, CORS validation |
| `@sre-ops-axiom` | API health monitoring, SLI definition |
| `spec-kickoff-axiom` | API spec design decisions |
| `prd-spec-merge-axiom` | API spec updates from PRD changes |

### Invocation Pattern

When this skill is loaded, the agent should:

1. Check if `openapi.json` exists at repo root
2. If API changes are in scope, verify `openapi.json` is updated
3. Run contract drift detection if a server can be started
4. Produce a contract validation verdict
5. Include the verdict in the evidence bundle

---

## AI-Assisted Development Risks (2026)

| Risk | Mitigation |
|------|------------|
| AI generates endpoints not in spec | Always regenerate `openapi.json` from code after changes |
| AI claims contract compliance without testing | Require runtime evidence (Tier 3+) |
| AI adds breaking changes without version bump | Automated breaking change detection in CI |
| AI generates overly permissive CORS | Validate CORS config against spec |
| AI hallucinates API responses in tests | Use real server responses, never mock for contract tests |
| AI skips negative tests | Mandate negative test checklist per endpoint |

---

## Anti-Patterns

| Anti-Pattern | Why Bad | Fix |
|-------------|---------|-----|
| Updating code without updating `openapi.json` | Contract drift | Same-commit spec + code updates |
| Status-code-only contract tests | Proves nothing about response shape | Add schema assertions |
| Testing against mocks for contract validation | Mocks can't detect drift | Test against live server |
| Manual spec maintenance for auto-gen frameworks | Spec falls behind | Auto-generate from code |
| No breaking change detection in CI | Silent breaking changes | Add `oasdiff` or equivalent to CI |
| Claiming "API complete" without negative tests | Auth bypass, injection risks | Mandate negative test checklist |

---

## Trace

`axiom:trace work_item=api-contract-validator-axiom spec=specs/30-External-API-And-Realtime.md plan= prompt=.opencode/skills/api-contract-validator-axiom/SKILL.md evidence= doc= test= commit=`
