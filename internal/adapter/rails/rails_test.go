package rails_test

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/Melphins/msync/internal/adapter"
	"github.com/Melphins/msync/internal/adapter/rails"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRailsAdapter_NameAndType(t *testing.T) {
	r := rails.NewRailsAdapter("", "")
	assert.Equal(t, adapter.AdapterTypeRails, r.Type())
	assert.Equal(t, "Rails", r.Name())
}

func TestRailsAdapter_MigrationTable(t *testing.T) {
	r := rails.NewRailsAdapter("", "")
	assert.Equal(t, "schema_migrations", r.MigrationTable())
}

func TestRailsAdapter_IsMigrationFile(t *testing.T) {
	r := rails.NewRailsAdapter("", "")

	// Valid Rails migrations (YYYYMMDDHHMMSS_name.rb)
	assert.True(t, r.IsMigrationFile("20250101120000_create_users.rb"))
	assert.True(t, r.IsMigrationFile("20250506153045_add_email_to_users.rb"))
	assert.True(t, r.IsMigrationFile("/full/path/db/migrate/20240101000000_initial.rb"))
	assert.True(t, r.IsMigrationFile("20241231235959_add_index.rb"))

	// Invalid files
	assert.False(t, r.IsMigrationFile("README.md"))
	assert.False(t, r.IsMigrationFile("create_users.rb"))
	assert.False(t, r.IsMigrationFile("20250101_create_users.rb")) // Too short timestamp
	assert.False(t, r.IsMigrationFile("2025010112_create_users.rb")) // Not 14 digits
	assert.False(t, r.IsMigrationFile("20250101120000.rb")) // Missing name
}

func TestRailsAdapter_ParseMigrationFile(t *testing.T) {
	tmpDir := t.TempDir()
	migrationFile := filepath.Join(tmpDir, "20250101120000_create_users.rb")
	content := `class CreateUsers < ActiveRecord::Migration[7.0]
  def change
    create_table :users do |t|
      t.string :email, null: false
      t.timestamps
    end
  end
end`
	err := os.WriteFile(migrationFile, []byte(content), 0644)
	require.NoError(t, err)

	r := rails.NewRailsAdapter("", "")
	migration, err := r.ParseMigrationFile(migrationFile)
	require.NoError(t, err)

	assert.Equal(t, "20250101120000", migration.Version)
	assert.Equal(t, "create_users", migration.Name)
	assert.Equal(t, migrationFile, migration.FilePath)
	assert.Equal(t, content, migration.UpSQL)
	assert.NotEmpty(t, migration.Checksum)
	assert.False(t, migration.Timestamp.IsZero())
}

func TestRailsAdapter_ParseMigrationFile_InvalidFilename(t *testing.T) {
	r := rails.NewRailsAdapter("", "")

	// Invalid filename format
	migration, err := r.ParseMigrationFile("/path/to/invalid_file.rb")
	assert.Error(t, err)
	assert.Equal(t, adapter.Migration{}, migration)
}

func TestRailsAdapter_ParseMigrationFile_FileReadError(t *testing.T) {
	r := rails.NewRailsAdapter("", "")

	// Use a valid filename pattern but non-existent file
	migration, err := r.ParseMigrationFile("/nonexistent/db/migrate/20250101120000_test.rb")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to read")
	assert.Equal(t, adapter.Migration{}, migration)
}

func TestRailsAdapter_LoadMigrations(t *testing.T) {
	tmpDir := t.TempDir()
	migrateDir := filepath.Join(tmpDir, "db", "migrate")
	err := os.MkdirAll(migrateDir, 0755)
	require.NoError(t, err)

	// Create test Rails migrations (14-digit timestamp format)
	migrations := []struct {
		filename string
		content  string
	}{
		{
			"20250101000000_create_users.rb",
			"class CreateUsers < ActiveRecord::Migration[7.0]\n  def change\n    create_table :users do |t|\n      t.string :email\n    end\n  end\nend",
		},
		{
			"20250102000000_add_email_to_users.rb",
			"class AddEmailToUsers < ActiveRecord::Migration[7.0]\n  def change\n    add_column :users, :email, :string\n  end\nend",
		},
		{
			"20250103000000_add_index_to_users.rb",
			"class AddIndexToUsers < ActiveRecord::Migration[7.0]\n  def change\n    add_index :users, :email\n  end\nend",
		},
		{
			"20250104000000_rename_column.rb",
			"class RenameColumn < ActiveRecord::Migration[7.0]\n  def change\n    rename_column :users, :email, :email_address\n  end\nend",
		},
	}

	for _, m := range migrations {
		filePath := filepath.Join(migrateDir, m.filename)
		err := os.WriteFile(filePath, []byte(m.content), 0644)
		require.NoError(t, err)
	}

	r := rails.NewRailsAdapter("", migrateDir)
	err = r.LoadMigrations()
	require.NoError(t, err)

	migs, err := r.GetMigrations()
	require.NoError(t, err)

	assert.Len(t, migs, 4)
	assert.Equal(t, "20250101000000", migs[0].Version)
	assert.Equal(t, "20250102000000", migs[1].Version)
	assert.Equal(t, "20250103000000", migs[2].Version)
	assert.Equal(t, "20250104000000", migs[3].Version)
	assert.Equal(t, "create_users", migs[0].Name)
	assert.Equal(t, "add_email_to_users", migs[1].Name)
	assert.Equal(t, "add_index_to_users", migs[2].Name)
	assert.Equal(t, "rename_column", migs[3].Name)
}

func TestRailsAdapter_LoadMigrations_DirectoryDoesNotExist(t *testing.T) {
	r := rails.NewRailsAdapter("", "/nonexistent")
	err := r.LoadMigrations()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "does not exist")
}

func TestRailsAdapter_LoadMigrations_MigrationDirNotSet(t *testing.T) {
	r := rails.NewRailsAdapter("", "")
	err := r.LoadMigrations()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "migration directory not set")
}

func TestRailsAdapter_LoadMigrations_InvalidMigrationFile(t *testing.T) {
	tmpDir := t.TempDir()
	migrateDir := filepath.Join(tmpDir, "db", "migrate")
	os.MkdirAll(migrateDir, 0755)

	// Create a valid migration file
	validFile := filepath.Join(migrateDir, "20250101120000_valid.rb")
	os.WriteFile(validFile, []byte("class Valid < ActiveRecord::Migration[7.0]\n  def change\n  end\nend"), 0644)

	r := rails.NewRailsAdapter("", migrateDir)
	err := r.LoadMigrations()
	// Should succeed and load the valid file
	require.NoError(t, err)

	migs, err := r.GetMigrations()
	require.NoError(t, err)
	assert.Len(t, migs, 1)
	assert.Equal(t, "20250101120000", migs[0].Version)
}

func TestRailsAdapter_LoadMigrations_SortedByTimestamp(t *testing.T) {
	tmpDir := t.TempDir()
	migrateDir := filepath.Join(tmpDir, "db", "migrate")
	os.MkdirAll(migrateDir, 0755)

	// Create migrations out of order
	migrations := []struct {
		filename string
		content  string
	}{
		{"20250103000000_third.rb", "class Third < ActiveRecord::Migration[7.0]\n  def change\n  end\nend"},
		{"20250101000000_first.rb", "class First < ActiveRecord::Migration[7.0]\n  def change\n  end\nend"},
		{"20250201000000_fourth.rb", "class Fourth < ActiveRecord::Migration[7.0]\n  def change\n  end\nend"},
		{"20250102000000_second.rb", "class Second < ActiveRecord::Migration[7.0]\n  def change\n  end\nend"},
	}

	for _, m := range migrations {
		filePath := filepath.Join(migrateDir, m.filename)
		os.WriteFile(filePath, []byte(m.content), 0644)
	}

	r := rails.NewRailsAdapter("", migrateDir)
	err := r.LoadMigrations()
	require.NoError(t, err)

	migs, _ := r.GetMigrations()

	// Should be sorted by timestamp
	assert.Equal(t, "20250101000000", migs[0].Version)
	assert.Equal(t, "20250102000000", migs[1].Version)
	assert.Equal(t, "20250103000000", migs[2].Version)
	assert.Equal(t, "20250201000000", migs[3].Version)
}

func TestRailsAdapter_FindMigrationByVersion(t *testing.T) {
	tmpDir := t.TempDir()
	migrateDir := filepath.Join(tmpDir, "db", "migrate")
	os.MkdirAll(migrateDir, 0755)

	// Create test migrations with 14-digit timestamps
	for i := 1; i <= 5; i++ {
		// Format: YYYYMMDDHHMMSS - use 20250101 + 6 digit sequence
		seq := fmt.Sprintf("%06d", i)
		ver := "20250101" + seq
		filename := ver + "_test_" + string(rune('0'+i)) + ".rb"
		filePath := filepath.Join(migrateDir, filename)
		content := "class Test" + string(rune('0'+i)) + " < ActiveRecord::Migration[7.0]\n  def change\n  end\nend"
		os.WriteFile(filePath, []byte(content), 0644)
	}

	r := rails.NewRailsAdapter("", migrateDir)
	err := r.LoadMigrations()
	require.NoError(t, err)

	// Test finding existing migration (full 14-digit version)
	mig, err := r.FindMigrationByVersion("20250101000001")
	require.NoError(t, err)
	assert.Equal(t, "20250101000001", mig.Version)

	// Test finding another existing migration (5th one: 20250101000005)
	mig, err = r.FindMigrationByVersion("20250101000005")
	require.NoError(t, err)
	assert.Equal(t, "20250101000005", mig.Version)

	// Test finding non-existent migration
	_, err = r.FindMigrationByVersion("99999999999999")
	assert.Error(t, err)
	assert.Equal(t, adapter.ErrMigrationNotFound, err)
}

func TestRailsAdapter_Diff_NoDifferences(t *testing.T) {
	current := &adapter.Schema{
		Tables: map[string]adapter.SchemaTable{
			"users": {
				Name: "users",
				Columns: []adapter.SchemaColumn{
					{Name: "id", Type: "integer", IsPrimaryKey: true},
					{Name: "email", Type: "varchar(255)"},
				},
				PrimaryKey: []string{"id"},
			},
			"schema_migrations": {
				Name: "schema_migrations",
				Columns: []adapter.SchemaColumn{
					{Name: "version", Type: "character varying", Nullable: false},
				},
			},
		},
	}

	target := &adapter.Schema{
		Tables: map[string]adapter.SchemaTable{
			"users": {
				Name: "users",
				Columns: []adapter.SchemaColumn{
					{Name: "id", Type: "integer", IsPrimaryKey: true},
					{Name: "email", Type: "varchar(255)"},
				},
				PrimaryKey: []string{"id"},
			},
			"schema_migrations": {
				Name: "schema_migrations",
				Columns: []adapter.SchemaColumn{
					{Name: "version", Type: "character varying", Nullable: false},
				},
			},
		},
	}

	r := &rails.RailsAdapter{}
	diff, err := r.Diff(current, target)
	require.NoError(t, err)

	assert.Len(t, diff.TablesAdded, 0)
	assert.Len(t, diff.TablesRemoved, 0)
	assert.Len(t, diff.ColumnsAdded, 0)
	assert.Len(t, diff.ColumnsRemoved, 0)
	assert.Len(t, diff.ColumnsModified, 0)
}

func TestRailsAdapter_Diff_ColumnAdded(t *testing.T) {
	current := &adapter.Schema{
		Tables: map[string]adapter.SchemaTable{
			"users": {
				Name: "users",
				Columns: []adapter.SchemaColumn{
					{Name: "id", Type: "integer", IsPrimaryKey: true},
					{Name: "email", Type: "varchar(255)"},
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
					{Name: "email", Type: "varchar(255)"},
					{Name: "role", Type: "varchar(50)", Nullable: true},
				},
				PrimaryKey: []string{"id"},
			},
		},
	}

	r := &rails.RailsAdapter{}
	diff, err := r.Diff(current, target)
	require.NoError(t, err)

	assert.Len(t, diff.ColumnsAdded, 1)
	assert.Equal(t, "role", diff.ColumnsAdded[0].Column)
	assert.Equal(t, "varchar(50)", diff.ColumnsAdded[0].NewType)
	assert.True(t, diff.ColumnsAdded[0].NewNullable)
}

func TestRailsAdapter_Diff_ColumnRemoved(t *testing.T) {
	current := &adapter.Schema{
		Tables: map[string]adapter.SchemaTable{
			"users": {
				Name: "users",
				Columns: []adapter.SchemaColumn{
					{Name: "id", Type: "integer", IsPrimaryKey: true},
					{Name: "email", Type: "varchar(255)"},
					{Name: "deprecated_field", Type: "varchar(50)"},
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
					{Name: "email", Type: "varchar(255)"},
				},
				PrimaryKey: []string{"id"},
			},
		},
	}

	r := &rails.RailsAdapter{}
	diff, err := r.Diff(current, target)
	require.NoError(t, err)

	assert.Len(t, diff.ColumnsRemoved, 1)
	assert.Equal(t, "deprecated_field", diff.ColumnsRemoved[0].Column)
	assert.Equal(t, "varchar(50)", diff.ColumnsRemoved[0].OldType)
}

func TestRailsAdapter_Diff_ColumnModified(t *testing.T) {
	current := &adapter.Schema{
		Tables: map[string]adapter.SchemaTable{
			"users": {
				Name: "users",
				Columns: []adapter.SchemaColumn{
					{Name: "id", Type: "integer", IsPrimaryKey: true},
					{Name: "email", Type: "varchar(100)", Nullable: true},
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
					{Name: "email", Type: "varchar(255)", Nullable: false},
				},
				PrimaryKey: []string{"id"},
			},
		},
	}

	r := &rails.RailsAdapter{}
	diff, err := r.Diff(current, target)
	require.NoError(t, err)

	assert.Len(t, diff.ColumnsModified, 1)
	assert.Equal(t, "email", diff.ColumnsModified[0].Column)
	assert.Equal(t, "varchar(100)", diff.ColumnsModified[0].OldType)
	assert.Equal(t, "varchar(255)", diff.ColumnsModified[0].NewType)
	assert.True(t, diff.ColumnsModified[0].OldNullable)
	assert.False(t, diff.ColumnsModified[0].NewNullable)
}

func TestRailsAdapter_Diff_TableAdded(t *testing.T) {
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
				Columns: []adapter.SchemaColumn{
					{Name: "id", Type: "integer", IsPrimaryKey: true},
					{Name: "title", Type: "varchar(255)"},
				},
				PrimaryKey: []string{"id"},
			},
		},
	}

	r := &rails.RailsAdapter{}
	diff, err := r.Diff(current, target)
	require.NoError(t, err)

	assert.Len(t, diff.TablesAdded, 1)
	assert.Contains(t, diff.TablesAdded, "posts")
}

func TestRailsAdapter_Diff_TableRemoved(t *testing.T) {
	current := &adapter.Schema{
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

	target := &adapter.Schema{
		Tables: map[string]adapter.SchemaTable{
			"users": {
				Name: "users",
				Columns: []adapter.SchemaColumn{{Name: "id", Type: "integer"}},
			},
		},
	}

	r := &rails.RailsAdapter{}
	diff, err := r.Diff(current, target)
	require.NoError(t, err)

	assert.Len(t, diff.TablesRemoved, 1)
	assert.Contains(t, diff.TablesRemoved, "posts")
}

func TestRailsAdapter_Diff_Complex(t *testing.T) {
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
			"schema_migrations": {
				Name: "schema_migrations",
				Columns: []adapter.SchemaColumn{
					{Name: "version", Type: "character varying", Nullable: false},
				},
			},
		},
	}

	target := &adapter.Schema{
		Tables: map[string]adapter.SchemaTable{
			"users": {
				Name: "users",
				Columns: []adapter.SchemaColumn{
					{Name: "id", Type: "integer", IsPrimaryKey: true},
					{Name: "email", Type: "varchar(255)"},         // Modified
					{Name: "role", Type: "varchar(50)", Nullable: true}, // Added
				},
				PrimaryKey: []string{"id"},
			},
			"comments": {
				Name: "comments",
				Columns: []adapter.SchemaColumn{
					{Name: "id", Type: "integer", IsPrimaryKey: true},
					{Name: "body", Type: "text"},
				},
				PrimaryKey: []string{"id"},
			},
			"schema_migrations": {
				Name: "schema_migrations",
				Columns: []adapter.SchemaColumn{
					{Name: "version", Type: "character varying", Nullable: false},
				},
			},
		},
	}

	r := &rails.RailsAdapter{}
	diff, err := r.Diff(current, target)
	require.NoError(t, err)

	// Tables: comments added, posts removed
	assert.Len(t, diff.TablesAdded, 1)
	assert.Contains(t, diff.TablesAdded, "comments")
	assert.Len(t, diff.TablesRemoved, 1)
	assert.Contains(t, diff.TablesRemoved, "posts")

	// Columns in users table
	assert.Len(t, diff.ColumnsAdded, 1)
	assert.Equal(t, "role", diff.ColumnsAdded[0].Column)
	assert.Len(t, diff.ColumnsRemoved, 1)
	assert.Equal(t, "old_field", diff.ColumnsRemoved[0].Column)
	assert.Len(t, diff.ColumnsModified, 1)
	assert.Equal(t, "email", diff.ColumnsModified[0].Column)
	assert.Equal(t, "varchar(100)", diff.ColumnsModified[0].OldType)
	assert.Equal(t, "varchar(255)", diff.ColumnsModified[0].NewType)
}

func TestRailsAdapter_simpleChecksum(t *testing.T) {
	// Test that checksum is deterministic
	data1 := []byte("test data")
	data2 := []byte("test data")
	data3 := []byte("different data")

	// We can't call the unexported function directly, but we can verify
	// that the same file produces the same checksum through ParseMigrationFile
	tmpDir := t.TempDir()
	file1 := filepath.Join(tmpDir, "20250101000000_test1.rb")
	file2 := filepath.Join(tmpDir, "20250101000000_test2.rb")
	file3 := filepath.Join(tmpDir, "20250102000000_test3.rb")

	os.WriteFile(file1, data1, 0644)
	os.WriteFile(file2, data2, 0644)
	os.WriteFile(file3, data3, 0644)

	r := rails.NewRailsAdapter("", "")

	mig1, _ := r.ParseMigrationFile(file1)
	mig2, _ := r.ParseMigrationFile(file2)
	mig3, _ := r.ParseMigrationFile(file3)

	// Same content should produce same checksum
	assert.Equal(t, mig1.Checksum, mig2.Checksum)
	// Different content should produce different checksum
	assert.NotEqual(t, mig1.Checksum, mig3.Checksum)
}
