package store

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

func OpenSQLite(ctx context.Context, path string) (*gorm.DB, error) {
	if err := ensureDatabaseDir(path); err != nil {
		return nil, err
	}

	cfg := &gorm.Config{
		DisableForeignKeyConstraintWhenMigrating: false,
		TranslateError:                           false,
		Logger:                                   gormlogger.Default.LogMode(gormlogger.Silent),
	}

	db, err := gorm.Open(sqlite.Open(path), cfg)
	if err != nil {
		return nil, fmt.Errorf("gorm open sqlite: %w", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("get sql db from gorm: %w", err)
	}

	sqlDB.SetMaxOpenConns(1)
	sqlDB.SetMaxIdleConns(1)
	sqlDB.SetConnMaxLifetime(0)
	sqlDB.SetConnMaxIdleTime(0)

	if _, err := sqlDB.ExecContext(ctx, "PRAGMA foreign_keys = ON;"); err != nil {
		_ = sqlDB.Close()
		return nil, fmt.Errorf("enable foreign keys: %w", err)
	}
	if _, err := sqlDB.ExecContext(ctx, "PRAGMA journal_mode = WAL;"); err != nil {
		_ = sqlDB.Close()
		return nil, fmt.Errorf("set wal mode: %w", err)
	}
	if err := sqlDB.PingContext(ctx); err != nil {
		_ = sqlDB.Close()
		return nil, fmt.Errorf("ping sqlite: %w", err)
	}

	migrateCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := Migrate(migrateCtx, db); err != nil {
		_ = sqlDB.Close()
		return nil, fmt.Errorf("migrate sqlite: %w", err)
	}

	return db, nil
}

func ensureDatabaseDir(path string) error {
	if path == "" || path == ":memory:" || strings.HasPrefix(path, "file:") {
		return nil
	}

	dir := filepath.Dir(path)
	if dir == "." || dir == "" {
		return nil
	}

	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("mkdir db dir %q: %w", dir, err)
	}

	return nil
}
