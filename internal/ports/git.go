package ports

import (
	"context"
	"time"

	models "emplacc-api/internal/domain"

	"github.com/google/uuid"
)

// ProviderCommit — сырой коммит от провайдера кода (до маппинга в доменную модель).
type ProviderCommit struct {
	SHA         string
	Message     string
	AuthorName  string
	AuthorEmail string
	AuthorLogin string
	URL         string
	CommittedAt time.Time
}

// CommitProvider — порт интеграции с хранилищем кода (граница гексагона).
// Реализации в internal/infra/git: GitHub, GitFlic, … — система остаётся
// независимой от конкретного хостинга.
type CommitProvider interface {
	Name() string
	FetchCommits(ctx context.Context, repo models.CodeRepository, since *time.Time) ([]ProviderCommit, error)
}

type GitRepository interface {
	CreateRepository(repo *models.CodeRepository) error
	GetRepository(id uuid.UUID) (*models.CodeRepository, error)
	ListRepositoriesByProject(projectID uuid.UUID) ([]models.CodeRepository, error)
	SoftDeleteRepository(id uuid.UUID) (bool, error)
	TouchRepositorySync(id uuid.UUID, at time.Time) error
	UpsertCommits(commits []models.Commit) (created int, err error)
	LinkCommitToTask(commitID uuid.UUID, taskID *uuid.UUID) (bool, error)
	ListCommitsByTask(taskID uuid.UUID, limit, offset int) ([]models.Commit, int64, error)
	ListCommitsByProject(projectID uuid.UUID, limit, offset int) ([]models.Commit, int64, error)
	ListCommitsByRepository(repoID uuid.UUID, limit, offset int) ([]models.Commit, int64, error)
}
