// role_test.go
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

func TestRole_FullCRUD(t *testing.T) {
	testDB := setupTestDB(t)

	// Создаем зависимости для новой архитектуры
	roleRepo := postgres.NewRoleRepository(testDB)
	roleService := service.NewRoleService(roleRepo)
	roleController := httpapi.NewRoleController(roleService)

	e := echo.New()

	var roleID uuid.UUID

	// === 1. CreateRole ===
	t.Run("createRole", func(t *testing.T) {
		name := "Test Role"
		description := "This is a test role"

		reqBody := request.RoleCreateRequest{
			Name:        &name,
			Description: &description,
		}

		body, _ := json.Marshal(reqBody)
		req := httptest.NewRequest(http.MethodPost, "/role", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)

		// Используем метод контроллера вместо глобальной функции
		err := roleController.CreateRole(c)
		assert.NoError(t, err)
		assert.Equal(t, http.StatusCreated, rec.Code)

		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
		assert.Contains(t, resp, "id")
		assert.NotEmpty(t, resp["id"])
		assert.Equal(t, "Роль успешно создана", resp["message"])

		// Сохраняем ID роли
		roleID = uuid.MustParse(resp["id"].(string))
	})

	// === 2. GetRoleById ===
	t.Run("getRoleById", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/role/%s", roleID), nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.SetParamNames("id")
		c.SetParamValues(roleID.String())

		err := roleController.GetRoleById(c)
		assert.NoError(t, err)
		assert.Equal(t, http.StatusOK, rec.Code)

		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
		assert.Equal(t, roleID.String(), resp["id"])
		assert.Equal(t, "Test Role", resp["name"])
		assert.Equal(t, "This is a test role", resp["description"])
	})

	// === 3. UpdateRole ===
	t.Run("updateRole", func(t *testing.T) {
		newName := "Updated Role Name"
		newDescription := "Updated role description"

		reqBody := request.RoleUpdateRequest{
			Name:        &newName,
			Description: &newDescription,
		}

		body, _ := json.Marshal(reqBody)
		req := httptest.NewRequest(http.MethodPatch, fmt.Sprintf("/role/%s", roleID), bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.SetParamNames("id")
		c.SetParamValues(roleID.String())

		err := roleController.UpdateRole(c)
		assert.NoError(t, err)
		assert.Equal(t, http.StatusOK, rec.Code)

		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
		assert.Equal(t, "Роль успешно обновлена", resp["message"])
	})

	// === 4. GetAllRoles ===
	t.Run("getAllRoles", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/role/all/1/10", nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.SetParamNames("page", "pagesize")
		c.SetParamValues("1", "10")

		err := roleController.GetAllRoles(c)
		assert.NoError(t, err)
		assert.Equal(t, http.StatusOK, rec.Code)

		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
		assert.Equal(t, float64(1), resp["page"])
		assert.Equal(t, float64(10), resp["page_size"])

		roles, ok := resp["roles"].([]interface{})
		assert.True(t, ok)
		assert.Greater(t, len(roles), 0)
	})

	// === 5. DeleteRole ===
	t.Run("deleteRole", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/role/%s", roleID), nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.SetParamNames("id")
		c.SetParamValues(roleID.String())

		err := roleController.DeleteRole(c)
		assert.NoError(t, err)
		assert.Equal(t, http.StatusOK, rec.Code)

		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
		assert.Equal(t, roleID.String(), resp["id"])
		assert.Equal(t, "Роль удалена", resp["message"])

		// Проверим, что роль действительно удалена
		var deleted models.Role
		require.NoError(t, testDB.Unscoped().First(&deleted, "id = ?", roleID).Error)
		assert.True(t, *deleted.Deleted)
	})
}
