package service

import (
	models "emplacc-api/internal/domain"
	"emplacc-api/internal/dto/request"
	"emplacc-api/internal/ports"
	"errors"
	"time"

	"github.com/google/uuid"
)

type AttendanceService interface {
	GetAllAttendances(page, pageSize int) ([]models.Attendance, int64, error)
	GetAttendancesByUserId(userID uuid.UUID) ([]models.Attendance, error)
	CreateAttendance(req request.AttendanceCreateRequest) (uuid.UUID, error)
	UpdateAttendance(attendanceID uuid.UUID, req request.AttendanceUpdateRequest) error
	DeleteAttendance(attendanceID uuid.UUID) error
}

type attendanceService struct {
	repo ports.AttendanceRepository
}

func NewAttendanceService(repo ports.AttendanceRepository) AttendanceService {
	return &attendanceService{
		repo: repo,
	}
}

func (s *attendanceService) GetAllAttendances(page, pageSize int) ([]models.Attendance, int64, error) {
	offset := (page - 1) * pageSize
	return s.repo.GetAllAttendances(pageSize, offset)
}

func (s *attendanceService) GetAttendancesByUserId(userID uuid.UUID) ([]models.Attendance, error) {
	return s.repo.GetAttendancesByUserId(userID)
}

func (s *attendanceService) CreateAttendance(req request.AttendanceCreateRequest) (uuid.UUID, error) {
	userID, err := uuid.Parse(req.UserId)
	if err != nil {
		return uuid.Nil, err
	}

	now := time.Now()
	del := false

	attendance := models.Attendance{
		ID:            uuid.New(),
		UserID:        userID,
		Date:          req.Date,
		WorkdayHours:  req.WorkdayHours,
		PlannedStart:  req.PlannedStart,
		ActualStart:   req.ActualStart,
		Commits:       req.Commits,
		MergeRequests: req.MergeRequests,
		CodeReviews:   req.CodeReviews,
		EndWork:       req.EndWork,
		Deleted:       &del,
		CreatedAt:     &now,
	}

	err = s.repo.CreateAttendance(attendance)
	if err != nil {
		return uuid.Nil, err
	}

	return attendance.ID, nil
}

func (s *attendanceService) UpdateAttendance(attendanceID uuid.UUID, req request.AttendanceUpdateRequest) error {
	updateData := make(map[string]interface{})
	if req.Date != nil {
		updateData["date"] = req.Date
	}
	if req.WorkdayHours != nil {
		updateData["workday_hours"] = req.WorkdayHours
	}
	if req.PlannedStart != nil {
		updateData["planned_start"] = req.PlannedStart
	}
	if req.ActualStart != nil {
		updateData["actual_start"] = req.ActualStart
	}
	if req.Commits != nil {
		updateData["commits"] = req.Commits
	}
	if req.MergeRequests != nil {
		updateData["merge_requests"] = req.MergeRequests
	}
	if req.CodeReviews != nil {
		updateData["code_reviews"] = req.CodeReviews
	}
	if req.EndWork != nil {
		updateData["end_work"] = req.EndWork
	}

	if len(updateData) == 0 {
		return errors.New("no fields to update")
	}

	now := time.Now()
	updateData["updated_at"] = &now

	updated, err := s.repo.UpdateAttendance(attendanceID, updateData)
	if err != nil {
		return err
	}

	if !updated {
		return errors.New("attendance not found")
	}

	return nil
}

func (s *attendanceService) DeleteAttendance(attendanceID uuid.UUID) error {
	deleted, err := s.repo.DeleteAttendance(attendanceID)
	if err != nil {
		return err
	}

	if !deleted {
		return errors.New("attendance not found")
	}

	return nil
}
