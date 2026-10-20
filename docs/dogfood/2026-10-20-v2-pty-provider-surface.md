# Dogfood 2026-10-20 — the two new v2 surfaces (/api/pty, /api/provider/{id}) + PUT /auth landmine

Tick: `consensus-dogfood-2026-10-20-06-54-47` · HEAD driven: `facf424` (build 0.9s warm,
go1.26.5) · scratch: `/tmp/consensus-df-1020` sqlite, port 18234, config via
`CONSENSUS_CONFIG` env (the DF-CONSENSUS-42 landmine — `--config` is still silently ignored).

## What this run drove

Two user-facing surfaces landed AFTER the 2026-10-16 dogfood run (both merged 2026-10-19,
634517b / a745ffb), so this was their first real-consumer drive:

1. **GET /api/pty** (v2.pty.list, ROUTE-ADD-028, source SHIM-DRIFT-024) — plus its narrowed
   sibling POST (v2.pty.create).
2. **GET /api/provider/{providerID}** (v2.provider.get, ROUTE-ADD-027) — driven as a real
   cross-endpoint workflow: PUT /auth/{id} → GET /provider/auth → GET /api/provider/{id}.

Also re-proven live: the user-visible half of DF-CONSENSUS-58 (dabab7a startup WARN).

## /api/pty — conformant on every arm

| Arm | Result |
|---|---|
| no auth | 401 `{"_tag":"UnauthorizedError"}` |
| GET (valid location) | 200 `{"data":[],"location":{"directory":…,"project":{"id":"consensus-f73c63a4b8cb"}}}` — truthful empty registry (this runtime spawns no ptys), real LocationInfo |
| `?location[directory]=` blank | 400 `InvalidRequestError` field=`location[directory]` kind=Query |
| `?location[directory]=` nonexistent dir | 400 with the underlying `stat` detail |
| `?location[bogus]=x` unknown field | 400 `unknown location field` |
| POST (declared sibling) | 501 typed `not_implemented` naming **v2.pty.create** with a plain-language detail (intentional narrowing per SHIM-GAP-002) |
| DELETE | 405 `METHOD_NOT_ALLOWED` "use GET or POST" |
| GET `/api/pty/pty_abc123` | net/http default 404 (deep family keeps its own rows, no catch-all) |
| v1 `GET /pty` control | 200 `[]` — untouched |

Contract: `specs/openapi/upstream/consensus-shim-served-surface.yaml` lines 475-486.

## GET /api/provider/{providerID} — conformant, but it amplifies a v1 hole

Direct arms: 200 for a provider with real credential rows, 404 `ProviderNotFoundError`
(truthful — fresh instance knows nothing), 400 blank-id 401 unauth, 405 non-GET, default 404
on deeper paths. All correct.

**The workflow broke one step upstream.** `PUT /auth/{providerID}` (`server.go:2238
handleAuthPut`) decodes the body into `map[string]any` and persists **every key verbatim**
as `auth.<provider>.<field>` rows — no validation against the declared `Auth` anyOf
(`ApiAuth` requires `[type,key]`, `additionalProperties:false`):

```
PUT /auth/deepseek  {"bogus":true}          -> 200 "auth configuration saved"
sqlite: auth.deepseek.bogus|true             <- junk stored as a credential field
PUT /auth/zz-fake-provider {"junk":1}        -> 200 "saved"
GET /api/provider/zz-fake-provider           -> 200 (phantom provider EXISTS)
CONTROL: GET /api/provider/never-touched     -> 404 (proves the PUT is the root)
PUT /auth/x {"type":"api"} (missing key)     -> 200 (required-field check missing too)
DELETE /auth/zz-fake-provider                -> 200 true, rows gone, phantom 404 again
```

Filed as **DF-CONSENSUS-59 (P1)**: silent credential-state poisoning, amplified into false
provider existence on a surface that landed one day before this drive.

## DF-CONSENSUS-58 partial verification

- WARN leg **re-proven**: keyless boot logs `level=WARN msg="consensus: No LLM API key
  configured — agent harness will fail on LLM calls (set DEEPSEEK_API_KEY or CONSENSUS_API_KEY)"`
  as the first boot line (dabab7a fix verified live).
- :8095 premise **independently confirmed**: this instance's pprof listener failed with
  `bind: address already in use` against a foreign process — any second instance collides.
- Restart-trap leg not re-driven (needs keyed-boot → keyless-restart → iteration burn; left
  to the row's owner). Evidence: board event 659.

## Performance

`GET /api/pty` warm p50 **0.53 ms** (min 0.45 / max 0.61, n=20); `GET /api/provider/{id}`
p50 **0.64 ms**; `/api/v1/health` p50 0.63 ms; cold-first-request 0.56 ms; hyperfine mean
6.1 ms is curl-spawn dominated (user 2.3 + sys 3.4 ms of the 6.1). Init 1.29 s, serve→first
200 inside the first 100 ms poll window. Nothing slow enough to profile — no PERF row (a
win nobody can feel is not a finding).

## Landmines for the next user

- `--config` flag still ignored — use `CONSENSUS_CONFIG=<path>` for init AND serve.
- Never PUT /auth with a body you have not validated yourself; the shim will "save" it and
  the v2 provider surface will then claim the provider exists.
- First admin key prints once at init (`cs_ak_…`); restarts print `created=false` + prefix.
