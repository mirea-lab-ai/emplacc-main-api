package models

import (
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
)

func (Problem) TableName() string      { return "problems" }
func (ForumMessage) TableName() string { return "forum_messages" }

type Problem struct {
	ID          uuid.UUID      `gorm:"type:uuid;primaryKey"`
	Description pq.StringArray `gorm:"type:text[]"`
	CreatorID   *uuid.UUID     `gorm:"type:uuid;index"`
	Name        *string        `gorm:"type:varchar(255)"`
	CreatedAt   *time.Time     `gorm:"type:timestamp"`
	UpdatedAt   *time.Time     `gorm:"type:timestamp"`
	Deleted     *bool          `gorm:"type:boolean"`

	User  *User          `gorm:"foreignKey:CreatorID;references:ID;constraint:OnDelete:CASCADE"`
	Forum []ForumMessage `gorm:"foreignKey:ProblemID;references:ID"`
}

type ForumMessage struct {
	ID          uuid.UUID      `gorm:"type:uuid;primaryKey"`
	ProblemID   uuid.UUID      `gorm:"type:uuid;index"`
	Description pq.StringArray `gorm:"type:text[]"`
	CreatorID   *uuid.UUID     `gorm:"type:uuid;index"`
	ReplyToID   *uuid.UUID     `gorm:"type:uuid;index"`
	CreatedAt   *time.Time     `gorm:"type:timestamp"`
	UpdatedAt   *time.Time     `gorm:"type:timestamp"`
	Deleted     *bool          `gorm:"type:boolean"`

	Problem *Problem      `gorm:"foreignKey:ProblemID;references:ID;constraint:OnDelete:CASCADE"`
	User    *User         `gorm:"foreignKey:CreatorID;references:ID;constraint:OnDelete:CASCADE"`
	ReplyTo *ForumMessage `gorm:"foreignKey:ReplyToID;references:ID"`
}
