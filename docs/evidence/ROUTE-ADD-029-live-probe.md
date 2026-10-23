# ROUTE-ADD-029 — POST /api/pty (v2.pty.create) live probe

TIER: T3 (the commands below were executed against a scratch instance; the
outputs are quoted, not paraphrased).

| Field | Value |
|---|---|
| Binary | `/tmp/consensus-ra029` (pre-change binary rebuilt in place after the change) |
| Post-change binary SHA-256 | `0c3fc82704f7da3f68b6d50a49434a51af4526732d2b1c724de5c149bbc3de15` |
| Toolchain | `go1.26.5 linux/amd64` |
| Listener | `127.0.0.1:52877` (verified free with `ss -tln` before start, closed after) |
| Database | scratch SQLite `/tmp/consensus-ra029-live/consensus.db` |
| Config | `/tmp/consensus-ra029-live/consensus.yaml` via `CONSENSUS_CONFIG` (the `--config` flag is still silently ignored — the 2026-10-20 dogfood landmine), `llm.provider: mock` + `CONSENSUS_MOCK_LLM=1` so no LLM key is needed |
| Credential | bootstrap admin key from `consensus init`, kept `chmod 600` at `/tmp/consensus-ra029-live/admin.key`; the raw value is not recorded here or in any committed file |
| Logs | `/tmp/consensus-ra029-live/{init,serve,serve-after}.log`, batteries `before.log` / `after.log` |
| Probe time | 2026-10-23T02:18Z (before) / 2026-10-23T02:21Z (after) |

Started with:

```bash
CONSENSUS_CONFIG=/tmp/consensus-ra029-live/consensus.yaml \
CONSENSUS_MOCK_LLM=1 /tmp/consensus-ra029 serve
```

`grep -c "opencode shim enabled" serve.log` → `1`, `/api/v1/health` → `200`.

## Contract exercised

The pinned document (`specs/openapi/upstream/openapi-1.18.33.json`,
`paths./api/pty.post`) declares `v2.pty.create`:

- one optional deepObject `location` selector (`location[directory]`,
  `location[workspace]`, `additionalProperties: false`);
- a **required** `application/json` requestBody: an object with
  `additionalProperties: false` and five optional members — `command`
  (string), `args` (string[]), `cwd` (string), `title` (string), `env`
  (object of string). No member is required, so `{}` is a well-formed body;
  the body itself is required (`"required": true` on `requestBody`);
- responses `200 {location: LocationInfo, data: Pty}`, `400
  InvalidRequestError`, `401 UnauthorizedError`.

## What was WRONG before this change (the gap this row actually had)

The board row's premise ("no shim route at all; net/http default 404") was
**stale**: ROUTE-ADD-028 (merged 2026-10-19) registered the exact `/api/pty`
path, so POST already reached `handleApiPty`. The real defect was the DECLARED
400 ARM: every POST body — absent, unparseable, non-object, undeclared member,
wrong-typed member — was answered by the typed 501 narrowing, so the shim never
told a client its input was wrong and the declared `400 InvalidRequestError`
was unreachable.

BEFORE (scratch instance, pre-change binary):

```text
== POST arms ==
POST_unauth_no_body                401  {"_tag":"UnauthorizedError","message":"invalid credentials"}
POST_unauth_bad_body               401  {"_tag":"UnauthorizedError","message":"invalid credentials"}
POST_auth_absent_body              501  {"error":"not_implemented","operation":"v2.pty.create",...}
POST_auth_empty_object             501  {"error":"not_implemented","operation":"v2.pty.create",...}
POST_auth_valid_body               501  {"error":"not_implemented","operation":"v2.pty.create",...}
POST_auth_unknown_field            501  {"error":"not_implemented","operation":"v2.pty.create",...}
POST_auth_wrong_type               501  {"error":"not_implemented","operation":"v2.pty.create",...}
POST_auth_bad_json                 501  {"error":"not_implemented","operation":"v2.pty.create",...}
POST_auth_non_object               501  {"error":"not_implemented","operation":"v2.pty.create",...}
POST_auth_null                     501  {"error":"not_implemented","operation":"v2.pty.create",...}
POST_auth_two_values               501  {"error":"not_implemented","operation":"v2.pty.create",...}
POST_auth_bad_selector             501  {"error":"not_implemented","operation":"v2.pty.create",...}
```

## AFTER — the declared 401 and 400 arms are served

```text
== GET arms ==
GET_unauth                         401  {"_tag":"UnauthorizedError","message":"invalid credentials"}
GET_auth                           200  {"data":[],"location":{"directory":"/home/kara/consensus","project":{...}}}
== POST arms ==
POST_unauth_no_body                401  {"_tag":"UnauthorizedError","message":"invalid credentials"}
POST_unauth_bad_body               401  {"_tag":"UnauthorizedError","message":"invalid credentials"}
POST_auth_absent_body              400  {"_tag":"InvalidRequestError","kind":"Body","message":"v2.pty.create declares a required application/json request body; send a JSON object (an empty object is valid)"}
POST_auth_empty_object             501  {"error":"not_implemented","operation":"v2.pty.create","detail":"..."}
POST_auth_valid_body               501  {"error":"not_implemented","operation":"v2.pty.create","detail":"..."}
POST_auth_unknown_field            400  {"_tag":"InvalidRequestError","field":"bogus","kind":"Payload","message":"field \"bogus\" is not declared by v2.pty.create (additionalProperties: false)"}
POST_auth_wrong_type               400  {"_tag":"InvalidRequestError","field":"command","kind":"Payload","message":"field \"command\" must be a string"}
POST_auth_bad_json                 400  {"_tag":"InvalidRequestError","kind":"Body","message":"malformed request body: unexpected EOF"}
POST_auth_non_object               400  {"_tag":"InvalidRequestError","kind":"Body","message":"request body must be a JSON object"}
POST_auth_null                     400  {"_tag":"InvalidRequestError","kind":"Body","message":"request body must be a JSON object"}
POST_auth_two_values               400  {"_tag":"InvalidRequestError","kind":"Body","message":"request body must contain exactly one JSON object"}
POST_auth_bad_selector             400  {"_tag":"InvalidRequestError","field":"location[bogus]","kind":"Query","message":"unknown location field \"location[bogus]\""}
== controls ==
PUT_auth_405                       405  {"error":{"code":"METHOD_NOT_ALLOWED","message":"use GET or POST"}}
DEEPER_404                         404  404 page not found
V1_GET_pty_200                     200  []
V1_POST_pty_501                    501  {"error":"not_implemented","operation":"pty.create","detail":"..."}
```

Reproduce: `bash /tmp/consensus-ra029-live/battery.sh` (reads the admin key
from the `chmod 600` file; the key is never echoed).

## What is deliberately NOT served — the declared 200

`POST /api/pty` still answers the typed 501 `not_implemented` for a
**well-formed** request. The declared `200 {location, data: Pty}` requires a
real `Pty` (`id` `^pty`, `title`, `command`, `args`, `cwd`, `status`, `pid`),
and this runtime keeps no pty registry and spawns no pseudo-terminals — the
same truth the sibling `GET /api/pty` records when it answers the empty,
non-null `data` array, SPEC-017 §3.9 records for the `pty.create` family, and
the served-surface artifact records as `intentional_narrowing`
(SHIM-NARROWED-U001). Emitting that object would fabricate a spawned terminal.

Consequence for the row's stated pass criteria: `401` and `400` are now served
live; `200` is NOT and must not be made reachable from this handler — it needs
a real PTY registry (owner/spec decision, not a route-wiring change). See the
commit body / worker report for the conflict record.

## Verification runs (this tree, post-change)

```text
$ go build ./...                                    # exit 0
$ go vet ./internal/shim/opencode/                  # exit 0
$ go test ./internal/shim/opencode/... -count=1     # ok 17.553s
$ go test ./internal/shim/opencode/ -run 'ApiPtyCreate' -count=1 -v
  --- PASS: TestApiPtyCreateDeclared401Unauthorized (3 subtests)
  --- PASS: TestApiPtyCreateDeclared400MalformedBody (19 subtests)
  --- PASS: TestApiPtyCreateDeclared400AbsentBody
  --- PASS: TestApiPtyCreateDeclared400MalformedSelector (5 subtests)
  --- PASS: TestApiPtyCreateWellFormedAnswersTyped501AndNeverFabricatesPty (3 subtests)
  --- PASS: TestApiPtyCreateMethodGuard
  --- PASS: TestApiPtyCreateV1RouteUntouched
  --- PASS: TestApiPtyCreateTraceRow
$ go test ./internal/shim/opencode/ -run \
    'TestDeclaredUnimplementedRoutesAnswerTypedEnvelope|TestNotImplementedEnvelopeIsNotBlanket501' -count=1
  --- PASS (SHIM-DRIFT-106 POST /pty, SHIM-DRIFT-025 POST /api/pty)
```

## Declared-vs-served regeneration

```text
$ python3 scripts/compare-opencode-declared-served.py \
    specs/openapi/upstream/openapi-1.18.33.surface.json \
    specs/openapi/upstream/consensus-shim-served-surface.yaml /tmp/ra029-cmp2
rc=0
$ grep -c '| /api/pty | POST' /tmp/ra029-cmp2/appendix-drift-declared.md
0
```

`POST /api/pty` is classified `intentional_narrowing` (SHIM-NARROWED-U001,
served outcome `501-typed`), **never** `NOT-SERVED` / `drift_declared_served`:
the route is registered and answers the declared 401/400 arms plus the typed
narrowing. The served-surface row's `narrowing-rationale` and `evidence` were
extended to record the newly served 400 arm.
