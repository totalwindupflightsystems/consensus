---
title: "Spec Formats Catalog"
version: "1.0"
created: "2026-02-27"
updated: "2026-02-27"
skill: spec-kickoff-axiom
purpose: >
  Catalog of 10 spec formats with signals for when to choose each, required sections,
  composability notes, and a decision matrix. Used by spec-kickoff-axiom to select
  the right format(s) based on the situation model.
---

# Spec Formats Catalog

> **Used by**: `.opencode/skills/spec-kickoff-axiom/SKILL.md`
>
> **Rule**: Formats are composable. Pick the smallest set that matches the dominant risks.
> Default to Axiom contract style unless a signal demands a different format.
> This is NOT a fixed menu — adapt to the situation.

---

## How to Choose Formats (Dynamic)

```
1. Start with the default Axiom contract style (always).
2. Add ONE primary format based on the dominant surface:
   - API boundary → API contract (Format 9)
   - User-facing behavior → Functional spec (Format 4) or User stories (Format 7)
   - Infrastructure/protocol → RFC-style technical spec (Format 5)
   - Business case → PRD (Format 1) or MRD/BRD (Format 2)
3. Add supporting formats only when signals demand them:
   - Many consequential decisions → ADR set (Format 6)
   - High verification risk → Test/verification plan (Format 10)
   - Compliance/regulated → SRS-like requirements (Format 3)
   - Actor-driven interactions → Use cases (Format 8)
4. If in doubt: keep it to (default contract style + functional spec).
```

---

## Decision Matrix

Given the dominant signals, use this matrix to select formats:

| Signal | Primary format | Supporting formats |
|---|---|---|
| API is the product boundary | API contract (9) | RFC-style (5), Test plan (10) |
| User-facing UI/CLI | Functional spec (4) | User stories (7), Use cases (8) |
| Business case / stakeholder alignment | PRD (1) | MRD/BRD (2) |
| Compliance / regulated domain | SRS (3) | Test plan (10), ADR set (6) |
| Infrastructure / protocol design | RFC-style (5) | ADR set (6) |
| Many irreversible decisions | ADR set (6) | RFC-style (5) |
| High verification risk | Test plan (10) | Functional spec (4) |
| Agile backlog-ready | User stories (7) | Functional spec (4) |
| Actor-driven step-by-step flows | Use cases (8) | Functional spec (4) |
| Multi-team / multi-service | RFC-style (5) | API contract (9), ADR set (6) |
| PII / financial / regulated | SRS (3) | Test plan (10), ADR set (6) |
| Internal tooling / low blast radius | Axiom contract (default) | Functional spec (4) |

---

## Default: Axiom Contract Style

**One-liner**: Explicit requirements (MUST/SHOULD/MAY) + open decisions + verification hooks + traceable references.

**When to use**: Most projects. This is the safe default.

**When NOT to use**: When stakeholders require a specific industry-standard format (e.g., ISO/IEC/IEEE 29148 for regulated domains, OpenAPI for API-first teams).

**Required sections**:
- Goal + non-goals
- Stakeholders
- Requirements (MUST/SHOULD/MAY language)
- Open decisions (numbered, with recommended defaults)
- Verification hooks (how to test each requirement)
- Trace markers (`axiom:trace ...`)

**Composability**: Pairs with any other format. Always include as the base layer.

**Axiom-specific notes**:
- Maps to `specs/NN-Title-Case.md` convention
- Requirements use MUST/SHOULD/MAY per `specs/` conventions
- Open decisions use numbered format with recommended defaults
- Trace markers at bottom of each spec file

---

## Format 1: PRD (Product Requirements Document)

**One-liner**: Defines what to build and why, for product/engineering alignment.

### When to use
- Signals: new product feature, user-facing behavior, business goals need alignment
- Surface types: any (especially web_ui, cli, api)
- Data classes: any
- Trust boundaries: any
- Risk posture: standard or high
- Best for: `concept` → `mvp` tiers

### When NOT to use
- Pure infrastructure changes with no user-facing behavior
- Small bug fixes or refactors
- When the audience is only engineers (use Functional spec instead)

### Required sections
1. **Problem statement**: what problem are we solving and for whom?
2. **Users / stakeholders**: who uses this and who cares?
3. **Goals**: what success looks like (measurable where possible)
4. **Non-goals**: explicit exclusions (prevents scope creep)
5. **Success metrics**: how we measure success post-launch
6. **Scope**: what is in/out of this release
7. **Acceptance criteria**: observable outcomes that define "done"
8. **Risks**: top 3–5 risks with mitigations
9. **Open decisions**: unresolved questions with recommended defaults
10. **Timeline sketch**: rough phases (not a project plan)

### Composability
- Pairs well with: Functional spec (4) for engineering detail, Test plan (10) for verification
- Often precedes: ADR set (6) for key architectural decisions
- Can be combined with: MRD/BRD (2) when business case is needed alongside product spec

### Axiom-specific notes
- Maps to `specs/00-PRD.md` pattern in Axiom repos
- Success metrics should map to Axiom confidence scoring signals where applicable
- Open decisions should use the numbered format from `spec-kickoff-axiom` decision menus

---

## Format 2: MRD / BRD (Market/Business Requirements Document)

**One-liner**: Business case, constraints, stakeholders, and positioning — for executive/business alignment.

### When to use
- Signals: business case needed, compliance constraints, stakeholder buy-in required
- Surface types: any
- Data classes: financial, regulated (especially)
- Trust boundaries: any
- Risk posture: high (especially regulated domains)
- Best for: `idea` → `concept` tiers (before engineering starts)

### When NOT to use
- When the audience is only engineers
- Internal tooling with no business case
- When the PRD already covers business goals sufficiently

### Required sections
1. **Business goals**: what business outcome does this achieve?
2. **Market context**: why now? what's the competitive/regulatory driver?
3. **Stakeholders**: business owners, compliance, legal, finance
4. **Constraints**: budget, timeline, regulatory, contractual
5. **Compliance requirements**: regulatory standards that apply
6. **Rollout constraints**: geography, user segment, phasing
7. **Success criteria**: business-level metrics (revenue, compliance, adoption)
8. **Risks**: business risks (not just technical)

### Composability
- Pairs well with: PRD (1) for product detail, SRS (3) for formal requirements
- Often precedes: PRD (1) in regulated or enterprise contexts
- Rarely used alone for engineering work

### Axiom-specific notes
- Rarely needed for internal Axiom tooling
- Use when Axiom is being deployed in a regulated enterprise context
- Business constraints should flow into `specs/` as explicit MUST-level requirements

---

## Format 3: SRS (Software Requirements Specification)

**One-liner**: Formal requirements document aligned to ISO/IEC/IEEE 29148 / IEEE 830 style.

### When to use
- Signals: regulated domain, compliance audit required, formal verification needed
- Surface types: any (especially safety-critical, medical, financial)
- Data classes: regulated, financial, PII (especially)
- Trust boundaries: any
- Risk posture: high (especially compliance-driven)
- Best for: `testing` → `production` tiers in regulated domains

### When NOT to use
- Internal tooling or developer tools
- Early-stage exploration (too heavyweight for `idea`/`concept`)
- When the team is not familiar with formal requirements notation

### Required sections
1. **Purpose**: what this document covers and its intended audience
2. **Scope**: system name, what it does/doesn't do
3. **Definitions, acronyms, abbreviations**: glossary
4. **System overview**: high-level description
5. **Functional requirements**: numbered, testable (FR-001, FR-002, ...)
6. **Non-functional requirements**: performance, security, reliability, usability
7. **Constraints**: design, implementation, regulatory
8. **Verification**: how each requirement is verified (test method, acceptance criteria)
9. **Traceability matrix**: requirements → tests → implementation

### Composability
- Pairs well with: Test plan (10) for verification, ADR set (6) for design decisions
- Can replace: PRD (1) + Functional spec (4) in formal contexts
- Often required alongside: MRD/BRD (2) in regulated enterprise contexts

### Axiom-specific notes
- Use numbered requirement IDs (e.g., `REQ-INPUT-001`) as in `specs/43-Input-Sanitization-And-Untrusted-Content.md`
- Traceability matrix maps to Axiom trace markers (`axiom:trace ...`)
- Verification section maps to Axiom evidence bundle schema (`specs/27-Evidence-Bundle-Schema.md`)

---

## Format 4: Functional Spec (FRD — Functional Requirements Document)

**One-liner**: Detailed behavior spec for engineering and QA — flows, states, validations, edge cases.

### When to use
- Signals: user-facing behavior, complex flows, QA needs testable spec
- Surface types: web_ui, cli, api (especially)
- Data classes: any
- Trust boundaries: any
- Risk posture: standard or high
- Best for: `testing` → `production` tiers

### When NOT to use
- Pure infrastructure changes with no user-facing behavior
- Very early exploration (`idea`/`concept`) — too detailed too soon
- When user stories (7) are sufficient for the team's workflow

### Required sections
1. **Feature overview**: what this feature does in 1–3 sentences
2. **Actors**: who interacts with this feature
3. **Flows**: step-by-step happy paths (numbered)
4. **States**: all states the system can be in (state diagram if complex)
5. **Validations**: input validation rules (field by field)
6. **Edge cases**: boundary conditions, empty states, concurrent access
7. **Error handling**: named error states with user-facing messages
8. **Acceptance criteria**: testable, observable outcomes
9. **Out of scope**: explicit exclusions

### Composability
- Pairs well with: User stories (7) for backlog, Test plan (10) for verification
- Often follows: PRD (1) as the engineering detail layer
- Can be combined with: API contract (9) when the feature has an API surface

### Axiom-specific notes
- Error handling section maps to Axiom error taxonomy conventions
- Acceptance criteria should be numbered and map to plan verification steps
- State diagrams can be expressed as Mermaid in `specs/` files

---

## Format 5: RFC-Style Technical Spec

**One-liner**: Normative requirements for protocols, APIs, infrastructure, or design decisions — uses MUST/SHOULD/MAY language.

### When to use
- Signals: protocol design, API design, infrastructure decisions, multi-team coordination
- Surface types: api, infra, data_pipeline (especially)
- Data classes: any
- Trust boundaries: multi-tenant, public-internet (especially)
- Risk posture: standard or high
- Best for: `testing` → `production` tiers

### When NOT to use
- User-facing behavior (use Functional spec instead)
- Business case (use PRD/MRD instead)
- Simple features with no protocol/infrastructure implications

### Required sections
1. **Motivation**: why this change is needed
2. **Design**: the proposed solution (normative language: MUST/SHOULD/MAY)
3. **Invariants**: what must always be true
4. **Alternatives considered**: what was rejected and why
5. **Security considerations**: threat model sketch, trust boundaries
6. **Rollout plan**: how this is deployed/adopted
7. **Open questions**: unresolved design questions
8. **References**: related specs, standards, prior art

### Composability
- Pairs well with: ADR set (6) for key decisions, API contract (9) for API surface
- Often precedes: implementation plan in Axiom
- Can be combined with: Test plan (10) for verification

### Axiom-specific notes
- MUST/SHOULD/MAY language aligns with Axiom `specs/` conventions
- Security considerations section maps to `security-review-axiom` review scope
- Invariants map to Axiom verification hooks and trace markers
- Example: `specs/04-XML-Protocol.md` is an RFC-style spec

---

## Format 6: ADR Set (Architecture Decision Records)

**One-liner**: Captures "why" behind key decisions — one decision per record, immutable once decided.

### When to use
- Signals: irreversible decisions, auth model, tenancy model, data retention, migrations, tech stack choices
- Surface types: any
- Data classes: any
- Trust boundaries: any (especially multi-tenant)
- Risk posture: any (especially when reversibility is low)
- Best for: any tier when a consequential decision is made

### When NOT to use
- Routine implementation choices (use code comments instead)
- Decisions that are easily reversible
- When the decision is already captured in a higher-level spec

### Required sections (per ADR)
1. **Title**: short, imperative (e.g., "Use PostgreSQL for primary storage")
2. **Status**: proposed | accepted | deprecated | superseded
3. **Context**: what situation led to this decision?
4. **Decision**: what was decided (normative)
5. **Consequences**: what becomes easier/harder as a result
6. **Alternatives considered**: what was rejected and why
7. **Date**: when the decision was made

### Composability
- Pairs well with: RFC-style (5) for protocol decisions, PRD (1) for product decisions
- Often accompanies: any format when irreversible decisions are made
- Can be embedded in: other spec formats as a "Decisions" section

### Axiom-specific notes
- ADRs in Axiom repos live in `specs/` as `specs/NN-ADR-Title.md` or in a `specs/adrs/` subfolder
- Status field maps to Axiom "Resolved Decisions" table in `specs/00-PRD.md`
- Superseded ADRs should be kept (immutable history) with a pointer to the new ADR
- Example: `specs/00-PRD.md#resolved-decisions` captures resolved ADRs inline

---

## Format 7: User Stories + Acceptance Criteria

**One-liner**: Backlog-ready specs in "As a [user], I want [goal], so that [value]" format.

### When to use
- Signals: agile team, backlog-driven workflow, user-facing behavior
- Surface types: web_ui, cli (especially)
- Data classes: any
- Trust boundaries: any
- Risk posture: low or standard
- Best for: `concept` → `mvp` tiers

### When NOT to use
- Infrastructure or protocol changes (no "user" in the traditional sense)
- Compliance-driven requirements (use SRS instead)
- When the team needs more detail than stories provide (use Functional spec instead)

### Required sections
1. **Story list**: numbered stories in "As a / I want / So that" format
2. **Acceptance criteria**: per story, testable and observable
3. **Definition of done**: shared team definition
4. **Edge cases**: per story or as a shared section
5. **Out of scope**: explicit exclusions

### Composability
- Pairs well with: Functional spec (4) for engineering detail, Test plan (10) for verification
- Often precedes: implementation plan in Axiom
- Can be combined with: Use cases (8) for complex actor interactions

### Axiom-specific notes
- Stories map to Axiom acceptance criteria in work item plans
- ACs should be numbered and map to plan verification steps
- "Definition of done" maps to Axiom evidence bundle requirements

---

## Format 8: Use Cases

**One-liner**: Actor-driven step-by-step interactions — main success scenario + extensions.

### When to use
- Signals: complex actor interactions, multiple paths through a flow, QA needs step-by-step scenarios
- Surface types: web_ui, cli (especially)
- Data classes: any
- Trust boundaries: any
- Risk posture: standard or high
- Best for: `testing` → `production` tiers

### When NOT to use
- Simple features with a single happy path
- Infrastructure or protocol changes
- When user stories (7) are sufficient

### Required sections (per use case)
1. **Use case name**: short, verb-noun (e.g., "Submit Payment")
2. **Actor**: who initiates this use case
3. **Preconditions**: what must be true before this use case starts
4. **Main success scenario**: numbered steps (actor action → system response)
5. **Extensions**: alternative paths (numbered, e.g., 3a, 3b)
6. **Postconditions**: what is true after success
7. **Failure conditions**: what happens when it fails

### Composability
- Pairs well with: Functional spec (4) for validation rules, Test plan (10) for test scenarios
- Often follows: User stories (7) as a more detailed layer
- Can be combined with: API contract (9) when the use case has an API surface

### Axiom-specific notes
- Extensions map to Axiom negative test cases
- Postconditions map to Axiom verification hooks
- Use cases can be expressed as Mermaid sequence diagrams in `specs/` files

---

## Format 9: API Contract (OpenAPI / AsyncAPI)

**One-liner**: Machine-readable API surface definition — the boundary between teams or services.

### When to use
- Signals: API is the product boundary, multiple clients, multi-team coordination, SDK generation
- Surface types: api (especially), data_pipeline
- Data classes: any
- Trust boundaries: multi-tenant, public-internet (especially)
- Risk posture: standard or high
- Best for: `testing` → `production` tiers

### When NOT to use
- Internal-only APIs with a single consumer (use Functional spec instead)
- Very early exploration (`idea`/`concept`) — too detailed too soon
- Non-HTTP APIs (use RFC-style (5) instead, or AsyncAPI for event-driven)

### Required sections
1. **OpenAPI/AsyncAPI document**: paths, operations, schemas, security schemes
2. **Narrative constraints**: what the spec doesn't capture (rate limits, ordering guarantees)
3. **Examples**: request/response examples for each operation
4. **Error catalog**: all error codes with descriptions
5. **Versioning policy**: how breaking changes are handled
6. **Authentication**: auth scheme documented with examples
7. **Rate limits**: documented in narrative (not just headers)

### Composability
- Pairs well with: RFC-style (5) for protocol decisions, Test plan (10) for contract tests
- Often follows: PRD (1) or Functional spec (4) as the machine-readable layer
- Can be combined with: ADR set (6) for versioning/breaking-change decisions

### Axiom-specific notes
- Maps to `openapi.json` in Axiom repos (required contract snapshot)
- `openapi.json` must stay in sync with implemented behavior (per `AGENTS.md`)
- API contract changes require runtime verification (Tier 4+)
- Example: `specs/30-External-API-And-Realtime.md` + `openapi.json`

---

## Format 10: Test Plan / Verification Plan

**One-liner**: Defines the test strategy, test matrix, environments, and required gates — the "how do we know it works?" document.

### When to use
- Signals: high verification risk, compliance audit, QA-driven workflow, complex test matrix
- Surface types: any
- Data classes: regulated, financial (especially)
- Trust boundaries: any
- Risk posture: high (especially)
- Best for: `testing` → `production` tiers

### When NOT to use
- Very early exploration (`idea`/`concept`) — too detailed too soon
- Simple features where ACs in the Functional spec are sufficient
- When the test strategy is already captured in CI/CD config

### Required sections
1. **Test strategy**: unit / integration / E2E split; what tools; what environments
2. **Test matrix**: features × test types × environments
3. **Required gates**: what must pass before each milestone
4. **Acceptance tests**: the specific tests that define "done"
5. **Negative test cases**: what failure modes are tested
6. **Test data**: what data is needed and how it is managed
7. **Environments**: dev / staging / production; what differs
8. **Evidence requirements**: what outputs prove the tests ran

### Composability
- Pairs well with: Functional spec (4) for behavior, SRS (3) for formal requirements
- Often follows: any format as the verification layer
- Can be combined with: API contract (9) for contract testing

### Axiom-specific notes
- Evidence requirements map to Axiom evidence bundle schema (`specs/27-Evidence-Bundle-Schema.md`)
- Required gates map to Axiom verification signal hierarchy (Tier 0–5)
- Test matrix should include Axiom verification tiers (Tier 3+ required for "done")
- Example: `specs/26-Local-Dev-Test-Harness.md` describes the test harness

---

## Format Composability Matrix

| Format | Pairs well with | Rarely combined with |
|---|---|---|
| Axiom contract (default) | All formats | — |
| PRD (1) | Functional spec (4), Test plan (10), MRD/BRD (2) | SRS (3) (redundant) |
| MRD/BRD (2) | PRD (1), SRS (3) | API contract (9) (too technical) |
| SRS (3) | Test plan (10), ADR set (6) | User stories (7) (different audience) |
| Functional spec (4) | User stories (7), Test plan (10), API contract (9) | MRD/BRD (2) (different audience) |
| RFC-style (5) | ADR set (6), API contract (9) | User stories (7) (different audience) |
| ADR set (6) | RFC-style (5), PRD (1) | Test plan (10) (different purpose) |
| User stories (7) | Functional spec (4), Test plan (10) | SRS (3) (different formality) |
| Use cases (8) | Functional spec (4), Test plan (10) | RFC-style (5) (different audience) |
| API contract (9) | RFC-style (5), Test plan (10) | MRD/BRD (2) (different audience) |
| Test plan (10) | Any format | — |

---

## If the User Requests a Different Format

1. **If web research is allowed**: delegate to `repo-researcher-axiom` to fetch an authoritative template and cite it. The research output MUST include a citation and a short "what we adopted" summary. Do not paste huge templates into specs.
2. **If web research is NOT allowed**: ask the user to paste the template they want followed, or accept the closest supported format from this catalog.
3. **If the format is unknown**: map it to the closest format in this catalog and note the mapping.

---

## Sources (Non-Normative Pointers)

- OpenCode skills: https://opencode.ai/docs/skills/
- ISO/IEC/IEEE 29148 template pointer: https://www.well-architected-guide.com/documents/iso-iec-ieee-29148-template/
- ADR overview: https://codesoapbox.dev/preserving-critical-software-knowledge-using-architectural-decision-records/
- OpenAPI specification: https://spec.openapis.org/oas/latest.html
- AsyncAPI specification: https://www.asyncapi.com/docs/reference/specification/latest

---

axiom:trace work_item=spec-kickoff-axiom spec=specs/00-PRD.md plan= prompt=.opencode/skills/spec-kickoff-axiom/SKILL.md evidence= doc=.opencode/skills/spec-kickoff-axiom/references/spec-formats.md test= commit=
