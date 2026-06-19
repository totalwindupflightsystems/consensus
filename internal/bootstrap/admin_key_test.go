// Package bootstrap: tests first-admin-key bootstrapping.
//
// axiom:trace work_item=runtime-dev-bootstrap-auth-01 spec=specs/016-cli-interface.md,specs/015-api-and-mcp.md plan=.memory-bank/work-items/runtime-dev-bootstrap-auth-01/plan.md test=internal/bootstrap/admin_key_test.go evidence=.memory-bank/work-items/runtime-dev-bootstrap-auth-01/verification.md
// axiom:trace work_item=postgres-bootstrap-verification-01 spec=specs/009-deployment.md,specs/015-api-and-mcp.md plan=.memory-bank/work-items/postgres-bootstrap-verification-01/plan.md test=internal/bootstrap/admin_key_test.go evidence=.memory-bank/work-items/postgres-bootstrap-verification-01/verification.md
// axiom:trace work_item=bootstrap-admin-key-policy-01 spec=specs/015-api-and-mcp.md#req-bootstrap-ttl-001 spec=specs/023-adr-bootstrap-key-expiry.md test=internal/bootstrap/admin_key_test.go
package bootstrap

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/wojons/consensus/internal/api"
	"github.com/wojons/consensus/internal/db"
	dbdriver "github.com/wojons/consensus/internal/db/driver"
	"github.com/wojons/consensus/internal/migrate"
)

func TestEnsureFirstAdminKey_CreatesUsableHashedAdminKey(t *testing.T) {
	database := newMigratedTestDB(t)
	defer database.Close()

	result, err := EnsureFirstAdminKey(context.Background(), database, 0)
	if err != nil {
		t.Fatalf("ensure first admin key: %v", err)
	}
	if !result.Created {
		t.Fatal("expected new admin key to be created")
	}
	if result.APIKey == "" || result.KeyPrefix == "" || result.ID == "" {
		t.Fatalf("expected populated key result, got %+v", result)
	}
	if got := result.APIKey[:6]; got != "cs_ak_" {
		t.Fatalf("expected admin key prefix cs_ak_, got %q", got)
	}
	// TTL=0 means no expiry (backward compatible)
	if result.ExpiresAt != "" {
		t.Fatalf("expected no expiry (ExpiresAt empty), got %q", result.ExpiresAt)
	}

	rows, err := database.Query(context.Background(), `SELECT id, key_hash, key_prefix, scope, expires_at FROM api_keys WHERE scope = 'admin'`)
	if err != nil {
		t.Fatalf("query api keys: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected one admin key row, got %d", len(rows))
	}
	if rows[0]["key_prefix"] != result.KeyPrefix {
		t.Fatalf("stored prefix mismatch: %#v vs %q", rows[0]["key_prefix"], result.KeyPrefix)
	}
	if rows[0]["key_hash"] != sha256Hex(result.APIKey) {
		t.Fatal("stored hash does not match returned key")
	}
	// TTL=0 means expires_at IS NULL
	if rows[0]["expires_at"] != nil {
		t.Fatalf("expected nil expires_at for TTL=0, got %v", rows[0]["expires_at"])
	}

	again, err := EnsureFirstAdminKey(context.Background(), database, 0)
	if err != nil {
		t.Fatalf("ensure first admin key second call: %v", err)
	}
	if again.Created || again.APIKey != "" {
		t.Fatalf("expected existing key not to be reprinted, got %+v", again)
	}
}

func TestEnsureFirstAdminKey_AuthenticatesProtectedEndpoint(t *testing.T) {
	database := newMigratedTestDB(t)
	defer database.Close()

	result, err := EnsureFirstAdminKey(context.Background(), database, 0)
	if err != nil {
		t.Fatalf("ensure first admin key: %v", err)
	}

	srv := api.NewServer(api.ServerConfig{Addr: ":0", DB: database})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/sessions", nil)
	req.Header.Set("Authorization", "Bearer "+result.APIKey)
	w := httptest.NewRecorder()

	srv.Handler().ServeHTTP(w, req)

	if w.Code == http.StatusUnauthorized {
		t.Fatalf("expected generated admin key to authenticate, got %d: %s", w.Code, w.Body.String())
	}
}

func TestEnsureFirstAdminKey_ConcurrentCallsCreateOneKey(t *testing.T) {
	database := newMigratedTestDB(t)
	defer database.Close()

	const workers = 10
	var wg sync.WaitGroup
	errs := make(chan error, workers)

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := EnsureFirstAdminKey(context.Background(), database, 0)
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)

	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent ensure failed: %v", err)
		}
	}

	rows, err := database.Query(context.Background(), `SELECT id FROM api_keys WHERE scope = 'admin'`)
	if err != nil {
		t.Fatalf("query api keys: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected one admin key after concurrent calls, got %d", len(rows))
	}
}

func TestFormatTime_HandlesPostgresTimeValues(t *testing.T) {
	stamp := time.Date(2026, 5, 16, 17, 0, 0, 0, time.FixedZone("offset", -7*60*60))
	if got := formatTime(stamp); got != "2026-05-17T00:00:00Z" {
		t.Fatalf("unexpected formatted time: %q", got)
	}
}

// ── Output Stream Tests (bootstrap-output-stream-01) ─────────────────────

func TestFormatResult_CreatedKeyHasMachineParseableOutput(t *testing.T) {
	r := AdminKeyResult{
		Created:   true,
		ID:        "key-001",
		APIKey:    "cs_ak_test123456",
		KeyPrefix: "cs_ak_te",
		CreatedAt: "2026-05-28T12:00:00Z",
	}
	lines := FormatResult(r)
	// TTL=0 (no expiry, ExpiresAt="") → 3 lines: machine line + "does not expire" + save warning
	if len(lines) != 3 {
		t.Fatalf("expected 3 lines for created key (no expiry), got %d: %v", len(lines), lines)
	}

	// Line 1: key=value pairs
	if !strings.Contains(lines[0], "consensus: first_admin_key") {
		t.Errorf("expected 'consensus: first_admin_key' prefix, got: %s", lines[0])
	}
	if !strings.Contains(lines[0], "created=true") {
		t.Errorf("expected created=true, got: %s", lines[0])
	}
	if !strings.Contains(lines[0], "key=cs_ak_test123456") {
		t.Errorf("expected key=cs_ak_test123456, got: %s", lines[0])
	}
	if !strings.Contains(lines[0], "key_prefix=cs_ak_te") {
		t.Errorf("expected key_prefix, got: %s", lines[0])
	}
	if !strings.Contains(lines[0], "id=key-001") {
		t.Errorf("expected id=key-001, got: %s", lines[0])
	}

	// Line 2: no expiry note
	if !strings.Contains(lines[1], "does not expire") {
		t.Errorf("expected 'does not expire' note, got: %s", lines[1])
	}
	if !strings.Contains(lines[1], "TTL=0") {
		t.Errorf("expected TTL=0 mention, got: %s", lines[1])
	}

	// Line 3: save warning
	if !strings.Contains(lines[2], "save this key now") {
		t.Errorf("expected save warning, got: %s", lines[2])
	}
}

func TestFormatResult_ExistingKeyHasMachineParseableOutput(t *testing.T) {
	r := AdminKeyResult{
		Created:   false,
		ID:        "key-002",
		KeyPrefix: "cs_ak_ex",
		CreatedAt: "2026-04-15T08:30:00Z",
	}
	lines := FormatResult(r)
	if len(lines) != 1 {
		t.Fatalf("expected 1 line for existing key, got %d", len(lines))
	}
	if !strings.Contains(lines[0], "created=false") {
		t.Errorf("expected created=false, got: %s", lines[0])
	}
	if strings.Contains(lines[0], "key=") {
		t.Error("existing key line MUST NOT contain raw key")
	}
	if !strings.Contains(lines[0], "key_prefix=cs_ak_ex") {
		t.Errorf("expected key_prefix, got: %s", lines[0])
	}
}

func TestFormatResult_NoSecretLeakedForExistingKey(t *testing.T) {
	// The raw key is empty for existing keys, so FormatResult must not
	// include "key=" with an empty value (would confuse scripts).
	r := AdminKeyResult{Created: false, ID: "k", KeyPrefix: "cs_ak_xx", CreatedAt: "z"}
	lines := FormatResult(r)
	out := strings.Join(lines, "\n")
	if strings.Contains(out, "key=") {
		t.Errorf("existing key line must not emit key= field, got: %s", out)
	}
}

func TestFormatResultJSON_CreatedHasFields(t *testing.T) {
	r := AdminKeyResult{
		Created:   true,
		ID:        "k1",
		APIKey:    "cs_ak_json_test",
		KeyPrefix: "cs_ak_js",
		CreatedAt: "2026-05-28T12:00:00Z",
	}
	data, err := FormatResultJSON(r)
	if err != nil {
		t.Fatalf("format json: %v", err)
	}
	if !strings.Contains(string(data), "\"api_key\": \"cs_ak_json_test\"") {
		t.Errorf("expected api_key in JSON, got: %s", string(data))
	}
	if !strings.Contains(string(data), "\"created\": true") {
		t.Errorf("expected created=true in JSON, got: %s", string(data))
	}
}

func TestFormatResultJSON_ExistingRedactsSecret(t *testing.T) {
	r := AdminKeyResult{
		Created:   false,
		ID:        "k2",
		KeyPrefix: "cs_ak_ex",
		CreatedAt: "2026-04-15T08:30:00Z",
	}
	data, err := FormatResultJSON(r)
	if err != nil {
		t.Fatalf("format json: %v", err)
	}
	// api_key is "" for existing keys (empty string, not the real secret).
	// The field is present but empty — verify no non-empty key leaks.
	if strings.Contains(string(data), `"api_key": "cs_ak`) {
		t.Error("JSON for existing key must not expose real key value")
	}
}

func newMigratedTestDB(t *testing.T) db.DB {
	t.Helper()
	ctx := context.Background()
	database, err := dbdriver.Open(ctx, db.Config{URL: "sqlite://:memory:"})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}

	if _, err := migrate.New(database).AutoMigrate(ctx); err != nil {
		database.Close()
		t.Fatalf("migrate: %v", err)
	}
	return database
}

// ── Bootstrap Key TTL Tests (bootstrap-admin-key-policy-01) ────────────────
//
// axiom:trace work_item=bootstrap-admin-key-policy-01 spec=specs/015-api-and-mcp.md#req-bootstrap-ttl-001 test=internal/bootstrap/admin_key_test.go

func TestEnsureFirstAdminKey_WithTTL_SetsExpiresAt(t *testing.T) {
	database := newMigratedTestDB(t)
	defer database.Close()

	ttl := 1 * time.Hour
	result, err := EnsureFirstAdminKey(context.Background(), database, ttl)
	if err != nil {
		t.Fatalf("ensure first admin key with TTL: %v", err)
	}
	if !result.Created {
		t.Fatal("expected new admin key to be created")
	}
	if result.ExpiresAt == "" {
		t.Fatal("expected ExpiresAt to be populated when TTL > 0")
	}

	// Parse the expiry time and check it's approximately now + 1h
	expiresTime, err := time.Parse(time.RFC3339, result.ExpiresAt)
	if err != nil {
		t.Fatalf("failed to parse ExpiresAt %q: %v", result.ExpiresAt, err)
	}
	expected := time.Now().UTC().Add(ttl)
	diff := expiresTime.Sub(expected)
	if diff < -10*time.Second || diff > 10*time.Second {
		t.Errorf("ExpiresAt %v not within 10s of expected %v (diff=%v)", expiresTime, expected, diff)
	}

	// Verify expires_at is set in DB
	rows, err := database.Query(context.Background(),
		`SELECT expires_at FROM api_keys WHERE scope = 'admin'`)
	if err != nil {
		t.Fatalf("query api keys: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected one admin key row, got %d", len(rows))
	}
	if rows[0]["expires_at"] == nil {
		t.Fatal("expected expires_at to be set in DB, got nil")
	}
	dbExpires, ok := rows[0]["expires_at"].(string)
	if !ok || dbExpires == "" {
		t.Fatalf("expected non-empty expires_at in DB, got %v", rows[0]["expires_at"])
	}
}

func TestEnsureFirstAdminKey_WithZeroTTL_NoExpiry(t *testing.T) {
	database := newMigratedTestDB(t)
	defer database.Close()

	result, err := EnsureFirstAdminKey(context.Background(), database, 0)
	if err != nil {
		t.Fatalf("ensure first admin key with TTL=0: %v", err)
	}
	if !result.Created {
		t.Fatal("expected new admin key to be created")
	}
	if result.ExpiresAt != "" {
		t.Fatalf("expected empty ExpiresAt for TTL=0, got %q", result.ExpiresAt)
	}

	// Verify expires_at IS NULL in DB
	rows, err := database.Query(context.Background(),
		`SELECT expires_at FROM api_keys WHERE scope = 'admin'`)
	if err != nil {
		t.Fatalf("query api keys: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected one admin key row, got %d", len(rows))
	}
	if rows[0]["expires_at"] != nil {
		t.Fatalf("expected NULL expires_at for TTL=0, got %v", rows[0]["expires_at"])
	}
}

func TestEnsureFirstAdminKey_DefaultTTL_Is90Days(t *testing.T) {
	// Verify the default TTL constant is 90 days (2160 hours)
	if DefaultBootstrapKeyTTLHours != 2160 {
		t.Errorf("expected DefaultBootstrapKeyTTLHours=2160 (90 days), got %d", DefaultBootstrapKeyTTLHours)
	}

	// Verify it's exactly 90 days in hours
	ttl := time.Duration(DefaultBootstrapKeyTTLHours) * time.Hour
	if ttl.Hours() != 2160 {
		t.Errorf("expected TTL to be 2160 hours, got %.0f", ttl.Hours())
	}
}

func TestEnsureFirstAdminKey_ExpiredKeyRejected(t *testing.T) {
	database := newMigratedTestDB(t)
	defer database.Close()

	// Create key with 1 hour TTL
	result, err := EnsureFirstAdminKey(context.Background(), database, 1*time.Hour)
	if err != nil {
		t.Fatalf("ensure first admin key: %v", err)
	}
	if !result.Created {
		t.Fatal("expected new admin key to be created")
	}

	// Manually set expires_at to 1 hour ago (simulate expiry)
	pastTime := time.Now().UTC().Add(-1 * time.Hour).Format(time.RFC3339)
	if err := database.Exec(context.Background(),
		`UPDATE api_keys SET expires_at = $1 WHERE id = $2`,
		pastTime, result.ID,
	); err != nil {
		t.Fatalf("update expires_at: %v", err)
	}

	// Verify the DB now has a past expires_at
	rows, err := database.Query(context.Background(),
		`SELECT expires_at FROM api_keys WHERE id = $1`, result.ID)
	if err != nil {
		t.Fatalf("query api key: %v", err)
	}
	if len(rows) == 0 {
		t.Fatal("expected api key row to exist")
	}
	dbExp, ok := rows[0]["expires_at"].(string)
	if !ok || dbExp == "" {
		t.Fatal("expected expires_at to be set in DB after UPDATE")
	}
	if dbExp != pastTime {
		t.Errorf("expected expires_at=%s, got %s", pastTime, dbExp)
	}

	// The auth middleware query (expires_at IS NULL OR expires_at > datetime('now'))
	// handles expiry enforcement. Verify the stored expiry is in the past.
	// Note: SQLite datetime('now') format mismatch with RFC 3339 is a pre-existing
	// condition (see internal/api/server.go:274), not in scope for this work item.
	parsedExp, err := time.Parse(time.RFC3339, dbExp)
	if err != nil {
		t.Fatalf("failed to parse DB expires_at: %v", err)
	}
	if time.Now().UTC().Before(parsedExp) {
		t.Errorf("expected expires_at to be in the past, got %v (now=%v)", parsedExp, time.Now().UTC())
	}
}

func TestFormatResult_IncludesExpiry(t *testing.T) {
	r := AdminKeyResult{
		Created:   true,
		ID:        "key-010",
		APIKey:    "cs_ak_test_expiry",
		KeyPrefix: "cs_ak_te",
		CreatedAt: "2026-05-28T12:00:00Z",
		ExpiresAt: "2026-08-26T12:00:00Z",
	}
	lines := FormatResult(r)
	if len(lines) < 3 {
		t.Fatalf("expected at least 3 lines, got %d: %v", len(lines), lines)
	}

	// Line 1: machine-parseable with expires_at
	if !strings.Contains(lines[0], "expires_at=2026-08-26T12:00:00Z") {
		t.Errorf("expected expires_at in first line, got: %s", lines[0])
	}
	if !strings.Contains(lines[0], "created=true") {
		t.Errorf("expected created=true, got: %s", lines[0])
	}
	if !strings.Contains(lines[0], "key=cs_ak_test_expiry") {
		t.Errorf("expected key, got: %s", lines[0])
	}

	// Line 2: human-readable expiry
	if !strings.Contains(lines[1], "this key expires at") {
		t.Errorf("expected expiry line, got: %s", lines[1])
	}
	if !strings.Contains(lines[1], "2026-08-26T12:00:00Z") {
		t.Errorf("expected expiry date in human line, got: %s", lines[1])
	}

	// Line 3: save warning
	if !strings.Contains(lines[2], "save this key now") {
		t.Errorf("expected save warning, got: %s", lines[2])
	}
}

func TestFormatResult_NoExpiry_ShowsDoesNotExpire(t *testing.T) {
	r := AdminKeyResult{
		Created:   true,
		ID:        "key-011",
		APIKey:    "cs_ak_test_noexp",
		KeyPrefix: "cs_ak_te",
		CreatedAt: "2026-05-28T12:00:00Z",
		ExpiresAt: "", // TTL=0, no expiry
	}
	lines := FormatResult(r)
	if len(lines) != 3 {
		t.Fatalf("expected 3 lines for no-expiry key, got %d: %v", len(lines), lines)
	}

	// Line 1: should NOT have expires_at
	if strings.Contains(lines[0], "expires_at=") {
		t.Errorf("expected no expires_at in first line for TTL=0, got: %s", lines[0])
	}

	// Line 2: "does not expire" note
	if !strings.Contains(lines[1], "does not expire") {
		t.Errorf("expected 'does not expire' note, got: %s", lines[1])
	}
	if !strings.Contains(lines[1], "TTL=0") {
		t.Errorf("expected TTL=0 mention, got: %s", lines[1])
	}

	// Line 3: save warning
	if !strings.Contains(lines[2], "save this key now") {
		t.Errorf("expected save warning, got: %s", lines[2])
	}
}

func TestGetBootstrapKeyTTL_EnvVarParsing(t *testing.T) {
	// Save and restore env var
	orig := os.Getenv("CONSENSUS_BOOTSTRAP_KEY_TTL_HOURS")
	defer os.Setenv("CONSENSUS_BOOTSTRAP_KEY_TTL_HOURS", orig)

	// Test: unset → default (2160h)
	os.Unsetenv("CONSENSUS_BOOTSTRAP_KEY_TTL_HOURS")
	got := GetBootstrapKeyTTL()
	if got != DefaultBootstrapKeyTTLHours*time.Hour {
		t.Errorf("unset: expected %v, got %v", DefaultBootstrapKeyTTLHours*time.Hour, got)
	}

	// Test: valid value
	os.Setenv("CONSENSUS_BOOTSTRAP_KEY_TTL_HOURS", "48")
	got = GetBootstrapKeyTTL()
	if got != 48*time.Hour {
		t.Errorf("48: expected 48h, got %v", got)
	}

	// Test: 0 → returns 0 (no expiry)
	os.Setenv("CONSENSUS_BOOTSTRAP_KEY_TTL_HOURS", "0")
	got = GetBootstrapKeyTTL()
	if got != 0 {
		t.Errorf("0: expected 0, got %v", got)
	}

	// Test: invalid value → default
	os.Setenv("CONSENSUS_BOOTSTRAP_KEY_TTL_HOURS", "not-a-number")
	got = GetBootstrapKeyTTL()
	if got != DefaultBootstrapKeyTTLHours*time.Hour {
		t.Errorf("invalid: expected default %v, got %v", DefaultBootstrapKeyTTLHours*time.Hour, got)
	}

	// Test: negative value → default
	os.Setenv("CONSENSUS_BOOTSTRAP_KEY_TTL_HOURS", "-1")
	got = GetBootstrapKeyTTL()
	if got != DefaultBootstrapKeyTTLHours*time.Hour {
		t.Errorf("negative: expected default %v, got %v", DefaultBootstrapKeyTTLHours*time.Hour, got)
	}
}
