package diff

import (
	"context"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/Melphins/msync/internal/adapter"
	"github.com/Melphins/msync/internal/adapter/registry"
	"github.com/Melphins/msync/internal/config"
)

// Runner executes the diff command
type Runner struct {
	cfg *config.Config
}

// NewRunner creates a new diff runner
func NewRunner(cfg *config.Config) *Runner {
	return &Runner{cfg: cfg}
}

// Run executes the diff comparison
func (r *Runner) Run(ctx context.Context, targetName string, mode string) error {
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

	// Create local adapter
	localAdapter, err := r.createLocalAdapter()
	if err != nil {
		return fmt.Errorf("failed to create local adapter: %w", err)
	}

	// Create target adapter
	targetAdapter, err := r.createTargetAdapter(*targetConfig)
	if err != nil {
		return fmt.Errorf("failed to create target adapter: %w", err)
	}

	// Get current schemas from both databases
	localSchema, err := localAdapter.CurrentSchema(ctx)
	if err != nil {
		return fmt.Errorf("failed to get local schema: %w", err)
	}

	targetSchema, err := targetAdapter.CurrentSchema(ctx)
	if err != nil {
		return fmt.Errorf("failed to get target schema: %w", err)
	}

	// Calculate diff
	diff, err := localAdapter.Diff(localSchema, targetSchema)
	if err != nil {
		return fmt.Errorf("failed to calculate schema diff: %w", err)
	}

	// Output results
	if mode == "json" {
		return r.outputJSON(diff)
	}

	return r.outputText(diff)
}

func (r *Runner) createLocalAdapter() (adapter.Adapter, error) {
	adapterType := adapter.AdapterType(r.cfg.Local.Adapter)
	return registry.GetAdapter(adapterType, r.cfg.Local.Connection, r.cfg.Local.MigrationDir)
}

func (r *Runner) createTargetAdapter(cfg config.TargetConfig) (adapter.Adapter, error) {
	adapterType := adapter.AdapterType(cfg.Adapter)
	return registry.GetAdapter(adapterType, cfg.Connection, cfg.MigrationDir)
}

func (r *Runner) outputText(diff *adapter.SchemaDiff) error {
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)

	hasChanges := false

	// Tables to be added
	if len(diff.TablesAdded) > 0 {
		hasChanges = true
		fmt.Fprintln(w, "=== Tables to be ADDED ===")
		fmt.Fprintln(w)
		fmt.Fprintln(w, "Table")
		fmt.Fprintln(w, "-----")
		for _, table := range diff.TablesAdded {
			fmt.Fprintln(w, table)
		}
		fmt.Fprintln(w)
	}

	// Tables to be removed
	if len(diff.TablesRemoved) > 0 {
		hasChanges = true
		fmt.Fprintln(w, "=== Tables to be REMOVED ===")
		fmt.Fprintln(w)
		fmt.Fprintln(w, "Table")
		fmt.Fprintln(w, "-----")
		for _, table := range diff.TablesRemoved {
			fmt.Fprintln(w, table)
		}
		fmt.Fprintln(w)
	}

	// Columns to be added
	if len(diff.ColumnsAdded) > 0 {
		hasChanges = true
		fmt.Fprintln(w, "=== Columns to be ADDED ===")
		fmt.Fprintln(w)
		fmt.Fprintln(w, "Table\tColumn\tType\tNullable")
		fmt.Fprintln(w, "-----\t------\t----\t--------")
		for _, col := range diff.ColumnsAdded {
			nullable := "YES"
			if !col.NewNullable {
				nullable = "NO"
			}
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", col.Table, col.Column, col.OldType, nullable)
		}
		fmt.Fprintln(w)
	}

	// Columns to be removed
	if len(diff.ColumnsRemoved) > 0 {
		hasChanges = true
		fmt.Fprintln(w, "=== Columns to be REMOVED ===")
		fmt.Fprintln(w)
		fmt.Fprintln(w, "Table\tColumn\tType\tNullable")
		fmt.Fprintln(w, "-----\t------\t----\t--------")
		for _, col := range diff.ColumnsRemoved {
			nullable := "YES"
			if !col.OldNullable {
				nullable = "NO"
			}
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", col.Table, col.Column, col.OldType, nullable)
		}
		fmt.Fprintln(w)
	}

	// Columns to be modified
	if len(diff.ColumnsModified) > 0 {
		hasChanges = true
		fmt.Fprintln(w, "=== Columns to be MODIFIED ===")
		fmt.Fprintln(w)
		fmt.Fprintln(w, "Table\tColumn\tOld Type\tNew Type\tOld Nullable\tNew Nullable")
		fmt.Fprintln(w, "-----\t------\t---------\t---------\t------------\t------------")
		for _, col := range diff.ColumnsModified {
			oldNull := "YES"
			if !col.OldNullable {
				oldNull = "NO"
			}
			newNull := "YES"
			if !col.NewNullable {
				newNull = "NO"
			}
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n",
				col.Table, col.Column, col.OldType, col.NewType, oldNull, newNull)
		}
		fmt.Fprintln(w)
	}

	w.Flush()

	if !hasChanges {
		fmt.Println("Schemas are identical - no changes needed.")
		return nil
	}

	fmt.Println("\nTip: Use 'msync up' to apply these changes to your local database.")
	return nil
}

func (r *Runner) outputJSON(diff *adapter.SchemaDiff) error {
	// Simple JSON output
	fmt.Println("{")
	fmt.Printf(`  "tables_added": %v,` + "\n", toStringArray(diff.TablesAdded))
	fmt.Printf(`  "tables_removed": %v,` + "\n", toStringArray(diff.TablesRemoved))
	fmt.Printf(`  "columns_added": %v,` + "\n", toColumnDiffJSON(diff.ColumnsAdded))
	fmt.Printf(`  "columns_removed": %v,` + "\n", toColumnDiffJSON(diff.ColumnsRemoved))
	fmt.Printf(`  "columns_modified": %v` + "\n", toColumnDiffJSON(diff.ColumnsModified))
	fmt.Println("}")
	return nil
}

func toStringArray(arr []string) string {
	if len(arr) == 0 {
		return "[]"
	}
	result := "["
	for i, s := range arr {
		if i > 0 {
			result += ", "
		}
		result += fmt.Sprintf("%q", s)
	}
	return result + "]"
}

func toColumnDiffJSON(diffs []adapter.ColumnDiff) string {
	if len(diffs) == 0 {
		return "[]"
	}
	result := "["
	for i, d := range diffs {
		if i > 0 {
			result += ", "
		}
		result += fmt.Sprintf(`{"table":%q,"column":%q,"old_type":%q,"new_type":%q,"old_nullable":%t,"new_nullable":%t}`,
			d.Table, d.Column, d.OldType, d.NewType, d.OldNullable, d.NewNullable)
	}
	return result + "]"
}
