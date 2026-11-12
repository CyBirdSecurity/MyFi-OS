package api

import (
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func TestTokenRevoked(t *testing.T) {
	cutoff := time.Date(2026, 10, 26, 12, 0, 0, 700_000_000, time.UTC) // 12:00:00.7
	at := func(h, m, s int) *jwt.NumericDate {
		return jwt.NewNumericDate(time.Date(2026, 10, 26, h, m, s, 0, time.UTC))
	}
	if tokenRevoked(at(11, 59, 59), nil) {
		t.Error("with no cutoff set, no token is revoked")
	}
	if !tokenRevoked(at(11, 59, 59), &cutoff) {
		t.Error("a token issued before the change must be revoked")
	}
	if tokenRevoked(at(12, 0, 0), &cutoff) {
		t.Error("a token issued in the same second as the change (the fresh one handed back) must stay valid")
	}
	if tokenRevoked(at(12, 0, 5), &cutoff) {
		t.Error("a token issued after the change must stay valid")
	}
	if !tokenRevoked(nil, &cutoff) {
		t.Error("a token with no issue time can't be shown to postdate the change")
	}
}
