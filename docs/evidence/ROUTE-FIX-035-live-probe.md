# ROUTE-FIX-035 live probe — DELETE /session/{sessionID}/message/{messageID}

Task: `ROUTE-FIX-035` (source item `SHIM-NARROWED-005`). Generated 2026-10-02.

A locally built `consensus` binary was served on a **fresh SQLite database**
and the opencode shim route was driven over HTTP. Every value below is copied
from a live HTTP response or a live `sqlite3` invocation — none is hand-typed.

## 1. Run identity

| property | value |
|---|---|
| binary | `/tmp/rf035-bin` (sha256 `3395ce7732d600503838a57acdffb3ea2f818810fcb495fee4212d6bb4160d7b`) |
| HEAD under test | `2d8a7baa3cb50a55f8d86708c95cf0c87e5c8ed4` (worktree `wt/ROUTE-FIX-035`) |
| server | `127.0.0.1:18493`, backend `sqlite` |
| db | `sqlite:///tmp/rf035-run/rf035c.db?_journal_mode=WAL` (created by the boot's auto-migrate) |
| auth | bootstrap admin key (Bearer), read from the server log; value never recorded |

Seeded for the probe (direct `sqlite3`, after the boot's migrations ran):

- `sessions sesprobe1` — status `idle`
- `sessions sesbusy1` — status `thinking` (the runtime's real mid-turn state)
- `memory_events id=4242` in `sesprobe1` (opencode id `msg-4242`)

## 2. Declared contract, live

| arm | request | observed |
|---|---|---|
| **success (declared 200 boolean)** | `DELETE /session/sesprobe1/message/msg-4242` | `200` body `false` |
| message still present (truthful no-op) | `GET /session/sesprobe1/message/msg-4242` | `200` `{"info":{"id":"msg-4242",...},"parts":[{"text":"hello probe",...}]}` |
| unknown session (declared 404) | `DELETE /session/smissing/message/msg-4242` | `404` `{"error":{"code":"NOT_FOUND","message":"session not found"}}` |
| unknown message (declared 404) | `DELETE /session/sesprobe1/message/msg-9999` | `404` `{"error":{"code":"NOT_FOUND","message":"message not found in session"}}` |
| **mid-turn (declared 409)** | `DELETE /session/sesbusy1/message/msg-4242` | `409` `{"_tag":"SessionBusyError","message":"...","sessionID":"sesbusy1"}` |
| ill-shaped id (declared 400) | `DELETE /session/sesprobe1/message/not-a-msg` | `400` `{"error":{"code":"INVALID_REQUEST","message":"messageID must match the declared pattern ^msg"}}` |
| absent id (declared 400) | `DELETE /session/sesprobe1/message/` | `400` `{"error":{"code":"INVALID_REQUEST","message":"required path parameter \"messageID\" is missing"}}` |

Every declared response code — `200`, `400`, `404`, `409` — is reachable on the
live surface.

## 3. Why the 200 body is `false` (no genuine delete analog exists)

A message **is** a `memory_events` row. That ledger is append-only by
construction (SPEC-002 §2.1), enforced by triggers created by migration 017 in
this very database. The live probe confirms both write paths are rejected and
the row survives:

```
-- runtime attempt: UPDATE memory_events (must abort)
Error in 2nd command line argument: memory_events is append-only: UPDATE is not permitted
-- runtime attempt: DELETE memory_events (must abort)
Error in 2nd command line argument: memory_events is append-only: DELETE is not permitted
-- row still present after both attempts:
4242|sesprobe1
```

The shim never deletes `memory_events` anywhere, and the runtime keeps no delete
engine, no message tombstone and no separate parts store (parts are synthesized
from `memory_events.content`). The declared 200 boolean therefore reports the
truthful `false` ("no message was deleted") — never a fabricated `true` — the
same convention the sibling `sessionSummarize` route uses for an operation the
runtime does not keep an engine for (`d8f1d59`, ROUTE-FIX-018). Serving `false`
also makes the declared success code reachable, closing the pre-fix gap where
the route answered an untyped 404 for every request.

## 4. Declared-vs-served flip

`specs/openapi/upstream/consensus-shim-served-surface.yaml` moves the route to
`outcome: 200` with per-route evidence, and
`specs/openapi/upstream/opencode-declared-vs-served-1.18.33.json` is
regenerated (`scripts/compare-opencode-declared-served.py`):

- `covered_operations` 47 -> 48
- `narrowed_error_contract_only` 11 -> 10 (`SHIM-NARROWED-005` leaves the set;
  the remaining narrowed ids renumber positionally, by design)
- `drift_declared_not_served` 128 unchanged (NOT-SERVED 103, OUTCOME-MISMATCH 25)
