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

func TestRenderApp_ListEditUsesFormPageWhenFormOnAnotherPage(t *testing.T) {
	t.Parallel()

	appSpec := specpkg.AppSpec{
		AppName: "Todo",
		Collections: []specpkg.CollectionSpec{
			{
				Name: "todos",
				Fields: []specpkg.FieldSpec{
					{Name: "title", Type: specpkg.FieldTypeText},
					{Name: "done", Type: specpkg.FieldTypeBool},
				},
			},
		},
		Pages: []specpkg.PageSpec{
			{
				ID: "dashboard",
				Blocks: []specpkg.BlockSpec{
					{Type: "list", Collection: "todos"},
				},
			},
			{
				ID: "create_task",
				Blocks: []specpkg.BlockSpec{
					{Type: "form", Collection: "todos"},
				},
			},
		},
	}

	view, err := RenderApp(RenderInput{
		Spec:           appSpec,
		Mode:           ModeEditor,
		SelectedPageID: "dashboard",
		Collections: map[string]CollectionData{
			"todos": {
				Schema: appSpec.Collections[0],
				Records: []Record{
					{ID: 1, Data: map[string]any{"title": "task 1", "done": false}},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("render app: %v", err)
	}

	if len(view.CurrentPage.Blocks) != 1 || view.CurrentPage.Blocks[0].List == nil {
		t.Fatalf("list block missing: %#v", view.CurrentPage.Blocks)
	}
	if got := view.CurrentPage.Blocks[0].List.EditPageID; got != "create_task" {
		t.Fatalf("list edit page = %q, want %q", got, "create_task")
	}
}

func TestRenderApp_ListBoolCellExposesInlineToggleState(t *testing.T) {
	t.Parallel()

	appSpec := specpkg.AppSpec{
		AppName: "Todo",
		Collections: []specpkg.CollectionSpec{
			{
				Name: "todos",
				Fields: []specpkg.FieldSpec{
					{Name: "title", Type: specpkg.FieldTypeText},
					{Name: "done", Type: specpkg.FieldTypeBool},
				},
			},
		},
		Pages: []specpkg.PageSpec{
			{
				ID: "home",
				Blocks: []specpkg.BlockSpec{
					{Type: "list", Collection: "todos"},
				},
			},
		},
	}

	view, err := RenderApp(RenderInput{
		Spec: appSpec,
		Mode: ModeEditor,
		Collections: map[string]CollectionData{
			"todos": {
				Schema: appSpec.Collections[0],
				Records: []Record{
					{ID: 1, Data: map[string]any{"title": "task 1", "done": true}},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("render app: %v", err)
	}

	list := view.CurrentPage.Blocks[0].List
	if list == nil || len(list.Rows) != 1 || len(list.Rows[0].Cells) < 2 {
		t.Fatalf("list rows invalid: %#v", list)
	}

	var doneCell *ListCellView
	for i := range list.Rows[0].Cells {
		if list.Rows[0].Cells[i].FieldName == "done" {
			doneCell = &list.Rows[0].Cells[i]
			break
		}
	}
	if doneCell == nil {
		t.Fatal("done cell not found")
	}
	if !doneCell.BoolToggleable {
		t.Fatal("expected bool cell to be toggleable inline")
	}
	if !doneCell.BoolValue {
		t.Fatal("expected bool cell current value=true")
	}
}

func TestRenderApp_GroupsConsecutiveStatsBlocks(t *testing.T) {
	t.Parallel()

	appSpec := specpkg.AppSpec{
		AppName: "Dashboard",
		Collections: []specpkg.CollectionSpec{
			{
				Name: "todos",
				Fields: []specpkg.FieldSpec{
					{Name: "title", Type: specpkg.FieldTypeText},
					{Name: "done", Type: specpkg.FieldTypeBool},
					{Name: "minutes", Type: specpkg.FieldTypeInt},
				},
			},
		},
		Pages: []specpkg.PageSpec{
			{
				ID: "dashboard",
				Blocks: []specpkg.BlockSpec{
					{Type: "stats", Collection: "todos", Metric: "count", Label: "任务数"},
					{Type: "stats", Collection: "todos", Metric: "sum", Field: "minutes", Label: "总分钟数"},
					{Type: "list", Collection: "todos"},
				},
			},
		},
	}

	view, err := RenderApp(RenderInput{
		Spec: appSpec,
		Mode: ModeEditor,
		Collections: map[string]CollectionData{
			"todos": {
				Schema: appSpec.Collections[0],
				Records: []Record{
					{ID: 1, Data: map[string]any{"title": "task 1", "done": false, "minutes": float64(25)}},
					{ID: 2, Data: map[string]any{"title": "task 2", "done": true, "minutes": float64(30)}},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("render app: %v", err)
	}

	if got := len(view.CurrentPage.Blocks); got != 2 {
		t.Fatalf("blocks len = %d, want 2 (stats group + list)", got)
	}
	stats := view.CurrentPage.Blocks[0]
	if stats.Type != "stats" {
		t.Fatalf("block[0] type = %q, want stats", stats.Type)
	}
	if got := len(stats.StatsCards); got != 2 {
		t.Fatalf("stats cards len = %d, want 2", got)
	}
	if stats.StatsCards[0].Label != "任务数" || stats.StatsCards[0].Value != "2" {
		t.Fatalf("stats card[0] = %#v", stats.StatsCards[0])
	}
	if stats.StatsCards[1].Label != "总分钟数" || stats.StatsCards[1].Value != "55" {
		t.Fatalf("stats card[1] = %#v", stats.StatsCards[1])
	}
	if stats.Stats == nil {
		t.Fatal("expected legacy single stats pointer to be populated for compatibility")
	}
	if view.CurrentPage.Blocks[1].List == nil {
		t.Fatalf("block[1] expected list, got %#v", view.CurrentPage.Blocks[1])
	}
}

func TestRenderApp_StatsBlocksSeparatedByOtherBlocksAreNotMerged(t *testing.T) {
	t.Parallel()

	appSpec := specpkg.AppSpec{
		AppName: "Dashboard",
		Collections: []specpkg.CollectionSpec{
			{
				Name: "todos",
				Fields: []specpkg.FieldSpec{
					{Name: "title", Type: specpkg.FieldTypeText},
					{Name: "done", Type: specpkg.FieldTypeBool},
				},
			},
		},
		Pages: []specpkg.PageSpec{
			{
				ID: "dashboard",
				Blocks: []specpkg.BlockSpec{
					{Type: "stats", Collection: "todos", Metric: "count", Label: "总数"},
					{Type: "toggle", Collection: "todos", Field: "done"},
					{Type: "stats", Collection: "todos", Metric: "count", Label: "总数2"},
				},
			},
		},
	}

	view, err := RenderApp(RenderInput{
		Spec: appSpec,
		Mode: ModeEditor,
		Collections: map[string]CollectionData{
			"todos": {
				Schema: appSpec.Collections[0],
				Records: []Record{
					{ID: 1, Data: map[string]any{"title": "task 1", "done": false}},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("render app: %v", err)
	}

	if got := len(view.CurrentPage.Blocks); got != 3 {
		t.Fatalf("blocks len = %d, want 3", got)
	}
	if got := len(view.CurrentPage.Blocks[0].StatsCards); got != 1 {
		t.Fatalf("first stats cards len = %d, want 1", got)
	}
	if view.CurrentPage.Blocks[1].Toggle == nil {
		t.Fatalf("middle block expected toggle, got %#v", view.CurrentPage.Blocks[1])
	}
	if got := len(view.CurrentPage.Blocks[2].StatsCards); got != 1 {
		t.Fatalf("last stats cards len = %d, want 1", got)
	}
}

func TestRenderApp_DefaultStatsLabelsUseChineseCopy(t *testing.T) {
	t.Parallel()

	appSpec := specpkg.AppSpec{
		AppName: "Dashboard",
		Collections: []specpkg.CollectionSpec{
			{
				Name: "todos",
				Fields: []specpkg.FieldSpec{
					{Name: "title", Type: specpkg.FieldTypeText},
					{Name: "minutes", Type: specpkg.FieldTypeInt},
				},
			},
		},
		Pages: []specpkg.PageSpec{
			{
				ID: "dashboard",
				Blocks: []specpkg.BlockSpec{
					{Type: "stats", Collection: "todos", Metric: "count"},
					{Type: "stats", Collection: "todos", Metric: "sum", Field: "minutes"},
				},
			},
		},
	}

	view, err := RenderApp(RenderInput{
		Spec: appSpec,
		Mode: ModeEditor,
		Collections: map[string]CollectionData{
			"todos": {
				Schema: appSpec.Collections[0],
				Records: []Record{
					{ID: 1, Data: map[string]any{"title": "task 1", "minutes": float64(25)}},
					{ID: 2, Data: map[string]any{"title": "task 2", "minutes": float64(30)}},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("render app: %v", err)
	}

	if got := len(view.CurrentPage.Blocks); got != 1 {
		t.Fatalf("blocks len = %d, want 1 grouped stats block", got)
	}
	if got := len(view.CurrentPage.Blocks[0].StatsCards); got != 2 {
		t.Fatalf("stats cards len = %d, want 2", got)
	}
	if got := view.CurrentPage.Blocks[0].StatsCards[0].Label; got != "记录总数" {
		t.Fatalf("count label = %q, want %q", got, "记录总数")
	}
	if got := view.CurrentPage.Blocks[0].StatsCards[1].Label; got != "合计" {
		t.Fatalf("sum label = %q, want %q", got, "合计")
	}
}
