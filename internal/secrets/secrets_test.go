// Package secrets: unit tests for secret injection, scrubbing, and store management.
//
// axiom:trace work_item=runtime-harness-01 spec=specs/008-harness.md plan=phase-1/task-1-1/step-1-1-4 test=internal/secrets/secrets_test.go
package secrets

import (
	"strings"
	"testing"
)

// ============================================================================
// Store Management Tests
// ============================================================================

func TestNewStore(t *testing.T) {
	s := New()
	if s == nil {
		t.Fatal("New() returned nil")
	}
	if s.Count() != 0 {
		t.Errorf("new store should be empty, got %d", s.Count())
	}
}

func TestNewFromMap(t *testing.T) {
	s := NewFromMap(map[string]string{
		"API_KEY": "sk-abc123",
		"DB_PASS": "s3cr3t",
	})
	if s.Count() != 2 {
		t.Errorf("expected 2 secrets, got %d", s.Count())
	}
	if s.Get("API_KEY") != "sk-abc123" {
		t.Error("Get('API_KEY') returned wrong value")
	}
	if s.Get("DB_PASS") != "s3cr3t" {
		t.Error("Get('DB_PASS') returned wrong value")
	}
}

func TestSetAndGet(t *testing.T) {
	s := New()
	s.Set("LLM_KEY", "sk-llm-123")
	s.Set("GH_TOKEN", "ghp_secret")

	if s.Count() != 2 {
		t.Errorf("expected 2 secrets, got %d", s.Count())
	}
	if s.Get("LLM_KEY") != "sk-llm-123" {
		t.Errorf("Get LLM_KEY: expected 'sk-llm-123', got %q", s.Get("LLM_KEY"))
	}
	if s.Get("GH_TOKEN") != "ghp_secret" {
		t.Errorf("Get GH_TOKEN: expected 'ghp_secret', got %q", s.Get("GH_TOKEN"))
	}
}

func TestGetMissing(t *testing.T) {
	s := New()
	if s.Get("NONEXISTENT") != "" {
		t.Error("Get on missing key should return empty string")
	}
}

func TestOverwrite(t *testing.T) {
	s := New()
	s.Set("A", "v1")
	s.Set("A", "v2")
	if s.Get("A") != "v2" {
		t.Error("overwrites should update the value")
	}
}

func TestAliases(t *testing.T) {
	s := NewFromMap(map[string]string{
		"A": "1",
		"B": "2",
		"C": "3",
	})
	aliases := s.Aliases()
	if len(aliases) != 3 {
		t.Fatalf("expected 3 aliases, got %d", len(aliases))
	}

	// Verify all three are present (order not guaranteed)
	found := make(map[string]bool)
	for _, a := range aliases {
		found[a] = true
	}
	if !found["A"] || !found["B"] || !found["C"] {
		t.Error("missing expected alias")
	}
}

func TestAliases_Empty(t *testing.T) {
	s := New()
	aliases := s.Aliases()
	if len(aliases) != 0 {
		t.Errorf("empty store should have no aliases, got %d", len(aliases))
	}
}

// ============================================================================
// Injection Tests (AC-004)
// ============================================================================

func TestInject_Simple(t *testing.T) {
	s := NewFromMap(map[string]string{
		"LLM_API_KEY": "sk-real-key-abc",
	})

	result, err := s.Inject("SELECT * FROM vault WHERE key = '{{SECRET.LLM_API_KEY}}'")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "sk-real-key-abc") {
		t.Errorf("secret was not injected: %q", result)
	}
	if strings.Contains(result, "{{SECRET.LLM_API_KEY}}") {
		t.Errorf("alias was not replaced: %q", result)
	}
}

func TestInject_MultipleSecrets(t *testing.T) {
	s := NewFromMap(map[string]string{
		"API_KEY": "sk-api-123",
		"DB_PASS": "db-secret",
	})

	result, err := s.Inject(
		"CONNECT '{{SECRET.DB_PASS}}' USING '{{SECRET.API_KEY}}'",
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "sk-api-123") {
		t.Error("API_KEY not injected")
	}
	if !strings.Contains(result, "db-secret") {
		t.Error("DB_PASS not injected")
	}
	if strings.Contains(result, "{{SECRET.") {
		t.Error("alias not fully replaced")
	}
}

func TestInject_SameAliasMultipleTimes(t *testing.T) {
	s := NewFromMap(map[string]string{
		"SECRET_KEY": "the-real-key",
	})

	result, err := s.Inject(
		"INSERT INTO x VALUES ('{{SECRET.SECRET_KEY}}', '{{SECRET.SECRET_KEY}}')",
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	count := strings.Count(result, "the-real-key")
	if count != 2 {
		t.Errorf("expected 2 occurrences of secret, got %d", count)
	}
}

func TestInject_MissingSecret(t *testing.T) {
	s := New()

	result, err := s.Inject("SELECT '{{SECRET.UNKNOWN}}'")
	if err == nil {
		t.Error("expected error for unknown secret")
	}
	// Alias should remain unchanged in result when missing
	if !strings.Contains(result, "{{SECRET.UNKNOWN}}") {
		t.Errorf("missing alias should be preserved: %q", result)
	}
}

func TestInject_MultipleMissing(t *testing.T) {
	s := New()

	_, err := s.Inject("{{SECRET.A}} {{SECRET.B}} {{SECRET.C}}")
	if err == nil {
		t.Error("expected error for unknown secrets")
	}
	if !strings.Contains(err.Error(), "A") {
		t.Errorf("error should mention missing aliases: %v", err)
	}
}

func TestInject_NoAliases(t *testing.T) {
	s := NewFromMap(map[string]string{"A": "1"})

	result, err := s.Inject("SELECT 1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "SELECT 1" {
		t.Errorf("text should be unchanged: %q", result)
	}
}

func TestInjectSQL(t *testing.T) {
	s := NewFromMap(map[string]string{
		"DB_URL": "postgres://real-host/db",
	})

	result, err := s.InjectSQL("CONNECT TO '{{SECRET.DB_URL}}'")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "real-host") {
		t.Errorf("secret not injected in SQL: %q", result)
	}
}

func TestInject_EmptySecret(t *testing.T) {
	s := NewFromMap(map[string]string{
		"EMPTY_KEY": "",
	})

	result, err := s.Inject("SELECT '{{SECRET.EMPTY_KEY}}'")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "''") {
		t.Errorf("empty secret injection: %q", result)
	}
}

func TestInject_SecretContainingSpecialSQLChars(t *testing.T) {
	s := NewFromMap(map[string]string{
		"QUERY_KEY": "O'Brien's \"special\" value",
	})

	result, err := s.Inject("SELECT * FROM t WHERE key = '{{SECRET.QUERY_KEY}}'")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "O'Brien") {
		t.Errorf("special chars in secret: %q", result)
	}
}

// ============================================================================
// Scrubbing Tests (AC-004)
// ============================================================================

func TestScrub_Simple(t *testing.T) {
	s := NewFromMap(map[string]string{
		"LLM_API_KEY": "sk-real-key-abc",
	})

	result := s.Scrub("The API key is sk-real-key-abc, use it.")
	if strings.Contains(result, "sk-real-key-abc") {
		t.Errorf("secret was not scrubbed: %q", result)
	}
	if !strings.Contains(result, "[REDACTED:LLM_API_KEY]") {
		t.Errorf("expected redaction marker: %q", result)
	}
}

func TestScrub_MultipleSecrets(t *testing.T) {
	s := NewFromMap(map[string]string{
		"API_TOKEN": "ghp_real_token_123",
		"PASSWORD":  "hunter2",
	})

	result := s.Scrub("Logged in with token ghp_real_token_123 and password hunter2")
	if strings.Contains(result, "ghp_real_token_123") {
		t.Error("API_TOKEN not scrubbed")
	}
	if strings.Contains(result, "hunter2") {
		t.Error("PASSWORD not scrubbed")
	}
	if !strings.Contains(result, "[REDACTED:API_TOKEN]") {
		t.Error("missing API_TOKEN redaction")
	}
	if !strings.Contains(result, "[REDACTED:PASSWORD]") {
		t.Error("missing PASSWORD redaction")
	}
}

func TestScrub_MultipleOccurrences(t *testing.T) {
	s := NewFromMap(map[string]string{
		"KEY": "secret123",
	})

	result := s.Scrub("key=secret123, backup=secret123, also=secret123")
	count := strings.Count(result, "[REDACTED:KEY]")
	if count != 3 {
		t.Errorf("expected 3 redactions, got %d: %q", count, result)
	}
	if strings.Contains(result, "secret123") {
		t.Error("secret still present")
	}
}

func TestScrub_NoSecrets(t *testing.T) {
	s := NewFromMap(map[string]string{"A": "1"})

	result := s.Scrub("This text has no secrets")
	if result != "This text has no secrets" {
		t.Errorf("unchanged text: %q", result)
	}
}

func TestScrub_EmptyText(t *testing.T) {
	s := NewFromMap(map[string]string{"A": "1"})

	result := s.Scrub("")
	if result != "" {
		t.Errorf("empty should stay empty: %q", result)
	}
}

func TestScrub_SharedPrefixSecrets(t *testing.T) {
	// Ensure scrubbing a shorter key doesn't leave partial longer keys
	s := NewFromMap(map[string]string{
		"A": "secret",
		"B": "secret_longer",
	})

	result := s.Scrub("Values: secret and secret_longer")
	if strings.Contains(result, "secret_longer") || strings.Contains(result, "secret") {
		t.Errorf("secrets not scrubbed: %q", result)
	}
}

func TestScrub_EmptySecretSkipped(t *testing.T) {
	s := NewFromMap(map[string]string{
		"EMPTY": "",
		"REAL":  "xyz",
	})

	result := s.Scrub("value is xyz")
	if strings.Contains(result, "xyz") {
		t.Error("REAL not scrubbed")
	}
}

func TestScrubResponse(t *testing.T) {
	s := NewFromMap(map[string]string{
		"KEY": "sk-top-secret",
	})

	result := s.ScrubResponse("LLM response: key=sk-top-secret, monologue: using sk-top-secret")
	if strings.Contains(result, "sk-top-secret") {
		t.Error("secret not scrubbed from response")
	}
}

func TestScrub_SubstringRisk(t *testing.T) {
	// If a secret is a substring of another, ensure both get scrubbed independently
	s := NewFromMap(map[string]string{
		"SHORT": "ab",
		"LONG":  "abcd",
	})

	result := s.Scrub("found ab and abcd")
	// After ReplaceAll for each key, all occurrences should be gone
	if strings.Contains(result, "ab") && !strings.Contains(result, "[REDACTED:SHORT]") {
		t.Error("SHORT substring not correctly handled")
	}
	if strings.Contains(result, "abcd") && !strings.Contains(result, "[REDACTED:LONG]") {
		t.Error("LONG substring not correctly handled")
	}
}

func TestScrub_OverlappingSecrets(t *testing.T) {
	// When one secret contains another as substring, scrubbing order matters
	s := NewFromMap(map[string]string{
		"KEY1": "abc",
		"KEY2": "abcdef",
	})

	result := s.Scrub("I found abcdef today")
	// Both should be scrubbed somehow — as long as raw values are gone, it's safe
	if strings.Contains(result, "abcdef") {
		t.Error("KEY2 not scrubbed")
	}
}

// ============================================================================
// Alias Validation Tests
// ============================================================================

func TestIsValidAlias_Valid(t *testing.T) {
	valid := []string{
		"API_KEY",
		"api_key",
		"MY_SECRET_123",
		"_leading_underscore",
		"A",
		"a_b_c_1_2_3",
	}
	for _, alias := range valid {
		if !IsValidAlias(alias) {
			t.Errorf("expected valid: %q", alias)
		}
	}
}

func TestIsValidAlias_Invalid(t *testing.T) {
	invalid := []string{
		"",             // empty
		"123",          // starts with digit
		"has-dash",     // contains hyphen
		"has space",    // contains space
		"special!char", // contains special char
	}
	for _, alias := range invalid {
		if IsValidAlias(alias) {
			t.Errorf("expected invalid: %q", alias)
		}
	}
}

// ============================================================================
// Integration: Inject + Scrub round-trip
// ============================================================================

func TestRoundTrip(t *testing.T) {
	s := NewFromMap(map[string]string{
		"API_KEY": "sk-real-api-key-abc123",
		"DB_PASS": "s3cr3t-db-p@ss",
	})

	// 1. Inject secrets into SQL
	sql := "CONNECT TO '{{SECRET.DB_PASS}}'"
	injected, err := s.Inject(sql)
	if err != nil {
		t.Fatalf("injection failed: %v", err)
	}
	if injected != "CONNECT TO 's3cr3t-db-p@ss'" {
		t.Errorf("injection wrong: %q", injected)
	}

	// 2. Simulate LLM response containing real secrets
	response := "Connected using s3cr3t-db-p@ss with API key sk-real-api-key-abc123"

	// 3. Scrub the response
	scrubbed := s.Scrub(response)
	if strings.Contains(scrubbed, "s3cr3t-db-p@ss") {
		t.Error("DB_PASS not scrubbed after round-trip")
	}
	if strings.Contains(scrubbed, "sk-real-api-key-abc123") {
		t.Error("API_KEY not scrubbed after round-trip")
	}
	if !strings.Contains(scrubbed, "[REDACTED:DB_PASS]") {
		t.Error("missing DB_PASS redaction marker")
	}
	if !strings.Contains(scrubbed, "[REDACTED:API_KEY]") {
		t.Error("missing API_KEY redaction marker")
	}
}

// ============================================================================
// Edge Cases
// ============================================================================

func TestInject_EmbeddedSecretInJSON(t *testing.T) {
	s := NewFromMap(map[string]string{
		"LLM_KEY": "sk-openai-key",
	})

	// Secret in JSON body
	jsonStr := `{"auth": {"key": "{{SECRET.LLM_KEY}}"}}`
	result, err := s.Inject(jsonStr)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, `"sk-openai-key"`) {
		t.Errorf("secret not injected into JSON: %q", result)
	}
}

func TestScrub_LLMHallucinatedSecret(t *testing.T) {
	// If the LLM somehow outputs a known secret value, it must be scrubbed
	s := NewFromMap(map[string]string{
		"AWS_KEY": "AKIA1234567890ABCDEF",
	})

	response := "I found the AWS key: AKIA1234567890ABCDEF in the logs"
	scrubbed := s.Scrub(response)
	if strings.Contains(scrubbed, "AKIA1234567890ABCDEF") {
		t.Error("hallucinated secret not scrubbed")
	}
}
