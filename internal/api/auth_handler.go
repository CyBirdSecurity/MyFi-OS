package api

import (
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/cybirdsecurity/myfi-os/internal/auth"
)

// setAuthCookie writes the session cookie for token, valid for ttl.
func (s *Server) setAuthCookie(w http.ResponseWriter, token string, ttl time.Duration) {
	http.SetCookie(w, &http.Cookie{
		Name:     "myfi_token",
		Value:    token,
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
		Path:     "/",
		Expires:  time.Now().Add(ttl),
		Secure:   s.cfg.SecureCookies,
	})
}

const (
	maxFailedAttempts = 10
	lockoutDuration   = 15 * time.Minute
)

// handleRegister creates the instance's only account during first-run setup.
// Once it exists, registration is closed.
func (s *Server) handleRegister(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Email       string `json:"email"`
		Password    string `json:"password"`
		DisplayName string `json:"displayName"`
	}
	if err := decode(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid JSON")
		return
	}
	if body.Email == "" || body.Password == "" || body.DisplayName == "" {
		writeError(w, http.StatusBadRequest, "VALIDATION", "email, password, and displayName are required")
		return
	}
	if len(body.Password) < 8 {
		writeError(w, http.StatusBadRequest, "VALIDATION", "password must be at least 8 characters")
		return
	}

	exists, err := s.users.Exists(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "SERVER_ERROR", "could not check setup state")
		return
	}
	if exists {
		writeError(w, http.StatusConflict, "ALREADY_SET_UP", "this instance already has an account; sign in instead")
		return
	}

	hash, err := auth.HashPassword(body.Password)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "SERVER_ERROR", "could not hash password")
		return
	}

	// The schema allows a single user row, so a racing second request fails here.
	user, err := s.users.Create(r.Context(), body.Email, hash, body.DisplayName)
	if err != nil {
		writeError(w, http.StatusConflict, "ALREADY_SET_UP", "this instance already has an account; sign in instead")
		return
	}

	writeJSON(w, http.StatusCreated, user)
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := decode(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid JSON")
		return
	}

	user, err := s.users.GetByEmail(r.Context(), body.Email)
	if err != nil {
		// Avoid timing oracle: return the same error regardless of whether the
		// account exists.
		writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "invalid credentials")
		return
	}

	// Check lockout before doing the (more expensive) password comparison.
	if user.LockedUntil != nil && user.LockedUntil.After(time.Now()) {
		writeError(w, http.StatusLocked,
			"ACCOUNT_LOCKED",
			fmt.Sprintf("account temporarily locked, try again after %s",
				user.LockedUntil.UTC().Format(time.RFC3339)),
		)
		return
	}

	if !auth.CheckPassword(user.PasswordHash, body.Password) {
		_ = s.users.IncrementFailedAttempts(r.Context(), user.ID, maxFailedAttempts, lockoutDuration)
		writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "invalid credentials")
		return
	}

	// Successful login — clear any previous lockout state.
	_ = s.users.ResetFailedAttempts(r.Context(), user.ID)

	token, err := auth.GenerateToken(s.cfg.JWTSecret, user.ID, user.Email)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "SERVER_ERROR", "could not generate token")
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     "myfi_token",
		Value:    token,
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
		Path:     "/",
		Expires:  time.Now().Add(30 * 24 * time.Hour),
		Secure:   s.cfg.SecureCookies,
	})

	writeJSON(w, http.StatusOK, user)
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     "myfi_token",
		Value:    "",
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
		Path:     "/",
		MaxAge:   -1,
	})
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleMe(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromCtx(r)
	user, err := s.users.GetByID(r.Context(), userID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "NOT_FOUND", "user not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "SERVER_ERROR", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, user)
}

func (s *Server) handleConfig(w http.ResponseWriter, r *http.Request) {
	exists, err := s.users.Exists(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "SERVER_ERROR", "could not check setup state")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"env":           s.cfg.Env,
		"plaidEnabled":  s.plaid != nil,
		"setupRequired": !exists,
	})
}

// handlePasswordChange sets a new password for the signed-in user after
// checking their current one. Every other session is signed out; the caller
// gets a fresh one.
//
// POST /auth/password/change
// Body: {"currentPassword": "...", "newPassword": "..."}
func (s *Server) handlePasswordChange(w http.ResponseWriter, r *http.Request) {
	var body struct {
		CurrentPassword string `json:"currentPassword"`
		NewPassword     string `json:"newPassword"`
	}
	if err := decode(r, &body); err != nil || len(body.NewPassword) < 8 {
		writeError(w, http.StatusBadRequest, "VALIDATION", "your current password and a new password of at least 8 characters are required")
		return
	}
	userID := userIDFromCtx(r)
	user, err := s.users.GetByID(r.Context(), userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "SERVER_ERROR", "could not load your account")
		return
	}
	if !auth.CheckPassword(user.PasswordHash, body.CurrentPassword) {
		writeError(w, http.StatusBadRequest, "INVALID_PASSWORD", "your current password is incorrect")
		return
	}

	hash, err := auth.HashPassword(body.NewPassword)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "SERVER_ERROR", "could not hash password")
		return
	}
	if err := s.users.UpdatePassword(r.Context(), userID, hash); err != nil {
		writeError(w, http.StatusInternalServerError, "SERVER_ERROR", "could not update password")
		return
	}
	_ = s.users.ResetFailedAttempts(r.Context(), userID)

	// UpdatePassword just signed out every session, this one included; hand the
	// caller a fresh token so they stay logged in.
	token, err := auth.GenerateToken(s.cfg.JWTSecret, userID, emailFromCtx(r))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "SERVER_ERROR", "password changed, but could not refresh your session; please sign in again")
		return
	}
	s.setAuthCookie(w, token, 30*24*time.Hour)
	w.WriteHeader(http.StatusNoContent)
}
