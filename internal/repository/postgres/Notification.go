package postgres

import (
	models "emplacc-api/internal/domain"
	"emplacc-api/internal/ports"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type notificationRepository struct {
	db *gorm.DB
}

func NewNotificationRepository(db *gorm.DB) ports.NotificationRepository {
	return &notificationRepository{db: db}
}

func (r *notificationRepository) sess() *gorm.DB {
	return r.db.Session(&gorm.Session{NewDB: true})
}

func (r *notificationRepository) Create(n *models.Notification) error {
	return r.sess().Create(n).Error
}

func (r *notificationRepository) ListByUser(userID uuid.UUID, limit, offset int, onlyUnread bool) ([]models.Notification, int64, error) {
	q := r.sess().Model(&models.Notification{}).Where("user_id = ?", userID)
	if onlyUnread {
		q = q.Where("read = ?", false)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var out []models.Notification
	err := q.Order("created_at DESC").Limit(limit).Offset(offset).Find(&out).Error
	return out, total, err
}

func (r *notificationRepository) UnreadCount(userID uuid.UUID) (int64, error) {
	var n int64
	err := r.sess().Model(&models.Notification{}).Where("user_id = ? AND read = ?", userID, false).Count(&n).Error
	return n, err
}

func (r *notificationRepository) MarkRead(id, userID uuid.UUID) (bool, error) {
	res := r.sess().Model(&models.Notification{}).
		Where("id = ? AND user_id = ?", id, userID).Update("read", true)
	if res.Error != nil {
		return false, res.Error
	}
	return res.RowsAffected > 0, nil
}

func (r *notificationRepository) MarkAllRead(userID uuid.UUID) error {
	return r.sess().Model(&models.Notification{}).
		Where("user_id = ? AND read = ?", userID, false).Update("read", true).Error
}
