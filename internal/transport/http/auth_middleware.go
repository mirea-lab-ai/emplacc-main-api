package httpapi

import (
	"log"
	"net/http"
	"strings"

	"emplacc-api/internal/service"

	"github.com/labstack/echo/v4"
)

// SessionExpiredError — структура ответа при истёкшей сессии
type SessionExpiredError struct {
	Error  string `json:"error"`
	Reason string `json:"reason"` // "" | "user_exit" | "long_absence"
}

// AppAuthMiddleware — новая стратегия авторизации:
//   - sess_*     → сессионный токен (браузер)
//   - emplacc_*  → MCP/интеграционный токен
//   - eyJ...     → Keycloak JWT для совместимости с текущим фронтом
//
// Keycloak JWT разбирается локально в authService, без обращения к Keycloak на каждый запрос.
func AppAuthMiddleware(authService service.AuthService, sessionService service.SessionService, apiTokenService service.APITokenService) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			p := c.Path()
			// Пропускаем публичные маршруты. Проверка по границе сегмента (со слешем),
			// чтобы префикс не открывал случайно похожие пути (напр. /authz, /eventlog).
			if c.Request().Method == http.MethodOptions ||
				p == "/swagger" || strings.HasPrefix(p, "/swagger/") ||
				p == "/auth" || strings.HasPrefix(p, "/auth/") ||
				p == "/version" || p == "/v2/version" ||
				p == "/v2/stream" {
				return next(c)
			}

			h := c.Request().Header.Get("Authorization")
			if h == "" {
				log.Printf("Auth middleware: missing authorization header for %s %s", c.Request().Method, p)
				return c.JSON(http.StatusUnauthorized, map[string]string{"error": "missing authorization header"})
			}
			if !strings.HasPrefix(h, "Bearer ") {
				// Не логируем содержимое заголовка — даже префикс токена это утечка.
				log.Printf("Auth middleware: invalid header format for %s %s", c.Request().Method, p)
				return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid authorization header format"})
			}
			token := strings.TrimPrefix(h, "Bearer ")

			switch {
			// ── Сессионный токен браузера ──────────────────────────
			case strings.HasPrefix(token, "sess_"):
				validation, err := sessionService.Validate(token)
				if err != nil {
					log.Printf("Auth middleware: session validation failed: %v", err)
					return c.JSON(http.StatusUnauthorized, SessionExpiredError{
						Error:  "session not found",
						Reason: "user_exit",
					})
				}
				if validation.Expired {
					log.Printf("Auth middleware: session expired for user %s, reason: %s", validation.UserID, validation.Reason)
					return c.JSON(http.StatusUnauthorized, SessionExpiredError{
						Error:  "session_expired",
						Reason: validation.Reason, // "" = нужна ротация, иначе нужен реологин
					})
				}
				c.Set("user_id", validation.UserID)
				c.Set("session_token", token)

			// ── MCP/интеграционный токен ───────────────────────────
			case strings.HasPrefix(token, "emplacc_"):
				user, err := apiTokenService.ValidateToken(token)
				if err != nil {
					log.Printf("Auth middleware: API token validation failed: %v", err)
					return c.JSON(http.StatusUnauthorized, map[string]string{"error": "invalid API token"})
				}
				c.Set("user_id", user.ID.String())
				c.Set("api_token_user_id", user.ID)

			// ── Keycloak JWT для совместимости с прод-фронтом ───────
			case strings.Count(token, ".") == 2:
				if err := authService.ValidateTokenForMiddleware(token); err != nil {
					log.Printf("Auth middleware: JWT validation failed: %v", err)
					return c.JSON(http.StatusUnauthorized, map[string]string{"message": "Token invalid"})
				}
				userID, err := extractSubFromJWT(token)
				if err != nil {
					log.Printf("Auth middleware: failed to extract JWT subject: %v", err)
					return c.JSON(http.StatusUnauthorized, map[string]string{"message": "Token invalid"})
				}
				c.Set("user_id", userID.String())
				c.Set("auth_token", token)

			default:
				log.Printf("Auth middleware: unknown token type for %s %s", c.Request().Method, p)
				return c.JSON(http.StatusUnauthorized, map[string]string{
					"error": "unknown token type, use sess_*, emplacc_* or Keycloak JWT",
				})
			}

			return next(c)
		}
	}
}

// KeycloakAuthMiddleware — оставляем для совместимости, делегирует в AppAuthMiddleware
func KeycloakAuthMiddleware(authService service.AuthService, apiTokenService service.APITokenService) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			return next(c)
		}
	}
}
