package opencode

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"
)

const apiSessionPromptID = "ses_prompt_054"

func TestApiSessionPromptDeclaredArms(t *testing.T) {
	cases := []struct {
		name       string
		mdb        *mockDB
		body       string
		wantStatus int
		wantTag    string
	}{
		{"happy path", &mockDB{queryRow: rowOf(map[string]any{"status": "idle", "iteration": int64(2)})}, `{"prompt":{"text":"ship it"}}`, http.StatusOK, ""},
		{"invalid body", &mockDB{queryRow: rowOf(map[string]any{"status": "idle"})}, `{}`, http.StatusBadRequest, "InvalidRequestError"},
		{"unknown session", &mockDB{queryRowErr: errors.New("sqlite: no rows in result")}, `{"prompt":{"text":"ship it"}}`, http.StatusNotFound, "SessionNotFoundError"},
		{"busy session", &mockDB{queryRow: rowOf(map[string]any{"status": "thinking", "iteration": int64(2)})}, `{"prompt":{"text":"ship it"}}`, http.StatusConflict, "SessionBusyError"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, srv := newTestServer(tc.mdb)
			defer srv.Close()
			status, _, body := doShimRequestWithBody(t, srv.URL, http.MethodPost, "/api/session/"+apiSessionPromptID+"/prompt", tc.body)
			if status != tc.wantStatus {
				t.Fatalf("got %d, want %d. Body: %s", status, tc.wantStatus, body)
			}
			if tc.wantTag == "" {
				var got struct {
					Data struct{ ID, SessionID, Delivery string } `json:"data"`
				}
				if err := json.Unmarshal(body, &got); err != nil {
					t.Fatalf("decode success: %v (%s)", err, body)
				}
				if got.Data.ID == "" || got.Data.SessionID != apiSessionPromptID || got.Data.Delivery != "queue" {
					t.Errorf("success data = %#v", got.Data)
				}
				return
			}
			var got map[string]any
			if err := json.Unmarshal(body, &got); err != nil {
				t.Fatalf("decode error: %v (%s)", err, body)
			}
			if got["_tag"] != tc.wantTag {
				t.Errorf("_tag = %v, want %s", got["_tag"], tc.wantTag)
			}
		})
	}
}

func TestApiSessionPromptValidatesDeclaredBody(t *testing.T) {
	_, srv := newTestServer(&mockDB{queryRow: rowOf(map[string]any{"status": "idle"})})
	defer srv.Close()
	for _, body := range []string{`{"prompt":{"text":" "}}`, `{"prompt":{"text":"ok"},"delivery":"nope"}`, `{"id":"msg-nope","prompt":{"text":"ok"}}`} {
		status, _, response := doShimRequestWithBody(t, srv.URL, http.MethodPost, "/api/session/"+apiSessionPromptID+"/prompt", body)
		if status != http.StatusBadRequest || !strings.Contains(string(response), "InvalidRequestError") {
			t.Errorf("body %s: got %d %s", body, status, response)
		}
	}
}

func TestApiSessionPromptUnauthenticatedAndMount(t *testing.T) {
	s := NewServer(&mockDB{}, "test-key", nil, nil)
	s.workdir = t.TempDir()
	_, srv := newTestServerWithServer(t, s)
	status, _, body := doShimRequestWithBody(t, srv.URL, http.MethodPost, "/api/session/ses_abc/prompt", `{"prompt":{"text":"ok"}}`)
	if status != http.StatusUnauthorized {
		t.Fatalf("got %d, want 401: %s", status, body)
	}
	var got map[string]any
	if err := json.Unmarshal(body, &got); err != nil || got["_tag"] != "UnauthorizedError" {
		t.Fatalf("body = %s", body)
	}
	found := false
	for _, pattern := range MountPatterns {
		if pattern == "/api/session/{sessionID}/prompt" {
			found = true
		}
	}
	if !found {
		t.Fatal("prompt route missing from MountPatterns")
	}
	if !isApiSessionPromptPath("/api/session/ses_abc/prompt") || isApiSessionPromptPath("/api/session/ses_abc/prompt/deeper") {
		t.Fatal("prompt path matcher has wrong boundaries")
	}
}

func TestApiSessionPromptWrongMethod(t *testing.T) {
	_, srv := newTestServer(&mockDB{queryRow: rowOf(map[string]any{"status": "idle"})})
	defer srv.Close()
	status, _, _ := doShimRequest(t, srv.URL, http.MethodGet, "/api/session/"+apiSessionPromptID+"/prompt")
	if status != http.StatusMethodNotAllowed {
		t.Fatalf("got %d, want 405", status)
	}
}
