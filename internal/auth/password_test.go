package auth

import "testing"

func TestHashAndCheckPassword(t *testing.T) {
	t.Parallel()

	hash, err := HashPassword("password123")
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}

	if !CheckPassword(hash, "password123") {
		t.Fatal("expected password to match hash")
	}
	if CheckPassword(hash, "wrong-password") {
		t.Fatal("expected wrong password to fail")
	}
}

func TestValidatePassword_MinLength(t *testing.T) {
	t.Parallel()

	if err := ValidatePassword("1234567"); err == nil {
		t.Fatal("expected short password validation error")
	}
	if err := ValidatePassword("12345678"); err != nil {
		t.Fatalf("unexpected validation error: %v", err)
	}
}
