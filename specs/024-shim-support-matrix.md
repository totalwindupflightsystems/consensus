# 024 — Shim & Provider Support Matrix (what we must ALWAYS support)

Status: **contract proposal** (Bane, 2026-09-28). This file is the list of external
protocol/API surfaces the Consensus shim and LLM client commit to supporting, and the
refresh procedure that keeps them true. Implementation work is tracked by the board rows
`SHIM-SOURCE-001`, `SHIM-SUPPORT-001`, `SHIM-SUPPORT-002`.

Rationale: the shim exists so an existing coding-agent client can point at Consensus and
get a real backend (see 017/018). That promise is only worth as much as the fidelity of the
two surfaces it stands between: the **client-facing opencode protocol** and the
**provider-facing DeepSeek harness API**. Both drift upstream. Neither stays correct by
accident.

---

## A. Client-facing: the opencode protocol

**Source of truth.** The protocol is not prose; it is a machine-readable contract published
by the upstream project.

| Item | Value |
|---|---|
| Upstream project | `sst/opencode` (mirrored for us at `anomalyco/opencode`) |
| Protocol contract | `packages/sdk/openapi.json` — OpenAPI **3.1.1**, described upstream as the single source of truth for both the server implementation and SDK generation |
| Served spec | the server exposes the same OpenAPI document (our `/doc` must serve the *document*, not a Swagger UI page — see DF-CONSENSUS-36) |
| SDK | `@opencode-ai/sdk`, generated from that same document |
| Pinned checkout | the commit our upstream suite runs against, recorded in our evidence (`docs/evidence/opencode-upstream-*/`) |

**Explicitly a DIFFERENT protocol (do not conflate):** the Agent Client Protocol (ACP,
`agentclientprotocol/agent-client-protocol`, JSON-RPC 2.0 over stdio) connects editors to
agents. Consensus does **not** implement ACP and this file does not require it; adopting it
would be a separate decision (see `REVIEW-CONSENSUS-9` for the cross-vendor/interop door).

**Standing obligation (SHIM-SOURCE-001).**
1. The upstream commit + version we pin against is recorded in-repo, never implied.
2. Refresh is a procedure, not a memory: re-pin upstream, regenerate the spec-derived
   surface, and produce a **diff report** of protocol paths/schemas changed since our pin.
3. The shim's *served* surface is compared against the *declared* surface after every
   refresh (declared == served, per path). Drift is a row, not a shrug.
4. The upstream compatibility suite is re-run and its number re-measured at the new pin.
   The number is only meaningful with its pin attached.

---

## B. Provider-facing: the DeepSeek harness API

Consensus's harness runs its agent loop against a provider API. For DeepSeek — our default
cheap lane — that API is documented at `api-docs.deepseek.com` and has a structure we must
support in full, not the subset that happened to work first.

### B1. Three request formats, all first-class

| Format | Base URL | Endpoint |
|---|---|---|
| OpenAI Chat Completions | `https://api.deepseek.com` | `POST /chat/completions` |
| Anthropic Messages | `https://api.deepseek.com/anthropic` | `POST /anthropic/v1/messages` (`x-api-key`, `anthropic-version` ignored) |
| Responses API | `https://api.deepseek.com` | Responses-style request/response |
| Beta features | `https://api.deepseek.com/beta` | prefix completion, FIM |

Model-name mapping on the Anthropic surface (upstream behaviour we inherit):
`claude-opus*` → `deepseek-v4-pro` (billed as Pro); `claude-haiku*` / `claude-sonnet*` →
`deepseek-flash`; any unsupported name → `deepseek-flash`.

### B2. Thinking mode (the part a harness gets wrong silently)

| Concern | Rule from the provider docs |
|---|---|
| Toggle (OpenAI format) | `{"thinking": {"type": "enabled"/"disabled"}}` (in `extra_body`) |
| Toggle (Anthropic format) | `{"reasoning": {"effort": "none"/"low"/"high"/"max"}}` — `none` disables thinking |
| Toggle (Responses format) | `{"output_config": {"effort": ...}}` |
| Effort control | OpenAI `reasoning_effort`; Anthropic `output_config.effort` |
| Effort mapping | minimal→low, low→low, medium→high, high→high, xhigh→high, max→max, ultra→max |
| Default | thinking **enabled**, effort **high** |
| CoT transport | returned as `reasoning_content`, at the same level as `content` |
| **CoT round-trip** | with `tools` present: **all previous turns' `reasoning_content` MUST be sent back** and is concatenated into context. Without `tools`: it must NOT be sent (ignored if sent) |
| CoT persistence | `reasoning_content` is **persisted with the turn** (turn snapshot `iteration_commits.llm_response`, key `reasoning_content`) and handed back on the turn object — never dropped after being read |
| Thinking control | the toggle and the effort are resolved **explicitly** (per-request options, then client config); a value outside the effort mapping is **refused**, never silently dropped |
| Effort with thinking off | explicit `disabled` thinking ⇒ `reasoning_effort` is not sent (the dial is meaningless when thinking is off) |
| CoT as the answer | promoting `reasoning_content` into `content` is a **last-resort** substitution: only when `content` is empty, always logged at WARN, always flagged on the response (`reasoning_promoted_to_output`), and disableable — never a silent swap |
| Silently ignored | `temperature`, `presence_penalty`, `frequency_penalty` — accepted, no error, no effect |
| `top_p` | thinking mode only, clamped to ≥0.95 (below is treated as 0.95); non-thinking mode fixed at 1.0 and the value is ignored |

### B3. Tool calls

Per the provider docs: `tools[].function.{name,description,parameters}`; the assistant turn
returns `tool_calls`; results return as role `tool` with `tool_call_id`. `tool_choice`
accepts none/auto/required/named-function. Documented limits include a maximum tool count
and a name-length cap (verify exact numbers at implementation time rather than trusting this
paragraph).

**Mid-conversation insertion differs by format and matters to an agent loop:** the Anthropic
API (`/messages`) and the Responses API support inserting tool-call messages *and* system
messages mid-conversation; Chat Completions supports only system messages mid-conversation
and requires the Anthropic/Responses surface for injected tool calls.

### B4. Cost-relevant accounting

Cache accounting (`prompt_cache_hit_tokens` / `prompt_cache_miss_tokens` in usage) is what
makes our cost numbers honest — our own agent loops run at a very high cache-hit rate, so
ignoring it mis-states spend in both directions.

---

## C. Current state vs this contract (baseline measured 2026-09-28, commit 84bf6a3)

Evidence: request structs as serialized on the wire, non-test code. Rows marked
**(SHIM-SUPPORT-002)** were re-measured 2026-09-29 against live provider exchanges recorded
verbatim in `internal/llm/testdata/thinking-cot-turns.json` (every request body in that file
was POSTed to `api.deepseek.com/chat/completions` and answered HTTP 200).

| Requirement | Current state | Evidence |
|---|---|---|
| OpenAI format | present (SHIM-SUPPORT-002 adds tools/tool_choice/thinking/reasoning_effort) | `internal/llm/openai_client.go` — `openaiChatRequest` carries `model, messages, max_tokens, temperature, response_format, stream` plus `tools, tool_choice, thinking, reasoning_effort` |
| Anthropic format | present, **minimal** | `internal/llm/anthropic_client.go` — `anthropicMessageRequest` carries only `model, max_tokens, temperature, system, messages` |
| Provider-native `tools` / `tool_choice` | **client-capable, wire-present** (SHIM-SUPPORT-002) | `openaiChatRequest` now carries `tools`/`tool_choice`, settable per request (`RequestOptions`) or per client (`Config`); the *harness* still plans with its own JSON (`internal/harness/planning.go:392` logs `plan.ToolRequests`), so no native tool loop exists yet |
| Thinking toggle / effort (`thinking`, `reasoning_effort`, `output_config`) | **present, OpenAI surface** (SHIM-SUPPORT-002) | `openai_client.go` `RequestOptions` + `Config.Thinking/ReasoningEffort`; `thinking`/`reasoning_effort` sent on the wire; `output_config` (Responses format) still absent |
| `reasoning_content` read | **present and separated** (SHIM-SUPPORT-002) | `openai_client.go` returns the whole turn (`Content`, `ReasoningContent`, `ToolCalls`) and flags a last-resort CoT promotion instead of silently substituting it |
| `reasoning_content` round-trip with tools | **present** (SHIM-SUPPORT-002) | re-sent per prior turn when the request carries `tools`, omitted entirely when it does not; RED/Green both arms pinned in `internal/llm/thinking_cot_test.go` against recorded exchanges |
| Cache token accounting | **absent** | `prompt_cache` 0 hits, `cache_hit_tokens` 0 hits |
| Beta surface (prefix completion, FIM) | **absent** | `/beta` base not used; FIM 0 hits |
| `top_p` / penalty semantics in thinking mode | unsupported (not sent) — currently harmless, becomes a correctness issue the moment they are sent | provider docs, §B2 |

**Consequence to keep in view:** the "silently ignored parameters" and "CoT must be re-sent
when tools are present" rules are not pedantry. Today the harness sees none of it because it
sends a minimal request; the day `tools` goes on the wire, a naive implementation degrades
silently rather than failing loudly. That is the failure mode this file exists to prevent.
