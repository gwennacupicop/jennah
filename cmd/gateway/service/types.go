package service

import "time"

type Tenant struct {
	TenantId      string
	UserEmail     string
	OAuthProvider string
	OAuthUserId   string
	CreatedAt     time.Time
}

type OAuthUser struct {
	Email    string
	UserId   string
	Provider string
}
