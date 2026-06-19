package dynamic_test

import (
	"context"
	"strings"
	"testing"

	"github.com/wojons/consensus/internal/db"
	"github.com/wojons/consensus/internal/db/driver"
	"github.com/wojons/consensus/internal/db/dynamic"
)

// ============================================================================
// Test Harness
// ============================================================================

func openTestDB(t *testing.T) db.DB {
	t.Helper()
	database, err := driver.Open(context.Background(), db.Config{
		URL: "sqlite://:memory:",
	})
	if err != nil {
		t.Fatalf("failed to open test DB: %v", err)
	}
	// Create minimal schema needed for FK references
	if err := database.Exec(context.Background(), `
		CREATE TABLE sessions (
			id TEXT PRIMARY KEY,
			parent_id TEXT,
			agent_name TEXT NOT NULL,
			model_id TEXT NOT NULL,
			status TEXT NOT NULL DEFAULT 'booting',
			goal TEXT,
			context_budget INTEGER NOT NULL DEFAULT 128000,
			tokens_used_in INTEGER NOT NULL DEFAULT 0,
			tokens_used_out INTEGER NOT NULL DEFAULT 0,
			iteration INTEGER NOT NULL DEFAULT 0,
			project_id TEXT,
			heartbeat_at TEXT NOT NULL DEFAULT (datetime('now')),
			planning_max_turns INTEGER NOT NULL DEFAULT 10,
			created_at TEXT NOT NULL DEFAULT (datetime('now')),
			completed_at TEXT
		)
	`); err != nil {
		t.Fatalf("failed to create sessions table: %v", err)
	}
	return database
}

func insertTestSession(t *testing.T, database db.DB) string {
	t.Helper()
	sessionID := "test-session-001"
	err := database.Exec(context.Background(),
		`INSERT INTO sessions (id, agent_name, model_id, status) VALUES ($1, 'test_agent', 'test_model', 'idle')`,
		sessionID,
	)
	if err != nil {
		t.Fatalf("failed to insert test session: %v", err)
	}
	return sessionID
}

// ============================================================================
// Name Sanitization Tests
// ============================================================================

func TestSanitizeName(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"order_tracking", "order_tracking"},
		{"Order Tracking", "order_tracking"},
		{"ORDER_TRACKING", "order_tracking"},
		{"order.tracking", "order_tracking"},
		{"order-tracking", "order_tracking"},
		{"order  tracking", "order_tracking"},
		{"__order__tracking__", "order_tracking"},
		{"123orders", "123orders"},
		{"", ""},
	}

	for _, tt := range tests {
		result := dynamic.SanitizeName(tt.input)
		if result != tt.expected {
			t.Errorf("SanitizeName(%q) = %q, want %q", tt.input, result, tt.expected)
		}
	}
}

// ============================================================================
// Reserved Name Tests
// ============================================================================

func TestIsReservedName(t *testing.T) {
	reserved := []string{
		"sessions", "memory_events", "display_modes", "iteration_commits",
		"memory_pages", "tasks", "tool_requests", "tool_results",
		"tools_registry", "skills_registry", "agent_billing", "workflows",
		"custom_agent_tools", "tool_files", "external_quarantine",
		"compression_queue", "model_registry", "staging_buffer", "audit_logs",
		"system_settings", "agent_messages", "api_keys", "api_rate_limits",
		"external_events", "webhook_registrations", "routing_rules",
		"agent_circuit_breakers", "agent_budget_limits", "secret_access_audit",
		"approval_requests", "schema_versions",
		"dynamic_table_schemas",
	}

	for _, name := range reserved {
		if !dynamic.IsReservedName(name) {
			t.Errorf("IsReservedName(%q) = false, want true", name)
		}
		// Case-insensitive
		if !dynamic.IsReservedName(strings.ToUpper(name)) {
			t.Errorf("IsReservedName(%q) = false (case-insensitive check), want true", strings.ToUpper(name))
		}
	}

	// Non-reserved names
	nonReserved := []string{"order_tracking", "bug_reports", "my_custom_table", "agent_memories"}
	for _, name := range nonReserved {
		if dynamic.IsReservedName(name) {
			t.Errorf("IsReservedName(%q) = true, want false", name)
		}
	}

	// Verify reserved count
	if dynamic.ReservedCount != 33 {
		t.Errorf("ReservedCount = %d, want 33", dynamic.ReservedCount)
	}
}

// ============================================================================
// Name Validation Tests
// ============================================================================

func TestValidateName(t *testing.T) {
	tests := []struct {
		name    string
		wantErr bool
		errMsg  string
	}{
		{"order_tracking", false, ""},
		{"bug_reports", false, ""},
		{"a", false, ""},
		{"_private_table", false, ""},
		{"", true, "empty"},
		{"sessions", true, "reserved"},
		{"SESSIONS", true, "invalid"}, // uppercase fails pattern check before reserved-name check
		{"tool_requests", true, "reserved"},
		{"has.dot", true, "invalid"},
		{"has-dash", true, "invalid"},
		{"has space", true, "invalid"},
		{"UPPERCASE", true, "invalid"},
		{strings.Repeat("a", 64), true, "too long"},
	}

	for _, tt := range tests {
		err := dynamic.ValidateName(tt.name)
		if tt.wantErr {
			if err == nil {
				t.Errorf("ValidateName(%q) = nil, want error containing %q", tt.name, tt.errMsg)
			} else if tt.errMsg != "" && !strings.Contains(strings.ToLower(err.Error()), strings.ToLower(tt.errMsg)) {
				t.Errorf("ValidateName(%q) error = %v, want error containing %q", tt.name, err, tt.errMsg)
			}
		} else {
			if err != nil {
				t.Errorf("ValidateName(%q) = %v, want nil", tt.name, err)
			}
		}
	}
}

// ============================================================================
// CreateTable Tests (SQLite)
// ============================================================================

func TestCreateTable(t *testing.T) {
	database := openTestDB(t)
	defer database.Close()

	sessionID := insertTestSession(t, database)
	ctx := context.Background()

	result, err := dynamic.CreateTable(ctx, database, "order_tracking", sessionID)
	if err != nil {
		t.Fatalf("CreateTable failed: %v", err)
	}

	if result.Name != "order_tracking" {
		t.Errorf("result.Name = %q, want %q", result.Name, "order_tracking")
	}
	if result.Existed {
		t.Error("result.Existed = true, want false (new table)")
	}
	if len(result.Columns) != 8 {
		t.Errorf("len(result.Columns) = %d, want 8", len(result.Columns))
	}

	// Verify the table exists in sqlite_master
	rows, err := database.Query(ctx,
		`SELECT name FROM sqlite_master WHERE type = 'table' AND name = 'order_tracking'`,
	)
	if err != nil {
		t.Fatalf("query sqlite_master failed: %v", err)
	}
	if len(rows) == 0 {
		t.Error("table 'order_tracking' not found in sqlite_master")
	}
}

func TestCreateTableWithSanitization(t *testing.T) {
	database := openTestDB(t)
	defer database.Close()

	sessionID := insertTestSession(t, database)
	ctx := context.Background()

	result, err := dynamic.CreateTable(ctx, database, "Order Tracking System", sessionID)
	if err != nil {
		t.Fatalf("CreateTable with messy input failed: %v", err)
	}

	if result.Name != "order_tracking_system" {
		t.Errorf("result.Name = %q, want %q", result.Name, "order_tracking_system")
	}
}

func TestCreateTableDuplicate(t *testing.T) {
	database := openTestDB(t)
	defer database.Close()

	sessionID := insertTestSession(t, database)
	ctx := context.Background()

	// First creation
	_, err := dynamic.CreateTable(ctx, database, "my_orders", sessionID)
	if err != nil {
		t.Fatalf("first CreateTable failed: %v", err)
	}

	// Second creation should return existed=true, not error
	result, err := dynamic.CreateTable(ctx, database, "my_orders", sessionID)
	if err != nil {
		t.Fatalf("second CreateTable failed: %v", err)
	}

	if !result.Existed {
		t.Error("result.Existed = false, want true (duplicate)")
	}
}

func TestCreateTableReservedName(t *testing.T) {
	database := openTestDB(t)
	defer database.Close()

	sessionID := insertTestSession(t, database)
	ctx := context.Background()

	_, err := dynamic.CreateTable(ctx, database, "sessions", sessionID)
	if err == nil {
		t.Error("CreateTable('sessions') should fail (reserved name)")
	}
	if err != nil && !strings.Contains(strings.ToLower(err.Error()), "reserved") {
		t.Errorf("error = %v, want 'reserved'", err)
	}
}

func TestCreateTableInvalidName(t *testing.T) {
	database := openTestDB(t)
	defer database.Close()

	sessionID := insertTestSession(t, database)
	ctx := context.Background()

	_, err := dynamic.CreateTable(ctx, database, strings.Repeat("a", 64), sessionID)
	if err == nil {
		t.Error("CreateTable with 64-char name should fail (too long)")
	}
}

// ============================================================================
// SoftDelete Tests (SQLite)
// ============================================================================

func TestSoftDelete(t *testing.T) {
	database := openTestDB(t)
	defer database.Close()

	sessionID := insertTestSession(t, database)
	ctx := context.Background()

	// Create a dynamic table
	_, err := dynamic.CreateTable(ctx, database, "my_logs", sessionID)
	if err != nil {
		t.Fatalf("CreateTable failed: %v", err)
	}

	// Insert a row
	id := "row-001"
	err = database.Exec(ctx,
		`INSERT INTO my_logs (id, session_id, data) VALUES ($1, $2, '{"msg":"hello"}')`,
		id, sessionID,
	)
	if err != nil {
		t.Fatalf("INSERT failed: %v", err)
	}

	// Soft-delete it
	result, err := dynamic.SoftDelete(ctx, database, "my_logs", id, sessionID)
	if err != nil {
		t.Fatalf("SoftDelete failed: %v", err)
	}
	if result.RewrittenSQL == "" {
		t.Error("RewrittenSQL is empty")
	}

	// Verify deleted_at is set
	rows, err := database.Query(ctx,
		`SELECT deleted_at FROM my_logs WHERE id = $1`, id,
	)
	if err != nil {
		t.Fatalf("query failed: %v", err)
	}
	if len(rows) == 0 {
		t.Fatal("row not found after soft-delete (should exist)")
	}
	deletedAt, ok := rows[0]["deleted_at"]
	if !ok || deletedAt == nil || deletedAt.(string) == "" {
		t.Error("deleted_at not set after soft-delete")
	}
}

// ============================================================================
// System Column Tests
// ============================================================================

func TestSystemColumnNames(t *testing.T) {
	expected := []string{
		"id", "session_id", "iteration_created", "deleted_at",
		"linked_memory_pages", "data", "created_at", "updated_at",
	}

	if len(dynamic.SystemColumnNames) != len(expected) {
		t.Errorf("len(SystemColumnNames) = %d, want %d", len(dynamic.SystemColumnNames), len(expected))
	}

	for i, col := range expected {
		if dynamic.SystemColumnNames[i] != col {
			t.Errorf("SystemColumnNames[%d] = %q, want %q", i, dynamic.SystemColumnNames[i], col)
		}
	}
}

// ============================================================================
// Full Lifecycle Test
// ============================================================================

func TestFullLifecycle(t *testing.T) {
	database := openTestDB(t)
	defer database.Close()

	sessionID := insertTestSession(t, database)
	ctx := context.Background()

	// 1. Create a table
	result, err := dynamic.CreateTable(ctx, database, "incident_reports", sessionID)
	if err != nil {
		t.Fatalf("step 1 create: %v", err)
	}
	if result.Existed {
		t.Error("step 1: table should be new")
	}

	// 2. Verify table exists
	rows, err := database.Query(ctx,
		`SELECT name FROM sqlite_master WHERE type = 'table' AND name = 'incident_reports'`,
	)
	if err != nil || len(rows) == 0 {
		t.Fatal("step 2: table not found after creation")
	}

	// 3. Insert data
	for i, msg := range []string{"disk full", "timeout", "panic"} {
		id := "inc-" + string(rune('0'+i+1))
		err = database.Exec(ctx,
			`INSERT INTO incident_reports (id, session_id, iteration_created, data)
			 VALUES ($1, $2, $3, $4)`,
			id, sessionID, i+1, `{"message":"`+msg+`"}`,
		)
		if err != nil {
			t.Fatalf("step 3 insert %d: %v", i, err)
		}
	}

	// 4. Read data back
	rows, err = database.Query(ctx,
		`SELECT data, deleted_at FROM incident_reports WHERE session_id = $1 ORDER BY iteration_created`,
		sessionID,
	)
	if err != nil {
		t.Fatalf("step 4 query: %v", err)
	}
	if len(rows) != 3 {
		t.Errorf("step 4: got %d rows, want 3", len(rows))
	}

	// 5. Soft-delete one row
	_, err = dynamic.SoftDelete(ctx, database, "incident_reports", "inc-2", sessionID)
	if err != nil {
		t.Fatalf("step 5 soft-delete: %v", err)
	}

	// 6. Verify soft-deleted row has deleted_at set
	row, err := database.QueryRow(ctx,
		`SELECT deleted_at FROM incident_reports WHERE id = $1`,
		"inc-2",
	)
	if err != nil {
		t.Fatalf("step 6 query: %v", err)
	}
	deletedAt, ok := row["deleted_at"]
	if !ok || deletedAt == nil || deletedAt.(string) == "" {
		t.Error("step 6: deleted_at not set on soft-deleted row")
	}

	// 7. Verify non-deleted row has NULL deleted_at
	row, err = database.QueryRow(ctx,
		`SELECT deleted_at FROM incident_reports WHERE id = $1`,
		"inc-1",
	)
	if err != nil {
		t.Fatalf("step 7 query: %v", err)
	}
	deletedAt, ok = row["deleted_at"]
	if ok && deletedAt != nil && deletedAt.(string) != "" {
		t.Error("step 7: non-deleted row should have NULL deleted_at")
	}

	// 8. Create same table again → existed
	result, err = dynamic.CreateTable(ctx, database, "incident_reports", sessionID)
	if err != nil {
		t.Fatalf("step 8 duplicate create: %v", err)
	}
	if !result.Existed {
		t.Error("step 8: duplicate create should return existed=true")
	}
}

// ============================================================================
// JSON Schema Constraint Tests (SQLite)
// ============================================================================

// axiom:trace work_item=WI-003 spec=specs/003-database.md#4 plan=phase-2/task-1 test=internal/db/dynamic/dynamic_test.go

const schemaOrderTracking = `{
	"type": "object",
	"required": ["item", "sku", "qty"],
	"properties": {
		"item": {"type": "string"},
		"sku":  {"type": "string", "pattern": "^[A-Z]{3}-\\d{4}$"},
		"qty":  {"type": "integer", "minimum": 1}
	},
	"additionalProperties": false
}`

func TestCreateTableWithJSONSchema(t *testing.T) {
	database := openTestDB(t)
	defer database.Close()

	sessionID := insertTestSession(t, database)
	ctx := context.Background()

	result, err := dynamic.CreateTable(ctx, database, "order_tracking", sessionID, schemaOrderTracking)
	if err != nil {
		t.Fatalf("CreateTable with schema failed: %v", err)
	}

	if result.JSONSchema == "" {
		t.Error("expected JSONSchema to be set on result")
	}
	if result.SchemaVersion != 1 {
		t.Errorf("expected SchemaVersion=1, got %d", result.SchemaVersion)
	}

	// Verify the schema is registered (SQLite in-memory registry)
	schema, version, err := dynamic.GetSchemaForTable(ctx, database, "order_tracking")
	if err != nil {
		t.Fatalf("GetSchemaForTable failed: %v", err)
	}
	if version != 1 {
		t.Errorf("expected version 1, got %d", version)
	}
	if schema == "" {
		t.Error("expected non-empty schema")
	}

	hasConstraint, err := dynamic.HasSchemaConstraint(ctx, database, "order_tracking")
	if err != nil {
		t.Fatalf("HasSchemaConstraint failed: %v", err)
	}
	if !hasConstraint {
		t.Error("expected HasSchemaConstraint to be true")
	}
}

func TestCreateTableWithInvalidSchema(t *testing.T) {
	database := openTestDB(t)
	defer database.Close()

	sessionID := insertTestSession(t, database)
	ctx := context.Background()

	// Invalid JSON schema (missing closing brace)
	_, err := dynamic.CreateTable(ctx, database, "bad_schema_table", sessionID, `{"type": "object"`)
	if err == nil {
		t.Error("expected error for invalid JSON Schema")
	}

	// Empty schema string should be accepted (treated as no constraint)
	result, err := dynamic.CreateTable(ctx, database, "no_schema_table", sessionID, "")
	if err != nil {
		t.Errorf("empty schema should be accepted: %v", err)
	}
	if result.JSONSchema != "" {
		t.Error("expected empty JSONSchema for empty schema arg")
	}
}

func TestValidateData(t *testing.T) {
	database := openTestDB(t)
	defer database.Close()

	sessionID := insertTestSession(t, database)
	ctx := context.Background()

	// Create table with schema
	_, err := dynamic.CreateTable(ctx, database, "validated_orders", sessionID, schemaOrderTracking)
	if err != nil {
		t.Fatalf("CreateTable failed: %v", err)
	}

	// Valid data should pass
	if err := dynamic.ValidateData(ctx, database, "validated_orders", `{"item": "Widget", "sku": "ABC-1234", "qty": 5}`); err != nil {
		t.Errorf("valid data should pass: %v", err)
	}

	// Invalid data: missing required field
	if err := dynamic.ValidateData(ctx, database, "validated_orders", `{"item": "Widget"}`); err == nil {
		t.Error("expected error for missing required field")
	}

	// Invalid data: wrong type
	if err := dynamic.ValidateData(ctx, database, "validated_orders", `{"item": "Widget", "sku": "ABC-1234", "qty": "five"}`); err == nil {
		t.Error("expected error for wrong type")
	}

	// Invalid data: value below minimum
	if err := dynamic.ValidateData(ctx, database, "validated_orders", `{"item": "Widget", "sku": "ABC-1234", "qty": 0}`); err == nil {
		t.Error("expected error for qty < minimum")
	}

	// Invalid data: bad pattern
	if err := dynamic.ValidateData(ctx, database, "validated_orders", `{"item": "Widget", "sku": "invalid", "qty": 1}`); err == nil {
		t.Error("expected error for invalid SKU pattern")
	}

	// Invalid data: additional property
	if err := dynamic.ValidateData(ctx, database, "validated_orders", `{"item": "Widget", "sku": "ABC-1234", "qty": 1, "extra": true}`); err == nil {
		t.Error("expected error for additionalProperties: false")
	}
}

func TestValidateDataNoConstraint(t *testing.T) {
	database := openTestDB(t)
	defer database.Close()

	sessionID := insertTestSession(t, database)
	ctx := context.Background()

	// Create table WITHOUT schema
	_, err := dynamic.CreateTable(ctx, database, "unvalidated_data", sessionID)
	if err != nil {
		t.Fatalf("CreateTable failed: %v", err)
	}

	// Any data should pass
	if err := dynamic.ValidateData(ctx, database, "unvalidated_data", `{"anything": "goes"}`); err != nil {
		t.Errorf("no constraint should accept any data: %v", err)
	}

	hasConstraint, err := dynamic.HasSchemaConstraint(ctx, database, "unvalidated_data")
	if err != nil {
		t.Fatalf("HasSchemaConstraint failed: %v", err)
	}
	if hasConstraint {
		t.Error("expected HasSchemaConstraint to be false for table without schema")
	}
}

func TestAddJSONConstraintAfterCreate(t *testing.T) {
	database := openTestDB(t)
	defer database.Close()

	sessionID := insertTestSession(t, database)
	ctx := context.Background()

	// Create table without schema
	_, err := dynamic.CreateTable(ctx, database, "late_schema", sessionID)
	if err != nil {
		t.Fatalf("CreateTable failed: %v", err)
	}

	// Data should be accepted initially
	if err := dynamic.ValidateData(ctx, database, "late_schema", `{"unvalidated": true}`); err != nil {
		t.Errorf("expected no constraint initially: %v", err)
	}

	// Add constraint
	result, err := dynamic.AddJSONConstraint(ctx, database, "late_schema", schemaOrderTracking, sessionID)
	if err != nil {
		t.Fatalf("AddJSONConstraint failed: %v", err)
	}
	if result.SchemaVersion != 1 {
		t.Errorf("expected SchemaVersion=1, got %d", result.SchemaVersion)
	}

	// Now data must conform
	if err := dynamic.ValidateData(ctx, database, "late_schema", `{"item": "Widget", "sku": "ABC-1234", "qty": 5}`); err != nil {
		t.Errorf("valid data should pass after constraint added: %v", err)
	}
	if err := dynamic.ValidateData(ctx, database, "late_schema", `{"unvalidated": true}`); err == nil {
		t.Error("expected error for invalid data after constraint added")
	}
}

func TestAlterJSONConstraintVersioning(t *testing.T) {
	database := openTestDB(t)
	defer database.Close()

	sessionID := insertTestSession(t, database)
	ctx := context.Background()

	// Create table with initial schema
	_, err := dynamic.CreateTable(ctx, database, "versioned_schema", sessionID, schemaOrderTracking)
	if err != nil {
		t.Fatalf("CreateTable failed: %v", err)
	}

	// Get version
	_, version, err := dynamic.GetSchemaForTable(ctx, database, "versioned_schema")
	if err != nil {
		t.Fatalf("GetSchemaForTable failed: %v", err)
	}
	if version != 1 {
		t.Errorf("expected version 1, got %d", version)
	}

	// Alter schema: relaxed schema (no pattern on SKU)
	relaxedSchema := `{
		"type": "object",
		"required": ["item"],
		"properties": {
			"item": {"type": "string"},
			"notes": {"type": "string"}
		}
	}`

	result, err := dynamic.AlterJSONConstraint(ctx, database, "versioned_schema", relaxedSchema, sessionID)
	if err != nil {
		t.Fatalf("AlterJSONConstraint failed: %v", err)
	}
	if result.SchemaVersion != 2 {
		t.Errorf("expected SchemaVersion=2, got %d", result.SchemaVersion)
	}
	if result.OldSchemaVersion != 1 {
		t.Errorf("expected OldSchemaVersion=1, got %d", result.OldSchemaVersion)
	}

	// Verify new version is active
	schema, newVersion, err := dynamic.GetSchemaForTable(ctx, database, "versioned_schema")
	if err != nil {
		t.Fatalf("GetSchemaForTable failed: %v", err)
	}
	if newVersion != 2 {
		t.Errorf("expected version 2, got %d", newVersion)
	}
	if schema != relaxedSchema {
		t.Error("expected relaxed schema to be active")
	}

	// Old schema data should now pass (relaxed)
	if err := dynamic.ValidateData(ctx, database, "versioned_schema", `{"item": "Widget", "notes": "relaxed"}`); err != nil {
		t.Errorf("valid data under relaxed schema should pass: %v", err)
	}

	// But old strict data still works
	if err := dynamic.ValidateData(ctx, database, "versioned_schema", `{"item": "Widget"}`); err != nil {
		t.Errorf("basic valid data should pass: %v", err)
	}
}

func TestRemoveJSONConstraint(t *testing.T) {
	database := openTestDB(t)
	defer database.Close()

	sessionID := insertTestSession(t, database)
	ctx := context.Background()

	// Create table with schema
	_, err := dynamic.CreateTable(ctx, database, "removable_schema", sessionID, schemaOrderTracking)
	if err != nil {
		t.Fatalf("CreateTable failed: %v", err)
	}

	// Remove constraint
	result, err := dynamic.RemoveJSONConstraint(ctx, database, "removable_schema")
	if err != nil {
		t.Fatalf("RemoveJSONConstraint failed: %v", err)
	}
	if result.SchemaVersion != 0 {
		t.Errorf("expected SchemaVersion=0 (removed), got %d", result.SchemaVersion)
	}

	// Now any data should pass
	if err := dynamic.ValidateData(ctx, database, "removable_schema", `{"anything": "goes"}`); err != nil {
		t.Errorf("data should pass after constraint removed: %v", err)
	}

	// Verify no constraint
	hasConstraint, err := dynamic.HasSchemaConstraint(ctx, database, "removable_schema")
	if err != nil {
		t.Fatalf("HasSchemaConstraint failed: %v", err)
	}
	if hasConstraint {
		t.Error("expected no constraint after removal")
	}
}

func TestValidateDynamicInsert(t *testing.T) {
	database := openTestDB(t)
	defer database.Close()

	sessionID := insertTestSession(t, database)
	ctx := context.Background()

	// Create table with schema
	_, err := dynamic.CreateTable(ctx, database, "insert_validation", sessionID, schemaOrderTracking)
	if err != nil {
		t.Fatalf("CreateTable failed: %v", err)
	}

	// ValidateDynamicInsert should work
	if err := dynamic.ValidateDynamicInsert(ctx, database, "insert_validation", `{"item": "Valid", "sku": "DEF-5678", "qty": 3}`); err != nil {
		t.Errorf("valid insert data should pass: %v", err)
	}

	if err := dynamic.ValidateDynamicInsert(ctx, database, "insert_validation", `{"bad": "data"}`); err == nil {
		t.Error("expected error for invalid insert data")
	}
}

func TestDynamicInsertWithSchemaEndToEnd(t *testing.T) {
	database := openTestDB(t)
	defer database.Close()

	sessionID := insertTestSession(t, database)
	ctx := context.Background()

	// Create table with schema
	_, err := dynamic.CreateTable(ctx, database, "e2e_validation", sessionID, schemaOrderTracking)
	if err != nil {
		t.Fatalf("CreateTable failed: %v", err)
	}

	// Insert valid data — should work
	err = database.Exec(ctx,
		`INSERT INTO e2e_validation (id, session_id, data) VALUES ($1, $2, $3)`,
		"row-001", sessionID, `{"item": "Widget", "sku": "ABC-1234", "qty": 2}`,
	)
	if err != nil {
		t.Fatalf("insert valid data failed: %v", err)
	}

	// Verify data was inserted
	rows, err := database.Query(ctx, `SELECT data FROM e2e_validation WHERE id = $1`, "row-001")
	if err != nil {
		t.Fatalf("query failed: %v", err)
	}
	if len(rows) != 1 {
		t.Fatal("expected 1 row")
	}
	data := dynamic.ParseDataColumn(rows[0])
	if !strings.Contains(data, "Widget") {
		t.Errorf("expected data to contain 'Widget', got: %s", data)
	}
}

func TestSchemaConcurrentAccess(t *testing.T) {
	// Test that the SQLite in-memory schema registry handles concurrent access
	database := openTestDB(t)
	defer database.Close()

	sessionID := insertTestSession(t, database)
	ctx := context.Background()

	_, err := dynamic.CreateTable(ctx, database, "concurrent_tbl", sessionID, schemaOrderTracking)
	if err != nil {
		t.Fatalf("CreateTable failed: %v", err)
	}

	// Concurrent reads/writes
	done := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		go func() {
			_, _, err := dynamic.GetSchemaForTable(ctx, database, "concurrent_tbl")
			if err != nil {
				t.Logf("concurrent GetSchemaForTable failed: %v", err)
			}
			has, err := dynamic.HasSchemaConstraint(ctx, database, "concurrent_tbl")
			if err != nil {
				t.Logf("concurrent HasSchemaConstraint failed: %v", err)
			}
			_ = has
			done <- true
		}()
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}
}

// ============================================================================
// Backend Detection Tests
// ============================================================================

func TestIsExecutableOnBackend(t *testing.T) {
	database := openTestDB(t)
	defer database.Close()

	// On SQLite, Postgres-specific functions are NOT executable
	if dynamic.IsExecutableOnBackend(database, "create_agent_memory_table") {
		t.Error("create_agent_memory_table should not be executable on SQLite")
	}
	if dynamic.IsExecutableOnBackend(database, "soft_delete_intercept") {
		t.Error("soft_delete_intercept should not be executable on SQLite")
	}
	if dynamic.IsExecutableOnBackend(database, "verify_dynamic_table") {
		t.Error("verify_dynamic_table should not be executable on SQLite")
	}
}
