---
name: opencode-programming-axiom
description: >
  Reusable integration playbook for building against the OpenCode HTTP server API:
  long-running /message semantics, /event monitoring, liveness-based completion,
  timeout layering, request-shape verification via /doc, SDK caution, and
  fail-closed session handling.
version: "1.1"
tags:
  vertical: [coding, ops]
  category: development
  core: false
---

# OpenCode Programming (Axiom)

Use this skill when you are implementing or debugging any runtime that talks directly to an OpenCode server — regardless of language (Go, Python, TypeScript, etc.).

This skill exists because OpenCode integration failures often look like generic network flakes when the real problem is one of these:

- A health-check timeout is reused for long-running LLM execution.
- `/message` is treated like a quick request instead of a blocking inference path.
- `/prompt_async` acceptance (HTTP 204) is treated as completed work.
- `/event` is ignored, so the runtime cannot tell whether a session is alive or merely slow.
- Request bodies drift from the live `/doc` contract.
- An SDK or generated client is trusted for operational logic it does not encode.

Authoritative contract: `specs/31-OpenCode-Integration-Contract.md`

axiom:trace spec=specs/31-OpenCode-Integration-Contract.md,specs/67-Go-Agent-Orchestration-Engine.md doc=.opencode/skills/opencode-programming-axiom/SKILL.md evidence=

## When To Load This Skill

Load it when you are:

- Implementing a new OpenCode client or adapter (any language).
- Debugging `/command`, `/message`, or `/prompt_async` behavior.
- Deciding whether to use `/event`, polling, or both.
- Tuning timeouts or fallback behavior.
- Validating request/response shapes against the running server.
- Diagnosing "server unreachable" or timeout errors on long-running LLM-backed requests.
- Evaluating whether an SDK or generated client covers the operational logic you need.

---

## 1. Core Rules

1. **Treat `/doc` as the live contract.** Before changing payloads, download the OpenAPI spec from the running server (`GET /doc`) and verify the exact request/response shape. Do not assume field names from memory or from an older SDK version.
2. **Separate timeout layers.** Health/startup timeouts, execution wall timeout, SSE connect timeout, stale threshold, and polling interval are different controls and must not be collapsed into one number.
3. **Treat `/message` as long-running.** It blocks while the LLM works. Do not classify it as a dead endpoint just because it runs longer than a health probe.
4. **Use `/event` as the primary monitoring path.** Open an SSE event stream when a session can outlive a single HTTP request.
5. **Use liveness polling as the safety net.** Poll `GET /session/{id}/message` (and descendants when supported) to detect forward progress when SSE is missing or unreliable.
6. **Do not confuse acceptance with completion.** HTTP 204 from `/prompt_async` means the async job was accepted, not that the stage is done.
7. **Fail closed on stale sessions.** If neither SSE nor liveness shows progress within the stale threshold, abort or surface a real error; do not silently return empty output.

---

## 2. SDK and Generated-Client Caution

SDKs (e.g., `@opencode-ai/sdk`) and OpenAPI-generated clients can help with:

- **Transport**: HTTP request construction, authentication headers, TLS.
- **Types**: Typed request/response structs, session/message/part models.
- **Endpoint discovery**: Knowing which paths exist and what fields they accept.

SDKs and generated clients typically **do not** encode:

- **Completion detection logic** — whether to wait for `session.idle`, poll `GET /session/{id}/message`, or both.
- **Timeout layering** — the distinction between health timeouts, execution wall timeouts, SSE connect timeouts, and stale thresholds.
- **Liveness monitoring** — tracking message/part growth as proof of forward progress.
- **Fallback chains** — when to fall from `/command` to `/message` to `/prompt_async`.
- **Stale-session handling** — recovery nudges, abort-on-inactivity, fail-closed discipline.
- **Long-running request semantics** — that `/message` is an LLM execution path, not a quick RPC.

**Rule:** Using an SDK for transport and types is fine and encouraged. But you must still implement (or verify the presence of) the operational logic listed above. Treat the SDK as a convenience layer, not as proof that the integration is complete.

**Verification checklist for SDK-based implementations:**

1. Does the client use a separate, long timeout for `/message` and `/prompt_async` dispatch (not the health-check timeout)?
2. Does the client open `/event` for session lifecycle monitoring?
3. Does the client poll `GET /session/{id}/message` as a liveness fallback?
4. Does the client distinguish HTTP 204 acceptance from actual completion?
5. Does the client fail closed when a session goes stale?
6. Does the client handle the `/command` → `/message` → `/prompt_async` fallback chain?

If any answer is "no" or "unknown," the integration is incomplete regardless of whether the SDK compiles and the types check.

---

## 3. Recommended Dispatch Model

### 3.1 Verify the live contract first

- Download the running server spec from `GET /doc`.
- Compare these endpoints:
  - `POST /session/{sessionID}/command`
  - `POST /session/{sessionID}/message`
  - `POST /session/{sessionID}/prompt_async`
  - `GET /event`
  - `GET /session/{sessionID}/message`

Questions to answer before writing dispatch code:

- Does `/command` require `arguments` or some other field?
- Does `/message` expect `parts` and an agent/model object?
- Does `/prompt_async` return `204 No Content`?
- Are `directory` or `workspace` query params documented for the session endpoints?

### 3.2 Use layered timeouts

| Timeout | Purpose | Typical default |
|---|---|---|
| `health_timeout` | Startup and `GET /global/health` | 30s |
| `execution_timeout` | Overall command/session wall timeout | 10m |
| `sse_connect_timeout` | Short timeout to open `GET /event` | 5s |
| `stale_threshold` | Inactivity window before a session is considered stalled | 5m |
| `keepalive_interval` | Polling cadence for message/liveness reads | 5s |

**Anti-pattern:** reusing `health_timeout` as the HTTP client timeout for long-running `/message` calls. This is the single most common integration bug.

### 3.3 Endpoint fallback chain

For structured command execution, attempt dispatch in this order:

1. **`POST /session/{id}/command`** — primary path (registered command execution).
2. **`POST /session/{id}/message`** — synchronous fallback. Blocks until the LLM response is complete. No SSE idle-wait required.
3. **`POST /session/{id}/prompt_async`** — last-resort async fallback. Returns 204 immediately; requires SSE + liveness-based completion detection.

Each tier falls to the next only on a real endpoint failure (HTTP 500, connection error, expired execution wall timeout) — not because a health-check-sized timeout expired on a long-running request.

### 3.4 Completion discipline

After dispatching work:

1. Open `GET /event` immediately and treat the SSE stream as the first source of truth.
2. Run liveness polling on `GET /session/{id}/message` in parallel as a safety net.
3. Read final assistant output **only after completion is actually proven** — via `session.idle` event, or via liveness polling confirming a substantive assistant response.

For `/prompt_async` specifically: continue through completion detection until one of:
- A matching `session.idle` event is observed and the final response is read.
- Liveness polling proves a completed assistant response is present.
- The execution wall timeout expires.
- The session is declared stale and recovery/abort logic fails closed.

---

## 4. Liveness Signals

**Proof of forward progress** (reset the stale timer):

- Matching `session.idle` event for the target session.
- New messages appearing in `GET /session/{id}/message`.
- Increased total number of parts in existing assistant messages.
- Child/descendant-session activity when the server supports it.

**Non-closing signals** (do not treat as completion):

- HTTP 204 from `/prompt_async`.
- A session existing without message/part growth.
- One successful health check before the run started.

---

## 5. Observability

Emit structured logs for:

- Dispatch tier chosen (command / message / prompt_async).
- Each fallback transition and reason.
- SSE connect success/failure.
- Keepalive seen (liveness confirmed).
- Keepalive overdue (approaching stale threshold).
- Recovery nudge sent / accepted / failed.
- Stale-session abort.
- Response extraction success/failure.

---

## 6. Quick Diagnostic Checklist

When an OpenCode-powered workflow is flaky, check in this order:

1. Does the request body still match `GET /doc`?
2. Are you using a health-sized timeout for LLM execution?
3. Are you opening `GET /event` for long-running sessions?
4. If SSE is missing, do you still poll `GET /session/{id}/message` for progress?
5. Are you treating HTTP 204 from `/prompt_async` as completion by mistake?
6. Does the final assistant output actually contain the keys/tags your caller expects?
7. If using an SDK, have you verified it covers completion detection and timeout layering (see §2)?

---

## 7. Key Lesson

Production experience across multiple runtimes (Python `.axiom` and Go `morty`) has confirmed:

- Fixing request-body field names is necessary but not sufficient.
- A runtime can still fall back too often if `/message` uses a health-sized client timeout.
- Final-stage completion can still fail if the runtime does not implement the `/event` + liveness model.
- An SDK that compiles and type-checks does not prove the operational integration is correct.

**OpenCode integration is not just request construction. It is request construction plus completion detection plus timeout discipline.**
