package h3

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// errTest is a sentinel-style error for injecting service failures.
type errTest string

func (e errTest) Error() string { return string(e) }

var _ error = errTest("")

// ============================================================================
// fakeSessionService — records calls, serves canned responses, injectable errors
// ============================================================================

type fakeCreateCall struct {
	agentName, goal, modelID, projectID string
	contextBudget                       int
}

type fakeProcessCall struct {
	sessionID string
	message   string
}

type fakeFeedCall struct {
	sessionID, toolName string
	success             bool
	data                any
}

type fakeSessionService struct {
	createCalls  []fakeCreateCall
	processCalls []fakeProcessCall
	feedCalls    []fakeFeedCall
	getCalls     []string

	createErr  error
	processErr error
	feedErr    error
	getErr     error

	sessionID   string
	processResp string
	feedResp    string
}

func newFakeSessionService() *fakeSessionService {
	return &fakeSessionService{
		sessionID:   "cs-sess-1",
		processResp: "thinking...",
		feedResp:    "continuing",
	}
}

func (f *fakeSessionService) CreateSession(ctx context.Context, agentName, goal, modelID, projectID string, contextBudget int) (string, string, error) {
	f.createCalls = append(f.createCalls, fakeCreateCall{agentName, goal, modelID, projectID, contextBudget})
	return f.sessionID, "active", f.createErr
}

func (f *fakeSessionService) GetSession(ctx context.Context, id string) (string, error) {
	f.getCalls = append(f.getCalls, id)
	return "active", f.getErr
}

func (f *fakeSessionService) ProcessMessage(ctx context.Context, sessionID, message string) (string, error) {
	f.processCalls = append(f.processCalls, fakeProcessCall{sessionID, message})
	return f.processResp, f.processErr
}

func (f *fakeSessionService) FeedToolResult(ctx context.Context, sessionID, toolName string, success bool, data any) (string, error) {
	f.feedCalls = append(f.feedCalls, fakeFeedCall{sessionID, toolName, success, data})
	return f.feedResp, f.feedErr
}

// ============================================================================
// Helpers
// ============================================================================

func newTestServer(f *fakeSessionService) *Server {
	// db.DB is never used by the server — nil is fine (verified: server.go
	// stores it but no handler touches s.db).
	return NewServer(nil, f)
}

// processBody builds a valid ProcessRequest JSON body.
func processBody(sessionID, userName, content string, maxIterations int, modelID string) string {
	req := ProcessRequest{
		SessionID: sessionID,
		Message:   Message{Role: "user", Content: content, Timestamp: "2026-08-14T00:00:00Z"},
		Identity:  Identity{Platform: "test", ChatID: "c1", UserName: userName, UserID: "u1"},
		Context: Context{
			Config: Config{MaxIterations: maxIterations, TimeoutSeconds: 60},
		},
	}
	if modelID != "" {
		req.Context.Models = []Model{{Name: modelID, Provider: "test", ContextWindow: 128000}}
	}
	b, err := json.Marshal(req)
	if err != nil {
		panic(err)
	}
	return string(b)
}

// resultBody builds a valid ResultRequest JSON body.
func resultBody(sessionID, toolName string, success bool, data any) string {
	req := ResultRequest{
		SessionID:  sessionID,
		DecisionID: "dec-1",
		Result: Result{
			Type:     "tool_result",
			ToolName: toolName,
			Data:     data,
			Success:  success,
		},
	}
	b, err := json.Marshal(req)
	if err != nil {
		panic(err)
	}
	return string(b)
}

func doPost(t *testing.T, h http.Handler, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func doGet(t *testing.T, h http.Handler, path string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func decodeDecision(t *testing.T, rec *httptest.ResponseRecorder) Decision {
	t.Helper()
	var d Decision
	if err := json.Unmarshal(rec.Body.Bytes(), &d); err != nil {
		t.Fatalf("decode decision response: %v (body=%s)", err, rec.Body.String())
	}
	return d
}

// ============================================================================
// 1. Route registration
// ============================================================================

func TestHealthGET(t *testing.T) {
	srv := newTestServer(newFakeSessionService())
	rec := doGet(t, srv.Handler(), "/v1/health")

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /v1/health = %d, want 200 (body=%s)", rec.Code, rec.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("health body not JSON: %v", err)
	}
	if body["status"] != "ok" {
		t.Errorf("health status = %v, want ok", body["status"])
	}
	if body["version"] != "1.0.0" {
		t.Errorf("health version = %v, want 1.0.0", body["version"])
	}
	if body["protocol_version"] != "1.0" {
		t.Errorf("health protocol_version = %v, want 1.0", body["protocol_version"])
	}
	if body["transport"] != "rest" {
		t.Errorf("health transport = %v, want rest", body["transport"])
	}
	caps, ok := body["capabilities"].([]any)
	if !ok || len(caps) == 0 {
		t.Errorf("health capabilities = %v, want non-empty array", body["capabilities"])
	}
}

func TestHealthMethodNotAllowed(t *testing.T) {
	srv := newTestServer(newFakeSessionService())
	rec := doPost(t, srv.Handler(), "/v1/health", `{}`)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("POST /v1/health = %d, want 405", rec.Code)
	}
}

// All four protocol routes must be registered — a non-404 response proves the
// handler is mounted (process/result reject bad bodies with 400, cancel 200s).
func TestAllRoutesRegistered(t *testing.T) {
	srv := newTestServer(newFakeSessionService())
	h := srv.Handler()

	cases := []struct {
		method, path, body string
	}{
		{http.MethodGet, "/v1/health", ""},
		{http.MethodPost, "/v1/process", `{`}, // malformed → 400, not 404
		{http.MethodPost, "/v1/result", `{}`}, // unknown session → 200 end/error, not 404
		{http.MethodPost, "/v1/cancel", `{}`}, // → 200 cancelled
	}
	for _, c := range cases {
		var rec *httptest.ResponseRecorder
		if c.method == http.MethodGet {
			rec = doGet(t, h, c.path)
		} else {
			rec = doPost(t, h, c.path, c.body)
		}
		if rec.Code == http.StatusNotFound {
			t.Errorf("%s %s = 404 — route not registered", c.method, c.path)
		}
	}
}

// ============================================================================
// 2. /v1/process request routing
// ============================================================================

func TestProcessTextDecision(t *testing.T) {
	f := newFakeSessionService()
	f.processResp = "hello from consensus"
	srv := newTestServer(f)

	rec := doPost(t, srv.Handler(), "/v1/process", processBody("h3-s1", "alice", "do the thing", 5, "deepseek-v4-flash"))

	if rec.Code != http.StatusOK {
		t.Fatalf("POST /v1/process = %d, want 200 (body=%s)", rec.Code, rec.Body.String())
	}
	d := decodeDecision(t, rec)
	if d.Decision != DecisionText {
		t.Fatalf("decision = %q, want text", d.Decision)
	}
	if d.Text == nil || d.Text.Content != "hello from consensus" {
		t.Errorf("text content = %+v, want echoed response", d.Text)
	}
	if d.Text != nil && d.Text.Finished {
		t.Errorf("text finished = true, want false (1 turn < max 5, no DONE)")
	}

	// CreateSession must be called once with the mapped arguments.
	if len(f.createCalls) != 1 {
		t.Fatalf("CreateSession calls = %d, want 1", len(f.createCalls))
	}
	c := f.createCalls[0]
	if c.agentName != "h3-alice" {
		t.Errorf("agentName = %q, want h3-alice", c.agentName)
	}
	if c.goal != "do the thing" {
		t.Errorf("goal = %q, want message content", c.goal)
	}
	if c.modelID != "deepseek-v4-flash" {
		t.Errorf("modelID = %q, want context model name", c.modelID)
	}
	if c.contextBudget != 200000 {
		t.Errorf("contextBudget = %d, want 200000", c.contextBudget)
	}
	if len(f.processCalls) != 1 || f.processCalls[0].message != "do the thing" {
		t.Errorf("ProcessMessage calls = %+v, want single call with content", f.processCalls)
	}
	if len(f.processCalls) == 1 && f.processCalls[0].sessionID != "cs-sess-1" {
		t.Errorf("ProcessMessage session = %q, want mapped consensus session", f.processCalls[0].sessionID)
	}
}

func TestProcessDefaultModelID(t *testing.T) {
	f := newFakeSessionService()
	srv := newTestServer(f)
	// No context.models → default "deepseek-v4-pro".
	rec := doPost(t, srv.Handler(), "/v1/process", processBody("h3-s2", "bob", "hi", 3, ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("POST /v1/process = %d (body=%s)", rec.Code, rec.Body.String())
	}
	if len(f.createCalls) != 1 || f.createCalls[0].modelID != "deepseek-v4-pro" {
		t.Errorf("modelID = %+v, want default deepseek-v4-pro", f.createCalls)
	}
}

func TestProcessReusesExistingSession(t *testing.T) {
	f := newFakeSessionService()
	srv := newTestServer(f)

	r1 := doPost(t, srv.Handler(), "/v1/process", processBody("h3-s1", "alice", "first", 5, ""))
	if r1.Code != http.StatusOK {
		t.Fatalf("first process = %d", r1.Code)
	}
	r2 := doPost(t, srv.Handler(), "/v1/process", processBody("h3-s1", "alice", "second", 5, ""))
	if r2.Code != http.StatusOK {
		t.Fatalf("second process = %d", r2.Code)
	}

	if len(f.createCalls) != 1 {
		t.Errorf("CreateSession calls = %d, want 1 (same h3 session must not re-create)", len(f.createCalls))
	}
	if len(f.processCalls) != 2 {
		t.Errorf("ProcessMessage calls = %d, want 2", len(f.processCalls))
	}
	// Both messages must hit the SAME consensus session.
	if f.processCalls[0].sessionID != f.processCalls[1].sessionID {
		t.Errorf("consensus sessions differ across messages: %q vs %q", f.processCalls[0].sessionID, f.processCalls[1].sessionID)
	}
}

func TestProcessInvalidJSON(t *testing.T) {
	srv := newTestServer(newFakeSessionService())
	rec := doPost(t, srv.Handler(), "/v1/process", `{"session_id": `)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("POST /v1/process (bad JSON) = %d, want 400", rec.Code)
	}
	assertErrorEnvelope(t, rec, "INVALID_REQUEST")
}

func TestProcessServiceError(t *testing.T) {
	f := newFakeSessionService()
	f.processErr = errTest("llm exploded")
	srv := newTestServer(f)

	rec := doPost(t, srv.Handler(), "/v1/process", processBody("h3-s1", "alice", "hi", 5, ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("POST /v1/process = %d, want 200 with end decision", rec.Code)
	}
	d := decodeDecision(t, rec)
	if d.Decision != DecisionEnd {
		t.Fatalf("decision = %q, want end", d.Decision)
	}
	if d.End == nil || d.End.Reason != EndError {
		t.Fatalf("end = %+v, want reason error", d.End)
	}
	if d.End != nil && !strings.Contains(d.End.Summary, "llm exploded") {
		t.Errorf("end summary = %q, want service error text", d.End.Summary)
	}
}

func TestProcessToolCallDecision(t *testing.T) {
	f := newFakeSessionService()
	f.processResp = `{"internal_monologue": "need current data", "tool_requests": [{"tool_name": "search", "parameters": {"q": "latest"}}]}`
	srv := newTestServer(f)

	rec := doPost(t, srv.Handler(), "/v1/process", processBody("h3-s1", "alice", "search for x", 5, ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("POST /v1/process = %d", rec.Code)
	}
	d := decodeDecision(t, rec)
	if d.Decision != DecisionToolCall {
		t.Fatalf("decision = %q, want tool_call", d.Decision)
	}
	if d.ToolCall == nil {
		t.Fatal("tool_call is nil")
	}
	if d.ToolCall.Name != "search" {
		t.Errorf("tool name = %q, want search", d.ToolCall.Name)
	}
	if d.ToolCall.Reasoning != "need current data" {
		t.Errorf("tool reasoning = %q, want internal_monologue", d.ToolCall.Reasoning)
	}
	params, ok := d.ToolCall.Params.(map[string]any)
	if !ok || params["q"] != "latest" {
		t.Errorf("tool params = %v, want {\"q\":\"latest\"}", d.ToolCall.Params)
	}
}

func TestProcessFinishedOnDONE(t *testing.T) {
	f := newFakeSessionService()
	f.processResp = "all done DONE"
	srv := newTestServer(f)

	rec := doPost(t, srv.Handler(), "/v1/process", processBody("h3-s1", "alice", "finish", 5, ""))
	d := decodeDecision(t, rec)
	if d.Decision != DecisionText {
		t.Fatalf("decision = %q, want text", d.Decision)
	}
	if d.Text == nil || !d.Text.Finished {
		t.Errorf("text = %+v, want finished=true when response contains DONE", d.Text)
	}
}

func TestProcessFinishedAtMaxIterations(t *testing.T) {
	f := newFakeSessionService()
	srv := newTestServer(f)

	// max_iterations=1 → after the first turn (turns=1), finished must be true.
	rec := doPost(t, srv.Handler(), "/v1/process", processBody("h3-s1", "alice", "go", 1, ""))
	d := decodeDecision(t, rec)
	if d.Decision != DecisionText {
		t.Fatalf("decision = %q, want text", d.Decision)
	}
	if d.Text == nil || !d.Text.Finished {
		t.Errorf("text = %+v, want finished=true when turns >= max_iterations", d.Text)
	}
}

// ============================================================================
// 3. /v1/result request routing
// ============================================================================

func TestResultUnknownSession(t *testing.T) {
	srv := newTestServer(newFakeSessionService())
	rec := doPost(t, srv.Handler(), "/v1/result", resultBody("no-such-session", "search", true, map[string]any{"q": "x"}))

	if rec.Code != http.StatusOK {
		t.Fatalf("POST /v1/result = %d, want 200 with end decision", rec.Code)
	}
	d := decodeDecision(t, rec)
	if d.Decision != DecisionEnd {
		t.Fatalf("decision = %q, want end", d.Decision)
	}
	if d.End == nil || d.End.Reason != EndError {
		t.Fatalf("end = %+v, want reason error", d.End)
	}
	if d.End != nil && !strings.Contains(d.End.Summary, "session not found: no-such-session") {
		t.Errorf("end summary = %q, want session not found message", d.End.Summary)
	}
}

func TestResultTextDecision(t *testing.T) {
	f := newFakeSessionService()
	f.feedResp = "tool result processed, continuing"
	srv := newTestServer(f)

	// Establish the session via /v1/process first.
	if rec := doPost(t, srv.Handler(), "/v1/process", processBody("h3-s1", "alice", "search for x", 5, "")); rec.Code != http.StatusOK {
		t.Fatalf("setup process = %d", rec.Code)
	}

	rec := doPost(t, srv.Handler(), "/v1/result", resultBody("h3-s1", "search", true, map[string]any{"results": []string{"a", "b"}}))
	if rec.Code != http.StatusOK {
		t.Fatalf("POST /v1/result = %d", rec.Code)
	}
	d := decodeDecision(t, rec)
	if d.Decision != DecisionText {
		t.Fatalf("decision = %q, want text", d.Decision)
	}
	if d.Text == nil || d.Text.Content != "tool result processed, continuing" {
		t.Errorf("text = %+v, want feed response", d.Text)
	}
	if d.Text != nil && d.Text.Finished {
		t.Errorf("text finished = true, want false")
	}

	if len(f.feedCalls) != 1 {
		t.Fatalf("FeedToolResult calls = %d, want 1", len(f.feedCalls))
	}
	fc := f.feedCalls[0]
	if fc.sessionID != "cs-sess-1" || fc.toolName != "search" || !fc.success {
		t.Errorf("feed call = %+v, want mapped session, tool name, success=true", fc)
	}
}

func TestResultTaskComplete(t *testing.T) {
	for _, resp := range []string{"DONE", "task COMPLETE", ""} {
		f := newFakeSessionService()
		f.feedResp = resp
		srv := newTestServer(f)
		if rec := doPost(t, srv.Handler(), "/v1/process", processBody("h3-s1", "alice", "go", 5, "")); rec.Code != http.StatusOK {
			t.Fatalf("setup process = %d", rec.Code)
		}
		rec := doPost(t, srv.Handler(), "/v1/result", resultBody("h3-s1", "search", true, map[string]any{}))
		d := decodeDecision(t, rec)
		if d.Decision != DecisionEnd {
			t.Errorf("response %q: decision = %q, want end", resp, d.Decision)
		}
		if d.End == nil || d.End.Reason != EndTaskComplete {
			t.Errorf("response %q: end = %+v, want task_complete", resp, d.End)
		}
	}
}

func TestResultToolCallDecision(t *testing.T) {
	f := newFakeSessionService()
	f.feedResp = `{"internal_monologue": "next step", "tool_requests": [{"tool_name": "query", "parameters": {"sql": "SELECT 1"}}]}`
	srv := newTestServer(f)

	if rec := doPost(t, srv.Handler(), "/v1/process", processBody("h3-s1", "alice", "go", 5, "")); rec.Code != http.StatusOK {
		t.Fatalf("setup process = %d", rec.Code)
	}
	rec := doPost(t, srv.Handler(), "/v1/result", resultBody("h3-s1", "search", true, map[string]any{}))
	d := decodeDecision(t, rec)
	if d.Decision != DecisionToolCall {
		t.Fatalf("decision = %q, want tool_call", d.Decision)
	}
	if d.ToolCall == nil || d.ToolCall.Name != "query" {
		t.Errorf("tool call = %+v, want name query", d.ToolCall)
	}
	if d.ToolCall.Reasoning != "next step" {
		t.Errorf("tool reasoning = %q, want internal_monologue", d.ToolCall.Reasoning)
	}
}

func TestResultFeedError(t *testing.T) {
	f := newFakeSessionService()
	f.feedErr = errTest("feed failed")
	srv := newTestServer(f)

	if rec := doPost(t, srv.Handler(), "/v1/process", processBody("h3-s1", "alice", "go", 5, "")); rec.Code != http.StatusOK {
		t.Fatalf("setup process = %d", rec.Code)
	}
	rec := doPost(t, srv.Handler(), "/v1/result", resultBody("h3-s1", "search", true, map[string]any{}))
	d := decodeDecision(t, rec)
	if d.Decision != DecisionEnd || d.End == nil || d.End.Reason != EndError {
		t.Fatalf("decision = %q end=%+v, want end/error", d.Decision, d.End)
	}
	if d.End != nil && !strings.Contains(d.End.Summary, "feed failed") {
		t.Errorf("end summary = %q, want feed error", d.End.Summary)
	}
}

func TestResultInvalidJSON(t *testing.T) {
	srv := newTestServer(newFakeSessionService())
	rec := doPost(t, srv.Handler(), "/v1/result", `{"session_id": `)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("POST /v1/result (bad JSON) = %d, want 400", rec.Code)
	}
	assertErrorEnvelope(t, rec, "INVALID_REQUEST")
}

// ============================================================================
// 4. Error mapping
// ============================================================================

func assertErrorEnvelope(t *testing.T, rec *httptest.ResponseRecorder, wantCode string) {
	t.Helper()
	var body struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("error body not JSON: %v (body=%s)", err, rec.Body.String())
	}
	if body.Error.Code != wantCode {
		t.Errorf("error code = %q, want %q", body.Error.Code, wantCode)
	}
	if body.Error.Message == "" {
		t.Errorf("error message empty, want detail")
	}
}

// ============================================================================
// 5. Smoke — full round trip across /v1/process → /v1/result
// ============================================================================

func TestSmokeProcessResultRoundTrip(t *testing.T) {
	f := newFakeSessionService()
	f.processResp = "searching..."
	f.feedResp = `{"tool_requests": [{"tool_name": "search", "parameters": {"q": "consensus"}}]}`
	srv := newTestServer(f)
	h := srv.Handler()

	// Phase 1: process — new session, text decision.
	rec := doPost(t, h, "/v1/process", processBody("h3-smoke", "alice", "find consensus docs", 3, ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("process = %d, want 200", rec.Code)
	}
	d1 := decodeDecision(t, rec)
	if d1.Decision != DecisionText || d1.Text == nil || d1.Text.Content != "searching..." {
		t.Fatalf("phase 1 decision = %+v, want text 'searching...'", d1)
	}
	if d1.Text.Finished {
		t.Fatal("phase 1 finished = true, want false")
	}

	// Phase 2: feed a tool result → agent requests another tool.
	rec = doPost(t, h, "/v1/result", resultBody("h3-smoke", "search", true, map[string]any{"hits": 3}))
	if rec.Code != http.StatusOK {
		t.Fatalf("result = %d, want 200", rec.Code)
	}
	d2 := decodeDecision(t, rec)
	if d2.Decision != DecisionToolCall || d2.ToolCall == nil || d2.ToolCall.Name != "search" {
		t.Fatalf("phase 2 decision = %+v, want tool_call 'search'", d2)
	}

	// Phase 3: feed the second tool result → agent says DONE.
	f.feedResp = "wrapped up DONE"
	rec = doPost(t, h, "/v1/result", resultBody("h3-smoke", "search", true, map[string]any{"hits": 3}))
	if rec.Code != http.StatusOK {
		t.Fatalf("result = %d, want 200", rec.Code)
	}
	d3 := decodeDecision(t, rec)
	if d3.Decision != DecisionEnd || d3.End == nil || d3.End.Reason != EndTaskComplete {
		t.Fatalf("phase 3 decision = %+v, want end/task_complete", d3)
	}

	// The H3 session map must persist across all requests: exactly one
	// CreateSession, and the same consensus session for every call.
	if len(f.createCalls) != 1 {
		t.Errorf("CreateSession calls = %d, want 1 (session must persist across requests)", len(f.createCalls))
	}
	if len(f.feedCalls) != 2 {
		t.Errorf("FeedToolResult calls = %d, want 2", len(f.feedCalls))
	}
	if len(f.processCalls) == 1 && len(f.feedCalls) == 2 {
		if f.feedCalls[0].sessionID != f.processCalls[0].sessionID || f.feedCalls[1].sessionID != f.processCalls[0].sessionID {
			t.Errorf("consensus session mismatch across requests: process=%q feed=%q,%q",
				f.processCalls[0].sessionID, f.feedCalls[0].sessionID, f.feedCalls[1].sessionID)
		}
	}
}
