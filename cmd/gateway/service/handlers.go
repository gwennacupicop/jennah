package service

import (
	"context"
	"errors"
	"fmt"
	"log"

	"connectrpc.com/connect"

	jennahv1 "github.com/alphauslabs/jennah/gen/proto"
)

func (s *GatewayService) GetCurrentTenant(
	ctx context.Context,
	req *connect.Request[jennahv1.GetCurrentTenantRequest],
) (*connect.Response[jennahv1.GetCurrentTenantResponse], error) {
	oauthUser, err := extractOAuthUser(req.Header())
	if err != nil {
		log.Printf("OAuth extraction failed: %v", err)
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("missing or invalid OAuth headers"))
	}
	tenantId, err := s.getOrCreateTenant(oauthUser)
	if err != nil {
		log.Printf("Failed to get or create tenant: %v", err)
		return nil, connect.NewError(connect.CodeInternal, err)
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
		CreatedAt:     tenant.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
	})

	log.Printf("Retrieved tenant info for user %s: tenantId=%s", oauthUser.Email, tenantId)
	return response, nil
}

func (s *GatewayService) SubmitJob(
	ctx context.Context,
	req *connect.Request[jennahv1.SubmitJobRequest],
) (*connect.Response[jennahv1.SubmitJobResponse], error) {
	log.Printf("Received job submission")

	oauthUser, err := extractOAuthUser(req.Header())
	if err != nil {
		log.Printf("OAuth authentication failed: %v", err)
		return nil, connect.NewError(connect.CodeUnauthenticated, err)
	}

	tenantId, err := s.getOrCreateTenant(oauthUser)
	if err != nil {
		log.Printf("Failed to get or create tenant: %v", err)
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	log.Printf("Job submission from user %s (tenantId=%s)", oauthUser.Email, tenantId)

	if req.Msg.ImageUri == "" {
		log.Printf("Error: imageUri is empty")
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("imageUri is required"))
	}

	workerIP := s.router.GetWorkerIP(tenantId)
	if workerIP == "" {
		log.Printf("No worker found for tenantId: %s", tenantId)
		return nil, connect.NewError(connect.CodeInternal, errors.New("no worker found for tenantId"))
	}
	log.Printf("Selected worker: %s for tenant: %s", workerIP, tenantId)

	workerClient, exists := s.workerClients[workerIP]
	if !exists {
		log.Printf("No worker client found for IP: %s", workerIP)
		return nil, connect.NewError(connect.CodeInternal, errors.New("no worker client found for tenantId"))
	}

	workerReq := connect.NewRequest(&jennahv1.SubmitJobRequest{
		ImageUri:         req.Msg.ImageUri,
		EnvVars:          req.Msg.EnvVars,
		ResourceProfile:  req.Msg.ResourceProfile,
		ResourceOverride: req.Msg.ResourceOverride,
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

	oauthUser, err := extractOAuthUser(req.Header())
	if err != nil {
		log.Printf("OAuth authentication failed: %v", err)
		return nil, connect.NewError(connect.CodeUnauthenticated, err)
	}

	tenantId, err := s.getOrCreateTenant(oauthUser)
	if err != nil {
		log.Printf("Failed to get or create tenant: %v", err)
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	log.Printf("List jobs request from user %s (tenantId=%s)", oauthUser.Email, tenantId)

	workerIP := s.router.GetWorkerIP(tenantId)
	if workerIP == "" {
		log.Printf("No worker found for tenantId: %s", tenantId)
		return nil, connect.NewError(connect.CodeInternal, errors.New("no worker found"))
	}
	log.Printf("Selected worker: %s for tenant: %s", workerIP, tenantId)

	workerClient, exists := s.workerClients[workerIP]
	if !exists {
		log.Printf("No worker client found for IP: %s", workerIP)
		return nil, connect.NewError(connect.CodeInternal, errors.New("no worker client found"))
	}

	workerReq := connect.NewRequest(&jennahv1.ListJobsRequest{})
	workerReq.Header().Set("X-Tenant-Id", tenantId)

	response, err := workerClient.ListJobs(ctx, workerReq)
	if err != nil {
		log.Printf("ERROR: Worker %s failed: %v", workerIP, err)
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("worker failed: %w", err))
	}

	log.Printf("Successfully listed %d jobs for tenant %s from worker %s",
		len(response.Msg.Jobs), tenantId, workerIP)

	return response, nil
}
