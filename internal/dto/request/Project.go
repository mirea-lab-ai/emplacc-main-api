package request

type CreateProjectRequest struct {
	Name              *string `json:"name" validate:"required,min=3,max=100"`
	Description       *string `json:"description" validate:"max=500"`
	Gitlab_project_id *int    `json:"gitlab_project_id" validate:"omitempty"`
	Gitlab_url        *string `json:"gitlab_url" validate:"omitempty,max=255"`
	CreatedBy         *string `json:"created_by" validate:"required"`
	Status            *string `json:"status" validate:"max=50"`
}

type UpdateProjectRequest struct {
	Name            *string `json:"name" validate:"omitempty"`
	Description     *string `json:"description" validate:"omitempty"`
	GitlabProjectId *int    `json:"gitlab_project_id" validate:"omitempty"`
	GitlabUrl       *string `json:"gitlab_url" validate:"omitempty"`
	Status          *string `json:"status" validate:"omitempty"`
}

type ProjectListRequest struct {
	Page     int `json:"page"`
	PageSize int `json:"page_size"`
}
