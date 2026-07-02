package httpapi

import (
	models "emplacc-api/internal/domain"
	"emplacc-api/internal/dto/request"
	"emplacc-api/internal/dto/response"
	"emplacc-api/internal/service"
	"emplacc-api/internal/utils"
	"log"
	"net/http"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
)

type TeamController struct {
	teamService    service.TeamService
	freshAvatarURL func(string) string
}

func NewTeamController(teamService service.TeamService, freshAvatarURL func(string) string) *TeamController {
	return &TeamController{
		teamService:    teamService,
		freshAvatarURL: freshAvatarURL,
	}
}

func RegisterTeamRoutes(e Router, teamService service.TeamService, freshAvatarURL func(string) string, managerMw echo.MiddlewareFunc) {
	controller := NewTeamController(teamService, freshAvatarURL)
	g := e.Group("/team")
	g.GET("/all", controller.GetTeams)
	g.GET("/:id", controller.GetTeamByID)
	g.GET("/project/:project_id", controller.GetProjectTeams)
	g.GET("/user/:id", controller.GetTeamByUserId)
	g.POST("", controller.CreateTeam, managerMw)
	g.PATCH("/:id", controller.UpdateTeam, managerMw)
	g.DELETE("/:id", controller.DeleteTeam, managerMw)
	g.POST("/user", controller.AddUserToTeam, managerMw)
	g.DELETE("/user", controller.DeleteUserFromTeam, managerMw)
	g.POST("/project", controller.AddProjectToTeam, managerMw)
	g.DELETE("/project", controller.DeleteProjectFromTeam, managerMw)
	g.PATCH("/member/role", controller.UpdateTeamMemberRole, managerMw)
}

// GetTeams godoc
// @Summary Получение списка всех команд
// @Description Получает список всех команд с их участниками
// @Tags Teams
// @Accept json
// @Produce json
// @Success 200 {object} response.TeamsListResponse "Список команд успешно получен"
// @Security BearerAuth
// @Failure 401 {object} map[string]string "Нет или неверный токен"
// @Failure 500 {object} map[string]string "Ошибка сервера при получении команд"
// @Router /team/all [get]
func (tc *TeamController) GetTeams(c echo.Context) error {
	teams, err := tc.teamService.GetTeams()
	if err != nil {
		log.Printf("service error (get teams): %v", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": "Ошибка при получении списка команд",
		})
	}

	teamListResponse := response.TeamsListResponse{Teams: make([]response.TeamResponse, 0, len(teams))}
	for _, team := range teams {
		members := make([]response.TeamMemberResponse, 0, len(team.TeamMembers))
		for _, tm := range team.TeamMembers {
			user := tm.User
			members = append(members, response.TeamMemberResponse{
				UserID:         user.ID.String(),
				Specialization: utils.GetString(tm.Specialization),
				FirstName:      user.FirstName,
				LastName:       user.LastName,
				Email:          user.Email,
				AvatarURL:      tc.freshAvatarURL(user.AvatarURL),
			})
		}

		teamListResponse.Teams = append(teamListResponse.Teams, response.TeamResponse{
			ID:          team.ID.String(),
			Name:        utils.GetString(team.Name),
			Description: utils.GetString(team.Description),
			UpdatedAt:   utils.GetTime(team.UpdatedAt),
			CreatedAt:   utils.GetTime(team.CreatedAt),
			Members:     members,
		})
	}

	return c.JSON(http.StatusOK, teamListResponse)
}

// GetProjectTeams godoc
// @Summary Получение списка команд проекта
// @Description Получает список всех команд, связанных с проектом, по project_id
// @Tags Projects
// @Accept json
// @Produce json
// @Param project_id path string true "ID проекта"
// @Success 200 {object} response.TeamsListResponse "Список команд успешно получен"
// @Security BearerAuth
// @Failure 400 {object} map[string]string "Некорректный project_id"
// @Failure 401 {object} map[string]string "Нет или неверный токен"
// @Failure 404 {object} map[string]string "Проект или команды не найдены"
// @Failure 500 {object} map[string]string "Ошибка сервера при получении команд"
// @Router /team/project/{project_id} [get]
func (tc *TeamController) GetProjectTeams(c echo.Context) error {
	projectIDStr := c.Param("project_id")
	projectID, err := uuid.Parse(projectIDStr)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "Некорректный project_id",
		})
	}

	projectTeams, err := tc.teamService.GetProjectTeams(projectID)
	if err != nil {
		if err.Error() == "project teams not found" {
			return c.JSON(http.StatusNotFound, map[string]string{
				"error": "Для данного проекта команды не найдены",
			})
		}
		log.Printf("service error (get project teams): %v", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": "Ошибка при получении связей проект-команда",
		})
	}

	seenTeams := make(map[uuid.UUID]models.Team)
	for _, pt := range projectTeams {
		if pt.Team != nil && !*pt.Team.Deleted {
			seenTeams[pt.Team.ID] = *pt.Team
		}
	}

	if len(seenTeams) == 0 {
		return c.JSON(http.StatusNotFound, map[string]string{
			"error": "Нет активных команд для данного проекта",
		})
	}

	teamListResponse := response.TeamsListResponse{Teams: make([]response.TeamResponse, 0, len(seenTeams))}
	for _, team := range seenTeams {
		members := make([]response.TeamMemberResponse, 0, len(team.TeamMembers))
		for _, tm := range team.TeamMembers {
			if tm.User == nil {
				continue
			}
			members = append(members, response.TeamMemberResponse{
				UserID:         tm.User.ID.String(),
				Specialization: utils.GetString(tm.Specialization),
				FirstName:      tm.User.FirstName,
				LastName:       tm.User.LastName,
				Email:          tm.User.Email,
				AvatarURL:      tc.freshAvatarURL(tm.User.AvatarURL),
			})
		}

		teamListResponse.Teams = append(teamListResponse.Teams, response.TeamResponse{
			ID:          team.ID.String(),
			Name:        utils.GetString(team.Name),
			Description: utils.GetString(team.Description),
			UpdatedAt:   utils.GetTime(team.UpdatedAt),
			CreatedAt:   utils.GetTime(team.CreatedAt),
			Members:     members,
		})
	}

	return c.JSON(http.StatusOK, teamListResponse)
}

// GetTeamByID godoc
// @Summary Получение команды по ID
// @Description Получает данные команды по её уникальному идентификатору, включая участников
// @Tags Teams
// @Accept json
// @Produce json
// @Param id path string true "ID команды"
// @Security BearerAuth
// @Failure 401 {object} map[string]string "Нет или неверный токен"
// @Success 200 {object} response.TeamResponse "Команда успешно получена"
// @Failure 400 {object} map[string]string "Некорректный идентификатор команды"
// @Failure 404 {object} map[string]string "Команда не найдена"
// @Failure 500 {object} map[string]string "Ошибка сервера при получении команды"
// @Router /team/{id} [get]
func (tc *TeamController) GetTeamByID(c echo.Context) error {
	teamIDParam := c.Param("id")
	teamUUID, err := uuid.Parse(teamIDParam)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "Некорректный идентификатор команды",
		})
	}

	team, err := tc.teamService.GetTeamByID(teamUUID)
	if err != nil {
		if err.Error() == "team not found" {
			return c.JSON(http.StatusNotFound, map[string]string{
				"error": "Команда не найдена",
			})
		}
		log.Printf("service error (get team by id): %v", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": "Ошибка при получении данных команды",
		})
	}

	membersResp := make([]response.TeamMemberResponse, 0, len(team.TeamMembers))
	for _, tm := range team.TeamMembers {
		if tm.User == nil {
			log.Printf("Warning: TeamMember.UserID=%s has no associated User", tm.UserID)
			continue
		}

		membersResp = append(membersResp, response.TeamMemberResponse{
			UserID:         tm.User.ID.String(),
			Specialization: utils.GetString(tm.Specialization),
			FirstName:      tm.User.FirstName,
			LastName:       tm.User.LastName,
			Email:          tm.User.Email,
			AvatarURL:      tc.freshAvatarURL(tm.User.AvatarURL),
		})
	}

	teamResponse := response.TeamResponse{
		ID:          team.ID.String(),
		Name:        utils.GetString(team.Name),
		Description: utils.GetString(team.Description),
		UpdatedAt:   utils.GetTime(team.UpdatedAt),
		CreatedAt:   utils.GetTime(team.CreatedAt),
		Members:     membersResp,
	}

	return c.JSON(http.StatusOK, teamResponse)
}

// CreateTeam godoc
// @Summary Создание новой команды
// @Description Создает новую команду с указанными параметрами
// @Tags Teams
// @Accept json
// @Produce json
// @Param team body request.TeamCreateRequest true "Данные для создания команды"
// @Security BearerAuth
// @Failure 401 {object} map[string]string "Нет или неверный токен"
// @Success 201 {object} response.TeamUniversalResponse "Команда успешно создана"
// @Failure 400 {object} map[string]string "Ошибка в запросе"
// @Failure 500 {object} map[string]string "Ошибка сервера при создании команды"
// @Router /team [post]
func (tc *TeamController) CreateTeam(c echo.Context) error {
	var req request.TeamCreateRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "Не удалось получить данные из запроса",
		})
	}

	teamID, err := tc.teamService.CreateTeam(req)
	if err != nil {
		log.Printf("service error (create team): %v", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Ошибка при создании команды"})
	}

	createResponse := response.TeamUniversalResponse{
		ID:      teamID.String(),
		Message: "Команда создана",
	}

	return c.JSON(http.StatusCreated, createResponse)
}

// UpdateTeam godoc
// @Summary Обновление команды
// @Description Обновляет данные команды по её ID
// @Tags Teams
// @Accept json
// @Produce json
// @Param id path string true "ID команды"
// @Param team body request.TeamUpdateRequest true "Данные для обновления команды"
// @Security BearerAuth
// @Failure 401 {object} map[string]string "Нет или неверный токен"
// @Success 200 {object} response.TeamUniversalResponse "Команда успешно обновлена"
// @Failure 400 {object} map[string]string "Некорректный идентификатор или ошибка в запросе"
// @Failure 500 {object} map[string]string "Ошибка сервера при обновлении команды"
// @Router /team/{id} [patch]
func (tc *TeamController) UpdateTeam(c echo.Context) error {
	teamIDParam := c.Param("id")
	teamUUID, err := uuid.Parse(teamIDParam)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Некорректный идентификатор команды"})
	}

	var req request.TeamUpdateRequest
	if err := c.Bind(&req); err != nil {
		log.Printf("Bind error: %v", err)
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "Не удалось получить данные из запроса",
		})
	}

	err = tc.teamService.UpdateTeam(teamUUID, req)
	if err != nil {
		if err.Error() == "no fields to update" {
			log.Printf("No fields provided for update")
			return c.JSON(http.StatusBadRequest, map[string]string{
				"error": "Не указаны поля для обновления",
			})
		}
		if err.Error() == "team not found" {
			log.Printf("Team not found: %v", err)
			return c.JSON(http.StatusNotFound, map[string]string{
				"error": "Команда не найдена",
			})
		}
		log.Printf("service error (update team): %v", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Ошибка при обновлении команды"})
	}

	updateResponse := response.TeamUniversalResponse{
		ID:      teamIDParam,
		Message: "Команда обновлена",
	}

	return c.JSON(http.StatusOK, updateResponse)
}

// DeleteTeam godoc
// @Summary Удаление команды
// @Description Логическое удаление команды по ID (поле deleted = true)
// @Tags Teams
// @Accept json
// @Produce json
// @Param id path string true "ID команды"
// @Security BearerAuth
// @Failure 401 {object} map[string]string "Нет или неверный токен"
// @Success 200 {object} response.TeamUniversalResponse "Команда успешно удалена"
// @Failure 404 {object} map[string]string "Команда не найдена"
// @Failure 500 {object} map[string]string "Ошибка сервера при удалении команды"
// @Router /team/{id} [delete]
func (tc *TeamController) DeleteTeam(c echo.Context) error {
	teamIDParam := c.Param("id")

	err := tc.teamService.DeleteTeam(teamIDParam)
	if err != nil {
		if err.Error() == "team not found" {
			return c.JSON(http.StatusNotFound, map[string]string{"message": "Ничего не удалено"})
		}
		log.Printf("service error (delete team): %v", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Ошибка при удалении команды"})
	}

	deleteResponse := response.TeamUniversalResponse{
		ID:      teamIDParam,
		Message: "Команда удалена",
	}

	return c.JSON(http.StatusOK, deleteResponse)
}

// AddUserToTeam godoc
// @Summary Добавление пользователя в команду
// @Description Добавляет пользователя в указанную команду
// @Tags Teams
// @Accept json
// @Produce json
// @Param addUser body request.TeamAddUsersRequest true "Данные для добавления пользователя в команду"
// @Security BearerAuth
// @Failure 401 {object} map[string]string "Нет или неверный токен"
// @Success 200 {object} response.TeamUniversalUserResponse "Пользователь успешно добавлен в команду"
// @Failure 400 {object} map[string]string "Ошибка в запросе или некорректные идентификаторы"
// @Failure 500 {object} map[string]string "Ошибка сервера при добавлении пользователя в команду"
// @Router /team/user [post]
func (tc *TeamController) AddUserToTeam(c echo.Context) error {
	var req request.TeamAddUsersRequest
	if err := c.Bind(&req); err != nil {
		log.Printf("Bind error: %v", err)
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "Не удалось получить данные из запроса",
		})
	}

	_, err := tc.teamService.AddUsersToTeam(req.TeamID, req.UserIDs)
	if err != nil {
		log.Printf("Failed to add users to team: %v", err)

		switch err.Error() {
		case "user not found", "not all users founded":
			return c.JSON(http.StatusBadRequest, map[string]string{
				"error": "Ошибка при получении данных пользователя",
			})
		case "team not found":
			return c.JSON(http.StatusBadRequest, map[string]string{
				"error": "Команда не найдена",
			})
		default:
			// Только настоящие внутренние ошибки → 500
			return c.JSON(http.StatusInternalServerError, map[string]string{
				"error": "Не удалось добавить пользователей в команду",
			})
		}
	}

	addResponse := response.UsersAddResponse{
		TeamID:  req.TeamID,
		UsersID: req.UserIDs,
		Message: "Пользователи добавлены в команду", // исправлена грамматика
	}

	return c.JSON(http.StatusOK, addResponse)
}

// DeleteUserFromTeam godoc
// @Summary Удаление пользователя из команды
// @Description Логически удаляет пользователя из команды (поле deleted = true)
// @Tags Teams
// @Accept json
// @Produce json
// @Param deleteUser body request.TeamDeleteUserRequest true "Данные для удаления пользователя из команды"
// @Security BearerAuth
// @Failure 401 {object} map[string]string "Нет или неверный токен"
// @Success 200 {object} response.TeamUniversalUserResponse "Пользователь успешно удален из команды"
// @Failure 400 {object} map[string]string "Ошибка в запросе или некорректные идентификаторы"
// @Failure 404 {object} map[string]string "Ничего не удалено"
// @Failure 500 {object} map[string]string "Ошибка сервера при удалении пользователя из команды"
// @Router /team/user [delete]
func (tc *TeamController) DeleteUserFromTeam(c echo.Context) error {
	var req request.TeamDeleteUserRequest
	if err := c.Bind(&req); err != nil {
		log.Printf("Bind error: %v", err)
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "Не удалось получить данные из запроса",
		})
	}

	err := tc.teamService.DeleteUserFromTeam(req)
	if err != nil {
		if err.Error() == "team not found" {
			log.Printf("DB error (select team): %v", err)
			return c.JSON(http.StatusInternalServerError, map[string]string{
				"error": "Ошибка при получении команды",
			})
		}
		if err.Error() == "user not found" {
			return c.JSON(http.StatusInternalServerError, map[string]string{
				"error": "Ошибка при получении пользователя",
			})
		}
		if err.Error() == "team member not found" {
			return c.JSON(http.StatusNotFound, map[string]string{"message": "Ничего не удалено"})
		}
		log.Printf("service error (delete user from team): %v", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Ошибка при удалении пользователя из команды"})
	}

	deleteResponse := response.TeamUniversalUserResponse{
		TeamID:  req.TeamID,
		UserID:  req.UserID,
		Message: "Пользователь успешно удален из команды",
	}

	return c.JSON(http.StatusOK, deleteResponse)
}

// AddProjectToTeam godoc
// @Summary Добавление проекта в команду
// @Description Привязывает проект к указанной команде
// @Tags Teams
// @Accept json
// @Produce json
// @Param addProject body request.TeamAddProjectRequest true "Данные для добавления проекта в команду"
// @Security BearerAuth
// @Failure 401 {object} map[string]string "Нет или неверный токен"
// @Success 200 {object} response.TeamUniversalProjectResponse "Проект успешно привязан к команде"
// @Failure 400 {object} map[string]string "Ошибка в запросе или некорректные идентификаторы"
// @Failure 500 {object} map[string]string "Ошибка сервера при добавлении проекта в команду"
// @Router /team/project [post]
func (tc *TeamController) AddProjectToTeam(c echo.Context) error {
	var req request.TeamAddProjectRequest
	if err := c.Bind(&req); err != nil {
		log.Printf("Bind error: %v", err)
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "Не удалось получить данные из запроса",
		})
	}

	projectTeam, err := tc.teamService.AddProjectToTeam(req)
	if err != nil {
		log.Printf("Failed to add project to team: %v", err)

		switch err.Error() {
		case "team not found":
			return c.JSON(http.StatusBadRequest, map[string]string{
				"error": "Команда не найдена",
			})
		case "project not found":
			return c.JSON(http.StatusBadRequest, map[string]string{
				"error": "Проект не найден",
			})
		default:
			// Только настоящие внутренние ошибки → 500
			return c.JSON(http.StatusInternalServerError, map[string]string{
				"error": "Ошибка при привязке проекта к команде",
			})
		}
	}

	addResponse := response.TeamUniversalProjectResponse{
		TeamID:    projectTeam.TeamID.String(),
		ProjectID: projectTeam.ProjectID.String(),
		Message:   "Проект успешно привязан к команде",
	}
	return c.JSON(http.StatusOK, addResponse)
}

// DeleteProjectFromTeam godoc
// @Summary Удаление проекта из команды
// @Description Логически удаляет привязку проекта к команде (поле deleted = true)
// @Tags Teams
// @Accept json
// @Produce json
// @Param deleteProject body request.TeamDeleteProjectRequest true "Данные для удаления проекта из команды"
// @Security BearerAuth
// @Failure 401 {object} map[string]string "Нет или неверный токен"
// @Success 200 {object} response.TeamUniversalProjectResponse "Проект успешно отвязан от команды"
// @Failure 400 {object} map[string]string "Некорректный идентификатор или ошибка в запросе"
// @Failure 404 {object} map[string]string "Ничего не удалено"
// @Failure 500 {object} map[string]string "Ошибка сервера при удалении проекта из команды"
// @Router /team/project [delete]
func (tc *TeamController) DeleteProjectFromTeam(c echo.Context) error {
	var req request.TeamDeleteProjectRequest
	if err := c.Bind(&req); err != nil {
		log.Printf("Bind error: %v", err)
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "Не удалось получить данные из запроса",
		})
	}

	teamID, err := uuid.Parse(req.TeamID)
	if err != nil {
		log.Printf("UUID parse error (teamID): %v", err)
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "Некорректный идентификатор команды",
		})
	}

	projectID, err := uuid.Parse(req.ProjectID)
	if err != nil {
		log.Printf("UUID parse error (projectID): %v", err)
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "Некорректный идентификатор проекта",
		})
	}

	err = tc.teamService.DeleteProjectFromTeam(teamID, projectID)
	if err != nil {
		if err.Error() == "project team not found" {
			return c.JSON(http.StatusNotFound, map[string]string{"message": "Ничего не удалено"})
		}
		log.Printf("service error (delete project from team): %v", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Ошибка при удалении проекта из команды"})
	}

	deleteResponse := response.TeamUniversalProjectResponse{
		TeamID:    teamID.String(),
		ProjectID: projectID.String(),
		Message:   "Проект успешно отвязан от команды",
	}

	return c.JSON(http.StatusOK, deleteResponse)
}

// GetTeamByUserId godoc
// @Summary Получение команд пользователя
// @Description Получает список всех активных команд, в которых состоит пользователь
// @Tags Teams
// @Accept json
// @Produce json
// @Param id path string true "ID пользователя"
// @Success 200 {object} response.TeamsListResponse "Список команд успешно получен"
// @Security BearerAuth
// @Failure 400 {object} map[string]string "Некорректный идентификатор пользователя"
// @Failure 401 {object} map[string]string "Нет или неверный токен"
// @Failure 404 {object} map[string]string "Команды для пользователя не найдены"
// @Failure 500 {object} map[string]string "Ошибка сервера при получении команд"
// @Router /team/user/{id} [get]
func (tc *TeamController) GetTeamByUserId(c echo.Context) error {
	userIDParam := c.Param("id")

	userID, err := uuid.Parse(userIDParam)
	if err != nil {
		log.Printf("UUID parse error (userID): %v", err)
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "Некорректный идентификатор пользователя",
		})
	}

	teams, err := tc.teamService.GetTeamsByUserID(userID)
	if err != nil {
		log.Printf("service error (get teams by user id): %v", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": "Ошибка при получении команд пользователя",
		})
	}

	if len(teams) == 0 {
		return c.JSON(http.StatusNotFound, map[string]string{
			"error": "Команды для данного пользователя не найдены",
		})
	}

	teamListResponse := response.TeamsListResponse{Teams: make([]response.TeamResponse, 0, len(teams))}
	for _, team := range teams {
		members := make([]response.TeamMemberResponse, 0, len(team.TeamMembers))
		for _, tm := range team.TeamMembers {
			if tm.User == nil {
				continue
			}
			members = append(members, response.TeamMemberResponse{
				UserID:         tm.User.ID.String(),
				Specialization: utils.GetString(tm.Specialization),
				FirstName:      tm.User.FirstName,
				LastName:       tm.User.LastName,
				Email:          tm.User.Email,
				AvatarURL:      tc.freshAvatarURL(tm.User.AvatarURL),
			})
		}

		teamListResponse.Teams = append(teamListResponse.Teams, response.TeamResponse{
			ID:          team.ID.String(),
			Name:        utils.GetString(team.Name),
			Description: utils.GetString(team.Description),
			UpdatedAt:   utils.GetTime(team.UpdatedAt),
			CreatedAt:   utils.GetTime(team.CreatedAt),
			Members:     members,
		})
	}

	return c.JSON(http.StatusOK, teamListResponse)
}

// UpdateTeamMemberRole godoc
// @Summary Изменение роли участника в команде
// @Description Обновляет специализацию (роль) пользователя в указанной команде
// @Tags Teams
// @Accept json
// @Produce json
// @Param updateRole body request.TeamUpdateMemberRoleRequest true "Данные для обновления роли"
// @Security BearerAuth
// @Success 200 {object} response.TeamUniversalUserResponse "Роль успешно обновлена"
// @Failure 400 {object} map[string]string "Ошибка в запросе или некорректные идентификаторы"
// @Failure 401 {object} map[string]string "Нет или неверный токен"
// @Failure 404 {object} map[string]string "Участник команды не найден"
// @Failure 500 {object} map[string]string "Ошибка сервера при обновлении роли"
// @Router /team/member/role [patch]
func (tc *TeamController) UpdateTeamMemberRole(c echo.Context) error {
	var req request.TeamUpdateMemberRoleRequest
	if err := c.Bind(&req); err != nil {
		log.Printf("Bind error (update role): %v", err)
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "Не удалось получить данные из запроса",
		})
	}

	err := tc.teamService.UpdateTeamMemberRole(req.TeamID, req.UserID, req.Specialization)
	if err != nil {
		switch err.Error() {
		case "team not found":
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "Команда не найдена"})
		case "user not found":
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "Пользователь не найден"})
		case "team member not found":
			return c.JSON(http.StatusNotFound, map[string]string{"error": "Участник команды не найден"})
		case "no changes":
			return c.JSON(http.StatusOK, response.TeamUniversalUserResponse{
				TeamID:  req.TeamID,
				UserID:  req.UserID,
				Message: "Роль не изменилась",
			})
		default:
			log.Printf("service error (update team member role): %v", err)
			return c.JSON(http.StatusInternalServerError, map[string]string{
				"error": "Ошибка при обновлении роли участника",
			})
		}
	}

	return c.JSON(http.StatusOK, response.TeamUniversalUserResponse{
		TeamID:  req.TeamID,
		UserID:  req.UserID,
		Message: "Роль успешно обновлена",
	})
}
