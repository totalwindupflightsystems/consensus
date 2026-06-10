// status command — system health and metrics.
//
// axiom:trace work_item=spec-016-hardening-01 spec=specs/016-cli-interface.md plan=phase-1/task-1/step-1 impl=internal/cli/status.go
package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newStatusCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show system health and metrics",
		Long:  `Display server health, active sessions, pending tasks, and system metrics.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			verbose, _ := cmd.Flags().GetBool("verbose")
			client := newClient()
			fm := newFormatter()

			health, err := client.Health()
			if err != nil {
				return err
			}

			metrics, err := client.GetMetrics()
			if err != nil {
				metrics = nil
			}

			// Build structured result so --format json/yaml works
			result := map[string]any{
				"server": valString(health["status"]),
			}
			if v, ok := health["version"]; ok {
				result["version"] = v
			}

			if metrics != nil {
				result["active_sessions"] = metrics["active_sessions"]
				result["pending_tasks"] = metrics["pending_tasks"]
				result["pending_approvals"] = metrics["pending_approvals"]

				if verbose {
					if total, ok := metrics["total_sessions"]; ok {
						result["total_sessions"] = total
					}
					if cost, ok := metrics["total_cost_usd"]; ok {
						result["total_cost_usd"] = cost
					}
				}
			} else {
				result["metrics"] = "unavailable"
			}

			return fm.Print(result)
		},
	}

	cmd.Flags().Bool("verbose", false, "Show additional system metrics (total sessions, costs)")

	return cmd
}

func valString(v any) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return fmt.Sprintf("%v", v)
}
