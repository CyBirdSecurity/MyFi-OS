package plaid

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/cybirdsecurity/myfi-os/internal/crypto"
	"github.com/cybirdsecurity/myfi-os/internal/db"
	"github.com/cybirdsecurity/myfi-os/internal/models"
	"github.com/cybirdsecurity/myfi-os/internal/prices"
)

// Store interface seams (autosnap.go precedent) so sync/backfill are testable
// without a database.

type itemStore interface {
	ListUnlinked(ctx context.Context, minAge time.Duration) ([]models.PlaidItem, error)
	Create(ctx context.Context, p db.CreatePlaidItemParams) (*models.PlaidItem, error)
	GetByItemID(ctx context.Context, userID uuid.UUID, itemID string) (*models.PlaidItem, error)
	ListByUser(ctx context.Context, userID uuid.UUID) ([]models.PlaidItem, error)
	ListAllActive(ctx context.Context) ([]models.PlaidItem, error)
	RecordSyncResult(ctx context.Context, id uuid.UUID, syncErr *string) error
	SetStatus(ctx context.Context, id uuid.UUID, status, errorCode string) error
	SetBackfillRequested(ctx context.Context, id uuid.UUID) error
	SetBackfilledAt(ctx context.Context, id uuid.UUID) error
	SetTransactionsCursor(ctx context.Context, id uuid.UUID, cursor string) error
	ClaimInvestmentsRefresh(ctx context.Context, id uuid.UUID, window time.Duration) (bool, error)
	Delete(ctx context.Context, id, userID uuid.UUID) error
}

// transactionStore persists banking transactions synced from Plaid.
type transactionStore interface {
	UpsertExternal(ctx context.Context, p db.UpsertExternalParams) (bool, error)
	TryAdoptExternalMatch(ctx context.Context, p db.UpsertExternalParams) (bool, error)
	DeleteByExternalID(ctx context.Context, userID uuid.UUID, provider, externalID string) error
}

// contributionEntryStore persists contributions found in investment transactions.
type contributionEntryStore interface {
	UpsertSynced(ctx context.Context, entries []db.ContributionEntryParams) error
}

// transactionClassifier resolves budget categories for synced transactions
// without an LLM call. Satisfied by *services.ClassifierService; declared here
// as a narrow seam so the plaid package doesn't import its parent.
type transactionClassifier interface {
	ClassifyBatchRulesOnly(ctx context.Context, userID uuid.UUID, merchants, sourceCategories []string) ([]*uuid.UUID, []bool)
	CategoryIDByName(name string) uuid.UUID
}

type accountStore interface {
	Get(ctx context.Context, id, userID uuid.UUID) (*models.Account, error)
	Create(ctx context.Context, p db.CreateAccountParams) (*models.Account, error)
	LinkPlaid(ctx context.Context, accountID, userID uuid.UUID, externalAccountID, itemID string) error
	UnlinkExternal(ctx context.Context, accountID, userID uuid.UUID) error
	ListByExternalItem(ctx context.Context, userID uuid.UUID, provider, itemID string) ([]models.Account, error)
	FindByExternalID(ctx context.Context, userID uuid.UUID, provider, externalAccountID string) (*models.Account, error)
	TouchSynced(ctx context.Context, accountID, userID uuid.UUID) error
}

type holdingStore interface {
	ListByAccount(ctx context.Context, accountID, userID uuid.UUID) ([]models.Holding, error)
	ReplaceForTickers(ctx context.Context, accountID, userID uuid.UUID, params []db.CreateHoldingParams) (int, error)
}

type snapshotStore interface {
	BulkUpsertSkipManual(ctx context.Context, snaps []db.UpsertSnapshotParams) (int, error)
	LatestForAccount(ctx context.Context, accountID, userID uuid.UUID) (*models.BalanceSnapshot, error)
	Upsert(ctx context.Context, p db.UpsertSnapshotParams) (*models.BalanceSnapshot, error)
}

// homeStore persists mortgage/home details synced from Plaid's liabilities
// product. Narrow seam over *db.HomeStore, following the existing pattern.
type homeStore interface {
	Get(ctx context.Context, accountID, userID uuid.UUID) (*models.HomeDetails, error)
	Upsert(ctx context.Context, p db.UpsertHomeParams) (*models.HomeDetails, error)
}

// creditCardStore persists credit-card liability details (APR, statement
// balance, minimum payment, due date) synced from Plaid.
type creditCardStore interface {
	Upsert(ctx context.Context, p db.UpsertCreditCardParams) (*models.CreditCardDetails, error)
}

type candleProvider interface {
	GetCandles(ctx context.Context, ticker string, from, to time.Time) ([]prices.HistoricalCandle, error)
}

// Service orchestrates Plaid linking, syncing, and snapshot backfill.
type Service struct {
	api API // built from PLAID_CLIENT_ID and PLAID_SECRET
	// clientFor overrides how a user's client is resolved (tests); nil uses api.
	clientFor      func(ctx context.Context, userID uuid.UUID) (API, error)
	cipher         *crypto.Cipher
	items          itemStore
	accounts       accountStore
	holdings       holdingStore
	snapshots      snapshotStore
	home           homeStore
	creditCards    creditCardStore
	candles        candleProvider
	transactions   transactionStore       // nil disables banking transaction sync
	classifier     transactionClassifier  // nil disables banking transaction sync
	contribEntries contributionEntryStore // nil disables contribution sync
	// refreshWait is how long to let Plaid process an investments refresh
	// before re-fetching holdings. Zero (tests) means don't wait.
	refreshWait time.Duration
}

// SetTransactionSync wires in banking transaction sync. Kept separate from
// NewService so the classifier (which lives in the parent services package and
// is constructed after the Plaid service) can be attached during wiring
// without an import cycle. When either dependency is nil, SyncItem skips
// transaction sync and only refreshes holdings/balances.
func (s *Service) SetTransactionSync(transactions transactionStore, classifier transactionClassifier) {
	s.transactions = transactions
	s.classifier = classifier
}

// SetContributionSync wires in syncing retirement contributions from
// investment transactions. When nil, SyncItem skips it.
func (s *Service) SetContributionSync(entries contributionEntryStore) {
	s.contribEntries = entries
}

func NewService(
	api API,
	cipher *crypto.Cipher,
	items *db.PlaidItemStore,
	accounts *db.AccountStore,
	holdings *db.HoldingStore,
	snapshots *db.SnapshotStore,
	home *db.HomeStore,
	creditCards *db.CreditCardStore,
	candles candleProvider,
) *Service {
	return &Service{
		api:         api,
		cipher:      cipher,
		items:       items,
		accounts:    accounts,
		holdings:    holdings,
		snapshots:   snapshots,
		home:        home,
		creditCards: creditCards,
		candles:     candles,
		refreshWait: investmentsRefreshWait,
	}
}

// apiFor returns the Plaid client used for a user's requests.
func (s *Service) apiFor(ctx context.Context, userID uuid.UUID) (API, error) {
	if s.clientFor != nil {
		return s.clientFor(ctx, userID)
	}
	return s.api, nil
}

// apiForItem returns the Plaid client used for an Item's requests.
func (s *Service) apiForItem(ctx context.Context, item models.PlaidItem) (API, error) {
	return s.apiFor(ctx, item.UserID)
}

// CreateLinkToken mints a Plaid Link token. mode is "investments" (holdings
// sync) or "banking" (checking/savings/credit cards/loans — covers both
// depository and credit-only institutions, see below). When itemID is
// non-empty, an update-mode token is issued for re-authenticating that
// existing Item.
func (s *Service) CreateLinkToken(ctx context.Context, userID uuid.UUID, mode, itemID string) (string, error) {
	if itemID != "" {
		item, err := s.items.GetByItemID(ctx, userID, itemID)
		if err != nil {
			return "", err
		}
		token, err := s.cipher.Decrypt(item.AccessTokenEncrypted)
		if err != nil {
			return "", fmt.Errorf("decrypt access token: %w", err)
		}
		api, err := s.apiFor(ctx, userID)
		if err != nil {
			return "", err
		}
		// Reconnecting an existing Item also consents to transactions, so the
		// "reconnect to enable transactions" prompt upgrades older Items (linked
		// before transaction sync existed) to banking sync on their next sync.
		return api.CreateLinkToken(ctx, userID.String(), nil, nil, nil, []string{"transactions"}, token)
	}

	var products, requiredIfSupported []string
	switch mode {
	case "investments":
		products = []string{"investments"}
	case "banking":
		// transactions is the required product because it's the one Plaid
		// product broadly supported across both depository (checking/savings)
		// and credit/loan-only institutions — unlike "auth" (which credit-only
		// institutions can't satisfy) or "liabilities" (which depository-only
		// institutions can't satisfy). auth and liabilities are
		// required-if-supported instead of hard-required: Plaid enables
		// whichever actually applies to the institution the user picks, so the
		// user never has to know in advance whether they're linking a bank or
		// a card issuer.
		products = []string{"transactions"}
		requiredIfSupported = []string{"auth", "liabilities"}
	default:
		return "", fmt.Errorf("invalid link mode %q (want investments or banking)", mode)
	}
	api, err := s.apiFor(ctx, userID)
	if err != nil {
		return "", err
	}
	return api.CreateLinkToken(ctx, userID.String(), products, requiredIfSupported, nil, nil, "")
}

// AccountPreview describes one Plaid account for the mapping UI.
type AccountPreview struct {
	PlaidAccountID        string   `json:"plaidAccountId"`
	Name                  string   `json:"name"`
	Mask                  string   `json:"mask"`
	Type                  string   `json:"type"`
	Subtype               string   `json:"subtype"`
	Balance               *float64 `json:"balance"`
	Currency              string   `json:"currency"`
	SuggestedAccountType  string   `json:"suggestedAccountType"`
	SuggestedTaxTreatment string   `json:"suggestedTaxTreatment"`
	// LinkedAccountID is set when this Plaid account was previously linked to
	// a MyFi account (re-link flow) so the UI can pre-select it.
	LinkedAccountID *string `json:"linkedAccountId"`
}

// ItemPreview is returned after a public token exchange, mirroring the OFX
// preview → map → apply flow.
type ItemPreview struct {
	ItemID          string           `json:"itemId"`
	InstitutionName string           `json:"institutionName"`
	Accounts        []AccountPreview `json:"accounts"`
}

// Exchange trades a Link public_token for an access token, stores the new
// Item (token encrypted at rest), and returns a preview of its accounts for
// the mapping step.
func (s *Service) Exchange(ctx context.Context, userID uuid.UUID, publicToken, institutionID, institutionName string) (*ItemPreview, error) {
	// The user's own keys minted the Link token being exchanged.
	api, err := s.apiFor(ctx, userID)
	if err != nil {
		return nil, err
	}
	accessToken, itemID, err := api.ExchangePublicToken(ctx, publicToken)
	if err != nil {
		return nil, err
	}

	encrypted, err := s.cipher.Encrypt(accessToken)
	if err != nil {
		return nil, fmt.Errorf("encrypt access token: %w", err)
	}
	var instID, instName *string
	if institutionID != "" {
		instID = &institutionID
	}
	if institutionName != "" {
		instName = &institutionName
	}
	if _, err := s.items.Create(ctx, db.CreatePlaidItemParams{
		UserID:               userID,
		ItemID:               itemID,
		AccessTokenEncrypted: encrypted,
		InstitutionID:        instID,
		InstitutionName:      instName,
	}); err != nil {
		return nil, err
	}

	plaidAccounts, err := api.GetAccounts(ctx, accessToken)
	if err != nil {
		return nil, err
	}

	preview := &ItemPreview{ItemID: itemID, InstitutionName: institutionName}
	for _, a := range plaidAccounts {
		accountType, taxTreatment := SubtypeToAccountType(a.Type, a.Subtype)
		p := AccountPreview{
			PlaidAccountID:        a.AccountID,
			Name:                  a.Name,
			Mask:                  a.Mask,
			Type:                  a.Type,
			Subtype:               a.Subtype,
			Balance:               a.CurrentBalance,
			Currency:              a.Currency,
			SuggestedAccountType:  accountType,
			SuggestedTaxTreatment: taxTreatment,
		}
		if existing, _ := s.accounts.FindByExternalID(ctx, userID, "plaid", a.AccountID); existing != nil {
			id := existing.ID.String()
			p.LinkedAccountID = &id
		}
		preview.Accounts = append(preview.Accounts, p)
	}
	return preview, nil
}

// CreateNewAccount are the fields for creating a fresh MyFi account for an
// unmapped Plaid account.
type CreateNewAccount struct {
	Name         string `json:"name"`
	AccountType  string `json:"accountType"`
	TaxTreatment string `json:"taxTreatment"`
	Institution  string `json:"institution"`
	Currency     string `json:"currency"`
}

// AccountMapping links one Plaid account to a MyFi account: either an existing
// one (MyFiAccountID) or a new one (CreateNew). Plaid accounts without a
// mapping are skipped.
type AccountMapping struct {
	PlaidAccountID string            `json:"plaidAccountId"`
	MyFiAccountID  *uuid.UUID        `json:"myfiAccountId"`
	CreateNew      *CreateNewAccount `json:"createNew"`
}

// ApplyResult reports the outcome of ApplyMappings.
type ApplyResult struct {
	Linked          int         `json:"linked"`
	Created         int         `json:"created"`
	Synced          *SyncResult `json:"synced"`
	BackfillStarted bool        `json:"backfillStarted"`
}

// ApplyMappings attaches Plaid accounts to MyFi accounts (preserving all
// existing settings on attached accounts), runs an immediate sync, and
// optionally kicks off the historical backfill in the background.
func (s *Service) ApplyMappings(ctx context.Context, userID uuid.UUID, itemID string, mappings []AccountMapping, backfill bool) (*ApplyResult, error) {
	item, err := s.items.GetByItemID(ctx, userID, itemID)
	if err != nil {
		return nil, err
	}

	result := &ApplyResult{}
	for _, m := range mappings {
		switch {
		case m.MyFiAccountID != nil:
			// Verify ownership before linking.
			acc, err := s.accounts.Get(ctx, *m.MyFiAccountID, userID)
			if err != nil || acc == nil {
				return nil, fmt.Errorf("account %s not found", m.MyFiAccountID)
			}
			if err := s.accounts.LinkPlaid(ctx, acc.ID, userID, m.PlaidAccountID, itemID); err != nil {
				return nil, linkError(err, m.PlaidAccountID)
			}
			result.Linked++
		case m.CreateNew != nil:
			n := m.CreateNew
			if !models.ValidAccountTypes[n.AccountType] {
				return nil, fmt.Errorf("invalid account type %q", n.AccountType)
			}
			if n.AccountType == "home" {
				return nil, fmt.Errorf("a Plaid mortgage account must be linked to an existing home account — create the home account manually (with its address) first, then link")
			}
			taxTreatment := n.TaxTreatment
			if taxTreatment == "" {
				taxTreatment = models.AccountTypeTaxDefaults[n.AccountType]
			}
			currency := n.Currency
			if currency == "" {
				currency = "USD"
			}
			var institution *string
			if n.Institution != "" {
				institution = &n.Institution
			}
			acc, err := s.accounts.Create(ctx, db.CreateAccountParams{
				UserID:       userID,
				Name:         n.Name,
				AccountType:  n.AccountType,
				TaxTreatment: taxTreatment,
				Institution:  institution,
				Currency:     currency,
			})
			if err != nil {
				return nil, err
			}
			if err := s.accounts.LinkPlaid(ctx, acc.ID, userID, m.PlaidAccountID, itemID); err != nil {
				return nil, linkError(err, m.PlaidAccountID)
			}
			result.Created++
		}
	}

	// Immediate first sync so holdings/balances appear right away.
	// Failures are logged by syncItem and surfaced via result.Synced.
	syncRes, _ := s.SyncItem(ctx, *item)
	result.Synced = syncRes

	if backfill {
		// Recorded synchronously so it's visible in the response that follows
		// and distinguishes "in progress" from "never requested" for the UI
		// while backfilled_at is still null.
		if err := s.items.SetBackfillRequested(ctx, item.ID); err != nil {
			log.Printf("plaid: record backfill requested for item %s: %v", item.ItemID, err)
		}
		// Detached context: the backfill outlives the HTTP request.
		go func() {
			bctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
			defer cancel()
			if err := s.BackfillItem(bctx, *item); err != nil {
				log.Printf("plaid: backfill for item %s: %v", item.ItemID, err)
			}
		}()
		result.BackfillStarted = true
	}
	return result, nil
}

// RemoveItem unlinks all accounts under an Item (they revert to manual
// tracking, keeping their data) and deletes the Item. The Plaid-side removal
// is best-effort — a dead token must not block local cleanup.
func (s *Service) RemoveItem(ctx context.Context, userID uuid.UUID, itemID string) error {
	item, err := s.items.GetByItemID(ctx, userID, itemID)
	if err != nil {
		return err
	}

	if token, err := s.cipher.Decrypt(item.AccessTokenEncrypted); err == nil {
		if api, err := s.apiForItem(ctx, *item); err != nil {
			log.Printf("plaid: remove item %s at Plaid: %v", itemID, err)
		} else if err := api.RemoveItem(ctx, token); err != nil {
			log.Printf("plaid: remove item %s at Plaid: %v", itemID, err)
		}
	}

	linked, err := s.accounts.ListByExternalItem(ctx, userID, "plaid", itemID)
	if err != nil {
		return err
	}
	for _, acc := range linked {
		if err := s.accounts.UnlinkExternal(ctx, acc.ID, userID); err != nil {
			return err
		}
	}
	return s.items.Delete(ctx, item.ID, userID)
}

// linkError translates the unique-constraint violation on
// (user_id, external_provider, external_account_id) into a friendly message.
func linkError(err error, plaidAccountID string) error {
	if strings.Contains(err.Error(), "duplicate key") || strings.Contains(err.Error(), "unique") {
		return fmt.Errorf("plaid account %s is already linked to another MyFi account — unlink it first", plaidAccountID)
	}
	return err
}
