// Package main: regression tests for the LLM base URL resolution seam.
//
// axiom:trace work_item=df-consensus-1 spec=specs/016-cli-interface.md impl=cmd/consensus/main.go test=cmd/consensus/main_test.go
package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/wojons/consensus/internal/config"
	"github.com/wojons/consensus/internal/llm"
)

// shippedLikeLLMYAML mirrors the committed consensus.yaml llm block: provider
// openai pinned to the DeepSeek endpoint.
const shippedLikeLLMYAML = `llm:
  default_model: deepseek-v4-flash
  provider: openai
  base_url: https://api.deepseek.com/v1
  api_key: ${DEEPSEEK_API_KEY}
`

// llmEnvVars are every variable these tests branch on; clearing them makes the
// assertions independent of a developer's shell (env-contamination fix).
var llmEnvVars = []string{
	"CONSENSUS_LLM_BASE_URL", "OPENROUTER_BASE_URL", "OPENROUTER_API_KEY",
	"OPENAI_API_KEY", "DEEPSEEK_API_KEY", "CONSENSUS_API_KEY", "CONSENSUS_CONFIG",
}

func clearLLMEnv(t *testing.T) {
	t.Helper()
	for _, k := range llmEnvVars {
		t.Setenv(k, "")
	}
}

// writeShippedLikeConfig writes the shipped-config shape to a temp file and
// returns its path.
func writeShippedLikeConfig(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "consensus.yaml")
	if err := os.WriteFile(path, []byte(shippedLikeLLMYAML), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return path
}

// TestResolveLLMBaseURL_IsPureAccessor pins where the precedence lives.
//
// DF-CONSENSUS-1: resolveLLMBaseURL used to re-read CONSENSUS_LLM_BASE_URL and
// OPENROUTER_BASE_URL *after* returning the config-file value, so those env
// lookups were dead code whenever consensus.yaml set llm.base_url (it ships
// pinned to https://api.deepseek.com/v1). Environment resolution now happens
// once, in internal/config. With a config carrying no base URL this seam must
// therefore return "" and leave the provider default to the client factory —
// not pick up the environment itself.
func TestResolveLLMBaseURL_IsPureAccessor(t *testing.T) {
	t.Setenv("CONSENSUS_LLM_BASE_URL", "https://should-not-be-read-here/v1")
	t.Setenv("OPENROUTER_BASE_URL", "https://should-not-be-read-here/v1")

	cfg := config.Defaults()
	cfg.LLM.BaseURL = ""

	if got := resolveLLMBaseURL(cfg); got != "" {
		t.Errorf("expected resolveLLMBaseURL to be a pure accessor (env resolved in internal/config), got %q", got)
	}
}

// TestResolveLLMBaseURL_ReturnsEffectiveValue: whatever config resolved —
// config file, CONSENSUS_LLM_BASE_URL, or OPENROUTER_BASE_URL — is what the
// LLM client is constructed with.
func TestResolveLLMBaseURL_ReturnsEffectiveValue(t *testing.T) {
	cfg := config.Defaults()
	cfg.LLM.BaseURL = "https://openrouter.ai/api/v1"

	if got := resolveLLMBaseURL(cfg); got != "https://openrouter.ai/api/v1" {
		t.Errorf("expected the effective config base URL, got %q", got)
	}
}

// TestEmbeddingBaseURL_ProviderDefaultFallback: with no base URL configured the
// embedding/summarizer endpoint must follow the provider, because the
// embedding client defaults to OpenAI regardless of provider.
func TestEmbeddingBaseURL_ProviderDefaultFallback(t *testing.T) {
	cases := []struct {
		name     string
		provider string
		baseURL  string
		want     string
	}{
		{"explicit value wins", "openai", "https://proxy.internal/v1", "https://proxy.internal/v1"},
		{"openrouter default", "openrouter", "", "https://openrouter.ai/api/v1"},
		{"openai default", "openai", "", "https://api.openai.com/v1"},
		{"unknown provider default", "", "", "https://api.openai.com/v1"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := config.Defaults()
			cfg.LLM.Provider = tc.provider
			cfg.LLM.BaseURL = tc.baseURL

			if got := embeddingBaseURL(cfg); got != tc.want {
				t.Errorf("embeddingBaseURL = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestEmbeddingBaseURL_IgnoresEnv pins the same purity invariant for the
// embedding seam: no environment reads outside internal/config.
func TestEmbeddingBaseURL_IgnoresEnv(t *testing.T) {
	t.Setenv("CONSENSUS_LLM_BASE_URL", "https://should-not-be-read-here/v1")

	cfg := config.Defaults()
	cfg.LLM.Provider = "openai"
	cfg.LLM.BaseURL = ""

	if got := embeddingBaseURL(cfg); got != "https://api.openai.com/v1" {
		t.Errorf("expected the provider default, got %q", got)
	}
}

// TestShippedConfigWithOpenRouterKeyProducesProviderDefault is the shipped-config
// regression for DF-CONSENSUS-1, asserted on the exact wiring runServer uses:
// config.Load → resolveLLMBaseURL → llm.Config{Provider, BaseURL}.
//
// Before the fix this seam handed the factory https://api.deepseek.com/v1 while
// the provider was OpenRouter, so every call 401'd against DeepSeek. The fix
// must yield provider=openrouter and an EMPTY base URL, which is the signal the
// llm package maps to https://openrouter.ai/api/v1 (pinned in
// internal/llm/client_test.go).
func TestShippedConfigWithOpenRouterKeyProducesProviderDefault(t *testing.T) {
	clearLLMEnv(t)
	t.Setenv("CONSENSUS_CONFIG", writeShippedLikeConfig(t))
	t.Setenv("OPENROUTER_API_KEY", "sk-or-test")

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("config.Load: %v", err)
	}

	llmCfg := &llm.Config{
		Provider: llm.Provider(cfg.LLM.Provider),
		BaseURL:  resolveLLMBaseURL(cfg),
		APIKey:   cfg.LLM.APIKey,
		Model:    cfg.LLM.DefaultModel,
	}
	if got := llmCfg.Provider; got != llm.ProviderOpenRouter {
		t.Fatalf("expected the OpenRouter provider, got %q", got)
	}
	if got := llmCfg.BaseURL; got != "" {
		t.Errorf("LLM base URL = %q, want \"\" so the OpenRouter provider default applies (shipped DeepSeek pin must not survive the switch)", got)
	}
	if got := llmCfg.APIKey; got != "sk-or-test" {
		t.Errorf("API key = %q, want the OpenRouter key", got)
	}
}

// TestShippedConfigWithConsensusBaseURLOverride: an explicitly supplied
// CONSENSUS_LLM_BASE_URL reaches the client even with the shipped file present.
func TestShippedConfigWithConsensusBaseURLOverride(t *testing.T) {
	clearLLMEnv(t)
	t.Setenv("CONSENSUS_CONFIG", writeShippedLikeConfig(t))
	t.Setenv("CONSENSUS_LLM_BASE_URL", "https://proxy.internal/v1")

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("config.Load: %v", err)
	}
	if got := resolveLLMBaseURL(cfg); got != "https://proxy.internal/v1" {
		t.Errorf("LLM base URL = %q, want the CONSENSUS_LLM_BASE_URL override", got)
	}
}

// TestShippedConfigWithoutOverridesKeepsDeepSeek is the preservation guard for
// that same wiring: no OpenRouter env → the shipped DeepSeek endpoint stands.
func TestShippedConfigWithoutOverridesKeepsDeepSeek(t *testing.T) {
	clearLLMEnv(t)
	t.Setenv("CONSENSUS_CONFIG", writeShippedLikeConfig(t))

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("config.Load: %v", err)
	}

	llmCfg := &llm.Config{
		Provider: llm.Provider(cfg.LLM.Provider),
		BaseURL:  resolveLLMBaseURL(cfg),
		Model:    cfg.LLM.DefaultModel,
	}
	if got := llmCfg.Provider; got != llm.ProviderOpenAI {
		t.Errorf("provider = %q, want openai (from the config file)", got)
	}
	if got := llmCfg.BaseURL; got != "https://api.deepseek.com/v1" {
		t.Errorf("LLM base URL = %q, want the config-file value preserved", got)
	}
}
