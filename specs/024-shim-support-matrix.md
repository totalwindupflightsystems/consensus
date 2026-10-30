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

**Intentional narrowing is explicit coverage, not upstream parity.** A declared
operation may remain unsupported only when the owning protocol spec names the
exclusion, the served-surface row sets `x-shim-intentional-narrowing: true` with
a non-empty `narrowing-rationale`, and the runtime answers the typed `501`
`not_implemented` envelope. The route comparator records these rows under
`intentional_narrowing`, separate from both `covered` and drift. It rejects a
missing rationale or any outcome other than `501-typed`, so a `404`, bare `501`,
or fabricated success cannot be hidden as an intentional narrowing. `POST /pty`
(`pty.create`) is the first such row; SPEC-017 §3.9 owns its exclusion.

**The second class is an operation that IS answered — with an error.** A declared
operation the shim routes and answers, but never with its declared SUCCESS code
(its served outcome is a declared error code while the document also declares a
success code the runtime cannot produce), sets
`x-shim-error-contract-narrowing: true` with a non-empty `narrowing-rationale`
naming why the declared success is unreachable and which declared arms ARE served.
The comparator DETECTS this class from the declared response set and the served
outcome — it is not a hand list, so such a row can no longer be scored `covered`
merely because the error it answers is in the declared set — records it under
`narrowed_error_contract_only` (with the rationale), and ABORTS on a detected
narrowing without one. `counts.narrowed_unexplained` is therefore `0` by
construction; SPEC-018 §12 is the per-row record and the table of decisions. A
client that can only ever receive an error from an endpoint cannot use the
feature, so widening is always the preferred outcome (SHIM-GAP-004 widened
`permission.reply` and `tui.selectSession`); the narrowing is the honest fallback
when the state the declared success describes does not exist.

---

### A1. v2 session interrupt

| Operation | Path | Status | Outcome | Evidence |
|---|---|---|---|---|
| `v2.session.interrupt` | `POST /api/session/{sessionID}/interrupt` | COVERED | 204 | `internal/shim/opencode/api_session_interrupt_test.go` (ROUTE-ADD-046) |

## B. Provider-facing: the DeepSeek harness API

Consensus's harness runs its agent loop against a provider API. For DeepSeek — our default
cheap lane — that API is documented at `api-docs.deepseek.com` and has a structure we must
support in full, not the subset that happened to work first.

**Documentation read on 2026-10-13.** Everything in §B was re-derived from the provider
documentation on that date and, where a call is cheap, from the live surface; every claim
that rests on a call points at `internal/llm/testdata/shim-support-exchanges.json`, which
holds verbatim exchanges (request body as sent, response bytes as received). See §E for how
to re-run this.

### B1. Three request formats, all first-class

| Format | Base URL | Endpoint |
|---|---|---|
| OpenAI Chat Completions | `https://api.deepseek.com` | `POST /chat/completions` |
| Anthropic Messages | `https://api.deepseek.com/anthropic` | `POST /anthropic/v1/messages` (`x-api-key`; `anthropic-version` ignored) |
| Responses API | `https://api.deepseek.com` | `POST /responses` |
| Beta features | `https://api.deepseek.com/beta` | `POST /beta/chat/completions` (prefix completion), `POST /beta/completions` (FIM) |

The Anthropic base carries no `/v1`: the endpoint is `base + /v1/messages`, while the
Anthropic SDK convention (a base ending in `/v1`) keeps `base + /messages`. The client
resolves both spellings.

Model-name mapping on the Anthropic surface (upstream behaviour we inherit):
`claude-opus*` → `deepseek-v4-pro` (billed as Pro); `claude-haiku*` / `claude-sonnet*` →
`deepseek-flash`; any unsupported name → `deepseek-flash`. The mapping happens provider-side,
so the client passes the requested name through — which means a `claude-opus*` request costs
V4 Pro money. Recorded here because that consequence is otherwise invisible.

### B2. Thinking mode (the part a harness gets wrong silently)

| Concern | Rule from the provider docs |
|---|---|
| Toggle (OpenAI format) | `{"thinking": {"type": "enabled"/"disabled"}}` (in `extra_body`) |
| Toggle (Anthropic format) | `{"reasoning": {"effort": "none"/"low"/"high"/"max"}}` — `none` disables thinking |
| Toggle (Responses format) | `{"reasoning": {"effort": ...}}` — measured 2026-10-13 to be the honoured spelling; see the divergence note below |
| Effort control | OpenAI `reasoning_effort`; Anthropic `output_config.effort` |
| Effort vocabulary | `none` (off, OpenAI spelling), `minimal`, `low`, `medium`, `high`, `xhigh`, `max`, `ultra` |
| Effort mapping | minimal→low, low→low, medium→high, high→high, xhigh→high, max→max, ultra→max |
| Default | thinking **enabled**, effort **high** |
| CoT transport | returned as `reasoning_content`, at the same level as `content` |
| **CoT round-trip** | with `tools` present: **all previous turns' `reasoning_content` MUST be sent back** and is concatenated into context. Without `tools`: it must NOT be sent (ignored if sent). The provider answers **HTTP 400** when a tool-bearing continuation omits it, so this is loud, not silent |
| Anthropic CoT round-trip | a prior assistant turn that carries `tool_use` must echo its thinking block — text **and** signature — or the request is refused (measured: missing block → 400 naming the thinking content; block echoed → 200) |
| CoT persistence | `reasoning_content` (and its signature) is **persisted with the turn** (turn snapshot `iteration_commits.llm_response`) and handed back on the turn object — never dropped after being read |
| Thinking control | the toggle and the effort are resolved **explicitly** (per-request options, then client config); a value outside the effort mapping is **refused**, never silently dropped |
| Effort with thinking off | explicit `disabled` thinking ⇒ no effort dial is sent (the dial is meaningless when thinking is off) |
| CoT as the answer | promoting `reasoning_content` into `content` is a **last-resort** substitution: only when `content` is empty, always logged at WARN, always flagged on the response (`reasoning_promoted_to_output`), and disableable — never a silent swap. The Anthropic surface needs it most: a thinking-mode turn can arrive with a `thinking` block and no text block at all |
| Silently ignored | `temperature`, `presence_penalty`, `frequency_penalty` — accepted, no error, no effect. While thinking is on the client **drops them from the request** (carrying them would imply an effect they do not have) and names each dropped parameter at WARN; a thinking-disabled request passes them through. Enforced by `TestSamplingContract_ThinkingModeRequestShape`, `_NonThinkingModeRequestShape` and `_CoversEveryFormat` |
| `top_p` | thinking mode only, floored to ≥0.95 (below is treated as 0.95); non-thinking mode fixed at 1.0 and the caller's value is ignored — the client sends that fixed 1.0, never the ignored number, and sends nothing when nobody asked. Enforced by `TestSamplingContract_TopPSemanticsAreEncoded` and `_CoversEveryFormat` |
| `max_tokens` | documented range 1–393216; provider defaults 8K non-thinking, 64K thinking, 128K at `effort: max` |

**Documented-vs-live divergences (measured 2026-10-13).** Two documented spellings do not do
what the docs say on the live surface. They are recorded as *not-supported* matrix cells with
rows (`SHIM-SUPPORT-007`, `SHIM-SUPPORT-008`) rather than smoothed over, because a caller who
trusts the documentation would believe they had changed provider behaviour:

1. Anthropic `reasoning.effort: "none"` — the documented off switch — still returns thinking
   blocks. The client sends the documented request and does not claim the effect.
2. Responses `output_config.effort` — this file previously listed it for the Responses
   format — is inert: with both spellings in one request, `reasoning.effort` was echoed back
   and no `output_config` appeared in the response.

### B3. Tool calls

Per the provider docs: `tools[].function.{name,description,parameters}` on the OpenAI surface,
`tools[].{name,description,input_schema}` on the Anthropic surface, and a flat
`tools[].{name,description,parameters}` on the Responses surface; the assistant turn returns
`tool_calls` (or `tool_use` blocks); results return as role `tool` with `tool_call_id` (or a
`tool_result` block with `tool_use_id`). `tool_choice` accepts none/auto/required/named-function
(the Anthropic vocabulary spells `required` as `any`).

Documented limits, verified against the API reference on 2026-10-13 rather than trusted from
this paragraph: function names are limited to `a-z`, `A-Z`, `0-9`, `_`, `-`, are at most
**128** characters, and **must be unique**. A `strict` flag exists for schema-enforced tool
calls (beta).

**Measured restriction:** `required` and named tool choices are **not** supported in thinking
mode — the provider answers HTTP 400. The client refuses those locally, before sending.

**Mid-conversation insertion differs by format and matters to an agent loop:** the Anthropic
API (`/messages`) and the Responses API support inserting tool-call messages *and* system
messages mid-conversation; Chat Completions supports only system messages mid-conversation
and requires the Anthropic/Responses surface for injected tool calls.

### B4. Cost-relevant accounting

Cache accounting (`prompt_cache_hit_tokens` / `prompt_cache_miss_tokens` in usage) is what
makes our cost numbers honest — our own agent loops run at a very high cache-hit rate, so
ignoring it mis-states spend in both directions. Measured: two identical 260-token prompts
went from `0 hit / 260 miss` to `126 hit / 134 miss`, and `prompt_tokens = hit + miss`.
`usage.prompt_tokens_details.cached_tokens` carries the same number as
`prompt_cache_hit_tokens`. The Anthropic surface reports
`cache_read_input_tokens` / `cache_creation_input_tokens`, and the Responses surface reports
`usage.input_tokens_details.cached_tokens`.

---

## C. Current state vs this contract (re-measured 2026-10-13, SHIM-SUPPORT-001)

Evidence: request structs as serialized on the wire and the recorded exchanges in
`internal/llm/testdata/shim-support-exchanges.json` (24 exchanges; every request body in that
file was POSTed to `api.deepseek.com` as written, and each response is the provider's own
bytes — two of them are 400s kept because they are the evidence for the refusal rules).
Rows marked **(SHIM-SUPPORT-002)** were first measured 2026-09-29 against
`internal/llm/testdata/thinking-cot-turns.json`.

| Requirement | Current state | Evidence |
|---|---|---|
| OpenAI format | present, complete | `internal/llm/openai_client.go` — `openaiChatRequest` carries `model, messages, max_tokens, temperature, top_p, presence_penalty, frequency_penalty, response_format, stream, tools, tool_choice, thinking, reasoning_effort`. The three sampling fields are populated by `ResolveSamplingParams`, so they reach the body only for a thinking-disabled request |
| Anthropic format | present, complete | `internal/llm/anthropic_client.go` — `anthropicMessageRequest` carries `model, max_tokens, temperature, top_p, presence_penalty, frequency_penalty, system, messages, reasoning, output_config, tools, tool_choice` plus content-block turns (thinking / text / tool_use / tool_result). `BuildResponsesRequest` resolves the same sampling contract on the Responses body |
| Responses format | wire layer present, **not selectable** | `internal/llm/responses_format.go` (builder + interpreter, pinned against a recorded exchange); `llm.NewClient` has no provider case for it → `SHIM-SUPPORT-005` |
| Provider-native `tools` / `tool_choice` | **present on all three formats** | `ToolDefinition` → OpenAI nested shape, Anthropic `input_schema` shape, Responses flat shape; `tool_choice` mapped per format (`required` → Anthropic `any`) |
| Thinking toggle / effort (`thinking`, `reasoning_effort`, `reasoning`, `output_config`) | **present**, all documented spellings | `RequestOptions.Thinking/ReasoningEffort`, `Config.Thinking/ReasoningEffort`; OpenAI `thinking`+`reasoning_effort`, Anthropic `reasoning.effort`+`output_config.effort`, Responses `reasoning.effort` |
| `reasoning_content` read + round-trip | **present on both wire formats** | OpenAI: re-sent per prior turn when tools are present, omitted otherwise. Anthropic: thinking block (+ signature) echoed with its turn, which the provider requires |
| `reasoning_signature` | **present** | `harness.Message`/`harness.LLMResponse` + turn snapshot; populated from the provider's `thinking.signature` |
| Cache token accounting | **present on all three formats** | `prompt_cache_hit_tokens` (preferred) / `prompt_cache_miss_tokens` / `prompt_tokens_details.cached_tokens`; Anthropic `cache_read_input_tokens` / `cache_creation_input_tokens`; Responses `input_tokens_details.cached_tokens` |
| `top_p` semantics | **present and enforced** | `ResolveTopP` — floored at 0.95 in thinking mode; the fixed 1.0 in non-thinking mode (the caller's ignored number is never forwarded); nothing sent when unconfigured. `TestSamplingContract_TopPSemanticsAreEncoded`, `TestSamplingContract_CoversEveryFormat` |
| Silently-ignored parameters | **named and dropped, not tuned blind** | `ResolveSamplingParams` drops the requested subset from a thinking-mode body (all three formats) and `InertThinkingParams` names the documented set; the client logs every drop at WARN. `TestSamplingContract_ThinkingModeRequestShape`, `TestSamplingContract_DroppedIsSubsetOfInertSet`; the recorded request carrying all three was accepted (200) |
| Tool limits / refusals | **present** | `ValidateToolDefinitions` (charset, 128-character cap, uniqueness) and `ValidateToolChoice` (refuses `required`/named in thinking mode locally) |
| Beta surface (prefix completion, FIM) | **absent** | `/beta` base not used; both endpoints recorded working → `SHIM-SUPPORT-006` |

**Consequence to keep in view:** the "silently ignored parameters" and "CoT must be re-sent
when tools are present" rules are not pedantry. The day `tools` goes on the wire, a naive
implementation degrades silently — or, on the Anthropic surface, fails with an opaque 400 —
rather than failing loudly. That is the failure mode this file exists to prevent, and §D is
how it is prevented: every item below is a checked cell with an explicit state.

---

## D. Conformance matrix (executable contract)

The matrix is implemented in `internal/llm/shim_matrix.go` and executed by
`internal/llm/shim_support_matrix_test.go` against the recorded exchanges in
`internal/llm/testdata/shim-support-exchanges.json`. Provider documentation read on
**2026-10-13**.

Every cell has an **explicit state**:

- `supported` — implemented, and the cell's check proves it (against the client, and
  against a recorded live exchange where the surface allows).
- `not-supported` — deliberately absent, and the cell **names the board row** that closes
  it. A cell cannot be blank, and a state cannot be inferred from silence.
- `not-applicable` — cannot apply to our surface (e.g. a capability the provider excludes
  for that format), with the reason stated.

Evidence kinds: `live` (a verbatim recorded exchange backs it), `unit` (the client's own
behaviour is asserted), `doc` (the provider documentation says it; not observable from a
call). A cell claims only the evidence it has.

<!-- SHIM-MATRIX-BEGIN -->
| Cell | § | Item | Format | State | Row / reason |
|---|---|---|---|---|---|
| B1-OPENAI-CHAT | §B1 | OpenAI Chat Completions format at base https://api.deepseek.com (POST /chat/completions) | openai | supported | base carries no /v1; client appends /chat/completions |
| B1-ANTHROPIC-MSGS | §B1 | Anthropic Messages format at base https://api.deepseek.com/anthropic (x-api-key; anthropic-version ignored) | anthropic | supported | endpoint derived from base: /v1 base keeps SDK spelling, /anthropic base gets /v1/messages |
| B1-ANTHROPIC-STREAM | §B1 | Anthropic streaming protocol (stream:true → SSE message_start / content_block_delta / message_delta / message_stop assembled into the same turn the buffered call returns) | anthropic | supported | SHIM-SUPPORT-004 — streamed and buffered turns agree by construction; malformed streams fail loudly (TestAnthropicConformance_*) |
| B3-ANTHROPIC-STREAM-TOOLUSE | §B3 | Anthropic tool-use over the stream (tool_use assembled from input_json_delta fragments; tool_result echoed on the next streamed request) | anthropic | supported | SHIM-SUPPORT-004 — fragments replace the input placeholder and concatenate byte-exactly (TestAnthropicConformance_*) |
| B1-ANTHROPIC-MODELMAP | §B1 | Documented Anthropic model-name mapping (claude-opus* → deepseek-v4-pro; claude-haiku*/claude-sonnet* → deepseek-flash; unsupported → deepseek-flash) | anthropic | supported | provider-side mapping; client passes the name through, so the cost consequence is recorded |
| B1-RESPONSES-SHAPE | §B1 | Responses format request/response shape (POST /responses) | responses | supported | builder + interpreter pinned against a recorded exchange |
| B1-RESPONSES-WIRING | §B1 | The harness can select the Responses format | responses | not-supported | SHIM-SUPPORT-005 — NewClient has no provider case; the wire layer is unreachable |
| B1-BETA-PREFIX | §B1 | Beta surface: chat prefix completion (base /beta, last assistant message with prefix=true) | beta | not-supported | SHIM-SUPPORT-006 — no /beta base or prefix path; endpoint recorded working |
| B1-BETA-FIM | §B1 | Beta surface: FIM completion (POST /beta/completions with prompt + suffix) | beta | not-supported | SHIM-SUPPORT-006 — no FIM request path; endpoint recorded working |
| B2-OPENAI-TOGGLE | §B2 | Thinking toggle in the OpenAI spelling ({"thinking":{"type":…}}) | openai | supported | — |
| B2-OPENAI-EFFORT | §B2 | Effort control via reasoning_effort incl. the documented "none" off value | openai | supported | measured: effort "none" returns a turn with no reasoning_content |
| B2-ANTHROPIC-TOGGLE | §B2 | Thinking toggle in the Anthropic spelling (reasoning.effort none/low/high/max) | anthropic | supported | — |
| B2-ANTHROPIC-TOGGLE-EFFECT | §B2 | Documented effect of the Anthropic off spelling: effort "none" disables thinking | anthropic | not-supported | SHIM-SUPPORT-007 — the documented request is sent, but the live surface still returns thinking blocks |
| B2-ANTHROPIC-EFFORT | §B2 | Anthropic effort dial via output_config.effort | anthropic | supported | — |
| B2-RESPONSES-EFFORT | §B2 | Responses effort dial via reasoning.effort (the spelling the live surface honours and echoes) | responses | supported | — |
| B2-RESPONSES-OUTPUT-CONFIG | §B2 | Responses effort spelling recorded in this spec as output_config.effort | responses | not-supported | SHIM-SUPPORT-008 — provider ignores output_config; the spec's spelling for this format must be reconciled |
| B2-DEFAULT | §B2 | Documented defaults: thinking enabled, effort high; unconfigured client sends no dial | all | supported | measured: an unconfigured request still comes back with reasoning |
| B2-MAX-TOKENS | §B2 | max_tokens bounds: documented range 1–393216 refused client-side; unset legal (provider defaults 8K / 64K / 128K) | all | supported | bounds verified in the API reference 2026-10-13 |
| B2-EFFORT-MAP | §B2 | Documented effort mapping (minimal→low … ultra→max) | all | supported | — |
| B2-UNKNOWN-EFFORT | §B2 | A value outside the effort vocabulary is refused, never silently dropped | all | supported | — |
| B2-COT-TRANSPORT | §B2 | Chain-of-thought returned as reasoning_content next to content | openai | supported | — |
| B2-COT-ROUNDTRIP | §B2 | CoT re-sent for every prior turn when tools are present; omitted without tools | openai | supported | — |
| B2-COT-REQUIRED-ANTHROPIC | §B2 | Anthropic: an assistant turn carrying tool_use must echo its thinking block | anthropic | supported | measured both arms: missing block → 400 naming thinking; echoed with signature → 200 |
| B2-COT-PERSIST | §B2 | CoT and its signature are persisted with the turn and handed back | all | supported | turn snapshot iteration_commits.llm_response |
| B2-EFFORT-WITH-THINKING-OFF | §B2 | With thinking disabled no effort dial is sent | all | supported | — |
| B2-COT-AS-ANSWER | §B2 | CoT promotion is last-resort, WARN-logged, flagged and disableable | all | supported | — |
| B2-INERT-PARAMS | §B2 | Documented silently-ignored parameters are named, not tuned blind | all | supported | measured: all three sent in one request → HTTP 200, no error. The client DROPS them from a thinking-mode body and names each drop at WARN; a thinking-disabled request passes them through. Enforced by TestSamplingContract_ThinkingModeRequestShape / _CoversEveryFormat |
| B2-TOP-P | §B2 | top_p: thinking mode only, clamped to ≥0.95; ignored and fixed at 1.0 otherwise | all | supported | resolved client-side so the EFFECTIVE value reaches the wire: 0.5 → 0.95 thinking, fixed 1.0 non-thinking, nothing when unconfigured (the recorded top_p 0.5 request was accepted, 200). Enforced by TestSamplingContract_TopPSemanticsAreEncoded / _CoversEveryFormat |
| B3-TOOLS-SHAPE | §B3 | Tool declaration shape tools[].function.{name,description,parameters} | openai | supported | — |
| B3-TOOLS-ANTHROPIC-SHAPE | §B3 | Anthropic tool shape (name, description, input_schema) | anthropic | supported | — |
| B3-TOOL-LIMITS | §B3 | Documented tool limits: name charset, 128-character cap, unique names | all | supported | limits verified in the API reference 2026-10-13 |
| B3-TOOL-CHOICE | §B3 | tool_choice vocabulary: none / auto / required / named function, mapped per format | all | supported | required → Anthropic "any"; named → {type:tool/function,name} |
| B3-TOOL-CHOICE-THINKING | §B3 | required / named tool_choice refused in thinking mode, locally | openai | supported | measured provider 400 recorded; client refuses before sending |
| B3-RESPONSE-SHAPE-OPENAI | §B3 | Tool-call response shape: assistant tool_calls; results as role tool with tool_call_id | openai | supported | — |
| B3-RESPONSE-SHAPE-ANTHROPIC | §B3 | Anthropic tool blocks: tool_use in assistant turns, tool_result with tool_use_id | anthropic | supported | — |
| B3-MIDCOV-CHAT-COMPLETIONS | §B3 | Chat Completions mid-conversation insertion: system messages only, kept in place | openai | supported | — |
| B3-MIDCOV-INJECTED-CHAT-COMPLETIONS | §B3 | Injecting tool calls mid-conversation on Chat Completions | openai | not-applicable | provider excludes it for this format; carried by the Anthropic and Responses cells instead |
| B3-MIDCOV-ANTHROPIC | §B3 | Anthropic mid-conversation insertion: system messages and tool messages in place | anthropic | supported | measured: only the leading system run is hoisted into the system parameter |
| B3-MIDCOV-RESPONSES | §B3 | Responses mid-conversation insertion: system messages and tool-call items in place | responses | supported | — |
| B4-OPENAI-CACHE | §B4 | Chat Completions cache accounting (prompt_cache_hit/miss tokens) | openai | supported | measured: 0/260 → 126/134; prompt_tokens = hit + miss |
| B4-ANTHROPIC-CACHE | §B4 | Anthropic cache accounting (cache_read/cache_creation input tokens) | anthropic | supported | measured: a tool round trip read 128 cached input tokens |
| B4-RESPONSES-CACHE | §B4 | Responses cache accounting (usage.input_tokens_details.cached_tokens) | responses | supported | — |
<!-- SHIM-MATRIX-END -->

SHIM-SUPPORT-002 adds a focused behavioral matrix inside the executable
`ShimSupportMatrix`. These rows close the evidence gap between “the struct has a
field” and “the real request/persistence path behaves correctly.” The tests
replay `internal/llm/testdata/thinking-cot-turns.json` through `httptest`; the
persistence row additionally runs a complete harness iteration against a
temporary SQLite database and reads `iteration_commits.llm_response` back.
No matrix test calls a live provider. SHIM-SUPPORT-003 adds the
`SAMPLING-CONTRACT` row, which is what turns the §B2 sampling rules above from
documentation into behaviour: the scenario names the test, and
`TestDeepSeekConformanceMatrix_ProducesVerifiableScenarioTable` fails if that
test function does not exist or this table stops naming it.
SHIM-SUPPORT-004 adds the two Anthropic streaming/tool-use cells above
(`B1-ANTHROPIC-STREAM`, `B3-ANTHROPIC-STREAM-TOOLUSE`): the client consumes the
provider's SSE protocol (`stream:true` → message_start → content_block_delta… →
message_delta → message_stop) and assembles the same turn the buffered call
returns, and the tool-use contract (tools[].input_schema, tool_use blocks,
tool_result results) works over both the buffered and the streaming surface.
Their transcripts are protocol fixtures built to the documented event shapes,
not provider recordings — the cells carry `unit` evidence for exactly that
reason, and the tool-declaration shape stays backed by the recorded exchange
(`B3-TOOLS-ANTHROPIC-SHAPE`).

| Scenario | Recorded fixture | Expected request shape | Expected response handling | Executable test |
|---|---|---|---|---|
| TURN-PERSIST | `tools_absent.turn1.response` | recorded DeepSeek turn enters one real harness iteration | `reasoning_content` and `AgentOutput` fields coexist in the same persisted row | `TestDeepSeekConformanceMatrix_PersistsReasoningContentWithTurn` |
| TOOLS-RESEND | `tools_present.turn2` | tools present; every previous assistant `reasoning_content` is emitted verbatim | recorded continuation response is replayed after request capture | `TestDeepSeekConformanceMatrix_ResendsAllReasoningOnlyWithTools/tools_present_resends_every_prior_reasoning_turn` |
| NO-TOOLS-OMIT | `tools_present.turn2.response` | tools absent; no message carries `reasoning_content` | same recorded response; omission remains the only changed condition | `TestDeepSeekConformanceMatrix_ResendsAllReasoningOnlyWithTools/tools_absent_omits_every_prior_reasoning_turn` |
| THINKING-PER-REQUEST | `tools_absent.turn1.response` | successive requests emit enabled+low, disabled+no-effort, enabled+max | each response is handled under that request's explicit options | `TestDeepSeekConformanceMatrix_ThinkingControlIsExplicitPerRequest` |
| FALLBACK-LAST-RESORT | `tools_present.turn1.response` | recorded response has empty `content` and non-empty `reasoning_content` | promotion is WARN-logged and flagged; non-empty content bypasses it; disabling it preserves CoT without promotion | `TestDeepSeekConformanceMatrix_ReasoningFallbackIsLoggedFlaggedAndLastResort` |
| SAMPLING-CONTRACT | `openai.thinking_disabled_inert_params.response` | thinking ON drops `temperature`, `presence_penalty` and `frequency_penalty` and floors `top_p` to 0.95; thinking OFF passes all three through and sends the fixed `top_p` 1.0 | the recorded 200 exchange is replayed unchanged; every dropped parameter is named at WARN | `TestSamplingContract_CoversEveryFormat` |

`TestDeepSeekConformanceMatrix_ProducesVerifiableScenarioTable` prints this
scenario → request shape → response handling table in every verbose matrix run
and fails if any row is blank, duplicated, or missing.

Totals at 2026-10-13: **36 supported, 5 not-supported, 1 not-applicable (42 cells)**. The
five not-supported cells rest on four rows — `SHIM-SUPPORT-005` (Responses wiring),
`SHIM-SUPPORT-006` (beta surface), `SHIM-SUPPORT-007` (Anthropic off-switch divergence),
`SHIM-SUPPORT-008` (Responses effort-spelling divergence) — all filed from this row's report
and listed by the matrix run itself. SHIM-SUPPORT-004 added the two Anthropic
streaming/tool-use cells (36 supported) without closing any not-supported row.

---

## E. Refresh procedure (when the provider docs change)

The matrix is only true with its documentation date attached. Refreshing it is a procedure,
not a memory:

1. **Record the date.** Read the provider documentation for the pages listed in
   `ShimMatrix.ProviderDocs` —
   https://api-docs.deepseek.com/guides/thinking_mode,
   https://api-docs.deepseek.com/guides/tool_calls,
   https://api-docs.deepseek.com/guides/anthropic_api,
   https://api-docs.deepseek.com/guides/responses_api,
   https://api-docs.deepseek.com/guides/chat_prefix_completion,
   https://api-docs.deepseek.com/guides/fim_completion,
   https://api-docs.deepseek.com/api/create-chat-completion and
   https://api-docs.deepseek.com/api/create-completion. Update `ShimDocDate` in
   `internal/llm/shim_matrix.go` — the run fails if the spec and the constant disagree, so
   the date cannot be updated in one place.
2. **Re-measure the live surface** where a call is cheap. `internal/llm/testdata/shim-support-exchanges.json`
   records the exchanges, not a summary of them: each entry holds the request body that was
   POSTed, the URL it went to, the auth mode, the status and the provider's response bytes.
   Replace the entries whose behaviour changed, keep the refusals (a 400 is evidence), and
   update `recorded_at_first` / `recorded_at_last`. Existing rows marked `live` are only
   meaningful against the same recording.
3. **Re-derive the cells.** Add a cell for a new documented item, and change an existing
   cell's state when its behaviour changed. A `not-supported` cell must name a board row; a
   `not-applicable` cell must state its reason; a `supported` cell must have a check.
4. **Re-run the matrix** (from the repository root):

   ```
   go test ./internal/llm ./internal/harness -run 'TestShimSupportMatrix|TestShimMatrixSpecParity|TestDeepSeekConformanceMatrix' -v
   ```

   This runs every cell, prints the state table with its evidence and the rows still to be
   filed, re-checks the state invariants, re-checks the fixture's provenance (that it is a
   live recording and carries no credential material), and fails if this file's §D table and
   the executable matrix have drifted apart.
5. **Re-run the whole package** before committing: `go test ./internal/llm ./internal/harness`.

A provider divergence discovered by a refresh is recorded as a `not-supported` cell with a
row — not smoothed into prose — for the same reason §D exists: the next reader must be able
to tell "we do not do this yet" from "we never noticed".
