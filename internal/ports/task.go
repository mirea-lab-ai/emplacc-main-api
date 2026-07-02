package ports

import (
	"time"

	models "emplacc-api/internal/domain"

	"github.com/google/uuid"
)

type TaskRepository interface {
	GetAllTasks(limit, offset int) ([]models.Task, int64, error)
	GetTaskByID(taskID uuid.UUID) (*models.Task, error)
	GetTasksByProjectID(projectID uuid.UUID, limit, offset int) ([]models.Task, int64, error)
	CreateTask(task models.Task) error
	UpdateTask(taskID uuid.UUID, updates map[string]interface{}) (bool, error)
	DeleteTask(taskID uuid.UUID) (bool, error)
	GetTasksByUserId(userID uuid.UUID, limit, offset int) ([]models.Task, int64, error)
	GetStatusByID(statusID uuid.UUID) (*models.Status, error)
	GetStatusesByBoardID(boardID uuid.UUID) ([]models.Status, error)
	UpdateTaskStatus(taskID uuid.UUID, toStatusID uuid.UUID, updatedAt time.Time) error
	UserExists(userID uuid.UUID) (bool, error)
	StatusExists(statusID uuid.UUID) (bool, error)
	GetTasksByUserIDAndProjectID(userID, projectID uuid.UUID, limit, offset int) ([]models.Task, int64, error)
	GetActiveTasksByUserId(userID uuid.UUID, limit, offset int) ([]models.Task, int64, error)
	GetTaskBoardAndProjectIDs(taskID uuid.UUID) (boardId, projectId uuid.UUID, err error)
	GetAllActiveTasks() ([]models.Task, error)
	SearchTasks(query, userID string, limit, offset int) ([]models.Task, int64, error)
}
