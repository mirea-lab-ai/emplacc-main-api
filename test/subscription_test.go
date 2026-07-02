// subscription_test.go
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

func TestSubscription_FullCRUD(t *testing.T) {
	testDB := setupTestDB(t)

	// Создаем зависимости для новой архитектуры
	subscriptionRepo := postgres.NewSubscriptionRepository(testDB)
	subscriptionService := service.NewSubscriptionService(subscriptionRepo)
	subscriptionController := httpapi.NewSubscriptionController(subscriptionService)

	e := echo.New()

	// Создаём тестовые сущности
	userID := createTestUser(t, testDB, "user@example.com")
	// Создадим задачу и проблему, чтобы протестировать типы 0 и 1 в CreateSubscription
	projectID := createTestProject(t, testDB, "Test Project")
	boardID := createTestBoard(t, testDB, projectID, "Test Board")
	statusID := createTestStatus(t, testDB, boardID, "To Do")
	taskID := createTestTask(t, testDB, statusID, userID, "Test Task")
	problemID := createTestProblem(t, testDB, "Test Problem", userID)

	var subscriptionID uuid.UUID

	// === 1. CreateSubscription (тип 0 - задача) ===
	t.Run("createSubscriptionTask", func(t *testing.T) {
		typeID := int8(0)

		reqBody := request.SubscriptionCreateRequest{
			UserId:         userID.String(),
			SubscriptionId: taskID.String(),
			TypeId:         &typeID,
		}

		body, _ := json.Marshal(reqBody)
		req := httptest.NewRequest(http.MethodPost, "/subscription", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.Set("user_id", userID.String())

		// Используем метод контроллера вместо глобальной функции
		err := subscriptionController.CreateSubscription(c)
		assert.NoError(t, err)
		assert.Equal(t, http.StatusCreated, rec.Code)

		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
		assert.Contains(t, resp, "id")
		assert.NotEmpty(t, resp["id"])
		assert.Equal(t, "Подписка создана", resp["message"])

		// Сохраняем ID подписки
		subscriptionID = uuid.MustParse(resp["id"].(string))
	})

	// === 2. CreateSubscription (тип 1 - проблема) ===
	t.Run("createSubscriptionProblem", func(t *testing.T) {
		typeID := int8(1)

		reqBody := request.SubscriptionCreateRequest{
			UserId:         userID.String(),
			SubscriptionId: problemID.String(),
			TypeId:         &typeID,
		}

		body, _ := json.Marshal(reqBody)
		req := httptest.NewRequest(http.MethodPost, "/subscription", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.Set("user_id", userID.String())

		err := subscriptionController.CreateSubscription(c)
		assert.NoError(t, err)
		assert.Equal(t, http.StatusCreated, rec.Code)

		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
		assert.Contains(t, resp, "id")
		assert.NotEmpty(t, resp["id"])
		assert.Equal(t, "Подписка создана", resp["message"])
	})

	// === 3. GetSubscriptionById ===
	t.Run("getSubscriptionById", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/subscription/%s", subscriptionID), nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.Set("user_id", userID.String())
		c.SetParamNames("id")
		c.SetParamValues(subscriptionID.String())

		err := subscriptionController.GetSubscriptionById(c)
		assert.NoError(t, err)
		assert.Equal(t, http.StatusOK, rec.Code)

		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
		assert.Equal(t, subscriptionID.String(), resp["id"])
		assert.Equal(t, userID.String(), resp["user_id"])
		assert.Equal(t, taskID.String(), resp["subscription_id"])
		assert.Equal(t, float64(0), resp["type_id"])
	})

	// === 4. GetSubscriptionsByUserId ===
	t.Run("getSubscriptionsByUserId", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/subscription/user/%s/1/10", userID), nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.Set("user_id", userID.String())
		c.SetParamNames("id", "page", "pagesize")
		c.SetParamValues(userID.String(), "1", "10")

		err := subscriptionController.GetSubscriptionsByUserId(c)
		assert.NoError(t, err)
		assert.Equal(t, http.StatusOK, rec.Code)

		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
		assert.Equal(t, userID.String(), resp["user_id"])
		assert.Equal(t, float64(1), resp["page"])
		assert.Equal(t, float64(10), resp["page_size"])

		subs, ok := resp["subscriptions"].([]interface{})
		assert.True(t, ok)
		assert.Greater(t, len(subs), 0)
	})

	// === 5. GetSubscriptionBySubObject (для задачи) ===
	t.Run("getSubscriptionBySubObjectTask", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/subscription/sub-object/%s/0/1/10", taskID), nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.Set("user_id", userID.String())
		c.SetParamNames("id", "type", "page", "pagesize")
		c.SetParamValues(taskID.String(), "0", "1", "10")

		err := subscriptionController.GetSubscriptionBySubObject(c)
		assert.NoError(t, err)
		assert.Equal(t, http.StatusOK, rec.Code)

		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
		assert.Equal(t, taskID.String(), resp["subscription_id"])
		assert.Equal(t, float64(0), resp["type_id"])
		assert.Equal(t, float64(1), resp["page"])
		assert.Equal(t, float64(10), resp["page_size"])

		subs, ok := resp["subscriptions"].([]interface{})
		assert.True(t, ok)
		assert.Greater(t, len(subs), 0)
	})

	// === 6. GetSubscriptionBySubObject (для проблемы) ===
	t.Run("getSubscriptionBySubObjectProblem", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/subscription/sub-object/%s/1/1/10", problemID), nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.Set("user_id", userID.String())
		c.SetParamNames("id", "type", "page", "pagesize")
		c.SetParamValues(problemID.String(), "1", "1", "10")

		err := subscriptionController.GetSubscriptionBySubObject(c)
		assert.NoError(t, err)
		assert.Equal(t, http.StatusOK, rec.Code)

		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
		assert.Equal(t, problemID.String(), resp["subscription_id"])
		assert.Equal(t, float64(1), resp["type_id"])
		assert.Equal(t, float64(1), resp["page"])
		assert.Equal(t, float64(10), resp["page_size"])

		subs, ok := resp["subscriptions"].([]interface{})
		assert.True(t, ok)
		assert.Greater(t, len(subs), 0)
	})

	// === 7. GetAllSubscriptions ===
	t.Run("getAllSubscriptions", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/subscription/all/1/10", nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.Set("user_id", userID.String())
		c.SetParamNames("page", "pagesize")
		c.SetParamValues("1", "10")

		err := subscriptionController.GetAllSubscriptions(c)
		assert.NoError(t, err)
		assert.Equal(t, http.StatusOK, rec.Code)

		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
		assert.Equal(t, float64(1), resp["page"])
		assert.Equal(t, float64(10), resp["page_size"])

		subs, ok := resp["subscriptions"].([]interface{})
		assert.True(t, ok)
		assert.Greater(t, len(subs), 0)
	})

	// === 8. DeleteSubscription ===
	t.Run("deleteSubscription", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/subscription/%s", subscriptionID), nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.Set("user_id", userID.String())
		c.SetParamNames("id")
		c.SetParamValues(subscriptionID.String())

		err := subscriptionController.DeleteSubscription(c)
		assert.NoError(t, err)
		assert.Equal(t, http.StatusOK, rec.Code)

		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
		assert.Equal(t, subscriptionID.String(), resp["id"])
		assert.Equal(t, "Подписка удалена", resp["message"])

		// Проверим, что подписка действительно удалена
		var deleted models.Subscription
		require.NoError(t, testDB.Unscoped().First(&deleted, "id = ?", subscriptionID).Error)
		assert.True(t, *deleted.Deleted)
	})
}
