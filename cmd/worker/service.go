package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	batch "cloud.google.com/go/batch/apiv1"
	"cloud.google.com/go/batch/apiv1/batchpb"
	"connectrpc.com/connect"
	"github.com/google/uuid"

	jennahv1 "github.com/alphauslabs/jennah/gen/proto"
	"github.com/alphauslabs/jennah/internal/database"
)

type WorkerServer struct {
	dbClient    *database.Client
	batchClient *batch.Client
	projectId   string
	region      string
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

	jobId := uuid.New().String()
	log.Printf("Generated job Id: %s", jobId)

	err := s.dbClient.InsertJob(ctx, tenantId, jobId, req.Msg.ImageUri, []string{})
	if err != nil {
		log.Printf("Error inserting job to database: %v", err)
		return nil, connect.NewError(
			connect.CodeInternal,
			fmt.Errorf("failed to create job record: %w", err),
		)
	}
	log.Printf("Job %s saved to database with PENDING status", jobId)

	batchJob, err := s.createGCPBatchJob(ctx, jobId, req.Msg.ImageUri, req.Msg.EnvVars)
	if err != nil {
		log.Printf("Error creating GCP Batch job: %v", err)
		failErr := s.dbClient.FailJob(ctx, tenantId, jobId, err.Error())
		if failErr != nil {
			log.Printf("Error updating job status to FAILED: %v", failErr)
		}
		return nil, connect.NewError(
			connect.CodeInternal,
			fmt.Errorf("failed to create GCP Batch job: %w", err),
		)
	}
	log.Printf("GCP Batch job created: %s", batchJob.Name)

	err = s.dbClient.UpdateJobStatus(ctx, tenantId, jobId, database.JobStatusRunning)
	if err != nil {
		log.Printf("Error updating job status to RUNNING: %v", err)
		return nil, connect.NewError(
			connect.CodeInternal,
			fmt.Errorf("failed to update job status: %w", err),
		)
	}
	log.Printf("Job %s status updated to RUNNING", jobId)

	response := connect.NewResponse(&jennahv1.SubmitJobResponse{
		JobId:  jobId,
		Status: database.JobStatusRunning,
	})

	log.Printf("Successfully submitted job %s for tenant %s", jobId, tenantId)
	return response, nil
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

func (s *WorkerServer) createGCPBatchJob(
	ctx context.Context,
	jobId string,
	imageURI string,
	envVars map[string]string,
) (*batchpb.Job, error) {
	parent := fmt.Sprintf("projects/%s/locations/%s", s.projectId, s.region)

	runnable := &batchpb.Runnable{
		Executable: &batchpb.Runnable_Container_{
			Container: &batchpb.Runnable_Container{
				ImageUri: imageURI,
			},
		},
	}

	if len(envVars) > 0 {
		runnable.Environment = &batchpb.Environment{
			Variables: envVars,
		}
	}

	job := &batchpb.Job{
		TaskGroups: []*batchpb.TaskGroup{
			{
				TaskSpec: &batchpb.TaskSpec{
					Runnables: []*batchpb.Runnable{runnable},
				},
				TaskCount: 1,
			},
		},
	}

	req := &batchpb.CreateJobRequest{
		Parent: parent,
		JobId:  jobId,
		Job:    job,
	}

	return s.batchClient.CreateJob(ctx, req)
}
