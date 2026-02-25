package auth

import (
	"testing"
	"time"
)

func TestNewSessionToken(t *testing.T) {
	t.Parallel()

	token1, err := NewSessionToken()
	if err != nil {
		t.Fatalf("token1: %v", err)
	}
	token2, err := NewSessionToken()
	if err != nil {
		t.Fatalf("token2: %v", err)
	}

	if token1 == "" || token2 == "" {
		t.Fatal("expected non-empty tokens")
	}
	if token1 == token2 {
		t.Fatal("expected unique tokens")
	}
}

func TestSessionExpiryHelpers(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 2, 24, 12, 0, 0, 0, time.UTC)
	expiresAt := NewSessionExpiry(now)

	if expiresAt.Sub(now) != SessionTTL {
		t.Fatalf("expiry delta = %v, want %v", expiresAt.Sub(now), SessionTTL)
	}
	if IsSessionExpired(now, expiresAt) {
		t.Fatal("session should not be expired at creation time")
	}
	if !IsSessionExpired(expiresAt, expiresAt) {
		t.Fatal("session should be expired at exact expiry time")
	}
}
