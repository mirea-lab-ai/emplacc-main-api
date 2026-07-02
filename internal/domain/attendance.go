package models

import (
	"time"

	"github.com/google/uuid"
)

func (Attendance) TableName() string { return "attendances" }

type Attendance struct {
	ID            uuid.UUID  `gorm:"type:uuid;primaryKey"`
	UserID        uuid.UUID  `gorm:"type:uuid;index"`
	Date          *time.Time `gorm:"type:date"`
	WorkdayHours  *int16
	PlannedStart  *time.Time `gorm:"type:time"`
	ActualStart   *time.Time `gorm:"type:time"`
	Commits       *int16
	MergeRequests *int16
	CodeReviews   *int16
	EndWork       *time.Time `gorm:"type:time"`
	Deleted       *bool      `gorm:"type:boolean"`
	CreatedAt     *time.Time `gorm:"type:timestamp"`
	UpdatedAt     *time.Time `gorm:"type:timestamp"`

	User *User `gorm:"foreignKey:UserID;references:ID;constraint:OnDelete:CASCADE"`
}
