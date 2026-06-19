// init command — bootstrap a new Consensus instance.
//
// axiom:trace work_item=runtime-dev-bootstrap-auth-01 spec=specs/016-cli-interface.md,specs/015-api-and-mcp.md plan=.memory-bank/work-items/runtime-dev-bootstrap-auth-01/plan.md impl=internal/cli/init.go test=internal/bootstrap/admin_key_test.go evidence=.memory-bank/work-items/runtime-dev-bootstrap-auth-01/verification.md
package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

// InitFunc is set by the main package to enable the init command.
var InitFunc func(dbURL string) error

func newInitCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "init",
		Short: "Bootstrap a new Consensus instance",
		Long: `Bootstrap a new Consensus instance with database tables, default
configuration, and an admin API key.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if InitFunc != nil {
				dbURL, _ := cmd.Flags().GetString("db-url")
				return InitFunc(dbURL)
			}
			return fmt.Errorf("init not wired in this build; run the bare binary to auto-initialize")
		},
	}

	cmd.Flags().String("db-url", "", "Database connection URL")
	cmd.Flags().String("llm-key", "", "LLM API key")
	cmd.Flags().String("llm-provider", "openai", "Default LLM provider: openai, anthropic")

	return cmd
}
