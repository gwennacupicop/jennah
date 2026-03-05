package main

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

var getCmd = &cobra.Command{
	Use:   "get <job-id>",
	Short: "Get job details",
	Long:  "jennah get <job-id> [--output json]\n\nFetches and displays full details of a specific job by ID.",
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

		pht, _ := time.LoadLocation("Asia/Manila")
		fmtTime := func(raw string) string {
			if raw == "" {
				return "—"
			}
			if t, err := time.Parse(time.RFC3339, raw); err == nil {
				return t.In(pht).Format("2006-01-02 15:04:05 PHT")
			}
			return raw
		}

		dash := func(s string) string {
			if s == "" {
				return "—"
			}
			return s
		}
		dashNum := func(n json.Number) string {
			s := n.String()
			if s == "" || s == "0" {
				return "—"
			}
			return s
		}

		retries := "—"
		if rc := j.RetryCount.String(); rc != "" && rc != "0" {
			retries = rc + " / " + j.MaxRetries.String()
		}

		bootDisk := "—"
		if b := j.BootDiskSizeGb.String(); b != "" && b != "0" {
			bootDisk = b + " GB"
		}

		spotVms := "no"
		if j.UseSpotVms {
			spotVms = "yes"
		}

		commands := "—"
		if len(j.Commands) > 0 {
			commands = strings.Join(j.Commands, " ")
		}

		fmt.Println("Job Details")
		fmt.Println("───────────────────────────────────────")
		fmt.Printf("Job ID:          %s\n", j.JobID)
		fmt.Printf("Name:            %s\n", dash(j.Name))
		fmt.Printf("Tenant:          %s\n", j.TenantID)
		fmt.Printf("Status:          %s\n", j.Status)
		fmt.Printf("Error:           %s\n", dash(j.ErrorMessage))
		fmt.Printf("Retries:         %s\n", retries)
		fmt.Printf("Created:         %s\n", fmtTime(j.CreatedAt))
		fmt.Printf("Updated:         %s\n", fmtTime(j.UpdatedAt))
		fmt.Printf("Scheduled:       %s\n", fmtTime(j.ScheduledAt))
		fmt.Printf("Started:         %s\n", fmtTime(j.StartedAt))
		fmt.Printf("Completed:       %s\n", fmtTime(j.CompletedAt))
		fmt.Printf("Commands:        %s\n", commands)
		fmt.Printf("Profile:         %s\n", dash(j.ResourceProfile))
		fmt.Printf("Memory (MiB):    %s\n", dashNum(j.ResourceOverride.MemoryMib))
		fmt.Printf("CPU (millis):    %s\n", dashNum(j.ResourceOverride.CpuMillis))
		fmt.Printf("Timeout (sec):   %s\n", dashNum(j.ResourceOverride.MaxRunDurationSeconds))
		fmt.Printf("Machine Type:    %s\n", dash(j.MachineType))
		fmt.Printf("Boot Disk:       %s\n", bootDisk)
		fmt.Printf("Spot VMs:        %s\n", spotVms)
		fmt.Printf("Service Account: %s\n", dash(j.ServiceAccount))
		fmt.Printf("GCP Job Path:    %s\n", dash(j.GcpBatchJobPath))
		fmt.Printf("Image:           %s\n", dash(j.ImageURI))
		if j.EnvVarsJson != "" && j.EnvVarsJson != "{}" && j.EnvVarsJson != "null" {
			var envMap map[string]string
			if json.Unmarshal([]byte(j.EnvVarsJson), &envMap) == nil && len(envMap) > 0 {
				fmt.Printf("Env Vars:\n")
				for k, v := range envMap {
					fmt.Printf("  %s=%s\n", k, v)
				}
			}
		} else {
			fmt.Printf("Env Vars:        —\n")
		}

		return nil
	},
}

func init() {
	getCmd.Flags().String("output", "", "Output format: json")
}
