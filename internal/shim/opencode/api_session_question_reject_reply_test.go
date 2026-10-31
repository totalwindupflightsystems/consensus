package opencode

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const apiSessionQuestionActionSessionID = "ses_question_action"

func TestApiSessionQuestionRejectReplyDeclaredArms(t *testing.T) {
	cases := []struct {
		name string
		path string
		body string
		db   *mockDB
		want int
		tag  string
	}{
		{"reject known session", "/api/session/" + apiSessionQuestionActionSessionID + "/question/que_reject/reject", "", &mockDB{queryRow: rowOf(map[string]any{"id": apiSessionQuestionActionSessionID})}, http.StatusNoContent, ""},
		{"reply known session", "/api/session/" + apiSessionQuestionActionSessionID + "/question/que_reply/reply", `{"answers":[["yes"]]}`, &mockDB{queryRow: rowOf(map[string]any{"id": apiSessionQuestionActionSessionID})}, http.StatusNoContent, ""},
		{"reject unknown session", "/api/session/ses_missing/question/que_reject/reject", "", &mockDB{queryRowErr: errors.New("sqlite: no rows in result")}, http.StatusNotFound, "SessionNotFoundError"},
		{"reply malformed body", "/api/session/" + apiSessionQuestionActionSessionID + "/question/que_reply/reply", `{}`, &mockDB{}, http.StatusBadRequest, "InvalidRequestError"},
		{"reject malformed request id", "/api/session/" + apiSessionQuestionActionSessionID + "/question/not-a-question/reject", "", &mockDB{}, http.StatusBadRequest, "InvalidRequestError"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, srv := newTestServer(tc.db)
			defer srv.Close()
			status, header, body := doShimRequestBody(t, srv.URL, http.MethodPost, tc.path, tc.body)
			if status != tc.want {
				t.Fatalf("POST %s: got %d, want %d; body=%s", tc.path, status, tc.want, body)
			}
			if tc.want == http.StatusNoContent {
				if len(body) != 0 || header.Get("Content-Type") != "" {
					t.Fatalf("204 response body=%q content-type=%q; want empty body and no content type", body, header.Get("Content-Type"))
				}
				return
			}
			var got map[string]any
			if err := json.Unmarshal(body, &got); err != nil {
				t.Fatalf("decode response: %v (%s)", err, body)
			}
			if got["_tag"] != tc.tag {
				t.Fatalf("_tag=%v, want %q; body=%s", got["_tag"], tc.tag, body)
			}
		})
	}
}

func TestApiSessionQuestionRejectReplyAuthMountAndExactShape(t *testing.T) {
	for _, tc := range []struct {
		path  string
		check func(string) bool
	}{
		{"/api/session/ses_question_action/question/que_1/reject", isApiSessionQuestionRejectPath},
		{"/api/session/ses_question_action/question/que_1/reply", isApiSessionQuestionReplyPath},
	} {
		if !tc.check(tc.path) || tc.check(tc.path+"/deeper") || tc.check("/api/session//question/que_1/reply") {
			t.Errorf("route shape check failed for %s", tc.path)
		}
		if !isV2EnvelopeRoutePath(tc.path) {
			t.Errorf("%s is not admitted to the v2 UnauthorizedError envelope", tc.path)
		}
	}
	for _, pattern := range []string{
		"/api/session/{sessionID}/question/{requestID}/reject",
		"/api/session/{sessionID}/question/{requestID}/reply",
	} {
		found := false
		for _, mounted := range MountPatterns {
			if mounted == pattern {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("%s is not in MountPatterns", pattern)
		}
	}

	s := NewServer(&mockDB{}, "test-key", nil, nil)
	s.workdir = t.TempDir()
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()
	for _, path := range []string{
		"/api/session/ses_question_action/question/que_1/reject",
		"/api/session/ses_question_action/question/que_1/reply",
	} {
		status, _, body := doShimRequest(t, srv.URL, http.MethodPost, path)
		if status != http.StatusUnauthorized || !strings.Contains(string(body), `"_tag":"UnauthorizedError"`) {
			t.Fatalf("unauthenticated POST %s: got %d body=%s, want v2 401", path, status, body)
		}
	}
}

func TestApiSessionQuestionReplyRejectUndeclaredMethods(t *testing.T) {
	for _, action := range []string{"reject", "reply"} {
		_, srv := newTestServer(&mockDB{})
		status, _, body := doShimRequest(t, srv.URL, http.MethodGet,
			"/api/session/"+apiSessionQuestionActionSessionID+"/question/que_1/"+action)
		srv.Close()
		if status != http.StatusMethodNotAllowed || !strings.Contains(string(body), "METHOD_NOT_ALLOWED") {
			t.Errorf("GET question/%s: got %d body=%s, want 405", action, status, body)
		}
	}
}
