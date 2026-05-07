package alembic_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Melphins/msync/internal/adapter"
	"github.com/Melphins/msync/internal/adapter/alembic"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAlembicAdapter_NameAndType(t *testing.T) {
	a := alembic.NewAlembicAdapter("", "")
	assert.Equal(t, adapter.AdapterTypeAlembic, a.Type())
	assert.Equal(t, "Alembic", a.Name())
}

func TestAlembicAdapter_MigrationTable(t *testing.T) {
	a := alembic.NewAlembicAdapter("", "")
	assert.Equal(t, "alembic_version", a.MigrationTable())
}

func TestAlembicAdapter_IsMigrationFile(t *testing.T) {
	a := alembic.NewAlembicAdapter("", "")

	// SQL migrations
	assert.True(t, a.IsMigrationFile("0001_initial.sql"))
	assert.True(t, a.IsMigrationFile("abc123_add_user.sql"))
	assert.True(t, a.IsMigrationFile("/path/to/versions/def456_add_email.sql"))

	// Python migrations
	assert.True(t, a.IsMigrationFile("versions/1234abcd_add_user.py"))
	assert.True(t, a.IsMigrationFile("1234abcd_add_user.py"))

	// Non-migrations
	assert.False(t, a.IsMigrationFile("README.md"))
	assert.False(t, a.IsMigrationFile("setup.py"))
	assert.False(t, a.IsMigrationFile("model.py"))
}

func TestAlembicAdapter_ParseMigrationFile_SQL(t *testing.T) {
	tmpDir := t.TempDir()
	migrationFile := filepath.Join(tmpDir, "abc123_add_users.sql")
	content := "-- Add users table\nCREATE TABLE users (id SERIAL PRIMARY KEY);"
	err := os.WriteFile(migrationFile, []byte(content), 0644)
	require.NoError(t, err)

	a := alembic.NewAlembicAdapter("", "")
	migration, err := a.ParseMigrationFile(migrationFile)
	require.NoError(t, err)

	assert.Equal(t, "abc123", migration.Version)
	assert.Equal(t, "add_users", migration.Name)
	assert.Equal(t, migrationFile, migration.FilePath)
	assert.Equal(t, content, migration.UpSQL)
	assert.NotEmpty(t, migration.Checksum)
}

func TestAlembicAdapter_ParseMigrationFile_Python(t *testing.T) {
	tmpDir := t.TempDir()
	migrationFile := filepath.Join(tmpDir, "abc123_add_users.py")
	content := `"""add users

Revision ID: abc123
Revises: previous_rev
Create Date: 2025-01-01

"""
from alembic import op
import sqlalchemy as sa

def upgrade():
    op.create_table('users', sa.Column('id', sa.Integer, primary_key=True))

def downgrade():
    op.drop_table('users')
`
	err := os.WriteFile(migrationFile, []byte(content), 0644)
	require.NoError(t, err)

	a := alembic.NewAlembicAdapter("", "")
	migration, err := a.ParseMigrationFile(migrationFile)
	// Python parsing not yet implemented
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not yet implemented")
	assert.Equal(t, adapter.Migration{}, migration)
}

func TestAlembicAdapter_ParseMigrationFile_Invalid(t *testing.T) {
	a := alembic.NewAlembicAdapter("", "")

	// Invalid extension
	migration, err := a.ParseMigrationFile("/path/to/README.md")
	assert.Error(t, err)
	assert.Equal(t, adapter.Migration{}, migration)
}

func TestAlembicAdapter_LoadMigrations(t *testing.T) {
	tmpDir := t.TempDir()
	versionsDir := filepath.Join(tmpDir, "versions")
	err := os.MkdirAll(versionsDir, 0755)
	require.NoError(t, err)

	// Create test migrations (Alembic typically puts them in versions/)
	migrations := []struct {
		name    string
		content string
	}{
		{"0001_initial.sql", "CREATE TABLE users (id SERIAL PRIMARY KEY);"},
		{"0002_add_email.sql", "ALTER TABLE users ADD COLUMN email VARCHAR(255);"},
		{"abc123_add_role.sql", "ALTER TABLE users ADD COLUMN role VARCHAR(50);"},
		{"def456_add_status.py", "print('python migration')"}, // Will be skipped by parser
	}

	for _, m := range migrations {
		filePath := filepath.Join(versionsDir, m.name)
		err := os.WriteFile(filePath, []byte(m.content), 0644)
		require.NoError(t, err)
	}

	a := alembic.NewAlembicAdapter("", versionsDir)
	err = a.LoadMigrations()
	require.NoError(t, err)

	migs, err := a.GetMigrations()
	require.NoError(t, err)

	// Should load 3 SQL migrations (Python file returns error on parse and is skipped)
	assert.Len(t, migs, 3)
	assert.Equal(t, "0001", migs[0].Version)
	assert.Equal(t, "0002", migs[1].Version)
	assert.Equal(t, "abc123", migs[2].Version)
}

func TestAlembicAdapter_Diff(t *testing.T) {
	current := &adapter.Schema{
		Tables: map[string]adapter.SchemaTable{
			"alembic_version": {
				Name: "alembic_version",
				Columns: []adapter.SchemaColumn{
					{Name: "version_num", Type: "character varying", Nullable: false},
				},
			},
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
			"alembic_version": {
				Name: "alembic_version",
				Columns: []adapter.SchemaColumn{
					{Name: "version_num", Type: "character varying", Nullable: false},
				},
			},
			"users": {
				Name: "users",
				Columns: []adapter.SchemaColumn{
					{Name: "id", Type: "integer"},
					{Name: "email", Type: "varchar(255)"}, // Modified
					{Name: "role", Type: "varchar(50)"},    // Added
				},
			},
		},
	}

	a := &alembic.AlembicAdapter{}
	diff, err := a.Diff(current, target)
	require.NoError(t, err)

	assert.Len(t, diff.TablesAdded, 0)
	assert.Len(t, diff.TablesRemoved, 0)
	assert.Len(t, diff.ColumnsAdded, 1)
	assert.Equal(t, "role", diff.ColumnsAdded[0].Column)
	assert.Len(t, diff.ColumnsModified, 1)
	assert.Equal(t, "email", diff.ColumnsModified[0].Column)
	assert.Equal(t, "varchar(100)", diff.ColumnsModified[0].OldType)
	assert.Equal(t, "varchar(255)", diff.ColumnsModified[0].NewType)
}
