// version command — print the build version.
//
// axiom:trace work_item=DF-CONSENSUS-26 spec=specs/016-cli-interface.md impl=internal/cli/version.go
package cli

import (
	"github.com/spf13/cobra"
)

// newVersionCmd returns the `consensus version` subcommand. It prints the
// same string the root --version flag yields (the shared version var in
// root.go — one source, so the two surfaces cannot drift). Like serve/init,
// it works offline: the PersistentPreRunE identity check skips it (root.go),
// so it runs with no server reachable. Structured output follows the
// formatter convention so --format json|yaml work like sibling commands.
func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the consensus version",
		Long:  `Print the consensus build version (same string as the --version flag).`,
		RunE: func(cmd *cobra.Command, args []string) error {
			fm := newFormatter()
			fm.Println("consensus version", version)
			return fm.Print(map[string]any{"version": version})
		},
	}
}
