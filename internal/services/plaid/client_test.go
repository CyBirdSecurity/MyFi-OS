package plaid

import (
	"errors"
	"fmt"
	"testing"
)

func TestProductRequested(t *testing.T) {
	// Bank link: transactions is an optional product → history window applies.
	if !productRequested("transactions", []string{"auth"}, []string{"liabilities", "transactions"}, nil) {
		t.Error("transactions in optionalProducts should be detected")
	}
	// Reconnect: transactions granted via additionalConsentedProducts.
	if !productRequested("transactions", nil, nil, []string{"transactions"}) {
		t.Error("transactions in additionalConsentedProducts should be detected")
	}
	// Investments link: no transactions → history window must not apply.
	if productRequested("transactions", []string{"investments"}, nil, nil) {
		t.Error("investments-only link should not request transactions history")
	}
}

// A Plaid error passed through wrapErr (or further wrapped) must still report
// its error_code, since callers branch on it (login required, keys rejected,
// item already gone).
func TestErrorCode_SurvivesWrapping(t *testing.T) {
	wrapped := fmt.Errorf("sync failed: %w", &plaidError{action: "accounts get", code: "INVALID_ACCESS_TOKEN", message: "could not find matching access token"})
	if got := ErrorCode(wrapped); got != "INVALID_ACCESS_TOKEN" {
		t.Errorf("ErrorCode = %q, want INVALID_ACCESS_TOKEN", got)
	}
	if got := ErrorCode(errors.New("connection refused")); got != "" {
		t.Errorf("ErrorCode of a non-Plaid error = %q, want empty", got)
	}
	if got := ErrorCode(nil); got != "" {
		t.Errorf("ErrorCode(nil) = %q, want empty", got)
	}
}

func TestPlaidError_MessageKeepsCodeAndDetail(t *testing.T) {
	msg := (&plaidError{action: "accounts get", code: "INVALID_ACCESS_TOKEN", message: "no match"}).Error()
	if msg != "plaid accounts get: INVALID_ACCESS_TOKEN: no match" {
		t.Errorf("message = %q", msg)
	}
}
