package db

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"

	"github.com/cybirdsecurity/myfi-os/internal/models"
)

type VehicleStore struct{ db *sqlx.DB }

func NewVehicleStore(db *sqlx.DB) *VehicleStore { return &VehicleStore{db: db} }

func (s *VehicleStore) Get(ctx context.Context, accountID, userID uuid.UUID) (*models.VehicleDetails, error) {
	var d models.VehicleDetails
	err := s.db.QueryRowxContext(ctx,
		`SELECT * FROM vehicle_details WHERE account_id = $1 AND user_id = $2`,
		accountID, userID,
	).StructScan(&d)
	if err != nil {
		return nil, fmt.Errorf("get vehicle details: %w", err)
	}
	return &d, nil
}

type UpsertVehicleParams struct {
	AccountID    uuid.UUID
	UserID       uuid.UUID
	VehicleYear  int
	VehicleMake  string
	VehicleModel string
	VehicleTrim  *string
}

func (s *VehicleStore) Upsert(ctx context.Context, p UpsertVehicleParams) (*models.VehicleDetails, error) {
	var d models.VehicleDetails
	err := s.db.QueryRowxContext(ctx, `
		INSERT INTO vehicle_details (account_id, user_id, vehicle_year, vehicle_make, vehicle_model, vehicle_trim)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (account_id) DO UPDATE
			SET vehicle_year  = EXCLUDED.vehicle_year,
			    vehicle_make  = EXCLUDED.vehicle_make,
			    vehicle_model = EXCLUDED.vehicle_model,
			    vehicle_trim  = EXCLUDED.vehicle_trim,
			    updated_at    = NOW()
		RETURNING *`,
		p.AccountID, p.UserID, p.VehicleYear, p.VehicleMake, p.VehicleModel, p.VehicleTrim,
	).StructScan(&d)
	if err != nil {
		return nil, fmt.Errorf("upsert vehicle details: %w", err)
	}
	return &d, nil
}

func (s *VehicleStore) UpdateValue(ctx context.Context, accountID uuid.UUID, value float64) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE vehicle_details
		SET last_value = $2, last_value_at = NOW(), updated_at = NOW()
		WHERE account_id = $1`,
		accountID, value,
	)
	if err != nil {
		return fmt.Errorf("update vehicle value: %w", err)
	}
	return nil
}
