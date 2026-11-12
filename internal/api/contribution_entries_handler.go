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
	"github.com/cybirdsecurity/myfi-os/internal/services"
)

// handleContributionProgress compares this year's actual contributions with
// the plan for every retirement account that has any to compare.
func (s *Server) handleContributionProgress(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromCtx(r)
	ctx := r.Context()
	now := time.Now().UTC()

	accounts, err := s.accounts.List(ctx, userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "SERVER_ERROR", err.Error())
		return
	}
	plans, err := s.contributions.ListByUser(ctx, userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "SERVER_ERROR", err.Error())
		return
	}
	members, err := s.householdMembers.ListByUser(ctx, userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "SERVER_ERROR", err.Error())
		return
	}
	// A year back covers January 1st and shows whether a linked account's
	// institution reports contributions at all.
	entries, err := s.contributionEntries.ListByUser(ctx, userID, now.AddDate(-1, 0, 0))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "SERVER_ERROR", err.Error())
		return
	}
	services.OverlayOwnerIncome(plans, accounts, members)

	plansByAccount := make(map[uuid.UUID][]models.Contribution)
	for _, c := range plans {
		plansByAccount[c.AccountID] = append(plansByAccount[c.AccountID], c)
	}
	entriesByAccount := make(map[uuid.UUID][]models.ContributionEntry)
	for _, e := range entries {
		entriesByAccount[e.AccountID] = append(entriesByAccount[e.AccountID], e)
	}

	out := []services.ContributionProgress{}
	for _, acc := range accounts {
		if !models.RetirementAccountTypes[acc.AccountType] {
			continue
		}
		if p := services.ProgressForAccount(acc, plansByAccount[acc.ID], entriesByAccount[acc.ID], now); p != nil {
			out = append(out, *p)
		}
	}
	writeJSON(w, http.StatusOK, out)
}

// handleListContributionEntries returns an account's contributions this year.
func (s *Server) handleListContributionEntries(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromCtx(r)
	acc, ok := s.contributionEntryAccount(w, r, userID)
	if !ok {
		return
	}
	yearStart := time.Date(time.Now().UTC().Year(), 1, 1, 0, 0, 0, 0, time.UTC)
	entries, err := s.contributionEntries.ListByAccount(r.Context(), acc.ID, userID, yearStart)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "SERVER_ERROR", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, entries)
}

type contributionEntryBody struct {
	EntryDate   string  `json:"entryDate"`
	Amount      float64 `json:"amount"`
	Source      string  `json:"source"`
	Description *string `json:"description"`
}

// parse validates the body into store params. Returns "" when valid.
func (b contributionEntryBody) parse(acc *models.Account, userID uuid.UUID) (db.ContributionEntryParams, string) {
	p := db.ContributionEntryParams{AccountID: acc.ID, UserID: userID, Amount: b.Amount, Source: b.Source, Description: b.Description}
	d, err := time.Parse("2006-01-02", b.EntryDate)
	if err != nil {
		return p, "entryDate must be YYYY-MM-DD"
	}
	p.EntryDate = d
	if b.Amount <= 0 {
		return p, "amount must be greater than 0"
	}
	if b.Source != "employee" && b.Source != "employer" {
		return p, "source must be 'employee' or 'employer'"
	}
	return p, ""
}

// handleCreateContributionEntry logs a contribution by hand. Linked accounts
// get theirs from Plaid, so only manual accounts take them.
func (s *Server) handleCreateContributionEntry(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromCtx(r)
	acc, ok := s.contributionEntryAccount(w, r, userID)
	if !ok {
		return
	}
	if acc.ExternalProvider != nil {
		writeError(w, http.StatusBadRequest, "VALIDATION", "contributions to a linked account come from its sync")
		return
	}
	var body contributionEntryBody
	if err := decode(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid JSON")
		return
	}
	p, msg := body.parse(acc, userID)
	if msg != "" {
		writeError(w, http.StatusBadRequest, "VALIDATION", msg)
		return
	}
	e, err := s.contributionEntries.CreateManual(r.Context(), p)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "SERVER_ERROR", err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, e)
}

// handleUpdateContributionEntry edits a hand-logged entry. For a synced one,
// only whose money it was (source) can be corrected.
func (s *Server) handleUpdateContributionEntry(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromCtx(r)
	acc, ok := s.contributionEntryAccount(w, r, userID)
	if !ok {
		return
	}
	entryID, err := uuid.Parse(chi.URLParam(r, "eId"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid entry id")
		return
	}
	var body contributionEntryBody
	if err := decode(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid JSON")
		return
	}
	p, msg := body.parse(acc, userID)
	if msg != "" {
		writeError(w, http.StatusBadRequest, "VALIDATION", msg)
		return
	}
	e, err := s.contributionEntries.UpdateManual(r.Context(), entryID, p)
	if errors.Is(err, sql.ErrNoRows) {
		e, err = s.contributionEntries.SetSource(r.Context(), entryID, acc.ID, userID, p.Source)
	}
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "NOT_FOUND", "entry not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "SERVER_ERROR", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, e)
}

func (s *Server) handleDeleteContributionEntry(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromCtx(r)
	acc, ok := s.contributionEntryAccount(w, r, userID)
	if !ok {
		return
	}
	entryID, err := uuid.Parse(chi.URLParam(r, "eId"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid entry id")
		return
	}
	if err := s.contributionEntries.DeleteManual(r.Context(), entryID, acc.ID, userID); err != nil {
		writeError(w, http.StatusNotFound, "NOT_FOUND", "entry not found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// contributionEntryAccount loads the URL's account, checking the user can see
// it and that it's a retirement account. On failure it writes the error.
func (s *Server) contributionEntryAccount(w http.ResponseWriter, r *http.Request, userID uuid.UUID) (*models.Account, bool) {
	accountID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid account id")
		return nil, false
	}
	acc, err := s.accounts.Get(r.Context(), accountID, userID)
	if err != nil {
		writeError(w, http.StatusNotFound, "NOT_FOUND", "account not found")
		return nil, false
	}
	if !models.RetirementAccountTypes[acc.AccountType] {
		writeError(w, http.StatusBadRequest, "VALIDATION", "contributions are tracked for 401(k), 403(b), 457(b) and IRA accounts")
		return nil, false
	}
	return acc, true
}
