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

type RoleController struct {
	roleService service.RoleService
}

func NewRoleController(roleService service.RoleService) *RoleController {
	return &RoleController{
		roleService: roleService,
	}
}

func RegisterRoleRoutes(e Router, roleService service.RoleService, adminMw echo.MiddlewareFunc) {
	controller := NewRoleController(roleService)
	group := e.Group("/role")

	// Read: любой авторизованный
	group.GET("/all/:page/:pagesize", controller.GetAllRoles)
	group.GET("/user/:id", controller.GetRoleByUserId)
	group.GET("/:id", controller.GetRoleById)

	// Write: только admin
	group.POST("", controller.CreateRole, adminMw)
	group.PATCH("/:id", controller.UpdateRole, adminMw)
	group.DELETE("/:id", controller.DeleteRole, adminMw)
}

// GetAllRoles godoc
// @Summary Получение списка всех ролей
// @Description Получает список всех ролей с учетом пагинации, исключая удаленные
// @Tags Roles
// @Accept json
// @Produce json
// @Param page path int true "Номер страницы"
// @Param pagesize path int true "Размер страницы"
// @Security BearerAuth
// @Failure 401 {object} map[string]string "Нет или неверный токен"
// @Success 200 {object} response.GetAllRolesResponse "Список ролей успешно получен"
// @Failure 400 {object} map[string]string "Ошибка в запросе"
// @Failure 500 {object} map[string]string "Ошибка сервера при получении ролей"
// @Router /role/all/{page}/{pagesize} [get]
func (rc *RoleController) GetAllRoles(c echo.Context) error {
	page, err := strconv.Atoi(c.Param("page"))
	if err != nil || page <= 0 {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Ошибка при парсинге страницы"})
	}
	pageSize, err := strconv.Atoi(c.Param("pagesize"))
	if err != nil || pageSize <= 0 {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Ошибка при парсинге номера страницы"})
	}

	roles, totalCount, err := rc.roleService.GetAllRoles(page, pageSize)
	if err != nil {
		log.Printf("service error (get all roles): %v", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Ошибка при подсчете ролей"})
	}

	out := response.GetAllRolesResponse{
		TotalCount: totalCount,
		PageSize:   pageSize,
		Page:       page,
	}
	for _, r := range roles {
		out.Roles = append(out.Roles, response.GetRoleResponse{
			ID:          r.ID.String(),
			Name:        utils.GetString(r.Name),
			Description: utils.GetString(r.Description),
			UpdatedAt:   utils.GetTime(r.UpdatedAt),
			CreatedAt:   utils.GetTime(r.CreatedAt),
		})
	}
	return c.JSON(http.StatusOK, out)
}

// GetRoleById godoc
// @Summary Получение роли по ID
// @Description Получает данные роли по её уникальному идентификатору
// @Tags Roles
// @Accept json
// @Produce json
// @Param id path string true "ID роли"
// @Security BearerAuth
// @Failure 401 {object} map[string]string "Нет или неверный токен"
// @Success 200 {object} response.GetRoleResponse "Роль успешно получена"
// @Failure 400 {object} map[string]string "Некорректный идентификатор роли"
// @Failure 404 {object} map[string]string "Роль не найдена"
// @Failure 500 {object} map[string]string "Ошибка сервера при получении роли"
// @Router /role/{id} [get]
func (rc *RoleController) GetRoleById(c echo.Context) error {
	roleID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Некорректный идентификатор роли"})
	}

	role, err := rc.roleService.GetRoleById(roleID)
	if err != nil {
		if err.Error() == "role not found" {
			return c.JSON(http.StatusNotFound, map[string]string{"error": "Роль не найдена"})
		}
		log.Printf("service error (get role by id): %v", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Ошибка при получении роли из базы данных"})
	}

	return c.JSON(http.StatusOK, response.GetRoleResponse{
		ID:          role.ID.String(),
		Name:        utils.GetString(role.Name),
		Description: utils.GetString(role.Description),
		UpdatedAt:   utils.GetTime(role.UpdatedAt),
		CreatedAt:   utils.GetTime(role.CreatedAt),
	})
}

// CreateRole godoc
// @Summary Создание новой роли
// @Description Создает новую роль с указанными параметрами
// @Tags Roles
// @Accept json
// @Produce json
// @Param role body request.RoleCreateRequest true "Данные для создания роли"
// @Security BearerAuth
// @Failure 401 {object} map[string]string "Нет или неверный токен"
// @Success 201 {object} response.RoleUniversalResponse "Роль успешно создана"
// @Failure 400 {object} map[string]string "Ошибка в запросе"
// @Failure 500 {object} map[string]string "Ошибка сервера при создании роли"
// @Router /role [post]
func (rc *RoleController) CreateRole(c echo.Context) error {
	var req request.RoleCreateRequest
	if err := c.Bind(&req); err != nil {
		log.Printf("Bind error: %v", err)
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Не удалось получить данные из запроса"})
	}

	roleID, err := rc.roleService.CreateRole(req)
	if err != nil {
		log.Printf("service error (create role): %v", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Ошибка при создании роли"})
	}

	return c.JSON(http.StatusCreated, response.RoleUniversalResponse{
		ID:      roleID.String(),
		Message: "Роль успешно создана",
	})
}

// UpdateRole godoc
// @Summary Обновление роли
// @Description Обновляет данные роли по её ID
// @Tags Roles
// @Accept json
// @Produce json
// @Param id path string true "ID роли"
// @Param role body request.RoleUpdateRequest true "Данные для обновления роли"
// @Security BearerAuth
// @Failure 401 {object} map[string]string "Нет или неверный токен"
// @Success 200 {object} response.RoleUniversalResponse "Роль успешно обновлена"
// @Failure 400 {object} map[string]string "Некорректный идентификатор или ошибка в запросе"
// @Failure 500 {object} map[string]string "Ошибка сервера при обновлении роли"
// @Router /role/{id} [patch]
func (rc *RoleController) UpdateRole(c echo.Context) error {
	roleID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Некорректный идентификатор роли"})
	}

	var req request.RoleUpdateRequest
	if err := c.Bind(&req); err != nil {
		log.Printf("Bind error: %v", err)
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Не удалось получить данные из запроса"})
	}

	err = rc.roleService.UpdateRole(roleID, req)
	if err != nil {
		if err.Error() == "no fields to update" {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "Не указаны поля для обновления"})
		}
		if err.Error() == "role not found" {
			return c.JSON(http.StatusNotFound, map[string]string{"message": "Ничего не обновлено"})
		}
		log.Printf("service error (update role): %v", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Ошибка при обновлении роли"})
	}

	return c.JSON(http.StatusOK, response.RoleUniversalResponse{
		ID:      roleID.String(),
		Message: "Роль успешно обновлена",
	})
}

// DeleteRole godoc
// @Summary Удаление роли
// @Description Логическое удаление роли по ID, включая связанные данные (поле deleted = true)
// @Tags Roles
// @Accept json
// @Produce json
// @Param id path string true "ID роли"
// @Security BearerAuth
// @Failure 401 {object} map[string]string "Нет или неверный токен"
// @Success 200 {object} response.RoleUniversalResponse "Роль успешно удалена"
// @Failure 404 {object} map[string]string "Роль не найдена"
// @Failure 500 {object} map[string]string "Ошибка сервера при удалении роли"
// @Router /role/{id} [delete]
func (rc *RoleController) DeleteRole(c echo.Context) error {
	roleID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Некорректный идентификатор роли"})
	}

	err = rc.roleService.DeleteRole(roleID)
	if err != nil {
		if err.Error() == "role not found" {
			return c.JSON(http.StatusNotFound, map[string]string{"message": "Ничего не удалено"})
		}
		log.Printf("service error (delete role): %v", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Ошибка при удалении роли"})
	}

	return c.JSON(http.StatusOK, response.RoleUniversalResponse{
		ID:      roleID.String(),
		Message: "Роль удалена",
	})
}

// GetRoleByUserId godoc
// @Summary Получение роли по ID пользователя
// @Description Получает роль пользователя по его ID
// @Tags Roles
// @Accept json
// @Produce json
// @Param id path string true "ID пользователя"
// @Security BearerAuth
// @Failure 401 {object} map[string]string "Нет или неверный токен"
// @Success 200 {object} response.GetRoleByUserId "Роль пользователя успешно получена"
// @Failure 400 {object} map[string]string "Некорректный идентификатор пользователя"
// @Failure 404 {object} map[string]string "Роль не найдена для данного пользователя"
// @Failure 500 {object} map[string]string "Ошибка сервера при получении роли"
// @Router /role/user/{id} [get]
func (rc *RoleController) GetRoleByUserId(c echo.Context) error {
	userId, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Некорректный идентификатор пользователя"})
	}

	role, err := rc.roleService.GetRoleByUserId(userId)
	if err != nil {
		if err.Error() == "role not found for user" {
			return c.JSON(http.StatusOK, response.GetRoleByUserId{
				UserId: userId.String(),
				Role:   nil,
			})
		}
		log.Printf("service error (get role by user id): %v", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Ошибка при получении роли пользователя"})
	}

	roleResponse := response.GetRoleResponse{
		ID:          role.ID.String(),
		Name:        utils.GetString(role.Name),
		Description: utils.GetString(role.Description),
		UpdatedAt:   utils.GetTime(role.UpdatedAt),
		CreatedAt:   utils.GetTime(role.CreatedAt),
	}

	return c.JSON(http.StatusOK, response.GetRoleByUserId{
		UserId: userId.String(),
		Role:   &roleResponse,
	})
}
