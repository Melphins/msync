package up

import (
	"context"
	"fmt"

	"github.com/Melphins/msync/internal/adapter"
	"github.com/Melphins/msync/internal/adapter/registry"
	"github.com/Melphins/msync/internal/config"
)

// Runner executes the up command
type Runner struct {
	cfg *config.Config
}

// NewRunner creates a new up runner
func NewRunner(cfg *config.Config) *Runner {
	return &Runner{cfg: cfg}
}

// Run executes the up command to apply pending migrations
func (r *Runner) Run(ctx context.Context, targetName string, dryRun bool, yes bool) error {
	// Resolve target config
	var targetConfig *config.TargetConfig
	if targetName == "" {
		if len(r.cfg.Targets) == 0 {
			return fmt.Errorf("no targets configured")
		}
		// Use first target
		for _, t := range r.cfg.Targets {
			targetConfig = &t
			break
		}
	} else {
		target, exists := r.cfg.Targets[targetName]
		if !exists {
			return fmt.Errorf("target '%s' not found in configuration", targetName)
		}
		targetConfig = &target
	}

	// Create local adapter (the database we're updating)
	localAdapter, err := r.createLocalAdapter()
	if err != nil {
		return fmt.Errorf("failed to create local adapter: %w", err)
	}

	// Create target adapter (the reference we're syncing from)
	targetAdapter, err := r.createTargetAdapter(*targetConfig)
	if err != nil {
		return fmt.Errorf("failed to create target adapter: %w", err)
	}

	// Load migrations from target (source of truth)
	targetMigs, err := targetAdapter.GetMigrations()
	if err != nil {
		return fmt.Errorf("failed to get target migrations: %w", err)
	}

	// Get current versions
	localVersion, err := localAdapter.CurrentVersion(ctx)
	if err != nil {
		return fmt.Errorf("failed to get local version: %w", err)
	}

	targetVersion, err := targetAdapter.CurrentVersion(ctx)
	if err != nil {
		return fmt.Errorf("failed to get target version: %w", err)
	}

	// Get applied migrations from local database
	localApplied, err := localAdapter.AppliedMigrations(ctx)
	if err != nil {
		return fmt.Errorf("failed to get local applied migrations: %w", err)
	}

	// Determine pending migrations (those in target but not in local database)
	pendingMigrations := r.findPendingMigrations(localApplied, targetMigs, localVersion, targetVersion)

	if len(pendingMigrations) == 0 {
		fmt.Println("[OK] Local database is already up to date with target.")
		return nil
	}

	// Display pending migrations
	fmt.Printf("Found %d pending migration(s):\n", len(pendingMigrations))
	for i, mig := range pendingMigrations {
		fmt.Printf("  %d. %s - %s\n", i+1, mig.Version, mig.Name)
	}

	// Dry run mode - just show what would be done
	if dryRun {
		fmt.Println("\n[DRY RUN] No changes will be applied")
		return nil
	}

	// Check if confirmation is required
	if !yes {
		fmt.Print("\n❓ Apply these migrations? (yes/no): ")
		var response string
		fmt.Scanln(&response)
		if response != "yes" && response != "y" {
			fmt.Println("Cancelled.")
			return nil
		}
	}

	// Apply migrations one by one with transaction per migration
	fmt.Println("\nApplying migrations...")
	for i, mig := range pendingMigrations {
		fmt.Printf("  [%d/%d] Applying %s - %s... ", i+1, len(pendingMigrations), mig.Version, mig.Name)

		// Check if we can apply
		canApply, reason, err := localAdapter.CanApply(ctx, mig)
		if err != nil {
			fmt.Printf("[ERROR]\n")
			return fmt.Errorf("failed to check migration %s: %w", mig.Version, err)
		}
		if !canApply {
			fmt.Printf("[SKIPPED] %s\n", reason)
			continue
		}

		// Apply the migration
		if err := localAdapter.Apply(ctx, mig); err != nil {
			fmt.Printf("[FAILED]\n")
			return fmt.Errorf("failed to apply migration %s: %w", mig.Version, err)
		}

		fmt.Println("")
	}

	fmt.Println("\n[SUCCESS] All pending migrations applied successfully!")
	return nil
}

// findPendingMigrations returns migrations that exist in target but not in local applied set
func (r *Runner) findPendingMigrations(localApplied []string, targetMigs []adapter.Migration, localVersion, targetVersion string) []adapter.Migration {
	// Build a set of applied local versions
	applied := make(map[string]struct{})
	for _, version := range localApplied {
		applied[version] = struct{}{}
	}

	// Find target migrations that haven't been applied
	var pending []adapter.Migration
	for _, mig := range targetMigs {
		if _, ok := applied[mig.Version]; !ok {
			pending = append(pending, mig)
		}
	}

	return pending
}

func (r *Runner) createLocalAdapter() (adapter.Adapter, error) {
	adapterType := adapter.AdapterType(r.cfg.Local.Adapter)
	return registry.GetAdapter(adapterType, r.cfg.Local.Connection, r.cfg.Local.MigrationDir)
}

func (r *Runner) createTargetAdapter(cfg config.TargetConfig) (adapter.Adapter, error) {
	adapterType := adapter.AdapterType(cfg.Adapter)
	return registry.GetAdapter(adapterType, cfg.Connection, cfg.MigrationDir)
}
