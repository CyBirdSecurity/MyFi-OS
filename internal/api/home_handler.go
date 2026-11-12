package api

import (
	"database/sql"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/cybirdsecurity/myfi-os/internal/db"
	"github.com/cybirdsecurity/myfi-os/internal/models"
	"github.com/cybirdsecurity/myfi-os/internal/services"
)

func (s *Server) handleGetHomeDetails(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromCtx(r)
	accountID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid account id")
		return
	}

	d, err := s.home.Get(r.Context(), accountID, userID)
	if errors.Is(err, sql.ErrNoRows) {
		writeJSON(w, http.StatusOK, &models.HomeDetails{AccountID: accountID, UserID: userID})
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "SERVER_ERROR", err.Error())
		return
	}

	services.RollForwardLoanBalance(d, time.Now())
	computeEquity(d)
	writeJSON(w, http.StatusOK, d)
}

func (s *Server) handleUpsertHomeDetails(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromCtx(r)
	accountID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid account id")
		return
	}

	var body struct {
		Address                  string   `json:"address"`
		InterestRate             float64  `json:"interestRate"`
		LoanTermYears            int      `json:"loanTermYears"`
		OriginalLoanValue        float64  `json:"originalLoanValue"`
		CurrentLoanBalance       float64  `json:"currentLoanBalance"`
		MortgageStartDate        string   `json:"mortgageStartDate,omitempty"`
		ExtraMonthlyPayment      float64  `json:"extraMonthlyPayment"`
		TrackMortgageAsLiability bool     `json:"trackMortgageAsLiability"`
		IncludeInRetirementCalcs *bool    `json:"includeInRetirementCalcs,omitempty"`
		HomeValue                *float64 `json:"homeValue,omitempty"` // optional manual property value
		PropertyTaxAnnual        float64  `json:"propertyTaxAnnual"`
		HomeInsuranceAnnual      float64  `json:"homeInsuranceAnnual"`
		HOAMonthly               float64  `json:"hoaMonthly"`
		PMIMonthly               float64  `json:"pmiMonthly"`
		PurchasePrice            float64  `json:"purchasePrice"`
	}
	if err := decode(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid JSON")
		return
	}

	var mortgageStartDate *time.Time
	if body.MortgageStartDate != "" {
		if t, err2 := time.Parse("2006-01-02", body.MortgageStartDate); err2 == nil {
			mortgageStartDate = &t
		}
	}

	includeInRetirementCalcs := false
	if body.IncludeInRetirementCalcs != nil {
		includeInRetirementCalcs = *body.IncludeInRetirementCalcs
	} else if existing, existErr := s.home.Get(r.Context(), accountID, userID); existErr == nil {
		includeInRetirementCalcs = existing.IncludeInRetirementCalcs
	}

	d, err := s.home.Upsert(r.Context(), db.UpsertHomeParams{
		AccountID:                accountID,
		UserID:                   userID,
		Address:                  body.Address,
		InterestRate:             body.InterestRate,
		LoanTermYears:            body.LoanTermYears,
		OriginalLoanValue:        body.OriginalLoanValue,
		CurrentLoanBalance:       body.CurrentLoanBalance,
		MortgageStartDate:        mortgageStartDate,
		ExtraMonthlyPayment:      body.ExtraMonthlyPayment,
		TrackMortgageAsLiability: body.TrackMortgageAsLiability,
		IncludeInRetirementCalcs: includeInRetirementCalcs,
		PropertyTaxAnnual:        body.PropertyTaxAnnual,
		HomeInsuranceAnnual:      body.HomeInsuranceAnnual,
		HOAMonthly:               body.HOAMonthly,
		PMIMonthly:               body.PMIMonthly,
		PurchasePrice:            body.PurchasePrice,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "SERVER_ERROR", err.Error())
		return
	}

	// If the caller provided a manual home value, persist it now.
	if body.HomeValue != nil && *body.HomeValue > 0 {
		if err := s.home.UpdateZestimate(r.Context(), accountID, *body.HomeValue); err != nil {
			writeError(w, http.StatusInternalServerError, "SERVER_ERROR", err.Error())
			return
		}
		d.LastZestimate = body.HomeValue
	}

	services.RollForwardLoanBalance(d, time.Now())
	computeEquity(d)

	if d.Equity != nil {
		_, _ = s.snapshots.Upsert(r.Context(), db.UpsertSnapshotParams{
			AccountID:    accountID,
			UserID:       userID,
			SnapshotDate: time.Now(),
			Balance:      *d.Equity,
			Source:       "computed",
		})
	}

	writeJSON(w, http.StatusOK, d)
}

func (s *Server) handleGetAmortization(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromCtx(r)
	accountID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid account id")
		return
	}

	d, err := s.home.Get(r.Context(), accountID, userID)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "NOT_FOUND", "no home details found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "SERVER_ERROR", err.Error())
		return
	}

	services.RollForwardLoanBalance(d, time.Now())

	// Default to the saved extra payment; allow a query-param override for what-if.
	extra := d.ExtraMonthlyPayment
	if qp := r.URL.Query().Get("extra"); qp != "" {
		if v, parseErr := strconv.ParseFloat(qp, 64); parseErr == nil && v >= 0 {
			extra = v
		}
	}

	result := services.BuildAmortizationSchedule(d, extra)
	writeJSON(w, http.StatusOK, result)
}

// computeEquity populates d.Equity when a zestimate is available.
func computeEquity(d *models.HomeDetails) {
	if d.LastZestimate != nil && *d.LastZestimate > 0 {
		eq := *d.LastZestimate - d.CurrentLoanBalance
		d.Equity = &eq
	}
}
