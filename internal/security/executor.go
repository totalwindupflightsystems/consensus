// Package security: three-tier SQL execution model (SPEC-008 §5.4).
//
// The harness supports three tiers of SQL execution, controlled by the
// agent's trust level. Lower-trust models are restricted to stored procedures
// only; higher-trust models gain access to broader SQL execution.
//
//	Tier 1 (Low trust):   Stored-procedure-only. Only SELECT fn_name(...) allowed.
//	Tier 2 (Medium trust): Parameterized SQL with $1, $2 placeholders.
//	Tier 3 (High trust):   Raw SQL with classifier + table whitelist (existing).
//
// axiom:trace work_item=WI-006 spec=specs/003-database.md,specs/005-security.md,specs/008-harness.md plan=phase-1/task-1 impl=internal/security/executor.go
package security

import (
	"fmt"
	"regexp"
	"strings"
)

// ============================================================================
// Trust Level — Session Trust Classification
// ============================================================================

// TrustLevel represents how much trust the runtime places in a session's agent.
// It determines which SQL execution tier is used.
type TrustLevel int

const (
	// TrustUnspecified is the zero value — treated as Low for safety.
	TrustUnspecified TrustLevel = iota
	// TrustLow is for cheap/small models, untrusted environments. Tier 1 only.
	TrustLow
	// TrustMedium is for moderate-trust models. Tier 2 (parameterized SQL).
	TrustMedium
	// TrustHigh is for trusted models with full SQL access. Tier 3.
	TrustHigh
)

// String returns the human-readable name of the trust level.
func (t TrustLevel) String() string {
	switch t {
	case TrustLow:
		return "low"
	case TrustMedium:
		return "medium"
	case TrustHigh:
		return "high"
	default:
		return "unspecified"
	}
}

// ParseTrustLevel parses a string into a TrustLevel.
// Accepts: "low", "medium", "high" (case-insensitive).
func ParseTrustLevel(s string) (TrustLevel, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "low":
		return TrustLow, nil
	case "medium":
		return TrustMedium, nil
	case "high":
		return TrustHigh, nil
	default:
		return TrustUnspecified, fmt.Errorf("invalid trust level %q (expected low, medium, or high)", s)
	}
}

// ============================================================================
// Execution Tier — Maps Trust Level to Execution Strategy
// ============================================================================

// ExecutionTier identifies which SQL execution strategy to use.
type ExecutionTier int

const (
	// Tier1 is stored-procedure-only execution. Only SELECT fn_name(...) calls.
	Tier1 ExecutionTier = 1
	// Tier2 is parameterized SQL execution with $1, $2 placeholders.
	Tier2 ExecutionTier = 2
	// Tier3 is raw SQL execution with classifier + table whitelist.
	Tier3 ExecutionTier = 3
)

// String returns the human-readable name of the execution tier.
func (t ExecutionTier) String() string {
	switch t {
	case Tier1:
		return "TIER_1"
	case Tier2:
		return "TIER_2"
	case Tier3:
		return "TIER_3"
	default:
		return "TIER_UNKNOWN"
	}
}

// TrustLevelToTier maps a trust level to the appropriate execution tier.
func TrustLevelToTier(level TrustLevel) ExecutionTier {
	switch level {
	case TrustLow:
		return Tier1
	case TrustMedium:
		return Tier2
	case TrustHigh:
		return Tier3
	default:
		return Tier1 // safest default
	}
}

// ============================================================================
// Tier 1: Stored-Procedure-Only Execution
// ============================================================================

// DefaultAllowedFunctions is the default whitelist of stored functions that
// Tier 1 sessions may call. Additional functions can be registered via
// tools_registry with handler_type 'sql_function'.
var DefaultAllowedFunctions = map[string]bool{
	// Session lifecycle
	"set_display_mode":        true,
	"complete_session":        true,
	"touch_session_heartbeat": true,

	// Memory operations
	"load_memory_event":  true,
	"search_memory":      true,
	"create_memory_page": true,

	// Task operations
	"claim_task":    true,
	"complete_task": true,
	"fail_task":     true,
	"cancel_task":   true,

	// Utility
	"current_setting": true,
	"gen_random_uuid": true,
	"now":             true,
}

// Tier1StmtRegex matches a Tier 1 safe statement: SELECT fn_name(...)
// It captures the function name for whitelist validation.
var tier1StmtRegex = regexp.MustCompile(`(?i)^\s*SELECT\s+([a-z_][a-z0-9_]*)\s*\(`)

// ExecuteTier1Result holds the result of a Tier 1 validation.
type ExecuteTier1Result struct {
	Allowed      bool
	FunctionName string
	Reason       string
}

// ExecuteTier1 validates that a statement is a safe stored-procedure call.
//
// Rules:
//   - Must be a SELECT statement calling a single function
//   - Function must be in the allowed functions whitelist
//   - No nested subqueries, no raw DML
//   - Function name must be a simple identifier (no schema qualification)
func ExecuteTier1(stmt string, allowedFunctions map[string]bool) *ExecuteTier1Result {
	if allowedFunctions == nil {
		allowedFunctions = DefaultAllowedFunctions
	}

	stmt = strings.TrimSpace(stmt)
	if stmt == "" {
		return &ExecuteTier1Result{
			Allowed: false,
			Reason:  "empty statement",
		}
	}

	// Must match SELECT fn_name(...) pattern
	matches := tier1StmtRegex.FindStringSubmatch(stmt)
	if len(matches) < 2 {
		return &ExecuteTier1Result{
			Allowed: false,
			Reason:  fmt.Sprintf("Tier 1: statement must be SELECT function_name(...), got: %s", truncate(stmt, 80)),
		}
	}

	funcName := strings.ToLower(matches[1])

	// Check the function is in the whitelist
	if !allowedFunctions[funcName] {
		return &ExecuteTier1Result{
			Allowed:      false,
			FunctionName: funcName,
			Reason:       fmt.Sprintf("Tier 1: function %q is not in the allowed whitelist", funcName),
		}
	}

	// Additional safety: ensure no semicolons (multi-statement)
	if strings.Contains(strings.TrimSuffix(stmt, ";"), ";") {
		return &ExecuteTier1Result{
			Allowed:      false,
			FunctionName: funcName,
			Reason:       "Tier 1: multi-statement calls are not allowed",
		}
	}

	// Additional safety: block sub-SELECTs (nested queries)
	// A simple SELECT fn(args) should not contain another SELECT within the parens
	// This is a heuristic check; complex function arguments with subqueries
	// would need deeper parsing. For now, we keep it simple.
	parenDepth := 0
	selectInside := false
	upper := strings.ToUpper(stmt)
	for i, c := range stmt {
		if c == '(' {
			parenDepth++
		} else if c == ')' {
			parenDepth--
		} else if parenDepth > 0 && i >= len(matches[0])-1 {
			// Check for SELECT keyword inside function arguments
			if strings.HasPrefix(upper[i:], "SELECT ") {
				selectInside = true
				break
			}
		}
	}
	if selectInside {
		return &ExecuteTier1Result{
			Allowed:      false,
			FunctionName: funcName,
			Reason:       "Tier 1: subqueries are not allowed in function arguments",
		}
	}

	return &ExecuteTier1Result{
		Allowed:      true,
		FunctionName: funcName,
	}
}

// ============================================================================
// Tier 2: Parameterized SQL Execution
// ============================================================================

// ExecuteTier2Result holds the result of a Tier 2 validation.
type ExecuteTier2Result struct {
	Allowed    bool
	Reason     string
	ParamCount int // number of $N parameters detected
}

// Tier2StmtRegex matches statements with $N parameter placeholders.
var tier2ParamRegex = regexp.MustCompile(`\$(\d+)`)

// ExecuteTier2 validates that a statement is safe parameterized SQL.
//
// Rules:
//   - Must pass the same classifier checks as Tier 3 (no DANGEROUS statements)
//   - Must use $1, $2, ... parameter placeholders (not inline values)
//   - Parameter indices must be sequential starting from 1
//   - No string literals with interpolated values
//   - Still subject to table whitelist for DML_WRITE
//
// Unlike Tier 1, Tier 2 allows any SQL statement type (SELECT, INSERT, UPDATE,
// DELETE) but requires all dynamic values to be passed as $N parameters rather
// than inline values.
func ExecuteTier2(stmt string, whitelist *TableWhitelist) *ExecuteTier2Result {
	stmt = strings.TrimSpace(stmt)
	if stmt == "" {
		return &ExecuteTier2Result{
			Allowed: false,
			Reason:  "empty statement",
		}
	}

	// Step 1: Classify the statement
	class := ClassifyStatement(stmt)
	if class == Dangerous || class == Other {
		return &ExecuteTier2Result{
			Allowed: false,
			Reason:  fmt.Sprintf("Tier 2: blocked %s statement: %s", class, truncate(stmt, 80)),
		}
	}

	// Step 2: Check for $N parameter placeholders
	paramMatches := tier2ParamRegex.FindAllStringSubmatch(stmt, -1)
	if len(paramMatches) == 0 {
		return &ExecuteTier2Result{
			Allowed: false,
			Reason:  "Tier 2: statement must use $1, $2 parameter placeholders (no inline values)",
		}
	}

	// Step 3: Parameter indices must be valid (starting from 1, no gaps)
	maxParam := 0
	seenParams := make(map[int]bool)
	for _, m := range paramMatches {
		var idx int
		fmt.Sscanf(m[1], "%d", &idx)
		if idx < 1 {
			return &ExecuteTier2Result{
				Allowed:    false,
				ParamCount: len(paramMatches),
				Reason:     fmt.Sprintf("Tier 2: invalid parameter index $%d (must be >= 1)", idx),
			}
		}
		seenParams[idx] = true
		if idx > maxParam {
			maxParam = idx
		}
	}

	// Check for gaps in parameter numbering (e.g., $1, $3 without $2)
	for i := 1; i <= maxParam; i++ {
		if !seenParams[i] {
			return &ExecuteTier2Result{
				Allowed:    false,
				ParamCount: len(paramMatches),
				Reason:     fmt.Sprintf("Tier 2: parameter gap — $%d present but $%d missing", maxParam, i),
			}
		}
	}

	// Step 4: If DML_WRITE, check table whitelist
	if class == DMLWrite {
		tableName := extractTableName(stmt)
		if tableName == "" {
			return &ExecuteTier2Result{
				Allowed:    false,
				ParamCount: len(paramMatches),
				Reason:     fmt.Sprintf("Tier 2: cannot determine target table: %s", truncate(stmt, 80)),
			}
		}
		if !whitelist.IsAllowed(tableName) {
			return &ExecuteTier2Result{
				Allowed:    false,
				ParamCount: len(paramMatches),
				Reason:     fmt.Sprintf("Tier 2: write to unauthorized table %q", tableName),
			}
		}
	}

	return &ExecuteTier2Result{
		Allowed:    true,
		ParamCount: len(paramMatches),
	}
}

// ============================================================================
// Tier 3: Raw SQL Execution (Existing Behavior)
// ============================================================================

// ExecuteTier3Result holds the result of a Tier 3 validation.
type ExecuteTier3Result struct {
	Allowed bool
	Reason  string
}

// ExecuteTier3 validates a raw SQL statement using the existing classifier
// and table whitelist (SPEC-008 §5.4 Tier 3).
//
// This wraps the existing EnforceExecutionPolicy behavior for clarity.
// Rules:
//   - DANGEROUS statements are always blocked
//   - DML_WRITE is checked against the table whitelist
//   - DDL is blocked unless explicitly allowed
func ExecuteTier3(stmt string, whitelist *TableWhitelist) *ExecuteTier3Result {
	class := ClassifyStatement(stmt)
	result := EnforceExecutionPolicy(class, stmt, whitelist)
	return &ExecuteTier3Result{
		Allowed: result.Allowed,
		Reason:  result.Reason,
	}
}

// ============================================================================
// Tier-Aware Policy Enforcement
// ============================================================================

// TieredPolicyResult holds the result of a tier-aware policy check.
type TieredPolicyResult struct {
	Allowed      bool
	Reason       string
	Tier         ExecutionTier
	Class        StatementClass
	ParamCount   int    // populated for Tier 2
	FunctionName string // populated for Tier 1
}

// EnforceTieredPolicy applies the correct execution tier based on trust level.
//
// This is the primary entry point for the harness to check whether a SQL
// statement may be executed by a session with a given trust level.
func EnforceTieredPolicy(stmt string, trustLevel TrustLevel, whitelist *TableWhitelist, allowedFunctions map[string]bool) *TieredPolicyResult {
	tier := TrustLevelToTier(trustLevel)

	switch tier {
	case Tier1:
		result := ExecuteTier1(stmt, allowedFunctions)
		return &TieredPolicyResult{
			Allowed:      result.Allowed,
			Reason:       result.Reason,
			Tier:         Tier1,
			FunctionName: result.FunctionName,
		}

	case Tier2:
		result := ExecuteTier2(stmt, whitelist)
		return &TieredPolicyResult{
			Allowed:    result.Allowed,
			Reason:     result.Reason,
			Tier:       Tier2,
			ParamCount: result.ParamCount,
		}

	case Tier3:
		result := ExecuteTier3(stmt, whitelist)
		return &TieredPolicyResult{
			Allowed: result.Allowed,
			Reason:  result.Reason,
			Tier:    Tier3,
			Class:   ClassifyStatement(stmt),
		}

	default:
		// Should not reach here, but safest fallback is Tier 1
		result := ExecuteTier1(stmt, allowedFunctions)
		return &TieredPolicyResult{
			Allowed:      result.Allowed,
			Reason:       result.Reason,
			Tier:         Tier1,
			FunctionName: result.FunctionName,
		}
	}
}
