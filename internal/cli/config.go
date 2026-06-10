// config command — configuration management.
//
// axiom:trace work_item=spec-016-hardening-01 spec=specs/016-cli-interface.md plan=phase-1/task-1/step-1 impl=internal/cli/config.go
package cli

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/spf13/cobra"
)

func newConfigCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Manage configuration",
		Long:  `List, get, set, and edit configuration values.`,
	}

	cmd.AddCommand(
		newConfigListCmd(),
		newConfigGetCmd(),
		newConfigSetCmd(),
		newConfigEditCmd(),
	)

	return cmd
}

func newConfigListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List all configuration",
		RunE: func(cmd *cobra.Command, args []string) error {
			client := newClient()
			fm := newFormatter()

			result, err := client.GetConfig()
			if err != nil {
				return err
			}

			return fm.Print(result)
		},
	}
}

func newConfigGetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "get <key>",
		Short: "Get a specific config value",
		Long: `Get a specific configuration value. Supports nested keys
using dot notation (e.g., 'llm.default_model', 'hitl.require_approval_for_destructive').`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client := newClient()
			fm := newFormatter()

			result, err := client.GetConfig()
			if err != nil {
				return err
			}

			// Support nested dot-notation lookup (SPEC-016 §5.6, sweep-019 gap)
			val := nestedGet(result, args[0])
			if val != nil {
				return fm.Print(val)
			}

			fm.Println("Key not found:", args[0])
			return nil
		},
	}
}

// nestedGet resolves a dot-separated key path against a nested map.
// For example, "llm.default_model" looks up result["llm"]["default_model"].
func nestedGet(m map[string]any, key string) any {
	parts := strings.Split(key, ".")
	if len(parts) == 0 {
		return nil
	}

	var current any = m
	for _, part := range parts {
		switch v := current.(type) {
		case map[string]any:
			if val, ok := v[part]; ok {
				current = val
			} else {
				return nil
			}
		default:
			return nil
		}
	}
	return current
}

func newConfigSetCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "set <key> <value>",
		Short: "Set a configuration value",
		Long: `Set a configuration value. Supports nested dot-notation keys
(e.g., 'llm.default_model', 'hitl.require_approval_for_destructive').`,
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			client := newClient()
			fm := newFormatter()

			// Build a nested payload from dot-notation key (SPEC-016 §5.6).
			// "llm.default_model" → {"llm": {"default_model": value}}
			payload := buildNestedMap(args[0], args[1])

			resp, err := client.patch("/api/v1/config", payload)
			if err != nil {
				return err
			}

			var result map[string]any
			if err := client.decodeBody(resp, &result); err != nil {
				return err
			}

			fm.Println("Config updated:", args[0])
			return fm.Print(result)
		},
	}
	return cmd
}

// buildNestedMap converts a dot-separated key and value into a nested map.
// "llm.default_model" + "gpt-4o" → {"llm": {"default_model": "gpt-4o"}}
func buildNestedMap(key, value string) map[string]any {
	parts := strings.Split(key, ".")
	if len(parts) == 0 {
		return map[string]any{key: value}
	}

	// Build from the inside out.
	result := map[string]any{parts[len(parts)-1]: value}
	for i := len(parts) - 2; i >= 0; i-- {
		result = map[string]any{parts[i]: result}
	}
	return result
}

func newConfigEditCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "edit",
		Short: "Edit configuration file",
		Long: `Open the configuration file in your editor ($EDITOR).

If no config file exists, opens a new file at ./conscience.yaml.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			configPath := "conscience.yaml"

			// Try the priority chain to find an existing file.
			for _, p := range []string{"conscience.yaml", "~/.conscience/config.yaml"} {
				path := p
				if strings.HasPrefix(p, "~/") {
					homeDir, _ := os.UserHomeDir()
					path = homeDir + p[1:]
				}
				if _, err := os.Stat(path); err == nil {
					configPath = path
					break
				}
			}

			editor := os.Getenv("EDITOR")
			if editor == "" {
				editor = "vi"
			}

			editCmd := exec.Command(editor, configPath)
			editCmd.Stdin = os.Stdin
			editCmd.Stdout = os.Stdout
			editCmd.Stderr = os.Stderr

			if err := editCmd.Run(); err != nil {
				return fmt.Errorf("editor: %w", err)
			}
			return nil
		},
	}
}
