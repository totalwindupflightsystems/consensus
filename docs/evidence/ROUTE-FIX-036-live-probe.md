# ROUTE-FIX-036 live probe — DELETE /session/{sessionID}/message/{messageID}/part/{partID}

ch:trace row=ROUTE-FIX-036
  spec=specs/openapi/upstream/openapi-1.18.33.json#part.delete
  wave=consensus-foreman-2026-10-04-06-15-38.json#task-1
  test=internal/shim/opencode/session_p1_routes_test.go (TestSessionPartDeleteTruthfulArms)
  doc=docs/evidence/ROUTE-FIX-036-live-probe.md
  evidence=docs/evidence/ROUTE-FIX-036-live-probe.md
  witness=none:self-verified-in-worktree — scripted curls against a locally built
    binary; every value below is copied verbatim from the probe transcript.
    verdict=/commit= are deliberately absent in-tree: a verdict binds to a commit
    that already exists, so it is recorded by the commit footer / foreman
    verification, never self-referenced by the change it judges.

Date: 2026-10-04 (local, UTC-05) / 2026-10-04T06:24Z
Worktree: /home/kara/consensus-wt-ROUTE-FIX-036 (branch `wt/ROUTE-FIX-036`,
base `c6babe3`); the probe ran with this change applied to the worktree — the
committed diff is identical to the probed tree.

A locally built `consensus` binary was served on a **fresh SQLite database** and
the opencode shim route was driven over HTTP. Every value below is copied from a
live HTTP response or a live `sqlite3` invocation — none is hand-typed.

## 1. Run identity

| property | value |
|---|---|
| binary | `/tmp/consensus-036` (sha256 `8738a71cd2a264afaa99c5e62712061c9166d9744f3a6b2a23b99018c57e0cbd`) |
| build | `go build -o /tmp/consensus-036 ./cmd/consensus`, from the worktree root |
| worktree / branch | `/home/kara/consensus-wt-ROUTE-FIX-036` @ `wt/ROUTE-FIX-036`, base `c6babe310f3e491449d8d7a1bbacf77c30045fc6` |
| server | `127.0.0.1:18886`, backend `sqlite`; `/global/health` = `{"healthy":true,"version":"consensus-0.1.0"}` |
| db | `sqlite:///tmp/consensus-036-run/probe.db`, created by `consensus init` (exit 0, `Migrations: current`) |
| auth | bootstrap admin key (Bearer, `created=true` on the fresh db); captured into a file and used from a curl header — the value is never printed or recorded |

Seeded for the probe (direct `sqlite3`, after init's migrations ran):

- `sessions sesprobe1` — `agent_name=probe`, `model_id=m-probe`, status `idle`
- `memory_events id=4242` (`user_message`, content `hello probe`) in `sesprobe1` (opencode id `msg-4242`)

## 2. Declared contract, live

No request body: the part.delete operation declares no requestBody.
`<n>:r<m>` in the redirect column is `http_code` + `num_redirects` as reported
by curl; the `307:r1` arm is the net/http ServeMux path-cleaning redirect on
the double-slash path (the client followed it; the target then answers the
declared 400 — same behaviour the sibling PATCH probe recorded).

| arm | request | observed |
|---|---|---|
| well-formed request, known message (no part store → truthful 404) | `DELETE /session/sesprobe1/message/msg-4242/part/prt-1` | `404` `{"error":{"code":"NOT_FOUND","message":"part not found in message"}}` |
| unknown session (declared 404) | `DELETE /session/smissing/message/msg-4242/part/prt-1` | `404` `{"error":{"code":"NOT_FOUND","message":"session not found"}}` |
| unknown message (declared 404) | `DELETE /session/sesprobe1/message/msg-9999/part/prt-1` | `404` `{"error":{"code":"NOT_FOUND","message":"message not found in session"}}` |
| ill-shaped messageID (declared 400) | `DELETE /session/sesprobe1/message/not-a-msg/part/prt-1` | `400` `{"error":{"code":"INVALID_REQUEST","message":"messageID must match the declared pattern ^msg"}}` |
| ill-shaped partID (declared 400) | `DELETE /session/sesprobe1/message/msg-4242/part/not-a-prt` | `400` `{"error":{"code":"INVALID_REQUEST","message":"partID must match the declared pattern ^prt"}}` |
| absent messageID (double slash; ServeMux 307 → cleaned path answers the declared 400) | `DELETE /session/sesprobe1/message//part/prt-1` (`-L`) | `307` → `Location: /session/sesprobe1/message/part/prt-1` → `400` `{"error":{"code":"INVALID_REQUEST","message":"messageID must match the declared pattern ^msg"}}` |
| absent partID (trailing slash reaches the handler) | `DELETE /session/sesprobe1/message/msg-4242/part/` | `400` `{"error":{"code":"INVALID_REQUEST","message":"required path parameter \"partID\" is missing"}}` |

Neighbours and auth (must be unchanged by this route):

| probe | observed |
|---|---|
| `GET /session/sesprobe1/message/msg-4242` after all probes | `200` — content `hello probe` unchanged |
| `DELETE …/part/prt-1` with **no** Authorization header | `401` `{"error":{"code":"UNAUTHENTICATED","message":"invalid credentials"}}` (pre-existing auth gate) |

## 3. Which declared codes are reachable

`part.delete` declares `200`, `400` (BadRequest | InvalidRequestError) and
`404` (NotFoundError).

- `400` — reachable live: malformed messageID / partID and the missing-partID
  arm all answer the declared envelope (section 2).
- `404` — reachable live: unknown session, unknown message, and every
  well-formed partID (the truthful no-part-store answer).
- `200` — NOT reachable, truthfully: the Consensus runtime keeps no part
  store and no part-editing engine. Parts are synthesized on read from
  `memory_events.content` and carry no id, `memory_events` is append-only
  (SPEC-002 §2.1, enforced by the triggers in migrations 017/018), and no
  `prt-...` id is ever issued by this runtime — so there is no part to delete
  and no truthful 200 exists to serve (mirrors the accepted ROUTE-FIX-037
  pattern; the sessionUnshare precedent, ROUTE-FIX-015). The declared 404 is
  the operation's own "resource absent" answer inside the declared vocabulary.

## 4. Verbatim probe transcript

```
$ curl -s -H "Authorization: Bearer $K" http://127.0.0.1:18886/global/health
{"healthy":true,"version":"consensus-0.1.0"}

$ sqlite3 /tmp/consensus-036-run/probe.db "insert into sessions (id, agent_name, model_id, status) values ('sesprobe1','probe','m-probe','idle'); insert into memory_events (id,type,content,session_id,iteration_created) values (4242,'user_message','hello probe','sesprobe1',1);"
ok

$ curl -s -w '\nHTTP %{http_code}\n' -X DELETE -H "Authorization: Bearer $K" http://127.0.0.1:18886/session/sesprobe1/message/msg-4242/part/prt-1
{"error":{"code":"NOT_FOUND","message":"part not found in message"}}
HTTP 404

$ curl -s -w '\nHTTP %{http_code}\n' -X DELETE -H "Authorization: Bearer $K" http://127.0.0.1:18886/session/smissing/message/msg-4242/part/prt-1
{"error":{"code":"NOT_FOUND","message":"session not found"}}
HTTP 404

$ curl -s -w '\nHTTP %{http_code}\n' -X DELETE -H "Authorization: Bearer $K" http://127.0.0.1:18886/session/sesprobe1/message/msg-9999/part/prt-1
{"error":{"code":"NOT_FOUND","message":"message not found in session"}}
HTTP 404

$ curl -s -w '\nHTTP %{http_code}\n' -X DELETE -H "Authorization: Bearer $K" http://127.0.0.1:18886/session/sesprobe1/message/not-a-msg/part/prt-1
{"error":{"code":"INVALID_REQUEST","message":"messageID must match the declared pattern ^msg"}}
HTTP 400

$ curl -s -w '\nHTTP %{http_code}\n' -X DELETE -H "Authorization: Bearer $K" http://127.0.0.1:18886/session/sesprobe1/message/msg-4242/part/not-a-prt
{"error":{"code":"INVALID_REQUEST","message":"partID must match the declared pattern ^prt"}}
HTTP 400

$ curl -s -w '\nHTTP %{http_code}\n' -X DELETE -H "Authorization: Bearer $K" http://127.0.0.1:18886/session/sesprobe1/message/msg-4242/part/
{"error":{"code":"INVALID_REQUEST","message":"required path parameter \"partID\" is missing"}}
HTTP 400

$ curl -s -w '\nHTTP %{http_code}\n' -X DELETE -H "Authorization: Bearer $K" -L http://127.0.0.1:18886/session/sesprobe1/message//part/prt-1
{"error":{"code":"INVALID_REQUEST","message":"messageID must match the declared pattern ^msg"}}
HTTP 400
```
