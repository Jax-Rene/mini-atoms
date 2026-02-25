package auth

import "testing"

func TestNormalizeAndValidateEmail(t *testing.T) {
	t.Parallel()

	got, err := NormalizeAndValidateEmail("  Foo@Example.COM ")
	if err != nil {
		t.Fatalf("normalize email: %v", err)
	}
	if got != "foo@example.com" {
		t.Fatalf("email = %q, want %q", got, "foo@example.com")
	}
}

func TestNormalizeAndValidateEmail_Invalid(t *testing.T) {
	t.Parallel()

	if _, err := NormalizeAndValidateEmail("not-an-email"); err == nil {
		t.Fatal("expected invalid email error")
	}
}
