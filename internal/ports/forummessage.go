package ports

import (
	models "emplacc-api/internal/domain"

	"github.com/google/uuid"
)

type ForumMessageRepository interface {
	GetAllForumMessages(limit, offset int) ([]models.ForumMessage, int64, error)
	GetForumMessagesByProblemId(problemID uuid.UUID, limit, offset int) ([]models.ForumMessage, int64, error)
	GetForumMessageById(messageID uuid.UUID) (*models.ForumMessage, error)
	CreateForumMessage(fm models.ForumMessage) error
	UpdateForumMessage(messageID uuid.UUID, updateData map[string]interface{}) (bool, error)
	DeleteForumMessage(messageID uuid.UUID) (bool, error)
}
