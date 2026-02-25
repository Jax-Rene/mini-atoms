package store

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"
)

type Project struct {
	ID                int64
	UserID            int64
	Slug              string
	Name              string
	GoalPrompt        string
	ShareSlug         string
	PublishedSlug     string
	DraftSpecJSON     string
	PublishedSpecJSON string
	IsShowcase        bool
	PublishedAt       *time.Time
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

type ProjectRepo struct {
	db *gorm.DB
}

func NewProjectRepo(db *gorm.DB) *ProjectRepo {
	return &ProjectRepo{db: db}
}

func (r *ProjectRepo) CreateProject(ctx context.Context, userID int64, name, goalPrompt string) (Project, error) {
	name = strings.TrimSpace(name)
	goalPrompt = strings.TrimSpace(goalPrompt)
	if userID == 0 {
		return Project{}, fmt.Errorf("create project: user id is required")
	}
	if goalPrompt == "" {
		return Project{}, fmt.Errorf("create project: goal prompt is required")
	}
	if name == "" {
		name = generateProjectNameFromPrompt(goalPrompt)
	}
	if len([]rune(name)) > projectNameMaxLen {
		name = trimRunes(name, projectNameMaxLen)
	}
	name = strings.TrimSpace(name)
	if name == "" {
		name = "Untitled Project"
	}

	slug, err := generateUniqueReadableProjectSlug(
		name,
		func(candidate string) (bool, error) { return r.ProjectSlugExists(ctx, candidate) },
		nil,
	)
	if err != nil {
		return Project{}, fmt.Errorf("create project slug: %w", err)
	}

	row := ProjectModel{
		UserID:            userID,
		Slug:              slug,
		Name:              name,
		GoalPrompt:        goalPrompt,
		IsShowcase:        false,
		DraftSpecJSON:     "",
		PublishedSpecJSON: "",
	}
	if err := r.db.WithContext(ctx).Create(&row).Error; err != nil {
		if isUniqueConstraintError(err) {
			return Project{}, ErrConflict
		}
		return Project{}, fmt.Errorf("create project: %w", err)
	}
	return toProject(row), nil
}

func (r *ProjectRepo) ListProjectsByUser(ctx context.Context, userID int64) ([]Project, error) {
	var rows []ProjectModel
	if err := r.db.WithContext(ctx).
		Where("user_id = ?", userID).
		Order("updated_at DESC, id DESC").
		Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("list projects by user: %w", err)
	}
	projects := make([]Project, 0, len(rows))
	for _, row := range rows {
		projects = append(projects, toProject(row))
	}
	return projects, nil
}

func (r *ProjectRepo) ListShowcaseProjects(ctx context.Context, limit int) ([]Project, error) {
	var rows []ProjectModel
	q := r.db.WithContext(ctx).
		Where("is_showcase = ?", true).
		Order("updated_at DESC, id DESC")
	if limit > 0 {
		q = q.Limit(limit)
	}
	if err := q.Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("list showcase projects: %w", err)
	}
	projects := make([]Project, 0, len(rows))
	for _, row := range rows {
		projects = append(projects, toProject(row))
	}
	return projects, nil
}

func (r *ProjectRepo) GetProjectByUserAndSlug(ctx context.Context, userID int64, slug string) (Project, error) {
	var row ProjectModel
	if err := r.db.WithContext(ctx).
		Where("user_id = ? AND slug = ?", userID, strings.TrimSpace(slug)).
		First(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return Project{}, ErrNotFound
		}
		return Project{}, fmt.Errorf("get project by user and slug: %w", err)
	}
	return toProject(row), nil
}

func (r *ProjectRepo) GetProjectByUserAndID(ctx context.Context, userID, projectID int64) (Project, error) {
	var row ProjectModel
	if err := r.db.WithContext(ctx).
		Where("user_id = ? AND id = ?", userID, projectID).
		First(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return Project{}, ErrNotFound
		}
		return Project{}, fmt.Errorf("get project by user and id: %w", err)
	}
	return toProject(row), nil
}

func (r *ProjectRepo) GetProjectByPublishedSlug(ctx context.Context, slug string) (Project, error) {
	var row ProjectModel
	if err := r.db.WithContext(ctx).
		Where("published_slug = ?", strings.TrimSpace(slug)).
		First(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return Project{}, ErrNotFound
		}
		return Project{}, fmt.Errorf("get project by published slug: %w", err)
	}
	return toProject(row), nil
}

func (r *ProjectRepo) GetProjectByShareSlug(ctx context.Context, slug string) (Project, error) {
	var row ProjectModel
	if err := r.db.WithContext(ctx).
		Where("share_slug = ?", strings.TrimSpace(slug)).
		First(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return Project{}, ErrNotFound
		}
		return Project{}, fmt.Errorf("get project by share slug: %w", err)
	}
	return toProject(row), nil
}

func (r *ProjectRepo) UpdateProjectSpecsByID(ctx context.Context, userID, projectID int64, draftSpecJSON, publishedSpecJSON string) error {
	tx := r.db.WithContext(ctx).
		Model(&ProjectModel{}).
		Where("id = ? AND user_id = ?", projectID, userID).
		Updates(map[string]any{
			"draft_spec_json":     draftSpecJSON,
			"published_spec_json": publishedSpecJSON,
			"updated_at":          time.Now().UTC(),
		})
	if tx.Error != nil {
		return fmt.Errorf("update project specs by id: %w", tx.Error)
	}
	if tx.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *ProjectRepo) ProjectSlugExists(ctx context.Context, slug string) (bool, error) {
	var count int64
	if err := r.db.WithContext(ctx).
		Model(&ProjectModel{}).
		Where("slug = ?", strings.TrimSpace(slug)).
		Count(&count).Error; err != nil {
		return false, fmt.Errorf("count project slug: %w", err)
	}
	return count > 0, nil
}

func (r *ProjectRepo) PublishedSlugExists(ctx context.Context, slug string) (bool, error) {
	var count int64
	if err := r.db.WithContext(ctx).
		Model(&ProjectModel{}).
		Where("published_slug = ?", strings.TrimSpace(slug)).
		Count(&count).Error; err != nil {
		return false, fmt.Errorf("count published slug: %w", err)
	}
	return count > 0, nil
}

func (r *ProjectRepo) ShareSlugExists(ctx context.Context, slug string) (bool, error) {
	var count int64
	if err := r.db.WithContext(ctx).
		Model(&ProjectModel{}).
		Where("share_slug = ?", strings.TrimSpace(slug)).
		Count(&count).Error; err != nil {
		return false, fmt.Errorf("count share slug: %w", err)
	}
	return count > 0, nil
}

func (r *ProjectRepo) PublishProjectByUserAndSlug(ctx context.Context, userID int64, slug string) (Project, error) {
	project, err := r.GetProjectByUserAndSlug(ctx, userID, slug)
	if err != nil {
		return Project{}, err
	}
	if strings.TrimSpace(project.DraftSpecJSON) == "" {
		return Project{}, fmt.Errorf("publish project: draft spec is empty")
	}

	publishedSlug := project.PublishedSlug
	if strings.TrimSpace(publishedSlug) == "" {
		publishedSlug, err = generateUniqueReadableProjectSlug(
			project.Name,
			func(candidate string) (bool, error) { return r.PublishedSlugExists(ctx, candidate) },
			nil,
		)
		if err != nil {
			return Project{}, fmt.Errorf("publish project slug: %w", err)
		}
	}

	now := time.Now().UTC()
	updates := map[string]any{
		"published_spec_json": project.DraftSpecJSON,
		"published_slug":      publishedSlug,
		"published_at":        now,
		"updated_at":          now,
	}
	tx := r.db.WithContext(ctx).
		Model(&ProjectModel{}).
		Where("id = ? AND user_id = ?", project.ID, userID).
		Updates(updates)
	if tx.Error != nil {
		if isUniqueConstraintError(tx.Error) {
			return Project{}, ErrConflict
		}
		return Project{}, fmt.Errorf("publish project: %w", tx.Error)
	}
	if tx.RowsAffected == 0 {
		return Project{}, ErrNotFound
	}
	return r.GetProjectByUserAndSlug(ctx, userID, slug)
}

func (r *ProjectRepo) EnsureShareSlugByUserAndSlug(ctx context.Context, userID int64, slug string) (Project, error) {
	project, err := r.GetProjectByUserAndSlug(ctx, userID, slug)
	if err != nil {
		return Project{}, err
	}
	if strings.TrimSpace(project.ShareSlug) != "" {
		return project, nil
	}

	var shareSlug string
	var lastErr error
	for range projectSlugMaxTries {
		candidate, err := randomShareSlug()
		if err != nil {
			return Project{}, fmt.Errorf("ensure share slug: %w", err)
		}
		exists, err := r.ShareSlugExists(ctx, candidate)
		if err != nil {
			return Project{}, fmt.Errorf("ensure share slug exists: %w", err)
		}
		if exists {
			lastErr = ErrConflict
			continue
		}
		shareSlug = candidate
		break
	}
	if shareSlug == "" {
		if lastErr != nil {
			return Project{}, fmt.Errorf("ensure share slug retries exhausted: %w", lastErr)
		}
		return Project{}, fmt.Errorf("ensure share slug retries exhausted")
	}

	tx := r.db.WithContext(ctx).
		Model(&ProjectModel{}).
		Where("id = ? AND user_id = ?", project.ID, userID).
		Updates(map[string]any{
			"share_slug": shareSlug,
			"updated_at": time.Now().UTC(),
		})
	if tx.Error != nil {
		if isUniqueConstraintError(tx.Error) {
			return Project{}, ErrConflict
		}
		return Project{}, fmt.Errorf("ensure share slug: %w", tx.Error)
	}
	if tx.RowsAffected == 0 {
		return Project{}, ErrNotFound
	}
	return r.GetProjectByUserAndSlug(ctx, userID, slug)
}

func toProject(row ProjectModel) Project {
	return Project{
		ID:                row.ID,
		UserID:            row.UserID,
		Slug:              row.Slug,
		Name:              row.Name,
		GoalPrompt:        row.GoalPrompt,
		ShareSlug:         stringValue(row.ShareSlug),
		PublishedSlug:     stringValue(row.PublishedSlug),
		DraftSpecJSON:     row.DraftSpecJSON,
		PublishedSpecJSON: row.PublishedSpecJSON,
		IsShowcase:        row.IsShowcase,
		PublishedAt:       row.PublishedAt,
		CreatedAt:         row.CreatedAt,
		UpdatedAt:         row.UpdatedAt,
	}
}

func stringValue(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}
