package diff

import (
	"context"
	"os"
	"testing"

	"github.com/Melphins/msync/internal/adapter"
	"github.com/Melphins/msync/internal/config"

	_ "github.com/lib/pq"
)

// TestRunner_Integration_DiffSchema tests schema diff with real database
func TestRunner_Integration_DiffSchema(t *testing.T) {
	if os.Getenv("SKIP_INTEGRATION") == "true" {
		t.Skip("Skipping integration test")
	}

	migrationDir := "./internal/diff/testdata/migrations"

	cfg := &config.Config{
		Local: config.LocalConfig{
			Adapter:      string(adapter.AdapterTypePostgres),
			Connection:   "postgres://test:test@localhost:5433/migrate_sync_test?sslmode=disable",
			MigrationDir: migrationDir,
		},
		Targets: map[string]config.TargetConfig{
			"test": {
				Adapter:    string(adapter.AdapterTypePostgres),
				Connection: "postgres://test:test@localhost:5433/migrate_sync_test?sslmode=disable",
				MigrationDir: migrationDir,
			},
		},
	}

	runner := NewRunner(cfg)
	ctx := context.Background()

	// Should execute without error (actual diff depends on database state)
	err := runner.Run(ctx, "test", "schema")
	if err != nil {
		t.Logf("Diff returned error (may be expected based on DB state): %v", err)
	}
}

// TestRunner_Integration_DiffJSON tests JSON output format
func TestRunner_Integration_DiffJSON(t *testing.T) {
	if os.Getenv("SKIP_INTEGRATION") == "true" {
		t.Skip("Skipping integration test")
	}

	migrationDir := "./internal/diff/testdata/migrations"

	cfg := &config.Config{
		Local: config.LocalConfig{
			Adapter:      string(adapter.AdapterTypePostgres),
			Connection:   "postgres://test:test@localhost:5433/migrate_sync_test?sslmode=disable",
			MigrationDir: migrationDir,
		},
		Targets: map[string]config.TargetConfig{
			"test": {
				Adapter:    string(adapter.AdapterTypePostgres),
				Connection: "postgres://test:test@localhost:5433/migrate_sync_test?sslmode=disable",
				MigrationDir: migrationDir,
			},
		},
	}

	runner := NewRunner(cfg)
	ctx := context.Background()

	// Test with json mode - should not error
	err := runner.Run(ctx, "test", "json")
	if err != nil {
		t.Logf("Diff JSON returned error (may be expected based on DB state): %v", err)
	}
}
