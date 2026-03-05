package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"
)

var submitCmd = &cobra.Command{
	Use:   "submit <job.json>",
	Short: "Submit a job",
	Long: `Reads base job parameters from a JSON file and submits the job.
Flags override values in the JSON file.

Routing tiers (decided automatically by the gateway):
  SIMPLE  → Cloud Run Jobs (no machine type, cpu ≤ 500m, memory ≤ 512 MiB, timeout ≤ 600s)
  MEDIUM  → Cloud Run Jobs (no machine type, moderate resources, up to 4000m/8192 MiB/3600s)
  COMPLEX → Cloud Batch    (machine type set, or exceeds MEDIUM thresholds)`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		wait, _ := cmd.Flags().GetBool("wait")

		data, err := os.ReadFile(args[0])
		if err != nil {
			return fmt.Errorf("failed to read %s: %w", args[0], err)
		}

		var body map[string]interface{}
		if err := json.Unmarshal(data, &body); err != nil {
			return fmt.Errorf("invalid JSON in %s: %w", args[0], err)
		}

		gw, err := newGatewayClient(cmd)
		if err != nil {
			return err
		}

		// Normalise snake_case keys from JSON file to camelCase
		snakeToCamel := map[string]string{
			"image_uri":         "imageUri",
			"resource_profile":  "resourceProfile",
			"env_vars":          "envVars",
			"machine_type":      "machineType",
			"boot_disk_size_gb": "bootDiskSizeGb",
			"use_spot_vms":      "useSpotVms",
			"service_account":   "serviceAccount",
		}
		for snake, camel := range snakeToCamel {
			if _, hasCamel := body[camel]; !hasCamel {
				if v, ok := body[snake]; ok {
					body[camel] = v
					delete(body, snake)
				}
			}
		}

		// --- Apply CLI flag overrides ---

		if v, _ := cmd.Flags().GetString("machine-type"); v != "" {
			body["machineType"] = v
		}
		if v, _ := cmd.Flags().GetString("profile"); v != "" {
			body["resourceProfile"] = v
		}
		if v, _ := cmd.Flags().GetString("name"); v != "" {
			body["name"] = v
		}
		if v, _ := cmd.Flags().GetString("service-account"); v != "" {
			body["serviceAccount"] = v
		}
		if v, _ := cmd.Flags().GetBool("spot"); v {
			body["useSpotVms"] = true
		}

		// resource_override sub-object (merges with existing if present)
		memMib, _ := cmd.Flags().GetInt64("memory-mib")
		cpuMillis, _ := cmd.Flags().GetInt64("cpu-millis")
		timeoutSec, _ := cmd.Flags().GetInt64("timeout-sec")

		if cpuMillis > 0 && cpuMillis < 1000 {
			return fmt.Errorf("--cpu-millis %d is too low: Cloud Run Jobs requires at least 1 vCPU (min 1000 millis).\nUse --cpu-millis 1000 or higher, or leave it unset for default routing", cpuMillis)
		}

		if memMib > 0 || cpuMillis > 0 || timeoutSec > 0 {
			override, _ := body["resourceOverride"].(map[string]interface{})
			if override == nil {
				override = map[string]interface{}{}
			}
			if memMib > 0 {
				override["memoryMib"] = memMib
			}
			if cpuMillis > 0 {
				override["cpuMillis"] = cpuMillis
			}
			if timeoutSec > 0 {
				override["maxRunDurationSeconds"] = timeoutSec
			}
			body["resourceOverride"] = override
		}

		// --- Print submission header ---
		profile, _ := body["resourceProfile"].(string)
		machineType, _ := body["machineType"].(string)

		fmt.Printf("Gateway URL:  %s\n", gw.baseURL)
		fmt.Printf("User ID:      %s\n", gw.userID)
		fmt.Printf("Tenant ID:    %s\n", gw.tenantID)
		if profile != "" {
			fmt.Printf("Profile:      %s\n", profile)
		}
		if machineType != "" {
			fmt.Printf("Machine Type: %s\n", machineType)
		}
		fmt.Println()

		payloadJSON, _ := json.MarshalIndent(body, "", "  ")
		fmt.Println("Request Payload:")
		fmt.Println(string(payloadJSON))
		fmt.Println()
		fmt.Println("Submitting job...")

		var statusCode int
		var rawResp []byte
		for attempt := 1; ; attempt++ {
			var submitErr error
			statusCode, rawResp, submitErr = gw.postRaw("/jennah.v1.DeploymentService/SubmitJob", body)
			if submitErr == nil {
				break
			}
			fmt.Printf("  [%s]  ⚠ Error (attempt %d): %v\n", time.Now().Format("15:04:05"), attempt, submitErr)
			fmt.Printf("  [%s]  Retrying...\n", time.Now().Format("15:04:05"))
			time.Sleep(3 * time.Second)
		}
		if statusCode != 200 {
			var errResp struct {
				Code    string `json:"code"`
				Message string `json:"message"`
			}
			if json.Unmarshal(rawResp, &errResp) == nil && errResp.Message != "" {
				return fmt.Errorf("%s: %s", errResp.Code, errResp.Message)
			}
			return fmt.Errorf("gateway error %d: %s", statusCode, string(rawResp))
		}

		var result struct {
			JobID           string `json:"jobId"`
			Status          string `json:"status"`
			WorkerAssigned  string `json:"workerAssigned"`
			ComplexityLevel string `json:"complexityLevel"`
			AssignedService string `json:"assignedService"`
			RoutingReason   string `json:"routingReason"`
		}
		json.Unmarshal(rawResp, &result)

		fmt.Println()
		fmt.Println("✅ Job submitted successfully!")
		fmt.Printf("  Job ID:     %s\n", result.JobID)
		fmt.Printf("  Status:     %s\n", result.Status)
		if result.WorkerAssigned != "" {
			fmt.Printf("  Worker:     %s\n", result.WorkerAssigned)
		}
		if result.ComplexityLevel != "" {
			fmt.Printf("  Complexity: %s\n", friendlyComplexity(result.ComplexityLevel))
		}
		if result.AssignedService != "" {
			fmt.Printf("  Service:    %s\n", friendlyService(result.AssignedService))
		}
		if result.RoutingReason != "" {
			fmt.Printf("  Reason:     %s\n", result.RoutingReason)
		}

		if !wait {
			fmt.Println()
			return nil
		}

		fmt.Println()
		fmt.Println("Waiting for job to complete... (Ctrl+C to stop waiting)")
		fmt.Println("============================================")

		stop := make(chan os.Signal, 1)
		signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

		lastStatus := result.Status
		fmt.Printf("  [%s]  %s\n", time.Now().Format("15:04:05"), lastStatus)

		terminalStates := map[string]bool{
			"SUCCEEDED": true,
			"COMPLETED": true,
			"FAILED":    true,
			"CANCELLED": true,
			"DELETED":   true,
		}

		const maxConsecutiveErrors = 20 // ~1.5 min of retries before warning
		consecutiveErrors := 0
		wasRecovering := false

		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-stop:
				fmt.Println()
				return nil
			case <-ticker.C:
				jobs, err := fetchJobs(gw)
				if err != nil {
					consecutiveErrors++
					fmt.Printf("  [%s]  ⚠ Error (attempt %d): %v\n", time.Now().Format("15:04:05"), consecutiveErrors, err)
					fmt.Printf("  [%s]  Retrying...\n", time.Now().Format("15:04:05"))
					wasRecovering = true
					continue
				}

				if wasRecovering {
					fmt.Printf("  [%s]  ✓ Worker recovered (was down for %d poll(s))\n", time.Now().Format("15:04:05"), consecutiveErrors)
					wasRecovering = false
				}
				consecutiveErrors = 0

				job := findJob(jobs, result.JobID)
				if job == nil {
					fmt.Println("============================================")
					fmt.Println("Done!")
					return nil
				}
				if job.Status != lastStatus {
					fmt.Printf("  [%s]  %s → %s\n", time.Now().Format("15:04:05"), lastStatus, job.Status)
					lastStatus = job.Status
				}
				if terminalStates[lastStatus] {
					fmt.Println("============================================")
					fmt.Println("Done!")
					return nil
				}
			}
		}
	},
}

// friendlyComplexity converts proto enum string to a readable label.
func friendlyComplexity(s string) string {
	switch {
	case strings.Contains(s, "SIMPLE"):
		return "SIMPLE"
	case strings.Contains(s, "MEDIUM"):
		return "MEDIUM"
	case strings.Contains(s, "COMPLEX"):
		return "COMPLEX"
	default:
		return s
	}
}

// friendlyService converts proto enum string to a readable label.
func friendlyService(s string) string {
	switch {
	case strings.Contains(s, "CLOUD_TASKS"):
		return "Cloud Tasks"
	case strings.Contains(s, "CLOUD_RUN"):
		return "Cloud Run Jobs"
	case strings.Contains(s, "CLOUD_BATCH"):
		return "Cloud Batch"
	default:
		return s
	}
}

func init() {
	submitCmd.Flags().Bool("wait", false, "Block until the job completes (polls every 5s)")
	submitCmd.Flags().String("machine-type", "", "GCP machine type — routes to Cloud Batch (e.g. e2-standard-4, n1-standard-16)")
	submitCmd.Flags().String("profile", "", "Resource preset — overrides resource flags (e.g. small, medium, large, xlarge)")
	submitCmd.Flags().Int64("memory-mib", 0, "Memory in MiB — overrides profile (e.g. 512, 2048)")
	submitCmd.Flags().Int64("cpu-millis", 0, "CPU in millicores — overrides profile (e.g. 1000, 2000)")
	submitCmd.Flags().Int64("timeout-sec", 0, "Job timeout in seconds (e.g. 600, 3600) — default no limit")
	submitCmd.Flags().String("name", "", "Optional human-readable job name")
	submitCmd.Flags().String("service-account", "", "Custom GCP service account email")
	submitCmd.Flags().Bool("spot", false, "Use Spot VMs (cheaper, preemptible)")
}
