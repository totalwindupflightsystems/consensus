package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
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
	// PERF-CONSENSUS-11: pprof debug listener defaults to loopback 127.0.0.1:8095.
	if cfg.Server.PprofAddr != "127.0.0.1:8095" {
		t.Errorf("expected default pprof address 127.0.0.1:8095, got %q", cfg.Server.PprofAddr)
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
