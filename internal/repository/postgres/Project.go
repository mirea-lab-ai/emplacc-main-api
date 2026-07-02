package postgres

import (
	models "emplacc-api/internal/domain"
	"emplacc-api/internal/ports"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type projectRepository struct {
	db *gorm.DB
}

func NewProjectRepository(db *gorm.DB) ports.ProjectRepository {
	return &projectRepository{
		db: db,
	}
}

func (r *projectRepository) GetAllProjects(limit, offset int) ([]models.Project, int64, error) {
	var totalCount int64
	if err := r.db.Session(&gorm.Session{NewDB: true}).
		Model(&models.Project{}).
		Where("projects.deleted = FALSE").
		Count(&totalCount).Error; err != nil {
		return nil, 0, err
	}

	var projects []models.Project
	if err := r.db.Session(&gorm.Session{NewDB: true}).
		Model(&models.Project{}).
		Where("projects.deleted = FALSE").
		Limit(limit).Offset(offset).
		Find(&projects).Error; err != nil {
		return nil, 0, err
	}

	return projects, totalCount, nil
}

func (r *projectRepository) SearchProjects(query, userID string, limit, offset int) ([]models.Project, int64, error) {
	if query == "" {
		return []models.Project{}, 0, nil
	}

	var totalCount int64
	var projects []models.Project

	// Подготавливаем поисковые запросы
	searchQueries := prepareSearchQueries(query)
	if len(searchQueries) == 0 {
		return []models.Project{}, 0, nil
	}

	// Приоритетная сортировка: сначала проекты команд пользователя, затем созданные им.
	// userID связывается как параметр (а не конкатенацией) — иначе это SQL-инъекция в ORDER BY.
	orderClause := clause.Expr{
		SQL: `
        CASE
            WHEN EXISTS (
                SELECT 1 FROM project_teams pt
                JOIN teams t ON pt.team_id = t.id AND t.deleted = false
                JOIN team_members tm ON t.id = tm.team_id AND tm.deleted = false
                WHERE pt.project_id = projects.id AND pt.deleted = false AND tm.user_id = ?::uuid
            ) THEN 1
            WHEN projects.created_by = ?::uuid THEN 2
            ELSE 3
        END ASC,
        projects.name ASC
    `,
		Vars: []interface{}{userID, userID},
	}

	// Базовый запрос
	baseQuery := r.db.Session(&gorm.Session{NewDB: true}).Model(&models.Project{}).
		Where("projects.deleted = ?", false)

	// Добавляем условия Full-Text Search
	baseQuery = addProjectFullTextConditions(baseQuery, searchQueries)

	// Считаем общее количество
	if err := baseQuery.Count(&totalCount).Error; err != nil {
		return nil, 0, err
	}

	if totalCount == 0 {
		return []models.Project{}, 0, nil
	}

	// Получаем проекты с приоритетной сортировкой
	err := baseQuery.
		Preload("CreatedByUser", func(db *gorm.DB) *gorm.DB {
			return db.Session(&gorm.Session{}).Select("id, first_name, last_name, email").Where("deleted = ?", false)
		}).
		// Убираем лишние прелоады для оптимизации
		Preload("ProjectTeams.Team.TeamMembers", func(db *gorm.DB) *gorm.DB {
			return db.Session(&gorm.Session{}).Where("team_members.user_id = ?", userID).Limit(1) // Только нужного пользователя
		}).
		Limit(limit).
		Offset(offset).
		Order(orderClause).
		Find(&projects).Error

	return projects, totalCount, err
}

func addProjectFullTextConditions(db *gorm.DB, searchQueries []string) *gorm.DB {
	if len(searchQueries) == 0 {
		return db
	}

	var conditions []string
	var args []interface{}

	for _, tsQuery := range searchQueries {
		condition := `(
            to_tsvector('russian', projects.name) @@ to_tsquery(?) OR 
            to_tsvector('russian', projects.description) @@ to_tsquery(?) OR 
            to_tsvector('russian', projects.gitlab_url) @@ to_tsquery(?)
        )`

		conditions = append(conditions, condition)
		for i := 0; i < 3; i++ {
			args = append(args, tsQuery)
		}
	}

	if len(conditions) > 0 {
		return db.Where(strings.Join(conditions, " OR "), args...)
	}

	return db
}

func (r *projectRepository) GetProjectByID(projectID uuid.UUID) (*models.Project, error) {
	var p models.Project
	res := r.db.Session(&gorm.Session{NewDB: true}).
		Model(&models.Project{}).
		Where("id = ? AND deleted = FALSE", projectID).
		First(&p)
	if res.Error != nil {
		if res.Error == gorm.ErrRecordNotFound {
			return nil, errors.New("project not found")
		}
		return nil, res.Error
	}

	return &p, nil
}

func (r *projectRepository) GetProjectsByUser(userID uuid.UUID) ([]models.Project, error) {
	var projects []models.Project
	res := r.db.Session(&gorm.Session{NewDB: true}).
		Model(&models.Project{}).
		Select("projects.*").
		Joins("JOIN project_teams pt ON pt.project_id = projects.id").
		Joins("JOIN teams t ON t.id = pt.team_id").
		Joins("JOIN team_members tm ON tm.team_id = t.id").
		Where(`
			tm.user_id = ? 
			AND projects.deleted = FALSE 
			AND tm.deleted = FALSE 
			AND pt.deleted = FALSE 
			AND t.deleted = FALSE
		`, userID).
		Find(&projects)
	if res.Error != nil {
		return nil, res.Error
	}

	return projects, nil
}

func (r *projectRepository) GetTeamProjects(teamID uuid.UUID) ([]models.ProjectTeam, error) {
	var pts []models.ProjectTeam
	if err := r.db.Session(&gorm.Session{NewDB: true}).
		Model(&models.ProjectTeam{}).
		Where("team_id = ? AND deleted = FALSE", teamID).
		Find(&pts).Error; err != nil {
		return nil, err
	}

	return pts, nil
}

func (r *projectRepository) GetProjectsByIDs(projectIDs []uuid.UUID) ([]models.Project, error) {
	var projects []models.Project
	if err := r.db.Session(&gorm.Session{NewDB: true}).
		Model(&models.Project{}).
		Where("id IN ? AND deleted = FALSE", projectIDs).
		Find(&projects).Error; err != nil {
		return nil, err
	}

	return projects, nil
}

func (r *projectRepository) CreateProjectWithBoardAndStatuses(project models.Project, board models.Board, statuses []models.Status) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Session(&gorm.Session{NewDB: true}).
			Model(&models.Project{}).
			Omit(clause.Associations).
			Create(&project).Error; err != nil {
			return err
		}

		if err := tx.Session(&gorm.Session{NewDB: true}).
			Model(&models.Board{}).
			Create(&board).Error; err != nil {
			return err
		}

		if err := tx.Session(&gorm.Session{NewDB: true}).
			Model(&models.Status{}).
			Create(&statuses).Error; err != nil {
			return err
		}

		return nil
	})
}

func (r *projectRepository) UpdateProject(projectID uuid.UUID, updateData map[string]interface{}) (bool, error) {
	var affected int64
	err := r.db.Transaction(func(tx *gorm.DB) error {
		res := tx.Session(&gorm.Session{NewDB: true}).
			Model(&models.Project{}).
			Where("id = ? AND deleted = FALSE", projectID).
			Updates(updateData)
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

func (r *projectRepository) DeleteProject(projectID uuid.UUID) (bool, error) {
	deleted := true
	now := time.Now()
	updateData := map[string]interface{}{
		"deleted":    &deleted,
		"updated_at": now,
	}

	var affected int64
	err := r.db.Transaction(func(tx *gorm.DB) error {
		// 1. Проверяем, существует ли неудалённый проект
		var count int64
		if err := tx.Session(&gorm.Session{NewDB: true}).
			Model(&models.Project{}).
			Where("id = ? AND deleted = ?", projectID, false).
			Count(&count).Error; err != nil {
			return err
		}
		if count == 0 {
			return errors.New("project not found or already deleted")
		}

		// 2. Получаем ID всех досок проекта
		var boardIDs []uuid.UUID
		if err := tx.Session(&gorm.Session{NewDB: true}).
			Model(&models.Board{}).
			Where("project_id = ? AND deleted = ?", projectID, false).
			Pluck("id", &boardIDs).Error; err != nil {
			return err
		}

		// 3. Получаем ID всех статусов этих досок
		var statusIDs []uuid.UUID
		if len(boardIDs) > 0 {
			if err := tx.Session(&gorm.Session{NewDB: true}).
				Model(&models.Status{}).
				Where("board_id IN ?", boardIDs).
				Pluck("id", &statusIDs).Error; err != nil {
				return err
			}
		}

		// 4. Удаляем задачи (привязаны к статусам)
		if len(statusIDs) > 0 {
			if err := tx.Session(&gorm.Session{NewDB: true}).
				Model(&models.Task{}).
				Where("status_id IN ?", statusIDs).
				Updates(updateData).Error; err != nil {
				return err
			}
		}

		// 5. Удаляем статусы
		if len(boardIDs) > 0 {
			if err := tx.Session(&gorm.Session{NewDB: true}).
				Model(&models.Status{}).
				Where("board_id IN ?", boardIDs).
				Updates(updateData).Error; err != nil {
				return err
			}
		}

		// 6. Удаляем доски
		if err := tx.Session(&gorm.Session{NewDB: true}).
			Model(&models.Board{}).
			Where("project_id = ?", projectID).
			Updates(updateData).Error; err != nil {
			return err
		}

		// 7. Удаляем связи с командами
		if err := tx.Session(&gorm.Session{NewDB: true}).
			Model(&models.ProjectTeam{}).
			Where("project_id = ?", projectID).
			Updates(updateData).Error; err != nil {
			return err
		}

		// 8. Удаляем сам проект
		if err := tx.Session(&gorm.Session{NewDB: true}).
			Model(&models.Project{}).
			Where("id = ?", projectID).
			Updates(updateData).Error; err != nil {
			return err
		}

		affected = count
		return nil
	})

	if err != nil {
		return false, err
	}

	return affected > 0, nil
}
