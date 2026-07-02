package httpapi

import (
	"emplacc-api/internal/service"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
)

type APITokenController struct {
	svc service.APITokenService
}

func NewAPITokenController(svc service.APITokenService) *APITokenController {
	return &APITokenController{svc: svc}
}

func RegisterAPITokenRoutes(e Router, svc service.APITokenService) {
	c := NewAPITokenController(svc)
	g := e.Group("/token")
	g.POST("", c.Create)
	g.GET("", c.List)
	g.DELETE("/:id", c.Revoke)
}

// getUserIDFromContext читает user_id из контекста (ставится AppAuthMiddleware).
// Работает с сессионными (sess_*) и MCP токенами (emplacc_*).
func getUserIDFromContext(ctx echo.Context) (uuid.UUID, error) {
	// MCP токен — user_id как uuid.UUID
	if id, ok := ctx.Get("api_token_user_id").(uuid.UUID); ok && id != uuid.Nil {
		return id, nil
	}
	// Сессионный токен — user_id как string
	if idStr, ok := ctx.Get("user_id").(string); ok && idStr != "" {
		return uuid.Parse(idStr)
	}
	return uuid.Nil, nil
}

type createTokenRequest struct {
	Name      string  `json:"name"`
	ExpiresIn *string `json:"expires_in"` // "30d", "90d", "365d"
}

func (c *APITokenController) Create(ctx echo.Context) error {
	userID, err := getUserIDFromContext(ctx)
	if err != nil || userID == uuid.Nil {
		return ctx.JSON(http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
	}

	var req createTokenRequest
	if err := ctx.Bind(&req); err != nil {
		return ctx.JSON(http.StatusBadRequest, map[string]string{"error": "bad request"})
	}
	if strings.TrimSpace(req.Name) == "" {
		req.Name = "Без названия"
	}

	var expiresIn *time.Duration
	if req.ExpiresIn != nil {
		switch *req.ExpiresIn {
		case "30d":
			d := 30 * 24 * time.Hour
			expiresIn = &d
		case "90d":
			d := 90 * 24 * time.Hour
			expiresIn = &d
		case "365d":
			d := 365 * 24 * time.Hour
			expiresIn = &d
		}
	}

	result, err := c.svc.Create(userID, req.Name, expiresIn)
	if err != nil {
		log.Printf("create token: %v", err)
		return ctx.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to create token"})
	}
	return ctx.JSON(http.StatusCreated, result)
}

func (c *APITokenController) List(ctx echo.Context) error {
	userID, err := getUserIDFromContext(ctx)
	if err != nil || userID == uuid.Nil {
		return ctx.JSON(http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
	}

	tokens, err := c.svc.ListByUser(userID)
	if err != nil {
		return ctx.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to list tokens"})
	}
	return ctx.JSON(http.StatusOK, tokens)
}

func (c *APITokenController) Revoke(ctx echo.Context) error {
	userID, err := getUserIDFromContext(ctx)
	if err != nil || userID == uuid.Nil {
		return ctx.JSON(http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
	}

	tokenID, err := uuid.Parse(ctx.Param("id"))
	if err != nil {
		return ctx.JSON(http.StatusBadRequest, map[string]string{"error": "invalid token id"})
	}

	if err := c.svc.Revoke(userID, tokenID); err != nil {
		return ctx.JSON(http.StatusNotFound, map[string]string{"error": "token not found"})
	}
	return ctx.JSON(http.StatusOK, map[string]string{"message": "token revoked"})
}
