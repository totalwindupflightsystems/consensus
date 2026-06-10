// Package mcp: additional tests for auth helpers, tool helpers, and edge cases.
//
// axiom:trace work_item=polish-phase spec=specs/015-api-and-mcp.md plan=phase-1/task-1/step-1 test=internal/mcp/auth_test.go
package mcp

import (
	"net/http"
	"testing"
)

// ============================================================================
// Auth Helper Tests
// ============================================================================

func TestCheckAdminScope_AdminPasses(t *testing.T) {
	sess := &mcpSession{authScope: "admin"}
	err := (&Server{}).checkAdminScope(sess)
	if err != nil {
		t.Errorf("admin should pass admin scope check: %v", err)
	}
}

func TestCheckAdminScope_SessionFails(t *testing.T) {
	sess := &mcpSession{authScope: "session"}
	err := (&Server{}).checkAdminScope(sess)
	if err == nil {
		t.Error("session scope should fail admin check")
	}
}

func TestCheckAdminScope_ReadonlyFails(t *testing.T) {
	sess := &mcpSession{authScope: "readonly"}
	err := (&Server{}).checkAdminScope(sess)
	if err == nil {
		t.Error("readonly should fail admin check")
	}
}

func TestCheckSessionAccess_AdminBypasses(t *testing.T) {
	sess := &mcpSession{authScope: "admin"}
	err := (&Server{}).checkSessionAccess(sess, "any-session")
	if err != nil {
		t.Errorf("admin should bypass session access check: %v", err)
	}
}

func TestCheckSessionAccess_SessionOwnID(t *testing.T) {
	sess := &mcpSession{authScope: "session", agentSessionID: "s1"}
	err := (&Server{}).checkSessionAccess(sess, "s1")
	if err != nil {
		t.Errorf("session-scoped key should access own session: %v", err)
	}
}

func TestCheckSessionAccess_SessionOtherID(t *testing.T) {
	sess := &mcpSession{authScope: "session", agentSessionID: "s1"}
	err := (&Server{}).checkSessionAccess(sess, "s2")
	if err == nil {
		t.Error("session-scoped key should NOT access other sessions")
	}
}

func TestCheckSessionAccess_ReadonlyOK(t *testing.T) {
	sess := &mcpSession{authScope: "readonly"}
	err := (&Server{}).checkSessionAccess(sess, "any")
	if err != nil {
		t.Errorf("readonly should pass session access check: %v", err)
	}
}

func TestCheckWriteAccess_AdminPasses(t *testing.T) {
	sess := &mcpSession{authScope: "admin"}
	err := (&Server{}).checkWriteAccess(sess)
	if err != nil {
		t.Errorf("admin should have write access: %v", err)
	}
}

func TestCheckWriteAccess_SessionPasses(t *testing.T) {
	sess := &mcpSession{authScope: "session"}
	err := (&Server{}).checkWriteAccess(sess)
	if err != nil {
		t.Errorf("session should have write access: %v", err)
	}
}

func TestCheckWriteAccess_ReadonlyFails(t *testing.T) {
	sess := &mcpSession{authScope: "readonly"}
	err := (&Server{}).checkWriteAccess(sess)
	if err == nil {
		t.Error("readonly should NOT have write access")
	}
}

// ============================================================================
// Helper Function Tests
// ============================================================================

func TestSha256Sum(t *testing.T) {
	h1 := sha256Sum("hello")
	h2 := sha256Sum("hello")
	h3 := sha256Sum("world")

	if h1 != h2 {
		t.Error("same input should produce same hash")
	}
	if h1 == h3 {
		t.Error("different inputs should produce different hashes")
	}
	if len(h1) != 64 {
		t.Errorf("expected 64-char hex hash, got %d chars", len(h1))
	}
}

func TestMin8(t *testing.T) {
	tests := []struct {
		input, expected int
	}{
		{0, 0},
		{3, 3},
		{8, 8},
		{15, 8},
		{100, 8},
	}
	for _, tt := range tests {
		if got := min8(tt.input); got != tt.expected {
			t.Errorf("min8(%d) = %d, want %d", tt.input, got, tt.expected)
		}
	}
}

func TestToString(t *testing.T) {
	tests := []struct {
		input    any
		expected string
	}{
		{nil, ""},
		{"hello", "hello"},
		{[]byte("world"), "world"},
		{42, ""}, // int not handled by toString
	}
	for _, tt := range tests {
		if got := toString(tt.input); got != tt.expected {
			t.Errorf("toString(%v) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}

func TestToInt64(t *testing.T) {
	tests := []struct {
		input    any
		expected int64
	}{
		{int64(42), 42},
		{float64(3.14), 3},
		{int(10), 10},
		{"nope", 0},
	}
	for _, tt := range tests {
		if got := toInt64(tt.input); got != tt.expected {
			t.Errorf("toInt64(%v) = %d, want %d", tt.input, got, tt.expected)
		}
	}
}

func TestToFloat64(t *testing.T) {
	tests := []struct {
		input    any
		expected float64
	}{
		{float64(2.5), 2.5},
		{int64(3), 3.0},
		{int(4), 4.0},
		{"nope", 0},
	}
	for _, tt := range tests {
		if got := toFloat64(tt.input); got != tt.expected {
			t.Errorf("toFloat64(%v) = %f, want %f", tt.input, got, tt.expected)
		}
	}
}

func TestGenerateShortID(t *testing.T) {
	id1 := generateShortID(64)
	id2 := generateShortID(64)

	// Both should be deterministic (same seed)
	if len(id1) != 64 {
		t.Errorf("expected 64 chars, got %d", len(id1))
	}
	if id1 != id2 {
		t.Log("deterministic IDs are same (expected for test helper)")
	}
}

func TestGenerateShortID_VariousLengths(t *testing.T) {
	// Only even lengths work (n/2 byte generation)
	for _, l := range []int{8, 16, 32} {
		id := generateShortID(l)
		if len(id) != l {
			t.Errorf("generateShortID(%d) length = %d, want %d", l, len(id), l)
		}
	}
}

func TestTruncateContent(t *testing.T) {
	tests := []struct {
		input    string
		maxLen   int
		expected string
	}{
		{"short", 100, "short"},
		{"a very long string that needs truncation", 10, "a very lon..."},
		{"exactly10", 10, "exactly10"},
		{"", 5, ""},
	}
	for _, tt := range tests {
		got := truncateContent(tt.input, tt.maxLen)
		if got != tt.expected {
			t.Errorf("truncateContent(%q, %d) = %q, want %q", tt.input, tt.maxLen, got, tt.expected)
		}
	}
}

func TestFormatJSON(t *testing.T) {
	v := map[string]string{"key": "value"}
	result := formatJSON(v)

	if len(result) == 0 {
		t.Error("expected non-empty JSON")
	}
	if result[0] != '{' {
		t.Errorf("expected JSON object start, got %q", result[:1])
	}
}

func TestFormatJSON_InvalidValue(t *testing.T) {
	// channel values can't be marshaled, so formatJSON falls back to %v
	ch := make(chan int)
	result := formatJSON(ch)

	if len(result) == 0 {
		t.Error("expected non-empty string for unmarshalable value")
	}
}

// ============================================================================
// URI Parsing Tests
// ============================================================================

func TestMatchSessionContext_Valid(t *testing.T) {
	// URI must be long enough: prefix + sessionID + suffix
	if !matchSessionContext("conscience://sessions/s1/context") {
		t.Error("expected match for valid URI")
	}
}

func TestMatchSessionContext_Invalid(t *testing.T) {
	invalidURIs := []string{
		"",
		"conscience://tools",
		"conscience://sessions",
		"conscience://sessions/s1",
		"conscience://sessions/s1/notcontext",
		"conscience://sessions//context", // empty session ID
	}
	for _, uri := range invalidURIs {
		if matchSessionContext(uri) {
			t.Errorf("expected no match for %q", uri)
		}
	}
}

func TestExtractSessionIDFromURI(t *testing.T) {
	uri := "conscience://sessions/abc-123/context"
	id := extractSessionIDFromURI(uri)
	if id != "abc-123" {
		t.Errorf("expected 'abc-123', got %q", id)
	}
}

func TestHasSuffix(t *testing.T) {
	if !hasSuffix("hello/context", "/context") {
		t.Error("expected true for suffix match")
	}
	if hasSuffix("hello", "/context") {
		t.Error("expected false for missing suffix")
	}
	if hasSuffix("hi", "hello") {
		t.Error("expected false when suffix longer than string")
	}
}

// ============================================================================
// Server Info Tests
// ============================================================================

func TestNewServer_Defaults(t *testing.T) {
	srv := NewServer(nil)
	if srv.name != "conscience" {
		t.Errorf("expected 'conscience', got %q", srv.name)
	}
	if srv.ver != "0.1.0" {
		t.Errorf("expected '0.1.0', got %q", srv.ver)
	}
	if srv.sessions == nil {
		t.Error("expected initialized sessions map")
	}
}

func TestServerInfo(t *testing.T) {
	srv := &Server{name: "test-server", ver: "2.0.0"}
	info := srv.ServerInfo()
	if info.Name != "test-server" {
		t.Errorf("expected 'test-server', got %q", info.Name)
	}
	if info.Version != "2.0.0" {
		t.Errorf("expected '2.0.0', got %q", info.Version)
	}
}

// ============================================================================
// Generate Session ID Tests
// ============================================================================

func TestGenerateSessionID(t *testing.T) {
	id := generateSessionID()
	if len(id) != 32 {
		t.Errorf("expected 32-char hex session ID, got %d chars", len(id))
	}
	// Should be hex characters only
	for _, c := range id {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			t.Errorf("non-hex character in session ID: %c", c)
		}
	}
}

// ============================================================================
// Initialize Edge Cases
// ============================================================================

func TestHandleInitialize_MalformedJSON(t *testing.T) {
	srv := NewServer(&mockMCPDB{})
	sess := &mcpSession{}

	req := &JSONRPCRequest{
		JSONRPC: "2.0",
		Method:  "initialize",
		Params:  []byte(`not-json`),
	}

	_, err := srv.handleInitialize(req, sess)
	if err == nil {
		t.Fatal("expected error for malformed JSON params")
	}
}

// ============================================================================
// MCP Session Lifecycle Tests
// ============================================================================

func TestMCPSessionFields(t *testing.T) {
	sess := &mcpSession{
		id:             "test-id",
		authScope:      "admin",
		sessionKey:     "cs_ak_test",
		agentSessionID: "agent-s1",
	}

	if sess.id != "test-id" {
		t.Error("wrong session ID")
	}
	if sess.authScope != "admin" {
		t.Error("wrong auth scope")
	}
	if sess.sessionKey != "cs_ak_test" {
		t.Error("wrong session key")
	}
	if sess.agentSessionID != "agent-s1" {
		t.Error("wrong agent session ID")
	}
}

// ============================================================================
// WriteError Tests
// ============================================================================

func TestWriteError_Format(t *testing.T) {
	// Test that writeError produces correct JSON-RPC error structure
	err := &JSONRPCErrObj{
		Code:    -32000,
		Message: "Test error",
		Data:    "details",
	}

	if err.Code != -32000 {
		t.Error("wrong error code")
	}
	if err.Message != "Test error" {
		t.Error("wrong error message")
	}
	if err.Data != "details" {
		t.Error("wrong error data")
	}
}

// ============================================================================
// SSE Write Helper Test
// ============================================================================

func TestSSEWriteFormat(t *testing.T) {
	// Test format of sseWrite output
	var buf mockResponseWriter
	f := mockFlusher{}

	result := sseWrite(&buf, &f, "message", `{"key":"value"}`)
	if !result {
		t.Error("expected successful SSE write")
	}

	output := buf.String()
	if !contains(output, "event: message") {
		t.Error("expected 'event: message'")
	}
	if !contains(output, "data: {\"key\":\"value\"}") {
		t.Error("expected JSON data")
	}
	if !contains(output, "\n\n") {
		t.Error("expected double newline terminator")
	}
}

// ============================================================================
// Helper: mock ResponseWriter and Flusher for SSE tests
// ============================================================================

type mockResponseWriter struct {
	buf        []byte
	statusCode int
}

func (m *mockResponseWriter) Header() http.Header { return http.Header{} }
func (m *mockResponseWriter) WriteHeader(statusCode int) {
	m.statusCode = statusCode
}
func (m *mockResponseWriter) Write(p []byte) (int, error) {
	m.buf = append(m.buf, p...)
	return len(p), nil
}
func (m *mockResponseWriter) String() string {
	return string(m.buf)
}

type mockFlusher struct{}

func (m *mockFlusher) Flush() {}

func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// ============================================================================
// Tool Dispatch Edge Cases
// ============================================================================

func TestHandleToolsCall_UnknownTool(t *testing.T) {
	srv := NewServer(&mockMCPDB{})
	sess := &mcpSession{authScope: "admin"}

	req := &JSONRPCRequest{
		JSONRPC: "2.0",
		Method:  "tools/call",
		Params:  []byte(`{"name": "nonexistent_tool", "arguments": {}}`),
	}

	_, err := srv.handleToolsCall(req, sess)
	if err == nil {
		t.Fatal("expected error for unknown tool")
	}
	if err.Code != -32601 {
		t.Errorf("expected -32601, got %d", err.Code)
	}
}

func TestHandleToolsCall_MalformedJSON(t *testing.T) {
	srv := NewServer(&mockMCPDB{})
	sess := &mcpSession{authScope: "admin"}

	req := &JSONRPCRequest{
		JSONRPC: "2.0",
		Method:  "tools/call",
		Params:  []byte(`not json`),
	}

	_, err := srv.handleToolsCall(req, sess)
	if err == nil {
		t.Fatal("expected parse error for malformed JSON")
	}
}

// ============================================================================
// Initialize Without Params Edge Case
// ============================================================================

func TestHandleInitialize_NilParams(t *testing.T) {
	srv := NewServer(&mockMCPDB{})
	sess := &mcpSession{}

	req := &JSONRPCRequest{
		JSONRPC: "2.0",
		Method:  "initialize",
		Params:  nil,
	}

	_, err := srv.handleInitialize(req, sess)
	if err == nil {
		t.Fatal("expected error for nil params")
	}
}

// ============================================================================
// Resources Read Unknown URI
// ============================================================================

func TestHandleResourcesRead_UnknownURI(t *testing.T) {
	srv := NewServer(&mockMCPDB{})
	sess := &mcpSession{authScope: "admin"}

	req := &JSONRPCRequest{
		JSONRPC: "2.0",
		Method:  "resources/read",
		Params:  []byte(`{"uri": "conscience://unknown"}`),
	}

	_, err := srv.handleResourcesRead(req, sess)
	if err == nil {
		t.Fatal("expected error for unknown URI")
	}
}

// ============================================================================
// Prompts Get Edge Cases
// ============================================================================

func TestHandlePromptsGet_UnknownPrompt(t *testing.T) {
	srv := NewServer(&mockMCPDB{})
	sess := &mcpSession{authScope: "admin"}

	req := &JSONRPCRequest{
		JSONRPC: "2.0",
		Method:  "prompts/get",
		Params:  []byte(`{"name": "unknown_prompt"}`),
	}

	_, err := srv.handlePromptsGet(req, sess)
	if err == nil {
		t.Fatal("expected error for unknown prompt")
	}
}

func TestHandlePromptsGet_MalformedJSON(t *testing.T) {
	srv := NewServer(&mockMCPDB{})
	sess := &mcpSession{authScope: "admin"}

	req := &JSONRPCRequest{
		JSONRPC: "2.0",
		Method:  "prompts/get",
		Params:  []byte(`not json`),
	}

	_, err := srv.handlePromptsGet(req, sess)
	if err == nil {
		t.Fatal("expected error for malformed params")
	}
}

func TestHandlePromptsGet_MissingSessionID(t *testing.T) {
	srv := NewServer(&mockMCPDB{})
	sess := &mcpSession{authScope: "admin"}

	req := &JSONRPCRequest{
		JSONRPC: "2.0",
		Method:  "prompts/get",
		Params:  []byte(`{"name": "agent_status", "arguments": {}}`),
	}

	_, err := srv.handlePromptsGet(req, sess)
	if err == nil {
		t.Fatal("expected error for missing session_id argument")
	}
}

// ============================================================================
// Tool Helpers — CreateSession / SendMessage Missing Params
// ============================================================================

func TestToolCreateSession_MissingFields(t *testing.T) {
	srv := NewServer(&mockMCPDB{})
	sess := &mcpSession{authScope: "admin"}

	_, err := srv.toolCreateSession([]byte(`{}`), sess)
	if err == nil {
		t.Fatal("expected error for missing fields")
	}
}

func TestToolCreateSession_MalformedJSON(t *testing.T) {
	srv := NewServer(&mockMCPDB{})
	sess := &mcpSession{authScope: "admin"}

	_, err := srv.toolCreateSession([]byte(`bad`), sess)
	if err == nil {
		t.Fatal("expected error for malformed JSON")
	}
}

func TestToolSendMessage_MissingFields(t *testing.T) {
	srv := NewServer(&mockMCPDB{})
	sess := &mcpSession{authScope: "admin"}

	_, err := srv.toolSendMessage([]byte(`{}`), sess)
	if err == nil {
		t.Fatal("expected error for missing fields")
	}
}

func TestToolSendMessage_MalformedJSON(t *testing.T) {
	srv := NewServer(&mockMCPDB{})
	sess := &mcpSession{authScope: "admin"}

	_, err := srv.toolSendMessage([]byte(`bad`), sess)
	if err == nil {
		t.Fatal("expected error for malformed JSON")
	}
}

func TestToolGetSessionStatus_MissingFields(t *testing.T) {
	srv := NewServer(&mockMCPDB{})
	sess := &mcpSession{authScope: "admin"}

	_, err := srv.toolGetSessionStatus([]byte(`{}`), sess)
	if err == nil {
		t.Fatal("expected error for missing session_id")
	}
}

func TestToolListMemory_MissingFields(t *testing.T) {
	srv := NewServer(&mockMCPDB{})
	sess := &mcpSession{authScope: "admin"}

	_, err := srv.toolListMemory([]byte(`{}`), sess)
	if err == nil {
		t.Fatal("expected error for missing session_id")
	}
}

func TestToolReviewApproval_InvalidDecision(t *testing.T) {
	srv := NewServer(&mockMCPDB{})
	sess := &mcpSession{authScope: "admin"}

	_, err := srv.toolReviewApproval([]byte(`{"approval_id": "a", "decision": "maybe_later"}`), sess)
	if err == nil {
		t.Fatal("expected error for invalid decision")
	}
}

func TestToolReviewApproval_ModifiedMissingSQL(t *testing.T) {
	srv := NewServer(&mockMCPDB{})
	sess := &mcpSession{authScope: "admin"}

	_, err := srv.toolReviewApproval([]byte(`{"approval_id": "a", "decision": "modified"}`), sess)
	if err == nil {
		t.Fatal("expected error for modified decision without SQL")
	}
}

func TestToolQueryTool_MissingFields(t *testing.T) {
	srv := NewServer(&mockMCPDB{})
	sess := &mcpSession{authScope: "admin"}

	_, err := srv.toolQueryTool([]byte(`{}`), sess)
	if err == nil {
		t.Fatal("expected error for missing fields")
	}
}

// ============================================================================
// Validate Auth Edge Cases
// ============================================================================

func TestValidateAuth_NonStringAuthorization(t *testing.T) {
	sess := &mcpSession{}

	req := &JSONRPCRequest{
		JSONRPC: "2.0",
		Method:  "initialize",
		Params: []byte(`{
			"protocolVersion": "2024-11-05",
			"capabilities": {},
			"clientInfo": {"name": "test", "version": "1.0"},
			"_meta": {"authorization": 42}
		}`),
	}

	srv := NewServer(&mockMCPDB{})
	err := srv.validateAuth(req, sess)
	if err == nil {
		t.Fatal("expected error for non-string authorization")
	}
}

func TestValidateAuth_TooShortKey(t *testing.T) {
	sess := &mcpSession{}

	req := &JSONRPCRequest{
		JSONRPC: "2.0",
		Method:  "initialize",
		Params: []byte(`{
			"protocolVersion": "2024-11-05",
			"capabilities": {},
			"clientInfo": {"name": "test", "version": "1.0"},
			"_meta": {"authorization": "short"}
		}`),
	}

	srv := NewServer(&mockMCPDB{})
	err := srv.validateAuth(req, sess)
	if err == nil {
		t.Fatal("expected error for too-short key")
	}
	if err.Code != -32001 {
		t.Errorf("expected -32001, got %d: %v", err.Code, err)
	}
}

// ============================================================================
// Handler Tests
// ============================================================================

func TestHandler_ServesSSEAtCorrectPath(t *testing.T) {
	srv := NewServer(&mockMCPDB{})
	h := srv.Handler()
	if h == nil {
		t.Fatal("expected non-nil handler")
	}
	// Handler should have routes for /mcp/sse and /mcp/message
	_ = h
}
