---
title: "Situation Model + Signal Heuristics"
version: "1.0"
created: "2026-02-27"
updated: "2026-02-27"
skill: spec-kickoff-axiom
purpose: >
  Full reference for the 5-dimension situation model used in spec-kickoff-axiom.
  Covers signal heuristics for inferring each dimension from minimal input, combination
  patterns, anti-patterns, and a combination matrix for high-risk configurations.
---

# Situation Model + Signal Heuristics

> **Used by**: `.opencode/skills/spec-kickoff-axiom/SKILL.md`
>
> **Goal**: Ask fewer questions, but ask the *right* ones.
> You maintain a small internal model inferred from source material.
> Only ask the user when inference is ambiguous or when the stakes are high.
>
> **Heuristic**: The best question is the one that prevents you from inventing a contract boundary.

---

## The 5-Dimension Model

```yaml
situation_model:
  surfaces: api|web_ui|cli|mobile|data_pipeline|infra        # what is being built
  data_classes: none|internal|PII|secrets|financial|regulated # what data is handled
  trust_boundaries: single-tenant|multi-tenant|public-internet|internal-only # who can access
  change_blast_radius: local|service|multi-service|org-wide  # how far a change propagates
  risk_posture: low|standard|high                            # overall risk level
```

You infer this model from the source material. You do NOT ask the user to fill it in. You ask only when inference is ambiguous or when the answer changes a MUST-level requirement.

---

## Dimension 1: Surfaces

**What it is**: The technical surface(s) being built or modified.

| Value | Meaning | Spec depth implication | Testing implication | Security implication |
|---|---|---|---|---|
| `api` | HTTP/gRPC/GraphQL/AsyncAPI endpoint | Contract-first; versioning required | Contract tests + negative cases | Auth, rate limiting, input validation |
| `web_ui` | Browser-based user interface | UX flows + error states + accessibility | E2E + visual regression | XSS, CSRF, auth flows |
| `cli` | Command-line interface | Flag/arg spec + help text + exit codes | CLI integration tests | Injection, env var leakage |
| `mobile` | iOS/Android native or hybrid app | Platform-specific constraints + offline | Device matrix + network conditions | Cert pinning, local storage |
| `data_pipeline` | ETL, streaming, batch processing | Data contracts + ordering guarantees | Data correctness + idempotency | Data exfiltration, injection |
| `infra` | Cloud resources, IaC, networking | Declarative spec + drift detection | Dry-run + canary + rollback | IAM, network isolation, secrets |

### Signal heuristics for surfaces

| Signal in source material | Inferred surface |
|---|---|
| "endpoint", "API", "REST", "GraphQL", "gRPC", "webhook" | `api` |
| "UI", "dashboard", "page", "form", "button", "modal", "browser" | `web_ui` |
| "CLI", "command", "flag", "terminal", "shell", "script" | `cli` |
| "iOS", "Android", "mobile", "app store", "push notification" | `mobile` |
| "pipeline", "ETL", "batch", "stream", "Kafka", "queue", "ingestion" | `data_pipeline` |
| "Kubernetes", "Terraform", "IaC", "cloud", "VPC", "IAM", "pod" | `infra` |

### What each surface implies for spec work

**`api`**:
- Require: versioning policy, error catalog, auth scheme, rate limits
- Add reviewers: `spec-verifier-axiom`, `security-review-axiom`
- Add formats: API contract (9), RFC-style (5)
- Ask: "What is the versioning strategy? What happens when a client sends an unknown field?"

**`web_ui`**:
- Require: primary flows, error states, empty states, loading states
- Add reviewers: `ux-writer-axiom`, `accessibility-review-axiom` (if WCAG required)
- Add formats: Functional spec (4), User stories (7)
- Ask: "What is the most important failure recovery? What does the empty state look like?"

**`cli`**:
- Require: flag/arg spec, help text, exit codes, error messages
- Add reviewers: `ux-writer-axiom` (for error copy), `spec-verifier-axiom`
- Add formats: Functional spec (4)
- Ask: "What is the exit code on error? What does --help show?"

**`data_pipeline`**:
- Require: data contracts, ordering guarantees, idempotency, retry behavior
- Add reviewers: `security-review-axiom` (data exfiltration), `performance-axiom`
- Add formats: RFC-style (5), Test plan (10)
- Ask: "What happens if a message is processed twice? What is the ordering guarantee?"

**`infra`**:
- Require: declarative spec, drift detection, rollback plan, IAM policy
- Add reviewers: `cloud-engineer-axiom`, `sre-ops-axiom`, `security-review-axiom`
- Add formats: RFC-style (5), ADR set (6)
- Ask: "What is the rollback story? Who has access to these resources?"

---

## Dimension 2: Data Classes

**What it is**: The sensitivity level of data handled by the system.

| Value | Meaning | Privacy implication | Retention implication | Audit implication |
|---|---|---|---|---|
| `none` | No persistent data | None | None | None |
| `internal` | Internal business data (non-sensitive) | Low | Standard retention policy | Basic logging |
| `PII` | Personally identifiable information | High — GDPR/CCPA applies | Explicit retention + deletion | Access audit log required |
| `secrets` | Credentials, tokens, keys | Critical — never log, never store in plaintext | Rotation policy required | Access audit log required |
| `financial` | Payment, billing, transaction data | High — PCI-DSS may apply | Regulatory retention | Full audit trail required |
| `regulated` | Medical, legal, government, compliance-driven | Domain-specific regulations apply | Regulatory retention | Compliance audit trail |

### Signal heuristics for data classes

| Signal in source material | Inferred data class |
|---|---|
| "name", "email", "address", "phone", "user profile", "account" | `PII` |
| "password", "token", "API key", "secret", "credential", "certificate" | `secrets` |
| "payment", "billing", "invoice", "credit card", "transaction", "revenue" | `financial` |
| "medical", "health", "HIPAA", "PHI", "patient", "diagnosis" | `regulated` |
| "GDPR", "CCPA", "compliance", "audit", "regulatory" | `regulated` |
| "config", "settings", "preferences" (non-sensitive) | `internal` |
| Stateless computation, no storage | `none` |

### What each data class implies for spec work

**`PII`**:
- Require: retention policy, deletion mechanism, access control, audit log
- Add reviewers: `security-review-axiom`, `privacy-compliance-axiom`
- Add formats: SRS (3) for formal requirements, Test plan (10) for verification
- Ask: "How long is PII retained? Who can access it? How is deletion triggered?"
- Spec MUST include: data flow diagram, retention schedule, deletion procedure

**`secrets`**:
- Require: secrets management strategy (never plaintext), rotation policy, audit log
- Add reviewers: `security-review-axiom`, `whitehat-axiom`
- Add formats: RFC-style (5) for secrets handling protocol
- Ask: "Where are secrets stored? How are they rotated? What happens on compromise?"
- Spec MUST include: secrets storage mechanism, rotation procedure, compromise response

**`financial`**:
- Require: transaction integrity, idempotency, audit trail, PCI-DSS assessment
- Add reviewers: `security-review-axiom`, `privacy-compliance-axiom`
- Add formats: SRS (3), Test plan (10)
- Ask: "What happens if a payment is processed twice? What is the reconciliation process?"
- Spec MUST include: idempotency keys, audit trail, reconciliation procedure

**`regulated`**:
- Require: domain-specific compliance requirements, audit trail, data residency
- Add reviewers: `security-review-axiom`, `privacy-compliance-axiom`
- Add formats: SRS (3), Test plan (10), MRD/BRD (2) for compliance context
- Ask: "Which regulations apply? What is the audit trail requirement? Where must data reside?"
- Spec MUST include: applicable regulations, compliance controls, audit trail

---

## Dimension 3: Trust Boundaries

**What it is**: Who can access the system and from where.

| Value | Meaning | Auth implication | Isolation implication | Abuse case implication |
|---|---|---|---|---|
| `internal-only` | Only internal services/users on private network | Service-to-service auth (mTLS, internal tokens) | Network isolation sufficient | Low abuse risk |
| `single-tenant` | One organization, multiple users | User auth + RBAC | Tenant isolation not needed | Insider threat |
| `multi-tenant` | Multiple organizations sharing infrastructure | User auth + RBAC + tenant isolation | Hard tenant isolation required | Cross-tenant data leakage |
| `public-internet` | Unauthenticated or self-registered users | Strong auth + rate limiting + abuse prevention | Full isolation required | High abuse risk |

### Signal heuristics for trust boundaries

| Signal in source material | Inferred trust boundary |
|---|---|
| "internal service", "microservice", "service mesh", "internal API" | `internal-only` |
| "our users", "our team", "single organization", "enterprise" | `single-tenant` |
| "multiple customers", "SaaS", "tenant", "organization isolation" | `multi-tenant` |
| "public API", "open to the internet", "self-service", "sign up" | `public-internet` |

### What each trust boundary implies for spec work

**`internal-only`**:
- Require: service-to-service auth mechanism, network policy
- Add reviewers: `security-review-axiom` (for auth mechanism)
- Ask: "What authenticates service-to-service calls? Is mTLS in place?"
- Spec MUST include: auth mechanism, network policy

**`single-tenant`**:
- Require: user auth, RBAC, session management
- Add reviewers: `security-review-axiom`
- Ask: "What roles exist? What is the default permission? How are sessions managed?"
- Spec MUST include: auth model, RBAC matrix, session policy

**`multi-tenant`**:
- Require: hard tenant isolation, tenant-scoped data access, cross-tenant leak prevention
- Add reviewers: `security-review-axiom`, `whitehat-axiom`, `redteam-axiom` (for production targets)
- Ask: "How is tenant isolation enforced? What prevents Tenant A from seeing Tenant B's data?"
- Spec MUST include: tenant isolation model, data scoping rules, cross-tenant leak prevention tests
- **High-risk combination**: multi-tenant + PII/financial → always Pack C + `privacy-compliance-axiom`

**`public-internet`**:
- Require: strong auth, rate limiting, abuse prevention, input validation
- Add reviewers: `security-review-axiom`, `whitehat-axiom`, `redteam-axiom`
- Ask: "What is the rate limit? What is the abuse scenario you fear most? How are bots handled?"
- Spec MUST include: auth model, rate limits, abuse cases, input validation rules

---

## Dimension 4: Change Blast Radius

**What it is**: How far a change propagates if something goes wrong.

| Value | Meaning | Review depth | Rollback implication | Coordination implication |
|---|---|---|---|---|
| `local` | Change affects only one module/component | Standard review | Simple rollback (revert commit) | No coordination needed |
| `service` | Change affects one service and its direct consumers | Standard + integration review | Service rollback (feature flag or deploy) | Notify direct consumers |
| `multi-service` | Change affects multiple services or shared libraries | Deep review + integration tests | Coordinated rollback across services | Cross-team coordination |
| `org-wide` | Change affects all services, all users, or core infrastructure | Adversarial review + staged rollout | Complex rollback with data migration | Org-wide change management |

### Signal heuristics for blast radius

| Signal in source material | Inferred blast radius |
|---|---|
| "this module", "this component", "internal refactor" | `local` |
| "this service", "this API", "this endpoint" | `service` |
| "shared library", "common module", "multiple services depend on" | `multi-service` |
| "database schema change", "auth model change", "all users", "platform-wide" | `org-wide` |

### What each blast radius implies for spec work

**`local`**:
- Standard review pack (Pack B)
- Rollback: revert commit
- No special coordination required

**`service`**:
- Standard review pack (Pack B)
- Require: rollback plan, consumer notification
- Ask: "Who are the direct consumers? How are they notified of breaking changes?"

**`multi-service`**:
- Standard review pack (Pack B) + integration review
- Require: coordinated rollback plan, integration tests, consumer impact assessment
- Ask: "Which services are affected? What is the rollback order? Are there circular dependencies?"
- Spec MUST include: affected services list, rollback order, integration test plan

**`org-wide`**:
- Adversarial review pack (Pack C) + staged rollout
- Require: staged rollout plan, feature flags, rollback procedure tested
- Ask: "What is the staged rollout plan? What is the rollback procedure? Who approves the rollout?"
- Spec MUST include: staged rollout plan, feature flag strategy, rollback procedure, approval gates

---

## Dimension 5: Risk Posture

**What it is**: The overall risk level of the change, combining blast radius, data sensitivity, and trust boundary.

| Value | Meaning | Review pack | Stop gates | Open decision limit |
|---|---|---|---|---|
| `low` | Low blast radius, no sensitive data, internal-only | Pack A | None | Unlimited |
| `standard` | Standard blast radius, internal data, single-tenant | Pack B | Blockers only | ≤ 10 |
| `high` | High blast radius, sensitive data, or public-internet | Pack C | Blockers + security floor | ≤ 5 |

### Signal heuristics for risk posture

Risk posture is derived from the other dimensions:

```
IF blast_radius = org-wide OR data_classes includes (PII|secrets|financial|regulated) OR trust_boundary = public-internet:
  risk_posture = high

ELSE IF blast_radius = multi-service OR data_classes includes internal OR trust_boundary = multi-tenant:
  risk_posture = standard

ELSE:
  risk_posture = low
```

**Override rule**: If the user explicitly states "this is high risk" or "this is safety-critical", set `risk_posture = high` regardless of the above.

### What each risk posture implies for spec work

**`low`**:
- Pack A review (assumption-buster only)
- No stop gates beyond blockers
- Unlimited open decisions (but still label them)
- Verification: Tier 1–2 sufficient for early tiers

**`standard`**:
- Pack B review (standard)
- Stop gates: blockers only
- Open decisions: ≤ 10 before stopping
- Verification: Tier 3+ required for "done"

**`high`**:
- Pack C review (adversarial)
- Stop gates: blockers + security floor (security-review-axiom score ≥ 80)
- Open decisions: ≤ 5 before stopping
- Verification: Tier 4–5 required for "done"
- **Unanswered security/data-loss decisions MUST block (fail closed)**

---

## Signal → Action Heuristics

Use these to pick reviewers, questions, and spec formats dynamically.

### If you see: auth, roles, permissions, tokens, sessions
- Mark: `trust_boundary != internal-only`
- Ask: one decision about the auth model + one about authorization rules
- Add reviewers: `security-review-axiom` (always), `whitehat-axiom` (for production targets), `redteam-axiom` (for high risk)
- Add formats: RFC-style (5) for auth boundary + Test plan (10)
- Spec MUST include: auth model, RBAC matrix, session policy, default-deny vs default-allow decision

### If you see: PII, payments, medical, regulated, compliance
- Mark: `data_classes includes regulated`
- Ask: retention + access control + audit logging
- Add reviewers: `security-review-axiom`, `privacy-compliance-axiom`, `whitehat-axiom`
- Add formats: SRS (3) + Test plan (10)
- Spec MUST include: data flow, retention schedule, deletion procedure, audit trail

### If you see: API boundary, SDK, multiple clients
- Mark: `surfaces includes api`
- Ask: versioning + compatibility rules
- Add reviewers: `devils-advocate-axiom` (keep surface small), `spec-verifier-axiom`
- Add formats: API contract (9) + RFC-style (5)
- Spec MUST include: versioning policy, error catalog, breaking change policy

### If you see: migrations, backfills, schema changes
- Mark: `change_blast_radius = org-wide`, `irreversibility = high`
- Ask: rollback story + data correctness gates
- Add reviewers: `sre-ops-axiom`, `security-review-axiom`
- Add formats: ADR set (6) for key decisions + Test plan (10)
- Spec MUST include: migration plan, rollback procedure, data correctness verification

### If you see: "needs to scale", "lots of users", "latency", "throughput"
- Ask: explicit budgets (SLO targets) and the growth assumption
- Add reviewers: `performance-axiom`, `finops-cost-axiom`
- Add formats: production tier sections + observability/metrics expectations
- Spec MUST include: SLO targets, capacity assumptions, cost guardrails

### If you see: UI, onboarding, error messages, workflows
- Mark: `surfaces includes web_ui or cli`
- Ask: one decision about the primary flow + one about the most important failure recovery
- Add reviewers: `ux-writer-axiom` (default), `accessibility-review-axiom` (only when UI/docs/interactive UX is in-scope)
- Add formats: Functional spec (4) + User stories (7)
- Spec MUST include: primary flows, error states, empty states, recovery instructions

### If you see: Kubernetes, cloud, IaC, infrastructure
- Mark: `surfaces includes infra`
- Ask: IAM policy + rollback story + drift detection
- Add reviewers: `cloud-engineer-axiom`, `sre-ops-axiom`, `security-review-axiom`
- Add formats: RFC-style (5) + ADR set (6)
- Spec MUST include: IAM policy, rollback procedure, drift detection strategy

---

## Combination Patterns (High-Risk Configurations)

These combinations require elevated review packs and additional spec sections.

| Combination | Risk level | Required review pack | Additional requirements |
|---|---|---|---|
| PII + public-internet + multi-tenant | Critical | Pack C + `privacy-compliance-axiom` | Data flow diagram, tenant isolation proof, deletion procedure |
| secrets + public-internet | Critical | Pack C | Secrets management spec, rotation policy, compromise response |
| financial + multi-tenant | Critical | Pack C + `privacy-compliance-axiom` | Idempotency keys, audit trail, reconciliation procedure |
| regulated + any | High | Pack C + `privacy-compliance-axiom` | Compliance controls, audit trail, data residency |
| org-wide blast radius + any | High | Pack C | Staged rollout plan, feature flags, rollback procedure |
| multi-tenant + public-internet | High | Pack C | Tenant isolation model, abuse prevention, rate limiting |
| data_pipeline + PII | High | Pack B + `privacy-compliance-axiom` | Data lineage, retention, deletion propagation |
| infra + org-wide | High | Pack C + `cloud-engineer-axiom` | IAM policy, network isolation, rollback procedure |
| api + public-internet + no auth | Critical | Pack C | Auth model decision (MUST block until resolved) |

---

## Anti-Patterns (Common Wrong Inferences)

These are the most common mistakes when inferring the situation model.

### Anti-pattern 1: Assuming internal-only when auth exists
**Wrong**: "This is an internal API, so trust boundary is internal-only."
**Correct**: If the API has auth (tokens, sessions, roles), the trust boundary is at least `single-tenant`. Auth implies users, and users imply trust boundaries.

### Anti-pattern 2: Ignoring data class because "it's just metadata"
**Wrong**: "We only store user preferences, not real PII."
**Correct**: User preferences linked to a user ID are PII. Email addresses, usernames, and IP addresses are PII. When in doubt, treat as PII.

### Anti-pattern 3: Underestimating blast radius for shared libraries
**Wrong**: "This is a small change to a utility function."
**Correct**: If the utility function is used by multiple services, the blast radius is `multi-service`. Check import graphs before inferring `local`.

### Anti-pattern 4: Setting risk_posture=low for public-facing features
**Wrong**: "This is a simple read-only API, so risk is low."
**Correct**: Public-internet trust boundary implies at least `standard` risk posture. Rate limiting, abuse prevention, and input validation are always required.

### Anti-pattern 5: Treating "no database" as "no data class"
**Wrong**: "We don't have a database, so data_classes=none."
**Correct**: Data classes apply to any data handled, not just persisted data. If the system processes PII in memory (even transiently), data_classes includes PII.

### Anti-pattern 6: Conflating surface with trust boundary
**Wrong**: "It's a CLI tool, so it's internal-only."
**Correct**: A CLI tool distributed to external users has a `public-internet` trust boundary. Surface (cli) and trust boundary are independent dimensions.

### Anti-pattern 7: Skipping security review for "low risk" changes
**Wrong**: "This is a minor config change, no security review needed."
**Correct**: Config changes that affect auth, secrets, or network policy always require `security-review-axiom`, regardless of perceived risk.

### Anti-pattern 8: Assuming single-tenant when "organization" is mentioned
**Wrong**: "This is for our organization, so it's single-tenant."
**Correct**: If multiple organizations share the same infrastructure (even if they don't know it), it's `multi-tenant`. Ask: "Do multiple organizations share the same database/service?"

---

## Question Ranking (How to Choose 3–7 Questions)

Rank unknowns by: `risk × blast_radius × irreversibility`

**Prefer questions that**:
- Define a boundary ("what is in/out of scope?")
- Prevent silent invention ("what happens on failure?")
- Set a default that is expensive to reverse later (auth model, data model, tenancy)
- Collapse many downstream choices (auth model → security requirements → test strategy)

**Avoid questions that**:
- Are implementation details at low tiers
- Do not change any MUST-level requirement
- Can be answered with a safe default without user input

**High-leverage questions** (ask these first):
1. "Who is the primary user and what is their job-to-be-done?" (collapses: surface, trust boundary, UX requirements)
2. "What data does this system handle?" (collapses: data class, privacy requirements, retention)
3. "Who can access this system?" (collapses: trust boundary, auth model, isolation requirements)
4. "What is the smallest success outcome?" (collapses: scope, acceptance criteria, rollout)
5. "What are the top 3 failure modes we must handle?" (collapses: error handling, rollback, negative tests)

---

## Situation Model → Kickoff Packet Mapping

When you infer the situation model, update the Kickoff Packet:

```yaml
kickoff:
  current_tier: <inferred>
  target_tier: <inferred or asked>
  risk_posture: <inferred from dimensions>
  spec_formats: <selected from decision matrix>
  review_pack: <selected from risk posture>

situation_model:
  surfaces: [<inferred>]
  data_classes: [<inferred>]
  trust_boundaries: [<inferred>]
  change_blast_radius: <inferred>
  risk_posture: <inferred>

inferences:
  - dimension: surfaces
    value: api
    signal: "source material mentions REST endpoints"
    confidence: high
  - dimension: data_classes
    value: PII
    signal: "source material mentions user email addresses"
    confidence: high
  - dimension: trust_boundaries
    value: public-internet
    signal: "source material mentions self-service signup"
    confidence: medium  # ask to confirm
```

**Rule**: Label each inference with confidence (high/medium/low). Ask the user to confirm medium/low confidence inferences when they affect MUST-level requirements.

---

axiom:trace work_item=spec-kickoff-axiom spec=specs/00-PRD.md,specs/43-Input-Sanitization-And-Untrusted-Content.md,specs/22-Agent-Roster-And-Interactions.md plan= prompt=.opencode/skills/spec-kickoff-axiom/SKILL.md evidence= doc=.opencode/skills/spec-kickoff-axiom/references/situation-model.md test= commit=
