package httpapi

import (
	"emplacc-api/internal/dto/request"
	"emplacc-api/internal/dto/response"
	"emplacc-api/internal/service"
	"emplacc-api/internal/utils"
	"log"
	"net/http"
	"strconv"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
)

type SubscriptionController struct {
	subscriptionService service.SubscriptionService
}

func NewSubscriptionController(subscriptionService service.SubscriptionService) *SubscriptionController {
	return &SubscriptionController{
		subscriptionService: subscriptionService,
	}
}

func RegisterSubscriptionRoutes(e Router, subscriptionService service.SubscriptionService) {
	controller := NewSubscriptionController(subscriptionService)
	subscriptionGroup := e.Group("/subscription")
	{
		subscriptionGroup.GET("/all/:page/:pagesize", controller.GetAllSubscriptions)
		subscriptionGroup.GET("/:id", controller.GetSubscriptionById)
		subscriptionGroup.GET("/user/:id/:page/:pagesize", controller.GetSubscriptionsByUserId)
		subscriptionGroup.GET("/sub-object/:id/:type/:page/:pagesize", controller.GetSubscriptionBySubObject)
		subscriptionGroup.POST("", controller.CreateSubscription)
		subscriptionGroup.DELETE("/:id", controller.DeleteSubscription)
	}
}

// GetAllSubscriptions godoc
// @Summary Получение всех подписок
// @Description Получение списка всех подписок с пагинацией (deleted = false)
// @Tags Subscriptions
// @Accept json
// @Produce json
// @Param page path int true "Номер страницы"
// @Param pagesize path int true "Размер страницы"
// @Security BearerAuth
// @Failure 400 {object} map[string]string "Ошибка при парсинге параметров"
// @Failure 401 {object} map[string]string "Нет или неверный токен"
// @Failure 500 {object} map[string]string "Ошибка сервера при получении подписок"
// @Success 200 {object} response.SubscriptionListResponse "Список подписок успешно получен"
// @Router /subscription/all/{page}/{pagesize} [get]
func (sc *SubscriptionController) GetAllSubscriptions(c echo.Context) error {
	pageReq := c.Param("page")
	pageSizeReq := c.Param("pagesize")

	page, err := strconv.Atoi(pageReq)
	if err != nil || page <= 0 {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Ошибка при парсинге страницы"})
	}
	pageSize, err := strconv.Atoi(pageSizeReq)
	if err != nil || pageSize <= 0 {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Ошибка при парсинге размера страницы"})
	}

	subs, totalCount, err := sc.subscriptionService.GetAllSubscriptions(page, pageSize)
	if err != nil {
		log.Printf("service error (get all subscriptions): %v", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Ошибка при подсчёте подписок"})
	}

	out := response.SubscriptionListResponse{
		Page:       page,
		PageSize:   pageSize,
		TotalCount: totalCount,
	}
	for _, s := range subs {
		out.Subscriptions = append(out.Subscriptions, response.SubscriptionResponse{
			ID:             s.ID.String(),
			UserId:         s.UserID.String(),
			SubscriptionId: utils.GetUUIDString(s.SubscriptionId),
			TypeId:         utils.GetInt8(s.TypeID),
			CreatedAt:      utils.GetTime(s.CreatedAt),
		})
	}

	return c.JSON(http.StatusOK, out)
}

// GetSubscriptionsByUserId godoc
// @Summary Получение подписок по ID пользователя
// @Description Получение списка подписок для конкретного пользователя с пагинацией (deleted = false)
// @Tags Subscriptions
// @Accept json
// @Produce json
// @Param page path int true "Номер страницы"
// @Param id path string true "user_id"
// @Param pagesize path int true "Размер страницы"
// @Security BearerAuth
// @Failure 400 {object} map[string]string "Ошибка при парсинге параметров или данных запроса"
// @Failure 401 {object} map[string]string "Нет или неверный токен"
// @Failure 500 {object} map[string]string "Ошибка сервера при получении подписок"
// @Success 200 {object} response.SubscriptionListByUserIdResponse "Список подписок успешно получен"
// @Router /subscription/user/{id}/{page}/{pagesize} [get]
func (sc *SubscriptionController) GetSubscriptionsByUserId(c echo.Context) error {
	userUUID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Некорректный идентификатор подписки"})
	}
	// IDOR-защита: список подписок можно смотреть только по своему user_id.
	if callerID, _ := c.Get("user_id").(string); userUUID.String() != callerID {
		return c.JSON(http.StatusForbidden, map[string]string{"error": "Нет доступа к подпискам другого пользователя"})
	}
	page, err := strconv.Atoi(c.Param("page"))
	if err != nil || page <= 0 {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Ошибка при парсинге страницы"})
	}
	pageSize, err := strconv.Atoi(c.Param("pagesize"))
	if err != nil || pageSize <= 0 {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Ошибка при парсинге размера страницы"})
	}

	subs, totalCount, err := sc.subscriptionService.GetSubscriptionsByUserId(userUUID, page, pageSize)
	if err != nil {
		log.Printf("service error (get subscriptions by user): %v", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Ошибка при подсчёте подписок"})
	}

	out := response.SubscriptionListByUserIdResponse{
		UserId:     c.Param("id"),
		Page:       page,
		PageSize:   pageSize,
		TotalCount: totalCount,
	}
	for _, s := range subs {
		out.Subscriptions = append(out.Subscriptions, response.SubscriptionResponse{
			ID:             s.ID.String(),
			UserId:         s.UserID.String(),
			SubscriptionId: utils.GetUUIDString(s.SubscriptionId),
			TypeId:         utils.GetInt8(s.TypeID),
			CreatedAt:      utils.GetTime(s.CreatedAt),
		})
	}

	return c.JSON(http.StatusOK, out)
}

// GetSubscriptionBySubObject godoc
// @Summary Получение подписок по объекту подписки
// @Description Получение списка подписок для конкретного объекта подписки и типа с пагинацией (deleted = false)
// @Tags Subscriptions
// @Accept json
// @Produce json
// @Param page path int true "Номер страницы"
// @Param pagesize path int true "Размер страницы"
// @Param id path string true "UUID объекта подписки"
// @Param type path int true "Тип объекта подписки"
// @Security BearerAuth
// @Failure 400 {object} map[string]string "Ошибка при парсинге параметров или данных запроса"
// @Failure 401 {object} map[string]string "Нет или неверный токен"
// @Failure 500 {object} map[string]string "Ошибка сервера при получении подписок"
// @Success 200 {object} response.SubscriptionListBySubObjectResponse "Список подписок успешно получен"
// @Router /subscription/sub-object/{id}/{type}/{page}/{pagesize} [get]
func (sc *SubscriptionController) GetSubscriptionBySubObject(c echo.Context) error {
	subObjUUID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Некорректный идентификатор объекта"})
	}
	typeId, err := strconv.Atoi(c.Param("type"))
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Ошибка при парсинге типа объекта"})
	}
	page, err := strconv.Atoi(c.Param("page"))
	if err != nil || page <= 0 {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Ошибка при парсинге страницы"})
	}
	pageSize, err := strconv.Atoi(c.Param("pagesize"))
	if err != nil || pageSize <= 0 {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Ошибка при парсинге размера страницы"})
	}

	subs, totalCount, err := sc.subscriptionService.GetSubscriptionBySubObject(subObjUUID, int8(typeId), page, pageSize)
	if err != nil {
		log.Printf("service error (get subscription by sub-object): %v", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Ошибка при подсчёте подписок"})
	}

	out := response.SubscriptionListBySubObjectResponse{
		SubscriptionId: c.Param("id"),
		TypeId:         int8(typeId),
		Page:           page,
		PageSize:       pageSize,
		TotalCount:     totalCount,
	}
	for _, s := range subs {
		out.Subscriptions = append(out.Subscriptions, response.SubscriptionResponse{
			ID:             s.ID.String(),
			UserId:         s.UserID.String(),
			SubscriptionId: utils.GetUUIDString(s.SubscriptionId),
			TypeId:         utils.GetInt8(s.TypeID),
			CreatedAt:      utils.GetTime(s.CreatedAt),
		})
	}

	return c.JSON(http.StatusOK, out)
}

// GetSubscriptionById godoc
// @Summary Получение подписки по ID
// @Description Получение детальной информации о подписке по её ID (deleted = false)
// @Tags Subscriptions
// @Accept json
// @Produce json
// @Param id path string true "ID подписки"
// @Security BearerAuth
// @Failure 400 {object} map[string]string "Некорректный ID подписки"
// @Failure 401 {object} map[string]string "Нет или неверный токен"
// @Failure 404 {object} map[string]string "Подписка не найдена"
// @Failure 500 {object} map[string]string "Ошибка сервера при получении подписки"
// @Success 200 {object} response.SubscriptionResponse "Подписка успешно получена"
// @Router /subscription/{id} [get]
func (sc *SubscriptionController) GetSubscriptionById(c echo.Context) error {
	subId, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Некорректный идентификатор подписки"})
	}

	subscription, err := sc.subscriptionService.GetSubscriptionById(subId)
	if err != nil {
		if err.Error() == "subscription not found" {
			return c.JSON(http.StatusNotFound, map[string]string{"error": "Подписка не найдена"})
		}
		log.Printf("service error (get subscription by id): %v", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Ошибка при получении подписки"})
	}

	// IDOR-защита: видеть можно только свою подписку.
	if callerID, _ := c.Get("user_id").(string); subscription.UserID.String() != callerID {
		return c.JSON(http.StatusForbidden, map[string]string{"error": "Нет доступа к этой подписке"})
	}

	return c.JSON(http.StatusOK, response.SubscriptionResponse{
		ID:             subscription.ID.String(),
		UserId:         subscription.UserID.String(),
		SubscriptionId: utils.GetUUIDString(subscription.SubscriptionId),
		TypeId:         utils.GetInt8(subscription.TypeID),
		CreatedAt:      utils.GetTime(subscription.CreatedAt),
	})
}

// CreateSubscription godoc
// @Summary Создание подписки
// @Description Создание новой подписки с указанными данными
// @Tags Subscriptions
// @Accept json
// @Produce json
// @Param body body request.SubscriptionCreateRequest true "Данные для создания подписки"
// @Security BearerAuth
// @Failure 400 {object} map[string]string "Ошибка при парсинге данных запроса"
// @Failure 401 {object} map[string]string "Нет или неверный токен"
// @Failure 500 {object} map[string]string "Ошибка сервера при создании подписки"
// @Success 201 {object} response.SubscriptionUniversalResponse "Подписка успешно создана"
// @Router /subscription [post]
func (sc *SubscriptionController) CreateSubscription(c echo.Context) error {
	var req request.SubscriptionCreateRequest
	if err := c.Bind(&req); err != nil {
		log.Printf("Bind error: %v", err)
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Не удалось получить данные из запроса"})
	}

	// IDOR-защита: подписка всегда создаётся от имени владельца токена,
	// user_id из тела игнорируется (иначе можно подписать любого пользователя).
	callerID, _ := c.Get("user_id").(string)
	if callerID == "" {
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
	}
	req.UserId = callerID

	subID, err := sc.subscriptionService.CreateSubscription(req)
	if err != nil {
		if err.Error() == "invalid user id" {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "Некорректный идентификатор пользователя"})
		}
		if err.Error() == "invalid subscription id" {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "Некорректный идентификатор подписки"})
		}
		if err.Error() == "task not found" {
			return c.JSON(http.StatusNotFound, map[string]string{"error": "Задача не найдена"})
		}
		if err.Error() == "problem not found" {
			return c.JSON(http.StatusNotFound, map[string]string{"error": "Проблема не найдена"})
		}
		if err.Error() == "invalid type object" {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "Некорректный тип объекта"})
		}
		log.Printf("service error (create subscription): %v", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Ошибка при создании подписки"})
	}

	return c.JSON(http.StatusCreated, response.SubscriptionUniversalResponse{
		ID:      subID.String(),
		Message: "Подписка создана",
	})
}

// DeleteSubscription godoc
// @Summary Удаление подписки
// @Description Логическое удаление подписки по ID (поле deleted = true)
// @Tags Subscriptions
// @Accept json
// @Produce json
// @Param id path string true "ID подписки"
// @Security BearerAuth
// @Failure 401 {object} map[string]string "Нет или неверный токен"
// @Success 200 {object} response.SubscriptionUniversalResponse "Подписка успешно удалена"
// @Failure 404 {object} map[string]string "Подписка не найдена"
// @Failure 500 {object} map[string]string "Ошибка сервера при удалении подписки"
// @Router /subscription/{id} [delete]
func (sc *SubscriptionController) DeleteSubscription(c echo.Context) error {
	subUUID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Некорректный идентификатор подписки"})
	}

	// IDOR-защита: удалять можно только свою подписку.
	callerID, _ := c.Get("user_id").(string)
	existing, err := sc.subscriptionService.GetSubscriptionById(subUUID)
	if err != nil {
		if err.Error() == "subscription not found" {
			return c.JSON(http.StatusNotFound, map[string]string{"message": "Ничего не удалено"})
		}
		log.Printf("service error (get subscription for delete): %v", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Ошибка при удалении подписки"})
	}
	if existing.UserID.String() != callerID {
		return c.JSON(http.StatusForbidden, map[string]string{"error": "Можно удалять только свои подписки"})
	}

	err = sc.subscriptionService.DeleteSubscription(subUUID)
	if err != nil {
		if err.Error() == "subscription not found" {
			return c.JSON(http.StatusNotFound, map[string]string{"message": "Ничего не удалено"})
		}
		log.Printf("service error (delete subscription): %v", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Ошибка при удалении подписки"})
	}

	return c.JSON(http.StatusOK, response.SubscriptionUniversalResponse{
		ID:      subUUID.String(),
		Message: "Подписка удалена",
	})
}
