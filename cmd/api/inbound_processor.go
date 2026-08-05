package main

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/amnasahamed/niosengine/internal/db"
	"github.com/amnasahamed/niosengine/internal/messages"

	bridgepb "github.com/amnasahamed/niosengine/internal/bridgepb"
)

// process persists a gRPC inbound event through the same composition the
// legacy /v1/inbound HTTP handler used (cmd/api/inbound.go:handleInbound):
//
//	1. validate the proto
//	2. look up the tenant's WhatsApp session
//	3. tenant/session ownership check (defence in depth — a buggy or
//	   malicious bridge must not write messages for an arbitrary tenant)
//	4. PersistInboundWithStatus — idempotent insert + reply-loop guard
//	5. GetOrCreate lead
//	6. TouchLastInbound for inbound direction
//
// Transport-neutral: the caller (gRPC ReportInbound or the JSON HTTP
// handler) maps errors to its own response format. We surface typed
// errors via sentinel `errSession*` so the gRPC handler can return
// codes.PermissionDenied / codes.NotFound / codes.Unavailable.
func (p *inboundProcessor) process(ctx context.Context, ev *bridgepb.InboundEvent) error {
	if p == nil {
		return fmt.Errorf("inboundProcessor: nil receiver")
	}
	if p.deps.Queries == nil {
		return fmt.Errorf("inboundProcessor: queries not wired")
	}
	if p.deps.MessagesSvc == nil || p.deps.LeadsSvc == nil {
		return fmt.Errorf("inboundProcessor: services not wired")
	}

	// 1) Validate the proto. The gRPC handler already checked the absolute
	// required fields (tenant_id, provider_message_id), so anything that
	// reaches here is a programming error in the bridge.
	tenantID, err := uuid.Parse(ev.TenantId)
	if err != nil {
		return status.Errorf(codes.InvalidArgument, "tenant_id is not a uuid: %v", err)
	}
	if ev.Phone == "" {
		return status.Error(codes.InvalidArgument, "phone is required")
	}
	if ev.IdempotencyKey == "" {
		return status.Error(codes.InvalidArgument, "idempotency_key is required")
	}
	if ev.Direction != "inbound" && ev.Direction != "outbound" {
		return status.Errorf(codes.InvalidArgument, "direction must be inbound|outbound, got %q", ev.Direction)
	}
	msgType := ev.MessageType
	if msgType == "" {
		msgType = "text"
	}
	ts := ev.Timestamp.AsTime()
	if ts.IsZero() {
		ts = time.Now().UTC()
	}

	// 2) Look up the session. The proto carries a whatsapp_session_id; the
	// HTTP path looked up by tenant_id (single-session per tenant). Use the
	// proto's ID when present and fall back to the tenant lookup when the
	// bridge hasn't told us which session (older bridge builds).
	sessionRow, err := p.lookupSession(ctx, ev.WhatsappSessionId, tenantID)
	if err != nil {
		return err
	}
	sessionID, err := db.UUIDFromPgtype(sessionRow.ID)
	if err != nil {
		return status.Errorf(codes.Internal, "decode session id: %v", err)
	}

	// 3) Defence in depth: a misbehaving bridge might claim a tenant_id that
	// is not the owner of the session_id it supplied. Compare explicitly.
	if ev.WhatsappSessionId != "" && ev.WhatsappSessionId != sessionID.String() {
		return status.Error(codes.PermissionDenied, "session does not belong to tenant")
	}
	// sessionRow.TenantID is a pgtype.UUID; compare to the parsed tenant.
	if got, _ := db.UUIDFromPgtype(sessionRow.TenantID); got != tenantID {
		return status.Error(codes.PermissionDenied, "session does not belong to tenant")
	}

	// 4) Persist idempotently. PersistInboundWithStatus owns the
	// reply-loop guard (skips outbound+ai).
	msg := &messages.Message{
		TenantID:          tenantID,
		WhatsAppSessionID: sessionID,
		ProviderMessageID: ev.ProviderMessageId,
		Phone:             ev.Phone,
		Direction:         ev.Direction,
		Sender:            ev.Sender,
		MessageType:       msgType,
		Body:              ev.Body,
		MediaS3Key:        ev.MediaS3Key,
		MediaMIME:         ev.MediaMimetype,
		IdempotencyKey:    buildIdempotencyKey(tenantID, ev.IdempotencyKey),
		Timestamp:         ts,
	}
	result, err := p.deps.MessagesSvc.PersistInboundWithStatus(ctx, msg)
	if err != nil {
		return status.Errorf(codes.Internal, "persist: %v", err)
	}
	if result.Skipped {
		p.deps.Log.Info("skipped outbound AI message (reply-loop guard)",
			"tenant_id", tenantID.String(),
			"phone", ev.Phone,
		)
		return nil
	}

	// 5) Upsert lead. We do this even for outbound messages because the
	// reply-loop guard already filtered, and a counselor reply should
	// still attach to the existing lead row.
	lead, err := p.deps.LeadsSvc.GetOrCreate(ctx, tenantID, ev.Phone)
	if err != nil {
		return status.Errorf(codes.Internal, "upsert lead: %v", err)
	}

	// 6) Bump last_inbound_at only for inbound direction.
	if ev.Direction == "inbound" {
		if err := p.deps.LeadsSvc.TouchLastInbound(ctx, lead.ID, ts); err != nil {
			// Non-fatal: the row is persisted, just the timestamp is stale.
			p.deps.Log.Warn("touch last_inbound",
				"lead_id", lead.ID.String(),
				"err", err.Error(),
			)
		}
	}

	if result.Duplicate {
		p.deps.Log.Info("inbound replay (idempotency_key collision)",
			"tenant_id", tenantID.String(),
			"provider_message_id", ev.ProviderMessageId,
		)
	}
	return nil
}

// lookupSession returns the session row for either an explicit session ID
// (preferred) or the single session belonging to the tenant.
func (p *inboundProcessor) lookupSession(ctx context.Context, sessionID string, tenantID uuid.UUID) (db.WhatsappSession, error) {
	if sessionID != "" {
		sid, err := uuid.Parse(sessionID)
		if err != nil {
			return db.WhatsappSession{}, status.Errorf(codes.InvalidArgument, "whatsapp_session_id is not a uuid: %v", err)
		}
		row, err := p.deps.Queries.GetWhatsAppSessionByID(ctx, db.PgtypeUUID(sid))
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return db.WhatsappSession{}, status.Error(codes.NotFound, "whatsapp_session_id not found")
			}
			return db.WhatsappSession{}, status.Errorf(codes.Internal, "lookup session: %v", err)
		}
		return row, nil
	}
	row, err := p.deps.Queries.GetWhatsAppSessionByTenant(ctx, db.PgtypeUUID(tenantID))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return db.WhatsappSession{}, status.Error(codes.NotFound, "no whatsapp session for tenant")
		}
		return db.WhatsappSession{}, status.Errorf(codes.Internal, "lookup session by tenant: %v", err)
	}
	return row, nil
}
