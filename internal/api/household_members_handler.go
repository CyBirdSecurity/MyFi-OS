package api

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/cybirdsecurity/myfi-os/internal/db"
)

// handleListHouseholdMembers returns all household members for the current user.
func (s *Server) handleListHouseholdMembers(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromCtx(r)
	members, err := s.householdMembers.ListByUser(r.Context(), userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "SERVER_ERROR", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, members)
}

// handleUpsertHouseholdMember creates or updates a household member.
// POST /household/members  — relationship must be 'self' or 'partner'
func (s *Server) handleUpsertHouseholdMember(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromCtx(r)

	var body struct {
		Relationship           string   `json:"relationship"`
		Name                   string   `json:"name"`
		Birthday               *string  `json:"birthday"`
		IsEmployed             *bool    `json:"isEmployed"`
		MonthlyIncome          *float64 `json:"monthlyIncome"`
		AnnualIncome           *float64 `json:"annualIncome"`
		SalaryGrowthPct        *float64 `json:"salaryGrowthPct"`
		RetirementAge          *int     `json:"retirementAge"`
		UseSocialSecurity      *bool    `json:"useSocialSecurity"`
		SocialSecurityMonthly  *float64 `json:"socialSecurityMonthly"`
		SocialSecurityStartAge *int     `json:"socialSecurityStartAge"`
	}
	if err := decode(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid JSON")
		return
	}
	if body.Relationship != "self" && body.Relationship != "partner" {
		writeError(w, http.StatusBadRequest, "VALIDATION", "relationship must be 'self' or 'partner'")
		return
	}

	// Check max 2 members (one 'self', one 'partner').
	existing, err := s.householdMembers.ListByUser(r.Context(), userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "SERVER_ERROR", err.Error())
		return
	}
	hasRelationship := false
	for _, m := range existing {
		if m.Relationship == body.Relationship {
			hasRelationship = true
		}
	}
	if !hasRelationship && len(existing) >= 2 {
		writeError(w, http.StatusBadRequest, "VALIDATION", "household already has maximum members")
		return
	}

	params := db.UpsertMemberParams{
		UserID:       userID,
		Relationship: body.Relationship,
		Name:         body.Name,
		IsEmployed:   true,
	}

	if body.Birthday != nil && *body.Birthday != "" {
		d, err := time.Parse("2006-01-02", *body.Birthday)
		if err != nil {
			writeError(w, http.StatusBadRequest, "VALIDATION", "birthday must be YYYY-MM-DD")
			return
		}
		params.Birthday = &d
	}
	if body.IsEmployed != nil {
		params.IsEmployed = *body.IsEmployed
	}
	// Accept either monthlyIncome or annualIncome.
	if body.MonthlyIncome != nil {
		params.MonthlyIncome = *body.MonthlyIncome
	} else if body.AnnualIncome != nil {
		params.MonthlyIncome = *body.AnnualIncome / 12
	}
	if body.SalaryGrowthPct != nil {
		params.SalaryGrowthPct = *body.SalaryGrowthPct
	}
	if body.RetirementAge != nil {
		params.RetirementAge = *body.RetirementAge
	}
	if body.UseSocialSecurity != nil {
		params.UseSocialSecurity = *body.UseSocialSecurity
	}
	if body.SocialSecurityMonthly != nil {
		params.SocialSecurityMonthly = *body.SocialSecurityMonthly
	}
	if body.SocialSecurityStartAge != nil {
		params.SocialSecurityStartAge = *body.SocialSecurityStartAge
	}

	member, err := s.householdMembers.Upsert(r.Context(), params)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "SERVER_ERROR", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, member)
}

// handleUpdateHouseholdMember updates a specific household member by ID.
// PUT /household/members/:id
func (s *Server) handleUpdateHouseholdMember(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromCtx(r)
	memberID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid member id")
		return
	}

	existing, err := s.householdMembers.GetByID(r.Context(), memberID, userID)
	if err != nil || existing == nil {
		writeError(w, http.StatusNotFound, "NOT_FOUND", "member not found")
		return
	}

	var body struct {
		Name                   string   `json:"name"`
		Birthday               *string  `json:"birthday"`
		IsEmployed             *bool    `json:"isEmployed"`
		MonthlyIncome          *float64 `json:"monthlyIncome"`
		AnnualIncome           *float64 `json:"annualIncome"`
		SalaryGrowthPct        *float64 `json:"salaryGrowthPct"`
		RetirementAge          *int     `json:"retirementAge"`
		UseSocialSecurity      *bool    `json:"useSocialSecurity"`
		SocialSecurityMonthly  *float64 `json:"socialSecurityMonthly"`
		SocialSecurityStartAge *int     `json:"socialSecurityStartAge"`
	}
	if err := decode(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid JSON")
		return
	}

	params := db.UpsertMemberParams{
		UserID:                 userID,
		Relationship:           existing.Relationship,
		Name:                   existing.Name,
		Birthday:               existing.Birthday,
		IsEmployed:             existing.IsEmployed,
		MonthlyIncome:          existing.MonthlyIncome,
		SalaryGrowthPct:        existing.SalaryGrowthPct,
		RetirementAge:          existing.RetirementAge,
		UseSocialSecurity:      existing.UseSocialSecurity,
		SocialSecurityMonthly:  existing.SocialSecurityMonthly,
		SocialSecurityStartAge: existing.SocialSecurityStartAge,
	}

	if body.Name != "" {
		params.Name = body.Name
	}
	if body.Birthday != nil {
		if *body.Birthday == "" {
			params.Birthday = nil
		} else {
			d, err := time.Parse("2006-01-02", *body.Birthday)
			if err != nil {
				writeError(w, http.StatusBadRequest, "VALIDATION", "birthday must be YYYY-MM-DD")
				return
			}
			params.Birthday = &d
		}
	}
	if body.IsEmployed != nil {
		params.IsEmployed = *body.IsEmployed
	}
	if body.MonthlyIncome != nil {
		params.MonthlyIncome = *body.MonthlyIncome
	} else if body.AnnualIncome != nil {
		params.MonthlyIncome = *body.AnnualIncome / 12
	}
	if body.SalaryGrowthPct != nil {
		params.SalaryGrowthPct = *body.SalaryGrowthPct
	}
	if body.RetirementAge != nil {
		params.RetirementAge = *body.RetirementAge
	}
	if body.UseSocialSecurity != nil {
		params.UseSocialSecurity = *body.UseSocialSecurity
	}
	if body.SocialSecurityMonthly != nil {
		params.SocialSecurityMonthly = *body.SocialSecurityMonthly
	}
	if body.SocialSecurityStartAge != nil {
		params.SocialSecurityStartAge = *body.SocialSecurityStartAge
	}

	member, err := s.householdMembers.Upsert(r.Context(), params)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "SERVER_ERROR", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, member)
}

// handleDeleteHouseholdMember removes the partner member. 'self' cannot be deleted.
func (s *Server) handleDeleteHouseholdMember(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromCtx(r)
	memberID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid member id")
		return
	}
	if err := s.householdMembers.Delete(r.Context(), memberID, userID); err != nil {
		writeError(w, http.StatusNotFound, "NOT_FOUND", err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
