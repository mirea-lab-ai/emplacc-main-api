package models

import (
	"time"

	"github.com/google/uuid"
)

const (
	AgentInboxAckStateDelivered = "delivered"
	AgentInboxAckStateRead      = "read"
	AgentInboxAckStateHandled   = "handled"
)

type AgentInboxItem struct {
	ID             uuid.UUID  `gorm:"type:uuid;primaryKey" json:"id"`
	RecipientID    uuid.UUID  `gorm:"type:uuid;not null;index:idx_agent_inbox_recipient_state" json:"recipient_id"`
	WorkItemID     *uuid.UUID `gorm:"type:uuid;index" json:"work_item_id,omitempty"`
	Kind           string     `gorm:"size:64;not null;index" json:"kind"`
	Source         string     `gorm:"size:100;not null;index" json:"source"`
	Title          string     `gorm:"type:text;not null" json:"title"`
	Summary        string     `gorm:"type:text" json:"summary"`
	Priority       int16      `gorm:"not null;default:1;index" json:"priority"`
	ActionRequired bool       `gorm:"not null;default:false;index" json:"action_required"`
	AckState       string     `gorm:"size:32;not null;default:delivered;index:idx_agent_inbox_recipient_state" json:"ack_state"`
	CreatedAt      time.Time  `gorm:"type:timestamp;not null;index" json:"created_at"`
	UpdatedAt      time.Time  `gorm:"type:timestamp;not null" json:"updated_at"`
}

func (AgentInboxItem) TableName() string { return "agent_inbox_items" }
