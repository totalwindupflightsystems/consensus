// tool & skill commands — inspect available tools and skills.
//
// axiom:trace work_item=interfaces-api-cli-01 spec=specs/016-cli-interface.md plan=phase-4/task-4-1/step-4-1-5 impl=internal/cli/tool.go
package cli

import (
	"github.com/spf13/cobra"
)

func newToolCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "tool",
		Short: "Inspect available tools",
		Long:  `List and show details for registered tools.`,
	}

	cmd.AddCommand(
		newToolListCmd(),
		newToolShowCmd(),
	)

	return cmd
}

func newToolListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List registered tools",
		RunE: func(cmd *cobra.Command, args []string) error {
			client := newClient()
			fmt := newFormatter()
			fmt.SetEmptyHint("No tools registered yet — see docs/TOOLS.md to learn how to register tools")

			results, err := client.ListTools()
			if err != nil {
				return err
			}

			return fmt.PrintTable(results, []string{"name", "description", "hemisphere", "handler_type", "status"})
		},
	}
}

func newToolShowCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show <tool-name>",
		Short: "Show tool details",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client := newClient()
			fmt := newFormatter()

			// List all tools and find the matching one
			results, err := client.ListTools()
			if err != nil {
				return err
			}

			for _, tool := range results {
				if tool["name"] == args[0] {
					return fmt.Print(tool)
				}
			}

			fmt.Println("Tool not found:", args[0])
			return nil
		},
	}
}

func newSkillCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "skill",
		Short: "Inspect available skills",
		Long:  `List and show details for registered skills.`,
	}

	cmd.AddCommand(
		newSkillListCmd(),
		newSkillShowCmd(),
	)

	return cmd
}

func newSkillListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List registered skills",
		RunE: func(cmd *cobra.Command, args []string) error {
			client := newClient()
			fmt := newFormatter()
			fmt.SetEmptyHint("No skills installed yet — see docs/TOOLS.md to learn how to install skills")

			results, err := client.ListSkills()
			if err != nil {
				return err
			}

			return fmt.PrintTable(results, []string{"name", "id", "enabled"})
		},
	}
}

func newSkillShowCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show <skill-name>",
		Short: "Show skill details (full instructions)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client := newClient()
			fmt := newFormatter()

			result, err := client.GetSkill(args[0])
			if err != nil {
				return err
			}

			return fmt.Print(result)
		},
	}
}
