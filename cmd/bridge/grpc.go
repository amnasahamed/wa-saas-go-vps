package main

import (
	"context"
	"errors"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	bridgepb "github.com/amnasahamed/niosengine/internal/bridgepb"
	"github.com/amnasahamed/niosengine/internal/whatsapp"
)

// bridgeService implements bridgepb.APIToBridgeServer.
//
// The orchestrator (cmd/api) calls these RPCs to send outbound messages and
// stream QR events while a tenant is linking WhatsApp.
type bridgeService struct {
	bridgepb.UnimplementedAPIToBridgeServer

	manager whatsapp.Manager
}

func newBridgeService(m whatsapp.Manager) *bridgeService {
	return &bridgeService{manager: m}
}

// SendMessage forwards the outbound message to the assigned whatsmeow client.
//
// The orchestrator only knows the (tenantID, sessionID) pair; the bridge
// worker owns the actual whatsmeow handle.
func (s *bridgeService) SendMessage(ctx context.Context, req *bridgepb.SendMessageRequest) (*bridgepb.SendMessageResponse, error) {
	if req.TenantId == "" || req.WhatsappSessionId == "" {
		return nil, status.Error(codes.InvalidArgument, "tenant_id and whatsapp_session_id are required")
	}
	if req.Phone == "" {
		return nil, status.Error(codes.InvalidArgument, "phone is required")
	}

	sent, err := s.manager.SendMessage(ctx, req.WhatsappSessionId, whatsapp.SendOptions{
		Phone:          req.Phone,
		Body:           req.Body,
		Caption:        req.Caption,
		MediaS3Key:     req.MediaS3Key,
		MIME:           req.MediaMimetype,
		Filename:       req.Filename,
		SendAsVoice:    req.SendAsVoice,
		SendAsDocument: req.SendAsDocument,
		ClientMsgID:    req.ClientMsgId,
	})
	if err != nil {
		if errors.Is(err, whatsapp.ErrSessionNotFound) {
			return nil, status.Error(codes.NotFound, "session not found")
		}
		if errors.Is(err, whatsapp.ErrSessionNotReady) {
			return nil, status.Error(codes.FailedPrecondition, "session not authenticated")
		}
		return nil, status.Errorf(codes.Internal, "send: %v", err)
	}

	return &bridgepb.SendMessageResponse{
		Ok:                true,
		ProviderMessageId: sent.ProviderMessageID,
	}, nil
}

// StreamQR streams QR events for the requested session until authentication
// succeeds or the context is cancelled.
//
// Real whatsmeow QR events are pushed to registered QRChannels from the
// manager's event handler. We wrap a gRPC ServerStream in a QRChannel
// adapter and let the manager do the dispatch. The stream ends naturally
// when Connected/LoggedOut closes the subscribers, or when the client
// disconnects (stream context cancellation).
func (s *bridgeService) StreamQR(req *bridgepb.StreamQRRequest, stream grpc.ServerStreamingServer[bridgepb.QREvent]) error {
	if req.WhatsappSessionId == "" {
		return status.Error(codes.InvalidArgument, "whatsapp_session_id is required")
	}

	// Verify the session exists before opening a long-lived stream.
	if _, err := s.manager.GetSession(stream.Context(), req.WhatsappSessionId); err != nil {
		if errors.Is(err, whatsapp.ErrSessionNotFound) {
			return status.Error(codes.NotFound, "session not found")
		}
		return status.Errorf(codes.Internal, "get session: %v", err)
	}

	ch := newStreamQRChannel(stream)
	cancelSub, err := s.manager.RegisterQRSubscriber(req.WhatsappSessionId, ch)
	if err != nil {
		return status.Errorf(codes.Internal, "subscribe: %v", err)
	}
	defer cancelSub()

	<-stream.Context().Done()
	return nil
}

// StartLink starts a new QR link for the given tenant and waits for the
// first event. Returns the LinkResult with authenticated=true once the
// whatsmeow client reaches the authenticated state, or false on cancel.
//
// Idempotent: if a session already exists for this tenant, we return it
// rather than spinning up a second whatsmeow client. The API's linking
// flow expects this so a retried POST does not orphan device state.
func (s *bridgeService) StartLink(ctx context.Context, req *bridgepb.StreamQRRequest) (*bridgepb.LinkResult, error) {
	if req.TenantId == "" {
		return nil, status.Error(codes.InvalidArgument, "tenant_id is required")
	}
	// Idempotency: if we already have a session for this tenant, return
	// it. The DB row lookup happens after the in-memory check because
	// AddSession is also idempotent at the SQL level (ON CONFLICT) and
	// would return the existing row either way — but returning the
	// in-memory session avoids creating a fresh whatsmeow client.
	if existing, err := s.manager.GetSessionByTenant(ctx, req.TenantId); err == nil {
		return &bridgepb.LinkResult{
			Authenticated: existing.Status == "authenticated" || existing.Status == "ready",
			PhoneNumber:   existing.PhoneNumber,
			PushName:      existing.PushName,
		}, nil
	}
	sess, err := s.manager.AddSession(ctx, req.TenantId)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "add session: %v", err)
	}
	return &bridgepb.LinkResult{
		Authenticated: sess.Status == "authenticated" || sess.Status == "ready",
		PhoneNumber:   sess.PhoneNumber,
		PushName:      sess.PushName,
	}, nil
}