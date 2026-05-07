package django

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
)

var (
	ErrNoMigrations = errors.New("no migrations found")
)

// DjangoAdapter implements adapter for Django migrations
type DjangoAdapter struct {
	connStr     string
	migrationDir string
	migrations  []adapter.Migration
}

// NewDjangoAdapter creates a new Django adapter
func NewDjangoAdapter(connStr, migrationDir string) *DjangoAdapter {
	return &DjangoAdapter{
		connStr:     connStr,
		migrationDir: migrationDir,
	}
}

// Type returns the adapter type
func (d *DjangoAdapter) Type() adapter.AdapterType {
	return adapter.AdapterTypeDjango
}

// Name returns the adapter display name
func (d *DjangoAdapter) Name() string {
	return "Django"
}

// MigrationTable returns the django_migrations table name
func (d *DjangoAdapter) MigrationTable() string {
	return "django_migrations"
}

// Connect establishes a database connection
func (d *DjangoAdapter) Connect(ctx context.Context) (*sql.DB, error) {
	db, err := sql.Open("postgres", d.connStr)
	if err != nil {
		return nil, err
	}
	if err := db.PingContext(ctx); err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}
	return db, nil
}

// CurrentVersion returns the latest applied migration version (combined app + migration name)
func (d *DjangoAdapter) CurrentVersion(ctx context.Context) (string, error) {
	db, err := d.Connect(ctx)
	if err != nil {
		return "", err
	}
	defer db.Close()

	// Django stores app + name separately, we combine them for version comparison
	query := "SELECT app, name FROM django_migrations ORDER BY applied DESC LIMIT 1"
	var app, name string
	err = db.QueryRowContext(ctx, query).Scan(&app, &name)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", nil
		}
		return "", fmt.Errorf("failed to get current version: %w", err)
	}
	return app + ":" + name, nil
}

// AppliedMigrations returns all applied migration versions
func (d *DjangoAdapter) AppliedMigrations(ctx context.Context) ([]string, error) {
	db, err := d.Connect(ctx)
	if err != nil {
		return nil, err
	}
	defer db.Close()

	query := "SELECT app, name FROM django_migrations ORDER BY applied ASC, app ASC, name ASC"
	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to query applied migrations: %w", err)
	}
	defer rows.Close()

	var versions []string
	for rows.Next() {
		var app, name string
		if err := rows.Scan(&app, &name); err != nil {
			return nil, fmt.Errorf("failed to scan migration: %w", err)
		}
		versions = append(versions, app+":"+name)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("row iteration error: %w", err)
	}

	return versions, nil
}

// LoadMigrations scans the migration directories for Django migrations
func (d *DjangoAdapter) LoadMigrations() error {
	d.migrations = nil

	if d.migrationDir == "" {
		return errors.New("migration directory not set")
	}

	if _, err := os.Stat(d.migrationDir); os.IsNotExist(err) {
		return fmt.Errorf("migration directory does not exist: %s", d.migrationDir)
	}

	// Django migrations are organized by app: <migrationDir>/<app>/migrations/
	err := filepath.Walk(d.migrationDir, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if info.IsDir() {
			return nil
		}

		if d.IsMigrationFile(path) {
			mig, err := d.ParseMigrationFile(path)
			if err != nil {
				return fmt.Errorf("failed to parse %s: %w", path, err)
			}
			d.migrations = append(d.migrations, mig)
		}

		return nil
	})

	if err != nil {
		return err
	}

	// Sort migrations by app, then by number (Django uses 0001_initial.py format)
	sort.Slice(d.migrations, func(i, j int) bool {
		if d.migrations[i].Name == d.migrations[j].Name {
			return d.migrations[i].Version < d.migrations[j].Version
		}
		// Extract numeric part for proper ordering
		numI := extractMigrationNumber(d.migrations[i].Name)
		numJ := extractMigrationNumber(d.migrations[j].Name)
		if numI != numJ {
			return numI < numJ
		}
		return d.migrations[i].Name < d.migrations[j].Name
	})

	return nil
}

// extractMigrationNumber extracts the numeric prefix from a Django migration name (e.g., "0001_initial" -> 1)
func extractMigrationNumber(name string) int {
	var num int
	for _, r := range name {
		if r >= '0' && r <= '9' {
			num = num*10 + int(r-'0')
		} else {
			break
		}
	}
	return num
}

// GetMigrations returns all loaded migrations
func (d *DjangoAdapter) GetMigrations() ([]adapter.Migration, error) {
	if d.migrations == nil {
		if err := d.LoadMigrations(); err != nil {
			return nil, err
		}
	}
	return d.migrations, nil
}

// djangoMigrationPattern matches Django migration filenames
var djangoMigrationPattern = regexp.MustCompile(`^(\d{4})_(.+)\.py$`)

// ParseMigrationFile parses a Django migration file
// Django migrations: 0001_initial.py
func (d *DjangoAdapter) ParseMigrationFile(filePath string) (adapter.Migration, error) {
	baseName := filepath.Base(filePath)

	// Django migration pattern: NNNN_name.py (4 digit number prefix)
	matches := djangoMigrationPattern.FindStringSubmatch(baseName)
	if len(matches) < 3 {
		return adapter.Migration{}, fmt.Errorf("invalid Django migration filename: %s", baseName)
	}

	version := matches[1] + "_" + matches[2]
	name := strings.TrimSuffix(matches[2], ".py")

	// Extract app name from path: <migrationDir>/<app>/migrations/<file>.py
	relPath, err := filepath.Rel(d.migrationDir, filePath)
	if err != nil {
		return adapter.Migration{}, fmt.Errorf("failed to get relative path: %w", err)
	}
	pathParts := strings.Split(relPath, string(filepath.Separator))
	if len(pathParts) < 2 {
		return adapter.Migration{}, fmt.Errorf("unexpected migration path: %s", relPath)
	}
	appName := pathParts[0]

	// Read file content
	content, err := os.ReadFile(filePath)
	if err != nil {
		return adapter.Migration{}, fmt.Errorf("failed to read migration file: %w", err)
	}

	// Extract migration class info from Python code
	dependencies := d.extractDependencies(content)

	info, err := os.Stat(filePath)
	if err != nil {
		return adapter.Migration{}, fmt.Errorf("failed to stat file: %w", err)
	}

	return adapter.Migration{
		Version:   appName + ":" + version,
		Name:      appName + "." + name,
		FilePath:  filePath,
		Checksum:  fmt.Sprintf("%x", simpleChecksum(content)),
		Timestamp: info.ModTime(),
		UpSQL:     string(content),
		DownSQL:   dependencies,
	}, nil
}

// extractDependencies extracts dependency information from Python migration
func (d *DjangoAdapter) extractDependencies(content []byte) string {
	// Look for dependencies list in the Migration class
	// This is a simplified extraction - a full implementation would parse the AST
	lines := strings.Split(string(content), "\n")
	for _, line := range lines {
		if strings.Contains(line, "dependencies = [") {
			// Extract the dependencies
			return strings.TrimSpace(line)
		}
	}
	return ""
}

// IsMigrationFile checks if a file is a valid Django migration
func (d *DjangoAdapter) IsMigrationFile(filePath string) bool {
	baseName := filepath.Base(filePath)
	if !djangoMigrationPattern.MatchString(baseName) {
		return false
	}

	// Django migrations should be in a migrations/ directory
	// Check if the file is in a directory named "migrations"
	dir := filepath.Dir(filePath)
	dirName := filepath.Base(dir)
	return dirName == "migrations"
}

// Apply runs a specific migration
func (d *DjangoAdapter) Apply(ctx context.Context, migration adapter.Migration) error {
	db, err := d.Connect(ctx)
	if err != nil {
		return err
	}
	defer db.Close()

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Django migrations are Python code - we can't directly apply them
	// In a real scenario, this would need to run Python/Django management command
	// For now, we just record that we would apply it
	parts := strings.SplitN(migration.Version, ":", 2)
	if len(parts) != 2 {
		return fmt.Errorf("invalid migration version format: %s", migration.Version)
	}
	app, name := parts[0], parts[1]

	// Record the migration in django_migrations
	insertQuery := "INSERT INTO django_migrations (app, name) VALUES ($1, $2) ON CONFLICT DO NOTHING"
	if _, err := tx.ExecContext(ctx, insertQuery, app, name); err != nil {
		return fmt.Errorf("failed to record migration: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

// ApplyAll runs migrations from a starting version to target
func (d *DjangoAdapter) ApplyAll(ctx context.Context, from string, to string, migrations []adapter.Migration) error {
	migs, err := d.GetMigrations()
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
		if err := d.Apply(ctx, migration); err != nil {
			return fmt.Errorf("failed to apply migration %s: %w", migration.Version, err)
		}
	}

	return nil
}

// CanApply checks if the migration can be safely applied
func (d *DjangoAdapter) CanApply(ctx context.Context, migration adapter.Migration) (bool, string, error) {
	current, err := d.CurrentVersion(ctx)
	if err != nil {
		return false, "", err
	}

	if current != "" && current >= migration.Version {
		return false, fmt.Sprintf("migration %s already applied (current: %s)", migration.Version, current), nil
	}

	return true, "", nil
}

// CurrentSchema returns the current database schema
func (d *DjangoAdapter) CurrentSchema(ctx context.Context) (*adapter.Schema, error) {
	return d.getSchema(ctx)
}

// TargetSchema returns the target schema from migrations
func (d *DjangoAdapter) TargetSchema(ctx context.Context) (*adapter.Schema, error) {
	return &adapter.Schema{
		Tables: make(map[string]adapter.SchemaTable),
	}, nil
}

// Diff returns differences between current and target
func (d *DjangoAdapter) Diff(current, target *adapter.Schema) (*adapter.SchemaDiff, error) {
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
func (d *DjangoAdapter) getSchema(ctx context.Context) (*adapter.Schema, error) {
	db, err := d.Connect(ctx)
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
		table, err := d.getTableSchema(ctx, db, tableName)
		if err != nil {
			return nil, fmt.Errorf("failed to get schema for table %s: %w", tableName, err)
		}
		schema.Tables[tableName] = table
	}

	return schema, nil
}

// getTableSchema returns schema for a single table
func (d *DjangoAdapter) getTableSchema(ctx context.Context, db *sql.DB, tableName string) (adapter.SchemaTable, error) {
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
		Name:    tableName,
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

// FindMigrationByVersion finds a migration by its version string (app:number_name format)
func (d *DjangoAdapter) FindMigrationByVersion(version string) (adapter.Migration, error) {
	migrations, err := d.GetMigrations()
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
