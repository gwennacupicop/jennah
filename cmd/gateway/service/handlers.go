package service

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"time"

	"connectrpc.com/connect"
	"github.com/google/uuid"

	jennahv1 "github.com/alphauslabs/jennah/gen/proto"
	jennahv1connect "github.com/alphauslabs/jennah/gen/proto/jennahv1connect"
	"github.com/alphauslabs/jennah/internal/database"
	"github.com/alphauslabs/jennah/internal/router"
)

func (s *GatewayService) resolveTenant(header http.Header) (string, error) {
	oauthUser, err := extractOAuthUser(header)
	if err != nil {
		log.Printf("OAuth authentication failed: %v", err)
		return "", connect.NewError(connect.CodeUnauthenticated, errors.New("missing or invalid OAuth headers"))
	}

	tenantId, err := s.getOrCreateTenant(oauthUser)
	if err != nil {
		log.Printf("Failed to get or create tenant: %v", err)
		return "", connect.NewError(connect.CodeInternal, err)
	}

	return tenantId, nil
}

func (s *GatewayService) getWorkerClient(routingKey string) (string, jennahv1connect.DeploymentServiceClient, error) {
	workerIP := s.router.GetWorkerIP(routingKey)
	if workerIP == "" {
		log.Printf("No worker found for routingKey: %s", routingKey)
		return "", nil, connect.NewError(connect.CodeInternal, errors.New("no worker found for routing key"))
	}

	workerClient, exists := s.workerClients[workerIP]
	if !exists {
		log.Printf("No worker client found for IP: %s", workerIP)
		return "", nil, connect.NewError(connect.CodeInternal, fmt.Errorf("no worker client found for IP: %s", workerIP))
	}

	return workerIP, workerClient, nil
}

func dbJobToProto(job *database.Job) *jennahv1.Job {
	p := &jennahv1.Job{
		JobId:      job.JobId,
		TenantId:   job.TenantId,
		ImageUri:   job.ImageUri,
		Status:     job.Status,
		CreatedAt:  job.CreatedAt.Format(time.RFC3339),
		UpdatedAt:  job.UpdatedAt.Format(time.RFC3339),
		RetryCount: job.RetryCount,
		MaxRetries: job.MaxRetries,
		Commands:   job.Commands,
	}

	if job.ScheduledAt != nil {
		p.ScheduledAt = job.ScheduledAt.Format(time.RFC3339)
	}
	if job.StartedAt != nil {
		p.StartedAt = job.StartedAt.Format(time.RFC3339)
	}
	if job.CompletedAt != nil {
		p.CompletedAt = job.CompletedAt.Format(time.RFC3339)
	}
	if job.ErrorMessage != nil {
		p.ErrorMessage = *job.ErrorMessage
	}
	if job.GcpBatchJobName != nil {
		p.GcpBatchJobName = *job.GcpBatchJobName
	}
	if job.GcpBatchTaskGroup != nil {
		p.GcpBatchTaskGroup = *job.GcpBatchTaskGroup
	}
	if job.EnvVarsJson != nil {
		p.EnvVarsJson = *job.EnvVarsJson
	}
	if job.Name != nil {
		p.Name = *job.Name
	}
	if job.ResourceProfile != nil {
		p.ResourceProfile = *job.ResourceProfile
	}
	if job.MachineType != nil {
		p.MachineType = *job.MachineType
	}
	if job.BootDiskSizeGb != nil {
		p.BootDiskSizeGb = *job.BootDiskSizeGb
	}
	if job.UseSpotVms != nil {
		p.UseSpotVms = *job.UseSpotVms
	}
	if job.ServiceAccount != nil {
		p.ServiceAccount = *job.ServiceAccount
	}

	return p
}

func (s *GatewayService) GetCurrentTenant(
	ctx context.Context,
	req *connect.Request[jennahv1.GetCurrentTenantRequest],
) (*connect.Response[jennahv1.GetCurrentTenantResponse], error) {
	tenantId, err := s.resolveTenant(req.Header())
	if err != nil {
		return nil, err
	}

	tenant, err := s.dbClient.GetTenant(ctx, tenantId)
	if err != nil {
		log.Printf("Failed to fetch tenant from database: %v", err)
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to fetch tenant: %w", err))
	}

	response := connect.NewResponse(&jennahv1.GetCurrentTenantResponse{
		TenantId:      tenant.TenantId,
		UserEmail:     tenant.UserEmail,
		OauthProvider: tenant.OAuthProvider,
		CreatedAt:     tenant.CreatedAt.Format(time.RFC3339),
	})

	log.Printf("Retrieved tenant info for user %s: tenantId=%s", tenant.UserEmail, tenantId)
	return response, nil
}

func (s *GatewayService) SubmitJob(
	ctx context.Context,
	req *connect.Request[jennahv1.SubmitJobRequest],
) (*connect.Response[jennahv1.SubmitJobResponse], error) {
	log.Printf("Received job submission")

	tenantId, err := s.resolveTenant(req.Header())
	if err != nil {
		return nil, err
	}

	if req.Msg.ImageUri == "" {
		log.Printf("Error: imageUri is empty")
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("imageUri is required"))
	}

	gatewayJobID := uuid.NewString()
	workerIP, workerClient, err := s.getWorkerClient(gatewayJobID)
	if err != nil {
		return nil, err
	}
	log.Printf("Selected worker: %s for tenant (routing key: %s)", workerIP, gatewayJobID)

	routingDecision := router.EvaluateJobComplexity(req.Msg)
	log.Printf("Routing decision: complexity=%s, service=%s, reason=%s",
		routingDecision.Complexity, routingDecision.AssignedService, routingDecision.Reason)

	workerReq := connect.NewRequest(&jennahv1.SubmitJobRequest{
		JobId:            gatewayJobID,
		ImageUri:         req.Msg.ImageUri,
		EnvVars:          req.Msg.EnvVars,
		ResourceProfile:  req.Msg.ResourceProfile,
		ResourceOverride: req.Msg.ResourceOverride,
		Name:             req.Msg.Name,
		MachineType:      req.Msg.MachineType,
		BootDiskSizeGb:   req.Msg.BootDiskSizeGb,
		UseSpotVms:       req.Msg.UseSpotVms,
		ServiceAccount:   req.Msg.ServiceAccount,
		Commands:         req.Msg.Commands,
	})
	workerReq.Header().Set("X-Tenant-Id", tenantId)

	response, err := workerClient.SubmitJob(ctx, workerReq)
	if err != nil {
		log.Printf("ERROR: Worker %s failed: %v", workerIP, err)
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("worker failed: %w", err))
	}

	response.Msg.WorkerAssigned = workerIP
	response.Msg.ComplexityLevel = routingDecision.Complexity.String()
	response.Msg.AssignedService = routingDecision.AssignedService.String()
	response.Msg.RoutingReason = routingDecision.Reason
	log.Printf("Job submitted successfully: jobId=%s, worker=%s, status=%s, complexity=%s, service=%s",
		response.Msg.JobId, workerIP, response.Msg.Status,
		response.Msg.ComplexityLevel, response.Msg.AssignedService)

	return response, nil
}

func (s *GatewayService) ListJobs(
	ctx context.Context,
	req *connect.Request[jennahv1.ListJobsRequest],
) (*connect.Response[jennahv1.ListJobsResponse], error) {
	log.Printf("Received list jobs request")

	tenantId, err := s.resolveTenant(req.Header())
	if err != nil {
		return nil, err
	}

	jobs, err := s.dbClient.ListJobs(ctx, tenantId)
	if err != nil {
		log.Printf("Failed to list jobs from database for tenant %s: %v", tenantId, err)
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to list jobs: %w", err))
	}

	protoJobs := make([]*jennahv1.Job, 0, len(jobs))
	for _, job := range jobs {
		protoJobs = append(protoJobs, dbJobToProto(job))
	}

	response := connect.NewResponse(&jennahv1.ListJobsResponse{Jobs: protoJobs})

	log.Printf("Successfully listed %d jobs for tenant %s directly from database", len(response.Msg.Jobs), tenantId)
	return response, nil
}

func (s *GatewayService) CancelJob(
	ctx context.Context,
	req *connect.Request[jennahv1.CancelJobRequest],
) (*connect.Response[jennahv1.CancelJobResponse], error) {
	log.Printf("Received cancel job request")

	if req.Msg.JobId == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("job_id is required"))
	}

	tenantId, err := s.resolveTenant(req.Header())
	if err != nil {
		return nil, err
	}

	workerIP, workerClient, err := s.getWorkerClient(req.Msg.JobId)
	if err != nil {
		return nil, err
	}

	workerReq := connect.NewRequest(&jennahv1.CancelJobRequest{JobId: req.Msg.JobId})
	workerReq.Header().Set("X-Tenant-Id", tenantId)

	response, err := workerClient.CancelJob(ctx, workerReq)
	if err != nil {
		log.Printf("ERROR: Worker %s CancelJob failed for job %s: %v", workerIP, req.Msg.JobId, err)
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("worker failed: %w", err))
	}

	log.Printf("Job cancelled successfully: jobId=%s, tenantId=%s, worker=%s", req.Msg.JobId, tenantId, workerIP)
	return response, nil
}

func (s *GatewayService) DeleteJob(
	ctx context.Context,
	req *connect.Request[jennahv1.DeleteJobRequest],
) (*connect.Response[jennahv1.DeleteJobResponse], error) {
	log.Printf("Received delete job request")

	if req.Msg.JobId == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("job_id is required"))
	}

	tenantId, err := s.resolveTenant(req.Header())
	if err != nil {
		return nil, err
	}

	workerIP, workerClient, err := s.getWorkerClient(req.Msg.JobId)
	if err != nil {
		return nil, err
	}

	workerReq := connect.NewRequest(&jennahv1.DeleteJobRequest{JobId: req.Msg.JobId})
	workerReq.Header().Set("X-Tenant-Id", tenantId)

	response, err := workerClient.DeleteJob(ctx, workerReq)
	if err != nil {
		log.Printf("ERROR: Worker %s DeleteJob failed for job %s: %v", workerIP, req.Msg.JobId, err)
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("worker failed: %w", err))
	}

	log.Printf("Job deleted successfully: jobId=%s, tenantId=%s, worker=%s", req.Msg.JobId, tenantId, workerIP)
	return response, nil
}

func (s *GatewayService) GetJob(
	ctx context.Context,
	req *connect.Request[jennahv1.GetJobRequest],
) (*connect.Response[jennahv1.GetJobResponse], error) {
	log.Printf("Received get job request")

	if req.Msg.JobId == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("job_id is required"))
	}

	tenantId, err := s.resolveTenant(req.Header())
	if err != nil {
		return nil, err
	}

	workerIP, workerClient, err := s.getWorkerClient(req.Msg.JobId)
	if err != nil {
		return nil, err
	}

	workerReq := connect.NewRequest(&jennahv1.GetJobRequest{JobId: req.Msg.JobId})
	workerReq.Header().Set("X-Tenant-Id", tenantId)

	response, err := workerClient.GetJob(ctx, workerReq)
	if err != nil {
		log.Printf("ERROR: Worker %s GetJob failed for job %s: %v", workerIP, req.Msg.JobId, err)
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("worker failed: %w", err))
	}

	log.Printf("Job retrieved successfully: jobId=%s, tenantId=%s, worker=%s", req.Msg.JobId, tenantId, workerIP)
	return response, nil
}
