package postgres

import (
	models "emplacc-api/internal/domain"
	"emplacc-api/internal/ports"
	"errors"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type statusRepository struct {
	db *gorm.DB
}

func NewStatusRepository(db *gorm.DB) ports.StatusRepository {
	return &statusRepository{
		db: db,
	}
}

func (r *statusRepository) GetAllStatuses(limit, offset int) ([]models.Status, int64, error) {
	var total int64
	if err := r.db.Session(&gorm.Session{NewDB: true}).
		Model(&models.Status{}).
		Where("deleted = ?", false).
		Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var rows []models.Status
	if err := r.db.Session(&gorm.Session{NewDB: true}).
		Model(&models.Status{}).
		Where("deleted = ?", false).
		Order("sort_order ASC").
		Limit(limit).Offset(offset).
		Find(&rows).Error; err != nil {
		return nil, 0, err
	}

	return rows, total, nil
}

func (r *statusRepository) GetStatusesByBoardId(boardID uuid.UUID) ([]models.Status, error) {
	var statuses []models.Status
	if err := r.db.Session(&gorm.Session{NewDB: true}).
		Model(&models.Status{}).
		Where("board_id = ? AND deleted = ?", boardID, false).
		Order("sort_order ASC").
		Preload("Tasks", func(db *gorm.DB) *gorm.DB {
			return db.Session(&gorm.Session{}).Where("deleted = ?", false)
		}).
		Find(&statuses).Error; err != nil {
		return nil, err
	}

	return statuses, nil
}

func (r *statusRepository) GetStatusByID(statusID uuid.UUID) (*models.Status, error) {
	var s models.Status
	if err := r.db.Session(&gorm.Session{NewDB: true}).
		Where("id = ? AND deleted = ?", statusID, false).
		Preload("Tasks", func(db *gorm.DB) *gorm.DB {
			return db.Session(&gorm.Session{}).Where("deleted = ?", false)
		}).
		First(&s).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, errors.New("status not found")
		}
		return nil, err
	}

	return &s, nil
}

func (r *statusRepository) BoardExists(boardID uuid.UUID) (bool, error) {
	var count int64
	if err := r.db.Session(&gorm.Session{NewDB: true}).
		Model(&models.Board{}).
		Where("id = ? AND deleted = ?", boardID, false).
		Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}

func (r *statusRepository) CreateStatus(row models.Status) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		return tx.Session(&gorm.Session{NewDB: true}).
			Model(&models.Status{}).
			Create(&row).Error
	})
}

func (r *statusRepository) UpdateStatus(statusID uuid.UUID, updateData map[string]interface{}) (bool, error) {
	var affected int64
	err := r.db.Transaction(func(tx *gorm.DB) error {
		res := tx.Session(&gorm.Session{NewDB: true}).
			Model(&models.Status{}).
			Where("id = ? AND deleted = FALSE", statusID).
			Updates(updateData)
		if res.Error != nil {
			return res.Error
		}
		affected = res.RowsAffected
		return nil
	})

	if err != nil {
		return false, err
	}

	return affected > 0, nil
}

func (r *statusRepository) DeleteStatus(statusID uuid.UUID) (bool, error) {
	deleted := true
	now := time.Now()
	updateData := map[string]interface{}{
		"deleted":    &deleted,
		"updated_at": now,
	}

	var affected int64
	err := r.db.Transaction(func(tx *gorm.DB) error {
		// 1. Проверяем, существует ли неудалённый статус
		var count int64
		if err := tx.Session(&gorm.Session{NewDB: true}).
			Model(&models.Status{}).
			Where("id = ? AND deleted = ?", statusID, false).
			Count(&count).Error; err != nil {
			return err
		}
		if count == 0 {
			return errors.New("status not found or already deleted")
		}

		// 2. Удаляем задачи, привязанные к этому статусу
		if err := tx.Session(&gorm.Session{NewDB: true}).
			Model(&models.Task{}).
			Where("status_id = ?", statusID).
			Updates(updateData).Error; err != nil {
			return err
		}

		// 3. Удаляем сам статус
		if err := tx.Session(&gorm.Session{NewDB: true}).
			Model(&models.Status{}).
			Where("id = ?", statusID).
			Updates(updateData).Error; err != nil {
			return err
		}

		affected = count
		return nil
	})

	if err != nil {
		return false, err
	}

	return affected > 0, nil
}
