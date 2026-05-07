package status

import (
	"context"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/Melphins/msync/internal/adapter"
	"github.com/Melphins/msync/internal/adapter/registry"
	"github.com/Melphins/msync/internal/config"
)

// Runner executes the status command
type Runner struct {
	cfg *config.Config
}

// NewRunner creates a new status runner
func NewRunner(cfg *config.Config) *Runner {
	return &Runner{cfg: cfg}
}

// SyncStatus represents the synchronization status between local and target
type SyncStatus string

const (
	SyncStatusSynced    SyncStatus = "synced"
	SyncStatusOutOfSync SyncStatus = "out_of_sync"
	SyncStatusAhead     SyncStatus = "ahead"
)

// CheckResult contains the result of a status check
type CheckResult struct {
	LocalVersion    string
	TargetVersion   string
	LocalCount      int
	TargetCount     int
	Status          SyncStatus
	PendingMigrations []adapter.Migration
	Notes           string
}

// Run executes the status check and returns structured result
func (r *Runner) Run(ctx context.Context, targetName string, format string) (*CheckResult, error) {
	// Resolve target config
	var targetConfig *config.TargetConfig
	if targetName == "" {
		if len(r.cfg.Targets) == 0 {
			return nil, fmt.Errorf("no targets configured")
		}
		// Use first target
		for _, t := range r.cfg.Targets {
			targetConfig = &t
			break
		}
	} else {
		target, exists := r.cfg.Targets[targetName]
		if !exists {
			return nil, fmt.Errorf("target '%s' not found in configuration", targetName)
		}
		targetConfig = &target
	}

	// Create local adapter
	localAdapter, err := r.createLocalAdapter()
	if err != nil {
		return nil, fmt.Errorf("failed to create local adapter: %w", err)
	}

	// Create target adapter
	targetAdapter, err := r.createTargetAdapter(*targetConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create target adapter: %w", err)
	}

	// Get local applied migrations (from database)
	localApplied, err := localAdapter.AppliedMigrations(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get local applied migrations: %w", err)
	}

	// Get target migrations (from migration directory)
	targetMigs, err := targetAdapter.GetMigrations()
	if err != nil {
		return nil, fmt.Errorf("failed to get target migrations: %w", err)
	}

	// Get current versions
	localVersion, err := localAdapter.CurrentVersion(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get local version: %w", err)
	}

	targetVersion, err := targetAdapter.CurrentVersion(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get target version: %w", err)
	}

	// Build result
	result := &CheckResult{
		LocalVersion:    localVersion,
		TargetVersion:   targetVersion,
		LocalCount:      len(localApplied),
		TargetCount:     len(targetMigs),
		PendingMigrations: []adapter.Migration{},
	}

	// Determine status
	localCount := len(localApplied)
	targetCount := len(targetMigs)

	switch {
	case targetCount > localCount:
		result.Status = SyncStatusOutOfSync
		result.Notes = fmt.Sprintf("%d target migrations vs %d local", targetCount, localCount)
		// Get pending migrations (those in target but not in local applied set)
		applied := make(map[string]struct{})
		for _, version := range localApplied {
			applied[version] = struct{}{}
		}
		for _, mig := range targetMigs {
			if _, ok := applied[mig.Version]; !ok {
				result.PendingMigrations = append(result.PendingMigrations, mig)
			}
		}
	case localCount > targetCount:
		result.Status = SyncStatusAhead
		result.Notes = fmt.Sprintf("%d local migrations vs %d target", localCount, targetCount)
	default:
		result.Status = SyncStatusSynced
		result.Notes = fmt.Sprintf("Both at %s", localVersion)
	}

	// Output based on format
	if format == "json" {
		r.outputJSON(result)
	} else {
		r.outputText(result)
	}

	return result, nil
}

func (r *Runner) createLocalAdapter() (adapter.Adapter, error) {
	adapterType := adapter.AdapterType(r.cfg.Local.Adapter)
	return registry.GetAdapter(adapterType, r.cfg.Local.Connection, r.cfg.Local.MigrationDir)
}

func (r *Runner) createTargetAdapter(cfg config.TargetConfig) (adapter.Adapter, error) {
	adapterType := adapter.AdapterType(cfg.Adapter)
	return registry.GetAdapter(adapterType, cfg.Connection, cfg.MigrationDir)
}

func (r *Runner) outputText(result *CheckResult) error {
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "LOCAL\tTARGET\tSTATUS\tNOTES")

	localCount := result.LocalCount
	targetCount := result.TargetCount

	if targetCount > localCount {
		fmt.Fprintf(w, "%s\t%s\t[WARNING] OUT OF SYNC\t%d target migrations vs %d local\n",
			result.LocalVersion, result.TargetVersion, targetCount, localCount)
	} else if localCount > targetCount {
		fmt.Fprintf(w, "%s\t%s\t[WARNING] AHEAD\t%d local migrations vs %d target\n",
			result.LocalVersion, result.TargetVersion, localCount, targetCount)
	} else {
		fmt.Fprintf(w, "%s\t%s\t SYNCED\tBoth at %s\n", result.LocalVersion, result.TargetVersion, result.LocalVersion)
	}

	w.Flush()

	// Show pending migrations if out of sync
	if result.Status == SyncStatusOutOfSync && len(result.PendingMigrations) > 0 {
		fmt.Println("\nPending migrations:")
		for i, mig := range result.PendingMigrations {
			fmt.Printf("  %d. %s - %s\n", i+1, mig.Version, mig.Name)
		}
	}

	return nil
}

func (r *Runner) outputJSON(result *CheckResult) error {
	// Simple JSON output
	fmt.Printf("{\n")
	fmt.Printf(`  "local": {`+"\n")
	fmt.Printf(`    "version": "%s",`+"\n", result.LocalVersion)
	fmt.Printf(`    "migration_count": %d`+"\n", result.LocalCount)
	fmt.Printf("  },\n")
	fmt.Printf(`  "target": {`+"\n")
	fmt.Printf(`    "version": "%s",`+"\n", result.TargetVersion)
	fmt.Printf(`    "migration_count": %d`+"\n", result.TargetCount)
	fmt.Printf("  },\n")
	fmt.Printf(`  "status": "%s"`+"\n", result.Status)
	if result.Notes != "" {
		fmt.Printf(`  ,"notes": "%s"`+"\n", result.Notes)
	}
	if result.Status == SyncStatusOutOfSync && len(result.PendingMigrations) > 0 {
		fmt.Printf(`  ,"pending_migrations": [`)
		for i, mig := range result.PendingMigrations {
			if i > 0 {
				fmt.Printf(",")
			}
			fmt.Printf(`{"version":"%s","name":"%s"}`, mig.Version, mig.Name)
		}
		fmt.Printf("]\n")
	}
	fmt.Printf("}\n")

	return nil
}
