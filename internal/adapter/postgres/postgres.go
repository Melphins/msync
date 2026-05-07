package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/Melphins/msync/internal/adapter"

	_ "github.com/lib/pq"
)

var (
	ErrNoMigrations   = errors.New("no migrations found")
	ErrInvalidVersion = errors.New("invalid version format")
)

var sqlMigrationPattern = regexp.MustCompile(`^(\d{4,})_.*\.sql$`)

// PostgresAdapter implements the adapter.Adapter interface for PostgreSQL databases
type PostgresAdapter struct {
	connStr       string
	migrationTable string
	migrationDir  string
	migrations    []adapter.Migration
}

// NewPostgresAdapter creates a new PostgreSQL adapter
func NewPostgresAdapter(connStr, migrationTable, migrationDir string) *PostgresAdapter {
	if migrationTable == "" {
		migrationTable = "schema_migrations"
	}
	return &PostgresAdapter{
		connStr:       connStr,
		migrationTable: migrationTable,
		migrationDir:  migrationDir,
	}
}

// Type returns the adapter type
func (p *PostgresAdapter) Type() adapter.AdapterType {
	return adapter.AdapterTypePostgres
}

// Name returns the adapter display name
func (p *PostgresAdapter) Name() string {
	return "PostgreSQL"
}

// Connect establishes a database connection
func (p *PostgresAdapter) Connect(ctx context.Context) (*sql.DB, error) {
	db, err := sql.Open("postgres", p.connStr)
	if err != nil {
		return nil, err
	}
	if err := db.PingContext(ctx); err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}
	return db, nil
}

// CurrentVersion returns the latest applied migration version
func (p *PostgresAdapter) CurrentVersion(ctx context.Context) (string, error) {
	db, err := p.Connect(ctx)
	if err != nil {
		return "", err
	}
	defer db.Close()

	var version string
	query := fmt.Sprintf("SELECT version FROM %s ORDER BY version DESC LIMIT 1", p.migrationTable)
	err = db.QueryRowContext(ctx, query).Scan(&version)
	if err != nil {
		// Check if table doesn't exist or no rows
		if strings.Contains(err.Error(), "does not exist") || errors.Is(err, sql.ErrNoRows) {
			return "", nil
		}
		return "", fmt.Errorf("failed to get current version: %w", err)
	}
	return version, nil
}

// AppliedMigrations returns all applied migration versions
func (p *PostgresAdapter) AppliedMigrations(ctx context.Context) ([]string, error) {
	db, err := p.Connect(ctx)
	if err != nil {
		return nil, err
	}
	defer db.Close()

	query := fmt.Sprintf("SELECT version FROM %s ORDER BY version ASC", p.migrationTable)
	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		// Check if table doesn't exist yet
		if strings.Contains(err.Error(), "does not exist") {
			return []string{}, nil
		}
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

// MigrationTable returns the name of the migration tracking table
func (p *PostgresAdapter) MigrationTable() string {
	return p.migrationTable
}

// LoadMigrations scans the migration directory and loads all SQL migrations
func (p *PostgresAdapter) LoadMigrations() error {
	p.migrations = nil

	if p.migrationDir == "" {
		return errors.New("migration directory not set")
	}

	if _, err := os.Stat(p.migrationDir); os.IsNotExist(err) {
		return fmt.Errorf("migration directory does not exist: %s", p.migrationDir)
	}

	err := filepath.Walk(p.migrationDir, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if info.IsDir() {
			return nil
		}

		if p.IsMigrationFile(path) {
			mig, err := p.ParseMigrationFile(path)
			if err != nil {
				return fmt.Errorf("failed to parse %s: %w", path, err)
			}
			p.migrations = append(p.migrations, mig)
		}

		return nil
	})

	if err != nil {
		return err
	}

	// Sort migrations by version
	sort.Slice(p.migrations, func(i, j int) bool {
		return p.migrations[i].Version < p.migrations[j].Version
	})

	return nil
}

// GetMigrations returns all loaded migrations
func (p *PostgresAdapter) GetMigrations() ([]adapter.Migration, error) {
	if p.migrations == nil {
		if err := p.LoadMigrations(); err != nil {
			return nil, err
		}
	}
	return p.migrations, nil
}

// FindMigrationByVersion finds a migration by its version string
func (p *PostgresAdapter) FindMigrationByVersion(version string) (adapter.Migration, error) {
	migrations, err := p.GetMigrations()
	if err != nil {
		return adapter.Migration{}, err
	}

	for _, m := range migrations {
		if m.Version == version {
			return m, nil
		}
	}

	return adapter.Migration{}, adapter.ErrMigrationNotFound
}

// ParseMigrationFile parses a SQL migration file into a Migration struct
func (p *PostgresAdapter) ParseMigrationFile(filePath string) (adapter.Migration, error) {
	// Extract version from filename (e.g., 0001_initial_schema.sql -> 0001)
	baseName := filepath.Base(filePath)
	matches := sqlMigrationPattern.FindStringSubmatch(baseName)
	if len(matches) < 2 {
		return adapter.Migration{}, fmt.Errorf("invalid migration filename format: %s", baseName)
	}
	version := matches[1]

	// Extract name from filename (e.g., 0001_initial_schema.sql -> initial_schema)
	name := strings.TrimSuffix(strings.TrimPrefix(baseName, version+"_"), ".sql")

	// Read file content
	content, err := os.ReadFile(filePath)
	if err != nil {
		return adapter.Migration{}, fmt.Errorf("failed to read migration file: %w", err)
	}

	// Get file info for timestamp
	info, err := os.Stat(filePath)
	if err != nil {
		return adapter.Migration{}, fmt.Errorf("failed to stat file: %w", err)
	}

	return adapter.Migration{
		Version:   version,
		Name:      name,
		FilePath:  filePath,
		Checksum:  fmt.Sprintf("%x", simpleChecksum(content)),
		Timestamp: info.ModTime(),
		UpSQL:     string(content),
	}, nil
}

// IsMigrationFile checks if a file is a valid migration for this adapter
func (p *PostgresAdapter) IsMigrationFile(filePath string) bool {
	baseName := filepath.Base(filePath)
	return sqlMigrationPattern.MatchString(baseName)
}

// simpleChecksum calculates a simple checksum (for now - will upgrade to SHA256)
func simpleChecksum(data []byte) []byte {
	sum := 0
	for _, b := range data {
		sum = (sum*31 + int(b)) & 0x7fffffff
	}
	return []byte{byte(sum >> 24), byte(sum >> 16), byte(sum >> 8), byte(sum)}
}

// Apply runs a specific migration
func (p *PostgresAdapter) Apply(ctx context.Context, migration adapter.Migration) error {
	db, err := p.Connect(ctx)
	if err != nil {
		return err
	}
	defer db.Close()

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Ensure migration tracking table exists
	createTableSQL := fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS %s (
			version VARCHAR(255) PRIMARY KEY
		)
	`, p.migrationTable)
	if _, err := tx.ExecContext(ctx, createTableSQL); err != nil {
		return fmt.Errorf("failed to create migration table: %w", err)
	}

	// Apply the migration
	if _, err := tx.ExecContext(ctx, migration.UpSQL); err != nil {
		return fmt.Errorf("failed to apply migration: %w", err)
	}

	// Record the migration in schema_migrations
	insertQuery := fmt.Sprintf(
		"INSERT INTO %s (version) VALUES ($1) ON CONFLICT (version) DO NOTHING",
		p.migrationTable,
	)
	if _, err := tx.ExecContext(ctx, insertQuery, migration.Version); err != nil {
		return fmt.Errorf("failed to record migration: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

// ApplyAll runs migrations from a starting version to target
func (p *PostgresAdapter) ApplyAll(ctx context.Context, from string, to string, migrations []adapter.Migration) error {
	migs, err := p.GetMigrations()
	if err != nil {
		return err
	}

	// Find the index range
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

	// Apply migrations in sequence
	for i := startIdx; i <= endIdx; i++ {
		migration := migs[i]
		if err := p.Apply(ctx, migration); err != nil {
			return fmt.Errorf("failed to apply migration %s: %w", migration.Version, err)
		}
	}

	return nil
}

// CanApply checks if the migration can be safely applied
func (p *PostgresAdapter) CanApply(ctx context.Context, migration adapter.Migration) (bool, string, error) {
	// Check if already applied
	current, err := p.CurrentVersion(ctx)
	if err != nil {
		return false, "", err
	}

	if current != "" && current >= migration.Version {
		return false, fmt.Sprintf("migration %s already applied (current: %s)", migration.Version, current), nil
	}

	return true, "", nil
}

// CurrentSchema returns the current database schema
func (p *PostgresAdapter) CurrentSchema(ctx context.Context) (*adapter.Schema, error) {
	return p.getSchema(ctx)
}

// TargetSchema returns the target schema from migrations
// This would need to execute all migrations against a clean database
func (p *PostgresAdapter) TargetSchema(ctx context.Context) (*adapter.Schema, error) {
	// Placeholder - would require applying migrations to a reference DB
	// For now, return empty schema
	return &adapter.Schema{
		Tables: make(map[string]adapter.SchemaTable),
	}, nil
}

// Diff returns differences between current and target
func (p *PostgresAdapter) Diff(current, target *adapter.Schema) (*adapter.SchemaDiff, error) {
	diff := &adapter.SchemaDiff{}

	// Find tables in target but not current
	for name := range target.Tables {
		if _, exists := current.Tables[name]; !exists {
			diff.TablesAdded = append(diff.TablesAdded, name)
		}
	}

	// Find tables in current but not target
	for name := range current.Tables {
		if _, exists := target.Tables[name]; !exists {
			diff.TablesRemoved = append(diff.TablesRemoved, name)
		}
	}

	// Compare columns in common tables
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

// getSchema inspects the current database schema
func (p *PostgresAdapter) getSchema(ctx context.Context) (*adapter.Schema, error) {
	db, err := p.Connect(ctx)
	if err != nil {
		return nil, err
	}
	defer db.Close()

	schema := &adapter.Schema{
		Tables: make(map[string]adapter.SchemaTable),
	}

	// Get all tables in public schema (excluding system tables)
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

	// Get columns for each table
	for _, tableName := range tableNames {
		table, err := p.getTableSchema(ctx, db, tableName)
		if err != nil {
			return nil, fmt.Errorf("failed to get schema for table %s: %w", tableName, err)
		}
		schema.Tables[tableName] = table
	}

	return schema, nil
}

// getTableSchema returns the schema for a single table
func (p *PostgresAdapter) getTableSchema(ctx context.Context, db *sql.DB, tableName string) (adapter.SchemaTable, error) {
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
		var nullableStr string
		if err := rows.Scan(&col.Name, &col.Type, &nullableStr, &defaultValue); err != nil {
			return adapter.SchemaTable{}, err
		}
		col.Nullable = (nullableStr == "YES")
		if defaultValue.Valid {
			col.DefaultValue = &defaultValue.String
		}
		col.IsPrimaryKey = false
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
			// Mark column as primary key
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

// diffTables compares two table schemas and returns the differences
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

	// Find added columns
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

	// Find removed and modified columns
	for name, currentCol := range currentCols {
		if targetCol, exists := targetCols[name]; !exists {
			diff.ColumnsRemoved = append(diff.ColumnsRemoved, adapter.ColumnDiff{
				Table: current.Name,
				Column: name,
				OldType: currentCol.Type,
				OldNullable: currentCol.Nullable,
			})
		} else {
			// Check if modified
			if currentCol.Type != targetCol.Type || currentCol.Nullable != targetCol.Nullable {
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
	}

	return diff
}
