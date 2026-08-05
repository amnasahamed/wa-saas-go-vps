package main

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/amnasahamed/niosengine/internal/auth"
	"github.com/amnasahamed/niosengine/internal/db"
)

type apiKeyOut struct {
	ID         string  `json:"id"`
	Name       string  `json:"name"`
	Prefix     string  `json:"prefix"`
	Scope      string  `json:"scope"`
	LastUsedAt *string `json:"lastUsedAt"`
	CreatedAt  string  `json:"createdAt"`
	RevokedAt  *string `json:"revokedAt"`
	Secret     string  `json:"secret,omitempty"` // only on creation response
}

type apiKeyListResponse struct {
	Items []apiKeyOut `json:"items"`
}

type mintKeyRequest struct {
	Name  string `json:"name"`
	Scope string `json:"scope"`
}

func handleListAPIKeys(d *dashboardDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		p := auth.FromContext(r.Context())
		tid := uuidMust(p.TenantID)
		rows, err := d.Queries.ListAPIKeysByTenant(r.Context(), db.PgtypeUUID(tid))
		if err != nil {
			d.Log.Error("list api keys", slog.String("err", err.Error()))
			writeError(w, http.StatusInternalServerError, "internal", "lookup failed")
			return
		}
		out := apiKeyListResponse{Items: make([]apiKeyOut, 0, len(rows))}
		for _, row := range rows {
			out.Items = append(out.Items, keyRowToOut(row))
		}
		writeJSON(w, http.StatusOK, out)
	}
}

func handleMintAPIKey(d *dashboardDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		p := auth.FromContext(r.Context())
		var req mintKeyRequest
		if err := decodeJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_body", err.Error())
			return
		}
		if strings.TrimSpace(req.Name) == "" {
			writeError(w, http.StatusBadRequest, "invalid_body", "name required")
			return
		}
		if req.Scope == "" {
			req.Scope = "bridge"
		}

		tid := uuidMust(p.TenantID)
		prefixBytes := make([]byte, 6)
		if _, err := rand.Read(prefixBytes); err != nil {
			writeError(w, http.StatusInternalServerError, "internal", "rand failed")
			return
		}
		secretBytes := make([]byte, 26)
		if _, err := rand.Read(secretBytes); err != nil {
			writeError(w, http.StatusInternalServerError, "internal", "rand failed")
			return
		}
		prefix := "wa_live_" + hex.EncodeToString(prefixBytes)
		secret := hex.EncodeToString(secretBytes)
		hash, err := bcryptGenerate(secret)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal", "hash failed")
			return
		}

		id := uuid.New()
		row, err := d.Queries.CreateAPIKey(r.Context(), db.CreateAPIKeyParams{
			ID:        db.PgtypeUUID(id),
			TenantID:  db.PgtypeUUID(tid),
			Name:      req.Name,
			KeyPrefix: prefix,
			KeyHash:   hash,
			Scope:     req.Scope,
		})
		if err != nil {
			d.Log.Error("create api key", slog.String("err", err.Error()))
			writeError(w, http.StatusInternalServerError, "internal", "create failed")
			return
		}
		out := keyRowToOut(row)
		out.Secret = prefix + "_" + secret
		writeJSON(w, http.StatusCreated, out)
	}
}

func handleRevokeAPIKey(d *dashboardDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		p := auth.FromContext(r.Context())
		id := pathKeyID(r.URL.Path)
		if id == "" {
			writeError(w, http.StatusBadRequest, "invalid_id", "key id required")
			return
		}
		kid, err := uuid.Parse(id)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid_id", "Bad key id")
			return
		}
		tid := uuidMust(p.TenantID)
		if err := d.Queries.RevokeAPIKey(r.Context(), db.RevokeAPIKeyParams{
			ID:       db.PgtypeUUID(kid),
			TenantID: db.PgtypeUUID(tid),
		}); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				writeError(w, http.StatusNotFound, "not_found", "Key not found")
				return
			}
			d.Log.Error("revoke api key", slog.String("err", err.Error()))
			writeError(w, http.StatusInternalServerError, "internal", "revoke failed")
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

// pathKeyID extracts the key id from /v1/me/api-keys/<id>.
func pathKeyID(path string) string {
	trim := strings.TrimPrefix(path, "/v1/me/api-keys/")
	if i := strings.Index(trim, "/"); i >= 0 {
		return trim[:i]
	}
	return trim
}

func keyRowToOut(row db.ApiKey) apiKeyOut {
	out := apiKeyOut{
		ID:        dbUUIDString(row.ID),
		Name:      row.Name,
		Prefix:    row.KeyPrefix,
		Scope:     row.Scope,
		CreatedAt: row.CreatedAt.Time.UTC().Format(time.RFC3339Nano),
	}
	if row.LastUsedAt.Valid {
		s := row.LastUsedAt.Time.UTC().Format(time.RFC3339Nano)
		out.LastUsedAt = &s
	}
	if row.RevokedAt.Valid {
		s := row.RevokedAt.Time.UTC().Format(time.RFC3339Nano)
		out.RevokedAt = &s
	}
	return out
}
