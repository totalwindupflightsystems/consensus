// Package api: integration tests for the PATCH /api/v1/sessions/{id}
// planning_max_turns surface (DF-CONSENSUS-083).
//
// planning_max_turns is the session's PER-PLANNING-RUN turn budget
// (sessions.planning_max_turns). The PATCH endpoint used to accept ONLY
// `status` — a body of {"planning_max_turns":50} returned
// INVALID_REQUEST "status field is required" — even though the column existed,
// leaving a budget-exhausted session with no remedy. These tests prove the
// knob is reachable: settings-only PATCH persists it (in any session state,
// failed included), status+settings PATCH applies both, validation rejects
// nonsense, and GET surfaces the stored value.
package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// df083Patch sends a PATCH against the integration server and decodes the
// response body as a JSON object.
func df083Patch(t *testing.T, srv *integrationServer, id, body string) (*httptest.ResponseRecorder, map[string]any) {
	t.Helper()
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/sessions/"+id, strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+srv.adminKey)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)
	var resp map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	return w, resp
}

func df083Get(t *testing.T, srv *integrationServer, id string) map[string]any {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/sessions/"+id, nil)
	req.Header.Set("Authorization", "Bearer "+srv.adminKey)
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("get session: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("get session: decode: %v", err)
	}
	return resp
}

func df083RowPlanningMaxTurns(t *testing.T, srv *integrationServer, id string) int {
	t.Helper()
	row, err := srv.conn.QueryRow(context.Background(),
		`SELECT planning_max_turns FROM sessions WHERE id = $1`, id)
	if err != nil || row == nil {
		t.Fatalf("read back session row: err=%v row=%v", err, row)
	}
	return toInt(row["planning_max_turns"])
}

// TestPATCHSession_PlanningMaxTurns_PersistedAndSurfaced is the
// DF-CONSENSUS-083 acceptance cell: a settings-only PATCH persists the
// per-run turn budget and the API surfaces it.
func TestPATCHSession_PlanningMaxTurns_PersistedAndSurfaced(t *testing.T) {
	srv := newIntegrationServer(t)
	defer srv.close()

	w, resp := createSessionWithBudget(t, srv,
		`{"agent_name":"df083-budget","goal":"prove the planning turn budget knob is patchable"}`)
	if w.Code != http.StatusCreated && w.Code != http.StatusOK {
		t.Fatalf("create session: expected 2xx, got %d: %s", w.Code, w.Body.String())
	}
	id, _ := resp["id"].(string)
	if id == "" {
		t.Fatalf("create session: no id in response: %s", w.Body.String())
	}

	patch, resp := df083Patch(t, srv, id, `{"planning_max_turns":50}`)
	if patch.Code != http.StatusOK {
		t.Fatalf("patch planning_max_turns: expected 200, got %d: %s", patch.Code, patch.Body.String())
	}

	// (a) The PATCH response echoes the new budget.
	if v, _ := resp["planning_max_turns"].(float64); v != 50 {
		t.Fatalf("patch response planning_max_turns=%v, want 50: %s", resp["planning_max_turns"], patch.Body.String())
	}
	// (b) The row carries it.
	if got := df083RowPlanningMaxTurns(t, srv, id); got != 50 {
		t.Fatalf("sessions.planning_max_turns=%d, want 50", got)
	}
	// (c) GET surfaces it, and no status action happened.
	got := df083Get(t, srv, id)
	if v, _ := got["planning_max_turns"].(float64); v != 50 {
		t.Fatalf("GET planning_max_turns=%v, want 50: %v", got["planning_max_turns"], got)
	}
}

// TestPATCHSession_PlanningMaxTurns_OnFailedSession pins the operator remedy:
// a budget-exhausted session goes status=failed, and a settings-only PATCH
// must still work on it (raise the budget, then resume).
func TestPATCHSession_PlanningMaxTurns_OnFailedSession(t *testing.T) {
	srv := newIntegrationServer(t)
	defer srv.close()

	w, resp := createSessionWithBudget(t, srv,
		`{"agent_name":"df083-failed","goal":"raise the budget after a failure"}`)
	id, _ := resp["id"].(string)
	if id == "" {
		t.Fatalf("create session: no id in response: %s", w.Body.String())
	}

	// Force the session into the state a budget exhaustion produces.
	if err := srv.conn.Exec(context.Background(),
		`UPDATE sessions SET status = 'failed' WHERE id = $1`, id); err != nil {
		t.Fatalf("set session failed: %v", err)
	}

	patch, resp := df083Patch(t, srv, id, `{"planning_max_turns":25}`)
	if patch.Code != http.StatusOK {
		t.Fatalf("patch planning_max_turns on failed session: expected 200, got %d: %s", patch.Code, patch.Body.String())
	}
	if got, _ := resp["status"].(string); got != "failed" {
		t.Fatalf("patch response status=%q, want failed (settings-only PATCH must not transition)", got)
	}
	if v, _ := resp["planning_max_turns"].(float64); v != 25 {
		t.Fatalf("patch response planning_max_turns=%v, want 25", resp["planning_max_turns"])
	}
	if got := df083RowPlanningMaxTurns(t, srv, id); got != 25 {
		t.Fatalf("sessions.planning_max_turns=%d, want 25", got)
	}
}

// TestPATCHSession_StatusAndPlanningMaxTurns pins the combined body: the
// status action runs through its usual transition rules AND the turn budget
// lands in the same request.
func TestPATCHSession_StatusAndPlanningMaxTurns(t *testing.T) {
	srv := newIntegrationServer(t)
	defer srv.close()

	w, resp := createSessionWithBudget(t, srv,
		`{"agent_name":"df083-combined","goal":"cancel and raise in one PATCH"}`)
	id, _ := resp["id"].(string)
	if id == "" {
		t.Fatalf("create session: no id in response: %s", w.Body.String())
	}
	// cancel is only valid from active states; move booting → idle first.
	if err := srv.conn.Exec(context.Background(),
		`UPDATE sessions SET status = 'idle' WHERE id = $1`, id); err != nil {
		t.Fatalf("set session idle: %v", err)
	}

	patch, resp := df083Patch(t, srv, id, `{"status":"cancel","planning_max_turns":20}`)
	if patch.Code != http.StatusOK {
		t.Fatalf("patch status+planning_max_turns: expected 200, got %d: %s", patch.Code, patch.Body.String())
	}
	if got, _ := resp["status"].(string); got != "failed" {
		t.Fatalf("patch response status=%q, want failed (cancel)", got)
	}
	if v, _ := resp["planning_max_turns"].(float64); v != 20 {
		t.Fatalf("patch response planning_max_turns=%v, want 20", resp["planning_max_turns"])
	}
	if got := df083RowPlanningMaxTurns(t, srv, id); got != 20 {
		t.Fatalf("sessions.planning_max_turns=%d, want 20", got)
	}
}

// TestPATCHSession_PlanningMaxTurns_Validation pins the request contract:
// nonsense budgets and a body with neither field are rejected, and a
// status-only PATCH still leaves the stored budget untouched.
func TestPATCHSession_PlanningMaxTurns_Validation(t *testing.T) {
	srv := newIntegrationServer(t)
	defer srv.close()

	w, resp := createSessionWithBudget(t, srv,
		`{"agent_name":"df083-validation","goal":"reject bad PATCH bodies"}`)
	id, _ := resp["id"].(string)
	if id == "" {
		t.Fatalf("create session: no id in response: %s", w.Body.String())
	}

	// Seed a known budget so the "untouched" assertions have something to compare.
	if patch, _ := df083Patch(t, srv, id, `{"planning_max_turns":12}`); patch.Code != http.StatusOK {
		t.Fatalf("seed patch: expected 200, got %d: %s", patch.Code, patch.Body.String())
	}

	for _, tc := range []struct {
		name       string
		body       string
		wantStatus int
		wantErr    string
	}{
		{name: "zero budget", body: `{"planning_max_turns":0}`, wantStatus: http.StatusBadRequest, wantErr: "between 1 and 1000"},
		{name: "negative budget", body: `{"planning_max_turns":-3}`, wantStatus: http.StatusBadRequest, wantErr: "between 1 and 1000"},
		{name: "over the cap", body: `{"planning_max_turns":1001}`, wantStatus: http.StatusBadRequest, wantErr: "between 1 and 1000"},
		{name: "neither field", body: `{}`, wantStatus: http.StatusBadRequest, wantErr: "status or planning_max_turns field is required"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			patch, resp := df083Patch(t, srv, id, tc.body)
			if patch.Code != tc.wantStatus {
				t.Fatalf("got HTTP %d, want %d: %s", patch.Code, tc.wantStatus, patch.Body.String())
			}
			errObj, _ := resp["error"].(map[string]any)
			msg, _ := errObj["message"].(string)
			if !strings.Contains(msg, tc.wantErr) {
				t.Fatalf("error.message=%q, want it to name %q (body: %s)", msg, tc.wantErr, patch.Body.String())
			}
		})
	}

	// The rejected bodies changed nothing.
	if got := df083RowPlanningMaxTurns(t, srv, id); got != 12 {
		t.Fatalf("sessions.planning_max_turns=%d after rejected PATCHes, want 12 (untouched)", got)
	}
	// A status-only PATCH also leaves the budget alone.
	if patch, _ := df083Patch(t, srv, id, `{"status":"pause"}`); patch.Code != http.StatusOK && patch.Code != http.StatusConflict {
		t.Fatalf("status-only patch: unexpected HTTP %d: %s", patch.Code, patch.Body.String())
	}
	if got := df083RowPlanningMaxTurns(t, srv, id); got != 12 {
		t.Fatalf("sessions.planning_max_turns=%d after status-only PATCH, want 12 (untouched)", got)
	}
}
