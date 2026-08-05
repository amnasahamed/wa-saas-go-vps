// Package tenants manages tenant records and per-tenant settings.
//
// A tenant represents one paying customer (e.g. Skillvard NIOS).
// Each tenant owns:
//   - one or more users (auth)
//   - one or more WhatsApp sessions
//   - one or more destinations (Sheets/Airtable/Notion)
//   - a per-tenant prompt template override
//   - per-tenant OpenAI key (BYO) or platform default
//   - per-tenant locale/country/timezone for phone normalization + AI prompts
package tenants

import (
	"context"
	"errors"
	"time"
)

// Tenant is the canonical tenant record.
type Tenant struct {
	ID               string
	Slug             string
	Name             string
	Plan             string // "free" | "pro" | "agency"
	Status           string // "active" | "suspended" | "canceled"
	StripeCustomerID string
	CreatedAt        time.Time
	UpdatedAt        time.Time

	// Settings (jsonb in DB)
	CountryCode    string // default phone country prefix, e.g. "91" for IN
	Timezone       string // IANA, e.g. "Asia/Kolkata"
	Locale         string // default "en-IN"
	PromptOverride string // optional nurture prompt override

	// BYO AI key (encrypted at rest)
	OpenAIKeyCiphertext []byte // null if using platform key

	// Plan limits (computed from plan)
	MaxWhatsAppSessions int
	MaxDestinations     int
	MonthlyMessagesCap  int
}

// Repository persists Tenant records.
type Repository interface {
	Create(ctx context.Context, t *Tenant) error
	GetByID(ctx context.Context, id string) (*Tenant, error)
	GetBySlug(ctx context.Context, slug string) (*Tenant, error)
	GetByStripeCustomer(ctx context.Context, customerID string) (*Tenant, error)
	Update(ctx context.Context, t *Tenant) error
	Suspend(ctx context.Context, id, reason string) error
	Delete(ctx context.Context, id string) error
}

// Service is the tenant business-logic layer.
type Service interface {
	Create(ctx context.Context, in CreateInput) (*Tenant, error)
	Get(ctx context.Context, id string) (*Tenant, error)
	UpdateSettings(ctx context.Context, id string, in SettingsInput) (*Tenant, error)
	UpdateOpenAIKey(ctx context.Context, id string, plaintextKey string) error
	Suspend(ctx context.Context, id, reason string) error
	Cancel(ctx context.Context, id string) error
}

// CreateInput is the request payload for Service.Create.
type CreateInput struct {
	Name       string
	Slug       string // optional
	OwnerEmail string // initial admin user
}

// SettingsInput is the partial-update payload for tenant settings.
type SettingsInput struct {
	CountryCode    *string
	Timezone       *string
	Locale         *string
	PromptOverride *string
	Name           *string
}

// ErrTenantNotFound is returned when no tenant matches the query.
var ErrTenantNotFound = errors.New("tenant not found")

// ErrSlugTaken is returned when slug uniqueness fails.
var ErrSlugTaken = errors.New("tenant slug already in use")

// ErrInvalidSlug is returned for malformed slugs.
var ErrInvalidSlug = errors.New("slug must be lowercase letters, digits, and hyphens")

// PlanLimits returns the canonical limits for a plan name.
func PlanLimits(plan string) (maxSessions, maxDestinations, monthlyMsgCap int) {
	switch plan {
	case "free":
		return 1, 1, 200
	case "pro":
		return 1, 3, 5000
	case "agency":
		return 5, 10, 50000
	default:
		return 1, 1, 200
	}
}
