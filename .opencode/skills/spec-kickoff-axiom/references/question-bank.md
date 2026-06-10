# Question bank (reference)

Use these as seeds; prefer subagent-generated questions tailored to the project.

This file is about how to THINK: generate and rank questions dynamically.

## How to pick the 3-7 questions (dynamic)

1) List unknowns implied by the source material.
2) Score each unknown by: risk * blast_radius * irreversibility.
3) Pick the top 3-7 that define contract boundaries.

Prefer questions that change a MUST-level requirement.
Avoid questions that are implementation detail at the current tier.

## Universal (always)

Boundary questions:
- Who is the primary user and what is their job-to-be-done?
- What is the smallest success outcome?
- What is explicitly out of scope?
- What are the top 3 failure modes we must handle?

Verification seed:
- How will we know this worked (observable outcomes / tests / demo criteria)?

## Data + privacy (if any data exists)
- What data classes exist (PII/secrets/financial/etc)?
- Where is data stored and for how long?
- Who can access it and how is access audited?

Dynamic prompt:
- If any data class is "regulated" or "PII", require explicit retention and deletion requirements.

## Security (if auth/permissions)
- Who can do what actions?
- What are the trust boundaries?
- What is the abuse scenario you fear most?

Dynamic prompt:
- If auth exists, require a decision: default-deny vs default-allow.

## Operability (if production)
- What does "down" mean for this system?
- What metrics/logs define healthy behavior?
- What is the rollback story?

Dynamic prompt:
- If irreversibility is high (migrations, deletes), require: rollback + backups + safety rails.

## UX/copy (if user-facing)
- What terms should we use/avoid?
- What are the error states and recovery instructions?

Dynamic prompt:
- If the UI has forms/modals/complex navigation, consider an optional a11y branch (do not block spec generation).
