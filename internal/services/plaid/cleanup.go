package plaid

import (
	"context"
	"log"
	"time"

	"github.com/google/uuid"

	"github.com/cybirdsecurity/myfi-os/internal/models"
)

// errorCodeOf extracts a Plaid error code; a variable so tests can simulate
// Plaid errors without constructing SDK errors.
var errorCodeOf = ErrorCode

// RemoveUnlinkedItems revokes and deletes Items older than minAge that never
// got a linked account (an abandoned or failed mapping step). Unlike
// RemoveItem, an Item is only deleted locally once Plaid confirms it is gone:
// dropping our record while Plaid still holds the Item would leave a
// connection that keeps counting against the plan's limit with no way to
// remove it. Failures are logged and retried on the next run. It returns how
// many Items were removed.
func (s *Service) RemoveUnlinkedItems(ctx context.Context, minAge time.Duration) (int, error) {
	items, err := s.items.ListUnlinked(ctx, minAge)
	if err != nil {
		return 0, err
	}
	removed := 0
	for _, item := range items {
		if err := s.revokeAtPlaid(ctx, item); err != nil {
			log.Printf("plaid: cleanup of unlinked item %s: %v", item.ItemID, err)
			continue
		}
		if err := s.items.Delete(ctx, item.ID, item.UserID); err != nil {
			log.Printf("plaid: delete unlinked item %s: %v", item.ItemID, err)
			continue
		}
		removed++
	}
	return removed, nil
}

// revokeAtPlaid removes the Item at Plaid. An Item Plaid no longer knows, or
// whose stored token can't be decrypted (so Plaid could never be called for
// it), counts as already removed.
func (s *Service) revokeAtPlaid(ctx context.Context, item models.PlaidItem) error {
	token, err := s.cipher.Decrypt(item.AccessTokenEncrypted)
	if err != nil {
		return nil
	}
	api, err := s.apiForItem(ctx, item)
	if err != nil {
		return err
	}
	if err := api.RemoveItem(ctx, token); err != nil {
		switch errorCodeOf(err) {
		case "ITEM_NOT_FOUND", "INVALID_ACCESS_TOKEN":
			return nil
		}
		return err
	}
	return nil
}

// RemoveAllItems revokes every connection the user has at Plaid and deletes
// our record of each one that Plaid confirms gone. It is used when a user is
// deleted, so their connections don't linger in their Plaid account and keep
// counting against its plan. Connections Plaid could not remove are kept and
// their institution names returned, so the caller can stop instead of
// forgetting connections that still exist.
func (s *Service) RemoveAllItems(ctx context.Context, userID uuid.UUID) (failed []string, err error) {
	items, err := s.items.ListByUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	for _, item := range items {
		if err := s.revokeAtPlaid(ctx, item); err != nil {
			log.Printf("plaid: revoke item %s for deleted user: %v", item.ItemID, err)
			failed = append(failed, displayName(item))
			continue
		}
		if err := s.items.Delete(ctx, item.ID, item.UserID); err != nil {
			return failed, err
		}
	}
	return failed, nil
}
