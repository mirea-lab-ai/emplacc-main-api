package service

import (
	models "emplacc-api/internal/domain"
	"emplacc-api/internal/dto/request"
	"emplacc-api/internal/ports"
	"errors"
	"time"

	"github.com/google/uuid"
)

type TeamService interface {
	GetTeams() ([]models.Team, error)
	GetProjectTeams(projectID uuid.UUID) ([]models.ProjectTeam, error)
	GetTeamByID(teamUUID uuid.UUID) (*models.Team, error)
	CreateTeam(req request.TeamCreateRequest) (uuid.UUID, error)
	UpdateTeam(teamUUID uuid.UUID, req request.TeamUpdateRequest) error
	DeleteTeam(teamIDParam string) error
	AddUsersToTeam(teamId string, userIds []string) ([]models.TeamMember, error)
	DeleteUserFromTeam(req request.TeamDeleteUserRequest) error
	AddProjectToTeam(req request.TeamAddProjectRequest) (*models.ProjectTeam, error)
	DeleteProjectFromTeam(teamID uuid.UUID, projectID uuid.UUID) error
	GetTeamsByUserID(userID uuid.UUID) ([]models.Team, error)
	UpdateTeamMemberRole(teamID, userID, specialization string) error
}

type teamService struct {
	repo ports.TeamRepository
}

func NewTeamService(repo ports.TeamRepository) TeamService {
	return &teamService{
		repo: repo,
	}
}

func (s *teamService) GetTeams() ([]models.Team, error) {
	return s.repo.GetTeams()
}

func (s *teamService) GetTeamsByUserID(userID uuid.UUID) ([]models.Team, error) {
	return s.repo.GetTeamsByUserID(userID)
}

func (s *teamService) GetProjectTeams(projectID uuid.UUID) ([]models.ProjectTeam, error) {
	return s.repo.GetProjectTeams(projectID)
}

func (s *teamService) GetTeamByID(teamUUID uuid.UUID) (*models.Team, error) {
	return s.repo.GetTeamByID(teamUUID)
}

func (s *teamService) CreateTeam(req request.TeamCreateRequest) (uuid.UUID, error) {
	newUUID := uuid.New()

	var name *string
	if req.Name != "" {
		name = &req.Name
	}
	var description *string
	if req.Description != "" {
		description = &req.Description
	}

	del := false
	now := time.Now()

	team := models.Team{
		ID:          newUUID,
		Name:        name,
		Description: description,
		Deleted:     &del,
		CreatedAt:   &now,
	}

	err := s.repo.CreateTeam(team)
	if err != nil {
		return uuid.Nil, err
	}

	users, err := s.repo.GetUsersFromArray(req.UsersIDs)
	if err != nil {
		return uuid.Nil, errors.New("user not found")
	}

	if len(req.UsersIDs) != len(users) {
		return uuid.Nil, errors.New("not all users founded")
	}

	teamMembers := []models.TeamMember{}
	if len(users) > 0 {
		for _, user := range users {
			teamMembers = append(teamMembers, models.TeamMember{
				UserID:         user.ID,
				TeamID:         team.ID,
				Specialization: &user.Profession,
				Deleted:        &del,
			})
		}

		err = s.repo.UpsertTeamMembers(teamMembers)
		if err != nil {
			return uuid.Nil, err
		}
	}

	return newUUID, nil
}

func (s *teamService) UpdateTeam(teamUUID uuid.UUID, req request.TeamUpdateRequest) error {
	updateData := make(map[string]interface{})
	if req.Name != nil {
		updateData["name"] = *req.Name
	}
	if req.Description != nil {
		updateData["description"] = *req.Description
	}
	if len(updateData) == 0 {
		return errors.New("no fields to update")
	}
	updateData["updated_at"] = time.Now()

	// существует и не удалена
	exists, err := s.repo.TeamExists(teamUUID)
	if err != nil {
		return err
	}
	if !exists {
		return errors.New("team not found")
	}

	updated, err := s.repo.UpdateTeam(teamUUID, updateData)
	if err != nil {
		return err
	}

	if !updated {
		return errors.New("team not found")
	}

	return nil
}

func (s *teamService) DeleteTeam(teamIDParam string) error {
	deleted, err := s.repo.DeleteTeam(teamIDParam)
	if err != nil {
		return err
	}

	if !deleted {
		return errors.New("team not found")
	}

	return nil
}

func (s *teamService) AddUsersToTeam(teamId string, userIds []string) ([]models.TeamMember, error) {
	users, err := s.repo.GetUsersFromArray(userIds)
	if err != nil {
		return nil, errors.New("user not found")
	}

	if len(userIds) != len(users) {
		return nil, errors.New("not all users founded")
	}

	team, err := s.repo.GetTeam(teamId)
	if err != nil {
		return nil, errors.New("team not found")
	}

	del := false
	teamMembers := make([]models.TeamMember, len(users))
	for i, user := range users {
		teamMembers[i] = models.TeamMember{
			UserID:         user.ID,
			TeamID:         team.ID,
			Specialization: &user.Profession,
			Deleted:        &del,
		}
	}

	// Используем UPSERT вместо Create
	err = s.repo.UpsertTeamMembers(teamMembers)
	if err != nil {
		return nil, err
	}

	return teamMembers, nil
}

func (s *teamService) DeleteUserFromTeam(req request.TeamDeleteUserRequest) error {
	team, err := s.repo.GetTeam(req.TeamID)
	if err != nil {
		return errors.New("team not found")
	}

	user, err := s.repo.GetUser(req.UserID)
	if err != nil {
		return errors.New("user not found")
	}

	deleted, err := s.repo.DeleteTeamMember(user.ID, team.ID)
	if err != nil {
		return err
	}

	if !deleted {
		return errors.New("team member not found")
	}

	return nil
}

func (s *teamService) AddProjectToTeam(req request.TeamAddProjectRequest) (*models.ProjectTeam, error) {
	team, err := s.repo.GetTeam(req.TeamID)
	if err != nil {
		return nil, errors.New("team not found")
	}

	project, err := s.repo.GetProject(req.ProjectID)
	if err != nil {
		return nil, errors.New("project not found")
	}

	del := false
	projectTeam := models.ProjectTeam{
		ProjectID: project.ID,
		TeamID:    team.ID,
		Deleted:   &del,
	}

	// Используем UPSERT вместо Create
	err = s.repo.UpsertProjectTeam(projectTeam)
	if err != nil {
		return nil, err
	}

	return &projectTeam, nil
}

func (s *teamService) DeleteProjectFromTeam(teamID uuid.UUID, projectID uuid.UUID) error {
	updateData := map[string]interface{}{
		"deleted":    true,
		"updated_at": time.Now(),
	}

	deleted, err := s.repo.DeleteProjectTeam(teamID, projectID, updateData)
	if err != nil {
		return err
	}

	if !deleted {
		return errors.New("project team not found")
	}

	return nil
}

func (s *teamService) UpdateTeamMemberRole(teamID, userID, specialization string) error {
	teamUUID, err := uuid.Parse(teamID)
	if err != nil {
		return errors.New("team not found") // или отдельная ошибка, но для простоты
	}
	userUUID, err := uuid.Parse(userID)
	if err != nil {
		return errors.New("user not found")
	}

	// Проверим, что команда и пользователь существуют
	_, err = s.repo.GetTeam(teamID)
	if err != nil {
		return errors.New("team not found")
	}
	_, err = s.repo.GetUser(userID)
	if err != nil {
		return errors.New("user not found")
	}

	// Обновляем specialization
	updated, err := s.repo.UpdateTeamMemberSpecialization(teamUUID, userUUID, specialization)
	if err != nil {
		return err
	}
	if !updated {
		return errors.New("team member not found")
	}

	return nil
}
