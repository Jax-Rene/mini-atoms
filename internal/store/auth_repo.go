package store

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"
)

var (
	ErrNotFound = errors.New("store: not found")
	ErrConflict = errors.New("store: conflict")
)

type User struct {
	ID           int64
	Email        string
	PasswordHash string
}

type Session struct {
	ID        int64
	UserID    int64
	Token     string
	ExpiresAt time.Time
}

type UserSession struct {
	User    User
	Session Session
}

type AuthRepo struct {
	db *gorm.DB
}

func NewAuthRepo(db *gorm.DB) *AuthRepo {
	return &AuthRepo{db: db}
}

func (r *AuthRepo) CreateUser(ctx context.Context, email, passwordHash string) (User, error) {
	row := UserModel{
		Email:        email,
		PasswordHash: passwordHash,
	}
	if err := r.db.WithContext(ctx).Create(&row).Error; err != nil {
		if isUniqueConstraintError(err) {
			return User{}, ErrConflict
		}
		return User{}, fmt.Errorf("create user: %w", err)
	}
	return toUser(row), nil
}

func (r *AuthRepo) GetUserByEmail(ctx context.Context, email string) (User, error) {
	var row UserModel
	if err := r.db.WithContext(ctx).Where("email = ?", email).First(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return User{}, ErrNotFound
		}
		return User{}, fmt.Errorf("get user by email: %w", err)
	}
	return toUser(row), nil
}

func (r *AuthRepo) CreateSession(ctx context.Context, userID int64, token string, expiresAt time.Time) error {
	row := UserSessionModel{
		UserID:       userID,
		SessionToken: token,
		ExpiresAt:    expiresAt.UTC(),
	}
	if err := r.db.WithContext(ctx).Create(&row).Error; err != nil {
		if isUniqueConstraintError(err) {
			return ErrConflict
		}
		return fmt.Errorf("create session: %w", err)
	}
	return nil
}

func (r *AuthRepo) GetUserBySessionToken(ctx context.Context, token string) (UserSession, error) {
	var row UserSessionModel
	if err := r.db.WithContext(ctx).
		Preload("User").
		Where("session_token = ?", token).
		First(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return UserSession{}, ErrNotFound
		}
		return UserSession{}, fmt.Errorf("get session by token: %w", err)
	}

	return UserSession{
		User: toUser(row.User),
		Session: Session{
			ID:        row.ID,
			UserID:    row.UserID,
			Token:     row.SessionToken,
			ExpiresAt: row.ExpiresAt.UTC(),
		},
	}, nil
}

func (r *AuthRepo) DeleteSessionByToken(ctx context.Context, token string) error {
	if token == "" {
		return nil
	}
	if err := r.db.WithContext(ctx).Where("session_token = ?", token).Delete(&UserSessionModel{}).Error; err != nil {
		return fmt.Errorf("delete session by token: %w", err)
	}
	return nil
}

func toUser(row UserModel) User {
	return User{
		ID:           row.ID,
		Email:        row.Email,
		PasswordHash: row.PasswordHash,
	}
}

func isUniqueConstraintError(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(strings.ToLower(err.Error()), "unique")
}
