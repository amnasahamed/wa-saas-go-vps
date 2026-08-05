package main

import (
	"context"

	"go.mau.fi/whatsmeow/types/events"

	bridgepb "github.com/amnasahamed/niosengine/internal/bridgepb"
	"github.com/amnasahamed/niosengine/internal/whatsapp"
)

// inboundReporter adapts the existing apiClient (which talks gRPC to the
// orchestrator) to the whatsapp.InboundReporter interface that the
// manager calls from its whatsmeow event handler.
//
// The reporter is installed once at boot via mgr.SetInboundReporter. It
// is goroutine-safe: the manager dispatches one event per goroutine per
// session, and the underlying gRPC client handles its own concurrency.
type inboundReporter struct {
	api *apiClient
}

// newInboundReporter wraps an apiClient.
func newInboundReporter(api *apiClient) *inboundReporter {
	return &inboundReporter{api: api}
}

// ReportInbound satisfies whatsapp.InboundReporter.
//
// _ is the original whatsmeow event, kept for future logging; the
// proto-shaped `ev` is what we actually send.
func (r *inboundReporter) ReportInbound(ctx context.Context, _ *whatsapp.Session, _ *events.Message, ev *bridgepb.InboundEvent) error {
	if r.api == nil {
		return nil
	}
	_, err := r.api.ReportInboundWithRetry(ctx, ev)
	return err
}
