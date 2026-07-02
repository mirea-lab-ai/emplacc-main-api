package postgres

import (
	models "emplacc-api/internal/domain"
	"emplacc-api/internal/ports"
	"emplacc-api/internal/utils"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type taskRepository struct {
	db *gorm.DB
}

func NewTaskRepository(db *gorm.DB) ports.TaskRepository {
	return &taskRepository{
		db: db,
	}
}

func (r *taskRepository) GetAllTasks(limit, offset int) ([]models.Task, int64, error) {
	var totalCount int64
	if err := r.db.Session(&gorm.Session{NewDB: true}).
		Table("tasks").
		Where("tasks.deleted = ?", false).
		Count(&totalCount).Error; err != nil {
		return nil, 0, err
	}

	var tasks []models.Task
	if err := r.db.Session(&gorm.Session{NewDB: true}).
		Table("tasks").
		Where("tasks.deleted = ?", false).
		Limit(limit).
		Offset(offset).
		Find(&tasks).Error; err != nil {
		return nil, 0, err
	}

	return tasks, totalCount, nil
}

func (r *taskRepository) SearchTasks(query, userID string, limit, offset int) ([]models.Task, int64, error) {
	if query == "" {
		return []models.Task{}, 0, nil
	}

	var totalCount int64
	var tasks []models.Task

	// Подготавливаем поисковые запросы
	searchQueries := prepareSearchQueries(query)
	if len(searchQueries) == 0 {
		return []models.Task{}, 0, nil
	}

	// Оптимизированная сортировка
	orderClause := `
        CASE 
            WHEN tasks.assigned_to = '` + userID + `' THEN 1
            WHEN tasks.created_by = '` + userID + `' THEN 2
            ELSE 3
        END ASC, 
        tasks.created_at DESC
    `

	buildBase := func() *gorm.DB {
		q := r.db.Session(&gorm.Session{NewDB: true}).Table("tasks").
			Joins("LEFT JOIN statuses ON tasks.status_id = statuses.id AND statuses.deleted = ?", false).
			Joins("LEFT JOIN boards ON statuses.board_id = boards.id AND boards.deleted = ?", false).
			Joins("LEFT JOIN projects ON boards.project_id = projects.id AND projects.deleted = ?", false).
			Joins("LEFT JOIN users assigned_user ON tasks.assigned_to = assigned_user.id AND assigned_user.deleted = ?", false).
			Where("tasks.deleted = ?", false)
		return addFullTextConditions(q, searchQueries)
	}

	if err := buildBase().Count(&totalCount).Error; err != nil {
		return nil, 0, err
	}

	if totalCount == 0 {
		return []models.Task{}, 0, nil
	}

	err := buildBase().
		Preload("Status", func(db *gorm.DB) *gorm.DB {
			return db.Session(&gorm.Session{}).
				Select("id, name, color, key, board_id").
				Where("deleted = ?", false)
		}).
		Preload("Status.Board", func(db *gorm.DB) *gorm.DB {
			return db.Session(&gorm.Session{}).
				Select("id, name, project_id").
				Where("deleted = ?", false)
		}).
		Preload("Status.Board.Project", func(db *gorm.DB) *gorm.DB {
			return db.Session(&gorm.Session{}).
				Select("id, name, description").
				Where("deleted = ?", false)
		}).
		Preload("CreatedByUser", func(db *gorm.DB) *gorm.DB {
			return db.Session(&gorm.Session{}).
				Select("id, first_name, last_name, email").
				Where("deleted = ?", false)
		}).
		Preload("AssignedToUser", func(db *gorm.DB) *gorm.DB {
			return db.Session(&gorm.Session{}).
				Select("id, first_name, last_name, email").
				Where("deleted = ?", false)
		}).
		Limit(limit).
		Offset(offset).
		Order(orderClause).
		Find(&tasks).Error

	return tasks, totalCount, err
}

func prepareSearchQueries(query string) []string {
	variants := utils.PrepareSearchVariants(query)
	searchQueries := make([]string, 0, len(variants))

	for _, variant := range variants {
		if tsQuery := utils.PrepareTSQuery(variant); tsQuery != "" {
			searchQueries = append(searchQueries, tsQuery)
		}
	}

	return searchQueries
}

func addFullTextConditions(db *gorm.DB, searchQueries []string) *gorm.DB {
	if len(searchQueries) == 0 {
		return db
	}

	var conditions []string
	var args []interface{}

	for _, tsQuery := range searchQueries {
		condition := `(
            to_tsvector('russian', tasks.name) @@ to_tsquery(?) OR 
            to_tsvector('russian', tasks.description) @@ to_tsquery(?) OR 
            to_tsvector('russian', projects.name) @@ to_tsquery(?) OR 
            to_tsvector('russian', assigned_user.first_name || ' ' || assigned_user.last_name) @@ to_tsquery(?)
        )`

		conditions = append(conditions, condition)
		for i := 0; i < 4; i++ {
			args = append(args, tsQuery)
		}
	}

	if len(conditions) > 0 {
		return db.Where(strings.Join(conditions, " OR "), args...)
	}

	return db
}

func (r *taskRepository) GetTaskByID(taskID uuid.UUID) (*models.Task, error) {
	var task models.Task
	if err := r.db.Session(&gorm.Session{NewDB: true}).
		Where("id = ? AND deleted = ?", taskID, false).
		Preload("Status", func(db *gorm.DB) *gorm.DB {
			return db.Session(&gorm.Session{}).Where("deleted = ?", false)
		}).
		Preload("CreatedByUser", func(db *gorm.DB) *gorm.DB {
			return db.Session(&gorm.Session{}).Where("deleted = ?", false)
		}).
		Preload("AssignedToUser", func(db *gorm.DB) *gorm.DB {
			return db.Session(&gorm.Session{}).Where("deleted = ?", false)
		}).
		First(&task).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, errors.New("task not found")
		}
		return nil, err
	}

	return &task, nil
}

func (r *taskRepository) GetTaskBoardAndProjectIDs(taskID uuid.UUID) (boardID uuid.UUID, projectID uuid.UUID, err error) {
	// 1. Получаем задачу
	var task models.Task
	if err := r.db.Session(&gorm.Session{NewDB: true}).
		Where("id = ? AND deleted = ?", taskID, false).
		First(&task).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return uuid.Nil, uuid.Nil, errors.New("task not found")
		}
		return uuid.Nil, uuid.Nil, err
	}

	// 2. Получаем статус (только board_id)
	var status models.Status
	if err := r.db.Session(&gorm.Session{NewDB: true}).
		Select("board_id").
		Where("id = ? AND deleted = ?", task.StatusID, false).
		First(&status).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return uuid.Nil, uuid.Nil, errors.New("status not found")
		}
		return uuid.Nil, uuid.Nil, err
	}

	// 3. Получаем доску (только project_id)
	var board models.Board
	if err := r.db.Session(&gorm.Session{NewDB: true}).
		Select("project_id").
		Where("id = ? AND deleted = ?", status.BoardID, false).
		First(&board).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return uuid.Nil, uuid.Nil, errors.New("board not found")
		}
		return uuid.Nil, uuid.Nil, err
	}

	return status.BoardID, board.ProjectID, nil
}

func (r *taskRepository) GetTasksByProjectID(projectID uuid.UUID, limit, offset int) ([]models.Task, int64, error) {
	// Подзапрос: все status_id, принадлежащие доскам этого проекта
	var statusIDs []uuid.UUID
	if err := r.db.Session(&gorm.Session{NewDB: true}).
		Model(&models.Status{}).
		Select("statuses.id").
		Joins("JOIN boards ON statuses.board_id = boards.id").
		Where("boards.project_id = ? AND boards.deleted = ? AND statuses.deleted = ?", projectID, false, false).
		Scan(&statusIDs).Error; err != nil {
		return nil, 0, err
	}

	var totalCount int64
	if len(statusIDs) == 0 {
		totalCount = 0
	} else {
		if err := r.db.Session(&gorm.Session{NewDB: true}).
			Model(&models.Task{}).
			Where("tasks.status_id IN ? AND tasks.deleted = ?", statusIDs, false).
			Count(&totalCount).Error; err != nil {
			return nil, 0, err
		}
	}

	var tasks []models.Task
	if len(statusIDs) > 0 {
		if err := r.db.Session(&gorm.Session{NewDB: true}).
			Where("tasks.status_id IN ? AND tasks.deleted = ?", statusIDs, false).
			Limit(limit).
			Offset(offset).
			Find(&tasks).Error; err != nil {
			return nil, 0, err
		}
	}

	return tasks, totalCount, nil
}

func (r *taskRepository) CreateTask(task models.Task) error {
	return r.db.Session(&gorm.Session{FullSaveAssociations: false}).
		Omit(clause.Associations).
		Create(&task).Error
}

func (r *taskRepository) UpdateTask(taskID uuid.UUID, updates map[string]interface{}) (bool, error) {
	var affected int64
	err := r.db.Transaction(func(tx *gorm.DB) error {
		res := tx.Session(&gorm.Session{NewDB: true}).Table("tasks").Where("id = ? AND deleted = FALSE", taskID).Updates(updates)
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

func (r *taskRepository) DeleteTask(taskID uuid.UUID) (bool, error) {
	updateData := map[string]interface{}{"deleted": true}

	var affected int64
	err := r.db.Transaction(func(tx *gorm.DB) error {
		res := tx.Session(&gorm.Session{NewDB: true}).Table("tasks").Where("id = ?", taskID).Updates(updateData)
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

func (r *taskRepository) GetStatusByID(statusID uuid.UUID) (*models.Status, error) {
	var status models.Status
	if err := r.db.Session(&gorm.Session{NewDB: true}).Table("statuses").
		Select("board_id").
		Where("id = ? AND deleted = ?", statusID, false).
		First(&status).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, errors.New("status not found")
		}
		return nil, err
	}

	return &status, nil
}

func (r *taskRepository) GetStatusesByBoardID(boardID uuid.UUID) ([]models.Status, error) {
	var statuses []models.Status
	if err := r.db.Session(&gorm.Session{NewDB: true}).
		Model(&models.Status{}).
		Where("board_id = ? AND deleted = ?", boardID, false).
		Order("sort_order ASC").
		Preload("Tasks", func(db *gorm.DB) *gorm.DB {
			return db.Session(&gorm.Session{}).Where("deleted = ?", false)
		}).
		Find(&statuses).Error; err != nil {
		return nil, err
	}

	return statuses, nil
}

func (r *taskRepository) UpdateTaskStatus(taskID uuid.UUID, toStatusID uuid.UUID, updatedAt time.Time) error {
	return r.db.Session(&gorm.Session{NewDB: true}).Table("tasks").
		Where("id = ?", taskID).
		Updates(map[string]interface{}{
			"status_id":  toStatusID,
			"updated_at": updatedAt,
		}).Error
}

func (r *taskRepository) UserExists(userID uuid.UUID) (bool, error) {
	var count int64
	err := r.db.Session(&gorm.Session{NewDB: true}).Table("users").
		Where("id = ? AND deleted = ?", userID, false).
		Count(&count).Error
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

func (r *taskRepository) StatusExists(statusID uuid.UUID) (bool, error) {
	var count int64
	if err := r.db.Session(&gorm.Session{NewDB: true}).Table("statuses").
		Joins("INNER JOIN boards ON statuses.board_id = boards.id").
		Where("statuses.id = ? AND statuses.deleted = ? AND boards.deleted = ?", statusID, false, false).
		Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}

func (r *taskRepository) GetTasksByUserIDAndProjectID(userID, projectID uuid.UUID, limit, offset int) ([]models.Task, int64, error) {
	var totalCount int64

	if err := r.db.Session(&gorm.Session{NewDB: true}).
		Model(&models.Task{}).
		Joins("JOIN statuses ON tasks.status_id = statuses.id").
		Joins("JOIN boards ON statuses.board_id = boards.id").
		Where("tasks.assigned_to = ? AND tasks.deleted = ?", userID, false).
		Where("boards.project_id = ? AND boards.deleted = ?", projectID, false).
		Where("statuses.deleted = ?", false).
		Count(&totalCount).Error; err != nil {
		return nil, 0, err
	}

	if totalCount == 0 {
		return []models.Task{}, 0, nil
	}

	var tasks []models.Task
	if err := r.db.Session(&gorm.Session{NewDB: true}).
		Where("tasks.assigned_to = ? AND tasks.deleted = ?", userID, false).
		Joins("JOIN statuses ON tasks.status_id = statuses.id").
		Joins("JOIN boards ON statuses.board_id = boards.id").
		Where("boards.project_id = ? AND boards.deleted = ?", projectID, false).
		Where("statuses.deleted = ?", false).
		Limit(limit).
		Offset(offset).
		Preload("Status", func(db *gorm.DB) *gorm.DB {
			return db.Session(&gorm.Session{}).
				Where("deleted = ?", false). // ← ИСПРАВЛЕНО
				Preload("Board", func(d *gorm.DB) *gorm.DB {
					return d.Session(&gorm.Session{NewDB: true}).
						Select("id, name, project_id").
						Where("deleted = ?", false)
				})
		}).
		Preload("CreatedByUser", func(db *gorm.DB) *gorm.DB {
			return db.Session(&gorm.Session{}).
				Select("id, first_name, last_name, email").
				Where("deleted = ?", false)
		}).
		Preload("AssignedToUser", func(db *gorm.DB) *gorm.DB {
			return db.Session(&gorm.Session{}).
				Select("id, first_name, last_name, email").
				Where("deleted = ?", false)
		}).
		Find(&tasks).Error; err != nil {
		return nil, 0, err
	}

	return tasks, totalCount, nil
}

func (r *taskRepository) GetActiveTasksByUserId(userID uuid.UUID, limit, offset int) ([]models.Task, int64, error) {
	var totalCount int64

	// Для подсчета используем подзапрос
	if err := r.db.Session(&gorm.Session{NewDB: true}).
		Model(&models.Task{}).
		Where("tasks.assigned_to = ? AND tasks.deleted = ?", userID, false).
		Where("tasks.status_id NOT IN (SELECT id FROM statuses WHERE name = ? AND deleted = ?)", "Done", false).
		Count(&totalCount).Error; err != nil {
		return nil, 0, err
	}

	if totalCount == 0 {
		return []models.Task{}, 0, nil
	}

	var tasks []models.Task
	// В основном запросе используем Preload с условием
	if err := r.db.Session(&gorm.Session{NewDB: true}).
		Where("tasks.assigned_to = ? AND tasks.deleted = ?", userID, false).
		Where("tasks.status_id NOT IN (SELECT id FROM statuses WHERE name = ? AND deleted = ?)", "Done", false).
		Limit(limit).
		Offset(offset).
		Preload("Status").
		Preload("Status.Board").
		Preload("Status.Board.Project"). // нужен для projectName на странице «Мои задачи»
		Preload("CreatedByUser", func(db *gorm.DB) *gorm.DB {
			return db.Session(&gorm.Session{}).
				Select("id, first_name, last_name, email").
				Where("deleted = ?", false)
		}).
		Preload("AssignedToUser", func(db *gorm.DB) *gorm.DB {
			return db.Session(&gorm.Session{}).
				Select("id, first_name, last_name, email").
				Where("deleted = ?", false)
		}).
		Find(&tasks).Error; err != nil {
		return nil, 0, err
	}

	return tasks, totalCount, nil
}

func (r *taskRepository) GetTasksByUserId(userID uuid.UUID, limit, offset int) ([]models.Task, int64, error) {
	var totalCount int64
	if err := r.db.Session(&gorm.Session{NewDB: true}).
		Model(&models.Task{}).
		Where("tasks.assigned_to = ? AND tasks.deleted = ?", userID, false).
		Count(&totalCount).Error; err != nil {
		return nil, 0, err
	}

	if totalCount == 0 {
		return []models.Task{}, 0, nil
	}

	var tasks []models.Task
	if err := r.db.Session(&gorm.Session{NewDB: true}).
		Where("tasks.assigned_to = ? AND tasks.deleted = ?", userID, false).
		Limit(limit).
		Offset(offset).
		Preload("Status", func(db *gorm.DB) *gorm.DB {
			return db.Session(&gorm.Session{}).
				Where("deleted = ?", false).
				Preload("Board", func(d *gorm.DB) *gorm.DB {
					return d.Session(&gorm.Session{NewDB: true}).
						Select("id, name, project_id").
						Where("deleted = ?", false)
				})
		}).
		Preload("CreatedByUser", func(db *gorm.DB) *gorm.DB {
			return db.Session(&gorm.Session{}).
				Select("id, first_name, last_name, email").
				Where("deleted = ?", false)
		}).
		Preload("AssignedToUser", func(db *gorm.DB) *gorm.DB {
			return db.Session(&gorm.Session{}).
				Select("id, first_name, last_name, email").
				Where("deleted = ?", false)
		}).
		Find(&tasks).Error; err != nil {
		return nil, 0, err
	}

	return tasks, totalCount, nil
}

func (r *taskRepository) GetAllActiveTasks() ([]models.Task, error) {
	var tasks []models.Task
	if err := r.db.Session(&gorm.Session{NewDB: true}).
		Where("tasks.deleted = ?", false).
		Where("tasks.status_id NOT IN (SELECT id FROM statuses WHERE name = ? AND deleted = ?)", "Done", false).
		Preload("Status", func(db *gorm.DB) *gorm.DB {
			return db.Session(&gorm.Session{}).
				Where("deleted = ? AND name != ?", false, "Done")
		}).
		Preload("Status.Board", func(d *gorm.DB) *gorm.DB {
			return d.Session(&gorm.Session{NewDB: true}).
				Select("id, name, project_id").
				Where("deleted = ?", false)
		}).
		Preload("Status.Board.Project", func(db *gorm.DB) *gorm.DB {
			return db.Session(&gorm.Session{}).
				Select("id, name").
				Where("deleted = ?", false)
		}).
		Preload("CreatedByUser", func(db *gorm.DB) *gorm.DB {
			return db.Session(&gorm.Session{}).
				Select("id, first_name, last_name, email").
				Where("deleted = ?", false)
		}).
		Preload("AssignedToUser", func(db *gorm.DB) *gorm.DB {
			return db.Session(&gorm.Session{}).
				Select("id, first_name, last_name, email").
				Where("deleted = ?", false)
		}).
		Find(&tasks).Error; err != nil {
		return nil, err
	}

	return tasks, nil
}
