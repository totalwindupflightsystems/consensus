// Package config loads and manages Consensus configuration.
//
// Configuration is loaded with priority: CLI flags > environment variables >
// YAML config file > defaults. The Config struct is the single source of truth
// for all runtime settings.
//
// axiom:trace work_item=spec-016-hardening-01 spec=specs/016-cli-interface.md plan=phase-1/task-1/step-1 impl=internal/config/config.go
package config

import (
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/wojons/consensus/internal/db"
)

// Config is the root configuration for the Consensus binary.
type Config struct {
	Server      ServerConfig      `yaml:"server"`
	Adapters    AdaptersConfig    `yaml:"adapters"`
	LLM         LLMConfig         `yaml:"llm"`
	Harness     HarnessConfig     `yaml:"harness"`
	Database    db.Config         `yaml:"database"`
	HITL        HITLConfig        `yaml:"hitl"`
	Logging     LoggingConfig     `yaml:"logging"`
	APIRate     APIRateConfig     `yaml:"api_rate"`
	Compression CompressionConfig `yaml:"compression"`
	Crier       CrierConfig       `yaml:"crier"`
	// configPath tracks the file that was loaded (for informational use).
	configPath string
}

// Path returns the config file path that was loaded, or "" if no file was loaded.
func (c *Config) Path() string { return c.configPath }

// ServerConfig holds HTTP server settings.
type ServerConfig struct {
	Hostname string `yaml:"hostname"`
	Port     int    `yaml:"port"`
	// PprofAddr is the loopback-only address for the pprof debug listener
	// (PERF-CONSENSUS-11). Empty disables the listener. It must never point
	// at a public interface — pprof handlers are unauthenticated.
	PprofAddr string `yaml:"pprof_addr"`
}

// LLMConfig holds LLM provider configuration.
type LLMConfig struct {
	DefaultModel string `yaml:"default_model"`
	Provider     string `yaml:"provider"` // openai | anthropic | openrouter
	APIKey       string `yaml:"api_key"`
	BaseURL      string `yaml:"base_url"` // override API base URL (e.g. OpenRouter)
	MaxContext   int    `yaml:"max_context_tokens"`
	MaxOutput    int    `yaml:"max_output_tokens"`
}

// HarnessConfig holds harness loop settings.
type HarnessConfig struct {
	HeartbeatIntervalSec int `yaml:"heartbeat_interval_seconds"`
	MaxIterations        int `yaml:"max_iterations"`
	MaxConsecutiveErrors int `yaml:"max_consecutive_errors"`
	BudgetLimitCents     int `yaml:"budget_limit_cents"`
	// PlanningTimeoutSec is the max seconds for the full planning session.
	// LLM calls + SQLite writes compete for this single deadline. If your
	// model runs slow (>170s), increase this or see BusyTimeoutMs.
	// Default: 180 (3 minutes). Local LLMs like LM Studio may need 300+.
	PlanningTimeoutSec int `yaml:"planning_timeout_seconds"`
	// TransactionTimeoutMs is the max milliseconds a single SQL statement may
	// hold the planning transaction before it's considered stuck.
	// Default: 60000 (60s).
	TransactionTimeoutMs int `yaml:"transaction_timeout_ms"`
}

// HITLConfig holds human-in-the-loop settings.
type HITLConfig struct {
	AutoPauseOnErrorThreshold       int  `yaml:"auto_pause_on_error_threshold"`
	RequireApprovalForDestructive   bool `yaml:"require_approval_for_destructive"`
	RequireApprovalForSchemaChanges bool `yaml:"require_approval_for_schema_changes"`
	ApprovalTimeoutMinutes          int  `yaml:"approval_timeout_minutes"`
}

// LoggingConfig holds logging settings.
type LoggingConfig struct {
	Level  string `yaml:"level"`  // debug | info | warn | error
	Format string `yaml:"format"` // text | json
}

// APIRateConfig holds API rate limiting defaults.
type APIRateConfig struct {
	AdminLimit    int `yaml:"admin_per_min"`
	SessionLimit  int `yaml:"session_per_min"`
	ReadonlyLimit int `yaml:"readonly_per_min"`
	WebhookLimit  int `yaml:"webhook_per_min"`
}

// CompressionConfig holds settings for the background compression worker (WI-012).
type CompressionConfig struct {
	// Enabled starts the compression worker if true.
	Enabled bool `yaml:"enabled"`

	// PollIntervalSeconds is the compression queue polling interval.
	// Default: 5
	PollIntervalSeconds int `yaml:"poll_interval_seconds"`

	// BatchSize is the max events to process per poll cycle.
	// Default: 5
	BatchSize int `yaml:"batch_size"`

	// CosineThreshold is the minimum cosine similarity for accepting a summary.
	// Default: 0.85 (SPEC-002 §8.2)
	CosineThreshold float64 `yaml:"cosine_threshold"`

	// EmbeddingModel overrides the default embedding model.
	// Default: "text-embedding-3-small"
	EmbeddingModel string `yaml:"embedding_model"`
}

// CrierConfig holds the inbound crier message-bus settings (CR-IN-001).
//
// The base URL resolves in one ladder — crier.url, then CONSENSUS_CRIER_URL,
// then crier.DefaultBaseURL (http://localhost:8767). The client does the same
// resolution for an empty value, so a config file that omits the section and
// an env-only deployment both end up at the same endpoint.
type CrierConfig struct {
	// URL is the crier relay base URL. Empty falls back to
	// CONSENSUS_CRIER_URL and then http://localhost:8767.
	URL string `yaml:"url"`

	// AgentID is the crier agent identity this Consensus instance consumes as
	// — the id it registers, and the inbox it retrieves from. Empty leaves
	// the identity to the caller (the intake takes it per run).
	AgentID string `yaml:"agent_id"`
}

// AdaptersConfig holds protocol adapter settings (SPEC-017).
type AdaptersConfig struct {
	OpenCode OpenCodeAdapterConfig `yaml:"opencode"`
	H3       H3AdapterConfig       `yaml:"h3"`
}

// OpenCodeAdapterConfig holds the opencode shim adapter settings.
type OpenCodeAdapterConfig struct {
	Enabled  bool   `yaml:"enabled"`
	AdminKey string `yaml:"admin_key"` // Admin API key for auth translation
}

// H3AdapterConfig holds the H3 protocol shim adapter settings (get-h3
// battery / Hermes h3 plugin clients).
type H3AdapterConfig struct {
	Enabled bool `yaml:"enabled"`
}

// Defaults returns a Config populated with safe defaults.
func Defaults() Config {
	return Config{
		Server: ServerConfig{
			Hostname:  "127.0.0.1",
			Port:      8090,
			PprofAddr: "127.0.0.1:8095", // PERF-CONSENSUS-11: loopback-only pprof
		},
		LLM: LLMConfig{
			Provider:   "openai",
			MaxContext: 128000,
			MaxOutput:  16384,
		},
		Harness: HarnessConfig{
			HeartbeatIntervalSec: 5,
			MaxIterations:        100,
			MaxConsecutiveErrors: 3,
			BudgetLimitCents:     1000,
			PlanningTimeoutSec:   180,
			TransactionTimeoutMs: 60000,
		},
		Database: db.Config{
			URL:          resolveDBURL("sqlite://$HOME/.consensus/consensus.db"),
			MaxOpenConns: 8, // POOL-FIX-PROOF: 1 wedges under concurrent heartbeat+planning+polling
		},
		HITL: HITLConfig{
			AutoPauseOnErrorThreshold:       3,
			RequireApprovalForDestructive:   true,
			RequireApprovalForSchemaChanges: true,
			ApprovalTimeoutMinutes:          60,
		},
		Logging: LoggingConfig{
			Level:  "info",
			Format: "text",
		},
		APIRate: APIRateConfig{
			AdminLimit:    1000,
			SessionLimit:  100,
			ReadonlyLimit: 200,
			WebhookLimit:  500,
		},
		Compression: CompressionConfig{
			Enabled:             true,
			PollIntervalSeconds: 5,
			BatchSize:           5,
			CosineThreshold:     0.85,
			EmbeddingModel:      "text-embedding-3-small",
		},
		Adapters: AdaptersConfig{
			OpenCode: OpenCodeAdapterConfig{
				Enabled: true,
			},
			H3: H3AdapterConfig{
				Enabled: true,
			},
		},
	}
}

// Load reads configuration from the priority chain and applies environment overrides.
//
// Priority chain (first file found wins):
//  1. CONSENSUS_CONFIG env var
//  2. ./consensus.yaml
//  3. ~/.consensus/config.yaml
//  4. /etc/consensus/config.yaml
//
// Priority: env vars > YAML file > defaults.
func Load() (Config, error) {
	return LoadWithPath(configPathOverride)
}

// SetConfigPath sets an explicit config path override for Load() and returns a
// function that restores the previous override. When set, this path takes
// highest priority above all chain entries. Call before Load() to apply a
// --config flag value and defer the returned restore function so repeated CLI
// executions do not inherit stale process-global state.
func SetConfigPath(p string) func() {
	previous := configPathOverride
	configPathOverride = p
	return func() {
		configPathOverride = previous
	}
}

// configPathOverride is set via SetConfigPath for CLI --config flag support.
var configPathOverride string

// LoadWithPath reads configuration with an explicit config file path override.
// When configPath is set (e.g. via --config flag), it takes highest priority,
// bypassing the normal chain, and MUST exist: a missing explicit file returns
// an error (QA-CONSENSUS-20) so an operator typo cannot silently boot the
// server on defaults. When empty, uses the standard priority chain, where a
// file that is absent everywhere still falls back to defaults + env.
func LoadWithPath(configPath string) (Config, error) {
	cfg := Defaults()

	// Resolve config path if not explicitly set.
	if configPath == "" {
		configPath = resolveConfigPath()
	}

	if configPath != "" {
		data, err := os.ReadFile(configPath)
		if err != nil {
			// Auto-discovery only returns paths that exist, so a NotExist
			// here means the caller passed an explicit override that is
			// missing — fail loudly instead of proceeding with defaults.
			if os.IsNotExist(err) && configPathOverride != "" && configPathOverride == configPath {
				return cfg, fmt.Errorf("config: config file not found: %s", configPath)
			}
			return cfg, fmt.Errorf("config: cannot read %s: %w", configPath, err)
		}
		if err := yaml.Unmarshal(data, &cfg); err != nil {
			return cfg, fmt.Errorf("config: cannot parse %s: %w", configPath, err)
		}
		cfg.configPath = configPath
	}

	// Apply environment variable overrides.
	applyEnvOverrides(&cfg)

	// Normalize the effective database URL: expand $HOME/~ references so a
	// YAML-provided URL behaves exactly like the built-in default and the
	// CONSENSUS_DB_URL env var (both already normalized at their own seams).
	// Idempotent for URLs without home references. (C-GAP-026)
	cfg.Database.URL = resolveDBURL(cfg.Database.URL)

	return cfg, nil
}

// resolveDBURL expands home-directory references in a database URL so that
// the built-in default, an explicitly-set CONSENSUS_DB_URL, and a YAML
// database.url all behave identically (C-GAP-026). Both $HOME and ${HOME}
// are expanded env-var-style, and a leading ~ or ~/ resolves to the user's
// home directory. $HOME wins; os.UserHomeDir() is the fallback when it is
// unset. If no home directory can be determined the URL is returned
// unchanged and the driver will surface a clear open error.
//
// Non-sqlite URLs (postgres://...) are returned unchanged to avoid
// false-positive expansion of DSNs whose credentials may contain ~ or $HOME.
func resolveDBURL(raw string) string {
	if !strings.HasPrefix(raw, "sqlite://") {
		return raw
	}
	home := os.Getenv("HOME")
	if home == "" {
		if h, err := os.UserHomeDir(); err == nil && h != "" {
			home = h
		}
	}
	if home == "" {
		return raw
	}
	path := strings.TrimPrefix(raw, "sqlite://")
	path = strings.ReplaceAll(path, "${HOME}", home)
	path = strings.ReplaceAll(path, "$HOME", home)
	if path == "~" || strings.HasPrefix(path, "~/") {
		path = home + strings.TrimPrefix(path, "~")
	}
	return "sqlite://" + path
}

// resolveConfigPath returns the first existing config file from the priority chain.
func resolveConfigPath() string {
	candidates := []string{}

	if v := os.Getenv("CONSENSUS_CONFIG"); v != "" {
		candidates = append(candidates, v)
	}

	candidates = append(candidates,
		"consensus.yaml",
		configFileAtHome(".consensus", "config.yaml"),
		"/etc/consensus/config.yaml",
	)

	for _, p := range candidates {
		if p != "" {
			if _, err := os.Stat(p); err == nil {
				return p
			}
		}
	}
	return ""
}

// configFileAtHome returns path joined with user's home directory, or "" if not available.
func configFileAtHome(elem ...string) string {
	homeDir, err := os.UserHomeDir()
	if err != nil || homeDir == "" {
		return ""
	}
	parts := append([]string{homeDir}, elem...)
	return filepath.Join(parts...)
}

func applyEnvOverrides(cfg *Config) {
	if v := os.Getenv("CONSENSUS_HOSTNAME"); v != "" {
		cfg.Server.Hostname = v
	}
	if v := os.Getenv("CONSENSUS_PORT"); v != "" {
		_, _ = fmt.Sscanf(v, "%d", &cfg.Server.Port)
	}
	if v := os.Getenv("CONSENSUS_DB_URL"); v != "" {
		cfg.Database.URL = resolveDBURL(v)
	}
	if v := os.Getenv("CONSENSUS_API_KEY"); v != "" {
		cfg.LLM.APIKey = v
	}
	if v := os.Getenv("CONSENSUS_LOG_LEVEL"); v != "" {
		cfg.Logging.Level = v
	}
	// Crier inbound message bus (CR-IN-001). CONSENSUS_CRIER_URL is the same
	// variable crier.NewClient falls back to, so a resolver that reads the
	// config key and one that reads only the environment agree on the
	// endpoint. The literals are duplicated, not imported: internal/config
	// deliberately depends on no feature package.
	if v := os.Getenv("CONSENSUS_CRIER_URL"); v != "" {
		cfg.Crier.URL = v
	}
	if v := os.Getenv("CONSENSUS_CRIER_AGENT"); v != "" {
		cfg.Crier.AgentID = v
	}
	// These explicit overrides make the documented env-only deployment path
	// take precedence over llm.provider/default_model from a config file.
	providerFromEnv := false
	if v := os.Getenv("CONSENSUS_LLM_PROVIDER"); v != "" {
		cfg.LLM.Provider = v
		providerFromEnv = true
	}
	if v := os.Getenv("CONSENSUS_LLM_MODEL"); v != "" {
		cfg.LLM.DefaultModel = v
	}
	// LLM base URL precedence (DF-CONSENSUS-1). The shipped consensus.yaml pins
	// llm.base_url to https://api.deepseek.com/v1, and before this fix the
	// resolver in cmd/consensus returned that config value first, so both
	// documented environment overrides were silently ignored. Environment
	// resolution lives here (as it does for every other setting), giving one
	// coherent ladder:
	//
	//   CONSENSUS_LLM_BASE_URL > OPENROUTER_BASE_URL > config llm.base_url
	//   > provider default (resolved by the client factories)
	//
	// baseURLFromEnv records that the effective value came from the environment,
	// so the OPENROUTER_API_KEY branch below can drop a stale CONFIG-file
	// endpoint without clobbering an explicit one.
	baseURLFromEnv := false
	if v := os.Getenv("CONSENSUS_LLM_BASE_URL"); v != "" {
		cfg.LLM.BaseURL = v
		baseURLFromEnv = true
	} else if v := os.Getenv("OPENROUTER_BASE_URL"); v != "" {
		cfg.LLM.BaseURL = v
		baseURLFromEnv = true
	}
	if v := os.Getenv("OPENAI_API_KEY"); v != "" && cfg.LLM.Provider == "openai" && cfg.LLM.APIKey == "" {
		cfg.LLM.APIKey = v
	}
	if v := os.Getenv("ANTHROPIC_API_KEY"); v != "" && cfg.LLM.Provider == "anthropic" && cfg.LLM.APIKey == "" {
		cfg.LLM.APIKey = v
	}
	if v := os.Getenv("DEEPSEEK_API_KEY"); v != "" && (cfg.LLM.APIKey == "" || strings.HasPrefix(cfg.LLM.APIKey, "${")) {
		cfg.LLM.APIKey = v
		// DEEPSEEK_API_KEY is a provider signal only when no endpoint or
		// provider override was configured. Preserve explicit environment and
		// config-file choices; otherwise make the README's env-only contract
		// real instead of sending the DeepSeek key to OpenAI.
		if cfg.LLM.BaseURL == "" && !providerFromEnv && cfg.LLM.Provider == "openai" {
			cfg.LLM.BaseURL = "https://api.deepseek.com/v1"
		}
		baseURL, err := url.Parse(cfg.LLM.BaseURL)
		usesDeepSeek := err == nil && strings.EqualFold(baseURL.Hostname(), "api.deepseek.com")
		if usesDeepSeek && cfg.LLM.DefaultModel == "" {
			// DeepSeek rejects the later OpenAI fallback with HTTP 400; use the
			// shipped model only while the compiled config default is still empty.
			cfg.LLM.DefaultModel = "deepseek-v4-flash"
		}
	}
	// OPENROUTER_API_KEY selects OpenRouter as the LLM backend (README:
	// "Alternative: use OpenRouter instead of DeepSeek direct"). Setting the
	// key alone switches provider + default base URL (NewOpenAIClient maps
	// provider "openrouter" to https://openrouter.ai/api/v1), so no separate
	// CONSENSUS_LLM_BASE_URL/OPENROUTER_BASE_URL is required. Checked after
	// DEEPSEEK_API_KEY so an explicitly-set OpenRouter key wins over a
	// leftover DeepSeek key in the shell environment. (C-GAP-015)
	//
	// DF-CONSENSUS-1: the provider switch alone was not enough while the
	// shipped consensus.yaml pinned llm.base_url to https://api.deepseek.com/v1
	// — that config value kept routing OpenRouter calls to DeepSeek (401).
	// A base URL from the config FILE is provider-specific and must not
	// survive the switch; an environment-supplied base URL is an explicit
	// operator choice and is preserved.
	if v := os.Getenv("OPENROUTER_API_KEY"); v != "" {
		cfg.LLM.APIKey = v
		cfg.LLM.Provider = "openrouter"
		if !baseURLFromEnv {
			cfg.LLM.BaseURL = ""
		}
	}
}

// ApplyStartupValidations checks the effective configuration for problems
// that would fail at runtime. Non-fatal misconfigurations are corrected in
// place; remaining problems are returned as prominent startup warnings for
// the caller to log.
//
// C-GAP-002: the compression worker needs an embeddings endpoint, which
// DeepSeek (api.deepseek.com) does not provide. When the LLM backend is
// DeepSeek, compression is disabled with a warning instead of failing on
// every embedding call at runtime.
//
// C-GAP-003: with no LLM API key (empty, or still a ${...} YAML template
// literal that yaml.v3 could not resolve), every LLM call fails with an
// opaque 401. A warning at startup makes the misconfiguration obvious.
//
// DF-CONSENSUS-19: the opencode shim's /instance/* surface is intentionally
// auth-free for protocol compatibility (SPEC-017 §3.10) and discloses host
// layout, so the shared listener that serves it must bind loopback. An
// explicit non-loopback (or unresolvable) server.hostname still works, but
// it warns loudly so the exposure is a stated operator choice, not a
// surprise.
func (cfg *Config) ApplyStartupValidations() []string {
	var warns []string

	if cfg.LLM.APIKey == "" || strings.HasPrefix(cfg.LLM.APIKey, "${") {
		warns = append(warns, "No LLM API key configured — agent harness will fail on LLM calls (set DEEPSEEK_API_KEY or CONSENSUS_API_KEY)")
	}

	if cfg.Compression.Enabled && strings.Contains(strings.ToLower(cfg.LLM.BaseURL), "deepseek") {
		cfg.Compression.Enabled = false
		warns = append(warns, "Compression worker DISABLED — provider DeepSeek has no embeddings endpoint (set compression.enabled=false or configure an OpenAI-compatible embeddings provider)")
	}

	// DF-CONSENSUS-19: fail closed on "cannot prove loopback". A wildcard or
	// unresolvable hostname must warn — only a resolvable loopback address
	// keeps the auth-free shim surface private.
	if h := cfg.Server.Hostname; !hostnameIsLoopback(h) {
		warns = append(warns, fmt.Sprintf(
			"server.hostname %q is not loopback — the opencode shim /instance/* surface is intentionally auth-free (SPEC-017 §3.10) and discloses host layout, so it is reachable from any network that can reach this listener; bind 127.0.0.1 (server.hostname / CONSENSUS_HOSTNAME) and publish selectively, or tunnel instead",
			h))
	}

	return warns
}

// hostnameIsLoopback reports whether h provably resolves to a loopback
// address. Literal loopback IPs (127.0.0.0/8, ::1) are accepted directly;
// anything else must resolve to loopback or the bind is treated as exposed
// (DF-CONSENSUS-19). Hostnames like "localhost" resolve via the system
// resolver, so the check stays honest for non-literal loopback names.
func hostnameIsLoopback(h string) bool {
	if ip := net.ParseIP(h); ip != nil {
		return ip.IsLoopback()
	}
	addrs, err := net.LookupHost(h)
	if err != nil || len(addrs) == 0 {
		return false
	}
	for _, a := range addrs {
		if ip := net.ParseIP(a); ip == nil || !ip.IsLoopback() {
			return false
		}
	}
	return true
}
