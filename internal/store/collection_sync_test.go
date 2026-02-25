package store

import (
	"context"
	"strings"
	"testing"

	specpkg "mini-atoms/internal/spec"
)

func TestCollectionRepo_SyncCollectionsFromSpec_AllowsNewCollectionAndField(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db, cleanup := openTestDB(t)
	defer cleanup()

	authRepo := NewAuthRepo(db)
	user, err := authRepo.CreateUser(ctx, "sync@example.com", "hash")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	projectRepo := NewProjectRepo(db)
	project, err := projectRepo.CreateProject(ctx, user.ID, "Todo App", "做一个待办应用")
	if err != nil {
		t.Fatalf("create project: %v", err)
	}

	repo := NewCollectionRepo(db)
	spec1 := collectionSyncBaseSpec()
	if err := repo.SyncCollectionsFromSpec(ctx, project.ID, spec1); err != nil {
		t.Fatalf("sync spec1: %v", err)
	}

	spec2 := collectionSyncBaseSpec()
	spec2.Collections[0].Fields = append(spec2.Collections[0].Fields, specpkg.FieldSpec{Name: "priority", Type: specpkg.FieldTypeInt})
	spec2.Collections = append(spec2.Collections, specpkg.CollectionSpec{
		Name: "notes",
		Fields: []specpkg.FieldSpec{
			{Name: "content", Type: specpkg.FieldTypeText},
		},
	})
	if err := repo.SyncCollectionsFromSpec(ctx, project.ID, spec2); err != nil {
		t.Fatalf("sync spec2: %v", err)
	}

	collections, err := repo.ListCollectionsByProject(ctx, project.ID)
	if err != nil {
		t.Fatalf("list collections: %v", err)
	}
	if len(collections) != 2 {
		t.Fatalf("collections len = %d, want 2", len(collections))
	}
}

func TestCollectionRepo_SyncCollectionsFromSpec_RejectsDeleteField(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db, cleanup := openTestDB(t)
	defer cleanup()

	authRepo := NewAuthRepo(db)
	user, err := authRepo.CreateUser(ctx, "sync-delete-field@example.com", "hash")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	projectRepo := NewProjectRepo(db)
	project, err := projectRepo.CreateProject(ctx, user.ID, "Todo App", "做一个待办应用")
	if err != nil {
		t.Fatalf("create project: %v", err)
	}

	repo := NewCollectionRepo(db)
	spec1 := collectionSyncBaseSpec()
	if err := repo.SyncCollectionsFromSpec(ctx, project.ID, spec1); err != nil {
		t.Fatalf("sync spec1: %v", err)
	}

	spec2 := collectionSyncBaseSpec()
	spec2.Collections[0].Fields = []specpkg.FieldSpec{
		{Name: "done", Type: specpkg.FieldTypeBool},
	}
	err = repo.SyncCollectionsFromSpec(ctx, project.ID, spec2)
	if err == nil {
		t.Fatal("expected incompatible schema error")
	}
	if !strings.Contains(err.Error(), "deleting field") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCollectionRepo_SyncCollectionsFromSpec_RejectsTypeChange(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db, cleanup := openTestDB(t)
	defer cleanup()

	authRepo := NewAuthRepo(db)
	user, err := authRepo.CreateUser(ctx, "sync-type@example.com", "hash")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	projectRepo := NewProjectRepo(db)
	project, err := projectRepo.CreateProject(ctx, user.ID, "Todo App", "做一个待办应用")
	if err != nil {
		t.Fatalf("create project: %v", err)
	}

	repo := NewCollectionRepo(db)
	spec1 := collectionSyncBaseSpec()
	if err := repo.SyncCollectionsFromSpec(ctx, project.ID, spec1); err != nil {
		t.Fatalf("sync spec1: %v", err)
	}

	spec2 := collectionSyncBaseSpec()
	spec2.Collections[0].Fields[0].Type = specpkg.FieldTypeInt
	err = repo.SyncCollectionsFromSpec(ctx, project.ID, spec2)
	if err == nil {
		t.Fatal("expected incompatible schema error")
	}
	if !strings.Contains(err.Error(), "changing field type") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func collectionSyncBaseSpec() specpkg.AppSpec {
	return specpkg.AppSpec{
		AppName: "Todo App",
		Collections: []specpkg.CollectionSpec{
			{
				Name: "todos",
				Fields: []specpkg.FieldSpec{
					{Name: "title", Type: specpkg.FieldTypeText, Required: true},
					{Name: "done", Type: specpkg.FieldTypeBool, Required: true},
				},
			},
		},
		Pages: []specpkg.PageSpec{
			{
				ID: "home",
				Blocks: []specpkg.BlockSpec{
					{Type: "list", Collection: "todos"},
					{Type: "toggle", Collection: "todos", Field: "done"},
				},
			},
		},
	}
}
