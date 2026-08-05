package messages

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/google/uuid"
)

// pgService is the pgx-backed implementation of Service.
type pgService struct {
	repo Repository
	log  *slog.Logger
}

// NewPGService wires the messages Service.
func NewPGService(repo Repository, log *slog.Logger) Service {
	if log == nil {
		log = slog.Default()
	}
	return &pgService{repo: repo, log: log}
}

// PersistInbound persists an inbound message with the reply-loop guard.
// If the message is from ourselves (sender=ai, direction=outbound), the
// call is a no-op — that's the legacy n8n reply-loop bug we're closing.
//
// On idempotency_key collision (replay), returns the existing message and
// nil error. The caller can treat this as a successful no-op.
func (s *pgService) PersistInbound(ctx context.Context, m *Message) (*Message, error) {
	if m == nil {
		return nil, errors.New("message is nil")
	}

	// Reply-loop guard: never re-process an outbound message we sent.
	// The bridge's sender classifier already tags these as "ai".
	if m.Direction == "outbound" && m.Sender == "ai" {
		s.log.Debug("skipping outbound AI message (reply-loop guard)",
			slog.String("tenant_id", m.TenantID.String()),
			slog.String("phone", m.Phone),
		)
		return m, nil
	}

	if m.TenantID == uuid.Nil {
		return nil, errors.New("tenant_id is required")
	}
	if m.WhatsAppSessionID == uuid.Nil {
		return nil, errors.New("whatsapp_session_id is required")
	}
	if m.IdempotencyKey == "" {
		return nil, errors.New("idempotency_key is required")
	}

	if err := s.repo.Insert(ctx, m); err != nil {
		if errors.Is(err, ErrDuplicate) {
			s.log.Info("replay detected, returning existing message",
				slog.String("idempotency_key", m.IdempotencyKey),
			)
			existing, getErr := s.repo.GetByIdempotencyKey(ctx, m.IdempotencyKey)
			if getErr != nil {
				return nil, fmt.Errorf("get by idempotency key after duplicate: %w", getErr)
			}
			return existing, nil
		}
		return nil, err
	}

	saved, err := s.repo.GetByIdempotencyKey(ctx, m.IdempotencyKey)
	if err != nil {
		return nil, fmt.Errorf("get by idempotency key after insert: %w", err)
	}
	return saved, nil
}

// PersistOutbound persists an outbound message (AI or counselor). Always
// inserts, but with the same idempotency_key UNIQUE so retries are safe.
func (s *pgService) PersistOutbound(ctx context.Context, m *Message) (*Message, error) {
	if m == nil {
		return nil, errors.New("message is nil")
	}
	if m.Direction != "outbound" {
		return nil, errors.New("direction must be 'outbound' for PersistOutbound")
	}
	if m.IdempotencyKey == "" {
		return nil, errors.New("idempotency_key is required")
	}

	if err := s.repo.Insert(ctx, m); err != nil {
		if errors.Is(err, ErrDuplicate) {
			existing, getErr := s.repo.GetByIdempotencyKey(ctx, m.IdempotencyKey)
			if getErr != nil {
				return nil, fmt.Errorf("get by idempotency key after duplicate: %w", getErr)
			}
			return existing, nil
		}
		return nil, err
	}

	saved, err := s.repo.GetByIdempotencyKey(ctx, m.IdempotencyKey)
	if err != nil {
		return nil, fmt.Errorf("get by idempotency key after insert: %w", err)
	}
	return saved, nil
}

// PersistInboundResult is what PersistInboundWithStatus returns: tells the
// caller whether the message was a fresh insert, a replay, or skipped by
// the reply-loop guard.
type PersistInboundResult struct {
	Message   *Message
	Duplicate bool // true if this was a replay (idempotency_key collision)
	Skipped   bool // true if reply-loop guard rejected the message
}

// PersistInboundWithStatus is like PersistInbound but returns enough
// metadata for the HTTP handler to set a useful response (e.g. the
// `duplicate` flag in the JSON body).
func (s *pgService) PersistInboundWithStatus(ctx context.Context, m *Message) (*PersistInboundResult, error) {
	if m == nil {
		return nil, errors.New("message is nil")
	}

	// Reply-loop guard: never re-process an outbound message we sent.
	if m.Direction == "outbound" && m.Sender == "ai" {
		s.log.Debug("skipping outbound AI message (reply-loop guard)",
			slog.String("tenant_id", m.TenantID.String()),
			slog.String("phone", m.Phone),
		)
		return &PersistInboundResult{Message: m, Skipped: true}, nil
	}

	if m.IdempotencyKey == "" {
		return nil, errors.New("idempotency_key is required")
	}

	if err := s.repo.Insert(ctx, m); err != nil {
		if errors.Is(err, ErrDuplicate) {
			existing, getErr := s.repo.GetByIdempotencyKey(ctx, m.IdempotencyKey)
			if getErr != nil {
				return nil, fmt.Errorf("get by idempotency key after duplicate: %w", getErr)
			}
			return &PersistInboundResult{Message: existing, Duplicate: true}, nil
		}
		return nil, err
	}

	saved, err := s.repo.GetByIdempotencyKey(ctx, m.IdempotencyKey)
	if err != nil {
		return nil, fmt.Errorf("get by idempotency key after insert: %w", err)
	}
	return &PersistInboundResult{Message: saved}, nil
}

// GetHistory returns the last `limit` messages for (tenant, phone), ASC by
// timestamp. Used by the extraction pipeline.
func (s *pgService) GetHistory(ctx context.Context, tenantID uuid.UUID, phone string, limit int) ([]*Message, error) {
	if limit <= 0 {
		limit = 20
	}
	return s.repo.History(ctx, tenantID, phone, limit)
}
