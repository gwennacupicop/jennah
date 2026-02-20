package main

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
)

var getCmd = &cobra.Command{
	Use:   "get [job-id]",
	Short: "Get job details",
	Long:  "jennah get <job-id> --tenant-id <id>",
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

		job, err := db.GetJob(context.Background(), tenantID, jobID)
		if err != nil {
			return fmt.Errorf("job not found: %w", err)
		}

		fmt.Println("Job Details")
		fmt.Println("\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500")
		fmt.Printf("Job ID:    %s\n", job.JobId)
		fmt.Printf("Tenant ID: %s\n", job.TenantId)
		fmt.Printf("Status:    %s\n", job.Status)
		fmt.Printf("Created:   %s\n", job.CreatedAt.Format("2006-01-02 15:04:05"))
		fmt.Printf("Updated:   %s\n", job.UpdatedAt.Format("2006-01-02 15:04:05"))
		if job.ImageUri != "" {
			fmt.Printf("Image URI: %s\n", job.ImageUri)
		}
		if len(job.Commands) > 0 {
			fmt.Printf("Commands:  %v\n", job.Commands)
		}
		if job.ErrorMessage != nil {
			fmt.Printf("Error:     %s\n", *job.ErrorMessage)
		}
		if job.CloudJobResourcePath != nil {
			fmt.Printf("Cloud Path:%s\n", *job.CloudJobResourcePath)
		}
		return nil
	},
}

func init() {
	getCmd.Flags().Bool("history", false, "Show state transition history")
}
