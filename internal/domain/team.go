package models

import (
	"time"

	"github.com/google/uuid"
)

func (Team) TableName() string        { return "teams" }
func (TeamMember) TableName() string  { return "team_members" }
func (ProjectTeam) TableName() string { return "project_teams" }

type Team struct {
	ID          uuid.UUID  `gorm:"type:uuid;primaryKey"`
	Name        *string    `gorm:"size:100"`
	Description *string    `gorm:"type:text"`
	Deleted     *bool      `gorm:"type:boolean"`
	CreatedAt   *time.Time `gorm:"type:timestamp"`
	UpdatedAt   *time.Time `gorm:"type:timestamp"`

	TeamMembers  []TeamMember  `gorm:"foreignKey:TeamID;references:ID;constraint:OnDelete:CASCADE"`
	ProjectTeams []ProjectTeam `gorm:"foreignKey:TeamID;references:ID;constraint:OnDelete:CASCADE"`
}

type TeamMember struct {
	UserID         uuid.UUID  `gorm:"type:uuid;primaryKey;index"`
	TeamID         uuid.UUID  `gorm:"type:uuid;primaryKey;index"`
	Specialization *string    `gorm:"size:50"`
	Deleted        *bool      `gorm:"type:boolean"`
	CreatedAt      *time.Time `gorm:"type:timestamp"`
	UpdatedAt      *time.Time `gorm:"type:timestamp"`

	User *User `gorm:"foreignKey:UserID;references:ID;constraint:OnDelete:CASCADE"`
	Team *Team `gorm:"foreignKey:TeamID;references:ID;constraint:OnDelete:CASCADE"`
}

type ProjectTeam struct {
	ProjectID uuid.UUID  `gorm:"type:uuid;primaryKey;index"`
	TeamID    uuid.UUID  `gorm:"type:uuid;primaryKey;index"`
	Deleted   *bool      `gorm:"type:boolean"`
	CreatedAt *time.Time `gorm:"type:timestamp"`
	UpdatedAt *time.Time `gorm:"type:timestamp"`

	Project *Project `gorm:"foreignKey:ProjectID;references:ID;constraint:OnDelete:CASCADE"`
	Team    *Team    `gorm:"foreignKey:TeamID;references:ID;constraint:OnDelete:CASCADE"`
}
