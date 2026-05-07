package adapter

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"
)

var (
	ErrMigrationNotFound = errors.New("migration not found")
	ErrInvalidVersion    = errors.New("invalid version format")
)

// FileAdapter provides common functionality for file-based migration adapters
type FileAdapter struct {
	migrationDir string
	migrations   []Migration
}

// NewFileAdapter creates a new FileAdapter with the given migration directory
func NewFileAdapter(migrationDir string) *FileAdapter {
	return &FileAdapter{
		migrationDir: migrationDir,
	}
}

// LoadMigrations scans the migration directory and loads all migrations
func (f *FileAdapter) LoadMigrations() error {
	f.migrations = nil

	if _, err := os.Stat(f.migrationDir); os.IsNotExist(err) {
		return fmt.Errorf("migration directory does not exist: %s", f.migrationDir)
	}

	err := filepath.Walk(f.migrationDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}

		if f.IsMigrationFile(path) {
			mig, err := f.ParseMigrationFile(path)
			if err != nil {
				return fmt.Errorf("failed to parse %s: %w", path, err)
			}
			f.migrations = append(f.migrations, mig)
		}

		return nil
	})

	if err != nil {
		return err
	}

	// Sort migrations by version/timestamp
	sort.Slice(f.migrations, func(i, j int) bool {
		return f.migrations[i].Timestamp.Before(f.migrations[j].Timestamp)
	})

	return nil
}

// GetMigrations returns all loaded migrations
func (f *FileAdapter) GetMigrations() ([]Migration, error) {
	if f.migrations == nil {
		if err := f.LoadMigrations(); err != nil {
			return nil, err
		}
	}
	return f.migrations, nil
}

// FindMigrationByVersion finds a migration by its version string
func (f *FileAdapter) FindMigrationByVersion(version string) (Migration, error) {
	migrations, err := f.GetMigrations()
	if err != nil {
		return Migration{}, err
	}

	for _, m := range migrations {
		if m.Version == version {
			return m, nil
		}
	}

	return Migration{}, ErrMigrationNotFound
}

// GetVersionIndex returns the index of a version in the migration list
func (f *FileAdapter) GetVersionIndex(version string) (int, error) {
	migrations, err := f.GetMigrations()
	if err != nil {
		return -1, err
	}

	for i, m := range migrations {
		if m.Version == version {
			return i, nil
		}
	}

	return -1, ErrMigrationNotFound
}

// CurrentVersion returns the latest migration version (for file-based adapters)
func (f *FileAdapter) CurrentVersion(ctx context.Context) (string, error) {
	migrations, err := f.GetMigrations()
	if err != nil {
		return "", err
	}
	if len(migrations) == 0 {
		return "", nil
	}
	return migrations[len(migrations)-1].Version, nil
}

// AppliedMigrations returns all migration versions (file-based assumes all are applied)
func (f *FileAdapter) AppliedMigrations(ctx context.Context) ([]string, error) {
	migrations, err := f.GetMigrations()
	if err != nil {
		return nil, err
	}
	versions := make([]string, len(migrations))
	for i, m := range migrations {
		versions[i] = m.Version
	}
	return versions, nil
}

// MigrationTable returns empty string for file-based adapters
func (f *FileAdapter) MigrationTable() string {
	return ""
}

// ParseMigrationFile must be implemented by specific adapters
func (f *FileAdapter) ParseMigrationFile(filePath string) (Migration, error) {
	return Migration{}, errors.New("must be implemented by adapter")
}

// IsMigrationFile must be implemented by specific adapters
func (f *FileAdapter) IsMigrationFile(filePath string) bool {
	return false
}

// MigrationFile represents a parsed migration file on disk
type MigrationFile struct {
	FilePath  string
	Timestamp time.Time
	UpSQL     string
	DownSQL   string
}

