---
name: enterprise-testing-standard
description: Portable enterprise-grade tiered testing standard for any project managed by Axiom. Enforces "no bad releases" philosophy with a 6-tier verification hierarchy. Load this skill when planning tests, verifying work items, or assessing release readiness.
version: "1.0"
tags:
  vertical: [coding]
  category: testing
  core: false
---

# Enterprise-Grade Testing Standard (Portable)

> **"We can have bad commits, but we cannot have bad releases."**

This is a **project-agnostic** testing standard. Any project managed by Axiom adopts this skill to set a quality bar where releases are always production-ready. Bad commits happen during development -- CI/CD catches them. Bad releases never ship.

## Core Rules

1. **Module/import tests passing does NOT equal working software.** A green unit test suite is necessary but insufficient.
2. **Every work item must reach at least Tier 3 (runtime execution) before it can be called "done."**
3. **Every release must reach Tier 5 (full end-to-end) before it ships.**
4. **Evidence is mandatory.** If you didn't run it and capture output, it's "unverified."

## Tiered Verification Signal Hierarchy

Every project has layers of confidence. Lower tiers are fast but prove less. Higher tiers are slower but prove the system actually works. You must climb the tiers -- never skip.

| Tier | Name | What It Proves | Required for "Done"? |
|------|------|----------------|----------------------|
| 0 | **Module imports** | Dependencies installed, no syntax errors, module structure valid | No (necessary but insufficient) |
| 1 | **Unit tests** | Business logic correct in isolation, edge cases handled, error paths work | No (isolated testing only) |
| 2 | **Interface tests** | CLI/API surface registered, help/schema available, commands/routes exist | No (interface only) |
| 3 | **Runtime execution** | The system actually runs, processes input, produces output, reaches the execution path | **YES -- MINIMUM** |
| 4 | **Service integration** | Servers start, health checks pass, endpoints respond, components talk to each other | Yes (if service paths touched) |
| 5 | **End-to-end workflow** | Complete user workflow works from input to final output across all components | Yes (for releases) |

### How to Map Tiers to Your Project

Each project defines its own concrete commands per tier. The pattern is:

**Tier 0** -- import/load check:
- Python: `python3 -c "from mypackage import main; print('ok')"`
- Node: `node -e "require('./src'); console.log('ok')"`
- Go: `go build ./...`

**Tier 1** -- unit tests:
- Python: `pytest tests/unit/ -q`
- Node: `npm test`
- Go: `go test ./...`

**Tier 2** -- interface tests:
- CLI: `myapp --help && myapp subcommand --help`
- API: schema validation, OpenAPI spec check
- Library: public API surface smoke test

**Tier 3** -- runtime execution (MINIMUM for "done"):
- CLI app: `myapp run --input test-data` (must reach execution path, not just parse args)
- Server app: start server, send real request, verify response
- Library: integration test that exercises the real code path end-to-end
- **Code quality (Tier 3+ supplement)**: run `axiom analyze --score` alongside runtime execution to produce a health score covering dead code, complexity, and lint. This provides additional Tier-3+ evidence that the code under test is clean, not just passing. Load `code-analysis-axiom` for full usage details.

**Tier 4** -- service integration:
- Start all services, verify health endpoints, test cross-service communication
- Database: run migrations, verify schema, test queries
- External APIs: test with real (or realistic mock) endpoints

**Tier 5** -- end-to-end workflow:
- Complete user journey from start to finish
- All components running together
- Real-world scenario with realistic data

### Anti-Pattern: Import-Only Testing

Do NOT claim a step is complete based solely on:
- Import checks passing (Tier 0)
- Isolated unit tests passing (Tier 1)
- Help text displaying (Tier 2)

These prove the code exists and compiles. They do NOT prove it works. Always include at least one Tier 3+ verification in your evidence.

### Anti-Pattern: Green Theater Testing

> **"95% coverage with 0% signal is worse than 80% coverage with real signal."**

Green theater is when tests make coverage numbers go up without catching real bugs. Per `specs/48-Test-Quality-Gates.md#REQ-TQ-011`, the following patterns MUST be detected and prevented during planning and review:

| Anti-Pattern | Why It's Dangerous | How to Fix |
|---|---|---|
| **Fake executor bypass** — Test uses a stub that returns `"ok"` instead of exercising the real execution path | The core value proposition is never tested; the system could be completely broken and all tests pass | Replace stub with a scripted executor that produces realistic outputs, or use the real executor with a mock backend |
| **Raw HTTP instead of adapter** — Test calls `urllib`/`httpx` directly instead of the adapter code it claims to test | The adapter's request construction, auth headers, error handling, and response parsing are never exercised | Call the actual adapter functions (e.g., `jira.get_issue_by_key()`) with mocked HTTP responses |
| **Coverage padding** — File named `test_*_coverage.py` that hits code paths without asserting behavior | Lines are "covered" but no regression would be caught if the behavior changed | Add specific value assertions or merge into the primary test file with real assertions |
| **Source inspection** — Uses `inspect.getsource()` to verify code structure instead of calling the function | Breaks on refactors without catching bugs; tests the text, not the behavior | Call the function with real inputs and assert on outputs |
| **Weak assertions** — `assert score >= 0` (always true), `assert isinstance(result, dict)` alone | Assertion passes regardless of whether the system works correctly | Assert specific expected values: `assert score == 87`, `assert result["status"] == "ok"` |
| **Accepts both outcomes** — `try: fn(); except Error: pass` | Test passes whether the function succeeds or fails | Split into separate success and failure tests with explicit expectations |
| **Missing negative tests** — Only happy path tested, error/failure paths untested | Error handling could be completely broken without detection | Add tests for each error class: 401, 404, 429, 500, timeout, connection refused |
| **No crash recovery** — Checkpoint write tested but recovery round-trip never exercised | System could lose all progress on crash without detection | Test the full cycle: write checkpoint → simulate crash → read checkpoint → verify resume |
| **Organizational false confidence** — Tests in wrong directory creating misleading coverage signals | Running `pytest tests/unit/` appears to cover a module that's actually tested elsewhere (or not at all) | Ensure test directory structure mirrors source directory structure |
| **Unbounded growth untested** — In-memory stores with no TTL/eviction/max-size tests | Production system will eventually OOM under sustained load | Add tests for TTL expiration, max-size eviction, and memory bounds |

### Test Value Assessment (Required During Planning)

Before writing tests, answer these questions (per `specs/48-Test-Quality-Gates.md#REQ-TQ-011-PLAN`):

1. **What spec requirement does this test verify?** → If none, don't write it.
2. **What real product code does this test call?** → If only mocks/stubs, redesign it.
3. **What would break if this test were deleted?** → If "nothing," don't write it.
4. **What regression would this test catch?** → If none, don't write it.
5. **Does this test match any green theater anti-pattern?** → If yes, redesign it.

Reference: `.memory-bank/best-practices/test-suite-adversarial-patterns.md`

## Test Suites by Context

### Quick Suite (CI fast path)

**When**: every commit, every push.
**Tiers**: 0-2.
**Target duration**: < 30 seconds.
**Purpose**: fast feedback; catch obvious breakage.

### Standard Suite (pre-PR / pre-merge)

**When**: before creating or merging a PR.
**Tiers**: 0-3.
**Target duration**: < 2 minutes.
**Purpose**: prove the change actually works at runtime. This is the minimum bar for claiming work is done.

### Full Suite (pre-release)

**When**: before any release.
**Tiers**: 0-5.
**Target duration**: < 5 minutes (adjust per project complexity).
**Purpose**: prove the entire system works end-to-end. No release ships without this.

## Quality Gates

| Gate | Trigger | Requirement | If Failed |
|------|---------|-------------|-----------|
| Gate 1 | Any code change | Tier 0-2 pass | Cannot proceed to review |
| Gate 2 | Work item completion | Tier 3 passes | Cannot claim "done" |
| Gate 3 | Service-touching change | Tier 4 passes | Cannot merge |
| Gate 4 | Release candidate | Tier 5 passes | Cannot release |
| Gate 5 | Any code change | Test quality gate passes (no assertionless, no tautology) | Cannot claim "done" |

### Test Quality Gate (Gate 5)

High coverage is a **tripwire**, not a quality signal. Gate 5 ensures coverage is backed
by real signal. It is enforced by `check_test_quality.py` (see `specs/48-Test-Quality-Gates.md`).

**Hard failures (zero tolerance)**:
- Assertionless tests: `test_*` functions with no `assert` / `pytest.raises` / `pytest.warns`.
- Tautology tests: tests that have assertions but call no product code (only builtins/inspect/ast).

**Soft failures (ratio thresholds, configurable)**:
- AST-only test ratio > 2%
- Skip/xfail ratio > 5%
- Untraced test ratio > 30%

Run the gate:
```bash
python3 .axiom/scripts/check_test_quality.py \
  --test-dir .axiom/tests \
  --config .axiom/axiom.config.yaml \
  --json > test-quality-report.json
```

Include in evidence bundle:
```
evidence.tests_quality.score
evidence.tests_quality.assertionless
evidence.tests_quality.tautology
evidence.runtime_tier_reached
```

See `.opencode/skills/test-quality-gates-axiom/SKILL.md` for the full workflow.

## Evidence Requirements

Every verification must produce evidence. No evidence = unverified.

### What to Capture

1. **Command executed**: exact command with arguments
2. **Output**: stdout and stderr (or meaningful excerpt)
3. **Exit code**: 0 for success, non-zero for failure
4. **Duration**: how long it took
5. **Trace marker**: `axiom:trace work_item=<ID> test=tier-<N>-<name>`

### Evidence Format

```markdown
## Test Evidence: <test-name>

**Date**: <timestamp>
**Work Item**: <work-item-id>
**Tier**: <0-5>

### Command
\`\`\`bash
<exact command>
\`\`\`

### Output
\`\`\`
<captured output>
\`\`\`

### Exit Code: <code>
### Duration: <time>
### Status: PASS / FAIL
```

### Evidence Storage

Store in the work item's run directory:
```
.memory-bank/work-items/<WORK_ITEM_ID>/runs/<RUN_ID>/verification.md
```

## Trace Markers

Standard trace markers per tier (grep-friendly, stable):

```
axiom:trace work_item=<ID> test=tier-0-import
axiom:trace work_item=<ID> test=tier-1-unit
axiom:trace work_item=<ID> test=tier-2-interface
axiom:trace work_item=<ID> test=tier-3-runtime
axiom:trace work_item=<ID> test=tier-4-service
axiom:trace work_item=<ID> test=tier-5-e2e
```

## Error Message Standards

All user-facing error messages in the project must be:

1. **Clear**: explain what went wrong in plain language
2. **Actionable**: tell the user what to do next
3. **Contextual**: show relevant state (config values, paths, URLs)
4. **Traceable**: include a trace marker or error code for debugging
5. **No jargon**: use terms the user understands

Bad: `Error: Connection failed`
Good: `ERROR: Cannot reach API server at http://localhost:8080. To fix: (1) verify the server is running, (2) check MYAPP_API_URL is correct.`

## Developer Checklists

### Before Committing

- [ ] Tier 0 passes (imports/build)
- [ ] Tier 1 passes (unit tests)
- [ ] Tier 2 passes (interface/help)
- [ ] No new linting errors
- [ ] No new security issues

### Before Creating PR

- [ ] All Tier 0-2 pass
- [ ] Tier 3 passes (runtime execution)
- [ ] Code coverage maintained or improved
- [ ] Documentation updated
- [ ] PR description explains what and why

### Before Release

- [ ] All Tier 0-5 pass
- [ ] Error messages tested
- [ ] Recovery procedures tested
- [ ] Evidence collected and stored
- [ ] Rollback plan documented
- [ ] Changelog updated

## Failure Recovery Patterns

### Stuck Process

Symptoms: no progress for > 5 minutes.
Recovery: check running processes, kill stuck ones, check logs, restart.

### Service Crash

Symptoms: connection refused, health check fails.
Recovery: kill remaining processes, restart fresh, verify health, resume work.

### Corrupted State

Symptoms: unexpected errors, missing files.
Recovery: check integrity, restore from backup if available, recreate if needed.

## Applying This Standard to a New Project

1. **Define your tier commands**: map each tier (0-5) to concrete commands for your stack.
2. **Configure CI/CD**: run Quick Suite on push, Standard Suite on PR, Full Suite on release branch.
3. **Set up evidence storage**: create `.memory-bank/work-items/` structure.
4. **Train the team**: no work item is "done" without Tier 3+ evidence. No release without Tier 5.
5. **Review quarterly**: update tier definitions as the project evolves.

## References

For Axiom-specific tier command examples, see:
- `.memory-bank/best-practices/enterprise-grade-testing.md`
- `AGENTS.md` (Build / Lint / Test Commands section)
- `specs/00-PRD.md#verification-signal-hierarchy`
