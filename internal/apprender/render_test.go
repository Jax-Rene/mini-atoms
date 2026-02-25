package apprender

import (
	"testing"

	specpkg "mini-atoms/internal/spec"
)

func TestRenderApp_DefaultFieldOrderAndFormatting(t *testing.T) {
	t.Parallel()

	appSpec := specpkg.AppSpec{
		AppName: "Books",
		Collections: []specpkg.CollectionSpec{
			{
				Name: "books",
				Fields: []specpkg.FieldSpec{
					{Name: "done", Type: specpkg.FieldTypeBool},
					{Name: "pages", Type: specpkg.FieldTypeInt},
					{Name: "title", Type: specpkg.FieldTypeText},
					{Name: "published_at", Type: specpkg.FieldTypeDatetime},
				},
			},
		},
		Pages: []specpkg.PageSpec{
			{
				ID: "home",
				Blocks: []specpkg.BlockSpec{
					{Type: "form", Collection: "books"},
					{Type: "list", Collection: "books"},
				},
			},
		},
	}

	view, err := RenderApp(RenderInput{
		Spec: appSpec,
		Mode: ModeEditor,
		Collections: map[string]CollectionData{
			"books": {
				Schema: appSpec.Collections[0],
				Records: []Record{
					{
						ID: 1,
						Data: map[string]any{
							"title":        "Designing Data-Intensive Applications",
							"pages":        float64(616),
							"done":         true,
							"published_at": "2026-02-24T18:45:12Z",
						},
					},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("render app: %v", err)
	}

	if len(view.CurrentPage.Blocks) != 2 {
		t.Fatalf("blocks len = %d, want 2", len(view.CurrentPage.Blocks))
	}
	form := view.CurrentPage.Blocks[0].Form
	if form == nil || len(form.Fields) < 4 {
		t.Fatalf("form fields invalid: %#v", form)
	}
	if form.Fields[0].Name != "title" {
		t.Fatalf("first form field = %q, want title", form.Fields[0].Name)
	}
	if form.Fields[len(form.Fields)-1].Name != "done" {
		t.Fatalf("last form field = %q, want done", form.Fields[len(form.Fields)-1].Name)
	}

	list := view.CurrentPage.Blocks[1].List
	if list == nil || len(list.Rows) != 1 {
		t.Fatalf("list rows invalid: %#v", list)
	}
	var foundDate bool
	for _, cell := range list.Rows[0].Cells {
		if cell.FieldName == "published_at" {
			foundDate = true
			if cell.ValueText == "2026-02-24T18:45:12Z" || cell.ValueText == "" {
				t.Fatalf("datetime formatting not applied, got %q", cell.ValueText)
			}
		}
	}
	if !foundDate {
		t.Fatal("published_at cell not found")
	}
}

func TestRenderApp_TimerBlockInfersSessionFields(t *testing.T) {
	t.Parallel()

	appSpec := specpkg.AppSpec{
		AppName: "Focus",
		Collections: []specpkg.CollectionSpec{
			{
				Name: "focus_sessions",
				Fields: []specpkg.FieldSpec{
					{Name: "task", Type: specpkg.FieldTypeText},
					{Name: "minutes", Type: specpkg.FieldTypeInt},
					{Name: "completed_at", Type: specpkg.FieldTypeDatetime},
				},
			},
		},
		Pages: []specpkg.PageSpec{
			{
				ID: "home",
				Blocks: []specpkg.BlockSpec{
					{Type: "timer", SessionCollection: "focus_sessions", WorkMinutes: 25, BreakMinutes: 5},
				},
			},
		},
	}

	view, err := RenderApp(RenderInput{
		Spec: appSpec,
		Mode: ModeEditor,
		Collections: map[string]CollectionData{
			"focus_sessions": {Schema: appSpec.Collections[0], Records: nil},
		},
	})
	if err != nil {
		t.Fatalf("render app: %v", err)
	}
	if len(view.CurrentPage.Blocks) != 1 || view.CurrentPage.Blocks[0].Timer == nil {
		t.Fatalf("timer block missing: %#v", view.CurrentPage.Blocks)
	}
	timer := view.CurrentPage.Blocks[0].Timer
	if !timer.CanSaveSession {
		t.Fatal("expected timer save enabled")
	}
	if timer.TaskField != "task" || timer.MinutesField != "minutes" || timer.CompletedAtField != "completed_at" {
		t.Fatalf("timer inferred fields = %#v", timer)
	}
}
