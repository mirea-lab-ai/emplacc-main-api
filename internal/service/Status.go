package service

import (
	"crypto/sha256"
	models "emplacc-api/internal/domain"
	"emplacc-api/internal/dto/request"
	"emplacc-api/internal/ports"
	"encoding/hex"
	"errors"
	"time"

	"github.com/google/uuid"
)

type StatusService interface {
	GetAllStatuses(page, pageSize int) ([]models.Status, int64, error)
	GetStatusesByBoardId(boardID uuid.UUID) ([]models.Status, error)
	GetStatusByID(statusID uuid.UUID) (*models.Status, error)
	CreateStatus(req request.CreateStatusRequest) (uuid.UUID, error)
	UpdateStatus(statusID uuid.UUID, req request.UpdateStatusRequest) error
	DeleteStatus(statusID uuid.UUID) error
}

type statusService struct {
	repo ports.StatusRepository
}

func NewStatusService(repo ports.StatusRepository) StatusService {
	return &statusService{
		repo: repo,
	}
}

func (s *statusService) GetAllStatuses(page, pageSize int) ([]models.Status, int64, error) {
	offset := (page - 1) * pageSize
	return s.repo.GetAllStatuses(pageSize, offset)
}

func (s *statusService) GetStatusesByBoardId(boardID uuid.UUID) ([]models.Status, error) {
	// Проверим, существует ли доска (опционально, но хорошо для 404)
	exists, err := s.repo.BoardExists(boardID)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, errors.New("board not found")
	}

	return s.repo.GetStatusesByBoardId(boardID)
}

func (s *statusService) GetStatusByID(statusID uuid.UUID) (*models.Status, error) {
	return s.repo.GetStatusByID(statusID)
}

func (s *statusService) CreateStatus(req request.CreateStatusRequest) (uuid.UUID, error) {
	id := uuid.New()
	now := time.Now()
	del := false
	h := sha256.Sum256(id[:])
	key := hex.EncodeToString(h[:])[:8]
	boardId, err := uuid.Parse(req.BoardId)
	if err != nil {
		return uuid.Nil, errors.New("invalid board id")
	}

	row := models.Status{
		ID:        id,
		Key:       &key,
		Name:      &req.Name,
		Color:     &req.Color,
		IsDefault: &req.IsDefault,
		IsActive:  &req.IsActive,
		IsOpen:    &req.IsOpen,
		CreatedAt: &now,
		Deleted:   &del,
		SortOrder: &req.Order,
		BoardID:   boardId,
	}

	err = s.repo.CreateStatus(row)
	if err != nil {
		return uuid.Nil, err
	}

	publishGlobal(StreamEvent{Type: "status.created", WorkItemID: boardId.String()})
	return id, nil
}

func (s *statusService) UpdateStatus(statusID uuid.UUID, req request.UpdateStatusRequest) error {
	update := make(map[string]interface{})
	if req.Name != nil {
		update["name"] = *req.Name
	}
	if req.Color != nil {
		update["color"] = *req.Color
	}
	if req.IsDefault != nil {
		update["is_default"] = *req.IsDefault
	}
	if req.IsActive != nil {
		update["is_active"] = *req.IsActive
	}
	if req.IsOpen != nil {
		update["is_open"] = *req.IsOpen
	}
	if req.BoardId != nil {
		update["board_id"] = *req.BoardId
	}
	if req.Order != nil {
		update["order"] = *req.Order
	}
	if len(update) == 0 {
		return errors.New("no fields to update")
	}
	now := time.Now()
	update["updated_at"] = &now

	updated, err := s.repo.UpdateStatus(statusID, update)
	if err != nil {
		return err
	}

	if !updated {
		return errors.New("status not found")
	}

	publishGlobal(StreamEvent{Type: "status.updated"})
	return nil
}

func (s *statusService) DeleteStatus(statusID uuid.UUID) error {
	deleted, err := s.repo.DeleteStatus(statusID)
	if err != nil {
		return err
	}

	if !deleted {
		return errors.New("status not found")
	}

	publishGlobal(StreamEvent{Type: "status.deleted"})
	return nil
}
