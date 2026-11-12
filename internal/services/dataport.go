package services

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/cybirdsecurity/myfi-os/internal/db"
	"github.com/cybirdsecurity/myfi-os/internal/models"
)

const dataPortVersion = 1

// ExportBundle is the full portable snapshot of a user's account data.
type ExportBundle struct {
	ExportedAt        time.Time                `json:"exportedAt"`
	Version           int                      `json:"version"`
	Accounts          []AccountExport          `json:"accounts"`
	Goals             []models.Goal            `json:"goals"`
	Profile           *models.UserProfile      `json:"profile"`
	AllocationTargets *models.AllocationTarget `json:"allocationTargets"`
	ReturnRates       []models.UserReturnRate  `json:"returnRates"`
}

// AccountExport bundles an account with all its sub-records.
type AccountExport struct {
	models.Account
	Snapshots      []models.BalanceSnapshot `json:"snapshots"`
	Holdings       []models.Holding         `json:"holdings"`
	Contributions  []ContributionExport     `json:"contributions"`
	HomeDetails    *models.HomeDetails      `json:"homeDetails,omitempty"`
	VehicleDetails *models.VehicleDetails   `json:"vehicleDetails,omitempty"`
}

// ContributionExport is a contribution as exported. EmployerMatchPct is only
// read from files exported before match tiers existed.
type ContributionExport struct {
	models.Contribution
	EmployerMatchPct float64 `json:"employerMatchPct,omitempty"`
}

// ImportSummary describes what was created or skipped during an import.
type ImportSummary struct {
	AccountsCreated       int      `json:"accountsCreated"`
	AccountsMatched       int      `json:"accountsMatched"`
	SnapshotsUpserted     int      `json:"snapshotsUpserted"`
	HoldingsRestored      int      `json:"holdingsRestored"`
	ContributionsRestored int      `json:"contributionsRestored"`
	GoalsRestored         int      `json:"goalsRestored"`
	Warnings              []string `json:"warnings,omitempty"`
}

type DataPortService struct {
	accounts          *db.AccountStore
	snapshots         *db.SnapshotStore
	holdings          *db.HoldingStore
	contributions     *db.ContributionStore
	goals             *db.GoalStore
	profiles          *db.ProfileStore
	allocationTargets *db.AllocationTargetStore
	returnRates       *db.ReturnRateStore
	home              *db.HomeStore
	vehicle           *db.VehicleStore
	householdMembers  *db.HouseholdMembersStore
}

func NewDataPortService(
	accounts *db.AccountStore,
	snapshots *db.SnapshotStore,
	holdings *db.HoldingStore,
	contributions *db.ContributionStore,
	goals *db.GoalStore,
	profiles *db.ProfileStore,
	allocationTargets *db.AllocationTargetStore,
	returnRates *db.ReturnRateStore,
	home *db.HomeStore,
	vehicle *db.VehicleStore,
	householdMembers *db.HouseholdMembersStore,
) *DataPortService {
	return &DataPortService{
		accounts: accounts, snapshots: snapshots, holdings: holdings,
		contributions: contributions, goals: goals, profiles: profiles,
		allocationTargets: allocationTargets, returnRates: returnRates,
		home: home, vehicle: vehicle, householdMembers: householdMembers,
	}
}

func (s *DataPortService) Export(ctx context.Context, userID uuid.UUID) (*ExportBundle, error) {
	allAccounts, err := s.accounts.List(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("list accounts: %w", err)
	}

	bundle := &ExportBundle{
		ExportedAt: time.Now().UTC(),
		Version:    dataPortVersion,
	}

	for _, acc := range allAccounts {
		ae := AccountExport{Account: acc}

		snaps, err := s.snapshots.List(ctx, acc.ID, userID)
		if err != nil {
			return nil, fmt.Errorf("list snapshots for account %s: %w", acc.Name, err)
		}
		ae.Snapshots = snaps

		holdings, err := s.holdings.ListByAccount(ctx, acc.ID, userID)
		if err != nil {
			return nil, fmt.Errorf("list holdings for account %s: %w", acc.Name, err)
		}
		ae.Holdings = holdings

		contribs, err := s.contributions.ListByAccount(ctx, acc.ID, userID)
		if err != nil {
			return nil, fmt.Errorf("list contributions for account %s: %w", acc.Name, err)
		}
		for _, c := range contribs {
			ae.Contributions = append(ae.Contributions, ContributionExport{Contribution: c})
		}

		if acc.AccountType == "home" {
			if hd, err := s.home.Get(ctx, acc.ID, userID); err == nil {
				ae.HomeDetails = hd
			}
		}
		if acc.AccountType == "automobile" {
			if vd, err := s.vehicle.Get(ctx, acc.ID, userID); err == nil {
				ae.VehicleDetails = vd
			}
		}

		bundle.Accounts = append(bundle.Accounts, ae)
	}

	goals, err := s.goals.List(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("list goals: %w", err)
	}
	bundle.Goals = goals

	profile, err := s.profiles.Get(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("get profile: %w", err)
	}
	bundle.Profile = profile

	at, err := s.allocationTargets.Get(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("get allocation targets: %w", err)
	}
	bundle.AllocationTargets = at

	rr, err := s.returnRates.ListByUser(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("list return rates: %w", err)
	}
	bundle.ReturnRates = rr

	return bundle, nil
}

func (s *DataPortService) Import(ctx context.Context, userID uuid.UUID, bundle *ExportBundle) (*ImportSummary, error) {
	summary := &ImportSummary{}

	existingAccounts, err := s.accounts.List(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("list existing accounts: %w", err)
	}

	accountKey := func(name, accType string) string {
		return strings.ToLower(strings.TrimSpace(name)) + "|" + accType
	}

	existingByKey := make(map[string]*models.Account, len(existingAccounts))
	for i := range existingAccounts {
		k := accountKey(existingAccounts[i].Name, existingAccounts[i].AccountType)
		existingByKey[k] = &existingAccounts[i]
	}

	// Member IDs differ between users, so partner ownership is restored by role.
	var partnerID *uuid.UUID
	if members, err := s.householdMembers.ListByUser(ctx, userID); err == nil {
		for i := range members {
			if members[i].Relationship == "partner" {
				partnerID = &members[i].ID
			}
		}
	}

	// oldToNew maps the exported account ID to the live account ID used for goal resolution.
	oldToNew := make(map[uuid.UUID]uuid.UUID, len(bundle.Accounts))

	for _, ae := range bundle.Accounts {
		k := accountKey(ae.Name, ae.AccountType)
		var targetID uuid.UUID

		if existing, ok := existingByKey[k]; ok {
			targetID = existing.ID
			summary.AccountsMatched++
		} else {
			params := db.CreateAccountParams{
				UserID:       userID,
				Name:         ae.Name,
				AccountType:  ae.AccountType,
				TaxTreatment: ae.TaxTreatment,
				Institution:  ae.Institution,
				Currency:     ae.Currency,
			}
			if ae.IsJoint || ae.IsPartnerAccount {
				if partnerID == nil {
					summary.Warnings = append(summary.Warnings, fmt.Sprintf("account %q was not owned by you but this household has no partner; imported as yours", ae.Name))
				} else if ae.IsJoint {
					params.IsJoint = true
				} else {
					params.MemberID = partnerID
				}
			}
			created, err := s.accounts.Create(ctx, params)
			if err != nil {
				summary.Warnings = append(summary.Warnings, fmt.Sprintf("could not create account %q: %v", ae.Name, err))
				continue
			}
			targetID = created.ID
			summary.AccountsCreated++
		}

		oldToNew[ae.ID] = targetID

		if len(ae.Snapshots) > 0 {
			params := make([]db.UpsertSnapshotParams, len(ae.Snapshots))
			for i, snap := range ae.Snapshots {
				params[i] = db.UpsertSnapshotParams{
					AccountID:    targetID,
					UserID:       userID,
					SnapshotDate: snap.SnapshotDate,
					Balance:      snap.Balance,
					Miles:        snap.Miles,
					Notes:        snap.Notes,
					Source:       "import",
				}
			}
			n, err := s.snapshots.BulkUpsert(ctx, params)
			if err != nil {
				summary.Warnings = append(summary.Warnings, fmt.Sprintf("snapshots for %q: %v", ae.Name, err))
			} else {
				summary.SnapshotsUpserted += n
			}
		}

		if len(ae.Holdings) > 0 {
			params := make([]db.CreateHoldingParams, len(ae.Holdings))
			for i, h := range ae.Holdings {
				params[i] = db.CreateHoldingParams{
					AccountID:     targetID,
					UserID:        userID,
					Ticker:        h.Ticker,
					Name:          h.Name,
					AssetClass:    h.AssetClass,
					Shares:        h.Shares,
					CostBasis:     h.CostBasis,
					ManualPrice:   h.ManualPrice,
					PricePerShare: h.PricePerShare,
				}
			}
			n, err := s.holdings.ReplaceForTickers(ctx, targetID, userID, params)
			if err != nil {
				summary.Warnings = append(summary.Warnings, fmt.Sprintf("holdings for %q: %v", ae.Name, err))
			} else {
				summary.HoldingsRestored += n
			}
		}

		if len(ae.Contributions) > 0 {
			// Deduplicate by (label, amount, frequency) against existing contributions.
			existing, _ := s.contributions.ListByAccount(ctx, targetID, userID)
			existingContribKeys := make(map[string]bool, len(existing))
			for _, ec := range existing {
				existingContribKeys[contribKey(ec.Label, ec.Amount, ec.Frequency)] = true
			}
			for _, c := range ae.Contributions {
				if existingContribKeys[contribKey(c.Label, c.Amount, c.Frequency)] {
					continue
				}
				if c.AmountType == "" {
					c.AmountType = "fixed"
				}
				if len(c.MatchTiers) == 0 && c.EmployerMatchPct > 0 {
					c.MatchTiers = models.MatchTiers{{MatchPct: c.EmployerMatchPct}}
				}
				_, err := s.contributions.Create(ctx, db.CreateContributionParams{
					AccountID:         targetID,
					UserID:            userID,
					Label:             c.Label,
					Amount:            c.Amount,
					Frequency:         c.Frequency,
					AnnualIncreasePct: c.AnnualIncreasePct,
					AmountType:        c.AmountType,
					IncomePct:         c.IncomePct,
					MatchTiers:        c.MatchTiers,
					NextPayDate:       c.NextPayDate,
					StartDate:         c.StartDate,
					EndDate:           c.EndDate,
				})
				if err != nil {
					summary.Warnings = append(summary.Warnings, fmt.Sprintf("contribution for %q: %v", ae.Name, err))
				} else {
					summary.ContributionsRestored++
				}
			}
		}

		if ae.HomeDetails != nil {
			_, err := s.home.Upsert(ctx, db.UpsertHomeParams{
				AccountID:                targetID,
				UserID:                   userID,
				Address:                  ae.HomeDetails.Address,
				InterestRate:             ae.HomeDetails.InterestRate,
				LoanTermYears:            ae.HomeDetails.LoanTermYears,
				OriginalLoanValue:        ae.HomeDetails.OriginalLoanValue,
				CurrentLoanBalance:       ae.HomeDetails.CurrentLoanBalance,
				MortgageStartDate:        ae.HomeDetails.MortgageStartDate,
				ExtraMonthlyPayment:      ae.HomeDetails.ExtraMonthlyPayment,
				IncludeInRetirementCalcs: ae.HomeDetails.IncludeInRetirementCalcs,
				PropertyTaxAnnual:        ae.HomeDetails.PropertyTaxAnnual,
				HomeInsuranceAnnual:      ae.HomeDetails.HomeInsuranceAnnual,
				HOAMonthly:               ae.HomeDetails.HOAMonthly,
				PMIMonthly:               ae.HomeDetails.PMIMonthly,
				PurchasePrice:            ae.HomeDetails.PurchasePrice,
			})
			if err != nil {
				summary.Warnings = append(summary.Warnings, fmt.Sprintf("home details for %q: %v", ae.Name, err))
			}
		}

		if ae.VehicleDetails != nil {
			_, err := s.vehicle.Upsert(ctx, db.UpsertVehicleParams{
				AccountID:    targetID,
				UserID:       userID,
				VehicleYear:  ae.VehicleDetails.VehicleYear,
				VehicleMake:  ae.VehicleDetails.VehicleMake,
				VehicleModel: ae.VehicleDetails.VehicleModel,
				VehicleTrim:  ae.VehicleDetails.VehicleTrim,
			})
			if err != nil {
				summary.Warnings = append(summary.Warnings, fmt.Sprintf("vehicle details for %q: %v", ae.Name, err))
			}
		}
	}

	// Goals — deduplicate by (name, goalType); resolve account ID references via oldToNew.
	existingGoals, _ := s.goals.List(ctx, userID)
	existingGoalKeys := make(map[string]bool, len(existingGoals))
	for _, eg := range existingGoals {
		existingGoalKeys[goalKey(eg.Name, eg.GoalType)] = true
	}
	for _, g := range bundle.Goals {
		if existingGoalKeys[goalKey(g.Name, g.GoalType)] {
			continue
		}
		var accountID *uuid.UUID
		if g.AccountID != nil {
			if newID, ok := oldToNew[*g.AccountID]; ok {
				accountID = &newID
			}
		}
		_, err := s.goals.Create(ctx, db.CreateGoalParams{
			UserID:            userID,
			Name:              g.Name,
			GoalType:          g.GoalType,
			AccountID:         accountID,
			AccountTypeFilter: g.AccountTypeFilter,
			AssetClassFilter:  g.AssetClassFilter,
			TargetAmount:      g.TargetAmount,
			TargetDate:        g.TargetDate,
			Notes:             g.Notes,
		})
		if err != nil {
			summary.Warnings = append(summary.Warnings, fmt.Sprintf("goal %q: %v", g.Name, err))
		} else {
			summary.GoalsRestored++
		}
	}

	// Profile, allocation targets, and return rates are always upserted.
	if bundle.Profile != nil {
		p := bundle.Profile
		_, err := s.profiles.Upsert(ctx, db.UpsertProfileParams{
			UserID:                 userID,
			UseInflationAdjustment: p.UseInflationAdjustment,
			InflationRate:          p.InflationRate,
			MonthlyExpenses:        p.MonthlyExpenses,
			CurrentMonthlyExpenses: p.CurrentMonthlyExpenses,
			UseBudgetForExpenses:   p.UseBudgetForExpenses,
		})
		if err != nil {
			summary.Warnings = append(summary.Warnings, fmt.Sprintf("profile: %v", err))
		}
	}

	if bundle.AllocationTargets != nil {
		at := bundle.AllocationTargets
		_, err := s.allocationTargets.Upsert(ctx, db.UpsertAllocationTargetParams{
			UserID:    userID,
			EquityPct: at.EquityPct,
			BondPct:   at.BondPct,
			CryptoPct: at.CryptoPct,
			CashPct:   at.CashPct,
			OtherPct:  at.OtherPct,
		})
		if err != nil {
			summary.Warnings = append(summary.Warnings, fmt.Sprintf("allocation targets: %v", err))
		}
	}

	for _, rr := range bundle.ReturnRates {
		_, err := s.returnRates.Upsert(ctx, userID, rr.AssetClass, rr.ReturnRate)
		if err != nil {
			summary.Warnings = append(summary.Warnings, fmt.Sprintf("return rate %s: %v", rr.AssetClass, err))
		}
	}

	return summary, nil
}

func contribKey(label *string, amount float64, frequency string) string {
	l := ""
	if label != nil {
		l = *label
	}
	return fmt.Sprintf("%s|%.4f|%s", l, amount, frequency)
}

func goalKey(name, goalType string) string {
	return strings.ToLower(name) + "|" + goalType
}
