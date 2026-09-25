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

func TestLoad_EnvOnlyDeepSeekUsesCompatibleModel(t *testing.T) {
	hermeticEnv(t)
	t.Setenv("CONSENSUS_CONFIG", filepath.Join(t.TempDir(), "absent.yaml"))
	t.Setenv("DEEPSEEK_API_KEY", "test-deepseek-key")
	t.Setenv("CONSENSUS_LLM_BASE_URL", "https://api.deepseek.com/v1")
	t.Setenv("CONSENSUS_LLM_PROVIDER", "openai")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.LLM.DefaultModel != "deepseek-v4-flash" {
		t.Errorf("expected env-only DeepSeek setup to use deepseek-v4-flash, got %q", cfg.LLM.DefaultModel)
	}
	if cfg.LLM.Provider != "openai" {
		t.Errorf("expected provider from env, got %q", cfg.LLM.Provider)
	}
	if cfg.LLM.BaseURL != "https://api.deepseek.com/v1" {
		t.Errorf("expected base URL from env, got %q", cfg.LLM.BaseURL)
	}
	t.Logf("model=%s provider=%s base_url=%s", cfg.LLM.DefaultModel, cfg.LLM.Provider, cfg.LLM.BaseURL)
}

func TestApplyEnvOverrides_DeepSeekKeyWithNonDeepSeekBaseKeepsModel(t *testing.T) {
	hermeticEnv(t)
	t.Setenv("DEEPSEEK_API_KEY", "test-deepseek-key")
	t.Setenv("CONSENSUS_LLM_BASE_URL", "https://openrouter.ai/api/v1")
	cfg := Defaults()

	applyEnvOverrides(&cfg)

	if cfg.LLM.DefaultModel != "" {
		t.Errorf("expected non-DeepSeek base URL to leave model untouched, got %q", cfg.LLM.DefaultModel)
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
