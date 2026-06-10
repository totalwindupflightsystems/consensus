---
name: axiom-confidence-scoring
description: Portable confidence scoring model (signals, weights, formula, thresholds).
version: "1.0"
tags:
  vertical: [coding, ops, sre, writing, security, planning, onboarding, benchmarking, personal-context]
  category: verification
  core: true
---

# Confidence Scoring (Portable v1)

Axiom computes a confidence score as a weighted average of present signals. Missing signals MUST NOT inflate confidence.

## Invariants

- Deterministic given the same inputs.
- Explainable: every score has a per-signal breakdown tied to evidence.
- If required verification fails, overall status is `fail` or `blocked` regardless of numeric confidence.

## Signals (v1)

All signals are integer values in `[0, 100]`.

- `requirements_clarity`
- `spec_alignment`
- `test_coverage`
- `checks_pass_rate`
- `plan_completion`
- `ambiguity_remaining`

If a signal cannot be computed (tooling unavailable / not yet applicable), mark it as ABSENT (exclude from scoring).

## Weights

Source weights from `.axiom/axiom.config.yaml` under `confidence.weights` when present.

Default weights (v1):

```yaml
requirements_clarity: 20
spec_alignment: 20
test_coverage: 20
checks_pass_rate: 25
plan_completion: 10
ambiguity_remaining: 5
```

## Combination Formula

Weighted average of present signals:

```
score = sum(value_i * weight_i for present signals) / sum(weight_i for present signals)
```

Rules:
- Exclude ABSENT signals from both numerator and denominator.
- If ALL signals are absent: score is `0` and status MUST be `blocked` with a diagnostic noting missing signals.
- Round to nearest integer.

## Thresholds / Bands

Use repo-configured thresholds when present in `.axiom/axiom.config.yaml` under `confidence.thresholds`.

If thresholds are missing, use defaults:
- LOW: `< 40`
- MEDIUM: `40-69`
- HIGH: `>= 70`
