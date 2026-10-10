# SG-7 T10 — the fake "baby" project end to end

Date: 2026-10-10 · Branch: `wt/sg7-t10` · Spec: `docs/testing/quorum-q1-merged-verdicts.md` §4 SG-7 T10

## Result: task-create 201 ✓ · goal session idle ✓ · non-empty artifact ✓ · sha256 in tool_results ✓

## 1. The exact task-create body (the 2026-09-24 400 explained)

The sweep POSTed a **string** to a tasks endpoint. The real contract:

- Create is **nested**: `POST /api/v1/sessions/{session_id}/tasks` (there is no bare `POST /api/v1/tasks`; that path only supports GET/PATCH by task id).
- Body is `api.CreateTaskRequest` (internal/api/types.go:181):

```json
{
  "title": "Write the fake baby project artifact",
  "description": "Create /tmp/sg7-baby/artifact/hello.txt with a short greeting",
  "priority": 5,
  "prerequisite_ids": []
}
```

`title` required; `priority` 1..10 defaults 5; `prerequisite_ids` []string.

Legacy shape reproduced live (fresh DB /tmp/sg7-runA): string body →
`400 {"error":{"code":"INVALID_REQUEST","message":"malformed request body: json: cannot unmarshal string into Go value of type api.CreateSessionRequest"}}` —
the sweep had hit session-create, not task-create. Documented body → **201** on two independent fresh DBs (runA + runB), full responses in `t10-ledger.json`.

## 2. The baby project round trip (fresh DB /tmp/sg7-runB/goal-b.db, never reused)

1. `POST /api/v1/sessions {agent_name: "sg7-baby", goal: "…use the baby_write tool to create /tmp/sg7-baby/artifact/hello.txt…"}` → 201
2. User message drives the loop. Turn 1 (real deepseek-v4-flash call): the agent **issued** `tool_requests: [{tool_name: "baby_write", parameters: {path, content}}]` — agent-initiated, persisted to `staging_buffer` (`tool_call_ref`).
3. Tool executed by the production `ToolExecutorImpl.PollOnce` (internal/harness), which wrote the result row.
4. Result fed back per the h3 adapter convention (staging result + user-message envelope + re-wake). Turns 2–3: agent confirms and the session closes **idle**, having recorded the sha256 in a `text_block`.
5. 3 billing rows (1820/625, 1830/632, 2094/773 tok, ~0.061 USD total).

- Artifact: `/tmp/sg7-baby/artifact/hello.txt` — `Hello from the Consensus baby project!` (38 bytes)
- **sha256 `85f12bd5108efeef6facdca4d35f51b593c0cdcf5712412c204b77d006f8c0de`** — byte-identical in `tool_results.output` (row id=3, is_error=0) and in `sha256sum` of the artifact.

## 3. What the probe had to work around (defects found — file as DF rows)

- **T10-F1 (high):** `cmd/consensus/main.go` never starts the ToolExecutor (`harness.NewToolExecutor` has zero production callers). A stock `serve` leaves `tool_requests` pending forever. The probe ran the production executor via a sidecar (`/tmp/sg7-baby/t10-executor`, source staged at `t10probe/`, git-excluded) that imports `harness.NewToolExecutor` unchanged.
- **T10-F2 (high):** agent-issued tool calls on the live path go to `staging_buffer.tool_call_ref` and the session suspends in `tool_exec`, but **nothing in-product bridges `tool_call_ref` → `tool_requests`** or re-wakes the session with the result. Only the H3 shim implements the hand-off for its own clients. Without an external executor the session hangs (observed 9+ min); the probe drained the rows with `/tmp/sg7-baby/bin/ref_bridge.sh` and re-woke via the shim's documented convention.
- **T10-F3 (medium):** `subprocess` tools silently ignore agent parameters (fixed `handler_ref` argv, empty stdin); `http_endpoint` is the only parameter-carrying handler. The `baby_write` row was switched to `http_endpoint` → local writer returning `{wrote, bytes, sha256}`.

The tool row itself is probe data (tools_registry starts empty by design, docs/TOOLS.md); no product code was changed.

## 4. Evidence files

- `t10-ledger.json` — full contract, session round trip, billing, tool_results excerpt, findings, recompute commands
- `baby-project-artifact.txt` — the artifact (sha256 above)
- Key values redacted `[REDACTED]`; probe DBs and scratch files live under `/tmp/sg7-*` (ephemeral by design; recompute steps in the ledger regenerate them).
