---
name: enterprise-release-quality
description: Portable enterprise release quality gates, checklists, CI/CD patterns, rollback procedures, and approval matrix. Use this skill when preparing releases, reviewing PRs, setting up CI/CD, or defining quality standards for any project managed by Axiom.
version: "1.0"
tags:
  vertical: [ops, coding]
  category: operations
  core: false
---

# Enterprise Release Quality Standard (Portable)

> **"We can have bad commits, but we cannot have bad releases."**

This is a **project-agnostic** release quality standard. Any project managed by Axiom adopts this skill to ensure every release is fully functional, production-ready, and provides extremely high confidence to the engineering team.

## Core Principles

1. **Zero tolerance for release defects.** Releases must work. Period.
2. **Bad commits are OK.** Development is messy -- CI/CD catches mistakes before they ship.
3. **100% feature completeness.** Every committed feature must work in the release.
4. **Zero regressions.** Existing functionality must continue working after every release.
5. **Evidence-based confidence.** "It works" means "we have proof it works."

## The Release Pipeline (How Bad Commits Become Good Releases)

```
Developer Commit
  -> Tests Run (Tier 0-2: fast feedback)
    -> FAIL? Bad commit -- OK, fix and retry
    -> PASS? CI/CD Pipeline
      -> Integration + Runtime Tests (Tier 3-4)
        -> FAIL? Bad commit -- CI catches it
        -> PASS? Release Candidate
          -> Full E2E + Quality Gates (Tier 5 + all gates)
            -> FAIL? NOT a release -- fix first
            -> PASS? Approved Release -> Deploy
```

The key insight: every stage is a filter. Bad code gets caught early. Only proven code becomes a release.

## Quality Gates (5 Gates)

Every project defines concrete commands for each gate. The gates themselves are universal.

### Gate 1: Code Quality Gate

**When**: before any code is merged (PR level).

**Requirements**:
- [ ] All unit tests pass
- [ ] Type checking passes (if applicable to language)
- [ ] Linting passes
- [ ] No security vulnerabilities (static analysis)
- [ ] Code coverage >= project threshold (recommend >= 85%)
- [ ] No untracked TODO comments without work items
- [ ] Documentation updated for changed behavior

**Project maps this to**: `<your unit test command>`, `<your linter>`, `<your type checker>`, `<your security scanner>`.

### Gate 2: Integration Quality Gate

**When**: before any PR is merged.

**Requirements**:
- [ ] All integration tests pass
- [ ] External service integrations work (or mocks are realistic)
- [ ] Configuration loading works across environments
- [ ] Data layer integration works (DB, cache, queues)
- [ ] No import/dependency conflicts
- [ ] Trace markers generated correctly

**Project maps this to**: `<your integration test command>`.

### Gate 3: End-to-End Quality Gate

**When**: before any release is created.

**Requirements**:
- [ ] All E2E tests pass
- [ ] CLI/API commands work as expected
- [ ] Services start and respond to health checks
- [ ] Cross-service communication works
- [ ] Performance benchmarks met
- [ ] Error handling works correctly

**Target duration**: < 5 minutes (adjust per project).

### Gate 4: Release Quality Gate

**When**: before any release is deployed.

**Requirements**:
- [ ] Release checklist completed (see template below)
- [ ] Changelog generated with all changes documented
- [ ] Version bumped correctly (semver)
- [ ] Release artifacts built and tested (Docker image, package, binary, etc.)
- [ ] Monitoring dashboards updated for new features/metrics
- [ ] Runbooks reviewed and updated

### Gate 5: Production Readiness Gate

**When**: before production deployment.

**Requirements**:
- [ ] Staging environment tested
- [ ] Load testing passed (if applicable)
- [ ] Security audit passed (if applicable)
- [ ] Disaster recovery tested
- [ ] Rollback tested and documented
- [ ] On-call team notified
- [ ] Communication plan ready

## Test Coverage Requirements

| Category | Coverage Target | Max Duration | Priority |
|----------|----------------|--------------|----------|
| Unit Tests | >= 85% | 30 seconds | Critical |
| Integration Tests | 100% critical paths | 1 minute | Critical |
| E2E Tests | 100% critical paths | 5 minutes | Critical |
| Performance Tests | All benchmarks | 2 minutes | High |
| Security Tests | All checks | 1 minute | Critical |
| Smoke Tests | All critical paths | 30 seconds | Critical |

## Critical Paths (Must Be 100% Tested)

Every project has critical paths that must never break. Identify yours and ensure 100% coverage:

1. **Installation path**: install -> verify -> first command works
2. **Configuration path**: config loaded -> validated -> applied correctly
3. **Primary workflow**: main command/API -> processes input -> produces correct output
4. **Service path** (if applicable): server starts -> health check -> API responds -> clean shutdown
5. **Error path**: invalid input -> clear error message -> no crash -> recoverable state

## Release Checklist Template

Copy and customize for your project:

```markdown
## Release Checklist: v<VERSION>

### Code Quality
- [ ] All unit tests pass
- [ ] Type checking passes
- [ ] Linting passes
- [ ] Security scan passes
- [ ] Code coverage >= <threshold>%

### Integration Quality
- [ ] Integration tests pass
- [ ] External service integrations work
- [ ] Configuration works across environments
- [ ] Data layer works

### E2E Quality
- [ ] All E2E tests pass
- [ ] Primary workflows work
- [ ] Services start and respond
- [ ] Error handling works

### Performance
- [ ] Startup time within budget
- [ ] Response times within budget
- [ ] Memory usage within limits
- [ ] No performance regressions

### Security
- [ ] No critical vulnerabilities
- [ ] No high vulnerabilities
- [ ] Secrets not exposed in artifacts
- [ ] Auth/authz works correctly

### Documentation
- [ ] CHANGELOG updated
- [ ] README updated
- [ ] API docs updated (if applicable)
- [ ] Runbooks updated (if applicable)

### Release Artifacts
- [ ] Version bumped
- [ ] Artifacts built (package/image/binary)
- [ ] Artifacts tested in staging

### Sign-off
- [ ] Tech Lead approval
- [ ] Security approval (if security-impacting)
- [ ] Product approval (if user-facing)
```

## Release Approval Matrix

| Change Type | Tech Lead | Security | Product | Eng Manager |
|-------------|-----------|----------|---------|-------------|
| Bug fix | Required | -- | -- | -- |
| Feature | Required | If security impact | Required | -- |
| Infrastructure | Required | Required | -- | Required |
| Security fix | Required | Required | -- | Required |
| Breaking change | Required | Required | Required | Required |

## PR Merge Requirements

A PR can be merged only when:

- All CI checks pass (100% green)
- Code coverage maintained or improved
- No security vulnerabilities introduced
- E2E tests pass (or explicitly scoped out with justification)
- Documentation updated for behavior changes
- At least one approver (for non-trivial changes)
- No unresolved review conversations

## Rollback Procedure

### Rollback Triggers

Rollback is required if any of these occur post-deploy:

- Production incident with this release as root cause
- Critical bug affecting multiple users
- Security vulnerability discovered in release
- Performance regression > 50% from baseline
- Data integrity issue

### Rollback Steps (Generic)

```bash
# 1. Identify last known good release
git tag -l "v*" | sort -V | tail -n 5

# 2. Create rollback branch
git checkout -b rollback-$(date +%Y%m%d)

# 3. Revert bad changes
git revert --no-commit <bad-commit-range>
git commit -m "Rollback: reverting to v<LAST_GOOD>"

# 4. Run rollback verification (at least Tier 3)
<your-test-command>

# 5. Deploy rollback artifact

# 6. Document the rollback
# Create incident report in .memory-bank/incidents/
```

### Rollback Documentation Template

```markdown
# Rollback Incident Report

**Date**: <date>
**Trigger**: <what went wrong>
**Affected Release**: v<BAD>
**Rolled Back To**: v<GOOD>
**Root Cause**: <description>
**Resolution**: <steps taken>
**Prevention**: <what changes to prevent recurrence>
**Follow-up Work Items**: <links>
```

## CI/CD Pipeline Pattern

Structure your pipeline in stages that map to quality gates:

```
Stage 1: quality-gates (Gate 1)
  -> unit tests, linting, type checking, security scan
  -> Runs on: every push, every PR

Stage 2: integration-tests (Gate 2)
  -> integration tests, service integration
  -> Runs on: every PR
  -> Requires: Stage 1 passes

Stage 3: e2e-tests (Gate 3)
  -> full end-to-end tests
  -> Runs on: every PR to main, release branches
  -> Requires: Stage 2 passes

Stage 4: release (Gate 4)
  -> build artifacts, generate changelog, tag release
  -> Runs on: main branch only, after all stages pass
  -> Requires: Stages 1-3 pass
```

## Exception Process

### When Exceptions Are Allowed

- Emergency fix for production outage
- Security vulnerability requiring immediate patch
- Critical third-party dependency issue
- Infrastructure emergency

### Exception Request Template

```markdown
## Exception Request: <Brief Title>

**Requester**: <name>
**Date**: <date>
**Severity**: Critical / High / Medium

### Reason
<Why this exception is needed>

### Risk Assessment
<What could go wrong>

### Mitigation Plan
<How we reduce the risk>

### Approval Required
- Tech Lead: <name/date>
- Security: <name/date> (if applicable)

### Follow-up Actions
<What must be done after the exception -- tests to add, docs to update, etc.>
```

Exceptions are temporary. Every exception must have follow-up actions to close the gap.

## Quality Metrics to Track

| Metric | Target | Why It Matters |
|--------|--------|----------------|
| Test Pass Rate | 100% | Releases must be fully tested |
| Code Coverage | >= 85% | Untested code is risky code |
| E2E Pass Rate | 100% | Workflows must work end-to-end |
| Release Defects | 0 | No bad releases |
| Mean Time to Recovery | < 30 min | Fast rollback when needed |
| Rollback Frequency | Trending down | Fewer rollbacks = better quality |

## Applying This Standard to a New Project

1. **Map your gates**: define concrete commands for each of the 5 quality gates.
2. **Define critical paths**: list the 3-5 workflows that must never break.
3. **Set up CI/CD stages**: configure your pipeline to match the 4-stage pattern.
4. **Create your release checklist**: customize the template for your project.
5. **Define rollback procedure**: document how to roll back your specific deployment.
6. **Set coverage thresholds**: start at 85% and adjust based on project maturity.
7. **Train the team**: everyone knows the gates, everyone follows the checklist.
8. **Review quarterly**: update standards as the project evolves.

## References

For Axiom-specific implementation of this standard, see:
- `.memory-bank/enterprise-release-quality.md`
- `.memory-bank/best-practices/enterprise-grade-testing.md`
- `specs/00-PRD.md`
