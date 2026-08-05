package main

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/amnasahamed/niosengine/internal/auth"
	"github.com/amnasahamed/niosengine/internal/db"

	bridgepb "github.com/amnasahamed/niosengine/internal/bridgepb"
)

// handlePostWhatsAppSession starts (or resumes) a WhatsApp linking flow for
// the authenticated tenant. Idempotent: a retried POST returns the same
// session state without spinning up a second whatsmeow client.
//
// The dashboard polls GET /v1/whatsapp/session every 5 s for the QR + status
// (see web/app/(app)/whatsapp/page.tsx), so this handler returns 202 with
// the canonical session row and the dashboard waits for the QR to appear.
func handlePostWhatsAppSession(d *dashboardDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		p := auth.FromContext(ctx)
		tid := uuidMust(p.TenantID)

		if d.BridgeCli == nil {
			writeError(w, http.StatusServiceUnavailable, "no_bridge", "bridge client not configured")
			return
		}
		// Single-bridge shortcut: pick the only configured worker. A
		// future multi-bridge setup will let the dashboard pick a
		// worker explicitly (the api then routes by session.worker_id
		// on subsequent sends).
		workerID, target, err := pickAnyBridgeWorker(d)
		if err != nil {
			writeError(w, http.StatusServiceUnavailable, "no_bridge", err.Error())
			return
		}

		linkCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		_, err = d.BridgeCli.StartLink(linkCtx, workerID, target, &bridgepb.StreamQRRequest{
			TenantId: tid.String(),
		})
		cancel()
		if err != nil {
			d.Log.Error("start link", slog.String("err", err.Error()))
			writeError(w, statusToHTTP(err), "start_link_failed", err.Error())
			return
		}

		// Re-read the session so the dashboard sees the canonical row
		// (status, worker_id, etc.).
		row, dbErr := d.Queries.GetWhatsAppSessionByTenant(ctx, db.PgtypeUUID(tid))
		if dbErr != nil {
			writeError(w, http.StatusInternalServerError, "lookup", "post-link lookup failed")
			return
		}
		writeJSON(w, http.StatusAccepted, renderSessionRow(row))
	}
}

// pickAnyBridgeWorker returns a worker/target pair that can be used to
// route a StartLink call. Without tenant-side routing information the
// only safe choice is the single-bridge default ("worker-1"). A future
// multi-bridge setup will replace this with a tenant->worker lookup.
func pickAnyBridgeWorker(d *dashboardDeps) (workerID, target string, err error) {
	const defaultWorker = "worker-1"
	t, err := d.BridgeCli.ResolveTarget(defaultWorker)
	if err != nil {
		return "", "", err
	}
	return defaultWorker, t, nil
}
