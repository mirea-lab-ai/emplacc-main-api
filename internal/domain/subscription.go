package models

import (
	"time"

	"github.com/google/uuid"
)

func (Subscription) TableName() string { return "subscriptions" }

type Subscription struct {
	ID             uuid.UUID  `gorm:"type:uuid;primaryKey"`
	UserID         uuid.UUID  `gorm:"type:uuid;index"`
	SubscriptionId *uuid.UUID `gorm:"type:uuid"`
	TypeID         *int8      `gorm:"type:int8"`
	CreatedAt      *time.Time `gorm:"type:timestamp"`
	Deleted        *bool      `gorm:"type:boolean;default:false"`

	User *User `gorm:"foreignKey:UserID;references:ID;constraint:OnDelete:CASCADE"`
}
