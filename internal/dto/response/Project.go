package response

import (
	"time"
)

type ProjectResponse struct {
	ID              string    `json:"id"`
	Name            string    `json:"name"`
	Description     string    `json:"description"`
	GitlabProjectId int       `json:"gitlab_project_id"`
	GitlabUrl       string    `json:"gitlab_url"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
	CreatedBy       string    `json:"created_by"`
	Status          string    `json:"status"`
}

type ProjectListResponse struct {
	Projects   []ProjectResponse `json:"projects"`
	TotalCount int64             `json:"total_count"`
	Page       int               `json:"page"`
	PageSize   int               `json:"page_size"`
}

type ProjectByTeamResponse struct {
	Projects []ProjectResponse `json:"projects"`
}

type ProjectUniversalResponse struct {
	ID      string `json:"id"`
	Message string `json:"message"`
}

type ProjectSearchResponse struct {
	Query      string                     `json:"query"`
	Page       int                        `json:"page"`
	PageSize   int                        `json:"pageSize"`
	TotalCount int64                      `json:"totalCount"`
	Projects   []ProjectForSearchResponse `json:"projects"`
}

type ProjectForSearchResponse struct {
	ID              string    `json:"id"`
	Name            string    `json:"name"`
	Description     string    `json:"description"`
	GitlabProjectId int       `json:"gitlab_project_id"`
	GitlabUrl       string    `json:"gitlab_url"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
	CreatedBy       string    `json:"created_by"`
	Status          string    `json:"status"`
	CreatedByUser   UserShort `json:"created_by_user"`
}
