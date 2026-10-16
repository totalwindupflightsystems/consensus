// Package llm: the DeepSeek harness-API conformance matrix (spec 024 §B/§D).
//
// The matrix is the contract in executable form. Every item the provider
// documents for the harness API is one cell, and every cell carries an EXPLICIT
// state, so a gap is a recorded decision with a reason instead of an absence:
//
//	supported          — the client implements it; the cell's check proves it
//	                     against the client and, where the surface allows, against
//	                     a recorded live exchange.
//	not-supported      — the documented capability is absent. The cell names the
//	                     board row that tracks closing it; a row id is mandatory.
//	not-applicable     — the item cannot apply to our surface (for example a
//	                     capability the provider excludes for that format). The
//	                     cell states why; a reason is mandatory.
//
// Cells are checked by internal/llm/shim_support_matrix_test.go, which fails if a
// cell has no check, if a check has no cell, or if a state is malformed. The
// rows are re-derived from the provider docs by the refresh procedure in spec 024
// §E; the date the docs were read is recorded here and mirrored in the spec.
//
// axiom:trace work_item=SHIM-SUPPORT-001 spec=specs/024-shim-support-matrix.md impl=internal/llm/shim_matrix.go
package llm

import "regexp"

// ShimCellState is the explicit state of one conformance-matrix cell.
type ShimCellState string

const (
	// CellSupported: implemented and checked.
	CellSupported ShimCellState = "supported"
	// CellNotSupported: absent, tracked by the named board row.
	CellNotSupported ShimCellState = "not-supported"
	// CellNotApplicable: cannot apply, with the reason stated.
	CellNotApplicable ShimCellState = "not-applicable"
)

// Evidence kinds a cell may cite. A cell claims only what it has: `live` means a
// verbatim provider exchange in testdata/shim-support-exchanges.json backs it,
// `unit` means the client's own behaviour is asserted, `doc` means the claim
// rests on the provider documentation and was not observable from a call.
const (
	EvidenceLive = "live"
	EvidenceUnit = "unit"
	EvidenceDoc  = "doc"
)

// ShimRowPattern is the board-row id shape a not-supported cell must name.
var ShimRowPattern = regexp.MustCompile(`^SHIM-SUPPORT-[0-9]{3}$`)

// ShimCell is one row of the conformance matrix.
type ShimCell struct {
	// ID is stable and must appear verbatim in specs/024 §D.
	ID string
	// Section is the spec section the item comes from (§B1, §B2, §B3, §B4).
	Section string
	// Item is the documented capability, phrased as the thing being supported.
	Item string
	// Format names the wire format(s) the item belongs to.
	Format string
	// State is the explicit state — never empty.
	State ShimCellState
	// Row names the board row that closes a not-supported cell.
	Row string
	// Reason states why a cell is not-supported or not-applicable.
	Reason string
	// Evidence lists how the cell is known (EvidenceLive/Unit/Doc).
	Evidence []string
	// Doc cites the provider documentation the item comes from.
	Doc string
	// Note carries a measurement or caveat worth keeping next to the state.
	Note string
}

// ShimBehaviorScenario is one executable, fixture-driven behavior row within
// the broader capability matrix. It records not just whether a capability is
// supported, but the exact request shape and response handling the named test
// proves. The focused table exists because a supported cell plus a helper-level
// assertion is not sufficient evidence for a cross-layer behavior claim.
type ShimBehaviorScenario struct {
	ID                       string
	Fixture                  string
	ExpectedRequestShape     string
	ExpectedResponseHandling string
	Test                     string
}

// ShimMatrix is the whole matrix plus the provenance that keeps it honest.
type ShimMatrix struct {
	// DocDate is the date the provider docs were read for this revision of the
	// matrix. A refresh with a stale date is a stale matrix.
	DocDate string
	// ProviderDocs are the doc pages the cells were derived from.
	ProviderDocs []string
	// RecordedFixture is the recorded-exchange file backing `live` cells.
	RecordedFixture string
	// RefreshCommand re-runs the matrix and its spec-parity check.
	RefreshCommand string
	// BehaviorScenarios is the focused SHIM-SUPPORT-002 request/response
	// conformance table. The test run prints it verbatim after executing the
	// named tests against recorded fixtures and httptest transports.
	BehaviorScenarios []ShimBehaviorScenario
	Cells             []ShimCell
}

// ShimDocDate is the date the provider documentation was read for this matrix.
const ShimDocDate = "2026-10-13"

// shimProviderDocs are the pages the cells were derived from (spec 024 §B).
var shimProviderDocs = []string{
	"https://api-docs.deepseek.com/guides/thinking_mode",
	"https://api-docs.deepseek.com/guides/tool_calls",
	"https://api-docs.deepseek.com/guides/anthropic_api",
	"https://api-docs.deepseek.com/guides/responses_api",
	"https://api-docs.deepseek.com/guides/chat_prefix_completion",
	"https://api-docs.deepseek.com/guides/fim_completion",
	"https://api-docs.deepseek.com/api/create-chat-completion",
	"https://api-docs.deepseek.com/api/create-completion",
}

// ShimSupportMatrix returns the conformance matrix for specs/024 §B.
func ShimSupportMatrix() ShimMatrix {
	return ShimMatrix{
		DocDate:         ShimDocDate,
		ProviderDocs:    shimProviderDocs,
		RecordedFixture: "testdata/shim-support-exchanges.json",
		RefreshCommand:  `go test ./internal/llm ./internal/harness -run 'TestShimSupportMatrix|TestShimMatrixSpecParity|TestDeepSeekConformanceMatrix' -v`,
		BehaviorScenarios: []ShimBehaviorScenario{
			{
				ID:                       "TURN-PERSIST",
				Fixture:                  "testdata/thinking-cot-turns.json#tools_absent.turn1.response",
				ExpectedRequestShape:     "recorded DeepSeek turn enters one real harness iteration",
				ExpectedResponseHandling: "reasoning_content and AgentOutput fields coexist in the same iteration_commits.llm_response row",
				Test:                     "TestDeepSeekConformanceMatrix_PersistsReasoningContentWithTurn",
			},
			{
				ID:                       "TOOLS-RESEND",
				Fixture:                  "testdata/thinking-cot-turns.json#tools_present.turn2",
				ExpectedRequestShape:     "tools present; every previous assistant reasoning_content is emitted verbatim",
				ExpectedResponseHandling: "recorded continuation response is parsed after the captured request is inspected",
				Test:                     "TestDeepSeekConformanceMatrix_ResendsAllReasoningOnlyWithTools/tools_present_resends_every_prior_reasoning_turn",
			},
			{
				ID:                       "NO-TOOLS-OMIT",
				Fixture:                  "testdata/thinking-cot-turns.json#tools_present.turn2.response",
				ExpectedRequestShape:     "tools absent; no message carries reasoning_content",
				ExpectedResponseHandling: "the same recorded continuation response is parsed without changing the omission rule",
				Test:                     "TestDeepSeekConformanceMatrix_ResendsAllReasoningOnlyWithTools/tools_absent_omits_every_prior_reasoning_turn",
			},
			{
				ID:                       "THINKING-PER-REQUEST",
				Fixture:                  "testdata/thinking-cot-turns.json#tools_absent.turn1.response",
				ExpectedRequestShape:     "successive requests explicitly emit enabled+low, disabled+no-effort, enabled+max",
				ExpectedResponseHandling: "each recorded response is handled under the options of its own request, not provider defaults",
				Test:                     "TestDeepSeekConformanceMatrix_ThinkingControlIsExplicitPerRequest",
			},
			{
				ID:                       "FALLBACK-LAST-RESORT",
				Fixture:                  "testdata/thinking-cot-turns.json#tools_present.turn1.response",
				ExpectedRequestShape:     "recorded response has empty content and non-empty reasoning_content",
				ExpectedResponseHandling: "promotion is WARN-logged and flagged; non-empty content bypasses it; disabling it preserves CoT without promotion",
				Test:                     "TestDeepSeekConformanceMatrix_ReasoningFallbackIsLoggedFlaggedAndLastResort",
			},
			{
				ID:      "SAMPLING-CONTRACT",
				Fixture: "testdata/shim-support-exchanges.json#openai.thinking_disabled_inert_params.response",
				ExpectedRequestShape: "thinking ON drops temperature, presence_penalty and frequency_penalty and floors top_p at 0.95; " +
					"thinking OFF passes all three through unchanged and sends the fixed top_p 1.0 (the caller's value is never forwarded there)",
				ExpectedResponseHandling: "the recorded 200 exchange is replayed unchanged; every dropped parameter is named at WARN",
				Test:                     "TestSamplingContract_CoversEveryFormat",
			},
		},
		Cells: []ShimCell{
			// ---- §B1 formats -------------------------------------------------
			{
				ID: "B1-OPENAI-CHAT", Section: "§B1", Format: "openai",
				Item:     "OpenAI Chat Completions request format at base https://api.deepseek.com (POST /chat/completions)",
				State:    CellSupported,
				Evidence: []string{EvidenceLive, EvidenceUnit},
				Doc:      "api/create-chat-completion",
				Note:     "base URL carries no /v1 segment; the client appends /chat/completions",
			},
			{
				ID: "B1-ANTHROPIC-MSGS", Section: "§B1", Format: "anthropic",
				Item:     "Anthropic Messages format at base https://api.deepseek.com/anthropic (POST /anthropic/v1/messages, x-api-key, anthropic-version ignored)",
				State:    CellSupported,
				Evidence: []string{EvidenceLive, EvidenceUnit},
				Doc:      "guides/anthropic_api",
				Note:     "endpoint path is derived from the base: a /v1 base keeps the SDK spelling, an /anthropic base gets /v1/messages",
			},
			{
				ID: "B1-ANTHROPIC-STREAM", Section: "§B1", Format: "anthropic",
				Item:     "Anthropic streaming protocol (stream:true → SSE message_start / content_block_start / content_block_delta / content_block_stop / message_delta / message_stop, assembled into the same turn the buffered call returns)",
				State:    CellSupported,
				Evidence: []string{EvidenceUnit},
				Doc:      "docs.anthropic.com/messages-streaming (protocol shapes); no DeepSeek recorded exchange carries SSE frames",
				Note:     "SHIM-SUPPORT-004 — assembly is delegated to the same buildAnthropicResponse the buffered path uses, so a streamed and a buffered turn agree by construction; TestAnthropicConformance_StreamedAndBufferedTurnsAgree fails if a parallel builder is reintroduced. Malformed streams (missing message_start/message_stop, frame without data, unknown event, delta before its block) fail loudly, never partially assemble — TestAnthropicConformance_MalformedStreamFailsLoudly",
			},
			{
				ID: "B3-ANTHROPIC-STREAM-TOOLUSE", Section: "§B3", Format: "anthropic",
				Item:     "Anthropic tool-use over the stream (tool_use block assembled from input_json_delta partial_json fragments; tool result echoed as tool_result on the next streamed request)",
				State:    CellSupported,
				Evidence: []string{EvidenceUnit, EvidenceLive},
				Doc:      "docs.anthropic.com/messages-streaming (input_json_delta)",
				Note:     "SHIM-SUPPORT-004 — the fragments replace the opener's input {} placeholder and concatenate byte-exactly (TestAnthropicConformance_StreamThinkingAndToolUseAssemblesToolCall); the full tool loop runs over the stream in TestAnthropicConformance_ToolUseRoundTripOverStream. The declaration shape itself is live-recorded (B3-TOOLS-ANTHROPIC-SHAPE); the streaming transcripts are protocol fixtures, hence the unit evidence",
			},
			{
				ID: "B1-ANTHROPIC-MODELMAP", Section: "§B1", Format: "anthropic",
				Item:     "Documented Anthropic model-name mapping (claude-opus* → deepseek-v4-pro, claude-haiku*/claude-sonnet* → deepseek-flash, unsupported → deepseek-flash)",
				State:    CellSupported,
				Evidence: []string{EvidenceDoc},
				Doc:      "guides/anthropic_api",
				Note:     "mapping happens provider-side, so the client passes the requested name through; the cost consequence is real (a claude-opus* name is billed as V4 Pro), which is why the mapping is recorded here rather than assumed away",
			},
			{
				ID: "B1-RESPONSES-SHAPE", Section: "§B1", Format: "responses",
				Item:     "Responses format request/response shape at base https://api.deepseek.com (POST /responses)",
				State:    CellSupported,
				Evidence: []string{EvidenceLive, EvidenceUnit},
				Doc:      "guides/responses_api",
				Note:     "builder and interpreter implemented and pinned against a recorded exchange; transport/provider selection is tracked separately (B1-RESPONSES-WIRING)",
			},
			{
				ID: "B1-RESPONSES-WIRING", Section: "§B1", Format: "responses",
				Item:     "The harness can select the Responses format (provider wiring, not just the wire shape)",
				State:    CellNotSupported,
				Row:      "SHIM-SUPPORT-005",
				Reason:   "llm.NewClient has no provider case that constructs a Responses client and Config has no way to choose it, so the format's builder/interpreter cannot be reached from the harness yet",
				Evidence: []string{EvidenceUnit},
				Doc:      "guides/responses_api",
			},
			{
				ID: "B1-BETA-PREFIX", Section: "§B1", Format: "beta",
				Item:     "Beta surface: chat prefix completion (base https://api.deepseek.com/beta, last assistant message with prefix=true)",
				State:    CellNotSupported,
				Row:      "SHIM-SUPPORT-006",
				Reason:   "the client never uses the /beta base and has no prefix-completion path; the provider accepts it (recorded 200) but nothing in the harness can ask for it",
				Evidence: []string{EvidenceLive},
				Doc:      "guides/chat_prefix_completion",
			},
			{
				ID: "B1-BETA-FIM", Section: "§B1", Format: "beta",
				Item:     "Beta surface: FIM completion (POST /beta/completions with prompt + suffix)",
				State:    CellNotSupported,
				Row:      "SHIM-SUPPORT-006",
				Reason:   "no FIM request path exists and no harness caller needs one yet; recorded 200 exchange kept as evidence the endpoint works",
				Evidence: []string{EvidenceLive},
				Doc:      "guides/fim_completion",
			},

			// ---- §B2 thinking mode -------------------------------------------
			{
				ID: "B2-OPENAI-TOGGLE", Section: "§B2", Format: "openai",
				Item:     "Thinking toggle in the OpenAI spelling ({\"thinking\":{\"type\":\"enabled\"|\"disabled\"}})",
				State:    CellSupported,
				Evidence: []string{EvidenceLive, EvidenceUnit},
				Doc:      "guides/thinking_mode",
			},
			{
				ID: "B2-OPENAI-EFFORT", Section: "§B2", Format: "openai",
				Item:     "Effort control via reasoning_effort, including the documented \"none\" value that disables thinking",
				State:    CellSupported,
				Evidence: []string{EvidenceLive, EvidenceUnit},
				Doc:      "api/create-chat-completion",
				Note:     "measured: reasoning_effort \"none\" returns a turn with no reasoning_content",
			},
			{
				ID: "B2-ANTHROPIC-TOGGLE", Section: "§B2", Format: "anthropic",
				Item:     "Thinking toggle in the Anthropic spelling ({\"reasoning\":{\"effort\":\"none\"|\"low\"|\"high\"|\"max\"}})",
				State:    CellSupported,
				Evidence: []string{EvidenceLive, EvidenceUnit},
				Doc:      "guides/thinking_mode",
			},
			{
				ID: "B2-ANTHROPIC-TOGGLE-EFFECT", Section: "§B2", Format: "anthropic",
				Item:     "Documented effect of the Anthropic off spelling: effort \"none\" disables thinking mode",
				State:    CellNotSupported,
				Row:      "SHIM-SUPPORT-007",
				Reason:   "the documented request is sent, but the live surface still returns thinking blocks for effort \"none\" (measured 2026-10-13), so the documented effect does not hold provider-side and no client change can make it hold",
				Evidence: []string{EvidenceLive},
				Doc:      "guides/thinking_mode",
				Note:     "divergence is provider-side; the client sends the documented spelling and does not claim the effect, and the matrix keeps the measurement so nobody tunes a toggle that does nothing",
			},
			{
				ID: "B2-ANTHROPIC-EFFORT", Section: "§B2", Format: "anthropic",
				Item:     "Anthropic effort dial via output_config.effort",
				State:    CellSupported,
				Evidence: []string{EvidenceLive, EvidenceUnit},
				Doc:      "guides/anthropic_api",
			},
			{
				ID: "B2-RESPONSES-EFFORT", Section: "§B2", Format: "responses",
				Item:     "Responses effort dial via reasoning.effort (the spelling the live surface honours and echoes)",
				State:    CellSupported,
				Evidence: []string{EvidenceLive, EvidenceUnit},
				Doc:      "guides/responses_api",
			},
			{
				ID: "B2-RESPONSES-OUTPUT-CONFIG", Section: "§B2", Format: "responses",
				Item:     "Responses effort spelling recorded in this spec as output_config.effort",
				State:    CellNotSupported,
				Row:      "SHIM-SUPPORT-008",
				Reason:   "the provider ignores output_config on the Responses surface (measured: no echo, no error, while reasoning.effort was echoed back), so the spec's spelling for this format is wrong; the builder sends reasoning.effort and the spec row must be reconciled",
				Evidence: []string{EvidenceLive},
				Doc:      "guides/responses_api",
				Note:     "measured 2026-10-13 with both spellings in one request: reasoning echo = max, responses body carried no output_config",
			},
			{
				ID: "B2-DEFAULT", Section: "§B2", Format: "all",
				Item:     "Documented defaults: thinking enabled, effort high (and an unconfigured client sends no dial at all)",
				State:    CellSupported,
				Evidence: []string{EvidenceLive, EvidenceUnit},
				Doc:      "guides/thinking_mode",
				Note:     "measured: a request that configures nothing still comes back with reasoning, i.e. thinking is on by default",
			},
			{
				ID: "B2-MAX-TOKENS", Section: "§B2", Format: "all",
				Item:     "max_tokens bounds: the documented range (1–393216) is refused client-side when violated, while unset stays legal (provider defaults 8K non-thinking / 64K thinking / 128K at effort max)",
				State:    CellSupported,
				Evidence: []string{EvidenceDoc, EvidenceUnit},
				Doc:      "api/create-chat-completion",
				Note:     "bounds verified in the API reference on 2026-10-13; a violating value is refused before the request is sent",
			},
			{
				ID: "B2-EFFORT-MAP", Section: "§B2", Format: "all",
				Item:     "Documented effort mapping (minimal→low, low→low, medium→high, high→high, xhigh→high, max→max, ultra→max)",
				State:    CellSupported,
				Evidence: []string{EvidenceUnit},
				Doc:      "guides/thinking_mode",
			},
			{
				ID: "B2-UNKNOWN-EFFORT", Section: "§B2", Format: "all",
				Item:     "A value outside the effort vocabulary is refused, never silently dropped",
				State:    CellSupported,
				Evidence: []string{EvidenceUnit},
				Doc:      "guides/thinking_mode",
			},
			{
				ID: "B2-COT-TRANSPORT", Section: "§B2", Format: "openai",
				Item:     "Chain-of-thought transport: returned as reasoning_content at the same level as content",
				State:    CellSupported,
				Evidence: []string{EvidenceLive, EvidenceUnit},
				Doc:      "guides/thinking_mode",
			},
			{
				ID: "B2-COT-ROUNDTRIP", Section: "§B2", Format: "openai",
				Item:     "CoT round-trip: with tools every prior turn's reasoning_content is re-sent; without tools it is omitted",
				State:    CellSupported,
				Evidence: []string{EvidenceLive, EvidenceUnit},
				Doc:      "guides/thinking_mode",
			},
			{
				ID: "B2-COT-REQUIRED-ANTHROPIC", Section: "§B2", Format: "anthropic",
				Item:     "Anthropic surface: a prior assistant turn that carries tool_use must echo its thinking block, or the request is refused",
				State:    CellSupported,
				Evidence: []string{EvidenceLive, EvidenceUnit},
				Doc:      "guides/anthropic_api",
				Note:     "measured both arms: missing block → HTTP 400 naming the thinking content; block echoed (with signature) → HTTP 200",
			},
			{
				ID: "B2-COT-PERSIST", Section: "§B2", Format: "all",
				Item:     "CoT (and its signature) is persisted with the turn and handed back on the turn object, never dropped after being read",
				State:    CellSupported,
				Evidence: []string{EvidenceUnit},
				Doc:      "spec 024 §B2",
				Note:     "turn snapshot iteration_commits.llm_response carries reasoning_content + reasoning_signature",
			},
			{
				ID: "B2-EFFORT-WITH-THINKING-OFF", Section: "§B2", Format: "all",
				Item:     "With thinking explicitly disabled, no effort dial is sent (the dial is meaningless with thinking off)",
				State:    CellSupported,
				Evidence: []string{EvidenceUnit},
				Doc:      "spec 024 §B2",
			},
			{
				ID: "B2-COT-AS-ANSWER", Section: "§B2", Format: "all",
				Item:     "Promoting CoT into content is a last-resort, WARN-logged, response-flagged and disableable substitution",
				State:    CellSupported,
				Evidence: []string{EvidenceLive, EvidenceUnit},
				Doc:      "spec 024 §B2",
			},
			{
				ID: "B2-INERT-PARAMS", Section: "§B2", Format: "all",
				Item:     "Documented silently-ignored parameters (temperature, presence_penalty, frequency_penalty) are named, not tuned blind",
				State:    CellSupported,
				Evidence: []string{EvidenceLive, EvidenceUnit},
				Doc:      "guides/thinking_mode",
				Note: "measured: all three sent in one request → HTTP 200, no error. The client DROPS them from a thinking-mode body " +
					"(sending them there implies an effect they do not have) and names each dropped parameter at WARN; a thinking-disabled " +
					"request passes them through. Enforced by TestSamplingContract_ThinkingModeRequestShape and _CoversEveryFormat",
			},
			{
				ID: "B2-TOP-P", Section: "§B2", Format: "all",
				Item:     "top_p semantics: thinking mode only, clamped to ≥0.95; fixed at 1.0 and ignored in non-thinking mode",
				State:    CellSupported,
				Evidence: []string{EvidenceDoc, EvidenceUnit},
				Doc:      "guides/thinking_mode",
				Note: "resolved client-side so the EFFECTIVE value is what reaches the wire: 0.5 → 0.95 in thinking mode, the fixed 1.0 " +
					"in non-thinking mode, nothing at all when unconfigured (the recorded request carrying top_p 0.5 was accepted, 200). " +
					"Enforced by TestSamplingContract_TopPSemanticsAreEncoded and _CoversEveryFormat",
			},

			// ---- §B3 tool calls ----------------------------------------------
			{
				ID: "B3-TOOLS-SHAPE", Section: "§B3", Format: "openai",
				Item:     "Tool declaration shape tools[].function.{name,description,parameters}",
				State:    CellSupported,
				Evidence: []string{EvidenceLive, EvidenceUnit},
				Doc:      "guides/tool_calls",
			},
			{
				ID: "B3-TOOLS-ANTHROPIC-SHAPE", Section: "§B3", Format: "anthropic",
				Item:     "Anthropic tool shape (name, description, input_schema)",
				State:    CellSupported,
				Evidence: []string{EvidenceLive, EvidenceUnit},
				Doc:      "guides/anthropic_api",
			},
			{
				ID: "B3-TOOL-LIMITS", Section: "§B3", Format: "all",
				Item:     "Documented tool limits: name charset, 128-character name cap, unique names",
				State:    CellSupported,
				Evidence: []string{EvidenceDoc, EvidenceUnit},
				Doc:      "api/create-chat-completion",
				Note:     "limits verified in the API reference on 2026-10-13 rather than trusted from this spec's paraphrase",
			},
			{
				ID: "B3-TOOL-CHOICE", Section: "§B3", Format: "all",
				Item:     "tool_choice vocabulary: none / auto / required / named function, mapped per format",
				State:    CellSupported,
				Evidence: []string{EvidenceLive, EvidenceUnit},
				Doc:      "api/create-chat-completion",
				Note:     "required maps to the Anthropic \"any\" spelling; named choices become {\"type\":\"tool\",\"name\":…} / {\"type\":\"function\",\"name\":…}",
			},
			{
				ID: "B3-TOOL-CHOICE-THINKING", Section: "§B3", Format: "openai",
				Item:     "required / named tool_choice is refused in thinking mode, locally rather than by a provider 400",
				State:    CellSupported,
				Evidence: []string{EvidenceLive, EvidenceUnit},
				Doc:      "api/create-chat-completion",
				Note:     "measured 400 recorded as openai.tool_choice_required_thinking_400; the client now refuses before sending",
			},
			{
				ID: "B3-RESPONSE-SHAPE-OPENAI", Section: "§B3", Format: "openai",
				Item:     "Tool-call response shape: assistant tool_calls, results returned as role tool with tool_call_id",
				State:    CellSupported,
				Evidence: []string{EvidenceLive, EvidenceUnit},
				Doc:      "guides/tool_calls",
			},
			{
				ID: "B3-RESPONSE-SHAPE-ANTHROPIC", Section: "§B3", Format: "anthropic",
				Item:     "Anthropic tool blocks: tool_use in assistant turns, tool_result (with tool_use_id) in user turns",
				State:    CellSupported,
				Evidence: []string{EvidenceLive, EvidenceUnit},
				Doc:      "guides/anthropic_api",
			},
			{
				ID: "B3-MIDCOV-CHAT-COMPLETIONS", Section: "§B3", Format: "openai",
				Item:     "Chat Completions mid-conversation insertion: system messages only, kept in place",
				State:    CellSupported,
				Evidence: []string{EvidenceDoc, EvidenceUnit},
				Doc:      "guides/tool_calls",
			},
			{
				ID: "B3-MIDCOV-INJECTED-CHAT-COMPLETIONS", Section: "§B3", Format: "openai",
				Item:     "Injecting tool calls mid-conversation on Chat Completions",
				State:    CellNotApplicable,
				Reason:   "the provider excludes this for the Chat Completions format and directs callers to the Anthropic or Responses surface; our harness must therefore never attempt it here, and the capability is carried by B3-MIDCOV-ANTHROPIC / B3-MIDCOV-RESPONSES instead",
				Evidence: []string{EvidenceDoc},
				Doc:      "guides/tool_calls",
			},
			{
				ID: "B3-MIDCOV-ANTHROPIC", Section: "§B3", Format: "anthropic",
				Item:     "Anthropic mid-conversation insertion: system messages and tool-call messages, in the caller's positions",
				State:    CellSupported,
				Evidence: []string{EvidenceLive, EvidenceUnit},
				Doc:      "guides/tool_calls",
				Note:     "measured: a mid-conversation system message is accepted; only the LEADING system run is hoisted into the system parameter",
			},
			{
				ID: "B3-MIDCOV-RESPONSES", Section: "§B3", Format: "responses",
				Item:     "Responses mid-conversation insertion: system messages and tool-call items, in place",
				State:    CellSupported,
				Evidence: []string{EvidenceDoc, EvidenceUnit},
				Doc:      "guides/tool_calls",
			},

			// ---- §B4 accounting ----------------------------------------------
			{
				ID: "B4-OPENAI-CACHE", Section: "§B4", Format: "openai",
				Item:     "Cache accounting on the Chat Completions surface (prompt_cache_hit_tokens / prompt_cache_miss_tokens, mirrored by prompt_tokens_details.cached_tokens)",
				State:    CellSupported,
				Evidence: []string{EvidenceLive, EvidenceUnit},
				Doc:      "api/create-chat-completion",
				Note:     "measured: identical 260-token prompt went from 0 hit / 260 miss to 126 hit / 134 miss, and prompt_tokens = hit + miss",
			},
			{
				ID: "B4-ANTHROPIC-CACHE", Section: "§B4", Format: "anthropic",
				Item:     "Cache accounting on the Anthropic surface (cache_read_input_tokens / cache_creation_input_tokens)",
				State:    CellSupported,
				Evidence: []string{EvidenceLive, EvidenceUnit},
				Doc:      "guides/anthropic_api",
				Note:     "measured: a tool-round-trip request read 128 cached input tokens",
			},
			{
				ID: "B4-RESPONSES-CACHE", Section: "§B4", Format: "responses",
				Item:     "Cache accounting on the Responses surface (usage.input_tokens_details.cached_tokens)",
				State:    CellSupported,
				Evidence: []string{EvidenceLive, EvidenceUnit},
				Doc:      "guides/responses_api",
			},
		},
	}
}
