package config_test

import (
	"os"
	"testing"

	"github.com/Melphins/msync/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConfig_Validate(t *testing.T) {
	// Test with valid config
	cfg := &config.Config{
		Version: 1,
		Local: config.LocalConfig{
			Adapter:      "postgres",
			MigrationDir: "./migrations",
		},
		Targets: map[string]config.TargetConfig{
			"test": {
				Type:         "database",
				Adapter:      "postgres",
				Connection:   "postgres://user:pass@localhost/db",
				MigrationDir: "./migrations",
			},
		},
	}

	err := cfg.Validate()
	assert.NoError(t, err)
}

func TestConfig_Validate_MissingLocalAdapter(t *testing.T) {
	cfg := &config.Config{
		Local: config.LocalConfig{
			MigrationDir: "./migrations",
		},
		Targets: map[string]config.TargetConfig{
			"test": {
				Type:         "database",
				Adapter:      "postgres",
				Connection:   "postgres://user:pass@localhost/db",
				MigrationDir: "./migrations",
			},
		},
	}

	err := cfg.Validate()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "local.adapter")
}

func TestConfig_Validate_NoTargets(t *testing.T) {
	cfg := &config.Config{
		Local: config.LocalConfig{
			Adapter:      "postgres",
			MigrationDir: "./migrations",
		},
		Targets: map[string]config.TargetConfig{},
	}

	err := cfg.Validate()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "at least one target")
}

func TestResolveEnvVars(t *testing.T) {
	// Set test env vars
	os.Setenv("TEST_DB_URL", "postgres://test:test@localhost/testdb")
	os.Setenv("TEST_PORT", "5432")
	defer func() {
		os.Unsetenv("TEST_DB_URL")
		os.Unsetenv("TEST_PORT")
	}()

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "single env var with braces",
			input:    "${TEST_DB_URL}",
			expected: "postgres://test:test@localhost/testdb",
		},
		{
			name:     "single env var without braces",
			input:    "$TEST_DB_URL",
			expected: "postgres://test:test@localhost/testdb",
		},
		{
			name:     "mixed text",
			input:    "postgres://user:$TEST_PORT@localhost/db",
			expected: "postgres://user:5432@localhost/db",
		},
		{
			name:     "multiple env vars",
			input:    "${TEST_DB_URL}?sslmode=disable",
			expected: "postgres://test:test@localhost/testdb?sslmode=disable",
		},
		{
			name:     "no env vars",
			input:    "postgres://localhost:5432/db",
			expected: "postgres://localhost:5432/db",
		},
		{
			name:     "undefined env var preserved",
			input:    "${UNDEFINED_VAR}/db",
			expected: "${UNDEFINED_VAR}/db",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := config.ResolveEnvVars(tt.input)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}
