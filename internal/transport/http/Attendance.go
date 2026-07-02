package httpapi

import (
	"emplacc-api/internal/dto/request"
	"emplacc-api/internal/dto/response"
	"emplacc-api/internal/service"
	utils "emplacc-api/internal/utils"
	"log"
	"net/http"
	"strconv"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
)

type AttendanceController struct {
	attendanceService service.AttendanceService
}

func NewAttendanceController(attendanceService service.AttendanceService) *AttendanceController {
	return &AttendanceController{
		attendanceService: attendanceService,
	}
}

func RegisterAttendanceRoutes(e Router, attendanceService service.AttendanceService, employeeMw echo.MiddlewareFunc, managerMw echo.MiddlewareFunc) {
	controller := NewAttendanceController(attendanceService)
	g := e.Group("/attendance")
	g.GET("/all/:page/:pagesize", controller.GetAllAttendances)
	g.GET("/user/:id", controller.GetAttendancesByUserId)
	g.POST("", controller.CreateAttendance, employeeMw)
	g.PATCH("/:id", controller.UpdateAttendance, employeeMw)
	g.DELETE("/:id", controller.DeleteAttendance, managerMw)
}

// GetAllAttendances godoc
// @Summary Получение всех посещений
// @Description Получение списка всех посещений с пагинацией (логически не удаленных)
// @Tags Attendance
// @Accept json
// @Produce json
// @Param page path int true "Номер страницы"
// @Param pagesize path int true "Размер страницы"
// @Security BearerAuth
// @Success 200 {object} response.AttendancesListResponse "Список посещений успешно получен"
// @Failure 400 {object} map[string]string "Ошибка при парсинге параметров пагинации"
// @Failure 401 {object} map[string]string "Нет или неверный токен"
// @Failure 500 {object} map[string]string "Ошибка сервера при получении посещений"
// @Router /attendance/all/{page}/{pagesize} [get]
func (ac *AttendanceController) GetAllAttendances(c echo.Context) error {
	page, err := strconv.Atoi(c.Param("page"))
	if err != nil || page <= 0 {
		log.Printf("failed to parse page: %v", err)
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Ошибка при парсинге страницы"})
	}
	pageSize, err := strconv.Atoi(c.Param("pagesize"))
	if err != nil || pageSize <= 0 {
		log.Printf("failed to parse pagesize: %v", err)
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Ошибка при парсинге размера страницы"})
	}

	attendances, totalCount, err := ac.attendanceService.GetAllAttendances(page, pageSize)
	if err != nil {
		log.Printf("service error (get all attendances): %v", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Ошибка при получении посещений из базы данных"})
	}

	resp := response.AttendancesListResponse{
		Page:       page,
		PageSize:   pageSize,
		TotalCount: totalCount,
	}

	for _, a := range attendances {
		resp.Attendances = append(resp.Attendances, response.AttendanceResponse{
			ID:            a.ID.String(),
			UserID:        a.UserID.String(),
			Date:          utils.GetTime(a.Date),
			WorkdayHours:  utils.GetInt16(a.WorkdayHours),
			PlannedStart:  utils.GetTime(a.PlannedStart),
			ActualStart:   utils.GetTime(a.ActualStart),
			Commits:       utils.GetInt16(a.Commits),
			MergeRequests: utils.GetInt16(a.MergeRequests),
			CodeReviews:   utils.GetInt16(a.CodeReviews),
			EndWork:       utils.GetTime(a.EndWork),
			UpdatedAt:     utils.GetTime(a.UpdatedAt),
			CreatedAt:     utils.GetTime(a.CreatedAt),
		})
	}
	return c.JSON(http.StatusOK, resp)
}

// GetAttendancesByUserId godoc
// @Summary Получение посещений по ID пользователя
// @Description Получение списка посещений для конкретного пользователя (логически не удаленных)
// @Tags Attendance
// @Accept json
// @Produce json
// @Param id path string true "ID пользователя"
// @Security BearerAuth
// @Success 200 {object} response.AttendancesByUserId "Список посещений пользователя успешно получен"
// @Failure 400 {object} map[string]string "Некорректный идентификатор пользователя"
// @Failure 401 {object} map[string]string "Нет или неверный токен"
// @Failure 500 {object} map[string]string "Ошибка сервера при получении посещений"
// @Router /attendance/user/{id} [get]
func (ac *AttendanceController) GetAttendancesByUserId(c echo.Context) error {
	userID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		log.Printf("UUID parse error: %v", err)
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Некорректный идентификатор пользователя"})
	}

	attendances, err := ac.attendanceService.GetAttendancesByUserId(userID)
	if err != nil {
		log.Printf("service error (get attendances by user): %v", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Ошибка при получении посещений из базы данных"})
	}

	resp := response.AttendancesByUserId{UserID: userID.String()}
	for _, a := range attendances {
		resp.Attendances = append(resp.Attendances, response.AttendanceResponse{
			ID:            a.ID.String(),
			UserID:        a.UserID.String(),
			Date:          utils.GetTime(a.Date),
			WorkdayHours:  utils.GetInt16(a.WorkdayHours),
			PlannedStart:  utils.GetTime(a.PlannedStart),
			ActualStart:   utils.GetTime(a.ActualStart),
			Commits:       utils.GetInt16(a.Commits),
			MergeRequests: utils.GetInt16(a.MergeRequests),
			CodeReviews:   utils.GetInt16(a.CodeReviews),
			EndWork:       utils.GetTime(a.EndWork),
			UpdatedAt:     utils.GetTime(a.UpdatedAt),
			CreatedAt:     utils.GetTime(a.CreatedAt),
		})
	}
	return c.JSON(http.StatusOK, resp)
}

// CreateAttendance godoc
// @Summary Создание посещения
// @Description Создание нового посещения
// @Tags Attendance
// @Accept json
// @Produce json
// @Param body body request.AttendanceCreateRequest true "Данные для создания посещения"
// @Security BearerAuth
// @Success 201 {object} response.AttendanceUniversalResponse "Посещение успешно создано"
// @Failure 400 {object} map[string]string "Некорректные данные запроса"
// @Failure 401 {object} map[string]string "Нет или неверный токен"
// @Failure 500 {object} map[string]string "Ошибка сервера при создании посещения"
// @Router /attendance [post]
func (ac *AttendanceController) CreateAttendance(c echo.Context) error {
	var req request.AttendanceCreateRequest
	if err := c.Bind(&req); err != nil {
		log.Printf("Bind error: %v", err)
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Не удалось получить данные из запроса"})
	}

	// IDOR-защита: обычный сотрудник может создавать посещения только для себя.
	// UserId из тела игнорируется и берётся из токена; менеджер/админ может указать чужого.
	callerID, _ := c.Get("user_id").(string)
	callerRole, _ := c.Get("user_role").(string)
	if callerRole != "manager" && callerRole != "admin" {
		req.UserId = callerID
	}

	attendanceID, err := ac.attendanceService.CreateAttendance(req)
	if err != nil {
		log.Printf("service error (create attendance): %v", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Ошибка при создании посещения"})
	}

	return c.JSON(http.StatusCreated, response.AttendanceUniversalResponse{
		ID:      attendanceID.String(),
		Message: "Attendance created",
	})
}

// UpdateAttendance godoc
// @Summary Обновление посещения
// @Description Обновление полей посещения по ID
// @Tags Attendance
// @Accept json
// @Produce json
// @Param id path string true "ID посещения"
// @Param body body request.AttendanceUpdateRequest true "Данные для обновления посещения"
// @Security BearerAuth
// @Success 200 {object} response.AttendanceUniversalResponse "Посещение успешно обновлено"
// @Failure 400 {object} map[string]string "Некорректные данные запроса или ID"
// @Failure 404 {object} map[string]string "Посещение не найдено"
// @Failure 401 {object} map[string]string "Нет или неверный токен"
// @Failure 500 {object} map[string]string "Ошибка сервера при обновлении посещения"
// @Router /attendance/{id} [patch]
func (ac *AttendanceController) UpdateAttendance(c echo.Context) error {
	attendanceID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		log.Printf("UUID parse error: %v", err)
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Некорректный идентификатор посещения"})
	}

	var req request.AttendanceUpdateRequest
	if err := c.Bind(&req); err != nil {
		log.Printf("Bind error: %v", err)
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Не удалось получить данные из запроса"})
	}

	err = ac.attendanceService.UpdateAttendance(attendanceID, req)
	if err != nil {
		if err.Error() == "Attendance not found" {
			return c.JSON(http.StatusNotFound, map[string]string{"error": "Посещение не найдено или уже удалено"})
		}
		log.Printf("service error (update attendance): %v", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Ошибка при обновлении посещения"})
	}

	return c.JSON(http.StatusOK, response.AttendanceUniversalResponse{
		ID:      attendanceID.String(),
		Message: "Посещение обновлено",
	})
}

// DeleteAttendance godoc
// @Summary Удаление посещения
// @Description Логическое удаление посещения по ID (поле deleted = true)
// @Tags Attendance
// @Accept json
// @Produce json
// @Param id path string true "ID посещения"
// @Security BearerAuth
// @Success 200 {object} response.AttendanceUniversalResponse "Посещение успешно удалено"
// @Failure 400 {object} map[string]string "Некорректный ID"
// @Failure 404 {object} map[string]string "Посещение не найдено"
// @Failure 401 {object} map[string]string "Нет или неверный токен"
// @Failure 500 {object} map[string]string "Ошибка сервера при удалении посещения"
// @Router /attendance/{id} [delete]
func (ac *AttendanceController) DeleteAttendance(c echo.Context) error {
	attendanceID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		log.Printf("UUID parse error: %v", err)
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Некорректный идентификатор посещения"})
	}

	err = ac.attendanceService.DeleteAttendance(attendanceID)
	if err != nil {
		if err.Error() == "Attendance not found" {
			return c.JSON(http.StatusNotFound, map[string]string{"error": "Посещение не найдено или уже удалено"})
		}
		log.Printf("service error (delete attendance): %v", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Ошибка при удалении посещения"})
	}

	return c.JSON(http.StatusOK, response.AttendanceUniversalResponse{
		ID:      attendanceID.String(),
		Message: "Посещение удалено",
	})
}
