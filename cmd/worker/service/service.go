package service

import (
	"sync"
	"time"

	gcpbatch "cloud.google.com/go/batch/apiv1"

	"github.com/alphauslabs/jennah/gen/proto/jennahv1connect"
	"github.com/alphauslabs/jennah/internal/batch"
	"github.com/alphauslabs/jennah/internal/config"
	"github.com/alphauslabs/jennah/internal/database"
)

// WorkerService implements the DeploymentService RPC handlers for the worker.
type WorkerService struct {
	jennahv1connect.UnimplementedDeploymentServiceHandler
	dbClient       *database.Client
	batchProvider  batch.Provider
	jobConfig      *config.JobConfigFile
	workerID       string
	leaseTTL       time.Duration
	claimInterval  time.Duration
	pollers        map[string]*JobPoller // Key: "tenantID/jobID"
	pollersMutex   sync.Mutex
	gcpBatchClient *gcpbatch.Client
}

// NewWorkerService creates a new WorkerService with the given dependencies.
func NewWorkerService(
	dbClient *database.Client,
	batchProvider batch.Provider,
	jobConfig *config.JobConfigFile,
	gcpBatchClient *gcpbatch.Client,
	workerID string,
	leaseTTL time.Duration,
	claimInterval time.Duration,
) *WorkerService {
	return &WorkerService{
		dbClient:       dbClient,
		batchProvider:  batchProvider,
		jobConfig:      jobConfig,
		workerID:       workerID,
		leaseTTL:       leaseTTL,
		claimInterval:  claimInterval,
		pollers:        make(map[string]*JobPoller),
		gcpBatchClient: gcpBatchClient,
	}
}
