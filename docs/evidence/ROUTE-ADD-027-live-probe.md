# ROUTE-ADD-027 — GET /api/provider/{providerID} (v2.provider.get) live probe

ch:trace row=ROUTE-ADD-027
  spec=specs/openapi/upstream/openapi-1.18.33.json#v2.provider.get
  test=internal/shim/opencode/api_provider_get_test.go
  doc=specs/openapi/upstream/consensus-shim-served-surface.yaml
  evidence=docs/evidence/ROUTE-ADD-027-live-probe.md
  witness=none:unattended-worker-session

Date: 2026-10-17T01:24:00-05:00
Base commit: `59093f182d674d133b7d0cf189cb441e107e3b7e` (`main`).

**Source item:** the board row cites `SHIM-DRIFT-023`, which in the committed
artifact is `POST /api/session/{sessionID}/permission` (v2.session.permission.create).
The artifact's drift ids are POSITIONAL — the comparator renumbers them on every
regeneration, which is why its own gate
(`TestNotImplementedTableMatchesDeclaredSurfaceArtifact`) keys on the
method+path+operationId identity instead. The id `v2.provider.get` actually
carried before this change was `SHIM-DRIFT-001`. The operation, the path and the
declared response set in the brief all name `GET /api/provider/{providerID}` =
`v2.provider.get` (200, 400, 401, 404, 503), so that is the row served here; the
stale id is recorded here and in the served-surface evidence rather than being
silently accepted.

## Run identity

| Field | Value |
|---|---|
| Binary | `/tmp/consensus-ra027` |
| Binary SHA-256 | `b8ccf5e4e8456d7f565237abc0231c41796216b88deb9c3e5664955325067107` |
| Listener (primary) | `127.0.0.1:52741` (200/400/404/401/405 arms) |
| Listener (503 arms) | `127.0.0.1:52742` (unreadable catalog, then unreadable credential store) |
| Database (primary) | scratch SQLite `/tmp/consensus-ra027-live.d389a495/consensus.db` |
| Database (503 arms) | scratch SQLite `/tmp/consensus-ra027-live-b2/consensus.db` |
| Config | explicit `consensus.yaml` in each scratch directory |
| Credential | bootstrap admin key from `consensus init`, kept in the scratch directory (`chmod 600`); the raw value is not recorded in this document or in any committed file |
| Server logs | `<scratch>/serve.log`, `<scratch>/serve-unreadable-catalog.log`, `<scratch>/serve-unreadable-credentials.log` |

The binary was built from this tree with `go build -o /tmp/consensus-ra027
./cmd/consensus`. Each instance was initialized with `consensus init --config
<scratch>/consensus.yaml`, seeded through `sqlite3` (see "Seeded state"), and
started with `consensus serve --adapter opencode`. Each listener was verified
free before start (`ss -tln`) and closed after shutdown.

## Seeded state

Both provider stores the handler reads were seeded with real rows — nothing in
this probe depends on a mock:

```sql
-- model_registry (migration 020 provider_id column; what `consensus models sync` writes)
deepseek          2 enabled rows  (deepseek-v4-flash, deepseek-v4-pro)
retired-provider  1 row, enabled = 0
-- system_settings (the store PUT /auth/{providerID} writes)
auth.anthropic.key = <probe placeholder value>
```

`retired-provider` exercises the "catalog knows it but no enabled model routes to
it" state; `anthropic` exercises the credential-only state; `deepseek` the normal
enabled state.

## Contract exercised

The pinned document declares `GET /api/provider/{providerID}` as
`v2.provider.get`: one path parameter `providerID` (plain required string, no
pattern), one optional deepObject `location` selector (`location[directory]`,
`location[workspace]`, `additionalProperties: false`), and responses 200
`{location: LocationInfo, data: ProviderV2Info}` ("Retrieve a single AI provider
so clients can inspect its availability and endpoint settings"), 400
`InvalidRequestError`, 401 `UnauthorizedError`, 404 `ProviderNotFoundError`,
503 `ServiceUnavailableError`. `ProviderV2Info` requires `id`, `name`, `api`
(`anyOf` `ProviderAISDK {type: "aisdk", package[, url, settings]}` |
`ProviderNative {type: "native", settings[, url]}`) and `request`
(`{headers, body}`); `disabled` is optional. Every schema sets
`additionalProperties: false`.

## RED proof

Arm 1 — the route is not registered (handler file present, `server.go` at the
pre-change registration state): the whole battery failed, naming the pre-fix
production signature.

```text
$ go test ./internal/shim/opencode/ -run 'ApiProviderGet' -count=1
--- FAIL: TestApiProviderGetServesDeclared200 (0.07s)
    api_provider_get_test.go:169: GET /api/provider/deepseek: got 404, want 200. Body: 404 page not found
--- FAIL: TestApiProviderGetUnknownProviderAnswersDeclared404 (0.07s)
    api_provider_get_test.go:250: 404 body is not JSON: invalid character 'p' after top-level value (404 page not found
--- FAIL: TestApiProviderGetDisabledProviderReportsCatalogState (0.08s)
    api_provider_get_test.go:278: GET /api/provider/retired-provider: got 404, want 200 (the catalog names it). Body: 404 page not found
--- FAIL: TestApiProviderGetCredentialOnlyProvider (0.08s)
    api_provider_get_test.go:303: GET /api/provider/anthropic: got 404, want 200 (stored credentials register the provider). Body: 404 page not found
--- FAIL: TestApiProviderGetUnreadableCatalogAnswersDeclared503 (0.00s)
    api_provider_get_test.go:332: GET /api/provider/deepseek with an unreadable catalog: got 404, want the declared 503. Body: 404 page not found
--- FAIL: TestApiProviderGetUnreadableCredentialStoreAnswersDeclared503 (0.04s)
    api_provider_get_test.go:392: GET /api/provider/anthropic with an unreadable credential store: got 404, want the declared 503. Body: 404 page not found
--- FAIL: TestApiProviderGetDeclared400MalformedInput (0.06s)
    (six subtests) ... got 404, want the declared 400. Body: 404 page not found
--- FAIL: TestApiProviderGetMethodNotAllowed (0.07s)
    api_provider_get_test.go:479: POST /api/provider/deepseek: got 404, want 405. Body: 404 page not found
FAIL	github.com/wojons/consensus/internal/shim/opencode
```

Full log: `/tmp/ra027/red/arm1.log`. The pre-fix answer for every arm is the
net/http default `404 page not found` — the artifact's NOT-SERVED class.

Arm 2 — a targeted mutation that disables the catalog-read decision
(`if false && err != nil` on the `model_registry` read) while everything else
stays fixed:

```text
$ go test ./internal/shim/opencode/ -run 'ApiProviderGetUnreadableCatalogAnswersDeclared503' -count=1
--- FAIL: TestApiProviderGetUnreadableCatalogAnswersDeclared503 (0.00s)
    api_provider_get_test.go:346: service = system_settings, want the unavailable store named (model_registry)
    api_provider_get_test.go:349: message = "provider credential store unavailable: no such table: model_registry",
      want it to say what could not be read
FAIL
```

That is exactly the silent lie the declared 503 exists to prevent: without the
catalog-read decision the handler would answer the credential-store arm for a
catalog failure and name the wrong unavailable component. The mutated file was
restored byte-for-byte (`diff` against the pre-mutation copy: identical).

Full log: `/tmp/ra027/red/arm2.log`.

## Live results

```text
GET_200_deepseek=200
GET_200_credonly=200
GET_200_disabled=200
GET_404_unknown=404
GET_400_blank_id=400
GET_400_bad_selector=400
POST_405=405
GET_401_unauth=401
GET_401_badkey=401
DEEPER_404=404
SIBLING_LIST_200=200
V1_COMPAT_200=200
UNREADABLE_CATALOG_503=503
UNREADABLE_CREDENTIAL_STORE_503=503
```

The authenticated success body (`Content-Type: application/json`):

```json
{"data":{"api":{"settings":{},"type":"native"},"disabled":false,"id":"deepseek","name":"deepseek","request":{"body":{},"headers":{}}},"location":{"directory":"/tmp/consensus-ra027-live.d389a495","project":{"directory":"/tmp/consensus-ra027-live.d389a495","id":"consensus-abfeac3947b2"}}}
```

The credential-only provider (no catalog rows, so the optional `disabled` member
is omitted rather than guessed):

```json
{"data":{"api":{"settings":{},"type":"native"},"id":"anthropic","name":"anthropic","request":{"body":{},"headers":{}}},"location":{…}}
```

The disabled-only provider (the real catalog state):

```json
{"data":{"api":{"settings":{},"type":"native"},"disabled":true,"id":"retired-provider","name":"retired-provider","request":{"body":{},"headers":{}}},"location":{…}}
```

The declared error arms used the declared typed bodies:

```json
{"_tag":"ProviderNotFoundError","message":"Provider not found: bogus","providerID":"bogus"}
{"_tag":"InvalidRequestError","field":"providerID","kind":"Path","message":"providerID must not be blank"}
{"_tag":"InvalidRequestError","field":"location[bogus]","kind":"Query","message":"unknown location field \"location[bogus]\""}
{"_tag":"UnauthorizedError","message":"invalid credentials"}
{"error":{"code":"METHOD_NOT_ALLOWED","message":"use GET"}}
```

The two declared 503 arms were proven on a second scratch instance whose stores
were renamed away (so the copy stays reproducible and no schema is lost):

```json
{"_tag":"ServiceUnavailableError","message":"provider registry unavailable: sqlite: query: SQL logic error: no such table: model_registry (1)","service":"model_registry"}
{"_tag":"ServiceUnavailableError","message":"provider credential store unavailable: sqlite: query: SQL logic error: no such table: system_settings (1)","service":"system_settings"}
```

With the catalog unreadable, a malformed selector still answered the declared
400 — the selector and path parameter are validated before any store is read, so
a bad request is never masked by an unavailable store.

Sibling routes are untouched: `GET /api/provider` (v2.provider.list) still
answers its own declared 200, the v1 compatibility route `GET /provider` still
answers its own body, and `GET /api/provider/a/b` still answers net/http's
default 404 (no catch-all was introduced).

No response body anywhere in the probe contains a credential value.

## Truthfulness

Every member of the served `ProviderV2Info` is sourced, and the members that
have no store are reported as empty rather than invented:

| member | source |
|---|---|
| `id` | the resolved `{providerID}`, matched against a real store row |
| `name` | the provider id itself — the runtime holds no display-name store, so the only name it has is its catalog id (echoed, not invented) |
| `api` | `{"type":"native","settings":{}}` — this Go runtime reaches providers with its own HTTP clients (`internal/llm`), not through an AI-SDK package, so `native` is the true `anyOf` arm; `settings` is empty because no provider settings document exists |
| `request` | `{"headers":{},"body":{}}` — no per-provider outbound request template is configured; the harness LLM config (whose headers are exactly where the credential goes) is never copied into a response body |
| `disabled` | omitted unless the catalog carries rows for the provider; then it is the real catalog state (no enabled `model_registry` row routes to it) |

The provider-existence decision is the conjunction of two real reads
(`model_registry.provider_id`, and the `auth.<providerID>.<field>` rows the
credential store holds via the same `storedProviderAuthMethods` derivation
`GET /provider/auth` advertises), so the shim cannot report a provider it does
not hold and cannot claim absence from an unreadable store — that is what the
declared 503 is for.

## Declared-vs-served result

The comparator was run from the repository root:

```text
python3 scripts/compare-opencode-declared-served.py \
  specs/openapi/upstream/openapi-1.18.33.surface.json \
  specs/openapi/upstream/consensus-shim-served-surface.yaml \
  /tmp/ra027/out
```

The generated `declared-vs-served.json` was copied byte-for-byte to
`specs/openapi/upstream/opencode-declared-vs-served-1.18.33.json`.

```text
before: covered_operations 105, drift_declared_not_served 64 (NOT-SERVED 60)
after:  covered_operations 106, drift_declared_not_served 63 (NOT-SERVED 59)
```

`GET /api/provider/{providerID}` now appears in the artifact's `covered` list with
`served_outcome: 200`; no drift row for `v2.provider.get` remains.
`drift_served_not_declared` stays 0 and `served_route_entries` moves 146 -> 147.

## Gates

```text
go build ./...                                  ok
go vet ./...                                    ok
go test ./internal/shim/opencode/ -count=1      ok (77.687s, includes the artifact consistency gates)
```

## Cleanup

Both scratch servers were stopped and both listeners verified closed
(`ss -tln` → no `:52741`, no `:52742`); the scratch databases, configs and logs
remain under `/tmp/consensus-ra027-live.d389a495` and
`/tmp/consensus-ra027-live-b2` for re-probing.
