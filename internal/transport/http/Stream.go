package httpapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"emplacc-api/internal/service"

	"github.com/labstack/echo/v4"
)

// RegisterStreamRoutes — SSE realtime под /v2. Эндпоинт исключён из AppAuthMiddleware
// (EventSource не умеет слать заголовки), поэтому авторизация по query-параметру token.
func RegisterStreamRoutes(e Router, hub *service.EventHub, sessionService service.SessionService, apiTokenService service.APITokenService) {
	e.GET("/v2/stream", func(c echo.Context) error {
		token := strings.TrimSpace(c.QueryParam("token"))
		if token == "" {
			return c.JSON(http.StatusUnauthorized, map[string]string{"error": "missing token"})
		}
		var userID string // для адресной доставки notification-событий
		switch {
		case strings.HasPrefix(token, "sess_"):
			v, err := sessionService.Validate(token)
			if err != nil || v.Expired {
				return c.JSON(http.StatusUnauthorized, map[string]string{"error": "session invalid"})
			}
			userID = v.UserID
		case strings.HasPrefix(token, "emplacc_"):
			u, err := apiTokenService.ValidateToken(token)
			if err != nil {
				return c.JSON(http.StatusUnauthorized, map[string]string{"error": "invalid token"})
			}
			userID = u.ID.String()
		default:
			return c.JSON(http.StatusUnauthorized, map[string]string{"error": "unknown token type"})
		}

		res := c.Response()
		res.Header().Set(echo.HeaderContentType, "text/event-stream")
		res.Header().Set("Cache-Control", "no-cache")
		res.Header().Set("Connection", "keep-alive")
		res.Header().Set("X-Accel-Buffering", "no") // не буферить за nginx
		res.WriteHeader(http.StatusOK)

		ch, cleanup := hub.Subscribe()
		defer cleanup()

		fmt.Fprint(res.Writer, "event: ready\ndata: {}\n\n")
		res.Flush()

		ticker := time.NewTicker(25 * time.Second)
		defer ticker.Stop()
		ctx := c.Request().Context()

		for {
			select {
			case <-ctx.Done():
				return nil
			case ev, ok := <-ch:
				if !ok {
					return nil
				}
				// notification-события адресные: шлём только их получателю.
				// Остальные (task/board/forum) — широковещательные сигналы инвалидации.
				if ev.Type == "notification" && ev.WorkItemID != "" && ev.WorkItemID != userID {
					continue
				}
				data, _ := json.Marshal(ev)
				// Фиксированное имя события — чтобы фронт ловил всё одним обработчиком; тип внутри data.
				fmt.Fprintf(res.Writer, "event: emplacc\ndata: %s\n\n", data)
				res.Flush()
			case <-ticker.C:
				fmt.Fprint(res.Writer, ": ping\n\n")
				res.Flush()
			}
		}
	})
}
