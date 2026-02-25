package store

import (
	"context"
	"strings"
	"testing"

	specpkg "mini-atoms/internal/spec"
)

func TestRecordRepo_CreateRecordValidatesFieldTypes(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db, cleanup := openTestDB(t)
	defer cleanup()

	authRepo := NewAuthRepo(db)
	user, err := authRepo.CreateUser(ctx, "records-validate@example.com", "hash")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	projectRepo := NewProjectRepo(db)
	project, err := projectRepo.CreateProject(ctx, user.ID, "Book Tracker", "做一个读书记录")
	if err != nil {
		t.Fatalf("create project: %v", err)
	}

	specObj := specpkg.AppSpec{
		AppName: "Book Tracker",
		Collections: []specpkg.CollectionSpec{
			{
				Name: "books",
				Fields: []specpkg.FieldSpec{
					{Name: "title", Type: specpkg.FieldTypeText, Required: true},
					{Name: "pages", Type: specpkg.FieldTypeInt},
					{Name: "read", Type: specpkg.FieldTypeBool, Required: true},
					{Name: "status", Type: specpkg.FieldTypeEnum, Options: []string{"todo", "doing", "done"}},
				},
			},
		},
		Pages: []specpkg.PageSpec{{ID: "home", Blocks: []specpkg.BlockSpec{{Type: "form", Collection: "books"}}}},
	}

	collectionRepo := NewCollectionRepo(db)
	if err := collectionRepo.SyncCollectionsFromSpec(ctx, project.ID, specObj); err != nil {
		t.Fatalf("sync collections: %v", err)
	}
	collection, err := collectionRepo.GetCollectionByProjectAndName(ctx, project.ID, "books")
	if err != nil {
		t.Fatalf("get collection: %v", err)
	}

	recordRepo := NewRecordRepo(db)

	_, err = recordRepo.CreateRecord(ctx, project.ID, collection, map[string]string{
		"title":  "Clean Code",
		"pages":  "abc",
		"read":   "false",
		"status": "todo",
	})
	if err == nil {
		t.Fatal("expected int parse error")
	}
	if !strings.Contains(err.Error(), "pages") {
		t.Fatalf("unexpected error: %v", err)
	}

	_, err = recordRepo.CreateRecord(ctx, project.ID, collection, map[string]string{
		"title":  "Clean Code",
		"pages":  "464",
		"read":   "true",
		"status": "paused",
	})
	if err == nil {
		t.Fatal("expected enum validation error")
	}
	if !strings.Contains(err.Error(), "status") {
		t.Fatalf("unexpected error: %v", err)
	}

	created, err := recordRepo.CreateRecord(ctx, project.ID, collection, map[string]string{
		"title":  "Clean Architecture",
		"pages":  "432",
		"read":   "1",
		"status": "doing",
	})
	if err != nil {
		t.Fatalf("create record: %v", err)
	}
	if created.ID == 0 {
		t.Fatal("expected record id")
	}

	records, err := recordRepo.ListRecordsByCollection(ctx, project.ID, collection.ID)
	if err != nil {
		t.Fatalf("list records: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("records len = %d, want 1", len(records))
	}
	if got := records[0].Data["title"]; got != "Clean Architecture" {
		t.Fatalf("title = %#v", got)
	}
	if got := records[0].Data["read"]; got != true {
		t.Fatalf("read = %#v, want true", got)
	}
	if got := records[0].Data["status"]; got != "doing" {
		t.Fatalf("status = %#v, want doing", got)
	}
}

func TestRecordRepo_ToggleBoolField(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db, cleanup := openTestDB(t)
	defer cleanup()

	authRepo := NewAuthRepo(db)
	user, err := authRepo.CreateUser(ctx, "records-toggle@example.com", "hash")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	projectRepo := NewProjectRepo(db)
	project, err := projectRepo.CreateProject(ctx, user.ID, "Todo App", "做一个待办应用")
	if err != nil {
		t.Fatalf("create project: %v", err)
	}

	collectionRepo := NewCollectionRepo(db)
	if err := collectionRepo.SyncCollectionsFromSpec(ctx, project.ID, collectionSyncBaseSpec()); err != nil {
		t.Fatalf("sync collections: %v", err)
	}
	collection, err := collectionRepo.GetCollectionByProjectAndName(ctx, project.ID, "todos")
	if err != nil {
		t.Fatalf("get collection: %v", err)
	}

	recordRepo := NewRecordRepo(db)
	rec, err := recordRepo.CreateRecord(ctx, project.ID, collection, map[string]string{
		"title": "第一项",
		"done":  "false",
	})
	if err != nil {
		t.Fatalf("create record: %v", err)
	}

	toggled, err := recordRepo.ToggleBoolField(ctx, project.ID, collection, rec.ID, "done")
	if err != nil {
		t.Fatalf("toggle bool field: %v", err)
	}
	if got := toggled.Data["done"]; got != true {
		t.Fatalf("done after toggle = %#v, want true", got)
	}

	_, err = recordRepo.ToggleBoolField(ctx, project.ID, collection, rec.ID, "title")
	if err == nil {
		t.Fatal("expected non-bool toggle error")
	}
	if !strings.Contains(err.Error(), "bool") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRecordRepo_UpdateAndDeleteRecord(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db, cleanup := openTestDB(t)
	defer cleanup()

	authRepo := NewAuthRepo(db)
	user, err := authRepo.CreateUser(ctx, "records-update-delete@example.com", "hash")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	projectRepo := NewProjectRepo(db)
	project, err := projectRepo.CreateProject(ctx, user.ID, "Todo App", "做一个待办应用")
	if err != nil {
		t.Fatalf("create project: %v", err)
	}

	collectionRepo := NewCollectionRepo(db)
	if err := collectionRepo.SyncCollectionsFromSpec(ctx, project.ID, collectionSyncBaseSpec()); err != nil {
		t.Fatalf("sync collections: %v", err)
	}
	collection, err := collectionRepo.GetCollectionByProjectAndName(ctx, project.ID, "todos")
	if err != nil {
		t.Fatalf("get collection: %v", err)
	}

	recordRepo := NewRecordRepo(db)
	rec, err := recordRepo.CreateRecord(ctx, project.ID, collection, map[string]string{
		"title": "旧标题",
		"done":  "0",
	})
	if err != nil {
		t.Fatalf("create record: %v", err)
	}

	updated, err := recordRepo.UpdateRecord(ctx, project.ID, collection, rec.ID, map[string]string{
		"title": "新标题",
		"done":  "1",
	})
	if err != nil {
		t.Fatalf("update record: %v", err)
	}
	if got := updated.Data["title"]; got != "新标题" {
		t.Fatalf("updated title = %#v, want 新标题", got)
	}
	if got := updated.Data["done"]; got != true {
		t.Fatalf("updated done = %#v, want true", got)
	}

	if err := recordRepo.DeleteRecord(ctx, project.ID, collection.ID, rec.ID); err != nil {
		t.Fatalf("delete record: %v", err)
	}
	records, err := recordRepo.ListRecordsByCollection(ctx, project.ID, collection.ID)
	if err != nil {
		t.Fatalf("list records: %v", err)
	}
	if len(records) != 0 {
		t.Fatalf("records len = %d, want 0", len(records))
	}
}
