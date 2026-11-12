package api

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/cybirdsecurity/myfi-os/internal/services"
)

type envelope struct {
	Data any `json:"data,omitempty"`
}

type errorEnvelope struct {
	Error errBody `json:"error"`
}

type errBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(envelope{Data: data})
}

// writeForecastError reports a failed projection. A missing birthday is an
// incomplete profile (422) rather than a failure; anything else uses the
// caller's status and code.
func writeForecastError(w http.ResponseWriter, err error, status int, code string) {
	if errors.Is(err, services.ErrBirthdayRequired) {
		writeError(w, http.StatusUnprocessableEntity, "PROFILE_INCOMPLETE", err.Error())
		return
	}
	writeError(w, status, code, err.Error())
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(errorEnvelope{Error: errBody{Code: code, Message: message}})
}

func decode(r *http.Request, dst any) error {
	return json.NewDecoder(r.Body).Decode(dst)
}
