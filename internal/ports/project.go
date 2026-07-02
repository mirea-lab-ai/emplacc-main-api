package ports

import (
	models "emplacc-api/internal/domain"

	"github.com/google/uuid"
)

type ProjectRepository interface {
	GetAllProjects(limit, offset int) ([]models.Project, int64, error)
	GetProjectByID(projectID uuid.UUID) (*models.Project, error)
	GetProjectsByUser(userID uuid.UUID) ([]models.Project, error)
	GetTeamProjects(teamID uuid.UUID) ([]models.ProjectTeam, error)
	GetProjectsByIDs(projectIDs []uuid.UUID) ([]models.Project, error)
	CreateProjectWithBoardAndStatuses(project models.Project, board models.Board, statuses []models.Status) error
	UpdateProject(projectID uuid.UUID, updateData map[string]interface{}) (bool, error)
	DeleteProject(projectID uuid.UUID) (bool, error)
	SearchProjects(query, userID string, limit, offset int) ([]models.Project, int64, error)
}
