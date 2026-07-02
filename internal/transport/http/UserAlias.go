package httpapi

import (
	"log"
	"net/http"
	"strings"

	"emplacc-api/internal/service"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
)

type UserAliasController struct {
	svc            service.UserAliasService
	freshAvatarURL func(string) string
}

func NewUserAliasController(svc service.UserAliasService, freshAvatarURL func(string) string) *UserAliasController {
	return &UserAliasController{svc: svc, freshAvatarURL: freshAvatarURL}
}

// RegisterUserAliasRoutes — приватные псевдонимы текущего пользователя (employee+).
func RegisterUserAliasRoutes(e Router, svc service.UserAliasService, freshAvatarURL func(string) string, employeeMw echo.MiddlewareFunc) {
	c := NewUserAliasController(svc, freshAvatarURL)
	g := e.Group("/alias", employeeMw)
	g.GET("", c.List)
	g.POST("", c.Create)
	g.DELETE("/:id", c.Delete)
}

func aliasCallerID(ctx echo.Context) (uuid.UUID, bool) {
	idStr, _ := ctx.Get("user_id").(string)
	id, err := uuid.Parse(idStr)
	if err != nil {
		return uuid.Nil, false
	}
	return id, true
}

type aliasResponse struct {
	ID              string `json:"id"`
	Alias           string `json:"alias"`
	TargetUserID    string `json:"target_user_id"`
	TargetName      string `json:"target_name,omitempty"`
	TargetAvatarURL string `json:"target_avatar_url,omitempty"`
}

// List godoc
// @Summary Мои псевдонимы
// @Tags Aliases
// @Produce json
// @Security BearerAuth
// @Success 200 {object} map[string]interface{}
// @Router /alias [get]
func (c *UserAliasController) List(ctx echo.Context) error {
	owner, ok := aliasCallerID(ctx)
	if !ok {
		return ctx.JSON(http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
	}
	items, err := c.svc.List(owner)
	if err != nil {
		log.Printf("alias list error: %v", err)
		return ctx.JSON(http.StatusInternalServerError, map[string]string{"error": "ошибка получения псевдонимов"})
	}
	out := make([]aliasResponse, 0, len(items))
	for _, a := range items {
		r := aliasResponse{ID: a.ID.String(), Alias: a.Alias, TargetUserID: a.TargetUserID.String()}
		if a.Target != nil {
			r.TargetName = strings.TrimSpace(a.Target.FirstName + " " + a.Target.LastName)
			r.TargetAvatarURL = c.freshAvatarURL(a.Target.AvatarURL)
		}
		out = append(out, r)
	}
	return ctx.JSON(http.StatusOK, map[string]interface{}{"aliases": out})
}

type createAliasRequest struct {
	Alias        string `json:"alias"`
	TargetUserID string `json:"target_user_id"`
}

// Create godoc
// @Summary Создать псевдоним
// @Tags Aliases
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param body body createAliasRequest true "alias + target_user_id"
// @Success 201 {object} aliasResponse
// @Router /alias [post]
func (c *UserAliasController) Create(ctx echo.Context) error {
	owner, ok := aliasCallerID(ctx)
	if !ok {
		return ctx.JSON(http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
	}
	var req createAliasRequest
	if err := ctx.Bind(&req); err != nil {
		return ctx.JSON(http.StatusBadRequest, map[string]string{"error": "некорректный запрос"})
	}
	target, err := uuid.Parse(req.TargetUserID)
	if err != nil {
		return ctx.JSON(http.StatusBadRequest, map[string]string{"error": "некорректный target_user_id"})
	}
	a, err := c.svc.Create(owner, target, req.Alias)
	if err != nil {
		return ctx.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
	}
	return ctx.JSON(http.StatusCreated, aliasResponse{ID: a.ID.String(), Alias: a.Alias, TargetUserID: a.TargetUserID.String()})
}

// Delete godoc
// @Summary Удалить псевдоним
// @Tags Aliases
// @Produce json
// @Security BearerAuth
// @Param id path string true "alias id"
// @Success 200 {object} map[string]bool
// @Router /alias/{id} [delete]
func (c *UserAliasController) Delete(ctx echo.Context) error {
	owner, ok := aliasCallerID(ctx)
	if !ok {
		return ctx.JSON(http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
	}
	id, err := uuid.Parse(ctx.Param("id"))
	if err != nil {
		return ctx.JSON(http.StatusBadRequest, map[string]string{"error": "некорректный id"})
	}
	deleted, err := c.svc.Delete(owner, id)
	if err != nil {
		return ctx.JSON(http.StatusInternalServerError, map[string]string{"error": "ошибка удаления"})
	}
	if !deleted {
		return ctx.JSON(http.StatusNotFound, map[string]string{"error": "не найдено"})
	}
	return ctx.JSON(http.StatusOK, map[string]bool{"deleted": true})
}
