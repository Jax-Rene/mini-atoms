package store

import (
	"context"
	"fmt"

	"gorm.io/gorm"
)

var clearAllDataTables = []string{
	"chat_messages",
	"records",
	"collections",
	"projects",
	"user_sessions",
	"users",
	"app_meta",
}

// ClearAllData deletes all application data while keeping the schema intact.
// It is intended for internal debug tooling only.
func ClearAllData(ctx context.Context, db *gorm.DB) error {
	if db == nil {
		return fmt.Errorf("clear all data: db is nil")
	}

	return db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for _, table := range clearAllDataTables {
			if err := tx.Exec("DELETE FROM " + table).Error; err != nil {
				return fmt.Errorf("clear all data table %s: %w", table, err)
			}
		}

		var sqliteSequenceExists int64
		if err := tx.Raw(
			"SELECT COUNT(1) FROM sqlite_master WHERE type = ? AND name = ?",
			"table",
			"sqlite_sequence",
		).Scan(&sqliteSequenceExists).Error; err != nil {
			return fmt.Errorf("clear all data check sqlite_sequence: %w", err)
		}
		if sqliteSequenceExists > 0 {
			if err := tx.Exec("DELETE FROM sqlite_sequence").Error; err != nil {
				return fmt.Errorf("clear all data sqlite_sequence: %w", err)
			}
		}

		return nil
	})
}
