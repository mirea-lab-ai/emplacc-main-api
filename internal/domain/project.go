package models

import (
	"time"

	"github.com/google/uuid"
)

func (Project) TableName() string { return "projects" }

type Project struct {
	ID              uuid.UUID  `gorm:"type:uuid;primaryKey"`
	Name            *string    `gorm:"size:100"`
	Description     *string    `gorm:"type:text"`
	CreatedAt       *time.Time `gorm:"type:timestamp"`
	GitlabProjectID *int
	GitlabURL       *string    `gorm:"size:255"`
	CreatedBy       *uuid.UUID `gorm:"type:uuid"`
	Status          *string    `gorm:"type:varchar(50)"`
	Deleted         *bool      `gorm:"type:boolean"`
	UpdatedAt       *time.Time `gorm:"type:timestamp"`

	Boards       []Board       `gorm:"foreignKey:ProjectID;references:ID;constraint:OnDelete:CASCADE"`
	ProjectTeams []ProjectTeam `gorm:"foreignKey:ProjectID;references:ID;constraint:OnDelete:CASCADE"`

	CreatedByUser *User `gorm:"foreignKey:CreatedBy;references:ID"`
}
