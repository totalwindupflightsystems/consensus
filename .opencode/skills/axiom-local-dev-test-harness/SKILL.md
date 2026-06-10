---
name: axiom-local-dev-test-harness
description: Portable local development and test harness for Axiom Python services — environment setup, test execution, fixture management, mock servers, CI pipeline, and containerized testing gates.
version: "1.0"
synopsis: |
  Defines how to run, test, and develop Axiom Python services locally. Covers Python environment
  setup, virtual environment, running services, mock OpenCode server (pytest-httpx), fixture format
  and loader, test directory structure, coverage gates, CI pipeline jobs (quality + security + ECR
  publish), containerized testing requirements, and mock integration servers (Prism for GitHub,
  WireMock for Jira).
when-to-use: |
  Load this skill when setting up a local dev environment for Axiom, writing or running tests,
  managing test fixtures, configuring mock servers, understanding the CI pipeline, or implementing
  containerized testing gates.
tags:
  vertical: [coding, ops]
  category: testing
  core: false
---

# Axiom Local Dev and Test Harness (Portable)

This skill defines the local development and testing infrastructure for Axiom Python services.

Source spec: `specs/26-Local-Dev-Test-Harness.md`

---

## Quick Reference

| Aspect | Detail |
|---|---|
| Python version | 3.11+ (pinned in `.python-version` or `pyproject.toml`) |
| Package root | `.axiom/src/axiom/` |
| Venv setup | `cd .axiom && python3 -m venv .venv && source .venv/bin/activate && pip install -e ".[dev]"` |
| Run all tests | `cd .axiom && pytest tests/ -v` |
| Coverage gate | 80% line coverage on `shared/` and `repo_runner/` |
| Mock approach | `pytest-httpx` for unit tests (in-process, no real server) |
| Fixture location | `.axiom/tests/fixtures/` |
| Test isolation | All tests use `tmp_path`; NEVER write to real `.memory-bank/` |

---

## Test Type Decision Table

| What to test | Test type | Location | Docker? | Credentials? |
|---|---|---|---|---|
| Config/registry loading, XML parsing, ID generation | Unit | `tests/unit/shared/` | No | No |
| Runner state machine, retry logic, cursor advancement | Unit (mocked) | `tests/unit/repo_runner/` | No | No |
| Full run with mock OpenCode | Integration | `tests/integration/` | No | No |
| HTTP client against mock server | Integration | `tests/integration/` | No | No |
| GitHub API contract validation | Integration (Prism) | `tests/integration/` | Yes | No |
| Jira API contract validation | Integration (WireMock) | `tests/integration/` | Yes | No |
| Real Jira/GitHub interaction | Live integration | `tests/integration/` | No | Yes (`@pytest.mark.credential_gated`) |
| Containerized E2E smoke | E2E | `tests/e2e/` | Yes | No |

---

## Environment Setup

### Virtual Environment

```bash
cd .axiom
python3 -m venv .venv
source .venv/bin/activate
pip install -e ".[dev]"
```

### Core Environment Variables

| Variable | Default | Purpose |
|---|---|---|
| `AXIOM_CONFIG_PATH` | `.axiom/axiom.config.yaml` | Path to repo config |
| `AXIOM_REGISTRY_PATH` | `.axiom/command-registry.yaml` | Path to command registry |
| `AXIOM_MEMORY_BANK_ROOT` | `.memory-bank/` | Memory Bank root path |
| `AXIOM_OPENCODE_BASE_URL` | `http://127.0.0.1:4096` | OpenCode server base URL |
| `AXIOM_LOG_LEVEL` | `INFO` | Log level |
| `AXIOM_LOG_FORMAT` | `json` | `json` (JSON Lines) or `console` (human-readable) |

### Configuration Precedence (highest first)

1. Environment variables
2. `.env` file in repo root
3. `.axiom/axiom.config.local.yaml` (gitignored overrides)
4. `.axiom/axiom.config.yaml` (committed base)
5. Hardcoded defaults

---

## Running Services Locally

### Repo Runner

```bash
cd .axiom && source .venv/bin/activate
uvicorn axiom.repo_runner.api.app:app --host 127.0.0.1 --port 8100 --reload
```

Endpoints: `GET /health`, `POST /runs`, `GET /runs/{run_id}`

### Control Plane

```bash
uvicorn axiom.control_plane.api.app:app --host 127.0.0.1 --port 8200 --reload
```

---

## Mock OpenCode Server

### Unit Tests: `pytest-httpx` (preferred)

```python
@pytest.fixture
def mock_opencode(httpx_mock):
    httpx_mock.add_response(url="http://localhost:3000/health", json={"status": "ok"})
    httpx_mock.add_response(url="http://localhost:3000/execute", method="POST",
        json={"status": "completed", "message": load_fixture("ok_response.xml")})
```

`pytest-httpx` raises an error if an unmocked httpx request is made -- no real network calls escape.

### Integration Tests: Standalone mock server (future, post-v1)

---

## Fixture Format

### Directory Structure

```
.axiom/tests/fixtures/
  commands/
    plan/ok.txt, missing_summary.txt, blocked.txt
    verify-step/pass.txt, fail_with_injection.txt
    update-specs/ok.txt
  errors/
    malformed_json.txt, empty_message.txt, no_xml.txt, partial_xml.txt
  health/
    ok.json, unavailable.json
```

### Template Variables

Fixtures use Python `str.format()` style: `{run_id}`, `{work_item_id}`, `{repo}`, `{phase_id}`, `{task_id}`, `{step_id}`.

### Fixture Loader

```python
from axiom.tests.fixtures import load_fixture

message = load_fixture("commands/plan/ok.txt",
    run_id="test-run-001", work_item_id="TEST-42", repo="org/test-repo",
    phase_id="phase-1", task_id="task-1", step_id="step-1")
```

Rules:
- Missing template variable -> `KeyError` (fail fast)
- Literal braces: escape as `{{` or `}}`

---

## Test Directory Structure

```
.axiom/tests/
  conftest.py                    # Shared fixtures
  fixtures/                      # XML/YAML response fixtures
  mocks/opencode_server.py       # Mock server (future)
  unit/
    shared/                      # Config, registry, XML, IDs, logging
    repo_runner/                 # Cursor, runner, retries, injection, checkpoint
  integration/
    test_runner_end_to_end.py
    test_opencode_client.py
    test_memory_bank_adapter.py
  cli/test_dispatch.py           # Dispatch daemon CLI tests
  e2e/test_containerized_e2e.py  # Full containerized e2e (requires Docker)
```

### Test Naming Conventions

- Files: `test_<module>.py`
- Functions: `test_<behavior_under_test>` (descriptive)
- Docstrings reference spec: `"""specs/04-XML-Protocol.md: Missing required tags should trigger retry."""`

### Coverage Requirements

- Minimum: 80% line coverage on `shared/` and `repo_runner/`
- Enforce: `pytest tests/ --cov=axiom --cov-report=term-missing --cov-fail-under=80`
- Branch coverage: recommended but not gated in v1

---

## Key Test Scenarios

### Config and Registry (Unit)

| Scenario | Expected |
|---|---|
| Valid config loads | Config object with all fields |
| Invalid config | Clear error with field path |
| Valid registry loads | Registry with computed required tags |
| Unknown command in registry | Falls back to default required tags |

### XML Parsing (Unit)

| Scenario | Expected |
|---|---|
| Complete envelope | All fields extracted |
| Missing required tag | Structured error listing missing tags |
| No XML in message | `missing_xml_tags` error |
| Malformed XML | Parse error, retryable |

### Runner State Machine (Unit, Mocked)

| Scenario | Expected |
|---|---|
| Happy path | Run completes, cursor at end |
| Step fails, retry succeeds | Step retried, then proceeds |
| Retries exhausted | Run fails, escalation event |
| Verification injects step | New step added, cursor moves to it |
| Resume after crash | Skips completed steps, resumes at cursor |

---

## Mock Integration Servers

### GitHub Mock (Prism)

```bash
docker run --rm -d --name github-mock \
  -v $(pwd)/.axiom/test-fixtures/github-api.json:/tmp/github-api.json:ro \
  -p 4010:4010 stoplight/prism:5 mock -h 0.0.0.0 /tmp/github-api.json
```

Port: `4010`. Health: `curl -f http://localhost:4010`

### Jira Mock (WireMock)

```bash
docker run --rm -d --name jira-mock \
  -v $(pwd)/.axiom/test-fixtures/jira-stubs:/home/wiremock:ro \
  -p 4011:8080 wiremock/wiremock:3.9.2
```

Port: `4011`. Health: `curl -f http://localhost:4011/__admin/health`

### Docker Compose for Mocks

```bash
docker compose -f docker-compose.test-mocks.yml up -d
cd .axiom && pytest tests/integration/ -k "github_mock or jira_mock" -q
docker compose -f docker-compose.test-mocks.yml down
```

---

## CI Pipeline

### Triggers

| Trigger | Scope |
|---|---|
| `push` to `main` | All jobs including ECR publish + scans |
| `pull_request` to `main` | Quality jobs + security scans only |
| `workflow_dispatch` | All jobs (manual trigger) |

### Quality Jobs

| Job | Purpose | Dependencies |
|---|---|---|
| `lint-and-typecheck` | Ruff lint + Mypy | None |
| `test-quality` | Assertionless/tautology detection | None |
| `test` | Pytest with 95% coverage | `test-quality` |
| `guards` | TODO/plan parity, no stubs, pass gate | None |

### Security and Publish Jobs

| Job | Purpose | Dependencies |
|---|---|---|
| `secrets-scan` | Detect leaked secrets | None |
| `sast` | Static Application Security Testing | `secrets-scan` |
| `ecr-publish` | Build + push Docker image to ECR | All quality + `secrets-scan` |
| `trivy-scan` | CVE scan on published image | `ecr-publish` |
| `anchore-scan` | Grype scan on published image | `ecr-publish` |

All security scans use **notify/non-blocking** mode -- findings reported but do not fail pipeline.

### OIDC Authentication

ECR publish and scans use OIDC (`id-token: write`) -- no long-lived AWS credentials.

---

## Containerized Testing Gates

### Container Build Gate

- `docker build` for controller and workspace images MUST succeed
- `docker run --rm <image> axiom --help` MUST print usage
- `docker run --rm <workspace> opencode --version` MUST print version

### Compose Validation

- `docker compose config` MUST validate without errors

### Containerized E2E Smoke

1. `docker compose up -d` starts control plane
2. Health check responds within startup timeout
3. Work item submitted via API dispatches to workspace
4. Workspace runs to completion, evidence written
5. Workspace torn down, `docker compose down` clean

### Test Isolation

- Docker-dependent tests: `@pytest.mark.docker` (skippable)
- Credential-dependent tests: `@pytest.mark.credential_gated` (skip gracefully)
- Containerized tests use isolated Docker networks and unique container name prefixes

---

## Agent Checklists

### Setting Up Local Dev

- [ ] Python 3.11+ installed
- [ ] Venv created and activated
- [ ] Dev deps installed: `pip install -e ".[dev]"`
- [ ] Imports work: `python -c "from axiom.shared.core.log import get_logger; print('ok')"`
- [ ] Tests run: `pytest tests/unit/ -v --tb=short`

### Adding a New Test

- [ ] File named `test_<module>.py` in correct subdirectory
- [ ] Functions named `test_<behavior_under_test>`
- [ ] Docstring references spec
- [ ] Uses `tmp_path` for file I/O
- [ ] Uses `httpx_mock` for network calls (unit tests)
- [ ] Docker-dependent: `@pytest.mark.docker`
- [ ] Credential-dependent: `@pytest.mark.credential_gated`
- [ ] Fixture files added if testing new commands
- [ ] Coverage >= 80% maintained
