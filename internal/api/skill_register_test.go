package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// DF-CONSENSUS-67: a skill created through the production registration API must
// be returned by the pre-existing skill read path. No skills_registry fixture
// is inserted by this test.
func TestSkillRegistrationReadPathEndToEnd(t *testing.T) {
	srv := newIntegrationServer(t)
	defer srv.close()

	request := func(method, path, body string) (int, string) {
		t.Helper()
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+srv.adminKey)
		w := httptest.NewRecorder()
		srv.router.ServeHTTP(w, req)
		return w.Code, w.Body.String()
	}

	status, body := request(http.MethodPost, "/api/v1/skills", `{
		"name":"df67-release-check",
		"description":"Verify a release before publishing it",
		"instructions":"Run the release checks and report their real output.",
		"enabled":true
	}`)
	if status != http.StatusCreated {
		t.Fatalf("POST /api/v1/skills = %d %s, want 201", status, body)
	}
	if !strings.Contains(body, `"name":"df67-release-check"`) || !strings.Contains(body, `"enabled":true`) {
		t.Fatalf("create response does not describe the registered skill: %s", body)
	}

	status, body = request(http.MethodGet, "/api/v1/skills/df67-release-check", "")
	if status != http.StatusOK {
		t.Fatalf("GET /api/v1/skills/df67-release-check = %d %s, want 200", status, body)
	}
	for _, want := range []string{
		`"name":"df67-release-check"`,
		`"description":"Verify a release before publishing it"`,
		`"instructions":"Run the release checks and report their real output."`,
		`"enabled":true`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("skill read body %s does not contain %s", body, want)
		}
	}
}
