package chronicle

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestNewServer(t *testing.T) {
	srv := NewServer("http://localhost:8080")
	if srv == nil {
		t.Fatal("NewServer returned nil")
	}
	if srv.apiURL != "http://localhost:8080" {
		t.Errorf("apiURL = %q, want %q", srv.apiURL, "http://localhost:8080")
	}
}

func TestHandleIndex(t *testing.T) {
	srv := NewServer("http://localhost:8080")

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	resp := w.Result()
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}

	contentType := resp.Header.Get("Content-Type")
	if !strings.Contains(contentType, "text/html") {
		t.Errorf("Content-Type = %q, want text/html", contentType)
	}

	body, _ := io.ReadAll(resp.Body)
	bodyStr := string(body)
	if !strings.Contains(bodyStr, "<!DOCTYPE html>") {
		t.Error("response does not contain DOCTYPE")
	}
	if !strings.Contains(bodyStr, "Chronicle") {
		t.Error("response does not contain Chronicle")
	}
	if !strings.Contains(bodyStr, "design-system.css") {
		t.Error("response does not reference design-system.css")
	}
}

func TestHandleHealth(t *testing.T) {
	srv := NewServer("http://localhost:8080")

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	resp := w.Result()
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}

	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "chronicle") {
		t.Error("health response does not contain chronicle")
	}
}

func TestHandleCSS_ServesDesignSystem(t *testing.T) {
	srv := NewServer("http://localhost:8080")

	req := httptest.NewRequest(http.MethodGet, "/css/design-system.css", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	resp := w.Result()
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}

	body, _ := io.ReadAll(resp.Body)
	bodyStr := string(body)

	// Verify key design tokens are present
	required := []string{
		"--color-bg-canvas",
		"--color-accent-primary",
		"--color-text-primary",
		"--font-sans",
		"--space-4",
		"chronicle-pulse-thinking",
		"[data-theme=\"light\"]",
	}
	for _, token := range required {
		if !strings.Contains(bodyStr, token) {
			t.Errorf("design-system.css missing token: %s", token)
		}
	}
}

func TestHandleNotFound(t *testing.T) {
	srv := NewServer("http://localhost:8080")

	req := httptest.NewRequest(http.MethodGet, "/nonexistent", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", w.Code)
	}
}

func TestCORSHeaders(t *testing.T) {
	srv := NewServer("http://localhost:8080")

	req := httptest.NewRequest(http.MethodOptions, "/", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Errorf("OPTIONS status = %d, want 204", w.Code)
	}
	if w.Header().Get("Access-Control-Allow-Origin") != "*" {
		t.Error("missing CORS Allow-Origin header")
	}
}

func TestAPIURLTrailingSlashTrimmed(t *testing.T) {
	srv := NewServer("http://localhost:8080/")
	if srv.apiURL != "http://localhost:8080" {
		t.Errorf("apiURL = %q, want %q (trailing slash not trimmed)", srv.apiURL, "http://localhost:8080")
	}
}
