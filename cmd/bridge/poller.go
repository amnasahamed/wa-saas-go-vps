package main

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/amnasahamed/niosengine/internal/queue"
	"github.com/amnasahamed/niosengine/internal/whatsapp"
)

// queuePoller drains the Redis delay queue and dispatches each reply through
// the whatsmeow manager. It runs as a single goroutine in the bridge process
// (one per worker); a second worker in the same Redis namespace would just
// race for the same ZSET entries (PopDue is atomic), which is fine — both
// would call SendMessage with different sessionIDs.
//
// Block window: 1 second. That keeps shutdown latency reasonable while still
// avoiding tight CPU spin when the queue is empty.
type queuePoller struct {
	q       queue.Queue
	manager whatsapp.Manager
	log     *slog.Logger
}

func newQueuePoller(q queue.Queue, m whatsapp.Manager, log *slog.Logger) *queuePoller {
	return &queuePoller{q: q, manager: m, log: log}
}

// Run loops until ctx is cancelled. Errors from PopDue other than ErrEmpty
// are logged and the loop continues (transient Redis errors shouldn't kill
// the worker).
func (p *queuePoller) Run(ctx context.Context) {
	for {
		if ctx.Err() != nil {
			return
		}
		replies, err := p.q.PopDue(ctx, time.Second, 16)
		if err != nil {
			if errors.Is(err, queue.ErrEmpty) || errors.Is(err, context.Canceled) {
				continue
			}
			p.log.Warn("pop due failed", slog.String("err", err.Error()))
			time.Sleep(500 * time.Millisecond)
			continue
		}
		for _, r := range replies {
			p.dispatch(ctx, r)
		}
	}
}

// dispatch sends a single scheduled reply. In MVP we route by tenant and let
// the manager pick the right session. Phone-level session lookup (used in
// later phases for multi-device) lands with the multi-device feature.
func (p *queuePoller) dispatch(ctx context.Context, r queue.ScheduledReply) {
	sessions, err := p.manager.ListSessions(ctx)
	if err != nil {
		p.log.Warn("list sessions failed",
			slog.String("tenant_id", r.TenantID),
			slog.String("err", err.Error()),
		)
		return
	}
	if len(sessions) == 0 {
		p.log.Warn("no sessions for tenant, dropping reply",
			slog.String("tenant_id", r.TenantID),
			slog.String("phone", r.Phone),
		)
		return
	}
	// Pick the first session for the tenant. Multi-tenant tenants with
	// multiple sessions will need an explicit (sessionID, phone) lookup
	// in a follow-up.
	var sessionID string
	for _, s := range sessions {
		if s.TenantID == r.TenantID {
			sessionID = s.ID
			break
		}
	}
	if sessionID == "" {
		p.log.Warn("no session for tenant",
			slog.String("tenant_id", r.TenantID),
		)
		return
	}

	if _, err := p.manager.SendMessage(ctx, sessionID, whatsapp.SendOptions{
		Phone: r.Phone,
		Body:  r.Body,
	}); err != nil {
		p.log.Warn("send reply failed",
			slog.String("tenant_id", r.TenantID),
			slog.String("phone", r.Phone),
			slog.String("err", err.Error()),
		)
		return
	}
	p.log.Info("scheduled reply sent",
		slog.String("tenant_id", r.TenantID),
		slog.String("phone", r.Phone),
	)
}