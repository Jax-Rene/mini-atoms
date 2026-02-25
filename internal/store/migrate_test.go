package store

import (
	"context"
	"path/filepath"
	"testing"
)

func TestMigrateIsIdempotent(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "migrate.db")

	db, err := OpenSQLite(ctx, dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("gorm db(): %v", err)
	}
	defer sqlDB.Close()

	if err := Migrate(ctx, db); err != nil {
		t.Fatalf("migrate second run: %v", err)
	}

	if !db.Migrator().HasTable(&AppMetaModel{}) {
		t.Fatal("expected app_meta table")
	}
	if !db.Migrator().HasTable(&UserModel{}) {
		t.Fatal("expected users table")
	}
	if !db.Migrator().HasTable(&UserSessionModel{}) {
		t.Fatal("expected user_sessions table")
	}
	if !db.Migrator().HasTable(&ProjectModel{}) {
		t.Fatal("expected projects table")
	}
	if !db.Migrator().HasTable(&CollectionModel{}) {
		t.Fatal("expected collections table")
	}
	if !db.Migrator().HasTable(&RecordModel{}) {
		t.Fatal("expected records table")
	}
	if !db.Migrator().HasTable(&ChatMessageModel{}) {
		t.Fatal("expected chat_messages table")
	}
}
