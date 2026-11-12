package ofx

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/google/uuid"

	"github.com/cybirdsecurity/myfi-os/internal/db"
	"github.com/cybirdsecurity/myfi-os/internal/models"
	"github.com/cybirdsecurity/myfi-os/internal/services/ofx/institutions"
)

// HoldingDiffStatus describes how a position differs from what's currently in the DB.
type HoldingDiffStatus string

const (
	DiffAdded     HoldingDiffStatus = "added"
	DiffRemoved   HoldingDiffStatus = "removed"
	DiffChanged   HoldingDiffStatus = "changed"
	DiffUnchanged HoldingDiffStatus = "unchanged"
)

// HoldingDiff represents a single ticker's before/after state.
type HoldingDiff struct {
	Ticker     string            `json:"ticker"`
	Name       string            `json:"name"`
	AssetClass string            `json:"assetClass"`
	Status     HoldingDiffStatus `json:"status"`
	OFXShares  *float64          `json:"ofxShares,omitempty"`
	OFXPrice   *float64          `json:"ofxPrice,omitempty"`
	CurShares  *float64          `json:"curShares,omitempty"`
	CurPrice   *float64          `json:"curPrice,omitempty"`
}

// AccountPreview is one OFX account's parsed data plus the diff vs current holdings.
type AccountPreview struct {
	OFXAccountID    string        `json:"ofxAccountId"`
	BrokerID        string        `json:"brokerId"`
	Institution     string        `json:"institution"`
	LinkedAccountID *string       `json:"linkedAccountId"` // null if not yet mapped
	Diffs           []HoldingDiff `json:"diffs"`
}

// PreviewResult is the full response to an OFX preview request.
type PreviewResult struct {
	Accounts []AccountPreview `json:"accounts"`
}

// ApplyMapping pairs an OFX account ID with the user's chosen MyFi account.
type ApplyMapping struct {
	OFXAccountID  string    `json:"ofxAccountId"`
	MyFiAccountID uuid.UUID `json:"myfiAccountId"`
}

// ApplyAccepted identifies one accepted diff row by account + ticker.
type ApplyAccepted struct {
	OFXAccountID string `json:"ofxAccountId"`
	Ticker       string `json:"ticker"`
}

// ApplyResult summarises the outcome of a holdings replace.
type ApplyResult struct {
	Replaced int `json:"replaced"`
	Skipped  int `json:"skipped"`
}

// Service orchestrates OFX file parsing, diff computation, and holdings replacement.
type Service struct {
	accounts *db.AccountStore
	holdings *db.HoldingStore
}

func NewService(accounts *db.AccountStore, holdings *db.HoldingStore) *Service {
	return &Service{accounts: accounts, holdings: holdings}
}

// Preview parses an OFX file and returns a diff per account without writing anything.
// For each OFX account found, it looks up whether it's already linked to a MyFi account
// (via external_account_id) and computes the holding-level diff.
func (s *Service) Preview(ctx context.Context, userID uuid.UUID, r io.Reader) (*PreviewResult, error) {
	stmts, err := ParseOFX(r)
	if err != nil {
		return nil, err
	}
	if len(stmts) == 0 {
		return nil, fmt.Errorf("no investment statements found in this OFX file")
	}

	result := &PreviewResult{}

	for _, stmt := range stmts {
		inst := institutions.Detect(stmt.BrokerID)

		// Check whether this OFX account is already linked to a MyFi account.
		existing, _ := s.accounts.FindByExternalID(ctx, userID, providerKey(stmt.BrokerID), stmt.AccountID)

		preview := AccountPreview{
			OFXAccountID: stmt.AccountID,
			BrokerID:     stmt.BrokerID,
			Institution:  inst.Name(),
		}
		if existing != nil {
			id := existing.ID.String()
			preview.LinkedAccountID = &id
		}

		// Build current holdings map for diff.
		var curHoldings []models.Holding
		if existing != nil {
			curHoldings, _ = s.holdings.ListByAccount(ctx, existing.ID, userID)
		}
		curMap := make(map[string]models.Holding, len(curHoldings))
		for _, h := range curHoldings {
			curMap[h.Ticker] = h
		}

		secInfoMap := toSecInfoMap(stmt.SecList)

		// Build diffs from OFX positions.
		seen := make(map[string]bool)
		for _, pos := range stmt.Positions {
			ticker := inst.ResolveTicker(pos.UniqueID, pos.IDType, pos.PosType, secInfoMap)
			if ticker == "" {
				continue
			}
			ticker = strings.ToUpper(ticker)
			seen[ticker] = true

			assetClass := inst.InferAssetClass(pos.UniqueID, pos.IDType, pos.PosType)
			name := ""
			if sec, ok := stmt.SecList[pos.UniqueID]; ok {
				name = sec.Name
			}

			units := pos.Units
			price := pos.UnitPrice

			diff := HoldingDiff{
				Ticker:     ticker,
				Name:       name,
				AssetClass: assetClass,
				OFXShares:  &units,
				OFXPrice:   &price,
			}

			if cur, ok := curMap[ticker]; ok {
				diff.CurShares = &cur.Shares
				diff.CurPrice = cur.PricePerShare
				if abs(cur.Shares-units) < 0.00001 {
					diff.Status = DiffUnchanged
				} else {
					diff.Status = DiffChanged
				}
			} else {
				diff.Status = DiffAdded
			}

			preview.Diffs = append(preview.Diffs, diff)
		}

		// Mark tickers that exist in DB but not in OFX as removed.
		for ticker, cur := range curMap {
			if !seen[ticker] {
				shares := cur.Shares
				name := ""
				if cur.Name != nil {
					name = *cur.Name
				}
				diff := HoldingDiff{
					Ticker:     ticker,
					Name:       name,
					AssetClass: cur.AssetClass,
					Status:     DiffRemoved,
					CurShares:  &shares,
					CurPrice:   cur.PricePerShare,
				}
				preview.Diffs = append(preview.Diffs, diff)
			}
		}

		result.Accounts = append(result.Accounts, preview)
	}

	return result, nil
}

// Apply writes accepted holdings replacements to the database and records the
// account linkage (external_provider + external_account_id) for future imports.
func (s *Service) Apply(
	ctx context.Context,
	userID uuid.UUID,
	stmts []ParsedStatement,
	mappings []ApplyMapping,
	accepted []ApplyAccepted,
) (*ApplyResult, error) {
	// Build lookup structures.
	mappingByOFX := make(map[string]uuid.UUID, len(mappings))
	for _, m := range mappings {
		mappingByOFX[m.OFXAccountID] = m.MyFiAccountID
	}

	acceptedSet := make(map[string]map[string]bool) // ofxAccountID → set of tickers
	for _, a := range accepted {
		if acceptedSet[a.OFXAccountID] == nil {
			acceptedSet[a.OFXAccountID] = make(map[string]bool)
		}
		acceptedSet[a.OFXAccountID][strings.ToUpper(a.Ticker)] = true
	}

	result := &ApplyResult{}

	for _, stmt := range stmts {
		accountID, ok := mappingByOFX[stmt.AccountID]
		if !ok {
			continue // unmapped accounts are skipped
		}

		// Verify the user owns this account.
		acc, err := s.accounts.Get(ctx, accountID, userID)
		if err != nil || acc == nil {
			result.Skipped++
			continue
		}

		inst := institutions.Detect(stmt.BrokerID)
		tickersAccepted := acceptedSet[stmt.AccountID]
		secInfoMap := toSecInfoMap(stmt.SecList)

		// Build the OFX ticker set for this account.
		ofxTickers := make(map[string]bool)
		for _, pos := range stmt.Positions {
			if t := strings.ToUpper(inst.ResolveTicker(pos.UniqueID, pos.IDType, pos.PosType, secInfoMap)); t != "" {
				ofxTickers[t] = true
			}
		}

		// Collect holdings to replace: accepted OFX positions → delete+insert.
		var toReplace []db.CreateHoldingParams
		for _, pos := range stmt.Positions {
			ticker := strings.ToUpper(inst.ResolveTicker(pos.UniqueID, pos.IDType, pos.PosType, secInfoMap))
			if ticker == "" || !tickersAccepted[ticker] {
				result.Skipped++
				continue
			}

			assetClass := inst.InferAssetClass(pos.UniqueID, pos.IDType, pos.PosType)
			var name *string
			if sec, ok := stmt.SecList[pos.UniqueID]; ok && sec.Name != "" {
				n := sec.Name
				name = &n
			}
			price := pos.UnitPrice
			toReplace = append(toReplace, db.CreateHoldingParams{
				AccountID:     accountID,
				UserID:        userID,
				Ticker:        ticker,
				Name:          name,
				AssetClass:    assetClass,
				Shares:        pos.Units,
				PricePerShare: &price,
			})
		}

		// Accepted "removed" tickers (in DB, not in OFX) — delete with zero-share sentinel.
		for ticker := range tickersAccepted {
			if !ofxTickers[ticker] {
				toReplace = append(toReplace, db.CreateHoldingParams{
					AccountID:  accountID,
					UserID:     userID,
					Ticker:     ticker,
					AssetClass: "other",
					Shares:     0, // sentinel: DELETE fires, INSERT skipped
				})
			}
		}

		if len(toReplace) > 0 {
			n, err := s.holdings.ReplaceForTickers(ctx, accountID, userID, toReplace)
			if err != nil {
				return nil, fmt.Errorf("apply holdings for account %s: %w", accountID, err)
			}
			result.Replaced += n
		}

		// Persist the account linkage.
		_ = s.accounts.LinkExternal(ctx, accountID, userID, providerKey(stmt.BrokerID), stmt.AccountID)
	}

	return result, nil
}

// providerKey normalises a BrokerID to a short lowercase provider key stored in external_provider.
func providerKey(brokerID string) string {
	lower := strings.ToLower(brokerID)
	switch {
	case strings.Contains(lower, "vanguard"):
		return "vanguard"
	default:
		return strings.ToLower(strings.ReplaceAll(brokerID, ".", "_"))
	}
}

func abs(f float64) float64 {
	if f < 0 {
		return -f
	}
	return f
}

// toSecInfoMap converts the parser's Security map into the institutions.SecurityInfo map
// used by institution overrides, avoiding a circular import.
func toSecInfoMap(secList map[string]Security) map[string]institutions.SecurityInfo {
	m := make(map[string]institutions.SecurityInfo, len(secList))
	for k, v := range secList {
		m[k] = institutions.SecurityInfo{Ticker: v.Ticker, Name: v.Name}
	}
	return m
}
