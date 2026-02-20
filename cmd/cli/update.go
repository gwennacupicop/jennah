package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

var validStatuses = map[string]bool{
	"PENDING":   true,
	"SCHEDULED": true,
	"RUNNING":   true,
	"COMPLETED": true,
	"FAILED":    true,
	"CANCELLED": true,
}

var updateCmd = &cobra.Command{
	Use:   "update [job-id]",
	Short: "Update a job's status",
	Long:  "jennah update <job-id> --status <status> --tenant-id <id>",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		jobID := args[0]
		tenantID, _ := cmd.Flags().GetString("tenant-id")
		status, _ := cmd.Flags().GetString("status")

		if tenantID == "" {
			return fmt.Errorf("--tenant-id flag is required")
		}
		if status == "" {
			return fmt.Errorf("--status flag is required")
		}

		status = strings.ToUpper(status)
		if !validStatuses[status] {
			return fmt.Errorf("invalid status %q: must be PENDING, SCHEDULED, RUNNING, COMPLETED, FAILED, or CANCELLED", status)
		}

		db, closeDB, err := newDBClient(cmd)
		if err != nil {
			return err
		}
		defer closeDB()

		if err := db.UpdateJobStatus(context.Background(), tenantID, jobID, status); err != nil {
			return fmt.Errorf("failed to update job: %w", err)
		}

		fmt.Printf("\u2713 Job %s status updated to %s\n", jobID, status)
		return nil
	},
}

func init() {
	updateCmd.Flags().String("status", "", "New status (required)")
	updateCmd.MarkFlagRequired("status")
}
