// Package compression: tests for tier logic, cosine validation, and helpers.
//
// axiom:trace work_item=vector-compression-01 spec=specs/002-memory.md plan=phase-1/task-1-2 test=internal/compression/compression_test.go
package compression

import (
	"testing"
)

// ============================================================================
// Display Tier Tests
// ============================================================================

func TestDisplayTier_String(t *testing.T) {
	tests := []struct {
		tier DisplayTier
		want string
	}{
		{TierRaw, "raw"},
		{TierCompressed, "compressed"},
		{TierAbstract, "abstract"},
		{TierCanonical, "canonical"},
		{DisplayTier(99), "tier(99)"},
	}
	for _, tt := range tests {
		if got := tt.tier.String(); got != tt.want {
			t.Errorf("DisplayTier(%d).String() = %q, want %q", int(tt.tier), got, tt.want)
		}
	}
}

func TestDisplayTier_DisplayMode(t *testing.T) {
	tests := []struct {
		tier DisplayTier
		want string
	}{
		{TierRaw, "full"},
		{TierCompressed, "compressed"},
		{TierAbstract, "compressed"},
		{TierCanonical, "compressed"},
	}
	for _, tt := range tests {
		if got := tt.tier.DisplayMode(); got != tt.want {
			t.Errorf("DisplayTier(%d).DisplayMode() = %q, want %q", int(tt.tier), got, tt.want)
		}
	}
}

// ============================================================================
// Tier Escalation Tests
// ============================================================================

func TestNextTier(t *testing.T) {
	tests := []struct {
		current DisplayTier
		want    DisplayTier
	}{
		{TierRaw, TierCompressed},
		{TierCompressed, TierAbstract},
		{TierAbstract, TierCanonical},
		{TierCanonical, TierCanonical},   // max — stays
		{DisplayTier(99), TierCanonical}, // invalid — returns max
	}
	for _, tt := range tests {
		if got := NextTier(tt.current); got != tt.want {
			t.Errorf("NextTier(%v) = %v, want %v", tt.current, got, tt.want)
		}
	}
}

func TestShouldEscalate(t *testing.T) {
	tests := []struct {
		score     float64
		threshold float64
		escalate  bool
	}{
		{0.90, 0.85, false}, // above threshold — accept
		{0.85, 0.85, false}, // at threshold — accept
		{0.84, 0.85, true},  // below threshold — escalate
		{0.0, 0.85, true},   // far below — escalate
		{1.0, 0.85, false},  // perfect — accept
	}
	for _, tt := range tests {
		if got := ShouldEscalate(tt.score, tt.threshold); got != tt.escalate {
			t.Errorf("ShouldEscalate(%f, %f) = %v, want %v", tt.score, tt.threshold, got, tt.escalate)
		}
	}
}

func TestTierFromInt(t *testing.T) {
	tests := []struct {
		tier int
		want DisplayTier
	}{
		{0, TierRaw},
		{1, TierCompressed},
		{2, TierAbstract},
		{3, TierCanonical},
		{-1, TierRaw},
		{5, TierCanonical},
	}
	for _, tt := range tests {
		if got := TierFromInt(tt.tier); got != tt.want {
			t.Errorf("TierFromInt(%d) = %v, want %v", tt.tier, got, tt.want)
		}
	}
}

// ============================================================================
// CompressionResult Tests
// ============================================================================

func TestCompressionResult_String(t *testing.T) {
	tests := []struct {
		r    CompressionResult
		want string
	}{
		{ResultAccepted, "accepted"},
		{ResultRejectedEscalate, "rejected_escalate"},
		{ResultFailed, "failed"},
		{CompressionResult(99), "unknown"},
	}
	for _, tt := range tests {
		if got := tt.r.String(); got != tt.want {
			t.Errorf("CompressionResult(%d).String() = %q, want %q", int(tt.r), got, tt.want)
		}
	}
}

// ============================================================================
// Summary Prompt Tests
// ============================================================================

func TestCompressionSummaryPrompt(t *testing.T) {
	prompts := []struct {
		tier  DisplayTier
		check string
	}{
		{TierCompressed, "40-60%"},
		{TierAbstract, "20-30%"},
		{TierCanonical, "10-15%"},
	}
	for _, p := range prompts {
		prompt := CompressionSummaryPrompt(p.tier)
		if len(prompt) < 50 {
			t.Errorf("prompt for %s too short: %d chars", p.tier, len(prompt))
		}
		if !contains(prompt, p.check) {
			t.Errorf("prompt for %s missing %q", p.tier, p.check)
		}
	}
}

func TestCompressionSummaryPrompt_Default(t *testing.T) {
	prompt := CompressionSummaryPrompt(DisplayTier(99))
	if !contains(prompt, "40-60%") {
		t.Error("default prompt should be TierCompressed")
	}
}

// ============================================================================
// Cosine Threshold Tests
// ============================================================================

func TestCosineThresholdForTier(t *testing.T) {
	tests := []struct {
		tier DisplayTier
		want float64
	}{
		{TierCompressed, 0.85},
		{TierAbstract, 0.80},
		{TierCanonical, 0.75},
		{TierRaw, 0.85},         // default fallback
		{DisplayTier(99), 0.85}, // default fallback
	}
	for _, tt := range tests {
		if got := CosineThresholdForTier(tt.tier); got != tt.want {
			t.Errorf("CosineThresholdForTier(%v) = %f, want %f", tt.tier, got, tt.want)
		}
	}
}

// ============================================================================
// Helpers
// ============================================================================

func contains(s, substr string) bool {
	return len(s) >= len(substr) && containsStr(s, substr)
}

func containsStr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
