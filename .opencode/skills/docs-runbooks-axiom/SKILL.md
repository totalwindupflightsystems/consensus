---
name: docs-runbooks-axiom
description: >
  Operational procedure documentation, troubleshooting flowcharts, recovery runbooks,
  and onboarding runbooks for AI-assisted development teams. Load this skill when
  creating runbooks, writing operational docs, generating troubleshooting guides, or
  linking documentation to alerts and SLOs. Designed for the @docs-runbooks-axiom
  agent but usable by any agent that produces operational documentation.
license: MIT
compatibility: opencode
metadata:
  version: "1.0"
  primary_spec: specs/00-PRD.md
  supporting_specs:
    - specs/34-Observability-And-Metrics.md
    - specs/46-Broken-Arrow-Emergency-Swarm.md
    - specs/25-Structured-Logging-Events.md
  agents:
    - docs-runbooks-axiom
    - sre-ops-axiom
    - sitrep-axiom
  integrates_with:
    - enterprise-release-quality
    - chaos-engineer-axiom
    - security-review-axiom
    - axiom-structured-logging-events
tags:
  vertical: [sre, writing]
  category: writing
  core: false
---

# Docs and Runbooks Skill (Portable)

> **"A runbook that cannot be followed by an on-call engineer at 3 AM is not a runbook. A runbook that cannot be followed by an AI agent is not machine-readable."**

This skill provides structured workflows for creating, maintaining, and validating operational documentation and runbooks. It is designed for the `@docs-runbooks-axiom` agent but can be loaded by any agent that produces operational documentation.

## When to Load This Skill

Load this skill when:
- Creating a new runbook for an alert, incident type, or operational procedure
- Writing operational documentation (deployment, configuration, troubleshooting)
- Generating documentation from specs, incident postmortems, or log patterns
- Linking runbooks to alerts, SLOs, or trace markers
- Validating that existing runbooks are complete, accurate, and verifiable
- Responding to a `@docs-runbooks-axiom` injection from another agent
- Reviewing runbook coverage as part of the release pipeline (Gate 4 per `enterprise-release-quality`)
- Creating onboarding documentation for new team members or new repos

## Core Principles

1. **Every step must be verifiable.** A runbook step without a verification command is an unverifiable claim.
2. **Every rollback must be tested.** A rollback procedure that has never been executed is a hope, not a plan.
3. **Machine-readable by default.** Runbooks must be structured so AI agents can follow them, not just humans.
4. **Linked to the system.** Every runbook must link to the alerts, SLOs, specs, and trace markers it serves.
5. **No contradiction with specs.** Documentation must not contradict `specs/`. If it does, the spec is authoritative and the doc must be updated.
6. **Living documents.** Runbooks are updated after every incident that reveals a gap. Stale runbooks are worse than no runbooks.

---

## Runbook Types

### Type 1: Operational Procedures

Standard operating procedures for routine tasks.

**Examples**: Deployment, scaling, configuration changes, secret rotation, backup/restore.

**Trigger**: Scheduled or on-demand by operator.

### Type 2: Troubleshooting Guides

Diagnostic flowcharts for known problem categories.

**Examples**: Service won't start, health check failing, performance degradation, integration errors.

**Trigger**: Alert fires or user reports a problem.

### Type 3: Incident Response

Step-by-step procedures for responding to incidents.

**Examples**: Security breach, data loss, service outage, cascading failure.

**Trigger**: Incident declared (manual or automatic per `specs/46-Broken-Arrow-Emergency-Swarm.md`).

### Type 4: Onboarding

Guides for new team members, new repos, or new environments.

**Examples**: Developer setup, repo onboarding, environment provisioning, tool installation.

**Trigger**: New team member joins or new repo is created.

---

## Runbook Template (Required Sections)

Every runbook MUST include these sections. Optional sections are marked.

```markdown
---
mb:
  type: runbook
  title: "<Runbook Title>"
  created: YYYY-MM-DDTHH:MM:SSZ
  updated: YYYY-MM-DDTHH:MM:SSZ
  tags: [runbook, <type>, <service>, <alert-name>]
  links:
    alert: <alert name or path>
    spec: <spec path>
    slo: <SLO reference> (optional)
    trace: <trace marker reference> (optional)
    work_item: <work item ID> (optional)
runbook_type: operational | troubleshooting | incident-response | onboarding
severity: critical | high | medium | low
last_validated: YYYY-MM-DDTHH:MM:SSZ
validated_by: <agent or human>
---

# <Runbook Title>

## Purpose

<1-3 sentences: what this runbook is for and when to use it.>

## Scope

<What this runbook covers and does not cover. Link to related runbooks for out-of-scope topics.>

## Prerequisites

<What must be true before starting this runbook.>

- [ ] Access to <system/tool/environment>
- [ ] Permissions: <specific permissions needed>
- [ ] Tools installed: <list of required tools>
- [ ] Knowledge of: <concepts the reader must understand>

## Steps

### Step 1: <Action Title>

**Action**: <What to do>

**Command**:
```bash
<exact command to run>
```

**Expected output**:
```
<what you should see if the step succeeds>
```

**If this fails**:
- <What to check>
- <Alternative approach>
- <When to escalate>

### Step 2: <Action Title>

<repeat pattern>

## Verification

<How to confirm the runbook achieved its goal.>

**Verification command**:
```bash
<exact command to verify success>
```

**Expected result**:
```
<what success looks like>
```

## Rollback

<How to undo what this runbook did, if needed.>

### Rollback Steps

1. <Step 1>
2. <Step 2>

### Rollback Verification

```bash
<command to verify rollback succeeded>
```

## Escalation

<When and how to escalate if this runbook does not resolve the issue.>

| Condition | Escalation Target | Method |
|---|---|---|
| <condition> | <team/person/agent> | <Slack/PagerDuty/Jira/etc.> |

## History

| Date | Change | Author |
|---|---|---|
| <date> | Created | <author> |
| <date> | Updated after incident <ID> | <author> |
```

---

## Generating Runbooks From Sources

### From Specs

When a spec defines alerts (per `specs/34-Observability-And-Metrics.md` REQ-OBS-RUNBOOK-001), generate a runbook for each alert:

1. Read the alert definition (condition, severity, recommended action).
2. Create a troubleshooting runbook using the template above.
3. Fill in the Steps section with diagnostic commands derived from the alert's source metrics/events.
4. Fill in the Verification section with the inverse of the alert condition.
5. Link the runbook to the alert via the `links.alert` field.
6. Store in `.axiom/runbooks/<alert-name-kebab-case>.md` per REQ-OBS-RUNBOOK-001.

**Alert-to-runbook mapping** (from `specs/34-Observability-And-Metrics.md`):

| Alert | Runbook Path |
|---|---|
| `CodeOpsStepDurationHigh` | `.axiom/runbooks/axiom-step-duration-high.md` |
| `CodeOpsVerificationFailureRateHigh` | `.axiom/runbooks/axiom-verification-failure-rate-high.md` |
| `CodeOpsPodOOMKilled` | `.axiom/runbooks/axiom-pod-oom-killed.md` |
| `CodeOpsQueueDepthHigh` | `.axiom/runbooks/axiom-queue-depth-high.md` |
| `CodeOpsWorkItemStuck` | `.axiom/runbooks/axiom-work-item-stuck.md` |
| `CodeOpsEscalationRate` | `.axiom/runbooks/axiom-escalation-rate.md` |
| `CodeOpsOpenCodeHealthFailed` | `.axiom/runbooks/axiom-opencode-health-failed.md` |
| `CodeOpsSnapshotCaptureFailed` | `.axiom/runbooks/axiom-snapshot-capture-failed.md` |
| `CodeOpsSnapshotRestoreFailed` | `.axiom/runbooks/axiom-snapshot-restore-failed.md` |
| `CodeOpsSnapshotSizeExceeded` | `.axiom/runbooks/axiom-snapshot-size-exceeded.md` |
| `CodeOpsSnapshotGCNotRunning` | `.axiom/runbooks/axiom-snapshot-gc-not-running.md` |
| `CodeOpsSnapshotChecksumMismatch` | `.axiom/runbooks/axiom-snapshot-checksum-mismatch.md` |
| `CodeOpsBenchmarkScoreRegression` | `.axiom/runbooks/axiom-benchmark-score-regression.md` |
| `CodeOpsSubscriptionHealthDegraded` | `.axiom/runbooks/axiom-subscription-health-degraded.md` |

### From Incident Postmortems

After an incident (including Broken Arrow incidents per `specs/46-Broken-Arrow-Emergency-Swarm.md`):

1. Read the incident report (root cause, timeline, fix applied).
2. Extract the diagnostic steps that led to root cause identification.
3. Create or update a troubleshooting runbook with those steps.
4. Add the incident as a "History" entry in the runbook.
5. If the incident revealed a gap in an existing runbook, update that runbook.

### From Log Patterns

When structured log analysis reveals recurring error patterns:

1. Identify the recurring `event_type` and `error_class` combination.
2. Create a troubleshooting runbook for that pattern.
3. Include log query commands in the Steps section.
4. Link to the relevant structured logging event definition in `specs/25-Structured-Logging-Events.md`.

---

## Machine-Readable Runbook Format

For AI agents to follow runbooks, the format must be structured and unambiguous.

### Requirements for Machine-Readable Runbooks

1. **Steps are numbered and atomic.** Each step does exactly one thing.
2. **Commands are in fenced code blocks with language tags.** Agents extract and execute these.
3. **Expected output is in fenced code blocks.** Agents compare actual output to expected.
4. **Conditional branches use explicit if/then.** "If this fails" sections have clear conditions.
5. **Variables use `<angle-bracket>` placeholders.** Agents substitute from context.
6. **No ambiguous prose in action steps.** "Check the logs" is not a step. "Run `kubectl logs -n axiom <pod-name> --tail=100 | grep ERROR`" is a step.

### Machine-Readable Step Format

```markdown
### Step N: <Title>

**Action**: <one-sentence description>
**Requires**: <prerequisites from earlier steps, if any>

**Command**:
```bash
<exact command with <placeholders> for variables>
```

**Expected output** (success):
```
<pattern or exact text>
```

**Expected output** (known failure):
```
<pattern that indicates a known failure mode>
```
**If known failure**: Go to Step <M> (or: Escalate per Escalation section)

**Expected output** (unknown failure):
```
<anything not matching success or known failure>
```
**If unknown failure**: Capture output, escalate per Escalation section.
```

---

## Documentation Types

### User Documentation

For end users of the system (developers using Axiom).

| Doc Type | Location | Content |
|---|---|---|
| Getting started | `docs/getting-started.md` or repo README | Installation, first run, basic workflow |
| Configuration reference | `docs/configuration.md` | All config options with defaults and examples |
| CLI reference | `docs/cli.md` | All commands, flags, examples |
| FAQ | `docs/faq.md` | Common questions and answers |

### Operator Documentation

For operators running Axiom in production.

| Doc Type | Location | Content |
|---|---|---|
| Deployment guide | `docs/ops/deployment.md` | How to deploy, configure, and verify |
| Runbooks | `.axiom/runbooks/` | Alert response, troubleshooting, incident response |
| Monitoring guide | `docs/ops/monitoring.md` | Dashboards, alerts, log queries |
| Scaling guide | `docs/ops/scaling.md` | How to scale, capacity planning |

### API Documentation

For consumers of the Axiom API.

| Doc Type | Location | Content |
|---|---|---|
| OpenAPI spec | `openapi.json` | Machine-readable API contract |
| API guide | `docs/api/` | Human-readable API usage guide |
| SSE event reference | `docs/api/events.md` | SSE event types and schemas |

### Architecture Documentation

For developers working on Axiom itself.

| Doc Type | Location | Content |
|---|---|---|
| Architecture overview | `specs/01-Architecture.md` | System topology, components, data flow |
| Code layout | `specs/19-Code-Layout.md` | Where code lives and why |
| Decision log | `.memory-bank/decisionLog.md` | Architectural decisions and rationale |

---

## Linking Runbooks to System Artifacts

Every runbook MUST be linked to at least one system artifact. Orphan runbooks are undiscoverable.

### Link Types

| Link Target | How to Link | Why |
|---|---|---|
| Alert | `links.alert` in frontmatter + alert `annotations.runbook_url` | On-call finds runbook from alert |
| SLO | `links.slo` in frontmatter | Runbook supports SLO maintenance |
| Spec | `links.spec` in frontmatter | Runbook implements spec requirement |
| Trace marker | `links.trace` in frontmatter | Runbook covers traced behavior |
| Work item | `links.work_item` in frontmatter | Runbook created for specific work |
| Incident | History section entry | Runbook updated after incident |

### Discoverability Rules

1. Alert-linked runbooks MUST be in `.axiom/runbooks/` with kebab-case naming matching the alert name.
2. Onboarding runbooks MUST be linked from the repo README or `docs/getting-started.md`.
3. All runbooks MUST be indexed in `.axiom/runbooks/README.md` (or `_index.md`).
4. Runbooks referenced by specs MUST use the exact path specified in the spec.

---

## Quality Checklist

Run this checklist for every runbook before it is considered complete.

### Content Quality

- [ ] Every step has an exact command (no "check the logs" without a command)
- [ ] Every step has expected output (success and failure)
- [ ] Every step has a "if this fails" section
- [ ] Rollback steps are present and have verification commands
- [ ] Escalation paths are defined with specific targets and methods
- [ ] Prerequisites are listed and verifiable
- [ ] No steps require undocumented tribal knowledge

### Accuracy

- [ ] All commands have been tested and produce the documented output
- [ ] All file paths are correct and exist (or are clearly marked as "to be created")
- [ ] No contradiction with `specs/` (if conflict found, update the doc, not the spec)
- [ ] Version/tool requirements are current

### Linkage

- [ ] Linked to at least one system artifact (alert, SLO, spec, trace marker)
- [ ] Indexed in `.axiom/runbooks/README.md` or equivalent
- [ ] Cross-referenced from related runbooks
- [ ] `last_validated` date is within 90 days (or marked as "needs revalidation")

### Machine-Readability

- [ ] Steps are numbered and atomic
- [ ] Commands are in fenced code blocks with language tags
- [ ] Variables use `<angle-bracket>` placeholders
- [ ] Conditional branches are explicit (not buried in prose)
- [ ] An AI agent could follow this runbook without human interpretation

---

## Integration with Other Agents and Skills

### With `@sre-ops-axiom`

- `@sre-ops-axiom` identifies operational needs (alerts, SLOs, scaling requirements)
- `@docs-runbooks-axiom` creates the runbooks and documentation
- Both agents validate runbooks together: SRE validates technical accuracy, docs validates completeness and readability

### With `@chaos-engineer-axiom`

- Chaos experiments validate runbooks under failure conditions
- After each chaos experiment, update the relevant runbook with lessons learned
- Runbook validation is a required output of chaos experiments (see `chaos-engineer-axiom` skill)

### With `enterprise-release-quality`

- Gate 4 (Release Quality Gate) requires "Runbooks reviewed and updated"
- This skill provides the checklist and template for that gate
- Missing runbooks for new alerts block release per REQ-OBS-RUNBOOK-001

### With `@security-review-axiom`

- Security incidents require incident response runbooks
- Security review may inject `@docs-runbooks-axiom` to create security-specific runbooks
- Runbooks for security alerts must be reviewed by `@security-review-axiom`

### With Broken Arrow (`specs/46-Broken-Arrow-Emergency-Swarm.md`)

- Broken Arrow incidents produce findings that should be converted to runbooks
- The Broken Arrow Report's "Recommended Fix Steps" section is a runbook seed
- Post-incident, `@docs-runbooks-axiom` creates or updates runbooks from the incident findings

---

## Non-Negotiables

These rules are absolute and cannot be overridden:

1. **No runbooks with unverifiable steps.** Every step must have a command and expected output. "Check that it works" is not a step.
2. **No docs that contradict specs.** `specs/` is the source of truth. If a doc contradicts a spec, update the doc.
3. **No untested rollback procedures.** A rollback that has never been executed is not a rollback. Mark untested rollbacks as "UNTESTED — validate before relying on this."
4. **No orphan runbooks.** Every runbook must be linked to at least one system artifact and indexed.
5. **No fabricated verification.** "Runbook validated" means someone (human or agent) followed the steps and they worked. Do not claim validation without evidence.
6. **No secrets in runbooks.** Use `<PLACEHOLDER>` for secrets. Never include actual tokens, passwords, or keys.
7. **Machine-readable is not optional.** If an AI agent cannot follow the runbook, it is incomplete.

---

## How to Use This Skill

### As `@docs-runbooks-axiom`

1. Load this skill at the start of every documentation task.
2. Identify the documentation type (runbook, user doc, operator doc, API doc, architecture doc).
3. For runbooks: use the template, fill all required sections, run the quality checklist.
4. For other docs: follow the documentation type guidelines above.
5. Link all documentation to system artifacts.
6. Index new documentation in the appropriate location.

### As Any Other Agent

1. Load this skill when you need to create or update operational documentation.
2. Use the runbook template for any operational procedure you document.
3. If you identify a missing runbook (e.g., new alert without a runbook), inject a step for `@docs-runbooks-axiom`.

### Injecting Documentation Steps

When another agent identifies a documentation gap, inject:

```yaml
injected_step:
  title: "Runbook required for <alert/procedure>"
  agent: "@docs-runbooks-axiom"
  objective: "Create runbook for <alert/procedure> using standard template with all required sections"
  verification: "Runbook passes quality checklist; all steps have commands and expected output"
  trace_refs:
    spec: specs/34-Observability-And-Metrics.md
    plan: <current plan step>
```

---

## References

- `specs/00-PRD.md` — Product requirements, observability NFR
- `specs/34-Observability-And-Metrics.md` — Metrics, alerts, REQ-OBS-RUNBOOK-001 (alert-to-runbook linkage)
- `specs/46-Broken-Arrow-Emergency-Swarm.md` — Emergency swarm, incident response
- `specs/25-Structured-Logging-Events.md` — Structured logging events, log patterns
- `.opencode/skills/enterprise-release-quality/SKILL.md` — Release quality gates (Gate 4)
- `.opencode/skills/chaos-engineer-axiom/SKILL.md` — Chaos engineering, runbook validation

---

axiom:trace work_item=docs-runbooks-axiom spec=specs/34-Observability-And-Metrics.md plan= prompt=.opencode/skills/docs-runbooks-axiom/SKILL.md evidence= doc= test= commit=
