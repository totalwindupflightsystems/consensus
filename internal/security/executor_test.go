// Package security: tests for three-tier SQL execution model (SPEC-008 §5.4).
//
// axiom:trace work_item=WI-006 spec=specs/008-harness.md plan=phase-1/task-2 impl=internal/security/executor_test.go
package security

import (
	"testing"
)

// ============================================================================
// Trust Level Tests
// ============================================================================

func TestParseTrustLevel(t *testing.T) {
	tests := []struct {
		input     string
		want      TrustLevel
		wantError bool
	}{
		{"low", TrustLow, false},
		{"medium", TrustMedium, false},
		{"high", TrustHigh, false},
		{"LOW", TrustLow, false},
		{"Medium", TrustMedium, false},
		{"HIGH", TrustHigh, false},
		{"unknown", TrustUnspecified, true},
		{"", TrustUnspecified, true},
		{" ultra", TrustUnspecified, true},
	}

	for _, tt := range tests {
		got, err := ParseTrustLevel(tt.input)
		if tt.wantError {
			if err == nil {
				t.Errorf("ParseTrustLevel(%q) expected error, got %v", tt.input, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("ParseTrustLevel(%q) unexpected error: %v", tt.input, err)
		}
		if got != tt.want {
			t.Errorf("ParseTrustLevel(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

func TestTrustLevelToTier(t *testing.T) {
	tests := []struct {
		trust TrustLevel
		want  ExecutionTier
	}{
		{TrustUnspecified, Tier1},
		{TrustLow, Tier1},
		{TrustMedium, Tier2},
		{TrustHigh, Tier3},
	}

	for _, tt := range tests {
		got := TrustLevelToTier(tt.trust)
		if got != tt.want {
			t.Errorf("TrustLevelToTier(%v) = %v, want %v", tt.trust, got, tt.want)
		}
	}
}

// ============================================================================
// Tier 1: Stored-Procedure-Only Tests
// ============================================================================

func TestExecuteTier1_ValidCalls(t *testing.T) {
	tests := []struct {
		name string
		stmt string
	}{
		{"simple function call", "SELECT set_display_mode(104, 'compressed')"},
		{"function with no args", "SELECT now()"},
		{"function with multiple args", "SELECT complete_session('session-uuid')"},
		{"function with hyphen UUID", "SELECT load_memory_event(42)"},
		{"function with trailing semicolon", "SELECT gen_random_uuid();"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ExecuteTier1(tt.stmt, nil)
			if !result.Allowed {
				t.Errorf("ExecuteTier1(%q) = {Allowed: false, Reason: %q}", tt.stmt, result.Reason)
			}
		})
	}
}

func TestExecuteTier1_BlockedCalls(t *testing.T) {
	tests := []struct {
		name     string
		stmt     string
		wantFunc string
	}{
		{"INSERT blocked", "INSERT INTO memory_events (type, content) VALUES ('text', 'hello')", ""},
		{"UPDATE blocked", "UPDATE tasks SET status = 'completed' WHERE id = 'abc'", ""},
		{"DELETE blocked", "DELETE FROM tasks WHERE id = 'abc'", ""},
		{"raw DDL blocked", "CREATE TABLE foo (id INT)", ""},
		{"DROP blocked", "DROP TABLE memory_events", ""},
		{"non-whitelist function", "SELECT pg_sleep(10)", "pg_sleep"},
		{"no function call", "SELECT 1", ""},
		{"multiple statements", "SELECT now(); SELECT now()", "now"},
		{"subquery in args", "SELECT set_display_mode((SELECT id FROM tasks))", "set_display_mode"},
		{"empty string", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ExecuteTier1(tt.stmt, nil)
			if result.Allowed {
				t.Errorf("ExecuteTier1(%q) expected blocked, got allowed", tt.stmt)
			}
			if tt.wantFunc != "" && result.FunctionName != tt.wantFunc {
				t.Errorf("ExecuteTier1(%q) function name = %q, want %q", tt.stmt, result.FunctionName, tt.wantFunc)
			}
		})
	}
}

func TestExecuteTier1_AllowedFunctionWhitelist(t *testing.T) {
	// Test with a custom allowed functions set
	allowed := map[string]bool{
		"my_custom_fn": true,
	}

	// Should pass with custom whitelist
	result := ExecuteTier1("SELECT my_custom_fn(42)", allowed)
	if !result.Allowed {
		t.Errorf("ExecuteTier1 with custom whitelist: expected allowed, got %q", result.Reason)
	}

	// Should fail with default whitelist (my_custom_fn not in defaults)
	result = ExecuteTier1("SELECT my_custom_fn(42)", nil)
	if result.Allowed {
		t.Errorf("ExecuteTier1 with default whitelist: expected blocked for custom fn")
	}
}

func TestExecuteTier1_CaseInsensitive(t *testing.T) {
	tests := []struct {
		name string
		stmt string
	}{
		{"lowercase", "select now()"},
		{"uppercase", "SELECT now()"},
		{"mixed case", "Select Now()"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ExecuteTier1(tt.stmt, nil)
			if !result.Allowed {
				t.Errorf("ExecuteTier1(%q) = {Allowed: false, Reason: %q}", tt.stmt, result.Reason)
			}
		})
	}
}

// ============================================================================
// Tier 2: Parameterized SQL Tests
// ============================================================================

func TestExecuteTier2_ValidParameterized(t *testing.T) {
	whitelist := NewTableWhitelist()

	tests := []struct {
		name       string
		stmt       string
		wantParams int
	}{
		{"INSERT with params", "INSERT INTO memory_events (type, content, session_id) VALUES ($1, $2, $3)", 3},
		{"SELECT with param", "SELECT * FROM memory_events WHERE session_id = $1", 1},
		{"UPDATE with params", "UPDATE tasks SET status = $1 WHERE id = $2", 2},
		{"DELETE with param", "DELETE FROM tasks WHERE id = $1", 1},
		{"multiple placeholders", "INSERT INTO memory_events (type, content, session_id, iteration_created) VALUES ($1, $2, $3, $4)", 4},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ExecuteTier2(tt.stmt, whitelist)
			if !result.Allowed {
				t.Errorf("ExecuteTier2(%q) = {Allowed: false, Reason: %q}", tt.stmt, result.Reason)
			}
			if result.ParamCount != tt.wantParams {
				t.Errorf("ExecuteTier2(%q) param count = %d, want %d", tt.stmt, result.ParamCount, tt.wantParams)
			}
		})
	}
}

func TestExecuteTier2_Blocked(t *testing.T) {
	whitelist := NewTableWhitelist()

	tests := []struct {
		name string
		stmt string
	}{
		{"no params in INSERT", "INSERT INTO memory_events (type, content) VALUES ('text', 'hello')"},
		{"no params in SELECT", "SELECT * FROM memory_events"},
		{"dangerous statement", "TRUNCATE TABLE memory_events"},
		{"unauthorized table", "INSERT INTO pg_class VALUES (1)"},
		{"DDL without params", "CREATE TABLE foo (id INT)"},
		{"empty statement", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ExecuteTier2(tt.stmt, whitelist)
			if result.Allowed {
				t.Errorf("ExecuteTier2(%q) expected blocked, got allowed", tt.stmt)
			}
		})
	}
}

func TestExecuteTier2_ParameterGaps(t *testing.T) {
	whitelist := NewTableWhitelist()

	tests := []struct {
		name string
		stmt string
	}{
		{"missing $1 starts at $2", "INSERT INTO tasks (id, title) VALUES ($2, $3)"},
		{"gap between params", "INSERT INTO memory_events (type, content) VALUES ($1, $3)"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ExecuteTier2(tt.stmt, whitelist)
			if result.Allowed {
				t.Errorf("ExecuteTier2(%q) expected blocked (parameter gap), got allowed", tt.stmt)
			}
		})
	}
}

func TestExecuteTier2_UnauthorizedTable(t *testing.T) {
	whitelist := NewTableWhitelist()

	// Write to memory_events is allowed
	result := ExecuteTier2("INSERT INTO memory_events (type, content, session_id) VALUES ($1, $2, $3)", whitelist)
	if !result.Allowed {
		t.Errorf("ExecuteTier2 memory_events should be allowed: %s", result.Reason)
	}

	// Write to a non-whitelisted table should be blocked
	result = ExecuteTier2("INSERT INTO secrets_table (value) VALUES ($1)", whitelist)
	if result.Allowed {
		t.Errorf("ExecuteTier2 secrets_table should be blocked")
	}
}

// ============================================================================
// Tier 3: Raw SQL Tests (Existing Behavior Preservation)
// ============================================================================

func TestExecuteTier3_ValidRawSQL(t *testing.T) {
	whitelist := NewTableWhitelist()

	tests := []struct {
		name string
		stmt string
	}{
		{"simple SELECT", "SELECT * FROM memory_events"},
		{"INSERT allowed table", "INSERT INTO memory_events (type, content, session_id) VALUES ('text', 'hello', 'abc')"},
		{"UPDATE allowed table", "UPDATE tasks SET status = 'completed' WHERE id = 'abc'"},
		{"DELETE allowed table", "DELETE FROM tasks WHERE id = 'abc'"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ExecuteTier3(tt.stmt, whitelist)
			if !result.Allowed {
				t.Errorf("ExecuteTier3(%q) = {Allowed: false, Reason: %q}", tt.stmt, result.Reason)
			}
		})
	}
}

func TestExecuteTier3_BlockedRawSQL(t *testing.T) {
	whitelist := NewTableWhitelist()

	tests := []struct {
		name string
		stmt string
	}{
		{"dangerous", "TRUNCATE TABLE memory_events"},
		{"unauthorized write", "INSERT INTO pg_class VALUES (1)"},
		{"GRANT", "GRANT ALL ON memory_events TO PUBLIC"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ExecuteTier3(tt.stmt, whitelist)
			if result.Allowed {
				t.Errorf("ExecuteTier3(%q) expected blocked", tt.stmt)
			}
		})
	}
}

// ============================================================================
// Tier-Aware Policy Enforcement Tests
// ============================================================================

func TestEnforceTieredPolicy_Routing(t *testing.T) {
	whitelist := NewTableWhitelist()

	tests := []struct {
		name       string
		stmt       string
		trust      TrustLevel
		wantTier   ExecutionTier
		wantResult bool
	}{
		// TrustLow → Tier 1
		{"low trust: valid Tier 1", "SELECT now()", TrustLow, Tier1, true},
		{"low trust: blocked Tier 2", "INSERT INTO memory_events (type, content, session_id) VALUES ($1, $2, $3)", TrustLow, Tier1, false},
		{"low trust: blocked Tier 3", "SELECT * FROM memory_events", TrustLow, Tier1, false},

		// TrustMedium → Tier 2
		{"medium trust: valid Tier 2", "INSERT INTO memory_events (type, content, session_id) VALUES ($1, $2, $3)", TrustMedium, Tier2, true},
		{"medium trust: blocked inline", "INSERT INTO memory_events (type, content, session_id) VALUES ('text', 'hello', 'abc')", TrustMedium, Tier2, false},

		// TrustHigh → Tier 3
		{"high trust: valid Tier 3", "SELECT * FROM memory_events", TrustHigh, Tier3, true},
		{"high trust: INSERT allowed", "INSERT INTO memory_events (type, content, session_id) VALUES ('text', 'hello', 'abc')", TrustHigh, Tier3, true},
		{"high trust: dangerous blocked", "TRUNCATE TABLE memory_events", TrustHigh, Tier3, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := EnforceTieredPolicy(tt.stmt, tt.trust, whitelist, nil)
			if result.Allowed != tt.wantResult {
				t.Errorf("EnforceTieredPolicy(%q, %v) = {Allowed: %v, Reason: %q}, want Allowed=%v",
					tt.stmt, tt.trust, result.Allowed, result.Reason, tt.wantResult)
			}
			if result.Tier != tt.wantTier {
				t.Errorf("EnforceTieredPolicy tier = %v, want %v", result.Tier, tt.wantTier)
			}
		})
	}
}

func TestEnforceTieredPolicy_UnspecifiedDefaultsToTier1(t *testing.T) {
	whitelist := NewTableWhitelist()

	// TrustUnspecified should default to Tier 1 (safest)
	result := EnforceTieredPolicy("SELECT now()", TrustUnspecified, whitelist, nil)
	if !result.Allowed {
		t.Errorf("TrustUnspecified should allow Tier 1 calls: %s", result.Reason)
	}
	if result.Tier != Tier1 {
		t.Errorf("TrustUnspecified should route to Tier 1, got %v", result.Tier)
	}

	// Tier 2 should be blocked
	result = EnforceTieredPolicy("INSERT INTO memory_events (type, content) VALUES ($1, $2)", TrustUnspecified, whitelist, nil)
	if result.Allowed {
		t.Errorf("TrustUnspecified should block Tier 2 calls")
	}
}
