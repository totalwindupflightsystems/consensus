---
name: multi-repo-coordinator-axiom
description: >
  Cross-repo dependency management, workspace-level planning, consistent trace markers across
  repos, unified CI/CD pipeline coordination, and cross-repo drift detection. Load this skill
  when working in a multi-repo workspace or coordinating changes across multiple repositories.
license: MIT
compatibility: opencode
metadata:
  version: "1.0"
  created: "2026-02-27"
  primary_spec: specs/40-Multi-Repo-Workspace.md
  secondary_specs:
    - specs/00-PRD.md
    - specs/07-Mission-North-Star.md
    - specs/21-Traceability-Doctrine.md
tags:
  vertical: [coding, ops]
  category: development
  core: false
---

# Multi-Repo Coordinator Skill (Portable)

> **"Never merge a cross-repo change without verifying all dependent repos."**
>
> **"A workspace is only as consistent as its weakest cross-repo link."**

This skill provides portable guidance for coordinating work across multiple repositories
in a Axiom workspace. It covers dependency management, workspace-level planning, consistent
traceability, CI/CD coordination, and cross-repo drift detection.

---

## Activation

Load this skill when:
- Working in a multi-repo workspace (detected by `workspace.yaml` at workspace root)
- Planning work that spans two or more repositories
- Managing cross-repo API dependencies
- Setting up or validating workspace-level CI/CD pipelines
- Detecting cross-repo drift (when repos diverge from workspace contracts)
- Onboarding a new repo into an existing workspace
- Coordinating a release that involves multiple repos

---

## Non-Negotiables

1. **Never merge a cross-repo change without verifying all dependent repos.** If repo A's
   API change breaks repo B, both repos must be verified before either is merged.

2. **Fail-closed on cross-repo breaking changes.** A breaking change in one repo that affects
   another repo MUST be coordinated. Uncoordinated breaking changes are BLOCKED.

3. **Trace markers must include `repo=<slug>`.** In workspace mode, ALL trace markers MUST
   include the `repo=` field per `specs/40-Multi-Repo-Workspace.md` REQ-MR-005.

4. **Write to the correct repo.** Never write specs, memory bank files, or code to the
   workspace root aggregation directories. Always write to `repos/<slug>/...`.

5. **One `workspace.yaml`, one source of truth.** The workspace manifest is the single
   source of truth for repo membership. Do not maintain parallel lists.

---

## Workspace Structure

### Canonical Layout

```
my-workspace/                          # NOT a git repo
  workspace.yaml                       # manifest (repo list + slugs)
  opencode.jsonc                       # single OpenCode config
  .opencode/                           # single agent/command/skill set
  AGENTS.md                            # workspace-level rules
  specs/                               # aggregated (symlinks)
    frontend/ -> repos/frontend/specs/
    backend/ -> repos/backend/specs/
    README.md                          # workspace-level inventory
  .memory-bank/                        # aggregated (symlinks)
    frontend/ -> repos/frontend/.memory-bank/
    backend/ -> repos/backend/.memory-bank/
    _index.md                          # workspace-level index
    _prompt.md                         # workspace-level rules
  repos/                               # member repos (each is a git repo)
    frontend/
    backend/
```

### Key Invariants (from spec)

- **INV-WS-001**: One `.opencode/` and one `opencode.jsonc` at workspace root
- **INV-WS-002**: Each repo retains its own `specs/` and `.memory-bank/`
- **INV-WS-003**: Each repo retains its own `.axiom/axiom.config.yaml`
- **INV-WS-004**: `workspace.yaml` is required at workspace root
- **INV-WS-005**: Symlinks point to member repo directories only

---

## Cross-Repo Dependency Management

### Identifying Dependencies

Cross-repo dependencies exist when:
- Repo A calls repo B's API
- Repo A imports repo B's shared library
- Repo A's config references repo B's service
- Repo A's data model is consumed by repo B
- Repo A's spec defines a contract that repo B implements

### Dependency Documentation

Document cross-repo dependencies in each repo's specs:

```markdown
<!-- In repos/frontend/specs/03-API-Client.md -->
## External Dependencies

| Dependency | Repo | Contract | Version |
|-----------|------|----------|---------|
| Billing API | backend | specs/10-API-Contract.md | v1 |
| Auth service | api-gateway | specs/05-Auth.md | v2 |
```

And in the workspace-level `specs/README.md`:

```markdown
## Cross-Repo Contracts

| Provider Repo | Consumer Repo | Contract | Notes |
|--------------|---------------|----------|-------|
| backend | frontend | Backend API v1 | frontend calls backend REST API |
| api-gateway | backend, frontend | Auth tokens | JWT validation |
```

### Dependency Verification

```bash
# For each cross-repo dependency, verify the contract is satisfied
# Example: frontend depends on backend API

# 1. Start backend
cd repos/backend && axiom serve --port 8200 &
sleep 3

# 2. Run frontend's contract tests against backend
cd repos/frontend && pytest tests/contract/test_backend_api.py \
  --backend-url http://127.0.0.1:8200 \
  2>&1 | tee /tmp/cross-repo-contract.txt

# 3. Cleanup
kill %1
```

---

## Workspace-Level Planning

### Cross-Repo Work Items

A cross-repo work item uses a single `work_item_id` across all repos:

```yaml
# In repos/backend/.memory-bank/work-items/PROJ-100/meta-planning.md
work_item_id: PROJ-100
repos_affected:
  - slug: backend
    changes: "Add billing API endpoint"
    dependencies: []
  - slug: frontend
    changes: "Add billing UI page"
    dependencies:
      - repo: backend
        reason: "Frontend calls billing API; backend must be deployed first"
```

### Planning Rules (from spec)

- **REQ-MR-001**: Planning artifacts stored in each repo that the work item touches
- **REQ-MR-002**: Consistent `work_item_id` across all repos
- **REQ-MR-003**: Cross-repo dependencies documented in each repo's meta-planning
- **REQ-MR-004**: Evidence bundles are per-repo
- **REQ-MR-012**: Execute dependent steps sequentially (no cross-repo parallelism in v1)

### Cross-Repo Plan Template

```markdown
# Cross-Repo Plan: <work_item_id>

## Repos Affected
| Repo | Changes | Dependencies |
|------|---------|-------------|
| backend | Add billing API | None |
| frontend | Add billing UI | backend API must be ready |

## Execution Order
1. **backend** Phase 1: Implement billing API
2. **backend** Phase 2: Verify billing API (contract tests)
3. **frontend** Phase 1: Implement billing UI (depends on backend Phase 2)
4. **frontend** Phase 2: Verify billing UI (integration tests against backend)

## Cross-Repo Verification
- [ ] Backend API contract tests pass
- [ ] Frontend integration tests pass against live backend
- [ ] Both repos' full test suites pass
```

---

## Consistent Trace Markers

### Format in Workspace Mode

All trace markers in workspace mode MUST include `repo=<slug>`:

```
axiom:trace work_item=PROJ-100 repo=backend spec=specs/10-API-Contract.md plan=phase-1/task-1/step-1
```

### Cross-Repo References

When referencing artifacts in another repo, use a separate trace line:

```typescript
// In repos/frontend/src/api-client.ts

// axiom:trace work_item=PROJ-100 repo=frontend spec=specs/03-API-Client.md plan=phase-1/task-1/step-1
// axiom:trace work_item=PROJ-100 repo=backend spec=specs/10-API-Contract.md  <-- cross-repo dependency
export async function fetchBillingReport(): Promise<Report> {
  // ...
}
```

### Rules (from spec)

- **REQ-MR-005**: ALL trace markers include `repo=<slug>` in workspace mode
- **REQ-MR-006**: Use slug from `workspace.yaml`, not full `org/repo`
- **REQ-MR-007**: Spec paths are relative to member repo root
- **REQ-MR-008**: Cross-repo references use separate trace lines

---

## Unified CI/CD Pipeline Coordination

### Pipeline Patterns

#### Pattern 1: Fan-Out (Independent Repos)

```yaml
# .github/workflows/workspace-ci.yaml (at workspace root)
name: Workspace CI
on: push

jobs:
  backend:
    runs-on: ubuntu-latest
    defaults:
      run:
        working-directory: repos/backend
    steps:
      - uses: actions/checkout@v4
      - run: pytest tests/ -q

  frontend:
    runs-on: ubuntu-latest
    defaults:
      run:
        working-directory: repos/frontend
    steps:
      - uses: actions/checkout@v4
      - run: npm test
```

#### Pattern 2: Fan-Out + Fan-In (Dependent Repos)

```yaml
name: Workspace CI
on: push

jobs:
  backend-test:
    runs-on: ubuntu-latest
    defaults:
      run:
        working-directory: repos/backend
    steps:
      - uses: actions/checkout@v4
      - run: pytest tests/ -q

  frontend-test:
    runs-on: ubuntu-latest
    defaults:
      run:
        working-directory: repos/frontend
    steps:
      - uses: actions/checkout@v4
      - run: npm test

  cross-repo-integration:
    needs: [backend-test, frontend-test]
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - name: Start backend
        run: cd repos/backend && axiom serve --port 8200 &
      - name: Run frontend integration tests
        run: cd repos/frontend && pytest tests/integration/ --backend-url http://127.0.0.1:8200
```

#### Pattern 3: Dependency-Ordered (Sequential)

```yaml
name: Workspace CI (Sequential)
on: push

jobs:
  backend:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - run: cd repos/backend && pytest tests/ -q
      - run: cd repos/backend && axiom serve --port 8200 &
      - run: sleep 3
      - run: curl -sf http://127.0.0.1:8200/health

  frontend:
    needs: [backend]
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - run: cd repos/frontend && npm test
```

### CI/CD Coordination Rules

1. **Per-repo tests first**: Each repo's tests must pass independently
2. **Cross-repo tests second**: Integration tests run after per-repo tests pass
3. **Dependency ordering**: If repo A depends on repo B, B's tests run first
4. **Fail-fast**: If any repo's tests fail, skip cross-repo integration
5. **Evidence per-repo**: Each repo produces its own test evidence

---

## Cross-Repo Spec Alignment

### When Specs Affect Multiple Repos

When a spec change in repo A affects repo B:

1. **Identify impact**: Check the workspace-level `specs/README.md` for cross-repo contracts
2. **Notify**: Create a work item or note in repo B's memory bank
3. **Coordinate**: Plan the changes together (cross-repo work item)
4. **Verify**: Run cross-repo contract tests after both changes

### Spec Propagation Workflow

```
1. Spec change proposed in repo A
2. Check: does this spec define a cross-repo contract?
   - If yes: identify all consumer repos
   - Create cross-repo work item
   - Plan changes in dependency order
3. Implement in provider repo (A) first
4. Update consumer repos (B, C, ...) to match
5. Run cross-repo verification
```

---

## Workspace-Level Memory Bank

### How Memory Bank Works in Multi-Repo Mode

Per `specs/40-Multi-Repo-Workspace.md` ADR-MR-001:

- **Workspace-level `.memory-bank/_prompt.md`**: Guardrail for write locations
- **Member repo `.memory-bank/_prompt.md`**: Repo-local formatting and conventions
- Agents MUST read BOTH prompts in workspace mode

### Write Location Rules

| What | Where to Write | NOT Here |
|------|---------------|----------|
| Repo-specific evidence | `repos/<slug>/.memory-bank/work-items/...` | `.memory-bank/<slug>/...` |
| Repo-specific plans | `repos/<slug>/.memory-bank/implementation-plans/...` | `.memory-bank/<slug>/...` |
| Repo-specific TODO | `repos/<slug>/.memory-bank/TODO.md` | `.memory-bank/TODO.md` |
| Workspace-level index | `.memory-bank/_index.md` | `repos/<slug>/.memory-bank/_index.md` |
| Workspace-level rules | `.memory-bank/_prompt.md` | `repos/<slug>/.memory-bank/_prompt.md` |

---

## Cross-Repo Drift Detection

### What is Cross-Repo Drift?

Drift occurs when repos diverge from the workspace contract:
- Repo A's API changes but repo B's client code doesn't update
- Repo A's spec is updated but repo B's implementation doesn't match
- Repo A's config schema changes but repo B's config is stale

### Detection Methods

#### Method 1: Contract Test Suite

```bash
# Run cross-repo contract tests
for consumer in frontend api-gateway; do
  cd repos/$consumer
  pytest tests/contract/ --provider-url http://127.0.0.1:8200 \
    2>&1 | tee /tmp/drift-$consumer.txt
  cd ../..
done
```

#### Method 2: Spec Comparison

```bash
# Compare API specs across repos
# Provider's spec
cat repos/backend/openapi.json | python3 -m json.tool > /tmp/provider-api.json

# Consumer's expected spec
cat repos/frontend/specs/backend-api-expected.json | python3 -m json.tool > /tmp/consumer-expected.json

# Diff
diff /tmp/provider-api.json /tmp/consumer-expected.json
```

#### Method 3: Workspace Validation Script

```bash
# Validate workspace consistency
python3 .axiom/scaffold/workspace-setup.py --validate \
  2>&1 | tee /tmp/workspace-validation.txt
```

### Drift Response

When drift is detected:

1. **Classify**: Is it a breaking change or a non-breaking addition?
2. **Assign**: Create a work item to resolve the drift
3. **Prioritize**: Breaking drift is urgent; non-breaking drift is planned
4. **Fix**: Update the drifted repo(s) to match the contract
5. **Prevent**: Add cross-repo contract tests to CI

---

## Workspace Setup and Validation

### Initial Setup

```bash
# Create workspace
mkdir my-workspace && cd my-workspace

# Create workspace.yaml
cat > workspace.yaml << 'EOF'
version: 1
workspace:
  name: "my-project"
repos:
  - slug: "frontend"
    path: "repos/frontend"
    url: "https://github.com/org/frontend.git"
  - slug: "backend"
    path: "repos/backend"
    url: "https://github.com/org/backend.git"
EOF

# Clone repos
mkdir -p repos
git clone https://github.com/org/frontend.git repos/frontend
git clone https://github.com/org/backend.git repos/backend

# Run workspace setup
python3 .axiom/scaffold/workspace-setup.py
```

### Validation Checklist

- [ ] `workspace.yaml` exists and is valid
- [ ] All repos listed in manifest exist under `repos/`
- [ ] Symlinks resolve correctly for all repos
- [ ] Workspace-level index files exist
- [ ] No `.opencode/` in member repos (or ignored)
- [ ] Trace markers include `repo=<slug>`

---

## Integration

### Works With

| Skill/Agent | Integration Point |
|-------------|-------------------|
| `axiom-onboarding` | Detects `workspace.yaml` for multi-repo onboarding |
| `traceability-doctrine` | Extended trace format with `repo=` field |
| `api-contract-validator-axiom` | Cross-repo API contract validation |
| `enterprise-release-quality` | Multi-repo release coordination |
| `@sre-ops-axiom` | Cross-repo deploy coordination |
| `migration-guide-generator-axiom` | Cross-repo migration planning |

---

## AI-Assisted Development Risks (2026)

| Risk | Mitigation |
|------|------------|
| AI writes to workspace root instead of member repo | Workspace-level `_prompt.md` guardrail |
| AI forgets `repo=` in trace markers | Workspace mode detection enforces it |
| AI merges cross-repo change without verifying dependents | Non-negotiable: verify all dependent repos |
| AI creates workspace-level TODO (doesn't exist) | Per-repo TODO only (REQ-MR-009) |
| AI runs cross-repo steps in parallel | Sequential only in v1 (REQ-MR-012) |
| AI modifies `workspace.yaml` without instruction | Explicit instruction required |

---

## Anti-Patterns

| Anti-Pattern | Why Bad | Fix |
|-------------|---------|-----|
| Writing to `specs/<slug>/` at workspace root | Writes to symlink target unexpectedly | Write to `repos/<slug>/specs/` |
| Workspace-level TODO.md | No such concept; causes confusion | Per-repo TODO only |
| Cross-repo work without shared work_item_id | Breaks traceability | Use consistent ID (REQ-MR-002) |
| Parallel cross-repo execution | Race conditions, dependency violations | Sequential in v1 |
| Ignoring cross-repo contract tests | Silent drift | Add to CI pipeline |
| Maintaining parallel repo lists | Drift from workspace.yaml | Single source: workspace.yaml |

---

## Trace

`axiom:trace work_item=multi-repo-coordinator-axiom spec=specs/40-Multi-Repo-Workspace.md plan= prompt=.opencode/skills/multi-repo-coordinator-axiom/SKILL.md evidence= doc= test= commit=`
