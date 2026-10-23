package config

import (
	"bytes"
	"log/slog"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// hermeticEnv clears every LLM-related environment variable these tests
// branch on, so a developer shell exporting e.g. OPENROUTER_API_KEY or
// CONSENSUS_LLM_BASE_URL cannot flip the assertions (env-contamination fix).
// t.Setenv restores the original values at test end.
func hermeticEnv(t *testing.T) {
	t.Helper()
	for _, k := range []string{
		"CONSENSUS_LLM_BASE_URL",
		"CONSENSUS_LLM_MODEL",
		"CONSENSUS_LLM_PROVIDER",
		"OPENROUTER_BASE_URL",
		"OPENROUTER_API_KEY",
		"OPENAI_API_KEY",
		"ANTHROPIC_API_KEY",
		"DEEPSEEK_API_KEY",
		"CONSENSUS_API_KEY",
		"CONSENSUS_CONFIG",
	} {
		t.Setenv(k, "")
	}
}

// shippedLikeLLMYAML mirrors the llm block of the repository's committed
// consensus.yaml: provider openai pinned to the DeepSeek endpoint.
const shippedLikeLLMYAML = `llm:
  default_model: deepseek-v4-flash
  provider: openai
  base_url: https://api.deepseek.com/v1
  api_key: ${DEEPSEEK_API_KEY}
`

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
	// CONFIG-INERT-1: planning_timeout_seconds is retained and wired (it is the
	// planning deadline); max_iterations/budget_limit_cents are session state
	// and were removed from this surface.
	if cfg.Harness.PlanningTimeoutSec != 180 {
		t.Errorf("expected planning timeout 180s, got %d", cfg.Harness.PlanningTimeoutSec)
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
	// PERF-CONSENSUS-11 / DF-CONSENSUS-58: pprof defaults to server.port+10000
	// on loopback (127.0.0.1:18090) to avoid collision across instances.
	if cfg.Server.PprofAddr != "127.0.0.1:18090" {
		t.Errorf("expected default pprof address 127.0.0.1:18090, got %q", cfg.Server.PprofAddr)
	}
}

func TestLoadNoFileUsesDefaults(t *testing.T) {
	// Ensure no config file is found.
	_ = os.Setenv("CONSENSUS_CONFIG", "/nonexistent/path")
	defer func() { _ = os.Unsetenv("CONSENSUS_CONFIG") }()

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if cfg.Server.Port != 8090 {
		t.Errorf("expected default port 8090, got %d", cfg.Server.Port)
	}
}

func TestEnvOverride(t *testing.T) {
	_ = os.Setenv("CONSENSUS_DB_URL", "postgres://override:5432/db")
	_ = os.Setenv("CONSENSUS_CONFIG", "/nonexistent/path")
	defer func() { _ = os.Unsetenv("CONSENSUS_DB_URL") }()
	defer func() { _ = os.Unsetenv("CONSENSUS_CONFIG") }()

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

// DF-CONSENSUS-58: empty/non-mock key returns an error, not a warning.
func TestApplyStartupValidations_EmptyAPIKeyReturnsError(t *testing.T) {
	cfg := Defaults()
	cfg.LLM.APIKey = ""

	_, err := cfg.ApplyStartupValidations()
	if err == nil {
		t.Fatal("expected error for empty API key")
	}
	if !strings.Contains(err.Error(), "LLM API key not configured") {
		t.Fatalf("expected 'LLM API key not configured' error, got: %v", err)
	}
}

// DF-CONSENSUS-58: unresolved ${...} key returns an error on non-mock providers.
func TestApplyStartupValidations_TemplateAPIKeyReturnsError(t *testing.T) {
	cfg := Defaults()
	cfg.LLM.APIKey = "${DEEPSEEK_API_KEY}"

	_, err := cfg.ApplyStartupValidations()
	if err == nil {
		t.Fatal("expected error for ${...} template key")
	}
	if !strings.Contains(err.Error(), "LLM API key not configured") {
		t.Fatalf("expected 'LLM API key not configured' error, got: %v", err)
	}
}

// DF-CONSENSUS-58/109: mock provider with no key must NOT error.
// Non-mock providers return an error (not a warning) for empty keys.
func TestApplyStartupValidations_MockProviderNoKeyNoError(t *testing.T) {
	cfg := Defaults()
	cfg.LLM.Provider = "mock"
	cfg.LLM.APIKey = ""

	_, err := cfg.ApplyStartupValidations()
	if err != nil {
		t.Fatalf("mock provider should not error without key: %v", err)
	}

	// Negative control: the same config on a real provider still errors.
	cfg.LLM.Provider = "openai"
	_, err = cfg.ApplyStartupValidations()
	if err == nil {
		t.Fatal("expected error for non-mock provider without key")
	}
	if !strings.Contains(err.Error(), "LLM API key not configured") {
		t.Fatalf("expected 'LLM API key not configured' error, got: %v", err)
	}
}

func TestApplyStartupValidations_SetAPIKeyNoError(t *testing.T) {
	cfg := Defaults()
	cfg.LLM.APIKey = "sk-test-key"

	warns, err := cfg.ApplyStartupValidations()
	if err != nil {
		t.Fatalf("unexpected error with valid API key: %v", err)
	}

	for _, w := range warns {
		if strings.Contains(w, "LLM API key") {
			t.Fatalf("unexpected API key warning with key set: %q", w)
		}
	}
}

func TestApplyStartupValidations_DeepSeekDisablesCompression(t *testing.T) {
	cfg := Defaults()
	cfg.LLM.BaseURL = "https://api.deepseek.com/v1"
	cfg.LLM.APIKey = "test-key"
	cfg.Compression.Enabled = true

	warns, err := cfg.ApplyStartupValidations()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

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

	_, err := cfg.ApplyStartupValidations()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !cfg.Compression.Enabled {
		t.Error("expected compression to stay enabled on OpenAI backend")
	}
}

func TestApplyStartupValidations_NoBaseURLKeepsCompression(t *testing.T) {
	cfg := Defaults()
	cfg.LLM.BaseURL = ""
	cfg.Compression.Enabled = true

	_, err := cfg.ApplyStartupValidations()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !cfg.Compression.Enabled {
		t.Error("expected compression to stay enabled when base URL is empty (provider default)")
	}
}

// --- applyEnvOverrides (C-GAP-015: OPENROUTER_API_KEY read path) ---

func TestEnvVarOverridesConfigKey(t *testing.T) {
	hermeticEnv(t)
	t.Setenv("DEEPSEEK_API_KEY", "env-key")

	cfg := Defaults()
	if err := yaml.Unmarshal([]byte("llm:\n  api_key: config-key\n"), &cfg); err != nil {
		t.Fatalf("unmarshal config: %v", err)
	}

	var logOutput bytes.Buffer
	previousLogger := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logOutput, nil)))
	t.Cleanup(func() { slog.SetDefault(previousLogger) })

	applyEnvOverrides(&cfg)

	if cfg.LLM.APIKey != "env-key" {
		t.Fatalf("LLM.APIKey = %q, want DEEPSEEK_API_KEY value", cfg.LLM.APIKey)
	}
	const warning = "LLM API key: using DEEPSEEK_API_KEY from environment (overrides config file value)"
	if got := logOutput.String(); !strings.Contains(got, "level=WARN") || !strings.Contains(got, warning) {
		t.Fatalf("startup log = %q, want WARN containing %q", got, warning)
	}
}

func TestEnvVarExpandsPlaceholderWithoutOverrideWarning(t *testing.T) {
	hermeticEnv(t)
	t.Setenv("DEEPSEEK_API_KEY", "env-key")

	cfg := Defaults()
	if err := yaml.Unmarshal([]byte("llm:\n  api_key: ${DEEPSEEK_API_KEY}\n"), &cfg); err != nil {
		t.Fatalf("unmarshal config: %v", err)
	}

	var logOutput bytes.Buffer
	previousLogger := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logOutput, nil)))
	t.Cleanup(func() { slog.SetDefault(previousLogger) })

	applyEnvOverrides(&cfg)

	if cfg.LLM.APIKey != "env-key" {
		t.Fatalf("LLM.APIKey = %q, want expanded DEEPSEEK_API_KEY value", cfg.LLM.APIKey)
	}
	if got := logOutput.String(); strings.Contains(got, "overrides config file value") {
		t.Fatalf("placeholder expansion emitted override warning: %q", got)
	}
}

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

// --- QA-CONSENSUS-20: explicit --config must fail loudly when missing ---

// TestLoadWithPath_ExplicitMissing_Fails pins the fail-loud contract for the
// CLI --config flag: when the caller passes an explicit path (configPathOverride
// via SetConfigPath) that does not exist, LoadWithPath must return an error
// naming that path instead of silently booting on defaults. An operator typo
// in --config would otherwise start a server with wrong settings.
func TestLoadWithPath_ExplicitMissing_Fails(t *testing.T) {
	hermeticEnv(t)
	// Move out of the repo so the auto-discovery chain (cwd consensus.yaml,
	// ~/.consensus/config.yaml, /etc/...) finds nothing and cannot shadow
	// the missing explicit path.
	t.Chdir(t.TempDir())

	missing := filepath.Join(t.TempDir(), "nonexistent.yaml")
	cfg, err := LoadWithPath(missing)
	if err == nil {
		t.Fatalf("LoadWithPath(%q) = no error, want explicit-path-not-found failure", missing)
	}
	if !strings.Contains(err.Error(), missing) {
		t.Errorf("error %q does not name the missing path %q", err.Error(), missing)
	}
	if cfg.configPath != "" {
		t.Errorf("failed load set configPath to %q, want empty", cfg.configPath)
	}
}

// TestLoadWithPath_AutoDiscoveryMissing_Succeeds preserves the pre-existing
// auto-discovery contract: with NO explicit override and no file anywhere on
// the chain, Load returns defaults (plus env) without error.
func TestLoadWithPath_AutoDiscoveryMissing_Succeeds(t *testing.T) {
	hermeticEnv(t)
	t.Chdir(t.TempDir())

	cfg, err := LoadWithPath("")
	if err != nil {
		t.Fatalf("LoadWithPath(\"\") with no discoverable file: %v", err)
	}
	if cfg.configPath != "" {
		t.Errorf("configPath = %q, want empty for defaults-only load", cfg.configPath)
	}
	if cfg.Server.Port != 8090 {
		t.Errorf("port = %d, want default 8090 when no config file exists", cfg.Server.Port)
	}
}

// --- Crier inbound message bus (CR-IN-001) ---

// TestLoad_CrierBlock pins the resolution the intake depends on: `crier` is a
// real config key, and CONSENSUS_CRIER_URL beats it. That variable is the same
// one crier.NewClient falls back to, so a config key that is left empty and an
// env-only deployment resolve to the same endpoint.
func TestLoad_CrierBlock(t *testing.T) {
	hermeticEnv(t)
	t.Setenv("CONSENSUS_CRIER_URL", "")
	t.Setenv("CONSENSUS_CRIER_AGENT", "")

	// No compiled-in default: the fallback to http://localhost:8767 belongs to
	// internal/crier, so an unset key stays visibly unset here.
	if got := Defaults().Crier; got != (CrierConfig{}) {
		t.Errorf("Defaults() crier block = %+v, want empty (the base-URL default lives in internal/crier)", got)
	}

	const block = `crier:
  url: http://config-crier:8767
  agent_id: consensus-config
`
	cfg, err := LoadWithPath(writeConfig(t, block))
	if err != nil {
		t.Fatalf("LoadWithPath: %v", err)
	}
	if cfg.Crier.URL != "http://config-crier:8767" {
		t.Errorf("crier.url = %q, want the value from the config file", cfg.Crier.URL)
	}
	if cfg.Crier.AgentID != "consensus-config" {
		t.Errorf("crier.agent_id = %q, want the value from the config file", cfg.Crier.AgentID)
	}

	t.Setenv("CONSENSUS_CRIER_URL", "http://env-crier:9999")
	t.Setenv("CONSENSUS_CRIER_AGENT", "consensus-env")

	cfg, err = LoadWithPath(writeConfig(t, block))
	if err != nil {
		t.Fatalf("LoadWithPath: %v", err)
	}
	if cfg.Crier.URL != "http://env-crier:9999" {
		t.Errorf("crier.url = %q, want CONSENSUS_CRIER_URL to win over the file", cfg.Crier.URL)
	}
	if cfg.Crier.AgentID != "consensus-env" {
		t.Errorf("crier.agent_id = %q, want CONSENSUS_CRIER_AGENT to win over the file", cfg.Crier.AgentID)
	}
}

func TestEnvOverride_LLMModelOverridesConfig(t *testing.T) {
	hermeticEnv(t)
	t.Setenv("CONSENSUS_CONFIG", writeConfig(t, `llm:
  default_model: config-model
`))
	t.Setenv("CONSENSUS_LLM_MODEL", "env-model")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.LLM.DefaultModel != "env-model" {
		t.Errorf("expected CONSENSUS_LLM_MODEL to override config model, got %q", cfg.LLM.DefaultModel)
	}
}

func TestEnvOverride_LLMProviderOverridesConfig(t *testing.T) {
	hermeticEnv(t)
	t.Setenv("CONSENSUS_CONFIG", writeConfig(t, `llm:
  provider: anthropic
`))
	t.Setenv("CONSENSUS_LLM_PROVIDER", "openai")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.LLM.Provider != "openai" {
		t.Errorf("expected CONSENSUS_LLM_PROVIDER to override config provider, got %q", cfg.LLM.Provider)
	}
}

func TestLoad_EnvOnlyDeepSeekUsesCompatibleEndpointAndModel(t *testing.T) {
	hermeticEnv(t)
	t.Setenv("CONSENSUS_CONFIG", filepath.Join(t.TempDir(), "absent.yaml"))
	t.Setenv("DEEPSEEK_API_KEY", "test-deepseek-key")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.LLM.DefaultModel != "deepseek-v4-flash" {
		t.Errorf("expected env-only DeepSeek setup to use deepseek-v4-flash, got %q", cfg.LLM.DefaultModel)
	}
	if cfg.LLM.Provider != "openai" {
		t.Errorf("expected the OpenAI-compatible provider, got %q", cfg.LLM.Provider)
	}
	if cfg.LLM.BaseURL != "https://api.deepseek.com/v1" {
		t.Errorf("expected env-only DeepSeek setup to select the DeepSeek endpoint, got %q", cfg.LLM.BaseURL)
	}
}

func TestLoad_DeepSeekKeyKeepsExplicitBaseURL(t *testing.T) {
	for _, tc := range []struct {
		name       string
		configYAML string
		envBaseURL string
		want       string
	}{
		{
			name:       "environment",
			envBaseURL: "https://proxy.internal/v1",
			want:       "https://proxy.internal/v1",
		},
		{
			name:       "config file",
			configYAML: "llm:\n  base_url: https://gateway.example/v1\n",
			want:       "https://gateway.example/v1",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			hermeticEnv(t)
			if tc.configYAML == "" {
				t.Setenv("CONSENSUS_CONFIG", filepath.Join(t.TempDir(), "absent.yaml"))
			} else {
				t.Setenv("CONSENSUS_CONFIG", writeConfig(t, tc.configYAML))
			}
			t.Setenv("DEEPSEEK_API_KEY", "test-deepseek-key")
			if tc.envBaseURL != "" {
				t.Setenv("CONSENSUS_LLM_BASE_URL", tc.envBaseURL)
			}

			cfg, err := Load()
			if err != nil {
				t.Fatalf("Load: %v", err)
			}
			if cfg.LLM.BaseURL != tc.want {
				t.Errorf("expected explicit base URL %q to win, got %q", tc.want, cfg.LLM.BaseURL)
			}
			if cfg.LLM.DefaultModel != "" {
				t.Errorf("expected explicit non-DeepSeek base URL to leave model untouched, got %q", cfg.LLM.DefaultModel)
			}
		})
	}
}

func TestLoad_DeepSeekKeyKeepsExplicitProvider(t *testing.T) {
	hermeticEnv(t)
	t.Setenv("CONSENSUS_CONFIG", filepath.Join(t.TempDir(), "absent.yaml"))
	t.Setenv("DEEPSEEK_API_KEY", "test-deepseek-key")
	t.Setenv("CONSENSUS_LLM_PROVIDER", "anthropic")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.LLM.Provider != "anthropic" {
		t.Errorf("expected explicit provider to win, got %q", cfg.LLM.Provider)
	}
	if cfg.LLM.BaseURL != "" {
		t.Errorf("expected explicit provider to prevent DeepSeek endpoint selection, got %q", cfg.LLM.BaseURL)
	}
	if cfg.LLM.DefaultModel != "" {
		t.Errorf("expected explicit provider to prevent DeepSeek model selection, got %q", cfg.LLM.DefaultModel)
	}
}

func TestLoad_ExplicitEnvModelSurvivesDeepSeekDefault(t *testing.T) {
	hermeticEnv(t)
	t.Setenv("CONSENSUS_CONFIG", filepath.Join(t.TempDir(), "absent.yaml"))
	t.Setenv("DEEPSEEK_API_KEY", "test-deepseek-key")
	t.Setenv("CONSENSUS_LLM_BASE_URL", "https://api.deepseek.com/v1")
	t.Setenv("CONSENSUS_LLM_MODEL", "gpt-4o")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.LLM.DefaultModel != "gpt-4o" {
		t.Errorf("expected explicit env model to survive DeepSeek defaulting, got %q", cfg.LLM.DefaultModel)
	}
}

func TestLoad_ExplicitModelSurvivesDeepSeekDefault(t *testing.T) {
	hermeticEnv(t)
	t.Setenv("CONSENSUS_CONFIG", writeConfig(t, `llm:
  default_model: operator-model
  base_url: https://api.deepseek.com/v1
`))
	t.Setenv("DEEPSEEK_API_KEY", "test-deepseek-key")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.LLM.DefaultModel != "operator-model" {
		t.Errorf("expected explicit config model to survive DeepSeek defaulting, got %q", cfg.LLM.DefaultModel)
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
	defer func() { _ = os.Unsetenv("CONSENSUS_DB_URL") }()
	defer func() { _ = os.Unsetenv("CONSENSUS_CONFIG") }()

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
	defer func() { _ = os.Unsetenv("CONSENSUS_DB_URL") }()
	defer func() { _ = os.Unsetenv("CONSENSUS_CONFIG") }()

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Database.URL != "sqlite:///tmp/cgap026-home/tilde.db" {
		t.Errorf("expected env override with expanded ~, got %q", cfg.Database.URL)
	}
}

// --- LLM base URL precedence (DF-CONSENSUS-1) ---
//
// The shipped consensus.yaml pins llm.base_url to https://api.deepseek.com/v1.
// Before this fix, resolveLLMBaseURL (cmd/consensus/main.go) returned
// cfg.LLM.BaseURL before consulting CONSENSUS_LLM_BASE_URL / OPENROUTER_BASE_URL,
// so the documented environment overrides were silently ignored and an
// OPENROUTER_API_KEY run still called api.deepseek.com — contradicting the
// README's "no separate CONSENSUS_LLM_BASE_URL required". Env resolution lives
// wholly in config (applyEnvOverrides), so the precedence is fixed here:
//
//	CONSENSUS_LLM_BASE_URL > OPENROUTER_BASE_URL > YAML base_url
//	> provider default (OpenRouter when OPENROUTER_API_KEY is set)

// writeConfig writes YAML into a temp dir and returns its path.
func writeConfig(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "consensus.yaml")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return path
}

func TestEnvOverride_LLMBaseURLAlwaysWins(t *testing.T) {
	hermeticEnv(t)
	t.Setenv("CONSENSUS_CONFIG", writeConfig(t, shippedLikeLLMYAML))
	t.Setenv("CONSENSUS_LLM_BASE_URL", "https://openrouter.ai/api/v1")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.LLM.BaseURL != "https://openrouter.ai/api/v1" {
		t.Errorf("expected CONSENSUS_LLM_BASE_URL to override the YAML base_url, got %q", cfg.LLM.BaseURL)
	}
}

func TestEnvOverride_OpenRouterBaseURLOverridesConfig(t *testing.T) {
	hermeticEnv(t)
	t.Setenv("CONSENSUS_CONFIG", writeConfig(t, shippedLikeLLMYAML))
	t.Setenv("OPENROUTER_BASE_URL", "https://openrouter.ai/api/v1")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.LLM.BaseURL != "https://openrouter.ai/api/v1" {
		t.Errorf("expected OPENROUTER_BASE_URL to override the YAML base_url, got %q", cfg.LLM.BaseURL)
	}
}

// TestEnvOverride_ConsensusBaseURLBeatsOpenRouterBaseURL pins the relative
// order of the two env vars: the provider-agnostic override is the most
// explicit, so it wins.
func TestEnvOverride_ConsensusBaseURLBeatsOpenRouterBaseURL(t *testing.T) {
	hermeticEnv(t)
	t.Setenv("CONSENSUS_CONFIG", writeConfig(t, shippedLikeLLMYAML))
	t.Setenv("CONSENSUS_LLM_BASE_URL", "https://proxy.internal/v1")
	t.Setenv("OPENROUTER_BASE_URL", "https://openrouter.ai/api/v1")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.LLM.BaseURL != "https://proxy.internal/v1" {
		t.Errorf("expected CONSENSUS_LLM_BASE_URL to beat OPENROUTER_BASE_URL, got %q", cfg.LLM.BaseURL)
	}
}

// TestEnvOverride_OpenRouterKeyDropsShippedDeepSeekBaseURL is the shipped-config
// regression: consensus.yaml pins the DeepSeek endpoint, OPENROUTER_API_KEY
// selects OpenRouter, and the effective base URL must no longer be DeepSeek.
// The empty value lets NewOpenAIClient/NewEmbeddingClient pick OpenRouter's
// provider default (https://openrouter.ai/api/v1).
func TestEnvOverride_OpenRouterKeyDropsShippedDeepSeekBaseURL(t *testing.T) {
	hermeticEnv(t)
	t.Setenv("CONSENSUS_CONFIG", writeConfig(t, shippedLikeLLMYAML))
	t.Setenv("OPENROUTER_API_KEY", "sk-or-test")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.LLM.Provider != "openrouter" {
		t.Fatalf("expected provider openrouter, got %q", cfg.LLM.Provider)
	}
	if cfg.LLM.BaseURL != "" {
		t.Errorf("expected the shipped DeepSeek base_url to be cleared for OpenRouter (provider default applies), got %q", cfg.LLM.BaseURL)
	}
}

// TestEnvOverride_OpenRouterKeyKeepsExplicitEnvBaseURL: clearing the stale
// YAML endpoint must not clobber a base URL the user set explicitly.
func TestEnvOverride_OpenRouterKeyKeepsExplicitEnvBaseURL(t *testing.T) {
	hermeticEnv(t)
	t.Setenv("CONSENSUS_CONFIG", writeConfig(t, shippedLikeLLMYAML))
	t.Setenv("OPENROUTER_API_KEY", "sk-or-test")
	t.Setenv("CONSENSUS_LLM_BASE_URL", "https://proxy.internal/v1")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.LLM.BaseURL != "https://proxy.internal/v1" {
		t.Errorf("expected explicit CONSENSUS_LLM_BASE_URL to survive provider switch, got %q", cfg.LLM.BaseURL)
	}
}

// TestLoad_NoOverrideKeepsConfigBaseURL is the preservation guard: with no
// override env set, the YAML base_url is untouched (existing behavior).
func TestLoad_NoOverrideKeepsConfigBaseURL(t *testing.T) {
	hermeticEnv(t)
	t.Setenv("CONSENSUS_CONFIG", writeConfig(t, shippedLikeLLMYAML))

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.LLM.BaseURL != "https://api.deepseek.com/v1" {
		t.Errorf("expected YAML base_url preserved when no override is set, got %q", cfg.LLM.BaseURL)
	}
	if cfg.LLM.Provider != "openai" {
		t.Errorf("expected provider from YAML when no override is set, got %q", cfg.LLM.Provider)
	}
}

// TestLoad_NoConfigNoOverrideLeavesBaseURLEmpty: no YAML, no env → empty base
// URL so the client factory applies the provider default.
func TestLoad_NoConfigNoOverrideLeavesBaseURLEmpty(t *testing.T) {
	hermeticEnv(t)
	t.Setenv("CONSENSUS_CONFIG", filepath.Join(t.TempDir(), "absent.yaml"))

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.LLM.BaseURL != "" {
		t.Errorf("expected empty base URL with no config and no override, got %q", cfg.LLM.BaseURL)
	}
}

// ============================================================================
// CONFIG-INERT-1 — every declared key reaches a runtime consumer
// ============================================================================
//
// ch:trace row=CONFIG-INERT-1 spec=internal/config/config.go#L63-L139 wave=none:serial-tick test=internal/config/config_test.go::TestConfigInertKeysResolved doc=docs/CONFIG.md prompt=/tmp/consensus-foreman-2026-10-23-07-21-45-CONFIG-INERT-1-brief.txt evidence=commit witness=none:local-repo-verification verdict=none:pending commit=none:pending-foreman-trace memory=none:pending
//
// The keys below were parsed and documented but read by nothing. Each is now
// either wired to its consumer (retainedWiredKeys) or gone from the struct,
// the shipped sample and docs/CONFIG.md (removedYAMLKeys). These assertions
// are a surface census: they fail if a later change re-adds a YAML key with no
// consumer, or silently drops one that has one.
//
// One key of the same class is deliberately outside this row's scope:
// adapters.opencode.admin_key is still declared and still unread by the shim,
// and both docs/CONFIG.md §5.10 and docs/runbooks/serve-adapters.md say so.

var removedYAMLKeys = []string{
	"harness.max_iterations",         // session state: sessions.planning_max_turns
	"harness.budget_limit_cents",     // session state: sessions.budget_limit_cents
	"harness.transaction_timeout_ms", // SQLite lock wait: database.busy_timeout_ms
	"llm.max_context_tokens",         // session state: sessions.context_budget
	"database.max_idle_conns",        // no consumer on either backend
}

var retainedWiredKeys = []struct {
	key      string
	consumer string
}{
	{"harness.heartbeat_interval_seconds", "harness heartbeat loop (cmd/consensus/main.go)"},
	{"harness.max_consecutive_errors", "circuit breaker threshold (internal/harness/circuit.go)"},
	{"harness.planning_timeout_seconds", "PlanningConfig.Timeout (internal/harness planningConfig)"},
	{"llm.max_output_tokens", "llm.Config.MaxTokens (cmd/consensus/main.go)"},
	{"database.max_open_conns", "sqlite/postgres pool sizing (internal/db/*)"},
	{"database.busy_timeout_ms", "SQLite PRAGMA busy_timeout (internal/db/sqlite)"},
	{"hitl.auto_pause_on_error_threshold", "hitl.Configuration (cmd hitlConfigurationFromConfig)"},
	{"hitl.require_approval_for_destructive", "hitl.Configuration (cmd hitlConfigurationFromConfig)"},
	{"hitl.require_approval_for_schema_changes", "hitl.Configuration (cmd hitlConfigurationFromConfig)"},
	{"hitl.approval_timeout_minutes", "approval expiry (internal/hitl RequestApproval)"},
	{"logging.level", "slog handler level (LoggingConfig.SlogHandler)"},
	{"logging.format", "slog text/json handler (LoggingConfig.SlogHandler)"},
	{"crier.url", "internal/crier client ladder + startup warning (ApplyStartupValidations)"},
	{"crier.agent_id", "internal/crier intake identity + startup warning"},
	{"api_rate.admin_per_min", "api.ServerConfig.AdminRate"},
	{"api_rate.session_per_min", "api.ServerConfig.SessionRate"},
	{"api_rate.readonly_per_min", "api.ServerConfig.ReadonlyRate"},
	{"api_rate.webhook_per_min", "api.ServerConfig.WebhookRate"},
	{"compression.enabled", "compression worker start (cmd/consensus/main.go)"},
	{"compression.poll_interval_seconds", "worker PollInterval"},
	{"compression.batch_size", "worker BatchSize"},
	{"compression.cosine_threshold", "worker CosineThreshold"},
	{"compression.embedding_model", "worker EmbeddingModel"},
	{"adapters.opencode.enabled", "opencode shim mount"},
	{"adapters.h3.enabled", "H3 shim mount"},
}

// yamlKeyPaths walks a config struct type and collects every dotted YAML key
// it declares (the `yaml` struct tags), which is the exact surface
// `yaml.Unmarshal` accepts.
func yamlKeyPaths(t reflect.Type, prefix string, out map[string]bool) {
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		if f.PkgPath != "" { // unexported: never decoded from YAML
			continue
		}
		tag := strings.Split(f.Tag.Get("yaml"), ",")[0]
		path := tag
		switch {
		case tag == "":
			path = prefix
		case prefix != "":
			path = prefix + "." + tag
		}
		out[path] = true
		if f.Type.Kind() == reflect.Struct && path != "" {
			yamlKeyPaths(f.Type, path, out)
		}
	}
}

func TestConfigInertKeysResolved(t *testing.T) {
	t.Run("surface census: removed keys gone, wired keys declared", func(t *testing.T) {
		paths := map[string]bool{}
		yamlKeyPaths(reflect.TypeOf(Config{}), "", paths)

		for _, key := range removedYAMLKeys {
			if paths[key] {
				t.Errorf("yaml key %q is still declared in Config but has no runtime consumer — "+
					"wire it to one or remove it (CONFIG-INERT-1)", key)
			}
		}
		for _, k := range retainedWiredKeys {
			if !paths[k.key] {
				t.Errorf("yaml key %q is missing from Config but %s consumes it", k.key, k.consumer)
			}
		}
	})

	t.Run("retained keys resolve from yaml, then env", func(t *testing.T) {
		hermeticEnv(t)
		t.Setenv("CONSENSUS_LOG_LEVEL", "")
		path := writeConfig(t, `harness:
  planning_timeout_seconds: 240
logging:
  level: debug
  format: json
hitl:
  auto_pause_on_error_threshold: 7
  require_approval_for_destructive: false
  require_approval_for_schema_changes: false
  approval_timeout_minutes: 15
crier:
  url: http://file-crier:8767
  agent_id: consensus-file
`)
		t.Setenv("CONSENSUS_CONFIG", path)

		cfg, err := Load()
		if err != nil {
			t.Fatalf("Load: %v", err)
		}
		if cfg.Harness.PlanningTimeoutSec != 240 {
			t.Errorf("harness.planning_timeout_seconds = %d, want 240", cfg.Harness.PlanningTimeoutSec)
		}
		if cfg.Logging.Level != "debug" || cfg.Logging.Format != "json" {
			t.Errorf("logging = %+v, want debug/json from the file", cfg.Logging)
		}
		if cfg.HITL.AutoPauseOnErrorThreshold != 7 || cfg.HITL.ApprovalTimeoutMinutes != 15 {
			t.Errorf("hitl = %+v, want the file's thresholds", cfg.HITL)
		}
		if cfg.HITL.RequireApprovalForDestructive || cfg.HITL.RequireApprovalForSchemaChanges {
			t.Errorf("hitl = %+v, want both approval gates disabled from the file", cfg.HITL)
		}
		if cfg.Crier.URL != "http://file-crier:8767" || cfg.Crier.AgentID != "consensus-file" {
			t.Errorf("crier = %+v, want the file's values", cfg.Crier)
		}

		// CONFIG-INERT-1: logging.level is also env-overridable, and the env
		// value must win (applyEnvOverrides, same ladder as every other key).
		t.Setenv("CONSENSUS_LOG_LEVEL", "warn")
		cfg, err = Load()
		if err != nil {
			t.Fatalf("Load: %v", err)
		}
		if cfg.Logging.Level != "warn" {
			t.Errorf("logging.level = %q, want CONSENSUS_LOG_LEVEL to win", cfg.Logging.Level)
		}
	})

	t.Run("logging keys build the handler the process installs", func(t *testing.T) {
		var buf bytes.Buffer
		infoCfg := LoggingConfig{Level: "warn", Format: "text"}
		logger := slog.New(infoCfg.SlogHandler(&buf))
		logger.Info("dropped")
		logger.Warn("kept", "k", "v")
		out := buf.String()
		if strings.Contains(out, "dropped") {
			t.Errorf("logging.level=warn let an INFO record through: %q", out)
		}
		if !strings.Contains(out, "kept") || !strings.Contains(out, "level=WARN") {
			t.Errorf("text handler did not emit the WARN record: %q", out)
		}

		buf.Reset()
		jsonCfg := LoggingConfig{Level: "debug", Format: "json"}
		slog.New(jsonCfg.SlogHandler(&buf)).Debug("structured", "session", "s1")
		out = strings.TrimSpace(buf.String())
		if !strings.HasPrefix(out, "{") || !strings.Contains(out, `"msg":"structured"`) {
			t.Errorf("logging.format=json did not produce a JSON line: %q", out)
		}

		if got := (LoggingConfig{Level: "not-a-level"}).SlogLevel(); got != slog.LevelInfo {
			t.Errorf("unknown level fell back to %v, want info", got)
		}
	})

	t.Run("stale removed keys in an existing file are ignored, not errors", func(t *testing.T) {
		hermeticEnv(t)
		// The loader is a plain yaml.Unmarshal with no KnownFields, so an
		// operator's pre-CONFIG-INERT-1 file must keep loading: the deleted
		// keys are dropped silently, and the retained keys still apply.
		path := writeConfig(t, `llm:
  max_context_tokens: 64000
harness:
  max_iterations: 250
  budget_limit_cents: 5000
  transaction_timeout_ms: 1000
  planning_timeout_seconds: 300
database:
  max_idle_conns: 4
`)
		t.Setenv("CONSENSUS_CONFIG", path)

		cfg, err := Load()
		if err != nil {
			t.Fatalf("a file carrying removed keys must still load: %v", err)
		}
		if cfg.Harness.PlanningTimeoutSec != 300 {
			t.Errorf("planning_timeout_seconds = %d, want 300 (retained key ignored?)",
				cfg.Harness.PlanningTimeoutSec)
		}
	})

	t.Run("configured crier section is reported, not silently accepted", func(t *testing.T) {
		cfg := Defaults()
		cfg.LLM.APIKey = "test-key"
		cfg.Compression.Enabled = false

		warns, err := cfg.ApplyStartupValidations()
		if err != nil {
			t.Fatalf("ApplyStartupValidations: %v", err)
		}
		for _, w := range warns {
			if strings.Contains(w, "crier") {
				t.Errorf("an unconfigured crier section warned: %q", w)
			}
		}

		cfg.Crier.URL = "http://crier:8767"
		cfg.Crier.AgentID = "consensus"
		warns, err = cfg.ApplyStartupValidations()
		if err != nil {
			t.Fatalf("ApplyStartupValidations: %v", err)
		}
		found := false
		for _, w := range warns {
			if strings.Contains(w, "crier intake runner") {
				found = true
			}
		}
		if !found {
			t.Errorf("configured crier section produced no warning; warns = %v", warns)
		}
	})
}
