// Package tools: tests for the tool registry.
//
// axiom:trace work_item=runtime-harness-01 spec=specs/010-tools.md plan=phase-3/task-3-1/step-3-1-1 test=internal/tools/tools_test.go
package tools

import (
	"context"
	"strings"
	"testing"
)

// ============================================================================
// Tool Type Tests
// ============================================================================

func TestHemisphere_Constants(t *testing.T) {
	tests := []struct {
		h Hemisphere
		s string
	}{
		{HemisphereInternal, "internal"},
		{HemisphereExternal, "external"},
	}

	for _, tt := range tests {
		if string(tt.h) != tt.s {
			t.Errorf("expected hemisphere %q to serialize as %q, got %q", tt.h, tt.s, string(tt.h))
		}
	}
}

func TestHandlerType_Constants(t *testing.T) {
	types := []struct {
		h HandlerType
		s string
	}{
		{HandlerSQLFunction, "sql_function"},
		{HandlerHTTPEndpoint, "http_endpoint"},
		{HandlerGoNative, "go_native"},
		{HandlerSubprocess, "subprocess"},
	}

	for _, tt := range types {
		if string(tt.h) != tt.s {
			t.Errorf("expected handler_type %q to serialize as %q, got %q", tt.h, tt.s, string(tt.h))
		}
	}
}

// ============================================================================
// Tool Tests
// ============================================================================

func TestTool_Fields(t *testing.T) {
	sessionID := "sess-1"
	tool := &Tool{
		ID:               "tool-1",
		Name:             "web_scraper",
		Description:      "Scrapes a web page",
		Hemisphere:       HemisphereExternal,
		HandlerType:      HandlerSubprocess,
		HandlerRef:       "scrape.js",
		OwnerSessionID:   &sessionID,
		Status:           "active",
		Enabled:          true,
		RequiresApproval: true,
	}

	if tool.Name != "web_scraper" {
		t.Errorf("name = %q, want web_scraper", tool.Name)
	}
	if tool.Hemisphere != HemisphereExternal {
		t.Error("hemisphere should be external")
	}
	if !tool.RequiresApproval {
		t.Error("should require approval")
	}
}

func TestTool_Internal(t *testing.T) {
	tool := &Tool{
		Name:        "set_display_mode",
		Description: "Changes display state",
		Hemisphere:  HemisphereInternal,
		HandlerType: HandlerSQLFunction,
		HandlerRef:  "set_display_mode",
		Enabled:     true,
	}

	if tool.Hemisphere != HemisphereInternal {
		t.Error("internal tool hemisphere mismatch")
	}
	if tool.HandlerType != HandlerSQLFunction {
		t.Error("internal tool should have sql_function handler")
	}
}

// ============================================================================
// ToolResult Tests
// ============================================================================

func TestToolResult_Success(t *testing.T) {
	r := &ToolResult{
		RequestID:  42,
		Output:     "scraped content here",
		IsError:    false,
		TokenCount: 1200,
	}

	if r.IsError {
		t.Error("success result should not be marked as error")
	}
	if r.Output == "" {
		t.Error("output should not be empty")
	}
}

func TestToolResult_Error(t *testing.T) {
	r := &ToolResult{
		RequestID: 42,
		Output:    "timeout after 30s",
		IsError:   true,
		ErrorCode: "TIMEOUT",
	}

	if !r.IsError {
		t.Error("error result should be marked as error")
	}
	if r.ErrorCode != "TIMEOUT" {
		t.Errorf("error code = %q, want TIMEOUT", r.ErrorCode)
	}
}

// ============================================================================
// Helpers Tests
// ============================================================================

func TestToString(t *testing.T) {
	tests := []struct {
		input    any
		expected string
	}{
		{"hello", "hello"},
		{[]byte("world"), "world"},
		{nil, ""},
		{42, "42"},
		{3.14, "3.14"},
	}

	for _, tt := range tests {
		result := toString(tt.input)
		if result != tt.expected {
			t.Errorf("toString(%v) = %q, want %q", tt.input, result, tt.expected)
		}
	}
}

func TestToBool(t *testing.T) {
	tests := []struct {
		input    any
		expected bool
	}{
		{true, true},
		{false, false},
		{"true", true},
		{"1", true},
		{"false", false},
		{42, false},
		{nil, false},
	}

	for _, tt := range tests {
		result := toBool(tt.input)
		if result != tt.expected {
			t.Errorf("toBool(%v) = %v, want %v", tt.input, result, tt.expected)
		}
	}
}

func TestToInt(t *testing.T) {
	tests := []struct {
		input    any
		expected int
	}{
		{int64(10), 10},
		{float64(20.5), 20},
		{int(30), 30},
		{"not a number", 0},
		{nil, 0},
	}

	for _, tt := range tests {
		result := toInt(tt.input)
		if result != tt.expected {
			t.Errorf("toInt(%v) = %d, want %d", tt.input, result, tt.expected)
		}
	}
}

// ============================================================================
// Registry - Caching Tests
// ============================================================================

func TestRegistry_Cache(t *testing.T) {
	// Test cache behavior in isolation
	r := &Registry{
		database: nil,
		cache:    make(map[string]*Tool),
	}

	// Pre-populate cache
	cached := &Tool{Name: "test_tool", Description: "cached", Enabled: true}
	r.cache["test_tool"] = cached

	// Invalidate
	r.Invalidate("test_tool")
	if _, ok := r.cache["test_tool"]; ok {
		t.Error("cache should be invalidated")
	}

	// Re-cache
	r.cache["test_tool2"] = cached
	r.Invalidate("test_tool2")
	if _, ok := r.cache["test_tool2"]; ok {
		t.Error("cache should be empty after invalidation")
	}
}

func TestRegistry_NilDB(t *testing.T) {
	r := &Registry{
		database: nil,
		cache:    make(map[string]*Tool),
	}

	_, err := r.List(context.Background())
	if err == nil || !strings.Contains(err.Error(), "no database") {
		t.Errorf("expected 'no database' error, got %v", err)
	}

	_, err = r.queryTool(context.Background(), "nonexistent")
	if err == nil || !strings.Contains(err.Error(), "no database") {
		t.Errorf("expected 'no database' error, got %v", err)
	}
}

// ============================================================================
// Edge Cases
// ============================================================================

func TestTool_NilOwnerSessionID(t *testing.T) {
	tool := &Tool{
		Name:           "system_tool",
		OwnerSessionID: nil, // system tool — no owner
	}
	if tool.OwnerSessionID != nil {
		t.Error("system tool should have nil owner")
	}
}

func TestTool_RateLimit(t *testing.T) {
	limit := 60
	tool := &Tool{
		Name:            "rate_limited_tool",
		RateLimitPerMin: &limit,
	}
	if tool.RateLimitPerMin == nil {
		t.Fatal("rate limit should be set")
	}
	if *tool.RateLimitPerMin != 60 {
		t.Errorf("rate limit = %d, want 60", *tool.RateLimitPerMin)
	}
}

func TestToolResult_NoTokenCount(t *testing.T) {
	r := &ToolResult{
		RequestID: 1,
		Output:    "done",
	}
	if r.TokenCount != 0 {
		t.Error("token count should default to 0")
	}
}
