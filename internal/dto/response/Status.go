package response

import (
	"time"
)

type StatusResponse struct {
	ID        string      `json:"id"`
	Key       string      `json:"key"`
	Name      string      `json:"name"`
	Color     string      `json:"color"`
	Order     int         `json:"order"`
	IsDefault bool        `json:"is_default"`
	IsActive  bool        `json:"is_active"`
	IsOpen    bool        `json:"is_open"`
	CreatedAt time.Time   `json:"created_at"`
	UpdatedAt time.Time   `json:"updated_at"`
	Tasks     []TaskShort `json:"tasks"`
}

type StatusShort struct {
	ID        string    `json:"id"`
	Key       string    `json:"key"`
	Name      string    `json:"name"`
	Color     string    `json:"color"`
	Order     int       `json:"order"`
	IsDefault bool      `json:"is_default"`
	IsActive  bool      `json:"is_active"`
	IsOpen    bool      `json:"is_open"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type StatusListResponse struct {
	Statuses   []StatusShort `json:"statuses"`
	TotalCount int64         `json:"total_count"`
	Page       int           `json:"page"`
	PageSize   int           `json:"page_size"`
}

type StatusUniversalResponse struct {
	ID      string `json:"id"`
	Message string `json:"message"`
}

type StatusByBoardIdResponse struct {
	BoardId  string           `json:"board_id"`
	Statuses []StatusResponse `json:"statuses"`
}

type StatusByTaskIdResponse struct {
	TaskId   string           `json:"task_id"`
	Statuses []StatusResponse `json:"statuses"`
}

type AddStatusToTaskResponse struct {
	TaskId   string `json:"task_id"`
	StatusId string `json:"status_id"`
	Message  string `json:"message"`
}

type AddStatusToBoardResponse struct {
	BoardId  string `json:"board_id"`
	StatusId string `json:"status_id"`
	Message  string `json:"message"`
}
