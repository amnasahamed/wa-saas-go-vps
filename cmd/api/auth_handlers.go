package main

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/amnasahamed/niosengine/internal/auth"
	"github.com/amnasahamed/niosengine/internal/db"
)

// authDeps holds the auth handler collaborators.
type authDeps struct {
	Svc        *auth.PGService
	Queries    *db.Queries
	Log        *slog.Logger
	SecureCookie bool
}

type signupRequest struct {
	Email         string `json:"email"`
	Password      string `json:"password"`
	WorkspaceName string `json:"workspace_name"`
}

type signupResponse struct {
	UserID      string `json:"user_id"`
	TenantID    string `json:"tenant_id"`
	VerifyToken string `json:"verify_token,omitempty"`
	TenantSlug  string `json:"tenant_slug"`
}

func handleSignup(d *authDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req signupRequest
		if err := decodeJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_body", err.Error())
			return
		}
		res, err := d.Svc.Signup(r.Context(), auth.SignupInput{
			Email:      req.Email,
			Password:   req.Password,
			TenantName: req.WorkspaceName,
		})
		if err != nil {
			if errors.Is(err, auth.ErrEmailTaken) {
				writeError(w, http.StatusConflict, "email_taken", "Email already in use")
				return
			}
			d.Log.Error("signup", slog.String("err", err.Error()))
			writeError(w, http.StatusBadRequest, "signup_failed", err.Error())
			return
		}
		writeJSON(w, http.StatusCreated, signupResponse{
			UserID:      res.UserID,
			TenantID:    res.TenantID,
			VerifyToken: res.VerifyToken,
			TenantSlug:  res.TenantSlug,
		})
	}
}

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

func handleLogin(d *authDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req loginRequest
		if err := decodeJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_body", err.Error())
			return
		}
		token, err := d.Svc.Login(r.Context(), req.Email, req.Password)
		if err != nil {
			switch {
			case errors.Is(err, auth.ErrEmailNotVerified):
				writeError(w, http.StatusForbidden, "email_not_verified", "Verify your email first")
			case errors.Is(err, auth.ErrInvalidCredentials):
				writeError(w, http.StatusUnauthorized, "invalid_credentials", "Wrong email or password")
			default:
				d.Log.Error("login", slog.String("err", err.Error()))
				writeError(w, http.StatusInternalServerError, "login_failed", "Sign-in failed")
			}
			return
		}
		auth.SetSessionCookie(w, token, d.SecureCookie)
		w.WriteHeader(http.StatusNoContent)
	}
}

func handleLogout(d *authDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		auth.ClearSessionCookie(w)
		w.WriteHeader(http.StatusNoContent)
	}
}

type verifyRequest struct {
	Token string `json:"token"`
}

func handleVerifyEmail(d *authDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req verifyRequest
		if err := decodeJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_body", err.Error())
			return
		}
		if err := d.Svc.VerifyEmail(r.Context(), req.Token); err != nil {
			writeError(w, http.StatusBadRequest, "token_invalid", "Token invalid or expired")
			return
		}
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	}
}

type meUserResponse struct {
	ID            string `json:"id"`
	Email         string `json:"email"`
	Role          string `json:"role"`
	EmailVerified bool   `json:"emailVerified"`
}

type meWorkspaceResponse struct {
	ID                  string `json:"id"`
	Slug                string `json:"slug"`
	Name                string `json:"name"`
	Plan                string `json:"plan"`
	Status              string `json:"status"`
	CountryCode         string `json:"countryCode"`
	Timezone            string `json:"timezone"`
	Locale              string `json:"locale"`
	PromptOverride      string `json:"promptOverride"`
	OpenAIKeyConfigured bool   `json:"openaiKeyConfigured"`
}

type meResponse struct {
	User      meUserResponse      `json:"user"`
	Workspace meWorkspaceResponse `json:"workspace"`
}

func handleMe(d *authDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		p := auth.FromContext(r.Context())
		if p == nil {
			writeError(w, http.StatusUnauthorized, "unauthorized", "Sign in first")
			return
		}
		uid, err := uuid.Parse(p.UserID)
		if err != nil {
			writeError(w, http.StatusUnauthorized, "unauthorized", "Bad principal")
			return
		}
		tid, err := uuid.Parse(p.TenantID)
		if err != nil {
			writeError(w, http.StatusUnauthorized, "unauthorized", "Bad principal")
			return
		}

		userRow, err := d.Queries.GetUserByID(r.Context(), db.PgtypeUUID(uid))
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				writeError(w, http.StatusUnauthorized, "unauthorized", "User no longer exists")
				return
			}
			d.Log.Error("get user", slog.String("err", err.Error()))
			writeError(w, http.StatusInternalServerError, "internal", "lookup failed")
			return
		}

		tRow, err := d.Queries.GetTenantByID(r.Context(), db.PgtypeUUID(tid))
		if err != nil {
			d.Log.Error("get tenant", slog.String("err", err.Error()))
			writeError(w, http.StatusInternalServerError, "internal", "lookup failed")
			return
		}

		whatsappStatus := "not_linked"
		var phoneNumber *string
		if sess, err := d.Queries.GetWhatsAppSessionByTenant(r.Context(), db.PgtypeUUID(tid)); err == nil {
			whatsappStatus = sess.Status
			if sess.PhoneNumber.Valid {
				phoneNumber = &sess.PhoneNumber.String
			}
		}

		out := meResponse{
			User: meUserResponse{
				ID:            p.UserID,
				Email:         userRow.Email,
				Role:          userRow.Role,
				EmailVerified: userRow.EmailVerifiedAt.Valid,
			},
			Workspace: meWorkspaceResponse{
				ID:                  p.TenantID,
				Slug:                tRow.Slug,
				Name:                tRow.Name,
				Plan:                tRow.Plan,
				Status:              tRow.Status,
				CountryCode:         tRow.CountryCode,
				Timezone:            tRow.Timezone,
				Locale:              tRow.Locale,
				PromptOverride:      db.StringFromText(tRow.PromptOverride),
				OpenAIKeyConfigured: len(tRow.OpenaiKeyCiphertext) > 0,
			},
		}
		_ = whatsappStatus
		_ = phoneNumber
		writeJSON(w, http.StatusOK, out)
	}
}
