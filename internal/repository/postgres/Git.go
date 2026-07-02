package postgres

import (
	models "emplacc-api/internal/domain"
	"emplacc-api/internal/ports"
	"errors"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type gitRepository struct {
	db *gorm.DB
}

func NewGitRepository(db *gorm.DB) ports.GitRepository {
	return &gitRepository{db: db}
}

func (r *gitRepository) sess() *gorm.DB {
	return r.db.Session(&gorm.Session{NewDB: true})
}

func (r *gitRepository) CreateRepository(repo *models.CodeRepository) error {
	return r.sess().Create(repo).Error
}

func (r *gitRepository) GetRepository(id uuid.UUID) (*models.CodeRepository, error) {
	var repo models.CodeRepository
	if err := r.sess().Where("id = ? AND deleted = ?", id, false).First(&repo).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("repository not found")
		}
		return nil, err
	}
	return &repo, nil
}

func (r *gitRepository) ListRepositoriesByProject(projectID uuid.UUID) ([]models.CodeRepository, error) {
	var repos []models.CodeRepository
	err := r.sess().Where("project_id = ? AND deleted = ?", projectID, false).Order("created_at ASC").Find(&repos).Error
	return repos, err
}

func (r *gitRepository) SoftDeleteRepository(id uuid.UUID) (bool, error) {
	res := r.sess().Model(&models.CodeRepository{}).Where("id = ? AND deleted = ?", id, false).Update("deleted", true)
	if res.Error != nil {
		return false, res.Error
	}
	return res.RowsAffected > 0, nil
}

func (r *gitRepository) TouchRepositorySync(id uuid.UUID, at time.Time) error {
	return r.sess().Model(&models.CodeRepository{}).Where("id = ?", id).Update("last_sync_at", at).Error
}

// UpsertCommits — идемпотентно по (repository_id, sha): дубликаты пропускаем.
func (r *gitRepository) UpsertCommits(commits []models.Commit) (int, error) {
	if len(commits) == 0 {
		return 0, nil
	}
	res := r.sess().Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "repository_id"}, {Name: "sha"}},
		DoNothing: true,
	}).Create(&commits)
	if res.Error != nil {
		return 0, res.Error
	}
	return int(res.RowsAffected), nil
}

func (r *gitRepository) LinkCommitToTask(commitID uuid.UUID, taskID *uuid.UUID) (bool, error) {
	res := r.sess().Model(&models.Commit{}).Where("id = ?", commitID).Update("task_id", taskID)
	if res.Error != nil {
		return false, res.Error
	}
	return res.RowsAffected > 0, nil
}

func (r *gitRepository) ListCommitsByTask(taskID uuid.UUID, limit, offset int) ([]models.Commit, int64, error) {
	var total int64
	q := r.sess().Model(&models.Commit{}).Where("task_id = ?", taskID)
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var commits []models.Commit
	err := r.sess().Where("task_id = ?", taskID).Order("committed_at DESC").Limit(limit).Offset(offset).Find(&commits).Error
	return commits, total, err
}

func (r *gitRepository) ListCommitsByProject(projectID uuid.UUID, limit, offset int) ([]models.Commit, int64, error) {
	base := r.sess().Model(&models.Commit{}).
		Joins("JOIN code_repositories ON code_repositories.id = commits.repository_id").
		Where("code_repositories.project_id = ? AND code_repositories.deleted = ?", projectID, false)

	var total int64
	if err := base.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var commits []models.Commit
	err := r.sess().
		Joins("JOIN code_repositories ON code_repositories.id = commits.repository_id").
		Where("code_repositories.project_id = ? AND code_repositories.deleted = ?", projectID, false).
		Order("commits.committed_at DESC").Limit(limit).Offset(offset).Find(&commits).Error
	return commits, total, err
}

func (r *gitRepository) ListCommitsByRepository(repoID uuid.UUID, limit, offset int) ([]models.Commit, int64, error) {
	var total int64
	if err := r.sess().Model(&models.Commit{}).Where("repository_id = ?", repoID).Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var commits []models.Commit
	err := r.sess().Where("repository_id = ?", repoID).Order("committed_at DESC").Limit(limit).Offset(offset).Find(&commits).Error
	return commits, total, err
}
