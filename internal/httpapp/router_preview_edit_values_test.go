package httpapp

import (
	"testing"

	specpkg "mini-atoms/internal/spec"
)

func TestBuildRecordEditValues_UsesLegacyFieldAliases(t *testing.T) {
	t.Parallel()

	schema := specpkg.CollectionSpec{
		Name: "todos",
		Fields: []specpkg.FieldSpec{
			{Name: "title", Type: specpkg.FieldTypeText},
			{Name: "completed", Type: specpkg.FieldTypeBool},
			{Name: "task", Type: specpkg.FieldTypeText},
			{Name: "done", Type: specpkg.FieldTypeBool},
		},
	}

	got := buildRecordEditValues(schema, map[string]any{
		"task": "历史任务标题",
		"done": true,
	})

	if got["title"] != "历史任务标题" {
		t.Fatalf("title edit value = %q, want 历史任务标题", got["title"])
	}
	if got["completed"] != "1" {
		t.Fatalf("completed edit value = %q, want 1", got["completed"])
	}
}
