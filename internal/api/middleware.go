package api

import (
	"context"
	"net/http"
	"time"

	"github.com/cybirdsecurity/myfi-os/internal/auth"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

type contextKey string

const ctxUserID contextKey = "userID"
const ctxEmail contextKey = "email"

func (s *Server) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie("myfi_token")
		if err != nil {
			writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
			return
		}
		claims, err := auth.ParseToken(s.cfg.JWTSecret, cookie.Value)
		if err != nil {
			writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "invalid or expired token")
			return
		}
		// A valid token isn't enough: the password may have changed since it was
		// issued (which revokes earlier sessions).
		active, validAfter, err := s.users.SessionState(r.Context(), claims.UserID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "SERVER_ERROR", "could not verify account")
			return
		}
		if !active {
			writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "account no longer exists")
			return
		}
		if tokenRevoked(claims.IssuedAt, validAfter) {
			writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "session expired, please sign in again")
			return
		}
		ctx := context.WithValue(r.Context(), ctxUserID, claims.UserID)
		ctx = context.WithValue(ctx, ctxEmail, claims.Email)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// tokenRevoked reports whether a token issued at iat predates the user's
// sessions_valid_after. Token times have one-second precision, so the cutoff is
// truncated to the second: a token issued in the same second as the change
// (such as the fresh one a password change hands back) stays valid.
func tokenRevoked(iat *jwt.NumericDate, validAfter *time.Time) bool {
	if validAfter == nil {
		return false
	}
	if iat == nil {
		return true
	}
	return iat.Time.Before(validAfter.Truncate(time.Second))
}

func emailFromCtx(r *http.Request) string {
	email, _ := r.Context().Value(ctxEmail).(string)
	return email
}

func userIDFromCtx(r *http.Request) uuid.UUID {
	return r.Context().Value(ctxUserID).(uuid.UUID)
}
