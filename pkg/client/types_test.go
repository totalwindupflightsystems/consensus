package client

import (
	"encoding/json"
	"reflect"
	"testing"
	"time"
)

func ptr[T any](v T) *T { return &v }

// jsonRoundTrip marshals v to JSON, unmarshals into a fresh T, and asserts
// the two are deeply equal.  Reports via t.Error/t.Fatalf.
//
// NOTE: JSON numbers unmarshal into any/map[string]any as float64, so any
// numeric values in those fields must use float64 literals, not int.
func jsonRoundTrip[T any](t *testing.T, v T) {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}
	var got T
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal error: %v\nJSON: %s", err, string(data))
	}
	if !reflect.DeepEqual(v, got) {
		t.Errorf("round-trip mismatch\n  original: %#v\n  got:      %#v\n  JSON:     %s", v, got, string(data))
	}
}

// TestTypes_JSONRoundTrip exercises every struct in types.go through a
// JSON marshal / unmarshal / compare cycle.
func TestTypes_JSONRoundTrip(t *testing.T) {
	t.Run("CreateSessionRequest", func(t *testing.T) {
		jsonRoundTrip(t, CreateSessionRequest{
			AgentName:     "test-agent",
			Goal:          "test goal",
			ModelID:       "model-1",
			ContextBudget: 100,
			HITLConfig:    map[string]any{"approval": "required"},
			ProjectID:     "proj-1",
		})
	})

	t.Run("CreateSessionResponse", func(t *testing.T) {
		original := CreateSessionResponse{
			ID:        "session-1",
			Status:    "active",
			APIKey:    "key-1",
			ModelID:   "model-1",
			ProjectID: "proj-1",
			CreatedAt: time.Date(2026, 7, 15, 12, 0, 5, 0, time.UTC),
		}
		data, err := json.Marshal(original)
		if err != nil {
			t.Fatalf("marshal error: %v", err)
		}
		var got CreateSessionResponse
		if err := json.Unmarshal(data, &got); err != nil {
			t.Fatalf("unmarshal error: %v\nJSON: %s", err, string(data))
		}
		// time.Time loses sub-second / location precision through JSON RFC3339.
		original.CreatedAt = original.CreatedAt.Truncate(time.Second)
		if !reflect.DeepEqual(original, got) {
			t.Errorf("round-trip mismatch\n  original: %#v\n  got:      %#v\n  JSON:     %s", original, got, string(data))
		}
	})

	t.Run("SessionResponse", func(t *testing.T) {
		jsonRoundTrip(t, SessionResponse{
			ID:            "session-1",
			ParentID:      ptr("parent-1"),
			AgentName:     "agent",
			ModelID:       "model-1",
			Status:        "active",
			Goal:          ptr("test goal"),
			ContextBudget: 100,
			TokensUsedIn:  10,
			TokensUsedOut: 20,
			Iteration:     3,
			ProjectID:     nil, // test nil survives omitempty
			HeartbeatAt:   "2026-07-15T12:00:00Z",
			CreatedAt:     "2026-07-15T12:00:00Z",
			CompletedAt:   nil, // test nil survives omitempty
		})
	})

	t.Run("UpdateSessionRequest", func(t *testing.T) {
		jsonRoundTrip(t, UpdateSessionRequest{
			Status: ptr("pause"),
		})
	})

	t.Run("SendMessageRequest", func(t *testing.T) {
		jsonRoundTrip(t, SendMessageRequest{
			Content: "hello",
			Type:    "user_instruction",
		})
	})

	t.Run("ToolResponse", func(t *testing.T) {
		jsonRoundTrip(t, ToolResponse{
			ID:               "tool-1",
			Name:             "my-tool",
			Description:      "a useful tool",
			Hemisphere:       "left",
			HandlerType:      "http",
			Status:           "active",
			Enabled:          true,
			RequiresApproval: false,
		})
	})

	t.Run("SkillResponse", func(t *testing.T) {
		jsonRoundTrip(t, SkillResponse{
			ID:   "skill-1",
			Name: "my-skill",
			Metadata: map[string]any{
				"version": float64(2),
				"author":  "test",
			},
			Enabled: true,
		})
	})

	t.Run("SkillDetailResponse", func(t *testing.T) {
		jsonRoundTrip(t, SkillDetailResponse{
			ID:            "skill-2",
			Name:          "skill-detail",
			Metadata:      "simple-string-metadata",
			Instructions:  "do the thing",
			LinkedToolIDs: []string{"t1", "t2"},
			Enabled:       false,
		})
	})

	t.Run("ExecuteToolRequest", func(t *testing.T) {
		jsonRoundTrip(t, ExecuteToolRequest{
			SessionID: "session-1",
			Parameters: map[string]any{
				"query":   "SELECT 1",
				"timeout": float64(30),
			},
		})
	})

	t.Run("ExecuteToolResponse", func(t *testing.T) {
		jsonRoundTrip(t, ExecuteToolResponse{
			ToolName:     "query-tool",
			Result:       map[string]any{"rows": []any{"a", "b"}},
			RowsAffected: 5,
			IsError:      false,
			Error:        "",
		})
	})

	t.Run("ConfigResponse", func(t *testing.T) {
		jsonRoundTrip(t, ConfigResponse{
			LLM:      map[string]any{"model": "gpt-4", "temperature": 0.7},
			HITL:     map[string]any{"enabled": true},
			Harness:  map[string]any{"type": "test", "timeout": float64(30)},
			Database: map[string]any{"dsn": ":memory:"},
			Logging:  map[string]any{"level": "info"},
			SystemSettings: map[string]any{
				"max_retries": float64(3),
			},
		})
	})

	t.Run("MetricsResponse", func(t *testing.T) {
		jsonRoundTrip(t, MetricsResponse{
			ActiveSessions:   3,
			PendingTasks:     5,
			PendingApprovals: 2,
			TotalSessions:    100,
			TotalCostUSD:     42.5,
		})
	})

	t.Run("MemoryEventResponse", func(t *testing.T) {
		jsonRoundTrip(t, MemoryEventResponse{
			ID:               1,
			Type:             "thought",
			Content:          "memory content",
			SummaryText:      ptr("brief summary"),
			SessionID:        "session-1",
			IterationCreated: 2,
			DisplayMode:      "full",
			CreatedAt:        "2026-07-15T12:00:00Z",
		})
	})

	t.Run("ActiveContextResponse", func(t *testing.T) {
		jsonRoundTrip(t, ActiveContextResponse{
			ID:               42,
			IterationCreated: 3,
			Type:             "thought",
			DisplayMode:      "full",
			RenderedText:     nil,
		})
	})

	t.Run("IterationCommitResponse", func(t *testing.T) {
		jsonRoundTrip(t, IterationCommitResponse{
			IterationID:    1,
			SessionID:      "session-1",
			ActivePointers: []int64{10, 20, 30},
			DisplayRules: map[string]any{
				"max_items": float64(5),
				"sort":      "desc",
			},
			LLMResponse: map[string]any{
				"content": "ok",
				"tokens":  float64(150),
			},
			SQLExecuted:  []string{"SELECT 1", "SELECT 2"},
			RowsAffected: 10,
			CreatedAt:    "2026-07-15T12:00:00Z",
		})
	})

	t.Run("CreateTaskRequest", func(t *testing.T) {
		jsonRoundTrip(t, CreateTaskRequest{
			Title:           "task-title",
			Description:     "task description",
			Priority:        5,
			PrerequisiteIDs: []string{"pre-1", "pre-2"},
		})
	})

	t.Run("TaskResponse", func(t *testing.T) {
		jsonRoundTrip(t, TaskResponse{
			ID:              "task-1",
			SessionID:       "session-1",
			ParentTaskID:    ptr("parent-1"),
			Title:           "task-title",
			Description:     ptr("detailed description"),
			Status:          "pending",
			Priority:        1,
			LockedByAgent:   nil,
			PrerequisiteIDs: []string{"pre-1"},
			ResultMemoryID:  ptr(int64(100)),
			CreatedAt:       "2026-07-15T12:00:00Z",
			ClaimedAt:       nil,
			CompletedAt:     nil,
		})
	})

	t.Run("UpdateTaskRequest", func(t *testing.T) {
		jsonRoundTrip(t, UpdateTaskRequest{
			Status: ptr("in_progress"),
		})
	})

	t.Run("HealthResponse", func(t *testing.T) {
		jsonRoundTrip(t, HealthResponse{
			Status:        "healthy",
			Version:       "1.0.0",
			UptimeSeconds: 42,
			APILatencyMs:  1.5,
			DBLatencyMs:   2.5,
			LLMLatencyMs:  3.5,
			ErrorRatePct:  0.25,
			DBBackend:     "sqlite",
			DBPath:        "/tmp/consensus.db",
			DBSizeMB:      4.5,
			DBTables:      12,
			DBMigrations:  3,
			ActiveConnections: ActiveConnections{
				WebSocket:          1,
				DBPoolActive:       2,
				DBPoolMax:          5,
				LLMActive:          3,
				APIRequestsLastMin: 10,
			},
			SystemLog: []string{"started", "ready"},
		})
	})

	t.Run("ActiveConnections", func(t *testing.T) {
		jsonRoundTrip(t, ActiveConnections{
			WebSocket:          1,
			DBPoolActive:       2,
			DBPoolMax:          5,
			LLMActive:          3,
			APIRequestsLastMin: 10,
		})
	})

	t.Run("ApprovalResponse", func(t *testing.T) {
		jsonRoundTrip(t, ApprovalResponse{
			ID:          "apr-1",
			SessionID:   "session-1",
			Iteration:   2,
			RequestType: "tool_exec",
			Description: "Approve tool execution?",
			RiskLevel:   "medium",
			Context:     map[string]any{"tool": "query", "args": "SELECT 1"},
			TargetTool:  nil,
			TargetSQL:   ptr("SELECT 1"),
			Status:      "pending",
			ReviewerID:  nil,
			ReviewNotes: nil,
			ModifiedSQL: nil,
			CreatedAt:   "2026-07-15T12:00:00Z",
			ReviewedAt:  nil,
			ExpiresAt:   nil,
		})
	})

	t.Run("ApprovalReviewRequest", func(t *testing.T) {
		jsonRoundTrip(t, ApprovalReviewRequest{
			Decision:    "approved",
			Notes:       "looks good",
			ModifiedSQL: "SELECT 1",
		})
	})
}
