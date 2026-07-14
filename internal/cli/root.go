// Root CLI command with global flags and subcommand registration.
//
// axiom:trace work_item=spec-016-hardening-01 spec=specs/016-cli-interface.md plan=phase-1/task-1/step-1 impl=internal/cli/root.go
package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

// Global CLI options, set by root command flags.
var (
	optServer string
	optAPIKey string
	optFormat string
	optQuiet  bool
	optConfig string
)

// NewRootCommand creates the root `consensus` command with all subcommands.
func NewRootCommand() *cobra.Command {
	root := &cobra.Command{
		Use:   "consensus",
		Short: "Consensus — database-native cognitive architecture for AI agents",
		Long: `Consensus is a database-native cognitive architecture for AI agents.

The CLI is a management tool for the Consensus runtime. It handles
operational tasks: starting the server, managing sessions, reviewing
approvals, running migrations, and inspecting system state.`,
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	// Global flags (SPEC-016 §4)
	root.PersistentFlags().StringVar(&optServer, "server", "http://localhost:8090",
		"Consensus server base URL (env: CONSENSUS_SERVER)")
	root.PersistentFlags().StringVar(&optAPIKey, "api-key", "",
		"API key for authentication (env: CONSENSUS_API_KEY)")
	root.PersistentFlags().StringVar(&optFormat, "format", "table",
		"Output format: table, json, yaml")
	root.PersistentFlags().BoolVar(&optQuiet, "quiet", false,
		"Suppress non-essential output")
	root.PersistentFlags().StringVar(&optConfig, "config", "",
		"Config file path (default: ./consensus.yaml or ~/.consensus/config.yaml)")

	// Register all command groups
	root.AddCommand(
		newServeCmd(),
		newInitCmd(),
		newMCPStdioCmd(),
		newSessionCmd(),
		newApproveCmd(),
		newRejectCmd(),
		newMigrateCmd(),
		newConfigCmd(),
		newModelsCmd(),
		newStatusCmd(),
		newMemoryCmd(),
		newToolCmd(),
		newSkillCmd(),
		newCompletionCmd(),
	)

	return root
}

// cliConfig represents the CLI-relevant portion of the consensus config file.
type cliConfig struct {
	Server struct {
		URL    string `yaml:"url"`
		APIKey string `yaml:"api_key"`
	} `yaml:"server"`
}

// loadCLIConfig loads config from the priority chain: --config flag > ./consensus.yaml >
// ~/.consensus/config.yaml > /etc/consensus/config.yaml. Returns nil if no config found.
func loadCLIConfig() *cliConfig {
	homeDir, _ := os.UserHomeDir()
	configPath := resolveConfigPath(homeDir)
	if configPath == "" {
		return nil
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil
	}

	var cfg cliConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil
	}
	return &cfg
}

// resolveConfigPath returns the first existing config file from the priority chain.
func resolveConfigPath(homeDir string) string {
	candidates := []string{}

	// 1. Explicit --config flag takes highest priority
	if optConfig != "" {
		candidates = append(candidates, optConfig)
	}

	// 2. Project-level
	candidates = append(candidates, "consensus.yaml")

	// 3. User-level
	if homeDir != "" {
		candidates = append(candidates, filepath.Join(homeDir, ".consensus", "config.yaml"))
	}

	// 4. System-level (Linux only)
	candidates = append(candidates, filepath.Join("/etc", "consensus", "config.yaml"))

	for _, p := range candidates {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}

// applyConfigOverrides applies CLI config values if flags/env haven't set them.
func applyConfigOverrides() {
	cfg := loadCLIConfig()
	if cfg == nil {
		return
	}

	// Only override if the default or env didn't set these already
	if optServer == "http://localhost:8090" && os.Getenv("CONSENSUS_SERVER") == "" && cfg.Server.URL != "" {
		optServer = cfg.Server.URL
	}
	if optAPIKey == "" && os.Getenv("CONSENSUS_API_KEY") == "" && cfg.Server.APIKey != "" {
		optAPIKey = cfg.Server.APIKey
	}
}

// Execute runs the root command and returns an exit code.
func Execute() int {
	// Priority: --config flag > ./consensus.yaml > ~/.consensus/config.yaml > /etc/consensus/config.yaml
	applyConfigOverrides()

	// Apply environment variable overrides (higher priority than config file, lower than -- flags)
	if optServer == "http://localhost:8090" && os.Getenv("CONSENSUS_SERVER") != "" {
		optServer = os.Getenv("CONSENSUS_SERVER")
	}
	if optAPIKey == "" && os.Getenv("CONSENSUS_API_KEY") != "" {
		optAPIKey = os.Getenv("CONSENSUS_API_KEY")
	}

	cmd := NewRootCommand()
	if err := cmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "consensus: %v\n", err)
		return exitCode(err)
	}
	return 0
}

// newClient creates an API client from global flags.
func newClient() *Client {
	return NewClient(optServer, optAPIKey)
}

// newFormatter creates an output formatter from global flags.
func newFormatter() *Formatter {
	f := Format(string(optFormat))
	if f != FormatJSON && f != FormatYAML && f != FormatTable {
		fmt.Fprintf(os.Stderr, "consensus: unknown format %q, falling back to table\n", optFormat)
		f = FormatTable
	}
	return NewFormatter(os.Stdout, f, optQuiet)
}

// exitCode maps errors to SPEC-016 §8 exit codes.
func exitCode(err error) int {
	if err == nil {
		return 0
	}
	msg := err.Error()

	switch {
	case strings.Contains(msg, "connection refused"), strings.Contains(msg, "no such host"),
		strings.Contains(msg, "i/o timeout"), strings.Contains(msg, "server unreachable"):
		return 3
	case strings.Contains(msg, "UNAUTHENTICATED"), strings.Contains(msg, "invalid or expired"):
		return 4
	case strings.Contains(msg, "NOT_FOUND"), strings.Contains(msg, "not found"):
		return 5
	case strings.Contains(msg, "CONFLICT"), strings.Contains(msg, "SESSION_PAUSED"):
		return 6
	case strings.Contains(msg, "RATE_LIMITED"):
		return 7
	case strings.Contains(msg, "unknown"), strings.Contains(msg, "invalid"),
		strings.Contains(msg, "required"), strings.Contains(msg, "missing"):
		return 2
	default:
		return 1
	}
}


