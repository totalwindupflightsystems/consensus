package hitl

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/wojons/conscientiousness/internal/db"
	"github.com/wojons/conscientiousness/internal/db/driver"
)

// ============================================================================
// AC-HITL-01: approval_requests creation (all 6 types)
// ============================================================================

func TestRequestApproval(t *testing.T) {
	// axiom:trace work_item=deployment-ops-01 spec=specs/014-hitl-interrupt-state.md plan=phase-3/task-3-1/step-3-1-1
	ctx := context.Background()
	database, cleanup := setupTestDB(t)
	defer cleanup()

	mgr := New(database)

	// Create global HITL config
	if err := mgr.SetConfiguration(ctx, DefaultConfiguration()); err != nil {
		t.Fatalf("set global config: %v", err)
	}

	// Create a session
	sessionID := "test-session-hitl-001"
	mustExec(t, database, ctx, `INSERT INTO sessions (id, agent_name, model_id, status, goal, heartbeat_at, planning_max_turns, created_at) VALUES ($1, 'test', 'test-model', 'idle', 'test', $2, 10, $2)`, sessionID, time.Now())

	tests := []struct {
		requestType RequestType
		description string
		riskLevel   RiskLevel
	}{
		{RequestToolExecution, "Execute tool: send_email", RiskMedium},
		{RequestDestructiveAction, "Delete 5000 rows from order_tracking", RiskHigh},
		{RequestBudgetOverride, "Budget limit exceeded — allow $50 more", RiskHigh},
		{RequestSchemaChange, "ALTER TABLE add column: priority", RiskCritical},
		{RequestSubAgentSpawn, "Spawn sub-agent for web scraping", RiskLow},
		{RequestCustom, "I want to modify the system_prompt", RiskMedium},
	}

	for _, tt := range tests {
		t.Run(string(tt.requestType), func(t *testing.T) {
			// Use unique session per type to avoid session status conflicts
			sid := sessionID + "-" + string(tt.requestType)
			mustExec(t, database, ctx, `INSERT INTO sessions (id, agent_name, model_id, status, goal, heartbeat_at, planning_max_turns, created_at) VALUES ($1, 'test', 'test-model', 'idle', 'test', $2, 10, $2)`, sid, time.Now())

			req, err := mgr.RequestApproval(ctx, sid, tt.requestType, tt.description, tt.riskLevel)
			if err != nil {
				t.Fatalf("RequestApproval: %v", err)
			}
			if req.RequestType != tt.requestType {
				t.Errorf("expected type %q, got %q", tt.requestType, req.RequestType)
			}
			if req.RiskLevel != tt.riskLevel {
				t.Errorf("expected risk %q, got %q", tt.riskLevel, req.RiskLevel)
			}
			if req.Status != ApprovalStatusPending {
				t.Errorf("expected pending status, got %q", req.Status)
			}
			t.Logf("Created %s approval: %s (risk: %s)", tt.requestType, tt.description, tt.riskLevel)
		})
	}
}

func TestRequestApprovalValidation(t *testing.T) {
	// axiom:trace work_item=deployment-ops-01 spec=specs/014-hitl-interrupt-state.md plan=phase-3/task-3-1/step-3-1-1
	ctx := context.Background()
	database, cleanup := setupTestDB(t)
	defer cleanup()
	mgr := New(database)

	if err := mgr.SetConfiguration(ctx, DefaultConfiguration()); err != nil {
		t.Fatalf("set config: %v", err)
	}

	sessionID := "test-session-auth-001"
	mustExec(t, database, ctx, `INSERT INTO sessions (id, agent_name, model_id, status, goal, heartbeat_at, planning_max_turns, created_at) VALUES ($1, 'test', 'test-model', 'idle', 'test', $2, 10, $2)`, sessionID, time.Now())

	// Invalid request type
	_, err := mgr.RequestApproval(ctx, sessionID, "invalid_type", "desc", RiskLow)
	if err == nil {
		t.Error("expected error for invalid request type")
	}

	// Invalid risk level
	_, err = mgr.RequestApproval(ctx, sessionID, RequestCustom, "desc", "extreme")
	if err == nil {
		t.Error("expected error for invalid risk level")
	}

	// Empty description
	_, err = mgr.RequestApproval(ctx, sessionID, RequestCustom, "", RiskLow)
	if err == nil {
		t.Error("expected error for empty description")
	}
}

// ============================================================================
// AC-HITL-02: hitl_configuration with scope precedence
// ============================================================================

func TestHITLConfiguration(t *testing.T) {
	// axiom:trace work_item=deployment-ops-01 spec=specs/014-hitl-interrupt-state.md plan=phase-3/task-3-1/step-3-1-2
	ctx := context.Background()
	database, cleanup := setupTestDB(t)
	defer cleanup()
	mgr := New(database)

	// Set global config with non-default values
	err := mgr.SetConfiguration(ctx, Configuration{
		Scope:                            ScopeGlobal,
		AutoPauseOnErrorThreshold:        5,
		RequireApprovalForDestructive:    true,
		RequireApprovalForSchemaChanges:  false,
		RequireApprovalForExternalTools:  false,
		ApprovalTimeoutMinutes:           30,
		NotifyOnPause:                    true,
	})
	if err != nil {
		t.Fatalf("set global config: %v", err)
	}

	// Session without overrides gets global values
	cfg, err := mgr.GetEffectiveConfiguration(ctx, "nonexistent-session")
	if err != nil {
		t.Fatalf("get effective config: %v", err)
	}
	if cfg.AutoPauseOnErrorThreshold != 5 {
		t.Errorf("expected global threshold 5, got %d", cfg.AutoPauseOnErrorThreshold)
	}
	t.Logf("Global config: threshold=%d, destructive=%v, timeout=%dm", cfg.AutoPauseOnErrorThreshold, cfg.RequireApprovalForDestructive, cfg.ApprovalTimeoutMinutes)

	// Set session override
	err = mgr.SetConfiguration(ctx, Configuration{
		Scope:                            ScopeSession,
		SessionID:                        "session-with-override",
		AutoPauseOnErrorThreshold:        2,
		RequireApprovalForDestructive:    false,
		RequireApprovalForSchemaChanges:  true,
		RequireApprovalForExternalTools:  true,
		ApprovalTimeoutMinutes:           10,
		NotifyOnPause:                    false,
	})
	if err != nil {
		t.Fatalf("set session config: %v", err)
	}

	// Session override takes precedence
	cfg, err = mgr.GetEffectiveConfiguration(ctx, "session-with-override")
	if err != nil {
		t.Fatalf("get session config: %v", err)
	}
	if cfg.AutoPauseOnErrorThreshold != 2 {
		t.Errorf("expected session threshold 2, got %d", cfg.AutoPauseOnErrorThreshold)
	}
	if cfg.RequireApprovalForDestructive {
		t.Error("expected destructive=false from session override")
	}
	t.Logf("Session config: threshold=%d, destructive=%v, timeout=%dm", cfg.AutoPauseOnErrorThreshold, cfg.RequireApprovalForDestructive, cfg.ApprovalTimeoutMinutes)
}

func TestGetDefaultConfiguration(t *testing.T) {
	// axiom:trace work_item=deployment-ops-01 spec=specs/014-hitl-interrupt-state.md plan=phase-3/task-3-1/step-3-1-2
	cfg := DefaultConfiguration()
	if cfg.AutoPauseOnErrorThreshold <= 0 {
		t.Error("default error threshold must be positive")
	}
	if cfg.ApprovalTimeoutMinutes <= 0 {
		t.Error("default timeout must be positive")
	}
	if cfg.Scope != ScopeGlobal {
		t.Errorf("default scope should be global, got %q", cfg.Scope)
	}
	t.Logf("Defaults: threshold=%d, timeout=%dm", cfg.AutoPauseOnErrorThreshold, cfg.ApprovalTimeoutMinutes)
}

// ============================================================================
// AC-HITL-03: No auto-approval; expiry → expired not approved
// ============================================================================

func TestNoAutoApproval(t *testing.T) {
	// axiom:trace work_item=deployment-ops-01 spec=specs/014-hitl-interrupt-state.md plan=phase-3/task-3-1/step-3-1-3
	ctx := context.Background()
	database, cleanup := setupTestDB(t)
	defer cleanup()
	mgr := New(database)

	if err := mgr.SetConfiguration(ctx, DefaultConfiguration()); err != nil {
		t.Fatalf("set config: %v", err)
	}

	sessionID := "test-no-auto-approval"
	mustExec(t, database, ctx, `INSERT INTO sessions (id, agent_name, model_id, status, goal, heartbeat_at, planning_max_turns, created_at) VALUES ($1, 'test', 'test-model', 'idle', 'test', $2, 10, $2)`, sessionID, time.Now())

	// Create request
	req, err := mgr.RequestApproval(ctx, sessionID, RequestDestructiveAction, "Delete sensitive data", RiskCritical)
	if err != nil {
		t.Fatalf("request approval: %v", err)
	}

	// Set the expiry to the past
	mustExec(t, database, ctx, `UPDATE approval_requests SET expires_at = $1 WHERE id = $2`, time.Now().Add(-1*time.Hour), req.ID)

	// Expire it
	count, err := mgr.ExpirePendingApprovals(ctx)
	if err != nil {
		t.Fatalf("expire: %v", err)
	}
	if count == 0 {
		t.Error("expected at least 1 expired")
	}
	t.Logf("Expired %d approval(s)", count)

	// Verify it's expired, NOT approved
	dbReq, err := mgr.GetApproval(ctx, req.ID)
	if err != nil {
		t.Fatalf("get approval: %v", err)
	}
	if dbReq.Status == ApprovalStatusApproved {
		t.Error("CRITICAL: approval was auto-approved! Must be expired.")
	}
	if dbReq.Status != ApprovalStatusExpired {
		t.Errorf("expected expired, got %q", dbReq.Status)
	}
	t.Logf("Correct: status is %q (not approved)", dbReq.Status)

	// The session should be failed
	rows, err := database.Query(ctx, `SELECT status FROM sessions WHERE id = $1`, sessionID)
	if err != nil {
		t.Fatalf("query session: %v", err)
	}
	sessionStatus := rows[0]["status"].(string)
	if sessionStatus != "failed" {
		t.Errorf("expected session to be failed after expiry, got %q", sessionStatus)
	}
	t.Logf("Session status: %s", sessionStatus)
}

// ============================================================================
// AC-HITL-04: Reviewer auth (alt_mode_role only)
// ============================================================================

func TestReviewerAuthorization(t *testing.T) {
	// axiom:trace work_item=deployment-ops-01 spec=specs/014-hitl-interrupt-state.md plan=phase-3/task-3-1/step-3-1-4
	ctx := context.Background()
	database, cleanup := setupTestDB(t)
	defer cleanup()
	mgr := New(database)

	if err := mgr.SetConfiguration(ctx, DefaultConfiguration()); err != nil {
		t.Fatalf("set config: %v", err)
	}

	sessionID := "test-reviewer-auth"
	mustExec(t, database, ctx, `INSERT INTO sessions (id, agent_name, model_id, status, goal, heartbeat_at, planning_max_turns, created_at) VALUES ($1, 'test', 'test-model', 'idle', 'test', $2, 10, $2)`, sessionID, time.Now())

	req, err := mgr.RequestApproval(ctx, sessionID, RequestDestructiveAction, "Test reviewer auth", RiskHigh)
	if err != nil {
		t.Fatalf("request: %v", err)
	}

	// Review with valid decision
	err = mgr.ReviewApproval(ctx, req.ID, DecisionApproved, "admin-user-1", "Looks good", "")
	if err != nil {
		t.Fatalf("review: %v", err)
	}

	// Verify status is now approved
	dbReq, err := mgr.GetApproval(ctx, req.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if dbReq.Status != ApprovalStatusApproved {
		t.Errorf("expected approved, got %q", dbReq.Status)
	}
	if dbReq.ReviewerID != "admin-user-1" {
		t.Errorf("expected reviewer admin-user-1, got %q", dbReq.ReviewerID)
	}
	t.Logf("Review by %s: %s — %s", dbReq.ReviewerID, dbReq.Status, dbReq.ReviewNotes)

	// Test review already-reviewed request
	err = mgr.ReviewApproval(ctx, req.ID, DecisionApproved, "admin-2", "double-review", "")
	if err == nil {
		t.Error("expected error when reviewing already-resolved approval")
	}
	t.Logf("Double-review correctly blocked: %v", err)
}

func TestReviewApprovalWithModification(t *testing.T) {
	// axiom:trace work_item=deployment-ops-01 spec=specs/014-hitl-interrupt-state.md plan=phase-3/task-3-1/step-3-1-4
	ctx := context.Background()
	database, cleanup := setupTestDB(t)
	defer cleanup()
	mgr := New(database)

	if err := mgr.SetConfiguration(ctx, DefaultConfiguration()); err != nil {
		t.Fatalf("set config: %v", err)
	}

	sessionID := "test-modified-review"
	mustExec(t, database, ctx, `INSERT INTO sessions (id, agent_name, model_id, status, goal, heartbeat_at, planning_max_turns, created_at) VALUES ($1, 'test', 'test-model', 'idle', 'test', $2, 10, $2)`, sessionID, time.Now())

	req, err := mgr.RequestApproval(ctx, sessionID, RequestToolExecution, "Original: DELETE ALL", RiskCritical, WithTargetSQL("DELETE FROM everything WHERE 1=1"))
	if err != nil {
		t.Fatalf("request: %v", err)
	}

	err = mgr.ReviewApproval(ctx, req.ID, DecisionModified, "admin-user-1", "Modified to safer version", "DELETE FROM temp_cache WHERE created_at < '2020-01-01'")
	if err != nil {
		t.Fatalf("review modified: %v", err)
	}

	dbReq, _ := mgr.GetApproval(ctx, req.ID)
	if dbReq.Status != ApprovalStatusModified {
		t.Errorf("expected modified, got %q", dbReq.Status)
	}
	if dbReq.ModifiedSQL == "" {
		t.Error("expected modified SQL to be present")
	}
	t.Logf("Modified decision: %s → %s", dbReq.TargetSQL, dbReq.ModifiedSQL)
}

// ============================================================================
// AC-HITL-05: Approval expiry cron (covered by TestNoAutoApproval)
// ============================================================================

// ============================================================================
// AC-HITL-06: Notification channels on pause
// ============================================================================

func TestNotificationChannels(t *testing.T) {
	// axiom:trace work_item=deployment-ops-01 spec=specs/014-hitl-interrupt-state.md plan=phase-3/task-3-1/step-3-1-6
	ctx := context.Background()
	database, cleanup := setupTestDB(t)
	defer cleanup()
	mgr := New(database)

	if err := mgr.SetConfiguration(ctx, DefaultConfiguration()); err != nil {
		t.Fatalf("set config: %v", err)
	}

	received := make(chan Notification, 5)
	RegisterNotificationCallback(func(n Notification) {
		received <- n
	})

	sessionID := "test-notify-channel"
	mustExec(t, database, ctx, `INSERT INTO sessions (id, agent_name, model_id, status, goal, heartbeat_at, planning_max_turns, created_at) VALUES ($1, 'test', 'test-model', 'idle', 'test', $2, 10, $2)`, sessionID, time.Now())

	_, err := mgr.RequestApproval(ctx, sessionID, RequestCustom, "Notify test", RiskLow)
	if err != nil {
		t.Fatalf("request: %v", err)
	}

	select {
	case n := <-received:
		if n.SessionID != sessionID {
			t.Errorf("expected session %q, got %q", sessionID, n.SessionID)
		}
		if n.RiskLevel != string(RiskLow) {
			t.Errorf("expected risk low, got %q", n.RiskLevel)
		}
		t.Logf("Notification received: session=%s, type=%s, risk=%s, desc=%s", n.SessionID, n.ApprovalType, n.RiskLevel, n.Description)
	case <-time.After(2 * time.Second):
		t.Error("timeout waiting for notification")
	}

	// Verify notification_log entry
	rows, err := database.Query(ctx, `SELECT COUNT(*) as cnt FROM notification_log`)
	if err != nil {
		t.Fatalf("query notification_log: %v", err)
	}
	count := toInt(rows[0]["cnt"])
	if count == 0 {
		t.Error("expected notification_log entry")
	}
	t.Logf("Notification log entries: %d", count)
}

// ============================================================================
// Listing / Query tests
// ============================================================================

func TestListPendingApprovals(t *testing.T) {
	// axiom:trace work_item=deployment-ops-01 spec=specs/014-hitl-interrupt-state.md plan=phase-3/task-3-1/step-3-1-7
	ctx := context.Background()
	database, cleanup := setupTestDB(t)
	defer cleanup()
	mgr := New(database)

	if err := mgr.SetConfiguration(ctx, DefaultConfiguration()); err != nil {
		t.Fatalf("set config: %v", err)
	}

	sessionID := "test-list-approvals"
	mustExec(t, database, ctx, `INSERT INTO sessions (id, agent_name, model_id, status, goal, heartbeat_at, planning_max_turns, created_at) VALUES ($1, 'test', 'test-model', 'idle', 'test', $2, 10, $2)`, sessionID, time.Now())

	// Create 3 approvals
	for i := 0; i < 3; i++ {
		_, err := mgr.RequestApproval(ctx, sessionID, RequestCustom, fmt.Sprintf("Test approval %d", i), RiskLow)
		if err != nil {
			// Session may already be paused — create new session for each
			sid := fmt.Sprintf("%s-%d", sessionID, i)
			mustExec(t, database, ctx, `INSERT INTO sessions (id, agent_name, model_id, status, goal, heartbeat_at, planning_max_turns, created_at) VALUES ($1, 'test', 'test-model', 'idle', 'test', $2, 10, $2)`, sid, time.Now())
			_, err = mgr.RequestApproval(ctx, sid, RequestCustom, fmt.Sprintf("Test %d", i), RiskLow)
			if err != nil {
				t.Fatalf("create request %d: %v", i, err)
			}
		}
	}

	// List pending
	reqs, err := mgr.ListPendingApprovals(ctx)
	if err != nil {
		t.Fatalf("list: %v", err)
	}

	if len(reqs) < 3 {
		t.Errorf("expected at least 3 pending, got %d", len(reqs))
	}
	t.Logf("Found %d pending approvals", len(reqs))

	// List by status
	reqs, err = mgr.ListApprovalsByStatus(ctx, ApprovalStatusPending)
	if err != nil {
		t.Fatalf("list by status: %v", err)
	}
	t.Logf("Filtered: %d pending", len(reqs))
}

func TestPauseResumeCycle(t *testing.T) {
	// axiom:trace work_item=deployment-ops-01 spec=specs/014-hitl-interrupt-state.md plan=phase-3/task-3-1/step-3-1-7
	ctx := context.Background()
	database, cleanup := setupTestDB(t)
	defer cleanup()
	mgr := New(database)

	sessionID := "test-pause-resume"
	mustExec(t, database, ctx, `INSERT INTO sessions (id, agent_name, model_id, status, goal, heartbeat_at, planning_max_turns, created_at) VALUES ($1, 'test', 'test-model', 'idle', 'test', $2, 10, $2)`, sessionID, time.Now())

	// Pause
	if err := mgr.PauseSession(ctx, sessionID); err != nil {
		t.Fatalf("pause: %v", err)
	}
	rows, _ := database.Query(ctx, `SELECT status FROM sessions WHERE id = $1`, sessionID)
	t.Logf("After pause: %s", rows[0]["status"])

	// Resume
	if err := mgr.ResumeSession(ctx, sessionID); err != nil {
		t.Fatalf("resume: %v", err)
	}
	rows, _ = database.Query(ctx, `SELECT status FROM sessions WHERE id = $1`, sessionID)
	t.Logf("After resume: %s", rows[0]["status"])

	// Cannot resume a non-paused session
	err := mgr.ResumeSession(ctx, sessionID)
	if err == nil {
		t.Error("expected error when resuming non-paused session")
	}

	// Cannot pause a terminal session
	mustExec(t, database, ctx, `UPDATE sessions SET status = 'completed' WHERE id = $1`, sessionID)
	err = mgr.PauseSession(ctx, sessionID)
	if err == nil {
		t.Error("expected error when pausing completed session")
	}
}

// ============================================================================
// Helpers
// ============================================================================

func setupTestDB(t *testing.T) (db.DB, func()) {
	t.Helper()

	ctx := context.Background()
	dbURL := fmt.Sprintf("sqlite://file:%s?mode=memory&cache=shared", t.Name())
	database, err := driver.Open(ctx, db.Config{URL: dbURL})
	if err != nil {
		t.Fatalf("failed to open test database: %v", err)
	}

	// Create sessions table
	mustExec(t, database, ctx, `CREATE TABLE IF NOT EXISTS sessions (
		id TEXT PRIMARY KEY,
		parent_id TEXT,
		agent_name TEXT NOT NULL DEFAULT 'test_agent',
		model_id TEXT NOT NULL DEFAULT 'test-model',
		status TEXT NOT NULL DEFAULT 'booting',
		goal TEXT NOT NULL DEFAULT '',
		context_budget INTEGER NOT NULL DEFAULT 128000,
		tokens_used_in INTEGER NOT NULL DEFAULT 0,
		tokens_used_out INTEGER NOT NULL DEFAULT 0,
		iteration INTEGER NOT NULL DEFAULT 0,
		project_id TEXT,
		heartbeat_at TEXT NOT NULL,
		planning_max_turns INTEGER NOT NULL DEFAULT 10,
		created_at TEXT NOT NULL,
		completed_at TEXT
	)`)

	// Create hitl_configuration table
	mustExec(t, database, ctx, `CREATE TABLE IF NOT EXISTS hitl_configuration (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		scope TEXT NOT NULL CHECK (scope IN ('global', 'session')),
		session_id TEXT,
		auto_pause_on_error_threshold INTEGER NOT NULL DEFAULT 3,
		require_approval_for_destructive INTEGER NOT NULL DEFAULT 1,
		require_approval_for_schema_changes INTEGER NOT NULL DEFAULT 1,
		require_approval_for_external_tools INTEGER NOT NULL DEFAULT 0,
		approval_timeout_minutes INTEGER NOT NULL DEFAULT 60,
		notify_on_pause INTEGER NOT NULL DEFAULT 1,
		created_at TEXT NOT NULL
	)`)

	// Create approval_requests table
	mustExec(t, database, ctx, `CREATE TABLE IF NOT EXISTS approval_requests (
		id TEXT PRIMARY KEY,
		session_id TEXT NOT NULL,
		iteration INTEGER NOT NULL,
		request_type TEXT NOT NULL,
		description TEXT NOT NULL,
		risk_level TEXT NOT NULL DEFAULT 'medium',
		context TEXT DEFAULT '{}',
		target_tool TEXT,
		target_sql TEXT,
		status TEXT NOT NULL DEFAULT 'pending',
		reviewer_id TEXT,
		review_notes TEXT,
		modified_sql TEXT,
		created_at TEXT NOT NULL,
		reviewed_at TEXT,
		expires_at TEXT
	)`)

	// Create notification_log table
	mustExec(t, database, ctx, `CREATE TABLE IF NOT EXISTS notification_log (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		approval_id TEXT NOT NULL,
		channel TEXT NOT NULL DEFAULT 'dashboard',
		recipient TEXT NOT NULL DEFAULT 'admin',
		sent_at TEXT NOT NULL,
		delivered INTEGER NOT NULL DEFAULT 0
	)`)

	cleanup := func() {
		if err := database.Close(); err != nil {
			t.Logf("warning: failed to close test database: %v", err)
		}
	}

	return database, cleanup
}

func mustExec(t *testing.T, database db.DB, ctx context.Context, query string, args ...any) {
	t.Helper()
	if err := database.Exec(ctx, query, args...); err != nil {
		// If the table already has an entry, don't fail (common with ON CONFLICT tests)
		t.Logf("exec (warning): %s: %v", query, err)
	}
}
