package httpapi

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"emplacc-api/internal/ports"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
)

// extractSubFromJWT — нужен только для CreateSession и совместимости
func extractSubFromJWT(token string) (uuid.UUID, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return uuid.Nil, fmt.Errorf("malformed JWT")
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return uuid.Nil, err
	}
	var claims struct {
		Sub string `json:"sub"`
	}
	if err := json.Unmarshal(raw, &claims); err != nil {
		return uuid.Nil, err
	}
	return uuid.Parse(claims.Sub)
}

// RequireRoles — читает user_id из контекста (установлен AppAuthMiddleware).
// Работает с сессионными токенами (sess_*) и MCP токенами (emplacc_*).
func RequireRoles(roleRepo ports.RoleRepository, roles ...string) echo.MiddlewareFunc {
	allowed := make(map[string]bool, len(roles))
	for _, r := range roles {
		allowed[strings.ToLower(r)] = true
	}
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			var userID uuid.UUID

			// MCP токен — user_id как uuid.UUID
			if id, ok := c.Get("api_token_user_id").(uuid.UUID); ok && id != uuid.Nil {
				userID = id
			} else if idStr, ok := c.Get("user_id").(string); ok && idStr != "" {
				// Сессионный токен — user_id как string
				parsed, err := uuid.Parse(idStr)
				if err != nil {
					return c.JSON(http.StatusUnauthorized, map[string]string{"error": "invalid user_id in context"})
				}
				userID = parsed
			} else {
				return c.JSON(http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
			}

			role, err := roleRepo.GetRoleByUserId(userID)
			if err != nil {
				return c.JSON(http.StatusForbidden, map[string]string{"error": "access denied: no role assigned"})
			}
			roleName := ""
			if role.Name != nil {
				roleName = *role.Name
			}
			if !allowed[strings.ToLower(roleName)] {
				return c.JSON(http.StatusForbidden, map[string]string{
					"error": fmt.Sprintf("access denied: role '%s' is not permitted", roleName),
				})
			}
			c.Set("user_role", strings.ToLower(roleName))
			return next(c)
		}
	}
}
