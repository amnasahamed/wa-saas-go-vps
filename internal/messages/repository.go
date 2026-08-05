package messages

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/amnasahamed/niosengine/internal/db"
)

// pgRepository is the pgx-backed implementation of Repository.
// Lives in this file (not messages.go) so the interface and the
// concrete type can evolve independently — callers depend on the
// interface, not on pgx.
type pgRepository struct {
	q *db.Queries
}

// Compile-time interface conformance check.
var _ Repository = (*pgRepository)(nil)

// NewPGRepository wraps a sqlc Queries instance.
func NewPGRepository(q *db.Queries) Repository {
	return &pgRepository{q: q}
}

// Insert persists a Message. Generates the ID. Returns ErrDuplicate if
// (tenant_id, idempotency_key) already exists — the caller should treat
// this as a successful no-op (replay-safe).
func (r *pgRepository) Insert(ctx context.Context, m *Message) error {
	_, err := r.q.InsertMessage(ctx, db.InsertMessageParams{
		ID:                db.PgtypeUUID(uuid.New()),
		TenantID:          db.PgtypeUUID(m.TenantID),
		WhatsappSessionID: db.PgtypeUUID(m.WhatsAppSessionID),
		ProviderMessageID: m.ProviderMessageID,
		Phone:             m.Phone,
		Direction:         m.Direction,
		Sender:            m.Sender,
		MessageType:       m.MessageType,
		Body:              db.StringToText(m.Body),
		MediaS3Key:        db.StringToText(m.MediaS3Key),
		MediaMimetype:     db.StringToText(m.MediaMIME),
		IdempotencyKey:    m.IdempotencyKey,
		Timestamp:         db.TimeToTimestamptz(m.Timestamp),
	})
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" { // unique_violation
			return ErrDuplicate
		}
		return fmt.Errorf("insert message: %w", err)
	}
	return nil
}

// GetByID fetches a message by its primary key. Returns ErrNotFound if absent.
func (r *pgRepository) GetByID(ctx context.Context, id uuid.UUID) (*Message, error) {
	row, err := r.q.GetMessageByID(ctx, db.PgtypeUUID(id))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get message by id: %w", err)
	}
	return convertMessage(row), nil
}

// GetByIdempotencyKey fetches a message by its idempotency key.
// Returns ErrNotFound if absent — useful for replay detection.
func (r *pgRepository) GetByIdempotencyKey(ctx context.Context, key string) (*Message, error) {
	row, err := r.q.GetMessageByIdempotencyKey(ctx, key)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get message by idempotency key: %w", err)
	}
	return convertMessage(row), nil
}

// History returns the most recent `limit` messages for (tenant, phone),
// ASC by timestamp (oldest first). Matches the AI prompt's "conversation
// so far" shape.
func (r *pgRepository) History(ctx context.Context, tenantID uuid.UUID, phone string, limit int) ([]*Message, error) {
	rows, err := r.q.ListMessagesByPhone(ctx, db.ListMessagesByPhoneParams{
		TenantID: db.PgtypeUUID(tenantID),
		Phone:    phone,
		Limit:    int32(limit),
	})
	if err != nil {
		return nil, fmt.Errorf("list messages by phone: %w", err)
	}
	out := make([]*Message, len(rows))
	for i, row := range rows {
		out[i] = convertMessage(row)
	}
	return out, nil
}

// ListByTenant returns a paginated list of messages for the dashboard.
func (r *pgRepository) ListByTenant(ctx context.Context, tenantID uuid.UUID, limit, offset int) ([]*Message, error) {
	rows, err := r.q.ListRecentMessagesByTenant(ctx, db.ListRecentMessagesByTenantParams{
		TenantID: db.PgtypeUUID(tenantID),
		Limit:    int32(limit),
		Offset:   int32(offset),
	})
	if err != nil {
		return nil, fmt.Errorf("list messages by tenant: %w", err)
	}
	out := make([]*Message, len(rows))
	for i, row := range rows {
		out[i] = convertMessage(row)
	}
	return out, nil
}

// convertMessage maps a sqlc-generated row to our domain Message type.
func convertMessage(row db.Message) *Message {
	return &Message{
		ID:                db.MustUUID(row.ID),
		TenantID:          db.MustUUID(row.TenantID),
		WhatsAppSessionID: db.MustUUID(row.WhatsappSessionID),
		ProviderMessageID: row.ProviderMessageID,
		Phone:             row.Phone,
		Direction:         row.Direction,
		Sender:            row.Sender,
		MessageType:       row.MessageType,
		Body:              db.StringFromText(row.Body),
		MediaS3Key:        db.StringFromText(row.MediaS3Key),
		MediaMIME:         db.StringFromText(row.MediaMimetype),
		IdempotencyKey:    row.IdempotencyKey,
		Timestamp:         db.TimeFromTimestamptz(row.Timestamp),
		CreatedAt:         db.TimeFromTimestamptz(row.CreatedAt),
	}
}
