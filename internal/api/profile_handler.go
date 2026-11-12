package api

import (
	"net/http"

	"github.com/cybirdsecurity/myfi-os/internal/db"
	"github.com/cybirdsecurity/myfi-os/internal/models"
)

func (s *Server) handleGetProfile(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromCtx(r)
	profile, err := s.profiles.Get(r.Context(), userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "SERVER_ERROR", err.Error())
		return
	}

	members, _ := s.householdMembers.ListByUser(r.Context(), userID)

	if profile == nil {
		profile = &models.UserProfile{
			UserID:                 userID,
			InflationRate:          0.03,
			FilingStatus:           "single",
			RetirementAge:          65,
			SocialSecurityStartAge: 67,
		}
	}

	overlayMembers(profile, members)
	writeJSON(w, http.StatusOK, profile)
}

func (s *Server) handleUpsertProfile(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromCtx(r)

	var body struct {
		UseInflationAdjustment *bool    `json:"useInflationAdjustment"`
		InflationRate          *float64 `json:"inflationRate"`
		MonthlyExpenses        *float64 `json:"monthlyExpenses"`
		CurrentMonthlyExpenses *float64 `json:"currentMonthlyExpenses"`
		UseBudgetForExpenses   *bool    `json:"useBudgetForExpenses"`
		WithdrawalRate         *float64 `json:"withdrawalRate"`
		FilingStatus           *string  `json:"filingStatus"`
		StateTaxRate           *float64 `json:"stateTaxRate"`
		StateTaxCode           *string  `json:"stateTaxCode"`
		DependentsCount        *int     `json:"dependentsCount"`
	}
	if err := decode(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid JSON")
		return
	}

	existing, _ := s.profiles.Get(r.Context(), userID)

	params := db.UpsertProfileParams{
		UserID:        userID,
		InflationRate: 0.03,
		FilingStatus:  "single",
	}
	if existing != nil {
		params.UseInflationAdjustment = existing.UseInflationAdjustment
		params.InflationRate = existing.InflationRate
		params.MonthlyExpenses = existing.MonthlyExpenses
		params.CurrentMonthlyExpenses = existing.CurrentMonthlyExpenses
		params.UseBudgetForExpenses = existing.UseBudgetForExpenses
		params.WithdrawalRate = existing.WithdrawalRate
		params.FilingStatus = existing.FilingStatus
		params.StateTaxRate = existing.StateTaxRate
		params.StateTaxCode = existing.StateTaxCode
		params.DependentsCount = existing.DependentsCount
	}

	if body.UseInflationAdjustment != nil {
		params.UseInflationAdjustment = *body.UseInflationAdjustment
	}
	if body.InflationRate != nil {
		params.InflationRate = *body.InflationRate
	}
	if body.MonthlyExpenses != nil {
		params.MonthlyExpenses = *body.MonthlyExpenses
	}
	if body.CurrentMonthlyExpenses != nil {
		params.CurrentMonthlyExpenses = *body.CurrentMonthlyExpenses
	}
	if body.UseBudgetForExpenses != nil {
		params.UseBudgetForExpenses = *body.UseBudgetForExpenses
	}
	if body.WithdrawalRate != nil {
		if *body.WithdrawalRate == 0 {
			params.WithdrawalRate = nil
		} else {
			params.WithdrawalRate = body.WithdrawalRate
		}
	}
	if body.FilingStatus != nil {
		params.FilingStatus = *body.FilingStatus
	}
	if body.StateTaxRate != nil {
		if *body.StateTaxRate == 0 {
			params.StateTaxRate = nil
		} else {
			params.StateTaxRate = body.StateTaxRate
		}
	}
	if body.StateTaxCode != nil {
		if *body.StateTaxCode == "" {
			params.StateTaxCode = nil
		} else {
			params.StateTaxCode = body.StateTaxCode
		}
	}
	if body.DependentsCount != nil && *body.DependentsCount >= 0 {
		params.DependentsCount = *body.DependentsCount
	}

	out, err := s.profiles.Upsert(r.Context(), params)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "SERVER_ERROR", err.Error())
		return
	}

	members, _ := s.householdMembers.ListByUser(r.Context(), userID)
	overlayMembers(out, members)
	writeJSON(w, http.StatusOK, out)
}

// overlayMembers fills the db:"-" per-person fields on profile from household members.
func overlayMembers(p *models.UserProfile, members []models.HouseholdMember) {
	p.RetirementAge = 65
	p.SocialSecurityStartAge = 67
	p.HouseholdSize = p.DependentsCount + len(members)
	for _, m := range members {
		if m.Relationship == "self" {
			p.Birthday = m.Birthday
			p.RetirementAge = m.RetirementAge
			p.UseSocialSecurity = m.UseSocialSecurity
			p.SocialSecurityMonthly = m.SocialSecurityMonthly
			p.SocialSecurityStartAge = m.SocialSecurityStartAge
		}
		if m.IsEmployed {
			p.MonthlyIncome += m.MonthlyIncome
		}
	}
}
