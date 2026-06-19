// serve command — start the Consensus server.
//
// axiom:trace work_item=spec-016-hardening-01 spec=specs/016-cli-interface.md plan=phase-1/task-3/step-3-1 impl=internal/cli/serve.go
package cli

import (
	"fmt"
	"os"
	"strconv"

	"github.com/spf13/cobra"

	"github.com/wojons/consensus/internal/config"
)

// ServerFunc is set by the main package to enable the serve command to start
// the actual server. If nil, serve prints a warning (server must be run via
// the bare binary).
var ServerFunc func()

func newServeCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Start the Consensus server",
		Long: `Start the Consensus server with the harness loop, REST API, MCP server,
and protocol shims. Connects to PostgreSQL or SQLite depending on
database configuration.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Push CLI flags into environment for config.Load() to pick up
			// (preserves the priority chain: flags > env > config file > defaults).
			if port, _ := cmd.Flags().GetInt("port"); port != 8090 {
				os.Setenv("CONSENSUS_PORT", strconv.Itoa(port))
			}
			if hostname, _ := cmd.Flags().GetString("hostname"); hostname != "127.0.0.1" {
				os.Setenv("CONSENSUS_HOSTNAME", hostname)
			}
			if dbURL, _ := cmd.Flags().GetString("db-url"); dbURL != "" {
				os.Setenv("CONSENSUS_DB_URL", dbURL)
			}
			if logLevel, _ := cmd.Flags().GetString("log-level"); logLevel != "info" {
				os.Setenv("CONSENSUS_LOG_LEVEL", logLevel)
			}
			// Wire the --config flag to config.Load() via SetConfigPath.
			if optConfig != "" {
				config.SetConfigPath(optConfig)
			}

			if ServerFunc != nil {
				ServerFunc()
				return nil
			}
			return fmt.Errorf("server startup not wired in this build; run the bare binary to start the server")
		},
	}

	cmd.Flags().Int("port", 8090, "Port to listen on")
	cmd.Flags().String("hostname", "127.0.0.1", "Bind address")
	cmd.Flags().Bool("mcp", true, "Enable MCP server endpoint")
	cmd.Flags().String("db-url", "", "Database connection URL")
	cmd.Flags().String("log-level", "info", "Log level: debug, info, warn, error")
	cmd.Flags().String("adapter", "opencode", "Enable protocol shims: opencode (SPEC-016 §5.1)")
	cmd.Flags().String("migrations", "", "Path to migration files (default: embedded)")

	return cmd
}
