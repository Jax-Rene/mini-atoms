package store

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"gorm.io/gorm"
)

func TestAuthRepo_CreateUserAndSession(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db, cleanup := openTestDB(t)
	defer cleanup()

	repo := NewAuthRepo(db)

	user, err := repo.CreateUser(ctx, "foo@example.com", "hash-value")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	if user.ID == 0 {
		t.Fatal("expected user id")
	}

	_, err = repo.CreateUser(ctx, "foo@example.com", "hash-value")
	if err != ErrConflict {
		t.Fatalf("duplicate user error = %v, want %v", err, ErrConflict)
	}

	expiresAt := time.Now().UTC().Add(24 * time.Hour).Truncate(time.Second)
	if err := repo.CreateSession(ctx, user.ID, "token-1", expiresAt); err != nil {
		t.Fatalf("create session: %v", err)
	}

	us, err := repo.GetUserBySessionToken(ctx, "token-1")
	if err != nil {
		t.Fatalf("get session user: %v", err)
	}
	if us.User.Email != "foo@example.com" {
		t.Fatalf("user email = %q", us.User.Email)
	}
	if us.Session.Token != "token-1" {
		t.Fatalf("session token = %q", us.Session.Token)
	}
	if us.Session.ExpiresAt.Unix() != expiresAt.Unix() {
		t.Fatalf("expires_at unix = %d, want %d", us.Session.ExpiresAt.Unix(), expiresAt.Unix())
	}
}

func TestAuthRepo_DeleteSessionByToken(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db, cleanup := openTestDB(t)
	defer cleanup()

	repo := NewAuthRepo(db)
	user, err := repo.CreateUser(ctx, "bar@example.com", "hash-value")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	if err := repo.CreateSession(ctx, user.ID, "token-del", time.Now().UTC().Add(time.Hour)); err != nil {
		t.Fatalf("create session: %v", err)
	}
	if err := repo.DeleteSessionByToken(ctx, "token-del"); err != nil {
		t.Fatalf("delete session: %v", err)
	}

	_, err = repo.GetUserBySessionToken(ctx, "token-del")
	if err != ErrNotFound {
		t.Fatalf("get deleted session err = %v, want %v", err, ErrNotFound)
	}
}

func openTestDB(t *testing.T) (*gorm.DB, func()) {
	t.Helper()

	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "auth_repo.db")
	db, err := OpenSQLite(ctx, dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("gorm db(): %v", err)
	}
	return db, func() { _ = sqlDB.Close() }
}
