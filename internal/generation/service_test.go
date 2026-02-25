package generation

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"testing"

	"mini-atoms/internal/spec"
	"mini-atoms/internal/store"

	"gorm.io/gorm"
)

func TestServiceGenerateDraft_SuccessWritesUserAssistantAndDraft(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db, cleanup := openGenerationTestDB(t)
	defer cleanup()

	project := seedProject(t, ctx, db)
	svc := NewService(ServiceDeps{
		Projects: store.NewProjectRepo(db),
		Chats:    store.NewChatRepo(db),
		Client: &fakeClient{
			responses: []fakeClientResponse{{content: mustSpecJSON(t, validTodoSpec())}},
		},
	})

	result, err := svc.GenerateDraft(ctx, GenerateDraftInput{
		UserID:     project.UserID,
		ProjectID:  project.ID,
		UserPrompt: "帮我生成一个待办应用，包含完成状态和数量统计",
	})
	if err != nil {
		t.Fatalf("GenerateDraft() error = %v", err)
	}
	if result.RoundNo != 1 {
		t.Fatalf("round no = %d, want 1", result.RoundNo)
	}
	if result.DraftSpecJSON == "" {
		t.Fatal("expected draft spec json")
	}

	projectRepo := store.NewProjectRepo(db)
	gotProject, err := projectRepo.GetProjectByUserAndID(ctx, project.UserID, project.ID)
	if err != nil {
		t.Fatalf("GetProjectByUserAndID() error = %v", err)
	}
	if gotProject.DraftSpecJSON == "" {
		t.Fatal("project draft spec not saved")
	}

	chatRepo := store.NewChatRepo(db)
	msgs, err := chatRepo.ListMessagesByProject(ctx, project.ID)
	if err != nil {
		t.Fatalf("ListMessagesByProject() error = %v", err)
	}
	if len(msgs) != 2 {
		t.Fatalf("messages len = %d, want 2", len(msgs))
	}
	if msgs[0].Role != ChatRoleUser {
		t.Fatalf("msg[0].Role = %q, want %q", msgs[0].Role, ChatRoleUser)
	}
	if msgs[1].Role != ChatRoleAssistant {
		t.Fatalf("msg[1].Role = %q, want %q", msgs[1].Role, ChatRoleAssistant)
	}
	if msgs[0].RoundNo != 1 || msgs[1].RoundNo != 1 {
		t.Fatalf("round numbers = %d,%d, want 1,1", msgs[0].RoundNo, msgs[1].RoundNo)
	}
}

func TestServiceGenerateDraft_RepairsOnInvalidJSON(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db, cleanup := openGenerationTestDB(t)
	defer cleanup()
	project := seedProject(t, ctx, db)

	fc := &fakeClient{
		responses: []fakeClientResponse{
			{content: "{invalid-json"},
			{content: mustSpecJSON(t, validTodoSpec())},
		},
	}
	svc := NewService(ServiceDeps{
		Projects: store.NewProjectRepo(db),
		Chats:    store.NewChatRepo(db),
		Client:   fc,
	})

	_, err := svc.GenerateDraft(ctx, GenerateDraftInput{
		UserID:     project.UserID,
		ProjectID:  project.ID,
		UserPrompt: "做一个待办应用",
	})
	if err != nil {
		t.Fatalf("GenerateDraft() error = %v", err)
	}
	if fc.callCount != 2 {
		t.Fatalf("client callCount = %d, want 2", fc.callCount)
	}
	if fc.requests[1].RepairError == "" {
		t.Fatal("expected repair error on second request")
	}
}

func TestServiceGenerateDraft_RepairsOnValidationError(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db, cleanup := openGenerationTestDB(t)
	defer cleanup()
	project := seedProject(t, ctx, db)

	invalid := validTodoSpec()
	invalid.Pages[0].Blocks = append(invalid.Pages[0].Blocks, spec.BlockSpec{
		Type:       "toggle",
		Collection: "todos",
		Field:      "title",
	})

	fc := &fakeClient{
		responses: []fakeClientResponse{
			{content: mustSpecJSON(t, invalid)},
			{content: mustSpecJSON(t, validTodoSpec())},
		},
	}
	svc := NewService(ServiceDeps{
		Projects: store.NewProjectRepo(db),
		Chats:    store.NewChatRepo(db),
		Client:   fc,
	})

	_, err := svc.GenerateDraft(ctx, GenerateDraftInput{
		UserID:     project.UserID,
		ProjectID:  project.ID,
		UserPrompt: "做一个待办应用",
	})
	if err != nil {
		t.Fatalf("GenerateDraft() error = %v", err)
	}
	if fc.callCount != 2 {
		t.Fatalf("client callCount = %d, want 2", fc.callCount)
	}
	if fc.requests[1].RepairError == "" {
		t.Fatal("expected repair error on second request")
	}
}

func TestServiceGenerateDraft_FailureDoesNotOverwriteOldDraftAndWritesSystem(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db, cleanup := openGenerationTestDB(t)
	defer cleanup()
	project := seedProject(t, ctx, db)

	projectRepo := store.NewProjectRepo(db)
	oldDraft := `{"app_name":"Existing Draft"}`
	if err := projectRepo.UpdateProjectSpecsByID(ctx, project.UserID, project.ID, oldDraft, ""); err != nil {
		t.Fatalf("seed draft: %v", err)
	}

	fc := &fakeClient{
		responses: []fakeClientResponse{
			{content: "{oops"},
			{content: "{still-bad"},
		},
	}
	svc := NewService(ServiceDeps{
		Projects: projectRepo,
		Chats:    store.NewChatRepo(db),
		Client:   fc,
	})

	_, err := svc.GenerateDraft(ctx, GenerateDraftInput{
		UserID:     project.UserID,
		ProjectID:  project.ID,
		UserPrompt: "继续优化",
	})
	if err == nil {
		t.Fatal("expected error")
	}

	gotProject, err := projectRepo.GetProjectByUserAndID(ctx, project.UserID, project.ID)
	if err != nil {
		t.Fatalf("GetProjectByUserAndID() error = %v", err)
	}
	if gotProject.DraftSpecJSON != oldDraft {
		t.Fatalf("draft overwritten: got %q want %q", gotProject.DraftSpecJSON, oldDraft)
	}

	msgs, err := store.NewChatRepo(db).ListMessagesByProject(ctx, project.ID)
	if err != nil {
		t.Fatalf("ListMessagesByProject() error = %v", err)
	}
	if len(msgs) != 2 {
		t.Fatalf("messages len = %d, want 2", len(msgs))
	}
	if msgs[0].Role != ChatRoleUser || msgs[1].Role != ChatRoleSystem {
		t.Fatalf("roles = %q,%q, want %q,%q", msgs[0].Role, msgs[1].Role, ChatRoleUser, ChatRoleSystem)
	}
}

func TestServiceGenerateDraft_PreservesExistingCollectionsAndFieldsWhenLLMOmitsThem(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db, cleanup := openGenerationTestDB(t)
	defer cleanup()
	project := seedProject(t, ctx, db)

	projectRepo := store.NewProjectRepo(db)
	existing := spec.AppSpec{
		AppName: "Task Tracker",
		Collections: []spec.CollectionSpec{
			{
				Name: "tasks",
				Fields: []spec.FieldSpec{
					{Name: "title", Type: spec.FieldTypeText, Required: true},
					{Name: "description", Type: spec.FieldTypeText},
					{Name: "done", Type: spec.FieldTypeBool, Required: true},
				},
			},
			{
				Name: "tags",
				Fields: []spec.FieldSpec{
					{Name: "name", Type: spec.FieldTypeText, Required: true},
				},
			},
		},
		Pages: []spec.PageSpec{
			{
				ID: "home",
				Blocks: []spec.BlockSpec{
					{Type: "form", Collection: "tasks"},
					{Type: "list", Collection: "tasks"},
					{Type: "toggle", Collection: "tasks", Field: "done"},
				},
			},
		},
	}
	if err := projectRepo.UpdateProjectSpecsByID(ctx, project.UserID, project.ID, mustSpecJSON(t, existing), ""); err != nil {
		t.Fatalf("seed existing draft: %v", err)
	}

	// 模拟二次对话时模型返回了一个合法 spec，但遗漏了已有字段 description 和整个 tags 集合。
	llmOutput := spec.AppSpec{
		AppName: "Task Tracker",
		Collections: []spec.CollectionSpec{
			{
				Name: "tasks",
				Fields: []spec.FieldSpec{
					{Name: "title", Type: spec.FieldTypeText, Required: true},
					{Name: "done", Type: spec.FieldTypeBool, Required: true},
					{Name: "priority", Type: spec.FieldTypeInt},
				},
			},
		},
		Pages: []spec.PageSpec{
			{
				ID: "home",
				Blocks: []spec.BlockSpec{
					{Type: "form", Collection: "tasks"},
					{Type: "list", Collection: "tasks"},
					{Type: "toggle", Collection: "tasks", Field: "done"},
					{Type: "stats", Collection: "tasks", Metric: "count"},
				},
			},
		},
	}

	svc := NewService(ServiceDeps{
		Projects: projectRepo,
		Chats:    store.NewChatRepo(db),
		Client: &fakeClient{
			responses: []fakeClientResponse{{content: mustSpecJSON(t, llmOutput)}},
		},
	})

	result, err := svc.GenerateDraft(ctx, GenerateDraftInput{
		UserID:     project.UserID,
		ProjectID:  project.ID,
		UserPrompt: "增加任务优先级和统计卡片",
	})
	if err != nil {
		t.Fatalf("GenerateDraft() error = %v", err)
	}

	var got spec.AppSpec
	if err := json.Unmarshal([]byte(result.DraftSpecJSON), &got); err != nil {
		t.Fatalf("json.Unmarshal() result error = %v", err)
	}

	tasks := mustFindCollection(t, got, "tasks")
	if !hasField(tasks, "description") {
		t.Fatalf("expected tasks.description to be preserved, got fields=%v", fieldNames(tasks))
	}
	if !hasField(tasks, "priority") {
		t.Fatalf("expected tasks.priority to be present, got fields=%v", fieldNames(tasks))
	}
	if _, ok := findCollection(got, "tags"); !ok {
		t.Fatalf("expected omitted collection tags to be preserved")
	}
}

func TestRepairCommonLLMSpecIssues_FillsMissingCollectionForSingleCollectionBlocks(t *testing.T) {
	t.Parallel()

	in := spec.AppSpec{
		AppName: "Todo App",
		Collections: []spec.CollectionSpec{
			{
				Name: "todos",
				Fields: []spec.FieldSpec{
					{Name: "title", Type: spec.FieldTypeText},
					{Name: "done", Type: spec.FieldTypeBool},
				},
			},
		},
		Pages: []spec.PageSpec{
			{
				ID: "dashboard",
				Blocks: []spec.BlockSpec{
					{Type: "list"},
					{Type: "form"},
					{Type: "toggle", Field: "done"},
					{Type: "stats", Metric: "count"},
					{Type: "timer"},
				},
			},
		},
	}

	got := repairCommonLLMSpecIssues(in)

	if got.Pages[0].Blocks[0].Collection != "todos" {
		t.Fatalf("list.collection = %q, want %q", got.Pages[0].Blocks[0].Collection, "todos")
	}
	if got.Pages[0].Blocks[1].Collection != "todos" {
		t.Fatalf("form.collection = %q, want %q", got.Pages[0].Blocks[1].Collection, "todos")
	}
	if got.Pages[0].Blocks[2].Collection != "todos" {
		t.Fatalf("toggle.collection = %q, want %q", got.Pages[0].Blocks[2].Collection, "todos")
	}
	if got.Pages[0].Blocks[3].Collection != "todos" {
		t.Fatalf("stats.collection = %q, want %q", got.Pages[0].Blocks[3].Collection, "todos")
	}
	if got.Pages[0].Blocks[4].Collection != "" {
		t.Fatalf("timer.collection = %q, want empty", got.Pages[0].Blocks[4].Collection)
	}
}

func TestRepairCommonLLMSpecIssues_DoesNotGuessWhenMultipleCollections(t *testing.T) {
	t.Parallel()

	in := spec.AppSpec{
		AppName: "CRM",
		Collections: []spec.CollectionSpec{
			{
				Name: "customers",
				Fields: []spec.FieldSpec{
					{Name: "name", Type: spec.FieldTypeText},
				},
			},
			{
				Name: "tasks",
				Fields: []spec.FieldSpec{
					{Name: "title", Type: spec.FieldTypeText},
					{Name: "done", Type: spec.FieldTypeBool},
				},
			},
		},
		Pages: []spec.PageSpec{
			{
				ID: "dashboard",
				Blocks: []spec.BlockSpec{
					{Type: "toggle", Field: "done"},
				},
			},
		},
	}

	got := repairCommonLLMSpecIssues(in)

	if got.Pages[0].Blocks[0].Collection != "" {
		t.Fatalf("toggle.collection = %q, want empty", got.Pages[0].Blocks[0].Collection)
	}
}

func TestServiceGenerateDraft_RepairsMissingCollectionForSingleCollectionBlocks(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db, cleanup := openGenerationTestDB(t)
	defer cleanup()
	project := seedProject(t, ctx, db)

	invalidButRepairable := spec.AppSpec{
		AppName: "Todo App",
		Collections: []spec.CollectionSpec{
			{
				Name: "todos",
				Fields: []spec.FieldSpec{
					{Name: "title", Type: spec.FieldTypeText, Required: true},
					{Name: "done", Type: spec.FieldTypeBool, Required: true},
				},
			},
		},
		Pages: []spec.PageSpec{
			{
				ID: "dashboard",
				Blocks: []spec.BlockSpec{
					{Type: "list"},
					{Type: "form"},
					{Type: "toggle", Field: "done"},
					{Type: "stats", Metric: "count"},
				},
			},
		},
	}

	fc := &fakeClient{
		responses: []fakeClientResponse{
			{content: mustSpecJSON(t, invalidButRepairable)},
		},
	}
	svc := NewService(ServiceDeps{
		Projects: store.NewProjectRepo(db),
		Chats:    store.NewChatRepo(db),
		Client:   fc,
	})

	result, err := svc.GenerateDraft(ctx, GenerateDraftInput{
		UserID:     project.UserID,
		ProjectID:  project.ID,
		UserPrompt: "做一个待办应用",
	})
	if err != nil {
		t.Fatalf("GenerateDraft() error = %v", err)
	}
	if fc.callCount != 1 {
		t.Fatalf("client callCount = %d, want 1", fc.callCount)
	}

	var got spec.AppSpec
	if err := json.Unmarshal([]byte(result.DraftSpecJSON), &got); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if len(got.Pages) != 1 || len(got.Pages[0].Blocks) != 4 {
		t.Fatalf("unexpected pages/blocks: %#v", got.Pages)
	}
	for i, b := range got.Pages[0].Blocks {
		if b.Collection != "todos" {
			t.Fatalf("page block[%d] collection = %q, want %q", i, b.Collection, "todos")
		}
	}
}

type fakeClientResponse struct {
	content string
	err     error
}

type fakeClient struct {
	responses []fakeClientResponse
	callCount int
	requests  []ClientRequest
}

func (f *fakeClient) GenerateSpecJSON(_ context.Context, req ClientRequest) (string, error) {
	f.callCount++
	f.requests = append(f.requests, req)
	if len(f.responses) == 0 {
		return "", errors.New("no fake response configured")
	}
	resp := f.responses[0]
	f.responses = f.responses[1:]
	return resp.content, resp.err
}

func openGenerationTestDB(t *testing.T) (*gorm.DB, func()) {
	t.Helper()

	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "generation.db")
	db, err := store.OpenSQLite(ctx, dbPath)
	if err != nil {
		t.Fatalf("OpenSQLite() error = %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("db.DB() error = %v", err)
	}
	return db, func() { _ = sqlDB.Close() }
}

func seedProject(t *testing.T, ctx context.Context, db *gorm.DB) store.Project {
	t.Helper()

	authRepo := store.NewAuthRepo(db)
	user, err := authRepo.CreateUser(ctx, "gen@example.com", "hash")
	if err != nil {
		t.Fatalf("CreateUser() error = %v", err)
	}
	project, err := store.NewProjectRepo(db).CreateProject(ctx, user.ID, "Todo App", "做一个待办应用")
	if err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}
	return project
}

func validTodoSpec() spec.AppSpec {
	return spec.AppSpec{
		AppName: "Todo App",
		Theme:   "light",
		Collections: []spec.CollectionSpec{
			{
				Name: "todos",
				Fields: []spec.FieldSpec{
					{Name: "title", Type: spec.FieldTypeText, Required: true},
					{Name: "done", Type: spec.FieldTypeBool, Required: true},
				},
			},
		},
		Pages: []spec.PageSpec{
			{
				ID:    "home",
				Title: "Home",
				Blocks: []spec.BlockSpec{
					{Type: "form", Collection: "todos"},
					{Type: "list", Collection: "todos"},
					{Type: "toggle", Collection: "todos", Field: "done"},
					{Type: "stats", Collection: "todos", Metric: "count"},
				},
			},
		},
	}
}

func mustSpecJSON(t *testing.T, s spec.AppSpec) string {
	t.Helper()
	b, err := json.Marshal(s)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	return string(b)
}

func findCollection(appSpec spec.AppSpec, name string) (spec.CollectionSpec, bool) {
	for _, c := range appSpec.Collections {
		if c.Name == name {
			return c, true
		}
	}
	return spec.CollectionSpec{}, false
}

func mustFindCollection(t *testing.T, appSpec spec.AppSpec, name string) spec.CollectionSpec {
	t.Helper()
	c, ok := findCollection(appSpec, name)
	if !ok {
		t.Fatalf("collection %q not found", name)
	}
	return c
}

func hasField(c spec.CollectionSpec, fieldName string) bool {
	for _, f := range c.Fields {
		if f.Name == fieldName {
			return true
		}
	}
	return false
}

func fieldNames(c spec.CollectionSpec) []string {
	names := make([]string, 0, len(c.Fields))
	for _, f := range c.Fields {
		names = append(names, f.Name)
	}
	return names
}
