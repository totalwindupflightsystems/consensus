// postgres_grants_test.go — PG-GRANT-42501 regression coverage.
//
// The application pool runs every connection under `SET ROLE agent_role`
// (internal/db/postgres/postgres.go, AfterConnect). Several migrations created
// tables that the pool queries but never granted them to that role, so those
// statements failed with SQLSTATE 42501 ("permission denied for table ...") —
// observed live as `webhook: fetch pending events: permission denied for table
// external_events`.
//
// Two layers of coverage:
//
//  1. TestGrantMigrationCoversPoolEnumeration — backend-agnostic (no live
//     Postgres needed). Asserts migration 028 is embedded and that its GRANT
//     statements cover EXACTLY the enumerated (table, verb) set — so the file
//     cannot drift from the code-derived enumeration, and no table outside the
//     enumeration is silently granted.
//
//  2. TestPostgresAgentRolePoolTablePrivileges — live Postgres only (gated by
//     CONSENSUS_TEST_POSTGRES_URL, skipped otherwise). Asserts
//     has_table_privilege('agent_role', <table>, <verb>) for the whole
//     enumeration, drives the reported webhook-poller lifecycle through a pool
//     that really runs as agent_role, and pins the append-only invariant that
//     this migration must NOT widen.
//
// axiom:trace work_item=PG-GRANT-42501
//   spec=specs/003-database.md,specs/005-security.md,specs/013-webhooks-and-events.md
//   impl=internal/migrate/postgres_grants_test.go

package migrate

import (
	"context"
	"os"
	"sort"
	"strings"
	"testing"

	"github.com/wojons/consensus/internal/db"
	"github.com/wojons/consensus/internal/db/driver"
	pgpostgres "github.com/wojons/consensus/internal/db/postgres"
)

// grantMigrationVersion is the migration that grants the non-admin pool role
// (agent_role) the privileges its own SQL needs.
const grantMigrationVersion = 28

// seqGrantMigrationVersion is the migration that grants agent_role USAGE on
// the sequences its BIGSERIAL-backed INSERTs draw from (PG-SEQ-42501).
const seqGrantMigrationVersion = 30

// poolPrivileges is the enumerated (table -> verbs) set derived from a census of
// the SQL in internal/** + cmd/** (non-test) that runs on a non-admin handle.
// It is the contract migration 028 must satisfy — keep the two in sync.
var poolPrivileges = map[string][]string{
	// 007 — webhook ingestion + routing (the reported failure).
	"external_events":       {"SELECT", "INSERT", "UPDATE"},
	"webhook_registrations": {"SELECT", "INSERT", "UPDATE", "DELETE"},
	"routing_rules":         {"SELECT"},
	// 008 — HITL interrupt state.
	"approval_requests":  {"SELECT", "INSERT", "UPDATE"},
	"hitl_configuration": {"SELECT", "INSERT", "DELETE"},
	"notification_log":   {"INSERT"},
	// 002 — shim session map + API keys.
	"api_keys":         {"SELECT", "INSERT", "UPDATE"},
	"shim_session_map": {"INSERT"},
	// 003 — circuit breakers (harness upsert: ON CONFLICT DO UPDATE).
	"agent_circuit_breakers": {"SELECT", "INSERT", "UPDATE"},
	// 015 / 016 — scope model + embedding store.
	"projects":         {"SELECT"},
	"event_embeddings": {"SELECT"},
	// 001 — created but never granted to the pool role.
	"audit_logs":        {"SELECT", "INSERT"},
	"compression_queue": {"SELECT", "UPDATE"},
	"system_settings":   {"SELECT", "INSERT", "UPDATE", "DELETE"},
	"tool_results":      {"SELECT", "INSERT"},
	"schema_versions":   {"SELECT"},
	// 001/004 — granted but missing a verb the pool executes.
	"external_quarantine": {"UPDATE"},
	"iteration_commits":   {"UPDATE"},
	"tool_requests":       {"UPDATE"},
	"model_registry":      {"INSERT", "UPDATE"},
}

// poolPrivilegeTables returns the enumerated tables in deterministic order.
func poolPrivilegeTables() []string {
	tables := make([]string, 0, len(poolPrivileges))
	for table := range poolPrivileges {
		tables = append(tables, table)
	}
	sort.Strings(tables)
	return tables
}

// grantStatements parses `GRANT <verbs> ON <table> TO agent_role;` lines out of
// migration SQL, ignoring comment lines. Verbs are returned upper-cased and
// sorted.
func grantStatements(t *testing.T, sql string) map[string][]string {
	t.Helper()
	grants := make(map[string][]string)
	for _, line := range strings.Split(sql, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "--") {
			continue
		}
		upper := strings.ToUpper(trimmed)
		if !strings.HasPrefix(upper, "GRANT ") || !strings.Contains(upper, " TO AGENT_ROLE") {
			continue
		}
		body := strings.TrimSuffix(strings.TrimSpace(upper), ";")
		// GRANT SELECT, INSERT ON <table> TO AGENT_ROLE
		parts := strings.SplitN(body, " ON ", 2)
		if len(parts) != 2 {
			t.Fatalf("unparseable GRANT statement: %q", trimmed)
		}
		verbs := strings.Split(strings.TrimPrefix(parts[0], "GRANT "), ",")
		table := strings.TrimSpace(strings.SplitN(parts[1], " TO ", 2)[0])
		out := make([]string, 0, len(verbs))
		for _, v := range verbs {
			out = append(out, strings.TrimSpace(v))
		}
		sort.Strings(out)
		grants[strings.ToLower(table)] = out
	}
	return grants
}

// TestGrantMigrationCoversPoolEnumeration is the backend-agnostic half: it runs
// without Postgres (so `go test -short ./internal/migrate/...` exercises it) and
// fails if migration 028 is missing, if it under-grants (a pool query would
// still 42501) or if it over-grants (a table outside the enumerated set).
func TestGrantMigrationCoversPoolEnumeration(t *testing.T) {
	runner := &Runner{}
	if err := runner.LoadMigrations(); err != nil {
		t.Fatalf("LoadMigrations failed: %v", err)
	}

	var sql string
	for _, m := range runner.migrations {
		if m.Version == grantMigrationVersion {
			sql = m.SQL
			break
		}
	}
	if sql == "" {
		t.Fatalf("migration %d (grant pool tables to agent_role) is not embedded — "+
			"the pool would keep hitting SQLSTATE 42501", grantMigrationVersion)
	}

	grants := grantStatements(t, sql)
	if len(grants) == 0 {
		t.Fatalf("migration %d contains no GRANT ... TO agent_role statements", grantMigrationVersion)
	}

	for _, table := range poolPrivilegeTables() {
		want := append([]string(nil), poolPrivileges[table]...)
		sort.Strings(want)
		got, ok := grants[table]
		if !ok {
			t.Errorf("migration %d does not grant %s to agent_role (pool queries it)", grantMigrationVersion, table)
			continue
		}
		if strings.Join(got, ",") != strings.Join(want, ",") {
			t.Errorf("migration %d grants %s=[%s], want [%s]",
				grantMigrationVersion, table, strings.Join(got, ","), strings.Join(want, ","))
		}
	}

	for table := range grants {
		if _, ok := poolPrivileges[table]; !ok {
			t.Errorf("migration %d grants %s, which is not in the enumerated pool set — "+
				"widen the enumeration deliberately or drop the grant", grantMigrationVersion, table)
		}
	}

	// The append-only ledger must never be re-granted here (001 REVOKEs it).
	if strings.Contains(strings.ToUpper(sql), "ON MEMORY_EVENTS TO AGENT_ROLE") {
		t.Errorf("migration %d re-grants memory_events to agent_role — "+
			"append-only enforcement is a REVOKE in 001", grantMigrationVersion)
	}
}

// pgHasTablePrivilege asks Postgres directly whether agent_role holds verb on
// table. has_table_privilege reports the named role's privileges regardless of
// the caller, so running it on the owner handle is a genuine assertion about the
// pool role.
func pgHasTablePrivilege(t *testing.T, database db.DB, table, verb string) bool {
	t.Helper()
	rows, err := database.Query(context.Background(),
		`SELECT has_table_privilege('agent_role', $1, $2) AS ok`, table, verb)
	if err != nil {
		t.Fatalf("has_table_privilege(agent_role, %s, %s): %v", table, verb, err)
	}
	if len(rows) != 1 {
		t.Fatalf("has_table_privilege(%s, %s): rows=%d, want 1", table, verb, len(rows))
	}
	ok, _ := rows[0]["ok"].(bool)
	return ok
}

// pgCurrentUser returns current_user for the connection the query runs on.
func pgCurrentUser(t *testing.T, database db.DB) string {
	t.Helper()
	rows, err := database.Query(context.Background(), `SELECT current_user AS who`)
	if err != nil || len(rows) != 1 {
		t.Fatalf("query current_user: %v (rows=%d)", err, len(rows))
	}
	who, _ := rows[0]["who"].(string)
	return who
}

// pgInt64 normalises a pgx integer-ish value (int32/int64/int) to int64.
func pgInt64(v any) int64 {
	switch n := v.(type) {
	case int64:
		return n
	case int32:
		return int64(n)
	case int:
		return int64(n)
	default:
		return 0
	}
}

// countPrivilegeVerbs totals the enumerated (table, verb) pairs.
func countPrivilegeVerbs() int {
	n := 0
	for _, verbs := range poolPrivileges {
		n += len(verbs)
	}
	return n
}

// TestPostgresAgentRolePoolTablePrivileges is the live half: with all migrations
// applied, every enumerated (table, verb) must be privileged for agent_role, the
// reported webhook-poller lifecycle must execute on a real agent_role pool
// without 42501, and the append-only REVOKE on memory_events must still hold.
//
// Gated by CONSENSUS_TEST_POSTGRES_URL. On SQLite (or without the env var) it
// skips — the backend-agnostic structural test above still runs.
func TestPostgresAgentRolePoolTablePrivileges(t *testing.T) {
	pgURL := os.Getenv("CONSENSUS_TEST_POSTGRES_URL")
	if pgURL == "" {
		t.Skip("CONSENSUS_TEST_POSTGRES_URL not set; skipping live Postgres grant regression test")
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

	// Migrations run as the table owner (GRANT requires ownership).
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

	// The pool opened BEFORE the migrations is not usable for the assertions
	// below on a fresh database: driver.Open() pings the pool, so its first
	// connection predates migration 001's CREATE ROLE agent_role and the
	// AfterConnect hook downgrades the missing role to a no-op
	// (postgres.go:50) — pgxpool then reuses that unprivileged connection.
	// Open a POST-migration pool: that is the connection state a restarted
	// service has. (The stale-connection window itself is worth a follow-up on
	// the Go side; it is not something a migration can fix.)
	if who := pgCurrentUser(t, database); who != "agent_role" {
		t.Logf("note: the pre-migration pool connection runs as %q (SET ROLE hook "+
			"no-op'd before the role existed); reopening a post-migration pool", who)
	}
	poolDB, err := driver.Open(ctx, db.Config{URL: pgURL})
	if err != nil {
		t.Fatalf("failed to open post-migration pool: %v", err)
	}
	defer func() { _ = poolDB.Close() }()

	poolAdmin := poolDB
	if pgdb, ok := poolDB.(*pgpostgres.DB); ok {
		poolAdmin = pgdb.AdminDB()
	}

	// Guard against a hollow green: the pool under test must really run as
	// agent_role, otherwise every privilege/error assertion below is meaningless.
	if who := pgCurrentUser(t, poolDB); who != "agent_role" {
		t.Fatalf("pool runs as %q, want agent_role — SET ROLE hook did not engage, "+
			"so this test would prove nothing", who)
	}

	// (1) The enumerated privilege set — asserted against the role, not the pool.
	missing := 0
	for _, table := range poolPrivilegeTables() {
		for _, verb := range poolPrivileges[table] {
			if !pgHasTablePrivilege(t, adminDB, table, verb) {
				missing++
				t.Errorf("agent_role lacks %s on %s — pool statements on that table raise SQLSTATE 42501", verb, table)
			}
		}
	}
	if missing == 0 {
		t.Logf("✓ agent_role holds all %d enumerated privileges across %d tables",
			countPrivilegeVerbs(), len(poolPrivileges))
	}

	// (2) The reported failure, end to end: the webhook poller statements from
	// internal/webhook/webhook.go, issued through the agent_role pool. Seed the
	// rows as the owner (webhook ingest writes them through the pool).
	if err := poolAdmin.Exec(ctx,
		`DELETE FROM external_events WHERE source = 'grant-probe'`); err != nil {
		t.Fatalf("reset probe events: %v", err)
	}
	if err := poolAdmin.Exec(ctx,
		`INSERT INTO external_events (source, source_id, event_type, payload, headers, signature_valid, status)
		 VALUES ('grant-probe', 'evt-1', 'ping', '{}', '{}', 1, 'pending')`); err != nil {
		t.Fatalf("seed external_events: %v", err)
	}

	// routePendingEvents: the exact poller statement that raised 42501 live
	// ("webhook: fetch pending events: permission denied for table external_events").
	rows, err := poolDB.Query(ctx, `
		SELECT id, source, source_id, event_type, payload, headers, signature_valid, session_id, workflow_id, status
		FROM external_events
		WHERE status = 'pending'
		ORDER BY created_at ASC
		LIMIT 100
	`)
	if err != nil {
		t.Fatalf("webhook poller SELECT on external_events (the reported 42501): %v", err)
	}
	var eventID int64
	found := false
	for _, row := range rows {
		if src, _ := row["source"].(string); src == "grant-probe" {
			found = true
			eventID = pgInt64(row["id"])
		}
	}
	if !found {
		t.Fatalf("probe event not visible to the agent_role pool (rows=%d)", len(rows))
	}

	// Routing transition + delivery-outcome transition (the 026 column).
	if err := poolDB.Exec(ctx, `
		UPDATE external_events
		SET session_id = $1, workflow_id = $2, status = 'routed', processed_at = $3
		WHERE id = $4
	`, nil, nil, nil, eventID); err != nil {
		t.Fatalf("webhook routed UPDATE on external_events: %v", err)
	}
	if err := poolDB.Exec(ctx, `
		UPDATE external_events SET status = $1, last_error = $2, processed_at = $3 WHERE id = $4
	`, "failed", "probe delivery error", nil, eventID); err != nil {
		t.Fatalf("webhook delivery-outcome UPDATE on external_events: %v", err)
	}

	// webhook_registrations + routing_rules — same 007 family, same pool.
	if err := poolDB.Exec(ctx, `DELETE FROM webhook_registrations WHERE name = 'grant-probe-hook'`); err != nil {
		t.Fatalf("webhook_registrations DELETE via pool: %v", err)
	}
	if err := poolDB.Exec(ctx, `
		INSERT INTO webhook_registrations (id, name, source, url_path, secret, event_types, enabled, created_at)
		VALUES ('grant-probe-hook-id', 'grant-probe-hook', 'grant-probe', '/hooks/grant-probe', 'probe-secret', '{}', 1, '2026-10-16T00:00:00Z')
	`); err != nil {
		t.Fatalf("webhook_registrations INSERT via pool: %v", err)
	}
	if _, err := poolDB.Query(ctx,
		`SELECT id, name, source FROM webhook_registrations WHERE name = $1`, "grant-probe-hook"); err != nil {
		t.Fatalf("webhook_registrations SELECT via pool: %v", err)
	}
	if err := poolDB.Exec(ctx,
		`UPDATE webhook_registrations SET enabled = $1 WHERE name = $2`, 0, "grant-probe-hook"); err != nil {
		t.Fatalf("webhook_registrations UPDATE via pool: %v", err)
	}
	if _, err := poolDB.Query(ctx,
		`SELECT id, name, priority, enabled FROM routing_rules ORDER BY priority ASC`); err != nil {
		t.Fatalf("routing_rules SELECT via pool: %v", err)
	}

	// (3) Negative control: this migration must NOT have widened the append-only
	// ledger. UPDATE/DELETE on memory_events stay REVOKEd from agent_role (001).
	for _, verb := range []string{"UPDATE", "DELETE"} {
		if pgHasTablePrivilege(t, adminDB, "memory_events", verb) {
			t.Errorf("agent_role holds %s on memory_events — append-only enforcement broken", verb)
		}
	}
}

// TestGrantMigrationCoversSequenceUsage is the backend-agnostic half of the
// PG-SEQ-42501 regression: it runs without Postgres and asserts that migration
// 030 grants USAGE + SELECT to agent_role on ALL sequences in schema public —
// the catalog-wide shape that keeps future BIGSERIAL-backed migrations from
// silently reintroducing "permission denied for sequence ..." (SQLSTATE 42501)
// on pool INSERTs.
func TestGrantMigrationCoversSequenceUsage(t *testing.T) {
	runner := &Runner{}
	if err := runner.LoadMigrations(); err != nil {
		t.Fatalf("LoadMigrations failed: %v", err)
	}

	var sql string
	for _, m := range runner.migrations {
		if m.Version == seqGrantMigrationVersion {
			sql = m.SQL
			break
		}
	}
	if sql == "" {
		t.Fatalf("migration %d (grant sequence usage to agent_role) is not embedded — "+
			"pool INSERTs drawing from BIGSERIALs keep hitting SQLSTATE 42501", seqGrantMigrationVersion)
	}

	// A single catalog-wide statement, matching 028's shape for tables: grant
	// on ALL sequences in the schema rather than an enumerated list.
	var sawGrant bool
	for _, line := range strings.Split(sql, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "--") {
			continue
		}
		upper := strings.ToUpper(trimmed)
		if strings.HasPrefix(upper, "GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA PUBLIC TO AGENT_ROLE") {
			sawGrant = true
		}
	}
	if !sawGrant {
		t.Fatalf("migration %d does not grant USAGE, SELECT on ALL SEQUENCES IN SCHEMA public "+
			"to agent_role — sequences stay unprivileged and pool INSERTs 42501", seqGrantMigrationVersion)
	}
}

// TestPostgresAgentRolePoolSequencePrivileges is the live half of the
// PG-SEQ-42501 regression: with all migrations applied, EVERY sequence in
// schema public must hold USAGE for agent_role (derived from the catalog, not
// a hardcoded list), and the reported failure — an INSERT INTO memory_events
// through a connection running as agent_role — must succeed.
//
// Gated by CONSENSUS_TEST_POSTGRES_URL. On SQLite (or without the env var) it
// skips — the backend-agnostic structural test above still runs.
func TestPostgresAgentRolePoolSequencePrivileges(t *testing.T) {
	pgURL := os.Getenv("CONSENSUS_TEST_POSTGRES_URL")
	if pgURL == "" {
		t.Skip("CONSENSUS_TEST_POSTGRES_URL not set; skipping live Postgres sequence-grant regression test")
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

	// Migrations run as the table owner (GRANT requires ownership).
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

	// Post-migration pool, so the SET ROLE hook engages (mirrors the table-
	// privilege test above: the pre-migration pool's first connection predates
	// CREATE ROLE agent_role).
	poolDB, err := driver.Open(ctx, db.Config{URL: pgURL})
	if err != nil {
		t.Fatalf("failed to open post-migration pool: %v", err)
	}
	defer func() { _ = poolDB.Close() }()
	if who := pgCurrentUser(t, poolDB); who != "agent_role" {
		t.Fatalf("pool runs as %q, want agent_role — SET ROLE hook did not engage, "+
			"so this test would prove nothing", who)
	}

	// (1) Census: has_sequence_privilege('agent_role', <seq>, 'USAGE') must be
	// true for EVERY sequence in schema public — derived from the catalog.
	rows, err := adminDB.Query(ctx, `
		SELECT sequence_name, has_sequence_privilege('agent_role', sequence_name, 'USAGE') AS ok
		FROM information_schema.sequences
		WHERE sequence_schema = 'public'
		ORDER BY sequence_name ASC
	`)
	if err != nil {
		t.Fatalf("sequence census query: %v", err)
	}
	if len(rows) == 0 {
		t.Fatal("no sequences in schema public — census is hollow")
	}
	unprivileged := 0
	for _, row := range rows {
		name, _ := row["sequence_name"].(string)
		ok, _ := row["ok"].(bool)
		if !ok {
			unprivileged++
			t.Errorf("agent_role lacks USAGE on sequence %s — pool INSERTs drawing from it raise SQLSTATE 42501", name)
		}
	}
	if unprivileged == 0 {
		t.Logf("✓ agent_role holds USAGE on all %d sequences in schema public", len(rows))
	}

	// (2) The reported failure, end to end: INSERT INTO memory_events through
	// the agent_role pool (memory_events has SELECT, INSERT granted in 001; the
	// id BIGSERIAL is the sequence the reported 42501 names). The app's own RLS
	// pattern is a transaction + SetSessionContext (SET LOCAL GUC) — the probe
	// runs inside one and ROLLS BACK, so it leaves zero residue (agent_role has
	// no DELETE on the append-only ledger, by design).
	tx, err := poolDB.BeginTx(ctx)
	if err != nil {
		t.Fatalf("begin probe tx: %v", err)
	}
	if err := tx.SetSessionContext(ctx, probeSeqSessionID); err != nil {
		_ = tx.Rollback()
		t.Fatalf("set RLS session context: %v", err)
	}
	if err := tx.Exec(ctx, `INSERT INTO sessions (id, agent_name, model_id, status)
		VALUES ($1, 'pg-seq-42501-probe', 'deepseek-v4-flash', 'booting')`, probeSeqSessionID); err != nil {
		_ = tx.Rollback()
		t.Fatalf("seed probe session: %v", err)
	}
	var memoryID int64
	if row, err := tx.QueryRow(ctx, `
		INSERT INTO memory_events (type, content, session_id, iteration_created)
		VALUES ('system', 'PG-SEQ-42501 sequence-usage probe', $1, 0)
		RETURNING id
	`, probeSeqSessionID); err != nil {
		_ = tx.Rollback()
		t.Fatalf("INSERT INTO memory_events via agent_role pool (the reported 42501): %v", err)
	} else if v, ok := row["id"].(int64); !ok || v <= 0 {
		_ = tx.Rollback()
		t.Fatalf("INSERT returned id=%v, want int64 > 0", row["id"])
	} else {
		memoryID = v
	}
	if err := tx.Rollback(); err != nil {
		t.Fatalf("rollback probe tx: %v", err)
	}
	t.Logf("✓ INSERT INTO memory_events via agent_role pool succeeded (id=%d, memory_events_id_seq drew correctly, rolled back)", memoryID)
}

// probeSeqSessionID is the isolated RLS session used by
// TestPostgresAgentRolePoolSequencePrivileges. The probe transaction is rolled
// back, so no residue survives either way.
const probeSeqSessionID = "00000000-0000-0000-0000-000000004251"
