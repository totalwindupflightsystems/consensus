# repo-hygiene-generated-artifacts-01 — Meta Planning

## Summary

The working tree is noisy with Morty logs, state DBs, coverage files, local dev DBs, and generated binaries. This makes agent staging risky and hides meaningful changes. This work item updates ignore/hygiene rules without deleting useful source artifacts.

## Acceptance Criteria

1. Generated DB, coverage, binary, and Morty runtime artifacts are ignored or documented.
2. Source-like Morty config and work-item files remain visible to agents.
3. `git status --short` becomes meaningfully smaller after cleanup guidance is applied.
4. No tracked source/spec/memory files are removed accidentally.

axiom:trace work_item=repo-hygiene-generated-artifacts-01 spec=specs/021-repository-layout.md plan=.memory-bank/work-items/repo-hygiene-generated-artifacts-01/meta-planning.md evidence=.memory-bank/work-items/repo-hygiene-generated-artifacts-01/verification.md
