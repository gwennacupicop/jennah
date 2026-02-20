package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List jobs",
	Long:  "jennah list --tenant-id <id> [--status <status>]",
	RunE: func(cmd *cobra.Command, args []string) error {
		tenantID, _ := cmd.Flags().GetString("tenant-id")
		status, _ := cmd.Flags().GetString("status")

		if tenantID == "" {
			return fmt.Errorf("--tenant-id flag is required")
		}

		db, closeDB, err := newDBClient(cmd)
		if err != nil {
			return err
		}
		defer closeDB()

		ctx := context.Background()

		fmt.Printf("Jobs for tenant: %s\n", tenantID)
		if status != "" {
			status = strings.ToUpper(status)
			fmt.Printf("Status filter:   %s\n", status)
		}
		fmt.Println()

		if status != "" {
			results, err := db.ListJobsByStatus(ctx, tenantID, status)
			if err != nil {
				return fmt.Errorf("failed to list jobs: %w", err)
			}
			if len(results) == 0 {
				fmt.Println("No jobs found.")
				return nil
			}
			fmt.Printf("%-38s  %-12s  %s\n", "JOB ID", "STATUS", "CREATED")
			fmt.Println(strings.Repeat("\u2500", 72))
			for _, job := range results {
				fmt.Printf("%-38s  %-12s  %s\n", job.JobId, job.Status, job.CreatedAt.Format("2006-01-02 15:04:05"))
			}
		} else {
			results, err := db.ListJobs(ctx, tenantID)
			if err != nil {
				return fmt.Errorf("failed to list jobs: %w", err)
			}
			if len(results) == 0 {
				fmt.Println("No jobs found.")
				return nil
			}
			fmt.Printf("%-38s  %-12s  %s\n", "JOB ID", "STATUS", "CREATED")
			fmt.Println(strings.Repeat("\u2500", 72))
			for _, job := range results {
				fmt.Printf("%-38s  %-12s  %s\n", job.JobId, job.Status, job.CreatedAt.Format("2006-01-02 15:04:05"))
			}
		}
		return nil
	},
}

func init() {
	listCmd.Flags().String("status", "", "Filter by status (PENDING, SCHEDULED, RUNNING, COMPLETED, FAILED, CANCELLED)")
}
