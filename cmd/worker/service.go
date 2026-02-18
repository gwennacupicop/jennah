package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"connectrpc.com/connect"
	"github.com/google/uuid"

	jennahv1 "github.com/alphauslabs/jennah/gen/proto"
	"github.com/alphauslabs/jennah/gen/proto/jennahv1connect"
	"github.com/alphauslabs/jennah/internal/batch"
	"github.com/alphauslabs/jennah/internal/config"
	"github.com/alphauslabs/jennah/internal/database"
)

type WorkerServer struct {
	jennahv1connect.UnimplementedDeploymentServiceHandler
	dbClient      *database.Client
	batchProvider batch.Provider
	jobConfig     *config.JobConfigFile
}

func (s *WorkerServer) SubmitJob(
	ctx context.Context,
	req *connect.Request[jennahv1.SubmitJobRequest],
) (*connect.Response[jennahv1.SubmitJobResponse], error) {
	tenantId := req.Header().Get("X-Tenant-Id")
	log.Printf("Received SubmitJob request for tenant: %s", tenantId)

	if tenantId == "" {
		log.Printf("Error: X-Tenant-Id header is missing")
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("X-Tenant-Id header is required"))
	}

	if req.Msg.ImageUri == "" {
		log.Printf("Error: image_uri is empty")
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("image_uri is required"))
	}

	// Generate internal UUID for Spanner primary key
	internalJobID := uuid.New().String()
	log.Printf("Generated internal job ID: %s", internalJobID)

	// Generate cloud provider-compatible job ID (lowercase, starts with letter, no underscores)
	providerJobID := generateProviderJobID(internalJobID)
	log.Printf("Generated provider job ID: %s", providerJobID)

	// Insert job record with PENDING status
	err := s.dbClient.InsertJob(ctx, tenantId, internalJobID, req.Msg.ImageUri, []string{})
	if err != nil {
		log.Printf("Error inserting job to database: %v", err)
		return nil, connect.NewError(
			connect.CodeInternal,
			fmt.Errorf("failed to create job record: %w", err),
		)
	}
	log.Printf("Job %s saved to database with PENDING status", internalJobID)

	// Submit job to cloud batch provider.
	// Resolve resource requirements: named preset merged with any per-field override.
	var resourceOverride *config.ResourceOverride
	if o := req.Msg.ResourceOverride; o != nil {
		resourceOverride = &config.ResourceOverride{
			CPUMillis:             o.CpuMillis,
			MemoryMiB:             o.MemoryMib,
			MaxRunDurationSeconds: o.MaxRunDurationSeconds,
		}
	}

	batchJobConfig := batch.JobConfig{
		JobID:     providerJobID,
		ImageURI:  req.Msg.ImageUri,
		EnvVars:   req.Msg.EnvVars,
		Resources: s.jobConfig.ResolveResources(req.Msg.ResourceProfile, resourceOverride),
	}

	jobResult, err := s.batchProvider.SubmitJob(ctx, batchJobConfig)
	if err != nil {
		log.Printf("Error submitting job to batch provider: %v", err)
		failErr := s.dbClient.FailJob(ctx, tenantId, internalJobID, err.Error())
		if failErr != nil {
			log.Printf("Error updating job status to FAILED: %v", failErr)
		}
		return nil, connect.NewError(
			connect.CodeInternal,
			fmt.Errorf("failed to submit batch job: %w", err),
		)
	}
	log.Printf("Batch job created: %s", jobResult.CloudResourcePath)

	// Update job status and cloud resource path based on provider's initial status
	statusToSet := string(jobResult.InitialStatus)
	if statusToSet == "" || statusToSet == string(batch.JobStatusUnknown) {
		statusToSet = database.JobStatusRunning
	}

	err = s.dbClient.UpdateJobStatusAndCloudPath(ctx, tenantId, internalJobID, statusToSet, jobResult.CloudResourcePath)
	if err != nil {
		log.Printf("Error updating job status to %s: %v", statusToSet, err)
		return nil, connect.NewError(
			connect.CodeInternal,
			fmt.Errorf("failed to update job status: %w", err),
		)
	}
	log.Printf("Job %s status updated to %s with cloud path: %s", internalJobID, statusToSet, jobResult.CloudResourcePath)

	response := connect.NewResponse(&jennahv1.SubmitJobResponse{
		JobId:  internalJobID, // Return internal UUID to client
		Status: statusToSet,
	})

	log.Printf("Successfully submitted job %s for tenant %s", internalJobID, tenantId)
	return response, nil
}

// generateProviderJobID creates a provider-compatible job ID from UUID.
// Most cloud providers require lowercase, starting with letter, no underscores.
func generateProviderJobID(uuid string) string {
	// Use "jennah-" prefix + first 8 chars of UUID (no hyphens)
	cleanUUID := strings.ReplaceAll(uuid, "-", "")
	return "jennah-" + strings.ToLower(cleanUUID[:8])
}

func (s *WorkerServer) ListJobs(
	ctx context.Context,
	req *connect.Request[jennahv1.ListJobsRequest],
) (*connect.Response[jennahv1.ListJobsResponse], error) {
	tenantId := req.Header().Get("X-Tenant-Id")
	log.Printf("Received ListJobs request for tenant: %s", tenantId)

	if tenantId == "" {
		log.Printf("Error: X-Tenant-Id header is missing")
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("X-Tenant-Id header is required"))
	}

	jobs, err := s.dbClient.ListJobs(ctx, tenantId)
	if err != nil {
		log.Printf("Error listing jobs from database: %v", err)
		return nil, connect.NewError(
			connect.CodeInternal,
			fmt.Errorf("failed to list jobs: %w", err),
		)
	}
	log.Printf("Retrieved %d jobs for tenant %s", len(jobs), tenantId)

	protoJobs := make([]*jennahv1.Job, 0, len(jobs))
	for _, job := range jobs {
		protoJob := &jennahv1.Job{
			JobId:     job.JobId,
			TenantId:  job.TenantId,
			ImageUri:  job.ImageUri,
			Status:    job.Status,
			CreatedAt: job.CreatedAt.Format(time.RFC3339),
		}
		protoJobs = append(protoJobs, protoJob)
	}

	response := connect.NewResponse(&jennahv1.ListJobsResponse{
		Jobs: protoJobs,
	})

	log.Printf("Successfully listed %d jobs for tenant %s", len(protoJobs), tenantId)
	return response, nil
}
