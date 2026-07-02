package service

import (
	models "emplacc-api/internal/domain"
	"emplacc-api/internal/ports"
	"time"
)

// LLMSettingsUpdate — частичное обновление настроек LLM (nil-поля не трогаем).
type LLMSettingsUpdate struct {
	WebUIURL     *string
	WebUIToken   *string
	WebUIModel   *string
	SystemPrompt *string
	UpdatedBy    string
}

type LLMSettingsService interface {
	// Get returns current settings, or (nil, nil) if none configured yet.
	Get() (*models.LLMSettings, error)
	Update(req LLMSettingsUpdate) error
}

type llmSettingsService struct {
	repo ports.LLMSettingsRepository
}

func NewLLMSettingsService(repo ports.LLMSettingsRepository) LLMSettingsService {
	return &llmSettingsService{repo: repo}
}

func (s *llmSettingsService) Get() (*models.LLMSettings, error) {
	return s.repo.Get()
}

func (s *llmSettingsService) Update(req LLMSettingsUpdate) error {
	cur, err := s.repo.Get()
	if err != nil {
		return err
	}
	if cur == nil {
		cur = &models.LLMSettings{}
	}
	if req.WebUIURL != nil {
		cur.WebUIURL = *req.WebUIURL
	}
	if req.WebUIToken != nil && *req.WebUIToken != "" {
		cur.WebUIToken = *req.WebUIToken
	}
	if req.WebUIModel != nil {
		cur.WebUIModel = *req.WebUIModel
	}
	if req.SystemPrompt != nil {
		cur.SystemPrompt = *req.SystemPrompt
	}
	cur.UpdatedAt = time.Now()
	cur.UpdatedBy = req.UpdatedBy
	return s.repo.Save(cur)
}
