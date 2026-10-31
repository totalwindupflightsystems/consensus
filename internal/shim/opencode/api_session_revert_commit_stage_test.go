package opencode

import (
	"encoding/json"
	"errors"
	"net/http"
	"testing"
)

const apiSessionRevertCommitStageID = "9b1c4d2e-0000-4000-8000-abcdefabcdef"

func TestApiSessionRevertCommitKnownSessionAnswersDeclared204(t *testing.T) {
	_, srv := newTestServer(&mockDB{queryRow: rowOf(map[string]any{"id": apiSessionRevertCommitStageID})})
	defer srv.Close()

	status, header, body := doShimRequest(t, srv.URL, http.MethodPost,
		"/api/session/"+apiSessionRevertCommitStageID+"/revert/commit")
	if status != http.StatusNoContent || len(body) != 0 || header.Get("Content-Type") != "" {
		t.Fatalf("commit success: status=%d content-type=%q body=%q, want 204 with no body", status, header.Get("Content-Type"), body)
	}
}

func TestApiSessionRevertCommitUnknownSessionAnswersDeclared404(t *testing.T) {
	_, srv := newTestServer(&mockDB{queryRowErr: errors.New("sqlite: no rows in result")})
	defer srv.Close()

	status, _, body := doShimRequest(t, srv.URL, http.MethodPost,
		"/api/session/unknown/revert/commit")
	if status != http.StatusNotFound {
		t.Fatalf("commit unknown session: got %d, want 404. Body: %s", status, body)
	}
	assertRevertErrorTag(t, body, "SessionNotFoundError")
}

func TestApiSessionRevertCommitBlankSessionAnswersDeclared400(t *testing.T) {
	_, srv := newTestServer(&mockDB{})
	defer srv.Close()

	status, _, body := doShimRequest(t, srv.URL, http.MethodPost,
		"/api/session/%20/revert/commit")
	if status != http.StatusBadRequest {
		t.Fatalf("commit blank session: got %d, want 400. Body: %s", status, body)
	}
	assertRevertErrorTag(t, body, "InvalidRequestError")
}

func TestApiSessionRevertStageValidRequestAnswersDeclared400Narrowing(t *testing.T) {
	_, srv := newTestServer(&mockDB{queryRow: rowOf(map[string]any{"id": apiSessionRevertCommitStageID})})
	defer srv.Close()

	status, _, body := doShimRequestWithBody(t, srv.URL, http.MethodPost,
		"/api/session/"+apiSessionRevertCommitStageID+"/revert/stage", `{"messageID":"msg_gap5"}`)
	if status != http.StatusOK {
		t.Fatalf("stage valid request: got %d, want 200. Body: %s", status, body)
	}
	var response map[string]any
	if err := json.Unmarshal(body, &response); err != nil || response["data"].(map[string]any)["messageID"] != "msg_gap5" {
		t.Fatalf("stage success body = %s, want data.messageID msg_gap5", body)
	}
}

func TestApiSessionRevertStageMalformedBodyAnswersDeclared400(t *testing.T) {
	_, srv := newTestServer(&mockDB{})
	defer srv.Close()

	status, _, body := doShimRequestWithBody(t, srv.URL, http.MethodPost,
		"/api/session/"+apiSessionRevertCommitStageID+"/revert/stage", `{}`)
	if status != http.StatusBadRequest {
		t.Fatalf("stage malformed body: got %d, want 400. Body: %s", status, body)
	}
	assertRevertErrorTag(t, body, "InvalidRequestError")
}

func TestApiSessionRevertStageUnknownSessionAnswersDeclared404(t *testing.T) {
	_, srv := newTestServer(&mockDB{queryRowErr: errors.New("sqlite: no rows in result")})
	defer srv.Close()

	status, _, body := doShimRequestWithBody(t, srv.URL, http.MethodPost,
		"/api/session/unknown/revert/stage", `{"messageID":"msg_gap5"}`)
	if status != http.StatusNotFound {
		t.Fatalf("stage unknown session: got %d, want 404. Body: %s", status, body)
	}
	assertRevertErrorTag(t, body, "SessionNotFoundError")
}

func TestApiSessionRevertCommitStageDeclared401(t *testing.T) {
	s := NewServer(&mockDB{}, "test-key", nil, nil)
	s.workdir = t.TempDir()
	_, srv := newTestServerWithServer(t, s)
	defer srv.Close()

	for _, path := range []string{
		"/api/session/ses_gap5/revert/commit",
		"/api/session/ses_gap5/revert/stage",
	} {
		status, _, body := doShimRequest(t, srv.URL, http.MethodPost, path)
		if status != http.StatusUnauthorized {
			t.Fatalf("missing auth %s: got %d, want 401. Body: %s", path, status, body)
		}
		assertRevertErrorTag(t, body, "UnauthorizedError")
	}
}

func TestApiSessionRevertCommitStageUndeclaredMethodsAnswer405(t *testing.T) {
	_, srv := newTestServer(&mockDB{queryRow: rowOf(map[string]any{"id": apiSessionRevertCommitStageID})})
	defer srv.Close()

	for _, method := range []string{http.MethodGet, http.MethodPut, http.MethodDelete, http.MethodPatch} {
		for _, suffix := range []string{"commit", "stage"} {
			status, _, body := doShimRequest(t, srv.URL, method,
				"/api/session/"+apiSessionRevertCommitStageID+"/revert/"+suffix)
			if status != http.StatusMethodNotAllowed {
				t.Errorf("%s %s: got %d, want 405. Body: %s", method, suffix, status, body)
			}
		}
	}
}

func assertRevertErrorTag(t *testing.T, body []byte, want string) {
	t.Helper()
	var got map[string]any
	if err := json.Unmarshal([]byte(body), &got); err != nil {
		t.Fatalf("decode error body: %v (%s)", err, body)
	}
	if got["_tag"] != want {
		t.Fatalf("error body _tag=%v, want %s: %s", got["_tag"], want, body)
	}
}
