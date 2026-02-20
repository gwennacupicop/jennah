package main

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
)

var deleteCmd = &cobra.Command{
	Use:   "delete [job-id]",
	Short: "Delete a job",
	Long:  "jennah delete <job-id> --tenant-id <id>",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		jobID := args[0]
		tenantID, _ := cmd.Flags().GetString("tenant-id")

		if tenantID == "" {
			return fmt.Errorf("--tenant-id flag is required")
		}

		db, closeDB, err := newDBClient(cmd)
		if err != nil {
			return err
		}
		defer closeDB()

		if err := db.DeleteJob(context.Background(), tenantID, jobID); err != nil {
			return fmt.Errorf("failed to delete job: %w", err)
		}

		fmt.Printf("\u2713 Job %s deleted\n", jobID)
		return nil
	},
}

func init() {
	// tenant-id is registered on rootCmd so all sub-commands inherit it
}
