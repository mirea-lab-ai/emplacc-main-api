package response

import "time"

type BoardUniversalResponse struct {
	Id      string `json:"id"`
	Message string `json:"message"`
}

type BoardResponse struct {
	Id          string           `json:"id"`
	ProjectId   string           `json:"project_id"`
	Name        string           `json:"name"`
	Description string           `json:"description"`
	UpdatedAt   time.Time        `json:"updated_at"`
	CreatedAt   time.Time        `json:"created_at"`
	Statuses    []StatusResponse `json:"statuses,omitempty"`
}

type BoardListResponse struct {
	Boards     []BoardResponse `json:"boards"`
	TotalCount int             `json:"total_count"`
	Page       int             `json:"page"`
	PageSize   int             `json:"page_size"`
}

type BoardForProjectResponse struct {
	Boards    []BoardResponse `json:"boards"`
	ProjectId string          `json:"project_id"`
}

type ProjectTasksXLSXData struct {
	ProjectName        string
	ProjectDescription string
	Boards             []BoardXLSXResponse
}

type BoardXLSXResponse struct {
	BoardName        string
	BoardDescription string
	Statuses         []StatusXLSXResponse
}

type StatusXLSXResponse struct {
	StatusName  string
	StatusColor string
	Tasks       []TaskXLSX
}

type TaskXLSX struct {
	ID          string
	Name        string
	Description string
	Priority    int16
	StartDate   time.Time
	Deadline    time.Time
	AssignedTo  string
	CreatedBy   string // Добавлено: кто создал задачу
	CreatedAt   time.Time
	UpdatedAt   time.Time
}
