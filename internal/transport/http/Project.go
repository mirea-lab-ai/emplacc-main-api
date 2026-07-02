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

type ProjectController struct {
	projectService service.ProjectService
}

func NewProjectController(projectService service.ProjectService) *ProjectController {
	return &ProjectController{
		projectService: projectService,
	}
}

func RegisterProjectRoutes(e Router, projectService service.ProjectService, managerMw echo.MiddlewareFunc) {
	controller := NewProjectController(projectService)
	g := e.Group("/project")
	g.GET("/all/:page/:pagesize", controller.GetAllProjects)
	g.GET("/:id", controller.GetProjectByID)
	g.GET("/user/:id", controller.GetProjectsByUser)
	g.GET("/team/:team_id", controller.GetTeamProjects)
	g.GET("/search", controller.SearchProjects)
	g.POST("", controller.CreateProject, managerMw)
	g.PATCH("/:id", controller.UpdateProject, managerMw)
	g.DELETE("/:id", controller.DeleteProject, managerMw)
}

// GetAllProjects godoc
// @Summary Получение списка всех проектов
// @Description Получает список всех проектов с учетом пагинации, исключая удаленные
// @Tags Projects
// @Accept json
// @Produce json
// @Param page path int true "Номер страницы"
// @Param pagesize path int true "Размер страницы"
// @Security BearerAuth
// @Failure 401 {object} map[string]string "Нет или неверный токен"
// @Success 200 {object} response.ProjectListResponse "Список проектов успешно получен"
// @Failure 400 {object} map[string]string "Ошибка в запросе"
// @Failure 500 {object} map[string]string "Ошибка сервера при получении проектов"
// @Router /project/all/{page}/{pagesize} [get]
func (pc *ProjectController) GetAllProjects(c echo.Context) error {
	page, err := strconv.Atoi(c.Param("page"))
	if err != nil || page <= 0 {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Ошибка при парсинге страницы"})
	}
	pageSize, err := strconv.Atoi(c.Param("pagesize"))
	if err != nil || pageSize <= 0 {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Ошибка при парсинге номера страницы"})
	}

	projects, totalCount, err := pc.projectService.GetAllProjects(page, pageSize)
	if err != nil {
		log.Printf("service error (get all projects): %v", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Ошибка при подсчете проектов"})
	}

	out := response.ProjectListResponse{Page: page, PageSize: pageSize, TotalCount: totalCount}
	for _, p := range projects {
		out.Projects = append(out.Projects, response.ProjectResponse{
			ID:              p.ID.String(),
			Name:            utils.GetString(p.Name),
			Description:     utils.GetString(p.Description),
			GitlabProjectId: utils.GetInt(p.GitlabProjectID),
			CreatedBy:       utils.GetUUIDString(p.CreatedBy),
			Status:          utils.GetString(p.Status),
			GitlabUrl:       utils.GetString(p.GitlabURL),
			CreatedAt:       utils.GetTime(p.CreatedAt),
			UpdatedAt:       utils.GetTime(p.UpdatedAt),
		})
	}
	return c.JSON(http.StatusOK, out)
}

// SearchProjects godoc
// @Summary Поиск проектов с автодополнением и пагинацией
// @Description Ищет проекты по имени, описанию или GitLab URL с автодополнением после каждого введенного символа. Поддерживает автоматическую замену раскладки клавиатуры (английская-русская) для расширенного поиска и пагинацию. Проекты сортируются: сначала проекты где пользователь состоит, потом создатель, потом остальные.
// @Tags Projects
// @Accept json
// @Produce json
// @Param query query string true "Поисковый запрос"
// @Param user_id query string true "ID пользователя для приоритетной сортировки"
// @Param page query int true "Номер страницы"
// @Param pagesize query int true "Размер страницы" minimum(1) maximum(100)
// @Security BearerAuth
// @Failure 401 {object} map[string]string "Нет или неверный токен"
// @Success 200 {object} response.ProjectSearchResponse "Результаты поиска проектов"
// @Failure 400 {object} map[string]string "Ошибка в запросе"
// @Failure 500 {object} map[string]string "Ошибка сервера при поиске проектов"
// @Router /project/search [get]
func (pc *ProjectController) SearchProjects(c echo.Context) error {
	query := strings.TrimSpace(c.QueryParam("query"))
	userID := strings.TrimSpace(c.QueryParam("user_id"))

	if query == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Поисковый запрос не может быть пустым"})
	}

	if userID == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "ID пользователя обязателен"})
	}

	if _, err := uuid.Parse(userID); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Некорректный формат ID пользователя"})
	}

	page, err := strconv.Atoi(c.QueryParam("page"))
	if err != nil || page <= 0 {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Некорректный номер страницы"})
	}

	pageSize, err := strconv.Atoi(c.QueryParam("pagesize"))
	if err != nil || pageSize <= 0 || pageSize > 100 {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Некорректный размер страницы"})
	}

	projects, totalCount, err := pc.projectService.SearchProjects(query, userID, page, pageSize)
	if err != nil {
		log.Printf("service error (search projects): %v", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Ошибка при поиске проектов"})
	}

	out := response.ProjectSearchResponse{
		Query:      query,
		Page:       page,
		PageSize:   pageSize,
		TotalCount: totalCount,
		Projects:   make([]response.ProjectForSearchResponse, 0, len(projects)),
	}

	for _, p := range projects {
		// Формируем информацию о создателе
		var createdByInfo response.UserShort
		if p.CreatedByUser != nil {
			createdByInfo = response.UserShort{
				ID:        p.CreatedByUser.ID.String(),
				FirstName: p.CreatedByUser.FirstName,
				LastName:  p.CreatedByUser.LastName,
			}
		}

		out.Projects = append(out.Projects, response.ProjectForSearchResponse{
			ID:              p.ID.String(),
			Name:            utils.GetString(p.Name),
			Description:     utils.GetString(p.Description),
			GitlabProjectId: utils.GetInt(p.GitlabProjectID),
			CreatedBy:       utils.GetUUIDString(p.CreatedBy),
			CreatedByUser:   createdByInfo,
			Status:          utils.GetString(p.Status),
			GitlabUrl:       utils.GetString(p.GitlabURL),
			CreatedAt:       utils.GetTime(p.CreatedAt),
			UpdatedAt:       utils.GetTime(p.UpdatedAt),
		})
	}

	return c.JSON(http.StatusOK, out)
}

// GetProjectByID godoc
// @Summary Получение проекта по ID
// @Description Получает данные проекта по его уникальному идентификатору
// @Tags Projects
// @Accept json
// @Produce json
// @Param id path string true "ID проекта"
// @Security BearerAuth
// @Failure 401 {object} map[string]string "Нет или неверный токен"
// @Success 200 {object} response.ProjectResponse "Проект успешно получен"
// @Failure 400 {object} map[string]string "Некорректный идентификатор проекта"
// @Failure 404 {object} map[string]string "Проект не найден"
// @Failure 500 {object} map[string]string "Ошибка сервера при получении проекта"
// @Router /project/{id} [get]
func (pc *ProjectController) GetProjectByID(c echo.Context) error {
	projectID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Некорректный идентификатор проекта"})
	}

	p, err := pc.projectService.GetProjectByID(projectID)
	if err != nil {
		if err.Error() == "project not found" {
			return c.JSON(http.StatusNotFound, map[string]string{"error": "Проект не найден"})
		}
		log.Printf("service error (get project by id): %v", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Ошибка при получении проекта из базы данных"})
	}

	return c.JSON(http.StatusOK, response.ProjectResponse{
		ID:              p.ID.String(),
		Name:            utils.GetString(p.Name),
		Description:     utils.GetString(p.Description),
		GitlabProjectId: utils.GetInt(p.GitlabProjectID),
		CreatedBy:       utils.GetUUIDString(p.CreatedBy),
		Status:          utils.GetString(p.Status),
		GitlabUrl:       utils.GetString(p.GitlabURL),
		CreatedAt:       utils.GetTime(p.CreatedAt),
		UpdatedAt:       utils.GetTime(p.UpdatedAt),
	})
}

// GetProjectsByUser godoc
// @Summary Получение проектов пользователя
// @Description Получает список проектов, в которых участвует пользователь через команды
// @Tags Projects
// @Accept json
// @Produce json
// @Param id path string true "Идентификатор пользователя (UUID)"
// @Security BearerAuth
// @Success 200 {array} response.ProjectResponse "Список проектов успешно получен"
// @Failure 400 {object} map[string]string "Некорректный идентификатор пользователя"
// @Failure 401 {object} map[string]string "Нет или неверный токен"
// @Failure 404 {object} map[string]string "Проекты не найдены"
// @Failure 500 {object} map[string]string "Ошибка сервера при получении проектов"
// @Router /project/user/{id} [get]
func (pc *ProjectController) GetProjectsByUser(c echo.Context) error {
	userID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Некорректный идентификатор пользователя"})
	}

	projects, err := pc.projectService.GetProjectsByUser(userID)
	if err != nil {
		log.Printf("service error (get projects by user): %v", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Ошибка при получении проектов из базы данных"})
	}

	if len(projects) == 0 {
		return c.JSON(http.StatusOK, []response.ProjectResponse{})
	}

	var out []response.ProjectResponse
	for _, p := range projects {
		out = append(out, response.ProjectResponse{
			ID:              p.ID.String(),
			Name:            utils.GetString(p.Name),
			Description:     utils.GetString(p.Description),
			GitlabProjectId: utils.GetInt(p.GitlabProjectID),
			CreatedBy:       utils.GetUUIDString(p.CreatedBy),
			Status:          utils.GetString(p.Status),
			GitlabUrl:       utils.GetString(p.GitlabURL),
			CreatedAt:       utils.GetTime(p.CreatedAt),
			UpdatedAt:       utils.GetTime(p.UpdatedAt),
		})
	}
	return c.JSON(http.StatusOK, out)
}

// GetTeamProjects godoc
// @Summary Получение списка проектов команды
// @Description Получает список всех проектов, связанных с командой, по team_id
// @Tags Teams
// @Accept json
// @Produce json
// @Param team_id path string true "ID команды"
// @Success 200 {object} response.ProjectByTeamResponse "Список проектов успешно получен"
// @Security BearerAuth
// @Failure 400 {object} map[string]string "Некорректный team_id"
// @Failure 401 {object} map[string]string "Нет или неверный токен"
// @Failure 404 {object} map[string]string "Команда или проекты не найдены"
// @Failure 500 {object} map[string]string "Ошибка сервера при получении проектов"
// @Router /project/team/{team_id} [get]
func (pc *ProjectController) GetTeamProjects(c echo.Context) error {
	teamID, err := uuid.Parse(c.Param("team_id"))
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Некорректный team_id"})
	}

	pts, err := pc.projectService.GetTeamProjects(teamID)
	if err != nil {
		if err.Error() == "no projects found for team" {
			return c.JSON(http.StatusNotFound, map[string]string{"error": "Для данной команды проекты не найдены"})
		}
		log.Printf("service error (get team projects): %v", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Ошибка при получении связей команда-проект"})
	}

	// Получаем ID проектов из связей
	projectIDs := make([]uuid.UUID, 0, len(pts))
	for _, pt := range pts {
		projectIDs = append(projectIDs, pt.ProjectID)
	}

	projects, err := pc.projectService.GetProjectsByIDs(projectIDs)
	if err != nil {
		log.Printf("service error (get projects by ids): %v", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Ошибка при получении списка проектов"})
	}

	resp := response.ProjectByTeamResponse{Projects: make([]response.ProjectResponse, 0, len(projects))}
	for _, p := range projects {
		resp.Projects = append(resp.Projects, response.ProjectResponse{
			ID:              p.ID.String(),
			Name:            utils.GetString(p.Name),
			Description:     utils.GetString(p.Description),
			GitlabProjectId: utils.GetInt(p.GitlabProjectID),
			CreatedBy:       utils.GetUUIDString(p.CreatedBy),
			Status:          utils.GetString(p.Status),
			GitlabUrl:       utils.GetString(p.GitlabURL),
			CreatedAt:       utils.GetTime(p.CreatedAt),
			UpdatedAt:       utils.GetTime(p.UpdatedAt),
		})
	}
	return c.JSON(http.StatusOK, resp)
}

// CreateProject godoc
// @Summary Создание нового проекта
// @Description Создает новый проект с указанными параметрами и автоматически создаёт главную доску с двумя статусами: начальным и конечным
// @Tags Projects
// @Accept json
// @Produce json
// @Param project body request.CreateProjectRequest true "Данные для создания проекта"
// @Security BearerAuth
// @Success 201 {object} response.ProjectUniversalResponse "Проект успешно создан"
// @Failure 400 {object} map[string]string "Ошибка в запросе"
// @Failure 401 {object} map[string]string "Нет или неверный токен"
// @Failure 500 {object} map[string]string "Ошибка сервера при создании проекта"
// @Router /project [post]
func (pc *ProjectController) CreateProject(c echo.Context) error {
	var req request.CreateProjectRequest
	if err := c.Bind(&req); err != nil {
		log.Printf("Bind error: %v", err)
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Не удалось получить данные из запроса"})
	}

	if req.CreatedBy == nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Отсутствует идентификатор создателя"})
	}

	projectID, err := pc.projectService.CreateProject(req)
	if err != nil {
		if err.Error() == "invalid creator id" {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "Некорректный идентификатор создателя"})
		}
		log.Printf("service error (create project): %v", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Ошибка при создании проекта"})
	}

	return c.JSON(http.StatusCreated, response.ProjectUniversalResponse{
		ID:      projectID.String(),
		Message: "Проект создан",
	})
}

// UpdateProject godoc
// @Summary Обновление проекта
// @Description Обновляет данные проекта по его ID
// @Tags Projects
// @Accept json
// @Produce json
// @Param id path string true "ID проекта"
// @Param project body request.UpdateProjectRequest true "Данные для обновления проекта"
// @Security BearerAuth
// @Failure 401 {object} map[string]string "Нет или неверный токен"
// @Success 200 {object} response.ProjectUniversalResponse "Проект успешно обновлен"
// @Failure 400 {object} map[string]string "Некорректный идентификатор или ошибка в запросе"
// @Failure 500 {object} map[string]string "Ошибка сервера при обновлении проекта"
// @Router /project/{id} [patch]
func (pc *ProjectController) UpdateProject(c echo.Context) error {
	projectID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Некорректный идентификатор проекта"})
	}

	var req request.UpdateProjectRequest
	if err := c.Bind(&req); err != nil {
		log.Printf("Bind error: %v", err)
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Не удалось получить данные из запроса"})
	}

	err = pc.projectService.UpdateProject(projectID, req)
	if err != nil {
		if err.Error() == "no fields to update" {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "Не указаны поля для обновления"})
		}
		if err.Error() == "project not found" {
			return c.JSON(http.StatusNotFound, map[string]string{"message": "Ничего не обновлено"})
		}
		log.Printf("service error (update project): %v", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Ошибка при обновлении проекта"})
	}

	return c.JSON(http.StatusOK, response.ProjectUniversalResponse{
		ID:      projectID.String(),
		Message: "Проект успешно обновлен",
	})
}

// DeleteProject godoc
// @Summary Удаление проекта
// @Description Логическое удаление проекта по ID, включая все связанные доски, статусы и задачи (поле deleted = true)
// @Tags Projects
// @Accept json
// @Produce json
// @Param id path string true "ID проекта"
// @Security BearerAuth
// @Success 200 {object} response.ProjectUniversalResponse "Проект успешно удален"
// @Failure 404 {object} map[string]string "Проект не найден"
// @Failure 401 {object} map[string]string "Нет или неверный токен"
// @Failure 500 {object} map[string]string "Ошибка сервера при удалении проекта"
// @Router /project/{id} [delete]
func (pc *ProjectController) DeleteProject(c echo.Context) error {
	projectID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Некорректный идентификатор проекта"})
	}

	err = pc.projectService.DeleteProject(projectID)
	if err != nil {
		if err.Error() == "project not found" {
			return c.JSON(http.StatusNotFound, map[string]string{"error": "Проект не найден или уже удалён"})
		}
		log.Printf("service error (delete project): %v", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Ошибка при удалении проекта"})
	}

	return c.JSON(http.StatusOK, response.ProjectUniversalResponse{
		ID:      projectID.String(),
		Message: "Проект удален",
	})
}
