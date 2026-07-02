package postgres

import (
	models "emplacc-api/internal/domain"
	"emplacc-api/internal/ports"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type attendanceRepository struct {
	db *gorm.DB
}

func NewAttendanceRepository(db *gorm.DB) ports.AttendanceRepository {
	return &attendanceRepository{
		db: db,
	}
}

func (r *attendanceRepository) GetAllAttendances(limit, offset int) ([]models.Attendance, int64, error) {
	dbq := r.db.Session(&gorm.Session{NewDB: true}).
		Model(&models.Attendance{}).
		Where("deleted = FALSE")

	var totalCount int64
	if err := dbq.Count(&totalCount).Error; err != nil {
		return nil, 0, err
	}

	var attendances []models.Attendance
	if err := dbq.
		Order("created_at DESC NULLS LAST").
		Limit(limit).
		Offset(offset).
		Find(&attendances).Error; err != nil {
		return nil, 0, err
	}

	return attendances, totalCount, nil
}

func (r *attendanceRepository) GetAttendancesByUserId(userID uuid.UUID) ([]models.Attendance, error) {
	dbq := r.db.Session(&gorm.Session{NewDB: true}).
		Model(&models.Attendance{}).
		Where("deleted = FALSE AND user_id = ?", userID)

	var attendances []models.Attendance
	if err := dbq.
		Order("date DESC NULLS LAST, created_at DESC NULLS LAST").
		Find(&attendances).Error; err != nil {
		return nil, err
	}

	return attendances, nil
}

func (r *attendanceRepository) CreateAttendance(attendance models.Attendance) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Session(&gorm.Session{NewDB: true}).
			Model(&models.Attendance{}).
			Create(&attendance).Error; err != nil {
			return err
		}
		return nil
	})
}

func (r *attendanceRepository) UpdateAttendance(attendanceID uuid.UUID, updateData map[string]interface{}) (bool, error) {
	var affected int64
	err := r.db.Transaction(func(tx *gorm.DB) error {
		res := tx.Session(&gorm.Session{NewDB: true}).
			Model(&models.Attendance{}).
			Where("id = ? AND deleted = FALSE", attendanceID).
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

func (r *attendanceRepository) DeleteAttendance(attendanceID uuid.UUID) (bool, error) {
	now := time.Now()
	del := true
	updateData := map[string]interface{}{
		"deleted":    &del,
		"updated_at": &now,
	}

	var affected int64
	err := r.db.Transaction(func(tx *gorm.DB) error {
		res := tx.Session(&gorm.Session{NewDB: true}).
			Model(&models.Attendance{}).
			Where("id = ? AND deleted = FALSE", attendanceID).
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
