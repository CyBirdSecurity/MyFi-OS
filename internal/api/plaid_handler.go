package api

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/cybirdsecurity/myfi-os/internal/models"
	plaidsvc "github.com/cybirdsecurity/myfi-os/internal/services/plaid"
)

// plaidEnabled guards every Plaid handler: the service is nil when
// encryption key isn't configured.
func (s *Server) plaidEnabled(w http.ResponseWriter) bool {
	if s.plaid == nil {
		writeError(w, http.StatusServiceUnavailable, "PLAID_DISABLED", "Plaid is not enabled on this server")
		return false
	}
	return true
}

// handlePlaidLinkToken mints a Plaid Link token.
//
// POST /plaid/link-token
// Body: {"mode": "investments"|"banking", "itemId": "..."} —
// itemId switches to update mode for re-authenticating an existing Item.
func (s *Server) handlePlaidLinkToken(w http.ResponseWriter, r *http.Request) {
	if !s.plaidEnabled(w) {
		return
	}
	userID := userIDFromCtx(r)

	var body struct {
		Mode   string `json:"mode"`
		ItemID string `json:"itemId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid JSON")
		return
	}
	if body.Mode == "" {
		body.Mode = "investments"
	}

	token, err := s.plaid.CreateLinkToken(r.Context(), userID, body.Mode, body.ItemID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "PLAID_ERROR", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"linkToken": token})
}

// handlePlaidExchange trades a Link public_token for an access token, stores
// the Item, and returns the account preview for the mapping step.
//
// POST /plaid/exchange
// Body: {"publicToken": "...", "institutionId": "...", "institutionName": "..."}
func (s *Server) handlePlaidExchange(w http.ResponseWriter, r *http.Request) {
	if !s.plaidEnabled(w) {
		return
	}
	userID := userIDFromCtx(r)

	var body struct {
		PublicToken     string `json:"publicToken"`
		InstitutionID   string `json:"institutionId"`
		InstitutionName string `json:"institutionName"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.PublicToken == "" {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "publicToken is required")
		return
	}

	preview, err := s.plaid.Exchange(r.Context(), userID, body.PublicToken, body.InstitutionID, body.InstitutionName)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "PLAID_ERROR", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, preview)
}

// plaidItemResponse is one connected institution with its linked accounts.
// The embedded PlaidItem excludes the encrypted access token from JSON.
type plaidItemResponse struct {
	models.PlaidItem
	Accounts []plaidLinkedAccountInfo `json:"accounts"`
}

type plaidLinkedAccountInfo struct {
	MyFiAccountID  string `json:"myfiAccountId"`
	Name           string `json:"name"`
	PlaidAccountID string `json:"plaidAccountId"`
}

// handlePlaidListItems returns the user's connected institutions.
//
// GET /plaid/items
func (s *Server) handlePlaidListItems(w http.ResponseWriter, r *http.Request) {
	if !s.plaidEnabled(w) {
		return
	}
	userID := userIDFromCtx(r)

	items, err := s.plaidItems.ListByUser(r.Context(), userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "DB_ERROR", err.Error())
		return
	}

	resp := make([]plaidItemResponse, 0, len(items))
	for _, item := range items {
		ir := plaidItemResponse{
			PlaidItem: item,
			Accounts:  []plaidLinkedAccountInfo{},
		}
		linked, err := s.accounts.ListByExternalItem(r.Context(), userID, "plaid", item.ItemID)
		if err == nil {
			for _, acc := range linked {
				info := plaidLinkedAccountInfo{MyFiAccountID: acc.ID.String(), Name: acc.Name}
				if acc.ExternalAccountID != nil {
					info.PlaidAccountID = *acc.ExternalAccountID
				}
				ir.Accounts = append(ir.Accounts, info)
			}
		}
		resp = append(resp, ir)
	}
	writeJSON(w, http.StatusOK, resp)
}

// handlePlaidApplyMappings attaches Plaid accounts to MyFi accounts, runs an
// immediate sync, and optionally starts the historical backfill.
//
// POST /plaid/items/{itemId}/mappings
// Body: {"mappings": [{"plaidAccountId": "...", "myfiAccountId": "..."} |
//
//	{"plaidAccountId": "...", "createNew": {...}}], "backfill": true}
func (s *Server) handlePlaidApplyMappings(w http.ResponseWriter, r *http.Request) {
	if !s.plaidEnabled(w) {
		return
	}
	userID := userIDFromCtx(r)
	itemID := chi.URLParam(r, "itemId")

	var body struct {
		Mappings []plaidsvc.AccountMapping `json:"mappings"`
		Backfill bool                      `json:"backfill"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid JSON")
		return
	}
	if len(body.Mappings) == 0 {
		writeError(w, http.StatusBadRequest, "VALIDATION", "at least one account mapping is required")
		return
	}

	result, err := s.plaid.ApplyMappings(r.Context(), userID, itemID, body.Mappings, body.Backfill)
	if err != nil {
		writeError(w, http.StatusConflict, "APPLY_ERROR", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, result)
}

// handlePlaidSyncItem manually syncs one item.
//
// POST /plaid/items/{itemId}/sync
func (s *Server) handlePlaidSyncItem(w http.ResponseWriter, r *http.Request) {
	if !s.plaidEnabled(w) {
		return
	}
	userID := userIDFromCtx(r)
	itemID := chi.URLParam(r, "itemId")

	item, err := s.plaidItems.GetByItemID(r.Context(), userID, itemID)
	if err != nil {
		writeError(w, http.StatusNotFound, "NOT_FOUND", "item not found")
		return
	}
	result, err := s.plaid.SyncItem(r.Context(), *item)
	if err != nil {
		writeError(w, http.StatusBadGateway, "SYNC_ERROR", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, result)
}

// handlePlaidSyncAll manually syncs all of the user's items.
//
// POST /plaid/sync
func (s *Server) handlePlaidSyncAll(w http.ResponseWriter, r *http.Request) {
	if !s.plaidEnabled(w) {
		return
	}
	userID := userIDFromCtx(r)

	result, err := s.plaid.SyncUserItems(r.Context(), userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "SYNC_ERROR", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, result)
}

// handlePlaidRemoveItem unlinks the item's accounts (they revert to manual
// tracking with all data intact) and removes the item.
//
// DELETE /plaid/items/{itemId}
func (s *Server) handlePlaidRemoveItem(w http.ResponseWriter, r *http.Request) {
	if !s.plaidEnabled(w) {
		return
	}
	userID := userIDFromCtx(r)
	itemID := chi.URLParam(r, "itemId")

	if err := s.plaid.RemoveItem(r.Context(), userID, itemID); err != nil {
		writeError(w, http.StatusInternalServerError, "REMOVE_ERROR", err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// plaidSyncInfoResponse combines the Item's persisted sync state (which
// reflects the last sync regardless of whether it was scheduled or manual)
// with the actual date range of transactions synced for this specific
// account, so the UI can distinguish "sync failed", "sync succeeded but the
// institution/Plaid only returned recent data", and "sync succeeded and the
// expected history is present."
type plaidSyncInfoResponse struct {
	Status           string     `json:"status"`
	LastSyncedAt     *time.Time `json:"lastSyncedAt,omitempty"`
	LastSyncError    *string    `json:"lastSyncError,omitempty"`
	EarliestTxnDate  *time.Time `json:"earliestTransactionDate,omitempty"`
	TransactionCount int        `json:"transactionCount"`
}

// handlePlaidAccountSyncInfo returns sync diagnostics for one Plaid-linked
// account: the Item's persisted status/error (from the last sync of any
// kind — scheduled or manual) plus the earliest transaction date and count
// actually on file for this account.
//
// GET /accounts/{id}/plaid-sync-info
func (s *Server) handlePlaidAccountSyncInfo(w http.ResponseWriter, r *http.Request) {
	if !s.plaidEnabled(w) {
		return
	}
	userID := userIDFromCtx(r)
	accountID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid account id")
		return
	}

	acc, err := s.accounts.Get(r.Context(), accountID, userID)
	if err != nil || acc == nil || acc.ExternalProvider == nil || *acc.ExternalProvider != "plaid" || acc.ExternalItemID == nil {
		writeError(w, http.StatusNotFound, "NOT_FOUND", "account is not Plaid-linked")
		return
	}

	item, err := s.plaidItems.GetByItemID(r.Context(), userID, *acc.ExternalItemID)
	if err != nil {
		writeError(w, http.StatusNotFound, "NOT_FOUND", "plaid item not found")
		return
	}

	rng, err := s.transactions.PlaidDateRange(r.Context(), userID, accountID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "SERVER_ERROR", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, plaidSyncInfoResponse{
		Status:           item.Status,
		LastSyncedAt:     item.LastSyncedAt,
		LastSyncError:    item.LastSyncError,
		EarliestTxnDate:  rng.EarliestDate,
		TransactionCount: rng.Count,
	})
}
