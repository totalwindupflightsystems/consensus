# ROUTE-ADD-039 — GET /api/session/active (v2.session.active) live probe

ch:trace row=ROUTE-ADD-039
  spec=specs/openapi/upstream/openapi-1.18.33.surface.json#v2.session.active
  test=internal/shim/opencode/api_session_active_test.go
  doc=specs/openapi/upstream/consensus-shim-served-surface.yaml
  evidence=docs/evidence/ROUTE-ADD-039-live-probe.md
  witness=none:worker-local-until-foreman-verifies

Date: 2026-10-25T14:48:53-05:00
Base commit: worktree `wt/ROUTE-ADD-039` on `a316c6e` (the implementation is
committed by this row; this evidence document rides in the same commit).

**Source item:** the board row cites `SHIM-DRIFT-035`. The committed artifact's
drift ids are POSITIONAL — the comparator renumbers them on every regeneration —
and in the artifact this worker started from `SHIM-DRIFT-035` was not this
operation; `GET /api/session/active` carried positional id `SHIM-DRIFT-001`
(class NOT-SERVED, "no shim route at all; net/http default 404"). The
operation, path and declared response set in the brief all name
`GET /api/session/active` = `v2.session.active`, so that is the row served
here; the id mapping is recorded here and in the served-surface evidence rather
than being silently accepted. The gates
(`declared_vs_served_consistency_test.go`, `declared_vs_served_artifact_test.go`)
key on method + path + operationId, which is the identity this row moves.

## Run identity

| Field | Value |
|---|---|
| Binary | `/tmp/consensus-route039` |
| Binary SHA-256 | `04371a53ba639f3a9d8f4c5d5ccffa9400f439a7a22148b3a3ce2d5f37caf495` |
| Listener | `127.0.0.1:18094` |
| Backend | scratch SQLite (`/tmp/route039-scratch-live/scratch.db`), migrations applied by `serve` |
| LLM | `provider: mock` (`CONSENSUS_MOCK_LLM=1`) — no provider credentials involved |
| Auth | one `api_keys` row (scope `admin`) seeded with a throwaway probe token |

## Probe transcript

```
## unauth GET /api/session/active
{"_tag":"UnauthorizedError","message":"invalid credentials"}
HTTP 401
## auth GET /api/session/active
{"data":{}}
HTTP 200
## auth GET /api/session/active?anything=1 (undeclared query parameter)
{"data":{}}
HTTP 200
## auth POST /api/session/active (undeclared method)
{"error":{"code":"METHOD_NOT_ALLOWED","message":"use GET"}}
HTTP 405
## auth PUT /api/session/active (undeclared method)
{"error":{"code":"METHOD_NOT_ALLOWED","message":"use GET"}}
HTTP 405
## control auth GET /api/session/ses_abc (sibling family stays unregistered)
404 page not found

HTTP 404
## control auth GET /api/session (ROUTE-ADD-037 sibling untouched)
{"cursor":{},"data":[]}
HTTP 200
## control unauth GET /api/session/active (repeat)
HTTP 401
```

## What each arm proves

- **200** answers the declared envelope exactly: the required `data` member as
  an empty but NON-NULL object — no SessionActive entry is fabricated, because
  every `data` key must match the declared `^ses` pattern while
  `sessions.id` is a native UUID, and upstream scopes the result to foreground
  Session drains owned by this OpenCode process, a concept this
  protocol-adapter runtime has no counterpart for (handler doc comment).
- **401** answers the declared `UnauthorizedError` envelope
  (`{_tag, message}`), i.e. the exact-path `v2EnvelopeRoutePaths` entry is in
  effect — not the shim's v1 `UNAUTHENTICATED` body.
- **400 InvalidRequestError** is exercised by
  `TestApiSessionActiveUnreadableStoreAnswersDeclared400` (an unreadable
  sessions store; this operation declares no 5xx arm). The document declares an
  EMPTY parameters list, so no query input can be malformed against the schema
  — the undeclared-parameter probe above shows one is ignored, not refused.
- **405** for POST and PUT, methods the pinned document does not declare on
  this path. GET is the ONLY declared method, so no sibling operation is
  reclassified by the registration (the Off-by-One
  single-method-registration METHOD-MISSING flip does not apply).
- **404 control**: the declared `/api/session/{sessionID}` family is NOT served
  by this exact-path registration (`/api/session/ses_abc` keeps the net/http
  default 404 its own contract rows are filed against), so no `/api/session/*`
  catch-all was introduced; the ROUTE-ADD-037 sibling `GET /api/session` is
  untouched.

## Reproduce

```bash
cd <repo>
go build -o /tmp/consensus-route039 ./cmd/consensus
mkdir -p /tmp/route039-scratch-live
cat > /tmp/route039-scratch-live/consensus.yaml <<'YAML'
llm:
  provider: mock
  default_model: mock-model
  max_context_tokens: 32000
  max_output_tokens: 4096
YAML
CONSENSUS_MOCK_LLM=1 /tmp/consensus-route039 serve \
  --config /tmp/route039-scratch-live/consensus.yaml \
  --db-url 'sqlite:///tmp/route039-scratch-live/scratch.db' \
  --port 18094 --hostname 127.0.0.1 --log-level error &

# seed a throwaway api key (the shim's auth reads the api_keys table)
TOK='route039-live-probe-token'
PREFIX=$(printf '%s' "$TOK" | cut -c1-8)
HASH=$(printf '%s' "$TOK" | sha256sum | cut -d' ' -f1)
sqlite3 /tmp/route039-scratch-live/scratch.db \
  "INSERT INTO api_keys (key_hash, key_prefix, scope) VALUES ('$HASH', '$PREFIX', 'admin');"

curl -s -w '\nHTTP %{http_code}\n' -H "Authorization: Bearer ***" \
  http://127.0.0.1:18094/api/session/active
```

Note for the next probe: the shim's auth wall accepts `Authorization: ***
(api_keys lookup) or Basic — it does NOT read a bare `x-api-key` header, so an
`x-api-key` request answers 401 even with a valid `--api-key` flag.
