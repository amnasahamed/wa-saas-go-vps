package leads

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"github.com/amnasahamed/niosengine/internal/extraction"
)

// pgService is the pgx-backed implementation of Service.
type pgService struct {
	repo Repository
	log  *slog.Logger
}

// NewPGService wires the leads Service.
func NewPGService(repo Repository, log *slog.Logger) Service {
	if log == nil {
		log = slog.Default()
	}
	return &pgService{repo: repo, log: log}
}

// GetOrCreate delegates to the repository.
func (s *pgService) GetOrCreate(ctx context.Context, tenantID uuid.UUID, phone string) (*Lead, error) {
	return s.repo.GetOrCreate(ctx, tenantID, phone)
}

// Get fetches a lead by ID.
func (s *pgService) Get(ctx context.Context, id uuid.UUID) (*Lead, error) {
	return s.repo.GetByID(ctx, id)
}

// List paginates leads for the dashboard.
func (s *pgService) List(ctx context.Context, tenantID uuid.UUID, limit, offset int, status string) ([]*Lead, error) {
	return s.repo.ListByTenant(ctx, tenantID, limit, offset, status)
}

// MergeFields applies a freshly-extracted fields map onto the existing
// lead, preserving any non-empty values that the extraction missed.
//
// This delegates the merge logic to extraction.MergeFields (the canonical
// implementation ported from workflownew.json's Merge With Existing Lead
// node). Then persists the result.
func (s *pgService) MergeFields(ctx context.Context, id uuid.UUID, extracted map[string]any) (*Lead, error) {
	current, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("get lead for merge: %w", err)
	}

	merged := extraction.MergeFields(current.Fields, extracted)
	if err := s.repo.UpdateFields(ctx, id, merged); err != nil {
		return nil, fmt.Errorf("update fields: %w", err)
	}

	current.Fields = merged
	return current, nil
}

// SetStage updates the lead's nurture stage.
func (s *pgService) SetStage(ctx context.Context, id uuid.UUID, stage string) error {
	if err := s.repo.UpdateStage(ctx, id, stage); err != nil {
		return err
	}
	s.log.Debug("lead stage updated", slog.String("lead_id", id.String()), slog.String("stage", stage))
	return nil
}

// SetStatus updates the lead's status.
func (s *pgService) SetStatus(ctx context.Context, id uuid.UUID, status string) error {
	return s.repo.UpdateStatus(ctx, id, status)
}

// IncrementFollowup bumps the followup_count.
func (s *pgService) IncrementFollowup(ctx context.Context, id uuid.UUID) error {
	return s.repo.IncrementFollowupCount(ctx, id)
}

// MarkNurturePaused is a convenience for the nurture pipeline: sets
// nurture_paused_until = now + 90s, suppressing further AI replies for
// the lead until then.
func (s *pgService) MarkNurturePaused(ctx context.Context, id uuid.UUID, until time.Time) error {
	return s.repo.SetNurturePausedUntil(ctx, id, until)
}

// TouchLastInbound bumps last_inbound_at and returns the updated lead.
func (s *pgService) TouchLastInbound(ctx context.Context, id uuid.UUID, at time.Time) error {
	return s.repo.UpdateLastInboundAt(ctx, id, at)
}
