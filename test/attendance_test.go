// attendance_integration_test.go
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

// Создаём тестового пользователя
func createTestUser(t *testing.T, db *gorm.DB, email string) uuid.UUID {
	userID := uuid.New()
	now := time.Now()
	user := models.User{
		ID:            userID,
		Email:         email,
		IsActive:      true,
		EmailVerified: true,
		FirstName:     "Test",
		LastName:      "User",
		LastLogin:     now,
		Deleted:       false,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	require.NoError(t, db.Create(&user).Error)
	return userID
}

func TestAttendance_FullCRUD(t *testing.T) {
	testDB := setupTestDB(t)

	// Создаем зависимости для новой архитектуры
	attendanceRepo := postgres.NewAttendanceRepository(testDB)
	attendanceService := service.NewAttendanceService(attendanceRepo)
	attendanceController := httpapi.NewAttendanceController(attendanceService)

	e := echo.New()

	// Создаём пользователя
	userID := createTestUser(t, testDB, "test@example.com")

	// === 1. createAttendance ===
	t.Run("createAttendance", func(t *testing.T) {
		now := time.Now()
		date := now.Truncate(24 * time.Hour)
		workdayHours := int16(9)
		commits := int16(10)
		mergeRequests := int16(3)
		codeReviews := int16(4)
		time1 := now.Add(-9 * time.Hour)
		time2 := now.Add(-8 * time.Hour)

		reqBody := request.AttendanceCreateRequest{
			UserId:        userID.String(),
			Date:          &date,
			WorkdayHours:  &workdayHours,
			PlannedStart:  &time1,
			ActualStart:   &time2,
			Commits:       &commits,
			MergeRequests: &mergeRequests,
			CodeReviews:   &codeReviews,
			EndWork:       &now,
		}
		body, _ := json.Marshal(reqBody)
		req := httptest.NewRequest(http.MethodPost, "/attendance", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.Set("user_id", userID.String())

		err := attendanceController.CreateAttendance(c)
		assert.NoError(t, err)
		assert.Equal(t, http.StatusCreated, rec.Code)

		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
		assert.Contains(t, resp, "id")
		assert.NotEmpty(t, resp["id"])
	})

	// === 2. getAttendancesByUserId ===
	t.Run("getAttendancesByUserId", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/attendance/user/%s", userID), nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.SetParamNames("id")
		c.SetParamValues(userID.String())

		err := attendanceController.GetAttendancesByUserId(c)
		assert.NoError(t, err)
		assert.Equal(t, http.StatusOK, rec.Code)

		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))

		// Адаптируем проверку под ваш формат ответа
		if userIDResp, exists := resp["user_id"]; exists {
			assert.Equal(t, userID.String(), userIDResp)
		}
		if attendances, exists := resp["attendances"]; exists {
			attendanceList, ok := attendances.([]interface{})
			assert.True(t, ok)
			assert.Greater(t, len(attendanceList), 0)
		}
	})

	// === 3. getAllAttendances ===
	t.Run("getAllAttendances", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/attendance/all/1/10", nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.SetParamNames("page", "pagesize")
		c.SetParamValues("1", "10")

		err := attendanceController.GetAllAttendances(c)
		assert.NoError(t, err)
		assert.Equal(t, http.StatusOK, rec.Code)

		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))

		// Проверяем структуру ответа
		if page, exists := resp["page"]; exists {
			assert.Equal(t, float64(1), page)
		}
		if pageSize, exists := resp["page_size"]; exists {
			assert.Equal(t, float64(10), pageSize)
		}
		if total, exists := resp["total_count"]; exists {
			totalCount, ok := total.(float64)
			assert.True(t, ok)
			assert.Greater(t, totalCount, float64(0))
		}
	})

	// Получим ID первого посещения для update/delete
	var firstAttendance models.Attendance
	require.NoError(t, testDB.Where("user_id = ? AND deleted = ?", userID, false).First(&firstAttendance).Error)
	attendanceID := firstAttendance.ID

	// === 4. updateAttendance ===
	t.Run("updateAttendance", func(t *testing.T) {
		newHours := int16(10)
		reqBody := request.AttendanceUpdateRequest{
			WorkdayHours: &newHours,
		}
		body, _ := json.Marshal(reqBody)
		req := httptest.NewRequest(http.MethodPatch, fmt.Sprintf("/attendance/%s", attendanceID), bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.SetParamNames("id")
		c.SetParamValues(attendanceID.String())

		err := attendanceController.UpdateAttendance(c)
		assert.NoError(t, err)
		assert.Equal(t, http.StatusOK, rec.Code)

		// Проверим в БД
		var updated models.Attendance
		require.NoError(t, testDB.Where("id = ? AND deleted = ?", attendanceID, false).First(&updated).Error)
		assert.Equal(t, newHours, *updated.WorkdayHours)
	})

	// === 5. deleteAttendance ===
	t.Run("deleteAttendance", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/attendance/%s", attendanceID), nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.SetParamNames("id")
		c.SetParamValues(attendanceID.String())

		err := attendanceController.DeleteAttendance(c)
		assert.NoError(t, err)
		assert.Equal(t, http.StatusOK, rec.Code)

		// Проверим, что deleted = true
		var deleted models.Attendance
		require.NoError(t, testDB.Unscoped().First(&deleted, "id = ?", attendanceID).Error)
		assert.True(t, *deleted.Deleted)
	})
}
