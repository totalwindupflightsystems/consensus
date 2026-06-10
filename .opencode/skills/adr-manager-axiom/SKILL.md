---
name: adr-manager-axiom
description: >
  Architecture Decision Record creation, lifecycle management, cross-reference with specs,
  decision context preservation, and ADR index maintenance. Load this skill when making
  architectural decisions, resolving open decisions in specs, or querying historical decisions.
license: MIT
compatibility: opencode
metadata:
  version: "1.0"
  created: "2026-02-27"
  primary_spec: specs/00-PRD.md
  secondary_specs:
    - specs/07-Mission-North-Star.md
    - specs/README.md
tags:
  vertical: [planning, writing]
  category: writing
  core: false
---

# ADR Manager Skill (Portable)

> **"Every significant decision deserves a record. Every record deserves a reason."**
>
> **"Never supersede an ADR without documenting why. Never delete an ADR -- only deprecate."**

This skill provides portable guidance for creating, managing, and querying Architecture
Decision Records (ADRs) in any Axiom-managed repository. ADRs capture the "why" behind
architectural choices, preserving decision context for future developers and AI agents.

---

## Activation

Load this skill when:
- Making an architectural decision that affects system structure, technology choice, or contract
- Resolving an "Open Decision" listed in a spec file
- Reviewing whether a past decision should be revisited
- Querying historical decisions by area, status, or date
- Onboarding to a project and needing to understand past decisions
- Running a spec-kickoff that produces decision sets
- Preparing a PRD-to-spec merge that resolves open decisions

---

## Non-Negotiables

1. **Never supersede an ADR without documenting why.** The superseding ADR must reference
   the original and explain what changed.

2. **Never delete an ADR.** ADRs are immutable historical records. Outdated decisions are
   marked `deprecated` or `superseded`, never removed.

3. **Every ADR must have alternatives considered.** A decision without alternatives is not
   a decision -- it's an assumption. Document at least one alternative, even if it's "do nothing."

4. **ADRs and specs serve different purposes.** ADRs capture "why we chose X over Y."
   Specs capture "what the contract is." Don't conflate them.

5. **Cross-reference is mandatory.** When an ADR resolves an open decision in a spec, both
   the ADR and the spec must link to each other.

---

## ADR Template (MADR Format)

Axiom uses the Markdown Any Decision Records (MADR) format, adapted for traceability.

### File Location

```
docs/decisions/NNNN-title-in-kebab-case.md
```

Or, for repos that prefer specs-adjacent storage:

```
specs/decisions/NNNN-title-in-kebab-case.md
```

The location MUST be consistent within a repo. Choose one and stick with it.

### Numbering

- `NNNN` is a zero-padded 4-digit sequence number (e.g., `0001`, `0042`)
- Numbers are never reused, even for deprecated ADRs
- Gaps in numbering are acceptable (e.g., if an ADR is drafted but never accepted)

### Template

```markdown
# NNNN: <Title>

## Status

<proposed | accepted | deprecated | superseded>

Superseded by: [NNNN: <Title>](NNNN-title.md) <!-- only if superseded -->
Deprecation date: <ISO 8601> <!-- only if deprecated -->

## Date

<ISO 8601 date of last status change>

## Context

<What is the issue that we're seeing that is motivating this decision or change?>
<What are the forces at play (technical, business, organizational)?>
<What constraints exist?>

## Decision

<What is the change that we're proposing and/or doing?>
<State the decision clearly and concisely.>

## Consequences

### Positive
- <Good outcome 1>
- <Good outcome 2>

### Negative
- <Trade-off 1>
- <Trade-off 2>

### Neutral
- <Side effect that is neither clearly good nor bad>

## Alternatives Considered

### Alternative 1: <Name>
- **Description**: <What this alternative would look like>
- **Pros**: <Advantages>
- **Cons**: <Disadvantages>
- **Why rejected**: <Specific reason>

### Alternative 2: <Name>
- **Description**: <What this alternative would look like>
- **Pros**: <Advantages>
- **Cons**: <Disadvantages>
- **Why rejected**: <Specific reason>

## Related

- Spec: `specs/<NN>-<Name>.md` <!-- link to related spec -->
- Open Decision resolved: `specs/<NN>-<Name>.md#open-decisions` <!-- if applicable -->
- Supersedes: [NNNN: <Title>](NNNN-title.md) <!-- if applicable -->
- Related ADRs: [NNNN: <Title>](NNNN-title.md) <!-- if applicable -->
- Work Item: <work_item_id> <!-- if applicable -->

## Trace

axiom:trace work_item=<ID> spec=<spec-ref> plan=<plan-ref> doc=docs/decisions/NNNN-title.md
```

---

## ADR Lifecycle

```
proposed --> accepted --> [deprecated | superseded]
    |                          |
    +--> rejected              +--> (new ADR created)
```

### Status Definitions

| Status | Meaning | Allowed Transitions |
|--------|---------|---------------------|
| `proposed` | Under discussion; not yet binding | `accepted`, `rejected` |
| `accepted` | Active and binding; guides implementation | `deprecated`, `superseded` |
| `deprecated` | No longer relevant; kept for history | Terminal |
| `superseded` | Replaced by a newer ADR | Terminal |
| `rejected` | Considered but not adopted | Terminal |

### Transition Rules

1. **proposed -> accepted**: Requires at least one reviewer's approval (human or adversarial agent)
2. **accepted -> deprecated**: Requires a reason and deprecation date
3. **accepted -> superseded**: Requires a new ADR that references the original
4. **proposed -> rejected**: Requires a reason; the ADR is kept for historical context

---

## ADR vs Spec: When to Write Which

| Situation | Write an ADR | Write/Update a Spec |
|-----------|-------------|---------------------|
| Choosing between two database technologies | Yes | No (unless the spec needs to reference the choice) |
| Defining the API contract for a new endpoint | No | Yes |
| Deciding on the retry strategy for a service | Yes | Yes (spec defines the contract; ADR explains why) |
| Resolving an "Open Decision" in a spec | Yes | Yes (update spec to reference the ADR) |
| Changing the CI/CD pipeline approach | Yes | No (unless it affects a spec) |
| Adding a new required field to an API | No | Yes (spec defines the contract) |
| Choosing between REST and gRPC for a service | Yes | Yes (spec defines the chosen protocol) |

### Rule of Thumb

- **ADR**: "Why did we choose X?" (decision rationale, alternatives, trade-offs)
- **Spec**: "What is X?" (contract, schema, behavior, invariants)

When both are needed, write the ADR first (to capture the decision), then update the spec
(to codify the contract).

---

## Cross-Reference with Specs

### Resolving Open Decisions

Many Axiom specs contain "Open Decisions" sections. When an ADR resolves one:

1. **Create the ADR** with the decision and alternatives
2. **Update the spec** to move the item from "Open Decisions" to "Resolved Decisions"
3. **Add cross-references** in both directions:

In the ADR:
```markdown
## Related
- Open Decision resolved: `specs/30-External-API-And-Realtime.md#open-decisions`
```

In the spec:
```markdown
## Resolved Decisions
| Decision | Resolution | ADR |
|----------|-----------|-----|
| State store for v1 | In-memory | [0042: Use in-memory state store for v1](docs/decisions/0042-use-in-memory-state-store.md) |
```

### Linking from Specs to ADRs

When a spec references a decision:
```markdown
The retry strategy uses exponential backoff with jitter
(see [ADR-0015: Retry Strategy](docs/decisions/0015-retry-strategy.md) for rationale).
```

---

## ADR Index Maintenance

### Index File

Maintain an index at `docs/decisions/README.md` (or `specs/decisions/README.md`):

```markdown
# Architecture Decision Records

## Index

| # | Title | Status | Date | Area |
|---|-------|--------|------|------|
| 0001 | [Use MADR format for ADRs](0001-use-madr-format.md) | accepted | 2026-01-15 | process |
| 0002 | [Use Python for runtime](0002-use-python-for-runtime.md) | accepted | 2026-01-20 | technology |
| 0003 | [In-memory state store for v1](0003-in-memory-state-store.md) | accepted | 2026-02-01 | architecture |
| 0004 | [Use SSE over WebSocket for v1](0004-sse-over-websocket.md) | accepted | 2026-02-05 | protocol |

## By Status

### Accepted
- [0001](0001-use-madr-format.md), [0002](0002-use-python-for-runtime.md), ...

### Proposed
- (none)

### Deprecated
- (none)

### Superseded
- (none)
```

### Index Update Rules

- The index MUST be updated whenever an ADR is created or its status changes
- The index MUST include: number, title (linked), status, date, and area tag
- The index SHOULD include a "By Status" section for quick filtering
- The index SHOULD include a "By Area" section when the repo has >10 ADRs

---

## Querying ADRs

### By Status

```bash
# Find all accepted ADRs
grep -l "^## Status" docs/decisions/*.md | xargs grep -l "accepted"

# Find all proposed (pending) ADRs
grep -l "^## Status" docs/decisions/*.md | xargs grep -l "proposed"
```

### By Area

Use the area tag in the index to filter. Common areas:
- `architecture` - System structure decisions
- `technology` - Technology/tool choices
- `protocol` - API/protocol decisions
- `process` - Development process decisions
- `security` - Security-related decisions
- `operations` - Deployment/ops decisions

### By Date

```bash
# Find ADRs from the last 30 days
find docs/decisions/ -name "*.md" -newer $(date -v-30d +%Y%m%d) -type f
```

### By Related Spec

```bash
# Find ADRs that reference a specific spec
grep -rl "specs/30-External-API" docs/decisions/
```

---

## Integration with Axiom Skills

### `spec-kickoff-axiom`

The spec-kickoff skill produces "ADR sets" -- groups of related decisions that need to be
made during spec design. When spec-kickoff produces an ADR set:

1. Create one ADR per decision in the set
2. Link all ADRs in the set to each other via "Related ADRs"
3. Update the spec with resolved decisions

### `prd-spec-merge-axiom`

When merging a PRD into specs, open decisions from the PRD become either:
- Resolved decisions (if the PRD provides the answer) -> create ADR + update spec
- Open decisions in the spec (if still unresolved) -> note in spec, create proposed ADR

### `@assumption-buster-axiom`

The assumption-buster agent may surface undocumented decisions. When it does:
- Create a retroactive ADR capturing the implicit decision
- Mark it as `accepted` with the date it was effectively made
- Link to the code/spec where the decision is implemented

### `@devils-advocate-axiom`

The devil's advocate agent may challenge existing decisions. When it does:
- If the challenge is valid, create a new `proposed` ADR with the alternative
- If the existing decision holds, add the challenge as a "Considered and Rejected" note

---

## Integration

### Works With

| Skill/Agent | Integration Point |
|-------------|-------------------|
| `spec-kickoff-axiom` | ADR sets from spec design |
| `prd-spec-merge-axiom` | Open decision resolution |
| `@assumption-buster-axiom` | Surfaces undocumented decisions |
| `@devils-advocate-axiom` | Challenges existing decisions |
| `traceability-doctrine` | ADR trace markers |
| `enterprise-release-quality` | Decision audit trail for releases |

---

## AI-Assisted Development Risks (2026)

| Risk | Mitigation |
|------|------------|
| AI makes decisions without recording them | Mandate ADR for any architectural choice |
| AI creates ADRs without alternatives | Template enforces "Alternatives Considered" section |
| AI supersedes ADRs without explanation | Non-negotiable: superseding ADR must reference original |
| AI conflates ADRs and specs | Clear guidance on when to write which |
| AI generates boilerplate ADRs with no real analysis | Review ADRs for substantive alternatives and trade-offs |
| AI deletes deprecated ADRs to "clean up" | Non-negotiable: never delete, only deprecate |

---

## Anti-Patterns

| Anti-Pattern | Why Bad | Fix |
|-------------|---------|-----|
| Decisions in Slack/meetings only | Lost context; no audit trail | Write an ADR |
| ADR without alternatives | Not a real decision record | Add at least one alternative |
| Deleting old ADRs | Destroys historical context | Deprecate or supersede instead |
| ADR that duplicates spec content | Maintenance burden; drift risk | ADR = why; spec = what |
| No cross-reference to spec | Decision disconnected from contract | Add bidirectional links |
| Numbering gaps treated as errors | Gaps are normal (rejected proposals) | Accept gaps |

---

## Trace

`axiom:trace work_item=adr-manager-axiom spec=specs/00-PRD.md plan= prompt=.opencode/skills/adr-manager-axiom/SKILL.md evidence= doc= test= commit=`
