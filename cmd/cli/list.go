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

		fmt.Printf("%-38s  %-12s  %-45s  %s\n", "JOB ID", "STATUS", "IMAGE", "CREATED")
		fmt.Println(strings.Repeat("─", 110))
		for _, j := range jobs {
			img := j.ImageURI
			if len(img) > 43 {
				img = "..." + img[len(img)-40:]
			}
			created := j.CreatedAt
			if t, err := time.Parse(time.RFC3339, j.CreatedAt); err == nil {
				if loc, err := time.LoadLocation("Asia/Manila"); err == nil {
					created = t.In(loc).Format("2006-01-02 15:04:05")
				} else {
					created = t.Local().Format("2006-01-02 15:04:05")
				}
			}
			fmt.Printf("%-38s  %-12s  %-45s  %s\n", j.JobID, j.Status, img, created)
		}
		return nil
	},
}
