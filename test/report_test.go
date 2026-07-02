// report_test.go
package test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	models "emplacc-api/internal/domain"
	"emplacc-api/internal/dto/request"
	"emplacc-api/internal/repository/postgres"
	"emplacc-api/internal/service"
	httpapi "emplacc-api/internal/transport/http"
)

// createTestStatus создаёт тестовый статус
func createTestStatus(t *testing.T, db *gorm.DB, boardID uuid.UUID, name string) uuid.UUID {
	statusID := uuid.New()
	now := time.Now()
	del := false

	namePtr := &name

	zero := 0
	key := statusID.String()[:8]
	status := models.Status{
		ID:        statusID,
		BoardID:   boardID,
		Key:       &key,
		Name:      namePtr,
		Color:     strPtr("#6C757D"),
		SortOrder: &zero,
		IsDefault: &del,
		IsActive:  boolPtr(true),
		IsOpen:    boolPtr(true),
		CreatedAt: &now,
		UpdatedAt: &now,
		Deleted:   &del,
	}
	require.NoError(t, db.Create(&status).Error)
	return statusID
}

// createTestTask создаёт тестовую задачу
func createTestTask(t *testing.T, db *gorm.DB, statusID, assignedTo uuid.UUID, name string) uuid.UUID {
	taskID := uuid.New()
	now := time.Now()
	del := false

	task := models.Task{
		ID:          taskID,
		StatusID:    statusID, // ← обязательно, вместо ProjectID
		Name:        &name,
		Description: nil,
		AssignedTo:  &assignedTo,
		CreatedAt:   &now,
		UpdatedAt:   &now,
		Deleted:     &del,
	}
	require.NoError(t, db.Create(&task).Error)
	return taskID
}

// createTestProblem создаёт тестовую проблему
func createTestProblem(t *testing.T, db *gorm.DB, name string, creatorID uuid.UUID) uuid.UUID {
	problemID := uuid.New()
	now := time.Now()
	del := false

	namePtr := &name
	problem := models.Problem{
		ID:          problemID,
		Name:        namePtr,
		Description: nil,
		CreatorID:   &creatorID,
		CreatedAt:   &now,
		UpdatedAt:   &now,
		Deleted:     &del,
	}
	require.NoError(t, db.Create(&problem).Error)
	return problemID
}

// Вспомогательные функции для указателей
func strPtr(s string) *string { return &s }
func boolPtr(b bool) *bool    { return &b }

func TestReport_FullCRUD(t *testing.T) {
	testDB := setupTestDB(t)

	// Создаем зависимости для новой архитектуры
	reportRepo := postgres.NewReportRepository(testDB)
	reportService := service.NewReportService(reportRepo)
	reportController := httpapi.NewReportController(reportService, testFreshAvatarURL)

	e := echo.New()

	// Создаём тестовые сущности
	userID := createTestUser(t, testDB, "user@example.com")
	projectID := createTestProject(t, testDB, "Test Project")
	boardID := createTestBoard(t, testDB, projectID, "Test Board")
	statusID := createTestStatus(t, testDB, boardID, "To Do")
	taskID := createTestTask(t, testDB, statusID, userID, "Test Task")
	problemID := createTestProblem(t, testDB, "Test Problem", userID)
	helperID := createTestUser(t, testDB, "helper@example.com")

	reportDate := time.Now()

	var reportID uuid.UUID
	var helpRequestID uuid.UUID
	var completedWorkID uuid.UUID
	var tomorrowPlanID uuid.UUID

	// === 1. CreateReport ===
	t.Run("createReport", func(t *testing.T) {
		status := "open"
		taskId := taskID.String()

		reqBody := request.ReportCreateRequest{
			UserId:     userID.String(),
			ReportDate: &reportDate,
			CompleteWork: []request.CompletedWorkCreateRequest{
				{
					Description: "Completed work 1",
					TaskID:      taskId,
				},
			},
			PlanTomorrow: []request.TomorrowPlanCreateRequest{
				{
					Description: "Plan for tomorrow",
					TaskId:      taskId,
				},
			},
			Problems: []string{problemID.String()},
			Helps: []request.HelpRequest{
				{
					HelperID:    helperID.String(),
					Description: "Help needed",
					Status:      &status,
				},
			},
		}

		body, _ := json.Marshal(reqBody)
		req := httptest.NewRequest(http.MethodPost, "/report", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)

		// Используем метод контроллера вместо глобальной функции
		err := reportController.CreateReport(c)
		assert.NoError(t, err)
		assert.Equal(t, http.StatusCreated, rec.Code)

		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
		assert.Contains(t, resp, "id")
		assert.NotEmpty(t, resp["id"])

		// Сохраняем ID отчёта
		reportID = uuid.MustParse(resp["id"].(string))
	})

	// === 2. GetReport (получаем ID вложенных сущностей) ===
	t.Run("getReport", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/report/%s", reportID), nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.SetParamNames("id")
		c.SetParamValues(reportID.String())

		err := reportController.GetReport(c)
		assert.NoError(t, err)
		assert.Equal(t, http.StatusOK, rec.Code)

		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
		assert.Equal(t, reportID.String(), resp["id"])
		assert.Equal(t, userID.String(), resp["user_id"])

		// Получаем ID из ответа
		helpRequests := resp["help_requests"].([]interface{})
		if len(helpRequests) > 0 {
			helpRequestID = uuid.MustParse(helpRequests[0].(map[string]interface{})["id"].(string))
		}

		completedWork := resp["completed_work"].([]interface{})
		if len(completedWork) > 0 {
			completedWorkID = uuid.MustParse(completedWork[0].(map[string]interface{})["id"].(string))
		}

		tomorrowPlans := resp["plan_tomorrow"].([]interface{})
		if len(tomorrowPlans) > 0 {
			tomorrowPlanID = uuid.MustParse(tomorrowPlans[0].(map[string]interface{})["id"].(string))
		}
	})

	// === 3. GetReportsByTaskId ===
	t.Run("getReportsByTaskId", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/report/task/%s", taskID), nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.SetParamNames("id")
		c.SetParamValues(taskID.String())

		err := reportController.GetReportsByTaskId(c)
		assert.NoError(t, err)
		assert.Equal(t, http.StatusOK, rec.Code)

		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
		assert.Equal(t, taskID.String(), resp["task_id"])

		reports, ok := resp["reports"].([]interface{})
		assert.True(t, ok)
		assert.Greater(t, len(reports), 0)
	})

	// === 4. GetReportsByProjectId ===
	t.Run("getReportsByProjectId", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/report/project/%s", projectID), nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.SetParamNames("id")
		c.SetParamValues(projectID.String())

		err := reportController.GetReportsByProjectId(c)
		assert.NoError(t, err)
		assert.Equal(t, http.StatusOK, rec.Code)

		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
		assert.Equal(t, projectID.String(), resp["project_id"])

		reports, ok := resp["reports"].([]interface{})
		assert.True(t, ok)
		assert.Greater(t, len(reports), 0)
	})

	// === 5. UpdateReport ===
	t.Run("updateReport", func(t *testing.T) {
		checked := int8(1)
		userId := userID.String()

		reqBody := request.ReportUpdateRequest{
			UserId:     &userId,
			ReportDate: &reportDate,
			Checked:    &checked,
		}

		body, _ := json.Marshal(reqBody)
		req := httptest.NewRequest(http.MethodPatch, fmt.Sprintf("/report/%s", reportID), bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.SetParamNames("id")
		c.SetParamValues(reportID.String())

		err := reportController.UpdateReport(c)
		assert.NoError(t, err)
		assert.Equal(t, http.StatusOK, rec.Code)

		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
		assert.Equal(t, "Отчет успешно изменен", resp["message"])
	})

	// === 6. UpdateHelpRequest ===
	t.Run("updateHelpRequest", func(t *testing.T) {
		newStatus := "closed"
		newDescription := "Updated help request description"

		reqBody := request.HelpRequestUpdateRequest{
			Description: &newDescription,
			Status:      &newStatus,
		}

		body, _ := json.Marshal(reqBody)
		req := httptest.NewRequest(http.MethodPatch, fmt.Sprintf("/report/help-request/%s", helpRequestID), bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.SetParamNames("id")
		c.SetParamValues(helpRequestID.String())

		err := reportController.UpdateHelpRequest(c)
		assert.NoError(t, err)
		assert.Equal(t, http.StatusOK, rec.Code)

		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
		assert.Equal(t, helpRequestID.String(), resp["id"])
		assert.Equal(t, "Запрос на помощь обновлен", resp["message"])
	})

	// === 7. UpdateCompletedWork ===
	t.Run("updateCompletedWork", func(t *testing.T) {
		newDescription := "Updated completed work description"

		reqBody := request.CompletedWorkUpdateRequest{
			Description: &newDescription,
		}

		body, _ := json.Marshal(reqBody)
		req := httptest.NewRequest(http.MethodPatch, fmt.Sprintf("/report/completed-work/%s", completedWorkID), bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.SetParamNames("id")
		c.SetParamValues(completedWorkID.String())

		err := reportController.UpdateCompletedWork(c)
		assert.NoError(t, err)
		assert.Equal(t, http.StatusOK, rec.Code)

		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
		assert.Equal(t, completedWorkID.String(), resp["id"])
		assert.Equal(t, "Выполненная работа обновлена", resp["message"])
	})

	// === 8. UpdateTomorrowPlans ===
	t.Run("updateTomorrowPlans", func(t *testing.T) {
		newDescription := "Updated tomorrow plan description"

		reqBody := request.TomorrowPlansUpdateRequest{
			Description: &newDescription,
		}

		body, _ := json.Marshal(reqBody)
		req := httptest.NewRequest(http.MethodPatch, fmt.Sprintf("/report/tomorrow-plans/%s", tomorrowPlanID), bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.SetParamNames("id")
		c.SetParamValues(tomorrowPlanID.String())

		err := reportController.UpdateTomorrowPlans(c)
		assert.NoError(t, err)
		assert.Equal(t, http.StatusOK, rec.Code)

		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
		assert.Equal(t, tomorrowPlanID.String(), resp["id"])
		assert.Equal(t, "Планы на завтра обновлены", resp["message"])
	})

	// === 9. GetHelpRequestsForUser ===
	t.Run("getHelpRequestsForUser", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/report/help-requests-by-user-id/%s", helperID), nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.SetParamNames("id")
		c.SetParamValues(helperID.String())

		err := reportController.GetHelpRequestsForUser(c)
		assert.NoError(t, err)
		assert.Equal(t, http.StatusOK, rec.Code)

		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))

		helpRequests, ok := resp["help_requests"].([]interface{})
		assert.True(t, ok)
		assert.Greater(t, len(helpRequests), 0)

		firstHelp := helpRequests[0].(map[string]interface{})
		assert.Equal(t, helpRequestID.String(), firstHelp["help_request"].(map[string]interface{})["id"])
	})

	// === 10. DeleteHelpRequest ===
	t.Run("deleteHelpRequest", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/report/help-request/%s", helpRequestID), nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.SetParamNames("id")
		c.SetParamValues(helpRequestID.String())

		err := reportController.DeleteHelpRequest(c)
		assert.NoError(t, err)
		assert.Equal(t, http.StatusOK, rec.Code)

		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
		assert.Equal(t, helpRequestID.String(), resp["id"])
		assert.Equal(t, "Запрос на помощь успешно удален", resp["message"])

		// Проверим, что запрос действительно удален
		var deleted models.HelpRequest
		require.NoError(t, testDB.Unscoped().First(&deleted, "id = ?", helpRequestID).Error)
		assert.True(t, *deleted.Deleted)
	})

	// === 11. GetAllReportsByUserId ===
	t.Run("getAllReportsByUserId", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/report/user/%s/1/10", userID), nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.SetParamNames("id", "page", "pagesize")
		c.SetParamValues(userID.String(), "1", "10")

		err := reportController.GetAllReportsByUserId(c)
		assert.NoError(t, err)
		assert.Equal(t, http.StatusOK, rec.Code)

		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
		assert.Equal(t, float64(1), resp["page"])
		assert.Equal(t, float64(10), resp["page_size"])

		reports, ok := resp["reports"].([]interface{})
		assert.True(t, ok)
		assert.Greater(t, len(reports), 0)
	})

	// === 12. DeleteReport ===
	t.Run("deleteReport", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/report/%s", reportID), nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.SetParamNames("id")
		c.SetParamValues(reportID.String())

		err := reportController.DeleteReport(c)
		assert.NoError(t, err)
		assert.Equal(t, http.StatusOK, rec.Code)

		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
		assert.Equal(t, reportID.String(), resp["id"])
		assert.Equal(t, "Отчет успешно удален", resp["message"])

		// Проверим, что отчет действительно удалён
		var deleted models.DailyReport
		require.NoError(t, testDB.Unscoped().First(&deleted, "id = ?", reportID).Error)
		assert.True(t, *deleted.Deleted)
	})
}
