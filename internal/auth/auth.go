// Package auth implements JWT-based authentication, password hashing,
// API key generation, and email-verify magic links.
//
// Two auth surfaces:
//   - Dashboard: email + password (bcrypt) → JWT in HTTP-only cookie
//   - Bridge: per-tenant API key (Bearer) — minted post-signup
//
// All tenant-scoped queries MUST carry the tenant_id from the verified
// principal; never trust a tenant_id from the request body.
package auth

import (
	"context"
	"errors"
	"time"
)

// Principal is the authenticated identity attached to a request context.
type Principal struct {
	UserID   string
	TenantID string
	Email    string
	Role     string // "admin" | "member"
	Scopes   []string
}

// ContextKey is unexported to prevent collisions.
type contextKey struct{}

// WithPrincipal attaches a Principal to ctx.
func WithPrincipal(ctx context.Context, p *Principal) context.Context {
	return context.WithValue(ctx, contextKey{}, p)
}

// FromContext returns the Principal from ctx, or nil if unauthenticated.
func FromContext(ctx context.Context) *Principal {
	p, _ := ctx.Value(contextKey{}).(*Principal)
	return p
}

// Service is the high-level auth interface used by the API layer.
type Service interface {
	// Signup creates a tenant + admin user + email-verify token.
	// Returns the verify token (caller emails it via SES).
	Signup(ctx context.Context, in SignupInput) (*SignupResult, error)

	// VerifyEmail consumes a verify token; sets users.email_verified_at.
	VerifyEmail(ctx context.Context, token string) error

	// Login verifies credentials and returns a JWT.
	Login(ctx context.Context, email, password string) (string, error)

	// ValidateToken parses a JWT and returns the Principal.
	ValidateToken(ctx context.Context, token string) (*Principal, error)

	// MintAPIKey issues a new per-tenant API key for bridge or webhook use.
	// Returns the plaintext secret exactly once.
	MintAPIKey(ctx context.Context, tenantID, name, scope string) (*APIKey, error)

	// ValidateAPIKey looks up a key by prefix + secret and returns the tenant.
	ValidateAPIKey(ctx context.Context, key string) (tenantID string, scope string, err error)

	// RevokeAPIKey soft-deletes a key.
	RevokeAPIKey(ctx context.Context, tenantID, keyID string) error
}

// SignupInput is the request payload for Service.Signup.
type SignupInput struct {
	Email      string
	Password   string
	TenantName string
	TenantSlug string // optional; auto-generated from name if empty
}

// SignupResult is what Service.Signup returns.
type SignupResult struct {
	UserID      string
	TenantID    string
	VerifyToken string
	TenantSlug  string
}

// APIKey is the persisted API key record (secret never stored plaintext).
type APIKey struct {
	ID         string
	TenantID   string
	Name       string
	Prefix     string // e.g. "wa_live_abc123"
	Scope      string // "bridge" | "webhook" | "dashboard"
	CreatedAt  time.Time
	LastUsedAt *time.Time
}

// ErrInvalidCredentials is returned for bad email/password or unknown API key.
var ErrInvalidCredentials = errors.New("invalid credentials")

// ErrEmailTaken is returned when a user already exists for the email.
var ErrEmailTaken = errors.New("email already in use")

// ErrTokenInvalid is returned for malformed or expired tokens.
var ErrTokenInvalid = errors.New("invalid or expired token")

// ErrEmailNotVerified is returned when login is attempted before verification.
var ErrEmailNotVerified = errors.New("email not verified")
