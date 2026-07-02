package models

import (
	"time"

	"github.com/google/uuid"
)

// CodeRepository — внешний репозиторий, привязанный к проекту.
// Один проект ↔ несколько репозиториев (репо в орг-аккаунте + личные аккаунты),
// независимо от провайдера (github / gitflic / gitverse). Это закрывает реальность
// «лаба разрозненно на GitHub» и заявленную в дипломе независимость от хранилища.
type CodeRepository struct {
	ID         uuid.UUID  `gorm:"type:uuid;primaryKey" json:"id"`
	ProjectID  uuid.UUID  `gorm:"type:uuid;index;not null" json:"project_id"`
	Provider   string     `gorm:"size:32;not null" json:"provider"` // github | gitflic | gitverse
	Owner      string     `gorm:"size:200;not null" json:"owner"`   // owner / namespace
	Name       string     `gorm:"size:200;not null" json:"name"`
	URL        string     `gorm:"size:500" json:"url,omitempty"`
	Branch     string     `gorm:"size:200" json:"branch,omitempty"`
	ExternalID string     `gorm:"size:100" json:"external_id,omitempty"`
	LastSyncAt *time.Time `gorm:"type:timestamp" json:"last_sync_at,omitempty"`
	CreatedAt  time.Time  `gorm:"type:timestamp" json:"created_at"`
	Deleted    bool       `gorm:"type:boolean;default:false" json:"-"`
}

// Commit — коммит из внешнего репозитория, опционально привязанный к задаче.
// Уникальность по (repository_id, sha) обеспечивает идемпотентный синк.
type Commit struct {
	ID           uuid.UUID  `gorm:"type:uuid;primaryKey" json:"id"`
	RepositoryID uuid.UUID  `gorm:"type:uuid;not null;uniqueIndex:idx_commit_repo_sha" json:"repository_id"`
	SHA          string     `gorm:"size:64;not null;uniqueIndex:idx_commit_repo_sha;index" json:"sha"`
	TaskID       *uuid.UUID `gorm:"type:uuid;index" json:"task_id,omitempty"`
	Message      string     `gorm:"type:text" json:"message"`
	AuthorName   string     `gorm:"size:200" json:"author_name,omitempty"`
	AuthorEmail  string     `gorm:"size:200" json:"author_email,omitempty"`
	AuthorLogin  string     `gorm:"size:200" json:"author_login,omitempty"`
	AuthorUserID *uuid.UUID `gorm:"type:uuid;index" json:"author_user_id,omitempty"`
	URL          string     `gorm:"size:500" json:"url,omitempty"`
	CommittedAt  time.Time  `gorm:"type:timestamp;index" json:"committed_at"`
	CreatedAt    time.Time  `gorm:"type:timestamp" json:"created_at"`

	Repository *CodeRepository `gorm:"foreignKey:RepositoryID;references:ID;constraint:OnDelete:CASCADE" json:"-"`
}
