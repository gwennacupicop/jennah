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

	batch "cloud.google.com/go/batch/apiv1"
	"github.com/alphauslabs/jennah/gen/proto/jennahv1connect"
	"github.com/alphauslabs/jennah/internal/database"
)

// Hardcoded config for now - will be moved to env vars or config file in the future
const (
	projectId       = "labs-169405"
	region          = "asia-northeast1"
	spannerInstance = "alphaus-dev"
	spannerDb       = "main"
	workerPort      = "8081"
)

func main() {
	log.Println("Starting worker...")

	ctx := context.Background()

	dbClient, err := database.NewClient(ctx, projectId, spannerInstance, spannerDb)
	if err != nil {
		log.Fatalf("Failed to create database client: %v", err)
	}
	defer dbClient.Close()
	log.Printf("Connected to Spanner: %s/%s/%s", projectId, spannerInstance, spannerDb)

	batchClient, err := batch.NewClient(ctx)
	if err != nil {
		log.Fatalf("Failed to create GCP Batch client: %v", err)
	}
	defer batchClient.Close()
	log.Printf("Connected to GCP Batch API in region: %s", region)

	workerServer := &WorkerServer{
		dbClient:    dbClient,
		batchClient: batchClient,
		projectId:   projectId,
		region:      region,
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

	addr := fmt.Sprintf("0.0.0.0:%s", workerPort)
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
		log.Printf("Worker configured for project: %s, region: %s", projectId, region)
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
