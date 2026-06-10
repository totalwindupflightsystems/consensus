// Package web: tests for the web admin UI server.
//
// axiom:trace work_item=polish-phase spec=specs/016-cli-interface.md plan=phase-1/task-1/step-1 test=internal/web/server_test.go
package web

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestNewServer_Defaults(t *testing.T) {
	s := NewServer("http://localhost:8090")
	if s == nil {
		t.Fatal("expected non-nil server")
	}
	if s.apiURL != "http://localhost:8090" {
		t.Errorf("expected API URL without trailing slash, got %q", s.apiURL)
	}
}

func TestNewServer_TrailingSlashStripped(t *testing.T) {
	s := NewServer("http://localhost:8090/")
	if s.apiURL != "http://localhost:8090" {
		t.Errorf("expected trailing slash stripped, got %q", s.apiURL)
	}
}

func TestHandler_ReturnsNonNil(t *testing.T) {
	s := NewServer("http://localhost:8090")
	h := s.Handler()
	if h == nil {
		t.Fatal("expected non-nil handler")
	}
}

func TestHealthEndpoint(t *testing.T) {
	s := NewServer("http://localhost:8090")
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "healthy") {
		t.Error("expected healthy indicator")
	}
	if !strings.Contains(w.Body.String(), "web-admin") {
		t.Error("expected web-admin UI indicator")
	}
}

func TestCORSHeaders(t *testing.T) {
	s := NewServer("http://localhost:8090")
	req := httptest.NewRequest(http.MethodOptions, "/", nil)
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Errorf("expected 204 for OPTIONS, got %d", w.Code)
	}
	if w.Header().Get("Access-Control-Allow-Origin") != "*" {
		t.Error("expected CORS header")
	}
}

func TestIndexPage(t *testing.T) {
	s := NewServer("http://localhost:8090")
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	contentType := w.Header().Get("Content-Type")
	if !strings.Contains(contentType, "text/html") {
		t.Errorf("expected HTML content type, got %q", contentType)
	}
	if !strings.Contains(w.Body.String(), "Conscience") {
		t.Error("expected Conscience in page body")
	}
}

func TestDashboardPage(t *testing.T) {
	s := NewServer("http://localhost:8090")
	req := httptest.NewRequest(http.MethodGet, "/dashboard", nil)
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "Dashboard") {
		t.Error("expected Dashboard in page body")
	}
}

func TestSessionsPage(t *testing.T) {
	s := NewServer("http://localhost:8090")
	req := httptest.NewRequest(http.MethodGet, "/sessions", nil)
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "Agent Sessions") {
		t.Error("expected sessions page content")
	}
}

func TestMemoryPage(t *testing.T) {
	s := NewServer("http://localhost:8090")
	req := httptest.NewRequest(http.MethodGet, "/memory", nil)
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "Memory Browser") {
		t.Error("expected memory page content")
	}
}

func TestNotFound(t *testing.T) {
	s := NewServer("http://localhost:8090")
	req := httptest.NewRequest(http.MethodGet, "/nonexistent", nil)
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestAPIProxy_GET(t *testing.T) {
	// Create a mock upstream API server
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/health" && r.Method == http.MethodGet {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"healthy":true}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer upstream.Close()

	s := NewServer(upstream.URL)
	req := httptest.NewRequest(http.MethodGet, "/api/api/v1/health", nil)
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200 from proxy, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), `"healthy":true`) {
		t.Error("expected proxied health response")
	}
}

func TestAPIProxy_AuthForwarding(t *testing.T) {
	// Create mock upstream that checks for auth header
	var receivedAuth string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"received":true}`))
	}))
	defer upstream.Close()

	s := NewServer(upstream.URL)
	req := httptest.NewRequest(http.MethodGet, "/api/api/v1/sessions", nil)
	req.Header.Set("Authorization", "Bearer test-key-123")
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	if receivedAuth != "Bearer test-key-123" {
		t.Errorf("expected auth forwarded, got %q", receivedAuth)
	}
}

func TestAPIProxy_QueryString(t *testing.T) {
	// Create mock upstream that checks for query string
	var receivedQuery string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedQuery = r.URL.RawQuery
		w.Write([]byte(`{"ok":true}`))
	}))
	defer upstream.Close()

	s := NewServer(upstream.URL)
	req := httptest.NewRequest(http.MethodGet, "/api/api/v1/sessions?status=idle", nil)
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	if receivedQuery != "status=idle" {
		t.Errorf("expected query string forwarded, got %q", receivedQuery)
	}
}

func TestAPIProxy_UpstreamError(t *testing.T) {
	s := NewServer("http://localhost:19999") // nothing listening
	req := httptest.NewRequest(http.MethodGet, "/api/api/v1/sessions", nil)
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusBadGateway {
		t.Errorf("expected 502 for unreachable upstream, got %d", w.Code)
	}
}

func TestCORS_NonOptionsRequest(t *testing.T) {
	s := NewServer("http://localhost:8090")
	req := httptest.NewRequest(http.MethodGet, "/dashboard", nil)
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)

	// CORS headers should still be set on non-OPTIONS requests
	if ao := w.Header().Get("Access-Control-Allow-Origin"); ao != "*" {
		t.Errorf("expected CORS allow-origin on GET, got %q", ao)
	}
}

func TestTemplateRendering_AllPages(t *testing.T) {
	s := NewServer("http://localhost:8090")
	h := s.Handler()

	pages := []string{"/", "/dashboard", "/sessions", "/memory"}
	for _, path := range pages {
		t.Run(path, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, path, nil)
			w := httptest.NewRecorder()
			h.ServeHTTP(w, req)

			if w.Code != http.StatusOK {
				t.Errorf("%s: expected 200, got %d", path, w.Code)
			}
			ct := w.Header().Get("Content-Type")
			if !strings.Contains(ct, "text/html") {
				t.Errorf("%s: expected HTML content type, got %q", path, ct)
			}
		})
	}
}

func TestServer_APIURLInTemplate(t *testing.T) {
	s := NewServer("http://custom-api:9090")
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)

	// The template should contain the API URL for frontend JS
	if !strings.Contains(w.Body.String(), "http://custom-api:9090") {
		t.Error("expected API URL in template output")
	}
}
