package models

import (
	"time"

	"github.com/google/uuid"
)

func (UserAlias) TableName() string { return "user_aliases" }

// UserAlias — приватный псевдоним: владелец (Owner) задаёт строку Alias,
// указывающую на целевого пользователя (Target). Псевдонимы приватны (видны/используются
// только владельцем при наборе); уникальность пары (owner_user_id, alias) — другой
// пользователь может занять ту же строку на кого угодно.
type UserAlias struct {
	ID           uuid.UUID `gorm:"type:uuid;primaryKey"`
	OwnerUserID  uuid.UUID `gorm:"type:uuid;index;index:idx_owner_alias,unique"`
	Alias        string    `gorm:"size:50;index:idx_owner_alias,unique"`
	TargetUserID uuid.UUID `gorm:"type:uuid;index"`
	CreatedAt    time.Time `gorm:"type:timestamp"`

	Target *User `gorm:"foreignKey:TargetUserID;references:ID"`
}
