package models

import (
	"time"

	"github.com/google/uuid"
)

func (Board) TableName() string  { return "boards" }
func (Status) TableName() string { return "statuses" }

type Board struct {
	ID          uuid.UUID  `gorm:"type:uuid;primaryKey"`
	ProjectID   uuid.UUID  `gorm:"type:uuid;index;not null"`
	Name        *string    `gorm:"size:100"`
	Description *string    `gorm:"type:text"`
	Deleted     *bool      `gorm:"type:boolean"`
	CreatedAt   *time.Time `gorm:"type:timestamp"`
	UpdatedAt   *time.Time `gorm:"type:timestamp"`

	Project *Project `gorm:"foreignKey:ProjectID;references:ID;constraint:OnDelete:CASCADE"`

	// явная джойн-таблица
	Statuses []Status `gorm:"foreignKey:BoardID;references:ID;constraint:OnDelete:CASCADE"`
}

type Status struct {
	ID        uuid.UUID  `gorm:"type:uuid;primaryKey"`
	BoardID   uuid.UUID  `gorm:"type:uuid;index;not null"`
	SortOrder *int       `gorm:"type:int"`
	Key       *string    `gorm:"type:varchar(8);uniqueIndex"`
	Name      *string    `gorm:"type:varchar(50)"`
	Color     *string    `gorm:"type:varchar(16)"`
	IsDefault *bool      `gorm:"type:boolean;default:false"`
	IsActive  *bool      `gorm:"type:boolean;default:true"`
	IsOpen    *bool      `gorm:"type:boolean;default:true"`
	CreatedAt *time.Time `gorm:"type:timestamp;default:CURRENT_TIMESTAMP"`
	UpdatedAt *time.Time `gorm:"type:timestamp;default:CURRENT_TIMESTAMP"`
	Deleted   *bool      `gorm:"type:boolean;default:false"`

	// Связь: Status принадлежит Board
	Board *Board `gorm:"foreignKey:BoardID;references:ID;constraint:OnDelete:CASCADE"`

	// Связь: много задач могут иметь этот статус
	Tasks []Task `gorm:"foreignKey:StatusID;references:ID"`
}
