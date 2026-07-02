package ports

import (
	models "emplacc-api/internal/domain"

	"github.com/google/uuid"
)

// NotificationRepository — driven-порт хранения уведомлений.
type NotificationRepository interface {
	Create(n *models.Notification) error
	ListByUser(userID uuid.UUID, limit, offset int, onlyUnread bool) ([]models.Notification, int64, error)
	UnreadCount(userID uuid.UUID) (int64, error)
	MarkRead(id, userID uuid.UUID) (bool, error)
	MarkAllRead(userID uuid.UUID) error
}

// Notifier — driving-порт для других сервисов: создать уведомление пользователю.
// Реализуется NotificationService; передаётся как опциональная зависимость
// (nil → no-op), чтобы сервисы не зависели жёстко от подсистемы уведомлений.
type Notifier interface {
	Notify(userID uuid.UUID, typ, title, body, entityType string, entityID *uuid.UUID)
}
