# ROUTE-FIX-003 live probe — POST /instance/dispose

ch:trace row=ROUTE-FIX-003
  spec=specs/openapi/upstream/openapi-1.18.33.json#instance.dispose
  test=internal/shim/opencode/instance_dispose_test.go (TestInstanceDispose*)
  doc=docs/evidence/ROUTE-FIX-003-live-probe.md
  evidence=docs/evidence/ROUTE-FIX-003-live-probe.md
  witness=none:unattended-worker-session — scripted curls against a locally
    built binary; every value below is copied verbatim from the probe
    transcripts (/tmp/rf003/red-transcript.txt, /tmp/rf003/green-transcript.txt).

Date: 2026-10-04 (local, UTC-05)
Worktree: /home/kara/consensus-wt-ROUTE-FIX-003 (branch `wt/ROUTE-FIX-003`;
rebuilt on master 05a52a2 after the original zero-commit worktree at c6babe3
was reaped mid-task by the foreman's wave-close tick — the fix itself was
never merged anywhere before this landing, see §5).

A locally built `consensus` binary was served on a **fresh SQLite database**
and the opencode shim routes were driven over HTTP. Every value below is
copied from a live HTTP response — none is hand-typed.

## 1. Run identity

| property | value |
|---|---|
| binary (pre-fix RED) | `/tmp/rf003/consensus-prefix` (sha256 `6c92ab4b4f0c4624c7c72fdd93a372e77cd8dc42f66f921d3bcbf46335e51dfe`), built from master c6babe3 |
| binary (post-fix GREEN) | `/tmp/rf003/consensus-fixed` (sha256 `f94c298b528af9481229a1bae44b561fec65dffb18cd2ea5a24de0c714c20784`), built from commit c619b26 (this fix) |
| server | `127.0.0.1:45167`, shim adapter `opencode` (default); `/global/health` = `200 {"healthy":true,"version":"consensus-0.1.0"}` |
| db | `sqlite:///tmp/rf003/probe.db?_journal_mode=WAL` (config `/tmp/rf003/consensus.yaml`), created by `consensus init` (`Migrations: applied`) |
| auth | bootstrap admin key (`cs_ak_15…`, printed once by init); used as `Authorization: Bearer …` — the full value appears only in the local transcript, masked here |

## 2. Declared contract (specs/openapi/upstream/openapi-1.18.33.json#instance.dispose)

- `POST /instance/dispose` — "Clean up and dispose the current OpenCode
  instance, releasing all resources." Declared responses: `200` boolean
  ("Instance disposed"), `400` `BadRequestError`. Optional query params
  `directory`, `workspace` (both strings). No requestBody declared.

## 3. Pre-fix RED (binary at master c6babe3, before the change)

Every arm below answered the typed 501 stub from `handleInstanceSub`'s
default arm (`instanceKnownSubpaths["dispose"]`):

| request | observed (pre-fix) |
|---|---|
| `POST /instance/dispose` | `501 {"error":{"code":"NOT_IMPLEMENTED","message":"endpoint \"dispose\" is opencode-specific, not supported by Consensus shim"}}` |
| `POST /instance/dispose?directory=/tmp/rf003&workspace=/tmp` | `501` same body |
| `POST /instance/dispose?directory=` | `501` same body |

Context (pre-fix, unchanged by the fix): `GET /instance` → `200 [{"createdAt":"2026-10-04T06:42:32Z","id":"consensus-b29a99aece8d","path":"/tmp/rf003","updatedAt":"2026-10-04T06:43:00Z"}]`; `GET /instance/vcs` → `200 {}`; `POST /instance/command` → `501`; `GET /instance/definitely-not-a-thing` → `404 {"error":{"code":"NOT_FOUND","message":"endpoint not found"}}`.

## 4. Post-fix GREEN (binary at c619b26)

| request | observed (post-fix) |
|---|---|
| `POST /instance/dispose` (call 1) | `200 true` |
| `POST /instance/dispose` (call 2) | `200 true` (idempotent) |
| `POST /instance/dispose?directory=/tmp/rf003&workspace=/tmp` | `200 true` (valued selectors are well-formed, scope nothing) |
| `POST /instance/dispose?directory=` | `400 {"data":{"kind":"Query","message":"query parameter \"directory\" must not be blank"},"name":"BadRequest"}` |
| `POST /instance/dispose?workspace=%20%09` | `400 {"data":{"kind":"Query","message":"query parameter \"workspace\" must not be blank"},"name":"BadRequest"}` |
| `GET /instance` | `200 [{"createdAt":"2026-10-04T07:07:42Z","id":"consensus-b29a99aece8d","path":"/tmp/rf003","updatedAt":"2026-10-04T07:07:50Z"}]` (neighbour intact) |
| `GET /instance/vcs` | `200 {}` (neighbour intact) |
| `POST /instance/command` | `501 {"error":{"code":"NOT_IMPLEMENTED","message":"endpoint \"command\" is opencode-specific, not supported by Consensus shim"}}` (sibling stub unchanged) |
| `GET /instance/definitely-not-a-thing` | `404 {"error":{"code":"NOT_FOUND","message":"endpoint not found"}}` (unchanged) |

The `400` body is the declared `BadRequestError` schema (`name: "BadRequest"`,
`data.kind` in the declared enum, `data.message` naming the offending
parameter) emitted by the shared `writeOpencodeBadRequest` helper — the same
convention `handleSkill`/`handleFormatter` use for their blank query params.

## 5. Truthfulness note (what the 200 does and does not claim)

`true` is the truthful answer to the declared contract under the shim's
singleton-instance model: there is no per-instance registry to release, the
Consensus server IS the instance, and an HTTP request to a compatibility
shim does not shut the server down (the sibling `POST /global/dispose`
precedent, ROUTE-ADD-091). The response deliberately does NOT fabricate a
"released N resources" payload — the schema declares a bare boolean and the
shim answers the bare boolean.

## 6. Test + artifact verification (same commit c619b26)

- `go build ./...` — clean.
- `go vet ./internal/shim/opencode/` — clean.
- `go test -short -count=1 ./internal/shim/opencode/` — `ok … 9.074s` (the
  GitReins pre-commit guard additionally ran the repo-wide
  `go test -short` lane: PASS on every package).
- Tests verified RED before the fix: every served arm
  (`TestInstanceDisposeAnswersDeclaredBoolean`, the chi-mount cell, the
  blank-param cells) failed against the pre-fix handler with the 501 body
  above; neighbour cells (501 stubs, 404s) passed unchanged.
- Artifact: `specs/openapi/upstream/opencode-declared-vs-served-1.18.33.json`
  — SHIM-DRIFT-091 removed from `drift_declared_not_served` (128 → 127,
  OUTCOME-MISMATCH 25 → 24), covered row added (52 → 53). Regenerated with
  the real comparator
  (`python3 scripts/compare-opencode-declared-served.py`) from the patched
  `consensus-shim-served-surface.yaml`: counts identical, covered set and
  drift-row key set (path, method, class) content-identical to the committed
  artifact. Targeted flip, not a full regen — the SHIM-DRIFT-NNN ids are
  positional and a regen would renumber every row from 091 up (documented,
  accepted; same approach as ROUTE-FIX-008/009/010).
