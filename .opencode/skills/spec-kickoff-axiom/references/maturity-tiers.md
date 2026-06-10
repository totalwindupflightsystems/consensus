---
title: "Maturity Tiers Reference"
version: "1.0"
created: "2026-02-27"
updated: "2026-02-27"
skill: spec-kickoff-axiom
purpose: >
  Defines what each maturity tier means, what artifacts are required, which review
  agents are appropriate, what is NOT required yet, and the gap checklist to advance
  to the next tier. Used by spec-kickoff-axiom to explain what is missing and to
  set appropriate review depth.
---

# Maturity Tiers Reference

> **Used by**: `.opencode/skills/spec-kickoff-axiom/SKILL.md`
>
> **Rule**: Tiers define the *minimum* set of spec sections and review agents required.
> They are not a fixed checklist — they teach what "more mature" means in context.
> When uncertain, pick the LOWER tier and label missing elements as "Open decisions."

---

## Quick-Reference Table

| Tier | One-liner | Dominant artifact | Minimum review pack | Axiom verification tier |
|---|---|---|---|---|
| `idea` | Intent + value + boundaries | Goal + non-goals + stakeholders | Pack A (assumption-buster) | Tier 0 (module import / doc exists) |
| `concept` | Primary actors + core flows + rough AC | Glossary + flow + rough AC | Pack A | Tier 0–1 |
| `poc` | Narrow demonstrator + explicit shortcuts | Demo scope + success criteria | Pack A | Tier 1 (isolated function / demo) |
| `testing` | Negative cases + verification hooks | Test strategy + invariants | Pack B (standard) | Tier 1–2 |
| `mvp` | Minimal shippable slice + rollout sketch | Operational constraints + support boundary | Pack B | Tier 2–3 |
| `alpha` | Early real users + instrumentation | Error recovery + observability basics | Pack B | Tier 3 (CLI runtime) |
| `beta` | Scale-up readiness + migration plan | Runbooks (draft) + capacity assumptions | Pack B + ops reviewers | Tier 3–4 |
| `production` | SLOs/SLIs + rollback + abuse cases | Incident workflow + risk gates | Pack C (adversarial) | Tier 4–5 |
| `battle-tested` | Adversarial + resilience + cost guardrails | Redteam findings + chaos + cost controls | Pack C + Pack D | Tier 5 (E2E) |

> **Axiom verification tiers** are from `specs/00-PRD.md#verification-signal-hierarchy`.
> Tier 3+ is the minimum for claiming a step "done."

---

## How to Infer the Current Tier (Dynamic)

Infer from what is present and what is missing:

| Signal in source material | Inferred tier |
|---|---|
| Only intent/why, no behavior boundaries | `idea` |
| Primary actor + one core flow described | `concept` |
| Narrow demo scope with explicit shortcuts listed | `poc` |
| Negative cases + verification hooks present | `testing` |
| Shippable slice + rollout constraints described | `mvp` |
| Error recovery + instrumentation basics present | `alpha` |
| Migration/compat + scale assumptions present | `beta` |
| SLOs/rollback/operability + abuse cases present | `production` |
| Adversarial findings integrated + resilience + cost guardrails | `battle-tested` |

**When uncertain**: pick the LOWER tier. Label missing elements as "Open decisions."

---

## Tier: `idea`

### What it means
You have intent, user value, and a rough boundary. You do NOT yet have a primary actor, a concrete flow, or acceptance criteria. This is the "napkin sketch" stage.

### Required artifacts
- [ ] **Goal statement**: one paragraph describing the problem and why it matters
- [ ] **Non-goals**: at least 3 explicit exclusions (prevents scope creep)
- [ ] **Stakeholders**: who cares about this and why
- [ ] **Known unknowns**: list of things you don't know yet
- [ ] **Open decisions**: at least the top 3 blocking questions

### Appropriate review agents
- `assumption-buster-axiom` — surfaces undocumented prerequisites
- `ux-writer-axiom` — only if user-facing (helps clarify value proposition language)

### What is NOT required yet
- Acceptance criteria (too early)
- Technical architecture
- Security posture
- Test strategy
- Rollout plan

### Concrete "done" example
```
Goal: Let developers see which Axiom agents are currently running and their status.
Non-goals: Not a full observability dashboard; not real-time alerting; not mobile.
Stakeholders: Developers using Axiom CLI; tech leads monitoring throughput.
Known unknowns: How many agents run concurrently? What "status" means per agent.
Open decisions: (1) Push vs pull model? (2) CLI-only or also web? (3) Polling interval?
```

### Gap checklist: `idea` → `concept`
- [ ] Name one primary actor (who uses this most?)
- [ ] Describe one primary flow end-to-end (even roughly)
- [ ] Write 3 acceptance criteria (observable outcomes)
- [ ] Resolve or explicitly defer the top 3 open decisions

---

## Tier: `concept`

### What it means
You can describe who uses the system, what they do, and what "success" looks like in rough terms. You have a glossary and core flows. Acceptance criteria exist but may not be fully testable yet.

### Required artifacts
- [ ] **Glossary**: key terms defined (prevents ambiguity in later specs)
- [ ] **Primary actors**: 1–3 actors with their job-to-be-done
- [ ] **Core flows**: 2–5 flows described at step level (not implementation)
- [ ] **Rough acceptance criteria**: 3–8 ACs (may be informal)
- [ ] **Non-goals** (carried from `idea`)
- [ ] **Open decisions**: updated list

### Appropriate review agents
- `assumption-buster-axiom` — still the primary reviewer
- `ux-writer-axiom` — if user-facing, to validate terminology and flow clarity
- `devils-advocate-axiom` — optional; useful if scope is already large

### What is NOT required yet
- Negative test cases
- Security threat model
- Rollout plan
- SLOs/SLIs
- Migration plan

### Concrete "done" example
```
Glossary: Agent (a Axiom subagent process), Run (one execution of a work item), Status (one of: idle/running/blocked/done).
Primary actor: Developer — wants to see which agents are active without reading logs.
Core flow: Developer runs `axiom agents status` → CLI polls /api/v1/agents → renders table with name/status/last-active.
Rough AC: (1) Command exits 0 when no agents running. (2) Shows at least name + status columns. (3) Refreshes on --watch flag.
```

### Gap checklist: `concept` → `poc`
- [ ] Define the explicit demo scope (what will be shown, to whom, when)
- [ ] List what is NOT proven by the demo (explicit shortcuts)
- [ ] Define success criteria for the demo (how do you know it worked?)
- [ ] Identify the riskiest assumption the demo will test

---

## Tier: `poc`

### What it means
You have a narrow demonstrator scope with explicit shortcuts. The goal is to validate the riskiest assumption, not to build a shippable product. Shortcuts are first-class citizens — they must be documented, not hidden.

### Required artifacts
- [ ] **Demo scope**: exactly what will be demonstrated (1–3 scenarios)
- [ ] **Explicit shortcuts**: what is hardcoded, mocked, or skipped
- [ ] **What is NOT proven**: explicit list of things the PoC does not validate
- [ ] **Success criteria**: how you know the PoC succeeded
- [ ] **Demo plan**: who runs it, when, what environment

### Appropriate review agents
- `assumption-buster-axiom` — validates that the PoC actually tests the riskiest assumption
- `devils-advocate-axiom` — challenges whether the PoC scope is too broad

### What is NOT required yet
- Negative test cases
- Error handling beyond happy path
- Security posture
- Operational constraints
- Migration plan

### Concrete "done" example
```
Demo scope: Show that the CLI can display 3 hardcoded agent statuses in a table.
Shortcuts: Agent list is hardcoded (no real API call). Status is always "running". No auth.
NOT proven: Real agent discovery, status accuracy, concurrent updates, error handling.
Success criteria: Demo runs in < 30s, table renders correctly, no crashes.
Demo plan: Run locally on dev laptop, show to team in sprint review.
```

### Gap checklist: `poc` → `testing`
- [ ] Write negative test cases (what happens when input is invalid/missing?)
- [ ] Define a test strategy (unit? integration? manual?)
- [ ] Add verification hooks (how will CI/CD validate this?)
- [ ] Document invariants (what must always be true?)
- [ ] Remove or explicitly carry forward each PoC shortcut

---

## Tier: `testing`

### What it means
You have a verifiable spec. Negative cases are documented. You have a test strategy and verification hooks. Invariants are explicit. This tier is the minimum for a spec that can drive a real implementation.

### Required artifacts
- [ ] **Negative test cases**: at least 3 (invalid input, missing data, error states)
- [ ] **Test strategy**: unit / integration / E2E split; what tools; what environments
- [ ] **Verification hooks**: how CI/CD validates the spec (commands, gates)
- [ ] **Invariants**: what must always be true (pre/post conditions)
- [ ] **Boundary conditions**: edge cases documented
- [ ] **Error taxonomy**: named error states with expected behavior

### Appropriate review agents
- `assumption-buster-axiom`
- `devils-advocate-axiom`
- `security-review-axiom` — if any auth/data/trust boundary exists
- `spec-verifier-axiom` — validates spec completeness and trace consistency

### What is NOT required yet
- Rollout plan
- SLOs/SLIs
- Incident playbooks
- Migration plan
- Cost guardrails

### Concrete "done" example
```
Negative cases: (1) API returns 503 → CLI shows "agents unavailable, retry in 30s". (2) No agents running → shows empty table with "No agents active." (3) --watch with Ctrl+C → exits cleanly.
Test strategy: Unit tests for table rendering; integration test against mock API; manual E2E on dev machine.
Verification hooks: `pytest tests/test_agents_status.py` in CI; `axiom agents status --help` must exit 0.
Invariants: Table always has header row. Exit code 0 on success, non-zero on error.
```

### Gap checklist: `testing` → `mvp`
- [ ] Define what is shipped (exact feature surface)
- [ ] Define to whom it is shipped (user segment, rollout group)
- [ ] Define what happens when it fails (user-facing error + recovery path)
- [ ] Add operational constraints (rate limits, resource limits, support boundaries)
- [ ] Write a rollout sketch (how does it go from 0 to users?)

---

## Tier: `mvp`

### What it means
You have a minimal shippable slice. You know what is shipped, to whom, and what happens when it fails. Operational constraints are documented. There is a rollout sketch. This is the minimum for a real release.

### Required artifacts
- [ ] **Feature surface**: exact scope of what ships (no more, no less)
- [ ] **Target users**: who gets it first (segment, rollout group)
- [ ] **Failure handling**: user-facing error messages + recovery paths
- [ ] **Operational constraints**: rate limits, resource limits, timeouts
- [ ] **Support boundaries**: what is supported vs unsupported
- [ ] **Rollout sketch**: phased rollout or feature flag plan
- [ ] **Rollback plan**: how to revert if something goes wrong

### Appropriate review agents
- `assumption-buster-axiom`
- `devils-advocate-axiom`
- `security-review-axiom`
- `spec-verifier-axiom`

### What is NOT required yet
- Full observability (metrics/alerts)
- Incident playbooks
- SLOs/SLIs
- Migration/compatibility plan
- Adversarial testing

### Concrete "done" example
```
Feature surface: `axiom agents status` command with --watch flag. No web UI.
Target users: Internal developers on the Axiom team only (v1).
Failure handling: API unavailable → "Cannot reach Axiom server. Is it running? Try: axiom serve"
Operational constraints: Max 10 agents displayed. Polling interval: 5s. Timeout: 10s.
Support boundary: Supported on macOS/Linux. Not supported on Windows.
Rollout: Ship behind --experimental flag. Remove flag after 2-week soak.
Rollback: Remove --experimental flag; feature is opt-in so no user impact.
```

### Gap checklist: `mvp` → `alpha`
- [ ] Add instrumentation basics (what metrics/logs prove the feature is working?)
- [ ] Add user-facing error recovery (not just error messages — recovery instructions)
- [ ] Add security posture notes (auth model, data exposure, trust boundary)
- [ ] Define observability baseline (what does "healthy" look like in logs/metrics?)

---

## Tier: `alpha`

### What it means
Real users are using it. You have instrumentation basics, error recovery, and a security posture. You are collecting feedback and fixing issues. This is the "early access" stage.

### Required artifacts
- [ ] **Instrumentation basics**: key metrics (request count, error rate, latency p50/p99)
- [ ] **Error recovery**: user-facing recovery instructions for all known failure modes
- [ ] **Security posture**: auth model documented, data exposure assessed, trust boundary explicit
- [ ] **Observability baseline**: what "healthy" looks like in logs/metrics
- [ ] **Feedback loop**: how user feedback is collected and triaged
- [ ] **Known limitations**: documented list of known issues and workarounds

### Appropriate review agents
- `assumption-buster-axiom`
- `devils-advocate-axiom`
- `security-review-axiom`
- `spec-verifier-axiom`
- `sre-ops-axiom` — if available, for observability review
- `ux-writer-axiom` — for error message and recovery copy review

### What is NOT required yet
- Full SLOs/SLIs (targets may be informal)
- Incident playbooks (draft is OK)
- Migration/compatibility plan
- Adversarial testing
- Cost guardrails

### Concrete "done" example
```
Instrumentation: Prometheus counter `codeops_agents_status_requests_total{status="ok|error"}`.
Error recovery: "Server unreachable" → shows server URL + "Run `axiom serve` to start it."
Security posture: Reads from local Unix socket only. No auth needed (local process). No PII.
Observability baseline: Error rate < 1% in logs. No panics. Latency < 200ms p99.
Known limitations: --watch does not handle terminal resize. Workaround: restart command.
```

### Gap checklist: `alpha` → `beta`
- [ ] Document compatibility assumptions (what versions/platforms are supported?)
- [ ] Write a migration plan (if any data/config format changes)
- [ ] Add capacity assumptions (how many users/requests can this handle?)
- [ ] Write draft runbooks (what do operators do when something goes wrong?)
- [ ] Define SLO targets (even informal ones)

---

## Tier: `beta`

### What it means
You are scaling up. You have migration/compatibility plans, capacity assumptions, and draft runbooks. You are preparing for production. This is the "public preview" stage.

### Required artifacts
- [ ] **Compatibility plan**: supported versions, platforms, upgrade path
- [ ] **Migration plan**: how existing users/data migrate to new versions
- [ ] **Capacity assumptions**: expected load, growth rate, resource requirements
- [ ] **Draft runbooks**: what operators do for common failure scenarios
- [ ] **SLO targets**: informal or formal (latency, availability, error rate)
- [ ] **Deprecation policy**: how old versions/features are retired
- [ ] **Incident response sketch**: who is on-call, how incidents are triaged

### Appropriate review agents
- `assumption-buster-axiom`
- `devils-advocate-axiom`
- `security-review-axiom`
- `spec-verifier-axiom`
- `sre-ops-axiom`
- `docs-runbooks-axiom`
- `performance-axiom` — if scale assumptions are non-trivial

### What is NOT required yet
- Formal SLOs with error budgets
- Full adversarial testing (redteam/whitehat)
- Cost guardrails
- Chaos engineering

### Concrete "done" example
```
Compatibility: Supports Axiom v1.x. Breaking changes require major version bump.
Migration: Config format v1 → v2 auto-migrated on first run with backup.
Capacity: Designed for 50 concurrent agents, 1000 status requests/min.
Draft runbook: "Agents not showing" → check `axiom serve` logs → restart if OOM.
SLO targets: 99.5% availability, p99 latency < 500ms, error rate < 0.5%.
```

### Gap checklist: `beta` → `production`
- [ ] Formalize SLOs/SLIs with error budgets
- [ ] Write a rollback plan (tested, not just documented)
- [ ] Add risk gates (what stops a bad deploy?)
- [ ] Document abuse cases (how could this be misused?)
- [ ] Complete incident workflow (on-call rotation, escalation path, postmortem template)
- [ ] Get security review sign-off

---

## Tier: `production`

### What it means
You are operating at production quality. SLOs/SLIs are formal. Rollback is tested. Risk gates exist. Abuse cases are documented. Incident workflow is complete. This is the "generally available" stage.

### Required artifacts
- [ ] **Formal SLOs/SLIs**: with error budgets and measurement methodology
- [ ] **Tested rollback plan**: rollback procedure has been exercised
- [ ] **Risk gates**: automated checks that block bad deploys
- [ ] **Abuse cases**: documented misuse scenarios with mitigations
- [ ] **Incident workflow**: on-call rotation, escalation path, postmortem template
- [ ] **Security review sign-off**: from `security-review-axiom` or equivalent
- [ ] **Operability checklist**: runbooks linked from alerts, dashboards exist
- [ ] **Change management**: how changes are reviewed and deployed

### Appropriate review agents
- Full Pack C (adversarial): `assumption-buster-axiom`, `devils-advocate-axiom`, `security-review-axiom`, `spec-verifier-axiom`, `whitehat-axiom`, `redteam-axiom`, `trace-auditor-axiom`
- `sre-ops-axiom`
- `docs-runbooks-axiom`
- `privacy-compliance-axiom` — if PII/regulated data

### What is NOT required yet
- Chaos engineering (recommended but not required)
- Cost guardrails (recommended but not required)
- Adversarial findings fully integrated (required for `battle-tested`)

### Concrete "done" example
```
SLOs: 99.9% availability (30-day rolling), p99 latency < 300ms, error rate < 0.1%.
Rollback: Tested in staging. Rollback takes < 5 min. Automated via feature flag.
Risk gates: CI blocks deploy if error rate > 1% in canary for 5 min.
Abuse cases: Rate limiting prevents >100 req/s per client. Auth required for write ops.
Incident workflow: PagerDuty on-call. Escalation: L1 (on-call) → L2 (team lead) → L3 (CTO).
Security sign-off: security-review-axiom PASS, score 87.
```

### Gap checklist: `production` → `battle-tested`
- [ ] Integrate adversarial findings (redteam + whitehat results addressed)
- [ ] Add resilience testing (chaos engineering or fault injection)
- [ ] Add cost guardrails (budget alerts, cardinality limits, cost attribution)
- [ ] Run postmortem loop (at least one postmortem completed and learnings integrated)
- [ ] Validate recovery time objectives (RTO/RPO tested, not just documented)

---

## Tier: `battle-tested`

### What it means
You have survived adversarial review, resilience testing, and cost scrutiny. Postmortem learnings are integrated. Recovery objectives are tested. This is the "we've been through the fire" stage. Do NOT force this for internal prototypes.

### Required artifacts
- [ ] **Adversarial findings integrated**: redteam + whitehat results addressed or explicitly accepted
- [ ] **Resilience testing**: chaos engineering or fault injection results documented
- [ ] **Cost guardrails**: budget alerts, cardinality limits, cost attribution in place
- [ ] **Postmortem loop**: at least one postmortem completed with learnings integrated
- [ ] **Tested RTO/RPO**: recovery time and recovery point objectives tested (not just documented)
- [ ] **Threat model**: current and reviewed by security team
- [ ] **Dependency risk assessment**: third-party dependencies assessed for supply chain risk

### Appropriate review agents
- Full Pack C + Pack D:
  - `assumption-buster-axiom`, `devils-advocate-axiom`
  - `security-review-axiom`, `whitehat-axiom`, `redteam-axiom`
  - `trace-auditor-axiom`, `spec-verifier-axiom`
  - `sre-ops-axiom`, `finops-cost-axiom`
  - `chaos-engineer-axiom`
  - `privacy-compliance-axiom` — if PII/regulated data

### What is NOT required yet
- Perfect zero-defect history (not realistic)
- Infinite scalability (scope to actual load)

### Concrete "done" example
```
Adversarial: redteam found 2 HIGH issues (rate limit bypass, token leakage in logs). Both fixed. Whitehat confirmed fixes.
Resilience: Chaos test: killed 1/3 of agent pods. System recovered in < 60s. No data loss.
Cost guardrails: Budget alert at $500/month. Cardinality limit on agent_id label (max 1000).
Postmortem: 2026-01-15 incident (agent stuck loop). Root cause: missing timeout. Fixed in v1.2.3.
RTO/RPO: RTO 5 min tested in DR drill. RPO 1 hour (checkpoint interval).
```

### Dynamic note
`battle-tested` is a target when the cost of failure is high (financial systems, medical, security-critical infrastructure). Do NOT force it for internal prototypes or developer tooling with low blast radius.

---

## Review Pack Reference

| Pack | Tiers | Agents |
|---|---|---|
| Pack A (Minimal) | `idea` → `concept` | `assumption-buster-axiom`, `ux-writer-axiom` (if user-facing) |
| Pack B (Standard) | `poc` → `beta` | `assumption-buster-axiom`, `devils-advocate-axiom`, `security-review-axiom`, `spec-verifier-axiom` |
| Pack C (Adversarial) | `production` → `battle-tested` | Pack B + `whitehat-axiom`, `redteam-axiom`, `trace-auditor-axiom` |
| Pack D (Operability) | `beta` → `battle-tested` | Pack B + `sre-ops-axiom`, `finops-cost-axiom`, `docs-runbooks-axiom` |

---

## Alignment with Axiom Verification Signal Hierarchy

From `specs/00-PRD.md#verification-signal-hierarchy`:

| Tier 0 | Module imports | Sufficient for `idea`/`concept` spec existence check |
| Tier 1 | Isolated function tests | Sufficient for `poc`/`testing` |
| Tier 2 | CLI help/version | Necessary but not sufficient for `mvp`+ |
| Tier 3 | CLI runtime execution | **Minimum for claiming a step "done"** — required at `mvp`+ |
| Tier 4 | HTTP server startup + health | Required when server path touched — required at `alpha`+ |
| Tier 5 | End-to-end workflow | Required for milestone completion — required at `production`+ |

**Rule**: A spec step is not "done" unless it has evidence at Tier 3 or above for the paths it touches.

---

axiom:trace work_item=spec-kickoff-axiom spec=specs/00-PRD.md#verification-signal-hierarchy plan= prompt=.opencode/skills/spec-kickoff-axiom/SKILL.md evidence= doc=.opencode/skills/spec-kickoff-axiom/references/maturity-tiers.md test= commit=
