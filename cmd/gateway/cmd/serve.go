package cmd

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/alphauslabs/jennah/cmd/gateway/service"
	jennahv1connect "github.com/alphauslabs/jennah/gen/proto/jennahv1connect"
	"github.com/alphauslabs/jennah/internal/hashing"
)

var (
	port      string
	workerIPs string
)

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start the gateway server",
	Long:  `Start the gateway server to handle requests and route them to workers.`,
	RunE:  runServe,
}

func init() {
	serveCmd.Flags().StringVar(&port, "port", "8080", "Port to listen on")
	serveCmd.Flags().StringVar(&workerIPs, "worker-ips", "10.146.0.26", "Comma-separated list of worker IPs")
}

func runServe(cmd *cobra.Command, args []string) error {
	log.Printf("Starting gateway")

	workers := strings.Split(workerIPs, ",")
	for i, ip := range workers {
		workers[i] = strings.TrimSpace(ip)
	}
	log.Printf("Worker IPs: %v", workers)

	router := hashing.NewRouter(workers)
	log.Printf("Initialized consistent hashing router with workers: %v", workers)

	workerClients := make(map[string]jennahv1connect.DeploymentServiceClient)
	httpClient := &http.Client{
		Timeout: 30 * time.Second,
	}
	for _, workerIP := range workers {
		workerURL := fmt.Sprintf("http://%s:8081", workerIP)
		workerClients[workerIP] = jennahv1connect.NewDeploymentServiceClient(httpClient, workerURL)
		log.Printf("Created client for worker at %s", workerURL)
	}

	gatewayService := service.NewGatewayService(router, workerClients)

	mux := http.NewServeMux()
	path, handler := jennahv1connect.NewDeploymentServiceHandler(gatewayService)
	mux.Handle(path, handler)
	log.Printf("Registered DeploymentService handler at path: %s", path)

	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})
	log.Println("Health check endpoint: /health")

	addr := fmt.Sprintf("0.0.0.0:%s", port)
	server := &http.Server{
		Addr:         addr,
		Handler:      mux,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	sigCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go func() {
		log.Printf("Gateway listening on %s", addr)
		log.Println("Available endpoints:")
		log.Printf("  • POST %sGetCurrentTenant", path)
		log.Printf("  • POST %sSubmitJob", path)
		log.Printf("  • POST %sListJobs", path)
		log.Printf("  • GET  /health")
		log.Println("OAuth-enabled - tenantId auto-generated from auth headers")
		log.Println("")

		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Failed to start server: %v", err)
		}
	}()

	<-sigCtx.Done()
	log.Println("Shutdown signal received, shutting down...")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Printf("Error during server shutdown: %v", err)
	}

	log.Println("Gateway stopped")
	return nil
}
