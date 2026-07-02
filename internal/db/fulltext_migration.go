package db

import (
	"log"
	"strings"

	"gorm.io/gorm"
)

func SetupFullTextSearch(db *gorm.DB) error {
	log.Println("Setting up Full-Text Search extensions and indexes...")

	// Создаем расширения
	if err := db.Exec("CREATE EXTENSION IF NOT EXISTS unaccent").Error; err != nil {
		log.Printf("Warning: Failed to create unaccent extension: %v", err)
	}

	if err := db.Exec("CREATE EXTENSION IF NOT EXISTS pg_trgm").Error; err != nil {
		log.Printf("Warning: Failed to create pg_trgm extension: %v", err)
	}

	// Индексы с правильными именами таблиц (как GORM их создает)
	indexes := []string{
		// === Users ===
		"CREATE INDEX IF NOT EXISTS idx_users_first_name_fts ON users USING gin(to_tsvector('russian', first_name))",
		"CREATE INDEX IF NOT EXISTS idx_users_last_name_fts ON users USING gin(to_tsvector('russian', last_name))",
		"CREATE INDEX IF NOT EXISTS idx_users_full_name_fts ON users USING gin(to_tsvector('russian', first_name || ' ' || last_name))",
		"CREATE INDEX IF NOT EXISTS idx_users_email_fts ON users USING gin(to_tsvector('russian', email))",
		"CREATE INDEX IF NOT EXISTS idx_users_profession_fts ON users USING gin(to_tsvector('russian', profession))",

		// === Tasks ===
		"CREATE INDEX IF NOT EXISTS idx_tasks_name_fts ON tasks USING gin(to_tsvector('russian', name))",
		"CREATE INDEX IF NOT EXISTS idx_tasks_description_fts ON tasks USING gin(to_tsvector('russian', description))",

		// === Projects ===
		"CREATE INDEX IF NOT EXISTS idx_projects_name_fts ON projects USING gin(to_tsvector('russian', name))",
		"CREATE INDEX IF NOT EXISTS idx_projects_description_fts ON projects USING gin(to_tsvector('russian', description))",

		// === Problems ===
		"CREATE INDEX IF NOT EXISTS idx_problems_name_fts ON problems USING gin(to_tsvector('russian', name))",

		// === Forum Messages ===
		"CREATE INDEX IF NOT EXISTS idx_forum_messages_description_fts ON forum_messages USING gin(to_tsvector('russian', description[1]))",

		// === Help Requests ===
		"CREATE INDEX IF NOT EXISTS idx_help_requests_description_fts ON help_requests USING gin(to_tsvector('russian', description))",

		// === Completed Work === (GORM создает таблицу как completed_works)
		"CREATE INDEX IF NOT EXISTS idx_completed_works_description_fts ON completed_works USING gin(to_tsvector('russian', description))",

		// === Tomorrow Plans === (GORM создает таблицу как tomorrow_plans)
		"CREATE INDEX IF NOT EXISTS idx_tomorrow_plans_description_fts ON tomorrow_plans USING gin(to_tsvector('russian', description))",

		// === Teams ===
		"CREATE INDEX IF NOT EXISTS idx_teams_name_fts ON teams USING gin(to_tsvector('russian', name))",
		"CREATE INDEX IF NOT EXISTS idx_teams_description_fts ON teams USING gin(to_tsvector('russian', description))",

		// === Оптимизационные индексы для быстрого поиска членства в командах ===

		// Для team_members - ускорение поиска по user_id
		"CREATE INDEX IF NOT EXISTS idx_team_members_user_id ON team_members(user_id)",

		// Для team_members - ускорение поиска по team_id
		"CREATE INDEX IF NOT EXISTS idx_team_members_team_id ON team_members(team_id)",

		// Составной индекс для частого запроса team_id + user_id
		"CREATE INDEX IF NOT EXISTS idx_team_members_team_user ON team_members(team_id, user_id)",

		// Индекс для deleted флага
		"CREATE INDEX IF NOT EXISTS idx_team_members_deleted ON team_members(deleted) WHERE deleted = false",

		// Для project_teams - ускорение поиска по project_id
		"CREATE INDEX IF NOT EXISTS idx_project_teams_project_id ON project_teams(project_id)",

		// Для project_teams - ускорение поиска по team_id
		"CREATE INDEX IF NOT EXISTS idx_project_teams_team_id ON project_teams(team_id)",

		// Составной индекс для частого запроса project_id + team_id
		"CREATE INDEX IF NOT EXISTS idx_project_teams_project_team ON project_teams(project_id, team_id)",

		// Индекс для deleted флага
		"CREATE INDEX IF NOT EXISTS idx_project_teams_deleted ON project_teams(deleted) WHERE deleted = false",

		// Для teams - индекс deleted флага
		"CREATE INDEX IF NOT EXISTS idx_teams_deleted ON teams(deleted) WHERE deleted = false",

		// Для projects - индекс created_by (для быстрого поиска созданных проектов)
		"CREATE INDEX IF NOT EXISTS idx_projects_created_by ON projects(created_by)",

		// Для projects - индекс deleted флага
		"CREATE INDEX IF NOT EXISTS idx_projects_deleted ON projects(deleted) WHERE deleted = false",

		// === Оптимизационные индексы для задач ===

		// Для быстрого поиска по assigned_to и created_by
		"CREATE INDEX IF NOT EXISTS idx_tasks_assigned_to ON tasks(assigned_to)",
		"CREATE INDEX IF NOT EXISTS idx_tasks_created_by ON tasks(created_by)",

		// Составные индексы для частых запросов
		"CREATE INDEX IF NOT EXISTS idx_tasks_assigned_created ON tasks(assigned_to, created_by)",

		// Индекс для deleted флага
		"CREATE INDEX IF NOT EXISTS idx_tasks_deleted ON tasks(deleted) WHERE deleted = false",

		// Индекс для статуса (часто используется в JOIN)
		"CREATE INDEX IF NOT EXISTS idx_tasks_status_id ON tasks(status_id)",

		// Индекс для даты создания (сортировка)
		"CREATE INDEX IF NOT EXISTS idx_tasks_created_at ON tasks(created_at DESC)",

		// Составной индекс для частого запроса статус + deleted
		"CREATE INDEX IF NOT EXISTS idx_tasks_status_deleted ON tasks(status_id, deleted) WHERE deleted = false",

		// === Индексы для связанных таблиц задач ===

		// Для statuses
		"CREATE INDEX IF NOT EXISTS idx_statuses_deleted ON statuses(deleted) WHERE deleted = false",
		"CREATE INDEX IF NOT EXISTS idx_statuses_board_id ON statuses(board_id)",
		"CREATE INDEX IF NOT EXISTS idx_statuses_board_deleted ON statuses(board_id, deleted) WHERE deleted = false",

		// Для boards
		"CREATE INDEX IF NOT EXISTS idx_boards_deleted ON boards(deleted) WHERE deleted = false",
		"CREATE INDEX IF NOT EXISTS idx_boards_project_id ON boards(project_id)",
		"CREATE INDEX IF NOT EXISTS idx_boards_project_deleted ON boards(project_id, deleted) WHERE deleted = false",

		// Для projects - дополнительные индексы
		"CREATE INDEX IF NOT EXISTS idx_projects_name_trgm ON projects USING gin(name gin_trgm_ops)",
		"CREATE INDEX IF NOT EXISTS idx_projects_description_trgm ON projects USING gin(description gin_trgm_ops)",

		// === Индексы для пользователей ===

		// Для users - дополнительные индексы
		"CREATE INDEX IF NOT EXISTS idx_users_first_name_trgm ON users USING gin(first_name gin_trgm_ops)",
		"CREATE INDEX IF NOT EXISTS idx_users_last_name_trgm ON users USING gin(last_name gin_trgm_ops)",
		"CREATE INDEX IF NOT EXISTS idx_users_email_trgm ON users USING gin(email gin_trgm_ops)",
		"CREATE INDEX IF NOT EXISTS idx_users_profession_trgm ON users USING gin(profession gin_trgm_ops)",

		// Составные индексы для пользователей
		"CREATE INDEX IF NOT EXISTS idx_users_first_last_name ON users(first_name, last_name)",
		"CREATE INDEX IF NOT EXISTS idx_users_deleted_active ON users(deleted, is_active) WHERE deleted = false AND is_active = true",

		// === Индексы для внешних ключей ===

		// Для task foreign keys
		"CREATE INDEX IF NOT EXISTS idx_tasks_status_id_deleted ON tasks(status_id, deleted) WHERE deleted = false",
		"CREATE INDEX IF NOT EXISTS idx_tasks_assigned_to_deleted ON tasks(assigned_to, deleted) WHERE deleted = false",
		"CREATE INDEX IF NOT EXISTS idx_tasks_created_by_deleted ON tasks(created_by, deleted) WHERE deleted = false",

		// Для status foreign keys
		"CREATE INDEX IF NOT EXISTS idx_statuses_board_id_deleted ON statuses(board_id, deleted) WHERE deleted = false",

		// Для board foreign keys
		"CREATE INDEX IF NOT EXISTS idx_boards_project_id_deleted ON boards(project_id, deleted) WHERE deleted = false",

		// === Индексы для полнотекстового поиска по связанным таблицам ===

		// Для поиска по именам пользователей в задачах
		"CREATE INDEX IF NOT EXISTS idx_users_full_name_composite_fts ON users USING gin(to_tsvector('russian', coalesce(first_name, '') || ' ' || coalesce(last_name, '')))",

		// Для поиска по проектам в задачах
		"CREATE INDEX IF NOT EXISTS idx_projects_name_description_fts ON projects USING gin(to_tsvector('russian', coalesce(name, '') || ' ' || coalesce(description, '')))",

		// === Индексы для сортировки ===

		// Для сортировки по имени проектов
		"CREATE INDEX IF NOT EXISTS idx_projects_name_lower ON projects(LOWER(name))",

		// Для сортировки по имени пользователей
		"CREATE INDEX IF NOT EXISTS idx_users_first_name_lower ON users(LOWER(first_name))",
		"CREATE INDEX IF NOT EXISTS idx_users_last_name_lower ON users(LOWER(last_name))",

		// Для сортировки задач по приоритету
		"CREATE INDEX IF NOT EXISTS idx_tasks_priority_created ON tasks(priority, created_at) WHERE deleted = false",

		// === Индексы для JOIN оптимизации ===

		// Составные индексы для часто используемых JOIN путей
		"CREATE INDEX IF NOT EXISTS idx_tasks_status_assigned_created ON tasks(status_id, assigned_to, created_at) WHERE deleted = false",
		"CREATE INDEX IF NOT EXISTS idx_statuses_board_deleted_order ON statuses(board_id, deleted, sort_order) WHERE deleted = false",
		"CREATE INDEX IF NOT EXISTS idx_boards_project_deleted_name ON boards(project_id, deleted, name) WHERE deleted = false",
	}

	successCount := 0
	for _, indexSQL := range indexes {
		if err := db.Exec(indexSQL).Error; err != nil {
			log.Printf("Warning: Failed to create index %s: %v", getIndexName(indexSQL), err)
		} else {
			successCount++
		}
	}

	log.Printf("Full-Text Search setup completed: %d/%d indexes created", successCount, len(indexes))
	return nil
}

func getIndexName(sql string) string {
	start := strings.Index(sql, "idx_")
	if start == -1 {
		return "unknown"
	}
	end := strings.Index(sql[start:], " ")
	if end == -1 {
		return sql[start:]
	}
	return sql[start : start+end]
}
