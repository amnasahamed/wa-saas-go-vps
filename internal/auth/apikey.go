// Package auth provides the API key validator used by the bridge and
// incoming webhooks. JWT + password + magic-link flow live here in
// later phases; for now we just resolve a Bearer token to a tenant_id.
package auth

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"golang.org/x/crypto/bcrypt"

	"github.com/amnasahamed/niosengine/internal/db"
)

// APIKeyValidator resolves an API key string to a tenant_id.
//
// Wire format: "wa_live_<12hex>_<32hex>" where the first 12 hex chars
// after "wa_live_" are the prefix used for the lookup index, and the
// remaining 32 hex chars are the secret that gets bcrypt-compared.
//
// See cmd/seed for how keys are issued.
type APIKeyValidator struct {
	q *db.Queries
}

// NewAPIKeyValidator wires the validator against sqlc-generated queries.
func NewAPIKeyValidator(q *db.Queries) *APIKeyValidator {
	return &APIKeyValidator{q: q}
}

// Validate parses the Authorization header value (or raw key) and returns
// the tenant_id + scope if the key is valid.
//
// On any error, returns a non-nil error and the caller should reject the
// request with 401.
func (v *APIKeyValidator) Validate(ctx context.Context, rawKey string) (tenantID uuid.UUID, scope string, err error) {
	rawKey = strings.TrimSpace(rawKey)
	rawKey = strings.TrimPrefix(rawKey, "Bearer ")
	rawKey = strings.TrimPrefix(rawKey, "bearer ")
	rawKey = strings.TrimSpace(rawKey)
	if rawKey == "" {
		return uuid.Nil, "", ErrInvalidCredentials
	}

	// Parse: "wa_live_<12hex>_<32hex>" → prefix="wa_live_<12hex>", secret="<32hex>"
	parts := strings.SplitN(rawKey, "_", 4)
	if len(parts) < 4 || parts[0] != "wa" || parts[1] != "live" {
		return uuid.Nil, "", ErrInvalidCredentials
	}
	prefix := parts[0] + "_" + parts[1] + "_" + parts[2]
	secret := parts[3]

	row, err := v.q.GetAPIKeyByPrefix(ctx, prefix)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return uuid.Nil, "", ErrInvalidCredentials
		}
		return uuid.Nil, "", fmt.Errorf("lookup api key: %w", err)
	}
	if row.RevokedAt.Valid {
		return uuid.Nil, "", ErrInvalidCredentials
	}

	if err := bcrypt.CompareHashAndPassword([]byte(row.KeyHash), []byte(secret)); err != nil {
		return uuid.Nil, "", ErrInvalidCredentials
	}

	tenantUUID, err := uuidFromPgtype(row.TenantID)
	if err != nil {
		return uuid.Nil, "", fmt.Errorf("parse tenant id: %w", err)
	}

	// Fire-and-forget: update last_used_at. Failure here shouldn't block
	// the request, so we don't propagate errors.
	go func() {
		_ = v.q.TouchAPIKey(context.Background(), row.ID)
	}()

	return tenantUUID, row.Scope, nil
}

// uuidFromPgtype converts a pgtype.UUID to google/uuid.UUID.
func uuidFromPgtype(p pgtype.UUID) (uuid.UUID, error) {
	if !p.Valid {
		return uuid.Nil, errors.New("null uuid")
	}
	return uuid.FromBytes(p.Bytes[:])
}
