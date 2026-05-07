package postgres_test

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/Melphins/msync/internal/adapter"
	"github.com/Melphins/msync/internal/adapter/postgres"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPostgresAdapter_CurrentVersion(t *testing.T) {
	// This test requires a real database
	// For unit tests, we test the parsing and logic without DB
	t.Skip("Requires running PostgreSQL database")
}

func TestPostgresAdapter_ParseMigrationFile(t *testing.T) {
	// Create test migration file
	tmpDir := t.TempDir()
	migrationFile := filepath.Join(tmpDir, "0001_initial_schema.sql")
	content := "-- Initial schema\nCREATE TABLE users (id SERIAL PRIMARY KEY);"
	err := os.WriteFile(migrationFile, []byte(content), 0644)
	require.NoError(t, err)

	pg := postgres.NewPostgresAdapter("", "schema_migrations", "")
	migration, err := pg.ParseMigrationFile(migrationFile)
	require.NoError(t, err)

	assert.Equal(t, "0001", migration.Version)
	assert.Equal(t, "initial_schema", migration.Name)
	assert.Equal(t, migrationFile, migration.FilePath)
	assert.Equal(t, content, migration.UpSQL)
	assert.NotEmpty(t, migration.Checksum)
	assert.False(t, migration.Timestamp.IsZero())
}

func TestPostgresAdapter_ParseMigrationFile_InvalidFilename(t *testing.T) {
	pg := postgres.NewPostgresAdapter("", "schema_migrations", "")

	// Invalid filename format
	migration, err := pg.ParseMigrationFile("/path/to/invalid_file.sql")
	assert.Error(t, err)
	assert.Equal(t, adapter.Migration{}, migration)
}

func TestPostgresAdapter_IsMigrationFile(t *testing.T) {
	pg := postgres.NewPostgresAdapter("", "schema_migrations", "")

	assert.True(t, pg.IsMigrationFile("0001_initial.sql"))
	assert.True(t, pg.IsMigrationFile("/full/path/0002_add_user.sql"))
	assert.True(t, pg.IsMigrationFile("0045_add_email_verified.sql"))
	assert.False(t, pg.IsMigrationFile("README.md"))
	assert.False(t, pg.IsMigrationFile("initial.sql"))
	assert.False(t, pg.IsMigrationFile("00_incomplete.sql"))
}

func TestPostgresAdapter_LoadMigrations(t *testing.T) {
	// Create test migration directory with multiple files
	tmpDir := t.TempDir()
	migrationsDir := filepath.Join(tmpDir, "migrations")
	err := os.MkdirAll(migrationsDir, 0755)
	require.NoError(t, err)

	// Create test migrations in order
	files := []struct {
		name    string
		content string
	}{
		{"0001_initial.sql", "CREATE TABLE users (id SERIAL PRIMARY KEY);"},
		{"0002_add_email.sql", "ALTER TABLE users ADD COLUMN email VARCHAR(255);"},
		{"0003_add_role.sql", "ALTER TABLE users ADD COLUMN role VARCHAR(50);"},
		{"0004_add_status.sql", "ALTER TABLE users ADD COLUMN status VARCHAR(20);"},
	}

	for _, f := range files {
		filePath := filepath.Join(migrationsDir, f.name)
		err := os.WriteFile(filePath, []byte(f.content), 0644)
		require.NoError(t, err)
	}

	pg := postgres.NewPostgresAdapter("", "schema_migrations", migrationsDir)
	err = pg.LoadMigrations()
	require.NoError(t, err)

	migrations, err := pg.GetMigrations()
	require.NoError(t, err)

	assert.Len(t, migrations, 4)
	assert.Equal(t, "0001", migrations[0].Version)
	assert.Equal(t, "0002", migrations[1].Version)
	assert.Equal(t, "0003", migrations[2].Version)
	assert.Equal(t, "0004", migrations[3].Version)
}

func TestPostgresAdapter_LoadMigrations_DirectoryDoesNotExist(t *testing.T) {
	pg := postgres.NewPostgresAdapter("", "schema_migrations", "/nonexistent")
	err := pg.LoadMigrations()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "does not exist")
}

func TestPostgresAdapter_LoadMigrations_InvalidMigration(t *testing.T) {
	tmpDir := t.TempDir()
	migrationsDir := filepath.Join(tmpDir, "migrations")
	err := os.MkdirAll(migrationsDir, 0755)
	require.NoError(t, err)

	// Create an invalid migration file
	invalidFile := filepath.Join(migrationsDir, "invalid.txt")
	err = os.WriteFile(invalidFile, []byte("not a sql file"), 0644)
	require.NoError(t, err)

	pg := postgres.NewPostgresAdapter("", "schema_migrations", migrationsDir)
	err = pg.LoadMigrations()
	// Should skip non-SQL files without error
	assert.NoError(t, err)

	migrations, err := pg.GetMigrations()
	require.NoError(t, err)
	assert.Len(t, migrations, 0)
}

func TestPostgresAdapter_FindMigrationByVersion(t *testing.T) {
	tmpDir := t.TempDir()
	migrationsDir := filepath.Join(tmpDir, "migrations")
	os.MkdirAll(migrationsDir, 0755)

	// Create test migrations
	for i := 1; i <= 5; i++ {
		filePath := filepath.Join(migrationsDir, fmt.Sprintf("%04d_test.sql", i))
		os.WriteFile(filePath, []byte("SELECT 1;"), 0644)
	}

	pg := postgres.NewPostgresAdapter("", "schema_migrations", migrationsDir)
	err := pg.LoadMigrations()
	require.NoError(t, err)

	// Test finding existing migration
	mig, err := pg.FindMigrationByVersion("0003")
	require.NoError(t, err)
	assert.Equal(t, "0003", mig.Version)

	// Test finding non-existent migration
	_, err = pg.FindMigrationByVersion("0099")
	assert.Error(t, err)
	assert.Equal(t, adapter.ErrMigrationNotFound, err)
}

func TestPostgresAdapter_MigrationTable(t *testing.T) {
	pg1 := postgres.NewPostgresAdapter("", "", "")
	assert.Equal(t, "schema_migrations", pg1.MigrationTable())

	pg2 := postgres.NewPostgresAdapter("", "custom_migrations", "")
	assert.Equal(t, "custom_migrations", pg2.MigrationTable())
}

func TestPostgresAdapter_NameAndType(t *testing.T) {
	pg := postgres.NewPostgresAdapter("", "schema_migrations", "")
	assert.Equal(t, adapter.AdapterTypePostgres, pg.Type())
	assert.Equal(t, "PostgreSQL", pg.Name())
}

func TestPostgresAdapter_Diff_ColumnAdded(t *testing.T) {
	current := &adapter.Schema{
		Tables: map[string]adapter.SchemaTable{
			"users": {
				Name: "users",
				Columns: []adapter.SchemaColumn{
					{Name: "id", Type: "integer"},
					{Name: "email", Type: "varchar(255)"},
				},
			},
		},
	}

	target := &adapter.Schema{
		Tables: map[string]adapter.SchemaTable{
			"users": {
				Name: "users",
				Columns: []adapter.SchemaColumn{
					{Name: "id", Type: "integer"},
					{Name: "email", Type: "varchar(255)"},
					{Name: "role", Type: "varchar(50)", Nullable: true},
				},
			},
		},
	}

	pg := &postgres.PostgresAdapter{}
	diff, err := pg.Diff(current, target)
	require.NoError(t, err)

	assert.Len(t, diff.TablesAdded, 0)
	assert.Len(t, diff.TablesRemoved, 0)
	assert.Len(t, diff.ColumnsAdded, 1)
	assert.Equal(t, "role", diff.ColumnsAdded[0].Column)
	assert.Equal(t, "varchar(50)", diff.ColumnsAdded[0].NewType)
	assert.True(t, diff.ColumnsAdded[0].NewNullable)
}

func TestPostgresAdapter_Diff_ColumnModified(t *testing.T) {
	current := &adapter.Schema{
		Tables: map[string]adapter.SchemaTable{
			"users": {
				Name: "users",
				Columns: []adapter.SchemaColumn{
					{Name: "id", Type: "integer"},
					{Name: "email", Type: "varchar(100)"},
				},
			},
		},
	}

	target := &adapter.Schema{
		Tables: map[string]adapter.SchemaTable{
			"users": {
				Name: "users",
				Columns: []adapter.SchemaColumn{
					{Name: "id", Type: "integer"},
					{Name: "email", Type: "varchar(255)"},
				},
			},
		},
	}

	pg := &postgres.PostgresAdapter{}
	diff, err := pg.Diff(current, target)
	require.NoError(t, err)

	assert.Len(t, diff.TablesAdded, 0)
	assert.Len(t, diff.TablesRemoved, 0)
	assert.Len(t, diff.ColumnsAdded, 0)
	assert.Len(t, diff.ColumnsModified, 1)
	assert.Equal(t, "email", diff.ColumnsModified[0].Column)
	assert.Equal(t, "varchar(100)", diff.ColumnsModified[0].OldType)
	assert.Equal(t, "varchar(255)", diff.ColumnsModified[0].NewType)
}

func TestPostgresAdapter_Diff_ColumnRemoved(t *testing.T) {
	current := &adapter.Schema{
		Tables: map[string]adapter.SchemaTable{
			"users": {
				Name: "users",
				Columns: []adapter.SchemaColumn{
					{Name: "id", Type: "integer"},
					{Name: "email", Type: "varchar(255)"},
					{Name: "deprecated_field", Type: "varchar(50)"},
				},
			},
		},
	}

	target := &adapter.Schema{
		Tables: map[string]adapter.SchemaTable{
			"users": {
				Name: "users",
				Columns: []adapter.SchemaColumn{
					{Name: "id", Type: "integer"},
					{Name: "email", Type: "varchar(255)"},
				},
			},
		},
	}

	pg := &postgres.PostgresAdapter{}
	diff, err := pg.Diff(current, target)
	require.NoError(t, err)

	assert.Len(t, diff.TablesAdded, 0)
	assert.Len(t, diff.TablesRemoved, 0)
	assert.Len(t, diff.ColumnsRemoved, 1)
	assert.Equal(t, "deprecated_field", diff.ColumnsRemoved[0].Column)
}

func TestPostgresAdapter_Diff_TableAdded(t *testing.T) {
	current := &adapter.Schema{
		Tables: map[string]adapter.SchemaTable{
			"users": {
				Name: "users",
				Columns: []adapter.SchemaColumn{{Name: "id", Type: "integer"}},
			},
		},
	}

	target := &adapter.Schema{
		Tables: map[string]adapter.SchemaTable{
			"users": {
				Name: "users",
				Columns: []adapter.SchemaColumn{{Name: "id", Type: "integer"}},
			},
			"posts": {
				Name: "posts",
				Columns: []adapter.SchemaColumn{{Name: "id", Type: "integer"}},
			},
		},
	}

	pg := &postgres.PostgresAdapter{}
	diff, err := pg.Diff(current, target)
	require.NoError(t, err)

	assert.Len(t, diff.TablesAdded, 1)
	assert.Contains(t, diff.TablesAdded, "posts")
}

func TestPostgresAdapter_Diff_Complex(t *testing.T) {
	current := &adapter.Schema{
		Tables: map[string]adapter.SchemaTable{
			"users": {
				Name: "users",
				Columns: []adapter.SchemaColumn{
					{Name: "id", Type: "integer", IsPrimaryKey: true},
					{Name: "email", Type: "varchar(100)"},
					{Name: "old_field", Type: "varchar(50)"},
				},
				PrimaryKey: []string{"id"},
			},
			"posts": {
				Name: "posts",
				Columns: []adapter.SchemaColumn{
					{Name: "id", Type: "integer", IsPrimaryKey: true},
					{Name: "title", Type: "varchar(255)"},
				},
				PrimaryKey: []string{"id"},
			},
		},
	}

	target := &adapter.Schema{
		Tables: map[string]adapter.SchemaTable{
			"users": {
				Name: "users",
				Columns: []adapter.SchemaColumn{
					{Name: "id", Type: "integer", IsPrimaryKey: true},
					{Name: "email", Type: "varchar(255)"}, // Modified
					{Name: "role", Type: "varchar(50)"},    // Added
				},
				PrimaryKey: []string{"id"},
			},
			"comments": {
				Name: "comments",
				Columns: []adapter.SchemaColumn{
					{Name: "id", Type: "integer"},
				},
				PrimaryKey: []string{"id"},
			},
		},
	}

	pg := &postgres.PostgresAdapter{}
	diff, err := pg.Diff(current, target)
	require.NoError(t, err)

	// Tables: comments added, posts removed
	assert.Len(t, diff.TablesAdded, 1)
	assert.Contains(t, diff.TablesAdded, "comments")
	assert.Len(t, diff.TablesRemoved, 1)
	assert.Contains(t, diff.TablesRemoved, "posts")

	// Columns in users table
	assert.Len(t, diff.ColumnsAdded, 1)
	assert.Equal(t, "role", diff.ColumnsAdded[0].Column)
	assert.Len(t, diff.ColumnsModified, 1)
	assert.Equal(t, "email", diff.ColumnsModified[0].Column)
	assert.Equal(t, "varchar(100)", diff.ColumnsModified[0].OldType)
	assert.Equal(t, "varchar(255)", diff.ColumnsModified[0].NewType)
}
