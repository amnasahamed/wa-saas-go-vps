// Package extraction implements the GPT-4o call that converts a WhatsApp
// conversation into structured CRM fields.
//
// Ported verbatim from workflownew.json's Build AI Prompt1 + GPT-4o Extract1 +
// Map to CRM Fields1 nodes. The system prompt is configurable per tenant;
// NIOS is the default template.
package extraction

import (
	"context"
	"errors"
	"time"
)

// Input is what the extraction pipeline passes in.
type Input struct {
	TenantID    string
	LeadPhone   string
	LeadID      string
	History     []HistoryEntry // last N messages, ASC by timestamp
	LatestBody  string         // the message that triggered this extraction
	LatestType  string         // "text" | "image" | etc.
	IsVoiceNote bool
	LeadFields  map[string]any // known fields so far (for context)
	LeadStage   string
	LeadStatus  string
}

// HistoryEntry is one row of conversation context.
type HistoryEntry struct {
	Direction string // "inbound" | "outbound"
	Sender    string // "customer" | "ai" | "counselor"
	Body      string
	Timestamp time.Time
}

// Output is the structured result from GPT-4o (JSON mode).
type Output struct {
	StudentName      string   `json:"student_name"`
	AlternatePhone   string   `json:"alternate_phone"`
	ClassLevel       string   `json:"class_level"`
	Stream           string   `json:"stream"`
	ExamResult       string   `json:"exam_result"` // "fail" | "first_attempt" | "appearing"
	SubjectsFailed   []string `json:"subjects_failed"`
	SubjectsCount    int      `json:"subjects_count"`
	HasCertificate   string   `json:"has_certificate"` // "yes" | "no"
	Place            string   `json:"place"`
	PriceDisclosed   string   `json:"price_disclosed"`
	Intent           string   `json:"intent"`      // "new_inquiry" | "fee_inquiry" | "demo_request" | "follow_up" | "not_interested" | "enrolled" | "other"
	LeadStatus       string   `json:"lead_status"` // "New" | "Contacted" | "Interested" | "Follow-up" | "Lost" | "Enrolled"
	LostReason       string   `json:"lost_reason"`
	NextFollowupDays int      `json:"next_followup_days"`
	Remarks          string   `json:"remarks"`
}

// Caller wraps the OpenAI API.
type Caller interface {
	Extract(ctx context.Context, in Input, systemPrompt string) (*Output, error)
}

// PromptBuilder constructs the extraction system prompt from a tenant
// template + the lead context.
type PromptBuilder interface {
	Build(in Input, customTemplate string) string
}

// Extractor is the high-level service.
//
// Behavior:
//   - Builds prompt from history + lead + latest message
//   - Calls gpt-4o in JSON mode (temp=0.2)
//   - Returns Output (caller merges into lead.Fields)
type Extractor interface {
	Extract(ctx context.Context, in Input) (*Output, error)
}

// ErrEmptyResult is returned when GPT-4o returns no usable JSON.
var ErrEmptyResult = errors.New("extraction returned empty result")

// ErrInvalidJSON is returned when GPT-4o returns malformed JSON.
var ErrInvalidJSON = errors.New("extraction returned invalid JSON")

// DefaultModel is the OpenAI model used for extraction.
const DefaultModel = "gpt-4o"

// DefaultTemperature for extraction (low — deterministic).
const DefaultTemperature = 0.2
