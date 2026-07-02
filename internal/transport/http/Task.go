package httpapi

import (
	"context"
	models "emplacc-api/internal/domain"
	"emplacc-api/internal/dto/request"
	"emplacc-api/internal/dto/response"
	"emplacc-api/internal/grpc/client"
	"emplacc-api/internal/service"
	utils "emplacc-api/internal/utils"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"github.com/xuri/excelize/v2"
)

type TaskController struct {
	taskService     service.TaskService
	userService     service.UserService
	projectService  service.ProjectService
	llmClient       taskLLMClient
	conveyorService service.ConveyorService
	llmSettings     service.LLMSettingsService
	freshAvatarURL  func(string) string
}

type taskLLMClient interface {
	ProcessTaskWithLLM(ctx context.Context, taskDescription, userText, taskID string, overrides *client.LLMOverrides) (string, error)
}

func NewTaskController(taskService service.TaskService, userService service.UserService, projectService service.ProjectService, llmClient *client.LLMClient, llmSettings service.LLMSettingsService, freshAvatarURL func(string) string) *TaskController {
	return &TaskController{
		taskService:    taskService,
		userService:    userService,
		projectService: projectService,
		llmClient:      llmClient,
		llmSettings:    llmSettings,
		freshAvatarURL: freshAvatarURL,
	}
}

func NewTaskControllerWithConveyor(taskService service.TaskService, userService service.UserService, projectService service.ProjectService, llmClient *client.LLMClient, conveyorService service.ConveyorService, llmSettings service.LLMSettingsService, freshAvatarURL func(string) string) *TaskController {
	controller := NewTaskController(taskService, userService, projectService, llmClient, llmSettings, freshAvatarURL)
	controller.conveyorService = conveyorService
	return controller
}

func RegisterTaskRoutes(e Router, taskService service.TaskService, userService service.UserService, projectService service.ProjectService, llmClient *client.LLMClient, conveyorService service.ConveyorService, llmSettings service.LLMSettingsService, freshAvatarURL func(string) string, employeeMw echo.MiddlewareFunc, managerMw echo.MiddlewareFunc) {
	controller := NewTaskControllerWithConveyor(taskService, userService, projectService, llmClient, conveyorService, llmSettings, freshAvatarURL)
	g := e.Group("/task")
	// Чтение — все авторизованные
	g.GET("/all/:page/:pagesize", controller.GetAllTasks)
	g.GET("/:id", controller.GetTaskByID)
	g.GET("/user/:id/:page/:pagesize", controller.GetTasksByUserId)
	g.GET("/user/:user_id/project/:project_id/:page/:pagesize", controller.GetUserProjectTasks)
	g.GET("/user/:id/:page/:pagesize/active", controller.GetActiveTasksByUserId)
	g.GET("/board-project/:id", controller.GetTaskBoardAndProject)
	g.GET("/export/active-tasks/xlsx", controller.ExportAllActiveTasksToXLSX)
	g.GET("/search", controller.SearchTasks)
	// Запись — employee и выше (гость не может)
	g.POST("", controller.CreateTask, employeeMw)
	g.PATCH("/:id", controller.UpdateTask, employeeMw)
	g.POST("/move", controller.TaskMoveFunc, employeeMw)
	g.POST("/:id/improve-report", controller.ImproveTaskReport, employeeMw)
	g.POST("/improve-text", controller.ImproveText, employeeMw)
	// Удаление — только manager и admin
	g.DELETE("/:id", controller.DeleteTask, managerMw)
}

// GetAllTasks godoc
// @Summary Получение списка всех задач
// @Description Получает список всех задач с учетом пагинации, исключая удаленные
// @Tags Tasks
// @Accept json
// @Produce json
// @Param page path int true "Номер страницы"
// @Param pagesize path int true "Размер страницы"
// @Security BearerAuth
// @Success 200 {object} response.TaskListResponse "Список задач успешно получен"
// @Failure 400 {object} map[string]string "Ошибка в запросе"
// @Failure 401 {object} map[string]string "Нет или неверный токен"
// @Failure 500 {object} map[string]string "Ошибка сервера при получении задач"
// @Router /task/all/{page}/{pagesize} [get]
func (tc *TaskController) GetAllTasks(c echo.Context) error {
	page, err := strconv.Atoi(c.Param("page"))
	if err != nil || page <= 0 {
		page = 1
	}
	pageSize, err := strconv.Atoi(c.Param("pagesize"))
	if err != nil || pageSize <= 0 {
		pageSize = 10
	}

	tasks, totalCount, err := tc.taskService.GetAllTasks(page, pageSize)
	if err != nil {
		log.Printf("service error (get all tasks): %v", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Ошибка при подсчёте задач"})
	}

	taskList := response.TaskListResponse{
		Page:       page,
		PageSize:   pageSize,
		TotalCount: totalCount,
		Tasks:      make([]response.TaskShort, 0, len(tasks)),
	}

	for _, task := range tasks {
		taskList.Tasks = append(taskList.Tasks, response.TaskShort{
			ID:        task.ID.String(),
			StatusID:  task.StatusID.String(), // ← uuid.UUID → string
			Name:      utils.GetString(task.Name),
			Priority:  utils.GetInt16(task.Priority),
			StartDate: utils.GetTime(task.StartDate),
			Deadline:  utils.GetTime(task.Deadline),
			CreatedAt: utils.GetTime(task.CreatedAt),
			UpdatedAt: utils.GetTime(task.UpdatedAt),
		})
	}

	return c.JSON(http.StatusOK, taskList)
}

// SearchTasks godoc
// @Summary Поиск задач с автодополнением и пагинацией
// @Description Ищет задачи по названию с автодополнением после каждого введенного символа. Поддерживает автоматическую замену раскладки клавиатуры (английская-русская) для расширенного поиска и пагинацию. Задачи сортируются: сначала задачи где пользователь исполнитель, потом где создатель, потом остальные.
// @Tags Tasks
// @Accept json
// @Produce json
// @Param query query string true "Поисковый запрос"
// @Param user_id query string true "ID пользователя для приоритетной сортировки"
// @Param page query int true "Номер страницы"
// @Param pagesize query int true "Размер страницы" minimum(1) maximum(100)
// @Security BearerAuth
// @Success 200 {object} response.TaskSearchResponse "Результаты поиска задач"
// @Failure 400 {object} map[string]string "Ошибка в запросе"
// @Failure 401 {object} map[string]string "Нет или неверный токен"
// @Failure 500 {object} map[string]string "Ошибка сервера при поиске задач"
// @Router /task/search [get]
func (tc *TaskController) SearchTasks(c echo.Context) error {
	query := strings.TrimSpace(c.QueryParam("query"))
	userID := strings.TrimSpace(c.QueryParam("user_id"))

	if query == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "search query is required"})
	}

	if userID == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "user_id is required"})
	}

	// Validate userID as UUID.
	if _, err := uuid.Parse(userID); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid user_id format"})
	}

	page, err := strconv.Atoi(c.QueryParam("page"))
	if err != nil || page <= 0 {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid page number"})
	}

	pageSize, err := strconv.Atoi(c.QueryParam("pagesize"))
	if err != nil || pageSize <= 0 || pageSize > 100 {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid page size"})
	}

	log.Printf("Search tasks for user %s: query='%s', page=%d, pageSize=%d", userID, query, page, pageSize)

	// Pass userID to the service.
	tasks, totalCount, err := tc.taskService.SearchTasks(query, userID, page, pageSize)
	if err != nil {
		log.Printf("Service error (search tasks): %v", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "task search failed"})
	}

	log.Printf("Search completed: found %d tasks out of %d total", len(tasks), totalCount)

	taskSearch := response.TaskSearchResponse{
		Query:      query,
		Page:       page,
		PageSize:   pageSize,
		TotalCount: totalCount,
		Tasks:      make([]response.TaskSearchItem, 0, len(tasks)),
	}

	for _, task := range tasks {
		// Build status information.
		var statusInfo response.TaskStatusInfo
		if task.Status != nil {
			statusInfo = response.TaskStatusInfo{
				ID:    task.Status.ID.String(),
				Name:  utils.GetString(task.Status.Name),
				Color: utils.GetString(task.Status.Color),
				Key:   utils.GetString(task.Status.Key),
			}
		}

		// Build project information.
		var projectInfo response.TaskProjectInfo
		if task.Status != nil && task.Status.Board != nil && task.Status.Board.Project != nil {
			projectInfo = response.TaskProjectInfo{
				ID:          task.Status.Board.Project.ID.String(),
				Name:        utils.GetString(task.Status.Board.Project.Name),
				Description: utils.GetString(task.Status.Board.Project.Description),
			}
		}

		// Build assigned user information.
		var assignedToInfo response.UserShort
		if task.AssignedToUser != nil {
			assignedToInfo = response.UserShort{
				ID:        task.AssignedToUser.ID.String(),
				FirstName: task.AssignedToUser.FirstName,
				LastName:  task.AssignedToUser.LastName,
				AvatarURL: tc.freshAvatarURL(task.AssignedToUser.AvatarURL),
			}
		}

		// Build creator information.
		var createdByInfo response.UserShort
		if task.CreatedByUser != nil {
			createdByInfo = response.UserShort{
				ID:        task.CreatedByUser.ID.String(),
				FirstName: task.CreatedByUser.FirstName,
				LastName:  task.CreatedByUser.LastName,
				AvatarURL: tc.freshAvatarURL(task.CreatedByUser.AvatarURL),
			}
		}

		taskSearch.Tasks = append(taskSearch.Tasks, response.TaskSearchItem{
			ID:          task.ID.String(),
			Name:        utils.GetString(task.Name),
			Description: utils.GetString(task.Description),
			Priority:    utils.GetInt16(task.Priority),
			StartDate:   utils.GetTime(task.StartDate),
			Deadline:    utils.GetTime(task.Deadline),
			CreatedAt:   utils.GetTime(task.CreatedAt),
			UpdatedAt:   utils.GetTime(task.UpdatedAt),
			Status:      statusInfo,
			Project:     projectInfo,
			AssignedTo:  assignedToInfo,
			CreatedBy:   createdByInfo,
		})
	}

	return c.JSON(http.StatusOK, taskSearch)
}

// GetTaskByID godoc
// @Summary Получение задачи по ID
// @Description Получает данные задачи по её уникальному идентификатору, включая статус и пользователей
// @Tags Tasks
// @Accept json
// @Produce json
// @Param id path string true "ID задачи"
// @Security BearerAuth
// @Success 200 {object} response.GetTaskByIDResponse "Задача успешно получена"
// @Failure 400 {object} map[string]string "invalid task id"
// @Failure 401 {object} map[string]string "Нет или неверный токен"
// @Failure 404 {object} map[string]string "task not found"
// @Failure 500 {object} map[string]string "Ошибка сервера при получении задачи"
// @Router /task/{id} [get]
func (tc *TaskController) GetTaskByID(c echo.Context) error {
	taskID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "invalid task id",
		})
	}

	task, err := tc.taskService.GetTaskByID(taskID)
	if err != nil {
		if err.Error() == "task not found" {
			return c.JSON(http.StatusNotFound, map[string]string{"error": "task not found"})
		}
		log.Printf("service error (find task by id): %v", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Ошибка при получении задачи"})
	}

	// Создатель
	creatorInfo := response.UserShort{}
	if task.CreatedByUser != nil {
		creatorInfo = response.UserShort{
			ID:        task.CreatedByUser.ID.String(),
			FirstName: task.CreatedByUser.FirstName,
			LastName:  task.CreatedByUser.LastName,
			AvatarURL: tc.freshAvatarURL(task.CreatedByUser.AvatarURL),
		}
	}

	// Исполнитель
	var assignerInfo response.UserShort
	if task.AssignedToUser != nil {
		assignerInfo = response.UserShort{
			ID:        task.AssignedToUser.ID.String(),
			FirstName: task.AssignedToUser.FirstName,
			LastName:  task.AssignedToUser.LastName,
			AvatarURL: tc.freshAvatarURL(task.AssignedToUser.AvatarURL),
		}
	}

	taskResponse := response.GetTaskByIDResponse{
		ID:            task.ID.String(),
		StatusID:      task.StatusID.String(), // ← только ID статуса
		Name:          utils.GetString(task.Name),
		Description:   utils.GetString(task.Description),
		Priority:      utils.GetInt16(task.Priority),
		CreatedBy:     creatorInfo,
		AssignedTo:    assignerInfo, // ← указатель, чтобыomitempty работал
		Deadline:      utils.GetTime(task.Deadline),
		TimeSpent:     utils.GetString(task.TimeSpent),
		StartDate:     utils.GetTime(task.StartDate),
		GitlabIssueID: utils.GetInt(task.GitlabIssueID),
		Category:      utils.GetInt8(task.Category),
		UpdatedAt:     utils.GetTime(task.UpdatedAt),
		CreatedAt:     utils.GetTime(task.CreatedAt),
	}

	return c.JSON(http.StatusOK, taskResponse)
}

/*
// GetTasksByProjectID godoc
// @Summary Получение задач по ID проекта
// @Description Получает список задач, связанных с указанным проектом через доски и статусы
// @Tags Tasks
// @Accept json
// @Produce json
// @Param projectId path string true "ID проекта"
// @Param page path int true "Номер страницы"
// @Param pagesize path int true "Размер страницы"
// @Security BearerAuth
// @Success 200 {object} response.TaskListResponse "Список задач успешно получен"
// @Failure 400 {object} map[string]string "Некорректный идентификатор проекта"
// @Failure 401 {object} map[string]string "Нет или неверный токен"
// @Failure 500 {object} map[string]string "Ошибка сервера при получении задач"
// @Router /task/project/{projectId}/{page}/{pagesize} [get]
func (tc *TaskController) GetTasksByProjectID(c echo.Context) error {
	projectID, err := uuid.Parse(c.Param("projectId"))
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Некорректный идентификатор проекта"})
	}

	page, err := strconv.Atoi(c.Param("page"))
	if err != nil || page <= 0 {
		page = 1
	}
	pageSize, err := strconv.Atoi(c.Param("pagesize"))
	if err != nil || pageSize <= 0 {
		pageSize = 10
	}

	tasks, totalCount, err := tc.taskService.GetTasksByProjectID(projectID, page, pageSize)
	if err != nil {
		if err.Error() == "project not found" {
			return c.JSON(http.StatusNotFound, map[string]string{"error": "project not found"})
		}
		log.Printf("service error (get tasks by project id): %v", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Ошибка при получении статусов проекта"})
	}

	taskList := response.TaskListResponse{
		Page:       page,
		PageSize:   pageSize,
		TotalCount: totalCount,
		Tasks:      make([]response.TaskShort, 0, len(tasks)),
	}

	for _, task := range tasks {
		taskList.Tasks = append(taskList.Tasks, response.TaskShort{
			ID:        task.ID.String(),
			StatusID:  task.StatusID.String(),
			Name:      utils.GetString(task.Name),
			Priority:  utils.GetInt16(task.Priority),
			StartDate: utils.GetTime(task.StartDate),
			Deadline:  utils.GetTime(task.Deadline),
			CreatedAt: utils.GetTime(task.CreatedAt),
			UpdatedAt: utils.GetTime(task.UpdatedAt),
		})
	}

	return c.JSON(http.StatusOK, taskList)
}*/

// CreateTask godoc
// @Summary Создание новой задачи
// @Description Создает новую задачу с указанными параметрами
// @Tags Tasks
// @Accept json
// @Produce json
// @Param task body request.TaskCreateRequest true "Данные для создания задачи"
// @Security BearerAuth
// @Success 201 {object} response.TaskUniversaResponse "Задача успешно создана"
// @Failure 400 {object} map[string]string "Ошибка в запросе или некорректные идентификаторы"
// @Failure 401 {object} map[string]string "Нет или неверный токен"
// @Failure 404 {object} map[string]string "Исполнитель, поручитель или статус не найдены"
// @Failure 500 {object} map[string]string "Ошибка сервера при создании задачи"
// @Router /task [post]
func (tc *TaskController) CreateTask(c echo.Context) error {
	var req request.TaskCreateRequest
	if err := c.Bind(&req); err != nil {
		log.Printf("Bind error: %v", err)
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "Не удалось получить данные из запроса",
		})
	}

	taskID, err := tc.taskService.CreateTask(req)
	if err != nil {
		if err.Error() == "invalid assigned_to" {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "Некорректный идентификатор исполнителя"})
		}
		if err.Error() == "assignee not found" {
			return c.JSON(http.StatusNotFound, map[string]string{"error": "Исполнитель не найден"})
		}
		if err.Error() == "invalid creator_id" {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "Некорректный идентификатор поручителя"})
		}
		if err.Error() == "creator not found" {
			return c.JSON(http.StatusNotFound, map[string]string{"error": "Поручитель не найден"})
		}
		if err.Error() == "invalid status_id" {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "Некорректный идентификатор статуса"})
		}
		if err.Error() == "status not found" {
			return c.JSON(http.StatusNotFound, map[string]string{"error": "status not found or linked to a deleted board"})
		}
		log.Printf("service error (create task): %v", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Ошибка при создании задачи"})
	}

	return c.JSON(http.StatusCreated, response.TaskUniversaResponse{
		ID:      taskID.String(),
		Message: "Задача создана",
	})
}

// UpdateTask godoc
// @Summary Обновление задачи
// @Description Обновляет данные задачи по её ID
// @Tags Tasks
// @Accept json
// @Produce json
// @Param id path string true "ID задачи"
// @Param task body request.TaskUpdateRequest true "Данные для обновления задачи"
// @Security BearerAuth
// @Failure 401 {object} map[string]string "Нет или неверный токен"
// @Success 200 {object} response.TaskUniversaResponse "Задача успешно обновлена"
// @Failure 400 {object} map[string]string "Некорректный идентификатор или ошибка в запросе"
// @Failure 404 {object} map[string]string "task not found"
// @Failure 500 {object} map[string]string "Ошибка сервера при обновлении задачи"
// @Router /task/{id} [patch]
func (tc *TaskController) UpdateTask(c echo.Context) error {
	id := c.Param("id")
	taskID, err := uuid.Parse(id)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid task id"})
	}

	var req request.TaskUpdateRequest
	if err = c.Bind(&req); err != nil {
		log.Printf("Bind error (update task): %v", err)
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Неверные данные запроса"})
	}

	err = tc.taskService.UpdateTask(taskID, req)
	if err != nil {
		if err.Error() == "no fields to update" {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "Нет данных для обновления"})
		}
		if err.Error() == "task not found" {
			return c.JSON(http.StatusNotFound, map[string]string{"error": "task not found"})
		}
		if err.Error() == "invalid assigned_to" {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "Некорректный идентификатор исполнителя"})
		}
		log.Printf("service error (update task): %v", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Ошибка при обновлении задачи"})
	}

	return c.JSON(http.StatusOK, response.TaskUniversaResponse{
		ID:      taskID.String(),
		Message: "Задача изменена",
	})
}

// DeleteTask godoc
// @Summary Удаление задачи
// @Description Логическое удаление задачи по ID, включая связанные отчеты (поле deleted = true)
// @Tags Tasks
// @Accept json
// @Produce json
// @Param id path string true "ID задачи"
// @Security BearerAuth
// @Failure 401 {object} map[string]string "Нет или неверный токен"
// @Success 200 {object} response.TaskUniversaResponse "Задача успешно удалена"
// @Failure 404 {object} map[string]string "task not found"
// @Failure 500 {object} map[string]string "Ошибка сервера при удалении задачи"
// @Router /task/{id} [delete]
func (tc *TaskController) DeleteTask(c echo.Context) error {
	id := c.Param("id")
	taskID, err := uuid.Parse(id)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid task id"})
	}

	err = tc.taskService.DeleteTask(taskID)
	if err != nil {
		if err.Error() == "task not found" {
			return c.JSON(http.StatusNotFound, map[string]string{"message": "Ничего не удалено"})
		}
		log.Printf("service error (delete task): %v", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Ошибка при удалении задачи"})
	}

	return c.JSON(http.StatusOK, response.TaskUniversaResponse{
		ID:      taskID.String(),
		Message: "Задача удалена",
	})
}

// GetTasksByUserId godoc
// @Summary Получение задач по ID пользователя
// @Description Получение списка задач, назначенных на конкретного пользователя, с пагинацией (deleted = false)
// @Tags Tasks
// @Accept json
// @Produce json
// @Param id path string true "ID пользователя"
// @Param page path int true "Номер страницы"
// @Param pagesize path int true "Размер страницы"
// @Security BearerAuth
// @Success 200 {object} response.UserTasksResponse "Список задач успешно получен"
// @Failure 400 {object} map[string]string "Ошибка при парсинге параметров или некорректный ID пользователя"
// @Failure 401 {object} map[string]string "Нет или неверный токен"
// @Failure 500 {object} map[string]string "Ошибка сервера при получении задач"
// @Router /task/user/{id}/{page}/{pagesize} [get]
func (tc *TaskController) GetTasksByUserId(c echo.Context) error {
	userID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Некорректный идентификатор пользователя"})
	}

	page, err := strconv.Atoi(c.Param("page"))
	if err != nil || page <= 0 {
		page = 1
	}
	pageSize, err := strconv.Atoi(c.Param("pagesize"))
	if err != nil || pageSize <= 0 {
		pageSize = 10
	}
	if pageSize > 100 {
		pageSize = 100
	}

	tasks, totalCount, err := tc.taskService.GetTasksByUserId(userID, page, pageSize)
	if err != nil {
		log.Printf("service error (get tasks by user id): %v", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Ошибка при получении задач"})
	}

	// === МАППИНГ В TaskFull ===
	tasksResp := make([]response.TaskFull, len(tasks))
	for i, task := range tasks {
		var projectInfo response.TaskProjectInfo
		if task.Status != nil && task.Status.Board != nil && task.Status.Board.Project != nil {
			projectInfo = response.TaskProjectInfo{
				ID:   task.Status.Board.Project.ID.String(),
				Name: utils.GetString(task.Status.Board.Project.Name),
			}
		}
		tasksResp[i] = response.TaskFull{
			ID:            task.ID.String(),
			Name:          utils.GetString(task.Name),
			Description:   utils.GetString(task.Description),
			Priority:      utils.GetInt16(task.Priority),
			Deadline:      utils.GetTime(task.Deadline),
			StartDate:     utils.GetTime(task.StartDate),
			TimeSpent:     utils.GetString(task.TimeSpent),
			GitlabIssueID: utils.GetInt(task.GitlabIssueID),
			Category:      utils.GetInt8(task.Category),
			Deleted:       utils.GetBool(task.Deleted),
			CreatedAt:     utils.GetTime(task.CreatedAt),
			UpdatedAt:     utils.GetTime(task.UpdatedAt),
			Status: response.StatusFull{
				ID:     task.Status.ID.String(),
				Name:   utils.GetString(task.Status.Name),
				Key:    utils.GetString(task.Status.Key),
				Color:  utils.GetString(task.Status.Color),
				IsOpen: utils.GetBool(task.Status.IsOpen),
				Board: response.BoardRef{
					ID:        task.Status.Board.ID.String(),
					Name:      utils.GetString(task.Status.Board.Name),
					ProjectID: task.Status.Board.ProjectID.String(),
				},
			},
			Project: projectInfo,
			CreatedByUser: response.UserFull{
				ID:        task.CreatedByUser.ID.String(),
				FirstName: task.CreatedByUser.FirstName,
				LastName:  task.CreatedByUser.LastName,
				Email:     task.CreatedByUser.Email,
			},
			AssignedToUser: response.UserFull{
				ID:        task.AssignedToUser.ID.String(),
				FirstName: task.AssignedToUser.FirstName,
				LastName:  task.AssignedToUser.LastName,
				Email:     task.AssignedToUser.Email,
			},
		}
	}

	resp := response.UserTasksResponse{
		Tasks:      tasksResp,
		TotalCount: totalCount,
		Page:       page,
		PageSize:   pageSize,
	}

	return c.JSON(http.StatusOK, resp)
}

// GetActiveTasksByUserId godoc
// @Summary Получение активных задач по ID пользователя (исключая статус Done)
// @Description Получение списка задач, назначенных на пользователя, где статус != 'done' и deleted = false
// @Tags Tasks
// @Accept json
// @Produce json
// @Param id path string true "ID пользователя"
// @Param page path int true "Номер страницы"
// @Param pagesize path int true "Размер страницы"
// @Security BearerAuth
// @Success 200 {object} response.UserTasksResponse "Список активных задач успешно получен"
// @Failure 400 {object} map[string]string "Ошибка при парсинге параметров или некорректный ID пользователя"
// @Failure 401 {object} map[string]string "Нет или неверный токен"
// @Failure 500 {object} map[string]string "Ошибка сервера при получении задач"
// @Router /task/user/{id}/{page}/{pagesize}/active [get]
func (tc *TaskController) GetActiveTasksByUserId(c echo.Context) error {
	userID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Некорректный идентификатор пользователя"})
	}

	page, err := strconv.Atoi(c.Param("page"))
	if err != nil || page <= 0 {
		page = 1
	}
	pageSize, err := strconv.Atoi(c.Param("pagesize"))
	if err != nil || pageSize <= 0 {
		pageSize = 10
	}
	if pageSize > 100 {
		pageSize = 100
	}

	tasks, totalCount, err := tc.taskService.GetActiveTasksByUserId(userID, page, pageSize)
	if err != nil {
		log.Printf("service error (get active tasks by user id): %v", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Ошибка при получении активных задач"})
	}

	tasksResp := make([]response.TaskFull, len(tasks))
	for i, task := range tasks {
		var projectInfo response.TaskProjectInfo
		if task.Status != nil && task.Status.Board != nil && task.Status.Board.Project != nil {
			projectInfo = response.TaskProjectInfo{
				ID:   task.Status.Board.Project.ID.String(),
				Name: utils.GetString(task.Status.Board.Project.Name),
			}
		}
		tasksResp[i] = response.TaskFull{
			ID:            task.ID.String(),
			Name:          utils.GetString(task.Name),
			Description:   utils.GetString(task.Description),
			Priority:      utils.GetInt16(task.Priority),
			Deadline:      utils.GetTime(task.Deadline),
			StartDate:     utils.GetTime(task.StartDate),
			TimeSpent:     utils.GetString(task.TimeSpent),
			GitlabIssueID: utils.GetInt(task.GitlabIssueID),
			Category:      utils.GetInt8(task.Category),
			Deleted:       utils.GetBool(task.Deleted),
			CreatedAt:     utils.GetTime(task.CreatedAt),
			UpdatedAt:     utils.GetTime(task.UpdatedAt),
			Status: response.StatusFull{
				ID:     task.Status.ID.String(),
				Name:   utils.GetString(task.Status.Name),
				Key:    utils.GetString(task.Status.Key),
				Color:  utils.GetString(task.Status.Color),
				IsOpen: utils.GetBool(task.Status.IsOpen),
				Board: response.BoardRef{
					ID:        task.Status.Board.ID.String(),
					Name:      utils.GetString(task.Status.Board.Name),
					ProjectID: task.Status.Board.ProjectID.String(),
				},
			},
			Project: projectInfo,
			CreatedByUser: response.UserFull{
				ID:        task.CreatedByUser.ID.String(),
				FirstName: task.CreatedByUser.FirstName,
				LastName:  task.CreatedByUser.LastName,
				Email:     task.CreatedByUser.Email,
			},
			AssignedToUser: response.UserFull{
				ID:        task.AssignedToUser.ID.String(),
				FirstName: task.AssignedToUser.FirstName,
				LastName:  task.AssignedToUser.LastName,
				Email:     task.AssignedToUser.Email,
			},
		}
	}

	resp := response.UserTasksResponse{
		Tasks:      tasksResp,
		TotalCount: totalCount,
		Page:       page,
		PageSize:   pageSize,
	}

	return c.JSON(http.StatusOK, resp)
}

// TaskMoveFunc godoc
// @Summary Переместить задачу в другой статус (столбец)
// @Description Перемещает задачу в указанный статус на той же доске
// @Tags Tasks
// @Accept json
// @Produce json
// @Param request body request.MoveTaskToAnotherStatus true "Данные для перемещения задачи"
// @Security BearerAuth
// @Success 200 {object} map[string]string "Задача успешно перемещена"
// @Failure 400 {object} map[string]string "Некорректные данные запроса"
// @Failure 401 {object} map[string]string "Нет или неверный токен"
// @Failure 404 {object} map[string]string "Задача или статус не найдены"
// @Failure 409 {object} map[string]string "Нельзя переместить задачу в статус с другой доски"
// @Failure 500 {object} map[string]string "Ошибка сервера при перемещении задачи"
// @Router /task/move [post]
func (tc *TaskController) TaskMoveFunc(c echo.Context) error {
	var req request.MoveTaskToAnotherStatus
	if err := c.Bind(&req); err != nil {
		log.Printf("Bind error: %v", err)
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Неверный формат запроса"})
	}

	taskID, err := uuid.Parse(req.TaskID)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Некорректный ID задачи"})
	}

	toStatusID, err := uuid.Parse(req.ToStatusID)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Некорректный ID статуса"})
	}

	statuses, err := tc.taskService.TaskMoveFunc(taskID, toStatusID)
	if err != nil {
		if err.Error() == "task not found" {
			return c.JSON(http.StatusNotFound, map[string]string{"error": "task not found"})
		}
		if err.Error() == "status not found" {
			return c.JSON(http.StatusNotFound, map[string]string{"error": "Целевой статус не найден"})
		}
		if err.Error() == "different board" {
			return c.JSON(http.StatusConflict, map[string]string{"error": "Нельзя переместить задачу в статус с другой доски"})
		}
		if errors.Is(err, service.ErrTaskMoveCloseGateRequired) {
			return c.JSON(http.StatusConflict, map[string]string{"error": "approval_required"})
		}
		log.Printf("service error (task move): %v", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Ошибка при обновлении статуса задачи"})
	}

	// Формируем ответ
	resp := response.StatusByBoardIdResponse{
		BoardId:  statuses[0].BoardID.String(),
		Statuses: make([]response.StatusResponse, 0, len(statuses)),
	}

	for _, status := range statuses {
		tasks := make([]response.TaskShort, 0, len(status.Tasks))
		for _, task := range status.Tasks {
			tasks = append(tasks, response.TaskShort{
				ID:        task.ID.String(),
				Name:      utils.GetString(task.Name),
				StatusID:  task.StatusID.String(),
				Priority:  utils.GetInt16(task.Priority),
				CreatedAt: utils.GetTime(task.CreatedAt),
				UpdatedAt: utils.GetTime(task.UpdatedAt),
				StartDate: utils.GetTime(task.StartDate),
				Deadline:  utils.GetTime(task.Deadline),
			})
		}

		resp.Statuses = append(resp.Statuses, response.StatusResponse{
			ID:        status.ID.String(),
			Key:       utils.GetString(status.Key),
			Name:      utils.GetString(status.Name),
			Color:     utils.GetString(status.Color),
			Order:     utils.GetInt(status.SortOrder), // ← не забудь!
			IsDefault: utils.GetBool(status.IsDefault),
			IsActive:  utils.GetBool(status.IsActive),
			IsOpen:    utils.GetBool(status.IsOpen),
			CreatedAt: utils.GetTime(status.CreatedAt),
			UpdatedAt: utils.GetTime(status.UpdatedAt),
			Tasks:     tasks, // ← задачи внутри статуса
		})
	}

	return c.JSON(http.StatusOK, resp)
}

// GetUserProjectTasks godoc
// @Summary Получение задач пользователя в рамках проекта
// @Description Получает список задач, назначенных на указанного пользователя и принадлежащих заданному проекту, с пагинацией
// @Tags Tasks
// @Accept json
// @Produce json
// @Param user_id path string true "ID пользователя"
// @Param project_id path string true "ID проекта"
// @Param page path int true "Номер страницы"
// @Param pagesize path int true "Размер страницы"
// @Security BearerAuth
// @Success 200 {object} response.UserProjectTasksResponse "Список задач успешно получен"
// @Failure 400 {object} map[string]string "Некорректный ID пользователя или проекта"
// @Failure 401 {object} map[string]string "Нет или неверный токен"
// @Failure 404 {object} map[string]string "user or project not found"
// @Failure 500 {object} map[string]string "Ошибка сервера при получении задач"
// @Router /task/user/{user_id}/project/{project_id}/{page}/{pagesize} [get]
func (tc *TaskController) GetUserProjectTasks(c echo.Context) error {
	userID, err := uuid.Parse(c.Param("user_id"))
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Некорректный ID пользователя"})
	}

	projectID, err := uuid.Parse(c.Param("project_id"))
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Некорректный ID проекта"})
	}

	page, err := strconv.Atoi(c.Param("page"))
	if err != nil || page <= 0 {
		page = 1
	}
	pageSize, err := strconv.Atoi(c.Param("pagesize"))
	if err != nil || pageSize <= 0 {
		pageSize = 10
	}
	if pageSize > 100 {
		pageSize = 100
	}

	// Получаем задачи (уже отфильтрованы на уровне SQL)
	tasks, totalCount, err := tc.taskService.GetTasksByUserIDAndProjectID(userID, projectID, page, pageSize)
	if err != nil {
		log.Printf("service error: %v", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Ошибка при получении задач"})
	}

	// Получаем данные пользователя и проекта для верхнего уровня
	user, err := tc.userService.GetUserById(userID)
	if err != nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "user not found"})
	}
	project, err := tc.projectService.GetProjectByID(projectID)
	if err != nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "project not found"})
	}

	userResp := response.UserFull{
		ID:        user.ID.String(),
		FirstName: utils.GetString(&user.FirstName),
		LastName:  utils.GetString(&user.LastName),
		Email:     user.Email,
	}

	projectResp := response.ProjectShort{
		ID:          project.ID.String(),
		Name:        utils.GetString(project.Name),
		Description: utils.GetString(project.Description),
		GitlabURL:   utils.GetString(project.GitlabURL),
		CreatedBy:   utils.GetUUIDString(project.CreatedBy),
	}

	tasksResp := make([]response.TaskFull, len(tasks))
	for i, task := range tasks {
		var projectInfo response.TaskProjectInfo
		if task.Status != nil && task.Status.Board != nil && task.Status.Board.Project != nil {
			projectInfo = response.TaskProjectInfo{
				ID:   task.Status.Board.Project.ID.String(),
				Name: utils.GetString(task.Status.Board.Project.Name),
			}
		}
		tasksResp[i] = response.TaskFull{
			ID:            task.ID.String(),
			Name:          utils.GetString(task.Name),
			Description:   utils.GetString(task.Description),
			Priority:      utils.GetInt16(task.Priority),
			Deadline:      utils.GetTime(task.Deadline),
			StartDate:     utils.GetTime(task.StartDate),
			TimeSpent:     utils.GetString(task.TimeSpent),
			GitlabIssueID: utils.GetInt(task.GitlabIssueID),
			Category:      utils.GetInt8(task.Category),
			Deleted:       utils.GetBool(task.Deleted),
			CreatedAt:     utils.GetTime(task.CreatedAt),
			UpdatedAt:     utils.GetTime(task.UpdatedAt),
			Status: response.StatusFull{
				ID:     task.Status.ID.String(),
				Name:   utils.GetString(task.Status.Name),
				Key:    utils.GetString(task.Status.Key),
				Color:  utils.GetString(task.Status.Color),
				IsOpen: utils.GetBool(task.Status.IsOpen),
				Board: response.BoardRef{
					ID:        task.Status.Board.ID.String(),
					Name:      utils.GetString(task.Status.Board.Name),
					ProjectID: task.Status.Board.ProjectID.String(),
				},
			},
			Project: projectInfo,
			CreatedByUser: response.UserFull{
				ID:        task.CreatedByUser.ID.String(),
				FirstName: task.CreatedByUser.FirstName,
				LastName:  task.CreatedByUser.LastName,
				Email:     task.CreatedByUser.Email,
			},
			AssignedToUser: response.UserFull{
				ID:        task.AssignedToUser.ID.String(),
				FirstName: task.AssignedToUser.FirstName,
				LastName:  task.AssignedToUser.LastName,
				Email:     task.AssignedToUser.Email,
			},
		}
	}

	resp := response.UserProjectTasksResponse{
		User:       userResp,
		Project:    projectResp,
		Tasks:      tasksResp,
		TotalCount: totalCount,
		Page:       page,
		PageSize:   pageSize,
	}

	return c.JSON(http.StatusOK, resp)
}

// ImproveTaskReport godoc
// @Summary Улучшить отчет по задаче с помощью LLM
// @Description Принимает текст пользователя и улучшает его на основе описания задачи через LLM
// @Tags Tasks
// @Accept json
// @Produce json
// @Param id path string true "ID задачи"
// @Param request body request.ImproveReportRequest true "Данные для улучшения отчета"
// @Security BearerAuth
// @Success 200 {object} response.ImprovedReportResponse "Улучшенный отчет"
// @Failure 400 {object} map[string]string "Некорректный ID задачи или данные запроса"
// @Failure 404 {object} map[string]string "task not found"
// @Failure 500 {object} map[string]string "Ошибка при обработке LLM"
// @Router /task/{id}/improve-report [post]
func (tc *TaskController) ImproveTaskReport(c echo.Context) error {
	taskId := c.Param("id")
	taskID, err := uuid.Parse(taskId)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "invalid task id",
		})
	}

	// Получаем задачу из БД
	task, err := tc.taskService.GetTaskByID(taskID)
	if err != nil {
		if err.Error() == "task not found" {
			return c.JSON(http.StatusNotFound, map[string]string{"error": "task not found"})
		}
		log.Printf("service error (get task by id): %v", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Ошибка при получении задачи"})
	}

	// Парсим запрос
	var req request.ImproveReportRequest
	if err := c.Bind(&req); err != nil {
		log.Printf("Bind error: %v", err)
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "Неверный формат запроса",
		})
	}

	if req.UserText == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "Текст пользователя не может быть пустым",
		})
	}

	// Получаем описание задачи (может быть nil)
	taskDescription := ""
	if task.Description != nil {
		taskDescription = *task.Description
	}

	// Если у задачи нет описания, используем название
	if taskDescription == "" && task.Name != nil {
		taskDescription = *task.Name
	}

	// Загружаем настройки LLM из БД, но используем промпт для описания задачи
	overrides := &client.LLMOverrides{SystemPrompt: taskDescriptionSystemPrompt}
	if tc.llmSettings != nil {
		if s, err := tc.llmSettings.Get(); err == nil && s != nil {
			overrides.Model = s.WebUIModel
			overrides.URL = s.WebUIURL
		}
	}

	actor, actorErr := actorFromContext(c)
	var agentRunID uuid.UUID
	if tc.conveyorService != nil && actorErr == nil {
		registered, err := tc.conveyorService.RegisterAgentRun(c.Request().Context(), actor, service.RegisterAgentRunRequest{
			WorkItemID: taskID,
			Source:     "backend",
			Harness:    "llm-task-report",
			Status:     models.AgentRunStatusRunning,
			Summary:    "LLM task report improvement started",
			Metadata:   llmAgentRunMetadata(taskID.String(), "ImproveTaskReport", "text/plain", safeLLMMetaKeys(overrides), "started"),
		})
		if err != nil {
			log.Printf("agent run registration failed: %v", err)
			return c.JSON(http.StatusServiceUnavailable, map[string]string{"error": "agent_run_unavailable"})
		}
		agentRunID = registered.EntityID
	}

	// Вызываем gRPC сервис
	improvedText, err := tc.llmClient.ProcessTaskWithLLM(c.Request().Context(), taskDescription, req.UserText, taskId, overrides)
	if err != nil {
		if tc.conveyorService != nil && actorErr == nil && agentRunID != uuid.Nil {
			if _, updateErr := tc.conveyorService.UpdateAgentRun(c.Request().Context(), actor, taskID, agentRunID, service.UpdateAgentRunRequest{Status: models.AgentRunStatusFailed, Summary: "LLM task report improvement failed", Metadata: llmAgentRunMetadata(taskID.String(), "ImproveTaskReport", "text/plain", nil, "grpc_error")}); updateErr != nil {
				log.Printf("agent run failure update failed: %v", updateErr)
			}
		}
		log.Printf("gRPC error: %v", err)
		// LLM — внешняя зависимость; её сбой это 503 (dependency_unavailable),
		// а не наша 500. Фронт распознаёт 503 как retryable, а не generic unknown.
		return c.JSON(http.StatusServiceUnavailable, map[string]string{
			"error": "dependency_unavailable",
		})
	}
	if tc.conveyorService != nil && actorErr == nil && agentRunID != uuid.Nil {
		if _, updateErr := tc.conveyorService.UpdateAgentRun(c.Request().Context(), actor, taskID, agentRunID, service.UpdateAgentRunRequest{Status: models.AgentRunStatusSucceeded, Summary: "LLM task report improvement succeeded", Metadata: llmAgentRunMetadata(taskID.String(), "ImproveTaskReport", "text/plain", nil, "succeeded")}); updateErr != nil {
			log.Printf("agent run success update failed: %v", updateErr)
		}
	}

	return c.JSON(http.StatusOK, response.ImprovedReportResponse{
		TaskID:          taskID.String(),
		OriginalText:    req.UserText,
		ImprovedText:    improvedText,
		TaskTitle:       utils.GetString(task.Name),
		TaskDescription: taskDescription,
	})
}

func llmAgentRunMetadata(taskID string, route string, contentType string, metaKeys []string, outcome string) json.RawMessage {
	metadata := map[string]any{"task_id": taskID, "route": route, "content_type": contentType}
	if len(metaKeys) > 0 {
		metadata["meta_keys"] = metaKeys
	}
	if outcome != "" {
		metadata["outcome"] = outcome
	}
	encoded, err := json.Marshal(metadata)
	if err != nil {
		return nil
	}
	return encoded
}

func safeLLMMetaKeys(overrides *client.LLMOverrides) []string {
	if overrides == nil {
		return nil
	}
	keys := make([]string, 0, 3)
	if overrides.Model != "" {
		keys = append(keys, "model")
	}
	if overrides.URL != "" {
		keys = append(keys, "url")
	}
	if overrides.SystemPrompt != "" {
		keys = append(keys, "system_prompt")
	}
	return keys
}

const taskDescriptionSystemPrompt = `Ты — помощник по управлению задачами. Твоя роль: улучшать и дополнять описания задач.

Правила:
- Сделай описание чётким, структурированным и информативным
- Сохрани исходный смысл, но улучши формулировки
- Добавь контекст и детали если они очевидны из контекста
- Используй Markdown для форматирования: заголовки, списки, выделение
- Пиши на том же языке, что и входной текст
- Не добавляй лишних вводных фраз — сразу давай результат
- Ответ должен быть готов к использованию как описание задачи`

// ImproveText godoc
// @Summary Улучшить произвольный текст для описания задачи через LLM
// @Description Улучшает текст без привязки к конкретной задаче — для использования при создании задачи
// @Tags Tasks
// @Accept json
// @Produce json
// @Param request body request.ImproveReportRequest true "Текст для улучшения"
// @Security BearerAuth
// @Success 200 {object} map[string]string "improved_text"
// @Router /task/improve-text [post]
func (tc *TaskController) ImproveText(c echo.Context) error {
	var req request.ImproveReportRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Неверный формат запроса"})
	}
	if req.UserText == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Текст не может быть пустым"})
	}

	overrides := &client.LLMOverrides{SystemPrompt: taskDescriptionSystemPrompt}
	if tc.llmSettings != nil {
		if s, err := tc.llmSettings.Get(); err == nil && s != nil {
			overrides.Model = s.WebUIModel
			overrides.URL = s.WebUIURL
		}
	}

	improved, err := tc.llmClient.ProcessTaskWithLLM(c.Request().Context(), "", req.UserText, "", overrides)
	if err != nil {
		log.Printf("gRPC error (improve-text): %v", err)
		return c.JSON(http.StatusServiceUnavailable, map[string]string{"error": "dependency_unavailable"})
	}

	return c.JSON(http.StatusOK, map[string]string{
		"improved_text": improved,
		"original_text": req.UserText,
	})
}

// GetTaskBoardAndProject godoc
// @Summary Получение доски и проекта задачи
// @Description Возвращает ID доски и ID проекта, к которым принадлежит задача
// @Tags Tasks
// @Accept json
// @Produce json
// @Param id path string true "ID задачи"
// @Security BearerAuth
// @Success 200 {object} interface{} "Информация о доске и проекте"
// @Failure 400 {object} map[string]string "Некорректный ID задачи"
// @Failure 404 {object} map[string]string "Задача, статус или доска не найдены"
// @Failure 500 {object} map[string]string "Ошибка сервера"
// @Router /task/board-project/{id} [get]
func (tc *TaskController) GetTaskBoardAndProject(c echo.Context) error {
	taskID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "invalid task id",
		})
	}

	boardId, projectId, err := tc.taskService.GetTaskBoardAndProjectIDs(taskID)

	if err != nil {
		if err.Error() == "task not found" {
			return c.JSON(http.StatusNotFound, map[string]string{"error": "task not found"})
		}
		if err.Error() == "status not found" {
			return c.JSON(http.StatusNotFound, map[string]string{"error": "task status not found"})
		}
		if err.Error() == "board not found" {
			return c.JSON(http.StatusNotFound, map[string]string{"error": "board not found"})
		}
		if err.Error() == "project not found" {
			return c.JSON(http.StatusNotFound, map[string]string{"error": "project not found"})
		}
		log.Printf("service error (get task board and project): %v", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to get task data"})
	}

	return c.JSON(http.StatusOK, response.TaskBoardProjectResponse{
		TaskID:    taskID.String(),
		BoardID:   boardId.String(),
		ProjectID: projectId.String(),
	})
}

// ExportAllActiveTasksToXLSX godoc
// @Summary Экспорт активных задач всех пользователей в XLSX
// @Description Генерирует XLSX-файл с активными задачами всех пользователей, сгруппированными по пользователям
// @Tags Tasks
// @Accept json
// @Produce application/vnd.openxmlformats-officedocument.spreadsheetml.sheet
// @Security BearerAuth
// @Success 200 {string} file "XLSX-файл с активными задачами"
// @Failure 500 {object} map[string]string "Ошибка при генерации отчёта"
// @Router /task/export/active-tasks/xlsx [get]
func (tc *TaskController) ExportAllActiveTasksToXLSX(c echo.Context) error {
	data, err := tc.taskService.GetAllActiveTasksForXLSX()
	if err != nil {
		log.Printf("service error (get all active tasks for XLSX): %v", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to prepare data"})
	}

	f := excelize.NewFile()

	summarySheet := "Active tasks summary"
	f.NewSheet(summarySheet)

	summaryHeaders := []interface{}{
		"User", "Task title", "Description",
		"Priority", "Start date", "Deadline", "Status",
		"Board", "Project", "Created", "Updated",
	}

	if err := f.SetSheetRow(summarySheet, "A1", &summaryHeaders); err != nil {
		log.Printf("XLSX header error: %v", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to create XLSX"})
	}

	rowIndex := 2
	totalTasks := 0

	for _, user := range data.Users {
		userSheet := user.UserName
		if userSheet == "" {
			userSheet = user.UserEmail
		}

		f.NewSheet(userSheet)

		userHeaders := []interface{}{
			"Task title", "Description",
			"Priority", "Start date", "Deadline", "Status",
			"Board", "Project", "Created", "Updated",
		}
		f.SetSheetRow(userSheet, "A1", &userHeaders)

		userRowIndex := 2

		for _, task := range user.Tasks {
			totalTasks++

			priorityText := utils.ConvertPriorityToText(task.Priority)

			summaryRow := []interface{}{
				user.UserName,
				task.Name,
				task.Description,
				priorityText,
				utils.FormatTimeForExcel(task.StartDate),
				utils.FormatTimeForExcel(task.Deadline),
				task.StatusName,
				task.BoardName,
				task.ProjectName,
				task.CreatedAt.Format("02.01.2006 15:04"),
				task.UpdatedAt.Format("02.01.2006 15:04"),
			}

			axis := fmt.Sprintf("A%d", rowIndex)
			if err := f.SetSheetRow(summarySheet, axis, &summaryRow); err != nil {
				log.Printf("XLSX summary row error: %v", err)
				return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to fill XLSX"})
			}
			rowIndex++

			userRow := []interface{}{
				task.Name,
				task.Description,
				priorityText,
				utils.FormatTimeForExcel(task.StartDate),
				utils.FormatTimeForExcel(task.Deadline),
				task.StatusName,
				task.BoardName,
				task.ProjectName,
				task.CreatedAt.Format("02.01.2006 15:04"),
				task.UpdatedAt.Format("02.01.2006 15:04"),
			}

			userAxis := fmt.Sprintf("A%d", userRowIndex)
			if err := f.SetSheetRow(userSheet, userAxis, &userRow); err != nil {
				log.Printf("XLSX user row error: %v", err)
				return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to fill XLSX"})
			}
			userRowIndex++
		}

		f.SetColWidth(userSheet, "A", "J", 20)
		f.SetColWidth(userSheet, "A", "B", 30)
		styleID, _ := f.NewStyle(&excelize.Style{
			Alignment: &excelize.Alignment{WrapText: true, Vertical: "top"},
		})
		f.SetCellStyle(userSheet, "A1", fmt.Sprintf("J%d", userRowIndex), styleID)
	}

	f.SetCellValue(summarySheet, "A1", "Total active tasks: "+strconv.Itoa(totalTasks))
	f.SetCellValue(summarySheet, "B1", "Total users: "+strconv.Itoa(len(data.Users)))

	f.SetColWidth(summarySheet, "A", "K", 20)
	f.SetColWidth(summarySheet, "B", "C", 30)
	styleID, _ := f.NewStyle(&excelize.Style{
		Alignment: &excelize.Alignment{WrapText: true, Vertical: "top"},
	})
	f.SetCellStyle(summarySheet, "A2", fmt.Sprintf("K%d", rowIndex), styleID)

	f.DeleteSheet("Sheet1")

	index, err := f.GetSheetIndex(summarySheet)
	if err != nil {
		log.Printf("Creating list error: %v", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to generate file"})
	}
	f.SetActiveSheet(index)

	buf, err := f.WriteToBuffer()
	if err != nil {
		log.Printf("XLSX buffer error: %v", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to generate file"})
	}

	filename := fmt.Sprintf("active_tasks_%s.xlsx", time.Now().Format("2006-01-02"))

	c.Response().Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	c.Response().Header().Set("Content-Disposition", "attachment; filename="+filename)
	return c.Blob(http.StatusOK, "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", buf.Bytes())
}
