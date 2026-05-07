package rails

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
	ErrNoMigrations = errors.New("no migrations found")
)

// RailsAdapter implements adapter for Ruby on Rails migrations
type RailsAdapter struct {
	connStr     string
	migrationDir string
	migrations  []adapter.Migration
}

// NewRailsAdapter creates a new Rails adapter
func NewRailsAdapter(connStr, migrationDir string) *RailsAdapter {
	return &RailsAdapter{
		connStr:     connStr,
		migrationDir: migrationDir,
	}
}

// Type returns the adapter type
func (r *RailsAdapter) Type() adapter.AdapterType {
	return adapter.AdapterTypeRails
}

// Name returns the adapter display name
func (r *RailsAdapter) Name() string {
	return "Rails"
}

// MigrationTable returns the schema_migrations table name
func (r *RailsAdapter) MigrationTable() string {
	return "schema_migrations"
}

// Connect establishes a database connection
func (r *RailsAdapter) Connect(ctx context.Context) (*sql.DB, error) {
	db, err := sql.Open("postgres", r.connStr)
	if err != nil {
		return nil, err
	}
	if err := db.PingContext(ctx); err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}
	return db, nil
}

// CurrentVersion returns the latest applied migration version (timestamp string)
func (r *RailsAdapter) CurrentVersion(ctx context.Context) (string, error) {
	db, err := r.Connect(ctx)
	if err != nil {
		return "", err
	}
	defer db.Close()

	var version string
	query := "SELECT version FROM schema_migrations ORDER BY version DESC LIMIT 1"
	err = db.QueryRowContext(ctx, query).Scan(&version)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", nil
		}
		return "", fmt.Errorf("failed to get current version: %w", err)
	}
	return version, nil
}

// AppliedMigrations returns all applied migration versions
func (r *RailsAdapter) AppliedMigrations(ctx context.Context) ([]string, error) {
	db, err := r.Connect(ctx)
	if err != nil {
		return nil, err
	}
	defer db.Close()

	query := "SELECT version FROM schema_migrations ORDER BY version ASC"
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

// LoadMigrations scans the db/migrate directory for Rails migrations
func (r *RailsAdapter) LoadMigrations() error {
	r.migrations = nil

	if r.migrationDir == "" {
		return errors.New("migration directory not set")
	}

	if _, err := os.Stat(r.migrationDir); os.IsNotExist(err) {
		return fmt.Errorf("migration directory does not exist: %s", r.migrationDir)
	}

	err := filepath.Walk(r.migrationDir, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if info.IsDir() {
			return nil
		}

		if r.IsMigrationFile(path) {
			mig, err := r.ParseMigrationFile(path)
			if err != nil {
				return fmt.Errorf("failed to parse %s: %w", path, err)
			}
			r.migrations = append(r.migrations, mig)
		}

		return nil
	})

	if err != nil {
		return err
	}

	// Sort migrations by timestamp (version string is timestamp)
	sort.Slice(r.migrations, func(i, j int) bool {
		return r.migrations[i].Version < r.migrations[j].Version
	})

	return nil
}

// GetMigrations returns all loaded migrations
func (r *RailsAdapter) GetMigrations() ([]adapter.Migration, error) {
	if r.migrations == nil {
		if err := r.LoadMigrations(); err != nil {
			return nil, err
		}
	}
	return r.migrations, nil
}

// FindMigrationByVersion finds a migration by its version string
func (r *RailsAdapter) FindMigrationByVersion(version string) (adapter.Migration, error) {
	migrations, err := r.GetMigrations()
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

// ParseMigrationFile parses a Rails migration file
// Rails migrations: YYYYMMDDHHMMSS_create_users.rb
func (r *RailsAdapter) ParseMigrationFile(filePath string) (adapter.Migration, error) {
	baseName := filepath.Base(filePath)

	// Rails migration pattern: YYYYMMDDHHMMSS_name.rb (14 digit timestamp)
	matches := railsMigrationPattern.FindStringSubmatch(baseName)
	if len(matches) < 2 {
		return adapter.Migration{}, fmt.Errorf("invalid Rails migration filename: %s", baseName)
	}

	version := matches[1]
	name := strings.TrimSuffix(matches[2], ".rb")

	// Read file content
	content, err := os.ReadFile(filePath)
	if err != nil {
		return adapter.Migration{}, fmt.Errorf("failed to read migration file: %w", err)
	}

	// Extract up/down SQL from Ruby code
	upSQL, downSQL := r.extractSQL(content)

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
		UpSQL:     upSQL,
		DownSQL:   downSQL,
	}, nil
}

// railsMigrationPattern matches Rails migration filenames
var railsMigrationPattern = regexp.MustCompile(`^(\d{14})_(.+)\.rb$`)

// extractSQL attempts to extract SQL from Ruby migration code
// This is a simplified version - full implementation would need to parse Ruby
func (r *RailsAdapter) extractSQL(content []byte) (string, string) {
	// For now, return the raw Ruby code
	// A full implementation would need a Ruby parser or AST analysis
	// to extract the SQL from `execute` calls or reversible operations
	return string(content), ""
}

// IsMigrationFile checks if a file is a valid Rails migration
func (r *RailsAdapter) IsMigrationFile(filePath string) bool {
	baseName := filepath.Base(filePath)
	return railsMigrationPattern.MatchString(baseName)
}

// Apply runs a specific migration
func (r *RailsAdapter) Apply(ctx context.Context, migration adapter.Migration) error {
	db, err := r.Connect(ctx)
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

	// Record the migration in schema_migrations
	insertQuery := "INSERT INTO schema_migrations (version) VALUES ($1) ON CONFLICT DO NOTHING"
	if _, err := tx.ExecContext(ctx, insertQuery, migration.Version); err != nil {
		return fmt.Errorf("failed to record migration: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

// ApplyAll runs migrations from a starting version to target
func (r *RailsAdapter) ApplyAll(ctx context.Context, from string, to string, migrations []adapter.Migration) error {
	migs, err := r.GetMigrations()
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
		if err := r.Apply(ctx, migration); err != nil {
			return fmt.Errorf("failed to apply migration %s: %w", migration.Version, err)
		}
	}

	return nil
}

// CanApply checks if the migration can be safely applied
func (r *RailsAdapter) CanApply(ctx context.Context, migration adapter.Migration) (bool, string, error) {
	current, err := r.CurrentVersion(ctx)
	if err != nil {
		return false, "", err
	}

	if current != "" && current >= migration.Version {
		return false, fmt.Sprintf("migration %s already applied (current: %s)", migration.Version, current), nil
	}

	return true, "", nil
}

// CurrentSchema returns the current database schema
func (r *RailsAdapter) CurrentSchema(ctx context.Context) (*adapter.Schema, error) {
	return r.getSchema(ctx)
}

// TargetSchema returns the target schema from migrations
func (r *RailsAdapter) TargetSchema(ctx context.Context) (*adapter.Schema, error) {
	return &adapter.Schema{
		Tables: make(map[string]adapter.SchemaTable),
	}, nil
}

// Diff returns differences between current and target
func (r *RailsAdapter) Diff(current, target *adapter.Schema) (*adapter.SchemaDiff, error) {
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
func (r *RailsAdapter) getSchema(ctx context.Context) (*adapter.Schema, error) {
	db, err := r.Connect(ctx)
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
		table, err := r.getTableSchema(ctx, db, tableName)
		if err != nil {
			return nil, fmt.Errorf("failed to get schema for table %s: %w", tableName, err)
		}
		schema.Tables[tableName] = table
	}

	return schema, nil
}

// getTableSchema returns schema for a single table
func (r *RailsAdapter) getTableSchema(ctx context.Context, db *sql.DB, tableName string) (adapter.SchemaTable, error) {
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

// diffTables compares two table schemas
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
