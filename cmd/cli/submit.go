package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"
)

var submitCmd = &cobra.Command{
	Use:   "submit <job.json>",
	Short: "Submit a job",
	Long:  "jennah submit <job.json> [--wait]\n\nReads job parameters from a JSON file and submits the job.\nUse --wait to stream status changes until the job completes.",
	Args:  cobra.ExactArgs(1),
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

		// Helper to read both camelCase and snake_case keys
		getField := func(camel, snake string) interface{} {
			if v, ok := body[camel]; ok && v != nil && v != "" {
				return v
			}
			return body[snake]
		}
		resourceProfile := getField("resourceProfile", "resource_profile")

		// Normalise keys to camelCase for the gateway
		if _, hasCamel := body["imageUri"]; !hasCamel {
			if v, ok := body["image_uri"]; ok {
				body["imageUri"] = v
				delete(body, "image_uri")
			}
		}
		if _, hasCamel := body["resourceProfile"]; !hasCamel {
			if v, ok := body["resource_profile"]; ok {
				body["resourceProfile"] = v
				delete(body, "resource_profile")
			}
		}
		if _, hasCamel := body["envVars"]; !hasCamel {
			if v, ok := body["env_vars"]; ok {
				body["envVars"] = v
				delete(body, "env_vars")
			}
		}

		// Print header info
		fmt.Printf("Gateway URL: %s\n", gw.baseURL)
		if resourceProfile != nil && resourceProfile != "" {
			fmt.Printf("Resource Profile: %v\n", resourceProfile)
		}

		// Print commands if present
		if cmds := getField("commands", "commands"); cmds != nil {
			fmt.Println()
			fmt.Println("Commands:")
			switch v := cmds.(type) {
			case []interface{}:
				for i, c := range v {
					if i == 0 {
						fmt.Printf("  %v", c)
					} else {
						fmt.Printf(" %v", c)
					}
				}
				fmt.Println()
			}
		}
		fmt.Println()

		// Print full request payload as formatted JSON
		payloadJSON, _ := json.MarshalIndent(body, "", "  ")
		fmt.Println("Request Payload:")
		fmt.Println(string(payloadJSON))
		fmt.Println()
		fmt.Println("Submitting job...")

		statusCode, rawResp, err := gw.postRaw("/jennah.v1.DeploymentService/SubmitJob", body)
		if err != nil {
			return fmt.Errorf("submit failed: %w", err)
		}
		fmt.Printf("HTTP Status: %d\n", statusCode)
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

		// Pretty-print response
		var prettyResp interface{}
		json.Unmarshal(rawResp, &prettyResp)
		respJSON, _ := json.MarshalIndent(prettyResp, "", "  ")
		fmt.Println()
		fmt.Println("Response:")
		fmt.Println(string(respJSON))
		fmt.Println()

		var result struct {
			JobID          string `json:"jobId"`
			Status         string `json:"status"`
			WorkerAssigned string `json:"workerAssigned"`
		}
		json.Unmarshal(rawResp, &result)

		fmt.Println("✅ Job submitted successfully!")
		fmt.Printf("Job ID: %s\n", result.JobID)

		if !wait {
			fmt.Println()
			fmt.Println("Done!")
			return nil
		}

		fmt.Println()
		fmt.Println("Streaming status...")
		fmt.Println("============================================")

		// Handle Ctrl+C gracefully
		stop := make(chan os.Signal, 1)
		signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

		lastStatus := result.Status
		fmt.Printf("  [%s]  %s\n", time.Now().Format("15:04:05"), lastStatus)

		terminalStates := map[string]bool{
			"SUCCEEDED": true,
			"FAILED":    true,
			"CANCELLED": true,
			"DELETED":   true,
		}

		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-stop:
				fmt.Println()
				return nil
			case <-ticker.C:
				jobs, err := fetchJobs(gw)
				if err != nil {
					fmt.Printf("  [%s]  polling error: %v\n", time.Now().Format("15:04:05"), err)
					continue
				}
				job := findJob(jobs, result.JobID)
				if job == nil {
					// Job no longer in list — it has completed
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

func init() {
	submitCmd.Flags().Bool("wait", false, "Stream status changes until the job completes")
}
