package ports

import (
	models "emplacc-api/internal/domain"

	"github.com/google/uuid"
)

type BoardRepository interface {
	GetAllBoards(limit, offset int) ([]models.Board, int64, error)
	GetBoardById(boardID uuid.UUID) (*models.Board, error)
	GetBoardByProjectId(projectID uuid.UUID) ([]models.Board, error)
	CreateBoardWithStatuses(board models.Board, statuses []models.Status) error
	UpdateBoard(boardID uuid.UUID, updateData map[string]interface{}) (bool, error)
	DeleteBoard(boardID uuid.UUID) (bool, error)
	GetBoardByProjectIdWithUsersAndProject(projectID uuid.UUID) ([]models.Board, error)
}
