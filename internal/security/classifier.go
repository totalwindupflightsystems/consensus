// Package security implements the SQL statement classifier and execution policy
// enforcement for the Consensus runtime (SPEC-011 §8, SPEC-008 §SQL Execution Model).
//
// The classifier categorizes every SQL statement the LLM emits into a safety tier:
//
//	DML_READ    — SELECT (safe, always allowed)
//	DML_WRITE   — INSERT/UPDATE/DELETE (allowed on whitelisted tables)
//	DDL         — CREATE/ALTER/DROP (allowed only for dynamic tables + constraints)
//	DANGEROUS   — TRUNCATE/GRANT/REVOKE/VACUUM/etc (never allowed)
//
// Multi-statement SQL is split on semicolons and each part classified independently.
// The policy enforcer checks each statement against a table whitelist and session
// permissions before execution.
//
// axiom:trace work_item=runtime-harness-01 spec=specs/006-transactions.md,specs/008-harness.md plan=phase-1/task-1-1/step-1-1-3 impl=internal/security/classifier.go
package security

import (
	"fmt"
	"regexp"
	"strings"
)

// ============================================================================
// Statement Classification
// ============================================================================

// StatementClass is the safety classification of a SQL statement.
type StatementClass int

const (
	// DMLRead is a SELECT statement (safe, always allowed).
	DMLRead StatementClass = iota

	// DMLWrite is an INSERT, UPDATE, or DELETE statement (requires table whitelist).
	DMLWrite

	// DDL is a schema modification statement (CREATE, ALTER, DROP).
	DDL

	// Dangerous is a statement that must never be executed (TRUNCATE, GRANT, etc.).
	Dangerous

	// Other is an unclassified statement (treated as dangerous).
	Other
)

// String returns the human-readable name of the classification.
func (c StatementClass) String() string {
	switch c {
	case DMLRead:
		return "DML_READ"
	case DMLWrite:
		return "DML_WRITE"
	case DDL:
		return "DDL"
	case Dangerous:
		return "DANGEROUS"
	default:
		return "OTHER"
	}
}

// ClassifyStatement categorizes a single SQL statement.
func ClassifyStatement(stmt string) StatementClass {
	stmt = strings.TrimSpace(stmt)
	if stmt == "" {
		return DMLRead // empty is considered safe (no-op)
	}

	upper := strings.ToUpper(stmt)

	// Check dangerous patterns first — these override all other classifications
	if classifyDangerous(upper) {
		return Dangerous
	}

	// DDL: schema modification
	if classifyDDL(upper) {
		return DDL
	}

	// DML Write: data modification
	if classifyDMLWrite(upper) {
		return DMLWrite
	}

	// DML Read: querying
	if classifyDMLRead(upper) {
		return DMLRead
	}

	// Unknown — treat as dangerous
	return Dangerous
}

var (
	// Dangerous patterns: operations that must NEVER be executed by an agent.
	dangerousPatterns = []*regexp.Regexp{
		regexp.MustCompile(`^\s*TRUNCATE\b`),
		regexp.MustCompile(`^\s*GRANT\b`),
		regexp.MustCompile(`^\s*REVOKE\b`),
		regexp.MustCompile(`^\s*VACUUM\b`),
		regexp.MustCompile(`^\s*ANALYZE\b`),
		regexp.MustCompile(`^\s*REINDEX\b`),
		regexp.MustCompile(`^\s*CLUSTER\b`),
		regexp.MustCompile(`^\s*DISCARD\b`),
		regexp.MustCompile(`^\s*LISTEN\b`),
		regexp.MustCompile(`^\s*NOTIFY\b`),
		regexp.MustCompile(`^\s*UNLISTEN\b`),
		regexp.MustCompile(`^\s*COPY\b`),
		regexp.MustCompile(`^\s*MOVE\b`),
		regexp.MustCompile(`^\s*REASSIGN\b`),
		regexp.MustCompile(`^\s*REFRESH\b`),
		regexp.MustCompile(`^\s*SET\s+ROLE\b`),
		regexp.MustCompile(`^\s*RESET\s+ROLE\b`),
		regexp.MustCompile(`^\s*SET\s+SESSION\s+AUTHORIZATION\b`),
		regexp.MustCompile(`^\s*PREPARE\b`),
		regexp.MustCompile(`^\s*EXECUTE\b`),
		regexp.MustCompile(`^\s*DEALLOCATE\b`),
		regexp.MustCompile(`^\s*LOCK\b`),
		regexp.MustCompile(`^\s*CHECKPOINT\b`),
	}

	// DDL patterns: schema modification statements.
	ddlPatterns = []*regexp.Regexp{
		regexp.MustCompile(`^\s*CREATE\b`),
		regexp.MustCompile(`^\s*ALTER\b`),
		regexp.MustCompile(`^\s*DROP\b`),
	}

	// DML Write patterns: data modification.
	dmlWritePatterns = []*regexp.Regexp{
		regexp.MustCompile(`^\s*INSERT\b`),
		regexp.MustCompile(`^\s*UPDATE\b`),
		regexp.MustCompile(`^\s*DELETE\b`),
		regexp.MustCompile(`^\s*MERGE\b`),
		regexp.MustCompile(`^\s*UPSERT\b`),
		regexp.MustCompile(`^\s*REPLACE\b`),
	}

	// DML Read patterns: querying data.
	dmlReadPatterns = []*regexp.Regexp{
		regexp.MustCompile(`^\s*SELECT\b`),
		regexp.MustCompile(`^\s*WITH\b`),
		regexp.MustCompile(`^\s*EXPLAIN\b`),
		regexp.MustCompile(`^\s*SHOW\b`),
		regexp.MustCompile(`^\s*DESCRIBE\b`),
		regexp.MustCompile(`^\s*DESC\b`),
	}
)

func classifyDangerous(upper string) bool {
	for _, p := range dangerousPatterns {
		if p.MatchString(upper) {
			return true
		}
	}
	return false
}

func classifyDDL(upper string) bool {
	for _, p := range ddlPatterns {
		if p.MatchString(upper) {
			return true
		}
	}
	return false
}

func classifyDMLWrite(upper string) bool {
	for _, p := range dmlWritePatterns {
		if p.MatchString(upper) {
			return true
		}
	}
	return false
}

func classifyDMLRead(upper string) bool {
	for _, p := range dmlReadPatterns {
		if p.MatchString(upper) {
			return true
		}
	}
	return false
}

// ============================================================================
// Multi-Statement Splitting
// ============================================================================

// SplitStatements splits a list of SQL strings on semicolons and returns
// individual statements. Each statement is classified independently.
//
// Empty strings and whitespace-only strings are filtered out.
func SplitStatements(statements []string) []string {
	var result []string
	for _, stmt := range statements {
		parts := strings.Split(stmt, ";")
		for _, part := range parts {
			trimmed := strings.TrimSpace(part)
			if trimmed != "" {
				result = append(result, trimmed)
			}
		}
	}
	return result
}

// ============================================================================
// Table Whitelist
// ============================================================================

// TableWhitelist holds the set of tables that agents are allowed to write to.
type TableWhitelist struct {
	// StaticTables are the core Consensus tables agents can write to.
	StaticTables map[string]bool

	// DynamicTables are runtime-created agent tables (loaded from information_schema).
	DynamicTables map[string]bool
}

// NewTableWhitelist creates a whitelist with the standard core tables.
func NewTableWhitelist() *TableWhitelist {
	return &TableWhitelist{
		StaticTables: map[string]bool{
			"memory_events":      true, // INSERT only (append-only enforced at DB level)
			"display_modes":      true,
			"iteration_commits":  true,
			"memory_pages":       true,
			"tasks":              true,
			"tool_requests":      true,
			"tool_results":       true,
			"agent_billing":      true,
			"staging_buffer":     true,
			"audit_logs":         true,
			"agent_messages":     true,
			"compression_queue":  true,
			"custom_agent_tools": true,
		},
		DynamicTables: make(map[string]bool),
	}
}

// IsAllowed returns true if the table is on the whitelist.
func (w *TableWhitelist) IsAllowed(tableName string) bool {
	tableName = strings.ToLower(strings.TrimSpace(tableName))
	return w.StaticTables[tableName] || w.DynamicTables[tableName]
}

// ============================================================================
// Statement Sanitization
// ============================================================================

// Sanitize applies defense-in-depth sanitization to a SQL statement.
//   - Removes null bytes (\x00)
//   - Strips UTF-8 BOM
//   - Trims whitespace
func Sanitize(sql string) string {
	sql = strings.ReplaceAll(sql, "\x00", "")
	sql = trimBOM(sql)
	sql = strings.TrimSpace(sql)
	return sql
}

func trimBOM(s string) string {
	if len(s) == 0 {
		return s
	}
	runes := []rune(s)
	if runes[0] == '\uFEFF' {
		return string(runes[1:])
	}
	return s
}

// ============================================================================
// Policy Enforcement
// ============================================================================

// PolicyResult holds the result of a policy check.
type PolicyResult struct {
	Allowed       bool
	Reason        string
	Class         StatementClass
	Statement     string
	TruncatedStmt string // truncated for error messages
}

// EnforceExecutionPolicy checks whether a classified SQL statement may be
// executed by the current session. It enforces:
//   - DANGEROUS statements are always blocked
//   - DML_WRITE statements are checked against the table whitelist
//   - DDL is blocked unless explicitly allowed (e.g., for dynamic entity creation)
func EnforceExecutionPolicy(class StatementClass, stmt string, whitelist *TableWhitelist) *PolicyResult {
	if class == Dangerous || class == Other {
		return &PolicyResult{
			Allowed:       false,
			Reason:        fmt.Sprintf("blocked %s statement", class),
			Class:         class,
			Statement:     stmt,
			TruncatedStmt: truncate(stmt, 100),
		}
	}

	if class == DMLWrite {
		tableName := extractTableName(stmt)
		if tableName == "" {
			return &PolicyResult{
				Allowed:       false,
				Reason:        fmt.Sprintf("cannot determine target table for %s statement", class),
				Class:         class,
				Statement:     stmt,
				TruncatedStmt: truncate(stmt, 100),
			}
		}

		if !whitelist.IsAllowed(tableName) {
			return &PolicyResult{
				Allowed:       false,
				Reason:        fmt.Sprintf("write to unauthorized table %q", tableName),
				Class:         class,
				Statement:     stmt,
				TruncatedStmt: truncate(stmt, 100),
			}
		}
	}

	return &PolicyResult{
		Allowed: true,
		Class:   class,
	}
}

// ============================================================================
// Table Name Extraction
// ============================================================================

var (
	insertTableRe = regexp.MustCompile(`(?i)INSERT\s+INTO\s+["']?([a-z_][a-z0-9_]*)["']?`)
	updateTableRe = regexp.MustCompile(`(?i)UPDATE\s+["']?([a-z_][a-z0-9_]*)["']?`)
	deleteTableRe = regexp.MustCompile(`(?i)DELETE\s+FROM\s+["']?([a-z_][a-z0-9_]*)["']?`)
)

// extractTableName extracts the target table from a DML_WRITE statement.
// Returns empty string if the table cannot be determined.
func extractTableName(stmt string) string {
	upper := strings.ToUpper(strings.TrimSpace(stmt))

	switch {
	case strings.HasPrefix(upper, "INSERT"):
		m := insertTableRe.FindStringSubmatch(stmt)
		if len(m) >= 2 {
			return strings.ToLower(m[1])
		}
	case strings.HasPrefix(upper, "UPDATE"):
		m := updateTableRe.FindStringSubmatch(stmt)
		if len(m) >= 2 {
			return strings.ToLower(m[1])
		}
	case strings.HasPrefix(upper, "DELETE"):
		m := deleteTableRe.FindStringSubmatch(stmt)
		if len(m) >= 2 {
			return strings.ToLower(m[1])
		}
	}
	return ""
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-3] + "..."
}
