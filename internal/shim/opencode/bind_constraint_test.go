package opencode

import (
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wojons/consensus/internal/config"
)

// --- opencode shim bind constraint (DF-CONSENSUS-19) ---
//
// The shim mounts on the main server's listener (cmd/consensus/main.go), so
// the shim's bind is the server bind: cmd/consensus/main.go passes
// cfg.Server.Hostname into NewServer as the listener host. The /instance/*
// surface is intentionally auth-free (SPEC-017 §3.10) and leaks host layout
// (absolute config.json path, home, worktree), so the mount chain's
// hostname/addr must default to loopback and the mount must resolve to a
// loopback network. A wildcard or non-loopback default anywhere in that
// chain exposes the auth-free surface beyond loopback.

// TestShimBindConstraint_DefaultListenerIsLoopback pins the loopback network
// of the listener address the mount chain resolves: cmd/consensus/main.go
// serves the shim from cfg.Server.Hostname:port, so a non-loopback default
// config directly exposes the auth-free surface.
func TestShimBindConstraint_DefaultListenerIsLoopback(t *testing.T) {
	cfg := config.Defaults()
	host := net.ParseIP(cfg.Server.Hostname)
	if host == nil || !host.IsLoopback() {
		t.Errorf("default listener host %q is not loopback — the auth-free shim surface (SPEC-017 §3.10) would be reachable beyond loopback", cfg.Server.Hostname)
	}
}

// TestShimBindConstraint_InstanceSurfaceAnonymousOnLoopback exercises the
// real behavior the constraint protects: the mounted shim serves /instance/*
// unauthenticated with host-layout data. This is the documented protocol
// compatibility contract (C19); its safety is the loopback bind pinned
// elsewhere, not auth.
func TestShimBindConstraint_InstanceSurfaceAnonymousOnLoopback(t *testing.T) {
	_, srv := newTestServer(&mockDB{})
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/instance/path")
	if err != nil {
		t.Fatalf("GET /instance/path: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /instance/path: expected 200 anonymous (SPEC-017 §3.10 protocol compatibility), got %d", resp.StatusCode)
	}
	var info map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		t.Fatalf("GET /instance/path: not JSON: %v", err)
	}
	if v, _ := info["config"].(string); !strings.Contains(v, "config.json") {
		t.Errorf("GET /instance/path: expected host-layout leak surface (config path) to stay pinned as anonymous data, got config=%v", info["config"])
	}
}

// TestShimBindConstraint_MountPatternsCoverInstance proves every /instance/*
// path the auth middleware exempts (SPEC-017 §3.10) is actually mounted in
// MountPatterns — a skip for an unmounted path would be dead surface; a
// mounted path without its skip would 401 against the opencode contract.
func TestShimBindConstraint_MountPatternsCoverInstance(t *testing.T) {
	for _, p := range []string{"/instance", "/instance/*"} {
		mounted := false
		for _, pattern := range MountPatterns {
			if pattern == p {
				mounted = true
				break
			}
		}
		if !mounted {
			t.Errorf("MountPatterns does not mount %q but the auth middleware exempts it (SPEC-017 §3.10)", p)
		}
	}
}

// TestShimBindConstraint_InstancePathHostLayoutShape pins the shape of the
// anonymous /instance/path payload so it is explicit what data the loopback
// bind protects: absolute home, config and workspace paths.
func TestShimBindConstraint_InstancePathHostLayoutShape(t *testing.T) {
	dir := t.TempDir()
	s := NewServer(&mockDB{}, "test-key", nil, nil)
	s.workdir = dir

	rec := httptest.NewRecorder()
	s.instancePath(rec, httptest.NewRequest(http.MethodGet, "/instance/path", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("instancePath: expected 200, got %d", rec.Code)
	}
	var info map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &info); err != nil {
		t.Fatalf("instancePath: not JSON: %v", err)
	}
	for _, f := range []string{"home", "state", "config", "worktree", "directory"} {
		v, _ := info[f].(string)
		if v == "" || !filepath.IsAbs(v) {
			t.Errorf("instancePath field %q must be an absolute host path (that is the data the loopback bind protects): got %v", f, info[f])
		}
	}
	if home, _ := os.UserHomeDir(); home != "" {
		v, _ := info["home"].(string)
		if v != home {
			t.Errorf("instancePath home = %q, want %q", v, home)
		}
	}
}
