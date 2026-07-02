package httpapi

import (
	"emplacc-api/internal/dto/request"
	"emplacc-api/internal/dto/response"
	"emplacc-api/internal/service"
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
)

type UserController struct {
	userService    service.UserService
	freshAvatarURL func(string) string
}

func NewUserController(userService service.UserService, freshAvatarURL func(string) string) *UserController {
	return &UserController{
		userService:    userService,
		freshAvatarURL: freshAvatarURL,
	}
}

func RegisterUserRoutes(e Router, userService service.UserService, freshAvatarURL func(string) string, adminMw echo.MiddlewareFunc) {
	controller := NewUserController(userService, freshAvatarURL)
	userGroup := e.Group("/user")

	// Read: любой авторизованный пользователь
	userGroup.GET("/all/:page/:pagesize", controller.GetAllUsers)
	userGroup.GET("/search", controller.SearchUsers)
	userGroup.GET("/me", controller.GetCurrentUser)
	userGroup.GET("/:id", controller.GetUserById)

	// Write: только admin
	userGroup.POST("", controller.CreateUser, adminMw)
	userGroup.POST("/:id", controller.UpdateUser, adminMw)
	userGroup.DELETE("/:id", controller.BanUser, adminMw)
	userGroup.POST("/restore", controller.RestoreUser, adminMw)
	userGroup.POST("/role", controller.AddUserRole, adminMw)
	userGroup.DELETE("/role", controller.RemoveUserRole, adminMw)
	userGroup.DELETE("/full-delete/:id", controller.DeleteUser, adminMw)
}

// GetAllUsers godoc
// @Summary Получение списка всех пользователей
// @Description Получает список всех пользователей с учетом пагинации, исключая удаленных
// @Tags Users
// @Accept json
// @Produce json
// @Param page path int true "Номер страницы"
// @Param pagesize path int true "Размер страницы"
// @Security BearerAuth
// @Failure 401 {object} map[string]string "Нет или неверный токен"
// @Success 200 {object} response.GetAllUsersResponse "Список пользователей успешно получен"
// @Failure 400 {object} map[string]string "Ошибка в запросе"
// @Failure 500 {object} map[string]string "Ошибка сервера при получении пользователей"
// @Router /user/all/{page}/{pagesize} [get]
func (uc *UserController) GetAllUsers(c echo.Context) error {
	pageReq := c.Param("page")
	pageSizeReq := c.Param("pagesize")

	page, err := strconv.Atoi(pageReq)
	if err != nil {
		log.Printf("failed to parse page: %v", err)
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "Ошибка при парсинге страницы",
		})
	}
	if page <= 0 {
		page = 1
	}

	pageSize, err := strconv.Atoi(pageSizeReq)
	if err != nil {
		log.Printf("failed to parse pagesize: %v", err)
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "Ошибка при парсинге номера страницы",
		})
	}
	if pageSize <= 0 {
		pageSize = 10
	}

	users, totalCount, err := uc.userService.GetAllUsers(page, pageSize)
	if err != nil {
		log.Printf("service error (get all users): %v", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": "Ошибка при подсчете пользователей",
		})
	}

	userList := response.GetAllUsersResponse{
		Page:       page,
		PageSize:   pageSize,
		TotalCount: totalCount,
	}

	for _, user := range users {
		userList.Users = append(userList.Users, response.GetUserResponse{
			ID:            user.ID.String(),
			Email:         user.Email,
			IsActive:      user.IsActive,
			CreatedAt:     user.CreatedAt,
			TgId:          user.TgID,
			TgUserId:      user.TgUserID,
			Profession:    user.Profession,
			EmailVerified: user.EmailVerified,
			FirstName:     user.FirstName,
			LastName:      user.LastName,
			LastLogin:     user.LastLogin,
			AvatarURL:     uc.freshAvatarURL(user.AvatarURL),
		})
	}
	return c.JSON(http.StatusOK, userList)
}

// SearchUsers godoc
// @Summary Поиск пользователей
// @Description Поиск по имени, фамилии, email или профессии с пагинацией
// @Tags Users
// @Accept json
// @Produce json
// @Param query query string true "Поисковый запрос"
// @Param page query int true "Номер страницы"
// @Param pagesize query int true "Размер страницы"
// @Security BearerAuth
// @Success 200 {object} response.GetAllUsersResponse "Список найденных пользователей"
// @Failure 400 {object} map[string]string "Ошибка в запросе"
// @Failure 500 {object} map[string]string "Ошибка сервера при поиске"
// @Router /user/search [get]
func (uc *UserController) SearchUsers(c echo.Context) error {
	query := strings.TrimSpace(c.QueryParam("query"))
	if query == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Поисковый запрос не может быть пустым"})
	}
	page, err := strconv.Atoi(c.QueryParam("page"))
	if err != nil || page <= 0 {
		page = 1
	}
	pageSize, err := strconv.Atoi(c.QueryParam("pagesize"))
	if err != nil || pageSize <= 0 || pageSize > 100 {
		pageSize = 10
	}

	users, totalCount, err := uc.userService.SearchUsers(query, page, pageSize)
	if err != nil {
		log.Printf("service error (search users): %v", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Ошибка при поиске пользователей"})
	}

	userList := response.GetAllUsersResponse{
		Page:       page,
		PageSize:   pageSize,
		TotalCount: totalCount,
		Users:      make([]response.GetUserResponse, 0, len(users)),
	}
	for _, user := range users {
		userList.Users = append(userList.Users, response.GetUserResponse{
			ID:            user.ID.String(),
			Email:         user.Email,
			IsActive:      user.IsActive,
			CreatedAt:     user.CreatedAt,
			TgId:          user.TgID,
			TgUserId:      user.TgUserID,
			Profession:    user.Profession,
			EmailVerified: user.EmailVerified,
			FirstName:     user.FirstName,
			LastName:      user.LastName,
			LastLogin:     user.LastLogin,
			AvatarURL:     uc.freshAvatarURL(user.AvatarURL),
		})
	}
	return c.JSON(http.StatusOK, userList)
}

// GetUserById godoc
// @Summary Получение пользователя по ID
// @Description Получает данные пользователя по его уникальному идентификатору
// @Tags Users
// @Accept json
// @Produce json
// @Param id path string true "ID пользователя"
// @Security BearerAuth
// @Failure 401 {object} map[string]string "Нет или неверный токен"
// @Success 200 {object} response.GetUserResponse "Пользователь успешно получен"
// @Failure 400 {object} map[string]string "Некорректный идентификатор пользователя"
// @Failure 404 {object} map[string]string "Пользователь не найден"
// @Failure 500 {object} map[string]string "Ошибка сервера при получении пользователя"
// @Router /user/{id} [get]
func (uc *UserController) GetUserById(c echo.Context) error {
	id := c.Param("id")
	userId, err := uuid.Parse(id)
	if err != nil {
		log.Printf("UUID parse error: %v", err)
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "Некорректный идентификатор пользователя",
		})
	}

	user, err := uc.userService.GetUserById(userId)
	if err != nil {
		if err.Error() == "user not found" {
			return c.JSON(http.StatusNotFound, map[string]string{
				"error": "Пользователь не найден",
			})
		}
		log.Printf("service error (get user by id): %v", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": "Ошибка при получении пользователя из базы данных",
		})
	}

	getUserResponse := response.GetUserResponse{
		ID:            user.ID.String(),
		Email:         user.Email,
		IsActive:      user.IsActive,
		CreatedAt:     user.CreatedAt,
		TgId:          user.TgID,
		TgUserId:      user.TgUserID,
		Profession:    user.Profession,
		EmailVerified: user.EmailVerified,
		FirstName:     user.FirstName,
		LastName:      user.LastName,
		LastLogin:     user.LastLogin,
		AvatarURL:     uc.freshAvatarURL(user.AvatarURL),
	}
	return c.JSON(http.StatusOK, getUserResponse)
}

// GetCurrentUser godoc
// @Summary Текущий пользователь
// @Description Возвращает данные пользователя из токена (sess_*, emplacc_*, JWT)
// @Tags Users
// @Produce json
// @Security BearerAuth
// @Success 200 {object} response.GetUserResponse
// @Failure 401 {object} map[string]string
// @Router /user/me [get]
func (uc *UserController) GetCurrentUser(c echo.Context) error {
	idStr, ok := c.Get("user_id").(string)
	if !ok || idStr == "" {
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
	}
	userId, err := uuid.Parse(idStr)
	if err != nil {
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
	}
	user, err := uc.userService.GetUserById(userId)
	if err != nil {
		if err.Error() == "user not found" {
			return c.JSON(http.StatusNotFound, map[string]string{"error": "Пользователь не найден"})
		}
		log.Printf("service error (get current user): %v", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Ошибка при получении пользователя"})
	}
	return c.JSON(http.StatusOK, response.GetUserResponse{
		ID:            user.ID.String(),
		Email:         user.Email,
		IsActive:      user.IsActive,
		CreatedAt:     user.CreatedAt,
		TgId:          user.TgID,
		TgUserId:      user.TgUserID,
		Profession:    user.Profession,
		EmailVerified: user.EmailVerified,
		FirstName:     user.FirstName,
		LastName:      user.LastName,
		LastLogin:     user.LastLogin,
		AvatarURL:     uc.freshAvatarURL(user.AvatarURL),
	})
}

// CreateUser godoc
// @Summary Создание нового пользователя
// @Description Создаёт пользователя с автоматически сгенерированным UUID
// @Tags Users
// @Accept json
// @Produce json
// @Param body body request.UserCreateRequest true "Данные для создания пользователя"
// @Security BearerAuth
// @Success 201 {object} map[string]string "Пользователь успешно создан"
// @Failure 400 {object} map[string]string "Ошибка при привязке данных"
// @Failure 401 {object} map[string]string "Нет или неверный токен"
// @Failure 500 {object} map[string]string "Ошибка при создании пользователя"
// @Router /user [post]
func (uc *UserController) CreateUser(c echo.Context) error {
	var req request.UserCreateRequest
	if err := c.Bind(&req); err != nil {
		log.Printf("Bind error: %v", err)
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "Не удалось получить данные из запроса",
		})
	}

	newUUID, err := uc.userService.CreateUser(req)
	if err != nil {
		log.Printf("service error (create user): %v", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": "Ошибка при создании пользователя",
		})
	}

	return c.JSON(http.StatusCreated, map[string]string{
		"message": "Пользователь успешно создан",
		"id":      newUUID.String(),
	})
}

// UpdateUser godoc
// @Summary Обновление пользователя
// @Description Обновляет данные пользователя по его ID
// @Tags Users
// @Accept json
// @Produce json
// @Param id path string true "ID пользователя"
// @Param user body request.UpdateUserRequest true "Данные для обновления пользователя"
// @Security BearerAuth
// @Failure 401 {object} map[string]string "Нет или неверный токен"
// @Success 200 {object} response.UserUniversalResponse "Пользователь успешно обновлен"
// @Failure 400 {object} map[string]string "Некорректный идентификатор или ошибка в запросе"
// @Failure 500 {object} map[string]string "Ошибка сервера при обновлении пользователя"
// @Router /user/{id} [post]
func (uc *UserController) UpdateUser(c echo.Context) error {
	id := c.Param("id")
	userId, err := uuid.Parse(id)
	if err != nil {
		log.Printf("UUID parse error: %v", err)
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "Некорректный идентификатор пользователя",
		})
	}

	var req request.UpdateUserRequest
	if err = c.Bind(&req); err != nil {
		log.Printf("Bind error: %v", err)
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "Не удалось получить данные из запроса",
		})
	}

	err = uc.userService.UpdateUser(userId, req)
	if err != nil {
		if err.Error() == "no fields to update" {
			return c.JSON(http.StatusBadRequest, map[string]string{
				"error": "Не указаны поля для обновления",
			})
		}
		if err.Error() == "user not found" {
			return c.JSON(http.StatusNotFound, map[string]string{"message": "Ничего не обновлено"})
		}
		log.Printf("service error (update user): %v", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Ошибка при обновлении пользователя"})
	}

	updateResponse := response.UserUniversalResponse{
		ID:      userId.String(),
		Message: "Пользователь с ID " + id + " обновлен",
	}
	return c.JSON(http.StatusOK, updateResponse)
}

// DeleteUser godoc
// @Summary Удаление пользователя(НЕ ИСПОЛЬЗОВАТЬ, ДОБАВЛЕНО ВРЕМЕННО ДЛЯ ТЕСТА ОШИБКИ 502)
// @Description Логическое удаление пользователя по ID, включая связанные данные (поле deleted = true)
// @Tags Users
// @Accept json
// @Produce json
// @Param id path string true "ID пользователя"
// @Security BearerAuth
// @Failure 401 {object} map[string]string "Нет или неверный токен"
// @Success 200 {object} response.UserUniversalResponse "Пользователь успешно удален"
// @Failure 404 {object} map[string]string "Пользователь не найден"
// @Failure 500 {object} map[string]string "Ошибка сервера при удалении пользователя"
// @Router /user/full-delete/{id} [delete]
func (uc *UserController) DeleteUser(c echo.Context) error {
	id := c.Param("id")
	userId, err := uuid.Parse(id)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Некорректный идентификатор пользователя"})
	}

	err = uc.userService.DeleteUser(userId)
	if err != nil {
		if err.Error() == "user not found" {
			return c.JSON(http.StatusBadRequest, map[string]string{"message": "Ничего не удалено"})
		}
		log.Printf("service error (delete user): %v", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": "Ошибка при удалении пользователя",
		})
	}

	deleteResponse := response.UserUniversalResponse{
		ID:      id,
		Message: "Пользователь с ID " + id + " удален",
	}
	return c.JSON(http.StatusOK, deleteResponse)
}

// BanUser godoc
// @Summary Ban пользователя
// @Description Логическое удаление пользователя по ID
// @Tags Users
// @Accept json
// @Produce json
// @Param id path string true "ID пользователя"
// @Security BearerAuth
// @Failure 401 {object} map[string]string "Нет или неверный токен"
// @Success 200 {object} response.UserUniversalResponse "Пользователь успешно удален"
// @Failure 404 {object} map[string]string "Пользователь не найден"
// @Failure 500 {object} map[string]string "Ошибка сервера при удалении пользователя"
// @Router /user/{id} [delete]
func (uc *UserController) BanUser(c echo.Context) error {
	id := c.Param("id")
	userId, err := uuid.Parse(id)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Некорректный идентификатор пользователя"})
	}

	err = uc.userService.BanUser(userId)
	if err != nil {
		if err.Error() == "user not found" {
			return c.JSON(http.StatusBadRequest, map[string]string{"message": "Ничего не удалено"})
		}
		log.Printf("service error (ban user): %v", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": "Ошибка при удалении пользователя",
		})
	}

	deleteResponse := response.UserUniversalResponse{
		ID:      id,
		Message: "Пользователь с ID " + id + " забанен",
	}
	return c.JSON(http.StatusOK, deleteResponse)
}

// RestoreUser godoc
// @Summary Восстановление пользователя
// @Description Восстанавливает пользователя по email (логическое удаление снимается)
// @Tags Users
// @Accept json
// @Produce json
// @Param request body request.RestoreUserRequest true "Email пользователя для восстановления"
// @Security BearerAuth
// @Success 200 {object} response.UserUniversalResponse "Пользователь успешно восстановлен"
// @Failure 400 {object} map[string]string "Неверный запрос или пользователь не найден"
// @Failure 401 {object} map[string]string "Нет или неверный токен"
// @Failure 500 {object} map[string]string "Ошибка сервера при восстановлении пользователя"
// @Router /user/restore [post]
func (uc *UserController) RestoreUser(c echo.Context) error {
	var req request.RestoreUserRequest
	if err := c.Bind(&req); err != nil {
		log.Printf("Bind error: %v", err)
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "Не удалось получить данные из запроса",
		})
	}

	userId, err := uc.userService.RestoreUser(req)
	if err != nil {
		if err.Error() == "user not found" {
			return c.JSON(http.StatusBadRequest, map[string]string{"message": "Ничего не восстановлено"})
		}
		log.Printf("service error (restore user): %v", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": "Ошибка при удалении пользователя",
		})
	}

	restoreResponse := response.UserUniversalResponse{
		ID:      userId.String(),
		Message: "Пользователь с email " + *req.Email + " восстановлен",
	}
	return c.JSON(http.StatusOK, restoreResponse)
}

// AddUserRole godoc
// @Summary Добавление роли пользователю
// @Description Добавляет роль указанному пользователю
// @Tags Users
// @Accept json
// @Produce json
// @Param addRole body request.AddRoleUserRequest true "Данные для добавления роли пользователю"
// @Security BearerAuth
// @Failure 401 {object} map[string]string "Нет или неверный токен"
// @Success 200 {object} response.AddRoleUserResponse "Роль успешно добавлена"
// @Failure 400 {object} map[string]string "Ошибка в запросе или некорректные идентификаторы"
// @Failure 500 {object} map[string]string "Ошибка сервера при добавлении роли"
// @Router /user/role [post]
func (uc *UserController) AddUserRole(c echo.Context) error {
	var req request.AddRoleUserRequest
	if err := c.Bind(&req); err != nil {
		log.Printf("Bind error: %v", err)
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "Не удалось получить данные из запроса",
		})
	}

	// assigner — текущий авторизованный пользователь
	if assignerID, ok := c.Get("user_id").(string); ok && assignerID != "" {
		req.AssignerId = assignerID
	}

	addResponse, err := uc.userService.AddUserRole(req)
	if err != nil {
		log.Printf("service error (add user role): %v", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": "Не удалось добавить роль пользователям",
		})
	}

	return c.JSON(http.StatusOK, addResponse)
}

// RemoveUserRole godoc
// @Summary Удаление роли у пользователя
// @Description Удаляет роль у указанного пользователя
// @Tags Users
// @Accept json
// @Produce json
// @Param removeRole body request.RemoveRoleUserRequest true "Данные для удаления роли у пользователя"
// @Security BearerAuth
// @Failure 401 {object} map[string]string "Нет или неверный токен"
// @Success 200 {object} response.RemoveRoleUserResponse "Роль успешно удалена"
// @Failure 400 {object} map[string]string "Ошибка в запросе или некорректные идентификаторы"
// @Failure 404 {object} map[string]string "Ничего не удалено"
// @Failure 500 {object} map[string]string "Ошибка сервера при удалении роли"
// @Router /user/role [delete]
func (uc *UserController) RemoveUserRole(c echo.Context) error {
	var req request.RemoveRoleUserRequest
	if err := c.Bind(&req); err != nil {
		log.Printf("Bind error: %v", err)
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "Не удалось получить данные из запроса",
		})
	}

	deleteResponse, err := uc.userService.RemoveUserRole(req)
	if err != nil {
		if err.Error() == "user role not found" {
			return c.JSON(http.StatusNotFound, map[string]string{"message": "Ничего не удалено"})
		}
		log.Printf("service error (remove user role): %v", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": "Ошибка при удалении роли у пользователя",
		})
	}

	return c.JSON(http.StatusOK, deleteResponse)
}
