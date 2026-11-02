# ROUTE-ADD-055 — GET /api/session/{sessionID}/question (v2.session.question.list) live probe

ch:trace row=ROUTE-ADD-055
  spec=specs/024-shim-support-matrix.md
  test=internal/shim/opencode/api_session_question_test.go
  doc=specs/openapi/upstream/consensus-shim-served-surface.yaml
  evidence=docs/evidence/ROUTE-ADD-055-live-probe.md
  witness=none:unattended-foreman-tick

Date: 2026-11-02T15:42:36Z (probe run; tick consensus-foreman-2026-11-02-15-09-35)
Base commit: `f95960d80aadf3e78910371604e13f464dd60ab9` (`origin/main`, the
integrated tree this probe exercises; the implementation landed in `a856e13`
`fix(shim): serve session question route`, merged as `d47086b1`. This evidence
document is added in the follow-up commit).

**Source item:** the board row cites `SHIM-DRIFT-051`. The committed artifact's
drift ids are POSITIONAL — the comparator renumbers them on every regeneration —
so this closeout matches on `operationId v2.session.question.list` exactly as the
row's acceptance directs ("Match on operationId ... never match drift ids"). In
the current artifact (`specs/openapi/upstream/opencode-declared-vs-served-1.18.33.json`)
the operation is recorded in `covered` with `served_outcome "200"` and zero
`drift_declared_not_served` entries for the route.

## Run identity

| Field | Value |
|---|---|
| Binary | `/tmp/consensus-route055` (built from base commit, `go build ./cmd/consensus`) |
| Binary SHA-256 | `4b0b4d93c8a94f5d33867c2bebe7fe592d0d6607d16b6620f6abd802882e31b6` |
| Listener | `127.0.0.1:18095` |
| Backend | scratch SQLite (`/tmp/route055-scratch-live/scratch.db`), migrations applied by `serve` |
| LLM | `provider: mock` (`CONSENSUS_MOCK_LLM=1`) — no provider credentials involved |
| Auth | one `api_keys` row (scope `admin`) seeded with throwaway probe token `route055-live-probe-token` |
| Sessions seed | one `sessions` row `ses_question_055` (agent_name `probe`, model_id `mock-model`) |

## Acceptance commands (row acceptance field)

```
$ python3 scripts/compare-opencode-declared-served.py specs/openapi/upstream/openapi-1.18.33.surface.json specs/openapi/upstream/consensus-shim-served-surface.yaml /tmp/parity-055
# exit 0; counts: covered_operations 141, drift_declared_not_served 19, narrowed_unexplained 0

$ jq -c '.drift_declared_not_served[]? | select(.method=="GET" and .path=="/api/session/{sessionID}/question")' /tmp/parity-055/declared-vs-served.json
# prints nothing (0 hits)

$ jq -c '.covered[]? | select(.method=="GET" and .path=="/api/session/{sessionID}/question")' /tmp/parity-055/declared-vs-served.json
{"path":"/api/session/{sessionID}/question","method":"GET","operationId":"v2.session.question.list","tags":"session questions","declared_responses":"200,400,401,404","family":"/api/*  (v2 surface)","served_outcome":"200","evidence":"api_session_question.go handleApiSessionQuestionList (ROUTE-ADD-055; operationId v2.session.question.list): resolves the real sessions store, answers the declared SessionNotFoundError 404 for a readable unknown session and InvalidRequestError 400 for blank or unreadable session input, then reads pending approval_requests rows scoped to the session. Consensus has no QuestionV2Request producer — native approval rows do not have the required ^que identity or question/header/options fields — so the truthful 200 is {data: []}, never fabricated question entries. Unauthenticated requests use the v2 UnauthorizedError envelope through isApiSessionQuestionPath."}
# exactly one hit, served_outcome "200"
```

## Focused tests (base commit)

```
$ go test -short -count=1 -run 'TestApiSessionQuestion' -v ./internal/shim/opencode/
--- PASS: TestApiSessionQuestionRejectReplyAuthMountAndExactShape
--- PASS: TestApiSessionQuestionReplyRejectUndeclaredMethods
--- PASS: TestApiSessionQuestionListDeclaredArms (blank_session / unknown_session / store_error / known_session)
--- PASS: TestApiSessionQuestionListReadsSessionApprovalStore
--- PASS: TestApiSessionQuestionListAuthMountAndExactShape
--- PASS: TestApiSessionQuestionListChiMountAndMethod
PASS    github.com/wojons/consensus/internal/shim/opencode  0.017s

$ go build ./...        # exit 0
$ go vet ./internal/shim/opencode/   # exit 0
```

## Probe transcript

```
## unauth GET /api/session/ses_question_055/question
{"_tag":"UnauthorizedError","message":"invalid credentials"}
HTTP 401
## auth GET happy path (known session)
{"data":[]}
HTTP 200
## auth GET blank sessionID (%20)
{"_tag":"InvalidRequestError","field":"sessionID","kind":"Path","message":"sessionID path parameter must not be blank"}
HTTP 400
## auth GET unknown session
{"_tag":"SessionNotFoundError","message":"no session \"ses_does_not_exist\" in this runtime: no question requests to list","sessionID":"ses_does_not_exist"}
HTTP 404
## control auth POST (undeclared method)
{"error":{"code":"METHOD_NOT_ALLOWED","message":"use GET"}}
HTTP 405
## control auth GET deeper subpath /question/x
404 page not found
HTTP 404
```

## What each arm proves

- **200** answers the declared response exactly: `{"data":[]}` — the declared
  non-null empty data array after reading the session-scoped pending
  `approval_requests` store; no QuestionV2Request entry is fabricated (the
  handler doc comment and the served-surface evidence explain why native
  approval rows cannot be reshaped into `^que` question entries).
- **401** answers the declared `UnauthorizedError` envelope
  (`{_tag, message}`) — the exact-path `isApiSessionQuestionPath` v2 envelope
  entry is in effect, not the shim's v1 `UNAUTHENTICATED` body.
- **400 InvalidRequestError** answers a present-but-blank `sessionID` path
  parameter and names it (`field: sessionID, kind: Path`) — a declared input
  violation, not a 500.
- **404 SessionNotFoundError** answers an unknown session id with the typed
  declared error, so absence is claimed only from a readable store.
- **405 control** for POST: undeclared method keeps the sibling
  METHOD_NOT_ALLOWED envelope (declared responses are 200/400/401/404 only).
- **404 control** for `/question/x`: the registration is exact-path — deeper
  `/api/session/{sessionID}/question/*` sub-paths keep the net/http default
  404, no catch-all was introduced.

The store-error 400 arm (unreadable `sessions`/`approval_requests` store) is
covered by `TestApiSessionQuestionListDeclaredArms/store_error`; a live
reproduction would require corrupting the scratch DB mid-serve and adds no
contract evidence beyond the unit arm.

## Reproduce

```bash
cd <repo>   # at f95960d80aadf3e78910371604e13f464dd60ab9
go build -o /tmp/consensus-route055 ./cmd/consensus
mkdir -p /tmp/route055-scratch-live
cat > /tmp/route055-scratch-live/consensus.yaml <<'YAML'
llm:
  provider: mock
  default_model: mock-model
  max_context_tokens: 32000
  max_output_tokens: 4096
YAML
CONSENSUS_MOCK_LLM=1 /tmp/consensus-route055 serve \
  --config /tmp/route055-scratch-live/consensus.yaml \
  --db-url 'sqlite:///tmp/route055-scratch-live/scratch.db' \
  --port 18095 --hostname 127.0.0.1 --log-level error &

TOK='route055-live-probe-token'
PREFIX=$(printf '%s' "$TOK" | cut -c1-8)
HASH=$(printf '%s' "$TOK" | sha256sum | cut -d' ' -f1)
sqlite3 /tmp/route055-scratch-live/scratch.db \
  "INSERT INTO api_keys (key_hash, key_prefix, scope) VALUES ('$HASH', '$PREFIX', 'admin');"
sqlite3 /tmp/route055-scratch-live/scratch.db \
  "INSERT INTO sessions (id, agent_name, model_id) VALUES ('ses_question_055', 'probe', 'mock-model');"

curl -s -w '\nHTTP %{http_code}\n' -H "Authorization: Bearer $TOK" \
  http://127.0.0.1:18095/api/session/ses_question_055/question
```

Note: `serve` on a fresh scratch DB prints a bootstrap `first_admin_key`; the
probe authenticates with the separately seeded throwaway token above (the shim's
auth wall reads the `api_keys` table; it does not accept a bare `x-api-key`
header).
