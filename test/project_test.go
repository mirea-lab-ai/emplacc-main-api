// project_test.go
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

func TestProject_FullCRUD(t *testing.T) {
	testDB := setupTestDB(t)

	// Создаем зависимости для новой архитектуры
	projectRepo := postgres.NewProjectRepository(testDB)
	projectService := service.NewProjectService(projectRepo)
	projectController := httpapi.NewProjectController(projectService)

	e := echo.New()

	// Создаём тестовые сущности
	userID := createTestUser(t, testDB, "user@example.com")
	teamID := createTestTeam(t, testDB, "Test Team")

	var projectID uuid.UUID

	// === 1. CreateProject ===
	t.Run("createProject", func(t *testing.T) {
		name := "Test Project"
		description := "This is a test project"
		gitlabProjectID := 123
		gitlabURL := "https://gitlab.com/test/project"
		status := "active"
		userId := userID.String()

		reqBody := request.CreateProjectRequest{
			Name:              &name,
			Description:       &description,
			Gitlab_project_id: &gitlabProjectID,
			Gitlab_url:        &gitlabURL,
			CreatedBy:         &userId,
			Status:            &status,
		}

		body, _ := json.Marshal(reqBody)
		req := httptest.NewRequest(http.MethodPost, "/project", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)

		// Используем метод контроллера вместо глобальной функции
		err := projectController.CreateProject(c)
		assert.NoError(t, err)
		assert.Equal(t, http.StatusCreated, rec.Code)

		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
		assert.Contains(t, resp, "id")
		assert.NotEmpty(t, resp["id"])
		assert.Equal(t, "Проект создан", resp["message"])

		// Сохраняем ID проекта
		projectID = uuid.MustParse(resp["id"].(string))
	})

	// === 2. GetProjectByID ===
	t.Run("getProjectById", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/project/%s", projectID), nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.SetParamNames("id")
		c.SetParamValues(projectID.String())

		err := projectController.GetProjectByID(c)
		assert.NoError(t, err)
		assert.Equal(t, http.StatusOK, rec.Code)

		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
		assert.Equal(t, projectID.String(), resp["id"])
		assert.Equal(t, "Test Project", resp["name"])
		assert.Equal(t, "active", resp["status"])
		assert.Equal(t, userID.String(), resp["created_by"])
	})

	// === 3. UpdateProject ===
	t.Run("updateProject", func(t *testing.T) {
		newName := "Updated Project Name"
		newStatus := "inactive"

		reqBody := request.UpdateProjectRequest{
			Name:   &newName,
			Status: &newStatus,
		}

		body, _ := json.Marshal(reqBody)
		req := httptest.NewRequest(http.MethodPatch, fmt.Sprintf("/project/%s", projectID), bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.SetParamNames("id")
		c.SetParamValues(projectID.String())

		err := projectController.UpdateProject(c)
		assert.NoError(t, err)
		assert.Equal(t, http.StatusOK, rec.Code)

		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
		assert.Equal(t, "Проект успешно обновлен", resp["message"])
	})

	// === 4. GetAllProjects ===
	t.Run("getAllProjects", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/project/all/1/10", nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.SetParamNames("page", "pagesize")
		c.SetParamValues("1", "10")

		err := projectController.GetAllProjects(c)
		assert.NoError(t, err)
		assert.Equal(t, http.StatusOK, rec.Code)

		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
		assert.Equal(t, float64(1), resp["page"])
		assert.Equal(t, float64(10), resp["page_size"])

		projects, ok := resp["projects"].([]interface{})
		assert.True(t, ok)
		assert.Greater(t, len(projects), 0)
	})

	// === 5. GetProjectsByUser ===
	t.Run("getProjectsByUser", func(t *testing.T) {
		// Создаём связь: команда -> пользователь, проект -> команда
		createTestTeamMember(t, testDB, userID, teamID)
		createTestProjectTeam(t, testDB, projectID, teamID)

		req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/project/user/%s", userID), nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.SetParamNames("id")
		c.SetParamValues(userID.String())

		err := projectController.GetProjectsByUser(c)
		assert.NoError(t, err)
		assert.Equal(t, http.StatusOK, rec.Code)

		var resp []map[string]interface{}
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
		assert.Greater(t, len(resp), 0)
		assert.Equal(t, projectID.String(), resp[0]["id"])
	})

	// === 6. GetTeamProjects ===
	t.Run("getTeamProjects", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/project/team/%s", teamID), nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.SetParamNames("team_id")
		c.SetParamValues(teamID.String())

		err := projectController.GetTeamProjects(c)
		assert.NoError(t, err)
		assert.Equal(t, http.StatusOK, rec.Code)

		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))

		projects, ok := resp["projects"].([]interface{})
		assert.True(t, ok)
		assert.Greater(t, len(projects), 0)
		assert.Equal(t, projectID.String(), projects[0].(map[string]interface{})["id"])
	})

	// === 7. DeleteProject ===
	t.Run("deleteProject", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/project/%s", projectID), nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.SetParamNames("id")
		c.SetParamValues(projectID.String())

		err := projectController.DeleteProject(c)
		assert.NoError(t, err)
		assert.Equal(t, http.StatusOK, rec.Code)

		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
		assert.Equal(t, projectID.String(), resp["id"])
		assert.Equal(t, "Проект удален", resp["message"])

		// Проверим, что проект действительно удален
		var deleted models.Project
		require.NoError(t, testDB.Unscoped().First(&deleted, "id = ?", projectID).Error)
		assert.True(t, *deleted.Deleted)
	})
}

// createTestTeam создаёт тестовую команду
func createTestTeam(t *testing.T, db *gorm.DB, name string) uuid.UUID {
	teamID := uuid.New()
	now := time.Now()
	del := false

	team := models.Team{
		ID:          teamID,
		Name:        &name,
		Description: nil,
		Deleted:     &del,
		CreatedAt:   &now,
		UpdatedAt:   &now,
	}
	require.NoError(t, db.Create(&team).Error)
	return teamID
}

// createTestTeamMember создаёт связь пользователь-команда
func createTestTeamMember(t *testing.T, db *gorm.DB, userID, teamID uuid.UUID) {
	now := time.Now()
	del := false

	member := models.TeamMember{
		UserID:    userID,
		TeamID:    teamID,
		Deleted:   &del,
		CreatedAt: &now,
		UpdatedAt: &now,
	}
	require.NoError(t, db.Create(&member).Error)
}

// createTestProjectTeam создаёт связь проект-команда
func createTestProjectTeam(t *testing.T, db *gorm.DB, projectID, teamID uuid.UUID) {
	now := time.Now()
	del := false

	pt := models.ProjectTeam{
		ProjectID: projectID,
		TeamID:    teamID,
		Deleted:   &del,
		CreatedAt: &now,
		UpdatedAt: &now,
	}
	require.NoError(t, db.Create(&pt).Error)
}
