package store

import "time"

type AppMetaModel struct {
	Key       string    `gorm:"column:key;primaryKey;type:text"`
	Value     string    `gorm:"column:value;type:text;not null"`
	CreatedAt time.Time `gorm:"column:created_at;not null;autoCreateTime"`
	UpdatedAt time.Time `gorm:"column:updated_at;not null;autoUpdateTime"`
}

func (AppMetaModel) TableName() string { return "app_meta" }

type UserModel struct {
	ID           int64     `gorm:"column:id;primaryKey;autoIncrement"`
	Email        string    `gorm:"column:email;type:text;not null;uniqueIndex"`
	PasswordHash string    `gorm:"column:password_hash;type:text;not null"`
	CreatedAt    time.Time `gorm:"column:created_at;not null;autoCreateTime"`
	UpdatedAt    time.Time `gorm:"column:updated_at;not null;autoUpdateTime"`
}

func (UserModel) TableName() string { return "users" }

type UserSessionModel struct {
	ID           int64     `gorm:"column:id;primaryKey;autoIncrement"`
	UserID       int64     `gorm:"column:user_id;not null;index"`
	SessionToken string    `gorm:"column:session_token;type:text;not null;uniqueIndex"`
	ExpiresAt    time.Time `gorm:"column:expires_at;not null;index"`
	CreatedAt    time.Time `gorm:"column:created_at;not null;autoCreateTime"`

	User UserModel `gorm:"foreignKey:UserID;references:ID;constraint:OnDelete:CASCADE"`
}

func (UserSessionModel) TableName() string { return "user_sessions" }

type ProjectModel struct {
	ID                int64      `gorm:"column:id;primaryKey;autoIncrement"`
	UserID            int64      `gorm:"column:user_id;not null;index"`
	Slug              string     `gorm:"column:slug;type:text;not null;uniqueIndex"`
	Name              string     `gorm:"column:name;type:text;not null"`
	GoalPrompt        string     `gorm:"column:goal_prompt;type:text;not null"`
	ShareSlug         *string    `gorm:"column:share_slug;type:text;uniqueIndex"`
	PublishedSlug     *string    `gorm:"column:published_slug;type:text;uniqueIndex"`
	DraftSpecJSON     string     `gorm:"column:draft_spec_json;type:text;not null;default:''"`
	PublishedSpecJSON string     `gorm:"column:published_spec_json;type:text;not null;default:''"`
	IsShowcase        bool       `gorm:"column:is_showcase;not null;default:false;index"`
	PublishedAt       *time.Time `gorm:"column:published_at"`
	CreatedAt         time.Time  `gorm:"column:created_at;not null;autoCreateTime"`
	UpdatedAt         time.Time  `gorm:"column:updated_at;not null;autoUpdateTime"`

	User UserModel `gorm:"foreignKey:UserID;references:ID;constraint:OnDelete:CASCADE"`
}

func (ProjectModel) TableName() string { return "projects" }

type CollectionModel struct {
	ID         int64     `gorm:"column:id;primaryKey;autoIncrement"`
	ProjectID  int64     `gorm:"column:project_id;not null;index;uniqueIndex:idx_collections_project_name"`
	Name       string    `gorm:"column:name;type:text;not null;uniqueIndex:idx_collections_project_name"`
	SchemaJSON string    `gorm:"column:schema_json;type:text;not null"`
	CreatedAt  time.Time `gorm:"column:created_at;not null;autoCreateTime"`
	UpdatedAt  time.Time `gorm:"column:updated_at;not null;autoUpdateTime"`

	Project ProjectModel `gorm:"foreignKey:ProjectID;references:ID;constraint:OnDelete:CASCADE"`
}

func (CollectionModel) TableName() string { return "collections" }

type RecordModel struct {
	ID           int64     `gorm:"column:id;primaryKey;autoIncrement"`
	ProjectID    int64     `gorm:"column:project_id;not null;index:idx_records_project_collection"`
	CollectionID int64     `gorm:"column:collection_id;not null;index:idx_records_project_collection;index"`
	DataJSON     string    `gorm:"column:data_json;type:text;not null"`
	CreatedAt    time.Time `gorm:"column:created_at;not null;autoCreateTime"`
	UpdatedAt    time.Time `gorm:"column:updated_at;not null;autoUpdateTime;index"`

	Project    ProjectModel    `gorm:"foreignKey:ProjectID;references:ID;constraint:OnDelete:CASCADE"`
	Collection CollectionModel `gorm:"foreignKey:CollectionID;references:ID;constraint:OnDelete:CASCADE"`
}

func (RecordModel) TableName() string { return "records" }

type ChatMessageModel struct {
	ID        int64     `gorm:"column:id;primaryKey;autoIncrement"`
	ProjectID int64     `gorm:"column:project_id;not null;index"`
	RoundNo   int       `gorm:"column:round_no;not null;index"`
	Role      string    `gorm:"column:role;type:text;not null;index"`
	Content   string    `gorm:"column:content;type:text;not null"`
	CreatedAt time.Time `gorm:"column:created_at;not null;autoCreateTime"`

	Project ProjectModel `gorm:"foreignKey:ProjectID;references:ID;constraint:OnDelete:CASCADE"`
}

func (ChatMessageModel) TableName() string { return "chat_messages" }
