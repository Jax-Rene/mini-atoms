package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	specpkg "mini-atoms/internal/spec"

	"gorm.io/gorm"
)

type Record struct {
	ID           int64
	ProjectID    int64
	CollectionID int64
	Data         map[string]any
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

type RecordRepo struct {
	db *gorm.DB
}

func NewRecordRepo(db *gorm.DB) *RecordRepo {
	return &RecordRepo{db: db}
}

func (r *RecordRepo) ListRecordsByCollection(ctx context.Context, projectID, collectionID int64) ([]Record, error) {
	var rows []RecordModel
	if err := r.db.WithContext(ctx).
		Where("project_id = ? AND collection_id = ?", projectID, collectionID).
		Order("created_at ASC, id ASC").
		Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("list records by collection: %w", err)
	}

	out := make([]Record, 0, len(rows))
	for _, row := range rows {
		rec, err := toRecord(row)
		if err != nil {
			return nil, err
		}
		out = append(out, rec)
	}
	return out, nil
}

func (r *RecordRepo) CreateRecord(ctx context.Context, projectID int64, collection Collection, raw map[string]string) (Record, error) {
	if projectID == 0 {
		return Record{}, fmt.Errorf("create record: project id is required")
	}
	if collection.ID == 0 {
		return Record{}, fmt.Errorf("create record: collection id is required")
	}

	schema, err := parseCollectionSchema(collection.SchemaJSON)
	if err != nil {
		return Record{}, err
	}
	data, err := coerceRecordInput(schema, raw)
	if err != nil {
		return Record{}, err
	}

	dataJSONBytes, err := json.Marshal(data)
	if err != nil {
		return Record{}, fmt.Errorf("create record: marshal data_json: %w", err)
	}
	now := time.Now().UTC()
	row := RecordModel{
		ProjectID:    projectID,
		CollectionID: collection.ID,
		DataJSON:     string(dataJSONBytes),
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	if err := r.db.WithContext(ctx).Create(&row).Error; err != nil {
		return Record{}, fmt.Errorf("create record: %w", err)
	}
	return toRecord(row)
}

func (r *RecordRepo) GetRecordByID(ctx context.Context, projectID, collectionID, recordID int64) (Record, error) {
	var row RecordModel
	if err := r.db.WithContext(ctx).
		Where("id = ? AND project_id = ? AND collection_id = ?", recordID, projectID, collectionID).
		First(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return Record{}, ErrNotFound
		}
		return Record{}, fmt.Errorf("get record by id: %w", err)
	}
	return toRecord(row)
}

func (r *RecordRepo) UpdateRecord(ctx context.Context, projectID int64, collection Collection, recordID int64, raw map[string]string) (Record, error) {
	if recordID == 0 {
		return Record{}, fmt.Errorf("update record: record id is required")
	}
	if _, err := r.GetRecordByID(ctx, projectID, collection.ID, recordID); err != nil {
		return Record{}, err
	}

	schema, err := parseCollectionSchema(collection.SchemaJSON)
	if err != nil {
		return Record{}, err
	}
	data, err := coerceRecordInput(schema, raw)
	if err != nil {
		return Record{}, err
	}
	dataJSONBytes, err := json.Marshal(data)
	if err != nil {
		return Record{}, fmt.Errorf("update record: marshal data_json: %w", err)
	}
	now := time.Now().UTC()
	tx := r.db.WithContext(ctx).
		Model(&RecordModel{}).
		Where("id = ? AND project_id = ? AND collection_id = ?", recordID, projectID, collection.ID).
		Updates(map[string]any{
			"data_json":  string(dataJSONBytes),
			"updated_at": now,
		})
	if tx.Error != nil {
		return Record{}, fmt.Errorf("update record: %w", tx.Error)
	}
	if tx.RowsAffected == 0 {
		return Record{}, ErrNotFound
	}
	return r.GetRecordByID(ctx, projectID, collection.ID, recordID)
}

func (r *RecordRepo) DeleteRecord(ctx context.Context, projectID, collectionID, recordID int64) error {
	tx := r.db.WithContext(ctx).
		Where("id = ? AND project_id = ? AND collection_id = ?", recordID, projectID, collectionID).
		Delete(&RecordModel{})
	if tx.Error != nil {
		return fmt.Errorf("delete record: %w", tx.Error)
	}
	if tx.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *RecordRepo) ToggleBoolField(ctx context.Context, projectID int64, collection Collection, recordID int64, fieldName string) (Record, error) {
	if recordID == 0 {
		return Record{}, fmt.Errorf("toggle bool field: record id is required")
	}
	fieldName = strings.TrimSpace(fieldName)
	if fieldName == "" {
		return Record{}, fmt.Errorf("toggle bool field: field name is required")
	}

	schema, err := parseCollectionSchema(collection.SchemaJSON)
	if err != nil {
		return Record{}, err
	}
	fieldSpec, ok := findFieldSpec(schema, fieldName)
	if !ok {
		return Record{}, fmt.Errorf("toggle bool field: field %q not found in collection %q", fieldName, schema.Name)
	}
	if fieldSpec.Type != specpkg.FieldTypeBool {
		return Record{}, fmt.Errorf("toggle bool field: field %q must be bool, got %s", fieldName, fieldSpec.Type)
	}

	var row RecordModel
	if err := r.db.WithContext(ctx).
		Where("id = ? AND project_id = ? AND collection_id = ?", recordID, projectID, collection.ID).
		First(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return Record{}, ErrNotFound
		}
		return Record{}, fmt.Errorf("toggle bool field: load record: %w", err)
	}

	rec, err := toRecord(row)
	if err != nil {
		return Record{}, err
	}

	current, err := toBool(rec.Data[fieldName])
	if err != nil {
		return Record{}, fmt.Errorf("toggle bool field: decode field %q: %w", fieldName, err)
	}
	rec.Data[fieldName] = !current

	dataJSONBytes, err := json.Marshal(rec.Data)
	if err != nil {
		return Record{}, fmt.Errorf("toggle bool field: marshal data_json: %w", err)
	}
	now := time.Now().UTC()
	if err := r.db.WithContext(ctx).
		Model(&RecordModel{}).
		Where("id = ? AND project_id = ? AND collection_id = ?", recordID, projectID, collection.ID).
		Updates(map[string]any{
			"data_json":  string(dataJSONBytes),
			"updated_at": now,
		}).Error; err != nil {
		return Record{}, fmt.Errorf("toggle bool field: update record: %w", err)
	}

	rec.UpdatedAt = now
	return rec, nil
}

func parseCollectionSchema(schemaJSON string) (specpkg.CollectionSpec, error) {
	var schema specpkg.CollectionSpec
	if err := json.Unmarshal([]byte(schemaJSON), &schema); err != nil {
		return specpkg.CollectionSpec{}, fmt.Errorf("parse collection schema_json: %w", err)
	}
	return schema, nil
}

func coerceRecordInput(schema specpkg.CollectionSpec, raw map[string]string) (map[string]any, error) {
	out := make(map[string]any, len(schema.Fields))
	for _, f := range schema.Fields {
		rawVal, has := raw[f.Name]
		if !has {
			rawVal = ""
		}
		rawVal = strings.TrimSpace(rawVal)

		if rawVal == "" {
			if f.Required {
				return nil, fmt.Errorf("create record: field %q is required", f.Name)
			}
			continue
		}

		switch f.Type {
		case specpkg.FieldTypeText, specpkg.FieldTypeDate, specpkg.FieldTypeDatetime:
			out[f.Name] = rawVal
		case specpkg.FieldTypeInt:
			n, err := strconv.ParseInt(rawVal, 10, 64)
			if err != nil {
				return nil, fmt.Errorf("create record: field %q expects int: %w", f.Name, err)
			}
			out[f.Name] = n
		case specpkg.FieldTypeReal:
			n, err := strconv.ParseFloat(rawVal, 64)
			if err != nil {
				return nil, fmt.Errorf("create record: field %q expects real: %w", f.Name, err)
			}
			out[f.Name] = n
		case specpkg.FieldTypeBool:
			b, err := parseBoolInput(rawVal)
			if err != nil {
				return nil, fmt.Errorf("create record: field %q expects bool: %w", f.Name, err)
			}
			out[f.Name] = b
		case specpkg.FieldTypeEnum:
			if !containsString(f.Options, rawVal) {
				return nil, fmt.Errorf("create record: field %q enum option %q is invalid", f.Name, rawVal)
			}
			out[f.Name] = rawVal
		default:
			return nil, fmt.Errorf("create record: unsupported field type %q", f.Type)
		}
	}
	return out, nil
}

func parseBoolInput(v string) (bool, error) {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "1", "true", "on", "yes":
		return true, nil
	case "0", "false", "off", "no":
		return false, nil
	default:
		return false, fmt.Errorf("invalid bool value %q", v)
	}
}

func containsString(items []string, target string) bool {
	for _, item := range items {
		if item == target {
			return true
		}
	}
	return false
}

func findFieldSpec(schema specpkg.CollectionSpec, fieldName string) (specpkg.FieldSpec, bool) {
	for _, f := range schema.Fields {
		if f.Name == fieldName {
			return f, true
		}
	}
	return specpkg.FieldSpec{}, false
}

func toRecord(row RecordModel) (Record, error) {
	data := map[string]any{}
	if strings.TrimSpace(row.DataJSON) != "" {
		if err := json.Unmarshal([]byte(row.DataJSON), &data); err != nil {
			return Record{}, fmt.Errorf("parse record data_json: %w", err)
		}
	}
	return Record{
		ID:           row.ID,
		ProjectID:    row.ProjectID,
		CollectionID: row.CollectionID,
		Data:         data,
		CreatedAt:    row.CreatedAt,
		UpdatedAt:    row.UpdatedAt,
	}, nil
}

func toBool(v any) (bool, error) {
	switch x := v.(type) {
	case nil:
		return false, nil
	case bool:
		return x, nil
	case string:
		return parseBoolInput(x)
	case float64:
		return x != 0, nil
	default:
		return false, fmt.Errorf("unsupported bool value type %T", v)
	}
}
