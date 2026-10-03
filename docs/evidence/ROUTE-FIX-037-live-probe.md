# ROUTE-FIX-037 live probe — PATCH /session/{sessionID}/message/{messageID}/part/{partID}

ch:trace row=ROUTE-FIX-037
  spec=specs/openapi/upstream/openapi-1.18.33.json#part.update
  wave=consensus-foreman-2026-10-03-00-46-49.json#task-1
  test=internal/shim/opencode/session_p1_routes_test.go (TestSessionPartUpdate*)
  doc=docs/evidence/ROUTE-FIX-037-live-probe.md
  evidence=docs/evidence/ROUTE-FIX-037-live-probe.md
  witness=none:self-verified-in-worktree — scripted curls against a locally built
    binary; every value below is copied verbatim from the probe transcript.
    verdict=/commit= are deliberately absent in-tree: a verdict binds to a commit
    that already exists, so it is recorded by the commit footer / foreman
    verification, never self-referenced by the change it judges.

Date: 2026-10-02 (local, UTC-05) / 2026-10-03T01:35Z
Worktree: /home/kara/worktrees/consensus-ROUTE-FIX-037 (branch `wt/ROUTE-FIX-037`,
base `5f9239f`); the probe ran with this change applied to the worktree — the
committed diff is identical to the probed tree.

A locally built `consensus` binary was served on a **fresh SQLite database** and
the opencode shim route was driven over HTTP. Every value below is copied from a
live HTTP response or a live `sqlite3` invocation — none is hand-typed.

## 1. Run identity

| property | value |
|---|---|
| binary | `/tmp/t037-consensus` (sha256 `12e78564c53523f439f775ace1ec688977781e7404de5d567f4389ed2980a9c3`) |
| build | `go build -o /tmp/t037-consensus ./cmd/consensus`, from the worktree root |
| worktree / branch | `/home/kara/worktrees/consensus-ROUTE-FIX-037` @ `wt/ROUTE-FIX-037`, base `5f9239f` |
| server | `127.0.0.1:18511`, backend `sqlite`; `/global/health` = `{"healthy":true,"version":"consensus-0.1.0"}` |
| db | `sqlite:///tmp/t037-run/t037.db?_journal_mode=WAL`, created by the boot's auto-migrate |
| init | `consensus init --db-url …` exit 0, `Migrations: applied` |
| auth | bootstrap admin key (Bearer); captured into a file and used from a curl header — the value is never printed or recorded |

Seeded for the probe (direct `sqlite3`, after the boot's migrations ran):

- `sessions sesprobe1` — status `idle`
- `sessions sesbusy1` — status `thinking` (the runtime's real mid-turn state)
- `memory_events id=4242` (`user_message`, content `hello probe`) in `sesprobe1` (opencode id `msg-4242`)
- `memory_events id=4243` (`user_message`, content `busy probe`) in `sesbusy1` (opencode id `msg-4243`)

## 2. Declared contract, live

Request bodies: `{"type":"text","text":"edited"}` (a well-formed Part — the
declared requestBody is a `Part`, an anyOf of the twelve object variants).
`<code>:<n>` in the response column is `http_code:num_redirects` as reported by
curl; the single `:1` is the net/http ServeMux path-cleaning redirect on the
double-slash arm (the client followed it, exactly as the Go test client does).

| arm | request | observed |
|---|---|---|
| well-formed request, known message (no part store) | `PATCH /session/sesprobe1/message/msg-4242/part/prt-1` + Part body | `404` `{"error":{"code":"NOT_FOUND","message":"part not found in message"}}` |
| no-effect proof | `GET /session/sesprobe1/message/msg-4242` | `200` `{"info":{…,"id":"msg-4242",…},"parts":[{"text":"hello probe","type":"text"}]}` |
| unknown session (declared 404) | `PATCH /session/smissing/message/msg-4242/part/prt-1` | `404` `{"error":{"code":"NOT_FOUND","message":"session not found"}}` |
| unknown message (declared 404) | `PATCH /session/sesprobe1/message/msg-9999/part/prt-1` | `404` `{"error":{"code":"NOT_FOUND","message":"message not found in session"}}` |
| mid-turn session + known message (part.update declares no 409) | `PATCH /session/sesbusy1/message/msg-4243/part/prt-1` | `404` `{"error":{"code":"NOT_FOUND","message":"part not found in message"}}` — **not** 409 |
| absent body (tolerated → resource 404) | `PATCH /session/sesprobe1/message/msg-4242/part/prt-1` (no body) | `404` `{"error":{"code":"NOT_FOUND","message":"part not found in message"}}` |
| absent messageID (declared 400) | `PATCH /session/sesprobe1/message//part/prt-1` | `400:1` `{"error":{"code":"INVALID_REQUEST","message":"messageID must match the declared pattern ^msg"}}` |
| ill-shaped messageID (declared 400) | `PATCH /session/sesprobe1/message/not-a-msg/part/prt-1` | `400` `{"error":{"code":"INVALID_REQUEST","message":"messageID must match the declared pattern ^msg"}}` |
| absent partID (declared 400) | `PATCH /session/sesprobe1/message/msg-4242/part/` | `400` `{"error":{"code":"INVALID_REQUEST","message":"required path parameter \"partID\" is missing"}}` |
| ill-shaped partID (declared 400) | `PATCH /session/sesprobe1/message/msg-4242/part/not-a-prt` | `400` `{"error":{"code":"INVALID_REQUEST","message":"partID must match the declared pattern ^prt"}}` |
| malformed body (declared 400) | `PATCH …/part/prt-1` body `{"type":` | `400` `{"error":{"code":"INVALID_REQUEST","message":"malformed request body"}}` |
| non-object body (declared 400) | `PATCH …/part/prt-1` body `[1,2,3]` | `400` `{"error":{"code":"INVALID_REQUEST","message":"malformed request body"}}` |

Neighbours and auth (must be unchanged by this route):

| probe | observed |
|---|---|
| `POST /session/sesprobe1/message/msg-4242/part/prt-1` (undeclared method) | `404` `{"error":{"code":"NOT_FOUND","message":"endpoint not found"}}` (router default, pre-existing) |
| `DELETE /session/sesprobe1/message/msg-4242/part/prt-1` (part.delete, ROUTE-FIX-036) | `404` `{"error":{"code":"NOT_FOUND","message":"message not found in session"}}` (sessionDeleteMessage, pre-existing) |
| `GET /session/sesprobe1/message/msg-4242` after all probes | `200` `…"parts":[{"text":"hello probe","type":"text"}]` — content unchanged |
| `GET /session/status` | `200` `{"sesbusy1":{"type":"idle"},"sesprobe1":{"type":"idle"}}` |
| `PATCH …/part/prt-1` with **no** Authorization header | `401` `{"error":{"code":"UNAUTHENTICATED","message":"invalid credentials"}}` |

## 3. Which declared codes are reachable

`part.update` declares `200` (Part), `400` (BadRequest | InvalidRequestError) and
`404` (NotFoundError).

- **400 is reachable** — malformed body, non-object body, absent/ill-shaped
  messageID, absent/ill-shaped partID (all four live above).
- **404 is reachable** — unknown session, unknown message, and a well-formed
  partID that the runtime does not hold (live above).
- **409 is not in the declared set** and is not served: the mid-turn session above
  is answered by the resource lookup, never `SessionBusyError`.
- **The declared 200 Part is NOT served, and is not fabricated.** The runtime
  keeps no part store and no part-editing engine, so there is no `prt-…` id that
  exists to update and no updated Part that could be returned truthfully. The
  declared `404 NotFoundError` is the answer for every well-formed partID — the
  `sessionUnshare` precedent (ROUTE-FIX-015, a resource-targeting operation whose
  resource the runtime does not keep, where the declared 404 IS the truth). The
  in-code justification lives at the `sessionPartUpdate` doc comment
  (`internal/shim/opencode/session_p1_routes.go`, ROUTE-FIX-037). Answering 200 for
  a part-update that did not happen would assert an effect that was not performed
  (the `handleSyncStart` / `handleGlobalUpgrade` truthfulness convention), and
  unlike `session.deleteMessage` (ROUTE-FIX-035, whose declared 200 body is a
  plain boolean that can report the truthful `false`) this operation declares an
  OBJECT with no truthful "nothing happened" value.

## 4. Why no truthful 200 exists — live evidence

The message-part model is `memory_events.content`, read through
`getMessageByID`, and that ledger is append-only in **this** database (triggers
created by the boot's migrations):

```
-- runtime attempt: UPDATE memory_events (must abort)
Error in 2nd command line argument: memory_events is append-only: UPDATE is not permitted
-- runtime attempt: DELETE memory_events (must abort)
Error in 2nd command line argument: memory_events is append-only: DELETE is not permitted
-- row survives both attempts
4242|hello probe
```

And there is no part store to update, nor any part identifier to address:

```
-- tables matching %part%:
0
-- columns matching %part% in memory_events:
0
-- triggers on memory_events:
trg_memory_events_append_only_update
trg_memory_events_append_only_delete
-- total tables in the fresh database:
38
```

The GET right after the PATCH still reports the synthesized, **id-less** part
`{"text":"hello probe","type":"text"}` — the thing a real `part.update` would have
had to change — so the 404 is a truthful report of an effect that did not happen,
not a masked success.

## 5. Declared-vs-served flip

`specs/openapi/upstream/consensus-shim-served-surface.yaml` registers the route
(`outcome: 404`, with the per-arm evidence) and
`specs/openapi/upstream/opencode-declared-vs-served-1.18.33.json` is regenerated
by `scripts/compare-opencode-declared-served.py`:

| measure | before | after |
|---|---|---|
| `covered_operations` | 48 | **49** (`part.update` PATCH enters) |
| `narrowed_error_contract_only` | 10 | **9** (`part.update` leaves; `SHIM-NARROWED-006` in the pre-change artifact's positional ids) |
| `drift_declared_not_served` | 128 (NOT-SERVED 103, OUTCOME-MISMATCH 25) | 128 (unchanged — no new drift row) |
| `served_route_entries` | 96 | 98 (the PATCH route + the sibling DELETE registration) |

`SHIM-NARROWED-NNN` ids are positional (re-assigned by enumeration order on every
regeneration), so the flip is: the row that carried `id: SHIM-NARROWED-006`
(`PATCH …/part/{partID}`, `part.update`, `served_outcome: 404`) is now in
`covered`; the narrowed list simply shrinks. The sibling `part.delete` row stays
`SHIM-NARROWED-005` and stays narrowed — the served-surface record for it now
exists (DELETE on the sub-path answers the declared 404 from the message-DELETE
case), but its declared 200 is still unreachable, so
`scripts/compare-opencode-declared-served.py` keeps it in `ERROR_CONTRACT_ONLY`
until ROUTE-FIX-036 gives `part.delete` its own handler. That is why the
comparison's drift classes stay clean (no new `METHOD-MISSING` row from the
newly registered path).
