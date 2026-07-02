package httpapi

import (
	"emplacc-api/internal/service"
	"net/http"
	"strconv"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
)

type NotificationController struct {
	svc service.NotificationService
}

func NewNotificationController(svc service.NotificationService) *NotificationController {
	return &NotificationController{svc: svc}
}

func RegisterNotificationRoutes(e Router, svc service.NotificationService, employeeMw echo.MiddlewareFunc) {
	c := NewNotificationController(svc)
	g := e.Group("/notification", employeeMw)
	g.GET("", c.List)
	g.GET("/unread-count", c.UnreadCount)
	g.POST("/:id/read", c.MarkRead)
	g.POST("/read-all", c.MarkAllRead)
	g.GET("/preferences", c.GetPreferences)
	g.PUT("/preferences", c.SetPreferences)
}

func (nc *NotificationController) GetPreferences(c echo.Context) error {
	userID, ok := nc.caller(c)
	if !ok {
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
	}
	enabled, err := nc.svc.GetEmailPref(userID)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Ошибка"})
	}
	return c.JSON(http.StatusOK, map[string]any{"email_notifications": enabled})
}

func (nc *NotificationController) SetPreferences(c echo.Context) error {
	userID, ok := nc.caller(c)
	if !ok {
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
	}
	var req struct {
		EmailNotifications *bool `json:"email_notifications"`
	}
	if err := c.Bind(&req); err != nil || req.EmailNotifications == nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "email_notifications обязателен"})
	}
	if err := nc.svc.SetEmailPref(userID, *req.EmailNotifications); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Ошибка сохранения"})
	}
	return c.JSON(http.StatusOK, map[string]any{"email_notifications": *req.EmailNotifications})
}

func (nc *NotificationController) caller(c echo.Context) (uuid.UUID, bool) {
	idStr, _ := c.Get("user_id").(string)
	id, err := uuid.Parse(idStr)
	if err != nil {
		return uuid.Nil, false
	}
	return id, true
}

func (nc *NotificationController) List(c echo.Context) error {
	userID, ok := nc.caller(c)
	if !ok {
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
	}
	page, _ := strconv.Atoi(c.QueryParam("page"))
	size, _ := strconv.Atoi(c.QueryParam("pagesize"))
	onlyUnread := c.QueryParam("unread") == "true"

	items, total, err := nc.svc.List(userID, page, size, onlyUnread)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Ошибка при получении уведомлений"})
	}
	return c.JSON(http.StatusOK, map[string]any{"notifications": items, "total_count": total})
}

func (nc *NotificationController) UnreadCount(c echo.Context) error {
	userID, ok := nc.caller(c)
	if !ok {
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
	}
	count, err := nc.svc.UnreadCount(userID)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Ошибка"})
	}
	return c.JSON(http.StatusOK, map[string]any{"count": count})
}

func (nc *NotificationController) MarkRead(c echo.Context) error {
	userID, ok := nc.caller(c)
	if !ok {
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
	}
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Некорректный идентификатор"})
	}
	if err := nc.svc.MarkRead(id, userID); err != nil {
		if err.Error() == "notification not found" {
			return c.JSON(http.StatusNotFound, map[string]string{"error": "Уведомление не найдено"})
		}
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Ошибка"})
	}
	return c.JSON(http.StatusOK, map[string]string{"message": "ok"})
}

func (nc *NotificationController) MarkAllRead(c echo.Context) error {
	userID, ok := nc.caller(c)
	if !ok {
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
	}
	if err := nc.svc.MarkAllRead(userID); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Ошибка"})
	}
	return c.JSON(http.StatusOK, map[string]string{"message": "ok"})
}
