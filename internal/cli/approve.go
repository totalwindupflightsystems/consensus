// approve command — human-in-the-loop approval management.
//
// axiom:trace work_item=spec-016-hardening-01 spec=specs/016-cli-interface.md plan=phase-1/task-1/step-2 impl=internal/cli/approve.go
package cli

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

func newApproveCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "approve",
		Short: "Manage human-in-the-loop approvals",
		Long: `List, show, approve, and reject pending HITL approval requests.

Without a subcommand, runs interactive approval mode.`,
		// Bare `conscience approve` enters interactive mode
		RunE: func(cmd *cobra.Command, args []string) error {
			// If an approval ID is given as arg, treat as approve+accept alias
			if len(args) == 1 {
				return approveApprove(cmd, args)
			}
			// Otherwise, interactive mode (SPEC-016 §5.4)
			return approveInteractive()
		},
	}

	// Flags for `conscience approve <id>` (SPEC-016 §5.4)
	cmd.Flags().String("notes", "", "Reviewer notes")
	cmd.Flags().String("modified-sql", "", "Modified SQL (for 'modified' decision)")

	cmd.AddCommand(
		newApproveListCmd(),
		newApproveShowCmd(),
		newApproveAcceptCmd(),
		newApproveRejectCmd(),
	)

	return cmd
}

// newRejectCmd creates the top-level `conscience reject` command (SPEC-016 §5.4).
func newRejectCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "reject <approval-id>",
		Short: "Reject a pending approval request",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client := newClient()
			fm := newFormatter()

			reason, _ := cmd.Flags().GetString("reason")

			req := map[string]any{"decision": "rejected"}
			if reason != "" {
				req["notes"] = reason
			}

			result, err := client.ReviewApproval(args[0], req)
			if err != nil {
				return err
			}

			fm.Println("Approval rejected:", args[0])
			return fm.Print(result)
		},
	}

	cmd.Flags().String("reason", "", "Reason for rejection")

	return cmd
}

// approveApprove handles `conscience approve <id>` — alias for `conscience approve accept <id>`.
func approveApprove(cmd *cobra.Command, args []string) error {
	client := newClient()
	fm := newFormatter()

	notes, _ := cmd.Flags().GetString("notes")
	modifiedSQL, _ := cmd.Flags().GetString("modified-sql")

	req := map[string]any{"decision": "approved"}
	if notes != "" {
		req["notes"] = notes
	}
	if modifiedSQL != "" {
		req["modified_sql"] = modifiedSQL
	}

	result, err := client.ReviewApproval(args[0], req)
	if err != nil {
		return err
	}

	fm.Println("Approval approved:", args[0])
	return fm.Print(result)
}

// approveInteractive implements the interactive approval mode (SPEC-016 §5.4).
// It lists pending approvals, shows a numbered menu, and accepts interactive input.
func approveInteractive() error {
	client := newClient()

	approvals, err := client.ListApprovals()
	if err != nil {
		return err
	}

	// Filter to only pending
	var pending []map[string]any
	for _, a := range approvals {
		if status, _ := a["status"].(string); status == "pending" {
			pending = append(pending, a)
		}
	}

	if len(pending) == 0 {
		fmt.Println("No pending approvals.")
		return nil
	}

	fmt.Printf("Pending approvals (%d):\n\n", len(pending))

	for i, a := range pending {
		risk := valString(a["risk_level"])
		reqType := valString(a["request_type"])
		sessionID := valString(a["session_id"])
		desc := valString(a["description"])
		age := valString(a["created_at"])

		// Truncate long descriptions
		if len(desc) > 80 {
			desc = desc[:77] + "..."
		}

		fmt.Printf("  [%d] %-7s %s (session: %.8s...)\n", i+1, risk, reqType, sessionID)
		fmt.Printf("      %q\n", desc)
		fmt.Printf("      Created: %s\n\n", age)
	}

	fmt.Print("Approve which? [1-N/a/r/q]: ")

	scanner := bufio.NewScanner(os.Stdin)
	if !scanner.Scan() {
		return nil
	}

	choice := strings.TrimSpace(strings.ToLower(scanner.Text()))

	switch {
	case choice == "q":
		fmt.Println("Cancelled.")
		return nil
	case choice == "a":
		// Approve all
		for _, a := range pending {
			id := valString(a["id"])
			req := map[string]any{"decision": "approved"}
			if _, err := client.ReviewApproval(id, req); err != nil {
				fmt.Fprintf(os.Stderr, "Failed to approve %s: %v\n", id[:8]+"...", err)
				continue
			}
			fmt.Printf("Approved: %s...\n", id[:8])
		}
		return nil
	case choice == "r":
		// Reject all
		for _, a := range pending {
			id := valString(a["id"])
			req := map[string]any{"decision": "rejected"}
			if _, err := client.ReviewApproval(id, req); err != nil {
				fmt.Fprintf(os.Stderr, "Failed to reject %s: %v\n", id[:8]+"...", err)
				continue
			}
			fmt.Printf("Rejected: %s...\n", id[:8])
		}
		return nil
	default:
		// Numeric choice
		idx := 0
		if _, err := fmt.Sscanf(choice, "%d", &idx); err != nil || idx < 1 || idx > len(pending) {
			fmt.Println("Invalid choice.")
			return nil
		}

		a := pending[idx-1]
		id := valString(a["id"])
		req := map[string]any{"decision": "approved"}
		if _, err := client.ReviewApproval(id, req); err != nil {
			return err
		}
		idShort := id
		if len(idShort) > 8 {
			idShort = idShort[:8]
		}
		fmt.Printf("Approved: %s...\n", idShort)
		return nil
	}
}

func newApproveListCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List pending approvals",
		RunE: func(cmd *cobra.Command, args []string) error {
			client := newClient()
			fm := newFormatter()

			results, err := client.ListApprovals()
			if err != nil {
				return err
			}

			// Filter by --session if provided
			sessionFilter, _ := cmd.Flags().GetString("session")
			riskFilter, _ := cmd.Flags().GetString("risk-level")

			var filtered []map[string]any
			for _, a := range results {
				if sessionFilter != "" && valString(a["session_id"]) != sessionFilter {
					continue
				}
				if riskFilter != "" {
					levels := strings.Split(riskFilter, ",")
					risk := valString(a["risk_level"])
					matched := false
					for _, l := range levels {
						if strings.EqualFold(strings.TrimSpace(l), risk) {
							matched = true
							break
						}
					}
					if !matched {
						continue
					}
				}
				filtered = append(filtered, a)
			}

			return fm.PrintTable(filtered, []string{"id", "session_id", "request_type", "risk_level", "status", "created_at"})
		},
	}

	cmd.Flags().String("session", "", "Filter by session ID")
	cmd.Flags().String("risk-level", "", "Filter by risk level (comma-separated: high,critical)")

	return cmd
}

func newApproveShowCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show <approval-id>",
		Short: "Show approval details",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client := newClient()
			fm := newFormatter()

			result, err := client.GetApproval(args[0])
			if err != nil {
				return err
			}

			return fm.Print(result)
		},
	}
}

func newApproveAcceptCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "accept <approval-id>",
		Short: "Approve a pending request",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return approveApprove(cmd, args)
		},
	}

	cmd.Flags().String("notes", "", "Reviewer notes")
	cmd.Flags().String("modified-sql", "", "Modified SQL (for 'modified' decision)")

	return cmd
}

func newApproveRejectCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "reject <approval-id>",
		Short: "Reject a pending request",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client := newClient()
			fm := newFormatter()

			reason, _ := cmd.Flags().GetString("reason")

			req := map[string]any{"decision": "rejected"}
			if reason != "" {
				req["notes"] = reason
			}

			result, err := client.ReviewApproval(args[0], req)
			if err != nil {
				return err
			}

			fm.Println("Approval rejected:", args[0])
			return fm.Print(result)
		},
	}

	cmd.Flags().String("reason", "", "Reason for rejection")

	return cmd
}
