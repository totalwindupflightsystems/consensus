// Package api: DF-CONSENSUS-084 — the POST message response carries an
// explicit `queued` flag.
//
// A message POSTed to a BUSY session (thinking / planning / tool_exec) cannot
// be answered by the in-flight turn (its context snapshot predates the
// insert); the harness requeues the session for a follow-up turn once the
// in-flight one settles. The response used to return only
// message_received, implying the message was already being answered — the
// flag makes the queueing explicit, per the acceptance criterion that a raced
// message must never vanish silently.
package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// postDF084Message posts a message and decodes the standard response body.
func postDF084Message(t *testing.T, is *integrationServer, sessionID, content string) map[string]any {
	t.Helper()
	body := `{"role":"user","content":"` + content + `"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/sessions/"+sessionID+"/message", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+is.adminKey)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	is.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("POST message: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v (body=%s)", err, w.Body.String())
	}
	return resp
}

func insertDF084Session(t *testing.T, is *integrationServer, id, status string) {
	t.Helper()
	if err := is.conn.Exec(context.Background(),
		`INSERT INTO sessions (id, agent_name, model_id, status, trust_level, goal) VALUES ($1, 'df084-agent', 'gpt-4o', $2, 'high', 'queued-flag test')`,
		id, status); err != nil {
		t.Fatalf("insert session (status=%s): %v", status, err)
	}
}

// TestSessionMessageQueuedFalseWhenWoken pins the woke arm: an idle session
// is flipped to thinking by this send, so queued is false.
func TestSessionMessageQueuedFalseWhenWoken(t *testing.T) {
	is := newIntegrationServer(t)
	defer is.close()

	sessionID := "08400000-0000-0000-0000-000000000001"
	insertDF084Session(t, is, sessionID, "idle")

	resp := postDF084Message(t, is, sessionID, "wake me")
	if resp["status"] != "message_received" {
		t.Fatalf("status = %v, want message_received", resp["status"])
	}
	if queued, ok := resp["queued"].(bool); !ok || queued {
		t.Errorf("queued = %v, want false (the send woke the session)", resp["queued"])
	}
}

// TestSessionMessageQueuedTrueWhenBusy pins the queue arm: a thinking session
// (a turn in flight) stores the message without waking — queued must be true
// so the client knows the answer comes from a LATER turn.
func TestSessionMessageQueuedTrueWhenBusy(t *testing.T) {
	is := newIntegrationServer(t)
	defer is.close()

	for _, status := range []string{"thinking", "planning", "tool_exec", "paused"} {
		t.Run(status, func(t *testing.T) {
			sessionID := "08400000-0000-0000-0000-00000000000" + map[string]string{
				"thinking": "2", "planning": "3", "tool_exec": "4", "paused": "5",
			}[status]
			insertDF084Session(t, is, sessionID, status)

			resp := postDF084Message(t, is, sessionID, "message during "+status)
			if resp["status"] != "message_received" {
				t.Fatalf("status = %v, want message_received", resp["status"])
			}
			if queued, ok := resp["queued"].(bool); !ok || !queued {
				t.Errorf("session status %s: queued = %v, want true (in-flight turn cannot see this message)", status, resp["queued"])
			}
		})
	}
}
