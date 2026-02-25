package auth

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"time"
)

const SessionTTL = 7 * 24 * time.Hour

func NewSessionToken() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate session token: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

func NewSessionExpiry(now time.Time) time.Time {
	return now.UTC().Add(SessionTTL)
}

func IsSessionExpired(now, expiresAt time.Time) bool {
	return !now.UTC().Before(expiresAt.UTC())
}
