package service

import (
	"context"
	"errors"
	"log"
	"net/http"

	"cloud.google.com/go/spanner"
	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
)

func extractOAuthUser(headers http.Header) (*OAuthUser, error) {
	email := headers.Get("X-OAuth-Email")
	oauthUserId := headers.Get("X-OAuth-UserId")
	provider := headers.Get("X-OAuth-Provider")

	if email == "" || oauthUserId == "" || provider == "" {
		return nil, errors.New("missing required OAuth headers")
	}

	return &OAuthUser{
		Email:    email,
		UserId:   oauthUserId,
		Provider: provider,
	}, nil
}

func (s *GatewayService) getOrCreateTenant(oauthUser *OAuthUser) (string, error) {
	ctx := context.Background()

	// Check in-memory cache first (fast path)
	s.mu.RLock()
	tenantId, exists := s.oauthToTenant[oauthUser.UserId]
	s.mu.RUnlock()

	if exists {
		log.Printf("Found existing tenant in cache for user %s: tenantId=%s",
			oauthUser.Email, tenantId)
		return tenantId, nil
	}

	// Not in cache, check database using OAuth credentials
	tenant, err := s.dbClient.GetTenantByOAuth(ctx, oauthUser.Provider, oauthUser.UserId)
	if err != nil {
		log.Printf("Error querying tenant by OAuth: %v", err)
		return "", err
	}

	if tenant != nil {
		// Found existing tenant in database
		s.mu.Lock()
		s.oauthToTenant[oauthUser.UserId] = tenant.TenantId
		s.mu.Unlock()

		log.Printf("Found existing tenant in database for user %s: tenantId=%s",
			oauthUser.Email, tenant.TenantId)
		return tenant.TenantId, nil
	}

	// Tenant doesn't exist, create new one
	s.mu.Lock()
	defer s.mu.Unlock()

	// Double-check cache after acquiring write lock
	if tenantId, exists = s.oauthToTenant[oauthUser.UserId]; exists {
		log.Printf("Found tenant created by another request: tenantId=%s", tenantId)
		return tenantId, nil
	}

	// Create new tenant
	tenantId = uuid.New().String()

	// Insert into database with OAuth info
	err = s.dbClient.InsertTenant(ctx, tenantId, oauthUser.Email, oauthUser.Provider, oauthUser.UserId)
	if err != nil {
		// Check if tenant already exists (race condition)
		if spanner.ErrCode(err) == codes.AlreadyExists {
			log.Printf("Tenant %s already exists in database (created by another instance)", tenantId)
			// Cache it and return
			s.oauthToTenant[oauthUser.UserId] = tenantId
			return tenantId, nil
		}
		log.Printf("Failed to insert tenant into database: %v", err)
		return "", err
	}

	// Cache the OAuth -> Tenant mapping
	s.oauthToTenant[oauthUser.UserId] = tenantId

	log.Printf("Created new tenant for user %s (provider: %s): tenantId=%s (persisted to database)",
		oauthUser.Email, oauthUser.Provider, tenantId)

	return tenantId, nil
}
