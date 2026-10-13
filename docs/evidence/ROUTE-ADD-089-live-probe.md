# ROUTE-ADD-089 — GET /global/config live probe

## Run identity

| Field | Value |
|---|---|
| Tree | `main` working tree (no worktree), base commit `938c63038a9b439302a9908050590acd8eea3082` |
| Binary | `/tmp/route089-live/consensus` |
| Binary SHA-256 | `b64cb3f3dd1e8c676d16a5e25804e222acb792377a73c0b576ebc95a639dcfa6` |
| Go toolchain | `go1.26.5 linux/amd64` |
| Listener | `127.0.0.1:48257` |
| Database | scratch SQLite: `/tmp/route089-live/consensus.db` |
| Credential | bootstrap admin key captured to a mode-600 scratch file; raw value never logged |
| Transcript | `/tmp/route089-live/transcript.txt` |

The probe built the binary from the base commit plus this row's working-tree delta. That delta is a
test file and two data files (`internal/shim/opencode/global_config_test.go`,
`specs/openapi/upstream/consensus-shim-served-surface.yaml`,
`specs/openapi/upstream/opencode-declared-vs-served-1.18.33.json`), none of which is compiled into
`cmd/consensus`, so the probed binary is the base-commit binary byte-for-byte. The probe used an
explicit scratch database and did not read the repository's ambient configuration.

## Premise correction (this row was filed against stale board text)

The row, and the dispatch brief derived from it, say: *"Today: no shim route at all; net/http
default 404"* and *"the shim currently does NOT serve this route (grep returns empty)"*. Both are
false at the base commit. The exact path was registered on 2026-10-01 by commit
`9657464e7f054be77d3dc456f7abcda52f9430dd` (`shim: serve PATCH /global/config per contract`,
ROUTE-ADD-090) as a single registration serving BOTH declared operations:

- `internal/shim/opencode/server.go:182` — `mux.HandleFunc("/global/config", s.handleGlobalConfig)`
- `internal/shim/opencode/server.go:3610` — `handleGlobalConfig`, GET arm answers 200, PATCH arm
  answers 200/400
- `internal/shim/opencode/global_config_test.go` — already present, covering GET 200, PATCH 200/400,
  405, neighbour isolation, chi mount, and the pinned Config schema

An exact-path `ServeMux` registration routes every method, so one path's two declared operations
cannot be served by two commits: a second registration is a duplicate `ServeMux` pattern (startup
panic) plus a duplicate handler symbol (compile error). ROUTE-ADD-090 therefore landed the GET arm
as a by-product. What was genuinely missing for this row:

1. **the declared-vs-served flip** (the row's own criterion 3) — the served-surface data file had no
   `/global/config` row, so the artifact still reported GET `global.config.get` as `NOT-SERVED`;
2. **the live-probe evidence** for criterion 2, committed as this document;
3. **the route's `AnswersDeclaredList` contract test** — every sibling route carries
   `Test<Route>AnswersDeclaredList` (`formatter_test.go`, `file_test.go`, `command_list_test.go`,
   `skill_test.go`, `sync_test.go`); `global_config_test.go` carried the equivalent assertions under
   `TestGlobalConfigGetAnswersEmptyConfig` but not the conventionally named, document-driven test
   the acceptance criteria name.

## Declared contract

The pinned upstream document is `specs/openapi/upstream/openapi-1.18.33.json`, family `/global/*`:

| Operation | Method | Path | Declared responses |
|---|---|---|---|
| `global.config.get` | GET | `/global/config` | 200 `Config`, 400 |
| `global.config.update` | PATCH | `/global/config` | 200 `Config`, 400 |

The `Config` schema sets `additionalProperties: false` and declares 36 properties; the handler's
enforced key surface is asserted one-to-one against that document by
`TestGlobalConfigDeclaredKeysMatchPinnedSchema`.

## RED proof

The new contract test is not vacuous. With the route registration at `server.go:182` commented out
and the tree otherwise identical, the targeted run reproduced the pre-fix production signature:

```
$ go test ./internal/shim/opencode/... -count=1 -run 'TestGlobalConfigAnswersDeclaredList' -v
=== RUN   TestGlobalConfigAnswersDeclaredList
    global_config_test.go:251: GET /global/config: got 404, want the declared 200 (declared set: [200 400]). Body: 404 page not found
--- FAIL: TestGlobalConfigAnswersDeclaredList (0.01s)
```

`server.go` was restored from a byte-identical copy afterwards
(sha256 `754ba6471e66c40f50bc910d3a2727180953d73a3254728fd6d7eaeb15666815`, `git diff` empty for
that path).

## GREEN live probe

Twelve arms against the scratch instance; the probe script asserted every status and JSON shape with
`jq -e` before writing the transcript, and exited 0.

| Arm | Request | Observed |
|---|---|---|
| Declared 200 | authenticated `GET /global/config` | `200 {}` — an empty JSON object, never `null` |
| Sibling 200 (same handler) | authenticated `PATCH /global/config` `{"shell":"/bin/zsh","snapshot":true,"model":"consensus/default"}` | `200 {"model":"consensus/default","shell":"/bin/zsh","snapshot":true}` — all three accepted keys echoed as the resulting Config |
| Sibling 200 — absent-body shape | authenticated `PATCH /global/config` `{}` | `200 {}` |
| Declared 400 — malformed JSON | authenticated `PATCH /global/config` `{"shell":` | `400 {"error":{"code":"INVALID_REQUEST","message":"malformed request body: unexpected EOF"}}` |
| Declared 400 — undeclared key | authenticated `PATCH /global/config` `{"shell":"/bin/sh","not_a_config_key":true}` | `400 INVALID_REQUEST`, message names the offending key and the `additionalProperties:false` premise |
| Declared 400 — explicit null | authenticated `PATCH /global/config` `null` | `400 INVALID_REQUEST` ("must be a JSON object matching the Config schema") |
| Declared 400 — array body | authenticated `PATCH /global/config` `["shell"]` | `400 INVALID_REQUEST` |
| Declared 400 — scalar body | authenticated `PATCH /global/config` `42` | `400 INVALID_REQUEST` |
| Undeclared method | authenticated `POST /global/config` | `405 {"error":{"code":"METHOD_NOT_ALLOWED","message":"use GET or PATCH"}}` |
| Undeclared method | authenticated `DELETE /global/config` | `405 METHOD_NOT_ALLOWED` |
| Neighbour guard | authenticated `GET /global/health` | `200 {"healthy":true,"version":"consensus-0.1.0"}` |
| Neighbour guard | authenticated `GET /global/unknown-sub` | `404 404 page not found` — the exact registration did not become a `/global/*` catch-all |

Additional observations recorded by the probe:

- `GET /global/config` answers `Content-Type: application/json`.
- An unauthenticated `GET /global/config` answers
  `401 {"error":{"code":"UNAUTHENTICATED","message":"invalid credentials"}}` from the shim's auth
  middleware. 401 is **not** a response the pinned document declares for `global.config.get`; that
  middleware applies to every api-key route, so the served-surface row records `auth: api-key`
  rather than claiming a declared 401.

## Truthfulness and scope

The 200 body is the empty `Config` document because the runtime keeps no opencode global-config
store — Consensus configuration lives in `system_settings` and is served as a different surface by
the shim's own `/config` route. The PATCH response is not persisted, and the handler documents that
it is not: there is no store to persist to, and inventing one would fabricate state the runtime does
not have (the same honesty shape as the sibling `/skill` stub, which answers the declared empty
array). No response is derived from `system_settings`, because those keys are Consensus settings,
not opencode `Config` properties.

## Declared-vs-served result

The served-surface row was added for both operations on the path and the comparator was rerun; the
generated result was reconciled with the artifact's existing stable drift IDs, the practice recorded
in commit `938c630` ("regenerate declared-vs-served data while preserving historical IDs"):

| Count | Before | After |
|---|---|---|
| `served_route_entries` | 106 | 108 |
| `covered_operations` | 78 | 80 |
| `drift_declared_not_served` | 102 | 100 |
| `drift_by_class.NOT-SERVED` | 98 | 96 |

- `GET /global/config` (`global.config.get`) left `drift_declared_not_served` and is present in
  `covered` with served outcome `200`. Its drift id in the committed artifact before this edit was
  `SHIM-DRIFT-084`; the board row and the source comments cite `SHIM-DRIFT-087`, which is the id the
  2026-09-29 baseline report assigned this operation
  (`docs/reports/shim-source-002-baseline-diff-2026-09-29.md:459`; its PATCH sibling is
  `SHIM-DRIFT-088` there, line 460). Artifact ids are positional and shrink as operations leave the
  drift list, so the row's number and the artifact's number for the same operation have diverged.
  The operation, not the number, is what flipped.
- `PATCH /global/config` (`global.config.update`, historical id `SHIM-DRIFT-085`) flipped with it:
  one exact-path handler serves both declared operations, and flipping only GET would have left the
  comparator classifying the declared PATCH as `METHOD-MISSING` ("path is routed and serves other
  methods") — a lie the artifact would then have shipped.
- No other drift row was renumbered. Extraction of the two rows is total: no `/global/config` entry
  remains in `drift_declared_not_served`, and the artifact is content-identical to a fresh comparator
  run modulo positional ids.

## Residual carried forward (not this row's scope)

After this change, `/global/dispose` (ROUTE-ADD-091) and `/global/upgrade` (ROUTE-ADD-092) are
**registered and served in `server.go`** (lines 187 and 193) but still have no served-surface row,
so the artifact continues to classify both as `NOT-SERVED` — ids `SHIM-DRIFT-086` and
`SHIM-DRIFT-087` after this edit. Those two rows own the same criterion-3 work as this one; they are
not touched here to keep this commit scoped to `/global/config`.

One consequence worth naming, because it is a trap for the two rows above: `SHIM-DRIFT-087` now
denotes `POST /global/upgrade` in the artifact, while the source comment at `server.go:179` and the
ROUTE-ADD-089 board row use `SHIM-DRIFT-087` to mean `GET /global/config`. Anyone resolving a row's
`SOURCE ITEM` by number alone will land on the wrong operation. Resolve the operation (method +
path), not the number.

## Commands run

```
go build ./...
go vet ./...
go test ./... -count=1 -short -parallel 4          # all packages ok
go test ./internal/shim/opencode -run 'TestGlobalConfig' -count=1 -v   # 8 tests, 8 pass
bash scripts/check-spec-parity.sh                  # PASS — declared == served (69 paths / 82 ops)
python3 scripts/compare-opencode-declared-served.py \
    specs/openapi/upstream/openapi-1.18.33.surface.json \
    specs/openapi/upstream/consensus-shim-served-surface.yaml /tmp/route089-after
bash /tmp/route089-live-probe.sh                   # PROBE_OK, 12/12 arms
```
