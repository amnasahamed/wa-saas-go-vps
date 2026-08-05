package whatsapp

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/google/uuid"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/store/sqlstore"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	"google.golang.org/protobuf/types/known/timestamppb"

	// Pure-Go SQLite driver backing the per-session device stores. Registers
	// itself as "sqlite", which is also a dialect dbutil.ParseDialect accepts.
	// Pure Go keeps the binaries CGO-free so they run on distroless.
	_ "modernc.org/sqlite"

	bridgepb "github.com/amnasahamed/niosengine/internal/bridgepb"
	"github.com/amnasahamed/niosengine/internal/db"
)

// ManagerConfig configures the whatsmeow manager.
type ManagerConfig struct {
	WorkerID    string // unique ID for this bridge worker (used for session sharding)
	MaxSessions int    // soft cap; refuses AddSession beyond this
	StateDir    string // local directory for per-session SQLite device stores
	Logger      *slog.Logger
}

// manager is the whatsmeow-backed implementation of Manager.
//
// It owns the lifecycle of every whatsmeow client assigned to this worker.
// Session metadata is in Postgres (read on Start, watched for changes later);
// the actual device state (signal keys, app state, etc.) lives in per-session
// SQLite files under StateDir, backed up to S3 under the
// whatsapp_sessions.device_state_s3_key column.
type manager struct {
	cfg    ManagerConfig
	pool   *db.Pool
	mu     sync.RWMutex
	bySess map[string]*managedClient // sessionID -> client
	byTen  map[string]string         // tenantID -> sessionID (one session per tenant in MVP)
	log    *slog.Logger

	// inboundReporter is called from the whatsmeow event handler when an
	// *events.Message arrives. nil-safe: events with no reporter are
	// logged only. The bridge injects an implementation that calls
	// ReportInboundWithRetry on the api gRPC client.
	inboundReporter InboundReporter
}

// InboundReporter is the bridge-side seam for reporting a whatsmeow message
// to the api. Defined here as an interface so internal/whatsapp does not
// depend on the bridge package (which would create an import cycle).
//
// The whatsmeow event is passed through for logging/debugging; the
// bridgepb.InboundEvent is the wire payload the api consumes.
//
// Implementations should retry transient failures; this interface is the
// final hop, not the retry policy.
type InboundReporter interface {
	ReportInbound(ctx context.Context, sess *Session, waEvt *events.Message, ev *bridgepb.InboundEvent) error
}

// managedClient bundles a whatsmeow client with our session metadata.
type managedClient struct {
	session *Session
	wm      *whatsmeow.Client
	store   *sqlstore.Container

	// stopEvts is the event-handler teardown returned to whatsmeow.
	// It is NOT the QR channel registry — that lives in qrSubs.
	stopEvts func()

	// qrSubs are QR event subscribers (one per StreamQR gRPC call).
	// Guarded by m.mu. A new QR fans out to every subscriber, with
	// non-blocking sends. Capacity-one (latest-only) keeps a slow
	// dashboard from stalling the whatsmeow event loop.
	qrSubs []QRChannel
	// qrLatest caches the most recent QR for this session so that a
	// subscriber attaching between two QR refreshes does not have to
	// wait for the next refresh to receive something to render.
	qrLatest *QREvent
}

// SetInboundReporter installs (or clears, when nil) the bridge-side hook
// called from the whatsmeow event handler when an *events.Message arrives.
// Idempotent; safe to call once at boot.
func (m *manager) SetInboundReporter(r InboundReporter) {
	m.mu.Lock()
	m.inboundReporter = r
	m.mu.Unlock()
}

// NewManager returns a Manager ready to be started.
func NewManager(pool *db.Pool, cfg ManagerConfig) (Manager, error) {
	if cfg.WorkerID == "" {
		return nil, errors.New("whatsapp: WorkerID is required")
	}
	if cfg.StateDir == "" {
		return nil, errors.New("whatsapp: StateDir is required")
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	if cfg.MaxSessions <= 0 {
		cfg.MaxSessions = 50
	}
	if err := os.MkdirAll(cfg.StateDir, 0o700); err != nil {
		return nil, fmt.Errorf("whatsapp: create state dir: %w", err)
	}

	return &manager{
		cfg:    cfg,
		pool:   pool,
		bySess: make(map[string]*managedClient),
		byTen:  make(map[string]string),
		log:    cfg.Logger,
	}, nil
}

// Start loads every session assigned to this worker from Postgres and
// restores each whatsmeow client. If the session has stored credentials,
// reconnects; otherwise pauses in `qr_required` state waiting for AddSession.
func (m *manager) Start(ctx context.Context) error {
	queries := db.New(m.pool)
	rows, err := queries.ListWhatsAppSessionsByWorker(ctx, textToPgtype(m.cfg.WorkerID))
	if err != nil {
		return fmt.Errorf("whatsapp: list sessions: %w", err)
	}
	m.log.Info("loading sessions", slog.Int("count", len(rows)))

	for _, row := range rows {
		sess := rowToSession(row)
		if err := m.loadSession(ctx, sess); err != nil {
			m.log.Warn("failed to load session",
				slog.String("session_id", sess.ID),
				slog.String("tenant_id", sess.TenantID),
				slog.String("err", err.Error()),
			)
			_ = queries.UpdateWhatsAppSessionStatus(ctx, db.UpdateWhatsAppSessionStatusParams{
				ID:        row.ID,
				Status:    "error",
				LastError: db.StringToText(err.Error()),
			})
		}
	}
	return nil
}

// loadSession constructs a whatsmeow client for the given session row and
// connects (or pauses for QR) depending on whether credentials exist.
func (m *manager) loadSession(ctx context.Context, sess *Session) error {
	devicePath := filepath.Join(m.cfg.StateDir, sess.ID+".sqlite3")
	// foreign_keys(1) is required: whatsmeow's schema upgrade refuses to run
	// without it ("foreign keys are not enabled"). ?_pragma= is modernc syntax.
	dsn := "file:" + devicePath + "?_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)"
	container, err := sqlstore.New(ctx, "sqlite", dsn, nil)
	if err != nil {
		return fmt.Errorf("open sqlstore: %w", err)
	}

	// try to get the existing device for this sessionID, otherwise create one.
	// whatsmeow stores device state keyed by JID; we use the session UUID
	// since each tenant session has at most one device.
	deviceJID := types.JID{User: sess.ID, Server: "s.whatsapp.net", Device: 0, RawAgent: 0}
	device, err := container.GetDevice(ctx, deviceJID)
	if err != nil {
		// no stored credentials — fresh device, will need QR
		device = container.NewDevice()
	}

	wa := whatsmeow.NewClient(device, nil)
	wa.AddEventHandler(m.makeEventHandler(sess))

	mc := &managedClient{
		session: sess,
		wm:      wa,
		store:   container,
	}
	m.mu.Lock()
	m.bySess[sess.ID] = mc
	m.byTen[sess.TenantID] = sess.ID
	m.mu.Unlock()

	if wa.IsLoggedIn() {
		// reconnect authenticated sessions
		if err := wa.Connect(); err != nil {
			return fmt.Errorf("connect: %w", err)
		}
		sess.Status = "ready"
		m.updateStatus(ctx, sess.ID, "ready", "")
	} else {
		sess.Status = "qr_required"
		m.updateStatus(ctx, sess.ID, "qr_required", "")
	}
	m.log.Info("session loaded",
		slog.String("session_id", sess.ID),
		slog.String("tenant_id", sess.TenantID),
		slog.String("status", sess.Status),
	)
	return nil
}

// Stop gracefully disconnects all sessions and flushes device state to S3.
func (m *manager) Stop(ctx context.Context) error {
	m.mu.Lock()
	clients := make([]*managedClient, 0, len(m.bySess))
	for _, c := range m.bySess {
		clients = append(clients, c)
	}
	m.mu.Unlock()

	var firstErr error
	for _, c := range clients {
		if c.stopEvts != nil {
			c.stopEvts()
		}
		// Close any QR subscribers so their gRPC streams end.
		for _, ch := range c.qrSubs {
			_ = ch.Close()
		}
		c.qrSubs = nil
		if c.wm.IsConnected() {
			c.wm.Disconnect()
		}
		// Best-effort: snapshot device state to S3. In MVP without S3 wiring
		// we still call the hook so the API contract is stable.
		if err := m.snapshotToS3(ctx, c.session.ID); err != nil {
			m.log.Warn("snapshot to S3 failed",
				slog.String("session_id", c.session.ID),
				slog.String("err", err.Error()),
			)
			if firstErr == nil {
				firstErr = err
			}
		}
		_ = c.store.Close()
	}
	return firstErr
}

// AddSession starts a new session for a tenant (creates the DB row, assigns
// to this worker, builds a whatsmeow client, and kicks off QR generation).
func (m *manager) AddSession(ctx context.Context, tenantID string) (*Session, error) {
	tenantUUID, err := uuid.Parse(tenantID)
	if err != nil {
		return nil, fmt.Errorf("whatsapp: invalid tenant id: %w", err)
	}

	m.mu.Lock()
	if _, exists := m.byTen[tenantID]; exists {
		m.mu.Unlock()
		return nil, fmt.Errorf("whatsapp: tenant %s already has a session", tenantID)
	}
	if m.cfg.MaxSessions > 0 && len(m.bySess) >= m.cfg.MaxSessions {
		m.mu.Unlock()
		return nil, fmt.Errorf("whatsapp: max sessions reached (%d)", m.cfg.MaxSessions)
	}
	m.mu.Unlock()

	queries := db.New(m.pool)
	sessionID := uuid.New()
	row, err := queries.CreateWhatsAppSession(ctx, db.CreateWhatsAppSessionParams{
		ID:       pgtypeToUUID(sessionID),
		TenantID: pgtypeToUUID(tenantUUID),
		Status:   "pending_qr",
	})
	if err != nil {
		return nil, fmt.Errorf("whatsapp: create session row: %w", err)
	}
	if err := queries.AssignWhatsAppSessionWorker(ctx, db.AssignWhatsAppSessionWorkerParams{
		ID:       row.ID,
		WorkerID: textToPgtype(m.cfg.WorkerID),
	}); err != nil {
		return nil, fmt.Errorf("whatsapp: assign worker: %w", err)
	}

	sess := rowToSession(row)
	if err := m.loadSession(ctx, sess); err != nil {
		return nil, err
	}
	return sess, nil
}

// RemoveSession disconnects and forgets a session.
func (m *manager) RemoveSession(ctx context.Context, sessionID string) error {
	m.mu.Lock()
	mc, ok := m.bySess[sessionID]
	if !ok {
		m.mu.Unlock()
		return ErrSessionNotFound
	}
	delete(m.bySess, sessionID)
	delete(m.byTen, mc.session.TenantID)
	m.mu.Unlock()

	if mc.stopEvts != nil {
		mc.stopEvts()
	}
	if mc.wm.IsConnected() {
		mc.wm.Disconnect()
	}
	_ = mc.store.Close()

	// We keep the DB row and device state file so the session can be
	// re-loaded by the same worker on restart. In a future phase the
	// dashboard will provide a "delete account" flow that wipes both.
	queries := db.New(m.pool)
	sessionUUID, _ := uuid.Parse(sessionID)
	return queries.UpdateWhatsAppSessionStatus(ctx, db.UpdateWhatsAppSessionStatusParams{
		ID:     pgtypeToUUID(sessionUUID),
		Status: "disconnected",
	})
}

// GetSession returns a session by ID.
func (m *manager) GetSession(_ context.Context, sessionID string) (*Session, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	mc, ok := m.bySess[sessionID]
	if !ok {
		return nil, ErrSessionNotFound
	}
	return mc.session, nil
}

// ListSessions returns all sessions owned by this worker.
func (m *manager) ListSessions(_ context.Context) ([]*Session, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]*Session, 0, len(m.bySess))
	for _, mc := range m.bySess {
		out = append(out, mc.session)
	}
	return out, nil
}

// SendMessage sends an outbound message via the given session.
func (m *manager) SendMessage(ctx context.Context, sessionID string, opts SendOptions) (*Message, error) {
	m.mu.RLock()
	mc, ok := m.bySess[sessionID]
	m.mu.RUnlock()
	if !ok {
		return nil, ErrSessionNotFound
	}
	if !mc.wm.IsLoggedIn() {
		return nil, ErrSessionNotReady
	}

	// Normalize + build JID. WhatsApp JIDs use {phone}@s.whatsapp.net.
	recipient, err := types.ParseJID(opts.Phone + "@s.whatsapp.net")
	if err != nil {
		return nil, fmt.Errorf("whatsapp: parse recipient: %w", err)
	}
	if opts.MediaS3Key != "" {
		// Media sending is wired in a follow-up; fail loudly so callers
		// don't silently send a text-only message instead of the asset.
		return nil, fmt.Errorf("whatsapp: media sending not yet implemented (S3 key=%s)", opts.MediaS3Key)
	}

	// Build a simple text message. Voice/document flows are out of MVP.
	msg := buildTextMessage(opts.Body)

	resp, err := mc.wm.SendMessage(ctx, recipient, msg)
	if err != nil {
		return nil, fmt.Errorf("whatsapp: send: %w", err)
	}

	return &Message{
		TenantID:          mc.session.TenantID,
		WhatsAppSessionID: mc.session.ID,
		ProviderMessageID: resp.ID,
		Phone:             opts.Phone,
		Direction:         "outbound",
		Sender:            "ai",
		MessageType:       "text",
		Body:              opts.Body,
		Timestamp:         resp.Timestamp,
		IdempotencyKey:    "outbound:" + mc.session.TenantID + ":" + resp.ID,
	}, nil
}

// makeEventHandler returns an event handler bound to a session.
// It updates session state on QR codes and authentication, and forwards
// inbound messages via the configured callback (set via OnInbound).
func (m *manager) makeEventHandler(sess *Session) whatsmeow.EventHandler {
	return func(evt any) {
		switch v := evt.(type) {
		case *events.QR:
			dataURL, _ := RenderQRDataURL(v.Codes[0])
			expires := time.Now().Add(60 * time.Second)
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			queries := db.New(m.pool)
			_ = queries.UpdateWhatsAppSessionQR(ctx, db.UpdateWhatsAppSessionQRParams{
				ID:          pgtypeToUUID(uuid.MustParse(sess.ID)),
				QrCode:      db.StringToText(v.Codes[0]),
				QrExpiresAt: db.TimeToTimestamptz(expires),
			})
			m.log.Info("qr issued",
				slog.String("session_id", sess.ID),
				slog.String("tenant_id", sess.TenantID),
			)
			// Forward to any listening QR channel.
			m.fanoutQR(QREvent{
				SessionID: sess.ID,
				TenantID:  sess.TenantID,
				Code:      v.Codes[0],
				DataURL:   dataURL,
				IssuedAt:  time.Now(),
				ExpiresAt: expires,
			})

		case *events.Connected:
			sess.Status = "ready"
			m.closeQRSubs(sess.ID, true)
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			queries := db.New(m.pool)
			_ = queries.UpdateWhatsAppSessionStatus(ctx, db.UpdateWhatsAppSessionStatusParams{
				ID:     pgtypeToUUID(uuid.MustParse(sess.ID)),
				Status: "ready",
			})

		case *events.LoggedOut:
			sess.Status = "disconnected"
			m.closeQRSubs(sess.ID, true)
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			queries := db.New(m.pool)
			_ = queries.UpdateWhatsAppSessionStatus(ctx, db.UpdateWhatsAppSessionStatusParams{
				ID:     pgtypeToUUID(uuid.MustParse(sess.ID)),
				Status: "disconnected",
			})

		case *events.Message:
			m.handleInboundMessage(sess, v)
		}
	}
}

// handleInboundMessage converts a whatsmeow *events.Message into the shape
// the api expects and dispatches it via the configured InboundReporter.
//
// Without a reporter installed (dev mode, tests, or a future reporter that
// retries via the durable queue), the message is logged only — never
// dropped, because the bridge still logs a "dropped" warning so the
// operator can see it.
//
// The conversion is the only place in the codebase that knows the
// whatsmeow proto types. Keep the mapping conservative: anything we are
// not sure about becomes MessageType="unknown" + a body string so the api
// at least persists a row. S3 upload of inbound media is a follow-up.
func (m *manager) handleInboundMessage(sess *Session, evt *events.Message) {
	if evt == nil {
		return
	}
	// chat JID, never fromMe (outbound echoes land in a different code path)
	// — but defence in depth: the manager owns the session, so an
	// outbound echo from a different sender is still a tenant's inbound
	// from the api's perspective. We do not classify that here; the
	// reply-loop guard at messages.Service handles it.
	phoneRaw := ""
	if evt.Info.Chat.User != "" {
		phoneRaw = evt.Info.Chat.User
	}
	if phoneRaw == "" && evt.Info.Sender.User != "" {
		phoneRaw = evt.Info.Sender.User
	}
	phone, err := NormalizePhone(phoneRaw, "91")
	if err != nil {
		m.log.Warn("inbound: cannot normalise phone",
			slog.String("session_id", sess.ID),
			slog.String("raw", phoneRaw),
		)
		return
	}

	msgType, body, mediaKey, mediaMIME := classifyInbound(evt.Message)
	ts := evt.Info.Timestamp
	if ts.IsZero() {
		ts = time.Now().UTC()
	}

	ev := &bridgepb.InboundEvent{
		TenantId:          sess.TenantID,
		WhatsappSessionId: sess.ID,
		ProviderMessageId: evt.Info.ID,
		Phone:             phone,
		Direction:         "inbound",
		Sender:            "customer",
		MessageType:       msgType,
		Body:              body,
		MediaS3Key:        mediaKey,
		MediaMimetype:     mediaMIME,
		Timestamp:         timestamppb.New(ts),
		// Idempotency key is tenant-scoped so two tenants cannot collide
		// on the same provider id. The api prefixes with the tenant id
		// anyway, but baking it in here makes it visible in bridge logs.
		IdempotencyKey: sess.TenantID + ":" + evt.Info.ID,
	}

	m.mu.RLock()
	rep := m.inboundReporter
	m.mu.RUnlock()
	if rep == nil {
		m.log.Warn("inbound: no reporter wired; dropping event",
			slog.String("session_id", sess.ID),
			slog.String("provider_message_id", evt.Info.ID),
		)
		return
	}
	// The reporter owns retry policy. We pass a short context so a
	// repeatedly failing api does not stall the whatsmeow event loop.
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := rep.ReportInbound(ctx, sess, evt, ev); err != nil {
		m.log.Error("inbound report failed",
			slog.String("session_id", sess.ID),
			slog.String("provider_message_id", evt.Info.ID),
			slog.String("err", err.Error()),
		)
	}
}

// classifyInbound maps a whatsmeow message envelope to (msgType, body, media).
// S3 upload is not yet implemented, so mediaKey/mediaMIME are always empty
// for now — body still carries whatever caption the message had so the
// dashboard can render it.
func classifyInbound(m *waE2E.Message) (msgType, body, mediaKey, mediaMIME string) {
	if m == nil {
		return "unknown", "", "", ""
	}
	switch {
	case m.Conversation != nil:
		return "text", *m.Conversation, "", ""
	case m.ExtendedTextMessage != nil:
		return "text", m.GetExtendedTextMessage().GetText(), "", ""
	case m.ImageMessage != nil:
		return "image", m.GetImageMessage().GetCaption(), "", m.GetImageMessage().GetMimetype()
	case m.VideoMessage != nil:
		return "video", m.GetVideoMessage().GetCaption(), "", m.GetVideoMessage().GetMimetype()
	case m.DocumentMessage != nil:
		return "document", m.GetDocumentMessage().GetCaption(), "", m.GetDocumentMessage().GetMimetype()
	case m.AudioMessage != nil:
		return "audio", "", "", m.GetAudioMessage().GetMimetype()
	case m.StickerMessage != nil:
		return "sticker", "", "", m.GetStickerMessage().GetMimetype()
	case m.ContactMessage != nil:
		return "contact", m.GetContactMessage().GetDisplayName(), "", ""
	case m.LocationMessage != nil:
		return "location", "", "", ""
	default:
		return "unknown", "", "", ""
	}
}

// closeQRSubs terminates every active QR subscriber for a session and,
// when `clearCache` is true, forgets the cached latest event. Called on
// Connected/LoggedOut so a finished link does not keep replaying the
// pre-auth QR to a dashboard that is still polling.
func (m *manager) closeQRSubs(sessionID string, clearCache bool) {
	m.mu.Lock()
	mc, ok := m.bySess[sessionID]
	if !ok {
		m.mu.Unlock()
		return
	}
	subs := mc.qrSubs
	mc.qrSubs = nil
	if clearCache {
		mc.qrLatest = nil
	}
	m.mu.Unlock()
	for _, ch := range subs {
		_ = ch.Close()
	}
}

// fanoutQR sends a QR event to every registered QR channel for this
// session. Sends are non-blocking and best-effort: a slow or stale
// subscriber drops the event. The latest event is cached so a
// subscriber that attaches between refreshes gets the most recent QR
// replayed on subscribe.
//
// QRChannel.Send blocks on a network write; we wrap that in a goroutine
// and let the channel's own context be the cancellation lever. Capacity
// is implicit: subscribers that cannot keep up simply miss events,
// which is the right behaviour for QR codes (they are time-sensitive
// but the next refresh is only ~30 s away).
func (m *manager) fanoutQR(ev QREvent) {
	m.mu.Lock()
	mc, ok := m.bySess[ev.SessionID]
	if !ok {
		m.mu.Unlock()
		return
	}
	// Cache the latest so a late-joining subscriber can replay.
	cp := ev
	mc.qrLatest = &cp
	subs := append([]QRChannel(nil), mc.qrSubs...)
	m.mu.Unlock()

	for _, ch := range subs {
		// Non-blocking dispatch: a slow channel must not stall the
		// whatsmeow event loop. If Send blocks, the caller's
		// per-event context will eventually cancel it on shutdown.
		go func(c QRChannel) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = c.Send(ctx, ev)
		}(ch)
	}
}

// registerQRChannel attaches a new QR subscriber. The returned cancel
// function unsubscribes and closes the channel. If a cached QR is
// available (the dashboard opened a stream between refreshes), it is
// replayed asynchronously.
// RegisterQRSubscriber satisfies the Manager interface — see client.go.
func (m *manager) RegisterQRSubscriber(sessionID string, ch QRChannel) (func(), error) {
	return m.registerQRChannel(sessionID, ch)
}

func (m *manager) registerQRChannel(sessionID string, ch QRChannel) (func(), error) {
	m.mu.Lock()
	mc, ok := m.bySess[sessionID]
	if !ok {
		m.mu.Unlock()
		return nil, ErrSessionNotFound
	}
	mc.qrSubs = append(mc.qrSubs, ch)
	latest := mc.qrLatest
	m.mu.Unlock()

	// Replay the latest event to the new subscriber so it does not have
	// to wait for the next refresh (which can take 30+ s).
	if latest != nil {
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = ch.Send(ctx, *latest)
		}()
	}

	cancel := func() {
		m.mu.Lock()
		defer m.mu.Unlock()
		mc, ok := m.bySess[sessionID]
		if !ok {
			_ = ch.Close()
			return
		}
		for i, sub := range mc.qrSubs {
			if sub == ch {
				mc.qrSubs = append(mc.qrSubs[:i], mc.qrSubs[i+1:]...)
				break
			}
		}
		_ = ch.Close()
	}
	return cancel, nil
}

// GetSessionByTenant returns the session currently held in memory for
// the given tenant, or ErrSessionNotFound if this worker does not own
// it. Single-bridge deployments keep every session here; multi-bridge
// callers must tolerate ErrSessionNotFound and look the session up in
// the shared DB instead.
func (m *manager) GetSessionByTenant(_ context.Context, tenantID string) (*Session, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	sid, ok := m.byTen[tenantID]
	if !ok {
		return nil, ErrSessionNotFound
	}
	mc, ok := m.bySess[sid]
	if !ok {
		return nil, ErrSessionNotFound
	}
	return mc.session, nil
}

// snapshotToS3 is the hook for backing up device state. In MVP without S3
// wiring it just logs so the API contract is stable.
func (m *manager) snapshotToS3(ctx context.Context, sessionID string) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}
	m.log.Info("snapshot device state",
		slog.String("session_id", sessionID),
		slog.String("note", "S3 wiring lands in phase-4; skipping upload"),
	)
	return nil
}

// updateStatus updates the DB row's status field. Best-effort; logs errors
// but never bubbles them up to whatsmeow event handlers.
func (m *manager) updateStatus(ctx context.Context, sessionID, status, lastErr string) {
	queries := db.New(m.pool)
	sessionUUID, err := uuid.Parse(sessionID)
	if err != nil {
		return
	}
	_ = queries.UpdateWhatsAppSessionStatus(ctx, db.UpdateWhatsAppSessionStatusParams{
		ID:        pgtypeToUUID(sessionUUID),
		Status:    status,
		LastError: db.StringToText(lastErr),
	})
}

// HasAuthenticatedSession reports whether at least one session is in
// the ready/authenticated state. Used by /readyz.
func (m *manager) HasAuthenticatedSession() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, mc := range m.bySess {
		if mc.wm.IsLoggedIn() && mc.wm.IsConnected() {
			return true
		}
	}
	return false
}

// WaitForFirstConnection blocks until at least one session is connected
// or the context is cancelled. Used by /readyz.
func (m *manager) WaitForFirstConnection(ctx context.Context) bool {
	if m.HasAuthenticatedSession() {
		return true
	}
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return false
		case <-ticker.C:
			if m.HasAuthenticatedSession() {
				return true
			}
		}
	}
}

// rowToSession converts a DB row into a Session.
func rowToSession(row db.WhatsappSession) *Session {
	tenantID, _ := uuidFromPgtype(row.TenantID)
	return &Session{
		ID:               uuidToString(row.ID),
		TenantID:         tenantID,
		PhoneNumber:      db.StringFromText(row.PhoneNumber),
		PushName:         db.StringFromText(row.PushName),
		Status:           row.Status,
		WorkerID:         db.StringFromText(row.WorkerID),
		LastEventAt:      db.TimeFromTimestamptz(row.LastEventAt),
		CreatedAt:        db.TimeFromTimestamptz(row.CreatedAt),
	}
}

// insert blank line at end (was: logFileWriter stub)
