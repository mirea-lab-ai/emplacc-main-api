package service

import (
	models "emplacc-api/internal/domain"
	"emplacc-api/internal/dto/request"
	"emplacc-api/internal/ports"
	"emplacc-api/internal/utils"
	"errors"
	"time"

	"github.com/google/uuid"
)

type ProjectService interface {
	GetAllProjects(page, pageSize int) ([]models.Project, int64, error)
	GetProjectByID(projectID uuid.UUID) (*models.Project, error)
	GetProjectsByUser(userID uuid.UUID) ([]models.Project, error)
	GetTeamProjects(teamID uuid.UUID) ([]models.ProjectTeam, error)
	GetProjectsByIDs(projectIDs []uuid.UUID) ([]models.Project, error)
	CreateProject(req request.CreateProjectRequest) (uuid.UUID, error)
	UpdateProject(projectID uuid.UUID, req request.UpdateProjectRequest) error
	DeleteProject(projectID uuid.UUID) error
	SearchProjects(query, userID string, page, pageSize int) ([]models.Project, int64, error)
}

type projectService struct {
	repo ports.ProjectRepository
}

func NewProjectService(repo ports.ProjectRepository) ProjectService {
	return &projectService{
		repo: repo,
	}
}

func (s *projectService) GetAllProjects(page, pageSize int) ([]models.Project, int64, error) {
	offset := (page - 1) * pageSize
	return s.repo.GetAllProjects(pageSize, offset)
}

func (s *projectService) SearchProjects(query, userID string, page, pageSize int) ([]models.Project, int64, error) {
	offset := (page - 1) * pageSize
	return s.repo.SearchProjects(query, userID, pageSize, offset)
}

func (s *projectService) GetProjectByID(projectID uuid.UUID) (*models.Project, error) {
	return s.repo.GetProjectByID(projectID)
}

func (s *projectService) GetProjectsByUser(userID uuid.UUID) ([]models.Project, error) {
	return s.repo.GetProjectsByUser(userID)
}

func (s *projectService) GetTeamProjects(teamID uuid.UUID) ([]models.ProjectTeam, error) {
	return s.repo.GetTeamProjects(teamID)
}

func (s *projectService) GetProjectsByIDs(projectIDs []uuid.UUID) ([]models.Project, error) {
	return s.repo.GetProjectsByIDs(projectIDs)
}

func (s *projectService) CreateProject(req request.CreateProjectRequest) (uuid.UUID, error) {
	if req.CreatedBy == nil {
		return uuid.Nil, errors.New("created_by is required")
	}

	creatorUUID, err := uuid.Parse(*req.CreatedBy)
	if err != nil {
		return uuid.Nil, errors.New("invalid creator id")
	}

	now := time.Now()
	deleted := false
	projectID := uuid.New()
	tr := true

	project := models.Project{
		ID:              projectID,
		Name:            req.Name,
		Description:     req.Description,
		CreatedAt:       &now,
		CreatedBy:       &creatorUUID,
		Status:          req.Status,
		GitlabProjectID: req.Gitlab_project_id,
		GitlabURL:       req.Gitlab_url,
		Deleted:         &deleted,
		UpdatedAt:       &now,
	}

	// Создаём главную доску
	boardID := uuid.New()
	mainBoard := models.Board{
		ID:          boardID,
		ProjectID:   projectID,
		Name:        strPtr("Главная"),
		Description: strPtr("Главная доска проекта"),
		Deleted:     &deleted,
		CreatedAt:   &now,
		UpdatedAt:   &now,
	}

	// Создаём два статуса: начальный и конечный
	makeStatus := func(order int, name, color string, isOpen bool) models.Status {
		id := uuid.New()
		key := id.String()[:8] // сокращённый UUID (8 символов)
		return models.Status{
			ID:        id,
			BoardID:   boardID,
			SortOrder: &order,
			Key:       &key,
			Name:      &name,
			Color:     &color,
			IsDefault: &tr,
			IsActive:  utils.BoolPtr(true),
			IsOpen:    &isOpen,
			Deleted:   &deleted,
			CreatedAt: &now,
			UpdatedAt: &now,
		}
	}

	statuses := []models.Status{
		makeStatus(0, "To Do", "#FF0000", true), // Начальный статус
		makeStatus(1, "Done", "#00FF00", false), // Конечный статус
	}

	err = s.repo.CreateProjectWithBoardAndStatuses(project, mainBoard, statuses)
	if err != nil {
		return uuid.Nil, err
	}

	return project.ID, nil
}

func (s *projectService) UpdateProject(projectID uuid.UUID, req request.UpdateProjectRequest) error {
	updateData := make(map[string]interface{})
	if req.Name != nil {
		updateData["name"] = *req.Name
	}
	if req.Description != nil {
		updateData["description"] = *req.Description
	}
	if req.GitlabProjectId != nil {
		updateData["gitlab_project_id"] = *req.GitlabProjectId
	}
	if req.GitlabUrl != nil {
		updateData["gitlab_url"] = *req.GitlabUrl
	}
	if req.Status != nil {
		updateData["status"] = *req.Status
	}
	if len(updateData) == 0 {
		return errors.New("no fields to update")
	}

	now := time.Now()
	updateData["updated_at"] = &now

	updated, err := s.repo.UpdateProject(projectID, updateData)
	if err != nil {
		return err
	}

	if !updated {
		return errors.New("project not found")
	}

	return nil
}

func (s *projectService) DeleteProject(projectID uuid.UUID) error {
	deleted, err := s.repo.DeleteProject(projectID)
	if err != nil {
		return err
	}

	if !deleted {
		return errors.New("project not found")
	}

	return nil
}

func strPtr(s string) *string {
	return &s
}
