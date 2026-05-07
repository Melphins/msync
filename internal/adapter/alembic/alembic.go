package alembic

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Melphins/msync/internal/adapter"

	_ "github.com/lib/pq"
)

var (
	ErrNoMigrations = errors.New("no migrations found")
)

// AlembicAdapter implements adapter for Alembic (SQLAlchemy) migrations
type AlembicAdapter struct {
	connStr     string
	migrationDir string
	migrations  []adapter.Migration
}

// NewAlembicAdapter creates a new Alembic adapter
func NewAlembicAdapter(connStr, migrationDir string) *AlembicAdapter {
	return &AlembicAdapter{
		connStr:     connStr,
		migrationDir: migrationDir,
	}
}

// Type returns the adapter type
func (a *AlembicAdapter) Type() adapter.AdapterType {
	return adapter.AdapterTypeAlembic
}

// Name returns the adapter display name
func (a *AlembicAdapter) Name() string {
	return "Alembic"
}

// MigrationTable returns the alembic_version table name
func (a *AlembicAdapter) MigrationTable() string {
	return "alembic_version"
}

// Connect establishes a database connection
func (a *AlembicAdapter) Connect(ctx context.Context) (*sql.DB, error) {
	db, err := sql.Open("postgres", a.connStr)
	if err != nil {
		return nil, err
	}
	if err := db.PingContext(ctx); err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}
	return db, nil
}

// CurrentVersion returns the current Alembic version (revision ID)
func (a *AlembicAdapter) CurrentVersion(ctx context.Context) (string, error) {
	db, err := a.Connect(ctx)
	if err != nil {
		return "", err
	}
	defer db.Close()

	var version string
	query := "SELECT version_num FROM alembic_version"
	err = db.QueryRowContext(ctx, query).Scan(&version)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", nil
		}
		return "", fmt.Errorf("failed to get current version: %w", err)
	}
	return version, nil
}

// AppliedMigrations returns all applied migration versions (revisions)
func (a *AlembicAdapter) AppliedMigrations(ctx context.Context) ([]string, error) {
	db, err := a.Connect(ctx)
	if err != nil {
		return nil, err
	}
	defer db.Close()

	query := "SELECT version_num FROM alembic_version ORDER BY version_num ASC"
	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to query applied migrations: %w", err)
	}
	defer rows.Close()

	var versions []string
	for rows.Next() {
		var version string
		if err := rows.Scan(&version); err != nil {
			return nil, fmt.Errorf("failed to scan version: %w", err)
		}
		versions = append(versions, version)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("row iteration error: %w", err)
	}

	return versions, nil
}

// LoadMigrations scans the Alembic versions directory
func (a *AlembicAdapter) LoadMigrations() error {
	a.migrations = nil

	if a.migrationDir == "" {
		return errors.New("migration directory not set")
	}

	if _, err := os.Stat(a.migrationDir); os.IsNotExist(err) {
		return fmt.Errorf("migration directory does not exist: %s", a.migrationDir)
	}

	// Alembic migrations can be either SQL files or Python files
	// For SQL-based workflow, we look for .sql files
	// For Python-based, we'd need to parse Python files
	err := filepath.Walk(a.migrationDir, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if info.IsDir() {
			return nil
		}

		if a.IsMigrationFile(path) {
			mig, err := a.ParseMigrationFile(path)
			if err != nil {
				// For now, skip Python migrations that can't be parsed
				if filepath.Ext(path) == ".py" {
					// Silently skip unimplemented Python parser
					return nil
				}
				return fmt.Errorf("failed to parse %s: %w", path, err)
			}
			a.migrations = append(a.migrations, mig)
		}

		return nil
	})

	if err != nil {
		return err
	}

	// Sort migrations by timestamp (Alembic uses timestamp in filename)
	sort.Slice(a.migrations, func(i, j int) bool {
		return a.migrations[i].Timestamp.Before(a.migrations[j].Timestamp)
	})

	return nil
}

// GetMigrations returns all loaded migrations
func (a *AlembicAdapter) GetMigrations() ([]adapter.Migration, error) {
	if a.migrations == nil {
		if err := a.LoadMigrations(); err != nil {
			return nil, err
		}
	}
	return a.migrations, nil
}

// ParseMigrationFile parses an Alembic migration file
// Supports both SQL files and Python files (placeholder for Python parsing)
func (a *AlembicAdapter) ParseMigrationFile(filePath string) (adapter.Migration, error) {
	ext := filepath.Ext(filePath)
	baseName := filepath.Base(filePath)

	var version, name string
	var upSQL string

	switch ext {
	case ".sql":
		// Alembic SQL files: <revision>_<description>.sql
		// Or just <revision>.sql
		parts := strings.SplitN(baseName, "_", 2)
		version = strings.TrimSuffix(parts[0], ".sql")
		if len(parts) > 1 {
			name = strings.TrimSuffix(parts[1], ".sql")
		} else {
			name = "migration"
		}

		content, err := os.ReadFile(filePath)
		if err != nil {
			return adapter.Migration{}, fmt.Errorf("failed to read file: %w", err)
		}
		upSQL = string(content)

	case ".py":
		// Alembic Python migrations - would need to parse for revision and down_revision
		// This is a simplified placeholder
		return adapter.Migration{}, fmt.Errorf("python migration parsing not yet implemented")
	default:
		return adapter.Migration{}, fmt.Errorf("unsupported file type: %s", ext)
	}

	info, err := os.Stat(filePath)
	if err != nil {
		return adapter.Migration{}, fmt.Errorf("failed to stat file: %w", err)
	}

	return adapter.Migration{
		Version:   version,
		Name:      name,
		FilePath:  filePath,
		Checksum:  fmt.Sprintf("%x", simpleChecksum([]byte(upSQL))),
		Timestamp: info.ModTime(),
		UpSQL:     upSQL,
	}, nil
}

// IsMigrationFile checks if a file is a valid Alembic migration
func (a *AlembicAdapter) IsMigrationFile(filePath string) bool {
	ext := filepath.Ext(filePath)
	baseName := filepath.Base(filePath)

	if ext == ".sql" {
		// Alembic SQL files: <revision>_<description>.sql or <revision>.sql
		// Revision is typically alphanumeric (at least 4 chars for Alembic)
		parts := strings.Split(baseName, "_")
		if len(parts) >= 1 && len(parts[0]) >= 4 {
			return true
		}
		return false
	}

	if ext == ".py" {
		// Python migrations: <revision>_<description>.py
		// Skip common Python files like setup.py, model.py, __init__.py
		if baseName == "setup.py" || baseName == "model.py" || baseName == "__init__.py" ||
		   strings.HasPrefix(baseName, "test_") || strings.HasPrefix(baseName, "conftest") {
			return false
		}
		parts := strings.SplitN(baseName, "_", 2)
		return len(parts) >= 1 && len(parts[0]) >= 4
	}

	return false
}

// Apply runs a specific migration
func (a *AlembicAdapter) Apply(ctx context.Context, migration adapter.Migration) error {
	db, err := a.Connect(ctx)
	if err != nil {
		return err
	}
	defer db.Close()

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, migration.UpSQL); err != nil {
		return fmt.Errorf("failed to apply migration: %w", err)
	}

	// Record the migration in alembic_version
	insertQuery := "INSERT INTO alembic_version (version_num) VALUES ($1) ON CONFLICT DO NOTHING"
	if _, err := tx.ExecContext(ctx, insertQuery, migration.Version); err != nil {
		return fmt.Errorf("failed to record migration: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

// ApplyAll runs migrations from a starting version to target
func (a *AlembicAdapter) ApplyAll(ctx context.Context, from string, to string, migrations []adapter.Migration) error {
	migs, err := a.GetMigrations()
	if err != nil {
		return err
	}

	startIdx := -1
	endIdx := -1
	for i, m := range migs {
		if from == "" || m.Version == from {
			if startIdx == -1 {
				startIdx = i
			}
		}
		if m.Version == to {
			endIdx = i
			break
		}
	}

	if startIdx == -1 {
		return fmt.Errorf("from version %s not found", from)
	}
	if endIdx == -1 {
		return fmt.Errorf("to version %s not found", to)
	}

	for i := startIdx; i <= endIdx; i++ {
		migration := migs[i]
		if err := a.Apply(ctx, migration); err != nil {
			return fmt.Errorf("failed to apply migration %s: %w", migration.Version, err)
		}
	}

	return nil
}

// CanApply checks if the migration can be safely applied
func (a *AlembicAdapter) CanApply(ctx context.Context, migration adapter.Migration) (bool, string, error) {
	current, err := a.CurrentVersion(ctx)
	if err != nil {
		return false, "", err
	}

	if current != "" && current >= migration.Version {
		return false, fmt.Sprintf("migration %s already applied (current: %s)", migration.Version, current), nil
	}

	return true, "", nil
}

// CurrentSchema returns the current database schema (delegates to Postgres implementation)
func (a *AlembicAdapter) CurrentSchema(ctx context.Context) (*adapter.Schema, error) {
	// Alembic typically uses PostgreSQL, so reuse Postgres schema inspection
	// In a full implementation, we'd have a shared schema inspector
	return a.getSchema(ctx)
}

// TargetSchema returns the target schema from migrations
func (a *AlembicAdapter) TargetSchema(ctx context.Context) (*adapter.Schema, error) {
	return &adapter.Schema{
		Tables: make(map[string]adapter.SchemaTable),
	}, nil
}

// Diff returns differences between current and target
func (a *AlembicAdapter) Diff(current, target *adapter.Schema) (*adapter.SchemaDiff, error) {
	diff := &adapter.SchemaDiff{}

	for name := range target.Tables {
		if _, exists := current.Tables[name]; !exists {
			diff.TablesAdded = append(diff.TablesAdded, name)
		}
	}

	for name := range current.Tables {
		if _, exists := target.Tables[name]; !exists {
			diff.TablesRemoved = append(diff.TablesRemoved, name)
		}
	}

	for name := range target.Tables {
		if currentTable, exists := current.Tables[name]; exists {
			tableDiff := diffTables(currentTable, target.Tables[name])
			diff.ColumnsAdded = append(diff.ColumnsAdded, tableDiff.ColumnsAdded...)
			diff.ColumnsRemoved = append(diff.ColumnsRemoved, tableDiff.ColumnsRemoved...)
			diff.ColumnsModified = append(diff.ColumnsModified, tableDiff.ColumnsModified...)
		}
	}

	return diff, nil
}

// getSchema inspects the database schema
func (a *AlembicAdapter) getSchema(ctx context.Context) (*adapter.Schema, error) {
	db, err := a.Connect(ctx)
	if err != nil {
		return nil, err
	}
	defer db.Close()

	schema := &adapter.Schema{
		Tables: make(map[string]adapter.SchemaTable),
	}

	tablesQuery := `
		SELECT table_name
		FROM information_schema.tables
		WHERE table_schema = 'public' AND table_type = 'BASE TABLE'
		AND table_name NOT LIKE 'pg_%' AND table_name NOT LIKE 'sql_%'
		ORDER BY table_name
	`
	rows, err := db.QueryContext(ctx, tablesQuery)
	if err != nil {
		return nil, fmt.Errorf("failed to query tables: %w", err)
	}
	defer rows.Close()

	var tableNames []string
	for rows.Next() {
		var tableName string
		if err := rows.Scan(&tableName); err != nil {
			return nil, fmt.Errorf("failed to scan table name: %w", err)
		}
		tableNames = append(tableNames, tableName)
	}

	for _, tableName := range tableNames {
		table, err := a.getTableSchema(ctx, db, tableName)
		if err != nil {
			return nil, fmt.Errorf("failed to get schema for table %s: %w", tableName, err)
		}
		schema.Tables[tableName] = table
	}

	return schema, nil
}

// getTableSchema returns schema for a single table
func (a *AlembicAdapter) getTableSchema(ctx context.Context, db *sql.DB, tableName string) (adapter.SchemaTable, error) {
	columnsQuery := `
		SELECT column_name, data_type, is_nullable, column_default
		FROM information_schema.columns
		WHERE table_schema = 'public' AND table_name = $1
		ORDER BY ordinal_position
	`
	rows, err := db.QueryContext(ctx, columnsQuery, tableName)
	if err != nil {
		return adapter.SchemaTable{}, err
	}
	defer rows.Close()

	table := adapter.SchemaTable{
		Name: tableName,
		Columns: make([]adapter.SchemaColumn, 0),
	}

	for rows.Next() {
		var col adapter.SchemaColumn
		var defaultValue sql.NullString
		if err := rows.Scan(&col.Name, &col.Type, &col.Nullable, &defaultValue); err != nil {
			return adapter.SchemaTable{}, err
		}
		if defaultValue.Valid {
			col.DefaultValue = &defaultValue.String
		}
		table.Columns = append(table.Columns, col)
	}

	// Get primary key info
	pkQuery := `
		SELECT kcu.column_name
		FROM information_schema.table_constraints tc
		JOIN information_schema.key_column_usage kcu
		  ON tc.constraint_name = kcu.constraint_name
		WHERE tc.table_schema = 'public' AND tc.table_name = $1
		  AND tc.constraint_type = 'PRIMARY KEY'
		ORDER BY kcu.ordinal_position
	`
	pkRows, err := db.QueryContext(ctx, pkQuery, tableName)
	if err == nil {
		defer pkRows.Close()
		for pkRows.Next() {
			var colName string
			if err := pkRows.Scan(&colName); err != nil {
				break
			}
			table.PrimaryKey = append(table.PrimaryKey, colName)
			for i := range table.Columns {
				if table.Columns[i].Name == colName {
					table.Columns[i].IsPrimaryKey = true
					break
				}
			}
		}
	}

	return table, nil
}

// simpleChecksum calculates a simple checksum
func simpleChecksum(data []byte) []byte {
	sum := 0
	for _, b := range data {
		sum = (sum*31 + int(b)) & 0x7fffffff
	}
	return []byte{byte(sum >> 24), byte(sum >> 16), byte(sum >> 8), byte(sum)}
}

// diffTables compares two table schemas (same as postgres adapter)
func diffTables(current, target adapter.SchemaTable) adapter.TableDiff {
	diff := adapter.TableDiff{
		ColumnsAdded:    make([]adapter.ColumnDiff, 0),
		ColumnsRemoved:  make([]adapter.ColumnDiff, 0),
		ColumnsModified: make([]adapter.ColumnDiff, 0),
	}

	currentCols := make(map[string]adapter.SchemaColumn)
	for _, c := range current.Columns {
		currentCols[c.Name] = c
	}

	targetCols := make(map[string]adapter.SchemaColumn)
	for _, c := range target.Columns {
		targetCols[c.Name] = c
	}

	for name, targetCol := range targetCols {
		if _, exists := currentCols[name]; !exists {
			diff.ColumnsAdded = append(diff.ColumnsAdded, adapter.ColumnDiff{
				Table: current.Name,
				Column: name,
				NewType: targetCol.Type,
				NewNullable: targetCol.Nullable,
			})
		}
	}

	for name, currentCol := range currentCols {
		if targetCol, exists := targetCols[name]; !exists {
			diff.ColumnsRemoved = append(diff.ColumnsRemoved, adapter.ColumnDiff{
				Table: current.Name,
				Column: name,
				OldType: currentCol.Type,
				OldNullable: currentCol.Nullable,
			})
		} else if currentCol.Type != targetCol.Type || currentCol.Nullable != targetCol.Nullable {
			diff.ColumnsModified = append(diff.ColumnsModified, adapter.ColumnDiff{
				Table: current.Name,
				Column: name,
				OldType: currentCol.Type,
				NewType: targetCol.Type,
				OldNullable: currentCol.Nullable,
				NewNullable: targetCol.Nullable,
			})
		}
	}

	return diff
}
