// Package memory: tests for context formatting, page resolution, and display modes.
//
// axiom:trace work_item=polish-phase spec=specs/002-memory.md plan=phase-1/task-1/step-1 test=internal/memory/memory_test.go
package memory

import (
	"strings"
	"testing"
	"time"
)

// ============================================================================
// Page Resolution Tests
// ============================================================================

func TestResolvePageMemoryIDs_Empty(t *testing.T) {
	ids := ResolvePageMemoryIDs(nil)
	if len(ids) != 0 {
		t.Errorf("expected empty set from nil pages, got %d entries", len(ids))
	}
}

func TestResolvePageMemoryIDs_SinglePage(t *testing.T) {
	pages := []MemoryPage{
		{ID: 1, Name: "page1", TargetIDs: []int64{10, 20, 30}},
	}
	ids := ResolvePageMemoryIDs(pages)

	if len(ids) != 3 {
		t.Fatalf("expected 3 IDs, got %d", len(ids))
	}
	for _, want := range []int64{10, 20, 30} {
		if !ids[want] {
			t.Errorf("expected ID %d in set", want)
		}
	}
}

func TestResolvePageMemoryIDs_Deduplication(t *testing.T) {
	pages := []MemoryPage{
		{ID: 1, Name: "page1", TargetIDs: []int64{10, 20}},
		{ID: 2, Name: "page2", TargetIDs: []int64{20, 30}},
	}
	ids := ResolvePageMemoryIDs(pages)

	if len(ids) != 3 {
		t.Errorf("expected 3 deduplicated IDs (10, 20, 30), got %d: %v", len(ids), ids)
	}
	if !ids[20] {
		t.Error("expected shared ID 20 to be present")
	}
}

func TestResolvePageMemoryIDs_LinkedPages(t *testing.T) {
	pages := []MemoryPage{
		{ID: 1, Name: "parent", TargetIDs: []int64{10}, LinkedPageIDs: []int64{2}},
		{ID: 2, Name: "child", TargetIDs: []int64{20, 30}},
	}
	ids := ResolvePageMemoryIDs(pages)

	if len(ids) != 3 {
		t.Errorf("expected 3 IDs (10, 20, 30), got %d: %v", len(ids), ids)
	}
	for _, want := range []int64{10, 20, 30} {
		if !ids[want] {
			t.Errorf("expected ID %d in resolved set", want)
		}
	}
}

func TestResolvePageMemoryIDs_NestedLinkedPages(t *testing.T) {
	// Linked_page_ids only resolve one level deep (SPEC-002)
	pages := []MemoryPage{
		{ID: 1, Name: "grandparent", TargetIDs: []int64{1}, LinkedPageIDs: []int64{2}},
		{ID: 2, Name: "parent", TargetIDs: []int64{10}, LinkedPageIDs: []int64{3}},
		{ID: 3, Name: "child", TargetIDs: []int64{100}},
	}
	ids := ResolvePageMemoryIDs(pages)

	// Should get: 1 (grandparent direct), 10 (parent direct, via grandparent link), 100 (child direct)
	// But NOT resolved via nested linking (parent's link to child is not followed from grandparent)
	if len(ids) != 3 {
		t.Errorf("expected 3 IDs (1, 10, 100), got %d", len(ids))
	}
	// Note: the child(3)'s targets are resolved by parent(2)'s link, since parent is in the page set
	for _, want := range []int64{1, 10, 100} {
		if !ids[want] {
			t.Errorf("expected ID %d in set", want)
		}
	}
}

func TestResolvePageMemoryIDs_SkipsZeroIDs(t *testing.T) {
	pages := []MemoryPage{
		{ID: 1, Name: "page1", TargetIDs: []int64{0, 10, 0}},
	}
	ids := ResolvePageMemoryIDs(pages)

	if len(ids) != 1 || !ids[10] {
		t.Errorf("expected only ID 10, got %v", ids)
	}
}

func TestResolvePageMemoryIDs_LinkedPageNotFound(t *testing.T) {
	pages := []MemoryPage{
		{ID: 1, Name: "page1", TargetIDs: []int64{10}, LinkedPageIDs: []int64{999}},
	}
	ids := ResolvePageMemoryIDs(pages)

	if len(ids) != 1 || !ids[10] {
		t.Errorf("expected only ID 10 (linked page not found), got %v", ids)
	}
}

// ============================================================================
// Display Mode Tests
// ============================================================================

func TestResolveDisplayText_Full(t *testing.T) {
	evt := MemoryEvent{DisplayMode: "full", Content: "hello world", SummaryText: "summary"}
	text := ResolveDisplayText(evt)
	if text != "hello world" {
		t.Errorf("expected 'hello world', got %q", text)
	}
}

func TestResolveDisplayText_Compressed(t *testing.T) {
	evt := MemoryEvent{DisplayMode: "compressed", Content: "long content", SummaryText: "short summary"}
	text := ResolveDisplayText(evt)
	if text != "short summary" {
		t.Errorf("expected 'short summary', got %q", text)
	}
}

func TestResolveDisplayText_CompressedNoSummary(t *testing.T) {
	evt := MemoryEvent{DisplayMode: "compressed", Content: "long content", SummaryText: ""}
	text := ResolveDisplayText(evt)
	if text != "long content" {
		t.Errorf("expected fallback to content, got %q", text)
	}
}

func TestResolveDisplayText_Hidden(t *testing.T) {
	evt := MemoryEvent{DisplayMode: "hidden", Content: "secret", SummaryText: "summary"}
	text := ResolveDisplayText(evt)
	if text != "" {
		t.Errorf("expected empty from hidden, got %q", text)
	}
}

func TestResolveDisplayText_UnknownMode(t *testing.T) {
	evt := MemoryEvent{DisplayMode: "unknown", Content: "hello", SummaryText: "summary"}
	text := ResolveDisplayText(evt)
	if text != "hello" {
		t.Errorf("expected content for unknown mode, got %q", text)
	}
}

func TestResolveDisplayText_EmptyMode(t *testing.T) {
	evt := MemoryEvent{DisplayMode: "", Content: "hello", SummaryText: "summary"}
	text := ResolveDisplayText(evt)
	if text != "hello" {
		t.Errorf("expected content for empty mode, got %q", text)
	}
}

func TestIsVisible(t *testing.T) {
	tests := []struct {
		mode    string
		visible bool
	}{
		{"full", true},
		{"compressed", true},
		{"hidden", false},
		{"", true},
		{"unknown", true},
	}
	for _, tt := range tests {
		if got := IsVisible(tt.mode); got != tt.visible {
			t.Errorf("IsVisible(%q) = %v, want %v", tt.mode, got, tt.visible)
		}
	}
}

// ============================================================================
// Filtering Tests
// ============================================================================

func TestFilterVisible(t *testing.T) {
	events := []MemoryEvent{
		{ID: 1, DisplayMode: "full", Content: "a"},
		{ID: 2, DisplayMode: "hidden", Content: "b"},
		{ID: 3, DisplayMode: "compressed", Content: "c"},
		{ID: 4, DisplayMode: "hidden", Content: "d"},
	}
	visible := FilterVisible(events)
	if len(visible) != 2 {
		t.Errorf("expected 2 visible events, got %d", len(visible))
	}
	for _, evt := range visible {
		if evt.DisplayMode == "hidden" {
			t.Errorf("hidden event %d should not be visible", evt.ID)
		}
	}
}

func TestFilterVisible_AllVisible(t *testing.T) {
	events := []MemoryEvent{
		{ID: 1, DisplayMode: "full"},
		{ID: 2, DisplayMode: "compressed"},
	}
	visible := FilterVisible(events)
	if len(visible) != 2 {
		t.Errorf("expected all visible, got %d", len(visible))
	}
}

func TestFilterVisible_AllHidden(t *testing.T) {
	events := []MemoryEvent{
		{ID: 1, DisplayMode: "hidden"},
		{ID: 2, DisplayMode: "hidden"},
	}
	visible := FilterVisible(events)
	if len(visible) != 0 {
		t.Errorf("expected none visible, got %d", len(visible))
	}
}

func TestFilterVisible_NilSlice(t *testing.T) {
	visible := FilterVisible(nil)
	if visible == nil {
		t.Error("expected non-nil result from nil input")
	}
	if len(visible) != 0 {
		t.Errorf("expected empty result, got %d", len(visible))
	}
}

func TestFilterByType(t *testing.T) {
	events := []MemoryEvent{
		{ID: 1, Type: "text_block", Content: "a"},
		{ID: 2, Type: "tool_call", Content: "b"},
		{ID: 3, Type: "thinking", Content: "c"},
		{ID: 4, Type: "tool_call", Content: "d"},
		{ID: 5, Type: "system", Content: "e"},
	}

	// Filter to tool_call and thinking
	filtered := FilterByType(events, "tool_call", "thinking")
	if len(filtered) != 3 {
		t.Errorf("expected 3 events (2 tool_call + 1 thinking), got %d", len(filtered))
	}
	for _, evt := range filtered {
		if evt.Type != "tool_call" && evt.Type != "thinking" {
			t.Errorf("unexpected type %q in filtered result", evt.Type)
		}
	}
}

func TestFilterByType_NoneMatch(t *testing.T) {
	events := []MemoryEvent{
		{ID: 1, Type: "text_block"},
	}
	filtered := FilterByType(events, "nonexistent")
	if len(filtered) != 0 {
		t.Errorf("expected 0 matches, got %d", len(filtered))
	}
}

func TestFilterByType_NoTypes(t *testing.T) {
	events := []MemoryEvent{
		{ID: 1, Type: "text_block"},
	}
	filtered := FilterByType(events)
	if len(filtered) != 0 {
		t.Errorf("expected 0 matches with no types, got %d", len(filtered))
	}
}

func TestInPageSet(t *testing.T) {
	pageIDs := PageMemoryIDs{10: true, 20: true}

	if !InPageSet(10, pageIDs) {
		t.Error("expected ID 10 to be in page set")
	}
	if InPageSet(30, pageIDs) {
		t.Error("expected ID 30 to NOT be in page set")
	}
	if InPageSet(10, nil) {
		t.Error("expected false for nil page set")
	}
}

// ============================================================================
// Context Formatting Tests
// ============================================================================

func TestFormatContextAsMarkdown_EmptyContext(t *testing.T) {
	ctx := &ActiveContext{}
	result := FormatContextAsMarkdown(ctx)

	if !strings.Contains(result, "# Active Context") {
		t.Error("missing heading in empty context")
	}
	if !strings.Contains(result, "## Constraints") {
		t.Error("missing constraints in empty context")
	}
}

func TestFormatContextAsMarkdown_WithSession(t *testing.T) {
	ctx := &ActiveContext{
		Session: &SessionInfo{
			AgentName: "test-agent",
			Goal:      "Do something useful",
			Status:    "idle",
			Iteration: 5,
		},
	}
	result := FormatContextAsMarkdown(ctx)

	if !strings.Contains(result, "test-agent") {
		t.Error("missing agent name")
	}
	if !strings.Contains(result, "Do something useful") {
		t.Error("missing goal")
	}
	if !strings.Contains(result, "idle") {
		t.Error("missing status")
	}
	if !strings.Contains(result, "**Iteration:** 5") {
		t.Error("missing iteration in session header")
	}
}

func TestFormatContextAsMarkdown_WithTools(t *testing.T) {
	ctx := &ActiveContext{
		Tools: []ToolInfo{
			{Name: "scraper", Description: "Scrapes web pages", Hemisphere: "external", HandlerType: "subprocess"},
			{Name: "page_context", Description: "Pages context", Hemisphere: "internal", HandlerType: "sql_function"},
		},
	}
	result := FormatContextAsMarkdown(ctx)

	if !strings.Contains(result, "scraper") {
		t.Error("missing tool name")
	}
	if !strings.Contains(result, "external") {
		t.Error("missing hemisphere label")
	}
	if !strings.Contains(result, "page_context") {
		t.Error("missing second tool")
	}
}

func TestFormatContextAsMarkdown_WithSkills(t *testing.T) {
	ctx := &ActiveContext{
		Skills: []SkillInfo{
			{Name: "excel_generator", Description: "Generates Excel files", Enabled: true},
			{Name: "deprecated_tool", Description: "Old skill", Enabled: false},
		},
	}
	result := FormatContextAsMarkdown(ctx)

	if !strings.Contains(result, "excel_generator") {
		t.Error("missing enabled skill")
	}
	if strings.Contains(result, "deprecated_tool") {
		t.Error("disabled skill should not appear")
	}
}

func TestFormatContextAsMarkdown_WithMemoryEvents(t *testing.T) {
	ctx := &ActiveContext{
		MemoryEvents: []MemoryEvent{
			{
				ID: 1, Type: "text_block", Content: "Hello world",
				DisplayMode: "full", CreatedAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
			},
			{
				ID: 2, Type: "tool_call", Content: "scraping...",
				DisplayMode: "full", CreatedAt: time.Date(2026, 1, 1, 1, 0, 0, 0, time.UTC),
			},
		},
	}
	result := FormatContextAsMarkdown(ctx)

	if !strings.Contains(result, "Hello world") {
		t.Error("missing first event content")
	}
	if !strings.Contains(result, "scraping...") {
		t.Error("missing second event content")
	}
	if !strings.Contains(result, "[text_block]") {
		t.Error("missing event type header")
	}
	if !strings.Contains(result, "[tool_call]") {
		t.Error("missing second event type header")
	}
}

func TestFormatContextAsMarkdown_HiddenEventsExcluded(t *testing.T) {
	ctx := &ActiveContext{
		MemoryEvents: []MemoryEvent{
			{ID: 1, Type: "text_block", Content: "visible", DisplayMode: "full"},
			{ID: 2, Type: "system", Content: "secret", DisplayMode: "hidden"},
			{ID: 3, Type: "text_block", Content: "also visible", DisplayMode: "full"},
		},
	}
	result := FormatContextAsMarkdown(ctx)

	if !strings.Contains(result, "visible") {
		t.Error("missing visible event")
	}
	if strings.Contains(result, "secret") {
		t.Error("hidden event should not appear in output")
	}
	if !strings.Contains(result, "also visible") {
		t.Error("missing second visible event")
	}
}

func TestFormatContextAsMarkdown_CompressedEvents(t *testing.T) {
	ctx := &ActiveContext{
		MemoryEvents: []MemoryEvent{
			{
				ID: 1, Type: "text_block", Content: "long original text that was compressed",
				SummaryText: "short summary", DisplayMode: "compressed",
			},
		},
	}
	result := FormatContextAsMarkdown(ctx)

	if !strings.Contains(result, "short summary") {
		t.Error("expected summary text for compressed event")
	}
	if !strings.Contains(result, "[compressed]") {
		t.Error("expected [compressed] label")
	}
}

func TestFormatContextAsMarkdown_WithMemoryPages(t *testing.T) {
	ctx := &ActiveContext{
		MemoryPages: []MemoryPage{
			{ID: 1, Name: "cached_analysis", TargetIDs: []int64{10, 20, 30}},
			{ID: 2, Name: "research_notes", TargetIDs: []int64{40}},
		},
	}
	result := FormatContextAsMarkdown(ctx)

	if !strings.Contains(result, "cached_analysis") {
		t.Error("missing page name")
	}
	if !strings.Contains(result, "events: 3") {
		t.Error("missing event count for first page")
	}
	if !strings.Contains(result, "research_notes") {
		t.Error("missing second page")
	}
}

func TestFormatContextAsMarkdown_ConstraintsOutput(t *testing.T) {
	ctx := &ActiveContext{
		Constraints: ContextConstraints{
			Iteration:         10,
			MaxIterations:     100,
			BudgetUsedCents:   42,
			BudgetLimitCents:  1000,
			ConsecutiveErrors: 2,
			MaxConsErrors:     5,
			PlanningMaxTurns:  10,
			ContextBudget:     128000,
		},
	}
	result := FormatContextAsMarkdown(ctx)

	checks := []string{
		"Iteration: 10 / 100",
		"Budget used: 42 / 1000 cents",
		"Consecutive errors: 2 / 5",
		"Planning turns: 10 max",
		"Context budget: 128000 tokens",
	}
	for _, check := range checks {
		if !strings.Contains(result, check) {
			t.Errorf("missing constraint: %q", check)
		}
	}
}

func TestFormatContextAsMarkdown_FullIntegration(t *testing.T) {
	ctx := &ActiveContext{
		Session: &SessionInfo{
			AgentName: "researcher", Goal: "analyze data", Status: "thinking", Iteration: 3,
		},
		Tools: []ToolInfo{
			{Name: "scraper", Description: "Scrapes web pages", Hemisphere: "external", HandlerType: "subprocess"},
		},
		Skills: []SkillInfo{
			{Name: "data_analysis", Description: "Analyze datasets", Enabled: true},
		},
		MemoryEvents: []MemoryEvent{
			{ID: 1, Type: "text_block", Content: "Starting analysis", DisplayMode: "full", CreatedAt: time.Now()},
			{ID: 2, Type: "tool_call", Content: "Scraping example.com...", DisplayMode: "full", CreatedAt: time.Now()},
		},
		MemoryPages: []MemoryPage{
			{ID: 1, Name: "research_cache", TargetIDs: []int64{1, 2}},
		},
		Constraints: ContextConstraints{
			Iteration: 3, MaxIterations: 100, BudgetUsedCents: 15, BudgetLimitCents: 1000,
			ConsecutiveErrors: 0, MaxConsErrors: 5, PlanningMaxTurns: 10, ContextBudget: 128000,
		},
	}
	result := FormatContextAsMarkdown(ctx)

	// Layer 1: Session and tools
	if !strings.Contains(result, "researcher") {
		t.Error("missing agent name")
	}
	if !strings.Contains(result, "scraper") {
		t.Error("missing tool")
	}
	if !strings.Contains(result, "data_analysis") {
		t.Error("missing skill")
	}

	// Layer 2: Memory events
	if !strings.Contains(result, "Starting analysis") {
		t.Error("missing first event")
	}
	if !strings.Contains(result, "Scraping example.com") {
		t.Error("missing second event")
	}

	// Memory pages
	if !strings.Contains(result, "research_cache") {
		t.Error("missing memory page")
	}

	// Layer 3: Constraints at end
	constraintsIdx := strings.Index(result, "## Constraints")
	memoryIdx := strings.LastIndex(result, "## Memory Events")
	if constraintsIdx < memoryIdx {
		t.Error("constraints should appear after memory events (cache layer 3)")
	}
}

// ============================================================================
// Edge Cases & Boundary Tests
// ============================================================================

func TestFormatContextAsMarkdown_EmptyGoal(t *testing.T) {
	ctx := &ActiveContext{
		Session: &SessionInfo{AgentName: "test", Goal: ""},
	}
	result := FormatContextAsMarkdown(ctx)
	if strings.Contains(result, "**Goal:**") {
		t.Error("should not show goal header when empty")
	}
}

func TestFormatContextAsMarkdown_NoSession(t *testing.T) {
	ctx := &ActiveContext{}
	result := FormatContextAsMarkdown(ctx)
	if strings.Contains(result, "Session:") {
		t.Error("should not show session section when nil")
	}
}

func TestResolvePageMemoryIDs_EmptySlice(t *testing.T) {
	ids := ResolvePageMemoryIDs([]MemoryPage{})
	if len(ids) != 0 {
		t.Errorf("expected empty set from empty slice, got %d entries", len(ids))
	}
}

func TestResolvePageMemoryIDs_LargeIDSet(t *testing.T) {
	pages := make([]MemoryPage, 0, 10)
	for i := int64(0); i < 10; i++ {
		pages = append(pages, MemoryPage{
			ID: i + 1, Name: "page", TargetIDs: []int64{i*10 + 1, i*10 + 2},
		})
	}
	ids := ResolvePageMemoryIDs(pages)
	if len(ids) != 20 {
		t.Errorf("expected 20 unique IDs from 10 pages, got %d", len(ids))
	}
}

func TestFormatContextAsMarkdown_ZeroTimeEvents(t *testing.T) {
	ctx := &ActiveContext{
		MemoryEvents: []MemoryEvent{
			{ID: 1, Type: "text_block", Content: "no timestamp", DisplayMode: "full"},
		},
	}
	result := FormatContextAsMarkdown(ctx)
	if !strings.Contains(result, "unknown time") {
		t.Error("expected 'unknown time' for zero timestamp")
	}
}
