package models

import (
	"time"

	"github.com/google/uuid"
)

func (APIToken) TableName() string { return "api_tokens" }

type APIToken struct {
	ID         uuid.UUID `gorm:"type:uuid;primaryKey"`
	UserID     uuid.UUID `gorm:"type:uuid;not null;index"`
	Name       string    `gorm:"size:100"`
	TokenHash  string    `gorm:"size:64;uniqueIndex"` // SHA-256 hex
	LastUsedAt *time.Time
	ExpiresAt  *time.Time
	Deleted    bool `gorm:"default:false"`
	CreatedAt  time.Time
	UpdatedAt  time.Time
}
