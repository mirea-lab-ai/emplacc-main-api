package ports

import (
	models "emplacc-api/internal/domain"

	"github.com/google/uuid"
)

type TeamRepository interface {
	GetTeams() ([]models.Team, error)
	GetProjectTeams(projectID uuid.UUID) ([]models.ProjectTeam, error)
	GetTeamByID(teamUUID uuid.UUID) (*models.Team, error)
	CreateTeam(team models.Team) error
	TeamExists(teamUUID uuid.UUID) (bool, error)
	UpdateTeam(teamUUID uuid.UUID, updateData map[string]interface{}) (bool, error)
	DeleteTeam(teamIDParam string) (bool, error)
	GetUser(userID string) (*models.User, error)
	GetTeam(teamID string) (*models.Team, error)
	GetProject(projectID string) (*models.Project, error)
	CreateTeamMember(teamMember models.TeamMember) error
	UpsertTeamMembers(teamMember []models.TeamMember) error
	DeleteTeamMember(userID uuid.UUID, teamID uuid.UUID) (bool, error)
	UpsertProjectTeam(projectTeam models.ProjectTeam) error
	DeleteProjectTeam(teamID uuid.UUID, projectID uuid.UUID, updateData map[string]interface{}) (bool, error)
	GetUsersFromArray(userIDs []string) ([]models.User, error)
	GetTeamsByUserID(userID uuid.UUID) ([]models.Team, error)
	UpdateTeamMemberSpecialization(teamID, userID uuid.UUID, specialization string) (bool, error)
}
