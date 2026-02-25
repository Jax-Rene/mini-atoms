package spec

import (
	"fmt"
	"strings"
)

const (
	MaxCollectionsPerProject = 5
	MaxFieldsPerCollection   = 10
)

func ValidateAppSpec(s AppSpec) error {
	if len(s.Collections) == 0 {
		return fmt.Errorf("spec.collections is required")
	}
	if len(s.Collections) > MaxCollectionsPerProject {
		return fmt.Errorf("spec.collections exceeds limit: got %d, max %d", len(s.Collections), MaxCollectionsPerProject)
	}
	if len(s.Pages) == 0 {
		return fmt.Errorf("spec.pages is required")
	}

	collectionsByName := make(map[string]CollectionSpec, len(s.Collections))
	fieldsByCollection := make(map[string]map[string]FieldSpec, len(s.Collections))
	for _, c := range s.Collections {
		if strings.TrimSpace(c.Name) == "" {
			return fmt.Errorf("collection.name is required")
		}
		if _, exists := collectionsByName[c.Name]; exists {
			return fmt.Errorf("duplicate collection name %q", c.Name)
		}
		if len(c.Fields) == 0 {
			return fmt.Errorf("collection %q has no fields", c.Name)
		}
		if len(c.Fields) > MaxFieldsPerCollection {
			return fmt.Errorf("collection %q fields exceed limit: got %d, max %d", c.Name, len(c.Fields), MaxFieldsPerCollection)
		}

		fieldMap := make(map[string]FieldSpec, len(c.Fields))
		for _, f := range c.Fields {
			if strings.TrimSpace(f.Name) == "" {
				return fmt.Errorf("collection %q has empty field name", c.Name)
			}
			if _, dup := fieldMap[f.Name]; dup {
				return fmt.Errorf("collection %q has duplicate field %q", c.Name, f.Name)
			}
			if _, ok := AllowedFieldTypes[f.Type]; !ok {
				return fmt.Errorf("collection %q field %q has unsupported type %q", c.Name, f.Name, f.Type)
			}
			if f.Type == FieldTypeEnum {
				if len(f.Options) == 0 {
					return fmt.Errorf("collection %q field %q enum options is required", c.Name, f.Name)
				}
				for _, opt := range f.Options {
					if strings.TrimSpace(opt) == "" {
						return fmt.Errorf("collection %q field %q enum options must be non-empty", c.Name, f.Name)
					}
				}
			}
			fieldMap[f.Name] = f
		}
		collectionsByName[c.Name] = c
		fieldsByCollection[c.Name] = fieldMap
	}

	pageIDs := make(map[string]struct{}, len(s.Pages))
	for _, p := range s.Pages {
		if strings.TrimSpace(p.ID) == "" {
			return fmt.Errorf("page.id is required")
		}
		if _, dup := pageIDs[p.ID]; dup {
			return fmt.Errorf("duplicate page id %q", p.ID)
		}
		pageIDs[p.ID] = struct{}{}
	}

	for _, p := range s.Pages {
		for i, b := range p.Blocks {
			if _, ok := AllowedBlockTypes[b.Type]; !ok {
				return fmt.Errorf("page %q block[%d]: unsupported block type %q", p.ID, i, b.Type)
			}
			switch b.Type {
			case "nav":
				for j, item := range b.Items {
					if strings.TrimSpace(item.PageID) == "" {
						return fmt.Errorf("page %q block[%d] nav item[%d]: page_id is required", p.ID, i, j)
					}
					if _, ok := pageIDs[item.PageID]; !ok {
						return fmt.Errorf("page %q block[%d] nav item[%d]: unknown page_id %q", p.ID, i, j, item.PageID)
					}
				}
			case "list", "form":
				fieldMap, err := requireCollectionFields(p.ID, i, b, fieldsByCollection)
				if err != nil {
					return err
				}
				for _, fname := range b.Fields {
					if _, ok := fieldMap[fname]; !ok {
						return fmt.Errorf("page %q block[%d] %s: unknown field %q in collection %q", p.ID, i, b.Type, fname, b.Collection)
					}
				}
			case "toggle":
				fieldMap, err := requireCollectionFields(p.ID, i, b, fieldsByCollection)
				if err != nil {
					return err
				}
				if strings.TrimSpace(b.Field) == "" {
					return fmt.Errorf("page %q block[%d] toggle.field is required", p.ID, i)
				}
				f, ok := fieldMap[b.Field]
				if !ok {
					return fmt.Errorf("page %q block[%d] toggle.field %q not found in collection %q", p.ID, i, b.Field, b.Collection)
				}
				if f.Type != FieldTypeBool {
					return fmt.Errorf("page %q block[%d] toggle.field %q must reference bool field, got %s", p.ID, i, b.Field, f.Type)
				}
			case "stats":
				fieldMap, err := requireCollectionFields(p.ID, i, b, fieldsByCollection)
				if err != nil {
					return err
				}
				switch b.Metric {
				case "count":
					// no-op
				case "sum":
					if strings.TrimSpace(b.Field) == "" {
						return fmt.Errorf("page %q block[%d] stats metric=sum requires field", p.ID, i)
					}
					f, ok := fieldMap[b.Field]
					if !ok {
						return fmt.Errorf("page %q block[%d] stats field %q not found in collection %q", p.ID, i, b.Field, b.Collection)
					}
					if f.Type != FieldTypeInt && f.Type != FieldTypeReal {
						return fmt.Errorf("page %q block[%d] stats sum field %q must be numeric, got %s", p.ID, i, b.Field, f.Type)
					}
				default:
					return fmt.Errorf("page %q block[%d] stats.metric %q is invalid", p.ID, i, b.Metric)
				}
			case "timer":
				if strings.TrimSpace(b.SessionCollection) != "" {
					if _, ok := collectionsByName[b.SessionCollection]; !ok {
						return fmt.Errorf("page %q block[%d] timer.session_collection %q not found", p.ID, i, b.SessionCollection)
					}
				}
			}
		}
	}

	return nil
}

func requireCollectionFields(pageID string, blockIdx int, b BlockSpec, fieldsByCollection map[string]map[string]FieldSpec) (map[string]FieldSpec, error) {
	if strings.TrimSpace(b.Collection) == "" {
		return nil, fmt.Errorf("page %q block[%d] %s.collection is required", pageID, blockIdx, b.Type)
	}
	fieldMap, ok := fieldsByCollection[b.Collection]
	if !ok {
		return nil, fmt.Errorf("page %q block[%d] %s.collection %q not found", pageID, blockIdx, b.Type, b.Collection)
	}
	return fieldMap, nil
}
