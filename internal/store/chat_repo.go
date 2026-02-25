package store

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"
)

const (
	ChatRoleUser      = "user"
	ChatRoleAssistant = "assistant"
	ChatRoleSystem    = "system"
)

type ChatMessage struct {
	ID        int64
	ProjectID int64
	RoundNo   int
	Role      string
	Content   string
	CreatedAt time.Time
}

type ChatRepo struct {
	db *gorm.DB
}

func NewChatRepo(db *gorm.DB) *ChatRepo {
	return &ChatRepo{db: db}
}

func (r *ChatRepo) CreateMessage(ctx context.Context, projectID int64, roundNo int, role, content string) (ChatMessage, error) {
	role = strings.TrimSpace(role)
	content = strings.TrimSpace(content)
	if projectID == 0 {
		return ChatMessage{}, fmt.Errorf("create chat message: project id is required")
	}
	if roundNo <= 0 {
		return ChatMessage{}, fmt.Errorf("create chat message: round no must be > 0")
	}
	if role == "" {
		return ChatMessage{}, fmt.Errorf("create chat message: role is required")
	}
	if content == "" {
		return ChatMessage{}, fmt.Errorf("create chat message: content is required")
	}

	row := ChatMessageModel{
		ProjectID: projectID,
		RoundNo:   roundNo,
		Role:      role,
		Content:   content,
	}
	if err := r.db.WithContext(ctx).Create(&row).Error; err != nil {
		return ChatMessage{}, fmt.Errorf("create chat message: %w", err)
	}
	return toChatMessage(row), nil
}

func (r *ChatRepo) ListMessagesByProject(ctx context.Context, projectID int64) ([]ChatMessage, error) {
	var rows []ChatMessageModel
	if err := r.db.WithContext(ctx).
		Where("project_id = ?", projectID).
		Order("id ASC").
		Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("list chat messages by project: %w", err)
	}

	out := make([]ChatMessage, 0, len(rows))
	for _, row := range rows {
		out = append(out, toChatMessage(row))
	}
	return out, nil
}

func (r *ChatRepo) NextRoundNo(ctx context.Context, projectID int64) (int, error) {
	var row ChatMessageModel
	err := r.db.WithContext(ctx).
		Where("project_id = ?", projectID).
		Order("round_no DESC, id DESC").
		First(&row).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return 1, nil
		}
		return 0, fmt.Errorf("next round no: %w", err)
	}
	return row.RoundNo + 1, nil
}

func toChatMessage(row ChatMessageModel) ChatMessage {
	return ChatMessage{
		ID:        row.ID,
		ProjectID: row.ProjectID,
		RoundNo:   row.RoundNo,
		Role:      row.Role,
		Content:   row.Content,
		CreatedAt: row.CreatedAt,
	}
}
