package db

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"

	"github.com/cybirdsecurity/myfi-os/internal/models"
)

type AccountStore struct{ db *sqlx.DB }

func NewAccountStore(db *sqlx.DB) *AccountStore { return &AccountStore{db: db} }

// List returns all non-deleted accounts for userID. Each account is annotated with
// IsPartnerAccount (true if the account is attributed to the 'partner' member) and
// OwnerName (the account's attributed member's name, "A & B" for a joint account; for an unattributed account,
// the user's own 'self' household member name — falling back to users.display_name
// only if no household profile has been set up yet, since display_name is a raw
// login-account label, not necessarily the name the user actually goes by).
func (s *AccountStore) List(ctx context.Context, userID uuid.UUID) ([]models.Account, error) {
	type row struct {
		models.Account
		IsPartner bool   `db:"is_partner_account"`
		OwnerName string `db:"owner_name"`
	}
	var rows []row
	err := s.db.SelectContext(ctx, &rows, `
		SELECT
			a.*,
			COALESCE(hm.relationship = 'partner', false)       AS is_partner_account,
			CASE WHEN a.is_joint
			     THEN COALESCE(self_hm.name, u.display_name) || ' & ' || COALESCE(partner_hm.name, 'Partner')
			     ELSE COALESCE(hm.name, self_hm.name, u.display_name) END AS owner_name
		FROM accounts a
		JOIN users u ON u.id = a.user_id
		LEFT JOIN household_members hm ON hm.id = a.member_id
		LEFT JOIN household_members self_hm
			ON self_hm.user_id = a.user_id AND self_hm.relationship = 'self'
		LEFT JOIN household_members partner_hm
			ON partner_hm.user_id = a.user_id AND partner_hm.relationship = 'partner'
		WHERE a.deleted_at IS NULL
		  AND a.user_id = $1
		ORDER BY a.name`,
		userID,
	)
	if err != nil {
		return nil, fmt.Errorf("list accounts: %w", err)
	}
	accounts := make([]models.Account, len(rows))
	for i, r := range rows {
		accounts[i] = r.Account
		accounts[i].IsPartnerAccount = r.IsPartner
		accounts[i].OwnerName = r.OwnerName
	}
	return accounts, nil
}

// Get returns a single account owned by userID. Returns an error when not found.
func (s *AccountStore) Get(ctx context.Context, id, userID uuid.UUID) (*models.Account, error) {
	type row struct {
		models.Account
		IsPartner bool   `db:"is_partner_account"`
		OwnerName string `db:"owner_name"`
	}
	var r row
	err := s.db.QueryRowxContext(ctx, `
		SELECT
			a.*,
			COALESCE(hm.relationship = 'partner', false)   AS is_partner_account,
			CASE WHEN a.is_joint
			     THEN COALESCE(self_hm.name, u.display_name) || ' & ' || COALESCE(partner_hm.name, 'Partner')
			     ELSE COALESCE(hm.name, self_hm.name, u.display_name) END AS owner_name
		FROM accounts a
		JOIN users u ON u.id = a.user_id
		LEFT JOIN household_members hm ON hm.id = a.member_id
		LEFT JOIN household_members self_hm
			ON self_hm.user_id = a.user_id AND self_hm.relationship = 'self'
		LEFT JOIN household_members partner_hm
			ON partner_hm.user_id = a.user_id AND partner_hm.relationship = 'partner'
		WHERE a.id = $1
		  AND a.user_id = $2
		  AND a.deleted_at IS NULL`,
		id, userID,
	).StructScan(&r)
	if err != nil {
		return nil, fmt.Errorf("get account: %w", err)
	}
	acc := r.Account
	acc.IsPartnerAccount = r.IsPartner
	acc.OwnerName = r.OwnerName
	return &acc, nil
}

type CreateAccountParams struct {
	UserID       uuid.UUID
	Name         string
	AccountType  string
	TaxTreatment string
	Institution  *string
	Currency     string
	MemberID     *uuid.UUID // optional attribution to a household member
	IsJoint      bool       // owned by both members; MemberID must be nil
}

func (s *AccountStore) Create(ctx context.Context, p CreateAccountParams) (*models.Account, error) {
	var a models.Account
	err := s.db.QueryRowxContext(ctx, `
		INSERT INTO accounts (user_id, name, account_type, tax_treatment, institution, currency, member_id, is_joint)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING *`,
		p.UserID, p.Name, p.AccountType, p.TaxTreatment, p.Institution, p.Currency, p.MemberID, p.IsJoint,
	).StructScan(&a)
	if err != nil {
		return nil, fmt.Errorf("create account: %w", err)
	}
	return &a, nil
}

type UpdateAccountParams struct {
	ID           uuid.UUID
	UserID       uuid.UUID
	Name         string
	AccountType  string
	TaxTreatment string
	Institution  *string
	MemberID     *uuid.UUID // nil = keep existing, use explicit zero UUID to clear
	SetMemberID  bool       // must be true to change MemberID (so nil means "clear")
	IsJoint      bool       // written together with MemberID when SetMemberID is true
}

func (s *AccountStore) Update(ctx context.Context, p UpdateAccountParams) (*models.Account, error) {
	var a models.Account
	var err error
	if p.SetMemberID {
		err = s.db.QueryRowxContext(ctx, `
			UPDATE accounts
			SET name          = $3,
			    account_type  = $4,
			    tax_treatment = $5,
			    institution   = $6,
			    member_id     = $7,
			    is_joint      = $8
			WHERE id = $1 AND user_id = $2 AND deleted_at IS NULL
			RETURNING *`,
			p.ID, p.UserID, p.Name, p.AccountType, p.TaxTreatment, p.Institution, p.MemberID, p.IsJoint,
		).StructScan(&a)
	} else {
		err = s.db.QueryRowxContext(ctx, `
			UPDATE accounts
			SET name          = $3,
			    account_type  = $4,
			    tax_treatment = $5,
			    institution   = $6
			WHERE id = $1 AND user_id = $2 AND deleted_at IS NULL
			RETURNING *`,
			p.ID, p.UserID, p.Name, p.AccountType, p.TaxTreatment, p.Institution,
		).StructScan(&a)
	}
	if err != nil {
		return nil, fmt.Errorf("update account: %w", err)
	}
	return &a, nil
}

// LinkExternal sets the external_provider, external_account_id, and last_synced_at
// fields on an account, recording the OFX institution link after a successful import.
func (s *AccountStore) LinkExternal(ctx context.Context, accountID, userID uuid.UUID, provider, externalAccountID string) error {
	res, err := s.db.ExecContext(ctx, `
		UPDATE accounts
		SET external_provider = $3, external_account_id = $4, last_synced_at = NOW()
		WHERE id = $1 AND user_id = $2 AND deleted_at IS NULL`,
		accountID, userID, provider, externalAccountID,
	)
	if err != nil {
		return fmt.Errorf("link external account: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("account not found")
	}
	return nil
}

// LinkPlaid records a Plaid link on an account: provider, external account ID,
// Plaid item ID, and flips is_manual off so the UI can distinguish synced accounts.
func (s *AccountStore) LinkPlaid(ctx context.Context, accountID, userID uuid.UUID, externalAccountID, itemID string) error {
	res, err := s.db.ExecContext(ctx, `
		UPDATE accounts
		SET external_provider = 'plaid', external_account_id = $3, external_item_id = $4,
		    is_manual = FALSE, last_synced_at = NOW()
		WHERE id = $1 AND user_id = $2 AND deleted_at IS NULL`,
		accountID, userID, externalAccountID, itemID,
	)
	if err != nil {
		return fmt.Errorf("link plaid account: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("account not found")
	}
	return nil
}

// UnlinkExternal clears an account's external link and reverts it to manual
// tracking. The account and all of its data (snapshots, holdings, contributions)
// are preserved.
func (s *AccountStore) UnlinkExternal(ctx context.Context, accountID, userID uuid.UUID) error {
	res, err := s.db.ExecContext(ctx, `
		UPDATE accounts
		SET external_provider = NULL, external_account_id = NULL, external_item_id = NULL,
		    is_manual = TRUE
		WHERE id = $1 AND user_id = $2 AND deleted_at IS NULL`,
		accountID, userID,
	)
	if err != nil {
		return fmt.Errorf("unlink external account: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("account not found")
	}
	return nil
}

// ListByExternalItem returns all non-deleted accounts linked to a given
// provider + external item ID (e.g. every account under one Plaid Item).
func (s *AccountStore) ListByExternalItem(ctx context.Context, userID uuid.UUID, provider, itemID string) ([]models.Account, error) {
	var accounts []models.Account
	err := s.db.SelectContext(ctx, &accounts, `
		SELECT * FROM accounts
		WHERE user_id = $1 AND external_provider = $2 AND external_item_id = $3 AND deleted_at IS NULL
		ORDER BY name`,
		userID, provider, itemID,
	)
	if err != nil {
		return nil, fmt.Errorf("list accounts by external item: %w", err)
	}
	return accounts, nil
}

// TouchSynced updates last_synced_at on an account after a successful sync.
func (s *AccountStore) TouchSynced(ctx context.Context, accountID, userID uuid.UUID) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE accounts SET last_synced_at = NOW() WHERE id = $1 AND user_id = $2 AND deleted_at IS NULL`,
		accountID, userID,
	)
	if err != nil {
		return fmt.Errorf("touch synced: %w", err)
	}
	return nil
}

// FindByExternalID returns the account linked to a given external provider + account ID,
// or nil if no such link exists.
func (s *AccountStore) FindByExternalID(ctx context.Context, userID uuid.UUID, provider, externalAccountID string) (*models.Account, error) {
	var a models.Account
	err := s.db.QueryRowxContext(ctx, `
		SELECT * FROM accounts
		WHERE user_id = $1 AND external_provider = $2 AND external_account_id = $3 AND deleted_at IS NULL`,
		userID, provider, externalAccountID,
	).StructScan(&a)
	if err != nil {
		return nil, nil // not found is not an error here
	}
	return &a, nil
}

// SoftDelete removes an account — owner only.
func (s *AccountStore) SoftDelete(ctx context.Context, id, userID uuid.UUID) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE accounts SET deleted_at = NOW() WHERE id = $1 AND user_id = $2 AND deleted_at IS NULL`,
		id, userID,
	)
	if err != nil {
		return fmt.Errorf("soft delete account: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("account not found")
	}
	return nil
}
