package postgres

import (
	"context"
	"errors"
	"time"

	models "emplacc-api/internal/domain"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

func (r *conveyorRepository) CreateAgentInboxItem(ctx context.Context, item *models.AgentInboxItem) error {
	return r.db.WithContext(ctx).Create(item).Error
}

func (r *conveyorRepository) ListAgentInboxItems(ctx context.Context, recipientID uuid.UUID) ([]models.AgentInboxItem, error) {
	var items []models.AgentInboxItem
	err := r.db.WithContext(ctx).
		Where("recipient_id = ?", recipientID).
		Order("priority DESC, created_at ASC").
		Find(&items).Error
	return items, err
}

func (r *conveyorRepository) GetAgentInboxItem(ctx context.Context, id uuid.UUID) (*models.AgentInboxItem, error) {
	var item models.AgentInboxItem
	if err := r.db.WithContext(ctx).First(&item, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, models.ErrConveyorNotFound
		}
		return nil, err
	}
	return &item, nil
}

func (r *conveyorRepository) UpdateAgentInboxItemAck(ctx context.Context, itemID uuid.UUID, recipientID uuid.UUID, state string, at time.Time) error {
	res := r.db.WithContext(ctx).Model(&models.AgentInboxItem{}).
		Where("id = ? AND recipient_id = ?", itemID, recipientID).
		Updates(map[string]any{"ack_state": state, "updated_at": at})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return models.ErrConveyorNotFound
	}
	return nil
}
