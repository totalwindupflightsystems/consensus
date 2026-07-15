// models command: sync model_registry from models.dev
package cli

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/wojons/consensus/internal/config"
	dbdriver "github.com/wojons/consensus/internal/db/driver"
	"github.com/wojons/consensus/internal/migrate"
	"github.com/wojons/consensus/internal/modelsync"
)

func newModelsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "models",
		Short: "Manage the model registry",
		Long:  "Sync model_registry from models.dev, list models, or manage static entries.",
	}
	cmd.AddCommand(newModelsSyncCmd())
	return cmd
}

func newModelsSyncCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "sync",
		Short: "Sync model_registry from models.dev",
		Long: `Fetch the latest model listing from models.dev and update the
model_registry table. Entries marked sync_source='static' are never
overwritten. Use --auto-sync with serve to keep models current.`,
		RunE: runModelsSync,
	}
	cmd.Flags().String("db-url", "", "Database URL (overrides config)")
	return cmd
}

func runModelsSync(cmd *cobra.Command, _ []string) error {
	ctx := context.Background()

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("config: %w", err)
	}
	if dbURL, _ := cmd.Flags().GetString("db-url"); dbURL != "" {
		cfg.Database.URL = dbURL
	}

	database, err := dbdriver.Open(ctx, cfg.Database)
	if err != nil {
		return fmt.Errorf("db: %w", err)
	}
	defer database.Close()

	adminDB := dbdriver.AdminDB(database)

	// Run migrations to ensure sync_source column exists
	migrator := migrate.New(adminDB)
	if _, err := migrator.AutoMigrate(ctx); err != nil {
		return fmt.Errorf("migrate: %w", err)
	}

	syncer := modelsync.New(database)
	result, err := syncer.Sync(ctx)
	if err != nil {
		return fmt.Errorf("sync: %w", err)
	}

	fmt.Printf("Models synced from models.dev:\n")
	fmt.Printf("  Added:   %d\n", result.Added)
	fmt.Printf("  Updated: %d\n", result.Updated)
	fmt.Printf("  Removed: %d\n", result.Removed)
	if len(result.Errors) > 0 {
		fmt.Printf("  Errors:  %d\n", len(result.Errors))
		for _, e := range result.Errors {
			fmt.Printf("    - %s\n", e)
		}
	}
	return nil
}
