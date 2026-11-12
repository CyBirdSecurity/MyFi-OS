package db

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"

	"github.com/cybirdsecurity/myfi-os/internal/models"
)

type HouseholdMembersStore struct{ db *sqlx.DB }

func NewHouseholdMembersStore(db *sqlx.DB) *HouseholdMembersStore {
	return &HouseholdMembersStore{db: db}
}

// ListByUser returns all household members for the given user (at most 2: self + partner).
func (s *HouseholdMembersStore) ListByUser(ctx context.Context, userID uuid.UUID) ([]models.HouseholdMember, error) {
	var out []models.HouseholdMember
	err := s.db.SelectContext(ctx, &out,
		`SELECT * FROM household_members WHERE user_id = $1 ORDER BY relationship`,
		userID,
	)
	if err != nil {
		return nil, fmt.Errorf("list household members: %w", err)
	}
	return out, nil
}

// GetByID returns a single member, verifying it belongs to userID.
func (s *HouseholdMembersStore) GetByID(ctx context.Context, id, userID uuid.UUID) (*models.HouseholdMember, error) {
	var m models.HouseholdMember
	err := s.db.QueryRowxContext(ctx,
		`SELECT * FROM household_members WHERE id = $1 AND user_id = $2`,
		id, userID,
	).StructScan(&m)
	if err != nil {
		return nil, nil
	}
	return &m, nil
}

type UpsertMemberParams struct {
	UserID                 uuid.UUID
	Relationship           string
	Name                   string
	Birthday               *time.Time
	IsEmployed             bool
	MonthlyIncome          float64
	SalaryGrowthPct        float64
	RetirementAge          int
	UseSocialSecurity      bool
	SocialSecurityMonthly  float64
	SocialSecurityStartAge int
}

// Upsert creates or updates a household member, matching on (user_id, relationship).
func (s *HouseholdMembersStore) Upsert(ctx context.Context, p UpsertMemberParams) (*models.HouseholdMember, error) {
	retirementAge := p.RetirementAge
	if retirementAge <= 0 {
		retirementAge = 65
	}
	ssStartAge := p.SocialSecurityStartAge
	if ssStartAge <= 0 {
		ssStartAge = 67
	}
	var out models.HouseholdMember
	err := s.db.QueryRowxContext(ctx, `
		INSERT INTO household_members (
			user_id, relationship, name, birthday, is_employed,
			monthly_income, salary_growth_pct, retirement_age,
			use_social_security, social_security_monthly, social_security_start_age,
			updated_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,now())
		ON CONFLICT (user_id, relationship) DO UPDATE SET
			name                     = EXCLUDED.name,
			birthday                 = EXCLUDED.birthday,
			is_employed              = EXCLUDED.is_employed,
			monthly_income           = EXCLUDED.monthly_income,
			salary_growth_pct        = EXCLUDED.salary_growth_pct,
			retirement_age           = EXCLUDED.retirement_age,
			use_social_security      = EXCLUDED.use_social_security,
			social_security_monthly  = EXCLUDED.social_security_monthly,
			social_security_start_age = EXCLUDED.social_security_start_age,
			updated_at               = now()
		RETURNING *`,
		p.UserID, p.Relationship, p.Name, p.Birthday, p.IsEmployed,
		p.MonthlyIncome, p.SalaryGrowthPct, retirementAge,
		p.UseSocialSecurity, p.SocialSecurityMonthly, ssStartAge,
	).StructScan(&out)
	if err != nil {
		return nil, fmt.Errorf("upsert household member: %w", err)
	}
	return &out, nil
}

// Delete removes a partner member. The 'self' member cannot be deleted. The
// user's joint accounts revert to single ownership by 'self'.
func (s *HouseholdMembersStore) Delete(ctx context.Context, id, userID uuid.UUID) error {
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	res, err := tx.ExecContext(ctx,
		`DELETE FROM household_members WHERE id = $1 AND user_id = $2 AND relationship = 'partner'`,
		id, userID,
	)
	if err != nil {
		return fmt.Errorf("delete household member: %w", err)
	}
	rows, _ := res.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("member not found or cannot be deleted")
	}
	if _, err := tx.ExecContext(ctx,
		`UPDATE accounts SET is_joint = FALSE WHERE user_id = $1 AND is_joint`, userID,
	); err != nil {
		return fmt.Errorf("reset joint accounts: %w", err)
	}
	return tx.Commit()
}
