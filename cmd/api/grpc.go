package main

import (
	"context"
	"fmt"
	"log/slog"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/amnasahamed/niosengine/internal/db"
	"github.com/amnasahamed/niosengine/internal/leads"
	"github.com/amnasahamed/niosengine/internal/messages"

	bridgepb "github.com/amnasahamed/niosengine/internal/bridgepb"
)

// apiBridgeService implements bridgepb.BridgeToAPIServer. The bridge worker
// calls ReportInbound when a whatsmeow event arrives; we ack so the bridge
// knows the message was durably accepted.
//
// Inbound persistence + extraction runs through the same path the legacy
// /v1/inbound webhook used: messages.Service.PersistInbound followed by
// extraction/nurture (in phase-3). For phase-1 we only persist the row and
// ack so the bridge can drop the event from its retry buffer.
type apiBridgeService struct {
	bridgepb.UnimplementedBridgeToAPIServer

	proc *inboundProcessor
	log  *slog.Logger
}

func newAPIBridgeService(proc *inboundProcessor, log *slog.Logger) *apiBridgeService {
	return &apiBridgeService{proc: proc, log: log}
}

// ReportInbound persists the inbound message and acks.
//
// We translate the proto event into the same shape the dashboard /v1/inbound
// webhook uses, then hand it off to messages.Service. The bridge is
// fire-and-forget on its side: a non-OK ack means "retry the event";
// persistent failure escalates to the bridge error log.
func (s *apiBridgeService) ReportInbound(ctx context.Context, ev *bridgepb.InboundEvent) (*bridgepb.InboundAck, error) {
	if ev.TenantId == "" || ev.ProviderMessageId == "" {
		return nil, status.Error(codes.InvalidArgument, "tenant_id and provider_message_id are required")
	}

	if s.proc != nil {
		if err := s.proc.Process(ctx, ev); err != nil {
			s.log.Error("inbound process failed",
				slog.String("tenant_id", ev.TenantId),
				slog.String("provider_message_id", ev.ProviderMessageId),
				slog.String("err", err.Error()),
			)
			// Map to gRPC codes so the bridge's retry helper can decide.
			// Anything that is not a clear "retry me" condition becomes
			// Unauthenticated, PermissionDenied, or InvalidArgument via the
			// processor; only transients land here.
			return nil, status.Error(codes.Unavailable, err.Error())
		}
	} else {
		// Defensive: the processor must be wired in main.go; if it isn't we
		// ack anyway so a misconfiguration does not block the bridge.
		s.log.Warn("inbound processor not wired — acking without persist",
			slog.String("tenant_id", ev.TenantId),
		)
	}

	return &bridgepb.InboundAck{Ok: true}, nil
}

// formatInboundEvent renders an event for debug logging. Kept here so the
// same shape is reused by tests.
func formatInboundEvent(ev *bridgepb.InboundEvent) string {
	return fmt.Sprintf("InboundEvent{tenant=%s phone=%s type=%s id=%s}",
		ev.TenantId, ev.Phone, ev.MessageType, ev.ProviderMessageId)
}

// inboundProcessorDeps groups the api-side collaborators needed to process an
// inbound event end to end. Same composition as cmd/api/inbound.go, but
// driven by the gRPC transport rather than the legacy JSON HTTP webhook.
type inboundProcessorDeps struct {
	Queries     *db.Queries
	MessagesSvc messages.Service
	LeadsSvc    leads.Service
	Log         *slog.Logger
}

// inboundProcessor is the transport-neutral inbound pipeline. Both
// ReportInbound (gRPC) and the legacy /v1/inbound HTTP handler compose
// through this same method so the persist+lead-upsert+touch logic lives
// in exactly one place.
//
// Phase 1b fills in the real implementation. The constructor exists now so
// the gRPC server boots; in the meantime ReportInbound will still ack but
// the proc pointer will be set, so the path is exercised.
type inboundProcessor struct {
	deps inboundProcessorDeps
}

func newInboundProcessor(d inboundProcessorDeps) *inboundProcessor {
	return &inboundProcessor{deps: d}
}

// Process persists an inbound event: looks up the session, validates tenant
// ownership, persists the message idempotently, upserts the lead, and
// bumps the lead's last_inbound_at. Implementation lives in
// inbound_processor.go (Phase 1b).
func (p *inboundProcessor) Process(ctx context.Context, ev *bridgepb.InboundEvent) error {
	if p == nil {
		return fmt.Errorf("inboundProcessor: nil receiver")
	}
	// Phase 1b implementation lives in inbound_processor.go.
	return p.process(ctx, ev)
}