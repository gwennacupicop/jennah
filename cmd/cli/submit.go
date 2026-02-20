package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

var submitCmd = &cobra.Command{
	Use:   "submit",
	Short: "Submit a new job",
	Long:  "jennah submit --tenant-id <id> [--status PENDING|SCHEDULED|RUNNING|COMPLETED|FAILED|CANCELLED]",
	RunE: func(cmd *cobra.Command, args []string) error {
		tenantID, _ := cmd.Flags().GetString("tenant-id")
		status, _ := cmd.Flags().GetString("status")

		if tenantID == "" {
			return fmt.Errorf("--tenant-id flag is required")
		}
		if !validStatuses[status] {
			return fmt.Errorf("invalid status %q: must be PENDING, SCHEDULED, RUNNING, COMPLETED, FAILED, or CANCELLED", status)
		}

		db, closeDB, err := newDBClient(cmd)
		if err != nil {
			return err
		}
		defer closeDB()

		jobID := newJobID()
		if err := db.InsertJobWithStatus(context.Background(), tenantID, jobID, status, "", nil); err != nil {
			if strings.Contains(err.Error(), "Parent row") || strings.Contains(err.Error(), "missing") {
				return fmt.Errorf("tenant %q not found — create it first with: jennah tenant create --tenant-id %s --email <email>", tenantID, tenantID)
			}
			return fmt.Errorf("failed to submit job: %w", err)
		}

		fmt.Printf("\u2713 Job submitted successfully\n")
		fmt.Printf("  Job ID:  %s\n", jobID)
		fmt.Printf("  Status:  %s\n", status)
		fmt.Printf("  Tenant:  %s\n", tenantID)
		return nil
	},
}

func init() {
	submitCmd.Flags().String("status", "PENDING", "Job status")
}
