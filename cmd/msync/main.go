package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

func main() {
	rootCmd := &cobra.Command{
		Use:   "msync",
		Short: "Keep your database migrations in sync across environments",
		Long: `msync is a CLI tool that helps development teams
keep their local databases synchronized with production migrations.

It detects when your local database is behind, shows schema differences,
and helps you apply migrations safely.`,
		Version: "0.0.1",
	}

	rootCmd.AddCommand(statusCmd())
	rootCmd.AddCommand(upCmd())
	rootCmd.AddCommand(diffCmd())
	rootCmd.AddCommand(verifyCmd())
	rootCmd.AddCommand(dashboardCmd())
	rootCmd.AddCommand(initCmd())
	rootCmd.AddCommand(adaptersCmd())
	rootCmd.AddCommand(installHookCmd())
	rootCmd.AddCommand(uninstallHookCmd())
	rootCmd.AddCommand(hookStatusCmd())
	rootCmd.AddCommand(versionCmd())

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func versionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print version information",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Println("msync version 0.0.1")
		},
	}
}
