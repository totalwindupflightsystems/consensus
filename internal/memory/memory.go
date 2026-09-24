// Package memory implements context formatting and memory page management (SPEC-002).
//
// Provides the canonical context view formatting for LLM consumption, memory page
// resolution with deduplication, and helper types for the active context view.
//
// axiom:trace work_item=polish-phase spec=specs/002-memory.md plan=phase-1/task-1/step-1 impl=internal/memory/memory.go
package memory

import (
	"fmt"
	"strings"
	"time"
)

// ============================================================================
// Context View Types
// ============================================================================

// MemoryEvent is a single event from the memory ledger.
type MemoryEvent struct {
	ID               int64
	Type             string
	Content          string
	SummaryText      string
	DisplayMode      string // "full", "compressed", "hidden"
	IterationCreated int64
	SessionID        string
	CreatedAt        time.Time
	RenderedText     string
}

// SessionInfo holds session metadata for context formatting.
type SessionInfo struct {
	ID            string
	AgentName     string
	Status        string
	Goal          string
	Iteration     int64
	TokensUsedIn  int64
	TokensUsedOut int64
	CreatedAt     time.Time
}

// ToolInfo holds tool metadata for context formatting.
type ToolInfo struct {
	Name        string
	Description string
	Hemisphere  string
	HandlerType string
}

// SkillInfo holds skill metadata for progressive disclosure.
type SkillInfo struct {
	Name         string
	Description  string
	Instructions string
	Enabled      bool
}

// MemoryPage holds a named group of memory IDs.
type MemoryPage struct {
	ID            int64
	Name          string
	TargetIDs     []int64
	LinkedPageIDs []int64
	CreatedAt     time.Time
}

// PageMemoryIDs is the set of memory IDs expanded from memory_pages.
type PageMemoryIDs map[int64]bool

// ============================================================================
// Active Context Model
// ============================================================================

// ActiveContext is the complete context view for a session iteration.
type ActiveContext struct {
	Session      *SessionInfo
	MemoryEvents []MemoryEvent
	Tools        []ToolInfo
	Skills       []SkillInfo
	MemoryPages  []MemoryPage
	Constraints  ContextConstraints
}

// ContextConstraints are runtime constraints shown to the agent.
type ContextConstraints struct {
	Iteration         int64
	MaxIterations     int64
	BudgetUsedCents   int64
	BudgetLimitCents  int64
	ConsecutiveErrors int
	MaxConsErrors     int
	PlanningMaxTurns  int
	ContextBudget     int64
}

// ============================================================================
// Memory Page Resolution (SPEC-002 §5)
// ============================================================================

// ResolvePageMemoryIDs expands memory_pages into a deduplicated set of memory
// event IDs. Supports single-level nesting: linked_page_ids resolve one level deep.
//
// The returned map can be used to identify which memory events are referenced
// by any page, enabling deduplication when multiple pages share events.
func ResolvePageMemoryIDs(pages []MemoryPage) PageMemoryIDs {
	ids := make(PageMemoryIDs)

	// First pass: collect all direct target IDs
	for _, page := range pages {
		for _, tid := range page.TargetIDs {
			if tid > 0 {
				ids[tid] = true
			}
		}
	}

	// Second pass: resolve linked_page_ids (depth 1)
	pageByID := make(map[int64]*MemoryPage, len(pages))
	for i := range pages {
		pageByID[pages[i].ID] = &pages[i]
	}

	for _, page := range pages {
		for _, lid := range page.LinkedPageIDs {
			if linked, ok := pageByID[lid]; ok {
				for _, tid := range linked.TargetIDs {
					if tid > 0 {
						ids[tid] = true
					}
				}
			}
		}
	}

	return ids
}

// ============================================================================
// Display Mode Resolution (SPEC-002 §4)
// ============================================================================

// ResolveDisplayText returns the text that should be displayed based on
// the event's display_mode. The memory_events table never mutates —
// display_modes only change which text is shown.
//
// Rules:
//   - "full" → original content
//   - "compressed" → summary_text (if available), else content
//   - "hidden" → empty string (event excluded from context)
func ResolveDisplayText(event MemoryEvent) string {
	switch event.DisplayMode {
	case "hidden":
		return ""
	case "compressed":
		if event.SummaryText != "" {
			return event.SummaryText
		}
		return event.Content
	default: // "full" or unknown
		return event.Content
	}
}

// IsVisible returns whether a memory event should be included in context
// based on its display mode.
func IsVisible(displayMode string) bool {
	return displayMode != "hidden"
}

// ============================================================================
// Context Formatting (SPEC-008 §ContextFormatting, SPEC-012)
// ============================================================================

// FormatContextAsMarkdown formats the active context as structured Markdown
// for LLM consumption. The output follows the prefix hierarchy for token caching:
//
//	Layer 1 (always cached): System instructions, tools
//	Layer 2 (append-cached): Memory events in iteration order
//	Layer 3 (never cached): Constraints, iteration counters
func FormatContextAsMarkdown(ctx *ActiveContext) string {
	var b strings.Builder

	b.WriteString("# Active Context\n\n")

	// Session info
	if ctx.Session != nil {
		b.WriteString(fmt.Sprintf("## Session: %s (%s)\n", ctx.Session.AgentName, ctx.Session.ID))
		if ctx.Session.Goal != "" {
			b.WriteString(fmt.Sprintf("**Goal:** %s\n", ctx.Session.Goal))
		}
		b.WriteString(fmt.Sprintf("**Status:** %s | **Iteration:** %d\n\n", ctx.Session.Status, ctx.Session.Iteration))
	}

	// Available tools (cache layer 1)
	if len(ctx.Tools) > 0 {
		b.WriteString("## Available Tools\n\n")
		for _, t := range ctx.Tools {
			b.WriteString(fmt.Sprintf("- **%s** (%s): %s\n", t.Name, t.Hemisphere, t.Description))
		}
		b.WriteString("\n")
	}

	// Skills metadata (progressive disclosure)
	if len(ctx.Skills) > 0 {
		b.WriteString("## Available Skills\n\n")
		for _, s := range ctx.Skills {
			if s.Enabled {
				b.WriteString(fmt.Sprintf("- **%s**: %s\n", s.Name, s.Description))
			}
		}
		b.WriteString("\n")
	}

	// Memory events (cache layer 2)
	if len(ctx.MemoryEvents) > 0 {
		b.WriteString("## Memory Events\n\n")
		for _, evt := range ctx.MemoryEvents {
			text := ResolveDisplayText(evt)
			if text == "" {
				continue // hidden events excluded
			}
			displayLabel := ""
			if evt.DisplayMode == "compressed" {
				displayLabel = " [compressed]"
			}
			b.WriteString(fmt.Sprintf("### [%s] %s%s\n", evt.Type, formatTimestamp(evt.CreatedAt), displayLabel))
			b.WriteString(fmt.Sprintf("%s\n\n", text))
		}
	}

	// Memory pages
	if len(ctx.MemoryPages) > 0 {
		b.WriteString("## Compressed Memory Pages\n\n")
		for _, p := range ctx.MemoryPages {
			b.WriteString(fmt.Sprintf("- **%s** (events: %d)\n", p.Name, len(p.TargetIDs)))
		}
		b.WriteString("\n")
	}

	// Constraints (cache layer 3 — always at end)
	b.WriteString("## Constraints\n\n")
	b.WriteString(fmt.Sprintf("- Iteration: %d / %d\n", ctx.Constraints.Iteration, ctx.Constraints.MaxIterations))
	b.WriteString(fmt.Sprintf("- Budget used: %d / %d cents\n", ctx.Constraints.BudgetUsedCents, ctx.Constraints.BudgetLimitCents))
	b.WriteString(fmt.Sprintf("- Consecutive errors: %d / %d\n", ctx.Constraints.ConsecutiveErrors, ctx.Constraints.MaxConsErrors))
	b.WriteString(fmt.Sprintf("- Planning turns: %d max\n", ctx.Constraints.PlanningMaxTurns))
	b.WriteString(fmt.Sprintf("- Context budget: %d tokens\n", ctx.Constraints.ContextBudget))
	b.WriteString("\n")

	return b.String()
}

// ============================================================================
// Filtering & Query Helpers
// ============================================================================

// FilterVisible returns only the visible memory events (excluding hidden).
func FilterVisible(events []MemoryEvent) []MemoryEvent {
	result := make([]MemoryEvent, 0, len(events))
	for _, e := range events {
		if IsVisible(e.DisplayMode) {
			result = append(result, e)
		}
	}
	return result
}

// FilterByType returns events matching any of the given types.
func FilterByType(events []MemoryEvent, types ...string) []MemoryEvent {
	typeSet := make(map[string]bool, len(types))
	for _, t := range types {
		typeSet[t] = true
	}
	result := make([]MemoryEvent, 0, len(events))
	for _, e := range events {
		if typeSet[e.Type] {
			result = append(result, e)
		}
	}
	return result
}

// InPageSet checks if a memory event ID is in the resolved page set.
func InPageSet(eventID int64, pageIDs PageMemoryIDs) bool {
	return pageIDs[eventID]
}

// ============================================================================
// Helpers
// ============================================================================

func formatTimestamp(t time.Time) string {
	if t.IsZero() {
		return "unknown time"
	}
	return t.Format(time.RFC3339)
}

// ============================================================================
// Type-to-Markdown Mapping (SPEC-002 §7.2) — AC-025
// ============================================================================

// FormatMemoryEventByType renders a single memory event's content according to
// its type. Each type gets a distinct markdown presentation:
//
//	header            → ## heading
//	text_block        → plain paragraph
//	tool_call         → **bold** inline
//	tool_result       → ```code block```
//	thinking          → <!-- HTML comment -->
//	system            → > blockquote
//	user_message      → plain paragraph
//	inherited_pointer → compact reference [→parent]
//
// The rendered_text field of MemoryEvent should be populated by calling this
// function when building the active context view on SQLite (where the PG view
// is unavailable due to the SQLite migration filter).
func FormatMemoryEventByType(evt MemoryEvent) string {
	text := ResolveDisplayText(evt)
	if text == "" {
		return "" // hidden events excluded
	}

	switch evt.Type {
	case "header":
		return fmt.Sprintf("## %s\n\n", text)

	case "text_block", "user_message":
		return fmt.Sprintf("%s\n\n", text)

	case "tool_call":
		return fmt.Sprintf("**%s**\n\n", text)

	case "tool_result":
		return fmt.Sprintf("```\n%s\n```\n\n", text)

	case "thinking":
		return fmt.Sprintf("<!-- %s -->\n\n", text)

	case "system":
		return fmt.Sprintf("> %s\n\n", text)

	case "inherited_pointer":
		return fmt.Sprintf("[→ %s]\n\n", text)

	default:
		// Unknown type — fall back to plain text
		return fmt.Sprintf("%s\n\n", text)
	}
}
