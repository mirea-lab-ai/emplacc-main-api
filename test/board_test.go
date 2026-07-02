// board_integration_test.go
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

// createTestProject создаёт тестовый проект
func createTestProject(t *testing.T, db *gorm.DB, name string) uuid.UUID {
	projectID := uuid.New()
	now := time.Now()
	del := false
	project := models.Project{
		ID:          projectID,
		Name:        &name,
		Description: nil,
		Deleted:     &del,
		CreatedAt:   &now,
		UpdatedAt:   &now,
	}
	require.NoError(t, db.Create(&project).Error)
	return projectID
}

// createTestBoard создаёт тестовую доску
func createTestBoard(t *testing.T, db *gorm.DB, projectID uuid.UUID, name string) uuid.UUID {
	boardID := uuid.New()
	now := time.Now()
	del := false

	board := models.Board{
		ID:          boardID,
		ProjectID:   projectID,
		Name:        &name,
		Description: nil,
		Deleted:     &del,
		CreatedAt:   &now,
		UpdatedAt:   &now,
	}
	require.NoError(t, db.Create(&board).Error)
	return boardID
}

func TestBoard_FullCRUD(t *testing.T) {
	testDB := setupTestDB(t)

	// Создаем зависимости для новой архитектуры
	boardRepo := postgres.NewBoardRepository(testDB)
	boardService := service.NewBoardService(boardRepo)
	boardController := httpapi.NewBoardController(boardService)

	// Создаём Echo instance
	e := echo.New()

	// Создаём проект
	projectName := "Test Project"
	projectID := createTestProject(t, testDB, projectName)

	// === 1. CreateBoard ===
	t.Run("createBoard", func(t *testing.T) {
		boardName := "Test Board"
		boardDesc := "Description for test board"
		projectIDStr := projectID.String()

		reqBody := request.BoardCreateRequest{
			ProjectID:   &projectIDStr,
			Name:        &boardName,
			Description: &boardDesc,
		}
		body, _ := json.Marshal(reqBody)
		req := httptest.NewRequest(http.MethodPost, "/boards", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)

		err := boardController.CreateBoard(c)
		assert.NoError(t, err)
		assert.Equal(t, http.StatusCreated, rec.Code)

		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
		assert.Contains(t, resp, "id")
		assert.NotEmpty(t, resp["id"])
	})

	// Получим ID созданной доски
	var board models.Board
	require.NoError(t, testDB.Where("project_id = ? AND deleted = ?", projectID, false).First(&board).Error)
	boardID := board.ID

	// === 2. GetBoardById ===
	t.Run("getBoardById", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/boards/%s", boardID), nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.SetParamNames("id")
		c.SetParamValues(boardID.String())

		err := boardController.GetBoardById(c)
		assert.NoError(t, err)
		assert.Equal(t, http.StatusOK, rec.Code)

		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
		assert.Equal(t, boardID.String(), resp["id"])
		assert.Equal(t, projectID.String(), resp["project_id"])
		assert.Equal(t, "Test Board", resp["name"])
		assert.Equal(t, "Description for test board", resp["description"])
	})

	// === 3. GetBoardByProjectId ===
	t.Run("getBoardByProjectId", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/boards/project/%s", projectID), nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.SetParamNames("projectId")
		c.SetParamValues(projectID.String())

		err := boardController.GetBoardByProjectId(c)
		assert.NoError(t, err)
		assert.Equal(t, http.StatusOK, rec.Code)

		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
		assert.Equal(t, projectID.String(), resp["project_id"])

		boards, ok := resp["boards"].([]interface{})
		assert.True(t, ok)
		assert.Greater(t, len(boards), 0)

		firstBoard := boards[0].(map[string]interface{})
		assert.Equal(t, boardID.String(), firstBoard["id"])
		assert.Equal(t, "Test Board", firstBoard["name"])
	})

	// === 4. UpdateBoard ===
	t.Run("updateBoard", func(t *testing.T) {
		newName := "Updated Board Name"
		newDesc := "Updated description"

		reqBody := request.BoardUpdateRequest{
			Name:        &newName,
			Description: &newDesc,
		}
		body, _ := json.Marshal(reqBody)
		req := httptest.NewRequest(http.MethodPatch, fmt.Sprintf("/boards/%s", boardID), bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.SetParamNames("id")
		c.SetParamValues(boardID.String())

		err := boardController.UpdateBoard(c)
		assert.NoError(t, err)
		assert.Equal(t, http.StatusOK, rec.Code)

		// Проверим в БД
		var updated models.Board
		require.NoError(t, testDB.Where("id = ? AND deleted = ?", boardID, false).First(&updated).Error)
		assert.Equal(t, newName, *updated.Name)
		assert.Equal(t, newDesc, *updated.Description)
	})

	// === 5. DeleteBoard ===
	t.Run("deleteBoard", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/boards/%s", boardID), nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.SetParamNames("id")
		c.SetParamValues(boardID.String())

		err := boardController.DeleteBoard(c)
		assert.NoError(t, err)
		assert.Equal(t, http.StatusOK, rec.Code)

		// Проверим, что deleted = true
		var deleted models.Board
		require.NoError(t, testDB.Unscoped().First(&deleted, "id = ?", boardID).Error)
		assert.True(t, *deleted.Deleted)
	})

	// === 6. GetAllBoards ===
	t.Run("getAllBoards", func(t *testing.T) {
		// Создадим ещё одну доску, чтобы было что пагинировать
		createTestBoard(t, testDB, projectID, "Second Board")

		req := httptest.NewRequest(http.MethodGet, "/boards/all/1/10", nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.SetParamNames("page", "pagesize")
		c.SetParamValues("1", "10")

		err := boardController.GetAllBoards(c)
		assert.NoError(t, err)
		assert.Equal(t, http.StatusOK, rec.Code)

		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
		assert.Equal(t, float64(1), resp["page"])
		assert.Equal(t, float64(10), resp["page_size"])

		boards, ok := resp["boards"].([]interface{})
		assert.True(t, ok)
		// Должна быть только одна активная доска (вторая создана, первая - удалена)
		assert.Equal(t, 1, len(boards))
	})
}
