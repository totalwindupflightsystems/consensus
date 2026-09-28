// Package sqlite: normalizer unit tests.
//
// DF-CONSENSUS-45: the SQLite row normalizer JSON-decoded every TEXT value,
// so a user message whose body is valid JSON ("Reply with \"quoted\" text,
// please" or {"role":"user"}) round-tripped as a re-encoded JSON string —
// POST /session/:id/message through the opencode shim then read back a
// quoted literal from memory_events while the native POST stored raw text.
// Content is raw text, never parsed: a string that merely LOOKS like JSON
// must survive byte-for-byte in both directions.
package sqlite

import "testing"

// TestNormalizeValuePassesJSONLookingStringsThrough pins the exact strings
// from the defect report: each must round-trip unchanged, including quotes
// and backslashes. Byte-equality both ways is the regression contract — any
// re-encoding (marshaling, quote-escaping) of the read side fails these.
func TestNormalizeValuePassesJSONLookingStringsThrough(t *testing.T) {
	cases := []string{
		`Reply with "quoted" text, please`,
		`{"role":"user"}`,
		`[1, 2, 3]`,
		"line one\nline \"two\"",
		`"already quoted"`,
	}
	for _, in := range cases {
		if got := normalizeValue(in); got != in {
			t.Errorf("normalizeValue(%q) = %#v, want unchanged", in, got)
		}
	}
}

// TestNormalizeValueKeepsBoolsAndNumbers pins the normalizer's real job —
// database/sql hands SQLite INTEGER values back as int64; converting them to
// string would break numeric scans. (Bane's probe pattern: prove bool handling
// live instead of claiming it.)
func TestNormalizeValueKeepsBoolsAndNumbers(t *testing.T) {
	if got := normalizeValue(int64(7)); got != int64(7) {
		t.Errorf("normalizeValue(int64(7)) = %#v, want int64(7)", got)
	}
	if got := normalizeValue(float64(2.5)); got != float64(2.5) {
		t.Errorf("normalizeValue(float64(2.5)) = %#v, want float64(2.5)", got)
	}
	if got := normalizeValue([]byte("raw")); got != "raw" {
		t.Errorf("normalizeValue([]byte) = %#v, want string conversion", got)
	}
}
