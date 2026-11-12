package db

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"

	"github.com/cybirdsecurity/myfi-os/internal/models"
)

type GoalStore struct{ db *sqlx.DB }

func NewGoalStore(db *sqlx.DB) *GoalStore { return &GoalStore{db: db} }

func (s *GoalStore) List(ctx context.Context, userID uuid.UUID) ([]models.Goal, error) {
	var goals []models.Goal
	err := s.db.SelectContext(ctx, &goals,
		`SELECT * FROM goals WHERE user_id = $1 ORDER BY target_date NULLS LAST, name`,
		userID,
	)
	if err != nil {
		return nil, fmt.Errorf("list goals: %w", err)
	}
	return goals, nil
}

type CreateGoalParams struct {
	UserID            uuid.UUID
	Name              string
	GoalType          string
	AccountID         *uuid.UUID
	AccountTypeFilter *string
	AssetClassFilter  *string
	TargetAmount      float64
	TargetDate        *time.Time
	Notes             *string
}

func (s *GoalStore) Create(ctx context.Context, p CreateGoalParams) (*models.Goal, error) {
	var g models.Goal
	err := s.db.QueryRowxContext(ctx, `
		INSERT INTO goals (user_id, name, goal_type, account_id, account_type_filter, asset_class_filter, target_amount, target_date, notes)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		RETURNING *`,
		p.UserID, p.Name, p.GoalType, p.AccountID, p.AccountTypeFilter, p.AssetClassFilter,
		p.TargetAmount, p.TargetDate, p.Notes,
	).StructScan(&g)
	if err != nil {
		return nil, fmt.Errorf("create goal: %w", err)
	}
	return &g, nil
}

type UpdateGoalParams struct {
	ID                uuid.UUID
	UserID            uuid.UUID
	Name              string
	GoalType          string
	AccountID         *uuid.UUID
	AccountTypeFilter *string
	AssetClassFilter  *string
	TargetAmount      float64
	TargetDate        *time.Time
	Notes             *string
	IsAchieved        bool
}

func (s *GoalStore) Update(ctx context.Context, p UpdateGoalParams) (*models.Goal, error) {
	var g models.Goal
	err := s.db.QueryRowxContext(ctx, `
		UPDATE goals
		SET name = $3, goal_type = $4, account_id = $5, account_type_filter = $6,
		    asset_class_filter = $7, target_amount = $8, target_date = $9, notes = $10, is_achieved = $11
		WHERE id = $1 AND user_id = $2
		RETURNING *`,
		p.ID, p.UserID, p.Name, p.GoalType, p.AccountID, p.AccountTypeFilter, p.AssetClassFilter,
		p.TargetAmount, p.TargetDate, p.Notes, p.IsAchieved,
	).StructScan(&g)
	if err != nil {
		return nil, fmt.Errorf("update goal: %w", err)
	}
	return &g, nil
}

func (s *GoalStore) Delete(ctx context.Context, id, userID uuid.UUID) error {
	res, err := s.db.ExecContext(ctx,
		`DELETE FROM goals WHERE id = $1 AND user_id = $2`,
		id, userID,
	)
	if err != nil {
		return fmt.Errorf("delete goal: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("goal not found")
	}
	return nil
}
