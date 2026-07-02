// task_test.go
package test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	models "emplacc-api/internal/domain"
	"emplacc-api/internal/dto/request"
	"emplacc-api/internal/dto/response"
	"emplacc-api/internal/grpc/client"
	"emplacc-api/internal/repository/postgres"
	"emplacc-api/internal/service"
	httpapi "emplacc-api/internal/transport/http"
)

func TestTask_FullCRUD(t *testing.T) {
	testDB := setupTestDB(t)

	// Создаем зависимости для новой архитектуры
	taskRepo := postgres.NewTaskRepository(testDB)
	taskService := service.NewTaskService(taskRepo, nil)
	userRepo := postgres.NewUserRepository(testDB)
	userService := service.NewUserService(userRepo)
	projectRepo := postgres.NewProjectRepository(testDB)
	projectService := service.NewProjectService(projectRepo)
	llmClient, err := client.NewLLMClient("grpc-service:50051", os.Getenv("LLM_GRPC_AUTH_TOKEN")) // используем docker service name
	if err != nil {
		log.Fatalf("Failed to create gRPC client: %v", err)
	}
	defer llmClient.Close()
	llmSettingsService := service.NewLLMSettingsService(postgres.NewLLMSettingsRepository(testDB))
	taskController := httpapi.NewTaskController(taskService, userService, projectService, llmClient, llmSettingsService, testFreshAvatarURL)

	e := echo.New()

	// Создаём тестовые сущности
	userID := createTestUser(t, testDB, "user@example.com")
	creatorID := createTestUser(t, testDB, "creator@example.com")
	projectID := createTestProject(t, testDB, "Test Project")
	boardID := createTestBoard(t, testDB, projectID, "Test Board")
	statusID := createTestStatus(t, testDB, boardID, "To Do")

	var taskID uuid.UUID

	// === 1. CreateTask ===
	t.Run("createTask", func(t *testing.T) {
		name := "Test Task"
		description := "This is a test task"
		priority := int16(5)
		category := int8(1)
		startDate := time.Now()
		deadline := time.Now().Add(24 * time.Hour)
		creatorId := creatorID.String()
		userId := userID.String()
		statusId := statusID.String() // ← обязательно

		reqBody := request.TaskCreateRequest{
			StatusID:    statusId,
			Name:        &name,
			Description: &description,
			Priority:    &priority,
			CreatorID:   &creatorId,
			AssignedTo:  &userId,
			StartDate:   &startDate,
			Deadline:    &deadline,
			Category:    &category,
		}

		body, _ := json.Marshal(reqBody)
		req := httptest.NewRequest(http.MethodPost, "/task", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)

		// Используем метод контроллера вместо глобальной функции
		err := taskController.CreateTask(c)
		assert.NoError(t, err)
		assert.Equal(t, http.StatusCreated, rec.Code)

		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
		assert.Contains(t, resp, "id")
		assert.NotEmpty(t, resp["id"])
		assert.Equal(t, "Задача создана", resp["message"])

		taskID = uuid.MustParse(resp["id"].(string))
	})

	// === 2. GetTaskByID ===
	t.Run("getTaskById", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/task/%s", taskID), nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.SetParamNames("id")
		c.SetParamValues(taskID.String())

		err := taskController.GetTaskByID(c)
		assert.NoError(t, err)
		assert.Equal(t, http.StatusOK, rec.Code)

		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
		assert.Equal(t, taskID.String(), resp["id"])
		assert.Equal(t, statusID.String(), resp["status_id"]) // ← проверяем status_id, а не project_id
		assert.Equal(t, "Test Task", resp["name"])
		assert.Equal(t, "This is a test task", resp["description"])
		assert.Equal(t, float64(5), resp["priority"])
		// Убедись, что нет поля project_id
		assert.NotContains(t, resp, "project_id")
	})

	/*
		// === 3. GetTasksByProjectID ===
		t.Run("getTasksByProjectId", func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/task/project/%s/1/10", projectID), nil)
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)
			c.SetParamNames("projectId", "page", "pagesize")
			c.SetParamValues(projectID.String(), "1", "10")

			err := taskController.GetTasksByProjectID(c)
			assert.NoError(t, err)
			assert.Equal(t, http.StatusOK, rec.Code)

			var resp map[string]interface{}
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
			assert.Equal(t, float64(1), resp["page"])
			assert.Equal(t, float64(10), resp["page_size"])

			tasks, ok := resp["tasks"].([]interface{})
			assert.True(t, ok)
			assert.Greater(t, len(tasks), 0)

			// Verify that the task contains status_id and not project_id.
			firstTask := tasks[0].(map[string]interface{})
			assert.Equal(t, taskID.String(), firstTask["id"])
			assert.Equal(t, statusID.String(), firstTask["status_id"])
			assert.NotContains(t, firstTask, "project_id")
		})
	*/

	// === 4. GetTasksByUserId ===
	t.Run("getTasksByUserId", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/task/user/%s/1/10", userID), nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.SetParamNames("id", "page", "pagesize")
		c.SetParamValues(userID.String(), "1", "10")

		err := taskController.GetTasksByUserId(c)
		assert.NoError(t, err)
		assert.Equal(t, http.StatusOK, rec.Code)

		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
		assert.Equal(t, float64(1), resp["page"])
		assert.Equal(t, float64(10), resp["page_size"])

		tasks, ok := resp["tasks"].([]interface{})
		assert.True(t, ok)
		assert.Greater(t, len(tasks), 0)

		firstTask := tasks[0].(map[string]interface{})
		assert.Equal(t, taskID.String(), firstTask["id"])
		status := firstTask["status"].(map[string]interface{})
		assert.Equal(t, statusID.String(), status["id"])
		assert.NotContains(t, firstTask, "project_id")
	})

	// === 5. UpdateTask ===
	t.Run("updateTask", func(t *testing.T) {
		newName := "Updated Task Name"
		newDescription := "Updated task description"

		reqBody := request.TaskUpdateRequest{
			Name:        &newName,
			Description: &newDescription,
		}

		body, _ := json.Marshal(reqBody)
		req := httptest.NewRequest(http.MethodPatch, fmt.Sprintf("/task/%s", taskID), bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.SetParamNames("id")
		c.SetParamValues(taskID.String())

		err := taskController.UpdateTask(c)
		assert.NoError(t, err)
		assert.Equal(t, http.StatusOK, rec.Code)

		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
		assert.Equal(t, "Задача изменена", resp["message"])
	})

	// === 6. GetAllTasks ===
	t.Run("getAllTasks", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/task/all/1/10", nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.SetParamNames("page", "pagesize")
		c.SetParamValues("1", "10")

		err := taskController.GetAllTasks(c)
		assert.NoError(t, err)
		assert.Equal(t, http.StatusOK, rec.Code)

		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
		assert.Equal(t, float64(1), resp["page"])
		assert.Equal(t, float64(10), resp["page_size"])

		tasks, ok := resp["tasks"].([]interface{})
		assert.True(t, ok)
		assert.Greater(t, len(tasks), 0)

		firstTask := tasks[0].(map[string]interface{})
		assert.Equal(t, taskID.String(), firstTask["id"])
		assert.Equal(t, statusID.String(), firstTask["status_id"])
		assert.NotContains(t, firstTask, "project_id")
	})

	// === 7. TaskMoveFunc ===
	t.Run("taskMoveFunc", func(t *testing.T) {
		// Создадим новый статус для перемещения
		newStatusID := createTestStatus(t, testDB, boardID, "In Progress")

		reqBody := request.MoveTaskToAnotherStatus{
			TaskID:     taskID.String(),
			ToStatusID: newStatusID.String(),
		}

		body, _ := json.Marshal(reqBody)
		req := httptest.NewRequest(http.MethodPost, "/task/move", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)

		err := taskController.TaskMoveFunc(c)
		assert.NoError(t, err)
		assert.Equal(t, http.StatusOK, rec.Code)

		// Проверим, что задача действительно переместилась — ЧИТАЕМ ИЗ БД
		var movedTask models.Task
		require.NoError(t, testDB.Where("id = ?", taskID).First(&movedTask).Error)
		assert.Equal(t, newStatusID, movedTask.StatusID)

		// Опционально: проверим, что в ответе есть корректная структура
		var resp response.StatusByBoardIdResponse
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
		assert.Equal(t, boardID.String(), resp.BoardId)
		assert.Greater(t, len(resp.Statuses), 0)

		// Найдём статус, в который переместили задачу
		var found bool
		for _, s := range resp.Statuses {
			if s.ID == newStatusID.String() {
				found = true
				// Проверим, что задача есть в списке задач этого статуса
				var taskFound bool
				for _, task := range s.Tasks {
					if task.ID == taskID.String() {
						taskFound = true
						break
					}
				}
				assert.True(t, taskFound, "Задача должна быть в списке задач нового статуса")
				break
			}
		}
		assert.True(t, found, "Новый статус должен быть в ответе")
	})

	// === 8. DeleteTask ===
	t.Run("deleteTask", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/task/%s", taskID), nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.SetParamNames("id")
		c.SetParamValues(taskID.String())

		err := taskController.DeleteTask(c)
		assert.NoError(t, err)
		assert.Equal(t, http.StatusOK, rec.Code)

		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
		assert.Equal(t, taskID.String(), resp["id"])
		assert.Equal(t, "Задача удалена", resp["message"])

		var deleted models.Task
		require.NoError(t, testDB.Unscoped().First(&deleted, "id = ?", taskID).Error)
		assert.True(t, *deleted.Deleted)
	})
}
