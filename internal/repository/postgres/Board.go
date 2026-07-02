package postgres

import (
	models "emplacc-api/internal/domain"
	"emplacc-api/internal/ports"
	"errors"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type boardRepository struct {
	db *gorm.DB
}

func NewBoardRepository(db *gorm.DB) ports.BoardRepository {
	return &boardRepository{
		db: db,
	}
}

func (r *boardRepository) GetAllBoards(limit, offset int) ([]models.Board, int64, error) {
	var total int64
	if err := r.db.Session(&gorm.Session{NewDB: true}).Model(&models.Board{}).
		Where("deleted = ?", false).
		Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var boards []models.Board
	if err := r.db.Session(&gorm.Session{NewDB: true}).
		Where("deleted = ?", false).
		Order("created_at DESC NULLS LAST").
		Limit(limit).Offset(offset).
		Preload("Statuses", func(db *gorm.DB) *gorm.DB {
			return db.Session(&gorm.Session{}).Where("deleted = ?", false).Order("sort_order ASC")
		}).
		Preload("Statuses.Tasks", "deleted = ?", false).
		Find(&boards).Error; err != nil {
		return nil, 0, err
	}

	return boards, total, nil
}

func (r *boardRepository) GetBoardById(boardID uuid.UUID) (*models.Board, error) {
	var board models.Board
	if err := r.db.Session(&gorm.Session{NewDB: true}).
		Where("id = ? AND deleted = ?", boardID, false).
		Preload("Statuses", func(db *gorm.DB) *gorm.DB {
			return db.Session(&gorm.Session{}).Where("deleted = ?", false).Order("sort_order ASC")
		}).
		Preload("Statuses.Tasks", func(db *gorm.DB) *gorm.DB {
			return db.Session(&gorm.Session{}).Where("deleted = ?", false)
		}).
		First(&board).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("board not found")
		}
		return nil, err
	}
	return &board, nil
}

func (r *boardRepository) GetBoardByProjectId(projectID uuid.UUID) ([]models.Board, error) {
	var boards []models.Board
	if err := r.db.Session(&gorm.Session{NewDB: true}).
		Where("project_id = ? AND deleted = ?", projectID, false).
		Order("created_at DESC NULLS LAST").
		Preload("Statuses", func(db *gorm.DB) *gorm.DB {
			return db.Session(&gorm.Session{}).Where("deleted = ?", false).Order("sort_order ASC")
		}).
		Preload("Statuses.Tasks", func(db *gorm.DB) *gorm.DB {
			return db.Session(&gorm.Session{}).Where("deleted = ?", false)
		}).
		Find(&boards).Error; err != nil {
		return nil, err
	}

	return boards, nil
}

func (r *boardRepository) GetBoardByProjectIdWithUsersAndProject(projectID uuid.UUID) ([]models.Board, error) {
	var boards []models.Board
	if err := r.db.Session(&gorm.Session{NewDB: true}).
		Where("project_id = ? AND deleted = ?", projectID, false).
		Order("created_at DESC NULLS LAST").
		Preload("Project", func(db *gorm.DB) *gorm.DB {
			return db.Session(&gorm.Session{}).Where("deleted = ?", false)
		}).
		Preload("Statuses", func(db *gorm.DB) *gorm.DB {
			return db.Session(&gorm.Session{}).Where("deleted = ?", false).Order("sort_order ASC")
		}).
		Preload("Statuses.Tasks", func(db *gorm.DB) *gorm.DB {
			return db.Session(&gorm.Session{}).Where("deleted = ?", false)
		}).
		Preload("Statuses.Tasks.AssignedToUser", func(db *gorm.DB) *gorm.DB {
			return db.Session(&gorm.Session{}).Where("deleted = ?", false)
		}).
		Preload("Statuses.Tasks.CreatedByUser", func(db *gorm.DB) *gorm.DB {
			return db.Session(&gorm.Session{}).Where("deleted = ?", false)
		}).
		Find(&boards).Error; err != nil {
		return nil, err
	}

	return boards, nil
}

func (r *boardRepository) CreateBoardWithStatuses(board models.Board, statuses []models.Status) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Session(&gorm.Session{NewDB: true}).Create(&board).Error; err != nil {
			return err
		}
		if err := tx.Session(&gorm.Session{NewDB: true}).Create(&statuses).Error; err != nil {
			return err
		}
		return nil
	})
}

func (r *boardRepository) UpdateBoard(boardID uuid.UUID, updateData map[string]interface{}) (bool, error) {
	result := r.db.Session(&gorm.Session{NewDB: true}).Model(&models.Board{}).
		Where("id = ? AND deleted = ?", boardID, false).
		Updates(updateData)

	if result.Error != nil {
		return false, result.Error
	}

	return result.RowsAffected > 0, nil
}

func (r *boardRepository) DeleteBoard(boardID uuid.UUID) (bool, error) {
	now := time.Now()
	deleted := true
	updateData := map[string]interface{}{
		"deleted":    &deleted,
		"updated_at": now,
	}

	var affected int64
	err := r.db.Transaction(func(tx *gorm.DB) error {
		// 1. Проверяем, существует ли неудалённая доска
		var count int64
		if err := tx.Session(&gorm.Session{NewDB: true}).
			Model(&models.Board{}).
			Where("id = ? AND deleted = ?", boardID, false).
			Count(&count).Error; err != nil {
			return err
		}
		if count == 0 {
			return errors.New("board not found or already deleted")
		}

		// 2. Удаляем доску
		result := tx.Session(&gorm.Session{NewDB: true}).
			Model(&models.Board{}).
			Where("id = ?", boardID).
			Updates(updateData)
		if result.Error != nil {
			return result.Error
		}

		// 3. Получаем ID всех статусов этой доски (до их удаления!)
		var statusIDs []uuid.UUID
		if err := tx.Session(&gorm.Session{NewDB: true}).
			Model(&models.Status{}).
			Where("board_id = ? AND deleted = ?", boardID, false).
			Pluck("id", &statusIDs).Error; err != nil {
			return err
		}

		// 4. Удаляем задачи, привязанные к этим статусам
		if len(statusIDs) > 0 {
			if err := tx.Session(&gorm.Session{NewDB: true}).
				Model(&models.Task{}).
				Where("status_id IN ?", statusIDs).
				Updates(updateData).Error; err != nil {
				return err
			}
		}

		// 5. Удаляем статусы доски
		if err := tx.Session(&gorm.Session{NewDB: true}).
			Model(&models.Status{}).
			Where("board_id = ?", boardID).
			Updates(updateData).Error; err != nil {
			return err
		}

		affected = result.RowsAffected
		return nil
	})

	if err != nil {
		return false, err
	}

	return affected > 0, nil
}
