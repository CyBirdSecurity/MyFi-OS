package plaid

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/cybirdsecurity/myfi-os/internal/crypto"
	"github.com/cybirdsecurity/myfi-os/internal/models"
)

// removeAPI is a fakeAPI whose RemoveItem outcome is configurable.
type removeAPI struct {
	fakeAPI
	removeErr error
	removed   int
}

func (a *removeAPI) RemoveItem(context.Context, string) error {
	a.removed++
	return a.removeErr
}

func cleanupService(t *testing.T, api *removeAPI, items *fakeItemStore) *Service {
	t.Helper()
	cipher, _ := crypto.NewCipher(testHexKey)
	return &Service{clientFor: fixedAPI(api), cipher: cipher, items: items}
}

func unlinkedItem(t *testing.T, s *Service) models.PlaidItem {
	t.Helper()
	return models.PlaidItem{ID: uuid.New(), UserID: uuid.New(), ItemID: "item-" + uuid.NewString(), AccessTokenEncrypted: encryptedToken(t, s.cipher)}
}

func TestRemoveUnlinkedItems_RevokesAtPlaidThenDeletes(t *testing.T) {
	api := &removeAPI{}
	items := &fakeItemStore{}
	svc := cleanupService(t, api, items)
	items.unlinked = []models.PlaidItem{unlinkedItem(t, svc), unlinkedItem(t, svc)}

	n, err := svc.RemoveUnlinkedItems(context.Background(), time.Hour)
	if err != nil || n != 2 {
		t.Fatalf("removed %d err=%v, want 2", n, err)
	}
	if api.removed != 2 || len(items.deleted) != 2 {
		t.Errorf("plaid removals=%d local deletes=%d, want 2 and 2", api.removed, len(items.deleted))
	}
}

func TestRemoveUnlinkedItems_KeepsItemWhenPlaidRemovalFails(t *testing.T) {
	api := &removeAPI{removeErr: errors.New("plaid unavailable")}
	items := &fakeItemStore{}
	svc := cleanupService(t, api, items)
	items.unlinked = []models.PlaidItem{unlinkedItem(t, svc)}

	n, err := svc.RemoveUnlinkedItems(context.Background(), time.Hour)
	if err != nil || n != 0 {
		t.Fatalf("removed %d err=%v, want 0", n, err)
	}
	if len(items.deleted) != 0 {
		t.Error("an Item Plaid still holds must not be forgotten locally; it would keep counting with no way to remove it")
	}
}

func TestRemoveUnlinkedItems_ItemPlaidDoesNotKnowCountsAsRemoved(t *testing.T) {
	orig := errorCodeOf
	errorCodeOf = func(error) string { return "ITEM_NOT_FOUND" }
	t.Cleanup(func() { errorCodeOf = orig })

	api := &removeAPI{removeErr: errors.New("item not found")}
	items := &fakeItemStore{}
	svc := cleanupService(t, api, items)
	items.unlinked = []models.PlaidItem{unlinkedItem(t, svc)}

	if n, _ := svc.RemoveUnlinkedItems(context.Background(), time.Hour); n != 1 || len(items.deleted) != 1 {
		t.Errorf("removed=%d deleted=%d, want the stale record cleared", n, len(items.deleted))
	}
}

func TestRemoveUnlinkedItems_UndecryptableTokenIsClearedWithoutCallingPlaid(t *testing.T) {
	api := &removeAPI{}
	items := &fakeItemStore{}
	svc := cleanupService(t, api, items)
	item := unlinkedItem(t, svc)
	item.AccessTokenEncrypted = "REVOKED-DEMO-TOKEN-NOT-DECRYPTABLE"
	items.unlinked = []models.PlaidItem{item}

	if n, _ := svc.RemoveUnlinkedItems(context.Background(), time.Hour); n != 1 {
		t.Errorf("removed=%d, want 1", n)
	}
	if api.removed != 0 {
		t.Error("Plaid cannot be called for a token that can't be decrypted")
	}
}

func TestRemoveAllItems_DeletesConfirmedAndReportsFailures(t *testing.T) {
	user := uuid.New()
	api := &removeAPI{}
	items := &fakeItemStore{}
	svc := cleanupService(t, api, items)
	good := unlinkedItem(t, svc)
	good.UserID = user
	bad := unlinkedItem(t, svc)
	bad.UserID = user
	bad.InstitutionName = strPtr("Marcus")
	items.items = []models.PlaidItem{good, bad}

	// Fail only the second revoke.
	calls := 0
	svc.clientFor = func(context.Context, uuid.UUID) (API, error) {
		return &funcRemoveAPI{remove: func() error {
			calls++
			if calls == 2 {
				return errors.New("plaid unavailable")
			}
			return nil
		}}, nil
	}

	failed, err := svc.RemoveAllItems(context.Background(), user)
	if err != nil {
		t.Fatalf("RemoveAllItems: %v", err)
	}
	if len(failed) != 1 || failed[0] != "Marcus" {
		t.Errorf("failed = %v, want [Marcus]", failed)
	}
	if len(items.deleted) != 1 || items.deleted[0] != good.ID {
		t.Errorf("deleted = %v, want only the connection Plaid confirmed removed", items.deleted)
	}
}

type funcRemoveAPI struct {
	fakeAPI
	remove func() error
}

func (a *funcRemoveAPI) RemoveItem(context.Context, string) error { return a.remove() }

func strPtr(s string) *string { return &s }
