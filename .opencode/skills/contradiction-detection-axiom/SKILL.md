---
name: contradiction-detection-axiom
description: >
  Detect and surface contradictions, misalignments, and self-fighting rules across specs,
  plans, code, config, docs, and agent outputs. Load this skill when two or more artifacts
  disagree, when a rule undermines another rule, or when behavior drifts from stated intent.
version: "1.1"
created: "2026-03-26"
primary_spec: specs/00-PRD.md
secondary_specs:
  - specs/07-Mission-North-Star.md
  - specs/21-Traceability-Doctrine.md
tags:
  vertical: [coding, planning, writing]
  category: methodology
  core: false
---

# Contradiction Detection

> **"A system that fights itself wastes energy and erodes trust. Surface the conflict; don't paper over it."**

This skill provides a systematic method for finding places where two or more things in the
system are not aligned or are actively fighting each other. Contradictions can exist between
specs, between a spec and its implementation, between two agent outputs, between config and
code, or between stated goals and actual behavior.

axiom:trace work_item=contradiction-detection-01 spec= plan= test= doc=.opencode/skills/contradiction-detection-axiom/SKILL.md evidence= commit=

---

## Activation

Load this skill when:
- Two specs, rules, or policies appear to conflict
- An agent output contradicts another agent's output or an existing spec
- Code behavior diverges from what the spec says it should do
- Config values undermine stated goals (e.g., a timeout set to 0 when the spec says "always retry")
- A plan step contradicts a constraint or acceptance criterion
- A user or agent says "this doesn't make sense" or "these two things fight each other"
- During verification when a PASS in one area implies a FAIL in another
- During merge of fan-out agent outputs that disagree

**When NOT to load this skill** (skip to avoid overhead):
- Single-file cosmetic changes (formatting, typos, comment rewording)
- The "contradiction" is already tracked as an open decision in a spec
- You're in the middle of a plan step and the apparent conflict is between the old state and the new state you're actively building (that's expected mid-flight, not a contradiction)

---

## Contradiction Taxonomy

Contradictions come in distinct types. Identifying the type determines the resolution path.

### 1. Spec-vs-Spec Contradiction

Two specs make incompatible claims about the same behavior or invariant.

**Example**: Spec A says "all API responses must include a `request_id` header" while Spec B says "health check endpoints return minimal headers only."

**Detection signals**:
- Same noun/concept defined differently in two specs
- Mutually exclusive requirements (MUST X and MUST NOT X)
- Overlapping scope fences with different rules

**Resolution**: Draft an ADR via `@specwriter-axiom` that resolves the conflict. Update both specs to reference the ADR. One spec must yield or both must be scoped more precisely.

### 2. Spec-vs-Implementation Contradiction

Code does something different from what the spec says.

**Example**: Spec says "retry 3 times with exponential backoff" but code retries 5 times with fixed delay.

**Detection signals**:
- Test passes but behavior doesn't match spec language
- Spec updated but code not updated (or vice versa)
- Trace marker points to spec but implementation diverges

**Resolution**: Determine which is correct (spec or code). If spec is authoritative, fix code. If code reflects a deliberate change, update spec. Either way, create a trace-linked change.

### 3. Config-vs-Intent Contradiction

Configuration values undermine the stated purpose of a feature or system.

**Example**: Feature flag enables a circuit breaker, but the threshold is set so high it never trips.

**Detection signals**:
- Config value makes a feature effectively inert
- Default values contradict documented behavior
- Environment-specific config overrides defeat the purpose of a safety mechanism

**Resolution**: Fix the config or update the documentation to reflect actual behavior. If the config is intentional, document why.

### 4. Plan-vs-Constraint Contradiction

A plan step violates a stated constraint or acceptance criterion.

**Example**: Plan says "migrate the database schema in-place" but constraints say "no downtime migrations."

**Detection signals**:
- Plan step requires something the constraints forbid
- Acceptance criterion is untestable given the plan's approach
- Rollback step contradicts the forward step

**Resolution**: Revise the plan to respect constraints, or escalate to the user to relax the constraint with documented risk acceptance.

### 5. Agent-vs-Agent Contradiction

Two agent outputs disagree on facts, approach, or assessment.

**Example**: `@dev-axiom` says "the endpoint returns 404 for missing resources" but `@qa-axiom` says "the endpoint returns 200 with an empty body."

**Detection signals**:
- Different status codes, schemas, or behaviors described for the same surface
- One agent says PASS while another says FAIL for the same criterion
- Conflicting risk assessments (low vs high) for the same change

**Resolution**: Prefer evidence-backed claims. Re-run the verifier with clarified context. If still ambiguous, escalate to `@devils-advocate-axiom` for arbitration or draft an ADR.

### 6. Goal-vs-Goal Contradiction

Two stated goals or principles are in tension.

**Example**: "Move fast and ship daily" vs "Every change requires full security review."

**Detection signals**:
- Process requirements make stated velocity goals impossible
- Quality gates conflict with delivery timelines
- Two non-negotiables that cannot both be satisfied simultaneously

**Resolution**: This is a prioritization problem, not a bug. Surface it explicitly, quantify the tradeoff, and escalate for a human decision. Document the resolution as an ADR.

### 7. Temporal Contradiction

Something was true at one point but is no longer true, and the system hasn't caught up.

**Example**: An ADR says "we chose SQLite for v1" but the system now uses Postgres and the ADR was never superseded.

**Detection signals**:
- Stale ADRs, deprecated specs still referenced as authoritative
- Memory bank notes that reference removed features
- Comments in code that describe behavior that no longer exists

**Resolution**: Update or supersede the stale artifact. Add a temporal marker showing when the change happened.

---

## Detection Method

Run this loop against a target scope (a spec, a plan, a set of files, or the full system).

### Step 1: Gather Claims

Extract explicit claims from each artifact in scope. A "claim" is any statement about what the system does, should do, must not do, or assumes.

Sources to scan:
- `specs/` — requirements, invariants, non-goals, open decisions
- `.memory-bank/` — decisions, plans, work item notes
- Code — behavior at boundaries (API handlers, validators, config loaders)
- Config — values that control behavior
- Tests — assertions about expected behavior
- Docs/runbooks — stated procedures and expected outcomes
- Agent outputs — status, evidence, recommendations

### Step 2: Normalize Claims

For each claim, extract:
- **Subject**: what entity/behavior is being described
- **Predicate**: what is being asserted (MUST, SHOULD, returns, accepts, blocks, allows)
- **Source**: which artifact makes this claim (file path + line/section)
- **Confidence**: is this a hard requirement (MUST), a preference (SHOULD), or an observation?

### Step 3: Cross-Reference

Compare claims pairwise within the same subject:
- Do any two claims make incompatible assertions?
- Does any claim contradict a constraint?
- Does any claim contradict an acceptance criterion?
- Does any temporal claim reference something that no longer exists?

### Step 4: Intentionality Check (false-positive filter)

Before classifying severity, determine whether the contradiction is **intentional**.

Not every apparent conflict is a bug. Some are deliberate design tensions, scoped tradeoffs, or
artifacts of polyglot/multi-paradigm systems where different subsystems follow different rules
on purpose.

**Intentionality signals** (if ≥2 are present, classify as intentional):
- An ADR or spec explicitly acknowledges the tension and documents the tradeoff
- The two claims apply to different bounded contexts or subsystems with an explicit scope fence
- A comment, decision log, or memory bank note explains why both sides coexist
- The "contradiction" is between a general rule and a documented exception (e.g., "all endpoints require auth" + "health check is exempt per spec §X")
- The system is polyglot/multi-paradigm and the two sides follow different paradigm conventions intentionally (e.g., Go service uses error returns, Python service uses exceptions — not a contradiction, just different idioms)

**Intentional contradiction classification**:

```yaml
- id: "C-003"
  type: "goal-vs-goal"
  intentional: true
  intentionality_evidence:
    - "ADR-0015 documents the velocity-vs-rigor tradeoff"
    - "Spec 09 (Baby Steps) and Spec 07 (Mission North Star) both acknowledge this tension"
  severity: "info"  # intentional contradictions are informational, not actionable
  recommendation: "No action needed. Tension is managed via [mechanism]. Re-evaluate if context changes."
```

**When intentionality is unclear**: default to treating it as a real contradiction (fail closed),
but mark it `intentional: uncertain` and include a question for the user or `@devils-advocate-axiom`.

### Step 5: Classify and Severity-Rank

For each **non-intentional** contradiction found:

| Severity | Meaning | Action |
|----------|---------|--------|
| **Critical** | Two MUST-level requirements conflict; system cannot satisfy both | Immediate resolution required; block progress |
| **High** | Spec says X but code does Y; or plan violates constraint | Fix before claiming done |
| **Medium** | SHOULD-level conflict or stale reference | Fix in current work item or create follow-up |
| **Low** | Cosmetic inconsistency or outdated comment | Fix opportunistically |
| **Info** | Intentional tension with documented rationale | No action; re-evaluate if context changes |

### Step 6: Produce Contradiction Report

Output a structured report:

```yaml
contradictions:
  - id: "C-001"
    type: "spec-vs-implementation"
    severity: "high"
    subject: "retry behavior on /v1/alerts endpoint"
    claim_a:
      source: "specs/30-External-API.md#retry-policy"
      assertion: "MUST retry 3 times with exponential backoff"
    claim_b:
      source: "src/api/alerts.py:47"
      assertion: "Retries 5 times with 1s fixed delay"
    resolution_options:
      - "Update code to match spec (3 retries, exponential backoff)"
      - "Update spec to match code (5 retries, fixed delay) with ADR justification"
    recommended: "Update code to match spec — spec is the contract"
    trace: "axiom:trace work_item=<ID> spec=specs/30-External-API.md"
```

---

## Resolution Paths

| Contradiction Type | Primary Resolver | Escalation |
|--------------------|-----------------|------------|
| Spec-vs-Spec | `@specwriter-axiom` (ADR) | User decision |
| Spec-vs-Implementation | `@dev-axiom` (fix code) or `@specwriter-axiom` (fix spec) | `@spec-verifier-axiom` |
| Config-vs-Intent | `@dev-axiom` or `@sre-ops-axiom` | `@devils-advocate-axiom` |
| Plan-vs-Constraint | `@pm-axiom` (revise plan) | User decision |
| Agent-vs-Agent | Re-run verifiers; `@devils-advocate-axiom` | ADR + user decision |
| Goal-vs-Goal | `@devils-advocate-axiom` | User decision (always) |
| Temporal | `@memory-bank-axiom` + relevant owner | `@trace-auditor-axiom` |

---

## Integration with Other Skills and Agents

**Standalone usage**: This skill works independently. You do not need to run `decision-archaeology-axiom`
or `hypothetical-alternatives-axiom` to use contradiction detection. Those skills are useful
follow-ups (archaeology to understand *why* a contradiction exists, alternatives to resolve it),
but they are optional. Most contradictions can be resolved directly using the resolution paths below.

| Skill/Agent | Integration Point |
|-------------|-------------------|
| `@devils-advocate-axiom` | Arbitrates when contradictions are ambiguous |
| `@assumption-buster-axiom` | Surfaces hidden assumptions that cause contradictions |
| `@redteam-axiom` | Exploits contradictions to falsify claims |
| `@spec-verifier-axiom` | Detects spec-vs-implementation contradictions |
| `@trace-auditor-axiom` | Detects temporal contradictions and stale references |
| `adr-manager-axiom` | Records resolution decisions |
| `axiom-gap-analysis` | Contradiction detection is a gap analysis input |
| `too-much-of-a-good-thing-axiom` | Goal-vs-goal contradictions often stem from over-optimization |

---

## Anti-Patterns

| Anti-Pattern | Why Bad | Fix |
|-------------|---------|-----|
| Ignoring contradictions and hoping they resolve | They don't; they compound | Surface and resolve explicitly |
| Resolving by deleting one side without ADR | Loses context for why the conflict existed | Always document the resolution |
| Treating all contradictions as critical | Creates alert fatigue; blocks progress | Use severity ranking |
| Fixing code without checking if spec is wrong | May "fix" correct behavior | Always determine which side is authoritative first |
| Papering over with "it depends" | Defers the conflict without resolving it | Force a concrete resolution or explicit scope fence |
| Blaming the model/agent instead of the inputs | Contradictions usually come from conflicting human inputs | Trace back to the source |
| Flagging intentional tensions as bugs | Design tensions (velocity vs rigor, flexibility vs consistency) are features, not defects | Run the intentionality check; classify as `info` when documented |
| Treating polyglot idiom differences as contradictions | Different subsystems may follow different conventions on purpose | Check bounded context boundaries before flagging |

---

## Output Format

When invoked via `/axiom-contradict`, produce:

1. **Contradiction inventory** — list of all contradictions found, classified and severity-ranked
2. **Resolution recommendations** — for each contradiction, the recommended fix and who should do it
3. **Injected steps** — executable next steps to resolve critical and high contradictions
4. **Trace updates** — which artifacts need trace marker updates after resolution

---

## One-Line Reminder

If two things fight each other, one of them is wrong — or both need a scope fence. Find out which.
