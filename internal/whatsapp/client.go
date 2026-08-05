package whatsapp

import (
	"context"
	"errors"
	"sync"
	"time"
)

// Session represents one tenant's connected WhatsApp account.
// whatsmeow stores its own device state on disk; Session is our wrapper.
type Session struct {
	TenantID    string
	ID          string // uuid
	PhoneNumber string // assigned after QR link succeeds
	PushName    string
	Status      string // "pending_qr" | "qr_required" | "authenticated" | "ready" | "disconnected" | "error"
	WorkerID    string // bridge worker that owns this session
	LastEventAt time.Time
	CreatedAt   time.Time
}

// Message is the inbound/outbound message envelope passed between bridge and API.
//
// The bridge constructs this from whatsmeow events and POSTs it to the API.
// The API uses this same type when persisting to the messages table.
type Message struct {
	TenantID          string
	WhatsAppSessionID string
	ProviderMessageID string // whatsmeow message ID (idempotency anchor)
	Phone             string // normalized
	Direction         string // "inbound" | "outbound"
	Sender            string // "customer" | "ai" | "counselor"
	MessageType       string // "text" | "image" | "audio" | "video" | "document" | "sticker"
	Body              string
	MediaS3Key        string // empty for text-only
	MediaMIME         string
	Timestamp         time.Time
	IdempotencyKey    string // tenantID + providerMessageID
}

// SenderClassify classifies a message as customer/ai/counselor based on
// whether we just sent it (ai) or the linked phone sent it manually (counselor).
//
// Ported from whatsapp-bridge/index.js:500-505. The bridge keeps an in-memory
// map of recently-sent message IDs; we mirror that pattern here so messages
// flowing back through `message_create` after our `/send` calls don't
// re-trigger the extraction pipeline (the reply-loop bug in the legacy
// n8n setup).
type SenderClassify struct {
	mu              sync.Mutex
	recentlySentIDs map[string]time.Time
	ttl             time.Duration
}

// NewSenderClassify returns a SenderClassify with the given TTL for the
// "recently sent" window. whatsmeow events for our own sent messages
// typically arrive within a few seconds; 10 minutes is safe.
func NewSenderClassify(ttl time.Duration) *SenderClassify {
	return &SenderClassify{
		recentlySentIDs: make(map[string]time.Time),
		ttl:             ttl,
	}
}

// MarkSent records that we just sent a message with the given provider ID.
func (s *SenderClassify) MarkSent(providerMessageID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.recentlySentIDs[providerMessageID] = time.Now()
}

// Classify returns the sender classification for a message.
// Caller should pass msg.FromMe and msg.ID.
func (s *SenderClassify) Classify(fromMe bool, providerMessageID string) string {
	if !fromMe {
		return "customer"
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.recentlySentIDs[providerMessageID]; ok {
		return "ai"
	}
	return "counselor"
}

// prune removes expired entries. Call periodically.
func (s *SenderClassify) prune() {
	s.mu.Lock()
	defer s.mu.Unlock()
	cutoff := time.Now().Add(-s.ttl)
	for id, ts := range s.recentlySentIDs {
		if ts.Before(cutoff) {
			delete(s.recentlySentIDs, id)
		}
	}
}

// Client wraps a whatsmeow.Client with our multi-tenant session management.
// Each tenant has exactly one Client. The Client's underlying *store.Device
// (SQLite or postgres backend) is loaded from per-tenant S3-backed state.
type Client struct {
	SessionID string
	TenantID  string
	// whatsmeowClient is *whatsmeow.Client (imported in Phase 1).
	// We keep it as `any` here so this scaffold compiles without the
	// dependency wired up yet.
	whatsmeowClient any
}

// SendOptions configures an outbound WhatsApp message.
type SendOptions struct {
	Phone            string
	Body             string
	Caption          string
	MediaS3Key       string
	MIME             string
	Filename         string
	SendAsVoice      bool
	SendAsDocument   bool
	SeparateMessages bool   // text + media as separate messages
	ClientMsgID      string // optional dedup key
}

// Manager is the high-level interface for the bridge pool.
//
// One Manager per bridge process. It owns the lifecycle of all whatsmeow
// clients assigned to this worker (sessions where worker_id = self).
type Manager interface {
	// Start boots the manager: loads assigned sessions, reconnects.
	Start(ctx context.Context) error

	// Stop gracefully disconnects all sessions and flushes state.
	Stop(ctx context.Context) error

	// AddSession starts a new session for a tenant (called after tenant
	// requests a QR link from the dashboard).
	AddSession(ctx context.Context, tenantID string) (*Session, error)

	// RemoveSession disconnects and forgets a session.
	RemoveSession(ctx context.Context, sessionID string) error

	// GetSession returns a session by ID.
	GetSession(ctx context.Context, sessionID string) (*Session, error)

	// GetSessionByTenant returns the session currently held in memory for
	// the given tenant. Returns ErrSessionNotFound if this worker does
	// not own the tenant's session (multi-bridge case).
	GetSessionByTenant(ctx context.Context, tenantID string) (*Session, error)

	// ListSessions returns all sessions owned by this worker.
	ListSessions(ctx context.Context) ([]*Session, error)

	// SendMessage sends an outbound message via the given session.
	SendMessage(ctx context.Context, sessionID string, opts SendOptions) (*Message, error)

	// HasAuthenticatedSession reports whether at least one session is
	// currently connected and authenticated. Used by /readyz.
	HasAuthenticatedSession() bool

	// SetInboundReporter installs the bridge-side hook called from the
	// whatsmeow event handler when an *events.Message arrives. Idempotent;
	// must be called once at boot, before Start. nil clears the reporter
	// (subsequent messages will be logged only).
	SetInboundReporter(InboundReporter)

	// RegisterQRSubscriber attaches a new QRChannel subscriber. The
	// returned cancel function unsubscribes and closes the channel. The
	// manager replays the most recent QR to a new subscriber.
	RegisterQRSubscriber(sessionID string, ch QRChannel) (func(), error)
}

// ErrSessionNotFound is returned when a session ID has no matching client.
var ErrSessionNotFound = errors.New("session not found")

// ErrSessionNotReady is returned when trying to send before authentication.
var ErrSessionNotReady = errors.New("whatsapp session not ready")
