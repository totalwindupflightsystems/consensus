# Completion scoring (reference)

Goal: stop when spec quality is high enough for the chosen target tier.

This is a thinking tool, not a mechanical checkbox.
The score exists to make "we're done" falsifiable and auditable.

## Required fields from each review agent
- `status`: PASS|WARN|FAIL|BLOCKED
- `score`: 0-100
- `notes`: 3-10 bullets, each tied to a specific spec section to change (or "none")
- `blockers`: list of questions or missing inputs (may be empty)

## Aggregation rule
- If any review returns `BLOCKED`: overall = BLOCKED.
- Otherwise:
  - readiness_avg = average(scores)
  - readiness_min = min(scores)
  - readiness_status:
    - PASS if readiness_avg and readiness_min meet the tier thresholds and there are no FAILs
    - WARN if thresholds met but one or more WARNs exist
    - FAIL if thresholds not met or any FAIL exists

Dynamic guardrails:
- If `risk_posture=high`, require a security floor when security reviewers were invoked (example: `security-review-axiom` score >= 80).
- If target tier implies operability (production/battle-tested), require an ops floor when ops reviewers were invoked (example: `sre-ops-axiom` score >= 75).
- Any FAIL must be resolved or the target tier must be downgraded explicitly.

## Tier threshold defaults
- `mvp` or earlier: avg>=70, min>=50
- `alpha`/`beta`: avg>=80, min>=65
- `production`: avg>=85, min>=70
- `battle-tested`: avg>=90, min>=75

## Anti-gaming rules
- Any FAIL must name a specific missing contract element (e.g., missing negative cases, missing auth requirement, contradiction).
- A "looks good" without pointers is invalid.

Dynamic rule:
- Score adjustments must be justified by concrete spec deltas (which requirement became testable, which contradiction was resolved).
