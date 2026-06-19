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
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"

	"github.com/wojons/consensus/internal/db"
)

// Config is the root configuration for the Consensus binary.
type Config struct {
	Server     ServerConfig     `yaml:"server"`
	Adapters   AdaptersConfig   `yaml:"adapters"`
	LLM        LLMConfig        `yaml:"llm"`
	Harness    HarnessConfig    `yaml:"harness"`
	Database   db.Config        `yaml:"database"`
	HITL       HITLConfig       `yaml:"hitl"`
	Logging    LoggingConfig    `yaml:"logging"`
	APIRate     APIRateConfig     `yaml:"api_rate"`
	Compression CompressionConfig `yaml:"compression"`
	// configPath tracks the file that was loaded (for informational use).
	configPath string
}

// Path returns the config file path that was loaded, or "" if no file was loaded.
func (c *Config) Path() string { return c.configPath }

// ServerConfig holds HTTP server settings.
type ServerConfig struct {
	Hostname string `yaml:"hostname"`
	Port     int    `yaml:"port"`
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

// AdaptersConfig holds protocol adapter settings (SPEC-017).
type AdaptersConfig struct {
	OpenCode OpenCodeAdapterConfig `yaml:"opencode"`
}

// OpenCodeAdapterConfig holds the opencode shim adapter settings.
type OpenCodeAdapterConfig struct {
	Enabled  bool   `yaml:"enabled"`
	AdminKey string `yaml:"admin_key"` // Admin API key for auth translation
}

// Defaults returns a Config populated with safe defaults.
func Defaults() Config {
	return Config{
		Server: ServerConfig{
			Hostname: "127.0.0.1",
			Port:     8090,
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
		},
		Database: db.Config{
			URL:          "sqlite://dev.db",
			MaxOpenConns: 1,
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

// SetConfigPath sets an explicit config path override for Load().
// When set, this path takes highest priority above all chain entries.
// Call before Load() to apply a --config flag value.
func SetConfigPath(p string) {
	configPathOverride = p
}

// configPathOverride is set via SetConfigPath for CLI --config flag support.
var configPathOverride string

// LoadWithPath reads configuration with an explicit config file path override.
// When configPath is set (e.g. via --config flag), it takes highest priority,
// bypassing the normal chain. When empty, uses the standard priority chain.
func LoadWithPath(configPath string) (Config, error) {
	cfg := Defaults()

	// Resolve config path if not explicitly set.
	if configPath == "" {
		configPath = resolveConfigPath()
	}

	if configPath != "" {
		data, err := os.ReadFile(configPath)
		if err != nil {
			if !os.IsNotExist(err) {
				return cfg, fmt.Errorf("config: cannot read %s: %w", configPath, err)
			}
			// File doesn't exist — proceed with defaults + env.
		} else {
			if err := yaml.Unmarshal(data, &cfg); err != nil {
				return cfg, fmt.Errorf("config: cannot parse %s: %w", configPath, err)
			}
			cfg.configPath = configPath
		}
	}

	// Apply environment variable overrides.
	applyEnvOverrides(&cfg)

	return cfg, nil
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
		fmt.Sscanf(v, "%d", &cfg.Server.Port)
	}
	if v := os.Getenv("CONSENSUS_DB_URL"); v != "" {
		cfg.Database.URL = v
	}
	if v := os.Getenv("CONSENSUS_API_KEY"); v != "" {
		cfg.LLM.APIKey = v
	}
	if v := os.Getenv("CONSENSUS_LOG_LEVEL"); v != "" {
		cfg.Logging.Level = v
	}
	if v := os.Getenv("OPENAI_API_KEY"); v != "" && cfg.LLM.Provider == "openai" && cfg.LLM.APIKey == "" {
		cfg.LLM.APIKey = v
	}
	if v := os.Getenv("ANTHROPIC_API_KEY"); v != "" && cfg.LLM.Provider == "anthropic" && cfg.LLM.APIKey == "" {
		cfg.LLM.APIKey = v
	}
}
