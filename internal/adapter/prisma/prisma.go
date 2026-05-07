package prisma

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
	"time"

	"github.com/Melphins/msync/internal/adapter"

	_ "github.com/lib/pq"
)

var (
	ErrNoMigrations = errors.New("no migrations found")
)

// PrismaAdapter implements adapter for Prisma Migrate
type PrismaAdapter struct {
	connStr     string
	migrationDir string
	migrations  []adapter.Migration
}

// NewPrismaAdapter creates a new Prisma adapter
func NewPrismaAdapter(connStr, migrationDir string) *PrismaAdapter {
	return &PrismaAdapter{
		connStr:     connStr,
		migrationDir: migrationDir,
	}
}

// New creates a new Prisma adapter (alias for test compatibility)
func New(_ *sql.DB, migrationDir string) *PrismaAdapter {
	return NewPrismaAdapter("", migrationDir)
}

// Type returns the adapter type
func (p *PrismaAdapter) Type() adapter.AdapterType {
	return adapter.AdapterTypePrisma
}

// Name returns the adapter display name
func (p *PrismaAdapter) Name() string {
	return "Prisma Migrate"
}

// MigrationTable returns the _prisma_migrations table name
func (p *PrismaAdapter) MigrationTable() string {
	return "_prisma_migrations"
}

// MigrationDir returns the migrations directory
func (p *PrismaAdapter) MigrationDir() string {
	return p.migrationDir
}

// Connect establishes a database connection
func (p *PrismaAdapter) Connect(ctx context.Context) (*sql.DB, error) {
	db, err := sql.Open("postgres", p.connStr)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}
	if err := db.PingContext(ctx); err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}
	return db, nil
}

// CurrentVersion returns the latest applied migration version (timestamp)
func (p *PrismaAdapter) CurrentVersion(ctx context.Context) (string, error) {
	db, err := p.Connect(ctx)
	if err != nil {
		return "", err
	}
	defer db.Close()

	var version string
	query := fmt.Sprintf(`
		SELECT migration_id FROM %s
		WHERE finished_at IS NOT NULL
		ORDER BY started_at DESC
		LIMIT 1
	`, p.MigrationTable())

	err = db.QueryRowContext(ctx, query).Scan(&version)
	if err == sql.ErrNoRows {
		return "", nil // No migrations applied yet
	}
	if err != nil {
		return "", fmt.Errorf("failed to query current version: %w", err)
	}
	return version, nil
}

// AppliedMigrations returns all applied migration versions
func (p *PrismaAdapter) AppliedMigrations(ctx context.Context) ([]string, error) {
	db, err := p.Connect(ctx)
	if err != nil {
		return nil, err
	}
	defer db.Close()

	query := fmt.Sprintf(`
		SELECT migration_id FROM %s
		WHERE finished_at IS NOT NULL
		ORDER BY started_at ASC
	`, p.MigrationTable())

	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to query applied migrations: %w", err)
	}
	defer rows.Close()

	var versions []string
	for rows.Next() {
		var version string
		if err := rows.Scan(&version); err != nil {
			return nil, fmt.Errorf("failed to scan migration version: %w", err)
		}
		versions = append(versions, version)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating rows: %w", err)
	}

	return versions, nil
}

// LoadMigrations scans the migrations directory for Prisma migrations
func (p *PrismaAdapter) LoadMigrations() error {
	p.migrations = nil

	if p.migrationDir == "" {
		return errors.New("migration directory not set")
	}

	if _, err := os.Stat(p.migrationDir); os.IsNotExist(err) {
		return fmt.Errorf("migration directory does not exist: %s", p.migrationDir)
	}

	entries, err := os.ReadDir(p.migrationDir)
	if err != nil {
		return fmt.Errorf("failed to read migration directory: %w", err)
	}

	re := regexp.MustCompile(prismaMigrationPattern)
	for _, entry := range entries {
		if entry.IsDir() {
			matches := re.FindStringSubmatch(entry.Name())
			if len(matches) >= 3 {
				timestamp := matches[1]
				name := matches[2]
				fullVersion := entry.Name()

				// Read migration.sql from the directory
				migrationSQL := ""
				sqlPath := filepath.Join(p.migrationDir, entry.Name(), "migration.sql")
				if sqlData, err := os.ReadFile(sqlPath); err == nil {
					migrationSQL = string(sqlData)
				}

				parsedTime, _ := time.Parse("20060102150405", timestamp)

				p.migrations = append(p.migrations, adapter.Migration{
					Version:   fullVersion,
					Name:      name,
					FilePath:  sqlPath,
					Checksum:  "",
					Timestamp: parsedTime,
					UpSQL:     migrationSQL,
					DownSQL:   "",
				})
			}
		}
	}

	// Sort by timestamp
	sort.Slice(p.migrations, func(i, j int) bool {
		return p.migrations[i].Timestamp.Before(p.migrations[j].Timestamp)
	})

	return nil
}

// GetMigrations returns all available migrations from the migration directory
func (p *PrismaAdapter) GetMigrations() ([]adapter.Migration, error) {
	if p.migrations == nil {
		if err := p.LoadMigrations(); err != nil {
			return nil, err
		}
	}
	return p.migrations, nil
}

// FindMigrationByVersion finds a migration by its version string
func (p *PrismaAdapter) FindMigrationByVersion(version string) (adapter.Migration, error) {
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

// ParseMigrationFile parses a migration file/directory
func (p *PrismaAdapter) ParseMigrationFile(path string) (adapter.Migration, error) {
	// For Prisma, path is typically: migrations/YYYYMMDDHHMMSS_name/migration.sql
	dirPath := path
	if strings.HasSuffix(path, "/migration.sql") || strings.HasSuffix(path, "\\migration.sql") {
		dirPath = filepath.Dir(path)
	}

	dirName := filepath.Base(dirPath)
	matches := regexp.MustCompile(prismaMigrationPattern).FindStringSubmatch(dirName)

	if len(matches) < 3 {
		return adapter.Migration{}, fmt.Errorf("invalid Prisma migration directory: %s", dirName)
	}

	timestamp := matches[1]
	name := matches[2]

	// Read the SQL file
	sqlPath := filepath.Join(dirPath, "migration.sql")
	content, err := os.ReadFile(sqlPath)
	if err != nil {
		return adapter.Migration{}, fmt.Errorf("failed to read migration.sql: %w", err)
	}

	parsedTime, _ := time.Parse("20060102150405", timestamp)

	return adapter.Migration{
		Version:   dirName,
		Name:      name,
		FilePath:  sqlPath,
		Checksum:  fmt.Sprintf("%x", simpleChecksum(content)),
		Timestamp: parsedTime,
		UpSQL:     string(content),
		DownSQL:   "",
	}, nil
}

// Prisma migration pattern: YYYYMMDDHHMMSS_name
const prismaMigrationPattern = `^(\d{14})_(.+)$`

// IsMigrationFile checks if a file/directory is a Prisma migration
func (p *PrismaAdapter) IsMigrationFile(path string) bool {
	base := filepath.Base(path)
	if strings.HasPrefix(base, "20") && len(base) >= 14 {
		re := regexp.MustCompile(`^\d{14}_.+$`)
		return re.MatchString(base)
	}
	return false
}

// CanApply checks if the migration can be safely applied
func (p *PrismaAdapter) CanApply(ctx context.Context, mig adapter.Migration) (bool, string, error) {
	// Check if already applied
	applied, err := p.AppliedMigrations(ctx)
	if err != nil {
		return false, "", err
	}

	for _, v := range applied {
		if v == mig.Version {
			return false, "migration already applied", nil
		}
	}

	// Check if there's a gap in migrations
	return true, "", nil
}

// Apply executes a migration against the database
func (p *PrismaAdapter) Apply(ctx context.Context, mig adapter.Migration) error {
	db, err := p.Connect(ctx)
	if err != nil {
		return fmt.Errorf("failed to connect to database: %w", err)
	}
	defer db.Close()

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Execute migration SQL
	if mig.UpSQL != "" {
		if _, err := tx.ExecContext(ctx, mig.UpSQL); err != nil {
			return fmt.Errorf("failed to execute migration: %w", err)
		}
	}

	// Record in _prisma_migrations
	now := time.Now()
	_, err = tx.ExecContext(ctx,
		fmt.Sprintf(`INSERT INTO %s (migration_id, finished_at) VALUES ($1, $2)`, p.MigrationTable()),
		mig.Version, now,
	)
	if err != nil {
		return fmt.Errorf("failed to record migration: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

// ApplyAll applies all pending migrations from a starting version
func (p *PrismaAdapter) ApplyAll(ctx context.Context, from string, to string, migrations []adapter.Migration) error {
	// Find migrations to apply
	var toApply []adapter.Migration
	for _, mig := range migrations {
		if mig.Version > from && (to == "" || mig.Version <= to) {
			toApply = append(toApply, mig)
		}
	}

	// Apply sequentially
	for _, mig := range toApply {
		if err := p.Apply(ctx, mig); err != nil {
			return fmt.Errorf("failed to apply migration %s: %w", mig.Version, err)
		}
	}

	return nil
}

// CurrentSchema returns the current database schema
func (p *PrismaAdapter) CurrentSchema(ctx context.Context) (*adapter.Schema, error) {
	db, err := p.Connect(ctx)
	if err != nil {
		return nil, err
	}
	defer db.Close()

	return p.getSchema(ctx, db)
}

// TargetSchema returns the target schema from migrations
func (p *PrismaAdapter) TargetSchema(ctx context.Context) (*adapter.Schema, error) {
	// For Prisma, the target schema is derived from applying all migrations
	// Since we can't easily calculate the final schema without applying them,
	// we return an empty schema for now (diff will be limited)
	return &adapter.Schema{
		Tables: make(map[string]adapter.SchemaTable),
	}, nil
}

// Diff returns differences between current and target
func (p *PrismaAdapter) Diff(current, target *adapter.Schema) (*adapter.SchemaDiff, error) {
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
func (p *PrismaAdapter) getSchema(ctx context.Context, db *sql.DB) (*adapter.Schema, error) {
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
		table, err := p.getTableSchema(ctx, db, tableName)
		if err != nil {
			return nil, fmt.Errorf("failed to get schema for table %s: %w", tableName, err)
		}
		schema.Tables[tableName] = table
	}

	return schema, nil
}

// getTableSchema returns schema for a single table
func (p *PrismaAdapter) getTableSchema(ctx context.Context, db *sql.DB, tableName string) (adapter.SchemaTable, error) {
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
		Name:     tableName,
		Columns:  make([]adapter.SchemaColumn, 0),
		PrimaryKey: []string{},
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

// simpleChecksum calculates a simple checksum of content
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
				Table:      current.Name,
				Column:     name,
				NewType:    targetCol.Type,
				NewNullable: targetCol.Nullable,
			})
		}
	}

	for name, currentCol := range currentCols {
		if targetCol, exists := targetCols[name]; !exists {
			diff.ColumnsRemoved = append(diff.ColumnsRemoved, adapter.ColumnDiff{
				Table:       current.Name,
				Column:      name,
				OldType:     currentCol.Type,
				OldNullable: currentCol.Nullable,
			})
		} else if currentCol.Type != targetCol.Type || currentCol.Nullable != targetCol.Nullable {
			diff.ColumnsModified = append(diff.ColumnsModified, adapter.ColumnDiff{
				Table:       current.Name,
				Column:      name,
				OldType:     currentCol.Type,
				NewType:     targetCol.Type,
				OldNullable: currentCol.Nullable,
				NewNullable: targetCol.Nullable,
			})
		}
	}

	return diff
}
