package prisma

import (
	"regexp"
	"testing"
	"time"

	"github.com/Melphins/msync/internal/adapter"
	"github.com/stretchr/testify/assert"
)

func TestPrismaAdapter_NameAndType(t *testing.T) {
	a := New(nil, "")
	assert.Equal(t, "Prisma Migrate", a.Name())
	assert.Equal(t, adapter.AdapterTypePrisma, a.Type())
}

func TestPrismaAdapter_MigrationTable(t *testing.T) {
	a := New(nil, "./migrations")
	assert.Equal(t, "_prisma_migrations", a.MigrationTable())
}

func TestPrismaAdapter_MigrationDir(t *testing.T) {
	dir := "/path/to/migrations"
	a := New(nil, dir)
	assert.Equal(t, dir, a.MigrationDir())
}

func TestPrismaAdapter_IsMigrationFile(t *testing.T) {
	a := New(nil, "")

	tests := []struct {
		path     string
		expected bool
	}{
		{"migrations/20240101120000_init_users/", true},
		{"migrations/20240101120000_add_email/", true},
		{"20240101120000_create_posts/", true},
		{"migrations/20240101_init/", false}, // Not 14-digit timestamp
		{"migrations/init_users/", false},     // No timestamp prefix
		{"somefile.sql", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			result := a.IsMigrationFile(tt.path)
			assert.Equal(t, tt.expected, result, "IsMigrationFile(%s)", tt.path)
		})
	}
}

func TestPrismaAdapter_ParseMigrationFile(t *testing.T) {
	tests := []struct {
		name        string
		dirName     string
		sqlContent  string
		wantErr     bool
		wantVersion string
		wantName    string
	}{
		{
			name:        "Valid Prisma migration directory",
			dirName:     "20240101120000_add_users",
			sqlContent:  "CREATE TABLE users (id INT PRIMARY KEY);",
			wantErr:     false,
			wantVersion: "20240101120000_add_users",
			wantName:    "add_users",
		},
		{
			name:        "Migration with underscore in name",
			dirName:     "20240101130000_add_user_email_verified",
			sqlContent:  "ALTER TABLE users ADD COLUMN email_verified BOOLEAN;",
			wantErr:     false,
			wantVersion: "20240101130000_add_user_email_verified",
			wantName:    "add_user_email_verified",
		},
		{
			name:        "Invalid directory name",
			dirName:     "invalid_name",
			sqlContent:  "",
			wantErr:     true,
		},
		{
			name:        "Directory without timestamp",
			dirName:     "add_users",
			sqlContent:  "",
			wantErr:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			re := regexp.MustCompile(prismaMigrationPattern)
			matches := re.FindStringSubmatch(tt.dirName)

			if tt.wantErr {
				assert.Empty(t, matches, "should not match invalid dir name")
				return
			}

			assert.Len(t, matches, 3, "should have 2 capture groups")
			assert.Equal(t, tt.dirName, matches[0])
			assert.Equal(t, tt.wantName, matches[2])
		})
	}
}

func TestPrismaAdapter_GetMigrations(t *testing.T) {
	t.Skip("Requires test fixtures with migration directories")
}

func TestPrismaAdapter_CurrentVersion(t *testing.T) {
	t.Skip("Requires running PostgreSQL database")
}

func TestPrismaAdapter_AppliedMigrations(t *testing.T) {
	t.Skip("Requires running PostgreSQL database")
}

func TestPrismaAdapter_Apply(t *testing.T) {
	t.Skip("Requires running PostgreSQL database")
}

// Test the timestamp parsing
func TestPrismaAdapter_TimestampParsing(t *testing.T) {
	tests := []struct {
		timestamp string
		valid     bool
	}{
		{"20240101120000", true},
		{"20250101153045", true},
		{"20241301120000", false}, // Invalid month
		{"20240132120000", false}, // Invalid day
		{"20240101250000", false}, // Invalid hour (25)
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.timestamp, func(t *testing.T) {
			_, err := time.Parse("20060102150405", tt.timestamp)
			if tt.valid {
				assert.NoError(t, err)
			} else {
				assert.Error(t, err)
			}
		})
	}
}

func TestPrismaAdapter_MigrationPattern(t *testing.T) {
	tests := []struct {
		path     string
		expected bool
	}{
		{"20240101120000_init_users", true},
		{"20240101120000_add_email_verified", true},
		{"20240101120000_", false},      // No name after underscore
		{"20240101120000", false},       // No underscore
		{"202401011200", false},         // Too short
		{"2024-01-01-120000_init", false},
		{"init_users", false},
		{"", false},
	}

	re := regexp.MustCompile(prismaMigrationPattern)

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			result := re.MatchString(tt.path)
			assert.Equal(t, tt.expected, result, "pattern matches %s", tt.path)
		})
	}
}

func TestPrismaAdapter_FindMigrationByVersion(t *testing.T) {
	a := New(nil, "./testdata/migrations")

	// Test with a mock migrations list
	a.migrations = []adapter.Migration{
		{Version: "20240101120000_init", Name: "init"},
		{Version: "20240102120000_add_users", Name: "add_users"},
		{Version: "20240103120000_add_posts", Name: "add_posts"},
	}

	tests := []struct {
		version  string
		wantErr  bool
		wantName string
	}{
		{"20240101120000_init", false, "init"},
		{"20240103120000_add_posts", false, "add_posts"},
		{"99999999999999_nonexistent", true, ""},
	}

	for _, tt := range tests {
		t.Run(tt.version, func(t *testing.T) {
			mig, err := a.FindMigrationByVersion(tt.version)
			if tt.wantErr {
				assert.Error(t, err)
				assert.Empty(t, mig.Version)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.wantName, mig.Name)
				assert.Equal(t, tt.version, mig.Version)
			}
		})
	}
}
