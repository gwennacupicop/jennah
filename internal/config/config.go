package config

import (
	"fmt"
	"os"
	"strconv"

	"github.com/alphauslabs/jennah/internal/batch"
)

// Config represents the complete worker configuration.
type Config struct {
	// ServerPort is the port the worker listens on.
	ServerPort string

	// BatchProvider configuration for cloud batch service.
	BatchProvider batch.ProviderConfig

	// Database configuration.
	Database DatabaseConfig
}

// DatabaseConfig contains database connection configuration.
type DatabaseConfig struct {
	// Provider is the database provider ("spanner", "dynamodb", "cosmosdb", "postgres").
	Provider string

	// ProjectID is used by GCP Spanner.
	ProjectID string

	// Instance is the database instance name (Spanner-specific).
	Instance string

	// Database is the database name.
	Database string

	// ProviderOptions contains provider-specific configuration.
	ProviderOptions map[string]string
}

// LoadFromEnv loads configuration from environment variables.
// This follows the 12-factor app methodology for configuration.
func LoadFromEnv() (*Config, error) {
	config := &Config{
		ServerPort: getEnvOrDefault("WORKER_PORT", "8081"),
		BatchProvider: batch.ProviderConfig{
			Provider:        getEnvOrDefault("BATCH_PROVIDER", "gcp"),
			Region:          os.Getenv("BATCH_REGION"),
			ProjectID:       os.Getenv("BATCH_PROJECT_ID"),
			ProviderOptions: make(map[string]string),
		},
		Database: DatabaseConfig{
			Provider:        getEnvOrDefault("DB_PROVIDER", "spanner"),
			ProjectID:       os.Getenv("DB_PROJECT_ID"),
			Instance:        os.Getenv("DB_INSTANCE"),
			Database:        os.Getenv("DB_DATABASE"),
			ProviderOptions: make(map[string]string),
		},
	}

	// Load provider-specific batch options
	if awsAccountID := os.Getenv("AWS_ACCOUNT_ID"); awsAccountID != "" {
		config.BatchProvider.ProviderOptions["account_id"] = awsAccountID
	}
	if awsJobQueue := os.Getenv("AWS_JOB_QUEUE"); awsJobQueue != "" {
		config.BatchProvider.ProviderOptions["job_queue"] = awsJobQueue
	}
	if azureSubscriptionID := os.Getenv("AZURE_SUBSCRIPTION_ID"); azureSubscriptionID != "" {
		config.BatchProvider.ProviderOptions["subscription_id"] = azureSubscriptionID
	}
	if azureResourceGroup := os.Getenv("AZURE_RESOURCE_GROUP"); azureResourceGroup != "" {
		config.BatchProvider.ProviderOptions["resource_group"] = azureResourceGroup
	}

	// Load provider-specific database options
	if dbEndpoint := os.Getenv("DB_ENDPOINT"); dbEndpoint != "" {
		config.Database.ProviderOptions["endpoint"] = dbEndpoint
	}
	if dbRegion := os.Getenv("DB_REGION"); dbRegion != "" {
		config.Database.ProviderOptions["region"] = dbRegion
	}

	// Validate configuration
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	return config, nil
}

// Validate checks if the configuration is valid for the selected providers.
func (c *Config) Validate() error {
	// Validate batch provider configuration
	switch c.BatchProvider.Provider {
	case "gcp":
		if c.BatchProvider.ProjectID == "" {
			return fmt.Errorf("BATCH_PROJECT_ID is required for GCP batch provider")
		}
		if c.BatchProvider.Region == "" {
			return fmt.Errorf("BATCH_REGION is required for GCP batch provider")
		}
	case "aws":
		if c.BatchProvider.Region == "" {
			return fmt.Errorf("BATCH_REGION is required for AWS batch provider")
		}
		if c.BatchProvider.ProviderOptions["account_id"] == "" {
			return fmt.Errorf("AWS_ACCOUNT_ID is required for AWS batch provider")
		}
	case "azure":
		if c.BatchProvider.Region == "" {
			return fmt.Errorf("BATCH_REGION is required for Azure batch provider")
		}
		if c.BatchProvider.ProviderOptions["subscription_id"] == "" {
			return fmt.Errorf("AZURE_SUBSCRIPTION_ID is required for Azure batch provider")
		}
	default:
		return fmt.Errorf("unsupported batch provider: %s", c.BatchProvider.Provider)
	}

	// Validate database configuration
	switch c.Database.Provider {
	case "spanner":
		if c.Database.ProjectID == "" {
			return fmt.Errorf("DB_PROJECT_ID is required for Spanner")
		}
		if c.Database.Instance == "" {
			return fmt.Errorf("DB_INSTANCE is required for Spanner")
		}
		if c.Database.Database == "" {
			return fmt.Errorf("DB_DATABASE is required for Spanner")
		}
	case "dynamodb":
		if c.Database.ProviderOptions["region"] == "" {
			return fmt.Errorf("DB_REGION is required for DynamoDB")
		}
	case "postgres":
		if c.Database.ProviderOptions["endpoint"] == "" {
			return fmt.Errorf("DB_ENDPOINT is required for PostgreSQL")
		}
	default:
		return fmt.Errorf("unsupported database provider: %s", c.Database.Provider)
	}

	return nil
}

// getEnvOrDefault returns the environment variable value or a default if not set.
func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// getEnvAsInt returns the environment variable as an integer or a default if not set.
func getEnvAsInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if intValue, err := strconv.Atoi(value); err == nil {
			return intValue
		}
	}
	return defaultValue
}

// GetMigrationGuide returns a migration guide from old hardcoded config to new env vars.
func GetMigrationGuide() string {
	return `
Migration Guide: Hardcoded Config to Environment Variables
============================================================

Old (hardcoded in main.go):
  projectId       = "labs-169405"
  region          = "asia-northeast1"
  spannerInstance = "alphaus-dev"
  spannerDb       = "main"
  workerPort      = "8081"

New (environment variables):
  BATCH_PROVIDER=gcp
  BATCH_PROJECT_ID=labs-169405
  BATCH_REGION=asia-northeast1
  DB_PROVIDER=spanner
  DB_PROJECT_ID=labs-169405
  DB_INSTANCE=alphaus-dev
  DB_DATABASE=main
  WORKER_PORT=8081

Example for AWS:
  BATCH_PROVIDER=aws
  BATCH_REGION=us-east-1
  AWS_ACCOUNT_ID=123456789012
  AWS_JOB_QUEUE=jennah-job-queue
  DB_PROVIDER=dynamodb
  DB_REGION=us-east-1

Example for Azure:
  BATCH_PROVIDER=azure
  BATCH_REGION=eastus
  AZURE_SUBSCRIPTION_ID=xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx
  AZURE_RESOURCE_GROUP=jennah-resources
  DB_PROVIDER=cosmosdb
  DB_ENDPOINT=https://xxx.documents.azure.com:443/
`
}
