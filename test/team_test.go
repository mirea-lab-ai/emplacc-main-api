// team_test.go
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

func TestTeam_FullCRUD(t *testing.T) {
	testDB := setupTestDB(t)

	// Создаем зависимости для новой архитектуры
	teamRepo := postgres.NewTeamRepository(testDB)
	teamService := service.NewTeamService(teamRepo)
	teamController := httpapi.NewTeamController(teamService, testFreshAvatarURL)

	e := echo.New()

	// Создаём тестовые сущности
	userID := createTestUser(t, testDB, "user@example.com")
	projectID := createTestProject(t, testDB, "Test Project")

	var teamID uuid.UUID

	// === 1. CreateTeam ===
	t.Run("createTeam", func(t *testing.T) {
		reqBody := request.TeamCreateRequest{
			Name:        "Test Team",
			Description: "This is a test team",
			UsersIDs:    []string{}, // можно и пустой массив
		}

		body, _ := json.Marshal(reqBody)
		req := httptest.NewRequest(http.MethodPost, "/team", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)

		err := teamController.CreateTeam(c)
		assert.NoError(t, err)
		assert.Equal(t, http.StatusCreated, rec.Code)

		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
		assert.Contains(t, resp, "id")
		assert.NotEmpty(t, resp["id"])
		assert.Equal(t, "Команда создана", resp["message"])

		teamID = uuid.MustParse(resp["id"].(string))
	})

	// === 2. GetTeamByID ===
	t.Run("getTeamById", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/team/%s", teamID), nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.SetParamNames("id")
		c.SetParamValues(teamID.String())

		err := teamController.GetTeamByID(c)
		assert.NoError(t, err)
		assert.Equal(t, http.StatusOK, rec.Code)

		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
		assert.Equal(t, teamID.String(), resp["id"])
		assert.Equal(t, "Test Team", resp["name"])
		assert.Equal(t, "This is a test team", resp["description"])
	})

	// === 3. AddUserToTeam ===
	t.Run("addUserToTeam", func(t *testing.T) {
		reqBody := request.TeamAddUsersRequest{ // ← Исправлено: Users (множественное число)
			TeamID:  teamID.String(),
			UserIDs: []string{userID.String()}, // ← массив
		}

		body, _ := json.Marshal(reqBody)
		req := httptest.NewRequest(http.MethodPost, "/team/user", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)

		err := teamController.AddUserToTeam(c)
		assert.NoError(t, err)
		assert.Equal(t, http.StatusOK, rec.Code)

		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
		assert.Equal(t, teamID.String(), resp["team_id"])
		assert.Equal(t, []interface{}{userID.String()}, resp["users_id"]) // ← массив
		assert.NotEmpty(t, resp["message"])
	})

	// === 4. GetTeams (проверяем, что команда с пользователем возвращается) ===
	t.Run("getTeams", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/team/all", nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)

		err := teamController.GetTeams(c)
		assert.NoError(t, err)
		assert.Equal(t, http.StatusOK, rec.Code)

		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))

		teams, ok := resp["teams"].([]interface{})
		assert.True(t, ok)
		assert.Greater(t, len(teams), 0)

		team := teams[0].(map[string]interface{})
		assert.Equal(t, teamID.String(), team["id"])

		members, ok := team["members"].([]interface{})
		assert.True(t, ok)
		assert.Greater(t, len(members), 0)

		firstMember := members[0].(map[string]interface{})
		assert.Equal(t, userID.String(), firstMember["user_id"])
	})

	// === 5. AddProjectToTeam ===
	t.Run("addProjectToTeam", func(t *testing.T) {
		reqBody := request.TeamAddProjectRequest{
			TeamID:    teamID.String(),
			ProjectID: projectID.String(),
		}

		body, _ := json.Marshal(reqBody)
		req := httptest.NewRequest(http.MethodPost, "/team/project", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)

		err := teamController.AddProjectToTeam(c)
		assert.NoError(t, err)
		assert.Equal(t, http.StatusOK, rec.Code)

		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
		assert.Equal(t, teamID.String(), resp["team_id"])
		assert.Equal(t, projectID.String(), resp["project_id"])
		assert.Equal(t, "Проект успешно привязан к команде", resp["message"])
	})

	// === 6. GetProjectTeams ===
	t.Run("getProjectTeams", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/team/project/%s", projectID), nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.SetParamNames("project_id")
		c.SetParamValues(projectID.String())

		err := teamController.GetProjectTeams(c)
		assert.NoError(t, err)
		assert.Equal(t, http.StatusOK, rec.Code)

		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))

		teams, ok := resp["teams"].([]interface{})
		assert.True(t, ok)
		assert.Greater(t, len(teams), 0)

		team := teams[0].(map[string]interface{})
		assert.Equal(t, teamID.String(), team["id"])
	})

	// === 7. UpdateTeam ===
	t.Run("updateTeam", func(t *testing.T) {
		newName := "Updated Team Name"
		newDescription := "Updated team description"

		reqBody := request.TeamUpdateRequest{
			Name:        &newName,
			Description: &newDescription,
		}

		body, _ := json.Marshal(reqBody)
		req := httptest.NewRequest(http.MethodPatch, fmt.Sprintf("/team/%s", teamID), bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.SetParamNames("id")
		c.SetParamValues(teamID.String())

		err := teamController.UpdateTeam(c)
		assert.NoError(t, err)
		assert.Equal(t, http.StatusOK, rec.Code)

		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
		assert.Equal(t, "Команда обновлена", resp["message"])
	})

	// === 8. DeleteUserFromTeam ===
	t.Run("deleteUserFromTeam", func(t *testing.T) {
		reqBody := request.TeamDeleteUserRequest{
			TeamID: teamID.String(),
			UserID: userID.String(),
		}

		body, _ := json.Marshal(reqBody)
		req := httptest.NewRequest(http.MethodDelete, "/team/user", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)

		err := teamController.DeleteUserFromTeam(c)
		assert.NoError(t, err)
		assert.Equal(t, http.StatusOK, rec.Code)

		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
		assert.Equal(t, teamID.String(), resp["team_id"])
		assert.Equal(t, userID.String(), resp["user_id"])
		assert.Equal(t, "Пользователь успешно удален из команды", resp["message"])

		// Проверим, что связь действительно удалена
		var deleted models.TeamMember
		require.NoError(t, testDB.Unscoped().Where("user_id = ? AND team_id = ?", userID, teamID).First(&deleted).Error)
		assert.True(t, *deleted.Deleted)
	})

	// === 9. DeleteProjectFromTeam ===
	t.Run("deleteProjectFromTeam", func(t *testing.T) {
		reqBody := request.TeamDeleteProjectRequest{
			TeamID:    teamID.String(),
			ProjectID: projectID.String(),
		}

		body, _ := json.Marshal(reqBody)
		req := httptest.NewRequest(http.MethodDelete, "/team/project", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)

		err := teamController.DeleteProjectFromTeam(c)
		assert.NoError(t, err)
		assert.Equal(t, http.StatusOK, rec.Code)

		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
		assert.Equal(t, teamID.String(), resp["team_id"])
		assert.Equal(t, projectID.String(), resp["project_id"])
		assert.Equal(t, "Проект успешно отвязан от команды", resp["message"])

		// Проверим, что связь действительно удалена
		var deleted models.ProjectTeam
		require.NoError(t, testDB.Unscoped().Where("project_id = ? AND team_id = ?", projectID, teamID).First(&deleted).Error)
		assert.True(t, *deleted.Deleted)
	})

	// === 10. DeleteTeam ===
	t.Run("deleteTeam", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/team/%s", teamID), nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.SetParamNames("id")
		c.SetParamValues(teamID.String())

		err := teamController.DeleteTeam(c)
		assert.NoError(t, err)
		assert.Equal(t, http.StatusOK, rec.Code)

		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
		assert.Equal(t, teamID.String(), resp["id"])
		assert.Equal(t, "Команда удалена", resp["message"])

		// Проверим, что команда действительно удалена
		var deleted models.Team
		require.NoError(t, testDB.Unscoped().First(&deleted, "id = ?", teamID).Error)
		assert.True(t, *deleted.Deleted)
	})
}
