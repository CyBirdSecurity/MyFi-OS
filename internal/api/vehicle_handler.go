package api

import (
	"database/sql"
	"errors"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/cybirdsecurity/myfi-os/internal/db"
	"github.com/cybirdsecurity/myfi-os/internal/models"
)

func (s *Server) handleGetVehicleDetails(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromCtx(r)
	accountID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid account id")
		return
	}

	d, err := s.vehicle.Get(r.Context(), accountID, userID)
	if errors.Is(err, sql.ErrNoRows) {
		writeJSON(w, http.StatusOK, &models.VehicleDetails{AccountID: accountID, UserID: userID})
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "SERVER_ERROR", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, d)
}

func (s *Server) handleUpsertVehicleDetails(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromCtx(r)
	accountID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid account id")
		return
	}

	var body struct {
		VehicleYear  int     `json:"vehicleYear"`
		VehicleMake  string  `json:"vehicleMake"`
		VehicleModel string  `json:"vehicleModel"`
		VehicleTrim  *string `json:"vehicleTrim"`
		// Value is the owner's own estimate of what the vehicle is worth.
		Value *float64 `json:"value"`
	}
	if err := decode(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid JSON")
		return
	}
	if body.VehicleYear < 1900 || body.VehicleMake == "" || body.VehicleModel == "" {
		writeError(w, http.StatusBadRequest, "VALIDATION", "vehicleYear, vehicleMake, and vehicleModel are required")
		return
	}

	d, err := s.vehicle.Upsert(r.Context(), db.UpsertVehicleParams{
		AccountID:    accountID,
		UserID:       userID,
		VehicleYear:  body.VehicleYear,
		VehicleMake:  body.VehicleMake,
		VehicleModel: body.VehicleModel,
		VehicleTrim:  body.VehicleTrim,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "SERVER_ERROR", err.Error())
		return
	}

	if body.Value != nil && *body.Value > 0 {
		if err := s.vehicle.UpdateValue(r.Context(), accountID, *body.Value); err != nil {
			writeError(w, http.StatusInternalServerError, "SERVER_ERROR", err.Error())
			return
		}
		if _, err := s.snapshots.Upsert(r.Context(), db.UpsertSnapshotParams{
			AccountID:    accountID,
			UserID:       userID,
			SnapshotDate: time.Now(),
			Balance:      *body.Value,
			Source:       "manual",
		}); err != nil {
			writeError(w, http.StatusInternalServerError, "SERVER_ERROR", err.Error())
			return
		}
		now := time.Now()
		d.LastValue = body.Value
		d.LastValueAt = &now
	}
	writeJSON(w, http.StatusOK, d)
}
