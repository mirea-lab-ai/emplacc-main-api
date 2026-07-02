package httpapi

import (
	"emplacc-api/internal/dto/request"
	"emplacc-api/internal/dto/response"
	"emplacc-api/internal/service"
	"emplacc-api/internal/utils"
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
)

type ProblemController struct {
	problemService service.ProblemService
}

func NewProblemController(problemService service.ProblemService) *ProblemController {
	return &ProblemController{
		problemService: problemService,
	}
}

func RegisterProblemRoutes(e Router, problemService service.ProblemService, employeeMw echo.MiddlewareFunc, managerMw echo.MiddlewareFunc) {
	controller := NewProblemController(problemService)
	g := e.Group("/problem")
	g.GET("/all/:page/:pagesize", controller.GetAllProblems)
	g.GET("/search", controller.SearchProblems)
	g.GET("/:id", controller.GetProblemByID)
	g.GET("/user/:id/:page/:pagesize", controller.GetProblemsByUserId)
	g.POST("", controller.CreateProblem, employeeMw)
	g.PATCH("/:id", controller.UpdateProblem, employeeMw)
	g.DELETE("/:id", controller.DeleteProblem, managerMw)
}

// GetAllProblems godoc
// @Summary Получение списка всех проблем
// @Description Получает список всех проблем с учетом пагинации, исключая удаленные
// @Tags Problems
// @Accept json
// @Produce json
// @Param page path int true "Номер страницы"
// @Param pagesize path int true "Размер страницы"
// @Security BearerAuth
// @Failure 401 {object} map[string]string "Нет или неверный токен"
// @Success 200 {object} response.ProblemListResponse "Список проблем успешно получен"
// @Failure 400 {object} map[string]string "Ошибка в запросе"
// @Failure 500 {object} map[string]string "Ошибка сервера при получении проблем"
// @Router /problem/all/{page}/{pagesize} [get]
func (pc *ProblemController) GetAllProblems(c echo.Context) error {
	page, err := strconv.Atoi(c.Param("page"))
	if err != nil || page <= 0 {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Ошибка при парсинге страницы"})
	}
	pageSize, err := strconv.Atoi(c.Param("pagesize"))
	if err != nil || pageSize <= 0 {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Ошибка при парсинге номера страницы"})
	}

	problems, totalCount, err := pc.problemService.GetAllProblems(page, pageSize)
	if err != nil {
		log.Printf("service error (get all problems): %v", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Ошибка при подсчете проблем"})
	}

	out := response.ProblemListResponse{
		Page:       page,
		PageSize:   pageSize,
		TotalCount: totalCount,
	}

	for _, p := range problems {
		desc := []string(p.Description)
		out.Problems = append(out.Problems, response.ProblemResponse{
			ID:          p.ID.String(),
			Name:        utils.GetString(p.Name),
			Description: desc,
			CreatorId:   utils.GetUUIDString(p.CreatorID),
			CreatedAt:   utils.GetTime(p.CreatedAt),
			UpdatedAt:   utils.GetTime(p.UpdatedAt),
		})
	}

	return c.JSON(http.StatusOK, out)
}

// SearchProblems godoc
// @Summary Полнотекстовый поиск проблем
// @Description Поиск проблем по имени и описанию с пагинацией
// @Tags Problems
// @Accept json
// @Produce json
// @Param query query string true "Поисковый запрос"
// @Param page query int true "Номер страницы"
// @Param pagesize query int true "Размер страницы"
// @Security BearerAuth
// @Success 200 {object} response.ProblemListResponse "Список найденных проблем"
// @Failure 400 {object} map[string]string "Ошибка в запросе"
// @Failure 500 {object} map[string]string "Ошибка сервера при поиске"
// @Router /problem/search [get]
func (pc *ProblemController) SearchProblems(c echo.Context) error {
	query := strings.TrimSpace(c.QueryParam("query"))
	if query == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Поисковый запрос не может быть пустым"})
	}
	page, err := strconv.Atoi(c.QueryParam("page"))
	if err != nil || page <= 0 {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Некорректный номер страницы"})
	}
	pageSize, err := strconv.Atoi(c.QueryParam("pagesize"))
	if err != nil || pageSize <= 0 || pageSize > 100 {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Некорректный размер страницы"})
	}

	problems, totalCount, err := pc.problemService.SearchProblems(query, page, pageSize)
	if err != nil {
		log.Printf("service error (search problems): %v", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Ошибка при поиске проблем"})
	}

	out := response.ProblemListResponse{
		Page:       page,
		PageSize:   pageSize,
		TotalCount: totalCount,
		Problems:   make([]response.ProblemResponse, 0, len(problems)),
	}
	for _, p := range problems {
		out.Problems = append(out.Problems, response.ProblemResponse{
			ID:          p.ID.String(),
			Name:        utils.GetString(p.Name),
			Description: []string(p.Description),
			CreatorId:   utils.GetUUIDString(p.CreatorID),
			CreatedAt:   utils.GetTime(p.CreatedAt),
			UpdatedAt:   utils.GetTime(p.UpdatedAt),
		})
	}

	return c.JSON(http.StatusOK, out)
}

// GetProblemsByUserId godoc
// @Summary Получение проблем по ID пользователя
// @Description Получает список проблем, созданных указанным пользователем, с учетом пагинации
// @Tags Problems
// @Accept json
// @Produce json
// @Param id path string true "ID пользователя"
// @Param page path int true "Номер страницы"
// @Param pagesize path int true "Размер страницы"
// @Security BearerAuth
// @Failure 401 {object} map[string]string "Нет или неверный токен"
// @Success 200 {object} response.ProblemsByUserId "Список проблем успешно получен"
// @Failure 400 {object} map[string]string "Ошибка в запросе"
// @Failure 500 {object} map[string]string "Ошибка сервера при получении проблем"
// @Router /problem/user/{id}/{page}/{pagesize} [get]
func (pc *ProblemController) GetProblemsByUserId(c echo.Context) error {
	creatorUUID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Некорректный идентификатор пользователя"})
	}

	page, err := strconv.Atoi(c.Param("page"))
	if err != nil || page <= 0 {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Ошибка при парсинге страницы"})
	}
	pageSize, err := strconv.Atoi(c.Param("pagesize"))
	if err != nil || pageSize <= 0 {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Ошибка при парсинге номера страницы"})
	}

	problems, totalCount, err := pc.problemService.GetProblemsByUserId(creatorUUID, page, pageSize)
	if err != nil {
		log.Printf("service error (get problems by user id): %v", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Ошибка при подсчете проблем"})
	}

	out := response.ProblemsByUserId{
		UserID:     creatorUUID.String(),
		Page:       page,
		PageSize:   pageSize,
		TotalCount: totalCount,
	}
	for _, p := range problems {
		desc := []string(p.Description)
		out.Problems = append(out.Problems, response.ProblemResponse{
			ID:          p.ID.String(),
			Name:        utils.GetString(p.Name),
			Description: desc,
			CreatorId:   utils.GetUUIDString(p.CreatorID),
			CreatedAt:   utils.GetTime(p.CreatedAt),
			UpdatedAt:   utils.GetTime(p.UpdatedAt),
		})
	}

	return c.JSON(http.StatusOK, out)
}

// GetProblemByID godoc
// @Summary Получение проблемы по ID
// @Description Получает данные проблемы по её уникальному идентификатору
// @Tags Problems
// @Accept json
// @Produce json
// @Param id path string true "ID проблемы"
// @Security BearerAuth
// @Failure 401 {object} map[string]string "Нет или неверный токен"
// @Success 200 {object} response.ProblemResponse "Проблема успешно получена"
// @Failure 400 {object} map[string]string "Некорректный идентификатор проблемы"
// @Failure 404 {object} map[string]string "Проблема не найдена"
// @Failure 500 {object} map[string]string "Ошибка сервера при получении проблемы"
// @Router /problem/{id} [get]
func (pc *ProblemController) GetProblemByID(c echo.Context) error {
	problemId, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Некорректный идентификатор проблемы"})
	}

	p, err := pc.problemService.GetProblemByID(problemId)
	if err != nil {
		if err.Error() == "problem not found" {
			return c.JSON(http.StatusNotFound, map[string]string{"error": "Проблема не найдена"})
		}
		log.Printf("service error (get problem by id): %v", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Ошибка при получении проблемы из базы данных"})
	}

	desc := []string(p.Description)

	return c.JSON(http.StatusOK, response.ProblemResponse{
		ID:          p.ID.String(),
		Name:        utils.GetString(p.Name),
		Description: desc,
		CreatorId:   utils.GetUUIDString(p.CreatorID),
		CreatedAt:   utils.GetTime(p.CreatedAt),
		UpdatedAt:   utils.GetTime(p.UpdatedAt),
	})
}

// CreateProblem godoc
// @Summary Создание новой проблемы
// @Description Создает новую проблему с указанными параметрами
// @Tags Problems
// @Accept json
// @Produce json
// @Param problem body request.ProblemCreateRequest true "Данные для создания проблемы"
// @Security BearerAuth
// @Failure 401 {object} map[string]string "Нет или неверный токен"
// @Success 201 {object} response.ProblemUniversalResponse "Проблема успешно создана"
// @Failure 400 {object} map[string]string "Ошибка в запросе или некорректный идентификатор пользователя"
// @Failure 500 {object} map[string]string "Ошибка сервера при создании проблемы"
// @Router /problem [post]
func (pc *ProblemController) CreateProblem(c echo.Context) error {
	var req request.ProblemCreateRequest
	if err := c.Bind(&req); err != nil {
		log.Printf("Bind error: %v", err)
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Не удалось получить данные из запроса"})
	}

	problemID, err := pc.problemService.CreateProblem(req)
	if err != nil {
		if err.Error() == "invalid creator id" {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "Некорректный идентификатор пользователя"})
		}
		log.Printf("service error (create problem): %v", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Ошибка при создании проблемы"})
	}

	return c.JSON(http.StatusCreated, response.ProblemUniversalResponse{
		ID:      problemID.String(),
		Message: "Проблема создана",
	})
}

// UpdateProblem godoc
// @Summary Обновление проблемы
// @Description Обновляет данные проблемы по её ID
// @Tags Problems
// @Accept json
// @Produce json
// @Param id path string true "ID проблемы"
// @Param problem body request.ProblemUpdateRequest true "Данные для обновления проблемы"
// @Security BearerAuth
// @Failure 401 {object} map[string]string "Нет или неверный токен"
// @Success 200 {object} response.ProblemUniversalResponse "Проблема успешно обновлена"
// @Failure 400 {object} map[string]string "Некорректный идентификатор или ошибка в запросе"
// @Failure 500 {object} map[string]string "Ошибка сервера при обновлении проблемы"
// @Router /problem/{id} [patch]
func (pc *ProblemController) UpdateProblem(c echo.Context) error {
	problemId, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Некорректный идентификатор проблемы"})
	}

	// IDOR-защита: редактировать может только автор проблемы либо менеджер/админ.
	userIDStr, _ := c.Get("user_id").(string)
	userRole, _ := c.Get("user_role").(string)
	existing, err := pc.problemService.GetProblemByID(problemId)
	if err != nil {
		if err.Error() == "problem not found" {
			return c.JSON(http.StatusNotFound, map[string]string{"message": "Ничего не обновлено"})
		}
		log.Printf("service error (get problem for update): %v", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Ошибка при обновлении проблемы"})
	}
	isPrivileged := userRole == "manager" || userRole == "admin"
	isOwner := existing.CreatorID != nil && existing.CreatorID.String() == userIDStr
	if !isPrivileged && !isOwner {
		return c.JSON(http.StatusForbidden, map[string]string{"error": "Можно редактировать только свои проблемы"})
	}

	var req request.ProblemUpdateRequest
	if err := c.Bind(&req); err != nil {
		log.Printf("Bind error: %v", err)
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Не удалось получить данные из запроса"})
	}

	err = pc.problemService.UpdateProblem(problemId, req)
	if err != nil {
		if err.Error() == "no fields to update" {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "Не указаны поля для обновления"})
		}
		if err.Error() == "problem not found" {
			return c.JSON(http.StatusNotFound, map[string]string{"message": "Ничего не обновлено"})
		}
		log.Printf("service error (update problem): %v", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Ошибка при обновлении проблемы"})
	}

	return c.JSON(http.StatusOK, response.ProblemUniversalResponse{
		ID:      problemId.String(),
		Message: "Проблема успешно обновлена",
	})
}

// DeleteProblem godoc
// @Summary Удаление проблемы
// @Description Логическое удаление проблемы по ID, включая связанные данные (поле deleted = true)
// @Tags Problems
// @Accept json
// @Produce json
// @Param id path string true "ID проблемы"
// @Security BearerAuth
// @Success 200 {object} response.ProblemUniversalResponse "Проблема успешно удалена"
// @Failure 404 {object} map[string]string "Проблема не найдена"
// @Failure 500 {object} map[string]string "Ошибка сервера при удалении проблемы"
// @Router /problem/{id} [delete]
func (pc *ProblemController) DeleteProblem(c echo.Context) error {
	problemId, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Некорректный идентификатор проблемы"})
	}

	err = pc.problemService.DeleteProblem(problemId)
	if err != nil {
		if err.Error() == "problem not found" {
			return c.JSON(http.StatusNotFound, map[string]string{"message": "Ничего не удалено"})
		}
		log.Printf("service error (delete problem): %v", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Ошибка при удалении проблемы"})
	}

	return c.JSON(http.StatusOK, response.ProblemUniversalResponse{
		ID:      problemId.String(),
		Message: "Проблема успешно удалена",
	})
}
