package ports

import (
	models "emplacc-api/internal/domain"

	"github.com/google/uuid"
)

type ProblemRepository interface {
	GetAllProblems(limit, offset int) ([]models.Problem, int64, error)
	GetProblemsByUserId(creatorUUID uuid.UUID, limit, offset int) ([]models.Problem, int64, error)
	GetProblemByID(problemId uuid.UUID) (*models.Problem, error)
	SearchProblems(query string, limit, offset int) ([]models.Problem, int64, error)
	CreateProblem(p models.Problem) error
	UpdateProblem(problemId uuid.UUID, updateData map[string]interface{}) (bool, error)
	DeleteProblem(problemId uuid.UUID) (bool, error)
}
