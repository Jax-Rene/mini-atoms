package store

import (
	"context"
	"strings"
	"testing"
)

func TestProjectRepo_CreateListAndOwnership(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db, cleanup := openTestDB(t)
	defer cleanup()

	authRepo := NewAuthRepo(db)
	owner, err := authRepo.CreateUser(ctx, "owner@example.com", "hash-1")
	if err != nil {
		t.Fatalf("create owner: %v", err)
	}
	other, err := authRepo.CreateUser(ctx, "other@example.com", "hash-2")
	if err != nil {
		t.Fatalf("create other: %v", err)
	}

	repo := NewProjectRepo(db)

	project, err := repo.CreateProject(ctx, owner.ID, "Resume Builder", "帮我做一个简历网站")
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	if project.ID == 0 {
		t.Fatal("expected project id")
	}
	if project.UserID != owner.ID {
		t.Fatalf("project user id = %d, want %d", project.UserID, owner.ID)
	}
	if project.Slug == "" {
		t.Fatal("expected project slug")
	}
	if project.PublishedSlug != "" {
		t.Fatalf("published slug = %q, want empty", project.PublishedSlug)
	}
	if project.ShareSlug != "" {
		t.Fatalf("share slug = %q, want empty", project.ShareSlug)
	}

	got, err := repo.GetProjectByUserAndSlug(ctx, owner.ID, project.Slug)
	if err != nil {
		t.Fatalf("get owner project by slug: %v", err)
	}
	if got.Name != "Resume Builder" {
		t.Fatalf("project name = %q", got.Name)
	}
	if got.GoalPrompt != "帮我做一个简历网站" {
		t.Fatalf("goal prompt = %q", got.GoalPrompt)
	}

	_, err = repo.GetProjectByUserAndSlug(ctx, other.ID, project.Slug)
	if err != ErrNotFound {
		t.Fatalf("cross-user get err = %v, want %v", err, ErrNotFound)
	}

	list, err := repo.ListProjectsByUser(ctx, owner.ID)
	if err != nil {
		t.Fatalf("list projects: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("list len = %d, want 1", len(list))
	}
	if list[0].ID != project.ID {
		t.Fatalf("listed project id = %d, want %d", list[0].ID, project.ID)
	}
}

func TestProjectRepo_CreateProjectAutoGeneratesNameWhenEmpty(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db, cleanup := openTestDB(t)
	defer cleanup()

	authRepo := NewAuthRepo(db)
	user, err := authRepo.CreateUser(ctx, "auto-name@example.com", "hash")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}

	repo := NewProjectRepo(db)
	project, err := repo.CreateProject(ctx, user.ID, "", "帮我做一个简历网站，支持项目经历、作品集展示")
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	if strings.TrimSpace(project.Name) == "" {
		t.Fatal("expected generated project name")
	}
	if len([]rune(project.Name)) > projectNameMaxLen {
		t.Fatalf("generated project name too long: %q", project.Name)
	}
}

func TestProjectRepo_UpdateProjectSpecsByID(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db, cleanup := openTestDB(t)
	defer cleanup()

	authRepo := NewAuthRepo(db)
	user, err := authRepo.CreateUser(ctx, "spec@example.com", "hash")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}

	repo := NewProjectRepo(db)
	project, err := repo.CreateProject(ctx, user.ID, "Todo App", "做一个待办应用")
	if err != nil {
		t.Fatalf("create project: %v", err)
	}

	draft := `{"app_name":"Todo Draft"}`
	published := `{"app_name":"Todo Published"}`
	if err := repo.UpdateProjectSpecsByID(ctx, user.ID, project.ID, draft, published); err != nil {
		t.Fatalf("update specs: %v", err)
	}

	got, err := repo.GetProjectByUserAndSlug(ctx, user.ID, project.Slug)
	if err != nil {
		t.Fatalf("get updated project: %v", err)
	}
	if got.DraftSpecJSON != draft {
		t.Fatalf("draft spec = %q, want %q", got.DraftSpecJSON, draft)
	}
	if got.PublishedSpecJSON != published {
		t.Fatalf("published spec = %q, want %q", got.PublishedSpecJSON, published)
	}
}

func TestProjectRepo_PublishDraftAndReusePublishedSlug(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db, cleanup := openTestDB(t)
	defer cleanup()

	authRepo := NewAuthRepo(db)
	user, err := authRepo.CreateUser(ctx, "publish@example.com", "hash")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}

	repo := NewProjectRepo(db)
	project, err := repo.CreateProject(ctx, user.ID, "Todo App", "做一个待办应用")
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	if err := repo.UpdateProjectSpecsByID(ctx, user.ID, project.ID, `{"app_name":"Draft v1","collections":[{"name":"todos","fields":[{"name":"title","type":"text"},{"name":"done","type":"bool"}]}],"pages":[{"id":"home","blocks":[{"type":"form","collection":"todos"},{"type":"list","collection":"todos"},{"type":"toggle","collection":"todos","field":"done"},{"type":"stats","collection":"todos","metric":"count"}]}]}`, ""); err != nil {
		t.Fatalf("update draft spec: %v", err)
	}

	published1, err := repo.PublishProjectByUserAndSlug(ctx, user.ID, project.Slug)
	if err != nil {
		t.Fatalf("publish v1: %v", err)
	}
	if strings.TrimSpace(published1.PublishedSpecJSON) == "" {
		t.Fatal("expected published spec json")
	}
	if strings.TrimSpace(published1.PublishedSlug) == "" {
		t.Fatal("expected published slug")
	}
	if published1.PublishedAt == nil {
		t.Fatal("expected published_at")
	}
	firstPublishedSlug := published1.PublishedSlug

	if err := repo.UpdateProjectSpecsByID(ctx, user.ID, project.ID, `{"app_name":"Draft v2","collections":[{"name":"todos","fields":[{"name":"title","type":"text"},{"name":"done","type":"bool"}]}],"pages":[{"id":"home","blocks":[{"type":"list","collection":"todos"},{"type":"toggle","collection":"todos","field":"done"},{"type":"stats","collection":"todos","metric":"count"}]}]}`, published1.PublishedSpecJSON); err != nil {
		t.Fatalf("update draft spec v2: %v", err)
	}
	published2, err := repo.PublishProjectByUserAndSlug(ctx, user.ID, project.Slug)
	if err != nil {
		t.Fatalf("publish v2: %v", err)
	}
	if published2.PublishedSlug != firstPublishedSlug {
		t.Fatalf("published slug changed = %q, want %q", published2.PublishedSlug, firstPublishedSlug)
	}
	if !strings.Contains(published2.PublishedSpecJSON, "Draft v2") {
		t.Fatalf("published spec not overwritten, got %q", published2.PublishedSpecJSON)
	}
}

func TestProjectRepo_EnsureShareSlugReusesExisting(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db, cleanup := openTestDB(t)
	defer cleanup()

	authRepo := NewAuthRepo(db)
	user, err := authRepo.CreateUser(ctx, "share@example.com", "hash")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}

	repo := NewProjectRepo(db)
	project, err := repo.CreateProject(ctx, user.ID, "Todo App", "做一个待办应用")
	if err != nil {
		t.Fatalf("create project: %v", err)
	}

	first, err := repo.EnsureShareSlugByUserAndSlug(ctx, user.ID, project.Slug)
	if err != nil {
		t.Fatalf("ensure share slug first: %v", err)
	}
	if strings.TrimSpace(first.ShareSlug) == "" {
		t.Fatal("expected share slug")
	}

	second, err := repo.EnsureShareSlugByUserAndSlug(ctx, user.ID, project.Slug)
	if err != nil {
		t.Fatalf("ensure share slug second: %v", err)
	}
	if second.ShareSlug != first.ShareSlug {
		t.Fatalf("share slug changed = %q, want %q", second.ShareSlug, first.ShareSlug)
	}

	got, err := repo.GetProjectByShareSlug(ctx, second.ShareSlug)
	if err != nil {
		t.Fatalf("get by share slug: %v", err)
	}
	if got.ID != project.ID {
		t.Fatalf("share slug project id = %d, want %d", got.ID, project.ID)
	}
}
