package plaid

import (
	"context"
	"fmt"
	"log"
	"math"

	"github.com/google/uuid"

	"github.com/cybirdsecurity/myfi-os/internal/db"
	"github.com/cybirdsecurity/myfi-os/internal/models"
)

// maxTxnSyncPages bounds a single sync run to 50 pages (~25k transactions) so
// a runaway cursor loop can't spin forever. The first sync of a fresh Item
// pulls its full available history; later syncs return only deltas.
const maxTxnSyncPages = 50

// isBankPlaidType reports whether a Plaid account type carries the banking
// transactions the budget consumes (depository, credit, loan). Investment
// accounts are handled separately via holdings/investment transactions.
func isBankPlaidType(plaidType string) bool {
	switch plaidType {
	case "depository", "credit", "loan":
		return true
	default:
		return false
	}
}

// syncItemTransactions pulls banking transactions for the Item's linked
// depository/credit accounts via /transactions/sync, categorises them without
// an LLM call, and upserts them into the budget. It is best-effort: any error
// is recorded on the result but never fails the enclosing holdings/balance
// sync. The cursor is persisted only after the full delta set is applied, so
// an interrupted run safely re-fetches from the last durable cursor (upserts
// are idempotent on external_transaction_id).
func (s *Service) syncItemTransactions(
	ctx context.Context,
	api API,
	item models.PlaidItem,
	token string,
	linkedByPlaidID map[string]models.Account,
	plaidAccounts []Account,
	result *SyncResult,
) {
	if s.transactions == nil || s.classifier == nil {
		return // transaction sync not configured
	}

	// Skip Items with no linked bank account — calling /transactions/sync on an
	// investments-only Item would just error on a product it never consented to.
	hasBank := false
	for _, pa := range plaidAccounts {
		if _, ok := linkedByPlaidID[pa.AccountID]; ok && isBankPlaidType(pa.Type) {
			hasBank = true
			break
		}
	}
	if !hasBank {
		return
	}

	incomeID := s.classifier.CategoryIDByName(CategoryIncome)
	transferID := s.classifier.CategoryIDByName(CategoryTransfer)

	cursor := ""
	if item.TransactionsCursor != nil {
		cursor = *item.TransactionsCursor
	}

	for page := 0; page < maxTxnSyncPages; page++ {
		pg, err := api.SyncTransactions(ctx, token, cursor)
		if err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("%s: transactions: %v", displayName(item), err))
			return
		}

		for _, id := range pg.RemovedIDs {
			if err := s.transactions.DeleteByExternalID(ctx, item.UserID, "plaid", id); err != nil {
				log.Printf("plaid: delete txn %s: %v", id, err)
			}
		}

		// Added and modified both funnel through the idempotent upsert.
		batch := make([]BankTransaction, 0, len(pg.Added)+len(pg.Modified))
		batch = append(batch, pg.Added...)
		batch = append(batch, pg.Modified...)
		result.TransactionsSynced += s.applyBankTransactions(ctx, item.UserID, batch, linkedByPlaidID, incomeID, transferID)

		cursor = pg.NextCursor
		if !pg.HasMore {
			break
		}
	}

	if err := s.items.SetTransactionsCursor(ctx, item.ID, cursor); err != nil {
		log.Printf("plaid: persist transactions cursor for item %s: %v", item.ItemID, err)
	}
}

// applyBankTransactions translates a batch of Plaid transactions into MyFi
// budget rows and upserts them. Returns the number of newly inserted rows
// (updates to existing rows aren't counted). Pending transactions are skipped
// — Plaid re-delivers them as posted (a removed pending id + an added posted
// id) once they settle.
func (s *Service) applyBankTransactions(
	ctx context.Context,
	userID uuid.UUID,
	txns []BankTransaction,
	linkedByPlaidID map[string]models.Account,
	incomeID, transferID uuid.UUID,
) int {
	params := make([]db.UpsertExternalParams, 0, len(txns))

	// Spend transactions with no PFC hint are batch-classified by vendor rules
	// and merchant history (no LLM). Track their index in params to backfill.
	var classifyIdx []int
	var classifyMerchants []string

	for _, t := range txns {
		if t.Pending {
			continue
		}
		acc, ok := linkedByPlaidID[t.AccountID]
		if !ok {
			continue // transaction for an unlinked account — can't attribute it
		}

		merchant := t.MerchantName
		if merchant == "" {
			merchant = t.Name
		}

		class, hint := ClassifyPFC(t.PFCPrimary, t.PFCDetailed, t.Amount)
		amount := t.Amount
		var categoryID *uuid.UUID

		switch class {
		case TxnTransfer:
			// Money movement: store positive under the Transfer system category
			// so the budget's spend/income queries exclude it.
			amount = math.Abs(amount)
			if transferID != uuid.Nil {
				categoryID = &transferID
			}
		case TxnIncome:
			// Inflows are negative in Plaid; store positive under Income so
			// MonthlyIncomeTotal (which sums amount > 0) picks them up.
			amount = math.Abs(amount)
			if incomeID != uuid.Nil {
				categoryID = &incomeID
			}
		case TxnSpend:
			if hint != "" {
				if id := s.classifier.CategoryIDByName(hint); id != uuid.Nil {
					categoryID = &id
				}
			}
		}

		accID := acc.ID
		label := acc.Name
		p := db.UpsertExternalParams{
			UserID:                userID,
			ExternalTransactionID: t.TransactionID,
			ExternalProvider:      "plaid",
			TransDate:             t.Date,
			Merchant:              merchant,
			Amount:                math.Round(amount*100) / 100,
			CategoryID:            categoryID,
			AccountID:             &accID,
			AccountLabel:          &label,
			RawDescription:        t.Name,
		}

		// Unresolved spend → defer to rules/history classification below.
		if class == TxnSpend && categoryID == nil {
			classifyIdx = append(classifyIdx, len(params))
			classifyMerchants = append(classifyMerchants, merchant)
		}
		params = append(params, p)
	}

	if len(classifyIdx) > 0 {
		srcHints := make([]string, len(classifyMerchants)) // no source-category hint; PFC already tried
		catIDs, needsReview := s.classifier.ClassifyBatchRulesOnly(ctx, userID, classifyMerchants, srcHints)
		for i, idx := range classifyIdx {
			params[idx].CategoryID = catIDs[i]
			params[idx].NeedsReview = needsReview[i]
		}
	}

	inserted := 0
	for _, p := range params {
		// Prevent a duplicate of a transaction the user already imported by CSV:
		// adopt the matching manual row instead of inserting a second copy.
		if adopted, err := s.transactions.TryAdoptExternalMatch(ctx, p); err != nil {
			log.Printf("plaid: adopt match for txn %s: %v", p.ExternalTransactionID, err)
		} else if adopted {
			inserted++
			continue
		}
		wasInserted, err := s.transactions.UpsertExternal(ctx, p)
		if err != nil {
			log.Printf("plaid: upsert txn %s: %v", p.ExternalTransactionID, err)
			continue
		}
		if wasInserted {
			inserted++
		}
	}
	return inserted
}
