package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"time"

	"connectrpc.com/connect"
	jennahv1 "github.com/alphauslabs/jennah/gen/proto"
	"github.com/alphauslabs/jennah/gen/proto/jennahv1connect"
	"github.com/alphauslabs/jennah/internal/hashing"
	"github.com/google/uuid"
)

type GatewayServer struct {
	router        *hashing.Router
	workerClients map[string]jennahv1connect.DeploymentServiceClient
	oauthToTenant map[string]string
	tenants       map[string]*Tenant
}

type Tenant struct {
	TenantId      string
	UserEmail     string
	OAuthProvider string
	OAuthUserId   string
	CreatedAt     time.Time
}

func extractOAuthUser(headers http.Header) (*OAuthUser, error) {
	email := headers.Get("X-OAuth-Email")
	oauthUserId := headers.Get("X-OAuth-UserID")
	provider := headers.Get("X-OAuth-Provider")

	if email == "" || oauthUserId == "" || provider == "" {
		return nil, errors.New("missing required OAuth headers")
	}

	return &OAuthUser{
		Email:    email,
		UserId:   oauthUserId,
		Provider: provider,
	}, nil
}

type OAuthUser struct {
	Email    string
	UserId   string
	Provider string
}

func (s *GatewayServer) getOrCreateTenant(oauthUser *OAuthUser) (string, error) {
	tenantId, exists := s.oauthToTenant[oauthUser.UserId]
	if exists {
		log.Printf("Found existing tenant for user %s: tenant_id=%s", oauthUser.Email, tenantId)
		return tenantId, nil
	}

	tenantId = uuid.New().String()
	tenant := &Tenant{
		TenantId:      tenantId,
		UserEmail:     oauthUser.Email,
		OAuthProvider: oauthUser.Provider,
		OAuthUserId:   oauthUser.UserId,
		CreatedAt:     time.Now(),
	}
	s.tenants[tenantId] = tenant
	s.oauthToTenant[oauthUser.UserId] = tenantId
	//TODO: add database persistence here
	log.Printf("Created new tenant for user %s: tenant_id=%s", oauthUser.Email, tenantId)
	return tenantId, nil
}

func (s *GatewayServer) GetCurrentTenant(
	ctx context.Context,
	req *connect.Request[jennahv1.GetCurrentTenantRequest],
) (*connect.Response[jennahv1.GetCurrentTenantResponse], error) {

	oauthUser, err := extractOAuthUser(req.Header())
	if err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, err)
	}

	tenantId, err := s.getOrCreateTenant(oauthUser)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	tenant := s.tenants[tenantId]

	response := connect.NewResponse(&jennahv1.GetCurrentTenantResponse{
		TenantId:      tenant.TenantId,
		UserEmail:     tenant.UserEmail,
		OauthProvider: tenant.OAuthProvider,
		CreatedAt:     tenant.CreatedAt.Format(time.RFC3339),
	})

	return response, nil
}

func (s *GatewayServer) SubmitJob(
	ctx context.Context,
	req *connect.Request[jennahv1.SubmitJobRequest],
) (*connect.Response[jennahv1.SubmitJobResponse], error) {
	log.Printf("recieved job submission")

	oauthUser, err := extractOAuthUser(req.Header())
	if err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, err)
	}
	tenantId, err := s.getOrCreateTenant(oauthUser)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	log.Printf("Job submission from user %s (tenant_id=%s)", oauthUser.Email, tenantId)
	if req.Msg.ImageUri == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("image_uri is required"))
	}
	workerIP := s.router.GetWorkerIP(tenantId)
	if workerIP == "" {
		return nil, connect.NewError(connect.CodeInternal, errors.New("no worker found for tenant_id"))
	}
	workerClient, exists := s.workerClients[workerIP]
	if !exists {
		return nil, connect.NewError(connect.CodeInternal, errors.New("no worker client found for tenant_id"))
	}

	workerReq := connect.NewRequest(&jennahv1.SubmitJobRequest{
		ImageUri: req.Msg.ImageUri,
		EnvVars:  req.Msg.EnvVars,
	})
	workerReq.Header().Set("X-Tenant-ID", tenantId)
	response, err := workerClient.SubmitJob(ctx, workerReq)
	if err != nil {
		log.Printf("ERROR: Worker %s failed: %v", workerIP, err)
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("worker failed: %w", err))
	}

	response.Msg.WorkerAssigned = workerIP
	return response, nil
}

func (s *GatewayServer) ListJobs(
	ctx context.Context,
	req *connect.Request[jennahv1.ListJobsRequest],
) (*connect.Response[jennahv1.ListJobsResponse], error) {
	log.Printf("Received list jobs request")

	oauthUser, err := extractOAuthUser(req.Header())
	if err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, err)
	}
	tenantID, err := s.getOrCreateTenant(oauthUser)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	workerIP := s.router.GetWorkerIP(tenantID)
	if workerIP == "" {
		return nil, connect.NewError(connect.CodeInternal, errors.New("no worker found"))
	}

	workerClient, exists := s.workerClients[workerIP]
	if !exists {
		return nil, connect.NewError(connect.CodeInternal, errors.New("no worker client found"))
	}
	workerReq := connect.NewRequest(&jennahv1.ListJobsRequest{})
	workerReq.Header().Set("X-Tenant-ID", tenantID)

	response, err := workerClient.ListJobs(ctx, workerReq)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("worker failed: %w", err))
	}

	return response, nil
}

func main() {
	log.Println("Starting gateway...")

	workerIPs := []string{
		"10.128.0.1",
		"10.128.0.2",
		"10.128.0.3",
	}

	router := hashing.NewRouter(workerIPs)
	workerClients := make(map[string]jennahv1connect.DeploymentServiceClient)
	httpClient := &http.Client{}

	for _, ip := range workerIPs {
		workerURL := fmt.Sprintf("http://%s:8081", ip)
		workerClients[ip] = jennahv1connect.NewDeploymentServiceClient(httpClient, workerURL)
	}

	gatewayServer := &GatewayServer{
		router:        router,
		workerClients: workerClients,
		oauthToTenant: make(map[string]string),
		tenants:       make(map[string]*Tenant),
	}

	mux := http.NewServeMux()
	path, handler := jennahv1connect.NewDeploymentServiceHandler(gatewayServer)
	mux.Handle(path, handler)

	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	addr := "0.0.0.0:8080"
	log.Printf("Gateway listening on %s", addr)
	log.Println("OAuth-enabled - tenant_id auto-generated from auth headers")

	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}
