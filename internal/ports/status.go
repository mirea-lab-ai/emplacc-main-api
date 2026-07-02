package ports

import (
	models "emplacc-api/internal/domain"

	"github.com/google/uuid"
)

type StatusRepository interface {
	GetAllStatuses(limit, offset int) ([]models.Status, int64, error)
	GetStatusesByBoardId(boardID uuid.UUID) ([]models.Status, error)
	GetStatusByID(statusID uuid.UUID) (*models.Status, error)
	BoardExists(boardID uuid.UUID) (bool, error)
	CreateStatus(row models.Status) error
	UpdateStatus(statusID uuid.UUID, updateData map[string]interface{}) (bool, error)
	DeleteStatus(statusID uuid.UUID) (bool, error)
}
