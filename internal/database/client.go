package database

import (
	"context"
	"fmt"

	"cloud.google.com/go/spanner"
)

// Client wraps the Cloud Spanner client
type Client struct {
	client *spanner.Client
}

// NewClient creates a new database client
// project: GCP project ID (e.g., "labs-169405")
// instance: Spanner instance ID (e.g., "alphaus-dev")
// database: Database name (e.g., "main")
func NewClient(ctx context.Context, project, instance, database string) (*Client, error) {
	dbPath := fmt.Sprintf("projects/%s/instances/%s/databases/%s", project, instance, database)

	client, err := spanner.NewClient(ctx, dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to create spanner client: %w", err)
	}

	return &Client{client: client}, nil
}

// Close closes the database client
func (c *Client) Close() {
	c.client.Close()
}
