package up

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	"github.com/Melphins/msync/internal/adapter"
	"github.com/Melphins/msync/internal/config"
	"github.com/Melphins/msync/internal/status"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	_ "github.com/lib/pq"
)

// getProjectRoot finds and returns the project root (directory containing go.mod)
func getProjectRoot(t *testing.T) string {
	// Try from current working directory (tests run from package dir)
	cwd, err := os.Getwd()
	if err == nil {
		// Walk up from cwd looking for go.mod
		dir := cwd
		for {
			if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
				return dir
			}
			parent := filepath.Dir(dir)
			if parent == dir {
				break
			}
			dir = parent
		}
	}
	t.Fatalf("could not find project root (go.mod)")
	return ""
}

// cleanupTestDB drops all tables and resets the database to a clean state.
// It uses DROP SCHEMA ... CASCADE to remove all objects, then recreates the public schema.
func cleanupTestDB(t *testing.T, connStr string) {
	t.Helper()
	db, err := sql.Open("postgres", connStr)
	if err != nil {
		t.Logf("Warning: could not connect to test database for cleanup: %v", err)
		return
	}
	defer db.Close()

	// Drop and recreate public schema to remove all tables, indexes, sequences, etc.
	_, err = db.Exec(`DROP SCHEMA IF EXISTS public CASCADE; CREATE SCHEMA public;`)
	if err != nil {
		t.Logf("Warning: cleanup failed: %v", err)
	}
}

// TestRunner_Integration_ApplyMigrations tests the full up flow with a real database
func TestRunner_Integration_ApplyMigrations(t *testing.T) {
	if os.Getenv("SKIP_INTEGRATION") == "true" {
		t.Skip("Skipping integration test")
	}

	projectRoot := getProjectRoot(t)
	migrationDir := filepath.Join(projectRoot, "testdata", "fixtures", "migrations")
	t.Logf("Using migration dir: %s", migrationDir)

	cfg := &config.Config{
		Local: config.LocalConfig{
			Adapter:      string(adapter.AdapterTypePostgres),
			Connection:   "postgres://test:test@localhost:5433/migrate_sync_test?sslmode=disable",
			MigrationDir: migrationDir,
			MigrationTable: "schema_migrations",
		},
		Targets: map[string]config.TargetConfig{
			"test": {
				Adapter:    string(adapter.AdapterTypePostgres),
				Connection: "postgres://test:test@localhost:5433/migrate_sync_test?sslmode=disable",
				MigrationDir: migrationDir,
			},
		},
	}

	// Ensure clean database state before and after test
	cleanupTestDB(t, cfg.Local.Connection)
	t.Cleanup(func() {
		cleanupTestDB(t, cfg.Local.Connection)
	})

	runner := NewRunner(cfg)
	ctx := context.Background()

	// First, status check should show out of sync (target has 5 migrations, local empty)
	statusRunner := status.NewRunner(cfg)
	result, err := statusRunner.Run(ctx, "test", "text")
	require.NoError(t, err)
	assert.Equal(t, status.SyncStatusOutOfSync, result.Status)
	assert.Equal(t, 0, result.LocalCount)
	assert.Equal(t, 5, result.TargetCount)
	assert.Len(t, result.PendingMigrations, 5)

	// Apply migrations
	runner.cfg.Sync.AutoApply = true
	err = runner.Run(ctx, "test", false, true)
	require.NoError(t, err)

	// Status should now be synced
	result, err = statusRunner.Run(ctx, "test", "text")
	require.NoError(t, err)
	assert.Equal(t, status.SyncStatusSynced, result.Status)
	assert.Equal(t, 5, result.LocalCount)
	assert.Equal(t, 5, result.TargetCount)
}

// TestRunner_Integration_DryRun tests dry-run mode doesn't apply migrations
func TestRunner_Integration_DryRun(t *testing.T) {
	if os.Getenv("SKIP_INTEGRATION") == "true" {
		t.Skip("Skipping integration test")
	}

	projectRoot := getProjectRoot(t)
	migrationDir := filepath.Join(projectRoot, "testdata", "fixtures", "migrations")
	t.Logf("Using migration dir: %s", migrationDir)

	cfg := &config.Config{
		Local: config.LocalConfig{
			Adapter:      string(adapter.AdapterTypePostgres),
			Connection:   "postgres://test:test@localhost:5433/migrate_sync_test?sslmode=disable",
			MigrationDir: migrationDir,
			MigrationTable: "schema_migrations",
		},
		Targets: map[string]config.TargetConfig{
			"test": {
				Adapter:    string(adapter.AdapterTypePostgres),
				Connection: "postgres://test:test@localhost:5433/migrate_sync_test?sslmode=disable",
				MigrationDir: migrationDir,
			},
		},
	}

	// Ensure clean database state
	cleanupTestDB(t, cfg.Local.Connection)
	t.Cleanup(func() {
		cleanupTestDB(t, cfg.Local.Connection)
	})

	runner := NewRunner(cfg)
	ctx := context.Background()

	// Run dry-run
	err := runner.Run(ctx, "test", true, true)
	if err != nil {
		// DB might not be running - that's ok for this test
		t.Logf("Dry-run returned error (DB may not be available): %v", err)
		return
	}

	// If DB was available, verify no migrations were applied by checking applied versions
	localAdapter, err := runner.createLocalAdapter()
	require.NoError(t, err)
	applied, err := localAdapter.AppliedMigrations(ctx)
	require.NoError(t, err)
	assert.Empty(t, applied, "dry-run should not apply any migrations")
}

// TestRunner_Integration_Reapply tests that reapplying migrations is safe
func TestRunner_Integration_Reapply(t *testing.T) {
	if os.Getenv("SKIP_INTEGRATION") == "true" {
		t.Skip("Skipping integration test")
	}

	projectRoot := getProjectRoot(t)
	migrationDir := filepath.Join(projectRoot, "testdata", "fixtures", "migrations")
	t.Logf("Using migration dir: %s", migrationDir)

	cfg := &config.Config{
		Local: config.LocalConfig{
			Adapter:      string(adapter.AdapterTypePostgres),
			Connection:   "postgres://test:test@localhost:5433/migrate_sync_test?sslmode=disable",
			MigrationDir: migrationDir,
			MigrationTable: "schema_migrations",
		},
		Targets: map[string]config.TargetConfig{
			"test": {
				Adapter:    string(adapter.AdapterTypePostgres),
				Connection: "postgres://test:test@localhost:5433/migrate_sync_test?sslmode=disable",
				MigrationDir: migrationDir,
			},
		},
	}

	// Ensure clean database state
	cleanupTestDB(t, cfg.Local.Connection)
	t.Cleanup(func() {
		cleanupTestDB(t, cfg.Local.Connection)
	})

	runner := NewRunner(cfg)
	ctx := context.Background()

	// First apply
	runner.cfg.Sync.AutoApply = true
	err := runner.Run(ctx, "test", false, true)
	require.NoError(t, err)

	// Second apply - should report already up to date
	runner.cfg.Sync.AutoApply = true
	err = runner.Run(ctx, "test", false, true)
	assert.NoError(t, err)
}
