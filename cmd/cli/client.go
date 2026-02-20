package main

import (
	"context"
	"crypto/rand"
	"fmt"
	"io"
	"log"
	"os"

	"github.com/alphauslabs/jennah/internal/database"
	"github.com/spf13/cobra"
)

// newDBClient creates a Spanner database client from flags or env vars.
// Looks for --project/JENNAH_PROJECT, --instance/JENNAH_INSTANCE, --database/JENNAH_DATABASE.
func newDBClient(cmd *cobra.Command) (*database.Client, func(), error) {
	project, _ := cmd.Flags().GetString("project")
	instance, _ := cmd.Flags().GetString("instance")
	dbName, _ := cmd.Flags().GetString("database")

	if project == "" {
		project = os.Getenv("JENNAH_PROJECT")
	}
	if instance == "" {
		instance = os.Getenv("JENNAH_INSTANCE")
	}
	if dbName == "" {
		dbName = os.Getenv("JENNAH_DATABASE")
	}

	if project == "" {
		return nil, nil, fmt.Errorf("Spanner project required: use --project or JENNAH_PROJECT env var")
	}
	if instance == "" {
		return nil, nil, fmt.Errorf("Spanner instance required: use --instance or JENNAH_INSTANCE env var")
	}
	if dbName == "" {
		return nil, nil, fmt.Errorf("Spanner database required: use --database or JENNAH_DATABASE env var")
	}

	// Suppress Spanner client SDK startup logs
	log.SetOutput(io.Discard)
	client, err := database.NewClient(context.Background(), project, instance, dbName)
	if err != nil {
		log.SetOutput(os.Stderr)
		return nil, nil, fmt.Errorf("failed to connect to Spanner: %w", err)
	}
	log.SetOutput(os.Stderr)
	return client, client.Close, nil
}

// newJobID generates a random UUID v4.
func newJobID() string {
	b := make([]byte, 16)
	rand.Read(b)
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
}
