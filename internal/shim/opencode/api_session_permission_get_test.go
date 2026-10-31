package opencode

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/wojons/consensus/internal/db"
)

func TestApiSessionPermissionGetDeclaredArms(t *testing.T) {
	const sessionID = "ses_permission"
	const requestID = "per_request"
	contextJSON := `{"protocol_id":"per_request","action":"run","resources":["tool:git"],"save":["always"],"metadata":{"source":"test"}}`

	tests := []struct {
		name string
		db   *mockDB
		path string
		want int
	}{
		{
			name: "blank session",
			db:   &mockDB{},
			path: "/api/session/%20/permission/per_request",
			want: http.StatusBadRequest,
		},
		{
			name: "invalid request id pattern",
			db:   &mockDB{},
			path: "/api/session/ses_permission/permission/not-permission",
			want: http.StatusBadRequest,
		},
		{
			name: "unknown session",
			db:   &mockDB{queryRowErr: errors.New("sqlite: no rows in result")},
			path: "/api/session/ses_missing/permission/per_request",
			want: http.StatusNotFound,
		},
		{
			name: "unknown permission",
			db: &mockDB{
				queryRow:     rowOf(map[string]any{"id": sessionID}),
				queryResults: nil,
			},
			path: "/api/session/ses_permission/permission/per_missing",
			want: http.StatusNotFound,
		},
		{
			name: "happy path",
			db: &mockDB{
				queryRow: rowOf(map[string]any{"id": sessionID}),
				queryResults: []db.Row{rowOf(map[string]any{
					"id":         "native-approval-id",
					"session_id": sessionID,
					"context":    contextJSON,
					"status":     "pending",
				})},
			},
			path: "/api/session/ses_permission/permission/per_request",
			want: http.StatusOK,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, srv := newTestServer(tc.db)
			defer srv.Close()

			status, _, body := doShimRequest(t, srv.URL, http.MethodGet, tc.path)
			if status != tc.want {
				t.Fatalf("GET %s: got %d, want %d; body=%s", tc.path, status, tc.want, body)
			}
			if status != http.StatusOK {
				return
			}

			var got struct {
				Data map[string]any `json:"data"`
			}
			if err := json.Unmarshal(body, &got); err != nil {
				t.Fatalf("decode 200: %v; body=%s", err, body)
			}
			if got.Data["id"] != "per_request" || got.Data["sessionID"] != sessionID || got.Data["action"] != "run" {
				t.Fatalf("data = %#v, want protocol-shaped permission request", got.Data)
			}
			if resources, ok := got.Data["resources"].([]any); !ok || len(resources) != 1 || resources[0] != "tool:git" {
				t.Fatalf("resources = %#v, want [tool:git]", got.Data["resources"])
			}
		})
	}
}

func TestApiSessionPermissionGetRequiresAuthAndMount(t *testing.T) {
	if !isApiSessionPermissionGetPath("/api/session/ses_permission/permission/per_request") {
		t.Fatal("permission detail path is not recognized")
	}
	if isApiSessionPermissionGetPath("/api/session/ses_permission/permission/per_request/deeper") {
		t.Fatal("deeper permission path was recognized")
	}
	if !isV2EnvelopeRoutePath("/api/session/ses_permission/permission/per_request") {
		t.Fatal("permission detail path is not admitted to the v2 401 envelope")
	}
	mounted := false
	for _, pattern := range MountPatterns {
		if pattern == "/api/session/{sessionID}/permission/{requestID}" {
			mounted = true
			break
		}
	}
	if !mounted {
		t.Fatal("permission detail path is not in MountPatterns")
	}

	s := NewServer(&mockDB{}, "test-key", nil, nil)
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()
	status, _, body := doShimRequest(t, srv.URL, http.MethodGet, "/api/session/ses_permission/permission/per_request")
	if status != http.StatusUnauthorized {
		t.Fatalf("unauthenticated GET: got %d, want 401; body=%s", status, body)
	}
}
