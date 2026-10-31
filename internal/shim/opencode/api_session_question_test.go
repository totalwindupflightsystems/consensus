package opencode

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/wojons/consensus/internal/db"
)

const apiSessionQuestionID = "ses_question_055"

func TestApiSessionQuestionListDeclaredArms(t *testing.T) {
	cases := []struct {
		name string
		db   *mockDB
		path string
		want int
		tag  string
	}{
		{"blank session", &mockDB{}, "/api/session/%20/question", http.StatusBadRequest, "InvalidRequestError"},
		{"unknown session", &mockDB{queryRowErr: errors.New("sqlite: no rows in result")}, "/api/session/ses_missing/question", http.StatusNotFound, "SessionNotFoundError"},
		{"store error", &mockDB{queryRowErr: errors.New("disk I/O error")}, "/api/session/ses_store/question", http.StatusBadRequest, "InvalidRequestError"},
		{"known session", &mockDB{queryRow: rowOf(map[string]any{"id": apiSessionQuestionID})}, "/api/session/" + apiSessionQuestionID + "/question", http.StatusOK, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, srv := newTestServer(tc.db)
			defer srv.Close()

			status, _, body := doShimRequest(t, srv.URL, http.MethodGet, tc.path)
			if status != tc.want {
				t.Fatalf("GET %s: got %d, want %d; body=%s", tc.path, status, tc.want, body)
			}
			var got map[string]any
			if err := json.Unmarshal(body, &got); err != nil {
				t.Fatalf("decode response: %v (%s)", err, body)
			}
			if tc.tag != "" {
				if got["_tag"] != tc.tag {
					t.Fatalf("_tag = %v, want %s", got["_tag"], tc.tag)
				}
				return
			}
			data, ok := got["data"].([]any)
			if !ok || data == nil || len(data) != 0 {
				t.Fatalf("data = %#v, want non-null empty array", got["data"])
			}
		})
	}
}

func TestApiSessionQuestionListReadsSessionApprovalStore(t *testing.T) {
	mdb := &mockDB{
		queryRow:     rowOf(map[string]any{"id": apiSessionQuestionID}),
		queryResults: []db.Row{{}},
	}
	_, srv := newTestServer(mdb)
	defer srv.Close()

	status, _, body := doShimRequest(t, srv.URL, http.MethodGet,
		"/api/session/"+apiSessionQuestionID+"/question")
	if status != http.StatusOK {
		t.Fatalf("GET session question: got %d, want 200; body=%s", status, body)
	}
	if !strings.Contains(string(body), `"data":[]`) {
		t.Fatalf("body = %s, want an empty data array", body)
	}
	if len(mdb.queries) != 2 || !strings.Contains(mdb.queries[0], "FROM sessions") || !strings.Contains(mdb.queries[1], "FROM approval_requests") {
		t.Fatalf("queries = %v, want session resolution followed by approval store read", mdb.queries)
	}
}

func TestApiSessionQuestionListAuthMountAndExactShape(t *testing.T) {
	if !isApiSessionQuestionPath("/api/session/ses_055/question") {
		t.Fatal("question path was not recognized")
	}
	for _, path := range []string{"/api/session/ses_055", "/api/session/ses_055/question/deeper", "/api/session//question"} {
		if isApiSessionQuestionPath(path) {
			t.Errorf("isApiSessionQuestionPath(%q) = true, want false", path)
		}
	}
	if !isV2EnvelopeRoutePath("/api/session/ses_055/question") {
		t.Fatal("question route is not admitted to v2 UnauthorizedError envelope")
	}
	mounted := false
	for _, pattern := range MountPatterns {
		if pattern == "/api/session/{sessionID}/question" {
			mounted = true
			break
		}
	}
	if !mounted {
		t.Fatal("question route is not registered in MountPatterns")
	}

	s := NewServer(&mockDB{}, "test-key", nil, nil)
	s.workdir = t.TempDir()
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()
	status, _, body := doShimRequest(t, srv.URL, http.MethodGet, "/api/session/ses_055/question")
	if status != http.StatusUnauthorized {
		t.Fatalf("unauthenticated GET: got %d, want 401; body=%s", status, body)
	}
	var got map[string]any
	if err := json.Unmarshal(body, &got); err != nil || got["_tag"] != "UnauthorizedError" {
		t.Fatalf("401 body = %s, want UnauthorizedError", body)
	}
}

func TestApiSessionQuestionListChiMountAndMethod(t *testing.T) {
	s := NewServer(&mockDB{queryRow: rowOf(map[string]any{"id": apiSessionQuestionID})}, "test-key", nil, nil)
	s.skipAuth = true
	router := chi.NewRouter()
	for _, pattern := range MountPatterns {
		router.Handle(pattern, s.Handler())
	}
	srv := httptest.NewServer(router)
	defer srv.Close()

	status, _, body := doShimRequest(t, srv.URL, http.MethodGet,
		"/api/session/"+apiSessionQuestionID+"/question")
	if status != http.StatusOK {
		t.Fatalf("chi-mounted GET: got %d, want 200; body=%s", status, body)
	}
	status, _, body = doShimRequest(t, srv.URL, http.MethodPost,
		"/api/session/"+apiSessionQuestionID+"/question")
	if status != http.StatusMethodNotAllowed || !strings.Contains(string(body), "METHOD_NOT_ALLOWED") {
		t.Fatalf("POST question: got %d %q, want 405 METHOD_NOT_ALLOWED", status, body)
	}
	status, _, _ = doShimRequest(t, srv.URL, http.MethodGet,
		"/api/session/"+apiSessionQuestionID+"/question/deeper")
	if status != http.StatusNotFound {
		t.Fatalf("GET deeper question path: got %d, want exact-route 404", status)
	}
}
