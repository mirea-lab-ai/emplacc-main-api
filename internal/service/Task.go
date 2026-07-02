package service

import (
	models "emplacc-api/internal/domain"
	"emplacc-api/internal/dto/request"
	"emplacc-api/internal/dto/response"
	"emplacc-api/internal/ports"
	"emplacc-api/internal/utils"
	"errors"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
)

var ErrTaskMoveCloseGateRequired = errors.New("close gate required")

type TaskService interface {
	GetAllTasks(page, pageSize int) ([]models.Task, int64, error)
	GetTaskByID(taskID uuid.UUID) (*models.Task, error)
	GetTasksByProjectID(projectID uuid.UUID, page, pageSize int) ([]models.Task, int64, error)
	CreateTask(req request.TaskCreateRequest) (uuid.UUID, error)
	UpdateTask(taskID uuid.UUID, req request.TaskUpdateRequest) error
	DeleteTask(taskID uuid.UUID) error
	GetTasksByUserId(userID uuid.UUID, page, pageSize int) ([]models.Task, int64, error)
	TaskMoveFunc(taskID, toStatusID uuid.UUID) ([]models.Status, error)
	GetTasksByUserIDAndProjectID(userID, projectID uuid.UUID, page, pageSize int) ([]models.Task, int64, error)
	GetActiveTasksByUserId(userID uuid.UUID, page, pageSize int) ([]models.Task, int64, error)
	GetTaskBoardAndProjectIDs(taskID uuid.UUID) (boardId, projectId uuid.UUID, err error)
	GetAllActiveTasksForXLSX() (*response.AllActiveTasksXLSXData, error)
	SearchTasks(query, userID string, page, pageSize int) ([]models.Task, int64, error)
}

type taskService struct {
	repo     ports.TaskRepository
	notifier ports.Notifier
}

func NewTaskService(repo ports.TaskRepository, notifier ports.Notifier) TaskService {
	return &taskService{
		repo:     repo,
		notifier: notifier,
	}
}

// notify уведомляет пользователя о задаче, если notifier подключён (nil-safe).
func (s *taskService) notify(userID uuid.UUID, typ, title, body string, taskID uuid.UUID) {
	if s.notifier != nil {
		s.notifier.Notify(userID, typ, title, body, "task", &taskID)
	}
}

func (s *taskService) GetAllTasks(page, pageSize int) ([]models.Task, int64, error) {
	offset := (page - 1) * pageSize
	return s.repo.GetAllTasks(pageSize, offset)
}

func (s *taskService) SearchTasks(query, userID string, page, pageSize int) ([]models.Task, int64, error) {
	offset := (page - 1) * pageSize
	return s.repo.SearchTasks(query, userID, pageSize, offset)
}

func (s *taskService) GetTaskByID(taskID uuid.UUID) (*models.Task, error) {
	return s.repo.GetTaskByID(taskID)
}

func (s *taskService) GetTasksByProjectID(projectID uuid.UUID, page, pageSize int) ([]models.Task, int64, error) {
	offset := (page - 1) * pageSize
	return s.repo.GetTasksByProjectID(projectID, pageSize, offset)
}

func (s *taskService) CreateTask(req request.TaskCreateRequest) (uuid.UUID, error) {
	// === Валидация и парсинг AssignedTo (исполнитель) ===
	if req.AssignedTo == nil {
		return uuid.Nil, errors.New("invalid assigned_to")
	}
	assigneeID, err := uuid.Parse(*req.AssignedTo)
	if err != nil {
		return uuid.Nil, errors.New("invalid assigned_to")
	}

	// Проверка существования исполнителя
	assigneeExists, err := s.repo.UserExists(assigneeID)
	if err != nil {
		return uuid.Nil, err
	}
	if !assigneeExists {
		return uuid.Nil, errors.New("assignee not found")
	}

	// === Валидация и парсинг CreatorID (поручитель) ===
	if req.CreatorID == nil {
		return uuid.Nil, errors.New("invalid creator_id")
	}
	creatorID, err := uuid.Parse(*req.CreatorID)
	if err != nil {
		return uuid.Nil, errors.New("invalid creator_id")
	}

	// Проверка существования поручителя
	creatorExists, err := s.repo.UserExists(creatorID)
	if err != nil {
		return uuid.Nil, err
	}
	if !creatorExists {
		return uuid.Nil, errors.New("creator not found")
	}

	// === Валидация StatusID ===
	statusID, err := uuid.Parse(req.StatusID)
	if err != nil {
		return uuid.Nil, errors.New("invalid status_id")
	}

	// Проверка существования статуса и его принадлежности к неудалённой доске
	statusExists, err := s.repo.StatusExists(statusID)
	if err != nil {
		return uuid.Nil, err
	}
	if !statusExists {
		return uuid.Nil, errors.New("status not found")
	}

	// === Создание задачи ===
	now := time.Now()
	deleted := false

	task := models.Task{
		ID:            uuid.New(),
		StatusID:      statusID,
		Priority:      req.Priority,
		Name:          req.Name,
		Description:   req.Description,
		CreatedBy:     &creatorID,
		AssignedTo:    &assigneeID,
		Deadline:      req.Deadline,
		StartDate:     req.StartDate,
		GitlabIssueID: req.GitlabIssueID,
		Category:      req.Category,
		Deleted:       &deleted,
		CreatedAt:     &now,
		UpdatedAt:     &now,
	}

	err = s.repo.CreateTask(task)
	if err != nil {
		return uuid.Nil, err
	}

	publishGlobal(StreamEvent{Type: "task.created", WorkItemID: task.ID.String()})
	if assigneeID != creatorID {
		name := ""
		if req.Name != nil {
			name = *req.Name
		}
		s.notify(assigneeID, "task.assigned", "Вам назначена задача", name, task.ID)
	}
	return task.ID, nil
}

func (s *taskService) UpdateTask(taskID uuid.UUID, req request.TaskUpdateRequest) error {
	var updates = make(map[string]interface{})
	if req.Name != nil {
		updates["name"] = *req.Name
	}
	if req.Description != nil {
		updates["description"] = *req.Description
	}
	if req.Priority != nil {
		updates["priority"] = *req.Priority
	}
	if req.Deadline != nil {
		updates["deadline"] = *req.Deadline
	}
	if req.StartDate != nil {
		updates["start_date"] = *req.StartDate
	}
	if req.GitlabIssueID != nil {
		updates["gitlab_issue_id"] = *req.GitlabIssueID
	}
	if req.Category != nil {
		updates["category"] = *req.Category
	}
	if req.AssignedTo != nil {
		if *req.AssignedTo == "" {
			// Очистить назначенного исполнителя
			updates["assigned_to"] = nil
		} else {
			assignedUUID, err := uuid.Parse(*req.AssignedTo)
			if err != nil {
				return errors.New("invalid assigned_to")
			}
			updates["assigned_to"] = assignedUUID
		}
	}

	if len(updates) == 0 {
		return errors.New("no fields to update")
	}

	// Запоминаем прежнего исполнителя ДО апдейта — чтобы уведомить только при реальной смене.
	var prevAssignee *uuid.UUID
	if req.AssignedTo != nil && *req.AssignedTo != "" {
		if cur, err := s.repo.GetTaskByID(taskID); err == nil && cur != nil {
			prevAssignee = cur.AssignedTo
		}
	}

	updates["updated_at"] = time.Now()

	updated, err := s.repo.UpdateTask(taskID, updates)
	if err != nil {
		return err
	}

	if !updated {
		return errors.New("task not found")
	}

	publishGlobal(StreamEvent{Type: "task.updated", WorkItemID: taskID.String()})
	if req.AssignedTo != nil && *req.AssignedTo != "" {
		if assignedUUID, err := uuid.Parse(*req.AssignedTo); err == nil {
			if prevAssignee == nil || *prevAssignee != assignedUUID {
				s.notify(assignedUUID, "task.assigned", "Вам назначена задача", "", taskID)
			}
		}
	}
	return nil
}

func (s *taskService) DeleteTask(taskID uuid.UUID) error {
	deleted, err := s.repo.DeleteTask(taskID)
	if err != nil {
		return err
	}

	if !deleted {
		return errors.New("task not found")
	}

	publishGlobal(StreamEvent{Type: "task.deleted", WorkItemID: taskID.String()})
	return nil
}

func (s *taskService) GetTasksByUserId(userID uuid.UUID, page, pageSize int) ([]models.Task, int64, error) {
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 10
	}
	if pageSize > 100 {
		pageSize = 100
	}
	offset := (page - 1) * pageSize
	return s.repo.GetTasksByUserId(userID, pageSize, offset)
}

func (s *taskService) GetActiveTasksByUserId(userID uuid.UUID, page, pageSize int) ([]models.Task, int64, error) {
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 10
	}
	if pageSize > 100 {
		pageSize = 100
	}
	offset := (page - 1) * pageSize
	return s.repo.GetActiveTasksByUserId(userID, pageSize, offset)
}

func (s *taskService) GetAllActiveTasksForXLSX() (*response.AllActiveTasksXLSXData, error) {
	tasks, err := s.repo.GetAllActiveTasks()
	if err != nil {
		return nil, err
	}

	userTasksMap := make(map[uuid.UUID]*response.UserTasksXLSX)

	for _, task := range tasks {
		if task.AssignedTo == nil {
			continue
		}

		userID := *task.AssignedTo
		if _, exists := userTasksMap[userID]; !exists {
			userName := "Unknown user"
			userEmail := ""
			if task.AssignedToUser != nil {
				firstName := task.AssignedToUser.FirstName
				lastName := task.AssignedToUser.LastName
				if firstName != "" || lastName != "" {
					userName = strings.TrimSpace(firstName + " " + lastName)
				} else {
					userName = task.AssignedToUser.Email
				}
				userEmail = task.AssignedToUser.Email
			}

			userTasksMap[userID] = &response.UserTasksXLSX{
				UserID:    userID.String(),
				UserName:  userName,
				UserEmail: userEmail,
				Tasks:     []response.TaskXLSXForTask{},
			}
		}

		projectName := "No project"
		if task.Status != nil && task.Status.Board != nil && task.Status.Board.Project != nil {
			if task.Status.Board.Project.Name != nil {
				projectName = *task.Status.Board.Project.Name
			}
		}

		taskXLSX := response.TaskXLSXForTask{
			ID:          task.ID.String(),
			Name:        utils.GetString(task.Name),
			Description: utils.GetString(task.Description),
			Priority:    utils.GetInt16(task.Priority),
			StartDate:   utils.GetTime(task.StartDate),
			Deadline:    utils.GetTime(task.Deadline),
			StatusName:  utils.GetString(task.Status.Name),
			BoardName:   utils.GetString(task.Status.Board.Name),
			ProjectName: projectName,
			CreatedAt:   utils.GetTime(task.CreatedAt),
			UpdatedAt:   utils.GetTime(task.UpdatedAt),
		}

		userTasksMap[userID].Tasks = append(userTasksMap[userID].Tasks, taskXLSX)
	}

	users := make([]response.UserTasksXLSX, 0, len(userTasksMap))
	for _, userTasks := range userTasksMap {
		users = append(users, *userTasks)
	}

	sort.Slice(users, func(i, j int) bool {
		return users[i].UserName < users[j].UserName
	})

	return &response.AllActiveTasksXLSXData{
		Users: users,
	}, nil
}

func (s *taskService) GetTaskBoardAndProjectIDs(taskID uuid.UUID) (boardId, projectId uuid.UUID, err error) {
	return s.repo.GetTaskBoardAndProjectIDs(taskID)
}

func (s *taskService) TaskMoveFunc(taskID, toStatusID uuid.UUID) ([]models.Status, error) {
	task, err := s.repo.GetTaskByID(taskID)
	if err != nil {
		if err.Error() == "task not found" {
			return nil, errors.New("task not found")
		}
		return nil, err
	}

	currentStatus, err := s.repo.GetStatusByID(task.StatusID)
	if err != nil {
		return nil, err
	}

	targetStatus, err := s.repo.GetStatusByID(toStatusID)
	if err != nil {
		if err.Error() == "status not found" {
			return nil, errors.New("status not found")
		}
		return nil, err
	}

	if currentStatus.BoardID != targetStatus.BoardID {
		return nil, errors.New("different board")
	}

	if targetStatus.IsOpen != nil && !*targetStatus.IsOpen {
		return nil, ErrTaskMoveCloseGateRequired
	}

	updatedAt := time.Now()
	err = s.repo.UpdateTaskStatus(taskID, toStatusID, updatedAt)
	if err != nil {
		return nil, err
	}

	statuses, err := s.repo.GetStatusesByBoardID(targetStatus.BoardID)
	if err != nil {
		return nil, err
	}

	publishGlobal(StreamEvent{Type: "task.moved", WorkItemID: taskID.String()})
	return statuses, nil
}

func (s *taskService) GetTasksByUserIDAndProjectID(userID, projectID uuid.UUID, page, pageSize int) ([]models.Task, int64, error) {
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 10
	}
	if pageSize > 100 {
		pageSize = 100
	}
	offset := (page - 1) * pageSize
	return s.repo.GetTasksByUserIDAndProjectID(userID, projectID, pageSize, offset)
}
