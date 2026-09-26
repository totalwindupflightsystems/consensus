// Regression tests for DF-CONSENSUS-33: the CONSENSUS_API_KEY /
// CONSENSUS_SERVER env fallbacks and the consensus.yaml config adoption must
// be resolved AFTER flag parsing. Previously Execute() wrote env/config values
// into the opt vars BEFORE NewRootCommand() bound the persistent flags, and
// pflag's StringVar reset the bound variables to their defaults at bind time,
// silently dropping every pre-resolved value — an exported CONSENSUS_API_KEY
// never reached the wire (every API call failed with "missing API key").
//
// Two layers are covered:
//  1. resolveDefaults (unit): the flag > env > config-file precedence matrix.
//  2. Execute / root command (E2E, no live server): the resolved values must
//     be on the wire (Authorization header, request URL) after flag parsing.
//
// axiom:trace work_item=DF-CONSENSUS-33 test=internal/cli/root_test.go
package cli

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

// captureGlobals snapshots the opt vars and the CONSENSUS_* env vars before a
// test and restores both afterwards, so tests that exercise the resolution
// logic cannot leak state into sibling tests.
func captureGlobals(t *testing.T) {
	t.Helper()
	prevServer, prevAPIKey, prevConfig := optServer, optAPIKey, optConfig
	prevServerEnv := os.Getenv("CONSENSUS_SERVER")
	prevKeyEnv := os.Getenv("CONSENSUS_API_KEY")
	t.Cleanup(func() {
		optServer, optAPIKey, optConfig = prevServer, prevAPIKey, prevConfig
		if prevServerEnv == "" {
			os.Unsetenv("CONSENSUS_SERVER")
		} else {
			os.Setenv("CONSENSUS_SERVER", prevServerEnv)
		}
		if prevKeyEnv == "" {
			os.Unsetenv("CONSENSUS_API_KEY")
		} else {
			os.Setenv("CONSENSUS_API_KEY", prevKeyEnv)
		}
	})
}

// newKeyCapturingServer returns an httptest server that records the
// Authorization header of every request and replies with an empty session
// list (the response shape `session list` decodes).
func newKeyCapturingServer() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`[]`))
	}))
}

// ============================================================================
// resolveDefaults — the flag > env > config-file precedence matrix
// ============================================================================

func TestResolveDefaults_FlagBeatsEnv(t *testing.T) {
	captureGlobals(t)

	fs := newFlagSetForResolution()
	fs.Set("server", "http://flag-server:1111")
	fs.Set("api-key", "flag-key")
	os.Setenv("CONSENSUS_SERVER", "http://env-server:2222")
	os.Setenv("CONSENSUS_API_KEY", "env-key")

	server, apiKey := resolveDefaults(fs, nil)

	if server != "http://flag-server:1111" {
		t.Errorf("server: expected flag value, got %q", server)
	}
	if apiKey != "flag-key" {
		t.Errorf("api-key: expected flag value, got %q", apiKey)
	}
}

func TestResolveDefaults_EnvBeatsConfig(t *testing.T) {
	captureGlobals(t)

	fs := newFlagSetForResolution()
	os.Setenv("CONSENSUS_SERVER", "http://env-server:2222")
	os.Setenv("CONSENSUS_API_KEY", "env-key")

	server, apiKey := resolveDefaults(fs, nil)

	if server != "http://env-server:2222" {
		t.Errorf("server: expected env value over config, got %q", server)
	}
	if apiKey != "env-key" {
		t.Errorf("api-key: expected env value over config, got %q", apiKey)
	}
}

func TestResolveDefaults_ConfigFillsUnset(t *testing.T) {
	captureGlobals(t)

	fs := newFlagSetForResolution()
	cfg := &cliConfig{}
	cfg.Server.URL = "http://config-server:3333"
	cfg.Server.APIKey = "config-key"

	server, apiKey := resolveDefaults(fs, cfg)

	if server != "http://config-server:3333" {
		t.Errorf("server: expected config value, got %q", server)
	}
	if apiKey != "config-key" {
		t.Errorf("api-key: expected config value, got %q", apiKey)
	}
}

func TestResolveDefaults_NothingSetIsEmpty(t *testing.T) {
	captureGlobals(t)

	fs := newFlagSetForResolution()

	server, apiKey := resolveDefaults(fs, nil)

	// Server falls back to the built-in default URL (what the --server flag
	// default used to be); the API key has no default.
	if server != defaultServerURL {
		t.Errorf("server: expected built-in default %q when nothing is set, got %q", defaultServerURL, server)
	}
	if apiKey != "" {
		t.Errorf("api-key: expected empty when nothing is set, got %q", apiKey)
	}
}

// ============================================================================
// Root command E2E — resolution must happen AFTER flag parsing, on the wire
// ============================================================================

// TestExecute_APIKeyFromEnv_ReachesWire is the DF-CONSENSUS-33 regression:
// a valid key exported in CONSENSUS_API_KEY must authenticate exactly like
// --api-key does. On the broken tree the env value was wiped by the flag
// bind, no Authorization header was sent, and the server answered
// UNAUTHENTICATED: missing API key (exit 4).
func TestExecute_APIKeyFromEnv_ReachesWire(t *testing.T) {
	captureGlobals(t)

	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`[]`))
	}))
	defer srv.Close()

	os.Setenv("CONSENSUS_API_KEY", "env-secret-key")
	os.Unsetenv("CONSENSUS_SERVER")

	cmd := NewRootCommand()
	cmd.SetArgs([]string{"--server", srv.URL, "session", "list"})
	if err, _ := captureStdout(func() error { return cmd.Execute() }); err != nil {
		t.Fatalf("execute: %v", err)
	}

	if gotAuth != "Bearer env-secret-key" {
		t.Errorf("env API key did not reach the wire: got Authorization %q, want %q", gotAuth, "Bearer env-secret-key")
	}
}

// The flag must still win when both flag and env are set.
func TestExecute_APIKeyFlagBeatsEnv_ReachesWire(t *testing.T) {
	captureGlobals(t)

	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`[]`))
	}))
	defer srv.Close()

	os.Setenv("CONSENSUS_API_KEY", "env-secret-key")

	cmd := NewRootCommand()
	cmd.SetArgs([]string{"--server", srv.URL, "--api-key", "flag-secret-key", "session", "list"})
	if err, _ := captureStdout(func() error { return cmd.Execute() }); err != nil {
		t.Fatalf("execute: %v", err)
	}

	if gotAuth != "Bearer flag-secret-key" {
		t.Errorf("flag API key should beat env: got Authorization %q, want %q", gotAuth, "Bearer flag-secret-key")
	}
}

// CONSENSUS_SERVER must survive flag parsing too: the request must land on
// the env-pointed server, not on the default localhost:8090.
func TestExecute_ServerFromEnv_ReachesWire(t *testing.T) {
	captureGlobals(t)

	var hitsDefault, hitsEnv int
	srvDefault := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hitsDefault++
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`[]`))
	}))
	defer srvDefault.Close()
	srvEnv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hitsEnv++
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`[]`))
	}))
	defer srvEnv.Close()

	os.Setenv("CONSENSUS_SERVER", srvEnv.URL)
	os.Unsetenv("CONSENSUS_API_KEY")

	cmd := NewRootCommand()
	cmd.SetArgs([]string{"session", "list"})
	if err, _ := captureStdout(func() error { return cmd.Execute() }); err != nil {
		t.Fatalf("execute: %v", err)
	}

	if hitsEnv != 1 {
		t.Errorf("CONSENSUS_SERVER env value not honored: env server got %d requests, want 1", hitsEnv)
	}
	if hitsDefault != 0 {
		t.Errorf("request leaked to the wrong server: default-target server got %d requests, want 0", hitsDefault)
	}
}

// Config-file adoption must still work end to end when neither flag nor env
// is set (existing behavior preserved by the refactor).
func TestExecute_APIKeyFromConfig_ReachesWire(t *testing.T) {
	captureGlobals(t)

	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`[]`))
	}))
	defer srv.Close()

	dir := t.TempDir()
	configPath := filepath.Join(dir, "consensus.yaml")
	configContent := "server:\n  url: " + srv.URL + "\n  api_key: config-file-key\n"
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatal(err)
	}

	os.Unsetenv("CONSENSUS_SERVER")
	os.Unsetenv("CONSENSUS_API_KEY")

	// --config is passed on the command line (the production shape): the
	// flag bind resets a pre-written optConfig var, so the config path must
	// arrive through parsing like any other flag value.
	cmd := NewRootCommand()
	cmd.SetArgs([]string{"--config", configPath, "session", "list"})
	if err, _ := captureStdout(func() error { return cmd.Execute() }); err != nil {
		t.Fatalf("execute: %v", err)
	}

	if gotAuth != "Bearer config-file-key" {
		t.Errorf("config-file API key did not reach the wire: got Authorization %q, want %q", gotAuth, "Bearer config-file-key")
	}
}

// TestExecute_Entry_APIKeyFromEnv exercises the production entry point
// Execute() (os.Args -> flag parse -> PersistentPreRunE resolution) so the
// fix cannot regress behind a hand-constructed command.
func TestExecute_Entry_APIKeyFromEnv(t *testing.T) {
	captureGlobals(t)

	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`[]`))
	}))
	defer srv.Close()

	os.Setenv("CONSENSUS_API_KEY", "env-secret-key")
	os.Unsetenv("CONSENSUS_SERVER")

	prevArgs := os.Args
	os.Args = []string{"consensus", "--server", srv.URL, "session", "list"}
	defer func() { os.Args = prevArgs }()

	prevStdout := os.Stdout
	devNull, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = devNull
	code := Execute()
	os.Stdout = prevStdout
	devNull.Close()

	if code != 0 {
		t.Errorf("Execute returned %d, want 0", code)
	}
	if gotAuth != "Bearer env-secret-key" {
		t.Errorf("env API key did not reach the wire via Execute(): got Authorization %q, want %q", gotAuth, "Bearer env-secret-key")
	}
}
