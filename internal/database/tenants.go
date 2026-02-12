package database

import (
	"context"
	"fmt"

	"cloud.google.com/go/spanner"
	"google.golang.org/api/iterator"
)

// InsertTenant creates a new tenant
func (c *Client) InsertTenant(ctx context.Context, tenantID, userEmail, oauthProvider, oauthUserId string) error {
	_, err := c.client.Apply(ctx, []*spanner.Mutation{
		spanner.Insert("Tenants",
			[]string{"TenantId", "UserEmail", "OAuthProvider", "OAuthUserId", "CreatedAt", "UpdatedAt"},
			[]interface{}{tenantID, userEmail, oauthProvider, oauthUserId, spanner.CommitTimestamp, spanner.CommitTimestamp},
		),
	})
	if err != nil {
		return fmt.Errorf("failed to insert tenant: %w", err)
	}
	return nil
}

// GetTenant retrieves a tenant by ID
func (c *Client) GetTenant(ctx context.Context, tenantID string) (*Tenant, error) {
	row, err := c.client.Single().ReadRow(ctx, "Tenants",
		spanner.Key{tenantID},
		[]string{"TenantId", "UserEmail", "OAuthProvider", "OAuthUserId", "CreatedAt", "UpdatedAt"},
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get tenant: %w", err)
	}

	var tenant Tenant
	if err := row.ToStruct(&tenant); err != nil {
		return nil, fmt.Errorf("failed to parse tenant: %w", err)
	}

	return &tenant, nil
}

// ListTenants returns all tenants
func (c *Client) ListTenants(ctx context.Context) ([]*Tenant, error) {
	stmt := spanner.Statement{
		SQL: `SELECT TenantId, UserEmail, OAuthProvider, OAuthUserId, CreatedAt, UpdatedAt FROM Tenants ORDER BY CreatedAt DESC`,
	}

	iter := c.client.Single().Query(ctx, stmt)
	defer iter.Stop()

	var tenants []*Tenant
	for {
		row, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("failed to iterate tenants: %w", err)
		}

		var tenant Tenant
		if err := row.ToStruct(&tenant); err != nil {
			return nil, fmt.Errorf("failed to parse tenant: %w", err)
		}
		tenants = append(tenants, &tenant)
	}

	return tenants, nil
}

// GetTenantByOAuth retrieves a tenant by OAuth provider and user ID
func (c *Client) GetTenantByOAuth(ctx context.Context, oauthProvider, oauthUserId string) (*Tenant, error) {
	stmt := spanner.Statement{
		SQL: `SELECT TenantId, UserEmail, OAuthProvider, OAuthUserId, CreatedAt, UpdatedAt 
		      FROM Tenants 
		      WHERE OAuthProvider = @provider AND OAuthUserId = @userId 
		      LIMIT 1`,
		Params: map[string]interface{}{
			"provider": oauthProvider,
			"userId":   oauthUserId,
		},
	}

	iter := c.client.Single().Query(ctx, stmt)
	defer iter.Stop()

	row, err := iter.Next()
	if err == iterator.Done {
		return nil, nil // No tenant found
	}
	if err != nil {
		return nil, fmt.Errorf("failed to query tenant by OAuth: %w", err)
	}

	var tenant Tenant
	if err := row.ToStruct(&tenant); err != nil {
		return nil, fmt.Errorf("failed to parse tenant: %w", err)
	}

	return &tenant, nil
}

// DeleteTenant removes a tenant and all its jobs (CASCADE)
func (c *Client) DeleteTenant(ctx context.Context, tenantID string) error {
	_, err := c.client.Apply(ctx, []*spanner.Mutation{
		spanner.Delete("Tenants", spanner.Key{tenantID}),
	})
	if err != nil {
		return fmt.Errorf("failed to delete tenant: %w", err)
	}
	return nil
}
