package main

import (
	"encoding/json"
	"fmt"
)

// Job is the common job structure returned by the gateway.
type Job struct {
	JobID     string `json:"jobId"`
	TenantID  string `json:"tenantId"`
	ImageURI  string `json:"imageUri"`
	Status    string `json:"status"`
	CreatedAt string `json:"createdAt"`
}

// fetchJobs calls ListJobs on the gateway and returns all jobs for the user.
func fetchJobs(gw *GatewayClient) ([]Job, error) {
	var result struct {
		Jobs []Job `json:"jobs"`
	}
	if err := gw.post("/jennah.v1.DeploymentService/ListJobs", map[string]interface{}{}, &result); err != nil {
		return nil, fmt.Errorf("failed to list jobs: %w", err)
	}
	return result.Jobs, nil
}

// findJob returns the job with the given ID from a list, or nil.
func findJob(jobs []Job, jobID string) *Job {
	for i := range jobs {
		if jobs[i].JobID == jobID {
			return &jobs[i]
		}
	}
	return nil
}

// printJobsJSON prints jobs as a JSON array.
func printJobsJSON(jobs []Job) {
	b, _ := json.MarshalIndent(jobs, "", "  ")
	fmt.Println(string(b))
}
