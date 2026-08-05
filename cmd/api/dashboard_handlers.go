package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	bridgepb "github.com/amnasahamed/niosengine/internal/bridgepb"
	"github.com/amnasahamed/niosengine/internal/auth"
	"github.com/amnasahamed/niosengine/internal/db"
	"github.com/amnasahamed/niosengine/internal/destinations"
	"github.com/amnasahamed/niosengine/internal/leads"
	"github.com/amnasahamed/niosengine/internal/messages"
	"github.com/amnasahamed/niosengine/internal/nurture"
	"github.com/amnasahamed/niosengine/internal/whatsapp"
)

// dashboardDeps bundles everything a dashboard handler might need.
type dashboardDeps struct {
	Queries     *db.Queries
	AuthSvc     *auth.PGService
	LeadsSvc    leads.Service
	MessagesSvc messages.Service
	DestSvc     destinations.Service
	Log         *slog.Logger
	BridgeCli   *bridgeClient // nil-safe via method receivers
}

// ============================================================================
// /v1/me/workspace
// ============================================================================

type workspaceResponse struct {
	ID                  string          `json:"id"`
	Slug                string          `json:"slug"`
	Name                string          `json:"name"`
	Plan                string          `json:"plan"`
	CountryCode         string          `json:"countryCode"`
	Timezone            string          `json:"timezone"`
	Locale              string          `json:"locale"`
	PromptOverride      string          `json:"promptOverride"`
	OpenAIKeyConfigured bool            `json:"openaiKeyConfigured"`
	WhatsApp            *whatsappStatus `json:"whatsapp,omitempty"`
}

type whatsappStatus struct {
	Status      string  `json:"status"`
	PhoneNumber *string `json:"phoneNumber"`
}

func handleGetWorkspace(d *dashboardDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		p := auth.FromContext(r.Context())
		tid := uuidMust(p.TenantID)
		row, err := d.Queries.GetTenantByID(r.Context(), db.PgtypeUUID(tid))
		if err != nil {
			d.Log.Error("get tenant", slog.String("err", err.Error()))
			writeError(w, http.StatusInternalServerError, "internal", "lookup failed")
			return
		}
		out := workspaceResponse{
			ID:                  p.TenantID,
			Slug:                row.Slug,
			Name:                row.Name,
			Plan:                row.Plan,
			CountryCode:         row.CountryCode,
			Timezone:            row.Timezone,
			Locale:              row.Locale,
			PromptOverride:      db.StringFromText(row.PromptOverride),
			OpenAIKeyConfigured: len(row.OpenaiKeyCiphertext) > 0,
		}
		if sess, err := d.Queries.GetWhatsAppSessionByTenant(r.Context(), db.PgtypeUUID(tid)); err == nil {
			ws := &whatsappStatus{Status: sess.Status}
			if sess.PhoneNumber.Valid {
				pn := sess.PhoneNumber.String
				ws.PhoneNumber = &pn
			}
			out.WhatsApp = ws
		}
		writeJSON(w, http.StatusOK, out)
	}
}

type patchWorkspaceRequest struct {
	Name           *string `json:"name,omitempty"`
	CountryCode    *string `json:"countryCode,omitempty"`
	Timezone       *string `json:"timezone,omitempty"`
	Locale         *string `json:"locale,omitempty"`
	PromptOverride *string `json:"promptOverride,omitempty"`
}

func handlePatchWorkspace(d *dashboardDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		p := auth.FromContext(r.Context())
		tid := uuidMust(p.TenantID)
		var req patchWorkspaceRequest
		if err := decodeJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_body", err.Error())
			return
		}
		params := db.UpdateTenantSettingsParams{
			ID:             db.PgtypeUUID(tid),
			Name:           pgTextFromStringPtr(req.Name),
			CountryCode:    pgTextFromStringPtr(req.CountryCode),
			Timezone:       pgTextFromStringPtr(req.Timezone),
			Locale:         pgTextFromStringPtr(req.Locale),
			PromptOverride: pgtype.Text{},
		}
		if req.PromptOverride != nil {
			params.PromptOverride = pgtype.Text{String: *req.PromptOverride, Valid: true}
		}
		if _, err := d.Queries.UpdateTenantSettings(r.Context(), params); err != nil {
			d.Log.Error("patch workspace", slog.String("err", err.Error()))
			writeError(w, http.StatusInternalServerError, "internal", "update failed")
			return
		}
		handleGetWorkspace(d).ServeHTTP(w, r)
	}
}

type openaiKeyRequest struct {
	Key string `json:"key"`
}

func handleSetOpenAIKey(d *dashboardDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		p := auth.FromContext(r.Context())
		tid := uuidMust(p.TenantID)
		var req openaiKeyRequest
		if err := decodeJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_body", err.Error())
			return
		}
		if strings.TrimSpace(req.Key) == "" {
			writeError(w, http.StatusBadRequest, "invalid_body", "key required")
			return
		}
		// TODO(phase-4): encrypt with KMS envelope before persisting.
		ciphertext := []byte("plain:" + req.Key)
		if err := d.Queries.SetTenantOpenAIKey(r.Context(), db.SetTenantOpenAIKeyParams{
			ID:                 db.PgtypeUUID(tid),
			OpenaiKeyCiphertext: ciphertext,
		}); err != nil {
			d.Log.Error("set openai key", slog.String("err", err.Error()))
			writeError(w, http.StatusInternalServerError, "internal", "update failed")
			return
		}
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	}
}

// ============================================================================
// /v1/leads
// ============================================================================

type leadListItem struct {
	ID              string  `json:"id"`
	Phone           string  `json:"phone"`
	Name            string  `json:"name"`
	Status          string  `json:"status"`
	Stage           string  `json:"stage"`
	LastInboundAt   *string `json:"lastInboundAt"`
	LastContactedAt *string `json:"lastContactedAt"`
}

type leadListResponse struct {
	Items  []leadListItem `json:"items"`
	Total  int            `json:"total"`
	Limit  int            `json:"limit"`
	Offset int            `json:"offset"`
}

func handleListLeads(d *dashboardDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		p := auth.FromContext(r.Context())
		tid := uuidMust(p.TenantID)
		limit := atoiOr(r.URL.Query().Get("limit"), 50)
		offset := atoiOr(r.URL.Query().Get("offset"), 0)
		status := r.URL.Query().Get("status")

		rows, err := d.Queries.ListLeadsByTenant(r.Context(), db.ListLeadsByTenantParams{
			TenantID: db.PgtypeUUID(tid),
			Column2:  status,
			Limit:    int32(limit),
			Offset:   int32(offset),
		})
		if err != nil {
			d.Log.Error("list leads", slog.String("err", err.Error()))
			writeError(w, http.StatusInternalServerError, "internal", "lookup failed")
			return
		}
		items := make([]leadListItem, 0, len(rows))
		for _, row := range rows {
			items = append(items, leadListItem{
				ID:              dbUUIDString(row.ID),
				Phone:           row.Phone,
				Name:            db.StringFromText(row.Name),
				Status:          row.Status,
				Stage:           row.Stage,
				LastInboundAt:   isoPtrFromOpt(db.TimeFromTimestamptzPtr(row.LastInboundAt)),
				LastContactedAt: isoPtrFromOpt(db.TimeFromTimestamptzPtr(row.LastContactedAt)),
			})
		}
		writeJSON(w, http.StatusOK, leadListResponse{
			Items:  items,
			Total:  len(items),
			Limit:  limit,
			Offset: offset,
		})
	}
}

type leadDetailResponse struct {
	Lead        leadDetail   `json:"lead"`
	History     []msgOut     `json:"history"`
	Extractions []extractOut `json:"extractions"`
}

type leadDetail struct {
	ID                 string         `json:"id"`
	Phone              string         `json:"phone"`
	Name               string         `json:"name"`
	Status             string         `json:"status"`
	Stage              string         `json:"stage"`
	Fields             map[string]any `json:"fields"`
	NurturePaused      bool           `json:"nurturePaused"`
	NurturePausedUntil *string        `json:"nurturePausedUntil"`
	LastInboundAt      *string        `json:"lastInboundAt"`
	LastContactedAt    *string        `json:"lastContactedAt"`
	NextFollowupAt     *string        `json:"nextFollowupAt"`
	FollowupCount      int            `json:"followupCount"`
	CreatedAt          string         `json:"createdAt"`
	UpdatedAt          string         `json:"updatedAt"`
}

type msgOut struct {
	ID          string `json:"id"`
	Phone       string `json:"phone"`
	Direction   string `json:"direction"`
	Sender      string `json:"sender"`
	Body        string `json:"body"`
	MessageType string `json:"messageType"`
	Timestamp   string `json:"timestamp"`
}

type extractOut struct {
	ID             string         `json:"id"`
	Fields         map[string]any `json:"fields"`
	PromptTemplate string         `json:"promptTemplate"`
	CreatedAt      string         `json:"createdAt"`
}

func handleGetLead(d *dashboardDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		p := auth.FromContext(r.Context())
		tid := uuidMust(p.TenantID)
		leadID, err := uuid.Parse(pathLeadID(r.URL.Path))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid_id", "Bad lead id")
			return
		}
		row, err := d.Queries.GetLeadByID(r.Context(), db.PgtypeUUID(leadID))
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				writeError(w, http.StatusNotFound, "not_found", "Lead not found")
				return
			}
			d.Log.Error("get lead", slog.String("err", err.Error()))
			writeError(w, http.StatusInternalServerError, "internal", "lookup failed")
			return
		}
		if dbUUIDString(row.TenantID) != tid.String() {
			writeError(w, http.StatusNotFound, "not_found", "Lead not found")
			return
		}

		fieldsMap, _ := db.JSONToMap(row.Fields)

		history, _ := d.MessagesSvc.GetHistory(r.Context(), tid, row.Phone, 100)
		historyOut := make([]msgOut, 0, len(history))
		for _, m := range history {
			historyOut = append(historyOut, msgOut{
				ID:          m.ID.String(),
				Phone:       m.Phone,
				Direction:   m.Direction,
				Sender:      m.Sender,
				Body:        m.Body,
				MessageType: m.MessageType,
				Timestamp:   m.Timestamp.UTC().Format(time.RFC3339Nano),
			})
		}

		extrRows, _ := d.Queries.ListExtractionsByLead(r.Context(), db.ListExtractionsByLeadParams{
			LeadID: db.PgtypeUUID(leadID),
			Limit:  100,
		})
		extrOut := make([]extractOut, 0, len(extrRows))
		for _, e := range extrRows {
			fmap, _ := db.JSONToMap(e.Fields)
			extrOut = append(extrOut, extractOut{
				ID:             dbUUIDString(e.ID),
				Fields:         fmap,
				PromptTemplate: db.StringFromText(e.PromptTemplate),
				CreatedAt:      e.CreatedAt.Time.UTC().Format(time.RFC3339Nano),
			})
		}

		writeJSON(w, http.StatusOK, leadDetailResponse{
			Lead: leadDetail{
				ID:                 dbUUIDString(row.ID),
				Phone:              row.Phone,
				Name:               db.StringFromText(row.Name),
				Status:             row.Status,
				Stage:              row.Stage,
				Fields:             fieldsMap,
				NurturePaused:      row.NurturePaused,
				NurturePausedUntil: isoPtrFromOpt(db.TimeFromTimestamptzPtr(row.NurturePausedUntil)),
				LastInboundAt:      isoPtrFromOpt(db.TimeFromTimestamptzPtr(row.LastInboundAt)),
				LastContactedAt:    isoPtrFromOpt(db.TimeFromTimestamptzPtr(row.LastContactedAt)),
				NextFollowupAt:     isoPtrFromOpt(db.TimeFromTimestamptzPtr(row.NextFollowupAt)),
				FollowupCount:      int(row.FollowupCount),
				CreatedAt:          row.CreatedAt.Time.UTC().Format(time.RFC3339Nano),
				UpdatedAt:          row.UpdatedAt.Time.UTC().Format(time.RFC3339Nano),
			},
			History:     historyOut,
			Extractions: extrOut,
		})
	}
}

type patchLeadRequest struct {
	Stage              *string         `json:"stage,omitempty"`
	Status             *string         `json:"status,omitempty"`
	Name               *string         `json:"name,omitempty"`
	Fields             json.RawMessage `json:"fields,omitempty"`
	NurturePausedUntil *string         `json:"nurturePausedUntil,omitempty"`
}

func handlePatchLead(d *dashboardDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		p := auth.FromContext(r.Context())
		tid := uuidMust(p.TenantID)
		leadID, err := uuid.Parse(pathLeadID(r.URL.Path))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid_id", "Bad lead id")
			return
		}
		var req patchLeadRequest
		if err := decodeJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_body", err.Error())
			return
		}

		existing, err := d.Queries.GetLeadByID(r.Context(), db.PgtypeUUID(leadID))
		if err != nil {
			writeError(w, http.StatusNotFound, "not_found", "Lead not found")
			return
		}
		if dbUUIDString(existing.TenantID) != tid.String() {
			writeError(w, http.StatusNotFound, "not_found", "Lead not found")
			return
		}

		if req.Stage != nil {
			if err := d.LeadsSvc.SetStage(r.Context(), leadID, *req.Stage); err != nil {
				d.Log.Error("set stage", slog.String("err", err.Error()))
				writeError(w, http.StatusInternalServerError, "internal", "update failed")
				return
			}
		}
		if req.Status != nil {
			if err := d.LeadsSvc.SetStatus(r.Context(), leadID, *req.Status); err != nil {
				d.Log.Error("set status", slog.String("err", err.Error()))
				writeError(w, http.StatusInternalServerError, "internal", "update failed")
				return
			}
		}
		if req.Name != nil {
			if err := d.Queries.UpdateLeadName(r.Context(), db.UpdateLeadNameParams{
				ID:   db.PgtypeUUID(leadID),
				Name: pgtype.Text{String: *req.Name, Valid: true},
			}); err != nil {
				d.Log.Error("update lead name", slog.String("err", err.Error()))
				writeError(w, http.StatusInternalServerError, "internal", "update failed")
				return
			}
		}
		if len(req.Fields) > 0 {
			if err := d.Queries.UpdateLeadFields(r.Context(), db.UpdateLeadFieldsParams{
				ID:     db.PgtypeUUID(leadID),
				Fields: []byte(req.Fields),
			}); err != nil {
				d.Log.Error("update lead fields", slog.String("err", err.Error()))
				writeError(w, http.StatusInternalServerError, "internal", "update failed")
				return
			}
		}
		if req.NurturePausedUntil != nil {
			if *req.NurturePausedUntil == "" {
				if err := d.Queries.ClearLeadNurturePaused(r.Context(), db.PgtypeUUID(leadID)); err != nil {
					d.Log.Error("clear nurture pause", slog.String("err", err.Error()))
				}
			} else {
				t, err := time.Parse(time.RFC3339, *req.NurturePausedUntil)
				if err != nil {
					writeError(w, http.StatusBadRequest, "invalid_body", "bad nurturePausedUntil")
					return
				}
				if err := d.Queries.SetLeadNurturePaused(r.Context(), db.SetLeadNurturePausedParams{
					ID:                 db.PgtypeUUID(leadID),
					NurturePausedUntil: db.TimeToTimestamptz(t),
				}); err != nil {
					d.Log.Error("set nurture pause", slog.String("err", err.Error()))
				}
			}
		}
		handleGetLead(d).ServeHTTP(w, r)
	}
}

type sendMessageRequest struct {
	Body        string `json:"body"`
	MessageType string `json:"messageType"`
}

func handleSendLeadMessage(d *dashboardDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		p := auth.FromContext(ctx)
		tid := uuidMust(p.TenantID)
		leadID, err := uuid.Parse(pathLeadID(r.URL.Path))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid_id", "Bad lead id")
			return
		}
		var req sendMessageRequest
		if err := decodeJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_body", err.Error())
			return
		}
		if strings.TrimSpace(req.Body) == "" {
			writeError(w, http.StatusBadRequest, "invalid_body", "body required")
			return
		}
		if req.MessageType == "" {
			req.MessageType = "text"
		}

		lead, err := d.Queries.GetLeadByID(ctx, db.PgtypeUUID(leadID))
		if err != nil || dbUUIDString(lead.TenantID) != tid.String() {
			writeError(w, http.StatusNotFound, "not_found", "Lead not found")
			return
		}
		session, err := d.Queries.GetWhatsAppSessionByTenant(ctx, db.PgtypeUUID(tid))
		if err != nil {
			writeError(w, http.StatusBadRequest, "no_session", "Link WhatsApp first")
			return
		}
		sessionID, _ := db.UUIDFromPgtype(session.ID)

		// Route the send to the bridge worker that owns this tenant's
		// session. Sessions are sharded by worker_id; if the column is
		// empty (not yet assigned) we surface a 503 so the operator
		// knows to wait or to seed worker assignments.
		workerID := ""
		if session.WorkerID.Valid {
			workerID = session.WorkerID.String
		}
		if workerID == "" {
			writeError(w, http.StatusServiceUnavailable, "no_worker", "session has no assigned bridge worker yet")
			return
		}
		if d.BridgeCli == nil {
			writeError(w, http.StatusServiceUnavailable, "no_bridge", "bridge client not configured")
			return
		}
		target, err := d.BridgeCli.ResolveTarget(workerID)
		if err != nil {
			d.Log.Warn("bridge worker not configured",
				slog.String("worker_id", workerID),
				slog.String("err", err.Error()),
			)
			writeError(w, http.StatusServiceUnavailable, "no_bridge", err.Error())
			return
		}

		// Stable client_msg_id so a retry from the dashboard (network
		// blip, double-click) does not double-send. client_msg_id
		// dedup at the bridge is a separate follow-up — for now we just
		// get one send per request and rely on the dashboard to surface
		// a clear error if it retries.
		clientMsgID := uuid.New().String()
		bridgeReq := &bridgepb.SendMessageRequest{
			TenantId:          tid.String(),
			WhatsappSessionId: sessionID.String(),
			Phone:             lead.Phone,
			Body:              req.Body,
			ClientMsgId:       clientMsgID,
		}
		bridgeResp, err := d.BridgeCli.SendMessage(ctx, workerID, target, bridgeReq)
		if err != nil {
			d.Log.Error("bridge send failed",
				slog.String("worker_id", workerID),
				slog.String("err", err.Error()),
			)
			writeError(w, statusToHTTP(err), "send_failed", err.Error())
			return
		}
		if !bridgeResp.Ok {
			writeError(w, http.StatusBadGateway, "send_failed", bridgeResp.Error)
			return
		}

		// Persist with the *real* provider_message_id from the bridge
		// (not the synthetic uuid we used to use).
		msg := &messages.Message{
			TenantID:          tid,
			WhatsAppSessionID: sessionID,
			ProviderMessageID: bridgeResp.ProviderMessageId,
			Phone:             lead.Phone,
			Direction:         "outbound",
			Sender:            "counselor",
			MessageType:       req.MessageType,
			Body:              req.Body,
			IdempotencyKey:    fmt.Sprintf("counselor:%s:%s", lead.ID, clientMsgID),
			Timestamp:         time.Now().UTC(),
		}
		saved, err := d.MessagesSvc.PersistOutbound(ctx, msg)
		if err != nil {
			// The send already went out. Log loudly; the operator can
			// reconcile from the bridge log. Returning 502 lets the
			// dashboard surface a clear "sent but DB out of sync" error.
			d.Log.Error("persist outbound after send",
				slog.String("provider_message_id", bridgeResp.ProviderMessageId),
				slog.String("err", err.Error()),
			)
			writeError(w, http.StatusBadGateway, "persist_after_send", "message was sent but DB persist failed: "+err.Error())
			return
		}
		writeJSON(w, http.StatusCreated, map[string]any{
			"id":                saved.ID.String(),
			"provider_message_id": bridgeResp.ProviderMessageId,
			"created_at":        saved.CreatedAt.UTC().Format(time.RFC3339Nano),
		})
	}
}

// statusToHTTP maps a gRPC status error to the most useful HTTP code for
// the dashboard. Unknown errors fall through to 502.
func statusToHTTP(err error) int {
	st, ok := status.FromError(err)
	if !ok {
		return http.StatusBadGateway
	}
	switch st.Code() {
	case codes.Unavailable, codes.DeadlineExceeded:
		return http.StatusServiceUnavailable
	case codes.NotFound:
		return http.StatusGone
	case codes.FailedPrecondition:
		return http.StatusConflict
	case codes.Unauthenticated, codes.PermissionDenied:
		return http.StatusForbidden
	case codes.InvalidArgument:
		return http.StatusBadRequest
	default:
		return http.StatusBadGateway
	}
}

// ============================================================================
// /v1/destinations
// ============================================================================

type destinationOut struct {
	ID           string            `json:"id"`
	Type         string            `json:"type"`
	Name         string            `json:"name"`
	Enabled      bool              `json:"enabled"`
	FieldMapping map[string]string `json:"fieldMapping"`
	LastSyncAt   *string           `json:"lastSyncAt"`
	LastError    *string           `json:"lastError"`
	CreatedAt    string            `json:"createdAt"`
}

type destinationListResponse struct {
	Items []destinationOut `json:"items"`
}

func destToOut(d *destinations.Destination) destinationOut {
	var ls *string
	if d.LastSyncAt != nil {
		s := d.LastSyncAt.UTC().Format(time.RFC3339Nano)
		ls = &s
	}
	return destinationOut{
		ID:           d.ID,
		Type:         d.Type,
		Name:         d.Name,
		Enabled:      d.Enabled,
		FieldMapping: d.FieldMapping,
		LastSyncAt:   ls,
		CreatedAt:    d.CreatedAt.UTC().Format(time.RFC3339Nano),
	}
}

func handleListDestinations(d *dashboardDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		p := auth.FromContext(r.Context())
		rows, err := d.DestSvc.List(r.Context(), p.TenantID)
		if err != nil {
			d.Log.Error("list destinations", slog.String("err", err.Error()))
			writeError(w, http.StatusInternalServerError, "internal", "lookup failed")
			return
		}
		out := destinationListResponse{Items: make([]destinationOut, 0, len(rows))}
		for _, row := range rows {
			out.Items = append(out.Items, destToOut(row))
		}
		writeJSON(w, http.StatusOK, out)
	}
}

type connectDestinationRequest struct {
	Type         string            `json:"type"`
	Name         string            `json:"name"`
	Config       map[string]any    `json:"config"`
	FieldMapping map[string]string `json:"field_mapping"`
}

func handleConnectDestination(d *dashboardDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		p := auth.FromContext(r.Context())
		var req connectDestinationRequest
		if err := decodeJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_body", err.Error())
			return
		}
		cfgBytes, err := json.Marshal(req.Config)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid_body", "bad config")
			return
		}
		dest, err := d.DestSvc.Connect(r.Context(), p.TenantID, req.Type, req.Name, cfgBytes, req.FieldMapping)
		if err != nil {
			if errors.Is(err, destinations.ErrUnsupportedType) {
				writeError(w, http.StatusBadRequest, "unsupported_type", "Unknown destination type")
				return
			}
			d.Log.Error("connect destination", slog.String("err", err.Error()))
			writeError(w, http.StatusInternalServerError, "internal", "create failed")
			return
		}
		writeJSON(w, http.StatusCreated, destToOut(dest))
	}
}

type patchDestinationRequest struct {
	Enabled      *bool             `json:"enabled,omitempty"`
	FieldMapping map[string]string `json:"field_mapping,omitempty"`
}

func handlePatchDestination(d *dashboardDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		p := auth.FromContext(r.Context())
		id := pathDestID(r.URL.Path)
		if id == "" {
			writeError(w, http.StatusBadRequest, "invalid_id", "destination id required")
			return
		}
		var req patchDestinationRequest
		if err := decodeJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_body", err.Error())
			return
		}
		uid, err := uuid.Parse(id)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid_id", "Bad destination id")
			return
		}
		if req.Enabled != nil {
			if err := d.Queries.SetDestinationEnabled(r.Context(), db.SetDestinationEnabledParams{
				ID:      db.PgtypeUUID(uid),
				Enabled: *req.Enabled,
			}); err != nil {
				d.Log.Error("set destination enabled", slog.String("err", err.Error()))
				writeError(w, http.StatusInternalServerError, "internal", "update failed")
				return
			}
		}
		if req.FieldMapping != nil {
			mappingJSON, _ := json.Marshal(req.FieldMapping)
			if err := d.Queries.UpdateDestinationFieldMapping(r.Context(), db.UpdateDestinationFieldMappingParams{
				ID:           db.PgtypeUUID(uid),
				FieldMapping: mappingJSON,
			}); err != nil {
				d.Log.Error("update destination mapping", slog.String("err", err.Error()))
				writeError(w, http.StatusInternalServerError, "internal", "update failed")
				return
			}
		}
		dests, err := d.DestSvc.List(r.Context(), p.TenantID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal", "lookup failed")
			return
		}
		for _, x := range dests {
			if x.ID == id {
				writeJSON(w, http.StatusOK, destToOut(x))
				return
			}
		}
		writeError(w, http.StatusNotFound, "not_found", "Destination not found")
	}
}

func handleDeleteDestination(d *dashboardDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		p := auth.FromContext(r.Context())
		id := pathDestID(r.URL.Path)
		if id == "" {
			writeError(w, http.StatusBadRequest, "invalid_id", "destination id required")
			return
		}
		if err := d.DestSvc.Disconnect(r.Context(), p.TenantID, id); err != nil {
			writeError(w, http.StatusInternalServerError, "internal", "delete failed")
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

// ============================================================================
// /v1/whatsapp/session
// ============================================================================

type whatsappSessionOut struct {
	ID          string  `json:"id"`
	Status      string  `json:"status"`
	PhoneNumber *string `json:"phoneNumber"`
	WorkerID    *string `json:"workerId"`
	QRDataURL   *string `json:"qrDataUrl"`
	QRExpiresAt *string `json:"qrExpiresAt"`
	LastError   *string `json:"lastError"`
}

func handleGetWhatsAppSession(d *dashboardDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		p := auth.FromContext(r.Context())
		tid := uuidMust(p.TenantID)
		row, err := d.Queries.GetWhatsAppSessionByTenant(r.Context(), db.PgtypeUUID(tid))
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				writeJSON(w, http.StatusOK, whatsappSessionOut{Status: "not_linked"})
				return
			}
			writeError(w, http.StatusInternalServerError, "internal", "lookup failed")
			return
		}
		writeJSON(w, http.StatusOK, renderSessionRow(row))
	}
}

// renderSessionRow converts a db.WhatsappSession into the dashboard-facing
// JSON shape. Shared by GET and POST /v1/whatsapp/session so both
// responses match exactly (the dashboard polls the same shape).
func renderSessionRow(row db.WhatsappSession) whatsappSessionOut {
	out := whatsappSessionOut{
		ID:     dbUUIDString(row.ID),
		Status: row.Status,
	}
	if row.PhoneNumber.Valid {
		s := row.PhoneNumber.String
		out.PhoneNumber = &s
	}
	if row.WorkerID.Valid {
		s := row.WorkerID.String
		out.WorkerID = &s
	}
	if row.QrCode.Valid && row.QrCode.String != "" {
		dataURL, derr := whatsapp.RenderQRDataURL(row.QrCode.String)
		if derr == nil {
			out.QRDataURL = &dataURL
		}
	}
	if row.QrExpiresAt.Valid {
		s := row.QrExpiresAt.Time.UTC().Format(time.RFC3339Nano)
		out.QRExpiresAt = &s
	}
	if row.LastError.Valid {
		s := row.LastError.String
		out.LastError = &s
	}
	return out
}

// ============================================================================
// /v1/prompts/*
// ============================================================================

func handleGetDefaultPrompt() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"template": nurture.DefaultSystemPrompt})
	}
}

type previewRequest struct {
	Template string                  `json:"template"`
	History  []previewHistoryMessage `json:"history"`
}

type previewHistoryMessage struct {
	Direction string `json:"direction"`
	Sender    string `json:"sender"`
	Body      string `json:"body"`
}

type previewResponse struct {
	CompiledPreview string `json:"compiledPreview"`
	PredictedStage  string `json:"predictedStage"`
	LanguageHint    string `json:"languageHint"`
}

func handlePreviewPrompt() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req previewRequest
		if err := decodeJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_body", err.Error())
			return
		}
		stage := "ask_name"
		fields := map[string]any{}
		for _, m := range req.History {
			if m.Direction == "inbound" {
				body := strings.ToLower(m.Body)
				if fields["name"] == nil && len(m.Body) > 0 && looksLikeName(m.Body) {
					fields["name"] = m.Body
				}
				if fields["class_level"] == nil && looksLikeClass(body) {
					fields["class_level"] = m.Body
				}
				if fields["exam_result"] == nil && looksLikeResult(body) {
					fields["exam_result"] = extractResult(body)
				}
				if fields["stream"] == nil && looksLikeStream(body) {
					fields["stream"] = m.Body
				}
			}
		}
		eval := nurture.NewStageEvaluator()
		next := eval.NextStage(stage, fields)
		_ = req.Template
		writeJSON(w, http.StatusOK, previewResponse{
			CompiledPreview: "Reply preview not generated — live replies use GPT-4o.",
			PredictedStage:  next,
			LanguageHint:    "english",
		})
	}
}

func looksLikeName(s string) bool {
	t := strings.TrimSpace(s)
	if len(t) == 0 || len(t) > 60 {
		return false
	}
	if strings.ContainsAny(t, "0123456789?.,") {
		return false
	}
	return true
}

func looksLikeClass(s string) bool {
	keys := []string{"10th", "12th", "plus two", "hssc", "sslc"}
	for _, k := range keys {
		if strings.Contains(s, k) {
			return true
		}
	}
	return false
}

func looksLikeResult(s string) bool {
	return strings.Contains(s, "pass") || strings.Contains(s, "fail") || strings.Contains(s, "appear")
}

func extractResult(s string) string {
	switch {
	case strings.Contains(s, "fail"):
		return "fail"
	case strings.Contains(s, "appear"):
		return "appearing"
	default:
		return "pass"
	}
}

func looksLikeStream(s string) bool {
	for _, k := range []string{"science", "commerce", "arts", "humanities"} {
		if strings.Contains(s, k) {
			return true
		}
	}
	return false
}

// ============================================================================
// /v1/me/onboarding + /v1/me/overview + /v1/me/stage-usage
// ============================================================================

type onboardingResponse struct {
	EmailVerified  bool `json:"email_verified"`
	WhatsAppLinked bool `json:"whatsapp_linked"`
	HasDestination bool `json:"has_destination"`
}

func handleOnboarding(d *dashboardDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		p := auth.FromContext(r.Context())
		tid := uuidMust(p.TenantID)
		uid, _ := uuid.Parse(p.UserID)
		userRow, err := d.Queries.GetUserByID(r.Context(), db.PgtypeUUID(uid))
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal", "lookup failed")
			return
		}
		out := onboardingResponse{
			EmailVerified:  userRow.EmailVerifiedAt.Valid,
			WhatsAppLinked: false,
			HasDestination: false,
		}
		if _, err := d.Queries.GetWhatsAppSessionByTenant(r.Context(), db.PgtypeUUID(tid)); err == nil {
			out.WhatsAppLinked = true
		}
		dests, _ := d.DestSvc.List(r.Context(), p.TenantID)
		out.HasDestination = len(dests) > 0
		writeJSON(w, http.StatusOK, out)
	}
}

type stageUsageOut struct {
	Stage string `json:"stage"`
	Count int    `json:"count"`
}

func handleStageUsage(d *dashboardDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		p := auth.FromContext(r.Context())
		tid := uuidMust(p.TenantID)
		rows, err := d.Queries.ListLeadsByTenant(r.Context(), db.ListLeadsByTenantParams{
			TenantID: db.PgtypeUUID(tid),
			Limit:    1000,
			Offset:   0,
		})
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal", "lookup failed")
			return
		}
		counts := map[string]int{}
		for _, row := range rows {
			counts[row.Stage]++
		}
		out := make([]stageUsageOut, 0, len(counts))
		for stage, n := range counts {
			out = append(out, stageUsageOut{Stage: stage, Count: n})
		}
		writeJSON(w, http.StatusOK, out)
	}
}

type overviewResponse struct {
	Totals   overviewTotals  `json:"totals"`
	Activity []overviewEntry `json:"activity"`
}

type overviewTotals struct {
	All           int `json:"all"`
	NewToday      int `json:"new_today"`
	AwaitingReply int `json:"awaiting_reply"`
	Enrolled      int `json:"enrolled"`
}

type overviewEntry struct {
	Kind      string `json:"kind"`
	LeadID    string `json:"leadId"`
	LeadName  string `json:"leadName"`
	Body      string `json:"body"`
	Timestamp string `json:"timestamp"`
}

func handleOverview(d *dashboardDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		p := auth.FromContext(r.Context())
		tid := uuidMust(p.TenantID)
		rows, err := d.Queries.ListLeadsByTenant(r.Context(), db.ListLeadsByTenantParams{
			TenantID: db.PgtypeUUID(tid),
			Limit:    1000,
			Offset:   0,
		})
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal", "lookup failed")
			return
		}
		totals := overviewTotals{All: len(rows)}
		today := time.Now().UTC().Truncate(24 * time.Hour)
		activity := []overviewEntry{}
		for _, row := range rows {
			if row.CreatedAt.Time.After(today) {
				totals.NewToday++
			}
			if row.Status == "Follow-up" {
				totals.AwaitingReply++
			}
			if row.Status == "Enrolled" {
				totals.Enrolled++
			}
			if len(activity) < 20 {
				body := "Status: " + row.Status + ", Stage: " + row.Stage
				ts := row.UpdatedAt.Time
				if row.LastInboundAt.Valid && row.LastInboundAt.Time.After(ts) {
					ts = row.LastInboundAt.Time
				}
				activity = append(activity, overviewEntry{
					Kind:      "stage",
					LeadID:    dbUUIDString(row.ID),
					LeadName:  db.StringFromText(row.Name),
					Body:      body,
					Timestamp: ts.UTC().Format(time.RFC3339Nano),
				})
			}
		}
		writeJSON(w, http.StatusOK, overviewResponse{Totals: totals, Activity: activity})
	}
}

// ============================================================================
// helpers
// ============================================================================

func uuidMust(s string) uuid.UUID {
	u, err := uuid.Parse(s)
	if err != nil {
		panic(err)
	}
	return u
}

func atoiOr(s string, fallback int) int {
	if s == "" {
		return fallback
	}
	var n int
	for _, c := range s {
		if c < '0' || c > '9' {
			return fallback
		}
		n = n*10 + int(c-'0')
	}
	return n
}

// pathLeadID extracts the lead id from "/v1/leads/<id>[/messages]".
func pathLeadID(path string) string {
	trim := strings.TrimPrefix(path, "/v1/leads/")
	if i := strings.Index(trim, "/"); i >= 0 {
		return trim[:i]
	}
	return trim
}

// pathDestID extracts the destination id from "/v1/destinations/<id>".
func pathDestID(path string) string {
	trim := strings.TrimPrefix(path, "/v1/destinations/")
	if i := strings.Index(trim, "/"); i >= 0 {
		return trim[:i]
	}
	return trim
}

func isoPtr(t time.Time) *string {
	if t.IsZero() {
		return nil
	}
	s := t.UTC().Format(time.RFC3339Nano)
	return &s
}

func isoPtrFromOpt(t *time.Time) *string {
	if t == nil || t.IsZero() {
		return nil
	}
	return isoPtr(*t)
}

func derefString(s *string, fallback string) string {
	if s == nil {
		return fallback
	}
	return *s
}

func pgTextFromStringPtr(s *string) pgtype.Text {
	if s == nil {
		return pgtype.Text{}
	}
	return pgtype.Text{String: *s, Valid: true}
}

func dbUUIDString(p pgtype.UUID) string {
	u, err := uuid.FromBytes(p.Bytes[:])
	if err != nil {
		return ""
	}
	return u.String()
}
