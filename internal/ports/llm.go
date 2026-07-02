package ports

import models "emplacc-api/internal/domain"

// LLMSettingsRepository — driven-порт для единственной строки настроек LLM.
type LLMSettingsRepository interface {
	// Get returns the settings row, or (nil, nil) if none exists yet.
	Get() (*models.LLMSettings, error)
	// Save creates the row when ID==0, otherwise updates it.
	Save(s *models.LLMSettings) error
}
