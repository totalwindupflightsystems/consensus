# repo-hygiene-generated-artifacts-01 — Plan

Reduce generated-artifact noise so future agents can stage and review changes safely. This is a hygiene change, not a source-code cleanup.

## Steps

1. Inspect `.gitignore`, `.ignore`, and current dirty generated artifacts.
2. Add ignore rules for local DBs, coverage outputs, built binary, and Morty runtime logs/state where appropriate.
3. Do not ignore source configs like `.morty/consensus.json` unless intentionally generated.
4. Verify with `git status --short` and targeted ignore checks.

## Verification

- `git status --short`
- `git check-ignore -v <sample-generated-file>` for representative artifacts.

## Rollback

Revert ignore-rule changes. Do not delete user work.

axiom:trace work_item=repo-hygiene-generated-artifacts-01 spec=specs/021-repository-layout.md plan=.memory-bank/work-items/repo-hygiene-generated-artifacts-01/plan.md evidence=.memory-bank/work-items/repo-hygiene-generated-artifacts-01/verification.md
