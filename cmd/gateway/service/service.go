package service

import (
	jennahv1connect "github.com/alphauslabs/jennah/gen/proto/jennahv1connect"
	"github.com/alphauslabs/jennah/internal/hashing"
)

type GatewayService struct {
	router        *hashing.Router
	workerClients map[string]jennahv1connect.DeploymentServiceClient
	oauthToTenant map[string]string
	tenants       map[string]*Tenant
}

func NewGatewayService(
	router *hashing.Router,
	workerClients map[string]jennahv1connect.DeploymentServiceClient,
) *GatewayService {
	return &GatewayService{
		router:        router,
		workerClients: workerClients,
		oauthToTenant: make(map[string]string),
		tenants:       make(map[string]*Tenant),
	}
}
