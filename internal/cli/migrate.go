// migrate command — database migration management.
//
// axiom:trace work_item=WI-010 spec=specs/016-cli-interface.md plan=.memory-bank/work-items/WI-010/plan.md impl=internal/cli/migrate.go
package cli

import (
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"
)

// MigrateFunc is set by the main package to enable direct database migration
// (without a running server). The action is one of "up", "down", or "status".
// When set, `consensus migrate --db-url <url>` runs directly against the DB.
// When nil, falls back to REST proxy mode.
var MigrateFunc func(action string, dbURL string) error

func newMigrateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "migrate",
		Short: "Run database migrations",
		Long: `Run all pending database migrations. This is an alias for 'consensus migrate run'.

The Consensus server auto-migrates on startup. This command enables
deliberate offline migration.

With --db-url, runs directly against the database (offline mode).
Without --db-url, sends requests to the running Consensus server.`,
		// SPEC-016 §5.5: bare `consensus migrate` runs all pending migrations.
		// §5.5 also says `consensus migrate` → run alias, `consensus migrate run` same.
		RunE: runMigrateRun,
	}

	// Add --db-url flag at the migrate command level so all subcommands inherit it.
	cmd.PersistentFlags().String("db-url", "", "Database URL for direct migration (e.g. sqlite://dev.db)")

	cmd.AddCommand(
		newMigrateRunCmd(),
		newMigrateVersionCmd(),
		newMigrateRollbackCmd(),
		newMigrateCreateCmd(),
	)

	return cmd
}

// migrateRun is the shared implementation for both `consensus migrate` and `consensus migrate run`.
func runMigrateRun(cmd *cobra.Command, args []string) error {
	dbURL, _ := cmd.Flags().GetString("db-url")

	// Direct mode: run migrations directly against the database.
	if dbURL != "" {
		if MigrateFunc == nil {
			return fmt.Errorf("direct migration not wired in this build; use the bare binary")
		}
		return MigrateFunc("up", dbURL)
	}

	// REST proxy mode: send request to running server.
	client := newClient()
	fm := newFormatter()

	resp, err := client.do("POST", "/api/v1/migrate", nil)
	if err != nil {
		return err
	}

	var result map[string]any
	if err := client.decodeBody(resp, &result); err != nil {
		return err
	}

	fm.Println("Schema version:", valString(result["version"]))
	fm.Println("Applied at:", valString(result["applied_at"]))
	return fm.Print(result)
}

func newMigrateRunCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "run",
		Short: "Run all pending migrations",
		Long:  `Run all pending database migrations (alias for 'consensus migrate').`,
		RunE:  runMigrateRun,
	}
}

func newMigrateVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Show current schema version",
		RunE: func(cmd *cobra.Command, args []string) error {
			dbURL, _ := cmd.Flags().GetString("db-url")

			// Direct mode: query the database directly.
			if dbURL != "" {
				if MigrateFunc == nil {
					return fmt.Errorf("direct migration not wired in this build; use the bare binary")
				}
				return MigrateFunc("status", dbURL)
			}

			// REST proxy mode.
			client := newClient()
			fm := newFormatter()

			result, err := client.Health()
			if err != nil {
				return err
			}

			if v, ok := result["schema_version"]; ok {
				fm.Println("Schema version:", v)
			} else {
				fm.Println("Schema version: (unknown — server does not report schema)")
			}
			return fm.Print(result)
		},
	}
}

func newMigrateRollbackCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "rollback",
		Short: "Rollback the last migration",
		RunE: func(cmd *cobra.Command, args []string) error {
			dbURL, _ := cmd.Flags().GetString("db-url")

			// Direct mode.
			if dbURL != "" {
				if MigrateFunc == nil {
					return fmt.Errorf("direct migration not wired in this build; use the bare binary")
				}
				return MigrateFunc("down", dbURL)
			}

			// REST proxy mode.
			client := newClient()

			resp, err := client.do("POST", "/api/v1/migrate/rollback", nil)
			if err != nil {
				return err
			}

			var result map[string]any
			if err := client.decodeBody(resp, &result); err != nil {
				return err
			}

			fmt.Println("Rollback complete.")
			return nil
		},
	}
}

func newMigrateCreateCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "create <name>",
		Short: "Create a new migration file",
		Long: `Create a new timestamped migration file in the migrations/ directory.

The file is created as '<timestamp>_<name>.sql' with up/down sections.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			timestamp := time.Now().UTC().Format("20060102150405")
			filename := fmt.Sprintf("%s_%s.sql", timestamp, args[0])
			dir := "migrations"
			path := dir + "/" + filename

			fm := newFormatter()

			// Ensure the migrations directory exists.
			if err := os.MkdirAll(dir, 0755); err != nil {
				return fmt.Errorf("cannot create migrations directory: %w", err)
			}

			content := fmt.Sprintf(`-- +goose Up
-- SQL in this section is executed when the migration is applied.

-- +goose Down
-- SQL in this section is executed when the migration is rolled back.

`)

			if err := os.WriteFile(path, []byte(content), 0644); err != nil {
				return fmt.Errorf("cannot create migration file: %w", err)
			}

			fm.Println("Created migration:", filename)
			return nil
		},
	}
}
