package postgres

import (
	models "emplacc-api/internal/domain"
	"emplacc-api/internal/ports"
	"errors"

	"gorm.io/gorm"
)

type llmSettingsRepository struct {
	db *gorm.DB
}

func NewLLMSettingsRepository(db *gorm.DB) ports.LLMSettingsRepository {
	return &llmSettingsRepository{db: db}
}

func (r *llmSettingsRepository) sess() *gorm.DB {
	return r.db.Session(&gorm.Session{NewDB: true})
}

func (r *llmSettingsRepository) Get() (*models.LLMSettings, error) {
	var s models.LLMSettings
	err := r.sess().First(&s).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &s, nil
}

func (r *llmSettingsRepository) Save(s *models.LLMSettings) error {
	if s.ID == 0 {
		return r.sess().Create(s).Error
	}
	return r.sess().Save(s).Error
}
