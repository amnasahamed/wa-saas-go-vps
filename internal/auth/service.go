package auth

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"golang.org/x/crypto/bcrypt"

	"github.com/amnasahamed/niosengine/internal/db"
)

// PGService implements Service backed by sqlc-generated queries.
type PGService struct {
	q         *db.Queries
	jwtSecret []byte
	log       *slog.Logger
}

// NewPGService constructs the auth service. jwtSecret must be at least 32 bytes.
func NewPGService(q *db.Queries, jwtSecret []byte, log *slog.Logger) *PGService {
	return &PGService{q: q, jwtSecret: jwtSecret, log: log}
}

// Signup creates a tenant + admin user + email-verify token.
func (s *PGService) Signup(ctx context.Context, in SignupInput) (*SignupResult, error) {
	in.Email = strings.ToLower(strings.TrimSpace(in.Email))
	in.TenantName = strings.TrimSpace(in.TenantName)
	if in.Email == "" || in.TenantName == "" {
		return nil, fmt.Errorf("email and tenant_name are required")
	}
	if len(in.Password) < 8 {
		return nil, fmt.Errorf("password must be at least 8 characters")
	}

	if _, err := s.q.GetUserByEmail(ctx, in.Email); err == nil {
		return nil, ErrEmailTaken
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("lookup user: %w", err)
	}

	tenantID := uuid.New()
	slug := strings.ToLower(strings.ReplaceAll(in.TenantName, " ", "-"))
	if in.TenantSlug != "" {
		slug = in.TenantSlug
	}

	if _, err := s.q.CreateTenant(ctx, db.CreateTenantParams{
		ID:          db.PgtypeUUID(tenantID),
		Slug:        slug,
		Name:        in.TenantName,
		Plan:        "free",
		Status:      "active",
		CountryCode: "91",
		Timezone:    "Asia/Kolkata",
		Locale:      "en-IN",
	}); err != nil {
		return nil, fmt.Errorf("create tenant: %w", err)
	}

	pwHash, err := bcrypt.GenerateFromPassword([]byte(in.Password), 12)
	if err != nil {
		return nil, fmt.Errorf("hash password: %w", err)
	}

	userID := uuid.New()
	if _, err := s.q.CreateUser(ctx, db.CreateUserParams{
		ID:           db.PgtypeUUID(userID),
		TenantID:     db.PgtypeUUID(tenantID),
		Email:        in.Email,
		PasswordHash: db.StringToText(string(pwHash)),
		Role:         "admin",
	}); err != nil {
		return nil, fmt.Errorf("create user: %w", err)
	}

	token, err := randomToken(32)
	if err != nil {
		return nil, err
	}
	if err := s.q.InsertEmailVerifyToken(ctx, db.InsertEmailVerifyTokenParams{
		Token:     token,
		UserID:    db.PgtypeUUID(userID),
		ExpiresAt: db.TimeToTimestamptz(time.Now().UTC().Add(24 * time.Hour)),
	}); err != nil {
		return nil, fmt.Errorf("insert verify token: %w", err)
	}

	return &SignupResult{
		UserID:      userID.String(),
		TenantID:    tenantID.String(),
		VerifyToken: token,
		TenantSlug:  slug,
	}, nil
}

// VerifyEmail consumes a token and marks the user as verified.
func (s *PGService) VerifyEmail(ctx context.Context, token string) error {
	userID, err := s.q.ConsumeEmailVerifyToken(ctx, token)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrTokenInvalid
		}
		return fmt.Errorf("consume token: %w", err)
	}
	if err := s.q.VerifyUserEmail(ctx, userID); err != nil {
		return fmt.Errorf("verify user: %w", err)
	}
	return nil
}

// Login verifies credentials and returns a signed JWT.
func (s *PGService) Login(ctx context.Context, email, password string) (string, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	row, err := s.q.GetUserByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", ErrInvalidCredentials
		}
		return "", err
	}
	if !row.PasswordHash.Valid {
		return "", ErrInvalidCredentials
	}
	if err := bcrypt.CompareHashAndPassword([]byte(row.PasswordHash.String), []byte(password)); err != nil {
		return "", ErrInvalidCredentials
	}
	if !row.EmailVerifiedAt.Valid {
		return "", ErrEmailNotVerified
	}
	tenantID, err := db.UUIDFromPgtype(row.TenantID)
	if err != nil {
		return "", err
	}
	userID, err := db.UUIDFromPgtype(row.ID)
	if err != nil {
		return "", err
	}
	p := &Principal{
		UserID:   userID.String(),
		TenantID: tenantID.String(),
		Email:    row.Email,
		Role:     row.Role,
	}
	return MintToken(p, s.jwtSecret)
}

// ValidateToken parses + verifies a JWT.
func (s *PGService) ValidateToken(ctx context.Context, token string) (*Principal, error) {
	return ParseToken(token, s.jwtSecret)
}

// MintAPIKey creates a new bridge/webhook/dashboard API key.
func (s *PGService) MintAPIKey(ctx context.Context, tenantID, name, scope string) (*APIKey, error) {
	tid, err := uuid.Parse(tenantID)
	if err != nil {
		return nil, err
	}
	prefixBytes := make([]byte, 6)
	if _, err := rand.Read(prefixBytes); err != nil {
		return nil, err
	}
	secretBytes := make([]byte, 26)
	if _, err := rand.Read(secretBytes); err != nil {
		return nil, err
	}
	prefix := "wa_live_" + hex.EncodeToString(prefixBytes)
	secret := hex.EncodeToString(secretBytes)
	hash, err := bcrypt.GenerateFromPassword([]byte(secret), 10)
	if err != nil {
		return nil, err
	}

	id := uuid.New()
	row, err := s.q.CreateAPIKey(ctx, db.CreateAPIKeyParams{
		ID:        db.PgtypeUUID(id),
		TenantID:  db.PgtypeUUID(tid),
		Name:      name,
		KeyPrefix: prefix,
		KeyHash:   string(hash),
		Scope:     scope,
	})
	if err != nil {
		return nil, err
	}
	return &APIKey{
		ID:        dbUUIDString(row.ID),
		TenantID:  tid.String(),
		Name:      row.Name,
		Prefix:    row.KeyPrefix,
		Scope:     row.Scope,
		CreatedAt: db.TimeFromTimestamptz(row.CreatedAt),
	}, nil
}

// ValidateAPIKey parses a full key string (prefix + secret) and returns the tenant.
func (s *PGService) ValidateAPIKey(ctx context.Context, key string) (tenantID string, scope string, err error) {
	v := NewAPIKeyValidator(s.q)
	tid, sc, err := v.Validate(ctx, "Bearer "+key)
	if err != nil {
		return "", "", err
	}
	return tid.String(), sc, nil
}

// RevokeAPIKey soft-deletes a key.
func (s *PGService) RevokeAPIKey(ctx context.Context, tenantID, keyID string) error {
	tid, err := uuid.Parse(tenantID)
	if err != nil {
		return err
	}
	kid, err := uuid.Parse(keyID)
	if err != nil {
		return err
	}
	return s.q.RevokeAPIKey(ctx, db.RevokeAPIKeyParams{
		ID:       db.PgtypeUUID(kid),
		TenantID: db.PgtypeUUID(tid),
	})
}

// --- helpers ---

func randomToken(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// dbUUIDString renders a pgtype.UUID as a string.
func dbUUIDString(p pgtype.UUID) string {
	u, err := db.UUIDFromPgtype(p)
	if err != nil {
		return ""
	}
	return u.String()
}
