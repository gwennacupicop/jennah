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

	workerServer := &WorkerServer{
		dbClient:      dbClient,
		batchProvider: batchProvider,
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

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Printf("Error during server shutdown: %v", err)
	}

	log.Println("Worker stopped")
}
