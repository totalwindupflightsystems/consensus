package config

import (
	"os"
	"strings"
	"testing"
)

// axiom:trace work_item=repo-bootstrap-01 spec=specs/016-cli-interface.md,specs/021-repository-layout.md plan=phase-1/task-1/step-2 test=internal/config/config_test.go

func TestDefaults(t *testing.T) {
	cfg := Defaults()

	if cfg.Server.Port != 8090 {
		t.Errorf("expected port 8090, got %d", cfg.Server.Port)
	}
	if cfg.Server.Hostname != "127.0.0.1" {
		t.Errorf("expected hostname 127.0.0.1, got %s", cfg.Server.Hostname)
	}
	if cfg.LLM.Provider != "openai" {
		t.Errorf("expected openai provider, got %s", cfg.LLM.Provider)
	}
	if cfg.Harness.HeartbeatIntervalSec != 5 {
		t.Errorf("expected heartbeat 5s, got %d", cfg.Harness.HeartbeatIntervalSec)
	}
	if cfg.Harness.MaxIterations != 100 {
		t.Errorf("expected max iterations 100, got %d", cfg.Harness.MaxIterations)
	}
	if cfg.Harness.MaxConsecutiveErrors != 3 {
		t.Errorf("expected max consecutive errors 3, got %d", cfg.Harness.MaxConsecutiveErrors)
	}
	// C-GAP-026: the default database URL is $HOME/.consensus/consensus.db,
	// matching the README Configuration table — not a CWD-relative dev.db.
	t.Setenv("HOME", "/tmp/cgap026-home")
	cfg = Defaults()
	wantDBURL := "sqlite:///tmp/cgap026-home/.consensus/consensus.db"
	if cfg.Database.URL != wantDBURL {
		t.Errorf("expected default DB URL %s, got %s", wantDBURL, cfg.Database.URL)
	}
	if cfg.HITL.RequireApprovalForDestructive != true {
		t.Errorf("expected require_approval_for_destructive=true")
	}
	if cfg.Logging.Level != "info" {
		t.Errorf("expected info log level, got %s", cfg.Logging.Level)
	}
}

func TestLoadNoFileUsesDefaults(t *testing.T) {
	// Ensure no config file is found.
	os.Setenv("CONSENSUS_CONFIG", "/nonexistent/path")
	defer os.Unsetenv("CONSENSUS_CONFIG")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if cfg.Server.Port != 8090 {
		t.Errorf("expected default port 8090, got %d", cfg.Server.Port)
	}
}

func TestEnvOverride(t *testing.T) {
	os.Setenv("CONSENSUS_DB_URL", "postgres://override:5432/db")
	os.Setenv("CONSENSUS_CONFIG", "/nonexistent/path")
	defer os.Unsetenv("CONSENSUS_DB_URL")
	defer os.Unsetenv("CONSENSUS_CONFIG")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if cfg.Database.URL != "postgres://override:5432/db" {
		t.Errorf("expected env override URL, got %s", cfg.Database.URL)
	}
}

func TestHITLDefaults(t *testing.T) {
	cfg := Defaults()

	if cfg.HITL.AutoPauseOnErrorThreshold != 3 {
		t.Errorf("expected auto_pause threshold 3, got %d", cfg.HITL.AutoPauseOnErrorThreshold)
	}
	if cfg.HITL.RequireApprovalForSchemaChanges != true {
		t.Errorf("expected require_approval_for_schema_changes=true")
	}
	if cfg.HITL.ApprovalTimeoutMinutes != 60 {
		t.Errorf("expected approval timeout 60, got %d", cfg.HITL.ApprovalTimeoutMinutes)
	}
}

func TestAPIRateDefaults(t *testing.T) {
	cfg := Defaults()

	if cfg.APIRate.AdminLimit != 1000 {
		t.Errorf("expected admin limit 1000, got %d", cfg.APIRate.AdminLimit)
	}
	if cfg.APIRate.SessionLimit != 100 {
		t.Errorf("expected session limit 100, got %d", cfg.APIRate.SessionLimit)
	}
}

// --- ApplyStartupValidations (C-GAP-002, C-GAP-003) ---

func TestApplyStartupValidations_EmptyAPIKeyWarns(t *testing.T) {
	cfg := Defaults()
	cfg.LLM.APIKey = ""

	warns := cfg.ApplyStartupValidations()

	if !strings.Contains(strings.Join(warns, "\n"), "No LLM API key") {
		t.Fatalf("expected API key warning, got %v", warns)
	}
}

func TestApplyStartupValidations_TemplateAPIKeyWarns(t *testing.T) {
	cfg := Defaults()
	// yaml.v3 leaves ${DEEPSEEK_API_KEY} as a literal when the env var is
	// unset — the startup validation must catch the template form too.
	cfg.LLM.APIKey = "${DEEPSEEK_API_KEY}"

	warns := cfg.ApplyStartupValidations()

	if !strings.Contains(strings.Join(warns, "\n"), "No LLM API key") {
		t.Fatalf("expected API key warning for ${...} template, got %v", warns)
	}
}

func TestApplyStartupValidations_SetAPIKeyNoWarning(t *testing.T) {
	cfg := Defaults()
	cfg.LLM.APIKey = "sk-test-key"

	warns := cfg.ApplyStartupValidations()

	for _, w := range warns {
		if strings.Contains(w, "LLM API key") {
			t.Fatalf("unexpected API key warning with key set: %q", w)
		}
	}
}

func TestApplyStartupValidations_DeepSeekDisablesCompression(t *testing.T) {
	cfg := Defaults()
	cfg.LLM.BaseURL = "https://api.deepseek.com/v1"
	cfg.Compression.Enabled = true

	warns := cfg.ApplyStartupValidations()

	if cfg.Compression.Enabled {
		t.Error("expected compression to be disabled on DeepSeek backend")
	}
	found := false
	for _, w := range warns {
		if strings.Contains(w, "Compression worker DISABLED") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected compression warning, got %v", warns)
	}
}

func TestApplyStartupValidations_NonDeepSeekKeepsCompression(t *testing.T) {
	cfg := Defaults()
	cfg.LLM.BaseURL = "https://api.openai.com/v1"
	cfg.Compression.Enabled = true

	_ = cfg.ApplyStartupValidations()

	if !cfg.Compression.Enabled {
		t.Error("expected compression to stay enabled on OpenAI backend")
	}
}

func TestApplyStartupValidations_NoBaseURLKeepsCompression(t *testing.T) {
	cfg := Defaults()
	cfg.LLM.BaseURL = ""
	cfg.Compression.Enabled = true

	_ = cfg.ApplyStartupValidations()

	if !cfg.Compression.Enabled {
		t.Error("expected compression to stay enabled when base URL is empty (provider default)")
	}
}

// --- applyEnvOverrides (C-GAP-015: OPENROUTER_API_KEY read path) ---

func TestApplyEnvOverrides_OpenRouterAPIKey(t *testing.T) {
	t.Setenv("OPENROUTER_API_KEY", "sk-or-test")
	cfg := Defaults()
	applyEnvOverrides(&cfg)

	if cfg.LLM.APIKey != "sk-or-test" {
		t.Errorf("expected LLM.APIKey from OPENROUTER_API_KEY, got %q", cfg.LLM.APIKey)
	}
	if cfg.LLM.Provider != "openrouter" {
		t.Errorf("expected provider openrouter, got %q", cfg.LLM.Provider)
	}
}

func TestApplyEnvOverrides_OpenRouterWinsOverDeepSeek(t *testing.T) {
	t.Setenv("DEEPSEEK_API_KEY", "sk-deepseek-test")
	t.Setenv("OPENROUTER_API_KEY", "sk-or-test")
	cfg := Defaults()
	applyEnvOverrides(&cfg)

	if cfg.LLM.APIKey != "sk-or-test" {
		t.Errorf("expected explicitly-set OPENROUTER_API_KEY to win over DEEPSEEK_API_KEY, got %q", cfg.LLM.APIKey)
	}
	if cfg.LLM.Provider != "openrouter" {
		t.Errorf("expected provider openrouter, got %q", cfg.LLM.Provider)
	}
}

func TestApplyEnvOverrides_OpenRouterUnsetKeepsDeepSeek(t *testing.T) {
	t.Setenv("DEEPSEEK_API_KEY", "sk-deepseek-test")
	// Hermetic: OPENAI_API_KEY (checked first when provider==openai) and
	// OPENROUTER_API_KEY would otherwise leak from the shell env and win
	// over the DeepSeek key this test asserts (env-contamination fix).
	t.Setenv("OPENAI_API_KEY", "")
	t.Setenv("OPENROUTER_API_KEY", "")
	cfg := Defaults()
	applyEnvOverrides(&cfg)

	if cfg.LLM.APIKey != "sk-deepseek-test" {
		t.Errorf("expected DEEPSEEK_API_KEY to apply when OPENROUTER_API_KEY unset, got %q", cfg.LLM.APIKey)
	}
}

// --- resolveDBURL (C-GAP-026: $HOME/~ expansion in database URLs) ---

func TestResolveDBURL_HomeEnvExpansion(t *testing.T) {
	t.Setenv("HOME", "/tmp/cgap026-home")

	if got := resolveDBURL("sqlite://$HOME/.consensus/consensus.db"); got != "sqlite:///tmp/cgap026-home/.consensus/consensus.db" {
		t.Errorf("expected expanded default URL, got %q", got)
	}
	if got := resolveDBURL("sqlite://${HOME}/data.db"); got != "sqlite:///tmp/cgap026-home/data.db" {
		t.Errorf("expected ${HOME} expansion, got %q", got)
	}
	if got := resolveDBURL("sqlite://~/db.db"); got != "sqlite:///tmp/cgap026-home/db.db" {
		t.Errorf("expected ~ expansion, got %q", got)
	}
	if got := resolveDBURL("sqlite://~"); got != "sqlite:///tmp/cgap026-home" {
		t.Errorf("expected bare ~ expansion, got %q", got)
	}
}

func TestResolveDBURL_NoHomeLeavesUnchanged(t *testing.T) {
	t.Setenv("HOME", "")

	if got := resolveDBURL("sqlite://$HOME/.consensus/consensus.db"); got != "sqlite://$HOME/.consensus/consensus.db" {
		t.Errorf("expected unchanged URL when HOME unset, got %q", got)
	}
}

func TestResolveDBURL_NonSQLiteUnchanged(t *testing.T) {
	t.Setenv("HOME", "/tmp/cgap026-home")

	// DSN credentials may legitimately contain ~ or $HOME — never touch them.
	dsn := "postgres://user:p@ss~word@host:5432/db?sslmode=require"
	if got := resolveDBURL(dsn); got != dsn {
		t.Errorf("expected postgres URL unchanged, got %q", got)
	}
}

func TestResolveDBURL_RelativeAndMemoryUnchanged(t *testing.T) {
	t.Setenv("HOME", "/tmp/cgap026-home")

	// Explicit CWD-relative choices (e.g. the repo's consensus.yaml dev.db)
	// and :memory: must survive resolution untouched.
	if got := resolveDBURL("sqlite://dev.db"); got != "sqlite://dev.db" {
		t.Errorf("expected relative URL unchanged, got %q", got)
	}
	if got := resolveDBURL("sqlite://:memory:"); got != "sqlite://:memory:" {
		t.Errorf("expected :memory: unchanged, got %q", got)
	}
}

func TestEnvOverrideDBURLHomeExpansion(t *testing.T) {
	t.Setenv("HOME", "/tmp/cgap026-home")
	t.Setenv("CONSENSUS_DB_URL", "sqlite://$HOME/custom/data.db")
	t.Setenv("CONSENSUS_CONFIG", "/nonexistent/path")
	defer os.Unsetenv("CONSENSUS_DB_URL")
	defer os.Unsetenv("CONSENSUS_CONFIG")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Database.URL != "sqlite:///tmp/cgap026-home/custom/data.db" {
		t.Errorf("expected env override with expanded $HOME, got %q", cfg.Database.URL)
	}
}

func TestEnvOverrideDBURLTildeExpansion(t *testing.T) {
	t.Setenv("HOME", "/tmp/cgap026-home")
	t.Setenv("CONSENSUS_DB_URL", "sqlite://~/tilde.db")
	t.Setenv("CONSENSUS_CONFIG", "/nonexistent/path")
	defer os.Unsetenv("CONSENSUS_DB_URL")
	defer os.Unsetenv("CONSENSUS_CONFIG")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Database.URL != "sqlite:///tmp/cgap026-home/tilde.db" {
		t.Errorf("expected env override with expanded ~, got %q", cfg.Database.URL)
	}
}
