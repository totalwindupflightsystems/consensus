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
	if cfg.Database.URL != "sqlite://dev.db" {
		t.Errorf("expected sqlite://dev.db, got %s", cfg.Database.URL)
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
	os.Unsetenv("OPENROUTER_API_KEY")
	cfg := Defaults()
	applyEnvOverrides(&cfg)

	if cfg.LLM.APIKey != "sk-deepseek-test" {
		t.Errorf("expected DEEPSEEK_API_KEY to apply when OPENROUTER_API_KEY unset, got %q", cfg.LLM.APIKey)
	}
}
