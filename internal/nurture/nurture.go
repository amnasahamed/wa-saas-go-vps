// Package nurture implements the progressive lead-nurturing state machine,
// the GPT-4o reply generator, and the 90-second debounce window.
//
// Ported verbatim from workflownew.json's Send Nurture? → Build Nurture Prompt
// → GPT Nurture Reply → Prepare Reply → Human Delay → Send WhatsApp Reply
// → Save Outbound Message pipeline.
//
// Stage transitions:
//
//	ask_name → ask_qualification → ask_pass_fail → ask_stream
//	  → ask_failed_subjects → ask_certificate → ask_place → complete
//
// Stream is only asked when class_level looks like Plus Two / HS / higher.
// Failed-subjects + certificate are only asked when exam_result == "fail".
// Place is only asked when has_certificate is known.
package nurture

import (
	"context"
	"errors"
	"time"

	"github.com/amnasahamed/niosengine/internal/extraction"
)

// ReplyMode determines the nurture reply style.
const (
	ReplyAskOnly       = "ask_only"        // pure next-stage question
	ReplyAnswerThenAsk = "answer_then_ask" // answer the user's question, then advance the stage
)

// Stage transitions: (currentStage, leadFields) → nextStage.
type StageEvaluator interface {
	NextStage(currentStage string, fields map[string]any) string
	IsComplete(fields map[string]any) bool
}

// ReplyGenerator produces the outbound WhatsApp reply text.
type ReplyGenerator interface {
	Generate(ctx context.Context, in ReplyInput) (*ReplyOutput, error)
}

// ReplyInput is what the generator needs.
type ReplyInput struct {
	TenantID    string
	LeadPhone   string
	LeadStage   string
	LeadFields  map[string]any
	History     []extraction.HistoryEntry
	LatestBody  string
	LatestType  string
	IsVoiceNote bool
	IsQuestion  bool // true if the customer's latest message asks about fees/duration/etc.
	Locale      string
	ReplyMode   string // ReplyAskOnly | ReplyAnswerThenAsk
}

// ReplyOutput is the generated reply plus delay metadata.
type ReplyOutput struct {
	Body     string
	Language string // "english" | "malayalam" | "manglish"
	DelayMs  int    // 8–23s per legacy
	Fallback bool   // true if we used the canned fallback instead of GPT-4o
}

// ReplyScheduler queues outbound replies on a Redis ZSET for the bridge to poll.
//
// The orchestrator schedules replies via ZADD with score = send_at_unix_ms;
// the bridge worker runs a polling loop and pops due entries.
type ReplyScheduler interface {
	Schedule(ctx context.Context, tenantID, phone string, body string, sendAt time.Time) error
	Cancel(ctx context.Context, tenantID, phone string) error
}

// Debounce enforces the 90s pause window: after a nurture reply is sent,
// suppress further automated replies to the same lead for 90 seconds.
//
// Implementation is on the Lead itself (nurture_paused_until column), but
// this interface centralizes the policy so callers don't sprinkle the
// magic number across packages.
type Debouncer interface {
	ShouldSuppress(ctx context.Context, leadID string, now time.Time) (bool, error)
	MarkReplied(ctx context.Context, leadID string, now time.Time) error
}

// Pipeline is the high-level orchestrator: stage evaluation + reply
// generation + scheduling + outbound persistence.
type Pipeline interface {
	// Run decides whether to nurture, generates the reply if so, and
	// schedules the send. Returns the ReplyOutput that was scheduled, or
	// nil if nurturing was suppressed.
	Run(ctx context.Context, in PipelineInput) (*ReplyOutput, error)
}

// PipelineInput is the package of data the pipeline needs.
type PipelineInput struct {
	TenantID    string
	LeadID      string
	LeadPhone   string
	LeadStage   string
	LeadStatus  string
	LeadFields  map[string]any
	History     []extraction.HistoryEntry
	LatestBody  string
	LatestType  string
	IsVoiceNote bool
	Locale      string
	CountryCode string
}

// DefaultDebounceWindow matches workflownew.json's 90-second window.
const DefaultDebounceWindow = 90 * time.Second

// DefaultReplyDelayMin / Max are the bounds for the "human-like" delay.
// workflownew.json's Prepare Reply node calculates 8–23 seconds.
const (
	DefaultReplyDelayMinMs = 8000
	DefaultReplyDelayMaxMs = 23000
)

// QuestionTopics are the regex patterns that detect "the customer is asking
// a question we should answer before continuing data collection".
// Ported from workflownew.json's Prepare Reply node.
var QuestionTopics = []string{
	"fee", "fees", "cost", "price", "pricing", "charges",
	"duration", "months", "how long",
	"toc", "transfer of credit",
	"certificate", "mark sheet", "marksheet",
	"exam", "exams", "examination",
	"details", "information", "info",
}

// ErrSuppressed is returned when the pipeline decides not to nurture.
var ErrSuppressed = errors.New("nurture suppressed (paused, complete, voice note, etc.)")
