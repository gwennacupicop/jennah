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

	"github.com/alphauslabs/jennah/cmd/gateway/middleware"
	"github.com/alphauslabs/jennah/cmd/gateway/service"
	jennahv1connect "github.com/alphauslabs/jennah/gen/proto/jennahv1connect"
	"github.com/alphauslabs/jennah/internal/database"
	"github.com/alphauslabs/jennah/internal/hashing"
)

var (
	port           string
	workerIPs      string
	dbProjectID    string
	dbInstance     string
	dbDatabase     string
	allowedOrigins string
)

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start the gateway server",
	Long:  `Start the gateway server to handle requests and route them to workers.`,
	RunE:  runServe,
}

func init() {
	serveCmd.Flags().StringVar(&port, "port", "8080", "Port to listen on")
	serveCmd.Flags().StringVar(&workerIPs, "worker-ips", "10.146.0.26,10.146.0.44", "Comma-separated list of worker IPs")
	serveCmd.Flags().StringVar(&dbProjectID, "db-project-id", "labs-169405", "Database project ID (GCP project for Spanner)")
	serveCmd.Flags().StringVar(&dbInstance, "db-instance", "alphaus-dev", "Database instance (Spanner instance name)")
	serveCmd.Flags().StringVar(&dbDatabase, "db-database", "main", "Database name")
	defaultOrigins := os.Getenv("ALLOWED_ORIGINS")
	if defaultOrigins == "" {
		// Include both production and localhost by default
		defaultOrigins = "https://jennah-ui-382915581671.asia-northeast1.run.app,http://localhost:5173"
	}
	serveCmd.Flags().StringVar(&allowedOrigins, "allowed-origins", defaultOrigins, "Comma-separated list of allowed CORS origins")
}

func runServe(cmd *cobra.Command, args []string) error {
	log.Printf("Starting gateway")

	ctx := context.Background()
	dbClient, err := database.NewClient(ctx, dbProjectID, dbInstance, dbDatabase)
	if err != nil {
		return fmt.Errorf("failed to initialize database client: %w", err)
	}
	defer dbClient.Close()
	log.Printf("Connected to database: %s/%s/%s", dbProjectID, dbInstance, dbDatabase)

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

	gatewayService := service.NewGatewayService(router, workerClients, dbClient)

	origins := strings.Split(allowedOrigins, ",")
	for i, origin := range origins {
		origins[i] = strings.TrimSpace(origin)
	}
	log.Printf("CORS allowed origins: %v", origins)
	corsMiddleware := middleware.CORSMiddleware(origins)

	mux := http.NewServeMux()
	path, handler := jennahv1connect.NewDeploymentServiceHandler(gatewayService)

	mux.Handle(path, corsMiddleware(handler))
	log.Printf("Registered DeploymentService handler at path: %s (with CORS)", path)

	mux.Handle("/health", corsMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})))
	log.Println("Health check endpoint: /health (with CORS)")

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
		log.Printf("  • POST %sSubmitJob", path)
		log.Printf("  • POST %sListJobs", path)
		log.Printf("  • POST %sGetCurrentTenant", path)
		log.Printf("  • POST %sCancelJob", path)
		log.Printf("  • POST %sDeleteJob", path)
		log.Printf("  • GET  /health")
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Failed to start server: %v", err)
		}
	}()

	<-sigCtx.Done()
	log.Println("Shutting down gateway gracefully...")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("server shutdown failed: %w", err)
	}

	log.Println("Gateway stopped")
	return nil
}
