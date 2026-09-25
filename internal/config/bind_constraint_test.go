package config

import (
	"net"
	"path/filepath"
	"strings"
	"testing"
)

// --- opencode shim bind constraint (DF-CONSENSUS-19) ---
//
// The shim's /instance/* surface is intentionally auth-free for opencode
// protocol compatibility (SPEC-017 §3.10) and answers 200 with host-layout
// data (absolute config.json path, home dir, workspace directory). Measured
// live on a bunker 2026-09-24: GET /instance, /instance/path, /instance/vcs,
// /instance/vcs/diff all answered 200 anonymously when the shim's host port
// (inside a container/agent port range 30000-30099) was reachable from the
// bunker host. The defense is the bind, not auth: the shim — and the process
// that serves it — must default to loopback. These tests pin that default at
// every seam and pin the startup warning so the constraint stays visible
// when an operator deliberately widens the bind.

// TestBindConstraint_Defaults pins the loopback default at each bind seam:
// compiled config defaults, shipped consensus.yaml, serve --hostname flag
// default, and CONSENSUS_HOSTNAME env override handling (absent by default).
func TestBindConstraint_Defaults(t *testing.T) {
	t.Run("config defaults bind loopback", func(t *testing.T) {
		cfg := Defaults()
		if cfg.Server.Hostname != "127.0.0.1" {
			t.Errorf("config default hostname = %q, want 127.0.0.1 — a wildcard default exposes the auth-free opencode shim surface (SPEC-017 §3.10) beyond loopback", cfg.Server.Hostname)
		}
	})

	t.Run("shipped consensus.yaml binds loopback", func(t *testing.T) {
		hermeticEnv(t)
		cfg, err := LoadWithPath(filepath.Join("..", "..", "consensus.yaml"))
		if err != nil {
			t.Fatalf("LoadWithPath(shipped consensus.yaml): %v", err)
		}
		if cfg.Server.Hostname != "127.0.0.1" {
			t.Errorf("shipped consensus.yaml server.hostname = %q, want 127.0.0.1 — the shipped file must bind loopback by default", cfg.Server.Hostname)
		}
	})

	t.Run("hostname default survives a full env+file load", func(t *testing.T) {
		// hermeticEnv clears the LLM keys; also neutralize the hostname and
		// config-path env vars so nothing ambient widens the bind.
		for _, k := range []string{"CONSENSUS_HOSTNAME", "CONSENSUS_CONFIG"} {
			t.Setenv(k, "")
		}
		hermeticEnv(t)

		cfg, err := Load()
		if err != nil {
			t.Fatalf("Load: %v", err)
		}
		host := net.ParseIP(cfg.Server.Hostname)
		if host == nil || !host.IsLoopback() {
			t.Errorf("effective default hostname %q is not loopback — the auth-free shim surface would be reachable beyond loopback", cfg.Server.Hostname)
		}
	})
}

// TestApplyStartupValidations_NonLoopbackHostnameWarns: an operator override
// to a non-loopback bind must keep working, but it must produce a loud
// startup warning naming the auth-free shim surface — the constraint is
// stated, not silently dropped.
func TestApplyStartupValidations_NonLoopbackHostnameWarns(t *testing.T) {
	cfg := Defaults()
	cfg.LLM.APIKey = "test-key" // silence the unrelated C-GAP-003 key warning
	cfg.Server.Hostname = "0.0.0.0"

	warns := cfg.ApplyStartupValidations()

	found := false
	for _, w := range warns {
		if strings.Contains(w, "opencode shim") && strings.Contains(w, "/instance") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected a startup warning mentioning the opencode shim /instance surface for non-loopback hostname 0.0.0.0, got %v", warns)
	}
}

// TestApplyStartupValidations_LoopbackHostnameNoShimWarn: the loopback default
// must not produce the shim-bind warning (no warning noise on the safe path).
func TestApplyStartupValidations_LoopbackHostnameNoShimWarn(t *testing.T) {
	cfg := Defaults()
	cfg.LLM.APIKey = "test-key" // silence the unrelated C-GAP-003 key warning

	warns := cfg.ApplyStartupValidations()

	for _, w := range warns {
		if strings.Contains(w, "opencode shim") {
			t.Errorf("loopback default must not warn about the opencode shim bind, got %q", w)
		}
	}
}

// TestApplyStartupValidations_LocalhostNoShimWarn: a hostname that resolves
// to loopback ("localhost") is an acceptable bind and must not warn.
func TestApplyStartupValidations_LocalhostNoShimWarn(t *testing.T) {
	cfg := Defaults()
	cfg.LLM.APIKey = "test-key"
	cfg.Server.Hostname = "localhost"

	warns := cfg.ApplyStartupValidations()

	for _, w := range warns {
		if strings.Contains(w, "opencode shim") {
			t.Errorf("resolvable loopback hostname must not warn, got %q", w)
		}
	}
}

// TestApplyStartupValidations_UnresolvableHostnameWarns: an unresolvable or
// unusual hostname string cannot be proven loopback, so it must warn — the
// check is fail-closed rather than silent. The .invalid TLD (RFC 2606) is
// guaranteed to return NXDOMAIN, so the test does not depend on resolver
// search-domain behavior.
func TestApplyStartupValidations_UnresolvableHostnameWarns(t *testing.T) {
	cfg := Defaults()
	cfg.LLM.APIKey = "test-key"
	cfg.Server.Hostname = "not-a-hostname.invalid"

	warns := cfg.ApplyStartupValidations()

	found := false
	for _, w := range warns {
		if strings.Contains(w, "opencode shim") && strings.Contains(w, "/instance") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected the shim-bind warning for unresolvable hostname, got %v", warns)
	}
}
