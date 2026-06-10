// memory command — inspect agent memory.
//
// axiom:trace work_item=spec-016-hardening-01 spec=specs/016-cli-interface.md plan=phase-1/task-1/step-1 impl=internal/cli/memory.go
package cli

import (
	"strings"

	"github.com/spf13/cobra"
)

func newMemoryCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "memory",
		Short: "Inspect agent memory",
		Long:  `List memory events, show context, and view iteration history.`,
	}

	cmd.AddCommand(
		newMemoryListCmd(),
		newMemoryShowCmd(),
		newMemoryIterationsCmd(),
		newMemoryPagesCmd(),
	)

	return cmd
}

func newMemoryListCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list <session-id>",
		Short: "List memory events",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client := newClient()
			fm := newFormatter()

			typeFilter, _ := cmd.Flags().GetString("type")
			limit, _ := cmd.Flags().GetInt("limit")

			results, err := client.ListMemory(args[0])
			if err != nil {
				return err
			}

			// Apply --type filter (SPEC-016 §5.8)
			if typeFilter != "" {
				types := strings.Split(typeFilter, ",")
				var filtered []map[string]any
				for _, m := range results {
					mType := valString(m["type"])
					for _, want := range types {
						if strings.EqualFold(strings.TrimSpace(want), mType) {
							filtered = append(filtered, m)
							break
						}
					}
				}
				results = filtered
			}

			// Apply --limit (SPEC-016 §5.8)
			if limit > 0 && len(results) > limit {
				results = results[:limit]
			}

			return fm.PrintTable(results, []string{"id", "type", "iteration_created", "display_mode", "created_at"})
		},
	}

	cmd.Flags().String("type", "", "Filter by event type (comma-separated: text_block,tool_result)")
	cmd.Flags().Int("limit", 50, "Max events to show")

	return cmd
}

func newMemoryShowCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show <session-id> <memory-id>",
		Short: "Show a specific memory event",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			client := newClient()
			fm := newFormatter()

			result, err := client.GetMemoryEvent(args[0], args[1])
			if err != nil {
				return err
			}

			return fm.Print(result)
		},
	}
}

func newMemoryIterationsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "iterations <session-id>",
		Short: "Show iteration history",
		Long:  `Show iteration history for a session. Use --diff to see memory state changes.`,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client := newClient()
			fm := newFormatter()

			showDiff, _ := cmd.Flags().GetBool("diff")

			results, err := client.ListIterations(args[0])
			if err != nil {
				return err
			}

			if showDiff {
				fm.Println("(--diff mode: showing iteration comparison)")
				// Iteration diffs surface active_pointer changes between iterations
				for i := 1; i < len(results); i++ {
					prev := results[i-1]
					curr := results[i]
					prevPointers := valString(prev["active_pointers"])
					currPointers := valString(curr["active_pointers"])
					if prevPointers != currPointers {
						fm.Println("# Iteration", valString(curr["iteration_id"]))
						fm.Println("  Pointers changed")
					}
				}
			}

			return fm.PrintTable(results, []string{"iteration_id", "session_id", "rows_affected", "created_at"})
		},
	}

	cmd.Flags().Bool("diff", false, "Show what changed between iterations")

	return cmd
}

func newMemoryPagesCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "pages <session-id>",
		Short: "Show compressed memory pages",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client := newClient()
			fm := newFormatter()

			resp, err := client.do("GET", "/api/v1/sessions/"+args[0]+"/memory/pages", nil)
			if err != nil {
				return err
			}

			var results []map[string]any
			if err := client.decodeBody(resp, &results); err != nil {
				return err
			}

			return fm.PrintTable(results, []string{"id", "name", "created_at"})
		},
	}
}
