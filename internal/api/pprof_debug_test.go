// Package api: PERF-CONSENSUS-11 tests — wake wiring on the message POST
// path and the loopback-only pprof debug listener.
//
// The api layer must not import internal/harness (harness tests import api,
// so the reverse edge would cycle). The wake hook is therefore injected as
// a ServerConfig function; a nil hook must be a no-op so every existing
// NewServer caller keeps working.
package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// TestSessionMessageWakeFiredOnThinkingTransition proves the POST message
// handler signals the harness wake hook when it flips a session to
// 'thinking' — the harness-side promptness is proven in the harness package
// (TestHeartbeatWakeDispatchesImmediately).
func TestSessionMessageWakeFiredOnThinkingTransition(t *testing.T) {
	is := newIntegrationServer(t)
	defer is.close()

	wakeMu := sync.Mutex{}
	wakes := make([]string, 0, 1)
	is.Server.wake = func(sessionID string) {
		wakeMu.Lock()
		defer wakeMu.Unlock()
		wakes = append(wakes, sessionID)
	}

	sessionID := "11111111-2222-3333-4444-555555555555"
	if err := is.conn.Exec(context.Background(),
		`INSERT INTO sessions (id, agent_name, model_id, status, trust_level, goal) VALUES ($1, 'wake-agent', 'gpt-4o', 'idle', 'high', 'wake test')`,
		sessionID); err != nil {
		t.Fatalf("insert session: %v", err)
	}

	body := `{"role":"user","content":"hello wake"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/sessions/"+sessionID+"/message", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+is.adminKey)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	is.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("POST message: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	wakeMu.Lock()
	defer wakeMu.Unlock()
	if len(wakes) != 1 || wakes[0] != sessionID {
		t.Fatalf("expected exactly one wake for %s, got %v", sessionID, wakes)
	}
}

// TestSessionMessageWakeNilHookNoOp pins backward compatibility: servers
// built without a wake hook (all pre-existing callers, tests, tools) must
// keep working unchanged.
func TestSessionMessageWakeNilHookNoOp(t *testing.T) {
	is := newIntegrationServer(t)
	defer is.close()

	sessionID := "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeee1"
	if err := is.conn.Exec(context.Background(),
		`INSERT INTO sessions (id, agent_name, model_id, status, trust_level, goal) VALUES ($1, 'noop-agent', 'gpt-4o', 'idle', 'high', 'nil hook test')`,
		sessionID); err != nil {
		t.Fatalf("insert session: %v", err)
	}

	body := `{"role":"user","content":"no hook"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/sessions/"+sessionID+"/message", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+is.adminKey)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	is.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("POST message with nil wake hook: expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

// TestPprofLoopbackListenerServesAndPublicRouterDoesNot proves the debug
// surface contract: /debug/pprof/ returns 200 on the loopback listener and
// 404 on the public API router (DF-CONSENSUS-19 exposure class).
func TestPprofLoopbackListenerServesAndPublicRouterDoesNot(t *testing.T) {
	is := newIntegrationServer(t)
	defer is.close()

	// Start the loopback debug listener exactly as the serve path does.
	addr := StartPprofListener("127.0.0.1:0")
	if addr == nil {
		t.Fatal("startPprofListener returned nil for a valid loopback address")
	}
	defer addr.Close()

	url := "http://" + addr.Addr().String() + "/debug/pprof/"
	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("loopback /debug/pprof/: expected 200, got %d", resp.StatusCode)
	}

	// Public router must NOT serve pprof.
	req := httptest.NewRequest(http.MethodGet, "/debug/pprof/", nil)
	w := httptest.NewRecorder()
	is.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("public router /debug/pprof/: expected 404, got %d", w.Code)
	}
}

// TestStartPprofListenerRejectsNonLoopback pins the loopback-only guard: a
// non-loopback address must be refused (logged + no listener), so pprof can
// never be exposed on a public interface by config error.
func TestStartPprofListenerRejectsNonLoopback(t *testing.T) {
	for _, host := range []string{"0.0.0.0", "192.168.1.50"} {
		if addr := StartPprofListener(host + ":0"); addr != nil {
			addr.Close()
			t.Errorf("startPprofListener accepted non-loopback host %q", host)
		}
	}
}
