package models

import "time"

func (LLMSettings) TableName() string { return "llm_settings" }

type LLMSettings struct {
	ID           uint   `gorm:"primaryKey;autoIncrement"`
	WebUIURL     string `gorm:"size:500"`
	WebUIToken   string `gorm:"size:500"`
	WebUIModel   string `gorm:"size:100"`
	SystemPrompt string `gorm:"type:text"`
	UpdatedAt    time.Time
	UpdatedBy    string `gorm:"size:100"`
}
