---
mb:
  type: finding
  title: "Generated Artifact Hygiene — gitignore patterns and tracked-stale-artifact removal"
  created: 2026-05-28
  updated: 2026-05-28
  tags: [finding, process, anti-pattern]
  severity: medium
  status: addressed
  links:
    up: "../_index.md"
    related: []
  source:
    type: self-discovery
    ref: "work_item=repo-hygiene-generated-artifacts-01"
  git:
    commit: ""
    paths:
      - ".gitignore"
      - ".axiom/state/materializer-state.json"
      - ".morty/"
    blame: ""
---

# Finding: Generated Artifact Hygiene

## Summary

The repository had two categories of generated/local artifacts that polluted `git status`:

1. **Tracked state files** — `.axiom/state/materializer-state.json` (Axiom runtime state) and all `.morty/` files (Morty test runner state, logs, DBs, cycle outputs) were tracked in git, so every change caused noise in `git status`.
2. **Untracked run output** — `runs/` directories under `.memory-bank/work-items/` accumulated per-timestamp test-run artifacts that showed as untracked.

## Details

- **Trigger**: `git status` showed modified `.axiom/state/materializer-state.json` on every Axiom session, plus untracked `runs/` directories.
- **Impact**: Git status noise makes it harder to spot meaningful changes, increases risk of accidentally committing transient state, and adds cognitive load to every commit.
- **Root cause**: Generated/local artifacts were added to `.gitignore` *after* they had already been committed, so the gitignore pattern had no effect on tracked files. No systematic pattern existed for ephemeral run outputs.

## Prevention / Fix

- **Rule**: Add generated/local artifacts to `.gitignore` BEFORE the first commit that would introduce them. If they're already tracked, `git rm --cached` must be done as part of the hygiene pass.
- **Process**: When creating work items that produce ephemeral run output (e.g., test runners), add a `**/runs/` gitignore entry in the same PR.
- **Checklist item** (for future): Before declaring a work item with runtime/loop/daemon test output as done, verify that any generated files are gitignored.

## Changes Applied

1. **`.gitignore`** — Added `**/runs/` pattern for ephemeral test-run output under work items. (`.morty/`, `/conscience`, `/bin/`, `dev.db`, `*.db-shm`, `*.db-wal` entries were already added in the same uncommitted change.)
2. **`git rm --cached`** — Untracked `.axiom/state/materializer-state.json` and all `.morty/` files so the existing `.gitignore` patterns take effect.

## Links

- [Up: Findings Index](../_index.md)
- [Spec: Repository Layout](../../specs/021-repository-layout.md)

## Traceability

- **Source**: Self-discovery during work item `repo-hygiene-generated-artifacts-01`
- **Git**: See commit for this work item
