package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/amnasahamed/niosengine/internal/auth"
	"github.com/amnasahamed/niosengine/internal/db"
	"github.com/amnasahamed/niosengine/internal/leads"
	"github.com/amnasahamed/niosengine/internal/messages"
)

// inboundRequest is the JSON body the bridge POSTs to /v1/inbound.
//
// Shape mirrors the messages.Message domain type but is the wire format
// the bridge uses. The handler translates to our domain and persists.
type inboundRequest struct {
	ProviderMessageID string    `json:"provider_message_id"`
	Phone             string    `json:"phone"`
	Direction         string    `json:"direction"` // "inbound" | "outbound"
	Sender            string    `json:"sender"`    // "customer" | "ai" | "counselor"
	MessageType       string    `json:"message_type"`
	Body              string    `json:"body"`
	MediaS3Key        string    `json:"media_s3_key"`
	MediaMIME         string    `json:"media_mimetype"`
	Timestamp         time.Time `json:"timestamp"`
	IdempotencyKey    string    `json:"idempotency_key"`
}

type inboundResponse struct {
	OK        bool   `json:"ok"`
	MessageID string `json:"message_id"`
	LeadID    string `json:"lead_id"`
	Duplicate bool   `json:"duplicate"`
	Error     string `json:"error,omitempty"`
}

// inboundDeps holds the handler's collaborators.
type inboundDeps struct {
	APIKey   *auth.APIKeyValidator
	Messages messages.Service
	Leads    leads.Service
	Queries  *db.Queries // for whatsapp session lookup (no full repo yet)
	Log      *slog.Logger
}

// handleInbound is POST /v1/inbound.
//
// Auth: Authorization: Bearer <api_key>
// Body: inboundRequest JSON
//
// Side effects: persists message (idempotent), upserts lead, bumps
// lead.last_inbound_at. Extraction + nurture reply come in Phase 3.
func handleInbound(d *inboundDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		// 1) Auth
		tenantID, scope, err := d.APIKey.Validate(ctx, r.Header.Get("Authorization"))
		if err != nil {
			if errors.Is(err, auth.ErrInvalidCredentials) {
				writeJSON(w, http.StatusUnauthorized, inboundResponse{OK: false, Error: "invalid api key"})
				return
			}
			d.Log.Error("api key validation", slog.String("err", err.Error()))
			writeJSON(w, http.StatusInternalServerError, inboundResponse{OK: false, Error: "auth check failed"})
			return
		}
		if scope != "bridge" {
			writeJSON(w, http.StatusForbidden, inboundResponse{OK: false, Error: "wrong key scope (need 'bridge')"})
			return
		}

		// 2) Parse body
		var req inboundRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, inboundResponse{OK: false, Error: "invalid json: " + err.Error()})
			return
		}
		if req.ProviderMessageID == "" || req.IdempotencyKey == "" || req.Phone == "" {
			writeJSON(w, http.StatusBadRequest, inboundResponse{OK: false, Error: "provider_message_id, idempotency_key, phone are required"})
			return
		}
		if req.Direction != "inbound" && req.Direction != "outbound" {
			writeJSON(w, http.StatusBadRequest, inboundResponse{OK: false, Error: "direction must be 'inbound' or 'outbound'"})
			return
		}
		if req.MessageType == "" {
			req.MessageType = "text"
		}
		if req.Timestamp.IsZero() {
			req.Timestamp = time.Now().UTC()
		}

		// 3) Look up the WhatsApp session for this tenant. There's a UNIQUE
		// constraint on whatsapp_sessions.tenant_id, so this returns at
		// most one row. (Until we ship the agency plan.)
		sessionRow, err := d.Queries.GetWhatsAppSessionByTenant(ctx, db.PgtypeUUID(tenantID))
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				writeJSON(w, http.StatusNotFound, inboundResponse{OK: false, Error: "no whatsapp session for tenant"})
				return
			}
			d.Log.Error("lookup session", slog.String("err", err.Error()))
			writeJSON(w, http.StatusInternalServerError, inboundResponse{OK: false, Error: "session lookup failed"})
			return
		}
		sessionID, err := db.UUIDFromPgtype(sessionRow.ID)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, inboundResponse{OK: false, Error: "invalid session id"})
			return
		}

		// 4) Persist message (idempotent + reply-loop guarded)
		msg := &messages.Message{
			TenantID:          tenantID,
			WhatsAppSessionID: sessionID,
			ProviderMessageID: req.ProviderMessageID,
			Phone:             req.Phone,
			Direction:         req.Direction,
			Sender:            req.Sender,
			MessageType:       req.MessageType,
			Body:              req.Body,
			MediaS3Key:        req.MediaS3Key,
			MediaMIME:         req.MediaMIME,
			IdempotencyKey:    buildIdempotencyKey(tenantID, req.IdempotencyKey),
			Timestamp:         req.Timestamp,
		}

		// The Service returns a PersistInboundResult so we can report
		// the right thing to the bridge (replay vs fresh insert vs skip).
		result, err := d.Messages.PersistInboundWithStatus(ctx, msg)
		if err != nil {
			d.Log.Error("persist message", slog.String("err", err.Error()))
			writeJSON(w, http.StatusInternalServerError, inboundResponse{OK: false, Error: "persist failed: " + err.Error()})
			return
		}
		saved := result.Message
		duplicate := result.Duplicate

		// Reply-loop guard short-circuit: if the service skipped the
		// message (outbound+ai), don't upsert a lead or bump timestamps.
		// Just return success — the bridge gets the same shape either way.
		if result.Skipped {
			d.Log.Info("skipped outbound AI message (reply-loop guard)",
				slog.String("tenant_id", tenantID.String()),
				slog.String("phone", req.Phone),
			)
			writeJSON(w, http.StatusOK, inboundResponse{OK: true, Duplicate: true})
			return
		}

		// 5) Upsert lead (idempotent)
		lead, err := d.Leads.GetOrCreate(ctx, tenantID, req.Phone)
		if err != nil {
			d.Log.Error("upsert lead", slog.String("err", err.Error()))
			writeJSON(w, http.StatusInternalServerError, inboundResponse{OK: false, Error: "lead upsert failed: " + err.Error()})
			return
		}

		// 6) Bump last_inbound_at if this is an inbound message
		if req.Direction == "inbound" {
			if err := d.Leads.TouchLastInbound(ctx, lead.ID, req.Timestamp); err != nil {
				// Non-fatal: log and continue
				d.Log.Warn("touch last inbound", slog.String("err", err.Error()))
			}
		}

		d.Log.Info("inbound processed",
			slog.String("tenant_id", tenantID.String()),
			slog.String("phone", req.Phone),
			slog.String("direction", req.Direction),
			slog.String("sender", req.Sender),
			slog.String("message_id", saved.ID.String()),
			slog.String("lead_id", lead.ID.String()),
			slog.Bool("duplicate", duplicate),
		)

		writeJSON(w, http.StatusOK, inboundResponse{
			OK:        true,
			MessageID: saved.ID.String(),
			LeadID:    lead.ID.String(),
			Duplicate: duplicate,
		})
	}
}

// buildIdempotencyKey composes the durable UNIQUE key from the tenant
// id and the provider message id. The bridge passes the raw provider id;
// we prefix it with the tenant to prevent cross-tenant collisions.
func buildIdempotencyKey(tenantID uuid.UUID, providerID string) string {
	return fmt.Sprintf("%s:%s", tenantID, providerID)
}

// writeJSON encodes v to w with the given status code and Content-Type.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// ensure context import is used
var _ = context.Background
