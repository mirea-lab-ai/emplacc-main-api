package postgres

import (
	models "emplacc-api/internal/domain"
	"emplacc-api/internal/ports"
	"errors"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type teamRepository struct {
	db *gorm.DB
}

func NewTeamRepository(db *gorm.DB) ports.TeamRepository {
	return &teamRepository{
		db: db,
	}
}

func (r *teamRepository) GetTeams() ([]models.Team, error) {
	var teams []models.Team

	err := r.db.Session(&gorm.Session{NewDB: true}).
		Where("deleted = ?", false).
		Preload("TeamMembers", "deleted = ?", false).
		Preload("TeamMembers.User", "deleted = ?", false).
		Find(&teams).Error

	if err != nil {
		return nil, err
	}

	return teams, nil
}

func (r *teamRepository) GetTeamsByUserID(userID uuid.UUID) ([]models.Team, error) {
	var teams []models.Team

	err := r.db.Session(&gorm.Session{NewDB: true}).
		Joins("JOIN team_members ON teams.id = team_members.team_id").
		Where("team_members.user_id = ? AND teams.deleted = ? AND team_members.deleted = ?",
			userID, false, false).
		Preload("TeamMembers", "deleted = ?", false).
		Preload("TeamMembers.User", "deleted = ?", false).
		Find(&teams).Error

	if err != nil {
		return nil, err
	}

	return teams, nil
}

func (r *teamRepository) GetProjectTeams(projectID uuid.UUID) ([]models.ProjectTeam, error) {
	var projectTeams []models.ProjectTeam
	err := r.db.Session(&gorm.Session{NewDB: true}).
		Where("project_id = ? AND deleted = ?", projectID, false).
		Preload("Team", "deleted = ?", false).
		Preload("Team.TeamMembers", "deleted = ?", false).
		Preload("Team.TeamMembers.User", "deleted = ?", false).
		Find(&projectTeams).Error

	if err != nil {
		return nil, err
	}

	return projectTeams, nil
}

func (r *teamRepository) GetTeamByID(teamUUID uuid.UUID) (*models.Team, error) {
	var team models.Team
	err := r.db.Session(&gorm.Session{NewDB: true}).
		Where("id = ? AND deleted = ?", teamUUID, false).
		Preload("TeamMembers", "deleted = ?", false).
		Preload("TeamMembers.User", "deleted = ?", false).
		First(&team).Error

	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, errors.New("team not found")
		}
		return nil, err
	}

	return &team, nil
}

func (r *teamRepository) CreateTeam(team models.Team) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		if res := tx.Session(&gorm.Session{NewDB: true}).Model(models.Team{}).Omit(clause.Associations).Create(&team); res.Error != nil {
			return res.Error
		}
		return nil
	})
}

func (r *teamRepository) TeamExists(teamUUID uuid.UUID) (bool, error) {
	var count int64
	if err := r.db.Session(&gorm.Session{NewDB: true}).Model(models.Team{}).
		Where("id = ? AND deleted = FALSE", teamUUID).
		Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}

func (r *teamRepository) UpdateTeam(teamUUID uuid.UUID, updateData map[string]interface{}) (bool, error) {
	var affected int64
	err := r.db.Transaction(func(tx *gorm.DB) error {
		res := tx.Session(&gorm.Session{NewDB: true}).Model(&models.Team{}).Where("id = ?", teamUUID).Updates(updateData)
		if res.Error != nil {
			return res.Error
		}
		affected = res.RowsAffected
		return nil
	})

	if err != nil {
		return false, err
	}

	return affected > 0, nil
}

func (r *teamRepository) DeleteTeam(teamIDParam string) (bool, error) {
	updateData := map[string]interface{}{"deleted": true}

	var affected int64
	err := r.db.Transaction(func(tx *gorm.DB) error {
		res := tx.Session(&gorm.Session{NewDB: true}).Model(models.Team{}).Where("id = ?", teamIDParam).Updates(updateData)
		if res.Error != nil {
			return res.Error
		}
		affected = res.RowsAffected
		if affected == 0 {
			return errors.New("team not found")
		}

		if res = tx.Session(&gorm.Session{NewDB: true}).Model(models.TeamMember{}).Where("team_id = ?", teamIDParam).Updates(updateData); res.Error != nil {
			return res.Error
		}
		if res = tx.Session(&gorm.Session{NewDB: true}).Model(models.ProjectTeam{}).Where("team_id = ?", teamIDParam).Updates(updateData); res.Error != nil {
			return res.Error
		}
		return nil
	})

	if err != nil {
		if err.Error() == "team not found" {
			return false, nil
		}
		return false, err
	}

	return affected > 0, nil
}

func (r *teamRepository) GetUser(userID string) (*models.User, error) {
	var user models.User
	if err := r.db.Session(&gorm.Session{NewDB: true}).Model(models.User{}).
		Select("id, profession").
		Where("id = ? AND deleted = FALSE", userID).
		First(&user).Error; err != nil {
		return nil, err
	}
	return &user, nil
}

func (r *teamRepository) GetUsersFromArray(userIds []string) ([]models.User, error) {
	if len(userIds) < 1 {
		return []models.User{}, nil
	}
	userIDs := []uuid.UUID{}
	for _, id := range userIds {
		userUUID, err := uuid.Parse(id)
		if err != nil {
			return nil, err
		}
		userIDs = append(userIDs, userUUID)
	}

	var users []models.User
	if err := r.db.Session(&gorm.Session{NewDB: true}).Model(models.User{}).
		Where("deleted = FALSE and id IN ?", userIDs).
		Find(&users).Error; err != nil {
		return nil, err
	}
	return users, nil
}

func (r *teamRepository) GetTeam(teamID string) (*models.Team, error) {
	var team models.Team
	if err := r.db.Session(&gorm.Session{NewDB: true}).Model(models.Team{}).
		Where("id = ? AND deleted = FALSE", teamID).
		First(&team).Error; err != nil {
		return nil, err
	}
	return &team, nil
}

func (r *teamRepository) GetProject(projectID string) (*models.Project, error) {
	var project models.Project
	if err := r.db.Session(&gorm.Session{NewDB: true}).Model(models.Project{}).
		Where("id = ? AND deleted = FALSE", projectID).
		First(&project).Error; err != nil {
		return nil, err
	}
	return &project, nil
}

func (r *teamRepository) CreateTeamMember(teamMember models.TeamMember) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		if res := tx.Session(&gorm.Session{NewDB: true}).Model(models.TeamMember{}).Create(&teamMember); res.Error != nil {
			return res.Error
		}
		return nil
	})
}

func (r *teamRepository) UpsertTeamMembers(teamMembers []models.TeamMember) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		return tx.Clauses(clause.OnConflict{
			Columns: []clause.Column{
				{Name: "user_id"},
				{Name: "team_id"},
			},
			DoUpdates: clause.Assignments(map[string]interface{}{
				"deleted":        false,
				"specialization": gorm.Expr("EXCLUDED.specialization"),
				"updated_at":     gorm.Expr("NOW()"),
			}),
		}).Create(&teamMembers).Error
	})
}

func (r *teamRepository) DeleteTeamMember(userID uuid.UUID, teamID uuid.UUID) (bool, error) {
	updateData := map[string]interface{}{"deleted": true}
	var affected int64
	err := r.db.Transaction(func(tx *gorm.DB) error {
		res := tx.Session(&gorm.Session{NewDB: true}).Model(models.TeamMember{}).
			Where("user_id = ? AND team_id = ?", userID, teamID).
			Updates(updateData)
		if res.Error != nil {
			return res.Error
		}
		affected = res.RowsAffected
		if affected == 0 {
			return errors.New("team member not found")
		}
		return nil
	})

	if err != nil {
		if err.Error() == "team member not found" {
			return false, nil
		}
		return false, err
	}

	return affected > 0, nil
}

func (r *teamRepository) UpsertProjectTeam(projectTeam models.ProjectTeam) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		return tx.Clauses(clause.OnConflict{
			Columns: []clause.Column{
				{Name: "project_id"},
				{Name: "team_id"},
			},
			DoUpdates: clause.Assignments(map[string]interface{}{
				"deleted":    false,
				"updated_at": gorm.Expr("NOW()"),
			}),
		}).Create(&projectTeam).Error
	})
}

func (r *teamRepository) DeleteProjectTeam(teamID uuid.UUID, projectID uuid.UUID, updateData map[string]interface{}) (bool, error) {
	var affected int64
	err := r.db.Transaction(func(tx *gorm.DB) error {
		res := tx.Session(&gorm.Session{NewDB: true}).Model(models.ProjectTeam{}).
			Where("team_id = ? AND project_id = ?", teamID, projectID).
			Updates(updateData)
		if res.Error != nil {
			return res.Error
		}
		affected = res.RowsAffected
		if affected == 0 {
			return errors.New("project team not found")
		}
		return nil
	})

	if err != nil {
		if err.Error() == "project team not found" {
			return false, nil
		}
		return false, err
	}

	return affected > 0, nil
}

func (r *teamRepository) UpdateTeamMemberSpecialization(teamID, userID uuid.UUID, specialization string) (bool, error) {
	var affected int64
	err := r.db.Transaction(func(tx *gorm.DB) error {
		res := tx.Model(&models.TeamMember{}).
			Where("team_id = ? AND user_id = ? AND deleted = ?", teamID, userID, false).
			Update("specialization", &specialization)
		if res.Error != nil {
			return res.Error
		}
		affected = res.RowsAffected
		if affected == 0 {
			return errors.New("team member not found")
		}
		return nil
	})

	if err != nil {
		if err.Error() == "team member not found" {
			return false, nil
		}
		return false, err
	}

	return affected > 0, nil
}
