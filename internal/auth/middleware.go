package auth

import (
	"net/http"
)

// SessionCookieName is the HTTP-only cookie that holds the dashboard JWT.
const SessionCookieName = "nios_session"

// RequireAuth is an HTTP middleware that validates the session cookie and
// attaches the Principal to the request context.
func RequireAuth(svc Service) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			c, err := r.Cookie(SessionCookieName)
			if err != nil || c.Value == "" {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			p, err := svc.ValidateToken(r.Context(), c.Value)
			if err != nil || p == nil {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			ctx := WithPrincipal(r.Context(), p)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// SetSessionCookie writes the JWT to a secure HTTP-only cookie.
func SetSessionCookie(w http.ResponseWriter, token string, secure bool) {
	http.SetCookie(w, &http.Cookie{
		Name:     SessionCookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   7 * 24 * 60 * 60,
	})
}

// ClearSessionCookie removes the session cookie.
func ClearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     SessionCookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		MaxAge:   -1,
	})
}
