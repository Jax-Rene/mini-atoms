package store

import (
	"context"
	"fmt"

	"gorm.io/gorm"
)

func Migrate(ctx context.Context, db *gorm.DB) error {
	if err := db.WithContext(ctx).AutoMigrate(
		&AppMetaModel{},
		&UserModel{},
		&UserSessionModel{},
		&ProjectModel{},
		&CollectionModel{},
		&RecordModel{},
		&ChatMessageModel{},
	); err != nil {
		return fmt.Errorf("auto migrate: %w", err)
	}
	return nil
}
