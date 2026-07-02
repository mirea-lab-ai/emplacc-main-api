package models

import (
	"time"

	"github.com/google/uuid"
)

func (Task) TableName() string { return "tasks" }

type Task struct {
	ID            uuid.UUID `gorm:"type:uuid;primaryKey"`
	Priority      *int16
	Name          *string    `gorm:"size:100"`
	Description   *string    `gorm:"type:text"`
	CreatedBy     *uuid.UUID `gorm:"type:uuid"`
	AssignedTo    *uuid.UUID `gorm:"type:uuid"`
	Deadline      *time.Time
	TimeSpent     *string `gorm:"type:interval"`
	StartDate     *time.Time
	GitlabIssueID *int
	StatusID      uuid.UUID `gorm:"type:uuid;index"`
	Category      *int8
	Deleted       *bool      `gorm:"type:boolean"`
	CreatedAt     *time.Time `gorm:"type:timestamp"`
	UpdatedAt     *time.Time `gorm:"type:timestamp"`

	Status         *Status `gorm:"foreignKey:StatusID;references:ID;constraint:OnDelete:CASCADE"`
	CreatedByUser  *User   `gorm:"foreignKey:CreatedBy;references:ID"`
	AssignedToUser *User   `gorm:"foreignKey:AssignedTo;references:ID"`
}
