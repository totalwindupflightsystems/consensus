// session command — manage agent sessions.
//
// axiom:trace work_item=spec-016-hardening-01 spec=specs/016-cli-interface.md plan=phase-1/task-1/step-1 impl=internal/cli/session.go
package cli

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

func newSessionCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "session",
		Short: "Manage agent sessions",
		Long:  `Create, list, show, pause, resume, and cancel agent sessions.`,
	}

	cmd.AddCommand(
		newSessionCreateCmd(),
		newSessionListCmd(),
		newSessionShowCmd(),
		newSessionLogsCmd(),
		newSessionPauseCmd(),
		newSessionResumeCmd(),
		newSessionCancelCmd(),
		newSessionCostCmd(),
	)

	return cmd
}

func newSessionCreateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a new agent session",
		RunE: func(cmd *cobra.Command, args []string) error {
			goal, _ := cmd.Flags().GetString("goal")
			if goal == "" {
				return fmt.Errorf("--goal is required — use: consensus session create --goal \"<task description>\"")
			}
			agentName, _ := cmd.Flags().GetString("agent-name")
			model, _ := cmd.Flags().GetString("model")

			client := newClient()
			fm := newFormatter()

			req := map[string]any{
				"agent_name": agentName,
				"goal":       goal,
			}
			if model != "" {
				req["model_id"] = model
			}

			result, err := client.CreateSession(req)
			if err != nil {
				return err
			}

			return fm.PrintTable(result, []string{"id", "status", "api_key", "created_at"})
		},
	}

	cmd.Flags().String("goal", "", "Task description for the agent")
	cmd.Flags().String("agent-name", "agent", "Name for the agent")
	cmd.Flags().String("model", "", "LLM model to use")

	return cmd
}

func newSessionListCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List agent sessions",
		RunE: func(cmd *cobra.Command, args []string) error {
			client := newClient()
			fm := newFormatter()

			status, _ := cmd.Flags().GetString("status")
			limit, _ := cmd.Flags().GetInt("limit")

			results, err := client.ListSessions()
			if err != nil {
				return err
			}

			// Apply --status filter (SPEC-016 §5.3)
			if status != "" {
				statuses := strings.Split(status, ",")
				var filtered []map[string]any
				for _, s := range results {
					sStatus := valString(s["status"])
					for _, want := range statuses {
						if strings.EqualFold(strings.TrimSpace(want), sStatus) {
							filtered = append(filtered, s)
							break
						}
					}
				}
				results = filtered
			}

			// Apply --limit (SPEC-016 §5.3)
			if limit > 0 && len(results) > limit {
				results = results[:limit]
			}

			return fm.PrintTable(results, []string{"id", "agent_name", "status", "iteration", "created_at"})
		},
	}

	cmd.Flags().String("status", "", "Filter by status (comma-separated: idle,thinking,paused)")
	cmd.Flags().Int("limit", 0, "Max sessions to show (default: unlimited)")

	return cmd
}

func newSessionShowCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "show <session-id>",
		Short: "Show session details",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client := newClient()
			fm := newFormatter()

			result, err := client.GetSession(args[0])
			if err != nil {
				return err
			}

			return fm.Print(result)
		},
	}
	return cmd
}

func newSessionLogsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "logs <session-id>",
		Short: "Show session iteration history",
		Long: `Tail session iteration history. Use --follow for polling-based live updates.

With --follow enabled, polls the server every 3 seconds for new iterations.
For true SSE-based real-time streaming, connect to the server's SSE endpoint
directly: GET /api/v1/events?session_id=<session-id>`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client := newClient()
			fm := newFormatter()

			follow, _ := cmd.Flags().GetBool("follow")
			iterations, _ := cmd.Flags().GetInt("iterations")

			results, err := client.ListIterations(args[0])
			if err != nil {
				return err
			}

			// Apply --iterations limit (SPEC-016 §5.3)
			if iterations > 0 && len(results) > iterations {
				results = results[len(results)-iterations:]
			}

			// When iteration_commits is empty, print an explanatory message instead
			// of the generic "(no results)" — the session may have failed or yielded
			// no committed iterations. Point users to the SSE stream for live events.
			if len(results) == 0 {
				fm.PrintText("No iteration commits yet — the session produced no committed iterations (failed or LLM-less run).\n")
				fm.PrintText("Stream live events via SSE: GET /api/v1/events?session_id=%s\n", args[0])
				return nil
			}

			if follow {
				return followSessionLogs(client, fm, args[0], results, iterations)
			}

			return fm.PrintTable(results, []string{"iteration_id", "session_id", "rows_affected", "created_at"})
		},
	}

	cmd.Flags().Bool("follow", false, "Poll for live iteration updates every 3s")
	cmd.Flags().Int("iterations", 0, "Show last N iterations (default: all)")

	return cmd
}

// followSessionLogs polls the server for new iterations and prints them
// as they appear. Falls back to SSE-based streaming when the server provides it.
func followSessionLogs(client *Client, fm *Formatter, sessionID string, initial []map[string]any, maxIterations int) error {
	// Print initial results
	if len(initial) > 0 {
		fm.PrintText("=== Current iterations ===\n")
		fm.PrintTable(initial, []string{"iteration_id", "session_id", "rows_affected", "created_at"})
	}

	lastCount := len(initial)
	fm.PrintText("\n=== Waiting for new iterations (Ctrl+C to stop) ===\n")

	// Poll every 3 seconds for new iterations
	for {
		select {
		case <-time.After(3 * time.Second):
			results, err := client.ListIterations(sessionID)
			if err != nil {
				// Don't exit on transient errors, just warn
				fm.PrintText("Poll error: %v (retrying)\n", err)
				continue
			}

			if len(results) > lastCount {
				newOnes := results[lastCount:]
				for _, iter := range newOnes {
					fm.PrintTable([]map[string]any{iter}, []string{"iteration_id", "session_id", "rows_affected", "created_at"})
				}
				lastCount = len(results)

				// Stop if we've exceeded the max
				if maxIterations > 0 && lastCount >= maxIterations {
					return nil
				}
			}
		}
	}
}

func newSessionPauseCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "pause <session-id>",
		Short: "Pause a running session",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client := newClient()
			fm := newFormatter()

			// The API expects the action verb "pause" (→ target status "paused"),
			// not the target state (DOGFOOD-002).
			result, err := client.UpdateSession(args[0], map[string]any{"status": "pause"})
			if err != nil {
				return err
			}

			return fm.Print(result)
		},
	}
	return cmd
}

func newSessionResumeCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "resume <session-id>",
		Short: "Resume a paused session",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client := newClient()
			fm := newFormatter()

			// The API expects the action verb "resume" (→ target status "idle"),
			// not the target state (DOGFOOD-002).
			result, err := client.UpdateSession(args[0], map[string]any{"status": "resume"})
			if err != nil {
				return err
			}

			return fm.Print(result)
		},
	}
	return cmd
}

func newSessionCancelCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "cancel <session-id>",
		Short: "Cancel and fail a session",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client := newClient()
			fm := newFormatter()

			fm.Println("Cancelling session:", args[0])
			if err := client.DeleteSession(args[0]); err != nil {
				return err
			}

			fm.Println("Session cancelled.")
			return nil
		},
	}
	return cmd
}

func newSessionCostCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "cost <session-id>",
		Short: "Show session cost breakdown",
		Long:  `Show per-iteration cost breakdown for a session, including total cost summary.`,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client := newClient()
			fm := newFormatter()

			result, err := client.GetSessionBilling(args[0])
			if err != nil {
				return err
			}

			// Print summary header (always visible unless quiet)
			if !optQuiet {
				sessionID := valString(result["session_id"])
				totalCost := result["total_cost_usd"]
				totalPrompt := result["total_prompt_tokens"]
				totalCompletion := result["total_completion_tokens"]
				fm.PrintText("Session: %s\n", sessionID)
				fm.PrintText("Total cost: $%.6f\n", totalCost)
				fm.PrintText("Total prompt tokens: %v\n", totalPrompt)
				fm.PrintText("Total completion tokens: %v\n\n", totalCompletion)
			}

			// Print per-iteration entries as a table
			if entries, ok := result["entries"].([]any); ok {
				var rows []map[string]any
				for _, e := range entries {
					if entry, ok := e.(map[string]any); ok {
						rows = append(rows, entry)
					}
				}
				return fm.PrintTable(rows, []string{"id", "iteration", "model_id", "category", "prompt_tokens", "completion_tokens", "cost_usd"})
			}

			return fm.Print(result)
		},
	}
	return cmd
}
