// seed_default_chat_model_test.go — REVIEW-CONSENSUS-3 Finding 2 regression
// coverage: a fresh database must carry an enabled, chat-capable model_registry
// row after migrations, so sessions.model_id -> model_registry(model_id)
// (fk_sessions_model) has a valid target and CreateSession never has to fall
// back to the "default" sentinel.
//
// Two layers:
//
//  1. TestSeedDefaultChatModelSQLite — backend-agnostic (no live Postgres):
//     migrates a scratch SQLite database and asserts the seed row is present,
//     chat-capable, and is what the API's own model-selection query returns.
//
//  2. TestPostgresSeedDefaultChatModel — live Postgres only (gated by
//     CONSENSUS_TEST_POSTGRES_URL). Asserts the seed row exists after
//     migrations, that a session INSERT with it satisfies the FK, and — the
//     negative control that makes the fix load-bearing — that the pre-fix
//     sentinel ("default") still violates the FK.
//
// axiom:trace work_item=REVIEW-CONSENSUS-3
//   spec=specs/003-database.md,specs/015-api-and-mcp.md
//   impl=internal/migrate/migrations/029_seed_default_chat_model.sql

package migrate

import (
	"context"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/wojons/consensus/internal/db"
	"github.com/wojons/consensus/internal/db/driver"
	pgpostgres "github.com/wojons/consensus/internal/db/postgres"
)

// seededDefaultModel is the model the shipped consensus.yaml names as its
// llm.default_model and that migration 029 registers.
const seededDefaultModel = "deepseek-v4-flash"

// createSessionModelSelectionQuery is the exact selection CreateSession
// (internal/api/service.go) runs when the caller supplies no model_id: cheapest
// enabled non-embedding model. Keep it in lockstep with the service — if that
// query changes shape, this test must keep asserting the real one.
const createSessionModelSelectionQuery = `SELECT model_id FROM model_registry
			WHERE enabled = true AND model_id NOT LIKE 'text-embedding%'
			ORDER BY tier ASC, cost_per_m_in ASC LIMIT 1`

// modelRegistryRow reads the registry row for modelID (nil when absent).
func modelRegistryRow(t *testing.T, database db.DB, modelID string) db.Row {
	t.Helper()
	rows, err := database.Query(context.Background(),
		`SELECT model_id, tier, max_context, cost_per_m_in, cost_per_m_out, classifier_tags, enabled
		 FROM model_registry WHERE model_id = $1`, modelID)
	if err != nil {
		t.Fatalf("query model_registry(%s): %v", modelID, err)
	}
	if len(rows) == 0 {
		return nil
	}
	if len(rows) > 1 {
		t.Fatalf("model_registry has %d rows for %s, want 1", len(rows), modelID)
	}
	return rows[0]
}

// assertChatSeedRow asserts the shared shape of the 029 seed row, whatever the
// backend's column types are.
func assertChatSeedRow(t *testing.T, database db.DB) {
	t.Helper()
	row := modelRegistryRow(t, database, seededDefaultModel)
	if row == nil {
		t.Fatalf("model_registry has no %q row after migrations — a fresh database "+
			"has no chat-capable FK target for sessions.model_id and CreateSession "+
			"falls back to the rejected \"default\" sentinel", seededDefaultModel)
	}
	if !isTruthy(row["enabled"]) {
		t.Errorf("seed row %s enabled=%v (%T), want true", seededDefaultModel, row["enabled"], row["enabled"])
	}
	tags := strings.ToLower(toStringValue(row["classifier_tags"]))
	if !strings.Contains(tags, "chat") {
		t.Errorf("seed row %s classifier_tags=%v does not carry the 'chat' tag", seededDefaultModel, row["classifier_tags"])
	}
	if tags := strings.ToLower(toStringValue(row["classifier_tags"])); strings.Contains(tags, "embedding") {
		t.Errorf("seed row %s is tagged embedding — it must stay a chat-candidate", seededDefaultModel)
	}
	if ctx := toIntValue(row["max_context"]); ctx <= 0 {
		t.Errorf("seed row %s max_context=%v, want > 0", seededDefaultModel, row["max_context"])
	}
}

// assertSelectionPicksRegisteredModel runs the API's own selection query and
// asserts the result is a registry row — never the "default" sentinel the FK
// rejects.
func assertSelectionPicksRegisteredModel(t *testing.T, database db.DB) {
	t.Helper()
	rows, err := database.Query(context.Background(), createSessionModelSelectionQuery)
	if err != nil {
		t.Fatalf("CreateSession model selection query: %v", err)
	}
	if len(rows) == 0 {
		t.Fatal("CreateSession model selection returns no row on a migrated database — " +
			"it would fall back to the \"default\" sentinel and violate fk_sessions_model")
	}
	selected := toStringValue(rows[0]["model_id"])
	if selected == "" || selected == "default" {
		t.Fatalf("selection returned %q, want a registered model id", selected)
	}
	if modelRegistryRow(t, database, selected) == nil {
		t.Fatalf("selection returned %q, which has no model_registry row — fk_sessions_model would reject it", selected)
	}
	if strings.HasPrefix(selected, "text-embedding") {
		t.Fatalf("selection returned the embedding model %q", selected)
	}
	t.Logf("✓ CreateSession selects registered chat model %q", selected)
}

// TestSeedDefaultChatModelSQLite is the backend-agnostic half: it runs without
// Postgres, so `go test -short ./internal/migrate/...` exercises it.
func TestSeedDefaultChatModelSQLite(t *testing.T) {
	ctx := context.Background()
	database, cleanup := setupTestDB(t)
	defer cleanup()

	if _, err := New(database).AutoMigrate(ctx); err != nil {
		t.Fatalf("AutoMigrate failed: %v", err)
	}

	assertChatSeedRow(t, database)
	assertSelectionPicksRegisteredModel(t, database)

	// The seed must be re-runnable and must not clobber an existing row: a
	// second apply of the migration SQL leaves the row untouched.
	runner := &Runner{}
	if err := runner.LoadMigrations(); err != nil {
		t.Fatalf("LoadMigrations failed: %v", err)
	}
	var seedSQL string
	for _, m := range runner.migrations {
		if m.Filename == "029_seed_default_chat_model.sql" {
			seedSQL = m.SQL
		}
	}
	if seedSQL == "" {
		t.Fatal("migration 029 is not embedded — a fresh Postgres DB would still reject session creation")
	}
	// Simulate a pre-existing syncer registration with different pricing.
	if err := database.Exec(ctx,
		`UPDATE model_registry SET cost_per_m_in = 9.99, tier = 3 WHERE model_id = $1`, seededDefaultModel); err != nil {
		t.Fatalf("mutate seed row: %v", err)
	}
	for _, stmt := range splitStatements(filterForSQLite(seedSQL)) {
		if strings.TrimSpace(strings.ToUpper(stmt)) == "" {
			continue
		}
		if err := database.Exec(ctx, stmt); err != nil {
			t.Fatalf("re-apply 029 statement %q: %v", stmt, err)
		}
	}
	row := modelRegistryRow(t, database, seededDefaultModel)
	if row == nil {
		t.Fatal("re-applying 029 dropped the seed row")
	}
	if got := toFloatValue(row["cost_per_m_in"]); got != 9.99 {
		t.Errorf("re-applying 029 overwrote an existing registration: cost_per_m_in=%v, want 9.99 (ON CONFLICT DO NOTHING)", got)
	}
	if got := toIntValue(row["tier"]); got != 3 {
		t.Errorf("re-applying 029 overwrote tier: got %v, want 3 (ON CONFLICT DO NOTHING)", got)
	}
}

// TestPostgresSeedDefaultChatModel is the live half: after migrations a fresh
// Postgres database must have the seed row, a session INSERT with it must
// satisfy fk_sessions_model, and the pre-fix sentinel must still be rejected.
// Gated by CONSENSUS_TEST_POSTGRES_URL.
func TestPostgresSeedDefaultChatModel(t *testing.T) {
	pgURL := os.Getenv("CONSENSUS_TEST_POSTGRES_URL")
	if pgURL == "" {
		t.Skip("CONSENSUS_TEST_POSTGRES_URL not set; skipping Postgres default-model seed test")
	}

	ctx := context.Background()
	database, err := driver.Open(ctx, db.Config{URL: pgURL})
	if err != nil {
		t.Fatalf("failed to connect to Postgres: %v", err)
	}
	defer func() { _ = database.Close() }()

	if database.Backend() != db.BackendPostgres {
		t.Fatalf("CONSENSUS_TEST_POSTGRES_URL names backend %s, want postgres", database.Backend())
	}

	// Migrations run as the table owner (they own the tables); sessions is
	// RLS-protected for the agent_role pool.
	adminDB := database
	if pgdb, ok := database.(*pgpostgres.DB); ok {
		adminDB = pgdb.AdminDB()
	}
	if err := grantAgentRoleSchemaPrivs(ctx, pgURL); err != nil {
		t.Logf("skip agent_role schema grant (%v) — assuming fresh DB", err)
	}

	runner := New(adminDB)
	if _, err := runner.AutoMigrate(ctx); err != nil {
		t.Fatalf("AutoMigrate failed: %v", err)
	}

	assertChatSeedRow(t, adminDB)
	assertSelectionPicksRegisteredModel(t, adminDB)

	const probeSession = "00000000-0000-0000-0000-0000000000d9"
	cleanupSession := func() {
		_ = adminDB.Exec(context.Background(), `DELETE FROM sessions WHERE id = $1`, probeSession)
	}
	cleanupSession()
	t.Cleanup(cleanupSession)

	// (1) The fix: a session with the seeded model satisfies the FK.
	if err := adminDB.Exec(ctx,
		`INSERT INTO sessions (id, agent_name, model_id, status)
		 VALUES ($1, 'seed-probe-agent', $2, 'idle')`, probeSession, seededDefaultModel); err != nil {
		t.Fatalf("session insert with seeded model %q failed fk_sessions_model: %v", seededDefaultModel, err)
	}

	// (2) Negative control: the sentinel the API used to fall back to still
	// violates the FK — so this migration (not a dropped constraint) is what
	// unblocked session creation.
	err = adminDB.Exec(ctx,
		`INSERT INTO sessions (id, agent_name, model_id, status)
		 VALUES ($1, 'seed-probe-agent', 'default', 'idle')`,
		"00000000-0000-0000-0000-0000000000da")
	if err == nil {
		_ = adminDB.Exec(ctx, `DELETE FROM sessions WHERE id = '00000000-0000-0000-0000-0000000000da'`)
		t.Fatal("inserting a session with model_id='default' SUCCEEDED — fk_sessions_model is gone, " +
			"so this test cannot prove the seed is what unblocked session creation")
	}
	if !strings.Contains(err.Error(), "fk_sessions_model") && !strings.Contains(err.Error(), "foreign key") {
		t.Errorf("sentinel insert failed with %v, want a fk_sessions_model violation", err)
	} else {
		t.Logf("✓ sentinel still rejected: %v", err)
	}
}

// toStringValue/toIntValue/toFloatValue normalize a db.Row value across
// backends (SQLite returns TEXT/INTEGER/REAL, Postgres returns column types).
// classifier_tags is TEXT[] on Postgres (pgx hands back []string / []any) and
// TEXT on SQLite, so both shapes are folded into one string here.
func toStringValue(v any) string {
	switch s := v.(type) {
	case string:
		return s
	case []byte:
		return string(s)
	case []string:
		return strings.Join(s, ",")
	case []any:
		parts := make([]string, 0, len(s))
		for _, e := range s {
			parts = append(parts, toStringValue(e))
		}
		return strings.Join(parts, ",")
	default:
		return ""
	}
}

// isTruthy accepts the boolean shapes both backends use for a BOOLEAN column:
// Postgres returns bool, SQLite returns 1/0 as an integer or the string "true".
func isTruthy(v any) bool {
	switch b := v.(type) {
	case bool:
		return b
	case int:
		return b != 0
	case int32:
		return b != 0
	case int64:
		return b != 0
	case string:
		return b == "1" || strings.EqualFold(b, "true")
	default:
		return false
	}
}

func toIntValue(v any) int {
	switch n := v.(type) {
	case int:
		return n
	case int32:
		return int(n)
	case int64:
		return int(n)
	case float64:
		return int(n)
	default:
		return 0
	}
}

func toFloatValue(v any) float64 {
	switch n := v.(type) {
	case float64:
		return n
	case float32:
		return float64(n)
	case int:
		return float64(n)
	case int32:
		return float64(n)
	case int64:
		return float64(n)
	case []byte:
		return parseFloat(string(n))
	case string:
		return parseFloat(n)
	default:
		return 0
	}
}

func parseFloat(s string) float64 {
	f, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
	if err != nil {
		return 0
	}
	return f
}
