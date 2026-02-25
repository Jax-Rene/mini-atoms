package spec

import (
	"strings"
	"testing"
)

func TestValidateAppSpec_ValidSpec(t *testing.T) {
	t.Parallel()

	if err := ValidateAppSpec(validTestSpec()); err != nil {
		t.Fatalf("ValidateAppSpec() error = %v", err)
	}
}

func TestValidateAppSpec_RejectsUnknownPrimitive(t *testing.T) {
	t.Parallel()

	s := validTestSpec()
	s.Pages[0].Blocks = append(s.Pages[0].Blocks, BlockSpec{
		Type:       "chart",
		Collection: "todos",
	})

	err := ValidateAppSpec(s)
	if err == nil {
		t.Fatal("expected error for unknown primitive")
	}
	if !strings.Contains(err.Error(), "unsupported block type") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateAppSpec_RejectsToggleNonBoolField(t *testing.T) {
	t.Parallel()

	s := validTestSpec()
	s.Pages[0].Blocks = []BlockSpec{
		{Type: "toggle", Collection: "todos", Field: "title"},
	}

	err := ValidateAppSpec(s)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "toggle.field") || !strings.Contains(err.Error(), "bool") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateAppSpec_RejectsStatsSumNonNumericField(t *testing.T) {
	t.Parallel()

	s := validTestSpec()
	s.Collections = []CollectionSpec{
		{
			Name: "todos",
			Fields: []FieldSpec{
				{Name: "title", Type: FieldTypeText},
				{Name: "done", Type: FieldTypeBool},
			},
		},
	}
	s.Pages[0].Blocks = []BlockSpec{
		{Type: "stats", Collection: "todos", Metric: "sum", Field: "title"},
	}

	err := ValidateAppSpec(s)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "stats") || !strings.Contains(err.Error(), "numeric") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func validTestSpec() AppSpec {
	return AppSpec{
		AppName: "Todo App",
		Theme:   "light",
		Collections: []CollectionSpec{
			{
				Name: "todos",
				Fields: []FieldSpec{
					{Name: "title", Type: FieldTypeText, Required: true},
					{Name: "done", Type: FieldTypeBool, Required: true},
					{Name: "minutes", Type: FieldTypeInt},
				},
			},
		},
		Pages: []PageSpec{
			{
				ID:    "home",
				Title: "Home",
				Blocks: []BlockSpec{
					{Type: "nav", Items: []NavItemSpec{{Label: "Home", PageID: "home"}}},
					{Type: "form", Collection: "todos"},
					{Type: "list", Collection: "todos"},
					{Type: "toggle", Collection: "todos", Field: "done"},
					{Type: "stats", Collection: "todos", Metric: "count"},
				},
			},
		},
	}
}
