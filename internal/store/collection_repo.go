package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	specpkg "mini-atoms/internal/spec"

	"gorm.io/gorm"
)

type Collection struct {
	ID         int64
	ProjectID  int64
	Name       string
	SchemaJSON string
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

type CollectionRepo struct {
	db *gorm.DB
}

func NewCollectionRepo(db *gorm.DB) *CollectionRepo {
	return &CollectionRepo{db: db}
}

func (r *CollectionRepo) ListCollectionsByProject(ctx context.Context, projectID int64) ([]Collection, error) {
	var rows []CollectionModel
	if err := r.db.WithContext(ctx).
		Where("project_id = ?", projectID).
		Order("id ASC").
		Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("list collections by project: %w", err)
	}

	out := make([]Collection, 0, len(rows))
	for _, row := range rows {
		out = append(out, Collection{
			ID:         row.ID,
			ProjectID:  row.ProjectID,
			Name:       row.Name,
			SchemaJSON: row.SchemaJSON,
			CreatedAt:  row.CreatedAt,
			UpdatedAt:  row.UpdatedAt,
		})
	}
	return out, nil
}

func (r *CollectionRepo) SyncCollectionsFromSpec(ctx context.Context, projectID int64, appSpec specpkg.AppSpec) error {
	if projectID == 0 {
		return fmt.Errorf("sync collections: project id is required")
	}
	if err := specpkg.ValidateAppSpec(appSpec); err != nil {
		return fmt.Errorf("sync collections validate spec: %w", err)
	}

	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var existingRows []CollectionModel
		if err := tx.Where("project_id = ?", projectID).Find(&existingRows).Error; err != nil {
			return fmt.Errorf("load existing collections: %w", err)
		}

		existingByName := make(map[string]CollectionModel, len(existingRows))
		for _, row := range existingRows {
			existingByName[row.Name] = row
		}

		desiredByName := make(map[string]specpkg.CollectionSpec, len(appSpec.Collections))
		for _, c := range appSpec.Collections {
			desiredByName[c.Name] = c
		}

		for name := range existingByName {
			if _, ok := desiredByName[name]; !ok {
				return fmt.Errorf("incompatible schema change: deleting collection %q is not allowed", name)
			}
		}

		now := time.Now().UTC()
		for _, desired := range appSpec.Collections {
			schemaJSON, err := marshalCollectionSchema(desired)
			if err != nil {
				return err
			}

			existing, exists := existingByName[desired.Name]
			if !exists {
				row := CollectionModel{
					ProjectID:  projectID,
					Name:       desired.Name,
					SchemaJSON: schemaJSON,
					CreatedAt:  now,
					UpdatedAt:  now,
				}
				if err := tx.Create(&row).Error; err != nil {
					if isUniqueConstraintError(err) {
						return ErrConflict
					}
					return fmt.Errorf("create collection %q: %w", desired.Name, err)
				}
				continue
			}

			var oldSpec specpkg.CollectionSpec
			if err := json.Unmarshal([]byte(existing.SchemaJSON), &oldSpec); err != nil {
				return fmt.Errorf("parse existing collection %q schema_json: %w", existing.Name, err)
			}

			if err := ensureCompatibleCollectionChange(oldSpec, desired); err != nil {
				return err
			}

			if existing.SchemaJSON == schemaJSON {
				continue
			}
			if err := tx.Model(&CollectionModel{}).
				Where("id = ? AND project_id = ?", existing.ID, projectID).
				Updates(map[string]any{
					"schema_json": schemaJSON,
					"updated_at":  now,
				}).Error; err != nil {
				return fmt.Errorf("update collection %q schema: %w", desired.Name, err)
			}
		}

		return nil
	})
}

func marshalCollectionSchema(c specpkg.CollectionSpec) (string, error) {
	data, err := json.Marshal(c)
	if err != nil {
		return "", fmt.Errorf("marshal collection %q schema_json: %w", c.Name, err)
	}
	return string(data), nil
}

func ensureCompatibleCollectionChange(oldSpec, newSpec specpkg.CollectionSpec) error {
	if strings.TrimSpace(oldSpec.Name) != strings.TrimSpace(newSpec.Name) {
		return fmt.Errorf("incompatible schema change: renaming collection %q to %q is not allowed", oldSpec.Name, newSpec.Name)
	}

	oldFields := make(map[string]specpkg.FieldSpec, len(oldSpec.Fields))
	for _, f := range oldSpec.Fields {
		oldFields[f.Name] = f
	}
	newFields := make(map[string]specpkg.FieldSpec, len(newSpec.Fields))
	for _, f := range newSpec.Fields {
		newFields[f.Name] = f
	}

	for oldName, oldField := range oldFields {
		newField, ok := newFields[oldName]
		if !ok {
			return fmt.Errorf("incompatible schema change: deleting field %q from collection %q is not allowed", oldName, oldSpec.Name)
		}
		if oldField.Type != newField.Type {
			return fmt.Errorf("incompatible schema change: changing field type %q in collection %q from %s to %s is not allowed", oldName, oldSpec.Name, oldField.Type, newField.Type)
		}
	}
	return nil
}

func (r *CollectionRepo) GetCollectionByProjectAndName(ctx context.Context, projectID int64, name string) (Collection, error) {
	var row CollectionModel
	if err := r.db.WithContext(ctx).
		Where("project_id = ? AND name = ?", projectID, strings.TrimSpace(name)).
		First(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return Collection{}, ErrNotFound
		}
		return Collection{}, fmt.Errorf("get collection by project and name: %w", err)
	}
	return Collection{
		ID:         row.ID,
		ProjectID:  row.ProjectID,
		Name:       row.Name,
		SchemaJSON: row.SchemaJSON,
		CreatedAt:  row.CreatedAt,
		UpdatedAt:  row.UpdatedAt,
	}, nil
}
