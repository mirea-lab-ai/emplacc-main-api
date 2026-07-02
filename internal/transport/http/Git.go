package httpapi

import (
	"emplacc-api/internal/service"
	"log"
	"net/http"
	"strconv"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
)

type GitController struct {
	gitService service.GitService
}

func NewGitController(gitService service.GitService) *GitController {
	return &GitController{gitService: gitService}
}

// RegisterGitRoutes — модуль трекера коммитов. Чтение — любой авторизованный;
// привязка репозиториев/синк — менеджер/админ.
func RegisterGitRoutes(e Router, gitService service.GitService, employeeMw echo.MiddlewareFunc, managerMw echo.MiddlewareFunc) {
	c := NewGitController(gitService)

	e.GET("/git/providers", c.Providers)

	e.GET("/project/:id/repos", c.ListRepos)
	e.POST("/project/:id/repos", c.LinkRepo, managerMw)
	e.POST("/project/:id/sync", c.SyncProject, employeeMw)
	e.GET("/project/:id/commits", c.ProjectCommits)

	e.DELETE("/repo/:id", c.UnlinkRepo, managerMw)
	e.POST("/repo/:id/sync", c.SyncRepo, employeeMw)

	e.GET("/task/:id/commits", c.TaskCommits)
	e.PATCH("/commit/:id/task", c.LinkCommitTask, employeeMw)
}

type linkRepoRequest struct {
	Provider string `json:"provider"`
	Owner    string `json:"owner"`
	Name     string `json:"name"`
	URL      string `json:"url"`
	Branch   string `json:"branch"`
}

type linkCommitRequest struct {
	TaskID *string `json:"task_id"`
}

func pageParams(c echo.Context) (int, int) {
	page, _ := strconv.Atoi(c.QueryParam("page"))
	if page <= 0 {
		page = 1
	}
	size, _ := strconv.Atoi(c.QueryParam("pagesize"))
	if size <= 0 || size > 100 {
		size = 50
	}
	return page, size
}

func (gc *GitController) Providers(c echo.Context) error {
	return c.JSON(http.StatusOK, map[string]any{"providers": gc.gitService.Providers()})
}

func (gc *GitController) ListRepos(c echo.Context) error {
	projectID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Некорректный идентификатор проекта"})
	}
	repos, err := gc.gitService.ListRepositories(projectID)
	if err != nil {
		log.Printf("git: list repos: %v", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Ошибка при получении репозиториев"})
	}
	return c.JSON(http.StatusOK, map[string]any{"repositories": repos})
}

func (gc *GitController) LinkRepo(c echo.Context) error {
	projectID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Некорректный идентификатор проекта"})
	}
	var req linkRepoRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Не удалось получить данные из запроса"})
	}
	repo, err := gc.gitService.LinkRepository(service.LinkRepositoryRequest{
		ProjectID: projectID, Provider: req.Provider, Owner: req.Owner, Name: req.Name, URL: req.URL, Branch: req.Branch,
	})
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
	}
	return c.JSON(http.StatusCreated, repo)
}

func (gc *GitController) UnlinkRepo(c echo.Context) error {
	repoID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Некорректный идентификатор репозитория"})
	}
	if err := gc.gitService.UnlinkRepository(repoID); err != nil {
		if err.Error() == "repository not found" {
			return c.JSON(http.StatusNotFound, map[string]string{"error": "Репозиторий не найден"})
		}
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Ошибка при удалении репозитория"})
	}
	return c.JSON(http.StatusOK, map[string]string{"message": "Репозиторий отвязан"})
}

func (gc *GitController) SyncRepo(c echo.Context) error {
	repoID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Некорректный идентификатор репозитория"})
	}
	summary, err := gc.gitService.SyncRepository(c.Request().Context(), repoID)
	if err != nil {
		log.Printf("git: sync repo: %v", err)
		return c.JSON(http.StatusBadGateway, map[string]string{"error": "Ошибка синхронизации: " + err.Error()})
	}
	return c.JSON(http.StatusOK, summary)
}

func (gc *GitController) SyncProject(c echo.Context) error {
	projectID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Некорректный идентификатор проекта"})
	}
	summary, err := gc.gitService.SyncProject(c.Request().Context(), projectID)
	if err != nil {
		log.Printf("git: sync project: %v", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Ошибка синхронизации"})
	}
	return c.JSON(http.StatusOK, summary)
}

func (gc *GitController) ProjectCommits(c echo.Context) error {
	projectID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Некорректный идентификатор проекта"})
	}
	page, size := pageParams(c)
	commits, total, err := gc.gitService.ListCommitsByProject(projectID, page, size)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Ошибка при получении коммитов"})
	}
	return c.JSON(http.StatusOK, map[string]any{"commits": commits, "total_count": total, "page": page, "page_size": size})
}

func (gc *GitController) TaskCommits(c echo.Context) error {
	taskID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Некорректный идентификатор задачи"})
	}
	page, size := pageParams(c)
	commits, total, err := gc.gitService.ListCommitsByTask(taskID, page, size)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Ошибка при получении коммитов"})
	}
	return c.JSON(http.StatusOK, map[string]any{"commits": commits, "total_count": total, "page": page, "page_size": size})
}

func (gc *GitController) LinkCommitTask(c echo.Context) error {
	commitID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Некорректный идентификатор коммита"})
	}
	var req linkCommitRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Не удалось получить данные из запроса"})
	}
	var taskID *uuid.UUID
	if req.TaskID != nil && *req.TaskID != "" {
		parsed, err := uuid.Parse(*req.TaskID)
		if err != nil {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "Некорректный идентификатор задачи"})
		}
		taskID = &parsed
	}
	if err := gc.gitService.LinkCommitToTask(commitID, taskID); err != nil {
		if err.Error() == "commit not found" {
			return c.JSON(http.StatusNotFound, map[string]string{"error": "Коммит не найден"})
		}
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Ошибка при привязке коммита"})
	}
	return c.JSON(http.StatusOK, map[string]string{"message": "Коммит обновлён"})
}
