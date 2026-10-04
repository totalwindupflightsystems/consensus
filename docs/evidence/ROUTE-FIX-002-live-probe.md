# ROUTE-FIX-002 live probe — GET /find/symbol

ch:trace row=ROUTE-FIX-002
  spec=specs/openapi/upstream/openapi-1.18.33.json#find.symbols
  test=internal/shim/opencode/find_symbol_routes_test.go (TestFindSymbols*)
  doc=docs/evidence/ROUTE-FIX-002-live-probe.md
  evidence=docs/evidence/ROUTE-FIX-002-live-probe.md
  witness=none:self-verified-in-worktree — scripted curls against a locally built
    binary; every value below is copied verbatim from the probe transcript
    (/tmp/t002-run/probe-transcript.txt, reproduced in §2). verdict=/commit=
    are deliberately absent in-tree: a verdict binds to a commit that already
    exists, so it is recorded by the commit footer / foreman verification,
    never self-referenced by the change it judges.

Date: 2026-10-04 (local, UTC-05) / 2026-10-04T07:16Z
Worktree: `/home/kara/consensus-wt-ROUTE-FIX-002` (branch `wt/ROUTE-FIX-002`,
base `c6babe3` — the worktree was recreated at the same path on the same base
mid-task after a wave reaper removed the zero-commit checkout; the probe ran
with this change applied to the recreated tree, and the committed diff is
identical to the probed tree).

A locally built `consensus` binary was served on a **fresh SQLite database**
and the opencode shim route was driven over HTTP. Every value below is copied
from a live HTTP response — none is hand-typed.

## 1. Run identity

| property | value |
|---|---|
| binary | `/tmp/t002-consensus` (sha256 `ad022c98b977355ebc71c43530c9c02f0e82d1967f37f7a073f488e090ba26af`) |
| build | `go build -o /tmp/t002-consensus ./cmd/consensus`, from the worktree root |
| worktree / branch | `/home/kara/consensus-wt-ROUTE-FIX-002` @ `wt/ROUTE-FIX-002`, base `c6babe3` |
| server | `127.0.0.1:18602`, backend `sqlite`; `/global/health` = `{"healthy":true,"version":"consensus-0.1.0"}` |
| db | `sqlite:///tmp/t002-run/t002.db`, created by `consensus init` (auto-migrate; `api_keys`/`sessions`/`tasks`/... schema) |
| sqlite3 | used only to confirm the bootstrap key row landed (`api_keys` → `cs_ak_55…`, scope `admin`) |
| auth | bootstrap admin key (Bearer). The value is extracted from the boot's stdout into a shell variable and used from a curl header — the key never appears in any transcript or evidence file |

## 2. Declared contract, live

`<code> <body>` below is curl's HTTP status and the response body, verbatim.

| arm | request | observed |
|---|---|---|
| valid `?query=` (declared 200) | `GET /find/symbol?query=handleFind` | `200 []` — the truthful EMPTY list (an array, never null) |
| declared optional `?workspace=` selector | `GET /find/symbol?query=handleFind&workspace=default` | `200 []` — the same empty list (the selector is validated, not narrowing) |
| both declared optional selectors | `GET /find/symbol?query=handleFind&directory=/tmp&workspace=default` | `200 []` |
| missing `?query=` (declared 400) | `GET /find/symbol` | `400 {"error":{"code":"INVALID_REQUEST","message":"?query= is required"}}` |
| blank `?query=` (declared 400) | `GET /find/symbol?query=` | `400 {"error":{"code":"INVALID_REQUEST","message":"?query= is required"}}` |
| whitespace `?query=` (declared 400) | `GET /find/symbol?query=%20` | `400 {"error":{"code":"INVALID_REQUEST","message":"?query= is required"}}` |
| blank `?directory=` (declared 400) | `GET /find/symbol?query=handleFind&directory=` | `400 {"error":{"code":"INVALID_REQUEST","message":"query parameter \"directory\" must not be blank"}}` |
| blank `?workspace=` (declared 400) | `GET /find/symbol?query=handleFind&workspace=` | `400 {"error":{"code":"INVALID_REQUEST","message":"query parameter \"workspace\" must not be blank"}}` |
| plural spelling (same prefix arm) | `GET /find/symbols?query=handleFind` | `200 []` |
| unknown find sub-path | `GET /find/nothing?query=x` | `404 {"error":{"code":"NOT_FOUND","message":"unknown find sub-path"}}` |
| no credentials | `GET /find/symbol?query=handleFind` (no Authorization header) | `401 {"error":{"code":"UNAUTHENTICATED","message":"invalid credentials"}}` — the route requires auth (`auth: api-key` in the served surface; not on the fixed-workspace exemption list, same policy as GET /question) |
| undeclared method POST | `POST /find/symbol?query=handleFind` | `501 {"error":"not_implemented","operation":"find.symbols","detail":"symbol search is a GET operation; use GET /find/symbol?query=\u003cname\u003e"}` — typed per the session.diff convention (405 is not in the declared response set) |
| undeclared method PUT | `PUT /find/symbol?query=handleFind` | same typed 501 |

Neighbours (must be unchanged by this route):

| arm | request | observed |
|---|---|---|
| sibling file search | `GET /find/file?query=*.go` | `200 {"count":0,"files":[],"query":"*.go"}` |
| root find | `GET /find?pattern=*.go` | `200 {"count":0,"files":[],"pattern":"*.go"}` |

## 3. Why the declared 200 is a translation, not a fabrication

The shim has no LSP integration: `handleLSP` reports
`{"enabled":false,"status":"unavailable"}` unconditionally, no language server
is spawned anywhere in the runtime, and there is no symbol table to query. The
declared 200 body is `Symbol[]` (name, kind, location{uri, range}) — shapes no
Consensus component produces. The truthful answer to "search workspace
symbols" is therefore the empty list: the handler validates the request,
reports no symbol producer, and answers `[]`. Any non-empty body would
fabricate Symbol entries that no tool produced — the exact fabrication the
handleSyncStart / handleGlobalUpgrade truthfulness convention (13189b1) and
the question.list precedent (ROUTE-ADD-110) forbid.

## 4. Declared-vs-served flip

`specs/openapi/upstream/consensus-shim-served-surface.yaml` moves the
`/find/symbol` GET route to `outcome: 200` (the `/find/` prefix rule now
covers only the UNDECLARED methods on the symbol sub-path, `stub_outcome:
501-typed`), and
`specs/openapi/upstream/opencode-declared-vs-served-1.18.33.json` has
SHIM-DRIFT-085 flipped to covered. Targeted flip, not a full regeneration
(the SHIM-DRIFT-NNN ids are positional — the session.todo/ROUTE-ADD-110
approach), then verified against the real comparator
(`python3 scripts/compare-opencode-declared-served.py` on the patched yaml):

| measure | before | after |
|---|---|---|
| `covered_operations` | 52 | **53** (`find.symbols` enters) |
| `drift_declared_not_served` | 128 (NOT-SERVED 103, OUTCOME-MISMATCH 25) | **127** (NOT-SERVED 103, OUTCOME-MISMATCH **24**) |
| `served_route_entries` | 101 | 101 (the route entry already existed — outcome flips in place) |
| `narrowed_error_contract_only` | 6 | 6 (unchanged) |
| `drift_served_not_declared` | 17 (PATH-ABSENT 16, METHOD-ABSENT 1) | 17 (unchanged) |

Comparator cross-check: the hand-flipped artifact's covered/drift
(path, method, served_outcome) multisets are IDENTICAL to the comparator's
fresh output on the patched yaml, `find.symbols` is covered at 200, and no
`/find/symbol` drift row remains. The 501-typed census pinned by
`TestNotImplementedTableMatchesDriftArtifact` stays 15 (SHIM-DRIFT-085 was an
OUTCOME-MISMATCH row with served_outcome "501", never a member of the
`notImplementedRoutes` table).

## 5. Test evidence

`go test -short -count=1 ./internal/shim/opencode/` →
`ok github.com/wojons/consensus/internal/shim/opencode 7.476s`
(verified RED before the fix: every `TestFindSymbols*` arm answered the 501
stub on the pre-fix tree).
