package api

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/cybirdsecurity/myfi-os/internal/db"
)

func (s *Server) handleListGoals(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromCtx(r)
	goals, err := s.goals.List(r.Context(), userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "SERVER_ERROR", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, goals)
}

func (s *Server) handleCreateGoal(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromCtx(r)
	var body struct {
		Name              string  `json:"name"`
		GoalType          string  `json:"goalType"`
		AccountID         *string `json:"accountId"`
		AccountTypeFilter *string `json:"accountTypeFilter"`
		AssetClassFilter  *string `json:"assetClassFilter"`
		TargetAmount      float64 `json:"targetAmount"`
		TargetDate        *string `json:"targetDate"`
		Notes             *string `json:"notes"`
	}
	if err := decode(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid JSON")
		return
	}
	if body.Name == "" || body.TargetAmount <= 0 {
		writeError(w, http.StatusBadRequest, "VALIDATION", "name and positive targetAmount are required")
		return
	}
	if body.GoalType == "" {
		body.GoalType = "net_worth"
	}
	validTypes := map[string]bool{"net_worth": true, "account": true, "account_type": true, "asset_class": true, "spend": true}
	if !validTypes[body.GoalType] {
		writeError(w, http.StatusBadRequest, "VALIDATION", "invalid goalType")
		return
	}
	if body.GoalType == "account" && body.AccountID == nil {
		writeError(w, http.StatusBadRequest, "VALIDATION", "accountId is required for account goals")
		return
	}
	if body.GoalType == "account_type" && (body.AccountTypeFilter == nil || *body.AccountTypeFilter == "") {
		writeError(w, http.StatusBadRequest, "VALIDATION", "accountTypeFilter is required for account_type goals")
		return
	}
	if body.GoalType == "asset_class" && (body.AssetClassFilter == nil || *body.AssetClassFilter == "") {
		writeError(w, http.StatusBadRequest, "VALIDATION", "assetClassFilter is required for asset_class goals")
		return
	}
	if body.GoalType == "spend" && body.TargetDate == nil {
		writeError(w, http.StatusBadRequest, "VALIDATION", "targetDate is required for spend goals")
		return
	}

	var accountID *uuid.UUID
	if body.AccountID != nil {
		parsed, err := uuid.Parse(*body.AccountID)
		if err != nil {
			writeError(w, http.StatusBadRequest, "VALIDATION", "invalid accountId")
			return
		}
		accountID = &parsed
	}

	var targetDate *time.Time
	if body.TargetDate != nil {
		d, err := time.Parse("2006-01-02", *body.TargetDate)
		if err != nil {
			writeError(w, http.StatusBadRequest, "VALIDATION", "targetDate must be YYYY-MM-DD")
			return
		}
		targetDate = &d
	}

	g, err := s.goals.Create(r.Context(), db.CreateGoalParams{
		UserID:            userID,
		Name:              body.Name,
		GoalType:          body.GoalType,
		AccountID:         accountID,
		AccountTypeFilter: body.AccountTypeFilter,
		AssetClassFilter:  body.AssetClassFilter,
		TargetAmount:      body.TargetAmount,
		TargetDate:        targetDate,
		Notes:             body.Notes,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "SERVER_ERROR", err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, g)
}

func (s *Server) handleUpdateGoal(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromCtx(r)
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid goal id")
		return
	}
	var body struct {
		Name              string  `json:"name"`
		GoalType          string  `json:"goalType"`
		AccountID         *string `json:"accountId"`
		AccountTypeFilter *string `json:"accountTypeFilter"`
		AssetClassFilter  *string `json:"assetClassFilter"`
		TargetAmount      float64 `json:"targetAmount"`
		TargetDate        *string `json:"targetDate"`
		Notes             *string `json:"notes"`
		IsAchieved        bool    `json:"isAchieved"`
	}
	if err := decode(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid JSON")
		return
	}
	if body.GoalType == "" {
		body.GoalType = "net_worth"
	}

	var accountID *uuid.UUID
	if body.AccountID != nil {
		parsed, err := uuid.Parse(*body.AccountID)
		if err != nil {
			writeError(w, http.StatusBadRequest, "VALIDATION", "invalid accountId")
			return
		}
		accountID = &parsed
	}

	var targetDate *time.Time
	if body.TargetDate != nil {
		d, err := time.Parse("2006-01-02", *body.TargetDate)
		if err != nil {
			writeError(w, http.StatusBadRequest, "VALIDATION", "targetDate must be YYYY-MM-DD")
			return
		}
		targetDate = &d
	}

	g, err := s.goals.Update(r.Context(), db.UpdateGoalParams{
		ID:                id,
		UserID:            userID,
		Name:              body.Name,
		GoalType:          body.GoalType,
		AccountID:         accountID,
		AccountTypeFilter: body.AccountTypeFilter,
		AssetClassFilter:  body.AssetClassFilter,
		TargetAmount:      body.TargetAmount,
		TargetDate:        targetDate,
		Notes:             body.Notes,
		IsAchieved:        body.IsAchieved,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "SERVER_ERROR", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, g)
}

func (s *Server) handleDeleteGoal(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromCtx(r)
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid goal id")
		return
	}
	if err := s.goals.Delete(r.Context(), id, userID); err != nil {
		writeError(w, http.StatusNotFound, "NOT_FOUND", "goal not found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
