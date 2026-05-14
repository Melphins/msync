package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/Melphins/msync/internal/config"
	"github.com/Melphins/msync/internal/dashboard"
	"github.com/Melphins/msync/internal/diff"
	"github.com/Melphins/msync/internal/hook"
	"github.com/Melphins/msync/internal/initcmd"
	"github.com/Melphins/msync/internal/status"
	"github.com/Melphins/msync/internal/up"
)

func statusCmd() *cobra.Command {
	var targetName string
	var format string

	cmd := &cobra.Command{
		Use:   "status [--target <target>] [--format <json|text>]",
		Short: "Check synchronization status",
		Long:  `Check the migration status between your local database and target environment.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Load configuration
			cfg, err := config.Load(".msync.yml")
			if err != nil {
				return fmt.Errorf("failed to load configuration: %w", err)
			}

			// Validate configuration
			if err := cfg.Validate(); err != nil {
				return fmt.Errorf("invalid configuration: %w", err)
			}

			// Run status check
			runner := status.NewRunner(cfg)
			ctx := cmd.Context()
			_, err = runner.Run(ctx, targetName, format)
			return err
		},
	}

	cmd.Flags().StringVarP(&targetName, "target", "t", "", "Target name to check against")
	cmd.Flags().StringVarP(&format, "format", "f", "text", "Output format (text or json)")

	return cmd
}

func upCmd() *cobra.Command {
	var targetName string
	var dryRun bool
	var yes bool

	cmd := &cobra.Command{
		Use:   "up [--target <target>] [--dry-run] [--yes]",
		Short: "Apply pending migrations",
		Long:  `Apply all pending migrations from the target to your local database.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Load configuration
			cfg, err := config.Load(".msync.yml")
			if err != nil {
				return fmt.Errorf("failed to load configuration: %w", err)
			}

			// Validate configuration
			if err := cfg.Validate(); err != nil {
				return fmt.Errorf("invalid configuration: %w", err)
			}

			// Set up context with cancellation support
			ctx, cancel := context.WithCancel(cmd.Context())
			defer cancel()

			// Handle interrupt signals for graceful shutdown
			sigChan := make(chan os.Signal, 1)
			signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
			go func() {
				<-sigChan
				fmt.Println("\n⚠ Interrupted")
				cancel()
			}()

			// Run up command
			runner := up.NewRunner(cfg)
			return runner.Run(ctx, targetName, dryRun, yes)
		},
	}

	cmd.Flags().StringVarP(&targetName, "target", "t", "", "Target name to sync from")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Show what would be done without applying")
	cmd.Flags().BoolVar(&yes, "yes", false, "Skip confirmation prompt")

	return cmd
}

func diffCmd() *cobra.Command {
	var targetName string
	var mode string

	cmd := &cobra.Command{
		Use:   "diff [--target <target>] [--mode <schema|data>]",
		Short: "Show schema differences",
		Long:  `Display the differences between local and target database schemas.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Load configuration
			cfg, err := config.Load(".msync.yml")
			if err != nil {
				return fmt.Errorf("failed to load configuration: %w", err)
			}

			// Validate configuration
			if err := cfg.Validate(); err != nil {
				return fmt.Errorf("invalid configuration: %w", err)
			}

			// Run diff check
			runner := diff.NewRunner(cfg)
			ctx := cmd.Context()
			return runner.Run(ctx, targetName, mode)
		},
	}

	cmd.Flags().StringVarP(&targetName, "target", "t", "", "Target name to diff against")
	cmd.Flags().StringVarP(&mode, "mode", "m", "schema", "Diff mode (schema or data)")
	cmd.Flags().Lookup("mode").NoOptDefVal = "schema"

	return cmd
}

func verifyCmd() *cobra.Command {
	var targetName string
	var warnThreshold int
	var errorThreshold int

	cmd := &cobra.Command{
		Use:   "verify [--target <target>] [--warn-threshold N] [--error-threshold N]",
		Short: "Verify CI compliance",
		Long:  `Check if the database is synchronized. Exits 0 if synced, 1 if behind, 2 if ahead.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Load configuration
			cfg, err := config.Load(".msync.yml")
			if err != nil {
				return fmt.Errorf("failed to load configuration: %w", err)
			}

			// Validate configuration
			if err := cfg.Validate(); err != nil {
				return fmt.Errorf("invalid configuration: %w", err)
			}

			// Run verification
			runner := status.NewRunner(cfg)
			ctx := cmd.Context()
			result, err := runner.Run(ctx, targetName, "text")
			if err != nil {
				return err
			}

			// Apply threshold logic
			pending := result.TargetCount - result.LocalCount
			if pending < 0 {
				pending = 0
			}

			// Determine exit code based on status and thresholds
			switch result.Status {
			case status.SyncStatusSynced:
				// All good
				return nil
			case status.SyncStatusOutOfSync:
				// Check thresholds
				if errorThreshold > 0 && pending >= errorThreshold {
					fmt.Printf("\n[ERROR] Error threshold exceeded: %d pending migrations (threshold: %d)\n", pending, errorThreshold)
					os.Exit(2)
				} else if warnThreshold > 0 && pending >= warnThreshold {
					fmt.Printf("\n[WARNING] Warning threshold exceeded: %d pending migrations (threshold: %d)\n", pending, warnThreshold)
					os.Exit(1)
				} else if errorThreshold == 0 && warnThreshold == 0 {
					// Default: exit with error if out of sync
					os.Exit(1)
				}
				return nil
			case status.SyncStatusAhead:
				// Local has migrations target doesn't - warning
				fmt.Printf("\n[WARNING] Local database is ahead of target by %d migrations\n", result.LocalCount-result.TargetCount)
				os.Exit(1)
			}

			return nil
		},
	}

	cmd.Flags().StringVarP(&targetName, "target", "t", "", "Target name to verify against")
	cmd.Flags().IntVar(&warnThreshold, "warn-threshold", 0, "Warning threshold for pending migrations")
	cmd.Flags().IntVar(&errorThreshold, "error-threshold", 0, "Error threshold for pending migrations")

	return cmd
}

func dashboardCmd() *cobra.Command {
	var host string
	var port int

	cmd := &cobra.Command{
		Use:   "dashboard [--host <host>] [--port <port>]",
		Short: "Start local web dashboard",
		Long:  `Start a local web dashboard for checking migration synchronization status.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load(".msync.yml")
			if err != nil {
				return fmt.Errorf("failed to load configuration: %w", err)
			}

			if err := cfg.Validate(); err != nil {
				return fmt.Errorf("invalid configuration: %w", err)
			}

			ctx, cancel := context.WithCancel(cmd.Context())
			defer cancel()

			sigChan := make(chan os.Signal, 1)
			signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
			defer signal.Stop(sigChan)
			go func() {
				<-sigChan
				fmt.Println("\nStopping dashboard...")
				cancel()
			}()

			fmt.Printf("msync dashboard: http://%s:%d\n", host, port)
			if err := dashboard.Serve(ctx, cfg, host, port); err != nil && err != context.Canceled {
				return err
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&host, "host", "127.0.0.1", "Host to bind the dashboard server")
	cmd.Flags().IntVarP(&port, "port", "p", 8080, "Port to bind the dashboard server")

	return cmd
}

func initCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "init",
		Short: "Generate configuration",
		Long:  `Create a new .msync.yml configuration file through an interactive wizard.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Check if config already exists
			if _, err := os.Stat(".msync.yml"); err == nil {
				fmt.Println("[WARNING] .msync.yml already exists")
				fmt.Print("Overwrite? (yes/no): ")
				var response string
				fmt.Scanln(&response)
				if response != "yes" && response != "y" {
					fmt.Println("Cancelled.")
					return nil
				}
			}

			// Run interactive configuration wizard
			runner := initcmd.NewRunner()
			return runner.Run()
		},
	}
}

func adaptersCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "adapters",
		Short: "List available adapters",
		Long:  `Show all supported database and migration framework adapters.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.Println("Available adapters:")
			cmd.Println("  Database: postgres, mysql, sqlite")
			cmd.Println("  Framework: alembic, django, prisma, rails, flyway, liquibase")
			return nil
		},
	}
}

func installHookCmd() *cobra.Command {
	var verbose bool

	cmd := &cobra.Command{
		Use:   "install-hook",
		Short: "Install pre-commit hook",
		Long:  `Install the msync pre-commit hook into your Git repository.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Load configuration
			cfg, err := config.Load(".msync.yml")
			if err != nil {
				return fmt.Errorf("failed to load configuration: %w", err)
			}

			// Validate configuration
			if err := cfg.Validate(); err != nil {
				return fmt.Errorf("invalid configuration: %w", err)
			}

			// Create hook manager
			mgr := hook.NewManager(cfg)

			// Check if already installed
			status, err := mgr.GetStatus()
			if err != nil {
				return fmt.Errorf("failed to check hook status: %w", err)
			}

			if status.Installed {
				fmt.Println("[WARNING] pre-commit hook is already installed")
				fmt.Println("   Run 'msync uninstall-hook' to remove it first")
				return nil
			}

			// Install the hook
			if verbose {
				fmt.Println("Installing msync pre-commit hook...")
			}
			if err := mgr.Install(); err != nil {
				return fmt.Errorf("failed to install hook: %w", err)
			}

			fmt.Println("[SUCCESS] pre-commit hook installed successfully!")
			fmt.Println("")
			fmt.Println("The hook will:")
			fmt.Println("  - Check your database sync status before each commit")
			fmt.Println("  - Block commits when out of sync")
			fmt.Println("  - Allow bypass with --no-verify if needed")
			fmt.Println("")
			fmt.Println("Configure hook behavior in .msync.yml under 'hook:'")
			fmt.Println("To uninstall: msync uninstall-hook")

			return nil
		},
	}

	cmd.Flags().BoolVar(&verbose, "verbose", false, "Show detailed output")

	return cmd
}

func uninstallHookCmd() *cobra.Command {
	var verbose bool

	cmd := &cobra.Command{
		Use:   "uninstall-hook",
		Short: "Remove pre-commit hook",
		Long:  `Remove the msync pre-commit hook from your Git repository.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Load configuration
			cfg, err := config.Load(".msync.yml")
			if err != nil {
				return fmt.Errorf("failed to load configuration: %w", err)
			}

			// Create hook manager
			mgr := hook.NewManager(cfg)

			// Check if installed
			status, err := mgr.GetStatus()
			if err != nil {
				return fmt.Errorf("failed to check hook status: %w", err)
			}

			if !status.Installed {
				fmt.Println("[WARNING] pre-commit hook is not installed")
				return nil
			}

			// Confirm uninstall
			fmt.Print("Are you sure you want to remove the msync pre-commit hook? (yes/no): ")
			var response string
			fmt.Scanln(&response)
			if response != "yes" && response != "y" {
				fmt.Println("Cancelled.")
				return nil
			}

			// Uninstall the hook
			if verbose {
				fmt.Println("Removing pre-commit hook...")
			}
			if err := mgr.Uninstall(); err != nil {
				return fmt.Errorf("failed to uninstall hook: %w", err)
			}

			fmt.Println("[SUCCESS] pre-commit hook removed successfully")

			return nil
		},
	}

	cmd.Flags().BoolVar(&verbose, "verbose", false, "Show detailed output")

	return cmd
}

func hookStatusCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "hook-status",
		Short: "Show hook installation status",
		Long:  `Display whether the pre-commit hook is installed and its configuration.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Load configuration
			cfg, err := config.Load(".msync.yml")
			if err != nil {
				return fmt.Errorf("failed to load configuration: %w", err)
			}

			// Create hook manager
			mgr := hook.NewManager(cfg)

			// Get status
			status, err := mgr.GetStatus()
			if err != nil {
				return fmt.Errorf("failed to get hook status: %w", err)
			}

			fmt.Println("msync pre-commit hook status:")
			fmt.Printf("  Installed: %v\n", status.Installed)
			fmt.Println("  Configuration:")
			fmt.Printf("    Enabled: %v\n", status.Config.Enabled)
			if len(status.Config.ExcludeBranches) > 0 {
				fmt.Printf("    Exclude branches: %v\n", status.Config.ExcludeBranches)
			}
			if len(status.Config.TriggerPaths) > 0 {
				fmt.Printf("    Trigger paths: %v\n", status.Config.TriggerPaths)
			}
			if len(status.Config.BlockOn) > 0 {
				fmt.Printf("    Block on: %v\n", status.Config.BlockOn)
			}

			return nil
		},
	}

	return cmd
}
