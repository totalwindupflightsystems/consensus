// Package harness: dynamic system prompt assembly (SPEC-012).
//
// The system prompt is assembled dynamically from multiple sources:
//
//	Layer 1: Identity + rules (static, always cached)
//	Layer 2: Schema discovery — core tables + dynamic tables
//	Layer 3: Tools registry metadata (progressive disclosure)
//	Layer 4: Skills registry metadata (metadata only, not full instructions)
//	Layer 5: Session constraints + budget
//	Layer 6: Current context (dynamic, changes every iteration)
//
// axiom:trace work_item=runtime-harness-01 spec=specs/012-system-prompt-and-discovery.md plan=phase-2/task-2-1/step-2-1-1 impl=internal/harness/prompt.go
package harness

import (
	"context"
	"fmt"
	"strings"
)

// ============================================================================
// System Prompt Builder
// ============================================================================

// SystemPromptConfig holds all inputs for system prompt assembly.
type SystemPromptConfig struct {
	// Session identity
	AgentName string
	ModelID   string
	SessionID string
	Goal      string
	Status    string

	// Budget
	ContextBudget    int
	BudgetLimitCents int64
	BudgetUsedCents  int64

	// Constraints
	MaxIterations        int
	MaxConsecutiveErrors int
	ConsecutiveErrors    int
	PlanningMaxTurns     int
	Iteration            int64

	// Whether to include Layer 6 dynamically
	IncludeCurrentContext bool

	// Sub-agent mode (SPEC-012 §6): filtered tool access
	IsSubAgent bool
}

// PromptLayers assembles the full system prompt from all layers.
// Each layer is a separate string block for cache-friendly concatenation.
type PromptLayers struct {
	Layer1Identity    string // static: agent identity + rules
	Layer2Schema      string // schema discovery (core + dynamic tables)
	Layer3Tools       string // tools registry metadata
	Layer4Skills      string // skills registry metadata (metadata only)
	Layer5Constraints string // session constraints + budget
	Layer6Context     string // current context (dynamic, changes each iteration)
}

// SystemPromptBuilder assembles the system prompt for the LLM.
type SystemPromptBuilder struct {
	harness *Harness
}

// NewSystemPromptBuilder creates a new prompt builder.
func NewSystemPromptBuilder(h *Harness) *SystemPromptBuilder {
	return &SystemPromptBuilder{harness: h}
}

// Build assembles the complete system prompt from all layers.
// Returns a single formatted string suitable for the system message.
func (b *SystemPromptBuilder) Build(ctx context.Context, config *SystemPromptConfig) (string, error) {
	layers, err := b.buildLayers(ctx, config)
	if err != nil {
		return "", fmt.Errorf("prompt: build layers: %w", err)
	}

	return layers.String(), nil
}

// BuildMessages returns a fully assembled set of messages for the LLM call.
func (b *SystemPromptBuilder) BuildMessages(ctx context.Context, config *SystemPromptConfig, memories []MemoryEventInfo, tools []ToolInfo) ([]Message, error) {
	layers, err := b.buildLayers(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("prompt: build messages: %w", err)
	}

	return []Message{
		{Role: "system", Content: layers.SystemPrompt()},
		{Role: "user", Content: b.formatUserContext(config, memories, tools, layers)},
	}, nil
}

// buildLayers assembles each layer independently.
func (b *SystemPromptBuilder) buildLayers(ctx context.Context, config *SystemPromptConfig) (*PromptLayers, error) {
	if config == nil {
		return nil, fmt.Errorf("prompt: config is nil")
	}

	layers := &PromptLayers{}

	// Layer 1: Identity + Rules (always cached)
	layers.Layer1Identity = b.buildIdentityLayer(config)

	// Layer 2: Schema Discovery
	schemaData, err := b.harness.discoverSchema(ctx, config.SessionID)
	if err != nil {
		// Schema discovery failure is non-fatal — fall back to hardcoded columns.
		coreInfos := make([]TableInfo, len(coreTableNames))
		for i, name := range coreTableNames {
			coreInfos[i] = TableInfo{
				Name:    name,
				Columns: coreTableColumns[name],
			}
		}
		schemaData = &SchemaData{
			CoreTables:    coreInfos,
			DynamicTables: nil,
		}
	}
	layers.Layer2Schema = b.buildSchemaLayer(schemaData)

	// Layer 3: Tools Registry (SPEC-012 §6 — sub-agents get filtered tools)
	tools, err := b.harness.readTools(ctx)
	if err != nil {
		tools = nil
	}

	// Auto-detect sub-agent status if not explicitly set in config
	isSubAgent := config.IsSubAgent
	if !isSubAgent && config.SessionID != "" && b.harness.db != nil {
		rows, qErr := b.harness.db.Query(ctx,
			`SELECT COALESCE(parent_id, '') = '' AS is_root FROM sessions WHERE id = $1`,
			config.SessionID)
		if qErr == nil && len(rows) > 0 {
			if parentEmpty, ok := rows[0]["is_root"]; ok {
				if v, ok2 := parentEmpty.(bool); ok2 {
					isSubAgent = !v
				}
			}
		}
	}

	// If this session is a sub-agent, filter to internal hemisphere only
	filteredTools := tools
	subAgentNotice := ""
	if isSubAgent {
		internal := make([]ToolInfo, 0, len(tools))
		for _, t := range tools {
			if t.Hemisphere == "internal" {
				internal = append(internal, t)
			}
		}
		filteredTools = internal
		subAgentNotice = " (sub-agent mode — internal tools only)"
	}
	layers.Layer3Tools = b.buildToolsLayer(filteredTools, subAgentNotice)

	// Layer 4: Skills Registry (metadata only)
	skills, err := b.harness.readSkillsMetadata(ctx)
	if err != nil {
		skills = nil
	}
	skillsNotice := ""
	if isSubAgent {
		skillsNotice = "sub-agent skills may be restricted — load on demand"
	}
	layers.Layer4Skills = b.buildSkillsLayer(skills, skillsNotice)

	// Layer 5: Constraints + Budget
	layers.Layer5Constraints = b.buildConstraintsLayer(config)

	// Layer 6: Current Context
	if config.IncludeCurrentContext {
		layers.Layer6Context = b.buildContextLayer(config)
	}

	return layers, nil
}

// ============================================================================
// Layer 1: Identity + Rules
// ============================================================================

func (b *SystemPromptBuilder) buildIdentityLayer(config *SystemPromptConfig) string {
	var sb strings.Builder

	sb.WriteString("# Consensus Agent Runtime\n\n")
	sb.WriteString("You are a **Consensus agent** running in a database-native cognitive architecture.\n")
	sb.WriteString("Your mind is a PostgreSQL (or SQLite) database. You think by writing SQL.\n\n")

	// Agent identity
	sb.WriteString(fmt.Sprintf("**Agent:** %s\n", config.AgentName))
	sb.WriteString(fmt.Sprintf("**Model:** %s\n", config.ModelID))
	sb.WriteString(fmt.Sprintf("**Session:** %s\n", config.SessionID))
	sb.WriteString(fmt.Sprintf("**Goal:** %s\n\n", config.Goal))

	// Core principles
	sb.WriteString("## Core Principles\n\n")
	sb.WriteString("- **Atomic Cognition:** Every action is a SQL transaction. Either all succeed or all roll back.\n")
	sb.WriteString("- **Append-Only Memory:** The `memory_events` table is immutable. You can INSERT but never UPDATE or DELETE.\n")
	sb.WriteString("- **RLS Isolation:** All queries are scoped to your `session_id`. You cannot see other agents' data.\n")
	sb.WriteString("- **Deterministic State:** Your state lives in database rows, not prompt strings. Use SQL, not prose.\n\n")

	// Output format
	sb.WriteString("## Output Format\n\n")
	sb.WriteString("Always output valid JSON matching this schema:\n\n")
	sb.WriteString("```json\n")
	sb.WriteString("{\n")
	sb.WriteString(`  "internal_monologue": "Your private reasoning (never shown to user)",` + "\n")
	sb.WriteString(`  "memory_state_changes": ["SQL statements to modify your memory"],` + "\n")
	sb.WriteString(`  "system_actions": ["Session-level SQL operations"],` + "\n")
	sb.WriteString(`  "tool_requests": [{"tool_name": "name", "parameters": {...}}],` + "\n")
	sb.WriteString(`  "sub_agent_spawns": [{"agent_name": "name", "goal": "task"}]` + "\n")
	sb.WriteString("}\n")
	sb.WriteString("```\n\n")

	// SQL rules
	sb.WriteString("## SQL Rules\n\n")
	sb.WriteString("- All SQL is classified: DML_READ (always allowed), DML_WRITE (whitelisted tables only), DDL (restricted), DANGEROUS (never allowed)\n")
	sb.WriteString("- Multi-statement SQL is split on semicolons and classified per-statement\n")
	sb.WriteString("- `{{SECRET.X}}` aliases are replaced with real secrets before execution (you never see the real values)\n")
	sb.WriteString("- Dates must be ISO 8601 format. Quote identifiers with double-quotes.\n")
	sb.WriteString("- Use `$1, $2` parameter placeholders for external data to prevent injection.\n\n")

	sb.WriteString("## Safety\n\n")
	sb.WriteString("- You cannot execute: TRUNCATE, GRANT, REVOKE, VACUUM, SET ROLE, or any DANGEROUS operations.\n")
	sb.WriteString("- Writes to non-whitelisted tables are blocked.\n")
	sb.WriteString("- On error, the transaction rolls back. The harness injects the error into your next context for recovery.\n")

	return sb.String()
}

// ============================================================================
// Layer 2: Schema Discovery
// ============================================================================

// SchemaData holds discovered table information.
type SchemaData struct {
	CoreTables    []TableInfo
	DynamicTables []TableInfo
}

// TableInfo holds a table's metadata for prompt assembly.
type TableInfo struct {
	Name    string
	Columns string // comma-joined column list
}

var coreTableNames = []string{
	"memory_events", "display_modes", "iteration_commits", "memory_pages",
	"tasks", "tool_requests", "tool_results", "agent_billing",
	"staging_buffer", "audit_logs", "agent_messages", "compression_queue",
	"custom_agent_tools",
}

// coreTableColumns provides fallback column lists when DB schema discovery fails.
// Maps table name → comma-separated column list.
// LLMs need this to generate valid SQL (e.g., memory_events.type not event_type).
var coreTableColumns = map[string]string{
	"memory_events":      "id, type, content, summary_text, session_id, iteration_created, linked_memory_pages, embedding, created_at",
	"display_modes":      "memory_id, mode, set_at, set_by_iteration, session_id",
	"iteration_commits":  "iteration_id, session_id, active_pointers, display_rules, llm_response, sql_executed, rows_affected, created_at",
	"memory_pages":       "id, name, target_ids, linked_page_ids, session_id, created_at",
	"tasks":              "id, session_id, description, status, assigned_to, result, created_at",
	"tool_requests":      "id, session_id, iteration_id, tool_name, parameters, status, created_at",
	"tool_results":       "id, request_id, session_id, result, error, created_at",
	"agent_billing":      "id, session_id, provider, model, prompt_tokens, completion_tokens, cost_cents, created_at",
	"staging_buffer":     "id, session_id, command_type, payload, status, turn, created_at",
	"audit_logs":         "id, session_id, iteration, monologue, sql_executed, result, error_message, created_at",
	"agent_messages":     "id, target_session_id, sender_session_id, payload, read, created_at",
	"compression_queue":  "id, event_id, current_tier, next_tier, status, attempts, max_attempts, created_at",
	"custom_agent_tools": "id, name, description, enabled, status, created_at",
}

func (b *SystemPromptBuilder) buildSchemaLayer(schema *SchemaData) string {
	var sb strings.Builder

	sb.WriteString("## Database Schema\n\n")
	sb.WriteString("You can read from and (where allowed) write to the following tables:\n\n")

	// Core tables with columns
	sb.WriteString("### Core Tables (Whitelisted)\n\n")
	sb.WriteString("| Table | Columns | Writable |\n")
	sb.WriteString("|---|---|---|\n")
	coreWritable := map[string]string{
		"memory_events":      "INSERT only (append-only)",
		"display_modes":      "Full CRUD",
		"iteration_commits":  "INSERT only",
		"memory_pages":       "Full CRUD (scoped)",
		"tasks":              "Full CRUD (scoped)",
		"tool_requests":      "INSERT only",
		"tool_results":       "INSERT only (by tool_executor)",
		"agent_billing":      "INSERT only",
		"staging_buffer":     "Full CRUD (scoped)",
		"audit_logs":         "INSERT only",
		"agent_messages":     "INSERT only",
		"compression_queue":  "INSERT only",
		"custom_agent_tools": "INSERT only (requires approval)",
	}

	for _, t := range schema.CoreTables {
		writable := coreWritable[t.Name]
		if writable == "" {
			writable = "Read only"
		}
		cols := t.Columns
		if cols == "" {
			// Fall back to hardcoded schema for core tables
			if known, ok := coreTableColumns[t.Name]; ok {
				cols = known
			} else {
				cols = "(discover failed)"
			}
		}
		sb.WriteString(fmt.Sprintf("| `%s` | %s | %s |\n", t.Name, cols, writable))
	}
	sb.WriteString("\n")

	// Dynamic tables
	if len(schema.DynamicTables) > 0 {
		sb.WriteString("### Dynamic Tables (Agent-Created)\n\n")
		sb.WriteString("| Table | Columns |\n")
		sb.WriteString("|---|---|\n")
		for _, t := range schema.DynamicTables {
			sb.WriteString(fmt.Sprintf("| `%s` | %s |\n", t.Name, t.Columns))
		}
		sb.WriteString("\n")
	}

	// Key relationships
	sb.WriteString("### Key Relationships\n\n")
	sb.WriteString("- `sessions.id` → `memory_events.session_id`, `display_modes.session_id`, all other tables\n")
	sb.WriteString("- `memory_events.id` → `display_modes.memory_id`\n")
	sb.WriteString("- `staging_buffer.session_id` — scoped staging area for multi-turn planning\n")
	sb.WriteString("- `tasks.session_id` — async task queue\n\n")

	return sb.String()
}

// ============================================================================
// Layer 3: Tools Registry (Metadata)
// ============================================================================

func (b *SystemPromptBuilder) buildToolsLayer(tools []ToolInfo, notice ...string) string {
	var sb strings.Builder

	sb.WriteString("## Available Tools\n\n")

	// Sub-agent notice (SPEC-012 §6)
	if len(notice) > 0 && notice[0] != "" {
		sb.WriteString(fmt.Sprintf("> **Notice:** %s\n\n", notice[0]))
	}

	if len(tools) == 0 {
		sb.WriteString("(No tools registered)\n\n")
		return sb.String()
	}

	sb.WriteString("| Tool | Hemisphere | Description |\n")
	sb.WriteString("|---|---|---|\n")
	for _, t := range tools {
		sb.WriteString(fmt.Sprintf("| `%s` | %s | %s |\n", t.Name, t.Hemisphere, t.Description))
	}
	sb.WriteString("\n")

	sb.WriteString("- **Internal** tools run as SQL functions inside the database transaction (fast, atomic)\n")
	sb.WriteString("- **External** tools run in sandboxed subprocesses (network calls, file I/O)\n")
	sb.WriteString("- Request tools via `tool_requests` in your JSON output\n\n")

	return sb.String()
}

// ============================================================================
// Layer 4: Skills Registry (Metadata Only — Progressive Disclosure)
// ============================================================================

// SkillMetadata holds lightweight skill info for progressive disclosure.
type SkillMetadata struct {
	Name        string
	Description string
	WhenToUse   string
	Version     string
}

func (b *SystemPromptBuilder) buildSkillsLayer(skills []SkillMetadata, notice ...string) string {
	var sb strings.Builder

	sb.WriteString("## Available Skills (Metadata Only)\n\n")

	// Sub-agent notice (SPEC-012 §6)
	if len(notice) > 0 && notice[0] != "" {
		sb.WriteString(fmt.Sprintf("> **Notice:** %s\n\n", notice[0]))
	}

	if len(skills) == 0 {
		sb.WriteString("(No skills registered)\n\n")
		return sb.String()
	}

	sb.WriteString("Skills provide specialized workflows and instructions. Only metadata is shown here.\n")
	sb.WriteString("To load a skill's full instructions, use: `SELECT load_skill('skill_name')`\n\n")

	sb.WriteString("| Skill | When to Use |\n")
	sb.WriteString("|---|---|\n")
	for _, s := range skills {
		sb.WriteString(fmt.Sprintf("| `%s` | %s |\n", s.Name, s.WhenToUse))
	}
	sb.WriteString("\n")

	return sb.String()
}

// ============================================================================
// Layer 5: Constraints + Budget
// ============================================================================

func (b *SystemPromptBuilder) buildConstraintsLayer(config *SystemPromptConfig) string {
	var sb strings.Builder

	sb.WriteString("## Session Constraints\n\n")
	sb.WriteString(fmt.Sprintf("- **Iteration:** %d / %d (max turns: %d)\n",
		config.Iteration, config.MaxIterations, config.PlanningMaxTurns))
	sb.WriteString(fmt.Sprintf("- **Tokens used:** %d / %d context budget\n",
		0, config.ContextBudget)) // tokens_used comes from LLM response, not tracked pre-call
	sb.WriteString(fmt.Sprintf("- **Budget:** %d / %d cents\n",
		config.BudgetUsedCents, config.BudgetLimitCents))
	sb.WriteString(fmt.Sprintf("- **Consecutive errors:** %d / %d (circuit breaker at %d)\n",
		config.ConsecutiveErrors, config.MaxConsecutiveErrors, config.MaxConsecutiveErrors))
	sb.WriteString("- **Status transitions:** booting → idle → thinking → (planning → executing) → idle/complete\n")
	sb.WriteString("- **Append-only enforcement:** UPDATE/DELETE on memory_events is REJECTED by the database kernel.\n")
	sb.WriteString("- **Secrets:** You reference secrets via `{{SECRET.NAME}}` aliases. The harness replaces them before execution. Real values are scrubbed from your responses before storage.\n")

	return sb.String()
}

// ============================================================================
// Layer 6: Current Context (Dynamic)
// ============================================================================

func (b *SystemPromptBuilder) buildContextLayer(config *SystemPromptConfig) string {
	return fmt.Sprintf("## Current State\n\nStatus: %s | Goal: %s\n", config.Status, config.Goal)
}

// ============================================================================
// User Message Context Builder
// ============================================================================

func (b *SystemPromptBuilder) formatUserContext(config *SystemPromptConfig, memories []MemoryEventInfo, tools []ToolInfo, layers *PromptLayers) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("# Active Context — Session %s\n\n", config.SessionID))

	// Goal + Status
	sb.WriteString("## Current Task\n")
	sb.WriteString(fmt.Sprintf("**Goal:** %s\n", config.Goal))
	sb.WriteString(fmt.Sprintf("**Status:** %s | Iteration: %d / %d\n\n", config.Status, config.Iteration, config.MaxIterations))

	// Memory ledger snapshot
	sb.WriteString(fmt.Sprintf("## Recent Memory (%d events)\n", len(memories)))
	if len(memories) == 0 {
		sb.WriteString("(No memory events recorded yet)\n\n")
	} else {
		for _, m := range memories {
			formatted := formatMemoryEvent(m)
			if formatted != "" {
				sb.WriteString(formatted)
			}
		}
		sb.WriteString("\n")
	}

	// Current state summary
	sb.WriteString("## Current State\n")
	sb.WriteString(fmt.Sprintf("- Status: %s\n", config.Status))
	sb.WriteString(fmt.Sprintf("- Iteration: %d / %d\n", config.Iteration, config.MaxIterations))
	sb.WriteString(fmt.Sprintf("- Consecutive errors: %d / %d\n", config.ConsecutiveErrors, config.MaxConsecutiveErrors))
	sb.WriteString(fmt.Sprintf("- Budget remaining: %d cents\n", config.BudgetLimitCents-config.BudgetUsedCents))

	return sb.String()
}

// ============================================================================
// PromptLayers String Methods
// ============================================================================

// SystemPrompt returns the concatenated system prompt (all layers except context).
func (l *PromptLayers) SystemPrompt() string {
	var sb strings.Builder
	sb.WriteString(l.Layer1Identity)
	sb.WriteString("\n")
	sb.WriteString(l.Layer2Schema)
	sb.WriteString("\n")
	sb.WriteString(l.Layer3Tools)
	sb.WriteString("\n")
	sb.WriteString(l.Layer4Skills)
	sb.WriteString("\n")
	sb.WriteString(l.Layer5Constraints)
	return sb.String()
}

// String returns all layers concatenated.
func (l *PromptLayers) String() string {
	var sb strings.Builder
	sb.WriteString(l.SystemPrompt())
	if l.Layer6Context != "" {
		sb.WriteString("\n")
		sb.WriteString(l.Layer6Context)
	}
	return sb.String()
}

// ============================================================================
// Schema Discovery (SPEC-012 §Schema Discovery)
// ============================================================================

// discoverSchema queries the database for core and dynamic table metadata.
func (h *Harness) discoverSchema(ctx context.Context, sessionID string) (*SchemaData, error) {
	if h.db == nil {
		return nil, fmt.Errorf("discover: no database configured")
	}

	schema := &SchemaData{
		CoreTables: make([]TableInfo, 0, len(coreTableNames)),
	}

	// Query information_schema for table metadata
	// On Postgres, this gives us real column lists
	// On SQLite, we may need sqlite_master instead
	rows, err := h.db.Query(ctx, `
		SELECT table_name
		FROM information_schema.tables
		WHERE table_schema = 'public'
		ORDER BY table_name
	`)
	if err != nil {
		// SQLite doesn't have information_schema — try sqlite_master
		rows, err = h.db.Query(ctx, `
			SELECT name AS table_name
			FROM sqlite_master
			WHERE type = 'table'
			ORDER BY name
		`)
		if err != nil {
			return nil, fmt.Errorf("discover: %w", err)
		}
	}

	// Classify tables
	coreSet := make(map[string]bool)
	for _, n := range coreTableNames {
		coreSet[n] = true
	}

	for _, row := range rows {
		tableName := toString(row["table_name"])

		if coreSet[tableName] {
			// Also discover columns for core tables — the LLM needs schema
			// details to generate valid SQL (e.g., memory_events.type, not
			// memory_events.event_type).
			columns := h.discoverColumns(ctx, tableName)
			schema.CoreTables = append(schema.CoreTables, TableInfo{
				Name:    tableName,
				Columns: columns,
			})
		} else {
			// Dynamic table — query columns
			columns := h.discoverColumns(ctx, tableName)
			schema.DynamicTables = append(schema.DynamicTables, TableInfo{
				Name:    tableName,
				Columns: columns,
			})
		}
	}

	return schema, nil
}

// discoverColumns returns a comma-separated list of columns for a table.
func (h *Harness) discoverColumns(ctx context.Context, tableName string) string {
	// Postgres path
	rows, err := h.db.Query(ctx, `
		SELECT column_name
		FROM information_schema.columns
		WHERE table_schema = 'public' AND table_name = $1
		ORDER BY ordinal_position
	`, tableName)
	if err != nil {
		// SQLite fallback
		rows, err = h.db.Query(ctx, fmt.Sprintf("PRAGMA table_info('%s')", tableName))
		if err != nil {
			return "(columns unknown)"
		}
	}

	cols := make([]string, 0, len(rows))
	colKey := "column_name"
	if len(rows) > 0 {
		if _, ok := rows[0]["column_name"]; !ok {
			colKey = "name" // SQLite PRAGMA uses 'name'
		}
	}
	for _, row := range rows {
		cols = append(cols, toString(row[colKey]))
	}

	return strings.Join(cols, ", ")
}

// readSkillsMetadata queries the skills_registry for metadata only (not full instructions).
func (h *Harness) readSkillsMetadata(ctx context.Context) ([]SkillMetadata, error) {
	if h.db == nil {
		return nil, fmt.Errorf("readSkills: no database configured")
	}
	rows, err := h.db.Query(ctx, `
		SELECT name, metadata
		FROM skills_registry
		WHERE enabled = true
		ORDER BY name
	`)
	if err != nil {
		return nil, err
	}

	skills := make([]SkillMetadata, 0, len(rows))
	for _, row := range rows {
		name := toString(row["name"])
		// metadata is JSONB — parse it for description/when_to_use
		metaStr := toString(row["metadata"])

		skill := SkillMetadata{
			Name:      name,
			WhenToUse: "See full instructions",
		}

		// Try to extract fields from metadata JSON
		// Simple key extraction for common patterns
		if strings.Contains(metaStr, `"description"`) {
			skill.Description = extractJSONField(metaStr, "description")
		}
		if strings.Contains(metaStr, `"when_to_use"`) {
			skill.WhenToUse = extractJSONField(metaStr, "when_to_use")
		}
		if strings.Contains(metaStr, `"version"`) {
			skill.Version = extractJSONField(metaStr, "version")
		}

		if skill.Description == "" {
			skill.Description = name
		}

		skills = append(skills, skill)
	}

	return skills, nil
}

// extractJSONField extracts a simple string field value from JSON.
// This is a lightweight extractor — for full JSON parsing, use encoding/json.
func extractJSONField(jsonStr, field string) string {
	// Look for "field": "value" pattern
	search := fmt.Sprintf(`"%s":`, field)
	idx := strings.Index(jsonStr, search)
	if idx < 0 {
		return ""
	}

	// Find the value after the colon
	start := idx + len(search)
	valStart := -1
	valEnd := -1
	inQuote := false
	for i := start; i < len(jsonStr); i++ {
		c := jsonStr[i]
		if !inQuote {
			if c == '"' {
				inQuote = true
				valStart = i + 1
			} else if c == ',' || c == '}' {
				break
			}
		} else {
			if c == '"' && (i == 0 || jsonStr[i-1] != '\\') {
				valEnd = i
				break
			}
		}
	}

	if valStart >= 0 && valEnd > valStart {
		return jsonStr[valStart:valEnd]
	}
	return ""
}
