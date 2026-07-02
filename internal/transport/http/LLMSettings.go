package httpapi

import (
	"emplacc-api/internal/service"
	"net/http"
	"time"

	"github.com/labstack/echo/v4"
)

type LLMSettingsController struct {
	svc service.LLMSettingsService
}

func NewLLMSettingsController(svc service.LLMSettingsService) *LLMSettingsController {
	return &LLMSettingsController{svc: svc}
}

func RegisterLLMSettingsRoutes(e Router, svc service.LLMSettingsService, adminMw echo.MiddlewareFunc) {
	c := NewLLMSettingsController(svc)
	g := e.Group("/admin/llm-settings")
	g.GET("", c.Get, adminMw)
	g.PATCH("", c.Update, adminMw)
}

type LLMSettingsResponse struct {
	WebUIURL     string `json:"webui_url"`
	WebUIModel   string `json:"webui_model"`
	SystemPrompt string `json:"system_prompt"`
	HasToken     bool   `json:"has_token"` // токен не возвращаем, только факт наличия
	UpdatedAt    string `json:"updated_at,omitempty"`
	UpdatedBy    string `json:"updated_by,omitempty"`
}

type LLMSettingsUpdateRequest struct {
	WebUIURL     *string `json:"webui_url"`
	WebUIToken   *string `json:"webui_token"`
	WebUIModel   *string `json:"webui_model"`
	SystemPrompt *string `json:"system_prompt"`
}

// Get godoc
// @Summary Получить настройки LLM
// @Tags Admin
// @Produce json
// @Security BearerAuth
// @Router /admin/llm-settings [get]
func (c *LLMSettingsController) Get(ctx echo.Context) error {
	s, err := c.svc.Get()
	if err != nil {
		return ctx.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to load settings"})
	}
	if s == nil {
		// Нет записи — возвращаем пустые дефолты
		return ctx.JSON(http.StatusOK, LLMSettingsResponse{})
	}
	return ctx.JSON(http.StatusOK, LLMSettingsResponse{
		WebUIURL:     s.WebUIURL,
		WebUIModel:   s.WebUIModel,
		SystemPrompt: s.SystemPrompt,
		HasToken:     s.WebUIToken != "",
		UpdatedAt:    s.UpdatedAt.Format(time.RFC3339),
		UpdatedBy:    s.UpdatedBy,
	})
}

// Update godoc
// @Summary Обновить настройки LLM
// @Tags Admin
// @Accept json
// @Produce json
// @Security BearerAuth
// @Router /admin/llm-settings [patch]
func (c *LLMSettingsController) Update(ctx echo.Context) error {
	var req LLMSettingsUpdateRequest
	if err := ctx.Bind(&req); err != nil {
		return ctx.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request"})
	}

	updatedBy := ""
	if token, _ := ctx.Get("auth_token").(string); token != "" {
		if sub, err := extractSubFromJWT(token); err == nil {
			updatedBy = sub.String()
		}
	}

	if err := c.svc.Update(service.LLMSettingsUpdate{
		WebUIURL:     req.WebUIURL,
		WebUIToken:   req.WebUIToken,
		WebUIModel:   req.WebUIModel,
		SystemPrompt: req.SystemPrompt,
		UpdatedBy:    updatedBy,
	}); err != nil {
		return ctx.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to save settings"})
	}

	return ctx.JSON(http.StatusOK, map[string]string{"message": "settings updated"})
}
