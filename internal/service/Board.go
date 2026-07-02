package service

import (
	models "emplacc-api/internal/domain"
	"emplacc-api/internal/dto/request"
	"emplacc-api/internal/dto/response"
	"emplacc-api/internal/ports"
	"emplacc-api/internal/utils"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
)

type BoardService interface {
	GetAllBoards(page, pageSize int) ([]models.Board, int64, error)
	GetBoardById(boardID uuid.UUID) (*models.Board, error)
	GetBoardByProjectId(projectID uuid.UUID) ([]models.Board, error)
	CreateBoard(req request.BoardCreateRequest) (uuid.UUID, error)
	UpdateBoard(boardID uuid.UUID, req request.BoardUpdateRequest) error
	DeleteBoard(boardID uuid.UUID) error
	GetProjectTasksForXLSX(projectID uuid.UUID) (*response.ProjectTasksXLSXData, error)
}

type boardService struct {
	repo ports.BoardRepository
}

func NewBoardService(repo ports.BoardRepository) BoardService {
	return &boardService{
		repo: repo,
	}
}

func (s *boardService) GetAllBoards(page, pageSize int) ([]models.Board, int64, error) {
	offset := (page - 1) * pageSize
	return s.repo.GetAllBoards(pageSize, offset)
}

func (s *boardService) GetBoardById(boardID uuid.UUID) (*models.Board, error) {
	return s.repo.GetBoardById(boardID)
}

func (s *boardService) GetBoardByProjectId(projectID uuid.UUID) ([]models.Board, error) {
	return s.repo.GetBoardByProjectId(projectID)
}

func (s *boardService) GetProjectTasksForXLSX(projectID uuid.UUID) (*response.ProjectTasksXLSXData, error) {
	boards, err := s.repo.GetBoardByProjectIdWithUsersAndProject(projectID)
	if err != nil {
		return nil, err
	}

	projectName := "Неизвестный проект"
	projectDescription := ""

	if len(boards) > 0 && boards[0].Project != nil {
		if boards[0].Project.Name != nil {
			projectName = *boards[0].Project.Name
		}
		if boards[0].Project.Description != nil {
			projectDescription = *boards[0].Project.Description
		}
	}

	xlsxBoards := make([]response.BoardXLSXResponse, 0, len(boards))

	for _, board := range boards {
		boardName := "Без названия"
		if board.Name != nil {
			boardName = *board.Name
		}

		boardDescription := ""
		if board.Description != nil {
			boardDescription = *board.Description
		}

		statuses := make([]response.StatusXLSXResponse, 0, len(board.Statuses))

		for _, status := range board.Statuses {
			statusName := "Без названия"
			if status.Name != nil {
				statusName = *status.Name
			}

			statusColor := ""
			if status.Color != nil {
				statusColor = *status.Color
			}

			tasks := make([]response.TaskXLSX, 0, len(status.Tasks))

			for _, task := range status.Tasks {
				taskName := "Без названия"
				if task.Name != nil {
					taskName = *task.Name
				}

				description := ""
				if task.Description != nil {
					description = *task.Description
				}

				assignedTo := "Не назначено"
				if task.AssignedToUser != nil {
					firstName := task.AssignedToUser.FirstName
					lastName := task.AssignedToUser.LastName
					if firstName != "" || lastName != "" {
						assignedTo = strings.TrimSpace(firstName + " " + lastName)
					} else if task.AssignedToUser.Email != "" {
						assignedTo = task.AssignedToUser.Email
					}
				}

				createdBy := "Неизвестно"
				if task.CreatedByUser != nil {
					firstName := task.CreatedByUser.FirstName
					lastName := task.CreatedByUser.LastName
					if firstName != "" || lastName != "" {
						createdBy = strings.TrimSpace(firstName + " " + lastName)
					} else if task.CreatedByUser.Email != "" {
						createdBy = task.CreatedByUser.Email
					}
				}

				tasks = append(tasks, response.TaskXLSX{
					ID:          task.ID.String(),
					Name:        taskName,
					Description: description,
					Priority:    utils.GetInt16(task.Priority),
					StartDate:   utils.GetTime(task.StartDate),
					Deadline:    utils.GetTime(task.Deadline),
					AssignedTo:  assignedTo,
					CreatedBy:   createdBy,
					CreatedAt:   utils.GetTime(task.CreatedAt),
					UpdatedAt:   utils.GetTime(task.UpdatedAt),
				})
			}

			statuses = append(statuses, response.StatusXLSXResponse{
				StatusName:  statusName,
				StatusColor: statusColor,
				Tasks:       tasks,
			})
		}

		xlsxBoards = append(xlsxBoards, response.BoardXLSXResponse{
			BoardName:        boardName,
			BoardDescription: boardDescription,
			Statuses:         statuses,
		})
	}

	return &response.ProjectTasksXLSXData{
		ProjectName:        projectName,
		ProjectDescription: projectDescription,
		Boards:             xlsxBoards,
	}, nil
}

func (s *boardService) CreateBoard(req request.BoardCreateRequest) (uuid.UUID, error) {
	if req.ProjectID == nil {
		return uuid.Nil, errors.New("project_id is required")
	}

	projectID, err := uuid.Parse(*req.ProjectID)
	if err != nil {
		return uuid.Nil, errors.New("invalid project id")
	}

	now := time.Now()
	deleted := false
	boardID := uuid.New()
	tr := true

	board := models.Board{
		ID:          boardID,
		ProjectID:   projectID,
		Name:        req.Name,
		Description: req.Description,
		Deleted:     &deleted,
		CreatedAt:   &now,
		UpdatedAt:   &now,
	}

	// Вспомогательная функция для создания статуса с уникальным Key и порядком
	makeStatus := func(order int, name, color string, isOpen bool) models.Status {
		id := uuid.New()
		key := id.String()[:8] // сокращённый UUID (8 символов)
		return models.Status{
			ID:        id,
			BoardID:   boardID,
			SortOrder: &order,
			Key:       &key,
			Name:      &name,
			Color:     &color,
			IsDefault: &tr,
			IsActive:  utils.BoolPtr(true),
			IsOpen:    &isOpen,
			Deleted:   &deleted,
			CreatedAt: &now,
			UpdatedAt: &now,
		}
	}

	defaultStatuses := []models.Status{
		makeStatus(0, "To Do", "#fc0000ff", true),
		makeStatus(1024, "Done", "#28A745", true),
	}

	err = s.repo.CreateBoardWithStatuses(board, defaultStatuses)
	if err != nil {
		return uuid.Nil, err
	}

	publishGlobal(StreamEvent{Type: "board.created", WorkItemID: projectID.String()})
	return boardID, nil
}

func (s *boardService) UpdateBoard(boardID uuid.UUID, req request.BoardUpdateRequest) error {
	updateData := make(map[string]interface{})
	if req.Name != nil {
		updateData["name"] = *req.Name
	}
	if req.Description != nil {
		updateData["description"] = *req.Description
	}

	if len(updateData) == 0 {
		return errors.New("no fields to update")
	}

	updateData["updated_at"] = time.Now()

	updated, err := s.repo.UpdateBoard(boardID, updateData)
	if err != nil {
		return err
	}

	if !updated {
		return errors.New("board not found")
	}

	publishGlobal(StreamEvent{Type: "board.updated"})
	return nil
}

func (s *boardService) DeleteBoard(boardID uuid.UUID) error {
	deleted, err := s.repo.DeleteBoard(boardID)
	if err != nil {
		return err
	}

	if !deleted {
		return errors.New("board not found")
	}

	publishGlobal(StreamEvent{Type: "board.deleted"})
	return nil
}
