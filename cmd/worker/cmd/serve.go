package cmd

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	gcpbatch "cloud.google.com/go/batch/apiv1"
	"github.com/spf13/cobra"

	"github.com/alphauslabs/jennah/cmd/worker/service"
	"github.com/alphauslabs/jennah/gen/proto/jennahv1connect"
	"github.com/alphauslabs/jennah/internal/batch"
	_ "github.com/alphauslabs/jennah/internal/batch/gcp" // Register GCP provider
	"github.com/alphauslabs/jennah/internal/config"
	"github.com/alphauslabs/jennah/internal/database"
)

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start the worker server",
	Long:  `Start the worker server to handle job deployment and GCP Batch orchestration.`,
	RunE:  runServe,
}

func runServe(cmd *cobra.Command, args []string) error {
	log.Println("Starting worker...")

	ctx := context.Background()

	// Load configuration from environment variables.
	cfg, err := config.LoadFromEnv()
	if err != nil {
		return fmt.Errorf("failed to load configuration: %w", err)
	}
	log.Printf("Loaded configuration: provider=%s, region=%s",
		cfg.BatchProvider.Provider, cfg.BatchProvider.Region)

	// Initialize database client.
	dbClient, err := database.NewClient(ctx, cfg.Database.ProjectID, cfg.Database.Instance, cfg.Database.Database)
	if err != nil {
		return fmt.Errorf("failed to create database client: %w", err)
	}
	defer dbClient.Close()
	log.Printf("Connected to database: %s/%s/%s",
		cfg.Database.ProjectID, cfg.Database.Instance, cfg.Database.Database)

	// Initialize batch provider.
	batchProvider, err := batch.NewProvider(ctx, cfg.BatchProvider)
	if err != nil {
		return fmt.Errorf("failed to create batch provider: %w", err)
	}
	log.Printf("Initialized %s batch provider in region: %s",
		cfg.BatchProvider.Provider, cfg.BatchProvider.Region)

	// Load job configuration from JSON file.
	jobConfigPath := os.Getenv("JOB_CONFIG_PATH")
	if jobConfigPath == "" {
		jobConfigPath = "config/job-config.json"
	}
	jobConfig, err := config.LoadJobConfig(jobConfigPath)
	if err != nil {
		return fmt.Errorf("failed to load job config: %w", err)
	}
	log.Printf("Loaded job config from: %s", jobConfigPath)
	log.Printf("Default resources: CPU=%dm, Memory=%dMiB, MaxRuntime=%ds",
		jobConfig.DefaultResources.CPUMillis,
		jobConfig.DefaultResources.MemoryMiB,
		jobConfig.DefaultResources.MaxRunDurationSeconds)

	// Initialize GCP Batch client.
	gcpBatchClient, err := gcpbatch.NewClient(ctx)
	if err != nil {
		return fmt.Errorf("failed to create GCP Batch client: %w", err)
	}
	defer gcpBatchClient.Close()
	log.Println("Initialized GCP Batch client")

	workerID := os.Getenv("WORKER_ID")
	if workerID == "" {
		hostname, err := os.Hostname()
		if err != nil || hostname == "" {
			workerID = "worker-unknown"
		} else {
			workerID = hostname
		}
	}

	leaseTTLSeconds := getEnvAsIntOrDefault("WORKER_LEASE_TTL_SECONDS", 30)
	claimIntervalSeconds := getEnvAsIntOrDefault("WORKER_CLAIM_INTERVAL_SECONDS", 5)
	leaseTTL := time.Duration(leaseTTLSeconds) * time.Second
	claimInterval := time.Duration(claimIntervalSeconds) * time.Second

	workerService := service.NewWorkerService(dbClient, batchProvider, jobConfig, gcpBatchClient, workerID, leaseTTL, claimInterval)
	log.Printf("Worker identity: %s (lease_ttl=%s, claim_interval=%s)", workerID, leaseTTL, claimInterval)

	// Resume polling for active jobs from before restart.
	if err := service.ResumeActiveJobPollers(ctx, workerService, dbClient); err != nil {
		log.Printf("Warning: failed to resume job pollers on startup: %v", err)
	}

	mux := http.NewServeMux()
	path, handler := jennahv1connect.NewDeploymentServiceHandler(workerService)
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

	workerService.StartLeaseReconciler(sigCtx)

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

	// Stop all active job pollers.
	workerService.StopAllPollers()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Printf("Error during server shutdown: %v", err)
	}

	log.Println("Worker stopped")
	return nil
}

func getEnvAsIntOrDefault(name string, fallback int) int {
	v := os.Getenv(name)
	if v == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(v)
	if err != nil || parsed <= 0 {
		return fallback
	}
	return parsed
}
