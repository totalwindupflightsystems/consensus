package opencode

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestApiSessionPermissionListDeclaredArms(t *testing.T) {
	cases := []struct {
		name string
		db   *mockDB
		path string
		want int
	}{
		{"blank session", &mockDB{}, "/api/session/%20/permission", http.StatusBadRequest},
		{"unknown session", &mockDB{queryRowErr: errors.New("sqlite: no rows in result")}, "/api/session/ses_missing/permission", http.StatusNotFound},
		{"store error", &mockDB{queryRowErr: errors.New("disk I/O error")}, "/api/session/ses_store/permission", http.StatusBadRequest},
		{"known session", &mockDB{queryRow: rowOf(map[string]any{"id": "ses_known"})}, "/api/session/ses_known/permission", http.StatusOK},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, srv := newTestServer(tc.db)
			defer srv.Close()
			status, _, body := doShimRequest(t, srv.URL, http.MethodGet, tc.path)
			if status != tc.want {
				t.Fatalf("GET %s: got %d, want %d; body=%s", tc.path, status, tc.want, body)
			}
			if status == http.StatusOK {
				var got map[string]any
				if err := json.Unmarshal(body, &got); err != nil {
					t.Fatalf("decode 200: %v", err)
				}
				data, ok := got["data"].([]any)
				if !ok || data == nil || len(data) != 0 {
					t.Fatalf("data = %#v, want non-null empty array", got["data"])
				}
			}
		})
	}
}

func TestApiSessionPermissionCreateDeclaredArms(t *testing.T) {
	cases := []struct {
		name string
		db   *mockDB
		path string
		body string
		want int
	}{
		{"malformed body", &mockDB{}, "/api/session/ses_create/permission", "{", http.StatusBadRequest},
		{"missing action", &mockDB{}, "/api/session/ses_create/permission", `{"resources":[]}`, http.StatusBadRequest},
		{"unknown session", &mockDB{queryRowErr: errors.New("sqlite: no rows in result")}, "/api/session/ses_missing/permission", `{"action":"run","resources":["tool"]}`, http.StatusNotFound},
		{"store error", &mockDB{queryRowErr: errors.New("disk I/O error")}, "/api/session/ses_store/permission", `{"action":"run","resources":["tool"]}`, http.StatusBadRequest},
		{"known session", &mockDB{queryRow: rowOf(map[string]any{"id": "ses_create"})}, "/api/session/ses_create/permission", `{"id":"per_client","action":"run","resources":["tool"]}`, http.StatusOK},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, srv := newTestServer(tc.db)
			defer srv.Close()
			status, _, body := doShimRequestWithBody(t, srv.URL, http.MethodPost, tc.path, tc.body)
			if status != tc.want {
				t.Fatalf("POST %s: got %d, want %d; body=%s", tc.path, status, tc.want, body)
			}
			if status == http.StatusOK {
				var got struct {
					Data struct {
						ID     string `json:"id"`
						Effect string `json:"effect"`
					} `json:"data"`
				}
				if err := json.Unmarshal(body, &got); err != nil {
					t.Fatalf("decode 200: %v", err)
				}
				if got.Data.ID != "per_client" || got.Data.Effect != "ask" {
					t.Fatalf("data = %#v, want per_client/ask", got.Data)
				}
			}
		})
	}
}

func TestApiSessionPermissionRequiresAuthAndDispatchesMethods(t *testing.T) {
	s := NewServer(&mockDB{}, "test-key", nil, nil)
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()
	status, _, body := doShimRequest(t, srv.URL, http.MethodGet, "/api/session/ses_auth/permission")
	if status != http.StatusUnauthorized {
		t.Fatalf("unauthenticated GET: got %d, want 401; body=%s", status, body)
	}
	var unauthorized map[string]any
	if err := json.Unmarshal(body, &unauthorized); err != nil || unauthorized["_tag"] != "UnauthorizedError" {
		t.Fatalf("401 body = %s, want v2 UnauthorizedError", body)
	}

	_, srv = newTestServer(&mockDB{})
	defer srv.Close()
	status, _, body = doShimRequest(t, srv.URL, http.MethodPut, "/api/session/ses_auth/permission")
	if status != http.StatusMethodNotAllowed || !strings.Contains(string(body), "METHOD_NOT_ALLOWED") {
		t.Fatalf("undeclared PUT: got %d body=%s, want 405 METHOD_NOT_ALLOWED", status, body)
	}
}

func TestIsApiSessionPermissionPath(t *testing.T) {
	for path, want := range map[string]bool{
		"/api/session/ses/permission":      true,
		"/api/session//permission":         false,
		"/api/session/ses/permission/deep": false,
		"/api/session/ses/model":           false,
	} {
		if got := isApiSessionPermissionPath(path); got != want {
			t.Errorf("isApiSessionPermissionPath(%q) = %v, want %v", path, got, want)
		}
	}
}
