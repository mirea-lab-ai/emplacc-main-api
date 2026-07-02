// problem_test.go
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

func TestProblem_FullCRUD(t *testing.T) {
	testDB := setupTestDB(t)

	// Create test entities before wiring services that need a system user.
	userID := createTestUser(t, testDB, "user@example.com")

	// Create dependencies for the current architecture.
	problemRepo := postgres.NewProblemRepository(testDB)
	forumMessageRepo := postgres.NewForumMessageRepository(testDB)
	problemService := service.NewProblemService(problemRepo, forumMessageRepo, userID)
	problemController := httpapi.NewProblemController(problemService)

	e := echo.New()

	var problemID uuid.UUID

	// === 1. CreateProblem ===
	t.Run("createProblem", func(t *testing.T) {
		name := "Test Problem"
		description := []string{"This is a test problem"}

		reqBody := request.ProblemCreateRequest{
			Name:        &name,
			Description: &description,
			CreatorID:   userID.String(),
		}

		body, _ := json.Marshal(reqBody)
		req := httptest.NewRequest(http.MethodPost, "/problem", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)

		// Используем метод контроллера вместо глобальной функции
		err := problemController.CreateProblem(c)
		assert.NoError(t, err)
		assert.Equal(t, http.StatusCreated, rec.Code)

		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
		assert.Contains(t, resp, "id")
		assert.NotEmpty(t, resp["id"])
		assert.Equal(t, "Проблема создана", resp["message"])

		// Сохраняем ID проблемы
		problemID = uuid.MustParse(resp["id"].(string))
	})

	// === 2. GetProblemByID ===
	t.Run("getProblemById", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/problem/%s", problemID), nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.SetParamNames("id")
		c.SetParamValues(problemID.String())

		err := problemController.GetProblemByID(c)
		assert.NoError(t, err)
		assert.Equal(t, http.StatusOK, rec.Code)

		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
		assert.Equal(t, problemID.String(), resp["id"])
		assert.Equal(t, "Test Problem", resp["name"])
		assert.Equal(t, userID.String(), resp["creator_id"])
	})

	// === 3. GetProblemsByUserId ===
	t.Run("getProblemsByUserId", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/problem/user/%s/1/10", userID), nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.SetParamNames("id", "page", "pagesize")
		c.SetParamValues(userID.String(), "1", "10")

		err := problemController.GetProblemsByUserId(c)
		assert.NoError(t, err)
		assert.Equal(t, http.StatusOK, rec.Code)

		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
		assert.Equal(t, userID.String(), resp["user_id"])
		assert.Equal(t, float64(1), resp["page"])
		assert.Equal(t, float64(10), resp["page_size"])

		problems, ok := resp["problems"].([]interface{})
		assert.True(t, ok)
		assert.Greater(t, len(problems), 0)
	})

	// === 4. UpdateProblem ===
	t.Run("updateProblem", func(t *testing.T) {
		newDescription := []string{"Updated problem description"}

		reqBody := request.ProblemUpdateRequest{
			Description: &newDescription,
		}

		body, _ := json.Marshal(reqBody)
		req := httptest.NewRequest(http.MethodPatch, fmt.Sprintf("/problem/%s", problemID), bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.SetParamNames("id")
		c.SetParamValues(problemID.String())
		c.Set("user_id", userID.String())

		err := problemController.UpdateProblem(c)
		assert.NoError(t, err)
		assert.Equal(t, http.StatusOK, rec.Code)

		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
		assert.Equal(t, "Проблема успешно обновлена", resp["message"])
	})

	// === 5. GetAllProblems ===
	t.Run("getAllProblems", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/problem/all/1/10", nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.SetParamNames("page", "pagesize")
		c.SetParamValues("1", "10")

		err := problemController.GetAllProblems(c)
		assert.NoError(t, err)
		assert.Equal(t, http.StatusOK, rec.Code)

		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
		assert.Equal(t, float64(1), resp["page"])
		assert.Equal(t, float64(10), resp["page_size"])

		problems, ok := resp["problems"].([]interface{})
		assert.True(t, ok)
		assert.Greater(t, len(problems), 0)
	})

	// === 6. DeleteProblem ===
	t.Run("deleteProblem", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/problem/%s", problemID), nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.SetParamNames("id")
		c.SetParamValues(problemID.String())

		err := problemController.DeleteProblem(c)
		assert.NoError(t, err)
		assert.Equal(t, http.StatusOK, rec.Code)

		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
		assert.Equal(t, problemID.String(), resp["id"])
		assert.Equal(t, "Проблема успешно удалена", resp["message"])

		// Проверим, что проблема действительно удалена
		var deleted models.Problem
		require.NoError(t, testDB.Unscoped().First(&deleted, "id = ?", problemID).Error)
		assert.True(t, *deleted.Deleted)
	})
}
