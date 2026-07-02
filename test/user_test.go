// user_test.go
package test

import (
	"bytes"
	"encoding/json"
	"errors"
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

// createTestRole создаёт тестовую роль
func createTestRole(t *testing.T, db *gorm.DB, name string) uuid.UUID {
	roleID := uuid.New()
	now := time.Now()
	del := false

	role := models.Role{
		ID:          roleID,
		Name:        &name,
		Description: nil,
		CreatedAt:   &now,
		UpdatedAt:   &now,
		Deleted:     &del,
	}
	require.NoError(t, db.Create(&role).Error)
	return roleID
}

func TestCreateUser_10Users(t *testing.T) {
	testDB := setupTestDB(t)

	// Создаем зависимости для новой архитектуры
	userRepo := postgres.NewUserRepository(testDB)
	userService := service.NewUserService(userRepo)
	userController := httpapi.NewUserController(userService, testFreshAvatarURL)

	e := echo.New()

	const total = 10
	start := time.Now()
	defer func() {
		t.Logf("Created %d users via CreateUser in %v", total, time.Since(start))
	}()

	for i := 1; i <= total; i++ {
		email := fmt.Sprintf("user%d@test.com", i)
		firstName := fmt.Sprintf("First%d", i)
		lastName := fmt.Sprintf("Last%d", i)
		isActive := true
		emailVerified := i%2 == 0

		reqBody := request.UserCreateRequest{
			Email:         &email,
			FirstName:     &firstName,
			LastName:      &lastName,
			IsActive:      &isActive,
			EmailVerified: &emailVerified,
		}

		body, _ := json.Marshal(reqBody)
		req := httptest.NewRequest(http.MethodPost, "/user", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)

		// Используем метод контроллера вместо глобальной функции
		err := userController.CreateUser(c)
		assert.NoError(t, err)
		assert.Equal(t, http.StatusCreated, rec.Code)

		var resp map[string]string
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
		assert.Contains(t, resp, "id")
		assert.NotEmpty(t, resp["id"])

		// Проверка в БД
		userID, err := uuid.Parse(resp["id"])
		require.NoError(t, err)

		var user models.User
		require.NoError(t, testDB.First(&user, "id = ?", userID).Error)

		assert.Equal(t, email, user.Email)
		assert.Equal(t, firstName, user.FirstName)
		assert.Equal(t, lastName, user.LastName)
		assert.Equal(t, isActive, user.IsActive)
		assert.Equal(t, emailVerified, user.EmailVerified)
		assert.False(t, user.Deleted)
	}

	// Финальная проверка
	var count int64
	testDB.Model(&models.User{}).Count(&count)
	assert.Equal(t, int64(total), count, "Expected 10 users in DB")
}

func TestUser_FullCRUD(t *testing.T) {
	testDB := setupTestDB(t)

	// Создаем зависимости для новой архитектуры
	userRepo := postgres.NewUserRepository(testDB)
	userService := service.NewUserService(userRepo)
	userController := httpapi.NewUserController(userService, testFreshAvatarURL)

	e := echo.New()

	// Создаём тестового пользователя
	email := "user@example.com"
	firstName := "First"
	lastName := "Last"
	isActive := true
	emailVerified := true

	var userID uuid.UUID
	var currentEmail string // для хранения актуального email

	// === 1. CreateUser ===
	t.Run("createUser", func(t *testing.T) {
		reqBody := request.UserCreateRequest{
			Email:         &email,
			FirstName:     &firstName,
			LastName:      &lastName,
			IsActive:      &isActive,
			EmailVerified: &emailVerified,
		}

		body, _ := json.Marshal(reqBody)
		req := httptest.NewRequest(http.MethodPost, "/user", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)

		err := userController.CreateUser(c)
		assert.NoError(t, err)
		assert.Equal(t, http.StatusCreated, rec.Code)

		var resp map[string]string
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
		assert.Contains(t, resp, "id")
		assert.NotEmpty(t, resp["id"])
		assert.Equal(t, "Пользователь успешно создан", resp["message"])

		// Сохраняем ID пользователя
		userID = uuid.MustParse(resp["id"])
		currentEmail = email // сохраняем начальный email
	})

	// === 2. GetUserById ===
	t.Run("getUserById", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/user/%s", userID), nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.SetParamNames("id")
		c.SetParamValues(userID.String())

		err := userController.GetUserById(c)
		assert.NoError(t, err)
		assert.Equal(t, http.StatusOK, rec.Code)

		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
		assert.Equal(t, userID.String(), resp["id"])
		assert.Equal(t, currentEmail, resp["email"])
		assert.Equal(t, firstName, resp["first_name"])
		assert.Equal(t, lastName, resp["last_name"])
		assert.Equal(t, isActive, resp["is_active"])
		assert.Equal(t, emailVerified, resp["email_verified"])
	})

	// === 3. UpdateUser ===
	t.Run("updateUser", func(t *testing.T) {
		newEmail := "updated@example.com"
		newFirstName := "UpdatedFirst"

		reqBody := request.UpdateUserRequest{
			Email:     &newEmail,
			FirstName: &newFirstName,
		}

		body, _ := json.Marshal(reqBody)
		req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/user/%s", userID), bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.SetParamNames("id")
		c.SetParamValues(userID.String())

		err := userController.UpdateUser(c)
		assert.NoError(t, err)
		assert.Equal(t, http.StatusOK, rec.Code)

		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
		assert.Equal(t, userID.String(), resp["id"])
		assert.Contains(t, resp["message"], "обновлен")

		// Проверим в БД
		var updated models.User
		require.NoError(t, testDB.First(&updated, "id = ?", userID).Error)
		assert.Equal(t, newEmail, updated.Email)
		assert.Equal(t, newFirstName, updated.FirstName)

		// Обновляем текущий email
		currentEmail = newEmail
	})

	// === 4. GetAllUsers ===
	t.Run("getAllUsers", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/user/all/1/10", nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.SetParamNames("page", "pagesize")
		c.SetParamValues("1", "10")

		err := userController.GetAllUsers(c)
		assert.NoError(t, err)
		assert.Equal(t, http.StatusOK, rec.Code)

		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
		assert.Equal(t, float64(1), resp["page"])
		assert.Equal(t, float64(10), resp["page_size"])

		users, ok := resp["users"].([]interface{})
		assert.True(t, ok)
		assert.Greater(t, len(users), 0)
	})

	// === 5. AddUserRole ===
	t.Run("addUserRole", func(t *testing.T) {
		// Сначала создадим роль
		roleID := createTestRole(t, testDB, "Test Role")

		reqBody := request.AddRoleUserRequest{
			UserId:     userID.String(),
			RoleId:     roleID.String(),
			AssignerId: userID.String(), // используем того же пользователя как assigner
		}

		body, _ := json.Marshal(reqBody)
		req := httptest.NewRequest(http.MethodPost, "/user/role", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)

		err := userController.AddUserRole(c)
		assert.NoError(t, err)
		assert.Equal(t, http.StatusOK, rec.Code)

		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
		assert.Equal(t, userID.String(), resp["user_id"])
		assert.Equal(t, roleID.String(), resp["role_id"])
		assert.Equal(t, "Роль успешно добавлена", resp["message"])
	})

	// === 6. RemoveUserRole ===
	t.Run("removeUserRole", func(t *testing.T) {
		// Сначала получим роль пользователя
		var userRole models.UserRole
		require.NoError(t, testDB.Where("user_id = ? AND deleted = FALSE", userID).First(&userRole).Error)
		roleID := userRole.RoleID

		reqBody := request.RemoveRoleUserRequest{
			UserId: userID.String(),
			RoleId: roleID.String(),
		}

		body, _ := json.Marshal(reqBody)
		req := httptest.NewRequest(http.MethodDelete, "/user/role", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)

		err := userController.RemoveUserRole(c)
		assert.NoError(t, err)
		assert.Equal(t, http.StatusOK, rec.Code)

		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
		assert.Equal(t, userID.String(), resp["user_id"])
		assert.Equal(t, roleID.String(), resp["role_id"])
		assert.Equal(t, "Роль успешно удалена", resp["message"])

		// Проверим в БД
		var deletedUserRole models.UserRole
		require.NoError(t, testDB.Unscoped().Where("user_id = ? AND role_id = ?", userID, roleID).First(&deletedUserRole).Error)
		assert.True(t, *deletedUserRole.Deleted)
	})

	// === 7. BanUser ===
	t.Run("banUser", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/user/%s", userID), nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.SetParamNames("id")
		c.SetParamValues(userID.String())

		err := userController.BanUser(c)
		assert.NoError(t, err)
		assert.Equal(t, http.StatusOK, rec.Code)

		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
		assert.Equal(t, userID.String(), resp["id"])
		assert.Contains(t, resp["message"], "забанен")

		// Проверим в БД - пользователь должен быть помечен как удаленный
		var banned models.User
		result := testDB.Unscoped().First(&banned, "id = ?", userID)
		require.NoError(t, result.Error)
		assert.True(t, banned.Deleted)
		// Сохраняем email для восстановления
		currentEmail = banned.Email
	})

	// === 8. RestoreUser ===
	t.Run("restoreUser", func(t *testing.T) {
		// Проверим, что пользователь действительно забанен перед восстановлением
		var checkBanned models.User
		result := testDB.Unscoped().First(&checkBanned, "id = ? AND deleted = TRUE", userID)
		require.NoError(t, result.Error)
		assert.True(t, checkBanned.Deleted)
		// Проверяем email
		assert.Equal(t, currentEmail, checkBanned.Email)

		reqBody := request.RestoreUserRequest{
			Email: &currentEmail,
		}

		body, _ := json.Marshal(reqBody)
		req := httptest.NewRequest(http.MethodPost, "/user/restore", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)

		err := userController.RestoreUser(c)
		assert.NoError(t, err)
		assert.Equal(t, http.StatusOK, rec.Code)

		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
		assert.Equal(t, userID.String(), resp["id"])
		assert.Contains(t, resp["message"], "восстановлен")

		// Проверим в БД - пользователь должен быть восстановлен
		var restored models.User
		result = testDB.Unscoped().First(&restored, "id = ?", userID)
		require.NoError(t, result.Error)
		assert.False(t, restored.Deleted)
	})

	// === 9. DeleteUser (hard delete) ===
	t.Run("deleteUser", func(t *testing.T) {
		// Удалим сначала все связанные роли, чтобы избежать ошибки внешнего ключа
		err := testDB.Exec("DELETE FROM user_roles WHERE user_id = ?", userID).Error
		require.NoError(t, err)

		// Теперь можно выполнить hard delete
		req := httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/user/full-delete/%s", userID), nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.SetParamNames("id")
		c.SetParamValues(userID.String())

		err = userController.DeleteUser(c)
		assert.NoError(t, err)
		assert.Equal(t, http.StatusOK, rec.Code)

		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
		assert.Equal(t, userID.String(), resp["id"])
		assert.Contains(t, resp["message"], "удален")

		// Проверим в БД - записи не должно быть (hard delete)
		var user models.User
		result := testDB.Unscoped().First(&user, "id = ?", userID)
		assert.Error(t, result.Error)
		assert.True(t, errors.Is(result.Error, gorm.ErrRecordNotFound))
	})
}
