package auth

import (
	"fmt"
	"net/mail"
	"strings"
)

func NormalizeAndValidateEmail(raw string) (string, error) {
	email := strings.ToLower(strings.TrimSpace(raw))
	if email == "" {
		return "", fmt.Errorf("email is required")
	}

	addr, err := mail.ParseAddress(email)
	if err != nil {
		return "", fmt.Errorf("invalid email")
	}
	if !strings.EqualFold(addr.Address, email) {
		return "", fmt.Errorf("invalid email")
	}

	return email, nil
}
