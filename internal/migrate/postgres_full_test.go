// Package migrate — comprehensive PostgreSQL integration test.
//
// This test applies all Postgres-compatible migrations (001-016), verifies
// every table, index, and trigger, then exercises CRUD, FK constraints, and
// append-only enforcement on memory_events.
//
// Skip condition: CONSENSUS_TEST_POSTGRES_URL must be set.
// Requires: docker compose up -d (postgres:16-alpine on port 5432).
//
// axiom:trace work_item=WI-postgres-full-integration
//
//	spec=specs/003-database.md
//	plan=phase-1/task-1/step-1
//	test=internal/migrate/postgres_full_test.go
package migrate

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/wojons/consensus/internal/db"
	"github.com/wojons/consensus/internal/db/driver"
)

// ============================================================================
// TestPostgresFullIntegration — comprehensive PG migration + schema + CRUD test
// ============================================================================

func TestPostgresFullIntegration(t *testing.T) {
	pgURL := os.Getenv("CONSENSUS_TEST_POSTGRES_URL")
	if pgURL == "" {
		t.Skip("CONSENSUS_TEST_POSTGRES_URL not set; skipping Postgres full integration test")
	}

	ctx := context.Background()
	database, err := driver.Open(ctx, db.Config{URL: pgURL})
	if err != nil {
		t.Fatalf("failed to open Postgres connection: %v", err)
	}
	defer func() {
		if err := database.Close(); err != nil {
			t.Logf("warning: failed to close database: %v", err)
		}
	}()

	backend := database.Backend()
	if backend != db.BackendPostgres {
		t.Fatalf("expected postgres backend, got %s", backend)
	}
	t.Logf("Connected to Postgres: %s", pgURL)

	runner := New(database)

	// =========================================================================
	// SUBTEST 1 — Bootstrap + Load + Apply all Postgres-compatible migrations
	// =========================================================================
	t.Run("ApplyAllMigrations", func(t *testing.T) {
		if err := runner.Bootstrap(ctx); err != nil {
			t.Fatalf("Bootstrap failed: %v", err)
		}
		t.Log("✓ schema_versions table created")

		if err := runner.LoadMigrations(); err != nil {
			t.Fatalf("LoadMigrations failed: %v", err)
		}
		t.Logf("Loaded %d embedded migrations", len(runner.migrations))

		// List which migrations will be applied (009, 011 are _sqlite_ → skipped)
		for _, m := range runner.migrations {
			t.Logf("  migration %03d: %s (%d bytes)", m.Version, m.Filename, len(m.SQL))
		}

		applied, err := runner.Up(ctx)
		if err != nil {
			t.Fatalf("Up failed: %v", err)
		}
		t.Logf("Applied %d migrations: %v", len(applied), applied)

		state, err := runner.GetState(ctx)
		if err != nil {
			t.Fatalf("GetState failed: %v", err)
		}
		t.Logf("Current version: %d, Pending: %v", state.CurrentVersion, state.PendingMigrations)
		if state.DriftDetected {
			t.Errorf("unexpected drift: %s", state.DriftDetails)
		}
	})

	// =========================================================================
	// SUBTEST 2 — Verify all expected tables exist
	// =========================================================================
	t.Run("VerifyAllTables", func(t *testing.T) {
		expectedTables := []string{
			// 001_initial_schema
			"sessions", "iteration_commits", "audit_logs", "system_settings",
			"agent_messages", "memory_events", "display_modes", "memory_pages",
			"compression_queue", "model_registry",
			"tasks", "tool_requests", "tool_results", "tools_registry",
			"skills_registry", "agent_billing", "workflows", "custom_agent_tools",
			"tool_files", "external_quarantine", "secret_access_audit",
			// 002_shim_session_map
			"shim_session_map", "api_keys", "api_rate_limits",
			// 003_circuit_breakers
			"agent_circuit_breakers",
			// 004_staging_buffer
			"staging_buffer",
			// 005_agent_budget_limits
			"agent_budget_limits",
			// 007_webhook_tables
			"webhook_registrations", "external_events", "routing_rules",
			// 008_hitl_tables
			"approval_requests", "hitl_configuration", "notification_log",
			// 015_projects_and_scope
			"projects",
			// 016_embedding_model
			"event_embeddings",
			// schema_versions (created by Bootstrap)
			"schema_versions",
		}

		rows, err := database.Query(ctx,
			`SELECT table_name FROM information_schema.tables
			 WHERE table_schema = 'public'
			 ORDER BY table_name`)
		if err != nil {
			t.Fatalf("failed to query information_schema.tables: %v", err)
		}

		actual := make(map[string]bool)
		for _, row := range rows {
			name, _ := row["table_name"].(string)
			actual[name] = true
		}

		for _, expected := range expectedTables {
			if !actual[expected] {
				t.Errorf("missing table: %s", expected)
			}
		}

		// Report any extra tables (views, etc.)
		for name := range actual {
			found := false
			for _, expected := range expectedTables {
				if name == expected {
					found = true
					break
				}
			}
			if !found {
				t.Logf("extra table in public schema: %s", name)
			}
		}

		t.Logf("Verified %d expected tables present", len(expectedTables))
	})

	// =========================================================================
	// SUBTEST 3 — Verify key indexes exist
	// =========================================================================
	t.Run("VerifyIndexes", func(t *testing.T) {
		expectedIndexes := []string{
			"idx_sessions_parent",
			"idx_sessions_status",
			"idx_iteration_session",
			"idx_session_iteration",
			"idx_audit_session",
			"idx_messages_target",
			"idx_memory_session",
			"idx_tasks_session_status",
			"idx_tasks_locked_by",
			"idx_tool_req_pending",
			"idx_tool_result_request",
			"idx_billing_session",
			"idx_billing_recorded",
			"idx_tool_files_session",
			"idx_quarantine_pending",
			"idx_secret_audit_session",
			"idx_shim_session_map",
			"idx_shim_session_map_session",
			"idx_api_keys_prefix",
			"idx_api_keys_session",
			"idx_budget_limits_agent",
			"idx_staging_session",
			"idx_staging_status",
			"idx_events_source_id",
			"idx_events_pending",
			"idx_events_status",
			"idx_events_session",
			"idx_routing_rules_priority",
			"idx_approvals_pending",
			"idx_approvals_session",
			"idx_hitl_config_scope",
			"idx_hitl_config_session",
			"idx_notif_log_approval",
			"idx_tool_requests_approval",
			"idx_sessions_project",
			"idx_tasks_project",
		}

		rows, err := database.Query(ctx,
			`SELECT indexname FROM pg_indexes
			 WHERE schemaname = 'public'
			 ORDER BY indexname`)
		if err != nil {
			t.Fatalf("failed to query pg_indexes: %v", err)
		}

		actual := make(map[string]bool)
		for _, row := range rows {
			name, _ := row["indexname"].(string)
			actual[name] = true
		}

		missing := 0
		for _, expected := range expectedIndexes {
			if !actual[expected] {
				t.Errorf("missing index: %s", expected)
				missing++
			}
		}

		if missing == 0 {
			t.Logf("All %d expected indexes present", len(expectedIndexes))
		}
	})

	// =========================================================================
	// SUBTEST 4 — Verify SSE notification triggers exist
	// =========================================================================
	t.Run("VerifyTriggers", func(t *testing.T) {
		expectedTriggers := []string{
			"session_status_notify",
			"session_create_notify",
			"approval_request_notify",
			"approval_status_notify",
		}

		rows, err := database.Query(ctx,
			`SELECT tgname FROM pg_trigger
			 WHERE tgisinternal = false
			 ORDER BY tgname`)
		if err != nil {
			t.Fatalf("failed to query pg_trigger: %v", err)
		}

		actual := make(map[string]bool)
		for _, row := range rows {
			name, _ := row["tgname"].(string)
			actual[name] = true
		}

		for _, expected := range expectedTriggers {
			if !actual[expected] {
				t.Errorf("missing trigger: %s", expected)
			} else {
				t.Logf("✓ trigger %s exists", expected)
			}
		}
	})

	// =========================================================================
	// SUBTEST 5 — CRUD: create session, create task, insert memory event, query
	// =========================================================================
	t.Run("CRUDOperations", func(t *testing.T) {
		// 5a. Seed a model so FK constraint is satisfied
		err := database.Exec(ctx,
			`INSERT INTO model_registry (model_id, tier, max_context, cost_per_m_in, cost_per_m_out, enabled)
			 VALUES ('test-model', 1, 8192, 1.0, 2.0, true)
			 ON CONFLICT (model_id) DO NOTHING`)
		if err != nil {
			t.Fatalf("seed model: %v", err)
		}

		// 5b. Create a session
		sessionID := "00000000-0000-0000-0000-000000000001"
		err = database.Exec(ctx,
			`INSERT INTO sessions (id, agent_name, model_id, status, goal)
			 VALUES ($1, 'test-agent', 'test-model', 'idle', 'integration test')`,
			sessionID)
		if err != nil {
			t.Fatalf("create session: %v", err)
		}
		t.Logf("✓ created session %s", sessionID)

		// 5b. Query the session back
		row, err := database.QueryRow(ctx,
			`SELECT id, agent_name, status, goal FROM sessions WHERE id = $1`,
			sessionID)
		if err != nil {
			t.Fatalf("query session: %v", err)
		}
		if row == nil {
			t.Fatal("session not found after insert")
		}
		agentName, _ := row["agent_name"].(string)
		status, _ := row["status"].(string)
		if agentName != "test-agent" || status != "idle" {
			t.Errorf("session data mismatch: agent=%q status=%q", agentName, status)
		}
		t.Logf("✓ queried session: agent=%s status=%s", agentName, status)

		// 5c. Create a task linked to the session
		taskID := "00000000-0000-0000-0000-000000000002"
		err = database.Exec(ctx,
			`INSERT INTO tasks (id, session_id, title, status, priority)
			 VALUES ($1, $2, 'test task', 'pending', 5)`,
			taskID, sessionID)
		if err != nil {
			t.Fatalf("create task: %v", err)
		}
		t.Logf("✓ created task %s", taskID)

		// 5d. Query the task back
		row, err = database.QueryRow(ctx,
			`SELECT id, title, status FROM tasks WHERE id = $1`, taskID)
		if err != nil {
			t.Fatalf("query task: %v", err)
		}
		if row == nil {
			t.Fatal("task not found after insert")
		}
		title, _ := row["title"].(string)
		if title != "test task" {
			t.Errorf("task title mismatch: %q", title)
		}
		t.Logf("✓ queried task: title=%s", title)

		// 5e. Insert a memory event
		err = database.Exec(ctx,
			`INSERT INTO memory_events (type, content, session_id, iteration_created)
			 VALUES ('text_block', 'Hello from integration test', $1, 1)`,
			sessionID)
		if err != nil {
			t.Fatalf("insert memory event: %v", err)
		}
		t.Log("✓ inserted memory event")

		// 5f. Query memory events back
		rows, err := database.Query(ctx,
			`SELECT id, type, content FROM memory_events
			 WHERE session_id = $1
			 ORDER BY id`, sessionID)
		if err != nil {
			t.Fatalf("query memory events: %v", err)
		}
		if len(rows) == 0 {
			t.Fatal("no memory events found")
		}
		for _, r := range rows {
			evType, _ := r["type"].(string)
			content, _ := r["content"].(string)
			t.Logf("  memory event: type=%s content=%q", evType, content)
		}
		t.Logf("✓ queried %d memory events", len(rows))
	})

	// =========================================================================
	// SUBTEST 6 — FK constraint: invalid reference must error
	// =========================================================================
	t.Run("FKConstraint", func(t *testing.T) {
		// Try to insert a task referencing a non-existent session
		badSessionID := "ffffffff-ffff-ffff-ffff-ffffffffffff"
		err := database.Exec(ctx,
			`INSERT INTO tasks (id, session_id, title, status, priority)
			 VALUES ($1, $2, 'orphan task', 'pending', 5)`,
			"00000000-0000-0000-0000-000000000099", badSessionID)
		if err == nil {
			t.Error("expected FK constraint error, but insert succeeded")
		} else {
			t.Logf("✓ FK constraint enforced: %v", err)
		}

		// Also try inserting a memory event with bad session reference
		err = database.Exec(ctx,
			`INSERT INTO memory_events (type, content, session_id, iteration_created)
			 VALUES ('text_block', 'orphan event', $1, 1)`,
			badSessionID)
		if err == nil {
			t.Error("expected FK constraint error for memory_events, but insert succeeded")
		} else {
			t.Logf("✓ FK constraint on memory_events enforced: %v", err)
		}
	})

	// =========================================================================
	// SUBTEST 7 — Append-only: UPDATE/DELETE on memory_events must error
	// =========================================================================
	t.Run("AppendOnly", func(t *testing.T) {
		// Migration 017 (_sqlite_) is skipped on Postgres.
		// Migration 018 (_postgres_) installs PL/pgSQL triggers that enforce
		// append-only on memory_events at the database level.

		// Verify the trigger function exists.
		funcRow, err := database.QueryRow(ctx,
			`SELECT proname FROM pg_proc WHERE proname = 'enforce_memory_events_append_only'`)
		if err != nil || funcRow == nil {
			t.Fatalf("migration 018: trigger function not installed: %v", err)
		}
		t.Log("✓ enforce_memory_events_append_only function exists")

		// Verify the triggers exist.
		triggerRows, err := database.Query(ctx,
			`SELECT tgname FROM pg_trigger WHERE tgname LIKE 'trg_memory_%' ORDER BY tgname`)
		if err != nil {
			t.Fatalf("query pg_trigger: %v", err)
		}
		if len(triggerRows) < 2 {
			t.Fatalf("migration 018: expected 2 triggers, got %d", len(triggerRows))
		}
		t.Logf("✓ %d append-only triggers installed on memory_events", len(triggerRows))

		// Get a real memory event ID to attempt UPDATE/DELETE on
		rows, err := database.Query(ctx,
			`SELECT id FROM memory_events ORDER BY id LIMIT 1`)
		if err != nil || len(rows) == 0 {
			t.Skip("no memory events to test append-only on")
			return
		}
		eventID := rows[0]["id"]
		t.Logf("testing append-only on memory event id=%v", eventID)

		// Attempt UPDATE — must fail
		err = database.Exec(ctx,
			`UPDATE memory_events SET content = 'modified' WHERE id = $1`, eventID)
		if err == nil {
			t.Error("UPDATE on memory_events succeeded — append-only violated")
		} else {
			t.Logf("✓ UPDATE blocked: %v", err)
		}

		// Attempt DELETE — must fail
		err = database.Exec(ctx,
			`DELETE FROM memory_events WHERE id = $1`, eventID)
		if err == nil {
			t.Error("DELETE on memory_events succeeded — append-only violated")
		} else {
			t.Logf("✓ DELETE blocked: %v", err)
		}
	})

	// =========================================================================
	// SUBTEST 8 — Verify migration 016 seeded data
	// =========================================================================
	t.Run("VerifySeedData", func(t *testing.T) {
		// embedding model in model_registry
		row, err := database.QueryRow(ctx,
			`SELECT model_id, classifier_tags FROM model_registry
			 WHERE model_id = 'text-embedding-3-small'`)
		if err != nil {
			t.Fatalf("query embedding model: %v", err)
		}
		if row == nil {
			t.Error("embedding model not seeded in model_registry")
		} else {
			tags := fmt.Sprintf("%v", row["classifier_tags"])
			if !strings.Contains(tags, "embedding") {
				t.Errorf("embedding model missing 'embedding' tag: %s", tags)
			}
			t.Logf("✓ embedding model seeded: %s tags=%s", row["model_id"], tags)
		}

		// embedding_model in system_settings
		row, err = database.QueryRow(ctx,
			`SELECT value FROM system_settings WHERE key = 'embedding_model'`)
		if err != nil {
			t.Fatalf("query system_settings: %v", err)
		}
		if row == nil {
			t.Error("embedding_model key not found in system_settings")
		} else {
			t.Logf("✓ system_settings.embedding_model = %s", row["value"])
		}
	})

	// =========================================================================
	// SUBTEST 9 — Verify projects table and session/task project_id columns
	// =========================================================================
	t.Run("VerifyProjectsScope", func(t *testing.T) {
		// projects table exists (verified in VerifyAllTables)
		// Insert a project
		projectID := "00000000-0000-0000-0000-000000000010"
		err := database.Exec(ctx,
			`INSERT INTO projects (id, name, description)
			 VALUES ($1, 'test-project', 'integration test project')
			 ON CONFLICT (id) DO NOTHING`,
			projectID)
		if err != nil {
			t.Fatalf("create project: %v", err)
		}
		t.Logf("✓ created project %s", projectID)

		// Verify sessions.project_id column exists
		rows, err := database.Query(ctx,
			`SELECT column_name FROM information_schema.columns
			 WHERE table_name = 'sessions' AND column_name = 'project_id'`)
		if err != nil || len(rows) == 0 {
			t.Error("sessions.project_id column missing")
		} else {
			t.Log("✓ sessions.project_id column exists")
		}

		// Verify tasks.project_id column exists
		rows, err = database.Query(ctx,
			`SELECT column_name FROM information_schema.columns
			 WHERE table_name = 'tasks' AND column_name = 'project_id'`)
		if err != nil || len(rows) == 0 {
			t.Error("tasks.project_id column missing")
		} else {
			t.Log("✓ tasks.project_id column exists")
		}
	})

	// =========================================================================
	// SUBTEST 10 — Verify trust_level column on sessions (migration 013)
	// =========================================================================
	t.Run("VerifyTrustLevel", func(t *testing.T) {
		rows, err := database.Query(ctx,
			`SELECT column_name, data_type, column_default
			 FROM information_schema.columns
			 WHERE table_name = 'sessions' AND column_name = 'trust_level'`)
		if err != nil || len(rows) == 0 {
			t.Error("sessions.trust_level column missing (migration 013)")
		} else {
			col := rows[0]
			t.Logf("✓ sessions.trust_level: type=%s default=%v",
				col["data_type"], col["column_default"])
		}
	})

	t.Log("=== Postgres full integration test complete ===")
}
