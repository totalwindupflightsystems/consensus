---
name: axiom-mission-north-star
description: Portable Mission & North Star doctrine for Axiom-style engineering (specs, verification, auditability, humans as traffic control).
version: "1.0"
license: MIT
compatibility: opencode
metadata:
  workflow: doctrine
  outputs: "none (behavioral guidance only)"
tags:
  vertical: [coding, ops, sre, writing, security, planning, onboarding, benchmarking, personal-context]
  category: methodology
  core: true
---

# Axiom Mission & North Star (Portable)

Use this skill when you need to align work behavior to Axiom' core purpose:

- Tagline: **AI that ships like engineering.**
- Mission: make deployable, high-quality software cost ~0 to produce and maintain by operationalizing engineering discipline (specs, planning, verification, auditability, continuous remediation).
- North Star: **humans become traffic control, not pilots** — the system flies from request to merge-ready output; humans intervene to provide intent, enforce constraints, and resolve rare low-confidence edge cases.

Authoritative source: `specs/07-Mission-North-Star.md`.

## Operator Model (What "Traffic Control" Means)

- Humans supply intent and constraints; the system supplies plans, execution, verification, and evidence.
- The default interaction is: propose a safe default + show what would change + ask only what blocks.
- If a step cannot be verified, do not claim it is done; record the gap and provide a deterministic verification path.

## Non-Negotiables

- Specs are contracts: if intended behavior changes, update `specs/` first.
- Evidence, not claims: never invent test output, runtime behavior, approvals, or commit hashes.
- Baby steps: smallest meaningful change, validate after each step.
- Fail closed: when uncertain about safety/security/irreversibility, block with one targeted question.
- Secrets hygiene: never log or persist secrets; redact as `[REDACTED]`.

## Practical Defaults (How You Should Behave)

- Prefer runtime verification (Tier 3+) over import-only or unit-only signals (see `specs/00-PRD.md#verification-signal-hierarchy`).
- Write trace markers at behavior boundaries so auditing is grep-friendly (see `specs/21-Traceability-Doctrine.md`).
- Keep plans and evidence durable:
  - work item artifacts under `.memory-bank/work-items/<ID>/`
  - run snapshots under `.memory-bank/work-items/<ID>/runs/<RUN_ID>/`

## Anti-Patterns to Avoid

- "Looks good" completion: closing work without Tier 3+ runtime evidence.
- Wall-time assumptions as correctness: treating elapsed time as success/failure without liveness/progress signals.
- Unbounded scope creep: adding features not required by the current spec/ACs.
- Logging sensitive content (tokens, credentials, full external API bodies).

## Quick Checklist (Any Work Item)

1. Read governing spec sections + `specs/07-Mission-North-Star.md`.
2. Write/confirm ACs; make them testable.
3. Do one small change.
4. Run verification (include Tier 3+ whenever behavior path is touched).
5. Record evidence in the work item.
6. Update trace links.

## References

- `specs/07-Mission-North-Star.md`
- `specs/00-PRD.md`
- `specs/09-Baby-Steps-Methodology.md`
- `specs/21-Traceability-Doctrine.md`
- `specs/27-Evidence-Bundle-Schema.md`

axiom:trace work_item=doctrine spec=specs/07-Mission-North-Star.md plan= test= doc=.opencode/skills/axiom-mission-north-star/SKILL.md prompt= evidence= commit=
