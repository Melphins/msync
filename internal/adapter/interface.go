package adapter

import (
	"context"
	"time"
)

// Migration represents a single migration file/entry
type Migration struct {
	Version   string
	Name      string
	FilePath  string
	Checksum  string
	Timestamp time.Time
	UpSQL     string
	DownSQL   string
}

// MigrationDetector finds the current migration state of a database or migration directory
type MigrationDetector interface {
	// CurrentVersion returns the latest applied migration version
	CurrentVersion(ctx context.Context) (string, error)

	// AppliedMigrations returns all applied migration versions
	AppliedMigrations(ctx context.Context) ([]string, error)

	// MigrationTable returns the name of the migration tracking table
	MigrationTable() string

	// GetMigrations returns all available migrations from the migration directory
	GetMigrations() ([]Migration, error)
}

// MigrationExecutor applies migrations to a database
type MigrationExecutor interface {
	// Apply runs a specific migration
	Apply(ctx context.Context, migration Migration) error

	// ApplyAll runs migrations from a starting version to target
	ApplyAll(ctx context.Context, from string, to string, migrations []Migration) error

	// CanApply checks if the migration can be safely applied
	CanApply(ctx context.Context, migration Migration) (bool, string, error)
}

// SchemaColumn represents a database column
type SchemaColumn struct {
	Name         string
	Type         string
	Nullable     bool
	DefaultValue *string
	IsPrimaryKey bool
	IsForeignKey bool
	Comment      string
}

// SchemaTable represents a database table
type SchemaTable struct {
	Name       string
	Columns    []SchemaColumn
	PrimaryKey []string
	Indexes    []SchemaIndex
}

// SchemaIndex represents a database index
type SchemaIndex struct {
	Name    string
	Table   string
	Columns []string
	Unique  bool
}

// Schema represents the complete database schema
type Schema struct {
	Tables   map[string]SchemaTable
	Indexes  []SchemaIndex
	Sequences []string
}

// SchemaComparer compares database schemas
type SchemaComparer interface {
	// CurrentSchema returns the current database schema
	CurrentSchema(ctx context.Context) (*Schema, error)

	// TargetSchema returns the target schema (from migrations or reference DB)
	TargetSchema(ctx context.Context) (*Schema, error)

	// Diff returns differences between current and target
	Diff(current, target *Schema) (*SchemaDiff, error)
}

// SchemaDiff represents the differences between two schemas
type SchemaDiff struct {
	TablesAdded   []string
	TablesRemoved []string
	ColumnsAdded  []ColumnDiff
	ColumnsRemoved []ColumnDiff
	ColumnsModified []ColumnDiff
}

// ColumnDiff describes a column-level difference
type ColumnDiff struct {
	Table       string
	Column      string
	OldType     string
	NewType     string
	OldNullable bool
	NewNullable bool
}

// ToJSON converts ColumnDiff to ColumnDiffJSON for serialization
func (d ColumnDiff) ToJSON() ColumnDiffJSON {
	var result ColumnDiffJSON
	result.Table = d.Table
	result.Column = d.Column
	result.OldType = d.OldType
	result.NewType = d.NewType
	result.OldNullable = d.OldNullable
	result.NewNullable = d.NewNullable
	return result
}

// TableDiff describes table-level differences
type TableDiff struct {
	ColumnsAdded    []ColumnDiff
	ColumnsRemoved  []ColumnDiff
	ColumnsModified []ColumnDiff
}


// JSONOutput converts SchemaDiff to JSON-serializable format
func (d *SchemaDiff) JSONOutput() *SchemaDiffJSON {
	return &SchemaDiffJSON{
		TablesAdded:    d.TablesAdded,
		TablesRemoved:  d.TablesRemoved,
		ColumnsAdded:   toColumnDiffJSON(d.ColumnsAdded),
		ColumnsRemoved: toColumnDiffJSON(d.ColumnsRemoved),
		ColumnsModified: toColumnDiffJSON(d.ColumnsModified),
	}
}

// SchemaDiffJSON provides JSON-serializable diff output
type SchemaDiffJSON struct {
	TablesAdded    []string            `json:"tables_added"`
	TablesRemoved  []string            `json:"tables_removed"`
	ColumnsAdded   []ColumnDiffJSON    `json:"columns_added"`
	ColumnsRemoved []ColumnDiffJSON   `json:"columns_removed"`
	ColumnsModified []ColumnDiffJSON  `json:"columns_modified"`
}

// ColumnDiffJSON provides JSON-serializable column diff
type ColumnDiffJSON struct {
	Table       string `json:"table"`
	Column      string `json:"column"`
	OldType     string `json:"old_type,omitempty"`
	NewType     string `json:"new_type,omitempty"`
	OldNullable bool   `json:"old_nullable,omitempty"`
	NewNullable bool   `json:"new_nullable,omitempty"`
}

func toColumnDiffJSON(diffs []ColumnDiff) []ColumnDiffJSON {
	result := make([]ColumnDiffJSON, len(diffs))
	for i, d := range diffs {
		result[i] = d.ToJSON()
	}
	return result
}


// AdapterType represents the type of adapter
type AdapterType string

const (
	AdapterTypePostgres   AdapterType = "postgres"
	AdapterTypeMySQL      AdapterType = "mysql"
	AdapterTypeSQLite     AdapterType = "sqlite"
	AdapterTypeAlembic    AdapterType = "alembic"
	AdapterTypeDjango     AdapterType = "django"
	AdapterTypePrisma     AdapterType = "prisma"
	AdapterTypeRails      AdapterType = "rails"
	AdapterTypeFlyway     AdapterType = "flyway"
	AdapterTypeLiquibase  AdapterType = "liquibase"
)

// Adapter is the main interface combining detector, executor, and comparer capabilities
type Adapter interface {
	MigrationDetector
	MigrationExecutor
	SchemaComparer

	// Type returns the adapter type
	Type() AdapterType

	// Name returns the adapter display name
	Name() string

	// IsMigrationFile checks if a file is a valid migration for this adapter
	IsMigrationFile(filePath string) bool

	// ParseMigrationFile parses a migration file into a Migration struct
	ParseMigrationFile(filePath string) (Migration, error)
}
