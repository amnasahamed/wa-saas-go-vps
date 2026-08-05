// Package messages handles persistence of inbound and outbound messages,
// idempotent inserts via UNIQUE(idempotency_key), and conversation history
// retrieval for the AI extraction pipeline.
package messages

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
)

// Message is the persisted record. Mirrors the `messages` table.
type Message struct {
	ID                uuid.UUID
	TenantID          uuid.UUID
	WhatsAppSessionID uuid.UUID
	ProviderMessageID string
	Phone             string
	Direction         string // "inbound" | "outbound"
	Sender            string // "customer" | "ai" | "counselor"
	MessageType       string
	Body              string
	MediaS3Key        string
	MediaMIME         string
	IdempotencyKey    string
	Timestamp         time.Time
	CreatedAt         time.Time
}

// Repository persists Message records.
//
// Implementation lives in pgRepository (repository.go). The interface
// stays here so callers depend on the abstraction, not the pgx detail.
type Repository interface {
	// Insert persists a message. Returns ErrDuplicate if idempotency_key exists.
	Insert(ctx context.Context, m *Message) error

	GetByID(ctx context.Context, id uuid.UUID) (*Message, error)

	GetByIdempotencyKey(ctx context.Context, key string) (*Message, error)

	// History returns up to `limit` messages for (tenant, phone), ASC by timestamp.
	// Excludes outbound AI messages older than the most recent inbound, so the
	// AI sees a natural conversation flow.
	History(ctx context.Context, tenantID uuid.UUID, phone string, limit int) ([]*Message, error)

	// ListByTenant paginates the dashboard messages view.
	ListByTenant(ctx context.Context, tenantID uuid.UUID, limit, offset int) ([]*Message, error)
}

// Service is the business-logic layer.
//
// PersistInbound applies the reply-loop guard: if the message is outbound
// and was sent by us (ai), it skips processing entirely so we don't
// re-trigger extraction on our own replies (the bug in the legacy n8n setup).
type Service interface {
	PersistInbound(ctx context.Context, m *Message) (*Message, error)
	PersistInboundWithStatus(ctx context.Context, m *Message) (*PersistInboundResult, error)
	PersistOutbound(ctx context.Context, m *Message) (*Message, error)
	GetHistory(ctx context.Context, tenantID uuid.UUID, phone string, limit int) ([]*Message, error)
}

// ErrDuplicate is returned when the idempotency_key already exists.
// Callers should treat this as a successful no-op (replay-safe).
var ErrDuplicate = errors.New("duplicate message (idempotency_key collision)")

// ErrNotFound is returned when no message matches the query.
var ErrNotFound = errors.New("message not found")
