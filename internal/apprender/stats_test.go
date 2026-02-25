package apprender

import (
	"testing"

	specpkg "mini-atoms/internal/spec"
)

func TestComputeStatValue_CountAndSum(t *testing.T) {
	t.Parallel()

	schema := specpkg.CollectionSpec{
		Name: "todos",
		Fields: []specpkg.FieldSpec{
			{Name: "title", Type: specpkg.FieldTypeText},
			{Name: "minutes", Type: specpkg.FieldTypeInt},
			{Name: "score", Type: specpkg.FieldTypeReal},
		},
	}
	records := []Record{
		{ID: 1, Data: map[string]any{"title": "a", "minutes": float64(25), "score": float64(1.5)}},
		{ID: 2, Data: map[string]any{"title": "b", "minutes": float64(30), "score": float64(2)}},
		{ID: 3, Data: map[string]any{"title": "c"}},
	}

	count, err := ComputeStatValue(specpkg.BlockSpec{Type: "stats", Collection: "todos", Metric: "count"}, schema, records)
	if err != nil {
		t.Fatalf("compute count: %v", err)
	}
	if count != "3" {
		t.Fatalf("count = %q, want 3", count)
	}

	sumInt, err := ComputeStatValue(specpkg.BlockSpec{Type: "stats", Collection: "todos", Metric: "sum", Field: "minutes"}, schema, records)
	if err != nil {
		t.Fatalf("compute int sum: %v", err)
	}
	if sumInt != "55" {
		t.Fatalf("sumInt = %q, want 55", sumInt)
	}

	sumReal, err := ComputeStatValue(specpkg.BlockSpec{Type: "stats", Collection: "todos", Metric: "sum", Field: "score"}, schema, records)
	if err != nil {
		t.Fatalf("compute real sum: %v", err)
	}
	if sumReal != "3.5" {
		t.Fatalf("sumReal = %q, want 3.5", sumReal)
	}
}
