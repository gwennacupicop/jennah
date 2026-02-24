package service

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"time"

	"connectrpc.com/connect"

	jennahv1 "github.com/alphauslabs/jennah/gen/proto"
	jennahv1connect "github.com/alphauslabs/jennah/gen/proto/jennahv1connect"
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

	routingKey := fmt.Sprintf("%s-%d", tenantId, time.Now().UnixNano())
	workerIP, workerClient, err := s.getWorkerClient(routingKey)
	if err != nil {
		return nil, err
	}
	log.Printf("Selected worker: %s for tenant (routing key: %s)", workerIP, routingKey)

	workerReq := connect.NewRequest(&jennahv1.SubmitJobRequest{
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
	log.Printf("Job submitted successfully: jobId=%s, worker=%s, status=%s",
		response.Msg.JobId, workerIP, response.Msg.Status)

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

	workerIP, workerClient, err := s.getWorkerClient(tenantId)
	if err != nil {
		return nil, err
	}

	workerReq := connect.NewRequest(&jennahv1.ListJobsRequest{})
	workerReq.Header().Set("X-Tenant-Id", tenantId)

	response, err := workerClient.ListJobs(ctx, workerReq)
	if err != nil {
		log.Printf("ERROR: Worker %s ListJobs failed: %v", workerIP, err)
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("worker failed: %w", err))
	}

	log.Printf("Successfully listed %d jobs for tenant %s via worker %s", len(response.Msg.Jobs), tenantId, workerIP)
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

	workerIP, workerClient, err := s.getWorkerClient(tenantId)
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

	workerIP, workerClient, err := s.getWorkerClient(tenantId)
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

	workerIP, workerClient, err := s.getWorkerClient(tenantId)
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
