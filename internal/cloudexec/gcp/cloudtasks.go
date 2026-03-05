package gcp

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	cloudtasks "cloud.google.com/go/cloudtasks/apiv2"
	taskspb "cloud.google.com/go/cloudtasks/apiv2/cloudtaskspb"

	batchpkg "github.com/alphauslabs/jennah/internal/cloudexec"
)

func init() {
	batchpkg.RegisterGCPCloudTasksProvider(NewGCPCloudTasksProvider)
}

// GCPCloudTasksProvider implements the batch.Provider interface for Google Cloud Tasks.
// Cloud Tasks is used for SIMPLE jobs: ≤500 mCPU, ≤512 MiB, ≤600 s.
//
// Jobs are submitted as HTTP Target tasks that invoke a Cloud Run service
// endpoint with the job configuration as a JSON payload. The target URL is
// configured via ProviderOptions["target_url"].
type GCPCloudTasksProvider struct {
	client    *cloudtasks.Client
	projectID string
	region    string
	// queueID is the Cloud Tasks queue name (default "jennah-simple").
	queueID string
	// targetURL is the HTTP endpoint that will process the task.
	targetURL string
	// serviceAccount is the default GCP service account email used for OIDC
	// authentication when the per-job config does not specify one.
	serviceAccount string
}

// ServiceType returns the service type identifier for Cloud Tasks.
func (p *GCPCloudTasksProvider) ServiceType() string {
	return batchpkg.ServiceTypeCloudTasks
}

// NewGCPCloudTasksProvider creates a new GCP Cloud Tasks provider.
func NewGCPCloudTasksProvider(ctx context.Context, config batchpkg.ProviderConfig) (batchpkg.Provider, error) {
	if config.ProjectID == "" {
		return nil, fmt.Errorf("project_id is required for GCP Cloud Tasks provider")
	}
	if config.Region == "" {
		return nil, fmt.Errorf("region is required for GCP Cloud Tasks provider")
	}

	client, err := cloudtasks.NewClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to create Cloud Tasks client: %w", err)
	}

	queueID := config.ProviderOptions["queue_id"]
	if queueID == "" {
		queueID = "jennah-simple"
	}

	targetURL := config.ProviderOptions["target_url"]
	if targetURL == "" {
		return nil, fmt.Errorf("target_url is required in ProviderOptions for Cloud Tasks provider")
	}

	serviceAccount := config.ProviderOptions["service_account"]
	if serviceAccount == "" {
		return nil, fmt.Errorf("service_account is required in ProviderOptions for Cloud Tasks provider (set CLOUD_TASKS_SERVICE_ACCOUNT)")
	}

	return &GCPCloudTasksProvider{
		client:         client,
		projectID:      config.ProjectID,
		region:         config.Region,
		queueID:        queueID,
		targetURL:      targetURL,
		serviceAccount: serviceAccount,
	}, nil
}

// taskPayload is the JSON payload sent to the target HTTP endpoint.
type taskPayload struct {
	JobID     string            `json:"job_id"`
	RequestID string            `json:"request_id"`
	TenantID  string            `json:"tenant_id"`
	ImageURI  string            `json:"image_uri"`
	Commands  []string          `json:"commands,omitempty"`
	EnvVars   map[string]string `json:"env_vars,omitempty"`
}

// SubmitJob submits a new task to GCP Cloud Tasks.
// The task is created as an HTTP Target task that POSTs the job configuration
// to the configured target URL.
func (p *GCPCloudTasksProvider) SubmitJob(ctx context.Context, config batchpkg.JobConfig) (*batchpkg.JobResult, error) {
	queuePath := fmt.Sprintf("projects/%s/locations/%s/queues/%s",
		p.projectID, p.region, p.queueID)

	payload := taskPayload{
		JobID:     config.JobID,
		RequestID: config.RequestID,
		ImageURI:  config.ImageURI,
		Commands:  config.Commands,
		EnvVars:   config.EnvVars,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal task payload: %w", err)
	}

	taskReq := &taskspb.CreateTaskRequest{
		Parent: queuePath,
		Task: &taskspb.Task{
			MessageType: &taskspb.Task_HttpRequest{
				HttpRequest: &taskspb.HttpRequest{
					HttpMethod: taskspb.HttpMethod_POST,
					Url:        p.targetURL,
					Headers:    map[string]string{"Content-Type": "application/json"},
					Body:       body,
					AuthorizationHeader: &taskspb.HttpRequest_OidcToken{
						OidcToken: &taskspb.OidcToken{
							ServiceAccountEmail: p.resolveServiceAccount(config.ServiceAccount),
						},
					},
				},
			},
		},
	}

	task, err := p.client.CreateTask(ctx, taskReq)
	if err != nil {
		return nil, fmt.Errorf("failed to create Cloud Tasks task: %w", err)
	}

	log.Printf("Cloud Tasks task created: %s", task.GetName())

	return &batchpkg.JobResult{
		CloudResourcePath: task.GetName(),
		InitialStatus:     batchpkg.JobStatusPending,
	}, nil
}

// resolveServiceAccount returns the per-job service account if provided,
// otherwise falls back to the provider-level default.
func (p *GCPCloudTasksProvider) resolveServiceAccount(perJob string) string {
	if perJob != "" {
		return perJob
	}
	return p.serviceAccount
}

// GetJobStatus retrieves the current status of a Cloud Tasks task.
func (p *GCPCloudTasksProvider) GetJobStatus(ctx context.Context, cloudResourcePath string) (batchpkg.JobStatus, error) {
	task, err := p.client.GetTask(ctx, &taskspb.GetTaskRequest{
		Name: cloudResourcePath,
	})
	if err != nil {
		return batchpkg.JobStatusUnknown, fmt.Errorf("failed to get Cloud Tasks task: %w", err)
	}

	return mapCloudTasksStatus(task), nil
}

// CancelJob cancels (deletes) a Cloud Tasks task.
// Cloud Tasks does not have a "cancel" operation; deleting the task prevents execution.
func (p *GCPCloudTasksProvider) CancelJob(ctx context.Context, cloudResourcePath string) error {
	err := p.client.DeleteTask(ctx, &taskspb.DeleteTaskRequest{
		Name: cloudResourcePath,
	})
	if err != nil {
		return fmt.Errorf("failed to delete Cloud Tasks task: %w", err)
	}

	log.Printf("Cloud Tasks task deleted (cancelled): %s", cloudResourcePath)
	return nil
}

// ListJobs lists tasks in the configured queue.
func (p *GCPCloudTasksProvider) ListJobs(ctx context.Context) ([]string, error) {
	queuePath := fmt.Sprintf("projects/%s/locations/%s/queues/%s",
		p.projectID, p.region, p.queueID)

	it := p.client.ListTasks(ctx, &taskspb.ListTasksRequest{
		Parent: queuePath,
	})

	var taskNames []string
	for {
		task, err := it.Next()
		if err != nil {
			// iterator.Done is returned when there are no more items.
			break
		}
		taskNames = append(taskNames, task.GetName())
	}

	return taskNames, nil
}

// mapCloudTasksStatus maps a Cloud Tasks Task to a Jennah JobStatus.
//
// Cloud Tasks task lifecycle:
//   - Task created but not yet dispatched → PENDING
//   - Task dispatched (DispatchCount > 0) but not completed → RUNNING
//   - Task completed (response received) → check response status
func mapCloudTasksStatus(task *taskspb.Task) batchpkg.JobStatus {
	if task == nil {
		return batchpkg.JobStatusUnknown
	}

	// If the task has a last attempt with a response status, it's been dispatched.
	if lastAttempt := task.GetLastAttempt(); lastAttempt != nil {
		if respStatus := lastAttempt.GetResponseStatus(); respStatus != nil {
			code := respStatus.GetCode()
			// HTTP 2xx is success; anything else is a failure.
			if code >= 200 && code < 300 {
				return batchpkg.JobStatusCompleted
			}
			return batchpkg.JobStatusFailed
		}
		// Dispatched but no response yet → RUNNING.
		return batchpkg.JobStatusRunning
	}

	// Not yet dispatched → PENDING.
	return batchpkg.JobStatusPending
}
