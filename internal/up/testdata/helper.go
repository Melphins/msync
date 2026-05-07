package integration

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"testing"
	"time"

	_ "github.com/lib/pq"
)

// TestDB manages a test database connection
type TestDB struct {
	DB       *sql.DB
	Cleanups []func()
}

// NewTestDB creates a new test database connection
func NewTestDB(t *testing.T) *TestDB {
	connStr := os.Getenv("TEST_DATABASE_URL")
	if connStr == "" {
		connStr = "postgres://test:test@localhost:5433/migrate_sync_test?sslmode=disable"
	}

	db, err := sql.Open("postgres", connStr)
	if err != nil {
		t.Fatalf("failed to connect to test database: %v", err)
	}

	// Wait for connection
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		t.Fatalf("failed to ping test database: %v", err)
	}

	return &TestDB{DB: db}
}

// SetupSchema creates the schema_migrations table
func (tdb *TestDB) SetupSchema(migrationTable string) error {
	_, err := tdb.DB.Exec(fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS %s (
			version VARCHAR(255) PRIMARY KEY
		)
	`, migrationTable))
	return err
}

// InsertMigration records a migration as applied
func (tdb *TestDB) InsertMigration(migrationTable, version string) error {
	_, err := tdb.DB.Exec(fmt.Sprintf(`
		INSERT INTO %s (version) VALUES ($1)
		ON CONFLICT (version) DO NOTHING
	`, migrationTable), version)
	return err
}

// AppliedMigrations returns all applied migration versions
func (tdb *TestDB) AppliedMigrations(migrationTable string) ([]string, error) {
	rows, err := tdb.DB.Query(fmt.Sprintf(`
		SELECT version FROM %s ORDER BY version ASC
	`, migrationTable))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var versions []string
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			return nil, err
		}
		versions = append(versions, v)
	}
	return versions, rows.Err()
}

// DropTable removes a table
func (tdb *TestDB) DropTable(tableName string) error {
	_, err := tdb.DB.Exec(fmt.Sprintf(`DROP TABLE IF EXISTS %s`, tableName))
	return err
}

// Close closes the database connection
func (tdb *TestDB) Close() error {
	return tdb.DB.Close()
}

// AddCleanup adds a cleanup function to be called on Close
func (tdb *TestDB) AddCleanup(fn func()) {
	tdb.Cleanups = append(tdb.Cleanups, fn)
}

// Close runs all cleanup functions and closes the database
func (tdb *TestDB) CloseWithCleanup() error {
	for _, fn := range tdb.Cleanups {
		fn()
	}
	return tdb.DB.Close()
}

// SetupTestDatabase prepares a clean test database
func SetupTestDatabase(t *testing.T, migrationTable string) *TestDB {
	tdb := NewTestDB(t)

	// Clean up any existing data
	tdb.DropTable(migrationTable)

	// Create fresh schema
	if err := tdb.SetupSchema(migrationTable); err != nil {
		t.Fatalf("failed to setup schema: %v", err)
	}

	// Register cleanup
	tdb.AddCleanup(func() {
		tdb.DropTable(migrationTable)
	})

	t.Cleanup(func() {
		tdb.CloseWithCleanup()
	})

	return tdb
}

// WaitForDB waits for the database to be ready
func WaitForDB(timeout time.Duration) error {
	connStr := "postgres://test:test@localhost:5433/migrate_sync_test?sslmode=disable"
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		db, err := sql.Open("postgres", connStr)
		if err == nil {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			if err := db.PingContext(ctx); err == nil {
				db.Close()
				return nil
			}
			db.Close()
		}
		time.Sleep(500 * time.Millisecond)
	}

	return fmt.Errorf("database not ready after %v", timeout)
}
