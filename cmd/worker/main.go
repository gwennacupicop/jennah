package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	gcpbatch "cloud.google.com/go/batch/apiv1"
	"github.com/alphauslabs/jennah/gen/proto/jennahv1connect"
	"github.com/alphauslabs/jennah/internal/batch"
	_ "github.com/alphauslabs/jennah/internal/batch/gcp" // Register GCP provider
	"github.com/alphauslabs/jennah/internal/config"
	"github.com/alphauslabs/jennah/internal/database"
)

func main() {
	log.Println("Starting worker...")

	ctx := context.Background()

	// Load configuration from environment variables
	cfg, err := config.LoadFromEnv()
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}
	log.Printf("Loaded configuration: provider=%s, region=%s", 
		cfg.BatchProvider.Provider, cfg.BatchProvider.Region)

	// Initialize database client
	dbClient, err := database.NewClient(ctx, cfg.Database.ProjectID, cfg.Database.Instance, cfg.Database.Database)
	if err != nil {
		log.Fatalf("Failed to create database client: %v", err)
	}
	defer dbClient.Close()
	log.Printf("Connected to database: %s/%s/%s", 
		cfg.Database.ProjectID, cfg.Database.Instance, cfg.Database.Database)

	// Initialize batch provider
	batchProvider, err := batch.NewProvider(ctx, cfg.BatchProvider)
	if err != nil {
		log.Fatalf("Failed to create batch provider: %v", err)
	}
	log.Printf("Initialized %s batch provider in region: %s", 
		cfg.BatchProvider.Provider, cfg.BatchProvider.Region)

	// Load job configuration from JSON file
	jobConfigPath := os.Getenv("JOB_CONFIG_PATH")
	if jobConfigPath == "" {
		jobConfigPath = "config/job-config.json" // Default path
	}
	jobConfig, err := config.LoadJobConfig(jobConfigPath)
	if err != nil {
		log.Fatalf("Failed to load job config: %v", err)
	}
	log.Printf("Loaded job config from: %s", jobConfigPath)
	log.Printf("Default resources: CPU=%dm, Memory=%dMiB, MaxRuntime=%ds",
		jobConfig.DefaultResources.CPUMillis,
		jobConfig.DefaultResources.MemoryMiB,
		jobConfig.DefaultResources.MaxRunDurationSeconds)

	// Initialize GCP Batch client
	gcpBatchClient, err := gcpbatch.NewClient(ctx)
	if err != nil {
		log.Fatalf("Failed to create GCP Batch client: %v", err)
	}
	defer gcpBatchClient.Close()
	log.Println("Initialized GCP Batch client")

	workerServer := &WorkerServer{
		dbClient:       dbClient,
		batchProvider:  batchProvider,
		jobConfig:      jobConfig,
		pollers:        make(map[string]*JobPoller),
		gcpBatchClient: gcpBatchClient,
	}

	mux := http.NewServeMux()
	path, handler := jennahv1connect.NewDeploymentServiceHandler(workerServer)
	mux.Handle(path, handler)
	log.Printf("ConnectRPC handler registered at path: %s", path)

	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})
	log.Println("Health check endpoint: /health")

	addr := fmt.Sprintf("0.0.0.0:%s", cfg.ServerPort)
	server := &http.Server{
		Addr:    addr,
		Handler: mux,
	}

	sigCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go func() {
		log.Printf("Worker listening on %s", addr)
		log.Println("Available endpoints:")
		log.Printf("  • POST %sSubmitJob", path)
		log.Printf("  • POST %sListJobs", path)
		log.Printf("  • GET  /health")
		log.Printf("Worker configured for provider: %s, region: %s", 
			cfg.BatchProvider.Provider, cfg.BatchProvider.Region)
		log.Println("")

		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Failed to start server: %v", err)
		}
	}()

	<-sigCtx.Done()
	log.Println("Shutdown signal received, gracefully shutting down...")

	// Stop all active job pollers
	workerServer.StopAllPollers()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Printf("Error during server shutdown: %v", err)
	}

	log.Println("Worker stopped")
}
