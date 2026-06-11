// Package harness: context reader and Markdown formatter (SPEC-008, SPEC-012).
//
// The context reader queries the database for session state, memory events,
// tool registries, and constraint data. It formats everything as structured
// Markdown for the LLM.
//
// Memory page resolution (§5.4 of SPEC-002) expands named page groups into
// resolved memory event IDs. Overlapping pages are deduplicated so shared
// events appear only once. Pages support single-level nesting via linked_page_ids.
//
// On PostgreSQL, the harness queries the enhanced active_context_view which
// includes tool call collapse, cache tier ordering, page resolution, and
// DISTINCT ON deduplication. The view is RLS-isolated via SET LOCAL.
// On SQLite, the harness uses Go-level fallback queries since views
// are stripped by the migration filter.
//
// axiom:trace work_item=WI-007 spec=specs/001-architecture.md,specs/002-memory.md,specs/003-database.md,specs/005-security.md plan=phase-2/task-1 impl=internal/harness/context.go
package harness

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/wojons/conscientiousness/internal/db"
	"github.com/wojons/conscientiousness/internal/memory"
)

// ============================================================================
// Memory Page Resolution (SPEC-002 §5) — AC-MEM-H01
// ============================================================================

// pageMemoryIDs stores the set of memory IDs expanded from memory_pages.
// Used to deduplicate events appearing in multiple pages (AC-MEM-H02).
type pageMemoryIDs map[int64]bool

// resolvePageMemoryIDs expands memory_pages for a session into a deduplicated
// set of memory event IDs. Supports single-level nesting:
//   page.target_ids → direct event IDs
//   page.linked_page_ids → resolve target_ids of linked pages (depth 1)
//
// Returns a deduplicated map of memory event ID → true.
func (h *Harness) resolvePageMemoryIDs(ctx context.Context, sessionID string) (pageMemoryIDs, error) {
	if h.db == nil {
		return pageMemoryIDs{}, nil
	}

	ids := make(pageMemoryIDs)

	// Query all memory pages for this session
	rows, err := h.db.Query(ctx, `
		SELECT id, target_ids, linked_page_ids
		FROM memory_pages
		WHERE session_id = $1
		ORDER BY created_at
	`, sessionID)
	if err != nil {
		return ids, fmt.Errorf("resolve pages: query: %w", err)
	}
	if len(rows) == 0 {
		return ids, nil // No pages defined
	}

	// Collect all page rows
	type pageRow struct {
		ID            int64
		TargetIDs     []int64
		LinkedPageIDs []int64
	}
	var pages []pageRow
	for _, r := range rows {
		p := pageRow{ID: toInt64(r["id"])}
		if raw, ok := r["target_ids"]; ok {
			p.TargetIDs = toInt64Array(raw)
		}
		if raw, ok := r["linked_page_ids"]; ok {
			p.LinkedPageIDs = toInt64Array(raw)
		}
		pages = append(pages, p)
	}

	// Build lookup of page ID → target_ids for linked page resolution
	pageTargets := make(map[int64][]int64, len(pages))
	for _, p := range pages {
		pageTargets[p.ID] = p.TargetIDs
	}

	// Resolve: direct target_ids + linked_page_ids → their target_ids
	for _, p := range pages {
		// Direct targets
		for _, tid := range p.TargetIDs {
			ids[tid] = true
		}
		// Single-level linked page resolution
		for _, lid := range p.LinkedPageIDs {
			if targets, ok := pageTargets[lid]; ok {
				for _, tid := range targets {
					ids[tid] = true
				}
			}
		}
	}

	return ids, nil
}

// annotatePageEvents marks events that are included in active memory pages
// and removes duplicates when the same event appears from multiple sources.
// This implements SPEC-002 §3.6 (zero-cost deduplication) via Go-layer filtering
// instead of SQL DISTINCT ON (which isn't portable to SQLite).
//
// Two-pass algorithm:
//  1. Mark: first pass annotates if event is in a page and tracks seen IDs
//  2. Filter: second pass excludes duplicates beyond first occurrence
func (h *Harness) annotatePageEvents(memories []MemoryEventInfo, pageIDs pageMemoryIDs) []MemoryEventInfo {
	if len(memories) == 0 || len(pageIDs) == 0 {
		return memories
	}

	// Pass 1: mark page membership and collect seen IDs (first occurrence wins)
	seenIDs := make(map[int64]bool, len(memories))
	type annotatedEvent struct {
		event  MemoryEventInfo
		inPage bool
		dupID  bool
	}
	annotated := make([]annotatedEvent, 0, len(memories))

	for _, m := range memories {
		ae := annotatedEvent{event: m}
		if pageIDs[m.ID] {
			ae.inPage = true
		}
		if seenIDs[m.ID] && !ae.inPage {
			ae.dupID = true // duplicate from non-page source — first occurrence already seen
		}
		if !ae.dupID {
			seenIDs[m.ID] = true
		}
		annotated = append(annotated, ae)
	}

	// Pass 2: filter out duplicates; keep page-resolved events
	result := make([]MemoryEventInfo, 0, len(memories)/2+len(pageIDs))
	for _, ae := range annotated {
		if ae.dupID {
			continue // duplicate, skip
		}
		// Add page annotation to type for visibility in context
		if ae.inPage {
			ae.event.Type = ae.event.Type + "(page)"
		}
		result = append(result, ae.event)
	}

	return result
}

// toInt64Array converts an any value (JSONB array from SQLite driver) to []int64.
func toInt64Array(v any) []int64 {
	if v == nil {
		return nil
	}
	switch raw := v.(type) {
	case []int64:
		return raw
	case []interface{}:
		result := make([]int64, 0, len(raw))
		for _, item := range raw {
			switch n := item.(type) {
			case int64:
				result = append(result, n)
			case float64:
				result = append(result, int64(n))
			case int:
				result = append(result, int64(n))
			}
		}
		return result
	case string:
		return parseInt64ArrayFromString(raw)
	default:
		return nil
	}
}

// parseInt64ArrayFromString parses a SQLite-serialized array string.
// Handles both PostgreSQL format: {12,45,102}
// And JSON format: [12,45,102]
func parseInt64ArrayFromString(s string) []int64 {
	if s == "" || s == "[]" || s == "null" || s == "{}" {
		return nil
	}
	// Strip braces (both {} and [])
	s = strings.Trim(s, "{}[]")
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	result := make([]int64, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		var n int64
		if _, err := fmt.Sscanf(p, "%d", &n); err == nil {
			result = append(result, n)
		}
	}
	return result
}

// ============================================================================
// Context Reading
// ============================================================================

// ReadActiveContext queries the database for all context data needed by the LLM.
// This includes session state, recent memory events, available tools, and
// constraint/budget information.
//
// The method opens a transaction and sets the RLS session context (SET LOCAL
// on Postgres) before any queries. On Postgres, memory events are read from
// the active_context_view (which handles dedup, display modes, tool call
// collapse, and cache tier ordering). On SQLite, Go-level fallback queries
// provide equivalent logic.
func (h *Harness) ReadActiveContext(ctx context.Context, sessionID string) (*IterationContext, error) {
	// Begin transaction for RLS context enforcement
	tx, err := h.db.BeginTx(ctx)
	if err != nil {
		return nil, fmt.Errorf("read context: begin tx: %w", err)
	}
	defer func() {
		if tx.IsActive() {
			_ = tx.Rollback()
		}
	}()

	// Set RLS session context (Postgres: SET LOCAL conscience.session_id;
	// SQLite: stores session ID for Go-layer enforcement)
	if err := tx.SetSessionContext(ctx, sessionID); err != nil {
		return nil, fmt.Errorf("read context: set session context: %w", err)
	}

	// Read session within the transaction
	session, err := h.readSessionTx(ctx, tx, sessionID)
	if err != nil {
		return nil, fmt.Errorf("read session: %w", err)
	}

	// Read memory events — backend-aware
	// On Postgres: query enhanced active_context_view (tool call collapse,
	// cache tier, dedup, page resolution via CTEs, SPEC-002 §3.5/§3.6/§5.2)
	// On SQLite: Go-level fallback with manual page resolution (SPEC-002 §5)
	var memories []MemoryEventInfo
	if h.db != nil && h.db.Backend() == db.BackendPostgres {
		memories, err = h.readMemoriesFromView(ctx, tx, sessionID)
	} else {
		// SQLite: resolve memory pages and read memory events via direct SQL
		pageIDs, pgErr := h.resolvePageMemoryIDsTx(ctx, tx, sessionID)
		if pgErr != nil {
			// Page resolution failure is non-fatal — proceed without page expansion
			pageIDs = pageMemoryIDs{}
		}
		memories, err = h.readMemoryEventsTx(ctx, tx, sessionID)
		if err == nil && len(pageIDs) > 0 {
			memories = h.annotatePageEvents(memories, pageIDs)
		}
	}
	if err != nil {
		return nil, fmt.Errorf("read memory: %w", err)
	}

	// Sort by cache tier for prompt caching optimization (SPEC-003 §6.2)
	// Layer 1 (static system) first, Layer 2 (immutable ledger) next, Layer 3 (dynamic) last
	h.sortByCacheTier(memories)

	// Read available tools (not session-scoped, but use tx for consistency)
	tools, err := h.readToolsTx(ctx, tx)
	if err != nil {
		return nil, fmt.Errorf("read tools: %w", err)
	}

	// Check if this session is a sub-agent (SPEC-012 §6: filtered tool access)
	isSubAgent := session.ParentID != ""

	// Assemble IterationContext
	ic := &IterationContext{
		SessionID:            sessionID,
		AgentName:            session.AgentName,
		ModelID:              session.ModelID,
		TrustLevel:           session.TrustLevel,
		Goal:                 session.Goal,
		Status:               session.Status,
		Iteration:            session.Iteration,
		ContextBudget:        session.ContextBudget,
		TokensUsedIn:         session.TokensUsedIn,
		TokensUsedOut:        session.TokensUsedOut,
		MaxIterations:        session.MaxIterations,
		MaxConsecutiveErrors: session.MaxConsecutiveErrors,
		IsSubAgent:           isSubAgent,
	}

	// Format Markdown messages
	ic.Messages = []Message{
		{
			Role:    "system",
			Content: h.formatSystemPrompt(ic, tools),
		},
		{
			Role:    "user",
			Content: h.formatContextMarkdown(ic, memories, tools),
		},
	}

	// Commit the read transaction
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("read context: commit: %w", err)
	}

	return ic, nil
}

// ============================================================================
// System Prompt Assembly (SPEC-012)
// ============================================================================

// formatSystemPrompt builds the system prompt from session state and tool metadata.
func (h *Harness) formatSystemPrompt(ic *IterationContext, tools []ToolInfo) string {
	var sb strings.Builder

	sb.WriteString("You are a Conscience agent. You run inside a database-native runtime.\n")
	sb.WriteString("Your goal is: ")
	sb.WriteString(ic.Goal)
	sb.WriteString("\n\n")

	sb.WriteString("You manage your state by outputting structured JSON. Each iteration you may:\n")
	sb.WriteString("- Write SQL to memory_state_changes (your persistent ledger)\n")
	sb.WriteString("- Request external tools via tool_requests\n")
	sb.WriteString("- Set system_actions (status updates)\n")
	sb.WriteString("- Spawn sub-agents via sub_agent_spawns\n\n")

	sb.WriteString("Rules:\n")
	sb.WriteString("- All SQL is executed in an atomic transaction. On failure, everything rolls back.\n")
	sb.WriteString("- You cannot UPDATE or DELETE from memory_events (append-only ledger).\n")
	sb.WriteString("- Only access tables scoped to your session_id.\n")
	sb.WriteString("- If you encounter an error, the harness injects it into the next context for recovery.\n")
	sb.WriteString(fmt.Sprintf("- Max iterations: %d. Max consecutive errors: %d.\n",
		ic.MaxIterations, ic.MaxConsecutiveErrors))
	sb.WriteString("- Format all dates as ISO 8601. Quote identifiers with double-quotes.\n\n")

	// Sub-agent filtering (SPEC-012 §6): sub-agents only see internal hemisphere tools
	filteredTools := tools
	if ic.IsSubAgent {
		internal := make([]ToolInfo, 0, len(tools))
		for _, t := range tools {
			if t.Hemisphere == "internal" {
				internal = append(internal, t)
			}
		}
		filteredTools = internal
		sb.WriteString("Note: You are a sub-agent. Only internal tools are available.\n\n")
	}

	sb.WriteString("Available tools:\n")
	for _, t := range filteredTools {
		sb.WriteString(fmt.Sprintf("- %s (%s): %s\n", t.Name, t.Hemisphere, t.Description))
	}

	return sb.String()
}

// ============================================================================
// Context Markdown (SPEC-008)
// ============================================================================

// formatContextMarkdown builds the Markdown context body from memory events,
// tool metadata, and session constraints.
func (h *Harness) formatContextMarkdown(ic *IterationContext, memories []MemoryEventInfo, tools []ToolInfo) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("# Active Context — Session %s\n\n", ic.SessionID))

	// Task state
	sb.WriteString("## Current Task\n")
	sb.WriteString(fmt.Sprintf("Goal: %s\n", ic.Goal))
	sb.WriteString(fmt.Sprintf("Status: %s | Iteration: %d / %d\n\n", ic.Status, ic.Iteration, ic.MaxIterations))

	// Memory ledger
	sb.WriteString(fmt.Sprintf("## Memory (%d events)\n", len(memories)))
	if len(memories) == 0 {
		sb.WriteString("(no memory events yet)\n\n")
	} else {
		for _, m := range memories {
			sb.WriteString(formatMemoryEvent(m))
		}
	}

	// Tool reference
	sb.WriteString("## Available Tools\n")
	for _, t := range tools {
		sb.WriteString(fmt.Sprintf("- **%s** (%s): %s\n", t.Name, t.Hemisphere, t.Description))
	}
	sb.WriteString("\n")

	// Constraints
	sb.WriteString("## Constraints\n")
	sb.WriteString(fmt.Sprintf("- Iteration: %d / %d\n", ic.Iteration, ic.MaxIterations))
	sb.WriteString(fmt.Sprintf("- Tokens used: %d in / %d out\n", ic.TokensUsedIn, ic.TokensUsedOut))
	sb.WriteString(fmt.Sprintf("- Budget: %d/%d cents\n", ic.BudgetUsedCents, ic.BudgetLimitCents))
	sb.WriteString(fmt.Sprintf("- Consecutive errors: %d / %d\n", ic.ConsecutiveErrors, ic.MaxConsecutiveErrors))

	return sb.String()
}

// formatMemoryEvent renders a single memory event for Markdown output.
// Uses type-specific formatting per SPEC-002 §7.2 (AC-025).
// Adds display mode labels ([compressed]) for compressed events (AC-024).
func formatMemoryEvent(m MemoryEventInfo) string {
	// Build a MemoryEvent for the formatter
	evt := memory.MemoryEvent{
		Type:         m.Type,
		Content:      m.Content,
		SummaryText:  m.SummaryText,
		DisplayMode:  m.DisplayMode,
	}
	rendered := memory.FormatMemoryEventByType(evt)
	// Add compressed label before the rendered content
	if m.DisplayMode == "compressed" {
		rendered = "[compressed] " + rendered
	}
	return rendered
}

// ============================================================================
// Database Query Helpers
// ============================================================================

// sessionRow holds a flattened session query result.
type sessionRow struct {
	AgentName           string
	ModelID             string
	Status              string
	TrustLevel          string // low, medium, high (SPEC-008 §5.4)
	Goal                string
	ContextBudget       int
	TokensUsedIn        int64
	TokensUsedOut       int64
	Iteration           int64
	MaxIterations       int
	MaxConsecutiveErrors int
	ParentID            string // empty string = root agent, non-empty = sub-agent (SPEC-012 §6)
}

// MemoryEventInfo holds a single memory event with display mode and cache metadata.
type MemoryEventInfo struct {
	ID               int64
	Type             string
	Content          string
	SummaryText      string
	DisplayMode      string
	IterationCreated int64
	CacheTier        int    // 1=static, 2=ledger, 3=dynamic (SPEC-003 §6.2)
	CollapseStatus   string // "full" or "collapsed" (SPEC-002 §3.5)
}

// ToolInfo holds a tool's metadata for system prompt assembly.
type ToolInfo struct {
	Name        string
	Description string
	Hemisphere  string // "internal" | "external"
}

func (h *Harness) readSession(ctx context.Context, sessionID string) (*sessionRow, error) {
	rows, err := h.db.Query(ctx, `
		SELECT agent_name, model_id, status,
		       COALESCE(trust_level, 'high') AS trust_level,
		       COALESCE(goal, '') AS goal,
		       context_budget, tokens_used_in, tokens_used_out,
		       iteration, planning_max_turns, 3,
		       COALESCE(parent_id, '') AS parent_id
		FROM sessions WHERE id = $1
	`, sessionID)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, fmt.Errorf("session not found: %s", sessionID)
	}
	r := rows[0]
	return &sessionRow{
		AgentName:            toString(r["agent_name"]),
		ModelID:              toString(r["model_id"]),
		Status:               toString(r["status"]),
		TrustLevel:           toString(r["trust_level"]),
		Goal:                 toString(r["goal"]),
		ContextBudget:        toInt(r["context_budget"]),
		TokensUsedIn:         toInt64(r["tokens_used_in"]),
		TokensUsedOut:        toInt64(r["tokens_used_out"]),
		Iteration:            toInt64(r["iteration"]),
		MaxIterations:        toInt(r["planning_max_turns"]),
		MaxConsecutiveErrors: 3,
		ParentID:             toString(r["parent_id"]),
	}, nil
}

// readSessionTx reads a session row within a transaction.
func (h *Harness) readSessionTx(ctx context.Context, tx db.Tx, sessionID string) (*sessionRow, error) {
	rows, err := tx.Query(ctx, `
		SELECT agent_name, model_id, status,
		       COALESCE(trust_level, 'high') AS trust_level,
		       COALESCE(goal, '') AS goal,
		       context_budget, tokens_used_in, tokens_used_out,
		       iteration, planning_max_turns, 3,
		       COALESCE(parent_id, '') AS parent_id
		FROM sessions WHERE id = $1
	`, sessionID)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, fmt.Errorf("session not found: %s", sessionID)
	}
	r := rows[0]
	return &sessionRow{
		AgentName:            toString(r["agent_name"]),
		ModelID:              toString(r["model_id"]),
		Status:               toString(r["status"]),
		TrustLevel:           toString(r["trust_level"]),
		Goal:                 toString(r["goal"]),
		ContextBudget:        toInt(r["context_budget"]),
		TokensUsedIn:         toInt64(r["tokens_used_in"]),
		TokensUsedOut:        toInt64(r["tokens_used_out"]),
		Iteration:            toInt64(r["iteration"]),
		MaxIterations:        toInt(r["planning_max_turns"]),
		MaxConsecutiveErrors: 3,
		ParentID:             toString(r["parent_id"]),
	}, nil
}

// readMemoryEventsTx reads memory events within a transaction.
// Used by the SQLite backend where the enhanced VIEW is not available.
func (h *Harness) readMemoryEventsTx(ctx context.Context, tx db.Tx, sessionID string) ([]MemoryEventInfo, error) {
	rows, err := tx.Query(ctx, `
		SELECT me.id, me.type,
		       COALESCE(me.content, '') as content,
		       COALESCE(me.summary_text, '') as summary_text,
		       COALESCE(dm.mode, 'full') as display_mode,
		       me.iteration_created
		FROM memory_events me
		LEFT JOIN display_modes dm ON dm.memory_id = me.id
		WHERE me.session_id = $1
		  AND COALESCE(dm.mode, 'full') != 'hidden'
		ORDER BY me.iteration_created, me.id
		LIMIT 100
	`, sessionID)
	if err != nil {
		return nil, err
	}

	memories := make([]MemoryEventInfo, 0, len(rows))
	for _, r := range rows {
		memories = append(memories, MemoryEventInfo{
			ID:               toInt64(r["id"]),
			Type:             toString(r["type"]),
			Content:          toString(r["content"]),
			SummaryText:      toString(r["summary_text"]),
			DisplayMode:      toString(r["display_mode"]),
			IterationCreated: toInt64(r["iteration_created"]),
		})
	}
	return memories, nil
}

// readTools reads all active tools from the tools_registry (SPEC-011 §Tools).
// Used by SystemPromptBuilder for prompt assembly.
func (h *Harness) readTools(ctx context.Context) ([]ToolInfo, error) {
	if h.db == nil {
		return nil, fmt.Errorf("readTools: no database configured")
	}
	rows, err := h.db.Query(ctx, `
		SELECT name, description, hemisphere
		FROM tools_registry
		WHERE enabled = true AND status = 'active'
		ORDER BY name
	`)
	if err != nil {
		return nil, err
	}

	tools := make([]ToolInfo, 0, len(rows))
	for _, r := range rows {
		tools = append(tools, ToolInfo{
			Name:        toString(r["name"]),
			Description: toString(r["description"]),
			Hemisphere:  toString(r["hemisphere"]),
		})
	}
	return tools, nil
}

// readMemoriesFromView queries the enhanced active_context_view on PostgreSQL.
// This replaces Go-level memory event assembly with the database-driven VIEW
// that provides:
//   - Tool call collapse via window functions (SPEC-002 §3.5)
//   - DISTINCT ON deduplication (SPEC-002 §3.6)
//   - Page resolution (direct + linked page expansion, SPEC-002 §5.2)
//   - Display mode rendering (CASE: compressed→summary, hidden→NULL)
//   - Cache tier ordering column (SPEC-003 §6.2)
//   - RLS isolation via SET LOCAL conscience.session_id
func (h *Harness) readMemoriesFromView(ctx context.Context, tx db.Tx, sessionID string) ([]MemoryEventInfo, error) {
	rows, err := tx.Query(ctx, `
		SELECT id, session_id, iteration_created, type,
		       COALESCE(raw_content, '') AS raw_content,
		       COALESCE(summary_text, '') AS summary_text,
		       display_mode,
		       COALESCE(rendered_text, '') AS rendered_text,
		       collapse_status,
		       cache_tier
		FROM active_context_view
		ORDER BY cache_tier, iteration_created, id
	`)
	if err != nil {
		return nil, fmt.Errorf("query active_context_view: %w", err)
	}

	memories := make([]MemoryEventInfo, 0, len(rows))
	for _, r := range rows {
		m := MemoryEventInfo{
			ID:               toInt64(r["id"]),
			Type:             toString(r["type"]),
			Content:          toString(r["rendered_text"]),     // VIEW provides rendered text
			SummaryText:      toString(r["summary_text"]),
			DisplayMode:      toString(r["display_mode"]),
			IterationCreated: toInt64(r["iteration_created"]),
			CacheTier:        toInt(r["cache_tier"]),
			CollapseStatus:   toString(r["collapse_status"]),
		}
		// If rendered_text is empty (shouldn't happen since hidden is filtered),
		// fall back to raw_content for backward compat
		if m.Content == "" {
			m.Content = toString(r["raw_content"])
		}
		memories = append(memories, m)
	}
	return memories, nil
}

// resolvePageMemoryIDsTx expands memory_pages for a session within a transaction.
func (h *Harness) resolvePageMemoryIDsTx(ctx context.Context, tx db.Tx, sessionID string) (pageMemoryIDs, error) {
	if h.db == nil {
		return pageMemoryIDs{}, nil
	}

	ids := make(pageMemoryIDs)

	rows, err := tx.Query(ctx, `
		SELECT id, target_ids, linked_page_ids
		FROM memory_pages
		WHERE session_id = $1
		ORDER BY created_at
	`, sessionID)
	if err != nil {
		return ids, fmt.Errorf("resolve pages tx: query: %w", err)
	}
	if len(rows) == 0 {
		return ids, nil
	}

	type pageRow struct {
		ID            int64
		TargetIDs     []int64
		LinkedPageIDs []int64
	}
	var pages []pageRow
	for _, r := range rows {
		p := pageRow{ID: toInt64(r["id"])}
		if raw, ok := r["target_ids"]; ok {
			p.TargetIDs = toInt64Array(raw)
		}
		if raw, ok := r["linked_page_ids"]; ok {
			p.LinkedPageIDs = toInt64Array(raw)
		}
		pages = append(pages, p)
	}

	pageTargets := make(map[int64][]int64, len(pages))
	for _, p := range pages {
		pageTargets[p.ID] = p.TargetIDs
	}

	for _, p := range pages {
		for _, tid := range p.TargetIDs {
			ids[tid] = true
		}
		for _, lid := range p.LinkedPageIDs {
			if targets, ok := pageTargets[lid]; ok {
				for _, tid := range targets {
					ids[tid] = true
				}
			}
		}
	}

	return ids, nil
}

// readToolsTx reads active tools within a transaction.
func (h *Harness) readToolsTx(ctx context.Context, tx db.Tx) ([]ToolInfo, error) {
	if h.db == nil {
		return nil, fmt.Errorf("readToolsTx: no database configured")
	}
	rows, err := tx.Query(ctx, `
		SELECT name, description, hemisphere
		FROM tools_registry
		WHERE enabled = true AND status = 'active'
		ORDER BY name
	`)
	if err != nil {
		return nil, err
	}

	tools := make([]ToolInfo, 0, len(rows))
	for _, r := range rows {
		tools = append(tools, ToolInfo{
			Name:        toString(r["name"]),
			Description: toString(r["description"]),
			Hemisphere:  toString(r["hemisphere"]),
		})
	}
	return tools, nil
}

// sortByCacheTier sorts memory events by cache tier for prompt caching optimization.
// Ordering: Layer 1 (static system) → Layer 2 (immutable ledger) → Layer 3 (dynamic).
// Within each tier, events are ordered chronologically (SPEC-003 §6.2).
func (h *Harness) sortByCacheTier(memories []MemoryEventInfo) {
	sort.SliceStable(memories, func(i, j int) bool {
		// Primary: cache tier ascending (1, 2, 3)
		if memories[i].CacheTier != memories[j].CacheTier {
			return memories[i].CacheTier < memories[j].CacheTier
		}
		// Secondary: iteration_created ascending within same tier
		if memories[i].IterationCreated != memories[j].IterationCreated {
			return memories[i].IterationCreated < memories[j].IterationCreated
		}
		// Tertiary: ID ascending (deterministic tiebreaker)
		return memories[i].ID < memories[j].ID
	})
}

// ============================================================================
// Type conversion helpers
// ============================================================================

func toString(v any) string {
	if v == nil {
		return ""
	}
	switch s := v.(type) {
	case string:
		return s
	case []byte:
		return string(s)
	default:
		return fmt.Sprintf("%v", v)
	}
}

func toInt(v any) int {
	switch n := v.(type) {
	case int64:
		return int(n)
	case float64:
		return int(n)
	case int:
		return n
	default:
		return 0
	}
}

func toInt64(v any) int64 {
	switch n := v.(type) {
	case int64:
		return n
	case float64:
		return int64(n)
	case int:
		return int64(n)
	default:
		return 0
	}
}
