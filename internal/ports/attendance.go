package ports

import (
	models "emplacc-api/internal/domain"

	"github.com/google/uuid"
)

type AttendanceRepository interface {
	GetAllAttendances(limit, offset int) ([]models.Attendance, int64, error)
	GetAttendancesByUserId(userID uuid.UUID) ([]models.Attendance, error)
	CreateAttendance(attendance models.Attendance) error
	UpdateAttendance(attendanceID uuid.UUID, updateData map[string]interface{}) (bool, error)
	DeleteAttendance(attendanceID uuid.UUID) (bool, error)
}
