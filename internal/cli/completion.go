// completion command — generate shell completion scripts.
//
// axiom:trace work_item=interfaces-api-cli-01 spec=specs/016-cli-interface.md plan=phase-4/task-4-1/step-4-1-6 impl=internal/cli/completion.go
package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

func newCompletionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "completion [bash|zsh|fish]",
		Short: "Generate shell completion script",
		Long: `Generate shell completion scripts for Consensus CLI.

To load completions:

  bash:
    source <(consensus completion bash)

  zsh:
    source <(consensus completion zsh)

  fish:
    consensus completion fish | source`,
		ValidArgs: []string{"bash", "zsh", "fish"},
		Args:      cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			switch args[0] {
			case "bash":
				return cmd.Root().GenBashCompletion(os.Stdout)
			case "zsh":
				return cmd.Root().GenZshCompletion(os.Stdout)
			case "fish":
				return cmd.Root().GenFishCompletion(os.Stdout, true)
			default:
				return fmt.Errorf("unsupported shell %q — supported shells: bash, zsh, fish", args[0])
			}
		},
	}
}
