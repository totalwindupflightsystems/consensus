package opencode

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
)

const apiSessionPermissionReplySessionID = "ses_reply"
const apiSessionPermissionReplyRequestID = "per_reply"

func TestApiSessionPermissionReplyDeclaredArms(t *testing.T) {
	tests := []struct {
		name string
		db   *mockDB
		path string
		body string
		want int
	}{
		{
			name: "happy path",
			db: &mockDB{queryRow: rowOf(map[string]any{
				"session_id": apiSessionPermissionReplySessionID,
				"status":     "pending",
			})},
			path: "/api/session/" + apiSessionPermissionReplySessionID + "/permission/" + apiSessionPermissionReplyRequestID + "/reply",
			body: `{"reply":"once","message":"approved for this turn"}`,
			want: http.StatusNoContent,
		},
		{
			name: "malformed body",
			db:   &mockDB{},
			path: "/api/session/" + apiSessionPermissionReplySessionID + "/permission/" + apiSessionPermissionReplyRequestID + "/reply",
			body: "{",
			want: http.StatusBadRequest,
		},
		{
			name: "unknown permission",
			db:   &mockDB{queryRowErr: errors.New("sqlite: no rows in result")},
			path: "/api/session/" + apiSessionPermissionReplySessionID + "/permission/per_missing/reply",
			body: `{"reply":"always"}`,
			want: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, srv := newTestServer(tt.db)
			defer srv.Close()
			status, header, body := doShimRequestBody(t, srv.URL, http.MethodPost, tt.path, tt.body)
			if status != tt.want {
				t.Fatalf("POST %s: got %d, want %d; body=%s", tt.path, status, tt.want, body)
			}
			switch status {
			case http.StatusNoContent:
				if len(body) != 0 || header.Get("Content-Type") != "" {
					t.Fatalf("204 response: body=%q content-type=%q, want both empty", body, header.Get("Content-Type"))
				}
			case http.StatusNotFound:
				var got map[string]any
				if err := json.Unmarshal([]byte(body), &got); err != nil {
					t.Fatalf("decode 404 body: %v (%s)", err, body)
				}
				if got["_tag"] != "PermissionNotFoundError" || got["requestID"] != "per_missing" {
					t.Fatalf("404 body = %v, want PermissionNotFoundError for per_missing", got)
				}
			}
		})
	}
}

func TestApiSessionPermissionReplyMissingAuthAnswersDeclared401(t *testing.T) {
	s := NewServer(&mockDB{}, "test-key", nil, nil)
	s.workdir = t.TempDir()
	_, srv := newTestServerWithServer(t, s)
	defer srv.Close()

	status, header, body := doShimRequestBody(t, srv.URL, http.MethodPost,
		"/api/session/"+apiSessionPermissionReplySessionID+"/permission/"+apiSessionPermissionReplyRequestID+"/reply",
		`{"reply":"always"}`)
	if status != http.StatusUnauthorized {
		t.Fatalf("missing auth: got %d, want 401; body=%s", status, body)
	}
	if !strings.HasPrefix(header.Get("Content-Type"), "application/json") {
		t.Fatalf("401 Content-Type = %q, want application/json", header.Get("Content-Type"))
	}
	var got map[string]any
	if err := json.Unmarshal([]byte(body), &got); err != nil || got["_tag"] != "UnauthorizedError" {
		t.Fatalf("401 body = %s, want UnauthorizedError", body)
	}
}

func TestApiSessionPermissionReplyPathValidation(t *testing.T) {
	_, srv := newTestServer(&mockDB{})
	defer srv.Close()

	for name, path := range map[string]string{
		"blank session":   "/api/session/%20/permission/per_reply/reply",
		"invalid session": "/api/session/not-a-session/permission/per_reply/reply",
		"invalid request": "/api/session/ses_reply/permission/not-a-permission/reply",
	} {
		t.Run(name, func(t *testing.T) {
			status, _, body := doShimRequestBody(t, srv.URL, http.MethodPost, path, `{"reply":"once"}`)
			if status != http.StatusBadRequest {
				t.Fatalf("POST %s: got %d, want 400; body=%s", path, status, body)
			}
		})
	}
}

func TestApiSessionPermissionReplyMountAndMethodGuard(t *testing.T) {
	s := NewServer(&mockDB{queryRow: rowOf(map[string]any{
		"session_id": apiSessionPermissionReplySessionID,
		"status":     "pending",
	})}, "test-key", nil, nil)
	s.skipAuth = true
	r := chi.NewRouter()
	for _, pattern := range MountPatterns {
		r.Handle(pattern, s.Handler())
	}
	srv := httptest.NewServer(r)
	defer srv.Close()

	path := "/api/session/" + apiSessionPermissionReplySessionID + "/permission/" + apiSessionPermissionReplyRequestID + "/reply"
	status, _, body := doShimRequestBody(t, srv.URL, http.MethodPost, path, `{"reply":"always"}`)
	if status != http.StatusNoContent || len(body) != 0 {
		t.Fatalf("mounted POST: got %d body=%q, want 204 empty", status, body)
	}
	status, _, _ = doShimRequest(t, srv.URL, http.MethodGet, path)
	if status != http.StatusMethodNotAllowed {
		t.Fatalf("mounted GET: got %d, want 405", status)
	}
}

func TestIsApiSessionPermissionReplyPath(t *testing.T) {
	for path, want := range map[string]bool{
		"/api/session/ses/permission/per/reply":        true,
		"/api/session//permission/per/reply":           false,
		"/api/session/ses/permission//reply":           false,
		"/api/session/ses/permission/per/reply/deeper": false,
		"/api/session/ses/permission/per":              false,
	} {
		if got := isApiSessionPermissionReplyPath(path); got != want {
			t.Errorf("isApiSessionPermissionReplyPath(%q) = %v, want %v", path, got, want)
		}
	}
}
