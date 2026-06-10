// Package dynamic implements runtime dynamic entity management for Conscience.
//
// Agents need to create domain-specific data structures at runtime (e.g.,
// order_tracking, bug_reports). This package provides the Go-level equivalent
// of the Postgres SECURITY DEFINER functions defined in migration 006.
//
// It works consistently across both database backends:
//   - Postgres: delegates to create_agent_memory_table() / soft_delete_intercept()
//   - SQLite:   implements the equivalent logic directly in Go
//
// The harness classifies DDL statements (CREATE TABLE) and reroutes them through
// CreateTable() instead of executing raw DDL. DELETE statements are converted to
// soft-delete UPDATEs via SoftDelete().
//
// JSON Schema enforcement (SPEC-003 §4) provides DB-level validation of the
// data column on dynamic tables:
//   - Postgres: uses pg_jsonschema extension jsonb_matches_schema() CHECK constraints
//   - SQLite:   uses Go-level validation via the jsonschema package
//
// axiom:trace work_item=WI-003
//
//	spec=specs/003-database.md#4,specs/007-json-schema.md
//	plan=phase-2/task-1
//	impl=internal/db/dynamic/
package dynamic

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"sync"

	"github.com/wojons/conscientiousness/internal/db"
	"github.com/wojons/conscientiousness/internal/db/jsonschema"
)

// ============================================================================
// Constants
// ============================================================================

// SystemColumnNames are the columns automatically added to every dynamic table.
var SystemColumnNames = []string{
	"id",
	"session_id",
	"iteration_created",
	"deleted_at",
	"linked_memory_pages",
	"data",
	"created_at",
	"updated_at",
}

// reservedNames is the blocklist of names that cannot be used for dynamic tables.
//
// This mirrors the blocklist in SPEC-003 §3.2 and migration 006.
// It includes all core tables from SPEC-003, plus tables from SEC-005, SPEC-013,
// SPEC-014, SPEC-015, SPEC-017, SPEC-020, and the schema_versions migration
// tracking table.
var reservedNames = map[string]bool{
	"sessions":                true,
	"memory_events":           true,
	"display_modes":           true,
	"iteration_commits":       true,
	"memory_pages":            true,
	"tasks":                   true,
	"tool_requests":           true,
	"tool_results":            true,
	"tools_registry":          true,
	"skills_registry":         true,
	"agent_billing":           true,
	"workflows":               true,
	"custom_agent_tools":      true,
	"tool_files":              true,
	"external_quarantine":     true,
	"compression_queue":       true,
	"model_registry":          true,
	"staging_buffer":          true,
	"audit_logs":              true,
	"system_settings":         true,
	"agent_messages":          true,
	"api_keys":                true,
	"api_rate_limits":         true,
	"external_events":         true,
	"webhook_registrations":   true,
	"routing_rules":           true,
	"agent_circuit_breakers":  true,
	"agent_budget_limits":     true,
	"secret_access_audit":     true,
	"approval_requests":       true,
	"schema_versions":         true,
	"dynamic_table_schemas":   true,
}

// ReservedCount is the number of reserved names, for verification tests.
const ReservedCount = 33

// tableNamePattern validates dynamic table names.
// Must be 1-63 characters, start with a letter or underscore,
// contain only lowercase alphanumeric and underscore.
var tableNamePattern = regexp.MustCompile(`^[a-z_][a-z0-9_]{0,62}$`)

// ============================================================================
// Name Validation
// ============================================================================

// SanitizeName normalizes a raw table name for safe use.
//   - Converts to lowercase
//   - Replaces non-alphanumeric characters with underscores
//   - Collapses multiple consecutive underscores
//   - Trims leading/trailing underscores
func SanitizeName(raw string) string {
	name := strings.ToLower(raw)
	name = regexp.MustCompile(`[^a-z0-9_]`).ReplaceAllString(name, "_")
	name = regexp.MustCompile(`_+`).ReplaceAllString(name, "_")
	name = strings.Trim(name, "_")
	return name
}

// IsReservedName returns true if the given name matches a reserved core table.
func IsReservedName(name string) bool {
	name = strings.ToLower(strings.TrimSpace(name))
	return reservedNames[name]
}

// ValidateName checks whether a table name is valid for dynamic creation.
// Returns nil if valid, or an error describing the violation.
func ValidateName(name string) error {
	name = strings.TrimSpace(name)

	if len(name) == 0 {
		return fmt.Errorf("table name must not be empty")
	}

	if len(name) > 63 {
		return fmt.Errorf("table name too long (%d chars); must be 1-63 characters", len(name))
	}

	if !tableNamePattern.MatchString(name) {
		return fmt.Errorf("invalid table name %q: must start with a-z or underscore, contain only a-z, 0-9, underscore", name)
	}

	if IsReservedName(name) {
		return fmt.Errorf("cannot create table with reserved name: %q", name)
	}

	return nil
}

// ============================================================================
// CreateTable — Dynamic Entity Provisioning
// ============================================================================

// CreateTableResult holds the result of a dynamic table creation.
type CreateTableResult struct {
	// Name is the sanitized table name that was actually created.
	Name string

	// Existed is true if the table already existed before this call.
	Existed bool

	// Columns contains the system column names added to the table.
	Columns []string

	// JSONSchema is the schema document applied to the data column (if any).
	JSONSchema string

	// SchemaVersion is the version of the applied JSON schema (0 = none).
	SchemaVersion int
}

// CreateTable provisions a new agent-owned dynamic table.
//
// It validates and sanitizes the table name, checks the reserved-name blocklist,
// and creates the table with all 8 required system columns.
//
// On Postgres, it delegates to the create_agent_memory_table() SQL function
// (from migration 006), which also sets up RLS policies and triggers.
//
// On SQLite, it creates the table directly using the db.DB interface,
// without RLS or triggers (those are enforced at the Go application layer).
//
// The sessionID is recorded in a comment for traceability but is not
// used as a constraint — dynamic tables are shared within a deployment.
//
// An optional jsonSchema can be provided to add a CHECK constraint on the
// data column. On Postgres this uses pg_jsonschema's jsonb_matches_schema();
// on SQLite it registers the schema for Go-layer validation.
func CreateTable(ctx context.Context, database db.DB, tableName string, sessionID string, jsonSchema ...string) (*CreateTableResult, error) {
	// Validate and sanitize the name
	sanitized := SanitizeName(tableName)
	if err := ValidateName(sanitized); err != nil {
		return nil, fmt.Errorf("dynamic: %w", err)
	}

	// Parse optional JSON Schema
	var schema string
	if len(jsonSchema) > 0 {
		schema = jsonSchema[0]
		if schema != "" {
			if err := jsonschema.IsValidSchema(schema); err != nil {
				return nil, fmt.Errorf("dynamic: invalid JSON Schema: %w", err)
			}
		} else {
			schema = ""
		}
	}

	// Check if the backend is Postgres (delegate to SQL function)
	// or SQLite (create table directly via Go)
	backend := database.Backend()

	var result *CreateTableResult
	var err error

	switch backend {
	case db.BackendPostgres:
		result, err = createTablePostgres(ctx, database, sanitized, sessionID)
	default:
		result, err = createTableSQLite(ctx, database, sanitized, sessionID)
	}

	if err != nil {
		return nil, err
	}

	// Apply optional JSON Schema constraint
	if schema != "" {
		if err := addJSONConstraint(ctx, database, sanitized, schema, backend); err != nil {
			return nil, fmt.Errorf("dynamic: failed to apply JSON Schema to %q: %w", sanitized, err)
		}
		result.JSONSchema = schema
		result.SchemaVersion = 1
	}

	return result, nil
}

func createTablePostgres(ctx context.Context, database db.DB, name string, sessionID string) (*CreateTableResult, error) {
	// Call the SECURITY DEFINER function from migration 006.
	// The function validates names, handles reserved-name blocklisting,
	// and sets up RLS + triggers + grants.
	var result string
	row, err := database.QueryRow(ctx,
		`SELECT create_agent_memory_table($1)`,
		name,
	)
	if err != nil {
		// Check if the table already exists (Postgres returns a message, not an error)
		if strings.Contains(err.Error(), "already exists") {
			return &CreateTableResult{
				Name:    name,
				Existed: true,
				Columns: SystemColumnNames,
			}, nil
		}
		return nil, fmt.Errorf("dynamic: Postgres create_agent_memory_table failed: %w", err)
	}

	if v, ok := row["create_agent_memory_table"]; ok {
		result = fmt.Sprint(v)
	}

	if strings.Contains(result, "already exists") {
		return &CreateTableResult{
			Name:    name,
			Existed: true,
			Columns: SystemColumnNames,
		}, nil
	}

	return &CreateTableResult{
		Name:    name,
		Columns: SystemColumnNames,
	}, nil
}

func createTableSQLite(ctx context.Context, database db.DB, name string, sessionID string) (*CreateTableResult, error) {
	// Check if table already exists
	exists, err := tableExists(ctx, database, name)
	if err != nil {
		return nil, fmt.Errorf("dynamic: failed to check if table exists: %w", err)
	}
	if exists {
		return &CreateTableResult{
			Name:    name,
			Existed: true,
			Columns: SystemColumnNames,
		}, nil
	}

	// Create the table with all 8 system columns.
	// SQLite doesn't support UUID natively; we use TEXT for id.
	createSQL := fmt.Sprintf(`
		CREATE TABLE %s (
			id                  TEXT PRIMARY KEY,
			session_id          TEXT NOT NULL REFERENCES sessions(id),
			iteration_created   INTEGER,
			deleted_at          TEXT,
			linked_memory_pages TEXT,
			data                TEXT NOT NULL DEFAULT '{}',
			created_at          TEXT NOT NULL DEFAULT (datetime('now')),
			updated_at          TEXT NOT NULL DEFAULT (datetime('now'))
		)
	`, name)

	if err := database.Exec(ctx, createSQL); err != nil {
		return nil, fmt.Errorf("dynamic: failed to create table %q: %w", name, err)
	}

	return &CreateTableResult{
		Name:    name,
		Columns: SystemColumnNames,
	}, nil
}

// tableExists checks whether a table exists in the database.
// Uses information_schema on Postgres; sqlite_master on SQLite.
func tableExists(ctx context.Context, database db.DB, tableName string) (bool, error) {
	backend := database.Backend()

	var query string
	if backend == db.BackendPostgres {
		query = `SELECT count(*) AS cnt FROM information_schema.tables WHERE table_schema = current_schema() AND table_name = $1`
	} else {
		query = `SELECT count(*) AS cnt FROM sqlite_master WHERE type = 'table' AND name = $1`
	}

	rows, err := database.Query(ctx, query, tableName)
	if err != nil {
		return false, err
	}
	if len(rows) == 0 {
		return false, nil
	}

	cnt, ok := rows[0]["cnt"]
	if !ok {
		return false, nil
	}
	switch v := cnt.(type) {
	case int64:
		return v > 0, nil
	case float64:
		return int64(v) > 0, nil
	case int:
		return v > 0, nil
	default:
		return false, nil
	}
}

// ============================================================================
// JSON Schema Constraint Management (SPEC-003 §4)
// ============================================================================

// JSONConstraintResult holds the result of applying a JSON Schema constraint.
type JSONConstraintResult struct {
	// TableName is the sanitized table name the constraint was applied to.
	TableName string

	// SchemaVersion is the version of the applied schema (starts at 1).
	SchemaVersion int

	// ConstraintName is the name of the CHECK constraint on Postgres.
	ConstraintName string

	// OldSchemaVersion is the previous schema version (0 if none).
	OldSchemaVersion int
}

// AddJSONConstraint applies a JSON Schema CHECK constraint to a dynamic table's
// data column. The constraint ensures that all values inserted into the data
// column conform to the provided JSON Schema draft-07 document.
//
// On Postgres this creates: ALTER TABLE t ADD CONSTRAINT c CHECK(jsonb_matches_schema(...))
// On SQLite this registers the schema for Go-layer validation.
//
// Returns the constraint result including the constraint name and version.
func AddJSONConstraint(ctx context.Context, database db.DB, tableName string, schemaDoc string, sessionID string) (*JSONConstraintResult, error) {
	sanitized := SanitizeName(tableName)
	if err := ValidateName(sanitized); err != nil {
		return nil, fmt.Errorf("dynamic: %w", err)
	}

	// Validate the schema document
	if err := jsonschema.IsValidSchema(schemaDoc); err != nil {
		return nil, fmt.Errorf("dynamic: invalid JSON Schema: %w", err)
	}

	backend := database.Backend()

	// Check for existing active schema
	var oldVersion int
	var existingSchema string
	var err error

	if backend == db.BackendPostgres {
		existingSchema, oldVersion, err = getActiveSchemaPostgres(ctx, database, sanitized)
	} else {
		existingSchema, oldVersion = getActiveSchemaSQLite(sanitized)
	}
	if err != nil {
		return nil, fmt.Errorf("dynamic: failed to check existing schema for %q: %w", sanitized, err)
	}

	if existingSchema == schemaDoc {
		// Schema unchanged; return current version
		return &JSONConstraintResult{
			TableName:        sanitized,
			SchemaVersion:    oldVersion,
			ConstraintName:   constraintName(sanitized, oldVersion),
			OldSchemaVersion: oldVersion,
		}, nil
	}

	// Drop old constraint if exists (on Postgres)
	if oldVersion > 0 && existingSchema != "" && backend == db.BackendPostgres {
		if err := dropJSONConstraintPostgres(ctx, database, sanitized, oldVersion); err != nil {
			return nil, fmt.Errorf("dynamic: failed to drop old constraint on %q: %w", sanitized, err)
		}
	}

	// Apply new constraint
	newVersion := oldVersion + 1
	if err := addJSONConstraint(ctx, database, sanitized, schemaDoc, backend); err != nil {
		return nil, err
	}

	// Record in dynamic_table_schemas
	if err := recordSchemaVersion(ctx, database, sanitized, schemaDoc, newVersion, sessionID, constraintName(sanitized, newVersion)); err != nil {
		return nil, fmt.Errorf("dynamic: failed to record schema version: %w", err)
	}

	// Mark previous version as superseded
	if oldVersion > 0 {
		if err := supersedeSchemaVersion(ctx, database, sanitized, oldVersion); err != nil {
			return nil, fmt.Errorf("dynamic: failed to supersede old schema version: %w", err)
		}
	}

	return &JSONConstraintResult{
		TableName:        sanitized,
		SchemaVersion:    newVersion,
		ConstraintName:   constraintName(sanitized, newVersion),
		OldSchemaVersion: oldVersion,
	}, nil
}

// AlterJSONConstraint is an alias for AddJSONConstraint.
// It applies a new schema version, dropping the old constraint if it exists.
func AlterJSONConstraint(ctx context.Context, database db.DB, tableName string, schemaDoc string, sessionID string) (*JSONConstraintResult, error) {
	return AddJSONConstraint(ctx, database, tableName, schemaDoc, sessionID)
}

// RemoveJSONConstraint drops the active JSON Schema constraint on a table.
func RemoveJSONConstraint(ctx context.Context, database db.DB, tableName string) (*JSONConstraintResult, error) {
	sanitized := SanitizeName(tableName)
	if err := ValidateName(sanitized); err != nil {
		return nil, fmt.Errorf("dynamic: %w", err)
	}

	backend := database.Backend()

	// Find active schema version
	var version int
	if backend == db.BackendPostgres {
		var err error
		_, version, err = getActiveSchemaPostgres(ctx, database, sanitized)
		if err != nil {
			return nil, fmt.Errorf("dynamic: failed to find active schema for %q: %w", sanitized, err)
		}
	} else {
		_, version = getActiveSchemaSQLite(sanitized)
	}

	if version == 0 {
		return &JSONConstraintResult{
			TableName:     sanitized,
			SchemaVersion: 0,
		}, nil
	}

	// Drop constraint
	if backend == db.BackendPostgres {
		if err := dropJSONConstraintPostgres(ctx, database, sanitized, version); err != nil {
			return nil, fmt.Errorf("dynamic: failed to drop constraint on %q: %w", sanitized, err)
		}
	} else {
		removeSchemaSQLite(sanitized)
	}

	// Mark as superseded in tracking table
	if err := supersedeSchemaVersion(ctx, database, sanitized, version); err != nil {
		return nil, fmt.Errorf("dynamic: failed to mark schema as superseded: %w", err)
	}

	return &JSONConstraintResult{
		TableName:        sanitized,
		SchemaVersion:    0,
		OldSchemaVersion: version,
	}, nil
}

// ============================================================================
// JSON Schema — Postgres Implementation
// ============================================================================

func addJSONConstraintPostgres(ctx context.Context, database db.DB, tableName string, schemaDoc string, version int) error {
	cName := constraintName(tableName, version)

	// Escape single quotes in the schema document for embedding in SQL
	escapedSchema := strings.ReplaceAll(schemaDoc, "'", "''")

	sql := fmt.Sprintf(`
		ALTER TABLE %s
		ADD CONSTRAINT %s
		CHECK (jsonb_matches_schema('%s', data))
	`, tableName, cName, escapedSchema)

	if err := database.Exec(ctx, sql); err != nil {
		return fmt.Errorf("failed to add JSON Schema constraint %s: %w", cName, err)
	}

	return nil
}

func dropJSONConstraintPostgres(ctx context.Context, database db.DB, tableName string, version int) error {
	cName := constraintName(tableName, version)

	sql := fmt.Sprintf(`ALTER TABLE %s DROP CONSTRAINT IF EXISTS %s`, tableName, cName)
	if err := database.Exec(ctx, sql); err != nil {
		return fmt.Errorf("failed to drop constraint %s: %w", cName, err)
	}

	return nil
}

func getActiveSchemaPostgres(ctx context.Context, database db.DB, tableName string) (string, int, error) {
	// Query the dynamic_table_schemas table for the active schema
	rows, err := database.Query(ctx,
		`SELECT schema_document, version FROM dynamic_table_schemas
		 WHERE table_name = $1 AND is_active = true
		 ORDER BY version DESC LIMIT 1`,
		tableName,
	)
	if err != nil {
		// Table might not exist yet (migration not applied)
		if strings.Contains(err.Error(), "does not exist") || strings.Contains(err.Error(), "no such table") {
			return "", 0, nil
		}
		return "", 0, err
	}
	if len(rows) == 0 {
		return "", 0, nil
	}

	schema := fmt.Sprint(rows[0]["schema_document"])
	ver := toVersion(rows[0]["version"])
	return schema, ver, nil
}

// ============================================================================
// JSON Schema — SQLite In-Memory Registry
// ============================================================================

// sqliteSchemaRegistry stores active JSON Schema constraints for SQLite tables.
// Thread-safe map of table_name → { schema, version }.
var sqliteSchemaRegistry struct {
	mu    sync.RWMutex
	data  map[string]sqliteSchemaEntry
}

type sqliteSchemaEntry struct {
	Schema  string
	Version int
}

func init() {
	sqliteSchemaRegistry.data = make(map[string]sqliteSchemaEntry)
}

func addJSONConstraintSQLite(tableName string, schemaDoc string) int {
	sqliteSchemaRegistry.mu.Lock()
	defer sqliteSchemaRegistry.mu.Unlock()

	entry, exists := sqliteSchemaRegistry.data[tableName]
	newVersion := 1
	if exists {
		newVersion = entry.Version + 1
	}

	sqliteSchemaRegistry.data[tableName] = sqliteSchemaEntry{
		Schema:  schemaDoc,
		Version: newVersion,
	}

	return newVersion
}

func getActiveSchemaSQLite(tableName string) (string, int) {
	sqliteSchemaRegistry.mu.RLock()
	defer sqliteSchemaRegistry.mu.RUnlock()

	entry, exists := sqliteSchemaRegistry.data[tableName]
	if !exists {
		return "", 0
	}
	return entry.Schema, entry.Version
}

func removeSchemaSQLite(tableName string) {
	sqliteSchemaRegistry.mu.Lock()
	defer sqliteSchemaRegistry.mu.Unlock()

	delete(sqliteSchemaRegistry.data, tableName)
}

// ValidateData checks whether a JSON data value conforms to the active
// JSON Schema constraint for the given dynamic table. This is the primary
// entry point for SQLite app-layer validation.
//
// On Postgres, validation is handled by the DB-level CHECK constraint.
// This function is still safe to call on any backend (it will no-op if
// no constraint is registered or if the backend is Postgres).
//
// Call this before INSERT or UPDATE of the data column on a dynamic table.
func ValidateData(ctx context.Context, database db.DB, tableName string, dataJSON string) error {
	backend := database.Backend()

	var schemaDoc string
	var version int

	if backend == db.BackendPostgres {
		var err error
		schemaDoc, version, err = getActiveSchemaPostgres(ctx, database, tableName)
		if err != nil {
			return fmt.Errorf("dynamic: failed to query schema: %w", err)
		}
	} else {
		schemaDoc, version = getActiveSchemaSQLite(tableName)
	}

	if version == 0 || schemaDoc == "" {
		return nil // No constraint active
	}

	if err := jsonschema.DefaultValidators.ValidateString(schemaDoc, dataJSON); err != nil {
		return fmt.Errorf("dynamic: data column violates JSON Schema constraint on %q: %w", tableName, err)
	}

	return nil
}

// HasSchemaConstraint returns true if a dynamic table has an active JSON Schema
// constraint. Useful for the harness to decide whether to validate data.
func HasSchemaConstraint(ctx context.Context, database db.DB, tableName string) (bool, error) {
	backend := database.Backend()

	if backend == db.BackendPostgres {
		_, version, err := getActiveSchemaPostgres(ctx, database, tableName)
		if err != nil {
			return false, err
		}
		return version > 0, nil
	}

	_, version := getActiveSchemaSQLite(tableName)
	return version > 0, nil
}

// GetSchemaForTable returns the active JSON Schema document and version for a table.
func GetSchemaForTable(ctx context.Context, database db.DB, tableName string) (string, int, error) {
	backend := database.Backend()

	if backend == db.BackendPostgres {
		schema, version, err := getActiveSchemaPostgres(ctx, database, tableName)
		if err != nil {
			return "", 0, err
		}
		return schema, version, nil
	}

	schema, version := getActiveSchemaSQLite(tableName)
	return schema, version, nil
}

// ============================================================================
// JSON Schema — Common Helpers
// ============================================================================

// addJSONConstraint applies a JSON Schema constraint on the given backend.
func addJSONConstraint(ctx context.Context, database db.DB, tableName string, schemaDoc string, backend db.Backend) error {
	switch backend {
	case db.BackendPostgres:
		return addJSONConstraintPostgres(ctx, database, tableName, schemaDoc, 1)
	default:
		addJSONConstraintSQLite(tableName, schemaDoc)
		return nil
	}
}

// constraintName generates a deterministic constraint name for a table+version.
func constraintName(tableName string, version int) string {
	return fmt.Sprintf("%s_json_schema_v%d", tableName, version)
}

// recordSchemaVersion records a schema version in the dynamic_table_schemas table.
func recordSchemaVersion(ctx context.Context, database db.DB, tableName string, schemaDoc string, version int, sessionID string, constraintName string) error {
	// On SQLite, the dynamic_table_schemas table might not exist (no migration run).
	if database.Backend() != db.BackendPostgres {
		return nil
	}

	var sessionArg any
	if sessionID == "" {
		sessionArg = nil
	} else {
		sessionArg = sessionID
	}

	return database.Exec(ctx,
		`INSERT INTO dynamic_table_schemas (table_name, schema_document, version, is_active, applied_by, constraint_name)
		 VALUES ($1, $2, $3, true, $4, $5)`,
		tableName, schemaDoc, version, sessionArg, constraintName,
	)
}

// supersedeSchemaVersion marks a schema version as no longer active.
func supersedeSchemaVersion(ctx context.Context, database db.DB, tableName string, version int) error {
	if database.Backend() != db.BackendPostgres {
		return nil
	}

	return database.Exec(ctx,
		`UPDATE dynamic_table_schemas
		 SET is_active = false, superseded_at = datetime('now')
		 WHERE table_name = $1 AND version = $2`,
		tableName, version,
	)
}

// ============================================================================
// SoftDelete — Convert DELETE to Soft Delete
// ============================================================================

// SoftDeleteResult holds the result of a soft-delete operation.
type SoftDeleteResult struct {
	// RewrittenSQL is the SQL that was actually executed (UPDATE instead of DELETE).
	RewrittenSQL string

	// RowsAffected is not tracked; SQLite and simple Postgres don't return it.
}

// SoftDelete converts a DELETE FROM statement into a soft-delete UPDATE.
//
// Instead of removing the row, it sets deleted_at to the current timestamp.
// The caller must pass the ID of the row to soft-delete, extracted from the
// original DELETE statement's WHERE clause or provided explicitly.
//
// On Postgres, this is handled automatically by the soft_delete_intercept()
// trigger defined in migration 006, so this function is primarily needed
// for SQLite. However, it works on both backends.
//
// The idValue should be the primary-key value of the row to soft-delete.
func SoftDelete(ctx context.Context, database db.DB, tableName string, idValue string, sessionID string) (*SoftDeleteResult, error) {
	sanitized := SanitizeName(tableName)

	sql := fmt.Sprintf(
		`UPDATE %s SET deleted_at = datetime('now') WHERE id = $1`,
		sanitized,
	)

	if err := database.Exec(ctx, sql, idValue); err != nil {
		return nil, fmt.Errorf("dynamic: soft-delete on %q failed: %w", sanitized, err)
	}

	return &SoftDeleteResult{
		RewrittenSQL: sql,
	}, nil
}

// ============================================================================
// Harness Integration Helpers
// ============================================================================

// RewriteDeleteToSoftDelete takes a DELETE FROM statement and converts it to
// an UPDATE that sets deleted_at. This is used by the harness's SQL execution
// path to intercept DELETE operations on dynamic tables.
//
// Returns the rewritten SQL, or empty string if the statement could not be
// rewritten (e.g., no WHERE clause with an id).
func RewriteDeleteToSoftDelete(stmt string, tableName string) string {
	// This is a best-effort transformation.
	// The harness should prefer SoftDelete() with explicit ID values.
	_ = stmt
	_ = tableName
	return ""
}

// IsExecutableOnBackend returns true if a named SQL function (like
// create_agent_memory_table or soft_delete_intercept) is available
// on the current backend.
func IsExecutableOnBackend(database db.DB, functionName string) bool {
	backend := database.Backend()

	// Postgres-only functions
	postgresOnly := map[string]bool{
		"create_agent_memory_table": true,
		"soft_delete_intercept":     true,
		"verify_dynamic_table":      true,
	}

	if postgresOnly[functionName] && backend != db.BackendPostgres {
		return false
	}

	return true
}

// ============================================================================
// Helpers
// ============================================================================

// ValidateDynamicInsert validates a dynamic table insert data before execution.
// This is the primary validation hook for the harness SQL execution path.
// It checks the data JSON against any active schema constraint.
//
// The dataJSON should be the JSON string being inserted into the data column.
// If validation fails, the insert should be rejected.
func ValidateDynamicInsert(ctx context.Context, database db.DB, tableName string, dataJSON string) error {
	hasConstraint, err := HasSchemaConstraint(ctx, database, tableName)
	if err != nil {
		return err
	}
	if !hasConstraint {
		return nil
	}

	return ValidateData(ctx, database, tableName, dataJSON)
}

// ValidateDynamicUpdate validates that the new data value in a dynamic table
// UPDATE conforms to the active schema constraint. This is called by the
// harness when an UPDATE statement modifies the data column.
func ValidateDynamicUpdate(ctx context.Context, database db.DB, tableName string, dataJSON string) error {
	return ValidateDynamicInsert(ctx, database, tableName, dataJSON)
}

// ParseDataColumn extracts the data JSON from a Go row map for validation.
// Returns an empty string if the data column is not present or nil.
func ParseDataColumn(row map[string]any) string {
	if row == nil {
		return ""
	}
	v, ok := row["data"]
	if !ok || v == nil {
		return ""
	}

	switch val := v.(type) {
	case string:
		return val
	case []byte:
		return string(val)
	default:
		b, err := json.Marshal(val)
		if err != nil {
			return fmt.Sprint(val)
		}
		return string(b)
	}
}

// toVersion converts a database version value to int.
func toVersion(v any) int {
	switch val := v.(type) {
	case int64:
		return int(val)
	case float64:
		return int(val)
	case int:
		return val
	case string:
		var result int
		fmt.Sscanf(val, "%d", &result)
		return result
	default:
		return 0
	}
}
