# ROUTE-ADD-026 — GET /api/provider (v2.provider.list) live probe

ch:trace row=ROUTE-ADD-026
  spec=specs/openapi/upstream/openapi-1.18.33.json#v2.provider.list
  test=internal/shim/opencode/api_provider_list_test.go
  doc=specs/openapi/upstream/consensus-shim-served-surface.yaml
  evidence=docs/evidence/ROUTE-ADD-026-live-probe.md
  witness=none:unattended-worker-session

Date: 2026-10-15T09:38:38-05:00
Branch: `wt/ROUTE-ADD-026`, base commit `d5a1b695a1b3b84364a088559b6266bcab14730f`.

**Source item:** the board brief cited `SHIM-DRIFT-022`, which in this artifact is
`GET /api/session/{sessionID}/message` (v2.session.messages). The id the row
actually carried before this change was `SHIM-DRIFT-003` — the artifact's drift
ids are POSITIONAL (the comparator renumbers them on every regeneration, and
`TestNotImplementedTableMatchesDeclaredSurfaceArtifact` matches on the
method+path+operationId identity for exactly that reason). The operation, the
path and the declared response set in the brief all name `GET /api/provider` =
`v2.provider.list` (200, 400, 401, 503), so that is the row served here; the
stale id is recorded in the served-surface evidence and this note instead of
being silently accepted.

## Run identity

| Field | Value |
|---|---|
| Binary | `/tmp/consensus-ra026` |
| Binary SHA-256 | `998dd59a6433a57b213b03e8646dc5227e14f80ffc229a62fd4658c0c35b7c20` |
| Listener | `127.0.0.1:52731` (init/serve), `127.0.0.1:52732` (200 arm), `127.0.0.1:52733` (503 arm) |
| Database | scratch SQLite `/tmp/consensus-ra026-live.BXnHFT/consensus.db` |
| Config | explicit `/tmp/consensus-ra026-live.BXnHFT/consensus.yaml` |
| Credential | bootstrap admin key from `consensus init`, kept in the scratch directory (`chmod 600`); raw value not recorded |
| Server log | `/tmp/consensus-ra026-live.BXnHFT/serve{,2,3}.log` |

The binary was built from this worktree with:

```text
go build -o /tmp/consensus-ra026 ./cmd/consensus
```

The scratch database was initialized with `consensus init` and the same binary
was then started with `consensus serve --adapter opencode`. Each listener was
verified free before start (`ss -tln`) and closed after shutdown.

## Contract exercised

The pinned document declares `GET /api/provider` as `v2.provider.list`:
one optional deepObject `location` selector (`location[directory]`,
`location[workspace]`, `additionalProperties: false`) and responses 200
`{location: LocationInfo, data: ProviderV2Info[]}` ("Retrieve active AI providers
so clients can show provider availability and configuration."), 400
`InvalidRequestError`, 401 `UnauthorizedError`, 503 `ServiceUnavailableError`.
`ProviderV2Info` requires `id`, `name`, `api` (`anyOf` `ProviderAISDK
{type: "aisdk", package}` | `ProviderNative {type: "native", settings}`) and
`request` (`{headers, body}`).

## RED proof

Arm 1 — the route is not registered (handler file present, `server.go` at base):
the whole battery failed, naming the pre-fix production signature.

```text
$ go test ./internal/shim/opencode/ -run 'ApiProviderList' -count=1
api_provider_list_test.go:118: GET /api/provider: got 404, want 200. Body: 404 page not found
api_provider_list_test.go:198: GET /api/provider with a seeded catalog: got 404, want 200. Body: 404 page not found
api_provider_list_test.go:227: GET /api/provider with an unreadable catalog: got 404, want the declared 503. Body: 404 page not found
api_provider_list_test.go:273: GET /api/provider?location%5Bbogus%5D=x: got 404, want the declared 400. Body: 404 page not found
api_provider_list_test.go:300: GET /api/provider with location[directory]: got 404, want 200. Body: 404 page not found
api_provider_list_test.go:320: POST /api/provider: got 404, want 405. Body: 404 page not found
api_provider_list_test.go:351: response is missing required member "_tag" (got [error])
api_provider_list_test.go:351: response carries undeclared member "error" — the pinned schema sets additionalProperties:false (got [error])
api_provider_list_test.go:353: _tag = <nil>, want UnauthorizedError (the declared 401 body)
api_provider_list_test.go:373: /api/provider is not registered in MountPatterns — a parent chi mount would 404 the route (ROUTE-ADD-026)
api_provider_list_test.go:385: chi-mounted GET /api/provider: got 404, want 200 (the mount must reach the shim route). Body: 404 page not found
FAIL github.com/wojons/consensus/internal/shim/opencode 0.566s
```

Full log: `/tmp/ra026/red/arm1.log`. The unauthenticated arm additionally shows
the shim's v1 `{"error": …}` UNAUTHENTICATED body (no v2 envelope) before the
exact-path `v2EnvelopeRoutePaths` entry existed.

Arm 2 — a targeted mutation that disables the 503 decision
(`if false && err != nil` in the handler) while everything else stays fixed:

```text
$ go test ./internal/shim/opencode/ -run 'ApiProviderListUnreadableCatalogAnswersDeclared503' -count=1
api_provider_list_test.go:227: GET /api/provider with an unreadable catalog: got 200, want the declared 503.
  Body: {"data":[],"location":{…}}
FAIL github.com/wojons/consensus/internal/shim/opencode 0.006s
```

That is exactly the silent lie the declared 503 exists to prevent, and it is what
the catalog read closes. The mutated file was restored byte-for-byte
(`diff` against the pre-mutation copy: identical).

## Live results

```text
AUTHENTICATED_200=200
VALID_LOCATION_200=200
MALFORMED_SELECTOR_400=400
BLANK_WORKSPACE_400=400
WORKSPACE_NO_PREFIX_400=400
WRONG_METHOD_POST_405=405
UNAUTHENTICATED_401=401
INVALID_CREDENTIAL_401=401
SIBLING_/api/provider/anthropic=404
V1_COMPAT_/provider=200
SIBLING_/api/model=200
UNREADABLE_CATALOG_503=503
SERVER_STOPPED=1
LISTENER_CLOSED=1
```

The authenticated success body (Content-Type `application/json`):

```json
{"data":[],"location":{"directory":"/tmp/consensus-ra026-live.BXnHFT","project":{"directory":"/tmp/consensus-ra026-live.BXnHFT","id":"consensus-734785a0302e"}}}
```

`location` is the real selected workspace (`requestWorkspaceDir`), and
`location[directory]` overrides it (the second 200 above selected the same
scratch directory explicitly).

The malformed-selector answers used the declared 400 envelope:

```json
{"_tag":"InvalidRequestError","field":"location[bogus]","kind":"Query","message":"unknown location field \"location[bogus]\""}
{"_tag":"InvalidRequestError","field":"location[workspace]","kind":"Query","message":"query parameter \"location[workspace]\" must not be blank"}
{"_tag":"InvalidRequestError","field":"location[workspace]","kind":"Query","message":"location[workspace] must start with wrk"}
```

The unauthenticated and invalid-credential answers used the declared v2 envelope,
not the shim's v1 body:

```json
{"_tag":"UnauthorizedError","message":"invalid credentials"}
```

The wrong-method answer kept the sibling shim convention:

```json
{"error":{"code":"METHOD_NOT_ALLOWED","message":"use GET"}}
```

The declared 503 arm was proven live on a third serve of the same scratch
instance after `model_registry` was dropped from the scratch database
(`/tmp/ra026/drop_model_registry.py` refuses any path outside
`/tmp/consensus-ra026-live.*`):

```json
{"_tag":"ServiceUnavailableError","message":"provider registry unavailable: sqlite: query: SQL logic error: no such table: model_registry (1)","service":"model_registry"}
```

With that same broken catalog, a malformed selector still answered 400 — the
selector is validated before the store read, so a bad request is never masked by
an unavailable catalog.

Sibling routes are untouched: `/api/provider/anthropic` keeps the net/http
default 404 (`404 page not found`, no catch-all), the v1 compatibility route
`/provider` still answers its own body
(`{"maxTokens":8191,"model":"text-embedding-3-small","provider":"consensus","version":"0.1.0"}`),
and `/api/model` (the closest sibling with the same declared response set)
answers exactly as before.

## Truthfulness

The runtime keeps no provider registry with protocol descriptors. Its provider
evidence is `model_registry.provider_id` (migration 020, the models.dev column
the routing catalog carries) and the stored credential rows `PUT
/auth/{providerID}` writes into `system_settings`. A `ProviderV2Info` needs an
`api` descriptor and a `request` configuration, and neither store holds one: this
Go runtime reaches every provider through its own OpenAI-compatible or Anthropic
HTTP client (`internal/llm`) and names no AI-SDK package or native settings
document, while the outbound provider configuration lives in the harness LLM
config whose headers are exactly where the provider credential goes — it is
never copied into a response body.

The handler therefore reports only what it can source: the real selected
location, and the declared empty (never null) `data` array. It does not invent an
id list, a display name, an `api` descriptor, a `request` configuration or a
`disabled` flag. The catalog is still READ — not probed with a dummy query — and
that read decides the two declared arms (readable → 200, unreadable → the
declared 503), the `handleApiModelList` precedent for `v2.model.list`, whose
declared response set this operation matches exactly.

## Declared-vs-served result

The comparator was run from the repository root:

```text
python3 scripts/compare-opencode-declared-served.py \
  specs/openapi/upstream/openapi-1.18.33.surface.json \
  specs/openapi/upstream/consensus-shim-served-surface.yaml \
  /tmp/ra026/out
```

The generated `declared-vs-served.json` was copied byte-for-byte to
`specs/openapi/upstream/opencode-declared-vs-served-1.18.33.json` (a second run
into another directory reproduces the committed file byte-identically). The
stable identity `GET /api/provider` + `v2.provider.list` moved from
`drift_declared_not_served` to `covered` with `served_outcome: 200` and
declared responses `200,400,401,503`; **no other operation changed class in
either direction** (set-diff of the two generated artifacts).

```text
counts before this change (the committed artifact):  covered_operations 102  drift_declared_not_served 67  NOT-SERVED 63
same tree, comparator re-run (pre-change yaml):      covered_operations 103  drift_declared_not_served 66  NOT-SERVED 62
this change (regenerated artifact):                  covered_operations 104  drift_declared_not_served 65  NOT-SERVED 61
narrowed_error_contract_only 16 (narrowed_unexplained 0) | intentional_narrowing 1 | semantic_different 2 | shim_extensions 17 | drift_served_not_declared 0
```

The regeneration also cleared a staleness the committed artifact carried before
this change: `POST /api/integration/{integrationID}/connect/key` was listed
BOTH in `covered` and in `drift_declared_not_served` (id `SHIM-DRIFT-001`), and
`counts.covered_operations` (102) was one behind its own `covered` list (103
rows) — ROUTE-ADD-019 added the served-surface row while the committed artifact
kept the old NOT-SERVED row. The regenerated artifact is self-consistent again
(counts equal their lists) and the row appears once, in `covered`.

## Verification

- Focused RED arm 1 (registration absent): PASS — 404 default answers, missing `MountPatterns` entry, v1 401 body.
- Focused RED arm 2 (503 decision mutated off): PASS — unreadable catalog answered 200; file restored byte-identically.
- Focused GREEN: `go test ./internal/shim/opencode/ -run 'ApiProviderList' -count=1 -v` — PASS (8 tests, 5 subtests).
- Whole package: `go test ./internal/shim/opencode/ -count=1` — PASS (`14.873s`).
- `go build ./...` — PASS. `go vet ./...` — PASS (rc 0).
- `gofmt -l` on the two new Go files and `server.go` — empty.
- Scratch init + serve + curl probe: PASS for the 200, 400 (3 arms), 401 (2 arms), 405 and 503 arms; sibling 404/200 routes unchanged; listener closed after shutdown.
