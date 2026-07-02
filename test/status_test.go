// status_test.go
package test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	models "emplacc-api/internal/domain"
	"emplacc-api/internal/dto/request"
	"emplacc-api/internal/repository/postgres"
	"emplacc-api/internal/service"
	httpapi "emplacc-api/internal/transport/http"
)

func TestStatus_FullCRUD(t *testing.T) {
	testDB := setupTestDB(t)

	// Создаем зависимости для новой архитектуры
	statusRepo := postgres.NewStatusRepository(testDB)
	statusService := service.NewStatusService(statusRepo)
	statusController := httpapi.NewStatusController(statusService)

	e := echo.New()

	// Создаём тестовые сущности
	userID := createTestUser(t, testDB, "user@example.com")
	projectID := createTestProject(t, testDB, "Test Project")
	boardID := createTestBoard(t, testDB, projectID, "Test Board")

	var statusID uuid.UUID

	// === 1. CreateStatus ===
	t.Run("createStatus", func(t *testing.T) {
		reqBody := request.CreateStatusRequest{
			BoardId:   boardID.String(), // ← обязательно указать доску
			Name:      "Test Status",
			Color:     "#FF0000",
			IsDefault: false,
			IsActive:  true,
			IsOpen:    true,
		}

		body, _ := json.Marshal(reqBody)
		req := httptest.NewRequest(http.MethodPost, "/status", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)

		// Используем метод контроллера вместо глобальной функции
		err := statusController.CreateStatus(c)
		assert.NoError(t, err)
		assert.Equal(t, http.StatusCreated, rec.Code)

		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
		assert.Contains(t, resp, "id")
		assert.NotEmpty(t, resp["id"])
		assert.Equal(t, "Статус успешно создан", resp["message"])

		statusID = uuid.MustParse(resp["id"].(string))
	})

	// === 2. GetStatusByID ===
	t.Run("getStatusById", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/status/%s", statusID), nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.SetParamNames("id")
		c.SetParamValues(statusID.String())

		err := statusController.GetStatusByID(c)
		assert.NoError(t, err)
		assert.Equal(t, http.StatusOK, rec.Code)

		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
		assert.Equal(t, statusID.String(), resp["id"])
		assert.Equal(t, "Test Status", resp["name"])
		assert.Equal(t, "#FF0000", resp["color"])
		// Убедись, что в ответе есть задачи (даже пустой массив)
		assert.Contains(t, resp, "tasks")
	})

	// === 3. UpdateStatus ===
	t.Run("updateStatus", func(t *testing.T) {
		newName := "Updated Status Name"
		newColor := "#00FF00"

		reqBody := request.UpdateStatusRequest{
			Name:  &newName,
			Color: &newColor,
		}

		body, _ := json.Marshal(reqBody)
		req := httptest.NewRequest(http.MethodPatch, fmt.Sprintf("/status/%s", statusID), bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.SetParamNames("id")
		c.SetParamValues(statusID.String())

		err := statusController.UpdateStatus(c)
		assert.NoError(t, err)
		assert.Equal(t, http.StatusOK, rec.Code)

		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
		assert.Equal(t, "Статус успешно обновлён", resp["message"])
	})

	// === 4. GetAllStatuses ===
	t.Run("getAllStatuses", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/status/all/1/10", nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.SetParamNames("page", "pagesize")
		c.SetParamValues("1", "10")

		err := statusController.GetAllStatuses(c)
		assert.NoError(t, err)
		assert.Equal(t, http.StatusOK, rec.Code)

		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
		assert.Equal(t, float64(1), resp["page"])
		assert.Equal(t, float64(10), resp["page_size"])

		statuses, ok := resp["statuses"].([]interface{})
		assert.True(t, ok)
		assert.Greater(t, len(statuses), 0)
	})

	// === 5. GetStatusesByBoardId ===
	t.Run("getStatusesByBoardId", func(t *testing.T) {
		// Исправленный URL: /status/board/{board_id}
		req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/status/board/%s", boardID), nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.SetParamNames("board_id")
		c.SetParamValues(boardID.String())

		err := statusController.GetStatusesByBoardId(c)
		assert.NoError(t, err)
		assert.Equal(t, http.StatusOK, rec.Code)

		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
		assert.Equal(t, boardID.String(), resp["board_id"])

		statuses, ok := resp["statuses"].([]interface{})
		assert.True(t, ok)
		assert.Greater(t, len(statuses), 0)

		// Проверим, что статус содержит задачи (даже если их нет)
		firstStatus := statuses[0].(map[string]interface{})
		assert.Contains(t, firstStatus, "tasks")
	})

	// === 6. DeleteStatus ===
	t.Run("deleteStatus", func(t *testing.T) {
		// Создадим задачу, чтобы проверить каскадное удаление
		taskName := "Task for deletion test"
		taskID := createTestTask(t, testDB, statusID, userID, taskName)

		req := httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/status/%s", statusID), nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.SetParamNames("id")
		c.SetParamValues(statusID.String())

		err := statusController.DeleteStatus(c)
		assert.NoError(t, err)
		assert.Equal(t, http.StatusOK, rec.Code)

		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
		assert.Equal(t, statusID.String(), resp["id"])
		assert.Equal(t, "Статус успешно удалён", resp["message"])

		// Проверим, что статус удалён
		var deletedStatus models.Status
		require.NoError(t, testDB.Unscoped().First(&deletedStatus, "id = ?", statusID).Error)
		assert.True(t, *deletedStatus.Deleted)

		// Проверим, что задача тоже удалена (каскад)
		var deletedTask models.Task
		require.NoError(t, testDB.Unscoped().First(&deletedTask, "id = ?", taskID).Error)
		assert.True(t, *deletedTask.Deleted)
	})
}
