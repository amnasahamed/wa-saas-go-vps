// Command seed provisions a tenant + admin user + API key for local dev.
//
// Usage:
//
//	cd /Users/amnasahamed/Desktop/niosengine
//	DATABASE_URL="postgres://amnasahamed@/niosengine?host=/tmp&sslmode=disable" \
//	  go run ./cmd/seed \
//	    --tenant=acme \
//	    --name="Acme Coaching" \
//	    --email=admin@acme.test \
//	    --country=91
//
// It prints the plaintext API key to stdout exactly once. Save it —
// the secret portion is not stored anywhere retrievable (only its bcrypt
// hash lives in api_keys.key_hash).
package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "fatal:", err)
		os.Exit(1)
	}
}

func run() error {
	var (
		tenantSlug = flag.String("tenant", "", "URL-safe slug for the tenant (required)")
		tenantName = flag.String("name", "", "Display name for the tenant (required)")
		email      = flag.String("email", "", "Admin email (required)")
		country    = flag.String("country", "91", "Default phone country code (e.g. 91 for IN)")
		timezone   = flag.String("timezone", "Asia/Kolkata", "Tenant timezone")
		locale     = flag.String("locale", "en-IN", "Tenant locale")
	)
	flag.Parse()

	if *tenantSlug == "" || *tenantName == "" || *email == "" {
		flag.Usage()
		return errors.New("missing required flags")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		return errors.New("DATABASE_URL is required")
	}

	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		return fmt.Errorf("connect: %w", err)
	}
	defer pool.Close()

	if err := pool.Ping(ctx); err != nil {
		return fmt.Errorf("ping: %w", err)
	}

	// 1) Tenant
	tenantID := uuid.New()
	_, err = pool.Exec(ctx, `
		INSERT INTO tenants (id, slug, name, plan, status, country_code, timezone, locale)
		VALUES ($1, $2, $3, 'pro', 'active', $4, $5, $6)
		ON CONFLICT (slug) DO UPDATE SET name = EXCLUDED.name
		RETURNING id
	`, tenantID, *tenantSlug, *tenantName, *country, *timezone, *locale)
	if err != nil {
		return fmt.Errorf("insert tenant: %w", err)
	}

	// Re-read to get the actual id (in case of conflict)
	var actualID uuid.UUID
	if err := pool.QueryRow(ctx, `SELECT id FROM tenants WHERE slug = $1`, *tenantSlug).Scan(&actualID); err != nil {
		return fmt.Errorf("reread tenant: %w", err)
	}
	tenantID = actualID

	// 2) WhatsApp session (placeholder; the QR link flow will fill status, phone)
	sessionID := uuid.New()
	_, err = pool.Exec(ctx, `
		INSERT INTO whatsapp_sessions (id, tenant_id, status)
		VALUES ($1, $2, 'pending_qr')
		ON CONFLICT (tenant_id) DO UPDATE SET status = 'pending_qr'
		RETURNING id
	`, sessionID, tenantID)
	if err != nil {
		return fmt.Errorf("insert whatsapp session: %w", err)
	}
	if err := pool.QueryRow(ctx, `SELECT id FROM whatsapp_sessions WHERE tenant_id = $1`, tenantID).Scan(&sessionID); err != nil {
		return fmt.Errorf("reread session: %w", err)
	}

	// 3) Admin user (no password — magic link only, for now)
	userID := uuid.New()
	_, err = pool.Exec(ctx, `
		INSERT INTO users (id, tenant_id, email, password_hash, role, email_verified_at)
		VALUES ($1, $2, $3, NULL, 'admin', NOW())
		ON CONFLICT (email) DO UPDATE SET role = 'admin'
		RETURNING id
	`, userID, tenantID, *email)
	if err != nil {
		return fmt.Errorf("insert user: %w", err)
	}
	if err := pool.QueryRow(ctx, `SELECT id FROM users WHERE email = $1`, *email).Scan(&userID); err != nil {
		return fmt.Errorf("reread user: %w", err)
	}

	// 4) API key: random 32-byte secret, prefix "wa_live_", bcrypt-hashed
	secret, err := randomSecret(32) // 32 bytes → 64 hex chars
	if err != nil {
		return fmt.Errorf("generate secret: %w", err)
	}
	prefix := "wa_live_" + secret[:12]    // first 12 hex chars = public lookup key
	fullKey := prefix + "_" + secret[12:] // remaining 52 hex chars = user-presented secret
	displayedSecret := secret[12:]        // what the user sends in Authorization header
	hashed, err := bcrypt.GenerateFromPassword([]byte(displayedSecret), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("bcrypt: %w", err)
	}

	apiKeyID := uuid.New()
	_, err = pool.Exec(ctx, `
		INSERT INTO api_keys (id, tenant_id, name, key_prefix, key_hash, scope)
		VALUES ($1, $2, 'seed-default', $3, $4, 'bridge')
		ON CONFLICT DO NOTHING
	`, apiKeyID, tenantID, prefix, string(hashed))
	if err != nil {
		return fmt.Errorf("insert api key: %w", err)
	}

	log := slog.Default()
	log.Info("seeded",
		slog.String("tenant_id", tenantID.String()),
		slog.String("tenant_slug", *tenantSlug),
		slog.String("whatsapp_session_id", sessionID.String()),
		slog.String("admin_email", *email),
		slog.String("api_key", fullKey),
	)

	fmt.Println("--- seed complete ---")
	fmt.Println("tenant_id:        ", tenantID)
	fmt.Println("tenant_slug:      ", *tenantSlug)
	fmt.Println("whatsapp_session: ", sessionID)
	fmt.Println("admin_email:      ", *email)
	fmt.Println("api_key:          ", fullKey)
	fmt.Println("--- save the api_key, it is shown only once ---")
	return nil
}

func randomSecret(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

var _ = strings.TrimSpace
var _ = errors.New
var _ = pgx.ErrNoRows
