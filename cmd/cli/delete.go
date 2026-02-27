package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

var deleteCmd = &cobra.Command{
	Use:   "delete <job-id>",
	Short: "Delete a job",
	Long:  "jennah delete <job-id> [--all]\n\nPermanently removes a job record from the system.\nUse --all to delete all jobs at once.",
	Args: func(cmd *cobra.Command, args []string) error {
		all, _ := cmd.Flags().GetBool("all")
		if all {
			return nil
		}
		return cobra.ExactArgs(1)(cmd, args)
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		all, _ := cmd.Flags().GetBool("all")

		gw, err := newGatewayClient(cmd)
		if err != nil {
			return err
		}

		if all {
			return deleteAllJobs(gw)
		}

		return deleteSingleJob(gw, args[0])
	},
}

func init() {
	deleteCmd.Flags().Bool("all", false, "Delete all jobs")
}

func deleteSingleJob(gw *GatewayClient, jobID string) error {
	fmt.Printf("Looking up job %s...\n", jobID)
	jobs, err := fetchJobs(gw)
	if err != nil {
		return fmt.Errorf("failed to fetch jobs: %w", err)
	}
	job := findJob(jobs, jobID)
	if job == nil {
		return fmt.Errorf("job %s not found", jobID)
	}

	fmt.Println("================================")
	fmt.Printf("  Job ID:   %s\n", job.JobID)
	fmt.Printf("  Status:   %s\n", job.Status)
	fmt.Printf("  Image:    %s\n", job.ImageURI)
	created := job.CreatedAt
	if t, err := time.Parse(time.RFC3339, job.CreatedAt); err == nil {
		if loc, err := time.LoadLocation("Asia/Manila"); err == nil {
			created = t.In(loc).Format("2006-01-02 15:04:05")
		} else {
			created = t.Local().Format("2006-01-02 15:04:05")
		}
	}
	fmt.Printf("  Created:  %s\n", created)
	fmt.Println("================================")
	fmt.Println()
	fmt.Printf("Deleting job %s...\n", jobID)

	var result struct {
		JobID   string `json:"jobId"`
		Message string `json:"message"`
	}
	if err := gw.post("/jennah.v1.DeploymentService/DeleteJob", map[string]string{"jobId": jobID}, &result); err != nil {
		if strings.Contains(err.Error(), "not_found") {
			return fmt.Errorf("job %s not found", jobID)
		}
		jobs2, listErr := fetchJobs(gw)
		if listErr == nil && findJob(jobs2, jobID) == nil {
			fmt.Println()
			fmt.Println("✅ Job deleted successfully!")
			return nil
		}
		return fmt.Errorf("delete failed: %w", err)
	}

	fmt.Println()
	fmt.Println("✅ Job deleted successfully!")
	return nil
}

func deleteAllJobs(gw *GatewayClient) error {
	jobs, err := fetchJobs(gw)
	if err != nil {
		return fmt.Errorf("failed to fetch jobs: %w", err)
	}
	if len(jobs) == 0 {
		fmt.Println("No jobs to delete.")
		return nil
	}

	fmt.Printf("Found %d job(s). Deleting all...\n", len(jobs))
	fmt.Println()

	succeeded := 0
	failed := 0
	for _, job := range jobs {
		fmt.Printf("  Deleting %s (%s)... ", job.JobID, job.Status)
		var result struct {
			JobID   string `json:"jobId"`
			Message string `json:"message"`
		}
		err := gw.post("/jennah.v1.DeploymentService/DeleteJob", map[string]string{"jobId": job.JobID}, &result)
		if err != nil {
			jobs2, listErr := fetchJobs(gw)
			if listErr == nil && findJob(jobs2, job.JobID) == nil {
				fmt.Println("✅")
				succeeded++
				continue
			}
			fmt.Printf("❌ failed: %v\n", err)
			failed++
			continue
		}
		fmt.Println("✅")
		succeeded++
	}

	fmt.Println()
	if failed == 0 {
		fmt.Printf("✅ All %d job(s) deleted successfully!\n", succeeded)
	} else {
		fmt.Printf("Deleted %d job(s), %d failed.\n", succeeded, failed)
	}
	return nil
}
