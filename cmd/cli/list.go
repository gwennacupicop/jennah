package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List all your jobs",
	Long:  "jennah list\n\nDisplays all jobs submitted under your account.",
	RunE: func(cmd *cobra.Command, args []string) error {
		gw, err := newGatewayClient(cmd)
		if err != nil {
			return err
		}

		jobs, err := fetchJobs(gw)
		if err != nil {
			return err
		}

		if len(jobs) == 0 {
			fmt.Println("No jobs found.")
			return nil
		}

		fmt.Printf("%-20s  %-38s  %-12s  %-10s  %-16s  %-30s  %s\n", "NAME", "JOB ID", "STATUS", "COMPLEXITY", "SERVICE", "IMAGE", "CREATED")
		fmt.Println(strings.Repeat("─", 150))
		for _, j := range jobs {
			name := j.Name
			if name == "" {
				name = "—"
			}
			if len(name) > 18 {
				name = name[:17] + "…"
			}
			img := j.ImageURI
			if len(img) > 28 {
				img = "..." + img[len(img)-25:]
			}
			complexity := friendlyComplexity(j.ComplexityLevel)
			if complexity == "" {
				complexity = "—"
			}
			service := friendlyService(j.AssignedService)
			if service == "" {
				service = "—"
			}
			created := j.CreatedAt
			if t, err := time.Parse(time.RFC3339, j.CreatedAt); err == nil {
				if loc, err := time.LoadLocation("Asia/Manila"); err == nil {
					created = t.In(loc).Format("2006-01-02 15:04:05")
				} else {
					created = t.Local().Format("2006-01-02 15:04:05")
				}
			}
			fmt.Printf("%-20s  %-38s  %-12s  %-10s  %-16s  %-30s  %s\n", name, j.JobID, j.Status, complexity, service, img, created)
		}
		return nil
	},
}
