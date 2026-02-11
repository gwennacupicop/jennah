package service

import (
	"errors"
	"log"
	"net/http"
	"time"

	"github.com/google/uuid"
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
	tenantId, exists := s.oauthToTenant[oauthUser.UserId]
	if exists {
		log.Printf("Found existing tenant for user %s: tenantId=%s", oauthUser.Email, tenantId)
		return tenantId, nil
	}

	tenantId = uuid.New().String()
	tenant := &Tenant{
		TenantId:      tenantId,
		UserEmail:     oauthUser.Email,
		OAuthProvider: oauthUser.Provider,
		OAuthUserId:   oauthUser.UserId,
		CreatedAt:     time.Now(),
	}
	s.tenants[tenantId] = tenant
	s.oauthToTenant[oauthUser.UserId] = tenantId

	// TODO: Persist to database/implement database client
	log.Printf("Created new tenant for user %s: tenantId=%s", oauthUser.Email, tenantId)

	return tenantId, nil
}
