// Package billing wraps Stripe: Checkout, Customer Portal, webhooks.
//
// Plan → Stripe Price ID mapping lives in this package.
// Webhook events we handle:
//   - checkout.session.completed
//   - customer.subscription.created
//   - customer.subscription.updated
//   - customer.subscription.deleted
//   - invoice.payment_failed
package billing

import (
	"context"
	"errors"
	"time"
)

// Plan represents a SaaS plan tier.
type Plan struct {
	ID              string // "free" | "pro" | "agency"
	StripePriceID   string // populated from env
	Name            string
	PriceCents      int
	Currency        string
	Interval        string // "month" | "year"
	MaxSessions     int
	MaxDestinations int
	MonthlyMessages int
}

// Subscription is the local mirror of a Stripe subscription.
type Subscription struct {
	ID                   string
	TenantID             string
	StripeSubscriptionID string
	Plan                 string
	Status               string // "active" | "trialing" | "past_due" | "canceled" | "unpaid"
	CurrentPeriodEnd     time.Time
	CreatedAt            time.Time
	UpdatedAt            time.Time
}

// Service is the high-level billing interface.
type Service interface {
	// CreateCheckoutSession returns a Stripe Checkout Session URL for the tenant.
	CreateCheckoutSession(ctx context.Context, tenantID, planID, successURL, cancelURL string) (string, error)

	// CreatePortalSession returns a Stripe Customer Portal URL for self-service.
	CreatePortalSession(ctx context.Context, tenantID, returnURL string) (string, error)

	// HandleWebhook processes a Stripe webhook payload; idempotent via event ID.
	HandleWebhook(ctx context.Context, payload []byte, signature string) error

	// GetSubscription returns the active subscription for a tenant.
	GetSubscription(ctx context.Context, tenantID string) (*Subscription, error)

	// Plans returns the catalog.
	Plans() []Plan
}

// Repository persists subscription state.
type Repository interface {
	UpsertSubscription(ctx context.Context, s *Subscription) error
	GetSubscriptionByTenant(ctx context.Context, tenantID string) (*Subscription, error)
	GetSubscriptionByStripeID(ctx context.Context, stripeSubID string) (*Subscription, error)
	MarkEventProcessed(ctx context.Context, eventID string) error
	WasEventProcessed(ctx context.Context, eventID string) (bool, error)
}

// ErrNoActiveSubscription is returned when a tenant has no active plan.
var ErrNoActiveSubscription = errors.New("no active subscription")

// ErrWebhookInvalid is returned when signature verification fails.
var ErrWebhookInvalid = errors.New("invalid webhook signature")
