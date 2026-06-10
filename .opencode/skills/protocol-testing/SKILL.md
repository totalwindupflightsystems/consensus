---
name: protocol-testing
description: >
  Real, tool-driven API and protocol testing for HTTP REST, gRPC, GraphQL, WebSocket, SSE,
  and message-queue protocols (MQTT, NATS, Kafka). Provides concrete CLI workflows, config-driven
  test patterns, positive+negative contract test templates, and evidence capture checklists.
  Load this skill when writing protocol tests, setting up a new service's test suite, or
  verifying that a protocol integration is contract-compliant. Emphasizes real tools (curl,
  grpcurl, buf, websocat, graphqurl) over pseudo-snippets.
version: "1.0"
tags:
  vertical: [coding]
  category: testing
  core: false
---

# Protocol Testing Skill (Portable)

> **"A test that only checks connectivity is not a contract test."**
>
> **"Every protocol endpoint needs at least one negative test."**

This skill provides portable, tool-driven guidance for testing APIs and protocols.
It is the operational companion to `enterprise-testing-standard` (tiers) and
`test-quality-gates-axiom` (quality gates).

## When to Load This Skill

Load this skill when:
- Writing tests for any HTTP REST, gRPC, GraphQL, WebSocket, SSE, or message-queue endpoint
- Setting up a new service's protocol test suite from scratch
- Reviewing whether a protocol integration has adequate positive AND negative coverage
- Running a QA sweep on protocol-level tests
- Onboarding a new repo that exposes any network API

## Core Rules

1. **Real tools first.** Use curl, grpcurl, websocat, graphqurl, nats CLI, kcat before
   writing custom code. Custom code is the fallback, not the default.
2. **Config-driven runs.** Every test suite MUST be driven by a config file (base URL,
   auth, test data). No hardcoded connection strings.
3. **Positive AND negative.** Every endpoint/method/topic MUST have at least one negative
   test (auth failure, invalid input, not-found).
4. **Schema validation.** Status code / connectivity checks are NOT contract tests.
   Always validate the response/message schema.
5. **Captured evidence.** Every test run MUST produce captured output. "I ran it and it
   worked" without output is unverified.
6. **Tier alignment.** Protocol tests map to Tier 3-5 in the enterprise testing hierarchy.
   They are NOT a substitute for unit tests (Tier 1).

---

## Protocol -> Tool -> Test Suite Mapping

| Protocol | Primary CLI Tool | Schema Tool | Python Library | Negative Test Focus |
|----------|-----------------|-------------|----------------|---------------------|
| HTTP REST | `curl`, `httpie` | `openapi-spec-validator`, `schemathesis` | `httpx` + `pytest` | 401, 403, 404, 422, 413 |
| gRPC | `grpcurl` | `buf lint`, `buf breaking` | `grpcio` + `pytest` | UNAUTHENTICATED, NOT_FOUND, INVALID_ARGUMENT |
| GraphQL | `gq` (graphqurl), `curl` | `graphql-inspector` | `gql` + `pytest` | errors array, introspection disabled |
| WebSocket | `websocat`, `wscat` | message schema assertion | `websockets` + `pytest` | close 4001, invalid JSON |
| SSE | `curl -N` | event field assertion | `httpx` streaming + `pytest` | 401, 404, invalid JSON events |
| MQTT | `mosquitto_pub/sub` | payload schema assertion | `paho-mqtt` + `pytest` | unauthorized topic |
| NATS | `nats` CLI | payload schema assertion | `nats-py` + `pytest` | permissions violation |
| Kafka | `kcat` | payload schema assertion | `kafka-python` + `pytest` | authorization failed |

---

## Workflow: Setting Up a Protocol Test Suite

### Step 1: Create Config File

Every protocol test suite starts with a config file. This makes runs reproducible and
environment-agnostic.

**Template: `tests/protocol/<protocol>-config.json`**
```json
{
  "base_url_or_host": "http://127.0.0.1:8100",
  "auth": {
    "type": "bearer | metadata | query_param | none",
    "token_env": "API_TOKEN"
  },
  "schema_file": "specs/openapi.yaml",
  "timeouts": { "connect_s": 5, "read_s": 30 },
  "test_data": {
    "valid_id": "smoke-test-001",
    "invalid_id": "does-not-exist-999"
  }
}
```

### Step 2: Smoke Test with CLI Tool

Before writing Python tests, verify the endpoint works with the CLI tool.
This is the fastest feedback loop.

```bash
# HTTP REST
curl -sf http://127.0.0.1:8100/global/health | python3 -m json.tool

# gRPC
grpcurl -plaintext 127.0.0.1:50051 list

# GraphQL
gq http://127.0.0.1:8100/graphql -q '{ __schema { queryType { name } } }'

# WebSocket
echo '{"type":"ping"}' | timeout 5 websocat ws://127.0.0.1:8100/ws

# SSE
timeout 5 curl -sf -N http://127.0.0.1:8100/api/v1/runs/run-001/events | head -20
```

### Step 3: Validate Schema / Contract

```bash
# HTTP: validate OpenAPI spec
openapi-spec-validator specs/openapi.yaml

# HTTP: run schemathesis (auto-generates negative cases)
schemathesis run specs/openapi.yaml --url http://127.0.0.1:8100 --checks all

# gRPC: lint proto files
buf lint proto/

# gRPC: check for breaking changes
buf breaking proto/ --against ".git#subdir=proto"

# GraphQL: check for breaking schema changes
graphql-inspector diff specs/schema.graphql specs/schema-new.graphql
```

### Step 4: Write Positive Tests

Positive tests verify the happy path with valid input and expected response schema.

```python
# Minimum positive test template
def test_<endpoint>_happy_path(http_client):
    """Tier 4: <endpoint> returns <expected_status> with <expected_fields>."""
    resp = http_client.<method>("<path>", json=<valid_payload>)
    assert resp.status_code == <expected_status>, \
        f"Expected <expected_status>, got {resp.status_code}: {resp.text}"
    body = resp.json()
    assert "<required_field>" in body, f"Missing field in: {body}"
    assert body["<required_field>"] == <expected_value>
```

### Step 5: Write Negative Tests (MANDATORY)

Negative tests are not optional. Use this checklist to ensure coverage.

**Universal Negative Test Checklist:**

```python
# Auth failures
def test_missing_auth_returns_<expected_error>(client_without_auth):
    """No credentials -> <expected_error_code>."""
    ...

def test_invalid_auth_returns_<expected_error>(client_with_bad_token):
    """Invalid credentials -> <expected_error_code>."""
    ...

# Input validation
def test_missing_required_field_returns_<expected_error>(http_client):
    """Missing required field -> <expected_error_code>."""
    ...

def test_invalid_field_type_returns_<expected_error>(http_client):
    """Wrong type for field -> <expected_error_code>."""
    ...

# Not found
def test_nonexistent_resource_returns_<expected_error>(http_client):
    """Non-existent resource -> <expected_error_code>."""
    ...
```

### Step 6: Capture Evidence

Every test run MUST produce captured output.

```bash
# Run tests and capture output
pytest tests/protocol/ -v 2>&1 | tee /tmp/protocol-test-results.txt

# Run smoke script and capture
bash tests/protocol/smoke-http.sh 2>&1 | tee /tmp/smoke-http.txt

# Store in evidence location
cp /tmp/protocol-test-results.txt \
  .memory-bank/work-items/<ID>/runs/<RUN_ID>/protocol-test-results.txt
```

---

## HTTP REST: Quick Reference

### Minimal Positive + Negative Test Suite

```python
# tests/protocol/test_http_contract.py
"""Minimal HTTP contract test suite. Expand per endpoint."""
import pytest

class TestHealthEndpoint:
    def test_health_returns_200_with_status(self, http_client):
        resp = http_client.get("/global/health")
        assert resp.status_code == 200
        assert "status" in resp.json()

class TestAuthGates:
    def test_missing_auth_returns_401(self, api_config):
        import httpx
        with httpx.Client(base_url=api_config["base_url"]) as c:
            resp = c.get("/api/v1/runs")
        assert resp.status_code == 401

    def test_invalid_token_returns_401(self, api_config):
        import httpx
        with httpx.Client(base_url=api_config["base_url"],
                          headers={"Authorization": "Bearer bad"}) as c:
            resp = c.get("/api/v1/runs")
        assert resp.status_code == 401

class TestInputValidation:
    def test_missing_required_field_returns_422(self, http_client):
        resp = http_client.post("/api/v1/work-items", json={})
        assert resp.status_code == 422

    def test_unknown_resource_returns_404(self, http_client):
        resp = http_client.get("/api/v1/work-items/does-not-exist-999")
        assert resp.status_code == 404
```

### schemathesis Auto-Negative Testing

```bash
# Auto-generate and run negative tests from OpenAPI spec
schemathesis run specs/openapi.yaml \
  --url http://127.0.0.1:8100 \
  --checks all \
  --hypothesis-max-examples 100 \
  --auth "Bearer $API_TOKEN" \
  2>&1 | tee /tmp/schemathesis.txt
```

---

## gRPC: Quick Reference

### grpcurl Smoke Commands

```bash
# List services (requires reflection or proto)
grpcurl -plaintext 127.0.0.1:50051 list

# Health check
grpcurl -plaintext -d '{"service":""}' \
  127.0.0.1:50051 grpc.health.v1.Health/Check

# Call with auth metadata
grpcurl -plaintext \
  -H "authorization: Bearer $GRPC_TOKEN" \
  -d '{"work_item_id":"smoke-001"}' \
  127.0.0.1:50051 axiom.v1.WorkItemService/GetWorkItem

# Negative: missing auth
grpcurl -plaintext \
  -d '{"work_item_id":"smoke-001"}' \
  127.0.0.1:50051 axiom.v1.WorkItemService/GetWorkItem 2>&1 \
  | grep -q "UNAUTHENTICATED" && echo "PASS" || echo "FAIL"
```

### buf Commands

```bash
buf lint proto/                                    # lint
buf breaking proto/ --against ".git#subdir=proto" # breaking change check
buf generate proto/ --template buf.gen.yaml        # code gen
```

---

## GraphQL: Quick Reference

### curl Smoke Commands

```bash
# Introspection
curl -sf -X POST http://127.0.0.1:8100/graphql \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $API_TOKEN" \
  -d '{"query":"{ __schema { queryType { name } } }"}' | python3 -m json.tool

# Negative: syntax error -> errors array
curl -sf -X POST http://127.0.0.1:8100/graphql \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $API_TOKEN" \
  -d '{"query":"{ invalid syntax !!!"}' | python3 -m json.tool
```

---

## WebSocket: Quick Reference

### websocat Smoke Commands

```bash
# Connect and send/receive
echo '{"type":"ping"}' | timeout 5 websocat "ws://127.0.0.1:8100/ws?token=$API_TOKEN"

# Negative: missing auth
echo '{"type":"ping"}' | timeout 5 websocat "ws://127.0.0.1:8100/ws" 2>&1 \
  | grep -q "401\|4001\|Unauthorized" && echo "PASS" || echo "WARN"
```

---

## SSE: Quick Reference

### curl Smoke Commands

```bash
# Stream events (5s timeout)
timeout 5 curl -sf -N \
  -H "Authorization: Bearer $API_TOKEN" \
  "http://127.0.0.1:8100/api/v1/runs/run-001/events" | head -20

# Negative: missing auth
curl -s -o /dev/null -w "%{http_code}" \
  "http://127.0.0.1:8100/api/v1/runs/run-001/events"
# Expected: 401
```

---

## Negative Testing Mandates by Protocol

### HTTP REST (MUST test all)
- 401 missing auth
- 401 invalid token
- 403 insufficient scope
- 404 resource not found
- 405 wrong method
- 422 missing required field
- 422 wrong field type
- 400/422 malformed JSON
- Response schema matches OpenAPI spec

### gRPC (MUST test all)
- UNAUTHENTICATED (16) missing auth
- UNAUTHENTICATED (16) invalid token
- PERMISSION_DENIED (7) insufficient scope
- NOT_FOUND (5) resource missing
- INVALID_ARGUMENT (3) empty/malformed required field
- DEADLINE_EXCEEDED (4) very short deadline
- Response message fields match proto definition

### GraphQL (MUST test all)
- 401 or errors[UNAUTHENTICATED] missing auth
- errors array for syntax error
- errors array for unknown field
- errors array for null required argument
- Introspection disabled in production

### WebSocket (MUST test all)
- 401 or close 4001 missing auth
- Error message or close for invalid JSON
- Clean close when server terminates

### SSE (MUST test all)
- 401 missing auth
- 404 unknown resource
- All data lines are valid JSON
- Events contain expected fields

### Message Queues (MUST test all)
- Unauthorized topic/subject rejected
- Malformed payload handled gracefully
- Message schema validated

---

## Config-Driven Test Pattern (Universal)

All protocol test suites SHOULD follow this pattern, inspired by `opencode.jsonc`:

```
tests/protocol/
  <protocol>-config.json     # environment config (base URL, auth, test data)
  conftest.py                # fixtures that load config and create clients
  smoke-<protocol>.sh        # CLI smoke script (fast, no Python required)
  test_<protocol>_positive.py  # happy-path contract tests
  test_<protocol>_negative.py  # error-path contract tests
  test_<protocol>_schema.py    # schema/contract validation tests
```

The config file is the single source of truth for connection parameters. CI overrides
via environment variables. Local dev uses defaults.

---

## Evidence Capture Template

```markdown
## Protocol Test Evidence: <protocol> - <date>

**Work Item**: <ID>
**Run ID**: <RUN_ID>
**Protocol**: HTTP REST | gRPC | GraphQL | WebSocket | SSE | MQTT | NATS | Kafka
**Tier**: 3 | 4 | 5

### Tools Used
- <tool> <version>

### Positive Tests
| Test | Status | Notes |
|------|--------|-------|
| health check | PASS | HTTP 200, status=healthy |
| create resource | PASS | HTTP 201, id field present |

### Negative Tests
| Test | Expected | Actual | Status |
|------|----------|--------|--------|
| missing auth | 401 | 401 | PASS |
| invalid token | 401 | 401 | PASS |
| missing field | 422 | 422 | PASS |
| not found | 404 | 404 | PASS |

### Schema Validation
- openapi-spec-validator: PASS
- schemathesis: PASS (50 examples, 0 violations)

### Raw Output (truncated)
```
<captured output>
```

### Trace
`axiom:trace work_item=<ID> test=protocol-<protocol> evidence=<path>`
```

---

## Anti-Patterns (All Protocols)

| Anti-Pattern | Why Bad | Fix |
|-------------|---------|-----|
| Status-code-only assertions | Proves nothing about response shape | Add schema assertion |
| No negative tests | Misses auth bypass, injection, error handling gaps | Add auth + input validation negatives |
| Hardcoded connection strings | Breaks across environments | Use config file |
| Testing only connectivity | Does not verify contract | Send real request, validate response |
| Skipping schema validation | Contract drift goes undetected | Run spec validator in CI |
| Ignoring protocol error codes | Misses specific failure modes | Assert exact code (gRPC status, WS close code) |
| No evidence capture | "It worked" is unverifiable | Capture and store output |
| Custom code when tools exist | Harder to maintain, less standard | Use curl/grpcurl/websocat first |

---

## Memory Bank References

- `.memory-bank/best-practices/http-api-testing.md` - HTTP REST testing (full guide)
- `.memory-bank/best-practices/grpc-api-testing.md` - gRPC testing (full guide)
- `.memory-bank/best-practices/protocol-testing-common.md` - GraphQL, WebSocket, SSE, MQTT, NATS, Kafka
- `.memory-bank/best-practices/enterprise-grade-testing.md` - Tier 0-5 verification hierarchy
- `.memory-bank/best-practices/mock-servers-contract-testing.md` - Mock servers for offline tests
- `.memory-bank/best-practices/sse-streaming-safety.md` - SSE server-side safety patterns
- `.memory-bank/best-practices/testing-verification-evidence.md` - Evidence bundle requirements

## Trace

`axiom:trace work_item=protocol-testing-bp spec=specs/27-Evidence-Bundle-Schema.md plan=protocol-testing/skill doc=.opencode/skills/protocol-testing/SKILL.md`
