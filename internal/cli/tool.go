// tool & skill commands — inspect available tools and skills.
//
// axiom:trace work_item=interfaces-api-cli-01 spec=specs/016-cli-interface.md plan=phase-4/task-4-1/step-4-1-5 impl=internal/cli/tool.go
package cli

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

func newToolCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "tool",
		Short: "Register and inspect tools",
		Long: `Register tools in the tools_registry table and inspect the registered set.

A tool row is what makes a capability dispatchable: the harness routes tool
requests through the registry, and rows with requires_approval=true are what
the HITL approval gate fires on.`,
	}

	cmd.AddCommand(
		newToolListCmd(),
		newToolShowCmd(),
		newToolRegisterCmd(),
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
			fmt.SetEmptyHint("No tools registered yet — run `consensus tool register <name> --description ... --handler-ref ...` to add one")

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
			fm := newFormatter()

			// List all tools and find the matching one
			results, err := client.ListTools()
			if err != nil {
				return err
			}

			for _, tool := range results {
				if tool["name"] == args[0] {
					return fm.Print(tool)
				}
			}

			// A missing tool is a failure, not an empty result: the verb's
			// output contract is data (contrast `tool list`, which exits 0 on
			// an empty registry) (SPEC-016 §8 → exit 5, DOC-10).
			return fmt.Errorf("NOT_FOUND: tool %q not found", args[0])
		},
	}
}

// newToolRegisterCmd registers a tool row via POST /api/v1/tools
// (DF-CONSENSUS-62). Registration requires an admin-scoped API key: the row
// decides what executable capability the runtime dispatches.
func newToolRegisterCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "register <name>",
		Short: "Register a tool in the registry",
		Long: `Register a tool row so the harness can dispatch it.

A tool with --requires-approval will park every execution request for human
review before it runs (the HITL approval gate). Handler types:

  sql_function   handler-ref is a SQL function called as SELECT ref($1)
  go_native      handler-ref names a Go handler in the runtime
  http_endpoint  handler-ref is the endpoint URL
  subprocess     handler-ref is "executable arg1 arg2"

Examples:

  consensus tool register list_sessions \
    --description "List the most recent sessions" \
    --handler-type go_native --handler-ref list_sessions

  consensus tool register purge_cache \
    --description "Flush the response cache" \
    --handler-type subprocess --handler-ref "purge_cache --all" \
    --requires-approval`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			description, _ := cmd.Flags().GetString("description")
			handlerRef, _ := cmd.Flags().GetString("handler-ref")
			handlerType, _ := cmd.Flags().GetString("handler-type")
			hemisphere, _ := cmd.Flags().GetString("hemisphere")
			status, _ := cmd.Flags().GetString("status")
			paramSchema, _ := cmd.Flags().GetString("parameter-schema")
			requiresApproval, _ := cmd.Flags().GetBool("requires-approval")
			disabled, _ := cmd.Flags().GetBool("disabled")
			rateLimit, _ := cmd.Flags().GetInt("rate-limit")

			// Fail locally on the fields the API would reject anyway: a round
			// trip cannot tell the user what is missing any better.
			if strings.TrimSpace(description) == "" {
				return fmt.Errorf("--description is required")
			}
			if strings.TrimSpace(handlerRef) == "" {
				return fmt.Errorf("--handler-ref is required")
			}

			payload := map[string]any{
				"name":              name,
				"description":       description,
				"handler_ref":       handlerRef,
				"requires_approval": requiresApproval,
			}
			if handlerType != "" {
				payload["handler_type"] = handlerType
			}
			if hemisphere != "" {
				payload["hemisphere"] = hemisphere
			}
			if status != "" {
				payload["status"] = status
			}
			if disabled {
				payload["enabled"] = false
			}
			if rateLimit > 0 {
				payload["rate_limit_per_min"] = rateLimit
			}
			if strings.TrimSpace(paramSchema) != "" {
				var schema map[string]any
				if err := json.Unmarshal([]byte(paramSchema), &schema); err != nil {
					return fmt.Errorf("--parameter-schema must be a JSON object: %w", err)
				}
				payload["parameter_schema"] = schema
			}

			client := newClient()
			fm := newFormatter()

			created, err := client.CreateTool(payload)
			if err != nil {
				return err
			}
			return fm.Print(created)
		},
	}

	cmd.Flags().String("description", "", "Tool description (required)")
	cmd.Flags().String("handler-ref", "", "Handler reference: SQL function, Go handler, URL, or executable (required)")
	cmd.Flags().String("handler-type", "", "Handler type: sql_function (default), http_endpoint, go_native, subprocess")
	cmd.Flags().String("hemisphere", "", "Hemisphere: internal (default) or external")
	cmd.Flags().String("status", "", "Status: active (default), testing, deprecated, disabled")
	cmd.Flags().String("parameter-schema", "", `JSON Schema for the tool's parameters, e.g. '{"type":"object"}'`)
	cmd.Flags().Bool("requires-approval", false, "Require HITL human approval before the tool executes")
	cmd.Flags().Bool("disabled", false, "Register the tool disabled")
	cmd.Flags().Int("rate-limit", 0, "Per-minute execution limit (0 = unlimited)")

	return cmd
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
