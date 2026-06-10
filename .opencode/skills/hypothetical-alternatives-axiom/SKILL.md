---
name: hypothetical-alternatives-axiom
description: >
  Generate and evaluate hypothetical alternatives and what-if scenarios for decisions,
  architectures, approaches, and plans. Produces structured option comparisons with
  tradeoff analysis, risk assessment, and recommendation. Load this skill when exploring
  "what if we did Y instead?" or when a decision needs alternatives before committing.
version: "1.1"
created: "2026-03-26"
primary_spec: specs/00-PRD.md
secondary_specs:
  - specs/07-Mission-North-Star.md
tags:
  vertical: [planning, coding]
  category: planning
  core: false
---

# Hypothetical Alternatives

> **"The best decision is the one that considered the roads not taken."**

This skill provides a structured method for generating, evaluating, and comparing alternative
approaches to a decision, design, or plan. It produces honest tradeoff analysis — not advocacy
for a predetermined answer. Every alternative gets a fair hearing, and the recommendation is
based on explicit criteria, not gut feel.

axiom:trace work_item=hypothetical-alternatives-01 spec= plan= test= doc=.opencode/skills/hypothetical-alternatives-axiom/SKILL.md evidence= commit=

---

## Activation

Load this skill when:
- A decision is being made and alternatives haven't been explored
- Someone asks "what if we did it differently?" or "what are our options?"
- A plan feels locked in and needs a pressure test
- An ADR needs its "Alternatives Considered" section filled out properly
- A contradiction is found and multiple resolution paths exist
- A technology, architecture, or process choice needs evaluation
- You want to stress-test a recommendation before committing
- During spec-kickoff when design options need structured comparison
- Before a major refactor to evaluate whether the refactor is the right move

**When NOT to load this skill** (skip to avoid overhead):
- The decision is trivial (variable naming, formatting choice, single config value)
- The decision is already made and irreversible — archaeology is more useful than alternatives at that point
- You're executing an approved plan step and the approach was already decided during planning
- There's only one viable option after hard-constraint elimination — no comparison needed

---

## The Hypothetical Alternatives Method

### Step 1: Frame the Decision

Define the decision clearly before generating alternatives.

**Decision Frame Template**:
```yaml
decision_frame:
  question: "How should we handle X?"
  context: "We need to solve Y because Z."
  constraints:
    hard:  # Non-negotiable — alternatives that violate these are eliminated
      - "Must not break existing API contracts"
      - "Must work within current infrastructure"
    soft:  # Preferred but negotiable — tradeoffs are acceptable
      - "Should minimize operational complexity"
      - "Should be implementable within 2 sprints"
  evaluation_criteria:  # How we'll compare alternatives
    - name: "Implementation effort"
      weight: 0.2
      scale: "low (1) / medium (3) / high (5) — lower is better"
    - name: "Operational complexity"
      weight: 0.2
      scale: "low (1) / medium (3) / high (5) — lower is better"
    - name: "Correctness confidence"
      weight: 0.3
      scale: "low (1) / medium (3) / high (5) — higher is better"
    - name: "Future flexibility"
      weight: 0.15
      scale: "low (1) / medium (3) / high (5) — higher is better"
    - name: "Risk"
      weight: 0.15
      scale: "low (1) / medium (3) / high (5) — lower is better"
  current_approach: "Brief description of what we do today (if applicable)"
```

### Step 2: Generate Alternatives

Produce at least 3 alternatives (including the current approach if one exists). Use these generation strategies:

**Strategy 1: Constraint Relaxation**
- What if we relaxed constraint X? What approach becomes possible?
- What if we had more time/budget/people?

**Strategy 2: Technology Swap**
- What if we used a different tool/library/service?
- What if we built vs bought?

**Strategy 3: Architecture Shift**
- What if we moved this responsibility to a different layer?
- What if we made this synchronous instead of async (or vice versa)?
- What if we split this into two services (or merged two into one)?

**Strategy 4: Scope Adjustment**
- What if we solved a smaller version of the problem first?
- What if we solved a bigger version that also addresses adjacent needs?

**Strategy 5: Inversion**
- What if we did the opposite of the current approach?
- What if the consumer did the work instead of the producer?

**Strategy 6: Precedent**
- How have similar systems solved this? (industry patterns, open source, competitors)
- What does the research/literature say?

### Step 3: Evaluate Each Alternative

For each alternative, produce a structured evaluation:

```yaml
alternative:
  name: "Option B: Event-driven with message queue"
  description: |
    Replace the synchronous API call with an event published to a message queue.
    Consumers process events asynchronously.
  
  hard_constraint_check:
    - constraint: "Must not break existing API contracts"
      passes: true
      note: "Existing API remains; events are additive"
    - constraint: "Must work within current infrastructure"
      passes: false
      note: "Requires deploying a message broker (RabbitMQ/Kafka)"
  
  # If any hard constraint fails, this alternative is eliminated
  eliminated: true
  elimination_reason: "Requires new infrastructure not in current scope"
  
  # Only evaluate soft criteria if not eliminated
  evaluation: null
```

For non-eliminated alternatives:

```yaml
alternative:
  name: "Option A: Retry with circuit breaker"
  description: |
    Add a circuit breaker pattern to the existing synchronous call.
    After N failures, the circuit opens and returns a cached/default response.
  
  hard_constraint_check:
    - constraint: "Must not break existing API contracts"
      passes: true
    - constraint: "Must work within current infrastructure"
      passes: true
  
  eliminated: false
  
  evaluation:
    - criterion: "Implementation effort"
      score: 2
      rationale: "Well-understood pattern; libraries exist (tenacity, pybreaker)"
    - criterion: "Operational complexity"
      score: 2
      rationale: "Adds circuit state monitoring but no new infrastructure"
    - criterion: "Correctness confidence"
      score: 4
      rationale: "Proven pattern; easy to test; clear failure modes"
    - criterion: "Future flexibility"
      score: 3
      rationale: "Works for current scale; may need rethinking at 10x load"
    - criterion: "Risk"
      score: 2
      rationale: "Low risk; worst case is overly aggressive circuit opening"
  
  weighted_score: 3.1  # Computed from weights × scores
  
  pros:
    - "Minimal infrastructure change"
    - "Well-understood failure modes"
    - "Can be implemented incrementally"
  
  cons:
    - "Doesn't solve the root cause if the downstream is fundamentally unreliable"
    - "Circuit breaker tuning requires production observation"
  
  risks:
    - "Circuit may open too aggressively during normal variance"
    - "Cached/default responses may not be acceptable for all consumers"
  
  unknowns:
    - "What is the acceptable staleness of cached responses?"
    - "What is the downstream's actual failure rate?"
```

### Step 4: Sensitivity Analysis (guard against false precision)

Weighted scores are a **thinking tool**, not a calculator. A score of 3.4 vs 3.1 may be noise,
not signal. Before trusting the numbers, run a sensitivity check.

**Sensitivity check procedure**:

1. **Score confidence**: For each score you assigned, rate your confidence:
   - `high` — based on concrete evidence, benchmarks, or prior experience
   - `medium` — reasonable estimate but no hard data
   - `low` — educated guess; could easily be ±1 on the scale
   
2. **Score range**: For any `medium` or `low` confidence score, provide a range (e.g., "2–4" instead of "3").

3. **Weighted score range**: Recompute the weighted score using the best-case and worst-case
   values from the ranges. This gives you a **score band** instead of a point estimate.

4. **Overlap check**: If the score bands of two alternatives overlap, the scores alone cannot
   distinguish them. The recommendation must be based on qualitative factors (reversibility,
   team familiarity, strategic alignment) rather than the numbers.

**Example with sensitivity**:

```yaml
alternative:
  name: "Option A: Circuit Breaker"
  evaluation:
    - criterion: "Implementation effort"
      score: 2
      confidence: high
      range: [2, 2]
    - criterion: "Correctness confidence"
      score: 4
      confidence: medium
      range: [3, 5]
    - criterion: "Risk"
      score: 2
      confidence: low
      range: [1, 4]
  
  weighted_score: 3.1
  weighted_score_range: [2.5, 3.8]  # best-case to worst-case
  score_confidence: "medium — 2 of 5 criteria have low/medium confidence"
```

```markdown
## Score Bands

| Alternative | Point Score | Score Band | Band Confidence |
|-------------|------------|------------|-----------------|
| Option A    | 3.1        | 2.5 – 3.8 | medium          |
| Option C    | 3.4        | 2.8 – 3.9 | medium          |
| Option D    | 1.7        | 1.3 – 2.2 | high            |

Options A and C have overlapping bands (2.5–3.8 vs 2.8–3.9).
The scores alone do not distinguish them. Recommendation is based on:
- Option A is reversible; Option C is not → prefer A
- Option A can ship this sprint; Option C requires 2 sprints → prefer A
```

**Key rule**: if score bands overlap, you MUST state this explicitly and base the recommendation
on qualitative differentiators, not the point scores. Never present a 0.3-point difference as
meaningful when the underlying scores have ±1 uncertainty.

### Step 5: Compare and Recommend

Produce a comparison matrix and a recommendation:

```markdown
## Comparison Matrix

| Criterion (weight) | Option A: Circuit Breaker | Option C: Async Fallback | Option D: Do Nothing |
|---------------------|--------------------------|--------------------------|----------------------|
| Implementation (0.2) | 2 — low effort | 3 — medium effort | 1 — zero effort |
| Ops complexity (0.2) | 2 — monitoring only | 3 — new async path | 1 — no change |
| Correctness (0.3) | 4 — proven pattern | 4 — proven pattern | 2 — known failures |
| Flexibility (0.15) | 3 — good for now | 4 — scales better | 1 — no improvement |
| Risk (0.15) | 2 — low | 3 — medium | 4 — high (status quo risk) |
| **Weighted Score** | **3.1** | **3.4** | **1.7** |

Option B eliminated: requires new infrastructure outside current scope.

## Recommendation

**Option A: Circuit Breaker** is recommended despite Option C scoring slightly higher, because:
1. It can be implemented in the current sprint (soft constraint alignment)
2. It addresses the immediate reliability problem without architectural change
3. Option C can be pursued later if scale demands it

This is a **reversible decision** — we can switch to Option C later without significant rework.
```

### Step 6: Document for ADR

If the decision is significant enough for an ADR, the output maps directly:
- Decision Frame → ADR Context
- Recommendation → ADR Decision
- Comparison Matrix → ADR Alternatives Considered
- Pros/Cons → ADR Consequences

---

## Hypothetical Scenario Mode

Beyond decision evaluation, this skill supports "what-if" scenario exploration:

### Scenario Template

```yaml
scenario:
  name: "What if we lose access to the GitHub API for 24 hours?"
  type: "failure"  # failure | growth | constraint_change | technology_shift
  
  assumptions:
    - "GitHub API returns 503 for all requests"
    - "Local git operations still work"
    - "CI/CD pipelines that depend on GitHub API are affected"
  
  impact_analysis:
    - area: "Development workflow"
      impact: "high"
      detail: "Cannot push, create PRs, or trigger CI"
    - area: "Deployment"
      impact: "critical"
      detail: "CD pipeline cannot pull latest code"
    - area: "Monitoring"
      impact: "low"
      detail: "Dashboards still work; alert webhooks may fail"
  
  current_mitigations:
    - "Local git works; developers can continue coding"
    - "Docker images are cached; last-known-good can be redeployed"
  
  gaps:
    - "No offline CI capability"
    - "No mirror/cache for GitHub API responses"
    - "No documented runbook for GitHub outage"
  
  recommended_actions:
    - "Create a GitHub outage runbook"
    - "Evaluate git mirror for critical repos"
    - "Add GitHub API health to monitoring dashboard"
```

### Scenario Types

| Type | Purpose | Example |
|------|---------|---------|
| **Failure** | What breaks if X fails? | "What if the database goes down?" |
| **Growth** | What breaks at 10x scale? | "What if we have 1000 concurrent users?" |
| **Constraint change** | What opens up if a constraint is removed? | "What if we could use a managed service?" |
| **Technology shift** | What changes if we swap a component? | "What if we replaced Redis with Valkey?" |
| **Regulatory** | What must change if a new rule applies? | "What if we need SOC 2 compliance?" |
| **Team change** | What breaks if knowledge is lost? | "What if the primary maintainer leaves?" |

---

## Integration with Other Skills and Agents

**Standalone usage**: This skill works independently. You do not need to run archaeology or
contradiction detection first. The most common use case is simply "what are our options for X?"
— which needs no prior analysis. If you want to understand why the current approach was chosen
before generating alternatives, you *may* load `decision-archaeology-axiom`. If alternatives
reveal a contradiction, you *may* load `contradiction-detection-axiom`. Both are optional.

| Skill/Agent | Integration Point |
|-------------|-------------------|
| `decision-archaeology-axiom` | Understand past decisions before generating new alternatives |
| `contradiction-detection-axiom` | Alternatives may resolve contradictions |
| `adr-manager-axiom` | Output feeds directly into ADR "Alternatives Considered" |
| `@devils-advocate-axiom` | Stress-tests the recommended alternative |
| `@assumption-buster-axiom` | Challenges assumptions in the decision frame |
| `@redteam-axiom` | Adversarially tests the recommended approach |
| `@performance-axiom` | Evaluates performance implications of alternatives |
| `@finops-cost-axiom` | Evaluates cost implications of alternatives |
| `@security-review-axiom` | Evaluates security implications of alternatives |
| `spec-kickoff-axiom` | Alternatives feed into spec design decisions |

---

## Rules

1. **Always generate at least 3 alternatives** (including "do nothing" or "status quo" when applicable).
2. **Never advocate** — present tradeoffs honestly. The recommendation should follow from the criteria, not from a predetermined preference.
3. **Hard constraints are hard** — if an alternative violates a hard constraint, eliminate it. Don't try to make it work.
4. **Soft constraints are tradeoffs** — document the cost of violating them, but don't auto-eliminate.
5. **Label unknowns** — if you don't have data for a criterion, say so. Don't invent scores.
6. **Reversibility matters** — always note whether a decision is reversible or one-way. Prefer reversible decisions when scores are close.
7. **Time-box generation** — don't spend more time generating alternatives than the decision warrants. A config value choice needs 10 minutes; an architecture choice needs hours.
8. **Include "do nothing"** — the status quo is always an alternative. Sometimes it's the right one.

---

## Anti-Patterns

| Anti-Pattern | Why Bad | Fix |
|-------------|---------|-----|
| Generating alternatives after the decision is made | Retroactive justification, not real analysis | Generate alternatives before committing |
| Only generating alternatives you want to reject | Straw-man comparison; biased outcome | Use the generation strategies honestly |
| Scoring without criteria | Gut feel masquerading as analysis | Define criteria and weights first |
| Ignoring "do nothing" | Sometimes the best action is no action | Always include status quo |
| Over-engineering the comparison for trivial decisions | Wastes time; analysis paralysis | Match depth to decision importance |
| Treating weighted scores as absolute truth | Scores are a thinking tool, not a calculator | Use scores to structure discussion, not replace judgment; run sensitivity analysis |
| Presenting small score differences as meaningful | A 3.4 vs 3.1 difference with ±1 uncertainty is noise, not signal | Compute score bands; if they overlap, decide on qualitative factors |
| Scoring criteria you can't actually measure | Inventing numbers for unmeasurable criteria creates false confidence | Mark confidence as `low`; widen the range; or drop the criterion and note why |
| Skipping hard constraint checks | Wastes time evaluating impossible options | Eliminate first, then evaluate |

---

## One-Line Reminder

Before you commit to a path, make sure you've honestly considered the roads not taken.
