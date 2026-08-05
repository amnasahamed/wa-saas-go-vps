package leads

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/amnasahamed/niosengine/internal/db"
)

// pgRepository is the pgx-backed implementation of Repository.
type pgRepository struct {
	q *db.Queries
}

// Compile-time interface conformance check.
var _ Repository = (*pgRepository)(nil)

// NewPGRepository wraps a sqlc Queries instance.
func NewPGRepository(q *db.Queries) Repository {
	return &pgRepository{q: q}
}

// GetOrCreate returns the existing lead for (tenant, phone) or creates
// a new one with stage=ask_name, status=New.
//
// Idempotent because of the UNIQUE (tenant_id, phone) constraint.
func (r *pgRepository) GetOrCreate(ctx context.Context, tenantID uuid.UUID, phone string) (*Lead, error) {
	emptyFields, _ := db.MapToJSON(map[string]any{}) // -> "{}"
	row, err := r.q.UpsertLead(ctx, db.UpsertLeadParams{
		ID:       db.PgtypeUUID(uuid.New()),
		TenantID: db.PgtypeUUID(tenantID),
		Phone:    phone,
		Name:     db.StringToText(""),
		Fields:   emptyFields, // NOT NULL; COALESCE in SQL preserves existing
		Status:   StatusNew,
		Stage:    StageAskName,
	})
	if err != nil {
		return nil, fmt.Errorf("upsert lead: %w", err)
	}
	return convertLead(row), nil
}

// GetByID fetches a lead by primary key. Returns ErrNotFound if absent.
func (r *pgRepository) GetByID(ctx context.Context, id uuid.UUID) (*Lead, error) {
	row, err := r.q.GetLeadByID(ctx, db.PgtypeUUID(id))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get lead by id: %w", err)
	}
	return convertLead(row), nil
}

// GetByPhone fetches a lead by (tenant, phone). Returns ErrNotFound if absent.
func (r *pgRepository) GetByPhone(ctx context.Context, tenantID uuid.UUID, phone string) (*Lead, error) {
	row, err := r.q.GetLeadByPhone(ctx, db.GetLeadByPhoneParams{
		TenantID: db.PgtypeUUID(tenantID),
		Phone:    phone,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get lead by phone: %w", err)
	}
	return convertLead(row), nil
}

// ListByTenant paginates the dashboard's leads table. status="" lists all.
func (r *pgRepository) ListByTenant(ctx context.Context, tenantID uuid.UUID, limit, offset int, status string) ([]*Lead, error) {
	rows, err := r.q.ListLeadsByTenant(ctx, db.ListLeadsByTenantParams{
		TenantID: db.PgtypeUUID(tenantID),
		Column2:  status, // sqlc named this Column2 because of the IS NULL OR pattern
		Limit:    int32(limit),
		Offset:   int32(offset),
	})
	if err != nil {
		return nil, fmt.Errorf("list leads by tenant: %w", err)
	}
	out := make([]*Lead, len(rows))
	for i, row := range rows {
		out[i] = convertLead(row)
	}
	return out, nil
}

// UpdateFields replaces the lead's fields jsonb with the given map.
func (r *pgRepository) UpdateFields(ctx context.Context, id uuid.UUID, fields map[string]any) error {
	jsonBytes, err := db.MapToJSON(fields)
	if err != nil {
		return fmt.Errorf("marshal fields: %w", err)
	}
	return r.q.UpdateLeadFields(ctx, db.UpdateLeadFieldsParams{
		ID:     db.PgtypeUUID(id),
		Fields: jsonBytes,
	})
}

// UpdateStage sets the lead's nurture stage.
func (r *pgRepository) UpdateStage(ctx context.Context, id uuid.UUID, stage string) error {
	return r.q.UpdateLeadStage(ctx, db.UpdateLeadStageParams{
		ID:    db.PgtypeUUID(id),
		Stage: stage,
	})
}

// UpdateStatus sets the lead's status.
func (r *pgRepository) UpdateStatus(ctx context.Context, id uuid.UUID, status string) error {
	return r.q.UpdateLeadStatus(ctx, db.UpdateLeadStatusParams{
		ID:     db.PgtypeUUID(id),
		Status: status,
	})
}

// UpdateName sets the lead's name.
func (r *pgRepository) UpdateName(ctx context.Context, id uuid.UUID, name string) error {
	return r.q.UpdateLeadName(ctx, db.UpdateLeadNameParams{
		ID:   db.PgtypeUUID(id),
		Name: db.StringToText(name),
	})
}

// SetNurturePausedUntil sets the lead's nurture_paused_until timestamp.
func (r *pgRepository) SetNurturePausedUntil(ctx context.Context, id uuid.UUID, until time.Time) error {
	return r.q.SetLeadNurturePaused(ctx, db.SetLeadNurturePausedParams{
		ID:                 db.PgtypeUUID(id),
		NurturePausedUntil: db.TimeToTimestamptz(until),
	})
}

// UpdateLastInboundAt bumps the lead's last_inbound_at timestamp.
func (r *pgRepository) UpdateLastInboundAt(ctx context.Context, id uuid.UUID, t time.Time) error {
	return r.q.UpdateLeadTimestamps(ctx, db.UpdateLeadTimestampsParams{
		ID:              db.PgtypeUUID(id),
		LastInboundAt:   db.TimeToTimestamptz(t),
		LastContactedAt: pgtype.Timestamptz{}, // don't touch
		NextFollowupAt:  pgtype.Timestamptz{}, // don't touch
	})
}

// IncrementFollowupCount bumps the lead's followup_count by 1.
func (r *pgRepository) IncrementFollowupCount(ctx context.Context, id uuid.UUID) error {
	return r.q.IncrementLeadFollowupCount(ctx, db.PgtypeUUID(id))
}

// convertLead maps a sqlc-generated row to our domain Lead type.
func convertLead(row db.Lead) *Lead {
	fields, _ := db.JSONToMap(row.Fields)
	return &Lead{
		ID:                 db.MustUUID(row.ID),
		TenantID:           db.MustUUID(row.TenantID),
		Phone:              row.Phone,
		AlternatePhone:     db.StringFromText(row.AlternatePhone),
		Name:               db.StringFromText(row.Name),
		Fields:             fields,
		Status:             row.Status,
		Stage:              row.Stage,
		NurturePaused:      row.NurturePaused,
		NurturePausedUntil: db.TimeFromTimestamptzPtr(row.NurturePausedUntil),
		LastInboundAt:      db.TimeFromTimestamptzPtr(row.LastInboundAt),
		LastContactedAt:    db.TimeFromTimestamptzPtr(row.LastContactedAt),
		NextFollowupAt:     db.TimeFromTimestamptzPtr(row.NextFollowupAt),
		FollowupCount:      int(row.FollowupCount),
		CreatedAt:          db.TimeFromTimestamptz(row.CreatedAt),
		UpdatedAt:          db.TimeFromTimestamptz(row.UpdatedAt),
	}
}
