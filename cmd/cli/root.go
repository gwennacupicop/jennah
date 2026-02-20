package main

import (
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "jennah",
	Short: "Jennah workload deployment CLI",
	Run: func(cmd *cobra.Command, args []string) {
		cmd.Help()
	},
}

func init() {
	// Disable alphabetical sorting to control command order
	cobra.EnableCommandSorting = false

	// Global flags (available to all subcommands)
	rootCmd.PersistentFlags().String("tenant-id", "", "Tenant ID (required for most commands)")

	// Spanner connection flags (can also be set via env vars)
	rootCmd.PersistentFlags().String("project", "", "GCP project ID (or JENNAH_PROJECT env var)")
	rootCmd.PersistentFlags().String("instance", "", "Spanner instance ID (or JENNAH_INSTANCE env var)")
	rootCmd.PersistentFlags().String("database", "", "Spanner database name (or JENNAH_DATABASE env var)")
	rootCmd.PersistentFlags().MarkHidden("project")
	rootCmd.PersistentFlags().MarkHidden("instance")
	rootCmd.PersistentFlags().MarkHidden("database")

	// Disable completion command
	rootCmd.CompletionOptions.DisableDefaultCmd = true

	// Add subcommands in desired order (help is added automatically at the end)
	rootCmd.AddCommand(submitCmd)
	rootCmd.AddCommand(getCmd)
	rootCmd.AddCommand(listCmd)
	rootCmd.AddCommand(updateCmd)
	rootCmd.AddCommand(deleteCmd)
	rootCmd.AddCommand(tenantCmd)
}
