// Package plaid integrates MyFi with the Plaid API: linking institutions,
// syncing holdings and balances into existing accounts, and backfilling
// historical weekly snapshots from investment transactions.
package plaid

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	plaidsdk "github.com/plaid/plaid-go/v43/plaid"
)

// Account is a Plaid account with its current balance.
type Account struct {
	AccountID      string
	Name           string
	Mask           string
	Type           string // depository | credit | loan | investment | brokerage | other
	Subtype        string // checking, savings, 401k, roth, brokerage, credit card, ...
	CurrentBalance *float64
	Currency       string
	// Limit is the credit limit for a credit-card account (nil otherwise).
	// Plaid reports this on the account's balances, not on the liability
	// record, so it's populated by GetAccounts/GetBalances rather than
	// GetLiabilities.
	Limit *float64
}

// MortgageLiability is Plaid's liabilities data for one mortgage account.
// Fields Plaid can't know (address, extra payment, tracking toggles) live
// only in home_details and are never touched by this type.
type MortgageLiability struct {
	AccountID                  string
	InterestRatePercentage     *float64 // e.g. 3.5 for 3.5%, not a fraction
	OriginationPrincipalAmount *float64
	OriginationDate            *time.Time
}

// CreditCardLiability is Plaid's liabilities data for one credit card
// account. APRPercentage surfaces only the "purchase_apr" entry from Plaid's
// per-balance-type APR array; other APR types can be added later without a
// breaking change.
type CreditCardLiability struct {
	AccountID              string
	APRPercentage          *float64
	IsOverdue              bool
	LastStatementBalance   *float64
	LastStatementIssueDate *time.Time
	MinimumPaymentAmount   *float64
	NextPaymentDueDate     *time.Time
	LastPaymentAmount      *float64
	LastPaymentDate        *time.Time
}

// Security identifies an instrument referenced by holdings and transactions.
type Security struct {
	SecurityID string
	Ticker     string // empty when the institution reports no symbol
	CUSIP      string
	Name       string
	Type       string // cash | cryptocurrency | derivative | equity | etf | fixed income | loan | mutual fund | other
}

// Holding is one position in an investment account.
type Holding struct {
	AccountID        string
	SecurityID       string
	Quantity         float64
	InstitutionPrice float64
	InstitutionValue float64
	CostBasis        *float64 // total cost basis
}

// InvestmentTransaction is one investment account transaction.
// Quantity is positive for buys, negative for sells. Amount is positive when
// cash is debited (purchases) and negative when cash is credited (sales).
type InvestmentTransaction struct {
	ID         string
	AccountID  string
	Name       string // institution's description, e.g. "EMPLOYER MATCH"
	SecurityID string // empty for cash-only movements
	Date       time.Time
	Type       string
	Subtype    string
	Quantity   float64
	Amount     float64
	Price      float64
}

// BankTransaction is one depository/credit account transaction from Plaid's
// Transactions product. Amount follows Plaid's convention: positive when money
// leaves the account (spending), negative when money enters (income/refund) —
// callers translate this into MyFi's own sign/category convention.
type BankTransaction struct {
	TransactionID string
	AccountID     string
	Date          time.Time
	Name          string // raw institution description
	MerchantName  string // cleaned merchant name; may be empty
	Amount        float64
	Currency      string
	Pending       bool
	PFCPrimary    string // Plaid Personal Finance Category primary (e.g. FOOD_AND_DRINK)
	PFCDetailed   string // Plaid Personal Finance Category detailed
}

// TransactionSyncPage is one page of /transactions/sync output. Callers loop,
// passing NextCursor back in, until HasMore is false, then persist the final
// cursor. Removed carries only the Plaid transaction IDs to delete.
type TransactionSyncPage struct {
	Added      []BankTransaction
	Modified   []BankTransaction
	RemovedIDs []string
	NextCursor string
	HasMore    bool
}

// API is the subset of the Plaid API the sync/backfill services consume.
// Kept narrow so tests can substitute a fake.
type API interface {
	CreateLinkToken(ctx context.Context, userID string, products, requiredIfSupportedProducts, optionalProducts, additionalConsentedProducts []string, updateModeAccessToken string) (string, error)
	ExchangePublicToken(ctx context.Context, publicToken string) (accessToken, itemID string, err error)
	GetAccounts(ctx context.Context, accessToken string) ([]Account, error)
	GetBalances(ctx context.Context, accessToken string) ([]Account, error)
	GetHoldings(ctx context.Context, accessToken string) ([]Holding, []Security, error)
	// RefreshInvestments asks Plaid to re-pull the Item's holdings and
	// transactions from the institution now (billed per call).
	RefreshInvestments(ctx context.Context, accessToken string) error
	GetLiabilities(ctx context.Context, accessToken string) ([]MortgageLiability, []CreditCardLiability, error)
	GetInvestmentTransactions(ctx context.Context, accessToken string, start, end time.Time, accountIDs []string) ([]InvestmentTransaction, []Security, error)
	// SyncTransactions fetches one page of banking transaction deltas starting
	// from cursor (empty cursor = start of history). The service loops on
	// HasMore and persists NextCursor.
	SyncTransactions(ctx context.Context, accessToken, cursor string) (*TransactionSyncPage, error)
	RemoveItem(ctx context.Context, accessToken string) error
}

// Client implements API against the real Plaid service.
type Client struct {
	api *plaidsdk.PlaidApiService
}

// NewClient builds a Plaid client for the given environment ("sandbox" or
// "production").
func NewClient(clientID, secret, env string) (*Client, error) {
	cfg := plaidsdk.NewConfiguration()
	cfg.AddDefaultHeader("PLAID-CLIENT-ID", clientID)
	cfg.AddDefaultHeader("PLAID-SECRET", secret)
	switch env {
	case "sandbox":
		cfg.UseEnvironment(plaidsdk.Sandbox)
	case "production":
		cfg.UseEnvironment(plaidsdk.Production)
	default:
		return nil, fmt.Errorf("invalid PLAID_ENV %q (want sandbox or production)", env)
	}
	return &Client{api: plaidsdk.NewAPIClient(cfg).PlaidApi}, nil
}

// ErrorCode extracts the Plaid error_code (e.g. ITEM_LOGIN_REQUIRED) from an
// error returned by this client, or "" if it is not a structured Plaid error.
// It sees through wrapping, so errors that have passed through wrapErr (or
// fmt.Errorf with %w) still report their code.
func ErrorCode(err error) string {
	if err == nil {
		return ""
	}
	var pe *plaidError
	if errors.As(err, &pe) {
		return pe.code
	}
	if raw, convErr := plaidsdk.ToPlaidError(err); convErr == nil {
		return raw.ErrorCode
	}
	return ""
}

// plaidError is a Plaid API error whose error_code survives wrapping.
type plaidError struct {
	action  string
	code    string
	message string
}

func (e *plaidError) Error() string {
	return fmt.Sprintf("plaid %s: %s: %s", e.action, e.code, e.message)
}

// wrapErr adds Plaid's actual error_code/error_message to an SDK error.
//
// The raw error from the generated client (e.g. "400 Bad Request") only
// carries the HTTP status — the useful diagnostic detail is in the JSON
// response body, which ToPlaidError parses out. The result keeps the code
// machine-readable (see ErrorCode) as well as in the message.
func wrapErr(action string, err error) error {
	if pe, convErr := plaidsdk.ToPlaidError(err); convErr == nil && pe.ErrorCode != "" {
		return &plaidError{action: action, code: pe.ErrorCode, message: pe.ErrorMessage}
	}
	return fmt.Errorf("plaid %s: %w", action, err)
}

func (c *Client) CreateLinkToken(ctx context.Context, userID string, products, requiredIfSupportedProducts, optionalProducts, additionalConsentedProducts []string, updateModeAccessToken string) (string, error) {
	req := plaidsdk.NewLinkTokenCreateRequest(
		"MyFi",
		"en",
		[]plaidsdk.CountryCode{plaidsdk.COUNTRYCODE_US},
	)
	req.SetUser(*plaidsdk.NewLinkTokenCreateRequestUser(userID))
	if updateModeAccessToken != "" {
		// Update mode: re-authenticate an existing Item; products must be omitted.
		req.SetAccessToken(updateModeAccessToken)
	} else {
		if len(products) == 0 {
			return "", fmt.Errorf("at least one product is required to create a link token outside update mode")
		}
		req.SetProducts(toPlaidProducts(products))
		// Required-if-supported products are enabled on the Item when the
		// selected institution supports them, without restricting which
		// institutions are selectable in Link the way a plain required
		// product would — lets one link mode cover institutions with
		// different account types (e.g. depository vs. credit-only) without
		// the user having to know in advance which applies.
		if len(requiredIfSupportedProducts) > 0 {
			req.SetRequiredIfSupportedProducts(toPlaidProducts(requiredIfSupportedProducts))
		}
		// Optional products are used only if the connected accounts support
		// them — unlike products, they never require a matching account type
		// to exist, so they can't block linking an Item that lacks one.
		if len(optionalProducts) > 0 {
			req.SetOptionalProducts(toPlaidProducts(optionalProducts))
		}
	}
	// Additional consented products grant the user's consent for a product
	// without requesting its data now — used in update mode to add transactions
	// consent to an already-linked Item so a reconnect enables banking sync.
	if len(additionalConsentedProducts) > 0 {
		req.SetAdditionalConsentedProducts(toPlaidProducts(additionalConsentedProducts))
	}
	// When transactions are requested (new link or reconnect), ask for the
	// maximum history the institution provides. Plaid defaults to only 90 days;
	// 730 (24 months) gives the budget as much context as possible.
	if productRequested("transactions", products, requiredIfSupportedProducts, optionalProducts, additionalConsentedProducts) {
		txnCfg := plaidsdk.NewLinkTokenTransactions()
		txnCfg.SetDaysRequested(transactionsDaysRequested)
		req.SetTransactions(*txnCfg)
	}
	resp, _, err := c.api.LinkTokenCreate(ctx).LinkTokenCreateRequest(*req).Execute()
	if err != nil {
		return "", wrapErr("link token create", err)
	}
	return resp.GetLinkToken(), nil
}

// transactionsDaysRequested is the historical transaction window asked of Plaid
// at link time — the product maximum (24 months). Institutions may return less.
const transactionsDaysRequested = 730

// productRequested reports whether product appears in any of the given lists.
func productRequested(product string, lists ...[]string) bool {
	for _, list := range lists {
		for _, p := range list {
			if p == product {
				return true
			}
		}
	}
	return false
}

func toPlaidProducts(products []string) []plaidsdk.Products {
	out := make([]plaidsdk.Products, 0, len(products))
	for _, p := range products {
		out = append(out, plaidsdk.Products(p))
	}
	return out
}

func (c *Client) ExchangePublicToken(ctx context.Context, publicToken string) (string, string, error) {
	req := plaidsdk.NewItemPublicTokenExchangeRequest(publicToken)
	resp, _, err := c.api.ItemPublicTokenExchange(ctx).ItemPublicTokenExchangeRequest(*req).Execute()
	if err != nil {
		return "", "", wrapErr("public token exchange", err)
	}
	return resp.GetAccessToken(), resp.GetItemId(), nil
}

func (c *Client) GetAccounts(ctx context.Context, accessToken string) ([]Account, error) {
	req := plaidsdk.NewAccountsGetRequest(accessToken)
	resp, _, err := c.api.AccountsGet(ctx).AccountsGetRequest(*req).Execute()
	if err != nil {
		return nil, wrapErr("accounts get", err)
	}
	return convertAccounts(resp.GetAccounts()), nil
}

func (c *Client) GetBalances(ctx context.Context, accessToken string) ([]Account, error) {
	req := plaidsdk.NewAccountsBalanceGetRequest(accessToken)
	resp, _, err := c.api.AccountsBalanceGet(ctx).AccountsBalanceGetRequest(*req).Execute()
	if err != nil {
		return nil, wrapErr("balances get", err)
	}
	return convertAccounts(resp.GetAccounts()), nil
}

func (c *Client) GetHoldings(ctx context.Context, accessToken string) ([]Holding, []Security, error) {
	req := plaidsdk.NewInvestmentsHoldingsGetRequest(accessToken)
	resp, _, err := c.api.InvestmentsHoldingsGet(ctx).InvestmentsHoldingsGetRequest(*req).Execute()
	if err != nil {
		return nil, nil, wrapErr("holdings get", err)
	}

	holdings := make([]Holding, 0, len(resp.GetHoldings()))
	for _, h := range resp.GetHoldings() {
		conv := Holding{
			AccountID:        h.GetAccountId(),
			SecurityID:       h.GetSecurityId(),
			Quantity:         h.GetQuantity(),
			InstitutionPrice: h.GetInstitutionPrice(),
			InstitutionValue: h.GetInstitutionValue(),
		}
		if cb, ok := h.GetCostBasisOk(); ok && cb != nil {
			v := *cb
			conv.CostBasis = &v
		}
		holdings = append(holdings, conv)
	}
	logHoldingsResponse(resp)
	return holdings, convertSecurities(resp.GetSecurities()), nil
}

func (c *Client) RefreshInvestments(ctx context.Context, accessToken string) error {
	req := plaidsdk.NewInvestmentsRefreshRequest(accessToken)
	if _, _, err := c.api.InvestmentsRefresh(ctx).InvestmentsRefreshRequest(*req).Execute(); err != nil {
		return wrapErr("investments refresh", err)
	}
	return nil
}

// logHoldingsResponse records what Plaid returned for an Item's holdings.
// An empty response is not an error, so without this a linked investment
// account that Plaid reports no positions for is indistinguishable from a
// sync that dropped them. The request_id is what Plaid Support or the
// Dashboard's logs need to look the call up. Account IDs are shortened.
func logHoldingsResponse(resp plaidsdk.InvestmentsHoldingsGetResponse) {
	perAccount := map[string]int{}
	for _, h := range resp.GetHoldings() {
		perAccount[h.GetAccountId()]++
	}
	accts := make([]string, 0, len(resp.GetAccounts()))
	for _, a := range resp.GetAccounts() {
		accts = append(accts, fmt.Sprintf("%s(%s/%s)=%d",
			ShortID(a.GetAccountId()), a.GetType(), a.GetSubtype(), perAccount[a.GetAccountId()]))
	}
	log.Printf("plaid: holdings get request_id=%s holdings=%d securities=%d accounts: %s",
		resp.GetRequestId(), len(resp.GetHoldings()), len(resp.GetSecurities()), strings.Join(accts, " "))
}

// ShortID trims a Plaid identifier to its last 6 characters for log lines.
func ShortID(id string) string {
	if len(id) > 6 {
		return id[len(id)-6:]
	}
	return id
}

// GetLiabilities fetches Plaid's liabilities data for an Item: mortgage terms
// (interest rate, original principal, origination date) and credit-card
// account data (APR, statement balance, minimum payment, due date). Callers
// match these back to MyFi accounts by AccountID.
func (c *Client) GetLiabilities(ctx context.Context, accessToken string) ([]MortgageLiability, []CreditCardLiability, error) {
	req := plaidsdk.NewLiabilitiesGetRequest(accessToken)
	resp, _, err := c.api.LiabilitiesGet(ctx).LiabilitiesGetRequest(*req).Execute()
	if err != nil {
		return nil, nil, wrapErr("liabilities get", err)
	}

	liab := resp.GetLiabilities()

	mortgages := make([]MortgageLiability, 0, len(liab.GetMortgage()))
	for _, m := range liab.GetMortgage() {
		ml := MortgageLiability{AccountID: m.GetAccountId()}
		if v := m.InterestRate.Percentage.Get(); v != nil {
			ml.InterestRatePercentage = v
		}
		if v := m.OriginationPrincipalAmount.Get(); v != nil {
			ml.OriginationPrincipalAmount = v
		}
		if v := m.OriginationDate.Get(); v != nil {
			if t, err := time.Parse("2006-01-02", *v); err == nil {
				ml.OriginationDate = &t
			}
		}
		mortgages = append(mortgages, ml)
	}

	creditCards := make([]CreditCardLiability, 0, len(liab.GetCredit()))
	for _, cc := range liab.GetCredit() {
		ccl := CreditCardLiability{}
		if v := cc.AccountId.Get(); v != nil {
			ccl.AccountID = *v
		}
		for _, apr := range cc.GetAprs() {
			if apr.AprType == "purchase_apr" {
				v := apr.AprPercentage
				ccl.APRPercentage = &v
				break
			}
		}
		if v := cc.IsOverdue.Get(); v != nil {
			ccl.IsOverdue = *v
		}
		if v := cc.LastStatementBalance.Get(); v != nil {
			ccl.LastStatementBalance = v
		}
		if v := cc.LastStatementIssueDate.Get(); v != nil {
			if t, err := time.Parse("2006-01-02", *v); err == nil {
				ccl.LastStatementIssueDate = &t
			}
		}
		if v := cc.MinimumPaymentAmount.Get(); v != nil {
			ccl.MinimumPaymentAmount = v
		}
		if v := cc.NextPaymentDueDate.Get(); v != nil {
			if t, err := time.Parse("2006-01-02", *v); err == nil {
				ccl.NextPaymentDueDate = &t
			}
		}
		if v := cc.LastPaymentAmount.Get(); v != nil {
			ccl.LastPaymentAmount = v
		}
		if v := cc.LastPaymentDate.Get(); v != nil {
			if t, err := time.Parse("2006-01-02", *v); err == nil {
				ccl.LastPaymentDate = &t
			}
		}
		creditCards = append(creditCards, ccl)
	}

	return mortgages, creditCards, nil
}

// investmentTxnPageSize is Plaid's maximum count per /investments/transactions/get page.
const investmentTxnPageSize = 500

func (c *Client) GetInvestmentTransactions(ctx context.Context, accessToken string, start, end time.Time, accountIDs []string) ([]InvestmentTransaction, []Security, error) {
	var (
		txns    []InvestmentTransaction
		secByID = map[string]Security{}
		offset  int32
	)
	for {
		req := plaidsdk.NewInvestmentsTransactionsGetRequest(
			accessToken, start.Format("2006-01-02"), end.Format("2006-01-02"),
		)
		opts := plaidsdk.NewInvestmentsTransactionsGetRequestOptions()
		count := int32(investmentTxnPageSize)
		opts.Count = &count
		opts.Offset = &offset
		if len(accountIDs) > 0 {
			opts.AccountIds = &accountIDs
		}
		req.SetOptions(*opts)

		resp, _, err := c.api.InvestmentsTransactionsGet(ctx).InvestmentsTransactionsGetRequest(*req).Execute()
		if err != nil {
			return nil, nil, wrapErr(fmt.Sprintf("investment transactions get (offset %d)", offset), err)
		}

		for _, s := range convertSecurities(resp.GetSecurities()) {
			secByID[s.SecurityID] = s
		}
		for _, t := range resp.GetInvestmentTransactions() {
			date, err := time.Parse("2006-01-02", t.GetDate())
			if err != nil {
				continue
			}
			txns = append(txns, InvestmentTransaction{
				ID:         t.GetInvestmentTransactionId(),
				AccountID:  t.GetAccountId(),
				Name:       t.GetName(),
				SecurityID: t.GetSecurityId(),
				Date:       date,
				Type:       string(t.GetType()),
				Subtype:    string(t.GetSubtype()),
				Quantity:   t.GetQuantity(),
				Amount:     t.GetAmount(),
				Price:      t.GetPrice(),
			})
		}

		offset += int32(len(resp.GetInvestmentTransactions()))
		if offset >= resp.GetTotalInvestmentTransactions() || len(resp.GetInvestmentTransactions()) == 0 {
			break
		}
	}

	securities := make([]Security, 0, len(secByID))
	for _, s := range secByID {
		securities = append(securities, s)
	}
	return txns, securities, nil
}

// transactionSyncPageSize is Plaid's maximum count per /transactions/sync page.
const transactionSyncPageSize = 500

func (c *Client) SyncTransactions(ctx context.Context, accessToken, cursor string) (*TransactionSyncPage, error) {
	req := plaidsdk.NewTransactionsSyncRequest(accessToken)
	if cursor != "" {
		req.SetCursor(cursor)
	}
	req.SetCount(transactionSyncPageSize)
	opts := plaidsdk.NewTransactionsSyncRequestOptions()
	opts.SetIncludePersonalFinanceCategory(true)
	req.SetOptions(*opts)

	resp, _, err := c.api.TransactionsSync(ctx).TransactionsSyncRequest(*req).Execute()
	if err != nil {
		return nil, wrapErr("transactions sync", err)
	}

	page := &TransactionSyncPage{
		NextCursor: resp.GetNextCursor(),
		HasMore:    resp.GetHasMore(),
	}
	for _, t := range resp.GetAdded() {
		page.Added = append(page.Added, convertBankTransaction(t))
	}
	for _, t := range resp.GetModified() {
		page.Modified = append(page.Modified, convertBankTransaction(t))
	}
	for _, r := range resp.GetRemoved() {
		if id := r.GetTransactionId(); id != "" {
			page.RemovedIDs = append(page.RemovedIDs, id)
		}
	}
	return page, nil
}

func convertBankTransaction(t plaidsdk.Transaction) BankTransaction {
	bt := BankTransaction{
		TransactionID: t.GetTransactionId(),
		AccountID:     t.GetAccountId(),
		Name:          t.GetName(),
		MerchantName:  t.GetMerchantName(),
		Amount:        t.GetAmount(),
		Currency:      t.GetIsoCurrencyCode(),
		Pending:       t.GetPending(),
	}
	if date, err := time.Parse("2006-01-02", t.GetDate()); err == nil {
		bt.Date = date
	}
	if pfc, ok := t.GetPersonalFinanceCategoryOk(); ok && pfc != nil {
		bt.PFCPrimary = pfc.GetPrimary()
		bt.PFCDetailed = pfc.GetDetailed()
	}
	return bt
}

func (c *Client) RemoveItem(ctx context.Context, accessToken string) error {
	req := plaidsdk.NewItemRemoveRequest(accessToken)
	if _, _, err := c.api.ItemRemove(ctx).ItemRemoveRequest(*req).Execute(); err != nil {
		return wrapErr("item remove", err)
	}
	return nil
}

func convertAccounts(accts []plaidsdk.AccountBase) []Account {
	out := make([]Account, 0, len(accts))
	for _, a := range accts {
		conv := Account{
			AccountID: a.GetAccountId(),
			Name:      a.GetName(),
			Mask:      a.GetMask(),
			Type:      string(a.GetType()),
			Subtype:   string(a.GetSubtype()),
			Currency:  a.Balances.GetIsoCurrencyCode(),
		}
		if cur, ok := a.Balances.GetCurrentOk(); ok && cur != nil {
			v := *cur
			conv.CurrentBalance = &v
		}
		if limit, ok := a.Balances.GetLimitOk(); ok && limit != nil {
			v := *limit
			conv.Limit = &v
		}
		out = append(out, conv)
	}
	return out
}

func convertSecurities(secs []plaidsdk.Security) []Security {
	out := make([]Security, 0, len(secs))
	for _, s := range secs {
		out = append(out, Security{
			SecurityID: s.GetSecurityId(),
			Ticker:     s.GetTickerSymbol(),
			CUSIP:      s.GetCusip(),
			Name:       s.GetName(),
			Type:       s.GetType(),
		})
	}
	return out
}
