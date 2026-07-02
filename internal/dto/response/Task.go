package response

import (
	"time"
)

// Полная информация о задаче (для GetTaskByID)
type GetTaskByIDResponse struct {
	ID            string    `json:"id"`
	StatusID      string    `json:"status_id"`
	Name          string    `json:"name"`
	Description   string    `json:"description"`
	Priority      int16     `json:"priority"`
	CreatedBy     UserShort `json:"created_by"`            // Связь с users
	AssignedTo    UserShort `json:"assigned_to,omitempty"` // Связь с users
	Deadline      time.Time `json:"deadline,omitempty"`    // DATE в БД
	TimeSpent     string    `json:"time_spent"`            // INTERVAL в БД
	StartDate     time.Time `json:"start_date"`            // DATE в БД
	GitlabIssueID int       `json:"gitlab_issue_id,omitempty"`
	Category      int8      `json:"community"`
	UpdatedAt     time.Time `json:"updated_at"`
	CreatedAt     time.Time `json:"created_at"`
}

// Краткая информация о задаче (для списков)
type TaskShort struct {
	ID        string    `json:"id"`
	StatusID  string    `json:"status_id"`
	Name      string    `json:"name"`
	Priority  int16     `json:"priority"`
	StartDate time.Time `json:"start_date"`
	Deadline  time.Time `json:"deadline,omitempty"`
	UpdatedAt time.Time `json:"updated_at"`
	CreatedAt time.Time `json:"created_at"`
}

// Ответ для списка задач с пагинацией
type TaskListResponse struct {
	Tasks      []TaskShort `json:"tasks"`
	TotalCount int64       `json:"total_count"`
	Page       int         `json:"page,omitempty"`
	PageSize   int         `json:"page_size,omitempty"`
}

// Универсальный ответ для операций
type TaskUniversaResponse struct {
	ID      string `json:"id"`
	Message string `json:"message"`
}

// Вспомогательная структура для пользователя
type UserShort struct {
	ID        string `json:"id"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
	AvatarURL string `json:"avatar_url,omitempty"`
}

type UserProjectTasksResponse struct {
	User       UserFull     `json:"user"`
	Project    ProjectShort `json:"project"`
	Tasks      []TaskFull   `json:"tasks"`
	TotalCount int64        `json:"total_count"`
	Page       int          `json:"page,omitempty"`
	PageSize   int          `json:"page_size,omitempty"`
}

type UserFull struct {
	ID        string `json:"id"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
	Email     string `json:"email"`
}

type ProjectShort struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	GitlabURL   string `json:"gitlab_url"`
	CreatedBy   string `json:"created_by"`
}

type TaskFull struct {
	ID             string          `json:"id"`
	Name           string          `json:"name"`
	Description    string          `json:"description"`
	Priority       int16           `json:"priority"`
	Deadline       time.Time       `json:"deadline,omitempty"`
	StartDate      time.Time       `json:"start_date"`
	TimeSpent      string          `json:"time_spent"`
	GitlabIssueID  int             `json:"gitlab_issue_id,omitempty"`
	Category       int8            `json:"category"`
	Deleted        bool            `json:"deleted"`
	CreatedAt      time.Time       `json:"created_at"`
	UpdatedAt      time.Time       `json:"updated_at"`
	Status         StatusFull      `json:"status"`
	Project        TaskProjectInfo `json:"project,omitempty"`
	CreatedByUser  UserFull        `json:"created_by_user"`
	AssignedToUser UserFull        `json:"assigned_to_user"`
}

type UserTasksResponse struct {
	Tasks      []TaskFull `json:"tasks"`
	TotalCount int64      `json:"total_count"`
	Page       int        `json:"page,omitempty"`
	PageSize   int        `json:"page_size,omitempty"`
}

type StatusFull struct {
	ID     string   `json:"id"`
	Name   string   `json:"name"`
	Key    string   `json:"key"`
	Color  string   `json:"color"`
	IsOpen bool     `json:"is_open"`
	Board  BoardRef `json:"board"`
}

type BoardRef struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	ProjectID string `json:"project_id"`
}

type TaskBoardProjectResponse struct {
	TaskID    string `json:"task_id"`
	BoardID   string `json:"board_id"`
	ProjectID string `json:"project_id"`
}

type AllActiveTasksXLSXData struct {
	Users []UserTasksXLSX `json:"users"`
}

type UserTasksXLSX struct {
	UserID    string            `json:"user_id"`
	UserName  string            `json:"user_name"`
	UserEmail string            `json:"user_email"`
	Tasks     []TaskXLSXForTask `json:"tasks"`
}

type TaskXLSXForTask struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Priority    int16     `json:"priority"`
	StartDate   time.Time `json:"start_date"`
	Deadline    time.Time `json:"deadline"`
	StatusName  string    `json:"status_name"`
	BoardName   string    `json:"board_name"`
	ProjectName string    `json:"project_name"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type TaskSearchResponse struct {
	Query      string           `json:"query"`
	Page       int              `json:"page"`
	PageSize   int              `json:"pageSize"`
	TotalCount int64            `json:"totalCount"`
	Tasks      []TaskSearchItem `json:"tasks"`
}

type TaskSearchItem struct {
	ID          string          `json:"id"`
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Priority    int16           `json:"priority,omitempty"`
	StartDate   time.Time       `json:"start_date,omitempty"`
	Deadline    time.Time       `json:"deadline,omitempty"`
	CreatedAt   time.Time       `json:"created_at"`
	UpdatedAt   time.Time       `json:"updated_at"`
	Status      TaskStatusInfo  `json:"status,omitempty"`
	Project     TaskProjectInfo `json:"project,omitempty"`
	AssignedTo  UserShort       `json:"assigned_to"`
	CreatedBy   UserShort       `json:"created_by"`
}

type TaskStatusInfo struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Color string `json:"color,omitempty"`
	Key   string `json:"key,omitempty"`
}

type TaskProjectInfo struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}
