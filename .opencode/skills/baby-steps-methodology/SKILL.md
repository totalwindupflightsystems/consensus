---
name: baby-steps-methodology
description: Portable Baby Steps rules (smallest meaningful change, validate each step, document with evidence).
version: "1.0"
tags:
  vertical: [coding, ops, sre, writing, security, planning, onboarding, benchmarking, personal-context]
  category: methodology
  core: true
---

# Baby Steps Methodology (Portable)

Operate under a single overriding principle: baby steps.

Primary goal: complete tasks by demonstrating a reliable, auditable process. The process is part of the deliverable.

## Unbreakable Rules

1) Smallest possible meaningful change
- Break work into the smallest atomic step that is understandable and verifiable.

2) The process is part of the product
- Prefer repeatable workflows and traceable outputs over clever shortcuts.

3) One substantive accomplishment at a time
- Do not start a second meaningful change until the current one is complete.

4) Complete each step fully
- A step is not done until it is implemented, validated, and documented.

5) Incremental validation is mandatory
- Validate after every step (tests, lint, checks, or targeted verification).

6) Document every step with focus
- Record what changed, why, how it was validated, and what happens next.

## Practical Step Template

Use this template to keep steps small and auditable:

```text
Step objective:
Actions:
- ...

Verification:
- Command/test to run:
- Expected result:

Evidence:
- Where recorded (e.g., .memory-bank/work-items/<ID>/runs/<RUN_ID>/verification.md)

Rollback:
- How to undo safely:
```

## When To Stop And Ask

Stop and ask (up to 7 questions) when:
- acceptance criteria are not testable
- changes are destructive/irreversible
- production/security posture changes without explicit constraints
