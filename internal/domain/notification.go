package models

import (
	"time"

	"github.com/google/uuid"
)

func (Notification) TableName() string { return "notifications" }

// Notification — in-app уведомление для конкретного пользователя. Доставка realtime
// через SSE-сигнал (клиент дозапрашивает свой список); email/браузер — поверх этой модели.
type Notification struct {
	ID         uuid.UUID  `gorm:"type:uuid;primaryKey" json:"id"`
	UserID     uuid.UUID  `gorm:"type:uuid;index;not null" json:"user_id"`
	Type       string     `gorm:"size:50" json:"type"`
	Title      string     `gorm:"size:200" json:"title"`
	Body       string     `gorm:"type:text" json:"body,omitempty"`
	EntityType string     `gorm:"size:50" json:"entity_type,omitempty"`
	EntityID   *uuid.UUID `gorm:"type:uuid" json:"entity_id,omitempty"`
	Read       bool       `gorm:"index;default:false" json:"read"`
	CreatedAt  time.Time  `gorm:"type:timestamp;index" json:"created_at"`
}
