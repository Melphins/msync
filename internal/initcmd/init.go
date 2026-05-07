package initcmd

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

// Runner executes the init command
type Runner struct{}

// NewRunner creates a new init runner
func NewRunner() *Runner {
	return &Runner{}
}

// ConfigPrompt holds a configuration prompt
type ConfigPrompt struct {
	Question string
	Default  string
	Help     string
}

// Run executes the interactive configuration wizard
func (r *Runner) Run() error {
	reader := bufio.NewReader(os.Stdin)

	fmt.Println("╔═══════════════════════════════════════════════════════════╗")
	fmt.Println("║          msync Configuration Wizard                      ║")
	fmt.Println("╚═══════════════════════════════════════════════════════════╝")
	fmt.Println()

	cfg := make(map[string]interface{})

	// Project name
	fmt.Println("1. Project Configuration")
	fmt.Println(strings.Repeat("─", 40))
	projectName := r.promptString(reader, "Project name", r.guessProjectName())
	cfg["project"] = map[string]interface{}{
		"name": projectName,
	}

	// Local database configuration
	fmt.Println()
	fmt.Println("2. Local Database Configuration")
	fmt.Println(strings.Repeat("─", 40))
	fmt.Println()
	fmt.Println("What database adapter are you using locally?")
	localAdapter := r.promptChoice(reader, []string{
		"postgres",
		"rails",
		"django",
		"prisma",
		"alembic",
	}, "postgres")

	localConn := r.promptString(reader, "Local database connection string",
		r.defaultConnectionString(localAdapter))

	localMigDir := r.promptString(reader, "Local migrations directory",
		"./db/migrate")

	cfg["local"] = map[string]interface{}{
		"adapter":       localAdapter,
		"connection":    localConn,
		"migration_dir": localMigDir,
		"migration_table": r.defaultMigrationTable(localAdapter),
	}

	// Target configuration
	fmt.Println()
	fmt.Println("3. Target Configuration")
	fmt.Println(strings.Repeat("─", 40))
	fmt.Println()
	fmt.Println("What is your target environment called? (e.g., staging, production)")
	targetName := r.promptString(reader, "Target name", "production")

	fmt.Printf("\nConfiguring target '%s':\n", targetName)

	fmt.Println("What database adapter is used in the target?")
	targetAdapter := r.promptChoice(reader, []string{
		"postgres",
		"rails",
		"django",
		"prisma",
		"alembic",
	}, localAdapter)

	targetConn := r.promptString(reader, "Target database connection string",
		r.defaultConnectionString(targetAdapter))

	targetMigDir := r.promptString(reader, "Target migrations directory",
		localMigDir)

	targetSchemaOnly := r.promptBool(reader, "Target uses schema-only migrations (read-only)",
		false)

	cfg["targets"] = map[string]interface{}{
		targetName: map[string]interface{}{
			"adapter":        targetAdapter,
			"connection":     targetConn,
			"migration_dir":  targetMigDir,
			"schema_only":    targetSchemaOnly,
		},
	}

	// Sync configuration
	fmt.Println()
	fmt.Println("4. Sync Configuration")
	fmt.Println(strings.Repeat("─", 40))
	fmt.Println()

	warnThreshold := r.promptInt(reader, "Warning threshold (pending migrations to trigger warning)",
		5)
	errorThreshold := r.promptInt(reader, "Error threshold (pending migrations to fail CI)",
		10)
	autoApply := r.promptBool(reader, "Auto-apply migrations when running 'up'",
		false)
	requireConfirmation := r.promptBool(reader, "Require confirmation before applying",
		!autoApply)

	cfg["sync"] = map[string]interface{}{
		"warn_threshold":       warnThreshold,
		"error_threshold":      errorThreshold,
		"auto_apply":           autoApply,
		"require_confirmation": requireConfirmation,
	}

	// Hook configuration
	fmt.Println()
	fmt.Println("5. Pre-commit Hook Configuration")
	fmt.Println(strings.Repeat("─", 40))
	fmt.Println()

	enableHook := r.promptBool(reader, "Enable pre-commit hook", true)

	if enableHook {
		excludeBranches := r.promptString(reader, "Exclude branches (comma-separated)",
			"main,master,staging,develop")
		triggerPaths := r.promptString(reader, "Trigger paths (comma-separated)",
			"migrations/,models/,schema/")

		cfg["hook"] = map[string]interface{}{
			"enabled":          true,
			"exclude_branches": strings.Split(excludeBranches, ","),
			"trigger_paths":    strings.Split(triggerPaths, ","),
			"block_on":         []string{"out_of_sync", "ahead"},
		}
	} else {
		cfg["hook"] = map[string]interface{}{
			"enabled": false,
		}
	}

	// Version
	cfg["version"] = "1"

	// Show summary
	fmt.Println()
	fmt.Println("╔═══════════════════════════════════════════════════════════╗")
	fmt.Println("║              Configuration Summary                       ║")
	fmt.Println("╚═══════════════════════════════════════════════════════════╝")
	fmt.Println()

	// Write configuration
	if err := r.writeConfig(cfg); err != nil {
		return fmt.Errorf("failed to write config: %w", err)
	}

	fmt.Println("[OK] Configuration written to .msync.yml")
	fmt.Println()
	fmt.Println("Next steps:")
	fmt.Println("  1. Review the generated configuration")
	fmt.Println("  2. Run 'msync status' to check sync status")
	fmt.Println("  3. Run 'msync install-hook' to enable pre-commit checks")
	fmt.Println()

	return nil
}

func (r *Runner) promptString(reader *bufio.Reader, question, defaultValue string) string {
	fmt.Printf("%s", question)
	if defaultValue != "" {
		fmt.Printf(" [%s]", defaultValue)
	}
	fmt.Printf(": ")

	input, _ := reader.ReadString('\n')
	input = strings.TrimSpace(input)

	if input == "" {
		return defaultValue
	}
	return input
}

func (r *Runner) promptChoice(reader *bufio.Reader, choices []string, defaultChoice string) string {
	fmt.Println("Choices:")
	for i, choice := range choices {
		marker := "  "
		if choice == defaultChoice {
			marker = "► "
		}
		fmt.Printf("%s%d. %s\n", marker, i+1, choice)
	}

	for {
		fmt.Printf("Select (1-%d, default=%s): ", len(choices), defaultChoice)
		input, _ := reader.ReadString('\n')
		input = strings.TrimSpace(input)

		if input == "" {
			return defaultChoice
		}

		var idx int
		fmt.Sscanf(input, "%d", &idx)
		if idx >= 1 && idx <= len(choices) {
			return choices[idx-1]
		}
	}
}

func (r *Runner) promptBool(reader *bufio.Reader, question string, defaultValue bool) bool {
	defaultStr := "y"
	if !defaultValue {
		defaultStr = "n"
	}

	for {
		fmt.Printf("%s [%s] (y/n): ", question, defaultStr)
		input, _ := reader.ReadString('\n')
		input = strings.TrimSpace(strings.ToLower(input))

		if input == "" {
			return defaultValue
		}

		if input == "y" || input == "yes" {
			return true
		}
		if input == "n" || input == "no" {
			return false
		}
	}
}

func (r *Runner) promptInt(reader *bufio.Reader, question string, defaultValue int) int {
	for {
		fmt.Printf("%s [%d]: ", question, defaultValue)
		input, _ := reader.ReadString('\n')
		input = strings.TrimSpace(input)

		if input == "" {
			return defaultValue
		}

		var val int
		_, err := fmt.Sscanf(input, "%d", &val)
		if err == nil {
			return val
		}
		// If parsing fails, loop and ask again
	}
}

func (r *Runner) guessProjectName() string {
	// Try to guess from git repo or directory name
	// For now, return empty
	return ""
}

func (r *Runner) defaultConnectionString(adapterType string) string {
	switch adapterType {
	case "postgres":
		return "postgres://localhost:5432/your_database"
	case "rails":
		return "postgres://localhost:5432/your_database"
	case "django":
		return "postgres://localhost:5432/your_database"
	case "prisma":
		return "postgres://localhost:5432/your_database"
	case "alembic":
		return "postgres://localhost:5432/your_database"
	default:
		return ""
	}
}

func (r *Runner) defaultMigrationTable(adapterType string) string {
	switch adapterType {
	case "postgres":
		return "schema_migrations"
	case "rails":
		return "schema_migrations"
	case "django":
		return "django_migrations"
	case "prisma":
		return "_prisma_migrations"
	case "alembic":
		return "alembic_version"
	default:
		return "migrations"
	}
}

func (r *Runner) writeConfig(cfg map[string]interface{}) error {
	file, err := os.Create(".msync.yml")
	if err != nil {
		return err
	}
	defer file.Close()

	writer := bufio.NewWriter(file)
	defer writer.Flush()

	// Write YAML configuration
	_, _ = writer.WriteString("# msync configuration\n")
	_, _ = writer.WriteString("# Generated by: msync init\n")
	_, _ = writer.WriteString("\n")
	_, _ = writer.WriteString(fmt.Sprintf("version: %d\n\n", 1))

	// Project
	if proj, ok := cfg["project"].(map[string]interface{}); ok {
		_, _ = writer.WriteString("project:\n")
		_, _ = writer.WriteString(fmt.Sprintf("  name: %s\n", proj["name"]))
		_, _ = writer.WriteString("\n")
	}

	// Local
	if local, ok := cfg["local"].(map[string]interface{}); ok {
		_, _ = writer.WriteString("local:\n")
		_, _ = writer.WriteString(fmt.Sprintf("  adapter: %s\n", local["adapter"]))
		_, _ = writer.WriteString(fmt.Sprintf("  connection: %s\n", local["connection"]))
		_, _ = writer.WriteString(fmt.Sprintf("  migration_dir: %s\n", local["migration_dir"]))
		if mt, ok := local["migration_table"]; ok && mt != "" {
			_, _ = writer.WriteString(fmt.Sprintf("  migration_table: %s\n", mt))
		}
		_, _ = writer.WriteString("\n")
	}

	// Targets
	if targets, ok := cfg["targets"].(map[string]interface{}); ok {
		_, _ = writer.WriteString("targets:\n")
		for name, t := range targets {
			if target, ok := t.(map[string]interface{}); ok {
				_, _ = writer.WriteString(fmt.Sprintf("  %s:\n", name))
				_, _ = writer.WriteString(fmt.Sprintf("    adapter: %s\n", target["adapter"]))
				_, _ = writer.WriteString(fmt.Sprintf("    connection: %s\n", target["connection"]))
				_, _ = writer.WriteString(fmt.Sprintf("    migration_dir: %s\n", target["migration_dir"]))
				if schemaOnly, ok := target["schema_only"]; ok && schemaOnly != false {
					_, _ = writer.WriteString(fmt.Sprintf("    schema_only: %t\n", schemaOnly))
				}
			}
		}
		_, _ = writer.WriteString("\n")
	}

	// Sync
	if sync, ok := cfg["sync"].(map[string]interface{}); ok {
		_, _ = writer.WriteString("sync:\n")
		if warn, ok := sync["warn_threshold"]; ok {
			_, _ = writer.WriteString(fmt.Sprintf("  warn_threshold: %d\n", warn))
		}
		if errThresh, ok := sync["error_threshold"]; ok {
			_, _ = writer.WriteString(fmt.Sprintf("  error_threshold: %d\n", errThresh))
		}
		if auto, ok := sync["auto_apply"]; ok {
			_, _ = writer.WriteString(fmt.Sprintf("  auto_apply: %t\n", auto))
		}
		if reqConf, ok := sync["require_confirmation"]; ok {
			_, _ = writer.WriteString(fmt.Sprintf("  require_confirmation: %t\n", reqConf))
		}
		_, _ = writer.WriteString("\n")
	}

	// Hook
	if hook, ok := cfg["hook"].(map[string]interface{}); ok {
		_, _ = writer.WriteString("hook:\n")
		if enabled, ok := hook["enabled"]; ok {
			_, _ = writer.WriteString(fmt.Sprintf("  enabled: %t\n", enabled))
		}
		if exclude, ok := hook["exclude_branches"]; ok {
			if branches, ok := exclude.([]string); ok && len(branches) > 0 {
				_, _ = writer.WriteString("  exclude_branches:\n")
				for _, branch := range branches {
					_, _ = writer.WriteString(fmt.Sprintf("    - %s\n", branch))
				}
			}
		}
		if paths, ok := hook["trigger_paths"]; ok {
			if triggerPaths, ok := paths.([]string); ok && len(triggerPaths) > 0 {
				_, _ = writer.WriteString("  trigger_paths:\n")
				for _, path := range triggerPaths {
					_, _ = writer.WriteString(fmt.Sprintf("    - %s\n", path))
				}
			}
		}
		if blockOn, ok := hook["block_on"]; ok {
			if conditions, ok := blockOn.([]string); ok && len(conditions) > 0 {
				_, _ = writer.WriteString("  block_on:\n")
				for _, cond := range conditions {
					_, _ = writer.WriteString(fmt.Sprintf("    - %s\n", cond))
				}
			}
		}
	}

	return nil
}
