// Package leads manages the lead aggregate: one per (tenant, phone).
//
// A lead carries the progressive-nurture state machine status, the JSONB
// `fields` bag that holds extracted CRM data (class_level, stream,
// subjects_failed, etc.), and the stage transitions driven by the
// nurture package.
package leads

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
)

// Stage constants ported verbatim from workflownew.json.
const (
	StageAskName           = "ask_name"
	StageAskQualification  = "ask_qualification"
	StageAskPassFail       = "ask_pass_fail"
	StageAskStream         = "ask_stream"
	StageAskFailedSubjects = "ask_failed_subjects"
	StageAskCertificate    = "ask_certificate"
	StageAskPlace          = "ask_place"
	StageComplete          = "complete"
)

// Status constants.
const (
	StatusNew        = "New"
	StatusContacted  = "Contacted"
	StatusInterested = "Interested"
	StatusFollowUp   = "Follow-up"
	StatusLost       = "Lost"
	StatusEnrolled   = "Enrolled"
)

// Lead is the persisted aggregate.
type Lead struct {
	ID                 uuid.UUID
	TenantID           uuid.UUID
	Phone              string
	AlternatePhone     string
	Name               string
	Fields             map[string]any // jsonb in DB
	Status             string
	Stage              string
	NurturePaused      bool
	NurturePausedUntil *time.Time
	LastInboundAt      *time.Time
	LastContactedAt    *time.Time
	NextFollowupAt     *time.Time
	FollowupCount      int
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

// Repository persists Lead records.
type Repository interface {
	GetOrCreate(ctx context.Context, tenantID uuid.UUID, phone string) (*Lead, error)
	GetByPhone(ctx context.Context, tenantID uuid.UUID, phone string) (*Lead, error)
	GetByID(ctx context.Context, id uuid.UUID) (*Lead, error)
	ListByTenant(ctx context.Context, tenantID uuid.UUID, limit, offset int, status string) ([]*Lead, error)
	UpdateFields(ctx context.Context, id uuid.UUID, fields map[string]any) error
	UpdateStage(ctx context.Context, id uuid.UUID, stage string) error
	UpdateStatus(ctx context.Context, id uuid.UUID, status string) error
	UpdateName(ctx context.Context, id uuid.UUID, name string) error
	SetNurturePausedUntil(ctx context.Context, id uuid.UUID, until time.Time) error
	UpdateLastInboundAt(ctx context.Context, id uuid.UUID, t time.Time) error
	IncrementFollowupCount(ctx context.Context, id uuid.UUID) error
}

// Service is the business-logic layer.
type Service interface {
	GetOrCreate(ctx context.Context, tenantID uuid.UUID, phone string) (*Lead, error)
	Get(ctx context.Context, id uuid.UUID) (*Lead, error)
	List(ctx context.Context, tenantID uuid.UUID, limit, offset int, status string) ([]*Lead, error)
	MergeFields(ctx context.Context, id uuid.UUID, extracted map[string]any) (*Lead, error)
	SetStage(ctx context.Context, id uuid.UUID, stage string) error
	SetStatus(ctx context.Context, id uuid.UUID, status string) error
	IncrementFollowup(ctx context.Context, id uuid.UUID) error
	TouchLastInbound(ctx context.Context, id uuid.UUID, at time.Time) error
	MarkNurturePaused(ctx context.Context, id uuid.UUID, until time.Time) error
}

// ErrNotFound is returned when no lead matches.
var ErrNotFound = errors.New("lead not found")

// ShouldNurture implements the gating logic from workflownew.json's
// `should_nurture` check, factoring in voice-note suppression (which lives
// in nurture.Debounce), and pause window.
//
// Returns true if:
//   - direction == "inbound"
//   - status != Lost AND != Enrolled
//   - stage != "complete"
//   - now >= nurture_paused_until (no recent AI message)
//   - message_type is not a voice note (caller passes in isVoiceNote)
func ShouldNurture(l *Lead, direction string, isVoiceNote bool, now time.Time) bool {
	if l == nil {
		return false
	}
	if direction != "inbound" {
		return false
	}
	if l.Status == StatusLost || l.Status == StatusEnrolled {
		return false
	}
	if l.Stage == StageComplete {
		return false
	}
	if isVoiceNote {
		return false
	}
	if l.NurturePausedUntil != nil && now.Before(*l.NurturePausedUntil) {
		return false
	}
	return true
}
