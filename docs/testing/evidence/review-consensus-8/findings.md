# REVIEW-CONSENSUS-8 — Uncovered surfaces: approvals, quarantine, dashboard, CLI verbs

Run date: 2026-10-15/16. Reviewer: coding-hermes worker (`wt/REVIEW-CONSENSUS-8`).
Scope: the four surfaces the board row named — the HITL approval gate, the
quarantine ("cognitive firewall") path, the web dashboard, and every CLI verb
past `init`/`serve`/`version`. No service code was modified; every defect below
is filed as a finding for a follow-up row.

Evidence: `docs/testing/evidence/review-consensus-8/raw/`.
Scratch instance description: `raw/09-scratch-environment-and-server-log.txt`.
Reproduce with `raw/tool-r8*.sh` against a fresh scratch instance.

---

## 1. Environment

| | |
|---|---|
| Binary | `go build -o /tmp/consensus-r8-bin ./cmd/consensus` @ `c9b7747` |
| Server | `consensus-r8-bin serve --config /tmp/r8/consensus-r8.yaml --port 18090` |
| Backend | SQLite, fresh DB created by `consensus init --config /tmp/r8/consensus-r8.yaml` (27 migrations, schema version 28) |
| Model | a local OpenAI-compatible **stub** on `:18123` (`raw/mock-llm.py`) |

**Why a stub model and not a provider key.** The live harness path is
`planning` (see §3), and a message only reaches an approval-gated tool call
after the model answers. Pointing `llm.base_url` at a local stub keeps the run
hermetic (no credentials, no egress) while still exercising the real
message → harness → tool-call → executor chain. Every model request the
harness made is captured verbatim in `raw/08-harness-model-requests.jsonl`.

**Admin key.** `consensus init` printed a fresh `cs_ak_…` admin key for the
scratch DB; it is redacted (`cs_ak_[REDACTED]`) everywhere in this evidence
tree. Nothing in the committed tree contains a live credential
(`grep -rn 'cs_ak_[A-Za-z0-9]\{20\}' docs/testing/evidence/review-consensus-8` → 0).

---

## 2. Scorecard against the pass criteria

| # | Criterion | Verdict |
|---|---|---|
| 1 | End-to-end approval gate: request → approve → session continues; reject → session blocks | **PARTIAL — approve resumes correctly, reject does NOT block (§4.2); and the gate does not actually gate (§4.1)** |
| 2 | A quarantined item is demonstrated quarantining and recovering | **MET** (`raw/04-quarantine.txt`) — with a follow-on defect (§4.6) |
| 3 | Every CLI verb past `init`/`serve`/`version` gets a smoke check | **MET** — 62 invocations, 46 captured command help pages (`raw/cli-help-tree/`, §5) |
| 4 | Results filed as evidence under `docs/testing/evidence/review-consensus-8/` | **MET** — this file + `raw/` |

Two of the route shapes the brief assumed do not exist on this tree; the real
API is recorded in §3.

---

## 3. What the surfaces actually are (measured, not read)

* **Approval review** — the only write route is
  `POST /api/v1/approvals/{id}/review` with `{"decision": "approved"|"rejected"|"modified"}`.
  There is no `POST /api/v1/sessions/{id}/approvals/{id}/approve|reject`
  (`raw/03b-approval-gate-and-review.txt`, E4 → `404 page not found`).
* **Two disjoint tool hand-off paths.** The *legacy* path (`RunAgentIteration`
  → `tool_requests` rows) is what the approval gate is wired to. The *live*
  harness path is `planning`: it stages the call as a `staging_buffer` row with
  `cmd_type='tool_call_ref'` and suspends the session in `tool_exec`. A
  `requires_approval` tool went through the live path with **no approval
  request created at all** (`raw/03a-planning-toolcall-handoff.txt`).
* **Quarantine** is reached through webhook ingestion
  (`POST /webhooks/{source}`), which scans the body and inserts a row into
  `external_quarantine`; operators then approve (promote to `memory_events`) or
  reject it. There is no API or CLI verb that creates a webhook registration or
  a routing rule — both had to be seeded in the DB (noted in §6).
* **Dashboard** is mounted at `/ui/` (`http.StripPrefix("/ui", webUI.Handler())`).

---

## 4. Findings

### 4.1 CRITICAL — on SQLite the approval gate does not gate: a `requires_approval` tool executes while its approval is still `pending`

**Claim.** `handleApprovalGated` parks an approval-gated `tool_request` by
writing `status='awaiting_approval'`. The live SQLite schema's CHECK constraint
does not allow that value, the write fails, the row stays `pending`, and the
same poll loop claims and executes it. The tool runs *before* any human
decision; the approval request is created and left `pending` alongside it.

**Evidence** (`raw/03b-approval-gate-and-review.txt`, section C2; probe in
`raw/07-awaiting-approval-check-constraint-probe.txt`):

```
-- sessions:
b0a129d3-7a0b-41c5-8c84-84db23bb0dff  paused             1     <- gate paused the session, as designed
-- tool_requests:
 1  b0a129d3-7a0b-41c5-8c84-84db23bb0dff  1  r8_approval_gated_echo  completed  (null)
-- approval_requests:
5a806490-1e2f-4389-aad9-6cdfaad3999c  tool_execution  r8_approval_gated_echo  pending  (null)
-- tool_results:
request_id  output                                  is_error
         1  R8-APPROVAL-GATE-BYPASS-PROOF-EXECUTED         0     <- it ran, with the approval pending
```

Server log, same run:

```
level=ERROR msg="tool_executor: failed to update tool_request status" request_id=1
  error="sqlite: exec: constraint failed: CHECK constraint failed: status IN
  ('pending', 'executing', 'completed', 'failed', 'timeout') (275)"
```

The constraint itself (live DB, `PRAGMA`-read from a copy):

```
$ sqlite3 constraint-probe.db "INSERT INTO tool_requests (..., status) VALUES (..., 'awaiting_approval');"
Error: CHECK constraint failed: status IN ('pending', 'executing', 'completed', 'failed', 'timeout')
$ sqlite3 constraint-probe.db "INSERT INTO tool_requests (..., status) VALUES (..., 'pending');"
  -> exit=0    (identical INSERT, only the status differs)
```

**Mechanism.** `internal/migrate/migrations/012_tool_sandbox.sql` deliberately
skips the CHECK-constraint rewrite on SQLite ("SQLite doesn't support ALTER
TABLE DROP CONSTRAINT … the Go runtime enforces status values at the
application level"). The Go runtime does **not** enforce it: it tries to write
the value the constraint forbids. `migrations/011_tool_sandbox.sql` (the
Postgres variant) *does* rewrite the constraint, so this is a SQLite-only hole.

**Impact.** The HITL gate is a no-op on the documented default backend. A tool
marked `requires_approval = true` — the mechanism for `delete_database`,
`send_email`, `charge_credit_card` in SPEC-014 §3 — performs its side effect
without a human in the loop. `tool_executor.go:285` counts
`'awaiting_approval'` as an outstanding state, so that branch is dead too.

**Suggested follow-up row.** Rebuild `tool_requests` in the SQLite migration
(new table + copy + drop + rename, the standard SQLite constraint-change
procedure) so `awaiting_approval` is legal; add a migration test that asserts
the live CHECK accepts all six statuses the Go code writes; add a regression
test that a `requires_approval` tool is *not* executed before the approval is
reviewed.

---

### 4.2 HIGH — a rejected approval resumes the session exactly like an approved one

**Claim.** `Manager.ReviewApproval` calls `ResumeSession` unconditionally, so
"reject" and "approve" have the same effect on the session: `paused → idle`.
Nothing distinguishes the two, and the linked tool request is not cancelled.

**Evidence** (`raw/03b-approval-gate-and-review.txt`, E3 → E5 → E6):

```
E3  sessions: b0a129d3-…  paused
E5  POST /api/v1/approvals/98f044b3-…/review  {"decision":"rejected", …}  -> 200
    {"id":"98f044b3-…","status":"rejected","reviewer_id":"admin",
     "review_notes":"R8 probe: operator rejects …", …}
E6  GET /api/v1/sessions/b0a129d3-…  ->  {"status":"idle", …}
```

The approve arm (D4 → D5) produces the identical transition
(`approved` → `status":"idle"`), so the two decisions are indistinguishable at
the session level. The pass criterion "reject → session blocks" is therefore
not met by the implementation.

**Mechanism.** `internal/hitl/hitl.go:301` — `if err := m.ResumeSession(ctx, sessionID); err != nil` runs on every decision path; there is no
`switch decision` around it. `handleApprovalGated`'s counterpart — re-queueing
an approved request, or cancelling a rejected one — does not exist anywhere.

**Impact.** Rejection carries the same information as approval: the operator
cannot stop an action by rejecting it, and the agent has no way to learn it was
refused (it is simply resumed).

**Suggested follow-up row.** Branch on the decision: approved → re-queue the
linked tool request (or leave it for the executor to pick up); rejected →
mark the request `failed`/`cancelled` with the reviewer's reason and surface
the refusal to the session (a `memory_events` row the harness projects into the
next turn) instead of silently resuming.

---

### 4.3 HIGH — the live harness path never consults the approval gate

**Claim.** On the path the server actually runs, a `requires_approval=true`
tool produces a `staging_buffer` hand-off with **no** `approval_requests` row.

**Evidence** (`raw/03a-planning-toolcall-handoff.txt`). An operator message
wakes the harness, the stub model requests the gated tool, and:

```
B3  GET /api/v1/sessions/{id}/tool-calls  ->  200
    [{"source":"staging_buffer","id":1,…,"tool_name":"r8_approval_gated_echo",
      "parameters":{"target":"production"},"status":"executed",
      "description":"tool_call: r8_approval_gated_echo","executed":true}]
B4  GET /api/v1/sessions/{id}/approvals   ->  200  []
B5  GET /api/v1/approvals                 ->  200  []
B6  GET /api/v1/sessions/{id}             ->  {"status":"tool_exec", …,"iteration":1}
```

Server log: `planning: tool call requested turn=1 tools=1` — and no approval
request exists for the session (B4/B5 both return `[]`, and `approval_requests`
is empty until §4.1's seeded `tool_requests` row creates one). The same tool
*did* get an approval request when a `tool_requests` row was created (§4.1),
which isolates the difference to the hand-off path, not the registry row or the
tool name.

*Caveat on log-based evidence:* the `tool_executor: approval gate created` info
line is not a usable signal here — on SQLite it is unreachable, because the
preceding status write fails and the loop `continue`s before reaching it (it
appears zero times in the whole run, including §4.1 where an approval was
definitely created). The decisive evidence is the `approval_requests` table and
the two API reads above.

**Impact.** The registry flag that SPEC-014 §3 describes as the gate —
`requires_approval = true` — is inert on the live path. A caller that consumes
`tool_call_ref` entries (the H3 shim's `service_adapter`, or anything polling
`GET /api/v1/tool-requests`) receives the call straight through.

**Suggested follow-up row.** Decide which path owns HITL and make it
explicit: either the planning path must consult `tools_registry.requires_approval`
before it stages a `tool_call_ref` (creating the approval request and suspending
in `paused` rather than `tool_exec`), or the two paths must be reconciled onto
one hand-off table with one gate.

---

### 4.4 MEDIUM — the tool request ⇄ approval link is never established

**Claim.** `handleApprovalGated` writes `approval_request_id` together with the
`awaiting_approval` status in one UPDATE. Because the status write fails (§4.1),
the link is never stored either, so the API cannot answer "which approval
belongs to this tool request" (or the reverse) — even in the runs where an
approval does exist.

**Evidence:** every `approval_request_id` in `raw/03b-…` C2/E3/E7 is `(null)`,
including the row whose approval was approved by the reviewer. The
`idx_tool_requests_approval` index exists on that column and is permanently empty.

---

### 4.5 MEDIUM — `POST /api/v1/tools/{name}/execute` cannot enqueue an external tool (HTTP 500)

**Claim.** The documented way to ask the runtime to run an `http_endpoint` or
`subprocess` tool always fails: `enqueueExternalTool` omits `iteration_id`,
which is `NOT NULL`.

**Evidence** (`raw/02-session-create-and-llm-key-gate.txt`, A2):

```
POST /api/v1/tools/r8_approval_gated_echo/execute
{"session_id":"b0a129d3-…","parameters":{"target":"production"}}

HTTP/1.1 500 Internal Server Error
{"error":{"code":"INTERNAL_ERROR","message":"failed to enqueue tool","details":"failed to enqueue tool"}}
```

with the server-side cause:

```
level=ERROR msg="api: failed to enqueue tool" tool=r8_approval_gated_echo
  error="sqlite: exec: constraint failed: NOT NULL constraint failed: tool_requests.iteration_id (1299)"
```

For comparison, the harness's own insert at
`internal/harness/executor.go:181` *does* supply `iteration_id`. This is why the
approval-gate arm in §4.1 had to seed the `tool_requests` row directly (stated in
the transcript at C1) — the API route that should have created it is dead.

**Impact.** The only REST entry point into the external-tool executor returns
500 for every request; a client cannot trigger a gated tool through the API at all.

---

### 4.6 HIGH — the cognitive firewall quarantines a copy but still delivers the payload to the agent

**Claim.** A signature-valid webhook whose body trips the scanner (here
`SQLI-DROP-TABLE`, confidence 0.8) is copied into `external_quarantine` **and**
left in status `pending`, so the routing loop delivers the malicious body into
the agent's memory verbatim.

**Evidence** (`raw/04-quarantine.txt`, Q1 → Q4):

```
level=WARN msg="webhook: quarantining event content" source=r8-quarantine-probe
  source_id=r8-delivery-1 signature_valid=true threat=true confidence=0.8
  reason="Quarantine scanner flagged content: [SQLI-DROP-TABLE] DROP TABLE statement in content"

-- external_events after routing:
 1  r8-delivery-1  1  routed      <- the flagged event, delivered
 2  r8-delivery-2  0  quarantined <- only the *signature* failure is withheld

-- memory_events delivered to the agent:
 2  user_message  [webhook r8.probe] {"tool":"db_admin","sql":"DROP TABLE users; -- r8 probe malicious …
```

**Mechanism.** `internal/webhook/webhook.go:279` sets the event status from
`statusForEvent(event.SignatureValid)` only — a valid signature yields
`pending` regardless of what the scanner said. The scanner branch
(`webhook.go:830-855`) inserts the quarantine row but never transitions the
event out of `pending`.

**Impact.** The firewall is advisory for the threat class it is primarily built
for (untrusted content from a trusted source). An operator approving or
rejecting the quarantine item afterwards changes nothing about what the agent
already read.

**Suggested follow-up row.** When the scanner flags a signature-valid event,
transition the event to `quarantined` in the same request (or gate delivery on
`external_quarantine.validation_status`), and add an integration test asserting
a flagged payload does not reach `memory_events` before it is approved.

---

### 4.7 MEDIUM — the web dashboard's sub-pages are unroutable, and its navigation links point outside the mount

**Claim.** `/ui/` renders, but every link it renders 404s, and the four
sub-pages declared in `internal/web/server.go` are unreachable through the
`/ui/` mount.

**Evidence** (`raw/06-dashboard-and-ui-surface.txt`, re-verified):

```
GET /ui/                200   5691 bytes  <title>Consensus — Admin UI</title>
GET /ui/dashboard       404   19 bytes
GET /ui/sessions        404   19 bytes
GET /ui/memory          404   19 bytes
GET /ui/health          404   19 bytes
GET /ui/api/v1/health   404   19 bytes   (the UI's own API proxy path)
GET /ui/abc             404   19 bytes   (identical shape to the four real pages)
```

and the links in the served HTML are root-relative, i.e. they leave the mount
entirely:

```
$ curl -s http://127.0.0.1:18090/ui/ | grep -o 'href="[^"]*"'
href="/dashboard"   href="/memory"   href="/sessions"
$ for p in /dashboard /sessions /memory /health; do …; done
/dashboard  404    /sessions  404    /memory  404    /health  404
```

**Mechanism.** Two independent defects:

1. The mount is `apiMux.Handle("/ui/", http.StripPrefix("/ui", webUI.Handler()))`
   (`cmd/consensus/main.go:395`). chi's `Handle` with a bare `/ui/` pattern
   matches only that exact path — a subtree needs `/ui/*`. The two mounts
   immediately below it get this right (`/mcp/*` **and** bare `/mcp`;
   `/chronicle/*`), and the code comment above them spells the rule out
   ("chi v5's `/mcp/*` wildcard matches subpaths but not the bare `/mcp` path,
   so both registrations are required", C-GAP-005). `/ui/` has neither form, so
   `/ui/dashboard` matches no route and falls through to chi's 404 — which is
   why `/ui/abc` and `/ui/dashboard` are byte-identical.
2. `pageShell` emits root-relative `href`s while the app is mounted under
   `/ui/`, so even a corrected mount would send the browser to the API router's
   namespace.

*Correction to an earlier reading during this run:* the 404s carry
`Access-Control-Allow-Origin: *` and friends, which initially looked like proof
that the request reached the web package's own CORS middleware. It does not —
`internal/api/server.go:141` applies `r.Use(s.allowCORSMiddleware)` to the whole
chi router, and that middleware sets the identical three headers. The response
headers are therefore not evidence of which handler ran; the route-matching
argument above is.

**Scope note:** the Chronicle workbench at `/chronicle/` was probed too and
serves 312 KB of HTML (200). `/mcp` holds an SSE stream open (expected;
curl's 10 s timeout fired first).

---

### 4.8 MEDIUM — CLI verbs that are dead against a live server

Recorded verbatim with exit codes in `raw/05a-cli-smoke-battery.txt`. The
verb family works only in part; these four are broken against a running server:

| Verb | Observed | Root cause (source) |
|---|---|---|
| `consensus config get <key>` | `exit 5`, `NOT_FOUND: config key "server.url" not found` (also for any real key) | `GET /api/v1/config` returns settings that never contain the dotted keys the CLI asks for |
| `consensus config set k v` | `exit 1`, `HTTP 405` | CLI does `PATCH /api/v1/config`; the server registers only `GET` on that path (`internal/cli/config.go:119` vs `internal/api/server.go:194`) |
| `consensus migrate` / `migrate run` / `migrate rollback` | `exit 5`, `HTTP 404: 404 page not found` | CLI POSTs `/api/v1/migrate` and `/api/v1/migrate/rollback`; neither route is registered (`internal/cli/migrate.go:65,141`). `migrate create` and `migrate version` do work. |
| `consensus models sync` | `exit 2`, `sync: decode models.dev response: invalid character '<' looking for beginning of value` | the fetch returns HTML, not JSON (blocked/redirected), and the decoder has no content-type guard |

`consensus memory pages <session>` also fails, but with `NOT_FOUND: memory
event not found` — the handler resolves a *memory event* where the verb asks
for compressed pages, so the route or its argument is mismatched (`exit 5`).

Everything else past `init`/`serve`/`version` smoke-passes: the `approve`
family (list/show/accept/reject), the top-level `reject`, the whole `session`
family (list/show/logs/cost/message/pause/resume/cancel/create), `memory
list|iterations|show`, `skill list|show`, `tool list|show`, `status`,
`config list`, `config edit`, `migrate version|create`, `completion`, `help`,
and `mcp-stdio`.

---

### 4.9 LOW — `GET /webhooks/{source}` is ingested as an event

`curl -sS -i http://127.0.0.1:18090/webhooks/r8-quarantine-probe` (no body, no
signature) returned `202 Accepted` and created
`external_events` row 3 (`event_type=unknown, status=quarantined`).
`HandleWebhook` never checks `r.Method`. Harmless when the signature check
rejects the body, but the route is a write endpoint that answers GET, which
makes it indexable/prefetchable by anything that crawls the origin.

---

### 4.10 LOW — zero and Go-`String()` timestamps surface in the API

**Claim.** Approval timestamps render as `0001-01-01T00:00:00Z`, and session
`heartbeat_at` carries a raw Go `time.Time` string including its monotonic
reading.

**Evidence** (`raw/03b-approval-gate-and-review.txt`, D4/D5; DB read-back):

```
GET /api/v1/approvals            -> "created_at":"0001-01-01T00:00:00Z"
POST …/review (approved)         -> "created_at":"0001-01-01T00:00:00Z","reviewed_at":"0001-01-01T00:00:00Z"
GET /api/v1/sessions/{id}        -> "heartbeat_at":"2026-10-15 21:36:39.458771224 -0500 -05 m=+90.923647239"

$ sqlite3 consensus.db "SELECT created_at, reviewed_at FROM approval_requests;"
2026-10-15 21:36:28.67873271 -0500 -05 m=+80.143608725
2026-10-15 21:36:39.451781856 -0500 -05 m=+90.916657872
```

Two distinct defects: the HITL/harness writers stringify `time.Time` directly
instead of formatting RFC3339, and the reader turns the unparseable text into
the zero `time.Time`. Rows written by the CLI/API path
(`created_at=2026-10-16T02:35:21Z`, `expires_at=…Z`) use the correct format, so
the tree contains both shapes — any SQL comparing `expires_at < now` therefore
compares a Go-`String()` against RFC3339 lexicographically.

**Impact.** Operators cannot see when an approval was raised or decided; the
HITL expiry cron's comparison is unreliable for these rows.

---

## 5. CLI verb coverage

* Enumerated by recursively walking `--help` from the root: **46 command help
  pages** (the root plus 45 subcommands) captured under `raw/cli-help-tree/`,
  generated by `raw/tool-r8-cli-help-walk.sh`.
* **62 minimal invocations** executed against the scratch server
  (`raw/05a-cli-smoke-battery.txt` 51, `raw/05b-cli-smoke-battery-part2.txt` 11),
  each with the command line, verbatim stdout+stderr and the exit code.
* Exit-code distribution: **43 × 0**, **1 × 1**, **7 × 2** (usage/argument
  errors, all with a clear message), **11 × 5** (not found — correct for the
  bad-id/unknown-name probes, wrong for §4.8's `config get`/`migrate`).
* Every expected-error path named the missing argument rather than failing
  silently (e.g. `session create` → `--goal is required`, `approve show` →
  `accepts 1 arg(s), received 0`). No verb hung.

---

## 6. Residuals, caveats, and side effects

1. **Approval-gate arm is not fully API-driven.** Because
   `POST /api/v1/tools/{name}/execute` is broken (§4.5), the `tool_requests`
   row that the gate consumes was seeded with `sqlite3` against the scratch DB
   (visible in `raw/03b-…` at C1/E1). Everything from that point on — the gate,
   the approval records, the review route, the session transitions — is real
   runtime behaviour, but the *entry* into that path is a fixture.
   The planning path (§4.3) *is* fully API-driven: session created and message
   sent over REST, harness and executor unmodified.
2. **The model is a stub.** Tool selection is therefore scripted, not sampled
   from a real model. Nothing in §4.1–§4.6 depends on model behaviour: the gate,
   the constraint, the delivery status and the resume-on-reject branch are all
   server-side.
3. **Webhook registrations and routing rules have no creating verb.** Both were
   seeded directly (`raw/04-quarantine.txt`, Q and Q4 preamble). If they are
   meant to be operator-manageable, that is itself a missing surface; the
   approval/quarantine API is exercised end to end once the row exists.
4. **Side effect observed on a non-scratch database.** `consensus mcp-stdio`
   run *without* `--config` resolves the default config chain and bootstraps
   `~/.consensus/consensus.db` (proved by the admin-key log line: it reported
   `key_prefix=cs_ak_76 id=f2252bca-…`, the key that exists in that DB).
   Re-running it with `--config /tmp/r8/consensus-r8.yaml` bootstraps the
   scratch DB instead (`cs_ak_6c`), recorded in
   `raw/05b-cli-smoke-battery-part2.txt`. Separate from that, that file was
   migrated from schema 26 to 28 at `2026-10-15T21:39:33-05:00` — inside the
   window of this battery, but **not attributable to it**: the WAL mtime did not
   move when `mcp-stdio` was re-run, and the host has two long-lived consensus
   processes (`consensus-sidecar`, `consensus-ra018-eval`) that own default-config
   databases. Flagged for the record; no R8 command was found that writes to
   that DB.
5. **Not covered.** Postgres-specific behaviour (the gate's CHECK *is* rewritten
   on Postgres by `migrations/011_tool_sandbox.sql`, so §4.1 is expected to be
   SQLite-only — unverified here); the shim surfaces beyond `/chronicle/` and
   `/mcp`; billing/budget paths; `delegate`/sub-agent spawning.
6. **No service code was modified.** All ten findings are filed for follow-up
   rows; the commit that carries this evidence changes documentation only.
