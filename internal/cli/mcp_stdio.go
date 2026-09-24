// MCP stdio subcommand — start the Consensus MCP server over stdin/stdout.
//
// This implements SPEC-015 §5.4 (Stdio Transport). When invoked, the server
// reads JSON-RPC 2.0 requests from stdin and writes responses to stdout.
// Stderr is reserved for logging.
//
// Usage: consensus mcp-stdio [--db-url postgres://...]
//
// axiom:trace work_item=WI-015 spec=specs/015-api-and-mcp.md plan=phase-5/task-5-1/step-5-1-1 impl=internal/cli/mcp_stdio.go
package cli

import (
	"fmt"
	"os"
	"strconv"

	"github.com/spf13/cobra"

	"github.com/wojons/consensus/internal/config"
)

// MCPStdioFunc is set by the main package to wire the MCP stdio startup.
// If nil, mcp-stdio prints a warning.
var MCPStdioFunc func()

func newMCPStdioCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "mcp-stdio",
		Short: "Start MCP server over stdin/stdout (SPEC-015 §5.4)",
		Long: `Start the MCP server in stdio transport mode.

Reads JSON-RPC 2.0 requests from stdin and writes responses to stdout.
Use stderr for logging. This enables MCP-compatible clients like Claude
Desktop to launch Consensus as a subprocess.

Example:
  consensus mcp-stdio --db-url sqlite://dev.db

Environment variables:
  CONSENSUS_DB_URL  Database connection URL
  CONSENSUS_LOG_LEVEL  Log level (debug, info, warn, error)
  CONSENSUS_API_KEY  API key for authentication (or --api-key)`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if dbURL, _ := cmd.Flags().GetString("db-url"); dbURL != "" {
				os.Setenv("CONSENSUS_DB_URL", dbURL)
			}
			if logLevel, _ := cmd.Flags().GetString("log-level"); logLevel != "info" {
				os.Setenv("CONSENSUS_LOG_LEVEL", logLevel)
			}
			if port, _ := cmd.Flags().GetInt("port"); port != 8090 {
				os.Setenv("CONSENSUS_PORT", strconv.Itoa(port))
			}
			// DOGFOOD-106: forward the resolved API key (priority:
			// --api-key flag > CONSENSUS_API_KEY env > consensus.yaml
			// server.api_key — see root.go Execute) so the stdio transport
			// can inject it into the initialize handshake. This makes the
			// documented invocation (`consensus mcp-stdio --api-key cs_ak_...`)
			// authenticate without the client crafting _meta.authorization.
			if optAPIKey != "" {
				os.Setenv("CONSENSUS_API_KEY", optAPIKey)
			}
			if optConfig != "" {
				config.SetConfigPath(optConfig)
			}

			if MCPStdioFunc != nil {
				MCPStdioFunc()
				return nil
			}
			return fmt.Errorf("mcp stdio startup not wired in this build; run the bare binary")
		},
	}

	cmd.Flags().String("db-url", "", "Database connection URL (env: CONSENSUS_DB_URL)")
	cmd.Flags().String("log-level", "info", "Log level: debug, info, warn, error")
	cmd.Flags().Int("port", 8090, "Server port (for bootstrap key info)")

	return cmd
}
