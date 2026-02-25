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

func TestValidateAppSpec_RequiresCollectionForCollectionBackedBlocks(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name      string
		block     BlockSpec
		wantError string
	}{
		{
			name:      "list",
			block:     BlockSpec{Type: "list"},
			wantError: `page "dashboard" block[2] list.collection is required`,
		},
		{
			name:      "form",
			block:     BlockSpec{Type: "form"},
			wantError: `page "dashboard" block[2] form.collection is required`,
		},
		{
			name:      "toggle",
			block:     BlockSpec{Type: "toggle", Field: "done"},
			wantError: `page "dashboard" block[2] toggle.collection is required`,
		},
		{
			name:      "stats",
			block:     BlockSpec{Type: "stats", Metric: "count"},
			wantError: `page "dashboard" block[2] stats.collection is required`,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			s := validTestSpec()
			s.Pages = []PageSpec{
				{
					ID:    "dashboard",
					Title: "Dashboard",
					Blocks: []BlockSpec{
						{Type: "nav", Items: []NavItemSpec{{Label: "Dashboard", PageID: "dashboard"}}},
						{Type: "list", Collection: "todos"},
						tc.block,
					},
				},
			}

			err := ValidateAppSpec(s)
			if err == nil {
				t.Fatal("expected error")
			}
			if !strings.Contains(err.Error(), tc.wantError) {
				t.Fatalf("unexpected error: %v", err)
			}
		})
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
