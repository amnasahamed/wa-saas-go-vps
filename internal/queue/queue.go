// Package queue provides a Redis-backed delay queue for scheduled outbound
// WhatsApp replies. The orchestrator schedules replies via ZADD with
// score = unix_ms; the bridge worker polls and pops due entries.
//
// Why a delay queue: the nurture layer wants to wait 8–23 seconds before
// sending an AI reply (mimics human response time). The bridge worker is
// the only thing that can actually call whatsmeow.SendMessage, so the
// orchestrator schedules the send, and the bridge picks it up.
package queue

import (
	"context"
	"errors"
	"time"
)

// ScheduledReply is the work item stored in the Redis ZSET.
type ScheduledReply struct {
	TenantID string
	Phone    string
	Body     string
	SendAt   time.Time
	ID       string
}

// Queue is the high-level interface.
type Queue interface {
	// Schedule enqueues a reply to be sent at sendAt.
	Schedule(ctx context.Context, r ScheduledReply) error

	// Cancel removes any pending reply for (tenantID, phone).
	Cancel(ctx context.Context, tenantID, phone string) error

	// PopDue atomically pops up to `limit` replies that are due now,
	// blocking for at most `block` if none are due.
	PopDue(ctx context.Context, block time.Duration, limit int) ([]ScheduledReply, error)
}

// ErrEmpty is returned by PopDue when no work is available after blocking.
var ErrEmpty = errors.New("queue empty")

// RedisKey is the canonical key for the scheduled-replies ZSET.
func RedisKey() string { return "niosengine:replies:scheduled" }

// Score returns the Redis ZSET score (unix milliseconds) for a given time.
func Score(t time.Time) float64 {
	return float64(t.UnixMilli())
}
