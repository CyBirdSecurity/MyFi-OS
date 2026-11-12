package api

import (
	"database/sql"
	"errors"
	"log"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/cybirdsecurity/myfi-os/internal/db"
	"github.com/cybirdsecurity/myfi-os/internal/models"
)

func (s *Server) handleListAccounts(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromCtx(r)
	accounts, err := s.accounts.List(r.Context(), userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "SERVER_ERROR", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, accounts)
}

func (s *Server) handleCreateAccount(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromCtx(r)
	var body struct {
		Name         string     `json:"name"`
		AccountType  string     `json:"accountType"`
		TaxTreatment string     `json:"taxTreatment"`
		Institution  *string    `json:"institution"`
		Currency     string     `json:"currency"`
		MemberID     *uuid.UUID `json:"memberId"`
		IsJoint      bool       `json:"isJoint"`
	}
	if err := decode(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid JSON")
		return
	}
	if body.Name == "" || body.AccountType == "" {
		writeError(w, http.StatusBadRequest, "VALIDATION", "name and accountType are required")
		return
	}
	if !models.ValidAccountTypes[body.AccountType] {
		writeError(w, http.StatusBadRequest, "VALIDATION", "invalid accountType")
		return
	}
	if body.IsJoint {
		if msg := s.validateJoint(r, userID, body.MemberID != nil); msg != "" {
			writeError(w, http.StatusBadRequest, "VALIDATION", msg)
			return
		}
	}
	if body.TaxTreatment == "" {
		body.TaxTreatment = models.AccountTypeTaxDefaults[body.AccountType]
	}
	currency := body.Currency
	if currency == "" {
		currency = "USD"
	}

	// Validate member_id belongs to this user if provided.
	if body.MemberID != nil {
		m, err := s.householdMembers.GetByID(r.Context(), *body.MemberID, userID)
		if err != nil || m == nil {
			writeError(w, http.StatusBadRequest, "VALIDATION", "member not found")
			return
		}
	}

	acc, err := s.accounts.Create(r.Context(), db.CreateAccountParams{
		UserID:       userID,
		Name:         body.Name,
		AccountType:  body.AccountType,
		TaxTreatment: body.TaxTreatment,
		Institution:  body.Institution,
		Currency:     currency,
		MemberID:     body.MemberID,
		IsJoint:      body.IsJoint,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "SERVER_ERROR", err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, acc)
}

// validateJoint returns a validation message when an account can't be made
// joint: it can't also name a member, and the household needs a partner.
func (s *Server) validateJoint(r *http.Request, userID uuid.UUID, hasMember bool) string {
	if hasMember {
		return "a joint account can't also belong to a single member"
	}
	members, err := s.householdMembers.ListByUser(r.Context(), userID)
	if err != nil {
		return "could not verify household"
	}
	for _, m := range members {
		if m.Relationship == "partner" {
			return ""
		}
	}
	return "add a partner to your household before creating joint accounts"
}

func (s *Server) handleGetAccount(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromCtx(r)
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid account id")
		return
	}
	acc, err := s.accounts.Get(r.Context(), id, userID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "NOT_FOUND", "account not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "SERVER_ERROR", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, acc)
}

func (s *Server) handleUpdateAccount(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromCtx(r)
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid account id")
		return
	}
	var body struct {
		Name         string     `json:"name"`
		AccountType  string     `json:"accountType"`
		TaxTreatment string     `json:"taxTreatment"`
		Institution  *string    `json:"institution"`
		MemberID     *uuid.UUID `json:"memberId"`
		ClearMember  bool       `json:"clearMember"`
		IsJoint      *bool      `json:"isJoint"`
	}
	if err := decode(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid JSON")
		return
	}
	if body.Name == "" || body.AccountType == "" {
		writeError(w, http.StatusBadRequest, "VALIDATION", "name and accountType are required")
		return
	}
	if !models.ValidAccountTypes[body.AccountType] {
		writeError(w, http.StatusBadRequest, "VALIDATION", "invalid accountType")
		return
	}
	if body.TaxTreatment == "" {
		body.TaxTreatment = models.AccountTypeTaxDefaults[body.AccountType]
	}

	params := db.UpdateAccountParams{
		ID:           id,
		UserID:       userID,
		Name:         body.Name,
		AccountType:  body.AccountType,
		TaxTreatment: body.TaxTreatment,
		Institution:  body.Institution,
	}

	// Only update ownership when explicitly provided or cleared. Joint and
	// member ownership are mutually exclusive, so both columns move together.
	if body.IsJoint != nil && *body.IsJoint {
		if msg := s.validateJoint(r, userID, body.MemberID != nil); msg != "" {
			writeError(w, http.StatusBadRequest, "VALIDATION", msg)
			return
		}
		params.SetMemberID = true
		params.MemberID = nil
		params.IsJoint = true
	} else if body.MemberID != nil || body.ClearMember {
		if body.MemberID != nil {
			m, err := s.householdMembers.GetByID(r.Context(), *body.MemberID, userID)
			if err != nil || m == nil {
				writeError(w, http.StatusBadRequest, "VALIDATION", "member not found")
				return
			}
		}
		params.SetMemberID = true
		params.MemberID = body.MemberID
	}

	// Capture the current name before the update so a rename can be propagated
	// to the denormalized account_label on this account's transactions.
	var oldName string
	if prev, getErr := s.accounts.Get(r.Context(), id, userID); getErr == nil && prev != nil {
		oldName = prev.Name
	}

	acc, err := s.accounts.Update(r.Context(), params)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "NOT_FOUND", "account not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "SERVER_ERROR", err.Error())
		return
	}

	// Propagate a rename to existing transactions' denormalized account_label so
	// they don't keep showing the old name (only auto-labeled rows are touched;
	// manual per-transaction overrides are preserved).
	if oldName != "" && oldName != acc.Name && s.transactions != nil {
		if _, relErr := s.transactions.RelabelByAccount(r.Context(), userID, id, oldName, acc.Name); relErr != nil {
			log.Printf("relabel transactions after account rename %s: %v", id, relErr)
		}
	}

	writeJSON(w, http.StatusOK, acc)
}

func (s *Server) handleDeleteAccount(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromCtx(r)
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid account id")
		return
	}
	if err := s.accounts.SoftDelete(r.Context(), id, userID); err != nil {
		// Distinguish "not owner" from "not found" by checking household access.
		acc, getErr := s.accounts.Get(r.Context(), id, userID)
		if getErr == nil && acc != nil && acc.IsPartnerAccount {
			writeError(w, http.StatusForbidden, "FORBIDDEN", "only the account owner can delete this account")
			return
		}
		writeError(w, http.StatusNotFound, "NOT_FOUND", "account not found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
