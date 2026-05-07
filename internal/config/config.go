package config

import (
	"bytes"
	"fmt"
	"os"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// Config represents the .msync.yml configuration
type Config struct {
	Version int `yaml:"version"`

	Project   ProjectConfig   `yaml:"project"`
	Targets   map[string]TargetConfig `yaml:"targets"`
	Local     LocalConfig     `yaml:"local"`
	Sync      SyncConfig      `yaml:"sync"`
	Seeds     SeedsConfig     `yaml:"seeds"`
	Team      TeamConfig      `yaml:"team"`
	Hook      HookConfig      `yaml:"hook"`
}

// ProjectConfig holds project identification
type ProjectConfig struct {
	Name string `yaml:"name"`
}

// TargetType indicates what kind of target this is
type TargetType string

const (
	TargetTypeDatabase      TargetType = "database"
	TargetTypeMigrationDir  TargetType = "migration-dir"
	TargetTypeReferenceDB   TargetType = "reference-db"
)

// TargetConfig defines a database or migration directory to sync against
type TargetConfig struct {
	Type         TargetType         `yaml:"type"`
	Adapter      string             `yaml:"adapter"`
	Connection   string             `yaml:"connection"`
	MigrationDir string             `yaml:"migration_dir"`
	SchemaOnly   bool               `yaml:"schema_only"`
	AutoReset    bool               `yaml:"auto_reset"`
}

// LocalConfig defines the local database configuration
type LocalConfig struct {
	Adapter        string `yaml:"adapter"`
	Connection     string `yaml:"connection"`
	MigrationTable string `yaml:"migration_table"`
	MigrationDir   string `yaml:"migration_dir"`
}

// SyncConfig defines sync behavior
type SyncConfig struct {
	WarnThreshold      int `yaml:"warn_threshold"`
	ErrorThreshold     int `yaml:"error_threshold"`
	AutoApply          bool `yaml:"auto_apply"`
	RequireConfirmation bool `yaml:"require_confirmation"`
}

// SeedsConfig defines seed data validation (Pro feature)
type SeedsConfig struct {
	Enabled bool              `yaml:"enabled"`
	Rules   []SeedValidationRule `yaml:"rules"`
}

// SeedValidationRule defines a rule for seed data
type SeedValidationRule struct {
	Table        string                 `yaml:"table"`
	MinRows      int                    `yaml:"min_rows"`
	ExpectedCount int                   `yaml:"expected_count"`
	RequiredValues map[string]interface{} `yaml:"required_values"`
}

// TeamConfig defines team reporting (Pro feature)
type TeamConfig struct {
	Enabled       bool   `yaml:"enabled"`
	APIKey        string `yaml:"api_key"`
	ServerURL     string `yaml:"server_url"`
	TeamName      string `yaml:"team_name"`
	ReportInterval string `yaml:"report_interval"`
}

// HookConfig defines pre-commit hook behavior
type HookConfig struct {
	Enabled       bool     `yaml:"enabled"`
	ExcludeBranches []string `yaml:"exclude_branches"`
	TriggerPaths   []string `yaml:"trigger_paths"`
	BlockOn        []string `yaml:"block_on"`
}

// ResolveEnvVars substitutes environment variables in strings like ${VAR_NAME}
func ResolveEnvVars(input string) (string, error) {
	if !strings.Contains(input, "${") && !strings.Contains(input, "$") {
		return input, nil
	}

	// Simple env var substitution: ${VAR} or $VAR
	re := regexp.MustCompile(`\$\{([^}]+)\}|\$([A-Za-z_][A-Za-z0-9_]*)`)
	result := re.ReplaceAllStringFunc(input, func(match string) string {
		var varName string
		if strings.HasPrefix(match, "${") {
			varName = strings.TrimSuffix(strings.TrimPrefix(match, "${"), "}")
		} else {
			varName = strings.TrimPrefix(match, "$")
		}

		val, ok := os.LookupEnv(varName)
		if !ok {
			return match // Leave as-is if not found
		}
		return val
	})

	return result, nil
}

// Validate checks configuration for common errors
func (c *Config) Validate() error {
	if c.Local.Adapter == "" {
		return fmt.Errorf("local.adapter is required")
	}
	if c.Local.MigrationDir == "" {
		return fmt.Errorf("local.migration_dir is required")
	}
	if len(c.Targets) == 0 {
		return fmt.Errorf("at least one target must be configured")
	}
	return nil
}

// Load reads and parses the YAML configuration file
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	// Remove BOM if present
	data = bytes.TrimPrefix(data, []byte{0xEF, 0xBB, 0xBF})

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	// Resolve environment variables in all relevant fields
	if err := resolveConfigEnvVars(&cfg); err != nil {
		return nil, err
	}

	return &cfg, nil
}

// resolveConfigEnvVars recursively resolves environment variables in config fields
func resolveConfigEnvVars(cfg *Config) error {
	// Resolve local config
	if cfg.Local.Connection != "" {
		conn, err := ResolveEnvVars(cfg.Local.Connection)
		if err != nil {
			return fmt.Errorf("local.connection: %w", err)
		}
		cfg.Local.Connection = conn
	}
	if cfg.Local.MigrationDir != "" {
		dir, err := ResolveEnvVars(cfg.Local.MigrationDir)
		if err != nil {
			return fmt.Errorf("local.migration_dir: %w", err)
		}
		cfg.Local.MigrationDir = dir
	}

	// Resolve targets
	for name, target := range cfg.Targets {
		if target.Connection != "" {
			conn, err := ResolveEnvVars(target.Connection)
			if err != nil {
				return fmt.Errorf("targets.%s.connection: %w", name, err)
			}
			target.Connection = conn
		}
		if target.MigrationDir != "" {
			dir, err := ResolveEnvVars(target.MigrationDir)
			if err != nil {
				return fmt.Errorf("targets.%s.migration_dir: %w", name, err)
			}
			target.MigrationDir = dir
		}
		cfg.Targets[name] = target
	}

	return nil
}
