# SHIM-GAP-004 — the narrowed / error-contract-only operations

Row: `SHIM-GAP-004` (P2) — "The 11 narrowed / error-contract-only operations:
widen to the declared contract or declare the narrowing".
Pin: upstream v1.18.33 (`7945de208964a49300d7f770d1a71d078db9a4c4`),
surface `specs/openapi/upstream/openapi-1.18.33.surface.json`.
Artifact: `specs/openapi/upstream/opencode-declared-vs-served-1.18.33.json`.

## What was wrong

Eleven operations in the 2026-09-29 triage baseline
(`SHIM-NARROWED-001…011`, `docs/reports/shim-source-002-baseline-diff-2026-09-29.md`)
were answered by the shim with only the ERROR half of their declared contract: a
client that calls them can only ever receive an error, and — worse — the
narrowing was INVISIBLE from outside. The comparator scored a row `covered` when
the served status was merely IN the declared set, so `POST /pty/{ptyID}` (declared
200/400/404, answered 404 for every well-formed id) and
`POST /mcp/{name}/auth` (declared 200/400/404, answered 400) looked fully covered.

## What changed

### Widened to the full declared contract (2)

| operation | path | before | after |
|---|---|---|---|
| `permission.reply` | `POST /permission/{requestID}/reply` | 404-typed only (`server.go`) | 400 (ill-shaped id, schema-violating body, absent/empty/non-enum `reply`), 404 typed `PermissionNotFoundError` (unknown id or not pending), **200 boolean `true` after the real `approval_requests` UPDATE + `approval_resolved` SSE** |
| `tui.selectSession` | `POST /tui/select-session` | `/tui/` catch-all 404 only | 400 (body violating `{sessionID*}`, absent/blank id), 404 `NotFoundError` (unknown session), **200 boolean `false`** (no TUI process is attached — the `tui.showToast`/`tui.executeCommand` precedent) |

`permission.reply` was widenable because the runtime already owned the state and
the write path (the same `approval_requests` rows `permission.respond` /
`POST /permission/{id}/resolve` resolve). `tui.selectSession` needed no new state
at all: session existence is a real lookup and the declared 200 boolean is a
truthful `false`.

Partial widening: `question.reply` now validates its declared
`{answers*: QuestionAnswer[]}` body and answers the declared 400 for a
schema-violating request (previously such a request fell through to the 404 —
the DF-CONSENSUS-40 defect class).

### Recorded as an explicit narrowing (14, all with reasons)

Detected by the comparator's new
`is_error_contract_narrowing()` and recorded on the served-surface row with
`x-shim-error-contract-narrowing: true` + `narrowing-rationale`:

`question.reject`, `question.reply`, `part.delete`, `part.update`,
`pty.get`, `pty.update`, `pty.remove`, `pty.connect`, `pty.connectToken`,
`mcp.auth.start`, `mcp.auth.authenticate`, `mcp.auth.callback`,
`session.share`, `session.unshare`.

The two `part.*` rows and the last ten above were previously scored `covered`;
they answer the declared 400/403/404 but their declared 200 describes runtime
state that does not exist (no part store; no PTY registry; no MCP OAuth runner /
MCP client; no session-share concept). The reasons are per-row in SPEC-018 §12.3
and on each served-surface row.

### The class can no longer be silent

* `scripts/compare-opencode-declared-served.py` now DETECTS an error-contract
  narrowing from the declared response set and the served outcome (never a hand
  list — the old `ERROR_CONTRACT_ONLY` set is deleted), requires the marker plus a
  non-empty rationale, and **exits non-zero naming the row** otherwise, including
  for a router catch-all that answers a declared error code while the operation
  declares a success code.
* The artifact carries `counts.narrowed_unexplained` — the number of narrowed rows
  without a rationale. It is `0` and cannot be anything else: a run that would
  produce a non-zero value aborts instead of writing the artifact.
* `internal/shim/opencode/shim_gap_004_test.go`
  (`TestDeclaredVsServedAccountsForEveryNarrowing`) asserts the count is 0, that
  every narrowed row carries a rationale, that the declared count agrees with the
  enumerated rows, and that each of the 11 findings is either covered or recorded
  with a rationale.
* The 17 shim extensions' per-entry `decision` / `decision_reason` (SHIM-GAP-005)
  are now carried on their served-surface rows and emitted by the comparator, so
  a regeneration reproduces the artifact instead of dropping them.

## Counts

| count | before | after |
|---|---|---|
| `covered_operations` | 92 | 83 |
| `narrowed_error_contract_only` | 5 | 14 |
| `narrowed_unexplained` | (absent) | **0** |
| `intentional_narrowing` | 1 | 1 (`pty.create`) |
| `semantic_different` | 2 | 2 (the `/mcp` delegation) |
| `drift_declared_not_served` | 88 | 88 |
| `drift_served_not_declared` | 0 | 0 |
| `shim_extension_operations` | 17 | 17 |
| `served_route_entries` | 120 | 121 (`/tui/select-session` is now a served route) |

Coverage went DOWN and narrowings went UP on purpose: the previous classification
counted an answered error code as full coverage. `covered + narrowed +
intentional + semantic + drift = 188` declared operations in both readings.

## Evidence

Reproduce (no server needed):

```bash
python3 scripts/compare-opencode-declared-served.py \
  specs/openapi/upstream/openapi-1.18.33.surface.json \
  specs/openapi/upstream/consensus-shim-served-surface.yaml /tmp/sds
diff <(jq -S . /tmp/sds/declared-vs-served.json) \
     specs/openapi/upstream/opencode-declared-vs-served-1.18.33.json   # no output
```

Live arms (real SQLite store, real approval row; `go test -run` output):

```
TestPermissionReplyServesDeclaredSuccess      POST /permission/per_gap004/reply {"reply":"once"}
   -> 200 true; approval_requests row: status=approved, review_notes="granted once",
      reviewer_id=opencode-shim, reviewed_at set; replay -> 404 PermissionNotFoundError
TestPermissionReplyRejectMapsToRejected       {"reply":"reject","message":"not this time"}
   -> 200 true; status=rejected, review_notes carries the rejection marker + message
TestPermissionReplyDeclaredErrorArms          8 body violations + ill-shaped id -> declared 400
TestTUISelectSessionServesDeclaredContract    known session -> 200 false;
   unknown -> 404 NotFoundError; 6 body violations -> 400; GET -> 405
TestQuestionFamilyServesDeclaredErrorArms     8 body violations -> declared 400;
   schema-valid body / no body -> declared 404; ill-shaped id -> declared 400
TestDeclaredVsServedAccountsForEveryNarrowing artifact: 0 unexplained narrowings,
   14 recorded, 11/11 findings accounted
```

RED proof for the artifact leg: before the artifact regeneration the same test
failed with `artifact records … (…) as narrowed with no rationale — an unexplained
narrowing` for all five rows of the pre-change artifact, plus
`SHIM-GAP-004 finding … is recorded as narrowed with an empty rationale` for each
of the five.

## Known residue (reported, not hidden)

* `meta.derived_at_commit` / `meta.derived_on` in the served-surface file still
  name the last full derivation (89394b1, 2026-10-07) although the route table has
  changed since; every intervening route row left it stale the same way. It is a
  documentation-rot item, not a contract claim, and needs its own row.
* `part.delete` / `part.update` could be converted to full coverage only by making
  message parts addressable (a stable `prt` id for the synthesized part), which is
  a design change to `getMessageByID`'s output — deliberately NOT taken here.
* `pty.*` and `mcp.auth.*` full coverage needs a PTY registry and an MCP OAuth
  runner respectively; both are feature work with their own rows.
