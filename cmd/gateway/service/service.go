package service

import (
	"sync"

	jennahv1connect "github.com/alphauslabs/jennah/gen/proto/jennahv1connect"
	"github.com/alphauslabs/jennah/internal/database"
	"github.com/alphauslabs/jennah/internal/hashing"
)

type GatewayService struct {
	jennahv1connect.UnimplementedDeploymentServiceHandler
	router        *hashing.Router
	workerClients map[string]jennahv1connect.DeploymentServiceClient
	dbClient      *database.Client
	mu            sync.RWMutex
	oauthToTenant map[string]string
}

func NewGatewayService(
	router *hashing.Router,
	workerClients map[string]jennahv1connect.DeploymentServiceClient,
	dbClient *database.Client,
) *GatewayService {
	return &GatewayService{
		router:        router,
		workerClients: workerClients,
		dbClient:      dbClient,
		oauthToTenant: make(map[string]string),
	}
}
