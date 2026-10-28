# INT-BOARD-1 evidence — board commit-ancestry audit

ch:trace row=INT-BOARD-1
  spec=.coding-hermes/board/tasks.jsonl#acceptance
  test=scripts/board-commit-ancestry-audit.py
  doc=scripts/board-commit-ancestry-audit.py
  evidence=docs/evidence/INT-BOARD-1/
  witness=live:git-object-store-queries
  verdict=none:foreman-will-run-after-merge

Date: 2026-10-27. Delivery ref: origin/main 71dc39a7ca45ff700cd7a64f60e6aaffc3afc1bf.

## Method

`scripts/board-commit-ancestry-audit.py` classifies every CLOSED row
(status complete/done/closed) with a present commit_hash:

- **landed** — `git cat-file -e HASH^{commit}` succeeds AND
  `git merge-base --is-ancestor HASH origin/main` succeeds.
- **landed-but-unmerged** — the object exists but is not an ancestor of the
  delivery ref.
- **missing-object** — SHA-shaped but no commit object in the repository.
  Object existence is a property of the fetched object database; recovery
  searches across `--all` refs and the reflog were performed separately
  (premise check below) and found none of these objects, so the verdict is
  stable from a clean clone.
- **non-sha-field** — the field is not a bare SHA (prose, `a + b` lists,
  `HEAD`, branch names). Never treated as a valid SHA; never fabricated.

Rows with a null/absent commit_hash are counted but not classified — no
evidence is invented for them.

## Canonical full-board run (at HEAD of this branch, pre-merge tree = 71dc39a)

Captured verbatim in `audit-full-board.txt`; machine-readable snapshot in
`audit-full-board.json`. Summary:

| class | rows |
|---|---|
| landed | 283 |
| landed-but-unmerged | 7 |
| missing-object | 7 |
| non-sha-field | 12 |
| closed rows total | 410 (309 with commit_hash, 101 without) |

### landed-but-unmerged (7)

| line | row | commit | disposition |
|---|---|---|---|
| 305 | ROUTE-ADD-040 | 75e417a (branch tip 7fabf15) | **merged by INT-BOARD-1** — wt/route-add-040 union-merged onto main (handler, tests, MountPatterns, one-segment v2-envelope 401 rule, served-surface row, regenerated artifact); merge completed the 404 arm (isNoRowsResult — the branch's declared 404 was unreachable live) |
| 314 | ROUTE-ADD-049 | 3e0b67f | **merged by INT-BOARD-1** — handler + tests cherry-applied, 401 arm completed via isApiSessionModelPath (the branch's literal v2EnvelopeRoutePaths key could never match), session-existence moved to the shared no-rows seam, served-surface row added |
| 449 | CONFIG-INERT-1 | a812103 (implementation 02527d2, same chain) | **merged by INT-BOARD-1** — 02527d2 + a812103 cherry-picked clean onto main (config wiring + docs + the trace-marker line) |
| 1 | BOARD-V2 | 9b641aa | superseded — its effect is live on main: the board-v2 artifacts it introduced (board.db, board.jsonl, tasks.parquet, events.parquet, fixtures.jsonl) are tracked on main, and the canonical store has since moved to tasks.jsonl; the migration itself has nothing left to land (verified `git ls-tree HEAD .coding-hermes/board/`) |
| 25 | HARNESS-SCHEMA-001 | f139e30 | superseded — the fix content is on main via a later commit: `coreTableColumns["tasks"]` carries the corrected column list (parent_task_id/title/priority/locked_by_agent/…, no assigned_to/result) AND the regression suite `internal/harness/schema_fallback_test.go` exists with the same migration-vs-fallback sync checks (verified at HEAD) |
| 28 | SHIM-EVENT-001 | da18e91 | superseded — the /event handler on main flushes SSE headers immediately (`flusher.Flush()`) and replays stored events on subscribe (`replaySessionEvents`, server.go:1272-1296); the late-subscriber hang this row fixed is not reproducible at HEAD |
| 29 | SHIM-VCS-001 | fa6eb6f | superseded — the /instance/* 501 stub registry exists on main via the SHIM-GAP-005 wave (`/instance/` sub-handler in server.go, the S010..017 `x-shim-extension` rows in the served surface); the stubs were registered without this commit's tree |

### missing-object (7) — recovery searches performed, no fabrication

`git cat-file -e` across the object store, `git rev-list --all --reflog`,
and `git cat-file --batch-all-objects` prefix search all come up EMPTY for
every hash below. They are unrecoverable in this repository (write short-hash
backfill rows that predate the JSONL board; the objects were never fetched to
this clone or were GC'd upstream):

| line | row | hash |
|---|---|---|
| 2 | BUG-008 | d506e33 |
| 4 | C-GAP-001 | c86961c |
| 5 | C-GAP-002 | c86961c |
| 6 | C-GAP-003 | c86961c |
| 240 | ROUTE-FIX-020 | 2d3e4f5 |
| 290 | ROUTE-ADD-025 | 9dbbf05d137b7c0 |
| 383 | QA-CONSENSUS-24 | 086b6287e |

### non-sha-field (12)

Prose / multi-commit / symbolic values — classified, never parsed as SHAs:
BUG-009, BUG-010, C-GAP-017, DOGFOOD-103, DF-CONSENSUS-13 ("FOREMAN-DIRECT"),
SHIM-SOURCE-003 ("HEAD"), ROUTE-FIX-023, ROUTE-ADD-062 ("origin/master"),
ROUTE-ADD-063, ROUTE-ADD-110, ROUTE-ADD-111, ROUTE-ADD-114.

## Rerun

```bash
python3 scripts/board-commit-ancestry-audit.py            # human-readable
python3 scripts/board-commit-ancestry-audit.py --json     # machine-readable
python3 scripts/test_board_commit_ancestry_audit.py       # 8 unit tests
```

The tool is deterministic: no timestamps, rows ordered by board line number,
stable key order in JSON — two runs over the same board and refs are
byte-identical, including from a clean clone.
