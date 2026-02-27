package main

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"
)

var getCmd = &cobra.Command{
	Use:   "get [job-id]",
	Short: "Get job details",
	Long:  "jennah get <job-id> [--output json]\n\nFetches and displays the details of a specific job by ID.",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		jobID := args[0]
		outputFmt, _ := cmd.Flags().GetString("output")

		gw, err := newGatewayClient(cmd)
		if err != nil {
			return err
		}

		jobs, err := fetchJobs(gw)
		if err != nil {
			return fmt.Errorf("failed to fetch jobs: %w", err)
		}

		j := findJob(jobs, jobID)
		if j == nil {
			return fmt.Errorf("job %q not found", jobID)
		}

		if outputFmt == "json" {
			printJobsJSON([]Job{*j})
			return nil
		}

		fmt.Println("Job Details")
		fmt.Println("───────────")
		fmt.Printf("Job ID:   %s\n", j.JobID)
		fmt.Printf("Status:   %s\n", j.Status)
		fmt.Printf("Image:    %s\n", j.ImageURI)
		created := j.CreatedAt
		if t, err := time.Parse(time.RFC3339, j.CreatedAt); err == nil {
			if loc, err := time.LoadLocation("Asia/Manila"); err == nil {
				created = t.In(loc).Format("2006-01-02 15:04:05")
			} else {
				created = t.Local().Format("2006-01-02 15:04:05")
			}
		}
		fmt.Printf("Created:  %s\n", created)
		fmt.Printf("Tenant:   %s\n", j.TenantID)
		return nil
	},
}

func init() {
	getCmd.Flags().String("output", "", "Output format: json")
}
