package httpapi

import (
	"net/http"
	"os"
	"runtime"

	"github.com/labstack/echo/v4"
)

// RegisterVersionRoutes публикует GET /version (и /v2/version через mountAPI).
// Эндпоинт публичный (исключён из AppAuthMiddleware) — версии не секрет, и панель
// «О системе» должна работать в т.ч. до входа.
func RegisterVersionRoutes(e Router) {
	e.GET("/version", func(c echo.Context) error {
		return c.JSON(http.StatusOK, versionInfo())
	})
}

// versionInfo собирает версии из env (проставляются в манифесте = теги образов).
// llm/mcp — best-effort: показываем только если соответствующий env задан.
func versionInfo() map[string]any {
	env := func(key, def string) string {
		if v := os.Getenv(key); v != "" {
			return v
		}
		return def
	}
	services := map[string]string{}
	if v := os.Getenv("LLM_VERSION"); v != "" {
		services["llm"] = v
	}
	if v := os.Getenv("MCP_VERSION"); v != "" {
		services["mcp"] = v
	}
	return map[string]any{
		"api":        env("APP_VERSION", "dev"),
		"build_time": os.Getenv("BUILD_TIME"),
		"go":         runtime.Version(),
		"services":   services,
	}
}
